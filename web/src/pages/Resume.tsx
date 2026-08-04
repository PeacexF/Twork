import { useEffect, useState } from 'react'
import { api, ApiError } from '../api'

export default function Resume() {
  const [text, setText] = useState('')
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState('')
  const [saved, setSaved] = useState(false)

  useEffect(() => {
    api
      .getResumeText()
      .then((r) => setText(r.text))
      .catch((e: Error) => setError(e.message))
      .finally(() => setLoading(false))
  }, [])

  async function handleSave() {
    setSaving(true)
    setError('')
    setSaved(false)
    try {
      await api.setResumeText(text)
      setSaved(true)
    } catch (e) {
      setError(e instanceof ApiError ? e.message : 'Failed to save.')
    } finally {
      setSaving(false)
    }
  }

  if (loading) return <p className="muted">Loading...</p>

  return (
    <div className="resume-page">
      <p className="muted">
        This is the default pitch broadcast into any group without its own text override
        (set per chat under Chats -&gt; Broadcast settings).
      </p>
      <textarea
        className="resume-textarea"
        value={text}
        onChange={(e) => {
          setText(e.target.value)
          setSaved(false)
        }}
        rows={14}
        placeholder="Backend Go developer, 5 years experience, available for freelance work. Portfolio: ..."
      />
      {error && <p className="error">{error}</p>}
      <div className="resume-actions">
        <button onClick={handleSave} disabled={saving}>
          {saving ? 'Saving...' : 'Save'}
        </button>
        {saved && <span className="status-on">Saved</span>}
      </div>
    </div>
  )
}
