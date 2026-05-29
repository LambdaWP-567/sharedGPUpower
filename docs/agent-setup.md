# Agent Setup

This guide covers everything about the sharedGPUpower agent: installation, configuration, resource limits, mTLS certificates, and the local orchestrator proxy.

---

## Requirements

| | Minimum | Recommended |
|---|---|---|
| OS | macOS 13 · Linux (any distro) | macOS 14+ · Ubuntu 22.04+ |
| Architecture | amd64 · arm64 (Apple Silicon · Raspberry Pi) | Apple M-series |
| RAM | 4 GB | 16 GB+ |
| Disk | 2 GB (models excluded) | 50 GB+ |
| Ollama | 0.3+ | latest |

Install Ollama from [ollama.ai](https://ollama.ai) and pull at least one model before installing the agent:

```bash
ollama pull llama3.2:3b     # small, fast (~2 GB)
ollama pull llama3.1:8b     # good quality (~5 GB)
```

---

## Installation

### One-Line Installer (Recommended)

```bash
curl -fsSL https://github.com/lambdawp-567/sharedgpupower/releases/latest/download/install.sh | bash
```

When prompted, paste your API key (`sgpu_...`) from the dashboard → Profile.

The installer:
1. Downloads the correct binary for your OS/arch
2. Writes `~/.sharedgpu/agent.yaml`
3. Installs a system service and starts it

### Manual Installation

```bash
# Download binary (replace OS and ARCH as needed: linux-amd64, linux-arm64, darwin-arm64)
curl -fsSL https://github.com/lambdawp-567/sharedgpupower/releases/latest/download/agent-darwin-arm64 \
  -o ~/.sharedgpu/agent
chmod +x ~/.sharedgpu/agent

# Create config
cp /path/to/agent.yaml.example ~/.sharedgpu/agent.yaml
# Edit ~/.sharedgpu/agent.yaml — set backend.endpoint and auth.user_api_key

# Run
~/.sharedgpu/agent --config ~/.sharedgpu/agent.yaml
```

---

## Configuration Reference

Config file: `~/.sharedgpu/agent.yaml`

```yaml
# Backend gRPC endpoint
backend:
  endpoint: "grpc.sharedgpupower.example.com:443"
  insecure: false          # set true only for local dev (no TLS)

# Resource limits — the agent will not exceed these
resources:
  cpu_percent: 50          # max % of CPU cores to use for jobs
  ram_percent: 25          # max % of total RAM
  gpu_layers: 20           # Ollama num_gpu_layers limit

# Ollama connection
ollama:
  host: "http://localhost:11434"
  default_model: "llama3.2:3b"

# Agent identity
agent:
  name: "my-mac-studio"   # name shown in dashboard (defaults to hostname)

# Authentication
auth:
  user_api_key: "sgpu_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"
  cert_path: "~/.sharedgpu/agent.crt"    # mTLS cert (auto-populated after first registration)
  key_path:  "~/.sharedgpu/agent.key"    # private key (auto-generated)
  ca_cert_path: "~/.sharedgpu/ca.crt"    # CA cert (auto-populated after first registration)
```

### Resource Limit Guidelines

| Machine | cpu_percent | ram_percent | gpu_layers |
|---|---|---|---|
| Mac Studio M2 Ultra (daily driver) | 30 | 20 | 20 |
| Mac Mini M4 (dedicated server) | 80 | 60 | 60 |
| Raspberry Pi 5 (8 GB) | 50 | 30 | 0 (CPU only) |
| Linux workstation (RTX 4090) | 40 | 25 | 80 |

Setting `gpu_layers: 0` disables GPU offloading entirely (CPU-only inference).

---

## Service Management

### macOS (launchd)

```bash
# Status
launchctl list | grep sharedgpu

# Start / stop
launchctl start com.sharedgpupower.agent
launchctl stop  com.sharedgpupower.agent

# Logs
tail -f ~/.sharedgpu/agent.log
tail -f ~/.sharedgpu/agent-error.log

# Uninstall
launchctl unload ~/Library/LaunchAgents/com.sharedgpupower.agent.plist
rm ~/Library/LaunchAgents/com.sharedgpupower.agent.plist
```

### Linux (systemd)

```bash
# Status
sudo systemctl status sharedgpupower-agent

# Start / stop / restart
sudo systemctl start   sharedgpupower-agent
sudo systemctl stop    sharedgpupower-agent
sudo systemctl restart sharedgpupower-agent

# Follow logs
journalctl -u sharedgpupower-agent -f

# Disable autostart
sudo systemctl disable sharedgpupower-agent
```

---

## mTLS Certificates

On first registration the agent:

1. Generates an ECDSA P-256 private key → saved to `auth.key_path`
2. Creates a Certificate Signing Request (CSR)
3. Sends the CSR to the backend with the API key
4. The backend forwards the CSR to step-ca and returns the signed certificate
5. Certificate saved to `auth.cert_path`, CA certificate to `auth.ca_cert_path`

On all subsequent runs the agent uses mTLS (mutual TLS) with the saved certificate. If a certificate is missing or expired, the agent re-generates the key pair and re-registers.

**Certificate lifetime:** 24 hours (configurable in step-ca). The agent must re-register before expiry. The `name` and `user_api_key` in the config are used for re-registration.

---

## Startup Benchmark

Each time the agent starts, it runs a benchmark to measure hardware performance:

| Metric | What is measured |
|---|---|
| `cpu_score` | Floating-point throughput, normalized to 1.0 on M1 Pro |
| `mem_bandwidth_gbps` | Memory read bandwidth in GB/s |
| `gpu_tflops` | GPU throughput (via Ollama matrix multiply) |
| `composite_score` | Weighted combination of the above |

The benchmark result is sent to the backend during registration and is used to calculate token earnings. Faster hardware earns more tokens per job.

---

## Updating the Agent

Re-run the installer to update to the latest version:

```bash
curl -fsSL https://github.com/lambdawp-567/sharedgpupower/releases/latest/download/install.sh | bash
```

Your config file (`~/.sharedgpu/agent.yaml`) and certificates are preserved.

---

## Regenerating Your API Key

If your API key is compromised:

1. Dashboard → Profile → **Regenerate** (next to the API key).
2. Update your config: `nano ~/.sharedgpu/agent.yaml` → change `auth.user_api_key`.
3. Restart the agent:
   - macOS: `launchctl stop com.sharedgpupower.agent && launchctl start com.sharedgpupower.agent`
   - Linux: `sudo systemctl restart sharedgpupower-agent`

---

## Orchestrator Proxy

The orchestrator proxy runs on `localhost:11435` and sits in front of your local Ollama instance. It automatically routes requests to the sharedGPUpower network when:

- The prompt contains the token `[OFFLOAD]`, or
- The request would consume more context than the local model can handle efficiently

```
Local LLM client  →  :11435 (proxy)  →  :11434 (Ollama, short tasks)
                                     →  sharedGPUpower network (large tasks)
```

### Start the Proxy

```bash
# The proxy binary is built from orchestrator/
./orchestrator --config ~/.sharedgpu/agent.yaml

# Or run via Docker Compose (included in the stack)
```

### Using the Proxy in Your Application

Change your Ollama endpoint from `http://localhost:11434` to `http://localhost:11435`. Everything else stays the same — the proxy is fully API-compatible with Ollama.

```python
# Example: LangChain
from langchain_ollama import ChatOllama
llm = ChatOllama(base_url="http://localhost:11435", model="llama3.2:3b")
```

---

## Troubleshooting

### Agent does not connect

```
ERROR  grpc connect failed  error="..."
```

- Check that the backend endpoint is correct in `agent.yaml` (`backend.endpoint`)
- For local dev: set `backend.insecure: true`
- Firewall: ensure TCP port 9090 (gRPC) is reachable

### "invalid api key"

```
ERROR  register failed  error="rpc error: code = Unauthenticated desc = invalid api key"
```

- The `auth.user_api_key` in `agent.yaml` is incorrect or has been regenerated on the dashboard
- Copy the current key from Dashboard → Profile → API Key

### "agent pending approval"

```
WARN   job stream ended, reconnecting in 5s  error="rpc error: code = PermissionDenied desc = agent pending approval"
```

This is expected for new agents. An admin must approve the agent before it can receive jobs. The agent will keep retrying — no action required on your end.

### Ollama not reachable

```
WARN   ollama not reachable (will retry per job)
```

- Check that Ollama is running: `ollama serve`
- Verify `ollama.host` in `agent.yaml` matches your Ollama port (default: 11434)

### Certificate errors

Delete the certificate files and let the agent re-register:

```bash
rm ~/.sharedgpu/agent.crt ~/.sharedgpu/agent.key ~/.sharedgpu/ca.crt
# Restart the agent service
```
