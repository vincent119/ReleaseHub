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
}))

vi.mock('@/generated/api', () => ({
  useGetCatalogResourceTree: api.useResources,
  useGetSystemStatus: api.useSystem,
  updateCatalogOrganization: api.updateOrganization,
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
