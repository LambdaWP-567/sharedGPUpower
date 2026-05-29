import { useState, useEffect } from 'react'
import { CheckCircle, XCircle, Clock, Users, Server } from 'lucide-react'

interface UserRow {
  id: string
  email: string
  display_name: string
  role: string
  status: string
  created_at: string
}

interface AgentRow {
  id: string
  name: string
  user_id: string
  approval_status: string
  bench_score: number
  arch: string
  gpu_model: string
}

type Tab = 'pending-users' | 'pending-agents' | 'all-users'

async function apiPost(path: string) {
  const r = await fetch(path, { method: 'POST', credentials: 'include' })
  return r.ok
}

export function AdminPanel() {
  const [tab, setTab] = useState<Tab>('pending-users')
  const [pendingUsers, setPendingUsers] = useState<UserRow[]>([])
  const [pendingAgents, setPendingAgents] = useState<AgentRow[]>([])
  const [allUsers, setAllUsers] = useState<UserRow[]>([])

  function reload() {
    fetch('/api/v1/admin/users/pending', { credentials: 'include' })
      .then((r) => r.json())
      .then((d) => setPendingUsers(d ?? []))
      .catch(() => {})
    fetch('/api/v1/admin/agents/pending', { credentials: 'include' })
      .then((r) => r.json())
      .then((d) => setPendingAgents(d ?? []))
      .catch(() => {})
    fetch('/api/v1/admin/users', { credentials: 'include' })
      .then((r) => r.json())
      .then((d) => setAllUsers(d ?? []))
      .catch(() => {})
  }

  useEffect(() => {
    reload()
  }, [])

  const tabs: { id: Tab; label: string; count?: number }[] = [
    { id: 'pending-users', label: 'Pending Users', count: pendingUsers.length },
    { id: 'pending-agents', label: 'Pending Agents', count: pendingAgents.length },
    { id: 'all-users', label: 'All Users' },
  ]

  return (
    <div className="space-y-6">
      <h2 className="text-xl font-bold text-white">Admin Panel</h2>

      <div className="flex gap-2 border-b border-gray-800">
        {tabs.map((t) => (
          <button
            key={t.id}
            onClick={() => setTab(t.id)}
            className={`px-4 py-2 text-sm font-medium border-b-2 transition-colors ${
              tab === t.id
                ? 'border-brand-500 text-brand-400'
                : 'border-transparent text-gray-400 hover:text-white'
            }`}
          >
            {t.label}
            {t.count !== undefined && t.count > 0 && (
              <span className="ml-2 bg-brand-500 text-white text-xs rounded-full px-1.5 py-0.5">
                {t.count}
              </span>
            )}
          </button>
        ))}
      </div>

      {tab === 'pending-users' && (
        <div className="space-y-2">
          {pendingUsers.length === 0 ? (
            <p className="text-gray-500 text-sm py-4">No pending users</p>
          ) : (
            pendingUsers.map((u) => (
              <div
                key={u.id}
                className="bg-gray-900 border border-gray-800 rounded-lg p-4 flex items-center justify-between"
              >
                <div>
                  <div className="flex items-center gap-2">
                    <Users className="w-4 h-4 text-gray-400" />
                    <span className="text-white font-medium">{u.display_name || u.email}</span>
                  </div>
                  <div className="text-gray-400 text-xs mt-1">{u.email}</div>
                  <div className="text-gray-600 text-xs">
                    Registered {new Date(u.created_at).toLocaleDateString()}
                  </div>
                </div>
                <div className="flex gap-2">
                  <button
                    onClick={() =>
                      apiPost(`/api/v1/admin/users/${u.id}/approve`).then(reload)
                    }
                    className="flex items-center gap-1 text-xs bg-green-900 hover:bg-green-800 text-green-300 px-3 py-1.5 rounded-lg"
                  >
                    <CheckCircle className="w-3 h-3" />
                    Approve
                  </button>
                  <button
                    onClick={() =>
                      apiPost(`/api/v1/admin/users/${u.id}/disable`).then(reload)
                    }
                    className="flex items-center gap-1 text-xs bg-red-900 hover:bg-red-800 text-red-300 px-3 py-1.5 rounded-lg"
                  >
                    <XCircle className="w-3 h-3" />
                    Disable
                  </button>
                </div>
              </div>
            ))
          )}
        </div>
      )}

      {tab === 'pending-agents' && (
        <div className="space-y-2">
          {pendingAgents.length === 0 ? (
            <p className="text-gray-500 text-sm py-4">No pending agents</p>
          ) : (
            pendingAgents.map((a) => (
              <div
                key={a.id}
                className="bg-gray-900 border border-gray-800 rounded-lg p-4 flex items-center justify-between"
              >
                <div>
                  <div className="flex items-center gap-2">
                    <Server className="w-4 h-4 text-gray-400" />
                    <span className="text-white font-medium">{a.name}</span>
                    <span className="text-xs text-gray-500">{a.arch}</span>
                  </div>
                  <div className="text-gray-400 text-xs mt-1">GPU: {a.gpu_model || 'N/A'}</div>
                  <div className="text-gray-500 text-xs">
                    Bench score: {a.bench_score.toFixed(2)}
                  </div>
                </div>
                <div className="flex gap-2">
                  <button
                    onClick={() =>
                      apiPost(`/api/v1/admin/agents/${a.id}/approve`).then(reload)
                    }
                    className="flex items-center gap-1 text-xs bg-green-900 hover:bg-green-800 text-green-300 px-3 py-1.5 rounded-lg"
                  >
                    <CheckCircle className="w-3 h-3" />
                    Approve
                  </button>
                  <button
                    onClick={() =>
                      apiPost(`/api/v1/admin/agents/${a.id}/disable`).then(reload)
                    }
                    className="flex items-center gap-1 text-xs bg-red-900 hover:bg-red-800 text-red-300 px-3 py-1.5 rounded-lg"
                  >
                    <XCircle className="w-3 h-3" />
                    Disable
                  </button>
                </div>
              </div>
            ))
          )}
        </div>
      )}

      {tab === 'all-users' && (
        <div className="overflow-x-auto">
          <table className="w-full text-sm text-left">
            <thead>
              <tr className="text-gray-500 border-b border-gray-800">
                <th className="pb-2 pr-4">User</th>
                <th className="pb-2 pr-4">Role</th>
                <th className="pb-2 pr-4">Status</th>
                <th className="pb-2">Joined</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-gray-900">
              {allUsers.map((u) => (
                <tr key={u.id} className="text-gray-300">
                  <td className="py-2 pr-4">
                    <div className="font-medium text-white">{u.display_name || u.email}</div>
                    <div className="text-xs text-gray-500">{u.email}</div>
                  </td>
                  <td className="py-2 pr-4">
                    <span
                      className={`text-xs px-2 py-0.5 rounded-full ${
                        u.role === 'admin'
                          ? 'bg-purple-900 text-purple-300'
                          : 'bg-gray-800 text-gray-400'
                      }`}
                    >
                      {u.role}
                    </span>
                  </td>
                  <td className="py-2 pr-4">
                    <span
                      className={`flex items-center gap-1 text-xs ${
                        u.status === 'active'
                          ? 'text-green-400'
                          : u.status === 'pending'
                          ? 'text-yellow-400'
                          : 'text-red-400'
                      }`}
                    >
                      {u.status === 'active' ? (
                        <CheckCircle className="w-3 h-3" />
                      ) : u.status === 'pending' ? (
                        <Clock className="w-3 h-3" />
                      ) : (
                        <XCircle className="w-3 h-3" />
                      )}
                      {u.status}
                    </span>
                  </td>
                  <td className="py-2 text-gray-500 text-xs">
                    {new Date(u.created_at).toLocaleDateString()}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  )
}
