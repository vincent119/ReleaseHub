import { expect, test, type Page } from '@playwright/test'

const ids = {
  organization: '019c1230-0000-7000-8000-000000000101',
  project: '019c1230-0000-7000-8000-000000000102',
  environment: '019c1230-0000-7000-8000-000000000103',
  plan: '019c1230-0000-7000-8000-000000000104',
  planVersion: '019c1230-0000-7000-8000-000000000105',
  workflow: '019c1230-0000-7000-8000-000000000106',
  workflowVersion: '019c1230-0000-7000-8000-000000000107',
}

test('在 Environment scope 管理發布排程且保留 Server version', async ({
  context,
  page,
}) => {
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
  })
  await mockSession(page)
  await mockResources(page)
  await mockWorkflows(page)
  await page.route('**/api/v1/deployment-plans**', (route) =>
    route.fulfill(json({ data: [], meta: meta() })),
  )
  await page.route('**/api/v1/deployment-bindings**', (route) =>
    route.fulfill(json({ data: null, meta: meta() })),
  )
  let updateBody = null as Record<string, unknown> | null
  await page.route('**/api/v1/deployment-schedules/*', async (route) => {
    if (route.request().method() === 'PUT') {
      updateBody = route.request().postDataJSON()
      return route.fulfill(
        json({
          data: {
            environmentId: ids.environment,
            ...updateBody,
            version: 1,
            canManage: true,
          },
          meta: meta(),
        }),
      )
    }
    return route.fulfill(
      json({
        data: {
          environmentId: ids.environment,
          enabled: false,
          timeZone: 'UTC',
          weeklyWindows: [],
          blackouts: [],
          version: 0,
          canManage: true,
        },
        meta: meta(),
      }),
    )
  })

  await page.goto('/plans')
  await page.getByLabel('選擇 Project').click()
  await page.getByText('Organization A / Project A').click()
  await page.getByLabel('選擇 Environment').click()
  await page.getByText('production', { exact: true }).click()

  await expect(
    page.getByText('發布排程與維護時段', { exact: true }),
  ).toBeVisible()
  await page.getByLabel('IANA 時區').fill('Asia/Taipei')
  await page.getByRole('button', { name: '儲存排程政策' }).click()

  await expect
    .poll(() => updateBody)
    .toEqual({
      enabled: false,
      timeZone: 'Asia/Taipei',
      weeklyWindows: [],
      blackouts: [],
      expectedVersion: 0,
    })
})

test('建立 A/C 到 B 的 Plan 並綁定已發布版本', async ({ context, page }) => {
  await context.addCookies([
    {
      name: 'releasehub_csrf',
      value: 'csrf-token',
      domain: '127.0.0.1',
      path: '/',
    },
  ])
  await page.addInitScript(() =>
    localStorage.setItem('releasehub.language', 'zh-TW'),
  )
  await mockSession(page)
  await mockResources(page)
  await mockWorkflows(page)
  let plan = null as Record<string, unknown> | null
  let bindingBody = null as Record<string, unknown> | null
  await page.route('**/api/v1/deployment-plans**', async (route) => {
    const request = route.request()
    const url = new URL(request.url())
    if (request.method() === 'GET')
      return route.fulfill(json({ data: plan ? [plan] : [], meta: meta() }))
    if (url.pathname.endsWith('/lifecycle')) {
      const version = (plan!.versions as Record<string, unknown>[])[0]
      version.lifecycle = 'Published'
      version.lockVersion = 2
      return route.fulfill(json({ data: version, meta: meta() }))
    }
    const body = request.postDataJSON()
    plan = planResponse(body)
    return route.fulfill({ ...json({ data: plan, meta: meta() }), status: 201 })
  })
  await page.route('**/api/v1/deployment-bindings**', async (route) => {
    if (route.request().method() === 'GET')
      return route.fulfill(json({ data: null, meta: meta() }))
    bindingBody = route.request().postDataJSON()
    return route.fulfill(
      json({
        data: { id: 'binding-1', ...bindingBody, version: 1 },
        meta: meta(),
      }),
    )
  })

  await page.goto('/plans')
  await page.getByLabel('選擇 Project').click()
  await page.getByText('Organization A / Project A').click()
  await page.getByRole('button', { name: '建立 Plan' }).click()
  await page.getByRole('button', { name: '新增 Application' }).click()
  await page.getByRole('button', { name: '新增 Application' }).click()
  await addDependency(page, 'application_1', 'application_2')
  await addDependency(page, 'application_3', 'application_2')
  await page.getByRole('button', { name: '儲存 Plan' }).click()
  await expect(page.getByRole('button', { name: '發布版本' })).toBeVisible()

  const document = (plan!.versions as Record<string, unknown>[])[0]
    .document as Record<string, unknown>
  expect(document.edges).toEqual([
    {
      from: 'application_1',
      to: 'application_2',
      condition: 'UpstreamSucceeded',
    },
    {
      from: 'application_3',
      to: 'application_2',
      condition: 'UpstreamSucceeded',
    },
  ])
  await page.getByRole('button', { name: '發布版本' }).click()
  await page.getByLabel('選擇 Environment').click()
  await page.getByText('production', { exact: true }).click()
  await page.getByLabel('Release Workflow Version').click()
  await page.getByText('Production approval · v1').click()
  await page.getByLabel('Deployment Plan Version').click()
  await page.getByText('Production plan · v1').click()
  await page.getByRole('button', { name: '儲存 Binding' }).click()
  await expect
    .poll(() => bindingBody)
    .toMatchObject({
      organizationId: ids.organization,
      projectId: ids.project,
      environmentId: ids.environment,
      workflowVersionId: ids.workflowVersion,
      planVersionId: ids.planVersion,
      expectedVersion: 0,
    })
})

