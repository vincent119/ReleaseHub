import { expect, test, type Page } from '@playwright/test'
import {
  DeploymentRequestStatus,
  type CatalogOrganizationNode,
  type DeploymentRequestSummary,
} from '../src/generated/model'

declare global {
  interface Window {
    requestListNotify: (event: string) => void
  }
}

const scope = {
  organizationId: '00000000-0000-4000-8000-000000000001',
  projectId: '00000000-0000-4000-8000-000000000002',
  environmentId: '00000000-0000-4000-8000-000000000003',
}
const meta = { requestId: 'pagination-e2e', timestamp: '2026-10-02T00:00:00Z' }
const catalog: CatalogOrganizationNode[] = [
  {
    id: scope.organizationId,
    name: '測試組織',
    version: 1,
    isDefault: true,
    canRename: false,
    canDelete: false,
    canCreateProject: false,
    projects: [
      {
        id: scope.projectId,
        name: 'Project A',
        canManage: false,
        environments: [
          {
            id: scope.environmentId,
            name: 'Production',
            type: 'Production',
            applications: [],
          },
        ],
      },
    ],
  },
]

function rows(): DeploymentRequestSummary[] {
  return Array.from({ length: 137 }, (_, index) => ({
    ...scope,
    id: `00000000-0000-4000-8000-${String(index + 100).padStart(12, '0')}`,
    title:
      index === 131
        ? '後頁部署 %_\\特殊'
        : `部署 ${String(index).padStart(3, '0')}`,
    status: Object.values(DeploymentRequestStatus)[index % 10],
    classification: 'Standard',
    activeVersionNumber: 1,
    applicationCount: 1,
    scheduleState: 'Ready',
    scheduleReason: 'Ready',
    nextEligibleAt: '2026-10-02T00:00:00Z',
    updatedAt: '2026-10-02T00:00:00Z',
  }))
}

async function prepare(page: Page, theme: 'light' | 'dark' = 'light') {
  const state = {
    calls: [] as URLSearchParams[],
    failure: 0,
    delayedSearch: '',
    delay: undefined as Promise<void> | undefined,
    oldResponded: false,
    emptyNextPage: false,
  }
  const cursors = new Map<string, { offset: number; context: string }>()
  await page.addInitScript((theme) => {
    localStorage.setItem('releasehub.language', 'zh-TW')
    localStorage.setItem('releasehub.theme', theme)
    class TestEventSource {
      onmessage: (() => void) | null = null
      listeners = new Map<string, () => void>()
      constructor() {
        window.requestListNotify = (event) => this.listeners.get(event)?.()
      }
      addEventListener(event: string, listener: () => void) {
        this.listeners.set(event, listener)
      }
      close() {}
    }
    Object.defineProperty(window, 'EventSource', { value: TestEventSource })
  }, theme)
  await page.route('**/api/v1/**', async (route) => {
    const url = new URL(route.request().url())
    const respond = (data: unknown) =>
      route.fulfill({
        contentType: 'application/json',
        body: JSON.stringify({ data, meta }),
      })
    if (url.pathname === '/api/v1/auth/session')
      return respond({ userId: 'test-user', username: '測試使用者' })
    if (url.pathname === '/api/v1/audit/capabilities')
      return respond({ visible: false })
    if (url.pathname.startsWith('/api/v1/notifications')) return respond([])
    if (url.pathname === '/api/v1/catalog/resource-tree')
      return respond(catalog)
    if (url.pathname !== '/api/v1/deployment-requests')
      return route.fulfill({ status: 404, body: '{}' })
    const params = url.searchParams
    state.calls.push(params)
    if (state.failure)
      return route.fulfill({
        status: state.failure,
        contentType: 'application/json',
        body: JSON.stringify({
          code: 'INVALID_REQUEST',
          retryable: false,
          category: 'validation',
        }),
      })
    const limit = Number(params.get('limit'))
    const search = params.get('search') ?? ''
    const status = params.get('status')
    const context = JSON.stringify([
      params.get('organizationId'),
      params.get('projectId'),
      params.get('environmentId'),
      limit,
      search,
      status,
    ])
    const cursor = params.get('cursor')
    const position = cursor ? cursors.get(cursor) : { offset: 0, context }
    if (!position || position.context !== context)
      throw new Error('測試 Server 拒絕混用游標')
    if (search === state.delayedSearch && state.delay) {
      await state.delay
      state.oldResponded = true
    }
    const filtered = rows().filter(
      (row) => row.title.includes(search) && (!status || row.status === status),
    )
    const emptyPage = Boolean(state.emptyNextPage && cursor)
    const data = emptyPage
      ? []
      : filtered.slice(position.offset, position.offset + limit)
    const hasMore = !emptyPage && position.offset + limit < filtered.length
    const nextCursor = hasMore ? `opaque+?&/${state.calls.length}` : undefined
    if (nextCursor)
      cursors.set(nextCursor, { offset: position.offset + limit, context })
    return route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({
        data,
        meta: { ...meta, hasMore, ...(nextCursor ? { nextCursor } : {}) },
      }),
    })
  })
  await page.goto('/requests')
  await page.getByRole('combobox', { name: 'Project', exact: true }).click()
  await page.getByRole('option', { name: 'Project A', exact: true }).click()
  await page
    .getByRole('combobox', { name: '環境（必填）', exact: true })
    .click()
  await page.getByRole('option', { name: 'Production', exact: true }).click()
  await expect(
    page.getByRole('link', { name: '部署 000', exact: true }),
  ).toBeVisible()
  return state
}

