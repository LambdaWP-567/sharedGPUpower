import { useState } from 'react'
import { submitJob, Agent } from '../../lib/api'
import { Send } from 'lucide-react'

interface Props {
  agents: Agent[]
  onSubmitted: (jobId: string) => void
}

export function JobSubmitForm({ agents, onSubmitted }: Props) {
  const [agentId, setAgentId] = useState('')
  const [model, setModel] = useState('llama3.2:3b')
  const [prompt, setPrompt] = useState('')
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const onlineAgents = agents.filter((a) => a.status === 'online')

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!agentId || !prompt) return
    setLoading(true)
    setError(null)
    try {
      const res = await submitJob({ submitter_agent_id: agentId, model, prompt })
      onSubmitted(res.job_id)
      setPrompt('')
    } catch (e) {
      setError(String(e))
    } finally {
      setLoading(false)
    }
  }

  return (
    <form onSubmit={handleSubmit} className="space-y-4">
      <div className="flex gap-3">
        <div className="flex-1">
          <label className="block text-xs text-gray-400 mb-1">Submitting Agent (your ID)</label>
          <select
            className="w-full bg-gray-900 border border-gray-700 rounded-lg px-3 py-2 text-sm text-white"
            value={agentId}
            onChange={(e) => setAgentId(e.target.value)}
            required
          >
            <option value="">Select agent...</option>
            {onlineAgents.map((a) => (
              <option key={a.id} value={a.id}>{a.name}</option>
            ))}
          </select>
        </div>
        <div>
          <label className="block text-xs text-gray-400 mb-1">Model</label>
          <input
            className="bg-gray-900 border border-gray-700 rounded-lg px-3 py-2 text-sm text-white w-40"
            value={model}
            onChange={(e) => setModel(e.target.value)}
            placeholder="llama3.2:3b"
          />
        </div>
      </div>

      <div>
        <label className="block text-xs text-gray-400 mb-1">Prompt</label>
        <textarea
          className="w-full bg-gray-900 border border-gray-700 rounded-lg px-3 py-2 text-sm text-white h-24 resize-none"
          value={prompt}
          onChange={(e) => setPrompt(e.target.value)}
          placeholder="Enter your prompt. Add [OFFLOAD] to force network routing."
          required
        />
      </div>

      {error && <div className="text-red-400 text-sm">{error}</div>}

      <button
        type="submit"
        disabled={loading || !agentId}
        className="flex items-center gap-2 bg-brand-500 hover:bg-brand-600 disabled:opacity-50 text-white px-4 py-2 rounded-lg text-sm font-medium transition-colors"
      >
        <Send className="w-4 h-4" />
        {loading ? 'Submitting...' : 'Submit Job'}
      </button>
    </form>
  )
}
