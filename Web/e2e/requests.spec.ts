import { expect, test, type Page } from '@playwright/test'

import type { DeploymentRequestVersion } from '../src/generated/model'

declare global {
  interface Window {
    requestListRuntimeErrors: string[]
  }
}

const ids = {
  organization: '019c1230-0000-7000-8000-000000000201',
  project: '019c1230-0000-7000-8000-000000000202',
  environment: '019c1230-0000-7000-8000-000000000203',
  request: '019c1230-0000-7000-8000-000000000204',
  version: '019c1230-0000-7000-8000-000000000205',
  workflow: '019c1230-0000-7000-8000-000000000206',
  workflowVersion: '019c1230-0000-7000-8000-000000000207',
  plan: '019c1230-0000-7000-8000-000000000208',
  planVersion: '019c1230-0000-7000-8000-000000000209',
  execution: '019c1230-0000-7000-8000-000000000210',
  appA: '019c1230-0000-7000-8000-000000000211',
  appB: '019c1230-0000-7000-8000-000000000212',
}

test.beforeEach(async ({ context, page }) => {
  await context.addCookies([
    {
      name: 'releasehub_csrf',
      value: 'csrf-token',
      domain: '127.0.0.1',
      path: '/',
    },
  ])
  await page.addInitScript(() => {
    localStorage.setItem('releasehub.language', 'zh-TW')
    localStorage.setItem('releasehub.theme', 'dark')
    class TestEventSource {
      onmessage = null
      addEventListener() {}
      close() {}
    }
    Object.defineProperty(window, 'EventSource', { value: TestEventSource })
  })
})

