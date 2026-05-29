import { Agent } from '../../lib/api'

interface Props {
  agents: Agent[]
}

const statusColor = (s: string) => {
  if (s === 'online') return 'bg-green-500'
  if (s === 'busy')   return 'bg-yellow-500'
  return 'bg-gray-600'
}

export function AgentTable({ agents }: Props) {
  if (agents.length === 0) {
    return (
      <div className="text-center text-gray-500 py-12">
        No agents connected yet.<br />
        <span className="text-sm">Run <code className="bg-gray-800 px-1 rounded">curl -fsSL .../install.sh | sh</code> on your Mac or Raspberry Pi.</span>
      </div>
    )
  }

  return (
    <div className="overflow-x-auto">
      <table className="w-full text-sm">
        <thead>
          <tr className="text-left text-gray-400 border-b border-gray-800">
            <th className="pb-2 pr-4">Status</th>
            <th className="pb-2 pr-4">Name</th>
            <th className="pb-2 pr-4">OS / Arch</th>
            <th className="pb-2 pr-4">CPU</th>
            <th className="pb-2 pr-4">RAM</th>
            <th className="pb-2 pr-4">GPU</th>
            <th className="pb-2 pr-4">Score</th>
            <th className="pb-2">Last Seen</th>
          </tr>
        </thead>
        <tbody>
          {agents.map((a) => (
            <tr key={a.id} className="border-b border-gray-900 hover:bg-gray-900 transition-colors">
              <td className="py-2 pr-4">
                <span className="flex items-center gap-1.5">
                  <span className={`w-2 h-2 rounded-full ${statusColor(a.status)}`} />
                  <span className="text-gray-300 capitalize">{a.status}</span>
                </span>
              </td>
              <td className="py-2 pr-4 font-medium text-white">{a.name}</td>
              <td className="py-2 pr-4 text-gray-400">{a.os}/{a.arch}</td>
              <td className="py-2 pr-4 text-gray-300">{a.cpu_cores}c</td>
              <td className="py-2 pr-4 text-gray-300">{a.ram_gb?.toFixed(0)}GB</td>
              <td className="py-2 pr-4 text-gray-300 truncate max-w-[120px]">{a.gpu_model || '—'}</td>
              <td className="py-2 pr-4">
                <span className="bg-brand-900 text-brand-500 text-xs font-mono px-2 py-0.5 rounded">
                  {a.bench_score?.toFixed(2)}
                </span>
              </td>
              <td className="py-2 text-gray-500 text-xs">
                {new Date(a.last_seen).toLocaleTimeString()}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  )
}
