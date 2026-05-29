# Admin Guide

Admins manage user access and agent approval for the sharedGPUpower network. This guide covers everything from first-time setup to day-to-day operations.

---

## Becoming an Admin

The **first user to register** on a fresh installation is automatically granted `role=admin` and `status=active` — no approval needed.

All subsequent users start with `role=user` and `status=pending`. Admins can promote any user to admin via the database (`SetRole`) or directly via a future API endpoint.

---

## Admin Panel

The Admin Panel is accessible from the header: click **Admin** (shown only to users with `role=admin`).

It has three tabs:

### Tab 1: Pending Users

Lists all users with `status=pending`. For each user:

| Field | Description |
|---|---|
| Name / Email | Display name and email from their OAuth provider |
| Registered | When they first logged in |
| **Approve** | Sets `status=active` — user can now log in and use the network |
| **Disable** | Sets `status=disabled` — blocks login indefinitely |

Approve a user when you recognise them or have verified their identity. Disable instead of delete so the audit trail is preserved.

### Tab 2: Pending Agents

Lists all agents with `approval_status=pending`. For each agent:

| Field | Description |
|---|---|
| Name | Hostname or configured agent name |
| Architecture | `arm64` / `amd64` |
| GPU model | From the benchmark at startup |
| Bench score | Composite performance score (1.0 = M1 Pro baseline) |
| **Approve** | Sets `approval_status=active` — agent starts receiving jobs |
| **Disable** | Sets `approval_status=disabled` — agent is blocked from the network |

Before approving an agent, check that:
- You recognise the machine (owner identity confirmed)
- The bench score looks reasonable for the claimed hardware
- The agent has been online recently (check the dashboard **Agents** table)

### Tab 3: All Users

Full list of all registered users with role, status, and registration date. Useful for auditing who has access and when they joined.

---

## User Lifecycle

```
Register → pending ──[Admin Approve]──▶ active ──[Admin Disable]──▶ disabled
                   ──[Admin Disable]──▶ disabled
```

| Status | Can log in | Can submit jobs | Can run agents |
|---|---|---|---|
| `pending` | No (sees pending notice) | No | No |
| `active` | Yes | Yes | If agent is also active |
| `disabled` | No (401) | No | No |

---

## Agent Lifecycle

```
Register → pending ──[Admin Approve]──▶ active ──[Admin Disable]──▶ disabled
                   ──[Agent reconnects]──▶ online (if active)
                   ──[No heartbeat 30s]──▶ offline (auto, reverts to online on reconnect)
```

| Approval Status | Receives jobs |
|---|---|
| `pending` | No — StreamJobs returns `PermissionDenied` |
| `active` | Yes — when online |
| `disabled` | No — StreamJobs returns `PermissionDenied` |

---

## Infrastructure Setup

### Zitadel (Identity Provider)

After starting Zitadel for the first time:

1. Open the Zitadel admin console (port 8081 locally).
2. Create a new **Project** → **Application** (type: Web).
3. Set the redirect URI to your backend callback URL:
   - Local: `http://localhost:8080/auth/callback`
   - Production: `https://api.sharedgpupower.example.com/auth/callback`
4. Set **Response Type** to `code` and enable **PKCE**.
5. Note the **Client ID** and optionally the **Client Secret**.
6. Add Google and/or GitHub as **Identity Providers** under the Zitadel instance settings (requires OAuth app credentials from each provider).

Set these environment variables on the backend:

```bash
ZITADEL_DOMAIN=localhost:8081           # or auth.sharedgpupower.example.com
ZITADEL_CLIENT_ID=<from Zitadel>
ZITADEL_CLIENT_SECRET=<from Zitadel>    # leave empty for PKCE-only
ZITADEL_REDIRECT_URI=http://localhost:8080/auth/callback
FRONTEND_URL=http://localhost:3000
```

Set these environment variables in the frontend build:

```bash
VITE_ZITADEL_DOMAIN=localhost:8081
VITE_ZITADEL_CLIENT_ID=<from Zitadel>
VITE_ZITADEL_REDIRECT_URI=http://localhost:8080/auth/callback
```

### step-ca (Certificate Authority)

step-ca is pre-initialised on first start via Docker (`DOCKER_STEPCA_INIT_*` env vars). For production:

1. Initialise step-ca manually to choose your own root CA name and key type:
   ```bash
   docker exec -it step-ca step ca init \
     --name "sharedGPUpower CA" \
     --dns "step-ca" \
     --address ":9000" \
     --provisioner "backend@sharedgpupower"
   ```
2. Export the provisioner's private key (JWK format) and set it as:
   ```bash
   STEPCA_PROVISIONER_KEY="-----BEGIN EC PRIVATE KEY-----
   ...
   -----END EC PRIVATE KEY-----"
   STEPCA_PROVISIONER=backend@sharedgpupower
   STEPCA_URL=http://step-ca:9000
   ```
