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
  mutateAsync: vi.fn(),
  isPending: false,
  logoutOptions: vi.fn(),
}))

vi.mock('@/generated/api', () => ({
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
    expect(items).toHaveLength(3)
    expect(items[0]).toHaveTextContent('Personal settings')
    expect(items[1]).toHaveTextContent('Theme settings')
    expect(items[2]).toHaveTextContent('Sign out')
    expect(screen.getByText('V')).toBeInTheDocument()
  })

  it('shows only read-only username and user ID', async () => {
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

  it('uses a Popover for desktop theme settings', async () => {
    renderMenu({ desktopOverride: true })
    openMenuItem('Theme settings')

    expect(await screen.findByText('Language')).toBeInTheDocument()
    expect(screen.getByText('Theme')).toBeInTheDocument()
    expect(screen.getByText('System')).toBeInTheDocument()
    expect(screen.queryByRole('dialog', { name: 'Theme settings' })).toBeNull()
  })

  it('uses a Modal for small-screen theme settings', async () => {
    renderMenu({ desktopOverride: false })
    openMenuItem('Theme settings')

    expect(
      await screen.findByRole('dialog', { name: 'Theme settings' }),
    ).toBeInTheDocument()
    expect(screen.getByText('System')).toBeInTheDocument()
    expect(screen.getByText('Light')).toBeInTheDocument()
    expect(screen.getByText('Dark')).toBeInTheDocument()
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

function openMenuItem(name: string) {
  fireEvent.click(
    screen.getByRole('button', { name: 'Open account menu for vincent' }),
  )
  fireEvent.click(screen.getByRole('menuitem', { name }))
}
