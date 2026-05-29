# API Reference

Base URL: `http://localhost:8080` (local dev) or `https://api.sharedgpupower.example.com` (production)

All authenticated endpoints require either:
- `Authorization: Bearer <zitadel_access_token>` header, or
- `sgpu_session` HttpOnly cookie (set by the backend after OIDC login)

---

## Authentication

### OIDC Login Flow

Redirect the user to:

```
GET /auth/callback?code=<authorization_code>
```

The backend exchanges the code for a Zitadel access token, creates/finds the local user, and sets the `sgpu_session` cookie. The user is redirected to `FRONTEND_URL/dashboard`.

### Logout

```
GET /auth/logout
```

Clears the session cookie and redirects to `FRONTEND_URL`.

---

## Public Endpoints

These endpoints do not require authentication.

### Health

```
GET /health
```

**Response:**
```json
{ "status": "ok" }
```

### List Agents

```
GET /api/v1/agents
```

Returns all registered agents (online and offline).

**Response:**
```json
[
  {
    "id": "550e8400-e29b-41d4-a716-446655440000",
    "name": "mac-studio-m2",
    "status": "online",
    "approval_status": "active",
    "arch": "arm64",
    "os": "darwin",
    "gpu_model": "Apple M2 Ultra",
    "cpu_cores": 24,
    "ram_gb": 192,
    "bench_score": 2.4,
    "last_seen": "2026-05-29T12:00:00Z"
  }
]
```

### Agent Token Balance

```
GET /api/v1/agents/{id}/balance
```

**Response:**
```json
{ "balance": 42.75 }
```

### Network Capacity

```
GET /api/v1/network/capacity
```

**Response:**
```json
{
  "total_agents": 12,
  "online_agents": 8,
  "total_cpu_cores": 192,
  "total_ram_gb": 768.0,
  "total_gpu_layers": 480,
  "avg_bench_score": 1.8
}
```

---

## Authenticated Endpoints

### User Profile

#### Get Current User

```
GET /api/v1/users/me
```

**Response:**
```json
{
  "id": "550e8400-...",
  "email": "user@example.com",
  "display_name": "Jane Doe",
  "avatar_url": "https://avatars.githubusercontent.com/u/...",
  "role": "user",
  "status": "active",
  "api_key": "sgpu_a1b2c3...",
  "created_at": "2026-05-01T10:00:00Z"
}
```

#### Regenerate API Key

```
POST /api/v1/users/me/regenerate-key
```

Invalidates the current API key and issues a new one. All agents using the old key will fail authentication until reconfigured.

**Response:**
```json
{ "api_key": "sgpu_d4e5f6..." }
```

#### List My Agents

```
GET /api/v1/users/me/agents
```

Returns all agents registered with the current user's API key.

**Response:** Array of agent objects (same schema as `GET /api/v1/agents`).

#### My Token Balance

```
GET /api/v1/users/me/balance
```

Aggregates token balances across all of the user's agents.

**Response:**
```json
{ "balance": 120.50 }
```

---

### Jobs

#### Submit a Job

```
POST /api/v1/jobs
Content-Type: application/json
```

**Request body:**
```json
{
  "submitter_agent_id": "550e8400-...",
  "type": "llm_inference",
  "model": "llama3.2:3b",
  "prompt": "Explain quantum entanglement in simple terms.",
  "max_tokens": 512,
  "required_gpu_layers": 10,
  "extra": {
    "temperature": 0.7
  }
}
```

| Field | Required | Description |
|---|---|---|
| `submitter_agent_id` | Yes | Your agent's ID (must be yours) |
| `prompt` | Yes | Text prompt |
| `type` | No | Job type, default `llm_inference` |
| `model` | No | Ollama model name |
| `max_tokens` | No | Max response tokens |
| `required_gpu_layers` | No | Minimum GPU layers needed on the executing agent |
| `extra` | No | Additional key-value pairs passed to Ollama |

**Response (202 Accepted):**
```json
{ "job_id": "f47ac10b-...", "status": "pending" }
```

#### Get Job Status

```
GET /api/v1/jobs/{id}
```

