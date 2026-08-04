import { useState, type FormEvent } from 'react'
import { setCredentials, clearCredentials } from '../api'

interface Props {
  onSuccess: () => void
  verify: () => Promise<unknown>
}

// asks for the username/password from config.yaml's web: block, verifies
// them with a real request (so a typo shows up immediately, not on the
// next click), then hands off to onSuccess
export default function Login({ onSuccess, verify }: Props) {
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)

  async function handleSubmit(e: FormEvent) {
    e.preventDefault()
    setError('')
    setBusy(true)
    setCredentials(username, password)
    try {
      await verify()
      onSuccess()
    } catch {
      clearCredentials()
      setError('Wrong username or password.')
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="login-screen">
      <form className="login-card" onSubmit={handleSubmit}>
        <h1>Twork</h1>
        <p className="muted">Sign in with the credentials from your config's web: block.</p>
        <label>
          Username
          <input
            value={username}
            onChange={(e) => setUsername(e.target.value)}
            autoFocus
            autoComplete="username"
          />
        </label>
        <label>
          Password
          <input
            type="password"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            autoComplete="current-password"
          />
        </label>
        {error && <p className="error">{error}</p>}
        <button type="submit" disabled={busy || !username || !password}>
          {busy ? 'Signing in...' : 'Sign in'}
        </button>
      </form>
    </div>
  )
}
