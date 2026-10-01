import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { App, ConfigProvider } from 'antd'
import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/react'
import { I18nextProvider } from 'react-i18next'
import { MemoryRouter } from 'react-router'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import i18n from '@/shared/i18n/config'

import { AccessPage } from './AccessPage'

const ids = {
  organization: '019c1230-0000-7000-8000-000000000010',
  project: '019c1230-0000-7000-8000-000000000011',
  otherProject: '019c1230-0000-7000-8000-000000000012',
  group: '019c1230-0000-7000-8000-000000000020',
  firstRole: '019c1230-0000-7000-8000-000000000030',
  secondRole: '019c1230-0000-7000-8000-000000000031',
}

const api = vi.hoisted(() => ({
  capabilities: vi.fn(),
  users: vi.fn(),
  groups: vi.fn(),
  roles: vi.fn(),
  memberships: vi.fn(),
  bindings: vi.fn(),
  denies: vi.fn(),
  resources: vi.fn(),
  system: vi.fn(),
  scopeOptions: vi.fn(),
  createBatch: vi.fn(),
  refetch: vi.fn(),
  error: vi.fn(),
  success: vi.fn(),
}))

vi.mock('@/generated/api', () => ({
  useGetAccessCapabilities: api.capabilities,
  useListAccessUsers: api.users,
  useListAccessGroups: api.groups,
  useListAccessRoles: api.roles,
  useListAccessMemberships: api.memberships,
  useListAccessBindings: api.bindings,
  useListAccessDenies: api.denies,
  useGetCatalogResourceTree: api.resources,
  useGetSystemStatus: api.system,
  useGetAccessScopeOptions: api.scopeOptions,
  createAccessBindingsBatch: api.createBatch,
  createAccessDeny: vi.fn(),
  createAccessGroup: vi.fn(),
  createAccessRole: vi.fn(),
  createAccessUser: vi.fn(),
  disableAccessGroup: vi.fn(),
  disableAccessRole: vi.fn(),
  disableAccessUser: vi.fn(),
  revokeAccessBinding: vi.fn(),
  revokeAccessDeny: vi.fn(),
  revokeAccessMembership: vi.fn(),
}))

vi.mock('@/shared/feedback/useFeedback', () => ({
  useFeedback: () => ({ error: api.error, success: api.success }),
}))

const clients = new Set<QueryClient>()

function query(data: unknown) {
  return {
    isPending: false,
    isFetching: false,
    isError: false,
    refetch: api.refetch,
    data: { status: 200, data: { data } },
  }
}

function renderPage() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  clients.add(client)
  render(
    <MemoryRouter initialEntries={['/access?tab=bindings']}>
      <I18nextProvider i18n={i18n}>
        <QueryClientProvider client={client}>
          <ConfigProvider theme={{ token: { motion: false } }}>
            <App>
              <AccessPage />
            </App>
          </ConfigProvider>
        </QueryClientProvider>
      </I18nextProvider>
    </MemoryRouter>,
  )
  fireEvent.click(screen.getByRole('button', { name: 'Create binding' }))
}

async function choose(label: string, option: string) {
  fireEvent.mouseDown(screen.getByLabelText(label))
  fireEvent.click(
    await screen.findByText(option, {
      selector: '.ant-select-item-option-content',
    }),
  )
}

async function fillBinding() {
  await choose('Scope resource', 'Payment')
  await choose('Group', 'release-managers')
  await choose('Role', 'viewer')
  await choose('Role', 'deployer')
}

function field(label: string) {
  return screen.getByLabelText(label).closest('.ant-select')
}

