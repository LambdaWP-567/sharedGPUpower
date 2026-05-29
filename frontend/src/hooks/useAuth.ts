import { useState, useEffect } from 'react'

export interface AuthUser {
  id: string
  email: string
  display_name: string
  avatar_url: string
  role: 'admin' | 'user'
  status: 'pending' | 'active' | 'disabled'
  api_key: string
}

interface UseAuthResult {
  user: AuthUser | null
  loading: boolean
  logout: () => void
}

export function useAuth(): UseAuthResult {
  const [user, setUser] = useState<AuthUser | null>(null)
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    fetch('/api/v1/users/me', { credentials: 'include' })
      .then((r) => (r.ok ? r.json() : null))
      .then((data) => setUser(data))
      .catch(() => setUser(null))
      .finally(() => setLoading(false))
  }, [])

  const logout = () => {
    window.location.href = '/auth/logout'
  }

  return { user, loading, logout }
}
