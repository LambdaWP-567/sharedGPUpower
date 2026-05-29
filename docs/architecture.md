# Architecture

Technical overview of sharedGPUpower for operators, contributors, and anyone who wants to understand how the system works.

---

## System Overview

```
┌──────────────────────────────────────────────────────────────────────┐
│                        User devices                                   │
│                                                                      │
│  [Browser Dashboard]  ─── REST/WS ──▶  ┌─────────────────────────┐ │
│                                         │       Backend (Go)       │ │
│  [Mac Agent]  ─── gRPC/mTLS ──────────▶│  - AgentService (gRPC)  │ │
│                                         │  - REST API (Chi)        │ │
│  [RPi Agent]  ─── gRPC/mTLS ──────────▶│  - WebSocket Hub         │ │
│                                         │  - Job Scheduler         │ │
│  [Local LLM]  ─── HTTP ───▶ [Proxy]    │  - Token Ledger          │ │
│                              └── REST ─▶│  - Auth Middleware        │ │
│                                         └──────────┬────────────────┘ │
└──────────────────────────────────────────────────┼────────────────────┘
                                                    │
              ┌─────────────────────────────────────┼──────────────────┐
              │           Infrastructure              │                  │
              │  [PostgreSQL] ◀──────────────────────┤                  │
              │  [Zitadel]    ◀── OIDC UserInfo ─────┤                  │
              │  [step-ca]    ◀── CSR sign ───────────┘                 │
              └──────────────────────────────────────────────────────────┘
```

---

## Components

### Backend (`backend/`)

Go service exposing two interfaces:

**REST API (port 8080)**
- Agents, jobs, network capacity — public read-only telemetry
- `/api/v1/users/*` — authenticated user endpoints
- `/api/v1/admin/*` — admin-only management
- `/auth/callback` — OIDC authorization code exchange
- `/ws` — WebSocket for live dashboard updates

**gRPC (port 9090)**
- `AgentService` — Register, Heartbeat, StreamJobs, ReportResult
- `SignalingService` — WebRTC ICE signaling for P2P data transfer

**Internal packages:**
- `auth/` — Zitadel JWT validation (UserInfo cache), step-ca client (ES256 OTT + CSR), gRPC mTLS interceptors, HTTP middleware
- `registry/` — in-memory agent map (fast dispatch) + PostgreSQL persistence
- `scheduler/` — matches jobs to agents by GPU layer requirement and bench score
- `jobs/` — job lifecycle (create → assign → complete/fail)
- `tokens/` — immutable ledger with credit/debit operations
- `users/` — user CRUD, API key generation, admin operations

### Agent (`agent/`)

Go binary that runs on each contributing machine:

1. **Startup benchmark** — measures CPU/RAM/GPU, sends result to backend
2. **Registration** — generates ECDSA P-256 key, creates CSR, registers with API key, receives signed mTLS cert
3. **Job stream** — long-lived gRPC stream; waits for `JobPayload` messages
4. **Execution** — forwards prompt to local Ollama via HTTP; max 4 concurrent jobs with 10-minute per-job timeout
5. **Result reporting** — sends success/failure + duration back via `ReportResult`
6. **Heartbeat** — pings backend every 10 seconds

### Frontend (`frontend/`)

React + Tailwind SPA:

- `useAuth` hook — fetches `/api/v1/users/me` on load; 401 → shows login page
- `useAgents` hook — polls REST + connects WebSocket for live updates
- `LoginPage` — redirects to Zitadel OIDC endpoint (Google/GitHub)
- `AdminPanel` — approve/disable users and agents; full user table
- `UserProfile` — API key display, copy, regenerate; agent setup instructions
- Main dashboard — network capacity, agent table, job submit, token balances, job history

### Orchestrator Proxy (`orchestrator/`)

Local HTTP proxy on `:11435` that intercepts Ollama API calls and decides:
- Short prompts / explicit `[OFFLOAD]` token → route to sharedGPUpower network
- Everything else → pass through to local Ollama on `:11434`

---

## Authentication & Security

### User Authentication (Zitadel)

```
Browser                Zitadel              Backend
  │                       │                    │
  │── redirect ──────────▶│                    │
  │◀─ login page ─────────│                    │
  │── Google/GitHub ─────▶│                    │
  │◀─ auth code ──────────│                    │
  │── GET /auth/callback?code=... ────────────▶│
  │                        │◀── UserInfo ──────│
  │                        │── sub,email,name ▶│
  │                        │                   │── FindOrCreate user
  │◀────────────────── Set-Cookie: sgpu_session │
  │── all subsequent requests with cookie ────▶│
```

