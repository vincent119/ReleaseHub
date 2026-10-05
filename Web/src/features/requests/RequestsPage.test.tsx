import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
  within,
} from '@testing-library/react'
import { ConfigProvider } from 'antd'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { I18nextProvider } from 'react-i18next'
import { MemoryRouter } from 'react-router'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { DeploymentRequestStatus } from '@/generated/model'
import type {
  CatalogOrganizationNode,
  DeploymentRequestSummary,
} from '@/generated/model'

import i18n from '@/shared/i18n/config'
import linkStyles from '@/shared/link/ThemedLink.module.css'
import tagStyles from '@/shared/tag/SemanticTag.module.css'

import { RequestsPage } from './RequestsPage'
import { RequestList } from './components/RequestList'
import { initialRequestListCriteria } from './model/listQuery'

const api = vi.hoisted(() => ({ resources: vi.fn(), requests: vi.fn() }))

vi.mock('@/generated/api', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/generated/api')>()),
  useGetCatalogResourceTree: api.resources,
  useListDeploymentRequests: api.requests,
}))

describe('RequestsPage schedule projection', () => {
  beforeEach(async () => {
    await i18n.changeLanguage('zh-TW')
    vi.clearAllMocks()
    api.resources.mockReturnValue({
      isPending: false,
      isError: false,
      data: { status: 200, data: { data: organizations } },
    })
    api.requests.mockReturnValue({
      isPending: false,
      isError: false,
      data: {
        status: 200,
        data: {
          meta: {
            requestId: 'test',
            timestamp: '2026-10-02T00:00:00Z',
            hasMore: false,
          },
          data: [
            {
              id: 'request-1',
              organizationId: 'organization-1',
              projectId: 'project-1',
              environmentId: 'environment-1',
              status: 'Approved',
              classification: 'Standard',
              activeVersionNumber: 2,
              title: 'Deploy payment',
              applicationCount: 2,
              scheduledFor: '2026-09-17T00:00:00Z',
              scheduleState: 'Waiting',
              nextEligibleAt: '2026-09-17T02:00:00Z',
              scheduleReason: 'Blackout',
              updatedAt: '2026-09-16T00:00:00Z',
            },
          ],
        },
      },
    })
  })

  afterEach(async () => {
    await act(async () => cleanup())
    vi.unstubAllGlobals()
  })

  it('保留 Server 排程列表與表格連結樣式', async () => {
    renderPage()
    await selectScope()

    expect(screen.queryByText('目前位於禁止時段')).toBeNull()
    expect(screen.getByText('等待排程')).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'Deploy payment' })).toHaveClass(
      linkStyles.link,
    )
    expect(screen.getByText('Standard')).toHaveClass(tagStyles.neutral)
  })

  it('詳細資訊保留 Server 排程原因與時間', async () => {
    renderPage()
    await selectScope()
    fireEvent.click(
      screen.getByRole('button', { name: '查看排程條件：request-1' }),
    )
    expect(await screen.findByRole('dialog')).toBeInTheDocument()
    expect(screen.getByText('目前位於禁止時段')).toBeInTheDocument()
    expect(
      screen.getByText(new Date('2026-09-17T02:00:00Z').toLocaleString()),
    ).toBeInTheDocument()
    expect(
      screen.getByText(new Date('2026-09-17T00:00:00Z').toLocaleString()),
    ).toBeInTheDocument()
  })

  it.each(Object.values(DeploymentRequestStatus))(
    '結果 %s 不被排程摘要改寫，也不新增部署操作',
    async (status) => {
      const request = listFixture({ status })
      mockList([request])
      renderPage()
      await selectScope()
      const historical = [
        'Succeeded',
        'Failed',
        'PartialFailed',
        'Superseded',
        'Terminated',
      ].includes(status)
      const button = screen.getByRole('button', {
        name: `查看排程條件：${request.id}`,
      })
      expect(button).toHaveTextContent(
        historical ? '查看排程條件' : '已符合排程條件',
      )
      expect(screen.queryByText('目前可部署')).toBeNull()
      expect(
        screen.getByText(
          status === 'PartialFailed' ? 'Partial Failed' : status,
        ),
      ).toBeInTheDocument()
      if (status === 'Superseded') {
        expect(screen.getByText(status)).toHaveClass(tagStyles.neutral)
      }
      expect(
        screen.queryByRole('button', { name: /重試|部署|核准/ }),
      ).toBeNull()
      fireEvent.click(button)
      const dialog = await screen.findByRole('dialog')
      expect(dialog).toHaveTextContent('已符合排程條件')
      expect(dialog).toHaveTextContent('已符合所有排程條件')
      expect(dialog).toHaveTextContent('不代表部署或重試授權')
      expect(dialog).not.toHaveTextContent('指定最早時間')
      expect(dialog).toHaveTextContent(
        new Date(request.nextEligibleAt).toLocaleString(),
      )
    },
  )

  it('重名長名稱保留完整連結 ID，詳細資訊與複製不使用短 ID', async () => {
    const title = '同名長部署申請'.repeat(30)
    const first = listFixture({
      id: '11111111-1111-4111-8111-111111111111',
      title,
    })
    const second = listFixture({
      id: '11111111-1111-4111-8111-222222222222',
      title,
    })
    mockList([first, second])
    const writeText = vi.fn().mockResolvedValue(undefined)
    vi.stubGlobal('navigator', { clipboard: { writeText } })
    renderPage()
    await selectScope()
    const links = screen.getAllByRole('link', { name: title })
    expect(links[0]).toHaveAttribute('href', `/requests/${first.id}`)
    expect(links[1]).toHaveAttribute('href', `/requests/${second.id}`)
    expect(screen.queryByText(second.id)).toBeNull()
    fireEvent.click(
      screen.getByRole('button', {
        name: '查看完整名稱與 ID：11111111…22222222',
      }),
    )
    const dialog = await screen.findByRole('dialog')
    expect(dialog).toHaveTextContent(title)
    expect(screen.getByText(second.id)).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: '複製完整 ID' }))
    expect(await within(dialog).findByRole('status')).toHaveTextContent(
      '已複製完整 ID',
    )
    expect(writeText).toHaveBeenCalledWith(second.id)
  })

  it.each([true, false])(
    '剪貼簿拒絕或不可用時保留完整 ID 與失敗提示：%s',
    async (available) => {
      const request = listFixture()
      mockList([request])
      vi.stubGlobal(
        'navigator',
        available
          ? {
              clipboard: {
                writeText: vi.fn().mockRejectedValue(new Error('拒絕')),
              },
            }
          : {},
      )
      renderList()
      fireEvent.click(
        screen.getByRole('button', {
          name: `查看完整名稱與 ID：${request.id}`,
        }),
      )
      await screen.findByRole('dialog')
      fireEvent.click(screen.getByRole('button', { name: '複製完整 ID' }))
      expect(await screen.findByRole('alert')).toHaveTextContent('無法複製')
      expect(screen.getByRole('dialog')).toHaveTextContent(request.id)
    },
  )

  it('未識別的 Server 排程值原樣顯示，不推論成 Ready', async () => {
    mockList([
      {
        ...listFixture(),
        scheduleState: 'UnknownState',
        scheduleReason: 'UnknownReason',
      },
    ])
    renderPage()
    await selectScope()
    fireEvent.click(
      screen.getByRole('button', { name: '查看排程條件：request-1' }),
    )
    const dialog = await screen.findByRole('dialog')
    expect(dialog).toHaveTextContent('UnknownState')
    expect(dialog).toHaveTextContent('UnknownReason')
    expect(dialog).not.toHaveTextContent('已符合排程條件')
  })

  it('切換語言後詳細資訊與複製控制使用對應翻譯', async () => {
    mockList([listFixture()])
    renderPage()
    await selectScope()
    await act(async () => {
      await i18n.changeLanguage('en')
    })
    fireEvent.click(
      screen.getByRole('button', {
        name: 'View schedule conditions: request-1',
      }),
    )
    const dialog = await screen.findByRole('dialog', {
      name: 'Request and schedule information',
    })
    expect(dialog).toHaveTextContent('Schedule conditions met')
    expect(dialog).toHaveTextContent('authorization to deploy or retry')
    expect(
      screen.getByRole('button', { name: 'Copy full ID' }),
    ).toBeInTheDocument()
    expect(dialog).not.toHaveTextContent('requests.details')
  })

  it('只有三個有效 ID 齊備才查詢，Global 維持實際環境 ID', async () => {
    renderPage()
    expectQuery(false)
    await choose('Project', 'Project A')
    expectQuery(false)
    expect(screen.queryByRole('link', { name: 'Deploy payment' })).toBeNull()
    await choose('環境（必填）', 'Global')
    expect(api.requests).toHaveBeenLastCalledWith(
      {
        organizationId: 'organization-1',
        projectId: 'project-1',
        environmentId: 'global-1',
        limit: 20,
      },
      { query: expect.objectContaining({ enabled: true }) },
    )
  })

  it('切換 Project 立即清除環境與舊列表，未選新環境不查詢', async () => {
    renderPage()
    await selectScope()
    expect(
      screen.getByRole('link', { name: 'Deploy payment' }),
    ).toBeInTheDocument()
    await choose('Project', 'Project B')
    expectQuery(false)
    expect(
      screen.getByLabelText('環境（必填）').closest('.ant-select'),
    ).not.toHaveTextContent('Production')
    expect(screen.queryByRole('link', { name: 'Deploy payment' })).toBeNull()
    await choose('環境（必填）', 'Staging')
    expect(api.requests.mock.lastCall?.[0]).toEqual({
      organizationId: 'organization-1',
      projectId: 'project-2',
      environmentId: 'environment-2',
      limit: 20,
    })
  })

  it('新 scope 載入時不把未回應誤報為錯誤或顯示舊列表', async () => {
    renderPage()
    await selectScope()
    api.requests.mockReturnValue({
      isPending: true,
      isError: false,
      data: undefined,
    })
    await choose('Project', 'Project B')
    await choose('環境（必填）', 'Staging')
    expect(screen.queryByText(i18n.t('requests.unavailable'))).toBeNull()
    expect(screen.queryByRole('link', { name: 'Deploy payment' })).toBeNull()
    expect(document.querySelector('.ant-skeleton')).toBeInTheDocument()
  })

  it('實際查詢失敗不呈現成空結果', async () => {
    api.requests.mockReturnValue({
      isPending: false,
      isError: true,
      data: undefined,
    })
    renderPage()
    await selectScope()
    expect(screen.getByText(i18n.t('requests.unavailable'))).toBeInTheDocument()
    expect(screen.queryByText(i18n.t('requests.empty'))).toBeNull()
  })

  it('Catalog 移除環境後清除有效 scope，不能沿用 cache 結果', async () => {
    const page = renderPage()
    await selectScope()
    const changed = structuredClone(organizations)
    changed[0].projects[0].environments = []
    api.resources.mockReturnValue({
      isPending: false,
      isError: false,
      data: { status: 200, data: { data: changed } },
    })
    page.rerender(pageElement())
    expectQuery(false)
    expect(screen.queryByRole('link', { name: 'Deploy payment' })).toBeNull()
    expect(
      screen.getByText(i18n.t('requests.scope.invalidated')),
    ).toBeInTheDocument()
    expect(screen.getByLabelText('環境（必填）')).toBeDisabled()
  })

  it('Catalog 載入與 HTTP 失敗各有獨立狀態，不能查詢舊範圍', async () => {
    const page = renderPage()
    await selectScope()
    api.resources.mockReturnValue({
      isPending: true,
      isError: false,
      data: { status: 200, data: { data: organizations } },
    })
    page.rerender(pageElement())
    expectQuery(false)
    expect(screen.getByText(i18n.t('scopeFilter.loading'))).toBeInTheDocument()
    expect(screen.queryByRole('link', { name: 'Deploy payment' })).toBeNull()
    api.resources.mockReturnValue({
      isPending: false,
      isError: false,
      data: { status: 500 },
    })
    page.rerender(pageElement())
    expectQuery(false)
    expect(screen.getByText(i18n.t('requests.unavailable'))).toBeInTheDocument()
  })
})

