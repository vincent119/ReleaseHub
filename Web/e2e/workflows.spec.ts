import { expect, test, type Page } from '@playwright/test'

test.use({ viewport: { width: 1440, height: 900 } })

for (const theme of ['light', 'dark'] as const) {
  test(`${theme} 主題 Workflow 頁內編輯器保有清楚層級與可讀節點`, async ({
    page,
  }) => {
    await page.addInitScript((resolvedTheme) => {
      localStorage.setItem('releasehub.language', 'zh-TW')
      localStorage.setItem('releasehub.theme', resolvedTheme)
    }, theme)
    await mockSession(page)
    await page.route('**/api/v1/release-workflows', (route) =>
      route.fulfill(json({ data: [], meta: meta() })),
    )
    await mockReviewOptions(page)

    await page.goto('/workflows')
    await page.getByRole('button', { name: '建立 Workflow' }).click()

    const workspace = page.getByRole('region', {
      name: 'Workflow 編輯工作區',
    })
    const expected = workflowPalette[theme]
    await expect(
      page.getByRole('heading', { name: '建立 Release Workflow' }),
    ).toBeVisible()
    await expect(workspace).toBeVisible()
    await expect(page.locator('.ant-drawer')).toHaveCount(0)
    await expect(page.getByText('Workflow 清單', { exact: true })).toHaveCount(
      0,
    )
    await expect
      .poll(async () => (await workspace.boundingBox())?.x)
      .toBeGreaterThan(80)
    await expect
      .poll(async () => (await workspace.boundingBox())?.width)
      .toBeLessThan(1360)

    await expect(
      page.getByRole('toolbar', { name: 'Workflow 結構工具列' }),
    ).toBeVisible()
    await expect
      .poll(() =>
        page
          .getByRole('toolbar', { name: 'Workflow 結構工具列' })
          .evaluate((element) => getComputedStyle(element).backgroundColor),
      )
      .toBe(expected.elevated)
    await expect(
      page.getByRole('complementary', { name: '設定面板' }),
    ).toContainText('尚未選取項目')

    const canvasStyle = await page
      .getByLabel('Workflow 圖形編輯區')
      .evaluate((element) => {
        const style = getComputedStyle(element)
        return {
          backgroundColor: style.backgroundColor,
          backgroundImage: style.backgroundImage,
        }
      })
    expect(canvasStyle.backgroundColor).toBe(expected.canvas)
    if (theme === 'dark')
      expect(canvasStyle.backgroundImage).toContain('radial-gradient')
    else expect(canvasStyle.backgroundImage).toBe('none')

    const nodes = page.locator(
      '.react-flow__node-workflowState [data-state-type]',
    )
    await expect(nodes).toHaveCount(5)
    await expect(nodes.filter({ hasText: 'Pending Review' })).toBeVisible()
    await expect(nodes.filter({ hasText: 'Deploying' })).toBeVisible()
    await expect(nodes.filter({ hasText: 'Succeeded' })).toBeVisible()
    await expect(nodes.filter({ hasText: 'Failed' })).toBeVisible()

    for (const [type, accent] of Object.entries(expected.accents)) {
      const state = page.locator(
        `.react-flow__node-workflowState [data-state-type='${type}']`,
      )
      await expect(state.first()).toBeVisible()
      const colors = await state.first().evaluate((element) => {
        const typeLabel = element.querySelector<HTMLElement>('span')!
        const style = getComputedStyle(element)
        return {
          background: style.backgroundColor,
          borderTop: style.borderTopColor,
          type: getComputedStyle(typeLabel).color,
        }
      })
      expect(colors.background).toBe(
        expected.stateSurfaces[type as keyof typeof expected.stateSurfaces],
      )
      expect(colors.borderTop).toBe(accent)
      expect(colors.type).toBe(accent)
    }

    for (const [name, outcome] of Object.entries(expected.terminalOutcomes)) {
      const state = nodes.filter({ hasText: name })
      await expect(state).toHaveAttribute('data-terminal-outcome', outcome.key)
      const colors = await state.evaluate((element) => {
        const typeLabel = element.querySelector<HTMLElement>('span')!
        const style = getComputedStyle(element)
        return {
          background: style.backgroundColor,
          borderTop: style.borderTopColor,
          type: getComputedStyle(typeLabel).color,
        }
      })
      expect(colors).toEqual({
        background: outcome.surface,
        borderTop: outcome.accent,
        type: outcome.accent,
      })
    }

    await nodes.filter({ hasText: 'Pending Review' }).click()
    await expect(page.getByText('狀態設定', { exact: true })).toBeVisible()
    await expect(
      page.getByRole('complementary', { name: '設定面板' }),
    ).toContainText('Pending Review')
    await expect
      .poll(() =>
        nodes
          .filter({ hasText: 'Pending Review' })
          .evaluate((element) => getComputedStyle(element).borderColor),
      )
      .toContain(expected.selected)
  })
}

