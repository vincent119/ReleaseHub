import { act, cleanup, fireEvent, render, screen } from '@testing-library/react'
import { ConfigProvider } from 'antd'
import { I18nextProvider } from 'react-i18next'
import { MemoryRouter } from 'react-router'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import type {
  CatalogApplication,
  CatalogOrganizationNode,
} from '@/generated/model'

import i18n from '@/shared/i18n/config'
import linkStyles from '@/shared/link/ThemedLink.module.css'
import {
  multipleScopeFixture,
  scopeFixture,
} from '@/shared/scope/scopeFixtures'

import { ApplicationsPage } from './ApplicationsPage'

const api = vi.hoisted(() => ({
  applications: vi.fn(),
  resources: vi.fn(),
  retry: vi.fn(),
}))

vi.mock('@/generated/api', () => ({
  confirmApplicationOnboarding: vi.fn(),
  useDryRunApplicationOnboarding: vi.fn(),
  useGetCatalogApplication: vi.fn(),
  useGetCatalogApplicationStatus: vi.fn(),
  useListVisibleCatalogApplications: api.applications,
  useGetCatalogResourceTree: api.resources,
}))

describe('ApplicationsPage 範圍篩選', () => {
  beforeEach(async () => {
    await i18n.changeLanguage('zh-TW')
    vi.resetAllMocks()
    setTree(scopeFixture())
    setApplications(visible)
  })

  afterEach(async () => {
    await act(async () => cleanup())
  })

  it('預設顯示授權全集，單組織預設 ID 不自行篩選且保留連結樣式', () => {
    const organizations = scopeFixture()
    organizations[0].projects[0].environments[0].applications = [
      application(
        'Unauthorized',
        'organization-a',
        'project-0',
        'environment-0',
      ),
    ]
    setTree(organizations)
    renderPage()
    expectNames(...visible.map((item) => item.name))
    expectMissing('Unauthorized')
    expect(screen.queryByRole('combobox', { name: '組織' })).toBeNull()
    expect(screen.getByRole('link', { name: 'Payment' })).toHaveAttribute(
      'href',
      '/applications/Payment',
    )
    expect(screen.getByRole('link', { name: 'Payment' })).toHaveClass(
      linkStyles.link,
    )
    expect(screen.getByRole('button', { name: '清除篩選' })).toBeDisabled()
    expect(api.applications).toHaveBeenLastCalledWith()
  })

  it('以 Catalog projectId 篩選，不使用不同的 argocdProject', async () => {
    renderPage()
    await choose('Project', '測試專案 0')
    expectNames('Payment', 'Global app')
    expectMissing('Other project', 'Other organization')
    expect(api.applications).toHaveBeenLastCalledWith()
  })

  it('Global 只對應實際 environmentId，清除環境恢復同 Project', async () => {
    renderPage()
    await choose('Project', '測試專案 0')
    await choose('環境（選填）', 'Global')
    expectNames('Global app')
    expectMissing('Payment', 'Other project', 'Other organization')
    clearField('環境（選填）')
    expectNames('Payment', 'Global app')
    expectMissing('Other project', 'Other organization')
  })

  it('跨組織同名 Project 不混淆，清除每層與全部均只恢復授權集合', async () => {
    setTree(multipleScopeFixture())
    renderPage()
    expectNames(...visible.map((item) => item.name))
    await choose('組織', '另一組織')
    expectNames('Other organization')
    expectMissing('Payment', 'Global app', 'Other project')
    await choose('Project', '測試專案 0')
    expectNames('Other organization')
    clearField('Project')
    expectNames('Other organization')
    clearField('組織')
    expectNames(...visible.map((item) => item.name))
    await choose('組織', '測試組織')
    await choose('Project', '測試專案 0')
    fireEvent.click(screen.getByRole('button', { name: '清除篩選' }))
    expectNames(...visible.map((item) => item.name))
    expect(screen.getByRole('button', { name: '清除篩選' })).toBeDisabled()
  })

  it('切換 Project 立即清除舊環境，不保留舊列表', async () => {
    renderPage()
    await choose('Project', '測試專案 0')
    await choose('環境（選填）', 'Global')
    await choose('Project', '測試專案 1')
    expectNames('Other project')
    expectMissing('Payment', 'Global app', 'Other organization')
    await choose('環境（選填）', 'Production')
    expectNames('Other project')
  })

  it('切換組織清除舊 Project 與環境，組織本身可篩選', async () => {
    setTree(multipleScopeFixture())
    renderPage()
    await choose('組織', '測試組織')
    await choose('Project', '測試專案 0')
    await choose('環境（選填）', 'Global')
    await choose('組織', '另一組織')
    expectNames('Other organization')
    expectMissing('Payment', 'Global app', 'Other project')
    expect(
      screen.getByRole('combobox', { name: '環境（選填）' }),
    ).toBeDisabled()
  })

  it.each(['project', 'environment', 'organization'] as const)(
    '成功 tree 確認 %s 失效時清除並提示，後續恢復不復活舊篩選',
    async (kind) => {
      const view = renderPage()
      await choose('Project', '測試專案 0')
      await choose('環境（選填）', 'Global')
      const organizations = scopeFixture()
      if (kind === 'project') organizations[0].projects.shift()
      if (kind === 'environment')
        organizations[0].projects[0].environments.pop()
      setTree(kind === 'organization' ? [] : organizations)
      view.rerender(page())
      expect(
        screen.getByText(
          '原範圍已無法使用，已清除篩選並顯示全部已授權 Applications。',
        ),
      ).toBeInTheDocument()
      expectNames(...visible.map((item) => item.name))
      setTree(scopeFixture())
      view.rerender(page())
      expectNames(...visible.map((item) => item.name))
      expect(screen.getByRole('button', { name: '清除篩選' })).toBeDisabled()
    },
  )

  it.each([
    { isPending: true, isError: false, data: undefined },
    { isPending: false, isError: true, data: undefined },
    { isPending: false, isError: false, data: { status: 403 } },
  ])('tree 載入或失敗不遮蔽成功的授權清單：%j', (state) => {
    api.resources.mockReturnValue({ ...state, refetch: api.retry })
    renderPage()
    expectNames(...visible.map((item) => item.name))
    expect(screen.getByRole('combobox', { name: 'Project' })).toBeDisabled()
    expect(screen.queryByText('無法取得 Applications')).toBeNull()
    if (!state.isPending) {
      expect(screen.getByText('無法載入範圍選項。')).toBeInTheDocument()
      expect(
        screen.getByText('已取得的 Application 清單仍可查看。'),
      ).toBeInTheDocument()
      fireEvent.click(screen.getByRole('button', { name: '重試範圍選項' }))
      expect(api.retry).toHaveBeenCalledOnce()
    }
  })

  it('tree 暫時失敗保留已選 ID 篩選與清除入口，不冒充範圍失效', async () => {
    const view = renderPage()
    await choose('Project', '測試專案 0')
    api.resources.mockReturnValue({
      isError: true,
      isPending: false,
      data: undefined,
      refetch: api.retry,
    })
    view.rerender(page())
    expectNames('Payment', 'Global app')
    expectMissing('Other project', 'Other organization')
    expect(
      screen.getByText(
        '既有篩選仍保留；可清除篩選以查看全部已授權 Applications。',
      ),
    ).toBeInTheDocument()
    expect(
      screen.queryByText(
        '原範圍已無法使用，已清除篩選並顯示全部已授權 Applications。',
      ),
    ).toBeNull()
    fireEvent.click(screen.getByRole('button', { name: '清除篩選' }))
    expectNames(...visible.map((item) => item.name))
  })

  it('tree 暫時 loading 不擴大已選篩選，復原後保留有效值', async () => {
    const view = renderPage()
    await choose('Project', '測試專案 0')
    api.resources.mockReturnValue({
      isPending: true,
      isError: false,
      data: undefined,
    })
    view.rerender(page())
    expectNames('Payment', 'Global app')
    expectMissing('Other project', 'Other organization')
    setTree(scopeFixture())
    view.rerender(page())
    expectNames('Payment', 'Global app')
    expect(screen.getByRole('button', { name: '清除篩選' })).toBeEnabled()
  })

  it.each([
    { isError: true, isPending: false, data: undefined },
    { isError: false, isPending: false, data: { status: 401 } },
  ])('授權列表失敗不顯示 tree 資料或偽空清單：%j', (state) => {
    api.applications.mockReturnValue(state)
    renderPage()
    expect(screen.getByText('無法取得 Applications')).toBeInTheDocument()
    expectMissing(...visible.map((item) => item.name))
    expect(screen.queryByText('目前沒有可查看的 Application。')).toBeNull()
  })

  it('兩份資料皆失敗時分別報錯，不宣稱列表仍可查看', () => {
    api.resources.mockReturnValue({
      isError: true,
      isPending: false,
      data: undefined,
      refetch: api.retry,
    })
    api.applications.mockReturnValue({
      isError: true,
      isPending: false,
      data: undefined,
    })
    renderPage()
    expect(screen.getAllByRole('alert')).toHaveLength(2)
    expect(screen.getByText('無法載入範圍選項。')).toBeInTheDocument()
    expect(screen.getByText('無法取得 Applications')).toBeInTheDocument()
    expect(screen.queryByText('已取得的 Application 清單仍可查看。')).toBeNull()
    expectMissing(...visible.map((item) => item.name))
  })

  it('單組織清除 Project 保留組織篩選，清除全部恢復授權全集', async () => {
    renderPage()
    await choose('Project', '測試專案 0')
    clearField('Project')
    expectNames('Payment', 'Global app', 'Other project')
    expectMissing('Other organization')
    expect(
      screen.getByRole('combobox', { name: '環境（選填）' }),
    ).toBeDisabled()
    fireEvent.click(screen.getByRole('button', { name: '清除篩選' }))
    expectNames(...visible.map((item) => item.name))
  })

  it('授權列表 loading 不誤報取得失敗', () => {
    api.applications.mockReturnValue({
      isError: false,
      isPending: true,
      data: undefined,
    })
    const view = renderPage()
    expect(screen.queryByRole('alert')).toBeNull()
    expect(
      view.container.querySelector('.ant-spin-spinning'),
    ).toBeInTheDocument()
  })

  it('授權空集合與篩選無結果使用不同提示', async () => {
    setApplications([])
    const view = renderPage()
    expect(
      screen.getByText('目前沒有可查看的 Application。'),
    ).toBeInTheDocument()
    setApplications([visible[2]])
    view.rerender(page())
    await choose('Project', '測試專案 0')
    expect(
      screen.getByText(
        '此範圍沒有符合的已授權 Application，請調整或清除篩選。',
      ),
    ).toBeInTheDocument()
    expect(screen.queryByText('目前沒有可查看的 Application。')).toBeNull()
  })

  it('授權列表刷新移除 Application 後，清除篩選也不復活舊資料', async () => {
    const view = renderPage()
    await choose('Project', '測試專案 0')
    setApplications([visible[2]])
    view.rerender(page())
    expectMissing('Payment', 'Global app')
    fireEvent.click(screen.getByRole('button', { name: '清除篩選' }))
    expectNames('Other project')
    expectMissing('Payment', 'Global app', 'Other organization')
  })
})

