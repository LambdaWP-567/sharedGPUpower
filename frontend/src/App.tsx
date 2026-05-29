import { useState } from 'react'
import { useAgents } from './hooks/useAgents'
import { useAuth } from './hooks/useAuth'
import { NetworkCapacityCard } from './components/NetworkCapacity/NetworkCapacity'
import { AgentTable } from './components/AgentTable/AgentTable'
import { JobSubmitForm } from './components/JobSubmitForm/JobSubmitForm'
import { TokenBalance } from './components/TokenBalance/TokenBalance'
import { JobHistory } from './components/JobHistory/JobHistory'
import { LoginPage } from './components/LoginPage/LoginPage'
import { AdminPanel } from './components/AdminPanel/AdminPanel'
import { UserProfile } from './components/UserProfile/UserProfile'
import { Zap, RefreshCw, Shield, User, LogOut } from 'lucide-react'

type View = 'dashboard' | 'admin' | 'profile'

export default function App() {
  const { agents, capacity, error, refresh } = useAgents(10_000)
  const { user, loading: authLoading, logout } = useAuth()
  const [jobRefresh, setJobRefresh] = useState(0)
  const [view, setView] = useState<View>('dashboard')

  if (authLoading) {
    return (
      <div className="min-h-screen bg-gray-950 flex items-center justify-center">
        <div className="text-gray-400 text-sm">Loading...</div>
      </div>
    )
  }

  if (!user) {
    return <LoginPage />
  }

  if (user.status === 'pending') {
    return <LoginPage isPending />
  }

  return (
    <div className="min-h-screen bg-gray-950">
      {/* Header */}
      <header className="border-b border-gray-800 bg-gray-950 sticky top-0 z-10">
        <div className="max-w-7xl mx-auto px-4 py-4 flex items-center justify-between">
          <div className="flex items-center gap-2">
            <button
              onClick={() => setView('dashboard')}
              className="flex items-center gap-2 hover:opacity-80 transition-opacity"
            >
              <Zap className="w-6 h-6 text-brand-500" />
              <span className="text-xl font-bold text-white">sharedGPUpower</span>
            </button>
            <span className="text-xs text-gray-500 ml-2">distributed compute network</span>
          </div>

          <div className="flex items-center gap-3">
            <button
              onClick={refresh}
              className="text-gray-400 hover:text-white transition-colors"
              title="Refresh"
            >
              <RefreshCw className="w-4 h-4" />
            </button>

            {user.role === 'admin' && (
              <button
                onClick={() => setView('admin')}
                className={`flex items-center gap-1.5 text-sm px-3 py-1.5 rounded-lg transition-colors ${
                  view === 'admin'
                    ? 'bg-purple-900 text-purple-300'
                    : 'text-gray-400 hover:text-white hover:bg-gray-800'
                }`}
              >
                <Shield className="w-4 h-4" />
                Admin
              </button>
            )}

            <button
              onClick={() => setView('profile')}
              className={`flex items-center gap-1.5 text-sm px-3 py-1.5 rounded-lg transition-colors ${
                view === 'profile'
                  ? 'bg-gray-800 text-white'
                  : 'text-gray-400 hover:text-white hover:bg-gray-800'
              }`}
            >
              {user.avatar_url ? (
                <img src={user.avatar_url} alt="" className="w-5 h-5 rounded-full" />
              ) : (
                <User className="w-4 h-4" />
              )}
              <span className="hidden sm:inline">{user.display_name || user.email}</span>
            </button>

            <button
              onClick={logout}
              className="text-gray-500 hover:text-gray-300 transition-colors"
              title="Logout"
            >
              <LogOut className="w-4 h-4" />
            </button>
          </div>
        </div>
      </header>

      <main className="max-w-7xl mx-auto px-4 py-8">
        {view === 'admin' && user.role === 'admin' && <AdminPanel />}

        {view === 'profile' && <UserProfile user={user} />}

        {view === 'dashboard' && (
          <div className="space-y-8">
            {error && (
              <div className="bg-red-950 border border-red-800 text-red-400 rounded-lg px-4 py-3 text-sm">
                Backend unreachable: {error}
              </div>
            )}

            <section>
              <h2 className="text-sm font-semibold text-gray-400 uppercase tracking-wider mb-3">
                Network Capacity
              </h2>
              <NetworkCapacityCard capacity={capacity} />
            </section>

            <section>
              <h2 className="text-sm font-semibold text-gray-400 uppercase tracking-wider mb-3">
                Agents ({agents.length})
              </h2>
              <div className="bg-gray-950 border border-gray-800 rounded-xl p-4">
                <AgentTable agents={agents} />
              </div>
            </section>

            <div className="grid md:grid-cols-2 gap-8">
              <section>
                <h2 className="text-sm font-semibold text-gray-400 uppercase tracking-wider mb-3">
                  Submit Job
                </h2>
                <div className="bg-gray-950 border border-gray-800 rounded-xl p-4">
                  <JobSubmitForm agents={agents} onSubmitted={() => setJobRefresh((n) => n + 1)} />
                </div>
              </section>

              <section>
                <h2 className="text-sm font-semibold text-gray-400 uppercase tracking-wider mb-3">
                  Token Balances
                </h2>
                <TokenBalance agents={agents} />
              </section>
            </div>

            <section>
              <h2 className="text-sm font-semibold text-gray-400 uppercase tracking-wider mb-3">
                Job History
              </h2>
              <div className="bg-gray-950 border border-gray-800 rounded-xl p-4">
                <JobHistory agents={agents} refreshTrigger={jobRefresh} />
              </div>
            </section>
          </div>
        )}
      </main>
    </div>
  )
}
