import { beforeEach, describe, expect, it, vi } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import App from './App'
import { clearCredentials } from './api'

vi.mock('./api', async (importOriginal) => {
  const actual = await importOriginal<typeof import('./api')>()
  return {
    ...actual,
    api: {
      getStats: vi.fn().mockResolvedValue({
        chats_monitored: 1,
        messages_indexed: 2,
        today_matches: 0,
        bookmarks: 0,
        ignored: 0,
        can_send: true,
      }),
      listChats: vi.fn().mockResolvedValue([]),
      getResumeText: vi.fn().mockResolvedValue({ text: '' }),
      getCompliance: vi.fn().mockResolvedValue({ min_delay_seconds: 300, max_per_hour: 10 }),
      addChat: vi.fn(),
      pauseChat: vi.fn(),
      resumeChat: vi.fn(),
      deleteChat: vi.fn(),
      setChatBroadcast: vi.fn(),
      setResumeText: vi.fn(),
      setCompliance: vi.fn(),
    },
  }
})

beforeEach(() => clearCredentials())

async function signIn() {
  fireEvent.change(screen.getByLabelText('Username'), { target: { value: 'admin' } })
  fireEvent.change(screen.getByLabelText('Password'), { target: { value: 'secret' } })
  fireEvent.click(screen.getByRole('button', { name: /sign in/i }))
  await waitFor(() => expect(screen.getByRole('heading', { name: 'Twork' })).toBeInTheDocument())
}

describe('App', () => {
  it('shows the login screen when there are no stored credentials', () => {
    render(<App />)
    expect(screen.getByText(/sign in with the credentials/i)).toBeInTheDocument()
  })

  it('shows the dashboard with tab navigation after a successful login', async () => {
    render(<App />)
    await signIn()
    expect(screen.getByRole('button', { name: 'Chats' })).toBeInTheDocument()
    expect(await screen.findByText('Chats monitored')).toBeInTheDocument()
  })

  it('switches tabs on click', async () => {
    render(<App />)
    await signIn()
    fireEvent.click(screen.getByRole('button', { name: 'Chats' }))
    expect(await screen.findByPlaceholderText(/username, invite link/i)).toBeInTheDocument()
  })

  it('signing out clears credentials and returns to the login screen', async () => {
    render(<App />)
    await signIn()
    fireEvent.click(screen.getByRole('button', { name: /sign out/i }))
    expect(screen.getByText(/sign in with the credentials/i)).toBeInTheDocument()
  })
})