test('建立 Plan 名稱衝突時顯示具體通知並保留編輯內容', async ({
  context,
  page,
}) => {
  await context.addCookies([
    {
      name: 'releasehub_csrf',
      value: 'csrf-token',
      domain: '127.0.0.1',
      path: '/',
    },
  ])
  await page.addInitScript(() =>
    localStorage.setItem('releasehub.language', 'zh-TW'),
  )
  await mockSession(page)
  await mockResources(page)
  await mockWorkflows(page)
  await page.route('**/api/v1/deployment-plans**', (route) =>
    route.request().method() === 'POST'
      ? route.fulfill({ ...json({}), status: 409 })
      : route.fulfill(json({ data: [], meta: meta() })),
  )
  await page.route('**/api/v1/deployment-bindings**', (route) =>
    route.fulfill(json({ data: null, meta: meta() })),
  )

  await page.goto('/plans')
  await page.getByLabel('選擇 Project').click()
  await page.getByText('Organization A / Project A').click()
  await page.getByRole('button', { name: '建立 Plan' }).click()
  await page.getByRole('button', { name: '儲存 Plan' }).click()

  const errorNotice = page.getByText(
    '目前作用域已存在相同名稱的 Plan，請使用其他名稱。',
  )
  await expect(errorNotice).toBeVisible()
  await expect(
    page.getByRole('heading', { name: '建立 Deployment Plan' }),
  ).toBeVisible()
  await expect(page.getByLabel('Plan 名稱')).toHaveValue('Production plan')
  await expect(errorNotice).toBeHidden({ timeout: 10_000 })
})

