# sharedGPUpower

**Distributed compute sharing network for LLM inference and GPU workloads.**

Idle Macs, Raspberry Pis, and any Linux box become nodes in a shared compute network. Contribute compute → earn tokens. Spend tokens → offload workloads. Local LLMs can transparently route large requests to the network.

```
[Your Mac / RPi]  ──gRPC/mTLS──▶  [Backend (k3s)]
                                       │
                              ┌────────┴────────┐
                              │   Job Broker    │
                              │  Token Ledger   │
                              │  Agent Registry │
                              └────────┬────────┘
                                       │
                         ◀──REST/WS──  [Dashboard]
```

## Quick Links

| Document | Audience |
|---|---|
| [Getting Started](docs/getting-started.md) | New users — sign up, connect an agent, submit a job |
| [Agent Setup](docs/agent-setup.md) | Machine owners — install, configure, share compute |
| [Admin Guide](docs/admin-guide.md) | Network admins — approve users/agents, manage the network |
| [API Reference](docs/api-reference.md) | Developers — REST and gRPC API |
| [Token System](docs/token-system.md) | Everyone — how tokens are earned and spent |
| [Architecture](docs/architecture.md) | Operators / contributors — system design |

## Features

- **One-command agent install** — `curl | bash` with launchd (macOS) and systemd (Linux) support
- **Benchmark-normalized tokens** — faster hardware earns more per unit time
- **Admin approval flow** — new users and agents require admin sign-off before joining the network
- **mTLS agent authentication** — step-ca issues certificates for registered agents
- **OAuth2 login** — Google and GitHub sign-in via Zitadel
- **WebSocket live dashboard** — real-time agent status and job progress
- **WebRTC P2P** — job payloads transfer peer-to-peer for large models
- **Ollama integration** — works with any model running locally on port 11434
- **Multi-arch** — linux/amd64, linux/arm64, darwin/arm64 (Apple Silicon native)

## Requirements

| Component | Minimum |
|---|---|
| Agent OS | macOS 13+ (Apple Silicon) · Linux (aarch64/x86_64) |
| Agent RAM | 4 GB |
| Ollama | [ollama.ai](https://ollama.ai) running on the agent machine |
| Backend | Docker Compose (local) or k3s (production) |

## Local Development

```bash
# Start all services (Postgres, Redis, Zitadel, step-ca, backend, frontend)
docker compose up -d

# Dashboard: http://localhost:3000
# Backend API: http://localhost:8080
# Zitadel admin: http://localhost:8081
```

See [Getting Started](docs/getting-started.md) for the full setup walkthrough.

## Repository Layout

```
sharedGPUpower/
├── agent/                 # Go binary — macOS + Linux
│   ├── cmd/agent/         # Entry point
│   ├── internal/
│   │   ├── benchmark/     # Startup benchmark (CPU/GPU/RAM)
│   │   ├── config/        # YAML config loader
│   │   ├── executor/      # Job execution via Ollama
│   │   ├── ollama/        # Ollama HTTP client
│   │   ├── p2p/           # WebRTC DataChannel (pion/webrtc)
│   │   └── registry/      # gRPC client + mTLS
│   └── configs/
│       └── agent.yaml.example
├── backend/               # Go REST + gRPC server
│   ├── cmd/server/
│   ├── internal/
│   │   ├── api/           # REST handlers + WebSocket hub
│   │   ├── auth/          # Zitadel middleware, step-ca client, gRPC interceptors
│   │   ├── grpc/          # AgentService + SignalingService
│   │   ├── jobs/          # Job lifecycle
│   │   ├── registry/      # In-memory + DB agent registry
│   │   ├── scheduler/     # Job matching and dispatch
│   │   ├── tokens/        # Token ledger
│   │   └── users/         # User store
│   └── migrations/        # SQL migrations (001_initial, 002_users_auth)
├── frontend/              # React + Tailwind dashboard
│   └── src/
│       ├── components/    # AgentTable, AdminPanel, UserProfile, …
│       └── hooks/         # useAgents, useAuth
├── orchestrator/          # Local Ollama proxy (:11435)
├── proto/                 # Protobuf definitions
├── k8s/                   # k3s manifests
└── docs/                  # User documentation
```

## License

MIT
