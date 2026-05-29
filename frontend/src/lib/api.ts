const BASE = '/api/v1'

export interface Agent {
  id: string
  name: string
  status: 'online' | 'offline' | 'busy'
  arch: string
  os: string
  gpu_model: string
  cpu_cores: number
  ram_gb: number
  bench_score: number
  last_seen: string
}

export interface NetworkCapacity {
  total_agents: number
  online_agents: number
  total_cpu_cores: number
  total_ram_gb: number
  total_gpu_layers: number
  avg_bench_score: number
}

export interface Job {
  ID: string
  SubmitterAgentID: string
  AssignedAgentID: string
  Type: string
  Status: string
  TokenCost: number
  Error: string
  CreatedAt: string
}

export interface SubmitJobRequest {
  submitter_agent_id: string
  model: string
  prompt: string
  max_tokens?: number
  required_gpu_layers?: number
}

export async function fetchAgents(): Promise<Agent[]> {
  const res = await fetch(`${BASE}/agents`)
  if (!res.ok) throw new Error('Failed to fetch agents')
  return res.json()
}

export async function fetchCapacity(): Promise<NetworkCapacity> {
  const res = await fetch(`${BASE}/network/capacity`)
  if (!res.ok) throw new Error('Failed to fetch capacity')
  return res.json()
}

export async function fetchAgentBalance(agentId: string): Promise<number> {
  const res = await fetch(`${BASE}/agents/${agentId}/balance`)
  if (!res.ok) throw new Error('Failed to fetch balance')
  const data = await res.json()
  return data.balance
}

export async function submitJob(req: SubmitJobRequest): Promise<{ job_id: string; status: string }> {
  const res = await fetch(`${BASE}/jobs`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(req),
  })
  if (!res.ok) throw new Error('Failed to submit job')
  return res.json()
}

export async function fetchJob(jobId: string): Promise<Job> {
  const res = await fetch(`${BASE}/jobs/${jobId}`)
  if (!res.ok) throw new Error('Failed to fetch job')
  return res.json()
}

export async function fetchJobs(agentId: string): Promise<Job[]> {
  const res = await fetch(`${BASE}/jobs?agent_id=${agentId}`)
  if (!res.ok) throw new Error('Failed to fetch jobs')
  return res.json()
}