for (const theme of ['light', 'dark'] as const) {
  test(`${theme} 主題 Plan Graph 與 Workflow 使用相同科技色調`, async ({
    page,
  }) => {
    await page.addInitScript((resolvedTheme) => {
      localStorage.setItem('releasehub.language', 'zh-TW')
      localStorage.setItem('releasehub.theme', resolvedTheme)
    }, theme)
    await mockSession(page)
    await mockResources(page)
    await mockWorkflows(page)
    await page.route('**/api/v1/deployment-plans**', (route) =>
      route.fulfill(json({ data: [], meta: meta() })),
    )
    await page.route('**/api/v1/deployment-bindings**', (route) =>
      route.fulfill(json({ data: null, meta: meta() })),
    )

    await page.goto('/plans')
    await page.getByLabel('選擇 Project').click()
    await page.getByText('Organization A / Project A').click()
    await page.getByRole('button', { name: '建立 Plan' }).click()
    await page.getByRole('button', { name: '新增 Application' }).click()
    await page.getByRole('button', { name: '新增 Application' }).click()
    await page.getByRole('button', { name: '新增 Application' }).click()

    const canvas = page.getByLabel('Deployment Plan 圖形編輯區')
    const editor = canvas.locator('..')
    const toolbar = page.getByRole('toolbar', {
      name: 'Deployment Plan 結構工具列',
    })
    const inspector = editor.locator('aside')
    const nodes = canvas.locator('.react-flow__node-default')
    const expected = planPalette[theme]
    await expect(nodes.first()).toBeVisible()
    const node = nodes.first()
    const stageStyles = await nodes.evaluateAll((elements) =>
      elements.map((element) => {
        const style = getComputedStyle(element)
        return {
          background: style.backgroundColor,
          borderTop: style.borderTopColor,
        }
      }),
    )
    expect(stageStyles).toEqual(expected.stages)
    const styles = await node.evaluate((element) => {
      const nodeStyle = getComputedStyle(element)
      const canvasStyle = getComputedStyle(element.closest('[aria-label]')!)
      return {
        color: nodeStyle.color,
        background: nodeStyle.backgroundColor,
        borderTop: nodeStyle.borderTopColor,
        canvasBackground: canvasStyle.backgroundColor,
        canvasAmbient: canvasStyle.backgroundImage,
      }
    })
    expect(styles.color).not.toBe(styles.background)
    expect(styles.background).toBe(expected.stages[0].background)
    expect(styles.borderTop).toBe(expected.stages[0].borderTop)
    expect(styles.canvasBackground).toBe(expected.canvas)
    if (theme === 'dark')
      expect(styles.canvasAmbient).toContain('radial-gradient')
    else expect(styles.canvasAmbient).toBe('none')
    await expect
      .poll(() =>
        toolbar.evaluate(
          (element) => getComputedStyle(element).backgroundColor,
        ),
      )
      .toBe(expected.elevated)
    await expect
      .poll(() =>
        inspector.evaluate(
          (element) => getComputedStyle(element).backgroundColor,
        ),
      )
      .toBe(expected.surface)

    await node.click()
    await expect(
      page.getByText('Application 設定', { exact: true }),
    ).toBeVisible()
    await expect
      .poll(() =>
        node.evaluate((element) => getComputedStyle(element).borderColor),
      )
      .toContain(expected.selected)
  })
}

test('Workflow 與 Plan 詳細頁使用一致的工作區與 Graph frame', async ({
  page,
}) => {
  await page.setViewportSize({ width: 1440, height: 900 })
  await page.addInitScript(() => {
    localStorage.setItem('releasehub.language', 'zh-TW')
    localStorage.setItem('releasehub.theme', 'dark')
  })
  await mockSession(page)
  await mockResources(page)
  await mockWorkflows(page)
  await page.route('**/api/v1/deployment-plans**', (route) =>
    route.fulfill(json({ data: [savedPlanResponse()], meta: meta() })),
  )
  await page.route('**/api/v1/deployment-bindings**', (route) =>
    route.fulfill(json({ data: null, meta: meta() })),
  )

  await page.goto('/plans')
  await page.getByLabel('選擇 Project').click()
  await page.getByText('Organization A / Project A').click()
  await expect(page.getByLabel('Deployment Plan 圖形編輯區')).toBeVisible()
  const planFrame = await graphFrameMetrics(page, 'Deployment Plan 結構工具列')
  const planSelection = await selectedListItemMetrics(page)
  const planHeader = page
    .locator('.ant-card-head')
    .filter({ hasText: 'Production plan' })
  await expect(planHeader.getByText('Draft', { exact: true })).toBeVisible()
  await expectReadableGraph(page, 'Deployment Plan 圖形編輯區')
  await expectFocusedListItem(page)

  await page.setViewportSize({ width: 720, height: 900 })
  const planCanvas = page.getByLabel('Deployment Plan 圖形編輯區')
  const planInspector = page.getByRole('complementary', { name: '設定面板' })
  await expect
    .poll(async () => (await planInspector.boundingBox())?.y ?? 0)
    .toBeGreaterThan((await planCanvas.boundingBox())?.y ?? 0)
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= window.innerWidth,
    ),
  ).toBe(true)

  await page.setViewportSize({ width: 1440, height: 900 })

  await page.goto('/workflows')
  await expect(page.getByLabel('Workflow 圖形編輯區')).toBeVisible()
  const workflowFrame = await graphFrameMetrics(page, 'Workflow 結構工具列')
  const workflowSelection = await selectedListItemMetrics(page)
  const workflowHeader = page
    .locator('.ant-card-head')
    .filter({ hasText: 'Production approval' })
  await expect(
    workflowHeader.getByText('Published', { exact: true }),
  ).toBeVisible()
  await expectReadableGraph(page, 'Workflow 圖形編輯區')
  await expectFocusedListItem(page)

  expect(planFrame).toEqual(workflowFrame)
  expect(planSelection).toEqual(workflowSelection)
  expect(Number.parseFloat(planFrame.inspectorWidth)).toBeLessThanOrEqual(328)
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= window.innerWidth,
    ),
  ).toBe(true)
})

