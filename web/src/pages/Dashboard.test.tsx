import { beforeEach, describe, expect, it, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import Dashboard from './Dashboard'
import { api } from '../api'

vi.mock('../api', () => ({
  api: { getStats: vi.fn() },
}))

beforeEach(() => vi.clearAllMocks())

describe('Dashboard', () => {
  it('renders the stat tiles', async () => {
    vi.mocked(api.getStats).mockResolvedValue({
      chats_monitored: 3,
      messages_indexed: 120,
      today_matches: 4,
      bookmarks: 2,
      ignored: 1,
      can_send: true,
    })
    render(<Dashboard />)
    expect(await screen.findByText('3')).toBeInTheDocument()
    expect(screen.getByText('120')).toBeInTheDocument()
    expect(screen.queryByText(/resume broadcasting is unavailable/i)).not.toBeInTheDocument()
  })

  it('shows a warning banner when the active source cannot send', async () => {
    vi.mocked(api.getStats).mockResolvedValue({
      chats_monitored: 0,
      messages_indexed: 0,
      today_matches: 0,
      bookmarks: 0,
      ignored: 0,
      can_send: false,
    })
    render(<Dashboard />)
    expect(await screen.findByText(/resume broadcasting is unavailable/i)).toBeInTheDocument()
  })

  it('shows an error message when the request fails', async () => {
    vi.mocked(api.getStats).mockRejectedValue(new Error('network down'))
    render(<Dashboard />)
    expect(await screen.findByText('network down')).toBeInTheDocument()
  })
})
