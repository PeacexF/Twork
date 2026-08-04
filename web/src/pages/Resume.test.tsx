import { beforeEach, describe, expect, it, vi } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import Resume from './Resume'
import { api, ApiError } from '../api'

vi.mock('../api', () => ({
  api: { getResumeText: vi.fn(), setResumeText: vi.fn() },
  ApiError: class ApiError extends Error {
    status: number
    constructor(status: number, message: string) {
      super(message)
      this.status = status
    }
  },
}))

beforeEach(() => {
  vi.clearAllMocks()
  vi.mocked(api.getResumeText).mockResolvedValue({ text: 'Existing pitch' })
  vi.mocked(api.setResumeText).mockResolvedValue(null)
})

describe('Resume', () => {
  it('loads and displays the current global text', async () => {
    render(<Resume />)
    expect(await screen.findByDisplayValue('Existing pitch')).toBeInTheDocument()
  })

  it('saves edited text and shows a confirmation', async () => {
    render(<Resume />)
    const textarea = await screen.findByDisplayValue('Existing pitch')

    fireEvent.change(textarea, { target: { value: 'Updated pitch' } })
    fireEvent.click(screen.getByRole('button', { name: /^save$/i }))

    await waitFor(() => expect(api.setResumeText).toHaveBeenCalledWith('Updated pitch'))
    expect(await screen.findByText('Saved')).toBeInTheDocument()
  })

  it('surfaces the ApiError message when saving fails', async () => {
    vi.mocked(api.setResumeText).mockRejectedValue(new ApiError(500, 'write failed'))
    render(<Resume />)
    await screen.findByDisplayValue('Existing pitch')

    fireEvent.click(screen.getByRole('button', { name: /^save$/i }))
    expect(await screen.findByText('write failed')).toBeInTheDocument()
  })

  it('falls back to a generic message for a non-ApiError failure', async () => {
    vi.mocked(api.setResumeText).mockRejectedValue(new Error('boom'))
    render(<Resume />)
    await screen.findByDisplayValue('Existing pitch')

    fireEvent.click(screen.getByRole('button', { name: /^save$/i }))
    expect(await screen.findByText('Failed to save.')).toBeInTheDocument()
  })
})