function application(
  name: string,
  organizationId: string,
  projectId: string,
  environmentId: string,
): CatalogApplication {
  return {
    id: name,
    name,
    organizationId,
    projectId,
    environmentId,
    argocdNamespace: 'argocd',
    argocdApplicationName: `${name}-production`,
    argocdProject: 'unrelated-argocd-project',
    destinationServer: 'https://kubernetes.default.svc',
    destinationNamespace: 'testing',
    sourceRepositoryUrl: 'https://example.invalid/manifest',
    sourceTargetRevision: 'main',
    sourcePath: 'test',
    active: true,
    version: 1,
  }
}

const visible = [
  application('Payment', 'organization-a', 'project-0', 'environment-0'),
  application('Global app', 'organization-a', 'project-0', 'global-0'),
  application('Other project', 'organization-a', 'project-1', 'environment-1'),
  application(
    'Other organization',
    'organization-b',
    'project-b',
    'environment-b',
  ),
]

function setTree(organizations: CatalogOrganizationNode[]) {
  api.resources.mockReturnValue({
    isPending: false,
    isError: false,
    data: { status: 200, data: { data: organizations } },
    refetch: api.retry,
  })
}

function setApplications(applications: CatalogApplication[]) {
  api.applications.mockReturnValue({
    isPending: false,
    isError: false,
    data: { status: 200, data: { data: applications } },
  })
}

function expectNames(...names: string[]) {
  for (const name of names)
    expect(screen.getByRole('link', { name })).toBeInTheDocument()
}

function expectMissing(...names: string[]) {
  for (const name of names)
    expect(screen.queryByRole('link', { name })).toBeNull()
}

async function choose(label: string, name: string) {
  fireEvent.mouseDown(screen.getByRole('combobox', { name: label }))
  fireEvent.click(
    await screen.findByText(name, {
      selector: '.ant-select-item-option-content',
    }),
  )
}

function clearField(label: string) {
  const clear = screen
    .getByRole('combobox', { name: label })
    .closest('.ant-select')
    ?.querySelector('.ant-select-clear')
  expect(clear).toBeTruthy()
  if (clear) fireEvent.click(clear)
}

function page() {
  return (
    <I18nextProvider i18n={i18n}>
      <ConfigProvider theme={{ token: { motion: false } }}>
        <MemoryRouter>
          <ApplicationsPage />
        </MemoryRouter>
      </ConfigProvider>
    </I18nextProvider>
  )
}

function renderPage() {
  return render(page())
}
