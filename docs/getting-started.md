# Getting Started

This guide walks you through joining sharedGPUpower as a new user: creating an account, connecting your first agent, and submitting a job.

## Prerequisites

- A Google or GitHub account (used for login)
- [Ollama](https://ollama.ai) installed and running on the machine you want to share (`ollama serve`)
- At least one model pulled: `ollama pull llama3.2:3b`

---

## 1. Create an Account

1. Open the dashboard in your browser (e.g. `http://localhost:3000` for local dev or your deployment URL).
2. Click **Continue with Google** or **Continue with GitHub**.
3. Authorize the OAuth app.

**First-time login**: your account is automatically set to `pending` and must be approved by an admin before you can access the network. You will see a yellow notice:

> *Your account is pending admin approval.*

Once an admin approves your account (see [Admin Guide](admin-guide.md)), you can log back in and access the full dashboard.

> **Note:** The very first user to register on a fresh installation is automatically granted `admin` status and `active` state — no approval needed.

---

## 2. Get Your API Key

After your account is approved:

1. Click your name in the top-right header → **Profile**.
2. Your API key is shown under **API Key** — it starts with `sgpu_`.
3. Click **Copy** to copy it to the clipboard.

You will need this key during agent installation.

---

## 3. Install an Agent

Run the one-line installer on the machine you want to share:

```bash
curl -fsSL https://github.com/lambdawp-567/sharedgpupower/releases/latest/download/install.sh | bash
```

The installer will:
- Download the agent binary for your OS/architecture
- Ask for your API key (paste the `sgpu_...` key from step 2)
- Write a config file to `~/.sharedgpu/agent.yaml`
- Register the agent as a system service (launchd on macOS, systemd on Linux)

After installation the agent starts automatically and registers with the backend. It sends a CSR to the backend, which forwards it to step-ca to receive an mTLS certificate — used for all subsequent communication.

### Verify the Agent is Running

**macOS:**
```bash
launchctl list | grep sharedgpu
tail -f ~/.sharedgpu/agent.log
```

**Linux:**
```bash
sudo systemctl status sharedgpupower-agent
journalctl -u sharedgpupower-agent -f
```

---

## 4. Wait for Agent Approval

New agents start in `pending` state. An admin must approve each agent before it can receive jobs.

- Go to the dashboard → **Profile** → **My Agents** to see your agents and their approval status.
- Once an admin approves your agent (the status changes to `active`), it will start receiving jobs from the network.

---

## 5. Submit a Job

Once you have an approved agent and an active account:

1. Open the dashboard → **Dashboard** tab.
2. Scroll to the **Submit Job** panel.
3. Fill in:
   - **Submitter Agent** — your agent (must be online and active)
   - **Prompt** — the text to send to the LLM
   - **Model** — e.g. `llama3.2:3b`
   - **Max tokens** — maximum response length
4. Click **Submit**.

The job is dispatched to the best available agent in the network. You can track its status in the **Job History** panel below.

---

## 6. Monitor Your Token Balance

Every job you run on someone else's hardware costs tokens. Every job your agent executes earns tokens. Your balance is shown in the **Token Balances** panel on the dashboard.

See [Token System](token-system.md) for how earnings and costs are calculated.

---

## Next Steps

- **Share more compute:** adjust resource limits in `~/.sharedgpu/agent.yaml` (see [Agent Setup](agent-setup.md))
- **Use the orchestrator proxy:** route your local Ollama client through the network for large tasks (see [Agent Setup — Orchestrator](agent-setup.md#orchestrator-proxy))
- **Administer the network:** if you are the admin, see [Admin Guide](admin-guide.md)