function listFixture(
  overrides: Partial<DeploymentRequestSummary> = {},
): DeploymentRequestSummary {
  return {
    id: 'request-1',
    organizationId: 'organization-1',
    projectId: 'project-1',
    environmentId: 'environment-1',
    status: 'Succeeded',
    classification: 'Standard',
    activeVersionNumber: 2,
    title: 'Deploy payment',
    applicationCount: 2,
    scheduleState: 'Ready',
    nextEligibleAt: '2026-09-17T02:00:00Z',
    scheduleReason: 'Ready',
    updatedAt: '2026-09-16T00:00:00Z',
    ...overrides,
  }
}

function mockList(data: unknown[]) {
  api.requests.mockReturnValue({
    isPending: false,
    isError: false,
    data: {
      status: 200,
      data: {
        data,
        meta: {
          requestId: 'test',
          timestamp: '2026-10-02T00:00:00Z',
          hasMore: false,
        },
      },
    },
  })
}

function pageElement(content = <RequestsPage />) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return (
    <I18nextProvider i18n={i18n}>
      <ConfigProvider theme={{ token: { motion: false } }}>
        <MemoryRouter>
          <QueryClientProvider client={queryClient}>
            {content}
          </QueryClientProvider>
        </MemoryRouter>
      </ConfigProvider>
    </I18nextProvider>
  )
}

