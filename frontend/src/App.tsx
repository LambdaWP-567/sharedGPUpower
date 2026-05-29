import { useState } from 'react'
import { useAgents } from './hooks/useAgents'
import { NetworkCapacityCard } from './components/NetworkCapacity/NetworkCapacity'
import { AgentTable } from './components/AgentTable/AgentTable'
import { JobSubmitForm } from './components/JobSubmitForm/JobSubmitForm'
import { TokenBalance } from './components/TokenBalance/TokenBalance'
import { JobHistory } from './components/JobHistory/JobHistory'
import { Zap, RefreshCw } from 'lucide-react'

export default function App() {
  const { agents, capacity, error, refresh } = useAgents(10_000)
  const [jobRefresh, setJobRefresh] = useState(0)

  return (
    <div className="min-h-screen bg-gray-950">
      {/* Header */}
      <header className="border-b border-gray-800 bg-gray-950 sticky top-0 z-10">
        <div className="max-w-7xl mx-auto px-4 py-4 flex items-center justify-between">
          <div className="flex items-center gap-2">
            <Zap className="w-6 h-6 text-brand-500" />
            <span className="text-xl font-bold text-white">sharedGPUpower</span>
            <span className="text-xs text-gray-500 ml-2">distributed compute network</span>
          </div>
          <button onClick={refresh} className="text-gray-400 hover:text-white transition-colors">
            <RefreshCw className="w-4 h-4" />
          </button>
        </div>
      </header>

      <main className="max-w-7xl mx-auto px-4 py-8 space-y-8">
        {error && (
          <div className="bg-red-950 border border-red-800 text-red-400 rounded-lg px-4 py-3 text-sm">
            Backend unreachable: {error}
          </div>
        )}

        {/* Network capacity */}
        <section>
          <h2 className="text-sm font-semibold text-gray-400 uppercase tracking-wider mb-3">Network Capacity</h2>
          <NetworkCapacityCard capacity={capacity} />
        </section>

        {/* Agents */}
        <section>
          <h2 className="text-sm font-semibold text-gray-400 uppercase tracking-wider mb-3">
            Agents ({agents.length})
          </h2>
          <div className="bg-gray-950 border border-gray-800 rounded-xl p-4">
            <AgentTable agents={agents} />
          </div>
        </section>

        <div className="grid md:grid-cols-2 gap-8">
          {/* Submit job */}
          <section>
            <h2 className="text-sm font-semibold text-gray-400 uppercase tracking-wider mb-3">Submit Job</h2>
            <div className="bg-gray-950 border border-gray-800 rounded-xl p-4">
              <JobSubmitForm agents={agents} onSubmitted={() => setJobRefresh((n) => n + 1)} />
            </div>
          </section>

          {/* Token balances */}
          <section>
            <h2 className="text-sm font-semibold text-gray-400 uppercase tracking-wider mb-3">Token Balances</h2>
            <TokenBalance agents={agents} />
          </section>
        </div>

        {/* Job history */}
        <section>
          <h2 className="text-sm font-semibold text-gray-400 uppercase tracking-wider mb-3">Job History</h2>
          <div className="bg-gray-950 border border-gray-800 rounded-xl p-4">
            <JobHistory agents={agents} refreshTrigger={jobRefresh} />
          </div>
        </section>
      </main>
    </div>
  )
}
