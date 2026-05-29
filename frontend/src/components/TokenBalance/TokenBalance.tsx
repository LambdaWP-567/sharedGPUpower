import { useEffect, useState } from 'react'
import { fetchAgentBalance, Agent } from '../../lib/api'
import { Coins } from 'lucide-react'

interface Props {
  agents: Agent[]
}

export function TokenBalance({ agents }: Props) {
  const [balances, setBalances] = useState<Record<string, number>>({})

  useEffect(() => {
    const onlineIds = agents.filter((a) => a.status === 'online').map((a) => a.id)
    Promise.all(
      onlineIds.map(async (id) => {
        try {
          const bal = await fetchAgentBalance(id)
          return [id, bal] as [string, number]
        } catch {
          return [id, 0] as [string, number]
        }
      })
    ).then((entries) => setBalances(Object.fromEntries(entries)))
  }, [agents])

  if (agents.length === 0) return null

  return (
    <div className="space-y-2">
      {agents
        .filter((a) => a.status === 'online')
        .map((a) => (
          <div key={a.id} className="flex items-center justify-between bg-gray-900 rounded-lg px-4 py-3">
            <div>
              <div className="font-medium text-white text-sm">{a.name}</div>
              <div className="text-xs text-gray-500">{a.id.slice(0, 8)}…</div>
            </div>
            <div className="flex items-center gap-1.5 text-yellow-400">
              <Coins className="w-4 h-4" />
              <span className="font-mono font-bold">
                {(balances[a.id] ?? 0).toFixed(2)}
              </span>
              <span className="text-xs text-gray-500">GPU-credits</span>
            </div>
          </div>
        ))}
    </div>
  )
}