**Response:**
```json
{
  "ID": "f47ac10b-...",
  "SubmitterAgentID": "550e8400-...",
  "AssignedAgentID": "6ba7b810-...",
  "Type": "llm_inference",
  "Status": "completed",
  "Payload": { "model": "llama3.2:3b", "prompt": "..." },
  "Result": { "response": "Quantum entanglement is..." },
  "CreatedAt": "2026-05-29T12:00:00Z",
  "CompletedAt": "2026-05-29T12:00:08Z",
  "DurationSeconds": 7.4
}
```

Job statuses: `pending` · `assigned` · `completed` · `failed`

#### List Jobs

```
GET /api/v1/jobs?agent_id={submitter_agent_id}
```

Returns the last 50 jobs submitted by the given agent.

**Response:** Array of job objects.

---

### Admin Endpoints

All admin endpoints require `role=admin`.

#### List All Users

```
GET /api/v1/admin/users
```

#### List Pending Users

```
GET /api/v1/admin/users/pending
```

#### Approve User

```
POST /api/v1/admin/users/{id}/approve
```

Sets `status=active`.

**Response:**
```json
{ "status": "active" }
```

#### Disable User

```
POST /api/v1/admin/users/{id}/disable
```

Sets `status=disabled`.

**Response:**
```json
{ "status": "disabled" }
```

#### List Pending Agents

```
GET /api/v1/admin/agents/pending
```

#### Approve Agent

```
POST /api/v1/admin/agents/{id}/approve
```

Sets `approval_status=active`. The agent can now receive jobs.

**Response:**
```json
{ "approval_status": "active" }
```

#### Disable Agent

```
POST /api/v1/admin/agents/{id}/disable
```

Sets `approval_status=disabled`.

**Response:**
```json
{ "approval_status": "disabled" }
```

---

## WebSocket

### Real-time Events

```
GET /ws
```

Connect with a standard WebSocket client. The server sends JSON messages whenever an agent comes online/offline or a job changes state:

```json
{ "type": "agent_update", "agent_id": "550e8400-...", "status": "online" }
{ "type": "job_update",   "job_id":   "f47ac10b-...", "status": "completed" }
```

The server sends a ping frame every 30 seconds; clients should respond with a pong.

---

## gRPC API

The gRPC service (`AgentService`) is on port 9090. It is used exclusively by the agent binary.

**Proto file:** `proto/agent.proto`

| RPC | Type | Description |
|---|---|---|
| `Register` | Unary | Register agent, get agent ID + mTLS cert |
| `Heartbeat` | Unary | Keep-alive every 10 seconds |
| `StreamJobs` | Server-streaming | Receive job payloads |
| `ReportResult` | Unary | Report job completion/failure |

### Register

**Request:**
```protobuf
RegisterRequest {
  string name          = 1;
  string public_key    = 2;
  HardwareInfo hardware = 3;
  ResourceLimits limits = 4;
  BenchmarkResult benchmark = 5;
  string user_api_key  = 6;  // sgpu_... from dashboard
  bytes  csr_pem       = 7;  // PEM-encoded ECDSA CSR (optional)
}
```

**Response:**
```protobuf
RegisterResponse {
  string agent_id   = 1;
  string token      = 2;   // same as agent_id (legacy)
  bytes  cert_pem   = 3;   // mTLS cert signed by step-ca
  bytes  ca_cert_pem = 4;  // step-ca root CA cert
}
```

### StreamJobs

Requires:
- `approval_status=active` on the agent (otherwise `PermissionDenied`)
- `MTLS_REQUIRED=true` → valid mTLS client cert (production)

Each message is a `JobPayload` with the job ID, type, and JSON payload.

---

## Error Codes

| HTTP | gRPC | Meaning |
|---|---|---|
| 400 | `InvalidArgument` | Missing or malformed request field |
| 401 | `Unauthenticated` | Missing/invalid token or API key |
| 403 | `PermissionDenied` | Account/agent not active or insufficient role |
| 404 | `NotFound` | Resource does not exist |
| 500 | `Internal` | Server-side error (check backend logs) |