const planPalette = {
  light: {
    canvas: 'rgb(246, 248, 251)',
    surface: 'rgb(255, 255, 255)',
    elevated: 'rgb(255, 255, 255)',
    stages: [
      {
        background: 'rgb(223, 227, 255)',
        borderTop: 'rgb(79, 70, 229)',
      },
      {
        background: 'rgb(255, 240, 194)',
        borderTop: 'rgb(138, 90, 0)',
      },
      {
        background: 'rgb(234, 220, 255)',
        borderTop: 'rgb(98, 53, 213)',
      },
      {
        background: 'rgb(220, 234, 255)',
        borderTop: 'rgb(20, 95, 215)',
      },
    ],
    selected: 'rgb(79, 70, 229)',
  },
  dark: {
    canvas: 'rgb(9, 11, 18)',
    surface: 'rgb(15, 23, 42)',
    elevated: 'rgb(17, 24, 39)',
    stages: [
      {
        background: 'rgb(33, 40, 68)',
        borderTop: 'rgb(129, 140, 248)',
      },
      {
        background: 'rgb(58, 45, 18)',
        borderTop: 'rgb(251, 191, 36)',
      },
      {
        background: 'rgb(46, 36, 80)',
        borderTop: 'rgb(196, 181, 253)',
      },
      {
        background: 'rgb(21, 46, 85)',
        borderTop: 'rgb(96, 165, 250)',
      },
    ],
    selected: 'rgb(165, 180, 252)',
  },
} as const

for (const theme of ['light', 'dark'] as const) {
  test(`${theme} 主題已存檔 Plan 詳細頁保留階段配色`, async ({ page }) => {
    await page.addInitScript((resolvedTheme) => {
      localStorage.setItem('releasehub.language', 'zh-TW')
      localStorage.setItem('releasehub.theme', resolvedTheme)
    }, theme)
    await mockSession(page)
    await mockResources(page)
    await mockWorkflows(page)
    await page.route('**/api/v1/deployment-plans**', (route) =>
      route.fulfill(json({ data: [savedPlanResponse()], meta: meta() })),
    )
    await page.route('**/api/v1/deployment-bindings**', (route) =>
      route.fulfill(json({ data: null, meta: meta() })),
    )

    await page.goto('/plans')
    await page.getByLabel('選擇 Project').click()
    await page.getByText('Organization A / Project A').click()

    const nodes = page
      .getByLabel('Deployment Plan 圖形編輯區')
      .locator('.react-flow__node-default')
    await expect(nodes).toHaveCount(4)
    await expect
      .poll(() =>
        nodes.evaluateAll((elements) =>
          elements.map((element) => {
            const style = getComputedStyle(element)
            return {
              background: style.backgroundColor,
              borderTop: style.borderTopColor,
            }
          }),
        ),
      )
      .toEqual(planPalette[theme].stages)
  })
}

async function addDependency(page: Page, source: string, target: string) {
  await page.getByRole('button', { name: '新增相依關係' }).click()
  const dialog = page.getByRole('dialog', {
    name: '新增 Application 相依關係',
  })
  await dialog.getByLabel('上游 Node').click()
  await page.getByText(source, { exact: true }).last().click()
  await dialog.getByLabel('下游 Node').click()
  await page.getByText(target, { exact: true }).last().click()
  await dialog.getByRole('button', { name: '新增相依關係' }).click()
  await expect(dialog).toBeHidden()
}

async function graphFrameMetrics(page: Page, toolbarName: string) {
  const toolbar = page.getByRole('toolbar', { name: toolbarName })
  const editor = toolbar.locator('..')
  const inspector = page.getByRole('complementary', { name: '設定面板' })
  await expect(toolbar).toBeVisible()
  await expect(inspector).toBeVisible()
  return editor.evaluate((element) => {
    const editorStyle = getComputedStyle(element)
    const toolbarStyle = getComputedStyle(
      element.querySelector('[role="toolbar"]')!,
    )
    const inspectorStyle = getComputedStyle(element.querySelector('aside')!)
    return {
      gridTemplateColumns: editorStyle.gridTemplateColumns,
      borderRadius: editorStyle.borderRadius,
      toolbarMinHeight: toolbarStyle.minHeight,
      toolbarPadding: toolbarStyle.padding,
      inspectorWidth: inspectorStyle.width,
      inspectorRows: inspectorStyle.gridTemplateRows,
    }
  })
}