describe('AccessPage 角色綁定', () => {
  beforeEach(async () => {
    vi.clearAllMocks()
    await i18n.changeLanguage('en')
    document.cookie = 'releasehub_csrf=binding-test-csrf; path=/'
    api.capabilities.mockReturnValue(
      query({
        collections: [
          { key: 'users', visible: true, canCreate: true },
          { key: 'bindings', visible: true, canCreate: true },
        ],
        permissions: [],
      }),
    )
    for (const hook of [
      api.users,
      api.groups,
      api.roles,
      api.memberships,
      api.bindings,
      api.denies,
    ])
      hook.mockReturnValue(query([]))
    api.resources.mockReturnValue(
      query([
        {
          id: ids.organization,
          name: 'default',
          projects: [
            { id: ids.project, name: 'Payment', environments: [] },
            { id: ids.otherProject, name: 'Orders', environments: [] },
          ],
        },
      ]),
    )
    api.system.mockReturnValue(query({ tenancyMode: 'single' }))
    api.scopeOptions.mockReturnValue(
      query({
        groups: [
          { id: ids.group, name: 'release-managers', disabled: false },
          { id: 'disabled-group', name: 'disabled-team', disabled: true },
        ],
        roles: [
          { id: ids.firstRole, name: 'viewer', active: true },
          { id: ids.secondRole, name: 'deployer', active: true },
          { id: 'inactive-role', name: 'inactive-role', active: false },
        ],
        permissions: [],
      }),
    )
    api.createBatch.mockResolvedValue({ status: 201, data: { data: [] } })
  })

  afterEach(async () => {
    await act(async () => {
      cleanup()
      clients.forEach((client) => client.clear())
      clients.clear()
    })
    document.cookie = 'releasehub_csrf=; Max-Age=0; path=/'
  })

  it('一次提交多個角色、攜帶 CSRF，成功刷新並在重新開啟時清空', async () => {
    renderPage()
    await fillBinding()
    fireEvent.click(screen.getByRole('button', { name: 'Save' }))
    await waitFor(() => expect(api.createBatch).toHaveBeenCalledTimes(1))
    expect(api.createBatch).toHaveBeenCalledWith(
      {
        scopeKind: 'project',
        organizationId: ids.organization,
        projectId: ids.project,
        groupId: ids.group,
        roleIds: [ids.firstRole, ids.secondRole],
      },
      { headers: { 'X-CSRF-Token': 'binding-test-csrf' } },
    )
    await waitFor(() =>
      expect(screen.queryByRole('dialog')).not.toBeInTheDocument(),
    )
    expect(api.success).toHaveBeenCalledTimes(1)
    expect(api.refetch).toHaveBeenCalledTimes(8)
    fireEvent.click(screen.getByRole('button', { name: 'Create binding' }))
    expect(field('Role')).not.toHaveTextContent('viewer')
    expect(field('Role')).not.toHaveTextContent('deployer')
    expect(field('Group')).not.toHaveTextContent('release-managers')
  })

  it.each([409, 400, 'network'])(
    '提交失敗 %s 後保留選擇且不刷新，可使用同一選擇重試',
    async (failure) => {
      if (failure === 'network')
        api.createBatch.mockRejectedValueOnce(
          new Error('connection unavailable'),
        )
      else
        api.createBatch.mockResolvedValueOnce({
          status: failure,
          data: {
            error: { code: 'access_conflict', message: 'fixture failure' },
          },
        })
      renderPage()
      await fillBinding()
      fireEvent.click(screen.getByRole('button', { name: 'Save' }))
      await waitFor(() => expect(api.error).toHaveBeenCalledTimes(1))
      expect(screen.getByRole('dialog')).toBeInTheDocument()
      expect(field('Role')).toHaveTextContent('viewer')
      expect(field('Role')).toHaveTextContent('deployer')
      expect(field('Group')).toHaveTextContent('release-managers')
      expect(field('Scope resource')).toHaveTextContent('Payment')
      expect(api.refetch).not.toHaveBeenCalled()
      expect(api.success).not.toHaveBeenCalled()
      fireEvent.click(screen.getByRole('button', { name: 'Save' }))
      await waitFor(() => expect(api.createBatch).toHaveBeenCalledTimes(2))
      expect(api.createBatch.mock.calls[1]).toEqual(
        api.createBatch.mock.calls[0],
      )
      await waitFor(() => expect(api.success).toHaveBeenCalledTimes(1))
    },
  )

  it('缺少 Group 或 Role 時不提交，選項排除停用項目且支援角色搜尋', async () => {
    renderPage()
    fireEvent.click(screen.getByRole('button', { name: 'Save' }))
    await waitFor(() =>
      expect(
        document.querySelectorAll('.ant-form-item-has-error'),
      ).toHaveLength(3),
    )
    expect(api.createBatch).not.toHaveBeenCalled()
    await choose('Scope resource', 'Payment')
    fireEvent.click(screen.getByRole('button', { name: 'Save' }))
    await waitFor(() =>
      expect(
        document.querySelectorAll('.ant-form-item-has-error'),
      ).toHaveLength(2),
    )
    expect(api.createBatch).not.toHaveBeenCalled()
    fireEvent.mouseDown(screen.getByLabelText('Group'))
    expect(screen.queryByText('disabled-team')).not.toBeInTheDocument()
    await choose('Group', 'release-managers')
    fireEvent.mouseDown(screen.getByLabelText('Role'))
    expect(screen.queryByText('inactive-role')).not.toBeInTheDocument()
    fireEvent.change(screen.getByLabelText('Role'), {
      target: { value: 'deploy' },
    })
    expect(
      await screen.findByText('deployer', {
        selector: '.ant-select-item-option-content',
      }),
    ).toBeInTheDocument()
    expect(
      document.querySelector('.ant-select-item-option[title="viewer"]'),
    ).not.toBeInTheDocument()
  })

  it.each(['kind', 'resource'])(
    '變更 Scope %s 清空 Group 與 Role，並使用新 Scope 載入選項',
    async (change) => {
      renderPage()
      await fillBinding()
      if (change === 'kind') await choose('Scope', 'platform')
      else await choose('Scope resource', 'Orders')
      await waitFor(() => expect(field('Role')).not.toHaveTextContent('viewer'))
      expect(field('Role')).not.toHaveTextContent('deployer')
      expect(field('Group')).not.toHaveTextContent('release-managers')
      const [kind, scopeId, options] = api.scopeOptions.mock.calls.at(-1) ?? []
      expect(kind).toBe(change === 'kind' ? 'platform' : 'project')
      expect(scopeId).toBe(change === 'kind' ? 'platform' : ids.otherProject)
      expect(options.query.enabled).toBe(true)
      if (change === 'kind')
        expect(
          screen.queryByLabelText('Scope resource'),
        ).not.toBeInTheDocument()
      expect(api.createBatch).not.toHaveBeenCalled()
    },
  )

  it('選項載入中顯示 loading，送出期間鎖定欄位並阻止重送及關閉', async () => {
    api.scopeOptions.mockReturnValue({
      ...api.scopeOptions(),
      isFetching: true,
    })
    let resolveRequest: ((response: { status: number }) => void) | undefined
    api.createBatch.mockImplementationOnce(
      () => new Promise((resolve) => (resolveRequest = resolve)),
    )
    renderPage()
    expect(field('Group')).toHaveClass('ant-select-loading')
    expect(field('Role')).toHaveClass('ant-select-loading')
    await fillBinding()
    fireEvent.click(screen.getByRole('button', { name: 'Save' }))
    await waitFor(() => expect(api.createBatch).toHaveBeenCalledTimes(1))
    for (const label of ['Scope', 'Scope resource', 'Group', 'Role'])
      expect(screen.getByLabelText(label)).toBeDisabled()
    const form = screen.getByLabelText('Role').closest('form')
    if (!form) throw new Error('角色綁定欄位沒有對應的表單')
    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: /Save/ }))
      fireEvent.submit(form)
      fireEvent.click(screen.getByRole('button', { name: 'Cancel' }))
    })
    expect(screen.getByRole('dialog')).toBeInTheDocument()
    expect(api.createBatch).toHaveBeenCalledTimes(1)
    await act(async () => resolveRequest?.({ status: 201 }))
    await waitFor(() => expect(api.success).toHaveBeenCalledTimes(1))
  })
})