for (const theme of ['light', 'dark']) {
  for (const width of [320, 768, 1440]) {
    test(`緊湊列表 ${theme} ${width}px：50 筆、完整 ID、排程唯讀與鍵盤`, async ({
      page,
      context,
    }, testInfo) => {
      await page.setViewportSize({ width, height: 900 })
      await context.grantPermissions(['clipboard-read', 'clipboard-write'])
      await page.addInitScript((theme) => {
        localStorage.setItem('releasehub.theme', theme)
        window.requestListRuntimeErrors = []
        window.addEventListener('error', (event) =>
          window.requestListRuntimeErrors.push(event.message),
        )
      }, theme)
      const state = {
        request: requestFixture(),
        retryBody: undefined as unknown,
      }
      await mockApplication(page, state)
      const title = '重名長部署申請與跨服務發布'.repeat(16)
      const statuses = [
        'Succeeded',
        'Failed',
        'PartialFailed',
        'Superseded',
        'Terminated',
        'Approved',
        'Deploying',
        'Blocked',
        'PendingReview',
        'Candidate',
      ] as const
      const rows = Array.from({ length: 50 }, (_, index) => ({
        ...requestSummary(state.request),
        id: `11111111-1111-4111-8111-${String(index).padStart(12, '0')}`,
        title,
        status: statuses[index % statuses.length],
        classification: index % 2 ? 'Standard' : 'ForwardRollback',
        scheduleState: 'Ready',
        scheduleReason: 'Ready',
        scheduledFor: undefined,
      }))
      const queryParams: string[] = []
      const mutations: string[] = []
      page.on('request', (request) => {
        if (
          request.url().includes('/api/v1/deployment-requests') &&
          request.method() !== 'GET'
        )
          mutations.push(request.method())
      })
      await page.route('**/api/v1/deployment-requests?**', (route) => {
        const url = new URL(route.request().url())
        queryParams.push(url.search)
        const limit = Number(url.searchParams.get('limit'))
        return route.fulfill(
          json({
            data: rows.slice(0, limit),
            meta: {
              ...meta(),
              hasMore: rows.length > limit,
              ...(rows.length > limit ? { nextCursor: 'list-next' } : {}),
            },
          }),
        )
      })
      await page.goto('/requests')
      await page.getByRole('combobox', { name: 'Project', exact: true }).click()
      await page.getByRole('option', { name: 'Project A', exact: true }).click()
      await page
        .getByRole('combobox', { name: '環境（必填）', exact: true })
        .click()
      await page
        .getByRole('option', { name: 'production', exact: true })
        .click()
      const table = page.locator('.ant-table')
      const bodyRows = table.locator('tbody tr[data-row-key]')
      await expect(bodyRows).toHaveCount(20)
      await page
        .getByRole('combobox', { name: '每頁筆數', exact: true })
        .click()
      await page.getByRole('option', { name: '50 筆', exact: true }).click()
      await expect(bodyRows).toHaveCount(50)
      await expect(page.locator('html')).toHaveAttribute('data-theme', theme)
      const heights = await bodyRows.evaluateAll((rows) =>
        rows.map((row) => row.getBoundingClientRect().height),
      )
      expect(Math.min(...heights)).toBeGreaterThanOrEqual(56)
      expect(Math.max(...heights)).toBeLessThanOrEqual(72)
      const neutralContrast = await table
        .getByText('Superseded', { exact: true })
        .first()
        .evaluate((node) => {
          const style = getComputedStyle(node)
          const luminance = (color: string) => {
            const rgb = color.match(/\d+(?:\.\d+)?/g)?.slice(0, 3)
            if (!rgb || rgb.length !== 3) throw new Error('無法解析標籤色彩')
            const values = rgb.map((part) => {
              const channel = Number(part) / 255
              return channel <= 0.04045
                ? channel / 12.92
                : ((channel + 0.055) / 1.055) ** 2.4
            })
            return values[0] * 0.2126 + values[1] * 0.7152 + values[2] * 0.0722
          }
          const text = luminance(style.color)
          const background = luminance(style.backgroundColor)
          return (
            (Math.max(text, background) + 0.05) /
            (Math.min(text, background) + 0.05)
          )
        })
      expect(neutralContrast).toBeGreaterThanOrEqual(4.5)
      const first = bodyRows.first()
      const link = first.getByRole('link', { name: title })
      await expect(link).toHaveAttribute('href', `/requests/${rows[0].id}`)
      expect(
        await link.evaluate((node) => node.scrollWidth > node.clientWidth),
      ).toBe(true)
      await expect(table.getByText('目前可部署', { exact: true })).toHaveCount(
        0,
      )
      await expect(
        table.getByText('已符合所有排程條件', { exact: true }),
      ).toHaveCount(0)
      await expect(
        table.getByText('查看排程條件', { exact: true }),
      ).toHaveCount(25)
      expect(
        await page.evaluate(
          () => document.documentElement.scrollWidth <= innerWidth + 1,
        ),
      ).toBe(true)
      const identity = first.getByRole('button', {
        name: '查看完整名稱與 ID：11111111…00000000',
        exact: true,
      })
      await identity.scrollIntoViewIfNeeded()
      await identity.focus()
      await identity.press('Enter')
      const dialog = page.getByRole('dialog')
      await expect(dialog).toBeVisible()
      await expect(dialog.getByText(title, { exact: true })).toBeVisible()
      await expect(dialog.getByText(rows[0].id, { exact: true })).toBeVisible()
      await expect(dialog).toContainText('不代表部署或重試授權')
      await expect(
        dialog.getByText('指定最早時間', { exact: true }),
      ).toHaveCount(0)
      const copy = dialog.getByRole('button', {
        name: '複製完整 ID',
        exact: true,
      })
      await copy.focus()
      await copy.press('Enter')
      await expect(dialog.getByRole('status')).toContainText('已複製完整 ID')
      expect(await page.evaluate(() => navigator.clipboard.readText())).toBe(
        rows[0].id,
      )
      const bounds = await dialog.boundingBox()
      expect(bounds?.x).toBeGreaterThanOrEqual(0)
      expect((bounds?.x ?? 0) + (bounds?.width ?? 0)).toBeLessThanOrEqual(width)
      expect(
        await dialog.evaluate(
          (node) => node.scrollWidth <= node.clientWidth + 1,
        ),
      ).toBe(true)
      await page.screenshot({
        path: testInfo.outputPath('request-details.png'),
      })
      await copy.press('Escape')
      await expect(dialog).toBeHidden()
      await expect(identity).toBeFocused()
      const schedule = first.getByRole('button', {
        name: '查看排程條件：11111111…00000000',
        exact: true,
      })
      await schedule.scrollIntoViewIfNeeded()
      await schedule.focus()
      await schedule.press('Enter')
      await expect(dialog).toBeVisible()
      await expect(dialog).toContainText('已符合排程條件')
      await dialog
        .getByRole('button', { name: '關閉', exact: true })
        .last()
        .click()
      await expect(dialog).toBeHidden()
      await expect(schedule).toBeFocused()
      await table.locator('.ant-table-content').evaluate((node) => {
        node.scrollLeft = 0
      })
      await page.screenshot({ path: testInfo.outputPath('request-list.png') })
      expect(queryParams).toHaveLength(2)
      expect(new URLSearchParams(queryParams[0]).get('limit')).toBe('20')
      expect(new URLSearchParams(queryParams[1]).get('limit')).toBe('50')
      expect(new URLSearchParams(queryParams[0]).get('environmentId')).toBe(
        ids.environment,
      )
      expect(new URLSearchParams(queryParams[0]).has('cursor')).toBe(false)
      expect(mutations).toEqual([])
      await page.evaluate(
        () =>
          new Promise<void>((resolve) =>
            requestAnimationFrame(() => requestAnimationFrame(() => resolve())),
          ),
      )
      expect(
        await page.evaluate(() => window.requestListRuntimeErrors),
      ).toEqual([])
    })
  }
}