async function selectedListItemMetrics(page: Page) {
  const item = page.locator('[role="button"][aria-current="page"]').first()
  await expect(item).toBeVisible()
  return item.evaluate((element) => {
    const style = getComputedStyle(element)
    return {
      backgroundColor: style.backgroundColor,
      borderRadius: style.borderRadius,
      paddingInline: style.paddingInline,
      cursor: style.cursor,
    }
  })
}

async function expectReadableGraph(page: Page, canvasName: string) {
  const node = page.getByLabel(canvasName).locator('.react-flow__node').first()
  await expect(node).toBeVisible()
  await expect
    .poll(async () => (await node.boundingBox())?.width ?? 0)
    .toBeGreaterThanOrEqual(100)
}

async function expectFocusedListItem(page: Page) {
  const item = page.locator('[role="button"][aria-current="page"]').first()
  await expect(item).toHaveAttribute('aria-current', 'page')
  await item.focus()
  await expect
    .poll(() => item.evaluate((element) => getComputedStyle(element).boxShadow))
    .not.toBe('none')
}

async function mockSession(page: Page) {
  await page.route('**/api/v1/auth/session', (route) =>
    route.fulfill(
      json({
        data: {
          id: '019c1230-0000-7000-8000-000000000001',
          username: 'vincent',
        },
        meta: meta(),
      }),
    ),
  )
}

async function mockResources(page: Page) {
  await page.route('**/api/v1/catalog/resource-tree', (route) =>
    route.fulfill(
      json({
        data: [
          {
            id: ids.organization,
            name: 'Organization A',
            canCreateProject: false,
            projects: [
              {
                id: ids.project,
                name: 'Project A',
                canManage: true,
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
        ],
        canCreateOrganization: false,
        meta: meta(),
      }),
    ),
  )
}

async function mockWorkflows(page: Page) {
  await page.route('**/api/v1/release-workflows', (route) =>
    route.fulfill(
      json({
        data: [
          {
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
                lockVersion: 2,
                createdAt: '2026-09-07T00:00:00Z',
                document: {
                  initialState: 'done',
                  states: [{ key: 'done', name: 'Done', type: 'Terminal' }],
                  transitions: [],
                },
              },
            ],
          },
        ],
        meta: meta(),
      }),
    ),
  )
}

function planResponse(body: Record<string, unknown>) {
  return {
    id: ids.plan,
    ownerKind: body.ownerKind,
    ownerProjectId: body.ownerProjectId,
    name: body.name,
    description: body.description,
    active: true,
    versions: [
      {
        id: ids.planVersion,
        planId: ids.plan,
        versionNumber: 1,
        lifecycle: 'Draft',
        document: body.document,
        lockVersion: 1,
        createdAt: '2026-09-07T00:00:00Z',
      },
    ],
  }
}

function savedPlanResponse() {
  const nodes = Array.from({ length: 4 }, (_, order) => ({
    key: `node-${order}`,
    applicationKey: `application-${order}`,
    order,
    successCondition: {
      syncStatuses: ['Synced'],
      healthStatuses: ['Healthy'],
      stabilizationSeconds: 0,
    },
  }))
  return {
    id: ids.plan,
    ownerKind: 'project',
    ownerProjectId: ids.project,
    name: 'Production plan',
    description: 'Production Application deployment flow.',
    active: true,
    versions: [
      {
        id: ids.planVersion,
        planId: ids.plan,
        versionNumber: 1,
        lifecycle: 'Draft',
        document: {
          nodes,
          edges: nodes.slice(1).map((node, index) => ({
            from: nodes[index].key,
            to: node.key,
            condition: 'UpstreamSucceeded',
          })),
        },
        lockVersion: 1,
        createdAt: '2026-09-15T00:00:00Z',
      },
    ],
  }
}

function json(body: unknown) {
  return { contentType: 'application/json', body: JSON.stringify(body) }
}

function meta() {
  return { requestId: 'e2e', timestamp: new Date().toISOString() }
}
