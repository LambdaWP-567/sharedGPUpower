import { NetworkCapacity } from '../../lib/api'
import { Cpu, MemoryStick, Layers, Activity } from 'lucide-react'

interface Props {
  capacity: NetworkCapacity | null
}

export function NetworkCapacityCard({ capacity }: Props) {
  if (!capacity) return <div className="text-gray-400 text-sm">Loading capacity...</div>

  const stats = [
    { label: 'Online Agents', value: `${capacity.online_agents} / ${capacity.total_agents}`, icon: Activity, color: 'text-green-400' },
    { label: 'Total CPU Cores', value: capacity.total_cpu_cores, icon: Cpu, color: 'text-blue-400' },
    { label: 'Total RAM', value: `${capacity.total_ram_gb.toFixed(0)} GB`, icon: MemoryStick, color: 'text-purple-400' },
    { label: 'GPU Layers', value: capacity.total_gpu_layers, icon: Layers, color: 'text-yellow-400' },
  ]

  return (
    <div className="grid grid-cols-2 md:grid-cols-4 gap-4">
      {stats.map((s) => (
        <div key={s.label} className="bg-gray-900 border border-gray-800 rounded-xl p-4">
          <div className="flex items-center gap-2 mb-1">
            <s.icon className={`w-4 h-4 ${s.color}`} />
            <span className="text-xs text-gray-400">{s.label}</span>
          </div>
          <div className="text-2xl font-bold text-white">{s.value}</div>
        </div>
      ))}
    </div>
  )
}
