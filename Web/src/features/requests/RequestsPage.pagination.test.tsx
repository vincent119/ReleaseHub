import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from '@testing-library/react'
import { HttpResponse, http } from 'msw'
import { setupServer } from 'msw/node'
import { useReducer } from 'react'
import {
  afterAll,
  afterEach,
  beforeAll,
  beforeEach,
  describe,
  expect,
  it,
} from 'vitest'

import { DeploymentRequestStatus } from '@/generated/model'
import i18n from '@/shared/i18n/config'
import { RequestList } from './components/RequestList'
import { RequestListFilters } from './components/RequestListFilters'
import { RequestsPage } from './RequestsPage'
import {
  listTestAPI,
  listTestWrapper,
  requestRows,
  testCatalog,
  testScope,
} from './model/listQuery.testHelpers'
import {
  initialRequestListCriteria,
  requestListCriteriaReducer,
} from './model/listQuery'

const server = setupServer()
beforeAll(() => server.listen({ onUnhandledRequest: 'error' }))
beforeEach(async () => {
  await i18n.changeLanguage('zh-TW')
  server.use(
    http.get('/api/v1/catalog/resource-tree', () =>
      HttpResponse.json({ data: testCatalog }),
    ),
  )
})
afterEach(() => {
  cleanup()
  server.resetHandlers()
})
afterAll(() => server.close())

async function choose(label: string, option: string) {
  const control = screen.getByRole('combobox', { name: label })
  await waitFor(() => expect(control).toBeEnabled())
  fireEvent.mouseDown(control)
  const selected = await screen.findByText(option, {
    selector: '.ant-select-item-option-content',
  })
  // 先完成互動引發的更新，避免後續可及性查詢反覆掃描過渡畫面。
  await act(async () => {
    fireEvent.click(selected)
  })
}

function ScopedListHarness() {
  const [criteria, dispatch] = useReducer(
    requestListCriteriaReducer,
    initialRequestListCriteria,
  )
  return (
    <>
      <RequestListFilters
        criteria={criteria}
        disabled={false}
        dispatch={dispatch}
      />
      <RequestList scope={testScope} criteria={criteria} />
    </>
  )
}

function start(mode: 'page' | 'list' = 'page') {
  const api = listTestAPI()
  server.use(api.handler)
  const { Wrapper, client } = listTestWrapper()
  // 列表專屬情境不重跑選擇器設定，完整頁面仍由 scope 案例與 E2E 驗證。
  render(mode === 'page' ? <RequestsPage /> : <ScopedListHarness />, {
    wrapper: Wrapper,
  })
  return { api, client }
}

async function selectScope() {
  await choose('Project', 'Project A')
  await choose('環境（必填）', 'Production')
  await screen.findByRole('link', { name: '部署 000' })
}

async function search(value: string) {
  const input = screen.getByRole('textbox', { name: '名稱搜尋' })
  fireEvent.change(input, { target: { value } })
  const form = input.closest('form')
  if (!form) throw new Error('搜尋缺少表單')
  await act(async () => {
    fireEvent.submit(form)
  })
}

function pageButton(name: '上一頁' | '下一頁') {
  return within(
    screen.getByRole('navigation', { name: '部署申請翻頁' }),
  ).getByRole('button', { name })
}