test('自動 Request 經審核與部署後呈現 DAG 及 Partial Failed', async ({
  page,
}) => {
  const state = { request: requestFixture(), retryBody: undefined as unknown }
  await mockApplication(page, state)

  await page.goto('/requests')
  await page.getByRole('combobox', { name: 'Project', exact: true }).click()
  await page.getByRole('option', { name: 'Project A', exact: true }).click()
  await page
    .getByRole('combobox', { name: '環境（必填）', exact: true })
    .click()
  await page.getByRole('option', { name: 'production', exact: true }).click()
  await page.getByRole('link', { name: 'Automatic payment deployment' }).click()
  await expect(page.getByText('目前部署進度')).toBeVisible()
  await expect(page.getByRole('link', { name: '前往審核' })).toBeVisible()
  await expect(page.getByRole('button', { name: '核准申請' })).toBeVisible()

  await page.getByRole('button', { name: '核准申請' }).click()
  await expect(
    page.getByRole('link', { name: '前往 Workflow 操作' }),
  ).toBeVisible()
  await page.getByRole('button', { name: 'deploy' }).click()
  await expect(page.getByText('Partial Failed').first()).toBeVisible()
  await expect(
    page.getByRole('link', { name: '檢視失敗項目並重試' }),
  ).toBeVisible()
  await expect(page.getByText('等待：app-a')).toBeVisible()
  await expect(page.getByText('Synced · Healthy').first()).toBeVisible()
  await expect(page.getByText('sync failed')).toBeVisible()

  await page.getByLabel('選取 app-b 進行重試').check()
  await page.getByRole('button', { name: '重試選取的失敗項目' }).click()
  await expect
    .poll(() => state.retryBody)
    .toMatchObject({
      applicationIds: [ids.appB],
      expectedVersion: 1,
    })
})

test('Forward Rollback 仍顯示正常審核，Superseded 版本不可操作', async ({
  page,
}) => {
  const request = requestFixture()
  request.classification = 'ForwardRollback'
  const state = { request, retryBody: undefined as unknown }
  await mockApplication(page, state)

  await page.goto(`/requests/${ids.request}`)
  await expect(page.getByText('Forward Rollback').first()).toBeVisible()
  await expect(page.getByRole('button', { name: '核准申請' })).toBeVisible()

  state.request = {
    ...state.request,
    status: 'Superseded',
    classification: 'Standard',
    capabilities: [],
  }
  await page.reload()
  await expect(page.getByText('Superseded').first()).toBeVisible()
  await expect(page.getByRole('button', { name: '核准申請' })).toHaveCount(0)
  await expect(page.getByRole('button', { name: 'deploy' })).toHaveCount(0)
})

test('多 Application 切換後各自保留拓樸視角', async ({ page }) => {
  const state = {
    request: deployedRequest(requestFixture()),
    retryBody: undefined as unknown,
  }
  await mockApplication(page, state)
  await page.route(
    '**/api/v1/catalog/applications/*/runtime/topology?**',
    (route) => {
      const applicationId = new URL(route.request().url()).pathname.split(
        '/',
      )[5]
      return route.fulfill(
        json({ data: topologyFixture(applicationId), meta: meta() }),
      )
    },
  )

  await page.goto(`/requests/${ids.request}`)
  await expect(page.getByLabel('Application 即時資源拓撲')).toHaveCount(0)
  await page.getByText('Application 即時部署狀態').click()
  const canvas = page.getByLabel('Application 即時資源拓撲')
  await expect(canvas.getByText('app-a-pod')).toBeVisible()
  await canvas.locator('.react-flow__controls-zoomin').click()
  const viewport = canvas.locator('.react-flow__viewport')
  const first = await viewport.getAttribute('style')

  await page.getByLabel('選擇 Application').click()
  await page.getByText('app-b', { exact: true }).last().click()
  await expect(canvas.getByText('app-b-pod')).toBeVisible()
  await canvas.locator('.react-flow__controls-zoomout').click()
  const second = await viewport.getAttribute('style')
  expect(second).not.toBe(first)

  await page.getByLabel('選擇 Application').click()
  await page.getByText('app-a', { exact: true }).last().click()
  await expect(canvas.getByText('app-a-pod')).toBeVisible()
  await expect(viewport).toHaveAttribute('style', first ?? '')

  await page.getByLabel('選擇 Application').click()
  await page.getByText('app-b', { exact: true }).last().click()
  await expect(canvas.getByText('app-b-pod')).toBeVisible()
  await expect(viewport).toHaveAttribute('style', second ?? '')
})

