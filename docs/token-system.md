# Token System

sharedGPUpower uses a benchmark-normalized token economy. Faster hardware earns more tokens per unit time; more demanding jobs cost more tokens.

---

## Core Formula

```
tokens = composite_score × duration_minutes × resource_fraction
```

| Variable | Description |
|---|---|
| `composite_score` | Hardware performance relative to M1 Pro = 1.0 |
| `duration_minutes` | Wall-clock time the job ran |
| `resource_fraction` | Fraction of configured resource limits consumed (min 0.01) |

### Example

An M2 Ultra (`composite_score = 2.4`) runs a 2-minute job at 60% of its configured limits:

```
tokens = 2.4 × 2.0 × 0.60 = 2.88 tokens earned
```

The same job costs the submitter 2.88 tokens.

---

## Benchmark Score

At startup the agent runs a short benchmark:

| Metric | Weight | Measurement |
|---|---|---|
| `cpu_score` | 40% | Single-thread + multi-thread floating-point ops |
| `mem_bandwidth_gbps` | 30% | Sequential memory read bandwidth |
| `gpu_tflops` | 30% | Ollama matrix multiply throughput |

The `composite_score` is a weighted combination, normalized so that an M1 Pro = 1.0.

### Reference Scores

| Hardware | Approximate composite_score |
|---|---|
| Raspberry Pi 4 (8 GB) | 0.15 |
| Raspberry Pi 5 (8 GB) | 0.25 |
| Apple M1 Pro | 1.00 |
| Apple M2 Pro | 1.35 |
| Apple M2 Ultra | 2.40 |
| Apple M4 Max | 2.80 |
| NVIDIA RTX 4090 (Linux) | 4.50 |

Scores are indicative. Actual values depend on thermal state, RAM capacity, and background load.

---

## Earning Tokens

Tokens are credited to your account when an agent you own **completes a job** for another user.

- The credit appears immediately after the job is marked `completed`
- Earnings are tied to your user account (aggregated across all your agents)
- View your balance: Dashboard → Profile → **Token Balances**, or `GET /api/v1/users/me/balance`

### Maximizing Earnings

- Run agents on faster hardware (higher `composite_score`)
- Allow higher resource fractions in `agent.yaml` (`cpu_percent`, `ram_percent`, `gpu_layers`)
- Keep agents online continuously — offline agents earn nothing
- Get your agents approved quickly — pending agents cannot accept jobs

---

## Spending Tokens

Tokens are debited when you submit a job and it is executed on another user's agent. The cost equals the earnings of the executing agent.

- **Insufficient balance**: the scheduler will still dispatch jobs if your balance is negative (no hard block at this time — this may change in a future version)
- View costs in Job History: each job shows its token cost once completed

---

## Token Ledger

Every credit and debit is recorded in the `token_ledger` table with:

| Column | Description |
|---|---|
| `agent_id` | Agent that executed (or submitted) the job |
| `delta` | Positive = earned, negative = spent |
| `reason` | `job_completed` or `job_consumed` |
| `job_id` | Reference to the job |
| `created_at` | Timestamp |

Balances are computed as `SUM(delta)` over all ledger rows for your agents — there is no separate balance column, which means the ledger is an immutable audit log.

### Querying Your History

```sql
-- All credits for a specific agent
SELECT j.id, tl.delta, tl.reason, tl.created_at
FROM token_ledger tl
JOIN jobs j ON j.id = tl.job_id
WHERE tl.agent_id = '<your_agent_id>'
ORDER BY tl.created_at DESC;
```

---

## Frequently Asked Questions

**Can I go negative?**  
Yes, currently there is no hard balance floor. Future versions may introduce a minimum balance requirement for job submission.

**What happens if a job fails?**  
Failed jobs are not charged. If a job is submitted, dispatched, but fails during execution, no tokens are debited from the submitter and none are credited to the executing agent.

**Are tokens transferable between users?**  
Not in the current version. Tokens are tied to user accounts via their agents.

**Is there inflation?**  
No. The total token supply grows only when real compute is contributed. There is no minting or reward pool unrelated to actual work.

**What is the minimum resource_fraction?**  
0.01 (1%) — enforced by the scheduler. This prevents jobs from being free due to rounding.