const workflowPalette = {
  light: {
    canvas: 'rgb(246, 248, 251)',
    elevated: 'rgb(255, 255, 255)',
    selected: 'rgb(79, 70, 229)',
    accents: {
      Review: 'rgb(138, 90, 0)',
      ManualAction: 'rgb(98, 53, 213)',
      Deployment: 'rgb(20, 95, 215)',
    },
    stateSurfaces: {
      Review: 'rgb(255, 240, 194)',
      ManualAction: 'rgb(234, 220, 255)',
      Deployment: 'rgb(220, 234, 255)',
    },
    terminalOutcomes: {
      Succeeded: {
        key: 'success',
        accent: 'rgb(11, 116, 81)',
        surface: 'rgb(217, 244, 230)',
      },
      Failed: {
        key: 'failure',
        accent: 'rgb(180, 35, 24)',
        surface: 'rgb(251, 218, 218)',
      },
    },
  },
  dark: {
    canvas: 'rgb(9, 11, 18)',
    elevated: 'rgb(17, 24, 39)',
    selected: 'rgb(165, 180, 252)',
    accents: {
      Review: 'rgb(251, 191, 36)',
      ManualAction: 'rgb(196, 181, 253)',
      Deployment: 'rgb(96, 165, 250)',
    },
    stateSurfaces: {
      Review: 'rgb(58, 45, 18)',
      ManualAction: 'rgb(46, 36, 80)',
      Deployment: 'rgb(21, 46, 85)',
    },
    terminalOutcomes: {
      Succeeded: {
        key: 'success',
        accent: 'rgb(52, 211, 153)',
        surface: 'rgb(18, 59, 50)',
      },
      Failed: {
        key: 'failure',
        accent: 'rgb(251, 113, 133)',
        surface: 'rgb(72, 30, 43)',
      },
    },
  },
} as const

for (const theme of ['light', 'dark'] as const) {
  test(`${theme} 主題已存檔 Workflow 詳細頁保留狀態配色`, async ({ page }) => {
    await page.addInitScript((resolvedTheme) => {
      localStorage.setItem('releasehub.language', 'zh-TW')
      localStorage.setItem('releasehub.theme', resolvedTheme)
    }, theme)
    await mockSession(page)
    await page.route('**/api/v1/release-workflows', (route) =>
      route.fulfill(json({ data: [savedWorkflowResponse()], meta: meta() })),
    )

    await page.goto('/workflows')

    const nodes = page.locator(
      '.react-flow__node-workflowState [data-state-type]',
    )
    const expected = workflowPalette[theme]
    await expect(nodes).toHaveCount(5)

    for (const [type, accent] of Object.entries(expected.accents)) {
      const state = nodes.filter({
        hasText:
          type === 'Review'
            ? 'Pending Review'
            : type === 'ManualAction'
              ? 'Approved'
              : 'Deploying',
      })
      const style = await state.evaluate((element) => {
        const computed = getComputedStyle(element)
        return {
          background: computed.backgroundColor,
          borderTop: computed.borderTopColor,
        }
      })
      expect(style).toEqual({
        background:
          expected.stateSurfaces[type as keyof typeof expected.stateSurfaces],
        borderTop: accent,
      })
    }

    for (const [name, outcome] of Object.entries(expected.terminalOutcomes)) {
      const style = await nodes
        .filter({ hasText: name })
        .evaluate((element) => {
          const computed = getComputedStyle(element)
          return {
            background: computed.backgroundColor,
            borderTop: computed.borderTopColor,
          }
        })
      expect(style).toEqual({
        background: outcome.surface,
        borderTop: outcome.accent,
      })
    }
  })
}

