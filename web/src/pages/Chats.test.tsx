import { beforeEach, describe, expect, it, vi } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import Chats from './Chats'
import { api } from '../api'
import type { Chat } from '../types'

vi.mock('../api', () => ({
  api: {
    listChats: vi.fn(),
    addChat: vi.fn(),
    pauseChat: vi.fn(),
    resumeChat: vi.fn(),
    deleteChat: vi.fn(),
    setChatBroadcast: vi.fn(),
  },
  ApiError: class ApiError extends Error {
    status: number
    constructor(status: number, message: string) {
      super(message)
      this.status = status
    }
  },
}))

const group: Chat = {
  telegram_id: 1,
  title: 'Go Freelance Jobs',
  username: 'go_freelance',
  tag: '',
  kind: 'group',
  paused: false,
  resume_enabled: false,
  resume_interval_seconds: 0,
  resume_text: '',
}

const channel: Chat = {
  telegram_id: 2,
  title: 'Vacancy Blast',
  username: '',
  tag: '',
  kind: 'channel',
  paused: false,
  resume_enabled: false,
  resume_interval_seconds: 0,
  resume_text: '',
}

beforeEach(() => {
  vi.clearAllMocks()
  vi.mocked(api.listChats).mockResolvedValue([group, channel])
  vi.mocked(api.pauseChat).mockResolvedValue(null)
  vi.mocked(api.resumeChat).mockResolvedValue(null)
  vi.mocked(api.deleteChat).mockResolvedValue(null)
  vi.mocked(api.setChatBroadcast).mockResolvedValue(null)
  vi.mocked(api.addChat).mockResolvedValue(group)
})

describe('Chats', () => {
  it('shows an empty state with no chats', async () => {
    vi.mocked(api.listChats).mockResolvedValue([])
    render(<Chats />)
    expect(await screen.findByText(/nothing monitored yet/i)).toBeInTheDocument()
  })

  it('lists chats and offers broadcast settings only for the group', async () => {
    render(<Chats />)
    expect(await screen.findByText('Go Freelance Jobs')).toBeInTheDocument()
    expect(screen.getByText('Vacancy Blast')).toBeInTheDocument()

    // one "Broadcast settings" button (the group), one "can't be broadcast to" note (the channel)
    expect(screen.getAllByRole('button', { name: /broadcast settings/i })).toHaveLength(1)
    expect(screen.getByText(/channels can't be broadcast to/i)).toBeInTheDocument()
  })

  it('adds a chat from the input field', async () => {
    render(<Chats />)
    await screen.findByText('Go Freelance Jobs')

    fireEvent.change(screen.getByPlaceholderText(/username, invite link/i), {
      target: { value: '@new_channel' },
    })
    fireEvent.click(screen.getByRole('button', { name: /^add chat$/i }))

    await waitFor(() => expect(api.addChat).toHaveBeenCalledWith('@new_channel'))
    expect(api.listChats).toHaveBeenCalledTimes(2) // initial load + refresh after add
  })

  it('pauses and resumes a chat', async () => {
    render(<Chats />)
    const pauseButtons = await screen.findAllByRole('button', { name: /pause monitoring/i })

    fireEvent.click(pauseButtons[0])
    await waitFor(() => expect(api.pauseChat).toHaveBeenCalledWith(1))
  })

  it('removes a chat after confirmation', async () => {
    vi.stubGlobal('confirm', vi.fn(() => true))
    render(<Chats />)
    const removeButtons = await screen.findAllByRole('button', { name: /remove/i })

    fireEvent.click(removeButtons[0])
    await waitFor(() => expect(api.deleteChat).toHaveBeenCalledWith(1))
    vi.unstubAllGlobals()
  })

  it('does not remove a chat when the confirmation is declined', async () => {
    vi.stubGlobal('confirm', vi.fn(() => false))
    render(<Chats />)
    const removeButtons = await screen.findAllByRole('button', { name: /remove/i })

    fireEvent.click(removeButtons[0])
    await new Promise((r) => setTimeout(r, 0))
    expect(api.deleteChat).not.toHaveBeenCalled()
    vi.unstubAllGlobals()
  })

  it('toggles broadcasting on for a group', async () => {
    render(<Chats />)
    fireEvent.click(await screen.findByRole('button', { name: /broadcast settings/i }))
    fireEvent.click(screen.getByRole('button', { name: /turn broadcasting on/i }))

    await waitFor(() =>
      expect(api.setChatBroadcast).toHaveBeenCalledWith(1, {
        enabled: true,
        interval_seconds: 0,
        text: '',
      }),
    )
  })
})
