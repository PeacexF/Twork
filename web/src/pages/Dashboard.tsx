import { useEffect, useState } from 'react'
import { api } from '../api'
import type { Stats } from '../types'

export default function Dashboard() {
  const [stats, setStats] = useState<Stats | null>(null)
  const [error, setError] = useState('')

  useEffect(() => {
    api
      .getStats()
      .then(setStats)
      .catch((e: Error) => setError(e.message))
  }, [])

  if (error) return <p className="error">{error}</p>
  if (!stats) return <p className="muted">Loading...</p>

  const tiles: { label: string; value: number }[] = [
    { label: 'Chats monitored', value: stats.chats_monitored },
    { label: 'Messages indexed', value: stats.messages_indexed },
    { label: "Today's matches", value: stats.today_matches },
    { label: 'Bookmarks', value: stats.bookmarks },
    { label: 'Ignored', value: stats.ignored },
  ]

  return (
    <div>
      <div className="tile-grid">
        {tiles.map((t) => (
          <div className="tile" key={t.label}>
            <div className="tile-value">{t.value}</div>
            <div className="tile-label">{t.label}</div>
          </div>
        ))}
      </div>

      {!stats.can_send && (
        <div className="banner banner-warn">
          Resume broadcasting is unavailable with the current chat source (RSSHub is
          read-only). Switch <code>source.kind</code> to <code>mtproto</code> to enable it.
        </div>
      )}
    </div>
  )
}