for (const width of [720, 320]) {
  test(`${width}px Workflow 編輯器將設定面板置於畫布下方且無水平溢出`, async ({
    page,
  }) => {
    await page.setViewportSize({ width, height: 900 })
    await page.addInitScript(() => {
      localStorage.setItem('releasehub.language', 'zh-TW')
      localStorage.setItem('releasehub.theme', 'dark')
    })
    await mockSession(page)
    await page.route('**/api/v1/release-workflows', (route) =>
      route.fulfill(json({ data: [], meta: meta() })),
    )
    await mockReviewOptions(page)

    await page.goto('/workflows')
    await page.getByRole('button', { name: '建立 Workflow' }).click()

    const canvas = page.getByLabel('Workflow 圖形編輯區')
    const inspector = page.getByRole('complementary', { name: '設定面板' })
    await expect(canvas).toBeVisible()
    await expect(inspector).toBeVisible()
    await expect
      .poll(async () => (await inspector.boundingBox())?.y ?? 0)
      .toBeGreaterThan((await canvas.boundingBox())?.y ?? 0)
    await expect(page.locator('.react-flow__minimap')).toBeHidden()
    await expect
      .poll(() =>
        page.evaluate(
          () => document.documentElement.scrollWidth <= window.innerWidth,
        ),
      )
      .toBe(true)
  })
}

async function mockSession(page: Page) {
  await page.route('**/api/v1/auth/session', (route) =>
    route.fulfill(
      json({
        data: {
          userId: '019c1230-0000-7000-8000-000000000001',
          username: 'vincent',
          mustChangePassword: false,
          passwordChangeAvailable: true,
        },
        meta: meta(),
      }),
    ),
  )
}

async function mockReviewOptions(page: Page) {
  await page.route('**/api/v1/release-workflows/review-options', (route) =>
    route.fulfill(json({ data: { users: [], roles: [] }, meta: meta() })),
  )
}

function savedWorkflowResponse() {
  return {
    id: 'saved-workflow',
    name: 'Production approval',
    description: 'Production workflow',
    active: true,
    versions: [
      {
        id: 'saved-workflow-version',
        workflowId: 'saved-workflow',
        versionNumber: 1,
        lifecycle: 'Published',
        lockVersion: 1,
        createdAt: '2026-09-15T00:00:00Z',
        document: {
          initialState: 'pending_review',
          states: [
            { key: 'pending_review', name: 'Pending Review', type: 'Review' },
            { key: 'approved', name: 'Approved', type: 'ManualAction' },
            { key: 'deploying', name: 'Deploying', type: 'Deployment' },
            { key: 'succeeded', name: 'Succeeded', type: 'Terminal' },
            { key: 'failed', name: 'Failed', type: 'Terminal' },
          ],
          transitions: [
            {
              key: 'succeed',
              from: 'deploying',
              to: 'succeeded',
              trigger: 'DeploymentResult',
              permission: 'deployment_request.update',
              conditions: [
                {
                  fact: 'deployment.status',
                  operator: 'Equals',
                  value: 'Succeeded',
                },
              ],
            },
            {
              key: 'fail',
              from: 'deploying',
              to: 'failed',
              trigger: 'DeploymentResult',
              permission: 'deployment_request.update',
              conditions: [
                {
                  fact: 'deployment.status',
                  operator: 'In',
                  value: ['Failed', 'PartialFailed'],
                },
              ],
            },
          ],
        },
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
