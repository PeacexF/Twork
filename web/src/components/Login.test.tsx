import { describe, expect, it, vi } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import Login from './Login'

describe('Login', () => {
  it('disables the submit button until both fields are filled', () => {
    render(<Login onSuccess={vi.fn()} verify={vi.fn()} />)
    expect(screen.getByRole('button', { name: /sign in/i })).toBeDisabled()
  })

  it('calls verify then onSuccess when credentials check out', async () => {
    const onSuccess = vi.fn()
    const verify = vi.fn().mockResolvedValue(undefined)
    render(<Login onSuccess={onSuccess} verify={verify} />)

    fireEvent.change(screen.getByLabelText('Username'), { target: { value: 'admin' } })
    fireEvent.change(screen.getByLabelText('Password'), { target: { value: 'secret' } })
    fireEvent.click(screen.getByRole('button', { name: /sign in/i }))

    await waitFor(() => expect(onSuccess).toHaveBeenCalledTimes(1))
    expect(verify).toHaveBeenCalledTimes(1)
  })

  it('shows an error and never calls onSuccess when verify rejects', async () => {
    const onSuccess = vi.fn()
    const verify = vi.fn().mockRejectedValue(new Error('unauthorized'))
    render(<Login onSuccess={onSuccess} verify={verify} />)

    fireEvent.change(screen.getByLabelText('Username'), { target: { value: 'admin' } })
    fireEvent.change(screen.getByLabelText('Password'), { target: { value: 'wrong' } })
    fireEvent.click(screen.getByRole('button', { name: /sign in/i }))

    expect(await screen.findByText(/wrong username or password/i)).toBeInTheDocument()
    expect(onSuccess).not.toHaveBeenCalled()
  })
})
