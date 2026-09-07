import { render, screen } from '@testing-library/react'
import { I18nextProvider } from 'react-i18next'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import i18n from '@/shared/i18n/config'

import { AccessPage } from './AccessPage'

const api = vi.hoisted(() => ({ useAccess: vi.fn(), useResources: vi.fn() }))

vi.mock('@/generated/api', () => ({
  useGetAccessManagementSnapshot: api.useAccess,
  useGetCatalogResourceTree: api.useResources,
  createAccessBinding: vi.fn(),
  createAccessDeny: vi.fn(),
  createAccessGroup: vi.fn(),
  createAccessMembership: vi.fn(),
  createAccessRole: vi.fn(),
  disableAccessGroup: vi.fn(),
  disableAccessUser: vi.fn(),
}))

describe('AccessPage', () => {
  beforeEach(async () => {
    await i18n.changeLanguage('zh-TW')
    Object.defineProperty(window, 'matchMedia', {
      configurable: true,
      value: () => ({
        matches: false,
        addEventListener: () => undefined,
        removeEventListener: () => undefined,
      }),
    })
    api.useAccess.mockReturnValue({
      isPending: false,
      isError: false,
      refetch: vi.fn(),
      data: {
        status: 200,
        data: {
          data: {
            canManagePlatform: false,
            managedProjectIds: [],
            permissions: [],
            users: [
              {
                id: '019c1230-0000-7000-8000-000000000001',
                username: 'vincent',
                disabled: false,
              },
            ],
            groups: [],
            roles: [],
            memberships: [],
            bindings: [],
            denies: [],
          },
        },
      },
    })
    api.useResources.mockReturnValue({
      isPending: false,
      isError: false,
      refetch: vi.fn(),
      data: { status: 200, data: { data: [] } },
    })
  })

  it('renders only access records returned by the authorized API', () => {
    render(
      <I18nextProvider i18n={i18n}>
        <AccessPage />
      </I18nextProvider>,
    )

    expect(screen.getByText('vincent')).toBeInTheDocument()
    expect(screen.getByText('啟用')).toBeInTheDocument()
  })
})
