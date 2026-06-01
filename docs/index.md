# Welcome to sharedGPUpower

**Distributed compute sharing network for LLM inference and GPU workloads.**

Idle Macs, Raspberry Pis, and any Linux box become nodes in a shared compute network.  
Contribute compute → earn tokens. Spend tokens → offload workloads.

```
[Your Mac / RPi]  ──gRPC/mTLS──▶  [Backend]
                                       │
                              ┌────────┴────────┐
                              │   Job Broker    │
                              │  Token Ledger   │
                              │  Agent Registry │
                              └────────┬────────┘
                                       │
                         ◀──REST/WS──  [Dashboard]
```

## Quick Start

=== "New User"

    1. Open the dashboard and sign in with Google or GitHub
    2. Wait for admin approval
    3. Copy your API key from **Profile**
    4. [Install an agent](agent-setup.md) on any machine running Ollama

=== "Agent Install"

    ```bash
    curl -fsSL https://github.com/lambdawp-567/sharedgpupower/releases/latest/download/install.sh | bash
    ```

    Enter your `sgpu_...` API key when prompted. Done.

=== "Local Dev"

    ```bash
    git clone https://github.com/lambdawp-567/sharedgpupower
    cd sharedgpupower
    docker compose up -d
    # Dashboard → http://localhost:3000
    ```

## Guides

<div class="grid cards" markdown>

-   :material-rocket-launch: **Getting Started**

    ---

    Sign up, get approved, install your first agent, submit a job.

    [:octicons-arrow-right-24: Getting Started](getting-started.md)

-   :material-server: **Agent Setup**

    ---

    Install, configure resource limits, manage the service, troubleshoot.

    [:octicons-arrow-right-24: Agent Setup](agent-setup.md)

-   :material-shield-account: **Admin Guide**

    ---

    Approve users and agents, configure Zitadel + step-ca, deploy to k3s.

    [:octicons-arrow-right-24: Admin Guide](admin-guide.md)

-   :material-api: **API Reference**

    ---

    Complete REST, WebSocket, and gRPC API with request/response examples.

    [:octicons-arrow-right-24: API Reference](api-reference.md)

-   :material-currency-usd: **Token System**

    ---

    How tokens are earned, spent, and calculated from benchmark scores.

    [:octicons-arrow-right-24: Token System](token-system.md)

-   :material-chart-tree: **Architecture**

    ---

    System design, auth flows, data model, job dispatch, concurrency model.

    [:octicons-arrow-right-24: Architecture](architecture.md)

</div>

## Features

| Feature | Description |
|---|---|
| One-command install | `curl \| bash` — sets up launchd (macOS) or systemd (Linux) |
| Benchmark-normalized tokens | Faster hardware earns more per unit time |
| Admin approval flow | Users and agents require sign-off before joining |
| mTLS agent auth | step-ca issues certificates for each registered agent |
| OAuth2 login | Google and GitHub via Zitadel |
| Live dashboard | WebSocket real-time agent status and job progress |
| WebRTC P2P | Job payloads transfer peer-to-peer for large models |
| Ollama integration | Works with any model on localhost:11434 |
| Multi-arch | linux/amd64 · linux/arm64 · darwin/arm64 (Apple Silicon) |

## Requirements

| Component | Minimum |
|---|---|
| Agent OS | macOS 13+ · Linux (aarch64 / x86_64) |
| Agent RAM | 4 GB |
| Ollama | Running on the agent machine (`ollama serve`) |
| Backend | Docker Compose (local) · k3s (production) |