function renderPage() {
  return render(pageElement())
}

function renderList() {
  return render(
    pageElement(
      <RequestList
        scope={{
          organizationId: 'organization-1',
          projectId: 'project-1',
          environmentId: 'environment-1',
        }}
        criteria={initialRequestListCriteria}
      />,
    ),
  )
}

async function choose(label: string, name: string) {
  fireEvent.mouseDown(screen.getByRole('combobox', { name: label }))
  fireEvent.click(
    await screen.findByText(name, {
      selector: '.ant-select-item-option-content',
    }),
  )
}

async function selectScope() {
  await choose('Project', 'Project A')
  await choose('環境（必填）', 'Production')
}

function expectQuery(enabled: boolean) {
  expect(api.requests.mock.lastCall?.[1]).toEqual({
    query: expect.objectContaining({ enabled }),
  })
}

const organizations: CatalogOrganizationNode[] = [
  {
    id: 'organization-1',
    name: '測試組織',
    version: 1,
    isDefault: true,
    canRename: false,
    canDelete: false,
    canCreateProject: false,
    projects: [
      {
        id: 'project-1',
        name: 'Project A',
        canManage: false,
        environments: [
          {
            id: 'environment-1',
            name: 'Production',
            type: 'Production',
            applications: [],
          },
          {
            id: 'global-1',
            name: 'Global',
            type: 'Production',
            applications: [],
          },
        ],
      },
      {
        id: 'project-2',
        name: 'Project B',
        canManage: false,
        environments: [
          {
            id: 'environment-2',
            name: 'Staging',
            type: 'Staging',
            applications: [],
          },
        ],
      },
    ],
  },
]
