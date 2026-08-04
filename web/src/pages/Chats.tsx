import { useEffect, useState, type FormEvent } from 'react'
import { api, ApiError } from '../api'
import type { Chat } from '../types'

export default function Chats() {
  const [chats, setChats] = useState<Chat[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [addInput, setAddInput] = useState('')
  const [adding, setAdding] = useState(false)

  function load() {
    setLoading(true)
    api
      .listChats()
      .then((c) => {
        setChats(c)
        setError('')
      })
      .catch((e: Error) => setError(e.message))
      .finally(() => setLoading(false))
  }

  useEffect(load, [])

  async function handleAdd(e: FormEvent) {
    e.preventDefault()
    if (!addInput.trim()) return
    setAdding(true)
    setError('')
    try {
      await api.addChat(addInput.trim())
      setAddInput('')
      load()
    } catch (e) {
      setError(e instanceof ApiError ? e.message : 'Failed to add chat.')
    } finally {
      setAdding(false)
    }
  }

  return (
    <div>
      <form className="add-chat-form" onSubmit={handleAdd}>
        <input
          value={addInput}
          onChange={(e) => setAddInput(e.target.value)}
          placeholder="@username, invite link, or folder link"
        />
        <button type="submit" disabled={adding || !addInput.trim()}>
          {adding ? 'Adding...' : 'Add chat'}
        </button>
      </form>

      {error && <p className="error">{error}</p>}
      {loading && <p className="muted">Loading...</p>}

      {!loading && chats.length === 0 && (
        <p className="muted">Nothing monitored yet -- add a channel or group above.</p>
      )}

      <div className="chat-list">
        {chats.map((c) => (
          <ChatRow key={c.telegram_id} chat={c} onChange={load} />
        ))}
      </div>
    </div>
  )
}

function ChatRow({ chat, onChange }: { chat: Chat; onChange: () => void }) {
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const [expanded, setExpanded] = useState(false)
  const [intervalMinutes, setIntervalMinutes] = useState(
    chat.resume_interval_seconds > 0 ? String(Math.round(chat.resume_interval_seconds / 60)) : '',
  )
  const [text, setText] = useState(chat.resume_text)

  async function run(action: () => Promise<unknown>) {
    setBusy(true)
    setError('')
    try {
      await action()
      onChange()
    } catch (e) {
      setError(e instanceof ApiError ? e.message : 'Request failed.')
    } finally {
      setBusy(false)
    }
  }

  function togglePause() {
    run(() => (chat.paused ? api.resumeChat(chat.telegram_id) : api.pauseChat(chat.telegram_id)))
  }

  function remove() {
    if (!confirm(`Stop monitoring "${chat.title}"? Already-indexed messages are kept.`)) return
    run(() => api.deleteChat(chat.telegram_id))
  }

  function toggleBroadcast() {
    run(() =>
      api.setChatBroadcast(chat.telegram_id, {
        enabled: !chat.resume_enabled,
        interval_seconds: chat.resume_interval_seconds,
        text: chat.resume_text,
      }),
    )
  }

  function saveInterval() {
    const minutes = parseInt(intervalMinutes, 10)
    if (!minutes || minutes <= 0) return
    run(() =>
      api.setChatBroadcast(chat.telegram_id, {
        enabled: chat.resume_enabled,
        interval_seconds: minutes * 60,
        text: chat.resume_text,
      }),
    )
  }

  function saveText() {
    run(() =>
      api.setChatBroadcast(chat.telegram_id, {
        enabled: chat.resume_enabled,
        interval_seconds: chat.resume_interval_seconds,
        text,
      }),
    )
  }

  return (
    <div className="chat-row">
      <div className="chat-row-main">
        <div className="chat-row-title">
          <span className={`badge badge-${chat.kind}`}>{chat.kind}</span>
          {chat.title}
          {chat.username && <span className="muted"> @{chat.username}</span>}
          {chat.tag && <span className="tag">{chat.tag}</span>}
        </div>
        <div className="chat-row-actions">
          <button onClick={togglePause} disabled={busy}>
            {chat.paused ? 'Resume monitoring' : 'Pause monitoring'}
          </button>
          {chat.kind === 'group' && (
            <button onClick={() => setExpanded((v) => !v)} disabled={busy}>
              {expanded ? 'Hide broadcast settings' : 'Broadcast settings'}
            </button>
          )}
          <button onClick={remove} disabled={busy} className="danger">
            Remove
          </button>
        </div>
      </div>

      {error && <p className="error">{error}</p>}

      {chat.kind !== 'group' && (
        <p className="muted small">Channels can't be broadcast to -- only groups.</p>
      )}

      {chat.kind === 'group' && expanded && (
        <div className="broadcast-panel">
          <div className="broadcast-status">
            <button onClick={toggleBroadcast} disabled={busy}>
              {chat.resume_enabled ? 'Turn broadcasting OFF' : 'Turn broadcasting ON'}
            </button>
            <span className={chat.resume_enabled ? 'status-on' : 'status-off'}>
              {chat.resume_enabled ? 'Enabled' : 'Disabled'}
            </span>
          </div>

          <label className="inline-field">
            Interval (minutes)
            <input
              value={intervalMinutes}
              onChange={(e) => setIntervalMinutes(e.target.value)}
              onBlur={saveInterval}
              inputMode="numeric"
              placeholder="uses the compliance minimum"
            />
          </label>

          <label className="field">
            Text override for this chat (blank = use the global resume text)
            <textarea
              value={text}
              onChange={(e) => setText(e.target.value)}
              onBlur={saveText}
              rows={3}
              placeholder="Leave blank to use the global resume text"
            />
          </label>
        </div>
      )}
    </div>
  )
}
