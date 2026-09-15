import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { App as AntdApp } from 'antd'
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from '@testing-library/react'
import { I18nextProvider } from 'react-i18next'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import i18n from '@/shared/i18n/config'
import { ThemePreferenceProvider } from '@/shared/theme/ThemePreferenceProvider'

import { AccountMenu } from './AccountMenu'

const api = vi.hoisted(() => ({
  changePassword: vi.fn(),
  mutateAsync: vi.fn(),
  isPending: false,
  logoutOptions: vi.fn(),
}))

vi.mock('@/generated/api', () => ({
  changeLocalPassword: api.changePassword,
  getGetAuthSessionQueryKey: () => ['/api/v1/auth/session'],
  useLogoutAuthSession: (options?: unknown) => {
    api.logoutOptions(options)
    return {
      mutateAsync: api.mutateAsync,
      isPending: api.isPending,
    }
  },
}))

describe('AccountMenu', () => {
  beforeEach(async () => {
    vi.clearAllMocks()
    api.isPending = false
    localStorage.clear()
    document.cookie = 'releasehub_csrf=csrf-token; path=/'
    await i18n.changeLanguage('en')
    Object.defineProperty(window, 'matchMedia', {
      configurable: true,
      value: () => ({
        matches: false,
        addEventListener: () => undefined,
        removeEventListener: () => undefined,
      }),
    })
  })

  afterEach(cleanup)

  it('shows the approved menu order and account identity', async () => {
    renderMenu()

    fireEvent.click(
      screen.getByRole('button', { name: 'Open account menu for vincent' }),
    )

    const items = await screen.findAllByRole('menuitem')
    expect(items).toHaveLength(4)
    expect(items[0]).toHaveTextContent('Personal settings')
    expect(items[1]).toHaveTextContent('Language')
    expect(items[2]).toHaveTextContent('Theme settings')
    expect(items[3]).toHaveTextContent('Sign out')
    expect(screen.getByText('V')).toBeInTheDocument()
  })

  it('shows read-only username and user ID', async () => {
    renderMenu()
    openMenuItem('Personal settings')

    const dialog = await screen.findByRole('dialog', {
      name: 'Personal settings',
    })
    expect(within(dialog).getByText('Username')).toBeInTheDocument()
    expect(within(dialog).getByText('vincent')).toBeInTheDocument()
    expect(within(dialog).getByText('User ID')).toBeInTheDocument()
    expect(within(dialog).getByText('user-123')).toBeInTheDocument()
    expect(
      within(dialog).queryByRole('button', { name: /save|edit/i }),
    ).toBeNull()
    expect(within(dialog).queryByRole('textbox')).toBeNull()
  })

  it('shows password change only when the session capability allows it', async () => {
    renderMenu({ passwordChangeAvailable: true })
    openMenuItem('Personal settings')

    const localDialog = await screen.findByRole('dialog', {
      name: 'Personal settings',
    })
    expect(
      within(localDialog).getByLabelText('Current password'),
    ).toBeInTheDocument()
    expect(
      within(localDialog).queryByRole('button', { name: 'Change password' }),
    ).toBeNull()
    expect(
      within(localDialog).getByRole('button', { name: 'Confirm' }),
    ).toBeInTheDocument()

    cleanup()
    renderMenu({ passwordChangeAvailable: false })
    openMenuItem('Personal settings')
    expect(
      within(
        await screen.findByRole('dialog', { name: 'Personal settings' }),
      ).queryByRole('button', { name: 'Change password' }),
    ).toBeNull()
    expect(screen.queryByLabelText('Current password')).toBeNull()
  })

  it('submits a valid password change without sending confirmation', async () => {
    api.changePassword.mockResolvedValue({ status: 204 })
    const { queryClient } = renderMenu({ passwordChangeAvailable: true })
    const invalidate = vi.spyOn(queryClient, 'invalidateQueries')
    const dialog = await openPasswordSettings()

    expect(screen.getAllByRole('dialog')).toHaveLength(1)
    expect(dialog).toHaveAccessibleName('Personal settings')

    fireEvent.change(screen.getByLabelText('Current password'), {
      target: { value: 'current-password' },
    })
    fireEvent.change(screen.getByLabelText('New password'), {
      target: { value: 'new-password' },
    })
    fireEvent.change(screen.getByLabelText('Confirm new password'), {
      target: { value: 'new-password' },
    })
    clickPasswordSubmit()

    await waitFor(() =>
      expect(api.changePassword).toHaveBeenCalledWith(
        {
          currentPassword: 'current-password',
          newPassword: 'new-password',
        },
        { headers: { 'X-CSRF-Token': 'csrf-token' } },
      ),
    )
    expect(invalidate).toHaveBeenCalledWith({
      queryKey: ['/api/v1/auth/session'],
    })
    expect(dialog).toHaveClass('ant-zoom-leave')
  })

  it('keeps the inline password section open after the request is rejected', async () => {
    api.changePassword.mockResolvedValue({ status: 400 })
    renderMenu({ passwordChangeAvailable: true })
    const dialog = await openPasswordSettings()

    fireEvent.change(screen.getByLabelText('Current password'), {
      target: { value: 'incorrect-password' },
    })
    fireEvent.change(screen.getByLabelText('New password'), {
      target: { value: 'new-password' },
    })
    fireEvent.change(screen.getByLabelText('Confirm new password'), {
      target: { value: 'new-password' },
    })
    clickPasswordSubmit()

    expect(
      await screen.findByText(
        'The password could not be changed. Check the current password and try again.',
      ),
    ).toBeInTheDocument()
    expect(screen.getByLabelText('Current password')).toBeInTheDocument()
    expect(screen.getAllByRole('dialog')).toHaveLength(1)
    expect(dialog).toHaveAccessibleName('Personal settings')
  })

  it('prevents duplicate password requests while submission is pending', async () => {
    api.changePassword.mockImplementation(() => new Promise(() => undefined))
    renderMenu({ passwordChangeAvailable: true })
    await openPasswordSettings()

    fireEvent.change(screen.getByLabelText('Current password'), {
      target: { value: 'current-password' },
    })
    fireEvent.change(screen.getByLabelText('New password'), {
      target: { value: 'new-password' },
    })
    fireEvent.change(screen.getByLabelText('Confirm new password'), {
      target: { value: 'new-password' },
    })
    clickPasswordSubmit()
    clickPasswordSubmit()

    await waitFor(() => expect(api.changePassword).toHaveBeenCalledOnce())
  })

  it('focuses the inline form and closes personal settings when cancelled', async () => {
    renderMenu({ passwordChangeAvailable: true })
    const dialog = await openPasswordSettings()
    const currentPassword = within(dialog).getByLabelText('Current password')

    expect(currentPassword).toHaveFocus()
    fireEvent.change(currentPassword, { target: { value: 'not-submitted' } })
    fireEvent.click(within(dialog).getByRole('button', { name: 'Cancel' }))

    expect(dialog).toHaveClass('ant-zoom-leave')
  })

  it('validates password byte length and confirmation before submitting', async () => {
    renderMenu({ passwordChangeAvailable: true })
    await openPasswordSettings()

    fireEvent.change(screen.getByLabelText('Current password'), {
      target: { value: 'current-password' },
    })
    fireEvent.change(screen.getByLabelText('New password'), {
      target: { value: '密碼' },
    })
    fireEvent.change(screen.getByLabelText('Confirm new password'), {
      target: { value: 'different' },
    })
    clickPasswordSubmit()

    expect(
      await screen.findByText('Use a password between 8 and 72 bytes.'),
    ).toBeInTheDocument()
    expect(screen.getByText('The passwords do not match.')).toBeInTheDocument()
    expect(api.changePassword).not.toHaveBeenCalled()
  })

  it('uses a Popover containing only the desktop language control', async () => {
    renderMenu({ desktopOverride: true })
    openMenuItem('Language')

    expect(
      await screen.findByRole('combobox', { name: 'Language' }),
    ).toBeInTheDocument()
    const popover = document.querySelector('.ant-popover-container')
    expect(popover).not.toBeNull()
    expect(
      within(popover as HTMLElement).getAllByText('Language'),
    ).toHaveLength(1)
    expect(screen.queryByRole('combobox', { name: 'Theme' })).toBeNull()
    expect(screen.queryByRole('dialog', { name: 'Language' })).toBeNull()
  })

  it('uses a Popover containing only the desktop theme control', async () => {
    renderMenu({ desktopOverride: true })
    openMenuItem('Theme settings')

    expect(
      await screen.findByRole('combobox', { name: 'Theme' }),
    ).toBeInTheDocument()
    const popover = document.querySelector('.ant-popover-container')
    expect(popover).not.toBeNull()
    expect(
      within(popover as HTMLElement).getByText('Theme settings'),
    ).toBeInTheDocument()
    expect(
      within(popover as HTMLElement).queryByText('Theme', { exact: true }),
    ).toBeNull()
    expect(screen.queryByRole('combobox', { name: 'Language' })).toBeNull()
    expect(screen.queryByRole('dialog', { name: 'Theme settings' })).toBeNull()
  })

  it('uses a Modal for small-screen theme settings', async () => {
    renderMenu({ desktopOverride: false })
    openMenuItem('Theme settings')

    expect(
      await screen.findByRole('dialog', { name: 'Theme settings' }),
    ).toBeInTheDocument()
    const dialog = screen.getByRole('dialog', { name: 'Theme settings' })
    expect(
      within(dialog).getByRole('combobox', { name: 'Theme' }),
    ).toBeInTheDocument()
    expect(within(dialog).queryByText('Theme', { exact: true })).toBeNull()
    expect(screen.queryByRole('combobox', { name: 'Language' })).toBeNull()
  })

  it('uses a Modal for small-screen language settings', async () => {
    renderMenu({ desktopOverride: false })
    openMenuItem('Language')

    expect(
      await screen.findByRole('dialog', { name: 'Language' }),
    ).toBeInTheDocument()
    const dialog = screen.getByRole('dialog', { name: 'Language' })
    expect(
      within(dialog).getByRole('combobox', { name: 'Language' }),
    ).toBeInTheDocument()
    expect(within(dialog).getAllByText('Language')).toHaveLength(1)
    expect(screen.queryByRole('combobox', { name: 'Theme' })).toBeNull()
  })

  it('invalidates the session after a local logout', async () => {
    api.mutateAsync.mockResolvedValue({
      status: 200,
      data: { data: { loggedOut: true } },
    })
    const { queryClient } = renderMenu()
    const invalidate = vi.spyOn(queryClient, 'invalidateQueries')

    openMenuItem('Sign out')

    await waitFor(() => expect(api.mutateAsync).toHaveBeenCalledOnce())
    expect(invalidate).toHaveBeenCalledWith({
      queryKey: ['/api/v1/auth/session'],
    })
  })

  it('configures logout with the CSRF cookie value', () => {
    renderMenu()

    expect(api.logoutOptions).toHaveBeenCalledWith({
      fetch: { headers: { 'X-CSRF-Token': 'csrf-token' } },
    })
  })

  it('does not request logout when the CSRF cookie is missing', async () => {
    document.cookie = 'releasehub_csrf=; Max-Age=0; path=/'
    renderMenu()

    openMenuItem('Sign out')

    expect(
      await screen.findByText('Sign out failed. Try again.'),
    ).toBeInTheDocument()
    expect(api.mutateAsync).not.toHaveBeenCalled()
  })

  it('uses only the API-provided OIDC redirect URL', async () => {
    const navigateTo = vi.fn()
    api.mutateAsync.mockResolvedValue({
      status: 200,
      data: {
        data: {
          loggedOut: true,
          redirectUrl: 'https://identity.example.test/logout',
        },
      },
    })
    renderMenu({ navigateTo })

    openMenuItem('Sign out')

    await waitFor(() =>
      expect(navigateTo).toHaveBeenCalledWith(
        'https://identity.example.test/logout',
      ),
    )
  })

  it('keeps the session and reports an invalid logout result', async () => {
    api.mutateAsync.mockResolvedValue({
      status: 200,
      data: { data: { loggedOut: false } },
    })
    const { queryClient } = renderMenu()
    const invalidate = vi.spyOn(queryClient, 'invalidateQueries')

    openMenuItem('Sign out')

    expect(
      await screen.findByText('Sign out failed. Try again.'),
    ).toBeInTheDocument()
    expect(invalidate).not.toHaveBeenCalled()
  })

  it('keeps the session and reports a logout request error', async () => {
    api.mutateAsync.mockRejectedValue(new Error('network unavailable'))
    const { queryClient } = renderMenu()
    const invalidate = vi.spyOn(queryClient, 'invalidateQueries')

    openMenuItem('Sign out')

    expect(
      await screen.findByText('Sign out failed. Try again.'),
    ).toBeInTheDocument()
    expect(invalidate).not.toHaveBeenCalled()
  })

  it('disables Sign out while a logout request is pending', () => {
    api.isPending = true
    renderMenu()

    fireEvent.click(
      screen.getByRole('button', { name: 'Open account menu for vincent' }),
    )

    expect(screen.getByRole('menuitem', { name: 'Sign out' })).toHaveAttribute(
      'aria-disabled',
      'true',
    )
  })
})

function renderMenu(
  props: Partial<React.ComponentProps<typeof AccountMenu>> = {},
) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  const view = render(
    <I18nextProvider i18n={i18n}>
      <ThemePreferenceProvider>
        <QueryClientProvider client={queryClient}>
          <AntdApp>
            <AccountMenu
              userId="user-123"
              username="vincent"
              passwordChangeAvailable={false}
              desktopOverride
              {...props}
            />
          </AntdApp>
        </QueryClientProvider>
      </ThemePreferenceProvider>
    </I18nextProvider>,
  )
  return { ...view, queryClient }
}

async function openPasswordSettings() {
  openMenuItem('Personal settings')
  const dialog = await screen.findByRole('dialog', {
    name: 'Personal settings',
  })
  await within(dialog).findByLabelText('Current password')
  return dialog
}

function clickPasswordSubmit() {
  const dialog = screen.getByRole('dialog', { name: 'Personal settings' })
  fireEvent.click(within(dialog).getByRole('button', { name: 'Confirm' }))
}

function openMenuItem(name: string) {
  fireEvent.click(
    screen.getByRole('button', { name: 'Open account menu for vincent' }),
  )
  fireEvent.click(screen.getByRole('menuitem', { name }))
}
