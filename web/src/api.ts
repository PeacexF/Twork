// thin fetch wrapper: attaches Basic Auth, unwraps JSON, and normalizes errors
import type { Chat, ChatBroadcastConfig, Compliance, ResumeText, Stats } from './types'

const STORAGE_KEY = 'twork.auth'

export class ApiError extends Error {
  status: number
  constructor(status: number, message: string) {
    super(message)
    this.status = status
  }
}

function authHeader(): string | null {
  const creds = sessionStorage.getItem(STORAGE_KEY)
  return creds ? `Basic ${creds}` : null
}

export function setCredentials(username: string, password: string): void {
  sessionStorage.setItem(STORAGE_KEY, btoa(`${username}:${password}`))
}

export function clearCredentials(): void {
  sessionStorage.removeItem(STORAGE_KEY)
}

export function hasCredentials(): boolean {
  return authHeader() !== null
}

async function request<T>(method: string, path: string, body?: unknown): Promise<T> {
  const headers: Record<string, string> = { 'Content-Type': 'application/json' }
  const auth = authHeader()
  if (auth) headers.Authorization = auth

  const res = await fetch(path, {
    method,
    headers,
    body: body !== undefined ? JSON.stringify(body) : undefined,
  })

  if (res.status === 401) {
    clearCredentials()
    throw new ApiError(401, 'Session expired -- please sign in again.')
  }
  if (!res.ok) {
    let message = `Request failed (${res.status})`
    try {
      const data = await res.json()
      if (data && data.error) message = data.error
    } catch {
      // non-JSON error body -- keep the generic message
    }
    throw new ApiError(res.status, message)
  }
  if (res.status === 204) return null as T
  return res.json() as Promise<T>
}

export const api = {
  getStats: () => request<Stats>('GET', '/api/stats'),

  listChats: () => request<Chat[]>('GET', '/api/chats'),
  addChat: (input: string) => request<Chat | Chat[]>('POST', '/api/chats', { input }),
  pauseChat: (id: number) => request<null>('POST', `/api/chats/${id}/pause`),
  resumeChat: (id: number) => request<null>('POST', `/api/chats/${id}/resume`),
  deleteChat: (id: number) => request<null>('DELETE', `/api/chats/${id}`),
  setChatBroadcast: (id: number, cfg: ChatBroadcastConfig) =>
    request<null>('PATCH', `/api/chats/${id}/broadcast`, cfg),

  getResumeText: () => request<ResumeText>('GET', '/api/resume'),
  setResumeText: (text: string) => request<null>('PUT', '/api/resume', { text }),

  getCompliance: () => request<Compliance>('GET', '/api/compliance'),
  setCompliance: (cfg: Compliance) => request<null>('PUT', '/api/compliance', cfg),
}
