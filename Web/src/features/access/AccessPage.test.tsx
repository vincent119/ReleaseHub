import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { App } from 'antd'
import { render, screen } from '@testing-library/react'
import { I18nextProvider } from 'react-i18next'
import { MemoryRouter } from 'react-router'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import type { CatalogOrganizationNode } from '@/generated/model'
import i18n from '@/shared/i18n/config'

import { AccessPage } from './AccessPage'
import { scopePayload, scopeResourceOptions } from './scopeResources'

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
  system: vi.fn(),
  listCandidates: vi.fn(),
  listMemberships: vi.fn(),
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
  listAccessMembershipCandidates: api.listCandidates,
  listAccessMemberships: api.listMemberships,
  useGetAccessScopeOptions: api.scopeOptions,
  useGetCatalogResourceTree: api.resources,
  useGetSystemStatus: api.system,
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
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return render(
    <MemoryRouter initialEntries={[initialEntry]}>
      <I18nextProvider i18n={i18n}>
        <QueryClientProvider client={queryClient}>
          <App>
            <AccessPage />
          </App>
        </QueryClientProvider>
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
    api.system.mockReturnValue(
      query({ name: 'ReleaseHub', version: 'test', tenancyMode: 'single' }),
    )
    api.listCandidates.mockResolvedValue({
      status: 200,
      data: {
        data: [{ id: '019c1230-0000-7000-8000-000000000002', username: 'amy' }],
        meta: { requestId: 'request', timestamp: '', hasMore: false },
      },
    })
    api.listMemberships.mockResolvedValue({
      status: 200,
      data: {
        data: [],
        meta: { requestId: 'request', timestamp: '', hasMore: false },
      },
    })
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

  it('offers Group member management from the Group row', () => {
    api.groups.mockReturnValue(
      query([
        {
          id: '019c1230-0000-7000-8000-000000000003',
          ownerKind: 'platform',
          name: 'release-managers',
          oidcViewerOnly: false,
          disabled: false,
          allowedActions: ['viewMemberships', 'addMember'],
        },
      ]),
    )

    renderPage('/access?tab=groups')

    expect(screen.getByText('管理成員')).toBeInTheDocument()
  })

  it('formats Scope labels by tenancy mode without changing ancestry IDs', () => {
    const organizations: CatalogOrganizationNode[] = [
      {
        id: '019c1230-0000-7000-8000-000000000010',
        name: 'default',
        version: 1,
        isDefault: true,
        canRename: true,
        canCreateProject: true,
        projects: [
          {
            id: '019c1230-0000-7000-8000-000000000011',
            name: 'Payment',
            canManage: true,
            environments: [
              {
                id: '019c1230-0000-7000-8000-000000000012',
                name: 'production',
                type: 'Production',
                applications: [
                  {
                    id: '019c1230-0000-7000-8000-000000000013',
                    organizationId: '019c1230-0000-7000-8000-000000000010',
                    projectId: '019c1230-0000-7000-8000-000000000011',
                    environmentId: '019c1230-0000-7000-8000-000000000012',
                    name: 'api',
                    argocdNamespace: 'argocd',
                    argocdApplicationName: 'api-production',
                    argocdProject: 'payment',
                    destinationServer: 'https://kubernetes.default.svc',
                    destinationNamespace: 'payment',
                    sourceRepositoryUrl: 'https://git.example/repo.git',
                    sourceTargetRevision: 'main',
                    sourcePath: 'production/api',
                    active: true,
                    version: 1,
                  },
                ],
              },
            ],
          },
        ],
      },
    ]

    expect(scopeResourceOptions('project', organizations, true)[0]?.label).toBe(
      'Payment',
    )
    expect(
      scopeResourceOptions('environment', organizations, true)[0]?.label,
    ).toBe('Payment / production')
    expect(
      scopeResourceOptions('application', organizations, true)[0]?.label,
    ).toBe('Payment / production / api')
    expect(
      scopeResourceOptions('project', organizations, false)[0]?.label,
    ).toBe('default / Payment')
    expect(
      scopeResourceOptions('environment', organizations, false)[0]?.label,
    ).toBe('default / Payment / production')
    expect(
      scopeResourceOptions('application', organizations, false)[0]?.label,
    ).toBe('default / Payment / production / api')
    expect(
      scopePayload(
        {
          scopeKind: 'application',
          scopeId: '019c1230-0000-7000-8000-000000000013',
        },
        organizations,
      ),
    ).toEqual({
      scopeKind: 'application',
      organizationId: '019c1230-0000-7000-8000-000000000010',
      projectId: '019c1230-0000-7000-8000-000000000011',
      environmentId: '019c1230-0000-7000-8000-000000000012',
      applicationId: '019c1230-0000-7000-8000-000000000013',
    })
  })
})
