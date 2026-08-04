import { beforeEach, describe, expect, it, vi } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import Compliance from './Compliance'
import { api, ApiError } from '../api'

vi.mock('../api', () => ({
  api: { getCompliance: vi.fn(), setCompliance: vi.fn() },
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
  vi.mocked(api.getCompliance).mockResolvedValue({ min_delay_seconds: 300, max_per_hour: 10 })
  vi.mocked(api.setCompliance).mockResolvedValue(null)
})

describe('Compliance', () => {
  it('loads the current limits', async () => {
    render(<Compliance />)
    expect(await screen.findByDisplayValue('300')).toBeInTheDocument()
    expect(screen.getByDisplayValue('10')).toBeInTheDocument()
    expect(screen.queryByText(/more aggressive than the recommended/i)).not.toBeInTheDocument()
  })

  it('warns when a value is more aggressive than recommended', async () => {
    render(<Compliance />)
    const minDelayInput = await screen.findByDisplayValue('300')
    fireEvent.change(minDelayInput, { target: { value: '30' } })
    expect(await screen.findByText(/more aggressive than the recommended/i)).toBeInTheDocument()
  })

  it('saves the edited limits', async () => {
    render(<Compliance />)
    const minDelayInput = await screen.findByDisplayValue('300')
    const maxPerHourInput = screen.getByDisplayValue('10')

    fireEvent.change(minDelayInput, { target: { value: '600' } })
    fireEvent.change(maxPerHourInput, { target: { value: '5' } })
    fireEvent.click(screen.getByRole('button', { name: /^save$/i }))

    await waitFor(() =>
      expect(api.setCompliance).toHaveBeenCalledWith({ min_delay_seconds: 600, max_per_hour: 5 }),
    )
    expect(await screen.findByText('Saved')).toBeInTheDocument()
  })

  it('surfaces the ApiError message when saving fails', async () => {
    vi.mocked(api.setCompliance).mockRejectedValue(new ApiError(400, 'rejected'))
    render(<Compliance />)
    await screen.findByDisplayValue('300')

    fireEvent.click(screen.getByRole('button', { name: /^save$/i }))
    expect(await screen.findByText('rejected')).toBeInTheDocument()
  })
})
