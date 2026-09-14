import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import i18n from '@/shared/i18n/config'

import { ChangePasswordPage, SignInPage } from './App'
import { AppProviders } from './providers/AppProviders'

const api = vi.hoisted(() => ({
  changePassword: vi.fn(),
  login: vi.fn(),
  systemStatus: vi.fn(),
}))

vi.mock('@/generated/api', () => ({
  changeLocalPassword: api.changePassword,
  loginLocal: api.login,
  useGetSystemStatus: api.systemStatus,
}))

describe('Authentication feedback', () => {
  beforeEach(async () => {
    vi.clearAllMocks()
    localStorage.clear()
    await i18n.changeLanguage('zh-TW')
    Object.defineProperty(window, 'matchMedia', {
      configurable: true,
      value: () => ({
        matches: false,
        addEventListener: () => undefined,
        removeEventListener: () => undefined,
      }),
    })
    api.systemStatus.mockReturnValue({
      data: { status: 200, data: { data: { oidcEnabled: false } } },
    })
  })

  afterEach(cleanup)

  it('shows the unified error notification when local sign-in fails', async () => {
    api.login.mockRejectedValue(new Error('rejected'))
    renderWithProviders(<SignInPage />)

    fireEvent.change(screen.getByRole('textbox', { name: '使用者名稱' }), {
      target: { value: 'admin' },
    })
    fireEvent.change(screen.getByLabelText('密碼'), {
      target: { value: 'incorrect' },
    })
    fireEvent.click(screen.getByRole('button', { name: '使用密碼登入' }))

    expect(await screen.findByRole('alert')).toHaveTextContent('操作失敗')
    expect(screen.getByRole('alert')).toHaveTextContent(
      '使用者名稱或密碼錯誤。',
    )
  })

  it('shows the unified error notification when password change fails', async () => {
    api.changePassword.mockRejectedValue(new Error('rejected'))
    renderWithProviders(<ChangePasswordPage />)

    fireEvent.change(screen.getByLabelText('目前密碼'), {
      target: { value: 'admin' },
    })
    fireEvent.change(screen.getByLabelText('新密碼'), {
      target: { value: 'new-password' },
    })
    fireEvent.click(screen.getByRole('button', { name: '變更密碼' }))

    await waitFor(() => expect(api.changePassword).toHaveBeenCalledOnce())
    expect(await screen.findByText('操作失敗')).toBeInTheDocument()
    expect(
      screen.getByText('無法變更密碼，請使用至少 8 個字元。'),
    ).toBeInTheDocument()
  })
})

function renderWithProviders(component: React.ReactNode) {
  return render(<AppProviders>{component}</AppProviders>)
}