const bodyRows = (page: Page) =>
  page.locator('.ant-table tbody tr[data-row-key]')

for (const limit of [20, 50, 100]) {
  test(`${limit} 筆游標遍歷 137 筆與上一頁，不假造總數或任意頁碼`, async ({
    page,
  }) => {
    const state = await prepare(page)
    if (limit !== 20) {
      await page
        .getByRole('combobox', { name: '每頁筆數', exact: true })
        .click()
      await page
        .getByRole('option', { name: `${limit} 筆`, exact: true })
        .click()
    }
    const seen: string[] = []
    const count = Math.ceil(137 / limit)
    for (let index = 0; index < count; index++) {
      await expect(bodyRows(page)).toHaveCount(
        Math.min(limit, 137 - index * limit),
      )
      await expect(
        page.getByRole('navigation', { name: '部署申請翻頁' }),
      ).toContainText(`第 ${index + 1} 頁`)
      seen.push(
        ...(await bodyRows(page).evaluateAll((nodes) =>
          nodes.map((node) => node.getAttribute('data-row-key')!),
        )),
      )
      const next = page.getByRole('button', { name: '下一頁', exact: true })
      if (index < count - 1) await next.click()
      else await expect(next).toBeDisabled()
    }
    expect(seen).toEqual(rows().map((row) => row.id))
    expect(new Set(seen).size).toBe(137)
    await page.getByRole('button', { name: '上一頁', exact: true }).click()
    await expect(
      page.getByRole('navigation', { name: '部署申請翻頁' }),
    ).toContainText(`第 ${count - 1} 頁`)
    await page.getByRole('button', { name: '下一頁', exact: true }).click()
    await expect(
      page.getByRole('navigation', { name: '部署申請翻頁' }),
    ).toContainText(`第 ${count} 頁`)
    expect(state.calls.every((call) => Number(call.get('limit')) <= 100)).toBe(
      true,
    )
    expect(
      state.calls
        .filter((call) => call.has('cursor'))
        .every((call) => call.get('cursor')?.startsWith('opaque+?&/')),
    ).toBe(true)
    await expect(page.getByRole('spinbutton')).toHaveCount(0)
  })
}

for (const theme of ['light', 'dark'] as const) {
  for (const width of [320, 768, 1440]) {
    test(`查詢控制 ${theme} ${width}px：主題、鍵盤、觸控目標與不溢位`, async ({
      page,
    }, testInfo) => {
      const errors: string[] = []
      page.on('pageerror', (error) => errors.push(error.message))
      await page.setViewportSize({ width, height: 900 })
      const state = await prepare(page, theme)
      await page.getByRole('button', { name: '下一頁', exact: true }).click()
      await expect(
        page.getByRole('link', { name: '部署 020', exact: true }),
      ).toBeVisible()
      const search = page.getByRole('textbox', {
        name: '名稱搜尋',
        exact: true,
      })
      await search.focus()
      await expect(search).toBeFocused()
      await search.fill(' %_\\特殊 ')
      await search.press('Enter')
      await expect(bodyRows(page)).toHaveCount(1)
      await expect(
        page.getByRole('link', { name: '後頁部署 %_\\特殊', exact: true }),
      ).toBeVisible()
      expect(state.calls.at(-1)?.get('search')).toBe('%_\\特殊')
      expect(state.calls.at(-1)?.has('cursor')).toBe(false)
      await expect(
        page.getByRole('button', { name: '上一頁', exact: true }),
      ).toBeDisabled()
      await expect(page.locator('html')).toHaveAttribute('data-theme', theme)
      const controls = [
        search.locator('..'),
        page
          .getByRole('combobox', { name: '申請狀態', exact: true })
          .locator('..')
          .locator('..'),
        page
          .getByRole('combobox', { name: '每頁筆數', exact: true })
          .locator('..')
          .locator('..'),
        page.getByRole('button', { name: /搜\s*尋/, exact: true }),
        page.getByRole('button', { name: '下一頁', exact: true }),
      ]
      for (const control of controls) {
        const box = await control.boundingBox()
        expect(box?.height).toBeGreaterThanOrEqual(44)
      }
      expect(
        await page.evaluate(
          () => document.documentElement.scrollWidth <= window.innerWidth + 1,
        ),
      ).toBe(true)
      await page.screenshot({
        path: testInfo.outputPath('request-query.png'),
        fullPage: true,
      })
      expect(errors).toEqual([])
    })
  }
}