for (const [theme, width] of [
  ['light', 900],
  ['dark', 900],
  ['light', 320],
  ['dark', 320],
] as const)
  test(`Request ${theme} ${width}px 分層拓樸共用群組及全部資源模式`, async ({
    page,
  }) => {
    await page.setViewportSize({ width, height: 900 })
    await page.addInitScript((mode) => {
      localStorage.setItem('releasehub.theme', mode)
    }, theme)
    const state = {
      request: deployedRequest(requestFixture()),
      retryBody: undefined as unknown,
    }
    await mockApplication(page, state)
    await page.route(
      '**/api/v1/catalog/applications/*/runtime/topology?**',
      (route) => {
        const applicationId = new URL(route.request().url()).pathname.split(
          '/',
        )[5]
        const base = topologyFixture(applicationId)
        const template = base.nodes[0]
        const node = (id: string, kind: string) => ({
          ...template,
          id,
          name: id,
          kind,
        })
        return route.fulfill(
          json({
            data: {
              ...base,
              nodes: [
                node('deployment', 'Deployment'),
                node('replica-1', 'ReplicaSet'),
                node('replica-2', 'ReplicaSet'),
                node('pod-1', 'Pod'),
                node('pod-2', 'Pod'),
                node('service', 'Service'),
                node('secret', 'Secret'),
                node('policy', 'NetworkPolicy'),
              ],
              edges: [
                {
                  id: 'one',
                  source: 'deployment',
                  target: 'replica-1',
                  kind: 'resource',
                },
                {
                  id: 'two',
                  source: 'deployment',
                  target: 'replica-2',
                  kind: 'resource',
                },
                {
                  id: 'three',
                  source: 'replica-1',
                  target: 'pod-1',
                  kind: 'resource',
                },
                {
                  id: 'four',
                  source: 'replica-2',
                  target: 'pod-2',
                  kind: 'resource',
                },
              ],
            },
            meta: meta(),
          }),
        )
      },
    )

    await page.goto(`/requests/${ids.request}`)
    await page.getByText('Application 即時部署狀態').click()
    const canvas = page.getByLabel('Application 即時資源拓撲')
    await expect(canvas.locator('.react-flow__node-runtime-group')).toHaveCount(
      width < 600 ? 1 : 2,
    )
    const expectedCard =
      theme === 'light' ? 'rgb(255, 255, 255)' : 'rgb(15, 23, 42)'
    const expectedIcon =
      theme === 'light' ? 'rgb(246, 248, 251)' : 'rgb(9, 11, 18)'
    for (const kind of ['runtime', 'runtime-group']) {
      const item = canvas.locator(`.react-flow__node-${kind}`).first()
      await expect(item.locator('button')).toHaveCSS(
        'background-color',
        expectedCard,
      )
      await expect(item.locator('[class*="nodeIcon"]')).toHaveCSS(
        'background-color',
        expectedIcon,
      )
    }
    await canvas.screenshot({
      path: `/tmp/releasehub-request-palette-after-${theme}-${width}.png`,
    })
    await expect(
      canvas.locator('.react-flow__controls-button').first(),
    ).toHaveCSS(
      'background-color',
      theme === 'light' ? 'rgb(255, 255, 255)' : 'rgb(15, 23, 42)',
    )
    await expect(canvas.locator('.react-flow__node-runtime')).toHaveCount(2)
    const zoom = await canvas
      .locator('.react-flow__viewport')
      .evaluate(
        (element) =>
          new DOMMatrixReadOnly(getComputedStyle(element).transform).a,
      )
    expect(zoom).toBeGreaterThanOrEqual(1)
    if (width < 600) {
      await expect
        .poll(() =>
          canvas.evaluate((element) => {
            const bounds = element.getBoundingClientRect()
            return [...element.querySelectorAll('.react-flow__node')].every(
              (node) => {
                const rect = node.getBoundingClientRect()
                return (
                  rect.left >= bounds.left + 4 &&
                  rect.right <= bounds.right - 4 &&
                  rect.top >= bounds.top + 4 &&
                  rect.bottom <= bounds.bottom - 4
                )
              },
            )
          }),
        )
        .toBe(true)
    }
    await page.getByText('全部資源', { exact: true }).click()
    await expect(canvas.locator('.react-flow__node-runtime')).toHaveCount(8)
    await page.getByText('分層總覽', { exact: true }).click()
    await expect(canvas.locator('.react-flow__node-runtime-group')).toHaveCount(
      width < 600 ? 1 : 2,
    )
  })

test('每個 Application 的執行證據可獨立展開並進入其拓樸', async ({ page }) => {
  const state = {
    request: deployedRequest(requestFixture()),
    retryBody: undefined as unknown,
  }
  await mockApplication(page, state)
  await page.route(
    '**/api/v1/catalog/applications/*/runtime/topology?**',
    (route) => {
      const applicationId = new URL(route.request().url()).pathname.split(
        '/',
      )[5]
      return route.fulfill(
        json({ data: topologyFixture(applicationId), meta: meta() }),
      )
    },
  )

  await page.goto(`/requests/${ids.request}`)
  const node = page
    .locator('.ant-collapse-item')
    .filter({ has: page.getByText('app-b', { exact: true }) })
    .first()
  await expect(node.getByText('sync failed')).toBeVisible()
  await node.locator('.ant-collapse-header').click()
  await expect(node.getByText('sync failed')).not.toBeVisible()
  await node.locator('.ant-collapse-header').click()
  await expect(node.getByText('sync failed')).toBeVisible()
  await node.getByRole('button', { name: '查看 app-b 的資源拓撲' }).click()
  await expect(
    page.getByLabel('Application 即時資源拓撲').getByText('app-b-pod'),
  ).toBeVisible()
})

