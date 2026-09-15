import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/react'
import { App as AntdApp } from 'antd'
import { I18nextProvider } from 'react-i18next'
import { MemoryRouter } from 'react-router'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import i18n from '@/shared/i18n/config'

import { ResourcesPage } from './ResourcesPage'

const api = vi.hoisted(() => ({
  refetch: vi.fn(),
  useResources: vi.fn(),
  useSystem: vi.fn(),
  updateOrganization: vi.fn(),
  deleteOrganization: vi.fn(),
}))

vi.mock('@/generated/api', () => ({
  useGetCatalogResourceTree: api.useResources,
  useGetSystemStatus: api.useSystem,
  updateCatalogOrganization: api.updateOrganization,
  deleteCatalogOrganization: api.deleteOrganization,
  createCatalogOrganization: vi.fn(),
  createCatalogProject: vi.fn(),
  createCatalogEnvironment: vi.fn(),
  createCatalogEnvironmentLabelMapping: vi.fn(),
}))

describe('ResourcesPage', () => {
  afterEach(cleanup)

  beforeEach(async () => {
    api.refetch.mockReset()
    api.updateOrganization.mockReset()
    api.deleteOrganization.mockReset()
    await i18n.changeLanguage('zh-TW')
    Object.defineProperty(window, 'matchMedia', {
      configurable: true,
      value: () => ({
        matches: false,
        addEventListener: () => undefined,
        removeEventListener: () => undefined,
      }),
    })
    api.useSystem.mockReturnValue({
      isPending: false,
      isError: false,
      data: {
        status: 200,
        data: {
          data: { name: 'ReleaseHub', version: 'test', tenancyMode: 'single' },
        },
      },
    })
    api.useResources.mockReturnValue({
      isPending: false,
      isError: false,
      refetch: api.refetch,
      data: {
        status: 200,
        data: {
          data: [
            {
              id: '019c1230-0000-7000-8000-000000000001',
              name: 'default',
              version: 1,
              isDefault: true,
              canRename: true,
              canDelete: false,
              canCreateProject: true,
              projects: [
                {
                  id: '019c1230-0000-7000-8000-000000000002',
                  name: 'Payment',
                  environments: [],
                  canManage: true,
                },
              ],
            },
          ],
        },
      },
    })
    api.updateOrganization.mockResolvedValue({ status: 200 })
    api.deleteOrganization.mockResolvedValue({ status: 204 })
  })

  it('hides the Organization presentation layer in single-tenant mode', () => {
    render(
      <AntdApp>
        <MemoryRouter>
          <I18nextProvider i18n={i18n}>
            <ResourcesPage />
          </I18nextProvider>
        </MemoryRouter>
      </AntdApp>,
    )

    expect(screen.getByText('Project: Payment')).toBeInTheDocument()
    expect(screen.queryByText('Organization: default')).not.toBeInTheDocument()
    expect(screen.getByText('工作區: default')).toBeInTheDocument()
  })

  it('renames the single-tenant workspace with the current version', async () => {
    document.cookie = 'releasehub_csrf=csrf-token'
    render(
      <AntdApp>
        <MemoryRouter>
          <I18nextProvider i18n={i18n}>
            <ResourcesPage />
          </I18nextProvider>
        </MemoryRouter>
      </AntdApp>,
    )

    fireEvent.click(screen.getByRole('button', { name: '重新命名工作區' }))
    const input = screen.getByRole('textbox', { name: '名稱' })
    expect(input).toHaveValue('default')
    fireEvent.change(input, { target: { value: '  Platform  ' } })
    fireEvent.click(screen.getByRole('button', { name: /儲\s*存/ }))

    await waitFor(() =>
      expect(api.updateOrganization).toHaveBeenCalledWith(
        '019c1230-0000-7000-8000-000000000001',
        { name: 'Platform', version: 1 },
        { headers: { 'X-CSRF-Token': 'csrf-token' } },
      ),
    )
    expect(api.refetch).toHaveBeenCalledOnce()
  })

  it('hides rename when the server does not grant the capability', () => {
    const state = api.useResources()
    state.data.data.data[0].canRename = false
    api.useResources.mockReturnValue(state)

    render(
      <AntdApp>
        <MemoryRouter>
          <I18nextProvider i18n={i18n}>
            <ResourcesPage />
          </I18nextProvider>
        </MemoryRouter>
      </AntdApp>,
    )

    expect(
      screen.queryByRole('button', { name: '重新命名工作區' }),
    ).not.toBeInTheDocument()
  })

  it('shows Organization rename in multi-tenant mode', () => {
    const state = api.useSystem()
    state.data.data.data.tenancyMode = 'multi'
    api.useSystem.mockReturnValue(state)

    render(
      <AntdApp>
        <MemoryRouter>
          <I18nextProvider i18n={i18n}>
            <ResourcesPage />
          </I18nextProvider>
        </MemoryRouter>
      </AntdApp>,
    )

    expect(screen.getByText('Organization: default')).toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: '重新命名 Organization' }),
    ).toBeInTheDocument()
    expect(screen.queryByText('工作區: default')).not.toBeInTheDocument()
  })

  it('deletes an empty non-default Organization after confirmation', async () => {
    document.cookie = 'releasehub_csrf=csrf-token'
    const systemState = api.useSystem()
    systemState.data.data.data.tenancyMode = 'multi'
    api.useSystem.mockReturnValue(systemState)
    const resourceState = api.useResources()
    resourceState.data.data.data = [
      {
        id: '019c1230-0000-7000-8000-000000000009',
        name: 'Temporary',
        version: 3,
        isDefault: false,
        canRename: true,
        canDelete: true,
        canCreateProject: true,
        projects: [],
      },
    ]
    api.useResources.mockReturnValue(resourceState)

    render(
      <AntdApp>
        <MemoryRouter>
          <I18nextProvider i18n={i18n}>
            <ResourcesPage />
          </I18nextProvider>
        </MemoryRouter>
      </AntdApp>,
    )

    fireEvent.click(screen.getByRole('button', { name: '刪除 Organization' }))
    expect(
      screen.getByText(
        '確定要刪除「Temporary」嗎？刪除後不會再顯示，且無法復原。',
      ),
    ).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: '確認刪除' }))

    await waitFor(() =>
      expect(api.deleteOrganization).toHaveBeenCalledWith(
        '019c1230-0000-7000-8000-000000000009',
        { expectedVersion: 3 },
        { headers: { 'X-CSRF-Token': 'csrf-token' } },
      ),
    )
    expect(api.refetch).toHaveBeenCalledOnce()
  })

  it('does not show delete for the default or non-deletable Organization', () => {
    const systemState = api.useSystem()
    systemState.data.data.data.tenancyMode = 'multi'
    api.useSystem.mockReturnValue(systemState)

    render(
      <AntdApp>
        <MemoryRouter>
          <I18nextProvider i18n={i18n}>
            <ResourcesPage />
          </I18nextProvider>
        </MemoryRouter>
      </AntdApp>,
    )

    expect(
      screen.queryByRole('button', { name: '刪除 Organization' }),
    ).not.toBeInTheDocument()
  })

  it('shows a conflict when the Organization cannot be deleted anymore', async () => {
    document.cookie = 'releasehub_csrf=csrf-token'
    const systemState = api.useSystem()
    systemState.data.data.data.tenancyMode = 'multi'
    api.useSystem.mockReturnValue(systemState)
    const resourceState = api.useResources()
    resourceState.data.data.data = [
      {
        id: '019c1230-0000-7000-8000-000000000009',
        name: 'Temporary',
        version: 3,
        isDefault: false,
        canRename: true,
        canDelete: true,
        canCreateProject: true,
        projects: [],
      },
    ]
    api.useResources.mockReturnValue(resourceState)
    api.deleteOrganization.mockResolvedValue({ status: 409 })

    render(
      <AntdApp>
        <MemoryRouter>
          <I18nextProvider i18n={i18n}>
            <ResourcesPage />
          </I18nextProvider>
        </MemoryRouter>
      </AntdApp>,
    )

    fireEvent.click(screen.getByRole('button', { name: '刪除 Organization' }))
    fireEvent.click(screen.getByRole('button', { name: '確認刪除' }))

    expect(
      await screen.findByText('資源與目前 Catalog 資料衝突。'),
    ).toBeInTheDocument()
    expect(api.refetch).not.toHaveBeenCalled()
  })

  it('keeps the delete confirmation loading while the request is pending', async () => {
    document.cookie = 'releasehub_csrf=csrf-token'
    const systemState = api.useSystem()
    systemState.data.data.data.tenancyMode = 'multi'
    api.useSystem.mockReturnValue(systemState)
    const resourceState = api.useResources()
    resourceState.data.data.data = [
      {
        id: '019c1230-0000-7000-8000-000000000009',
        name: 'Temporary',
        version: 3,
        isDefault: false,
        canRename: true,
        canDelete: true,
        canCreateProject: true,
        projects: [],
      },
    ]
    api.useResources.mockReturnValue(resourceState)
    let resolveDelete!: (value: { status: number }) => void
    api.deleteOrganization.mockReturnValue(
      new Promise((resolve) => {
        resolveDelete = resolve
      }),
    )

    render(
      <AntdApp>
        <MemoryRouter>
          <I18nextProvider i18n={i18n}>
            <ResourcesPage />
          </I18nextProvider>
        </MemoryRouter>
      </AntdApp>,
    )

    fireEvent.click(screen.getByRole('button', { name: '刪除 Organization' }))
    const confirm = screen.getByRole('button', { name: '確認刪除' })
    fireEvent.click(confirm)
    await waitFor(() => expect(confirm).toHaveClass('ant-btn-loading'))

    resolveDelete({ status: 204 })
    await waitFor(() => expect(api.refetch).toHaveBeenCalledOnce())
  })

  it('shows a conflict message when the workspace version is stale', async () => {
    document.cookie = 'releasehub_csrf=csrf-token'
    api.updateOrganization.mockResolvedValue({ status: 409 })
    render(
      <AntdApp>
        <MemoryRouter>
          <I18nextProvider i18n={i18n}>
            <ResourcesPage />
          </I18nextProvider>
        </MemoryRouter>
      </AntdApp>,
    )

    fireEvent.click(screen.getByRole('button', { name: '重新命名工作區' }))
    fireEvent.click(screen.getByRole('button', { name: /儲\s*存/ }))

    expect(
      await screen.findByText('資源與目前 Catalog 資料衝突。'),
    ).toBeInTheDocument()
  })

  it('keeps the rename action loading while the request is pending', async () => {
    document.cookie = 'releasehub_csrf=csrf-token'
    let resolveUpdate!: (value: { status: number }) => void
    api.updateOrganization.mockReturnValue(
      new Promise((resolve) => {
        resolveUpdate = resolve
      }),
    )
    render(
      <AntdApp>
        <MemoryRouter>
          <I18nextProvider i18n={i18n}>
            <ResourcesPage />
          </I18nextProvider>
        </MemoryRouter>
      </AntdApp>,
    )

    fireEvent.click(screen.getByRole('button', { name: '重新命名工作區' }))
    const save = screen.getByRole('button', { name: /儲\s*存/ })
    fireEvent.click(save)
    await waitFor(() => expect(save).toHaveClass('ant-btn-loading'))

    resolveUpdate({ status: 200 })
    await waitFor(() => expect(api.refetch).toHaveBeenCalledOnce())
  })
})
