import { useState } from 'react'
import { Copy, RefreshCw, Check } from 'lucide-react'
import type { AuthUser } from '../../hooks/useAuth'

interface UserProfileProps {
  user: AuthUser
}

export function UserProfile({ user }: UserProfileProps) {
  const [apiKey, setApiKey] = useState(user.api_key)
  const [copied, setCopied] = useState(false)
  const [regenerating, setRegenerating] = useState(false)

  async function copyKey() {
    await navigator.clipboard.writeText(apiKey)
    setCopied(true)
    setTimeout(() => setCopied(false), 2000)
  }

  async function regenerateKey() {
    setRegenerating(true)
    try {
      const r = await fetch('/api/v1/users/me/regenerate-key', {
        method: 'POST',
        credentials: 'include',
      })
      if (r.ok) {
        const data = await r.json()
        setApiKey(data.api_key)
      }
    } finally {
      setRegenerating(false)
    }
  }

  return (
    <div className="space-y-6">
      <h2 className="text-xl font-bold text-white">Your Profile</h2>

      {/* User info */}
      <div className="bg-gray-900 border border-gray-800 rounded-xl p-5 flex items-center gap-4">
        {user.avatar_url ? (
          <img src={user.avatar_url} alt="" className="w-12 h-12 rounded-full" />
        ) : (
          <div className="w-12 h-12 rounded-full bg-brand-600 flex items-center justify-center text-white font-bold text-lg">
            {(user.display_name || user.email)[0].toUpperCase()}
          </div>
        )}
        <div>
          <div className="text-white font-semibold">{user.display_name || user.email}</div>
          <div className="text-gray-400 text-sm">{user.email}</div>
          <div className="flex gap-2 mt-1">
            <span
              className={`text-xs px-2 py-0.5 rounded-full ${
                user.role === 'admin'
                  ? 'bg-purple-900 text-purple-300'
                  : 'bg-gray-800 text-gray-400'
              }`}
            >
              {user.role}
            </span>
            <span
              className={`text-xs px-2 py-0.5 rounded-full ${
                user.status === 'active'
                  ? 'bg-green-900 text-green-300'
                  : 'bg-yellow-900 text-yellow-300'
              }`}
            >
              {user.status}
            </span>
          </div>
        </div>
      </div>

      {/* API Key */}
      <div className="bg-gray-900 border border-gray-800 rounded-xl p-5 space-y-3">
        <div>
          <h3 className="text-white font-medium">API Key</h3>
          <p className="text-gray-500 text-xs mt-1">
            Use this key in your agent config (<code>auth.user_api_key</code>)
          </p>
        </div>
        <div className="flex gap-2">
          <code className="flex-1 bg-gray-800 border border-gray-700 rounded-lg px-3 py-2 text-sm text-brand-300 font-mono truncate">
            {apiKey}
          </code>
          <button
            onClick={copyKey}
            className="flex items-center gap-1 text-xs bg-gray-800 hover:bg-gray-700 text-gray-300 px-3 py-2 rounded-lg border border-gray-700 transition-colors"
          >
            {copied ? <Check className="w-4 h-4 text-green-400" /> : <Copy className="w-4 h-4" />}
            {copied ? 'Copied' : 'Copy'}
          </button>
          <button
            onClick={regenerateKey}
            disabled={regenerating}
            className="flex items-center gap-1 text-xs bg-gray-800 hover:bg-gray-700 text-gray-300 px-3 py-2 rounded-lg border border-gray-700 transition-colors disabled:opacity-50"
          >
            <RefreshCw className={`w-4 h-4 ${regenerating ? 'animate-spin' : ''}`} />
            Regenerate
          </button>
        </div>
        <p className="text-yellow-600 text-xs">
          Regenerating the key will disconnect all agents using the old key.
        </p>
      </div>

      {/* Agent install instructions */}
      <div className="bg-gray-900 border border-gray-800 rounded-xl p-5 space-y-3">
        <h3 className="text-white font-medium">Agent Setup</h3>
        <p className="text-gray-400 text-sm">
          Run the installer and enter your API key when prompted:
        </p>
        <pre className="bg-gray-800 rounded-lg p-3 text-xs text-gray-300 overflow-x-auto">
          {`curl -fsSL https://github.com/lambdawp-567/sharedgpupower/releases/latest/download/install.sh | bash`}
        </pre>
      </div>
    </div>
  )
}
