import { useEffect, useRef, useState } from 'react'
import { Agent, NetworkCapacity, fetchAgents, fetchCapacity } from '../lib/api'

export function useAgents(refreshInterval = 10_000) {
  const [agents, setAgents] = useState<Agent[]>([])
  const [capacity, setCapacity] = useState<NetworkCapacity | null>(null)
  const [error, setError] = useState<string | null>(null)
  const wsRef = useRef<WebSocket | null>(null)

  const load = async () => {
    try {
      const [a, c] = await Promise.all([fetchAgents(), fetchCapacity()])
      setAgents(a ?? [])
      setCapacity(c)
      setError(null)
    } catch (e) {
      setError(String(e))
    }
  }

  useEffect(() => {
    load()
    const interval = setInterval(load, refreshInterval)

    // WebSocket for live updates
    const protocol = window.location.protocol === 'https:' ? 'wss' : 'ws'
    const ws = new WebSocket(`${protocol}://${window.location.host}/ws`)
    wsRef.current = ws

    ws.onmessage = (evt) => {
      try {
        const msg = JSON.parse(evt.data)
        if (msg.event === 'agent_update' || msg.event === 'job_update') {
          load()
        }
      } catch {}
    }

    return () => {
      clearInterval(interval)
      ws.close()
    }
  }, [refreshInterval])

  return { agents, capacity, error, refresh: load }
}
