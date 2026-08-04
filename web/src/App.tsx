import { useState } from 'react'
import { api, clearCredentials, hasCredentials } from './api'
import Login from './components/Login'
import Dashboard from './pages/Dashboard'
import Chats from './pages/Chats'
import Resume from './pages/Resume'
import Compliance from './pages/Compliance'

const TABS = ['Dashboard', 'Chats', 'Resume', 'Compliance'] as const
type Tab = (typeof TABS)[number]

export default function App() {
  const [authed, setAuthed] = useState(hasCredentials())
  const [tab, setTab] = useState<Tab>('Dashboard')

  if (!authed) {
    return <Login onSuccess={() => setAuthed(true)} verify={() => api.getStats()} />
  }

  function signOut() {
    clearCredentials()
    setAuthed(false)
  }

  return (
    <div className="app">
      <header className="app-header">
        <h1>Twork</h1>
        <nav className="tabs">
          {TABS.map((t) => (
            <button
              key={t}
              className={t === tab ? 'tab active' : 'tab'}
              onClick={() => setTab(t)}
            >
              {t}
            </button>
          ))}
        </nav>
        <button className="sign-out" onClick={signOut}>
          Sign out
        </button>
      </header>

      <main className="app-main">
        {tab === 'Dashboard' && <Dashboard />}
        {tab === 'Chats' && <Chats />}
        {tab === 'Resume' && <Resume />}
        {tab === 'Compliance' && <Compliance />}
      </main>
    </div>
  )
}