3. The `STEPCA_PROVISIONER_KEY` must be the private key of the JWK provisioner configured in step-ca. The backend uses it to sign short-lived one-time tokens (OTT) that authorise the CA to sign agent CSRs.

### Database Migrations

Migrations are applied automatically on backend startup from the `migrations/` directory. The order is:

| File | Contents |
|---|---|
| `001_initial.sql` | `agents`, `token_ledger`, `jobs` tables |
| `002_users_auth.sql` | `users` table, `user_id` / `approval_status` on existing tables |

To apply migrations manually:

```bash
psql $DATABASE_URL -f backend/migrations/001_initial.sql
psql $DATABASE_URL -f backend/migrations/002_users_auth.sql
```

---

## Environment Variables Reference

### Backend

| Variable | Default | Description |
|---|---|---|
| `POSTGRES_HOST` | `localhost` | PostgreSQL host |
| `POSTGRES_PORT` | `5432` | PostgreSQL port |
| `POSTGRES_USER` | `sharedgpu` | PostgreSQL user |
| `POSTGRES_PASSWORD` | — | PostgreSQL password |
| `POSTGRES_DB` | `sharedgpu` | PostgreSQL database name |
| `HTTP_PORT` | `8080` | REST API port |
| `GRPC_PORT` | `9090` | gRPC port |
| `CORS_ORIGINS` | `http://localhost:3000` | Comma-separated allowed origins |
| `ZITADEL_DOMAIN` | — | Zitadel host:port (enables auth when set) |
| `ZITADEL_CLIENT_ID` | — | OIDC client ID |
| `ZITADEL_CLIENT_SECRET` | — | OIDC client secret (optional with PKCE) |
| `ZITADEL_REDIRECT_URI` | — | OAuth2 redirect URI |
| `FRONTEND_URL` | `http://localhost:3000` | Post-login redirect target |
| `STEPCA_URL` | `http://localhost:9000` | step-ca API base URL |
| `STEPCA_PROVISIONER` | `backend@sharedgpupower` | Provisioner name in step-ca |
| `STEPCA_PROVISIONER_KEY` | — | PEM private key of the step-ca JWK provisioner |
| `MTLS_REQUIRED` | `false` | Set `true` to enforce mTLS for all non-Register gRPC calls |

### Frontend (build-time)

| Variable | Default | Description |
|---|---|---|
| `VITE_ZITADEL_DOMAIN` | `localhost:8081` | Zitadel host:port for login redirect |
| `VITE_ZITADEL_CLIENT_ID` | — | OIDC client ID |
| `VITE_ZITADEL_REDIRECT_URI` | `<origin>/auth/callback` | OAuth2 redirect URI |

---

## Production Deployment (k3s)

All k8s manifests are in the `k8s/` directory:

```bash
# Apply in order
kubectl apply -f k8s/cert-manager-issuer.yaml
kubectl apply -f k8s/postgres-statefulset.yaml
kubectl apply -f k8s/redis-deployment.yaml
kubectl apply -f k8s/zitadel-deployment.yaml
kubectl apply -f k8s/step-ca-deployment.yaml
kubectl apply -f k8s/backend-deployment.yaml
kubectl apply -f k8s/frontend-deployment.yaml
kubectl apply -f k8s/ingress.yaml
```

Before applying, update the secrets in `zitadel-deployment.yaml` and create the `STEPCA_PROVISIONER_KEY` as a k8s Secret:

```bash
kubectl create secret generic stepca-provisioner \
  --from-literal=key="$(cat provisioner.key)" \
  --namespace sharedgpu
```

Reference the secret in the backend deployment:

```yaml
env:
  - name: STEPCA_PROVISIONER_KEY
    valueFrom:
      secretKeyRef:
        name: stepca-provisioner
        key: key
```

---

## Monitoring

### Health Checks

- **Backend health:** `GET /health` → `{"status":"ok"}`
- **Zitadel:** `GET http://zitadel:8080/debug/ready`
- **step-ca:** `GET https://step-ca:9000/health`

### Useful Queries

```sql
-- Agents pending approval
SELECT id, name, created_at FROM agents WHERE approval_status = 'pending';

-- Users pending approval
SELECT id, email, created_at FROM users WHERE status = 'pending';

-- Token leaderboard (top earners)
SELECT u.email, SUM(tl.delta) AS balance
FROM token_ledger tl
JOIN agents a ON a.id = tl.agent_id
JOIN users u ON u.id = a.user_id
GROUP BY u.email
ORDER BY balance DESC
LIMIT 20;

-- Job throughput last 24h
SELECT COUNT(*), status FROM jobs
WHERE created_at > NOW() - INTERVAL '24 hours'
GROUP BY status;

-- Average job duration by agent
SELECT a.name, AVG(j.duration_seconds) AS avg_s, COUNT(*) AS jobs
FROM jobs j
JOIN agents a ON a.id = j.assigned_agent_id
WHERE j.status = 'completed'
GROUP BY a.name
ORDER BY avg_s;
```
