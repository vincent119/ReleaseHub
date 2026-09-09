import { render, screen } from '@testing-library/react'
import { I18nextProvider } from 'react-i18next'
import { MemoryRouter } from 'react-router'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import i18n from '@/shared/i18n/config'

import { AccessPage } from './AccessPage'

const api = vi.hoisted(() => ({
  capabilities: vi.fn(),
  users: vi.fn(),
  groups: vi.fn(),
  roles: vi.fn(),
  memberships: vi.fn(),
  bindings: vi.fn(),
  denies: vi.fn(),
  candidates: vi.fn(),
  scopeOptions: vi.fn(),
  resources: vi.fn(),
}))

vi.mock('@/generated/api', () => ({
  useGetAccessCapabilities: api.capabilities,
  useListAccessUsers: api.users,
  useListAccessGroups: api.groups,
  useListAccessRoles: api.roles,
  useListAccessMemberships: api.memberships,
  useListAccessBindings: api.bindings,
  useListAccessDenies: api.denies,
  useListAccessMembershipCandidates: api.candidates,
  useGetAccessScopeOptions: api.scopeOptions,
  useGetCatalogResourceTree: api.resources,
  createAccessBinding: vi.fn(),
  createAccessDeny: vi.fn(),
  createAccessGroup: vi.fn(),
  createAccessMembership: vi.fn(),
  createAccessRole: vi.fn(),
  createAccessUser: vi.fn(),
  disableAccessGroup: vi.fn(),
  disableAccessRole: vi.fn(),
  disableAccessUser: vi.fn(),
  revokeAccessBinding: vi.fn(),
  revokeAccessDeny: vi.fn(),
  revokeAccessMembership: vi.fn(),
}))

function query(data: unknown) {
  return {
    isPending: false,
    isFetching: false,
    isError: false,
    refetch: vi.fn(),
    data: { status: 200, data: { data } },
  }
}

function renderPage(initialEntry = '/') {
  return render(
    <MemoryRouter initialEntries={[initialEntry]}>
      <I18nextProvider i18n={i18n}>
        <AccessPage />
      </I18nextProvider>
    </MemoryRouter>,
  )
}

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
    api.capabilities.mockReturnValue(
      query({
        collections: [
          { key: 'users', visible: true, canCreate: false },
          { key: 'groups', visible: true, canCreate: false },
          { key: 'roles', visible: true, canCreate: false },
          { key: 'memberships', visible: true, canCreate: false },
          { key: 'bindings', visible: true, canCreate: false },
          { key: 'denies', visible: true, canCreate: false },
        ],
        permissions: [],
      }),
    )
    api.users.mockReturnValue(
      query([
        {
          id: '019c1230-0000-7000-8000-000000000001',
          username: 'vincent',
          disabled: false,
          allowedActions: [],
        },
      ]),
    )
    api.groups.mockReturnValue(query([]))
    api.roles.mockReturnValue(query([]))
    api.memberships.mockReturnValue(query([]))
    api.bindings.mockReturnValue(query([]))
    api.denies.mockReturnValue(query([]))
    api.candidates.mockReturnValue(query([]))
    api.scopeOptions.mockReturnValue(
      query({ groups: [], roles: [], permissions: [] }),
    )
    api.resources.mockReturnValue(query([]))
  })

  it('renders only access records returned by the authorized API', () => {
    renderPage()

    expect(screen.getByText('vincent')).toBeInTheDocument()
    expect(screen.getByText('啟用')).toBeInTheDocument()
    expect(screen.getByRole('tab', { name: '成員關係' })).toBeInTheDocument()
  })

  it('renders the contextual create action for the active tab only', () => {
    api.capabilities.mockReturnValue(
      query({
        collections: [
          { key: 'users', visible: true, canCreate: true },
          { key: 'groups', visible: true, canCreate: true },
          { key: 'roles', visible: true, canCreate: false },
          { key: 'memberships', visible: true, canCreate: false },
          { key: 'bindings', visible: true, canCreate: false },
          { key: 'denies', visible: true, canCreate: false },
        ],
        permissions: [],
      }),
    )

    renderPage('/access?tab=groups')

    expect(
      screen.getByRole('button', { name: '建立 Group' }),
    ).toBeInTheDocument()
    expect(
      screen.queryByRole('button', { name: '新增使用者' }),
    ).not.toBeInTheDocument()
  })

  it('does not infer a destructive action missing from allowedActions', () => {
    renderPage()

    expect(
      screen.queryByRole('button', { name: '停用' }),
    ).not.toBeInTheDocument()
  })
})
