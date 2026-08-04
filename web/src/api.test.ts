import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { api, ApiError, clearCredentials, hasCredentials, setCredentials } from './api'

function jsonResponse(status: number, body: unknown): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'Content-Type': 'application/json' },
  })
}

describe('credentials', () => {
  afterEach(() => clearCredentials())

  it('reports no credentials by default', () => {
    expect(hasCredentials()).toBe(false)
  })

  it('reports credentials once set', () => {
    setCredentials('admin', 'secret')
    expect(hasCredentials()).toBe(true)
  })

  it('forgets credentials on clear', () => {
    setCredentials('admin', 'secret')
    clearCredentials()
    expect(hasCredentials()).toBe(false)
  })
})

describe('api requests', () => {
  let fetchMock: ReturnType<typeof vi.fn>

  beforeEach(() => {
    setCredentials('admin', 'secret')
    fetchMock = vi.fn()
    vi.stubGlobal('fetch', fetchMock)
  })

  afterEach(() => {
    vi.unstubAllGlobals()
    clearCredentials()
  })

  it('attaches a Basic Auth header built from the stored credentials', async () => {
    fetchMock.mockResolvedValueOnce(
      jsonResponse(200, {
        chats_monitored: 0,
        messages_indexed: 0,
        today_matches: 0,
        bookmarks: 0,
        ignored: 0,
        can_send: true,
      }),
    )

    await api.getStats()

    expect(fetchMock).toHaveBeenCalledTimes(1)
    const [path, init] = fetchMock.mock.calls[0] as [string, RequestInit]
    expect(path).toBe('/api/stats')
    const headers = init.headers as Record<string, string>
    expect(headers.Authorization).toBe(`Basic ${btoa('admin:secret')}`)
  })

  it('sends the request body as JSON', async () => {
    fetchMock.mockResolvedValueOnce(new Response(null, { status: 204 }))
    await api.setResumeText('my pitch')

    const [, init] = fetchMock.mock.calls[0] as [string, RequestInit]
    expect(init.method).toBe('PUT')
    expect(init.body).toBe(JSON.stringify({ text: 'my pitch' }))
  })

  it('returns null for a 204 response', async () => {
    fetchMock.mockResolvedValueOnce(new Response(null, { status: 204 }))
    const result = await api.pauseChat(1)
    expect(result).toBeNull()
  })

  it('clears credentials and throws an ApiError on 401', async () => {
    fetchMock.mockResolvedValueOnce(new Response(null, { status: 401 }))
    await expect(api.getStats()).rejects.toBeInstanceOf(ApiError)
    expect(hasCredentials()).toBe(false)
  })

  it('surfaces the server error message on failure', async () => {
    fetchMock.mockResolvedValueOnce(
      jsonResponse(400, { error: 'resume broadcasting only works in groups' }),
    )
    await expect(
      api.setChatBroadcast(1, { enabled: true, interval_seconds: 60, text: '' }),
    ).rejects.toThrow('resume broadcasting only works in groups')
  })

  it('falls back to a generic message for a non-JSON error body', async () => {
    fetchMock.mockResolvedValueOnce(new Response('boom', { status: 500 }))
    await expect(api.getStats()).rejects.toThrow('Request failed (500)')
  })
})