for (const theme of ['light', 'dark']) {
  for (const width of [320, 768, 1440]) {
    test(`進度優先詳情 ${theme} ${width}px 保持可讀且不水平溢位`, async ({
      page,
    }, testInfo) => {
      await page.setViewportSize({ width, height: 900 })
      await page.addInitScript((selectedTheme) => {
        localStorage.setItem('releasehub.theme', selectedTheme)
      }, theme)
      const request = deployedRequest(requestFixture())
      request.title = '跨服務部署申請'.repeat(12)
      const state = { request, retryBody: undefined as unknown }
      await mockApplication(page, state)

      await page.goto(`/requests/${ids.request}`)
      await expect(page.locator('html')).toHaveAttribute('data-theme', theme)
      await expect(page.getByText('目前部署進度')).toBeVisible()
      await expect(
        page.getByRole('link', { name: '檢視失敗項目並重試' }),
      ).toBeVisible()
      await expect(page.getByText('受影響 Applications（2）')).toBeVisible()
      expect(
        await page.evaluate(
          () => document.documentElement.scrollWidth <= innerWidth + 1,
        ),
      ).toBe(true)
      await page.screenshot({ path: testInfo.outputPath('progress-first.png') })
      const nextAction = page.getByRole('link', {
        name: '檢視失敗項目並重試',
      })
      await nextAction.focus()
      await nextAction.press('Enter')
      await expect(page).toHaveURL(/#request-application-evidence$/)
      await expect(page.locator('#request-application-evidence')).toBeFocused()
    })
  }
}

test('單一 Application 拓樸失敗不遮蔽 Request 與其他 Application', async ({
  page,
}) => {
  const state = {
    request: deployedRequest(requestFixture()),
    retryBody: undefined as unknown,
  }
  await mockApplication(page, state)
  await page.route(
    '**/api/v1/catalog/applications/*/runtime/topology?**',
    (route) => {
      const applicationId = new URL(route.request().url()).pathname.split(
        '/',
      )[5]
      if (applicationId === ids.appA)
        return route.fulfill({
          status: 503,
          ...json({ error: { code: 'argocd_unavailable' } }),
        })
      return route.fulfill(
        json({ data: topologyFixture(applicationId), meta: meta() }),
      )
    },
  )

  await page.goto(`/requests/${ids.request}`)
  await page.getByText('Application 即時部署狀態').click()
  await expect(page.getByText('無法取得 Application 即時資源。')).toBeVisible()
  await expect(
    page.getByRole('heading', { name: 'Automatic payment deployment' }),
  ).toBeVisible()
  await expect(page.getByText('Partial Failed').first()).toBeVisible()

  await page.getByLabel('選擇 Application').click()
  await page.getByText('app-b', { exact: true }).last().click()
  await expect(
    page.getByLabel('Application 即時資源拓撲').getByText('app-b-pod'),
  ).toBeVisible()
})

for (const warning of [
  { code: 'node_limit', text: '資源超過 500 個節點，畫面僅顯示有界結果。' },
  { code: 'edge_limit', text: '關係超過 1,000 條，畫面僅顯示有界結果。' },
  {
    code: 'network_evidence_unresolved',
    text: '部分 Argo CD 網路關係證據無法安全解析。',
  },
]) {
  test(`Request 的非空 ${warning.code} 不改變部署結果或其他 Application`, async ({
    page,
  }) => {
    const state = {
      request: deployedRequest(requestFixture()),
      retryBody: undefined as unknown,
    }
    await mockApplication(page, state)
    let recovered = false
    await page.route(
      '**/api/v1/catalog/applications/*/runtime/topology?**',
      (route) => {
        const url = new URL(route.request().url())
        const applicationId = url.pathname.split('/')[5]
        const limited = applicationId === ids.appA && !recovered
        const base = topologyFixture(applicationId)
        const network = warning.code === 'network_evidence_unresolved'
        const serviceId = `service-${applicationId}`
        return route.fulfill(
          json({
            data: {
              ...base,
              view: url.searchParams.get('view'),
              nodes: network
                ? [
                    ...base.nodes,
                    {
                      ...base.nodes[0],
                      id: serviceId,
                      kind: 'Service',
                      name: `service-${applicationId}`,
                    },
                  ]
                : base.nodes,
              edges: network
                ? [
                    {
                      id: `network-${applicationId}`,
                      source: serviceId,
                      target: base.nodes[0].id,
                      kind: 'network',
                    },
                  ]
                : base.edges,
              warnings: limited ? [warning.code] : [],
              partial: limited,
            },
            meta: meta(),
          }),
        )
      },
    )
    await page.goto(`/requests/${ids.request}`)
    await page.getByText('Application 即時部署狀態').click()
    if (warning.code === 'network_evidence_unresolved')
      await page.getByText('網路拓撲', { exact: true }).click()
    await expect(page.getByRole('alert')).toContainText(warning.text)
    const canvas = page.getByLabel('Application 即時資源拓撲')
    await expect(canvas.getByText('app-a-pod')).toBeVisible()
    await expect(
      page.getByRole('heading', { name: 'Automatic payment deployment' }),
    ).toBeVisible()
    await expect(page.getByText('Partial Failed').first()).toBeVisible()
    await expect(page.getByText('sync failed')).toBeVisible()
    await page.getByLabel('選擇 Application').click()
    await page.getByText('app-b', { exact: true }).last().click()
    await expect(canvas.getByText('app-b-pod')).toBeVisible()
    await expect(page.getByText(warning.text)).toHaveCount(0)
    await page.getByLabel('選擇 Application').click()
    await page.getByText('app-a', { exact: true }).last().click()
    await expect(page.getByRole('alert')).toContainText(warning.text)
    const refresh = page.getByRole('button', { name: /重新整理/ })
    await expect(refresh).not.toHaveClass(/ant-btn-loading/)
    recovered = true
    await refresh.click()
    await expect(page.getByText(warning.text)).toHaveCount(0)
    await expect(canvas.getByText('app-a-pod')).toBeVisible()
    await expect(page.getByText('Partial Failed').first()).toBeVisible()
    expect(state.retryBody).toBeUndefined()
  })
}

test('舊版拓樸回傳 null 陣列時 Request 詳細頁仍可顯示', async ({ page }) => {
  await page.setViewportSize({ width: 1440, height: 900 })
  const state = {
    request: deployedRequest(requestFixture()),
    retryBody: undefined as unknown,
  }
  await mockApplication(page, state)
  await page.route(
    `**/api/v1/catalog/applications/${ids.appA}/runtime/topology?**`,
    (route) =>
      route.fulfill(
        json({
          data: {
            ...topologyFixture(ids.appA),
            nodes: topologyFixture(ids.appA).nodes.map((node) => ({
              ...node,
              images: null,
              info: null,
              ingress: null,
              externalUrls: null,
            })),
            edges: null,
            warnings: null,
          },
          meta: meta(),
        }),
      ),
  )

  await page.goto(`/requests/${ids.request}`)
  await page.getByText('Application 即時部署狀態').click()

  await expect(
    page.getByRole('heading', { name: 'Automatic payment deployment' }),
  ).toBeVisible()
  await expect(
    page.getByLabel('Application 即時資源拓撲').getByText('app-a-pod'),
  ).toBeVisible()
  await expect(page.getByText('無法顯示此頁面')).toHaveCount(0)
  await page
    .getByLabel('Application 即時資源拓撲')
    .getByRole('button', { name: /app-a-pod/ })
    .click()
  await expect(page.getByRole('dialog')).toBeVisible()
  await expect(page.getByRole('dialog').getByText('Images')).toBeVisible()
  await expect
    .poll(async () =>
      Math.round((await page.getByRole('dialog').boundingBox())?.width ?? 0),
    )
    .toBe(1296)
  await expect(page.getByText('無法顯示此頁面')).toHaveCount(0)
})

test('active 拓樸收合後停止定期查詢', async ({ page }) => {
  const request = deployedRequest(requestFixture())
  request.status = 'Deploying'
  request.executionStatus = 'Running'
  const state = { request, retryBody: undefined as unknown }
  await mockApplication(page, state)
  await page.route(
    `**/api/v1/deployment-executions/${ids.execution}`,
    (route) =>
      route.fulfill(
        json({
          data: { ...executionFixture(), status: 'Running' },
          meta: meta(),
        }),
      ),
  )
  let topologyRequests = 0
  await page.route(
    `**/api/v1/catalog/applications/${ids.appA}/runtime/topology?**`,
    (route) => {
      topologyRequests += 1
      return route.fulfill(
        json({ data: topologyFixture(ids.appA), meta: meta() }),
      )
    },
  )
  await page.clock.install()
  await page.goto(`/requests/${ids.request}`)
  await expect(page.getByLabel('Application 即時資源拓撲')).toBeVisible()
  expect(topologyRequests).toBeGreaterThan(0)

  await page.getByText('Application 即時部署狀態').click()
  await expect(page.getByLabel('Application 即時資源拓撲')).not.toBeVisible()
  const requestsAfterClose = topologyRequests
  await page.clock.runFor(6000)
  expect(topologyRequests).toBe(requestsAfterClose)
})

test('scope-limited 與 explicit-denied actor 的 UI 與直接 API 拒絕一致', async ({
  page,
}) => {
  const scopeLimited = requestFixture()
  const state = { request: scopeLimited, retryBody: undefined as unknown }
  await mockApplication(page, state)

  await page.goto(`/requests/${ids.request}`)
  await expect(page.getByRole('button', { name: '核准申請' })).toBeVisible()
  await expect(page.getByRole('button', { name: '編輯選填資訊' })).toHaveCount(
    0,
  )
  await expect(
    page.getByRole('button', { name: '重試選取的失敗項目' }),
  ).toHaveCount(0)

  state.request = { ...state.request, capabilities: [] }
  await page.reload()
  await expect(page.getByRole('button', { name: '核准申請' })).toHaveCount(0)

  const status = await page.evaluate(
    async ({ requestID, versionID }) => {
      const response = await fetch(
        `/api/v1/deployment-requests/${requestID}/versions/${versionID}/retry`,
        {
          method: 'POST',
          headers: {
            'Content-Type': 'application/json',
            'X-CSRF-Token': 'csrf-token',
            'Idempotency-Key': 'matrix-explicit-deny',
          },
          body: JSON.stringify({
            applicationIds: ['019c1230-0000-7000-8000-000000000212'],
            expectedVersion: 1,
          }),
        },
      )
      return response.status
    },
    { requestID: ids.request, versionID: ids.version },
  )
  expect(status).toBe(404)
  expect(state.retryBody).toBeUndefined()
})

async function mockApplication(
  page: Page,
  state: { request: DeploymentRequestVersion; retryBody: unknown },
) {
  await page.route('**/api/v1/auth/session', (route) =>
    route.fulfill(
      json({ data: { id: 'user-1', username: 'vincent' }, meta: meta() }),
    ),
  )
  await page.route('**/api/v1/notifications**', (route) =>
    route.fulfill(json({ data: [], meta: meta() })),
  )
  await page.route('**/api/v1/catalog/resource-tree', (route) =>
    route.fulfill(json({ data: resourceTree(), meta: meta() })),
  )
  await page.route('**/api/v1/release-workflows**', (route) =>
    route.fulfill(json({ data: [workflowFixture()], meta: meta() })),
  )
  await page.route('**/api/v1/deployment-plans**', (route) =>
    route.fulfill(json({ data: [planFixture()], meta: meta() })),
  )
  await page.route('**/api/v1/deployment-history**', (route) =>
    route.fulfill(json({ data: [], meta: meta() })),
  )
  await page.route(
    `**/api/v1/deployment-executions/${ids.execution}`,
    (route) => route.fulfill(json({ data: executionFixture(), meta: meta() })),
  )
  await page.route('**/api/v1/deployment-requests**', async (route) => {
    const request = route.request()
    const url = new URL(request.url())
    if (
      request.method() === 'GET' &&
      url.pathname === '/api/v1/deployment-requests'
    )
      return route.fulfill(
        json({
          data: [requestSummary(state.request)],
          meta: { ...meta(), hasMore: false },
        }),
      )
    if (request.method() === 'GET')
      return route.fulfill(json({ data: state.request, meta: meta() }))
    if (url.pathname.endsWith('/decisions')) {
      if (!state.request.capabilities.includes('deployment_request.review'))
        return route.fulfill({ status: 404, ...json({}) })
      state.request = approvedRequest(state.request)
      return route.fulfill(json({ data: state.request, meta: meta() }))
    }
    if (url.pathname.endsWith('/transitions')) {
      if (!state.request.capabilities.includes('deployment_request.deploy'))
        return route.fulfill({ status: 404, ...json({}) })
      state.request = deployedRequest(state.request)
      return route.fulfill({
        status: 202,
        ...json({ data: { accepted: true }, meta: meta() }),
      })
    }
    if (url.pathname.endsWith('/retry')) {
      if (!state.request.capabilities.includes('deployment_request.retry'))
        return route.fulfill({ status: 404, ...json({}) })
      state.retryBody = request.postDataJSON()
      return route.fulfill({
        status: 202,
        ...json({ data: { accepted: true }, meta: meta() }),
      })
    }
    return route.fulfill({ status: 404, ...json({}) })
  })
}

function requestFixture(): DeploymentRequestVersion {
  return {
    id: ids.version,
    requestId: ids.request,
    organizationId: ids.organization,
    projectId: ids.project,
    environmentId: ids.environment,
    versionNumber: 1,
    status: 'PendingReview',
    classification: 'Standard',
    fingerprint: 'fingerprint-1',
    workflowVersionId: ids.workflowVersion,
    planVersionId: ids.planVersion,
    title: 'Automatic payment deployment',
    changeDescription: '',
    issueUrl: '',
    scheduleState: 'Waiting',
    nextEligibleAt: '2026-09-07T01:00:00Z',
    scheduleReason: 'MaintenanceWindow',
    lockVersion: 1,
    workflowStateKey: 'review',
    capabilities: ['deployment_request.review'],
    createdAt: '2026-09-07T00:00:00Z',
    reviews: [
      {
        id: 'review-1',
        stateKey: 'review',
        stageNumber: 1,
        policyType: 'AnyApprover',
        requiredApprovals: 1,
        allowSelfReview: false,
        status: 'Pending',
      },
    ],
    applications: [
      applicationSnapshot(ids.appA, 'app-a', 0),
      applicationSnapshot(ids.appB, 'app-b', 1),
    ],
  }
}

function approvedRequest(
  request: DeploymentRequestVersion,
): DeploymentRequestVersion {
  return {
    ...request,
    status: 'Approved',
    lockVersion: 2,
    workflowStateKey: 'approved',
    capabilities: ['deployment_request.deploy'],
    reviews: [{ ...request.reviews[0], status: 'Approved' }],
  }
}

function deployedRequest(
  request: DeploymentRequestVersion,
): DeploymentRequestVersion {
  return {
    ...request,
    status: 'PartialFailed',
    lockVersion: 3,
    workflowStateKey: 'result',
    executionId: ids.execution,
    executionStatus: 'PartialFailed',
    capabilities: ['deployment_request.retry'],
  }
}

function applicationSnapshot(id: string, key: string, order: number) {
  return {
    id: `snapshot-${key}`,
    applicationId: id,
    applicationKey: key,
    liveRevision: 'commit-a',
    targetRevision: 'commit-b',
    targetRevisions: ['commit-b'],
    manifestHash: `manifest-${key}`,
    diffHash: `diff-${key}`,
    order,
    images: [],
    diff: {
      resources: [
        { group: 'apps', kind: 'Deployment', namespace: 'payment', name: key },
      ],
    },
  }
}

function executionFixture() {
  return {
    id: ids.execution,
    requestVersionId: ids.version,
    planVersionId: ids.planVersion,
    attempt: 1,
    status: 'PartialFailed',
    triggerKind: 'Workflow',
    lockVersion: 1,
    createdAt: '2026-09-07T00:00:00Z',
    nodes: [
      executionNode(ids.appA, 'app-a', 'Succeeded', 'Synced', 'Healthy', ''),
      executionNode(
        ids.appB,
        'app-b',
        'Failed',
        'OutOfSync',
        'Degraded',
        'sync failed',
      ),
    ],
  }
}

function executionNode(
  applicationId: string,
  nodeKey: string,
  status: string,
  syncStatus: string,
  healthStatus: string,
  errorMessage: string,
) {
  return {
    id: `node-${nodeKey}`,
    applicationId,
    nodeKey,
    status,
    operationId: `operation-${nodeKey}`,
    syncStatus,
    healthStatus,
    actualRevision: 'commit-b',
    actualImages: [],
    errorCode: errorMessage ? 'SYNC_FAILED' : '',
    errorMessage,
  }
}

function workflowFixture() {
  return {
    id: ids.workflow,
    name: 'Production approval',
    description: '',
    active: true,
    versions: [
      {
        id: ids.workflowVersion,
        workflowId: ids.workflow,
        versionNumber: 1,
        lifecycle: 'Published',
        lockVersion: 1,
        createdAt: '2026-09-07T00:00:00Z',
        document: {
          initialState: 'review',
          states: [
            { key: 'review', name: 'Review', type: 'Review' },
            { key: 'approved', name: 'Approved', type: 'ManualAction' },
            { key: 'result', name: 'Result', type: 'Terminal' },
          ],
          transitions: [
            {
              key: 'deploy',
              from: 'approved',
              to: 'result',
              trigger: 'Manual',
              permission: 'deployment_request.deploy',
              conditions: [],
            },
          ],
        },
      },
    ],
  }
}

function planFixture() {
  return {
    id: ids.plan,
    ownerKind: 'Project',
    ownerProjectId: ids.project,
    name: 'Production plan',
    description: '',
    active: true,
    versions: [
      {
        id: ids.planVersion,
        planId: ids.plan,
        versionNumber: 1,
        lifecycle: 'Published',
        lockVersion: 1,
        createdAt: '2026-09-07T00:00:00Z',
        document: {
          maxParallel: 2,
          nodes: [
            { key: 'app-a', applicationKey: 'app-a', order: 0 },
            { key: 'app-b', applicationKey: 'app-b', order: 1 },
          ],
          edges: [
            { from: 'app-a', to: 'app-b', condition: 'UpstreamSucceeded' },
          ],
        },
      },
    ],
  }
}

function requestSummary(request: DeploymentRequestVersion) {
  return {
    id: ids.request,
    organizationId: ids.organization,
    projectId: ids.project,
    environmentId: ids.environment,
    status: request.status,
    classification: request.classification,
    activeVersionNumber: request.versionNumber,
    title: request.title,
    applicationCount: request.applications.length,
    scheduledFor: request.scheduledFor,
    scheduleState: request.scheduleState,
    nextEligibleAt: request.nextEligibleAt,
    scheduleReason: request.scheduleReason,
    updatedAt: request.createdAt,
  }
}

function resourceTree() {
  return [
    {
      id: ids.organization,
      name: 'Organization A',
      canCreateProject: false,
      projects: [
        {
          id: ids.project,
          name: 'Project A',
          canManage: false,
          environments: [
            {
              id: ids.environment,
              name: 'production',
              type: 'Production',
              applications: [],
            },
          ],
        },
      ],
    },
  ]
}

function topologyFixture(applicationId: string) {
  return {
    applicationId,
    view: 'resources',
    observedAt: new Date().toISOString(),
    nodes: [
      {
        id: `pod-${applicationId}`,
        group: '',
        version: 'v1',
        kind: 'Pod',
        namespace: 'payment',
        name: applicationId === ids.appA ? 'app-a-pod' : 'app-b-pod',
        healthStatus: 'Healthy',
        healthMessage: '',
        orphaned: false,
        images: [],
        info: [],
        ingress: [],
        externalUrls: [],
      },
    ],
    edges: [],
    warnings: [],
    partial: false,
  }
}

function json(body: unknown) {
  return { contentType: 'application/json', body: JSON.stringify(body) }
}

function meta() {
  return { requestId: 'e2e', timestamp: new Date().toISOString() }
}
