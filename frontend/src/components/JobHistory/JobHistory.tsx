import { useEffect, useState } from 'react'
import { fetchJobs, Job, Agent } from '../../lib/api'

interface Props {
  agents: Agent[]
  refreshTrigger?: number
}

const statusBadge = (s: string) => {
  const map: Record<string, string> = {
    done: 'bg-green-900 text-green-400',
    running: 'bg-blue-900 text-blue-400',
    pending: 'bg-gray-800 text-gray-400',
    failed: 'bg-red-900 text-red-400',
  }
  return map[s] ?? 'bg-gray-800 text-gray-400'
}

export function JobHistory({ agents, refreshTrigger }: Props) {
  const [jobs, setJobs] = useState<Job[]>([])

  useEffect(() => {
    const onlineAgents = agents.filter((a) => a.status === 'online')
    if (onlineAgents.length === 0) return

    Promise.all(onlineAgents.map((a) => fetchJobs(a.id).catch(() => [] as Job[])))
      .then((results) => {
        const all = results.flat()
        all.sort((a, b) => new Date(b.CreatedAt).getTime() - new Date(a.CreatedAt).getTime())
        setJobs(all.slice(0, 20))
      })
  }, [agents, refreshTrigger])

  if (jobs.length === 0) return <div className="text-gray-500 text-sm text-center py-8">No jobs yet.</div>

  return (
    <div className="space-y-2">
      {jobs.map((j) => (
        <div key={j.ID} className="bg-gray-900 rounded-lg px-4 py-3 flex items-center justify-between gap-4">
          <div className="flex-1 min-w-0">
            <div className="font-mono text-xs text-gray-500">{j.ID.slice(0, 12)}…</div>
            <div className="text-sm text-gray-300 truncate">{j.Type}</div>
          </div>
          <span className={`text-xs font-medium px-2 py-0.5 rounded ${statusBadge(j.Status)}`}>
            {j.Status}
          </span>
          <div className="text-right text-xs text-gray-500 shrink-0">
            <div className="text-yellow-400 font-mono">{j.TokenCost?.toFixed(3)} cr</div>
            <div>{new Date(j.CreatedAt).toLocaleTimeString()}</div>
          </div>
        </div>
      ))}
    </div>
  )
}