The `sgpu_session` cookie holds the Zitadel access token. The backend validates it on every request by calling Zitadel's `/oidc/v1/userinfo` endpoint with a 5-minute local cache (keyed by token).

### Agent Authentication (API key + mTLS)

```
Agent                  Backend                step-ca
  │── Register(user_api_key, CSR) ───────────▶│
  │                                            │── RequestCert(CSR, agentID)
  │                                            │                │
  │                                            │◀── cert_pem ───│
  │◀── RegisterResponse(agent_id, cert_pem) ──│
  │
  │  [reconnect with mTLS]
  │
  │── StreamJobs (mTLS) ──────────────────────▶│
  │── Heartbeat  (mTLS) ──────────────────────▶│
  │── ReportResult (mTLS) ────────────────────▶│
```

The backend calls step-ca's `/1.0/sign` endpoint with a short-lived ES256 JWT (one-time token) signed with the provisioner private key. step-ca validates the JWT and signs the agent's CSR.

In production, set `MTLS_REQUIRED=true` to enforce mTLS client certificates on all gRPC calls except `Register`.

---

## Data Model

### Key Tables

```sql
users
  id, zitadel_id, email, display_name, avatar_url
  role ('user'|'admin')
  status ('pending'|'active'|'disabled')
  api_key  -- 'sgpu_' + 32 hex bytes

agents
  id, name, user_id (FK users)
  arch, os, gpu_model, cpu_cores, ram_gb
  bench_score, cpu_limit, ram_limit, gpu_layers_limit
  status ('online'|'offline')
  approval_status ('pending'|'active'|'disabled')
  last_seen

token_ledger
  agent_id (FK agents), delta, reason, job_id, created_at

jobs
  id, submitter_agent_id, assigned_agent_id
  type, status, payload, result
  created_at, completed_at, duration_seconds
  user_id (FK users)
```

---

## Job Dispatch Flow

```
POST /api/v1/jobs
      │
      ▼
jobs.Store.Create()          -- persists job as 'pending'
      │
      ▼
scheduler.Enqueue()          -- puts JobRequest into channel
      │
      ▼
scheduler.Run() (goroutine)
  │
  ├── GetAvailable()          -- agents with status='online' AND approval_status='active'
  ├── filter by required_gpu_layers
  ├── pick agent with highest bench_score
  ├── marshal payload as JSON
  └── agent.JobCh ◀──────────  channel send (buffered, size 8)
                                     │
                                     ▼
                            AgentService.StreamJobs()
                              stream.Send(JobPayload)
                                     │
                                     ▼ (gRPC stream to agent)
                            agent executor.Execute()
                              Ollama HTTP call
                                     │
                                     ▼
                            AgentService.ReportResult()
                              sched.CompleteJob() / FailJob()
                              tokens.Credit() / no charge
```

---

## Concurrency Model

| Concern | Solution |
|---|---|
| Concurrent job handlers in agent | Semaphore (max 4) + `context.WithTimeout(10 min)` per job |
| Duplicate StreamJobs streams per agent | `activeStreams map[string]struct{}` + mutex in `AgentServer` |
| Agent registry reads/writes | `sync.RWMutex` in `Registry` |
| WebSocket broadcast | Snapshot connections under read lock, write outside lock |
| SignalingService sessions | `sync.RWMutex` with `getSession()` helper |
| Stale agents | Ticker goroutine sweeps every 15s; `SetOffline()` closes `JobCh` |

---

## Protocols

| Connection | Protocol | Port | Auth |
|---|---|---|---|
| Browser → Backend | HTTP/1.1 + WebSocket | 8080 | Session cookie |
| Agent → Backend (gRPC) | HTTP/2 | 9090 | API key (Register) · mTLS (other) |
| Backend → Zitadel | HTTPS | 8081 | Bearer token (UserInfo call) |
| Backend → step-ca | HTTP | 9000 | ES256 JWT (OTT) |
| Agent → Ollama | HTTP | 11434 | None (local) |
| P2P (WebRTC) | DTLS/SCTP | dynamic | ICE via SignalingService |