test('400 不循環重試；明確回第一頁，通知與手動重新整理清除舊游標', async ({
  page,
}) => {
  const state = await prepare(page)
  state.failure = 400
  await page.getByRole('button', { name: '下一頁', exact: true }).click()
  const alert = page.getByRole('alert')
  await expect(alert).toContainText('查詢條件或游標已失效')
  const count = state.calls.length
  await expect(
    page.getByRole('button', { name: '下一頁', exact: true }),
  ).toBeDisabled()
  expect(state.calls).toHaveLength(count)
  state.failure = 0
  await alert
    .getByRole('button', { name: '重新載入第一頁', exact: true })
    .click()
  await expect(
    page.getByRole('link', { name: '部署 000', exact: true }),
  ).toBeVisible()
  expect(state.calls.at(-1)?.has('cursor')).toBe(false)
  await page.getByRole('button', { name: '下一頁', exact: true }).click()
  await expect(
    page.getByRole('link', { name: '部署 020', exact: true }),
  ).toBeVisible()
  await page.evaluate(() =>
    window.requestListNotify('deployment.execution.completed'),
  )
  await expect(page.getByRole('alert')).toContainText('已回到第一頁')
  await expect(
    page.getByRole('link', { name: '部署 000', exact: true }),
  ).toBeVisible()
  expect(state.calls.at(-1)?.has('cursor')).toBe(false)
  await page.getByRole('button', { name: '下一頁', exact: true }).click()
  await expect(
    page.getByRole('link', { name: '部署 020', exact: true }),
  ).toBeVisible()
  await page.getByRole('button', { name: '重新整理列表', exact: true }).click()
  await expect(
    page.getByRole('link', { name: '部署 000', exact: true }),
  ).toBeVisible()
  expect(state.calls.at(-1)?.has('cursor')).toBe(false)
})

test('遲到名稱查詢不蓋掉新條件，status 對全 scope 篩選與無匹配提示', async ({
  page,
}) => {
  const state = await prepare(page)
  let release: (() => void) | undefined
  state.delay = new Promise<void>((resolve) => {
    release = resolve
  })
  state.delayedSearch = '部署 000'
  const search = page.getByRole('textbox', { name: '名稱搜尋', exact: true })
  await search.fill(state.delayedSearch)
  await search.press('Enter')
  await expect
    .poll(() => state.calls.at(-1)?.get('search'))
    .toBe(state.delayedSearch)
  await expect(
    page.getByRole('link', { name: '部署 000', exact: true }),
  ).toHaveCount(0)
  await search.fill('%_\\特殊')
  await search.press('Enter')
  await expect(
    page.getByRole('link', { name: '後頁部署 %_\\特殊', exact: true }),
  ).toBeVisible()
  release?.()
  await expect.poll(() => state.oldResponded).toBe(true)
  await expect(
    page.getByRole('link', { name: '部署 000', exact: true }),
  ).toHaveCount(0)
  await page.getByRole('combobox', { name: '申請狀態', exact: true }).click()
  await page.getByRole('option', { name: 'Failed', exact: true }).click()
  await expect(
    page.getByText('沒有符合名稱或狀態條件的部署申請，請調整或清除條件。'),
  ).toBeVisible()
  expect(state.calls.at(-1)?.get('status')).toBe('Failed')
  await page.getByRole('button', { name: '清除條件', exact: true }).click()
  await expect(
    page.getByRole('link', { name: '部署 000', exact: true }),
  ).toBeVisible()
  expect(state.calls.at(-1)?.has('search')).toBe(false)
  expect(state.calls.at(-1)?.has('status')).toBe(false)
})

test('後頁空資料不推論整個 scope 為空，保留上一頁與刷新', async ({ page }) => {
  const state = await prepare(page)
  state.emptyNextPage = true
  await page.getByRole('button', { name: '下一頁', exact: true }).click()
  await expect(
    page.getByText('本頁沒有部署申請，可回上一頁或重新整理列表。'),
  ).toBeVisible()
  await expect(
    page.getByText('目前 scope 沒有 Deployment Request。'),
  ).toHaveCount(0)
  await expect(
    page.getByRole('button', { name: '上一頁', exact: true }),
  ).toBeEnabled()
  await expect(
    page.getByRole('button', { name: '下一頁', exact: true }),
  ).toBeDisabled()
  await page.getByRole('button', { name: '重新整理列表', exact: true }).click()
  await expect(bodyRows(page)).toHaveCount(20)
  expect(state.calls.at(-1)?.has('cursor')).toBe(false)
})
