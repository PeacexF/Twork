import { useEffect, useState } from 'react'
import { api, ApiError } from '../api'

const RECOMMENDED_MIN_DELAY = 300
const RECOMMENDED_MAX_PER_HOUR = 10

export default function Compliance() {
  const [minDelay, setMinDelay] = useState('')
  const [maxPerHour, setMaxPerHour] = useState('')
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState('')
  const [saved, setSaved] = useState(false)

  useEffect(() => {
    api
      .getCompliance()
      .then((c) => {
        setMinDelay(String(c.min_delay_seconds))
        setMaxPerHour(String(c.max_per_hour))
      })
      .catch((e: Error) => setError(e.message))
      .finally(() => setLoading(false))
  }, [])

  const minDelayNum = parseInt(minDelay, 10)
  const maxPerHourNum = parseInt(maxPerHour, 10)
  const belowRecommended =
    (!Number.isNaN(minDelayNum) && minDelayNum < RECOMMENDED_MIN_DELAY) ||
    (!Number.isNaN(maxPerHourNum) && maxPerHourNum > RECOMMENDED_MAX_PER_HOUR)

  async function handleSave() {
    setSaving(true)
    setError('')
    setSaved(false)
    try {
      await api.setCompliance({ min_delay_seconds: minDelayNum, max_per_hour: maxPerHourNum })
      setSaved(true)
    } catch (e) {
      setError(e instanceof ApiError ? e.message : 'Failed to save.')
    } finally {
      setSaving(false)
    }
  }

  if (loading) return <p className="muted">Loading...</p>

  return (
    <div className="compliance-page">
      <div className="banner banner-warn">
        These limits exist to keep your account from getting rate-limited or permanently
        banned for spam-like behavior. Lowering the delay or raising the hourly cap is{' '}
        <strong>not recommended</strong>, even on a Telegram Premium account.
      </div>

      <label className="field">
        Minimum delay between two sends into the same group (seconds)
        <input
          value={minDelay}
          onChange={(e) => {
            setMinDelay(e.target.value)
            setSaved(false)
          }}
          inputMode="numeric"
        />
      </label>

      <label className="field">
        Maximum sends across all groups combined, per rolling hour
        <input
          value={maxPerHour}
          onChange={(e) => {
            setMaxPerHour(e.target.value)
            setSaved(false)
          }}
          inputMode="numeric"
        />
      </label>

      {belowRecommended && (
        <p className="error">
          These values are more aggressive than the recommended {RECOMMENDED_MIN_DELAY}s /{' '}
          {RECOMMENDED_MAX_PER_HOUR}-per-hour defaults.
        </p>
      )}
      {error && <p className="error">{error}</p>}

      <div className="resume-actions">
        <button
          onClick={handleSave}
          disabled={saving || !minDelay || !maxPerHour}
        >
          {saving ? 'Saving...' : 'Save'}
        </button>
        {saved && <span className="status-on">Saved</span>}
      </div>
    </div>
  )
}