describe('Request 列表分頁控制', () => {
  it('scope 未齊不查詢', () => {
    const { api } = start()
    expect(screen.getByRole('button', { name: /搜\s*尋/ })).toBeDisabled()
    expect(api.calls).toHaveLength(0)
  })

  it('選定 scope 後預設 20 筆', async () => {
    const { api } = start()
    await selectScope()
    expect(api.calls[0].get('limit')).toBe('20')
    expect(screen.getAllByRole('link', { name: /^部署 \d/ })).toHaveLength(20)
    expect(pageButton('上一頁')).toBeDisabled()
  })

  it('下一頁後搜尋重設游標', async () => {
    const { api } = start('list')
    await screen.findByRole('link', { name: '部署 000' })
    await act(async () => {
      fireEvent.click(pageButton('下一頁'))
    })
    await screen.findByRole('link', { name: '部署 020' })
    expect(api.calls.at(-1)?.get('cursor')).toMatch(/^opaque/)
    await search('  部署  ')
    await screen.findByRole('link', { name: '部署 000' })
    expect(api.calls.at(-1)?.get('search')).toBe('部署')
    expect(api.calls.at(-1)?.has('cursor')).toBe(false)
  })

  it('下一頁後改變狀態與筆數會重設游標', async () => {
    const { api } = start('list')
    await screen.findByRole('link', { name: '部署 000' })
    fireEvent.click(pageButton('下一頁'))
    await screen.findByRole('link', { name: '部署 020' })
    await choose('申請狀態', 'Failed')
    await screen.findByRole('link', { name: '部署 005' })
    expect(api.calls.at(-1)?.get('status')).toBe('Failed')
    expect(api.calls.at(-1)?.has('cursor')).toBe(false)
    await choose('每頁筆數', '50 筆')
    await waitFor(() => expect(api.calls.at(-1)?.get('limit')).toBe('50'))
    expect(api.calls.at(-1)?.has('cursor')).toBe(false)
    await screen.findByRole('link', { name: '部署 005' })
  })

  it('下一頁後改變 scope 會重設游標', async () => {
    const { api } = start()
    await selectScope()
    fireEvent.click(pageButton('下一頁'))
    await screen.findByRole('link', { name: '部署 020' })
    await choose('環境（必填）', 'Global')
    await waitFor(() =>
      expect(api.calls.at(-1)?.get('environmentId')).toBe(
        testCatalog[0].projects[0].environments[1].id,
      ),
    )
    expect(api.calls.at(-1)?.has('cursor')).toBe(false)
    expect(api.calls.at(-1)?.has('search')).toBe(false)
  })

  it('改變 scope 時保留已套用的狀態與筆數', async () => {
    const { api } = start()
    await selectScope()
    await choose('申請狀態', 'Failed')
    await screen.findByRole('link', { name: '部署 005' })
    await choose('每頁筆數', '50 筆')
    await waitFor(() => expect(api.calls.at(-1)?.get('limit')).toBe('50'))
    await choose('環境（必填）', 'Global')
    await waitFor(() =>
      expect(api.calls.at(-1)?.get('environmentId')).toBe(
        testCatalog[0].projects[0].environments[1].id,
      ),
    )
    expect(api.calls.at(-1)?.get('status')).toBe('Failed')
    expect(api.calls.at(-1)?.get('limit')).toBe('50')
  })

  it('名稱搜尋送 Server，命中第 132 筆並保留 wildcard 字面值；清除不保留搜尋或 status', async () => {
    const { api } = start()
    await selectScope()
    const statuses = screen.getByRole('combobox', { name: '申請狀態' })
    fireEvent.mouseDown(statuses)
    const options = screen
      .getAllByText(/./, { selector: '.ant-select-item-option-content' })
      .map((node) => node.textContent)
    for (const status of Object.values(DeploymentRequestStatus))
      expect(options).toContain(
        status === 'PartialFailed' ? 'Partial Failed' : status,
      )
    fireEvent.click(
      screen.getByText('全部狀態', {
        selector: '.ant-select-item-option-content',
      }),
    )
    await search(' %_\\特殊 ')
    await screen.findByRole('link', { name: '後頁部署 %_\\特殊' })
    expect(screen.getAllByRole('link')).toHaveLength(1)
    expect(api.calls.at(-1)?.get('search')).toBe('%_\\特殊')
    expect(pageButton('下一頁')).toBeDisabled()
    await search('完全無匹配')
    await screen.findByText(i18n.t('requests.query.noResults'))
    expect(screen.queryByText(i18n.t('requests.empty'))).toBeNull()
    fireEvent.click(screen.getByRole('button', { name: '清除條件' }))
    await screen.findByRole('link', { name: '部署 000' })
    expect(api.calls.at(-1)?.has('search')).toBe(false)
    expect(api.calls.at(-1)?.has('status')).toBe(false)
    expect(screen.getByRole('textbox', { name: '名稱搜尋' })).toHaveValue('')
  })

  it('Unicode 255 字可送出，256 字顯示驗證而不查詢；清除會移除錯誤', async () => {
    const { api } = start()
    await selectScope()
    const valid = '界'.repeat(254) + '𠮷'
    await search(valid)
    await screen.findByText(i18n.t('requests.query.noResults'))
    expect(api.calls.at(-1)?.get('search')).toBe(valid)
    const count = api.calls.length
    await search(valid + '𠮷')
    await screen.findByText(i18n.t('requests.query.searchTooLong'))
    expect(api.calls).toHaveLength(count)
    fireEvent.click(screen.getByRole('button', { name: '清除條件' }))
    await screen.findByRole('link', { name: '部署 000' })
    await waitFor(() =>
      expect(
        screen.queryByText(i18n.t('requests.query.searchTooLong')),
      ).toBeNull(),
    )
  })

  it.each([400, 404, 500, 'network', 'meta'] as const)(
    '失敗 %s 不當成空結果，不自動重試，明確操作回第一頁',
    async (failure) => {
      const { api } = start()
      await selectScope()
      let calls = 0
      server.use(
        http.get('/api/v1/deployment-requests', () => {
          calls++
          if (failure === 'network') return HttpResponse.error()
          if (failure === 'meta')
            return HttpResponse.json({ data: [], meta: { hasMore: true } })
          return HttpResponse.json(
            { code: 'INVALID_REQUEST', retryable: false },
            { status: failure },
          )
        }),
      )
      fireEvent.click(pageButton('下一頁'))
      const alert = await screen.findByRole('alert')
      expect(alert).toHaveTextContent(
        i18n.t(
          failure === 400
            ? 'requests.query.invalid'
            : failure === 'meta'
              ? 'requests.query.badPage'
              : 'requests.unavailable',
        ),
      )
      expect(calls).toBe(1)
      expect(screen.queryByText(i18n.t('requests.empty'))).toBeNull()
      expect(screen.queryByText(i18n.t('requests.query.noResults'))).toBeNull()
      expect(pageButton('下一頁')).toBeDisabled()
      server.use(api.handler)
      fireEvent.click(
        within(alert).getByRole('button', { name: '重新載入第一頁' }),
      )
      await screen.findByRole('link', { name: '部署 000' })
      expect(api.calls.at(-1)?.has('cursor')).toBe(false)
      expect(pageButton('上一頁')).toBeDisabled()
    },
  )

  it('後頁重新整理取得第一頁並只顯示更新通知', async () => {
    const { api } = start('list')
    await screen.findByRole('link', { name: '部署 000' })
    fireEvent.click(pageButton('下一頁'))
    await screen.findByRole('link', { name: '部署 020' })
    fireEvent.click(screen.getByRole('button', { name: '重新整理列表' }))
    await screen.findByRole('link', { name: '部署 000' })
    expect(screen.getByRole('alert')).toHaveTextContent(
      i18n.t('requests.query.updated'),
    )
    expect(api.calls.at(-1)?.has('cursor')).toBe(false)
  })

  it('空列表重新整理後中文與英文文案可識別', async () => {
    start('list')
    await screen.findByRole('link', { name: '部署 000' })
    server.use(listTestAPI(requestRows(0)).handler)
    fireEvent.click(screen.getByRole('button', { name: '重新整理列表' }))
    await screen.findByText(i18n.t('requests.empty'))
    expect(screen.queryByText(i18n.t('requests.query.noResults'))).toBeNull()
    await act(async () => {
      await i18n.changeLanguage('en')
    })
    expect(
      screen.getByRole('textbox', { name: 'Search by name' }),
    ).toBeInTheDocument()
    expect(
      screen.getByRole('navigation', { name: 'Deployment request pages' }),
    ).toHaveTextContent('Page 1, 0 rows on this page')
    expect(document.body.textContent).not.toContain('requests.query.')
  })

  it('Server 回傳已走訪游標時阻止循環，不繼續前進或自動重試', async () => {
    const { api } = start()
    await selectScope()
    server.use(
      http.get('/api/v1/deployment-requests', ({ request }) => {
        const cursor = new URL(request.url).searchParams.get('cursor')
        return HttpResponse.json({
          data: [],
          meta: {
            requestId: 'cycle',
            timestamp: '2026-10-02T00:00:00Z',
            hasMore: true,
            nextCursor: cursor,
          },
        })
      }),
    )
    fireEvent.click(pageButton('下一頁'))
    await screen.findByText(i18n.t('requests.query.badPage'))
    expect(pageButton('下一頁')).toBeDisabled()
    expect(api.calls).toHaveLength(1)
  })

  it('後頁為空不推論整個 scope 沒有申請，仍可回上一頁或刷新', async () => {
    start()
    await selectScope()
    server.use(
      http.get('/api/v1/deployment-requests', () =>
        HttpResponse.json({
          data: [],
          meta: {
            requestId: 'empty-page',
            timestamp: '2026-10-02T00:00:00Z',
            hasMore: false,
          },
        }),
      ),
    )
    fireEvent.click(pageButton('下一頁'))
    await screen.findByText(i18n.t('requests.query.emptyPage'))
    expect(screen.queryByText(i18n.t('requests.empty'))).toBeNull()
    expect(screen.queryByText(i18n.t('requests.query.noResults'))).toBeNull()
    expect(pageButton('上一頁')).toBeEnabled()
    expect(pageButton('下一頁')).toBeDisabled()
    expect(screen.getByRole('button', { name: '重新整理列表' })).toBeEnabled()
  })
})
