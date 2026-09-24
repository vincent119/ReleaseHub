import { expect, test, type Page } from '@playwright/test'

const applicationID = '019c1230-0000-7000-8000-000000000010'

for (const theme of ['light', 'dark'] as const) {
  for (const width of [1440, 900]) {
    test(`${theme} theme renders readable runtime topology at ${width}px`, async ({
      page,
    }) => {
      await page.setViewportSize({ width, height: 900 })
      await preparePage(page, theme)
      await page.goto(`/applications/${applicationID}`)
      await page.getByRole('tab', { name: '資源拓撲' }).click()

      const canvas = page.getByLabel('Application 即時資源拓撲')
      await expect(canvas.locator('.react-flow__node-application')).toHaveCount(
        1,
      )
      await expect(canvas.locator('.react-flow__node-runtime')).toHaveCount(4)
      await expect(canvas.locator('.runtime-presentation-edge')).toHaveCount(2)
      await expect(
        canvas.getByText('payment-api', { exact: true }),
      ).toBeVisible()
      const applicationNode = canvas.locator('.react-flow__node-application')
      await expect(applicationNode.locator('button')).toHaveCount(0)
      await expect(applicationNode).toHaveCSS('pointer-events', 'none')
      await expect(applicationNode).not.toHaveClass(/selected/)
      await expect(page.getByRole('dialog')).toHaveCount(0)

      const expectedSurface =
        theme === 'light' ? 'rgb(255, 255, 255)' : 'rgb(15, 23, 42)'
      const expectedText =
        theme === 'light' ? 'rgb(23, 32, 51)' : 'rgb(248, 250, 252)'
      await expect(
        canvas.locator('.react-flow__controls-button').first(),
      ).toHaveCSS('background-color', expectedSurface)
      await expect(
        canvas.locator('.react-flow__controls-button').first(),
      ).toHaveCSS('color', expectedText)
      await expect(canvas.locator('.react-flow__minimap')).toHaveCSS(
        'background-color',
        expectedSurface,
      )
      await expect(
        canvas.locator('.react-flow__node-runtime').first().locator('button'),
      ).toHaveCSS('color', expectedText)
      await expect(
        canvas.locator('.react-flow__node-runtime').first().locator('button'),
      ).toHaveCSS('animation-name', /runtimeAppear/)
      const kindChip = canvas.getByText('Deployment', { exact: true })
      await expect(kindChip).toHaveCSS(
        'background-color',
        theme === 'light' ? 'rgb(234, 242, 255)' : 'rgb(23, 32, 51)',
      )
      await expect(kindChip).toHaveCSS(
        'color',
        theme === 'light' ? 'rgb(82, 96, 116)' : 'rgb(203, 213, 225)',
      )
      const textContrast = await kindChip.evaluate((element) => {
        const style = getComputedStyle(element)
        const channels = (value: string) =>
          (value.match(/[\d.]+/g) ?? []).slice(0, 3).map(Number)
        const luminance = (value: string) => {
          const [red, green, blue] = channels(value).map((channel) => {
            const normalized = channel / 255
            return normalized <= 0.04045
              ? normalized / 12.92
              : ((normalized + 0.055) / 1.055) ** 2.4
          })
          return 0.2126 * red + 0.7152 * green + 0.0722 * blue
        }
        const foreground = luminance(style.color)
        const background = luminance(style.backgroundColor)
        return (
          (Math.max(foreground, background) + 0.05) /
          (Math.min(foreground, background) + 0.05)
        )
      })
      expect(textContrast).toBeGreaterThanOrEqual(4.5)

      const zoom = await canvas
        .locator('.react-flow__viewport')
        .evaluate((element) => {
          return new DOMMatrixReadOnly(getComputedStyle(element).transform).a
        })
      expect(zoom).toBeGreaterThanOrEqual(0.74)

      await canvas
        .locator('.react-flow__node-runtime')
        .first()
        .locator('button')
        .focus()
      await page.keyboard.press('Shift+Tab')
      await page.keyboard.press('Tab')
      await expect(
        canvas.locator('.react-flow__node-runtime').first().locator('button'),
      ).toHaveCSS('outline-style', 'solid')
      const focusContrast = await canvas
        .locator('.react-flow__node-runtime')
        .first()
        .locator('button')
        .evaluate((element) => {
          const style = getComputedStyle(element)
          const luminance = (value: string) => {
            const [red, green, blue] = (value.match(/[\d.]+/g) ?? [])
              .slice(0, 3)
              .map(Number)
              .map((channel) => {
                const normalized = channel / 255
                return normalized <= 0.04045
                  ? normalized / 12.92
                  : ((normalized + 0.055) / 1.055) ** 2.4
              })
            return 0.2126 * red + 0.7152 * green + 0.0722 * blue
          }
          const outline = luminance(style.outlineColor)
          const surface = luminance(style.backgroundColor)
          return (
            (Math.max(outline, surface) + 0.05) /
            (Math.min(outline, surface) + 0.05)
          )
        })
      expect(focusContrast).toBeGreaterThanOrEqual(3)
      expect(
        await page.evaluate(
          () => document.documentElement.scrollWidth <= window.innerWidth,
        ),
      ).toBe(true)
    })
  }
}

test('reduced motion keeps the topology legible without node animation', async ({
  page,
}) => {
  await page.emulateMedia({ reducedMotion: 'reduce' })
  await preparePage(page, 'dark')
  await page.goto(`/applications/${applicationID}`)
  await page.getByRole('tab', { name: '資源拓撲' }).click()
  const node = page
    .getByLabel('Application 即時資源拓撲')
    .locator('.react-flow__node-runtime')
    .first()
    .locator('button')
  await expect(node).toBeVisible()
  await expect(node).toHaveCSS('animation-name', 'none')
  const edge = page
    .getByLabel('Application 即時資源拓撲')
    .locator('.react-flow__edge')
    .first()
  await edge.evaluate((element) => element.classList.add('animated'))
  await expect(edge.locator('path').first()).toHaveCSS('animation-name', 'none')
})

test('refresh preserves the user zoom level', async ({ page }) => {
  await preparePage(page, 'dark')
  await page.goto(`/applications/${applicationID}`)
  await page.getByRole('tab', { name: '資源拓撲' }).click()
  const canvas = page.getByLabel('Application 即時資源拓撲')
  await expect(canvas.locator('.react-flow__node-runtime')).toHaveCount(4)
  await canvas.locator('.react-flow__controls-zoomin').click()
  const viewport = canvas.locator('.react-flow__viewport')
  const before = await viewport.evaluate(
    (element) => new DOMMatrixReadOnly(getComputedStyle(element).transform).a,
  )

  const refreshed = page.waitForResponse((response) =>
    response
      .url()
      .includes(`/catalog/applications/${applicationID}/runtime/topology`),
  )
  await page.getByRole('button', { name: '重新整理' }).click()
  await refreshed
  const after = await viewport.evaluate(
    (element) => new DOMMatrixReadOnly(getComputedStyle(element).transform).a,
  )
  expect(after).toBeCloseTo(before, 2)
})

test('resource drawer opens by keyboard and restores node focus and viewport on Escape', async ({
  page,
}) => {
  await preparePage(page, 'dark')
  await page.goto(`/applications/${applicationID}`)
  await page.getByRole('tab', { name: '資源拓撲' }).click()
  const canvas = page.getByLabel('Application 即時資源拓撲')
  const node = canvas.getByRole('button', { name: /pod/ }).first()
  await node.focus()
  const viewport = canvas.locator('.react-flow__viewport')
  const before = await viewport.getAttribute('style')

  await page.keyboard.press('Enter')
  await expect(page.getByRole('dialog')).toBeVisible()
  await expect(page.getByRole('tab', { name: '摘要' })).toBeVisible()
  await page.keyboard.press('Escape')
  await expect(page.getByRole('dialog')).toHaveCount(0)
  await expect(node).toBeFocused()
  await expect(viewport).toHaveAttribute('style', before ?? '')

  await page.keyboard.press('Space')
  await expect(page.getByRole('dialog')).toBeVisible()
})

test('drawer tab failure stays local and non-Pod nodes never request logs', async ({
  page,
}) => {
  const requests = { events: 0, logs: 0, detail: 0 }
  await preparePage(page, 'dark')
  await page.route(/\/runtime\/resources\/events(?:\?|$)/, (route) => {
    requests.events += 1
    return route.fulfill({ ...json({ error: {} }), status: 503 })
  })
  await page.route(/\/runtime\/resources\/detail(?:\?|$)/, (route) => {
    requests.detail += 1
    return route.fulfill(
      json({ data: { manifest: 'kind: Pod', resource: {} }, meta: meta() }),
    )
  })
  await page.route(/\/runtime\/pods\/logs(?:\?|$)/, (route) => {
    requests.logs += 1
    return route.fulfill(json({ data: [], meta: meta() }))
  })
  await page.goto(`/applications/${applicationID}`)
  await page.getByRole('tab', { name: '資源拓撲' }).click()
  const canvas = page.getByLabel('Application 即時資源拓撲')
  await canvas.getByRole('button', { name: /pod/ }).first().click()
  const drawer = page.getByRole('dialog')
  await expect(drawer.getByText('Healthy')).toBeVisible()
  expect(requests).toEqual({ events: 0, logs: 0, detail: 0 })

  await drawer.getByRole('tab', { name: '事件' }).click()
  await expect.poll(() => requests.events).toBe(1)
  await expect(drawer.getByRole('alert')).toContainText(
    '無法取得 Application 即時資源。',
  )
  expect(requests.events).toBe(1)
  await drawer.getByRole('tab', { name: '摘要' }).click()
  await expect(drawer.getByText('Healthy')).toBeVisible()
  await drawer.getByRole('tab', { name: '即時 Manifest' }).click()
  await expect(drawer.getByText('kind: Pod')).toBeVisible()
  expect(requests.detail).toBe(1)
  expect(requests.logs).toBe(0)

  await page.keyboard.press('Escape')
  await canvas
    .getByRole('button', { name: /service/ })
    .first()
    .click()
  await expect(drawer.getByRole('tab', { name: '摘要' })).toHaveAttribute(
    'aria-selected',
    'true',
  )
  await expect(drawer.getByRole('tab', { name: '日誌' })).toHaveAttribute(
    'aria-disabled',
    'true',
  )
  expect(requests.logs).toBe(0)
})

async function preparePage(page: Page, theme: 'light' | 'dark') {
  await page.addInitScript((resolvedTheme) => {
    localStorage.setItem('releasehub.language', 'zh-TW')
    localStorage.setItem('releasehub.theme', resolvedTheme)
  }, theme)
  await page.route('**/api/v1/auth/session', (route) =>
    route.fulfill(
      json({ data: { id: 'user-1', username: 'vincent' }, meta: meta() }),
    ),
  )
  await page.route('**/api/v1/notifications**', (route) =>
    route.fulfill(json({ data: [], meta: meta() })),
  )
  await page.route(`**/api/v1/catalog/applications/${applicationID}`, (route) =>
    route.fulfill(json({ data: application(), meta: meta() })),
  )
  await page.route(
    `**/api/v1/catalog/applications/${applicationID}/status`,
    (route) => route.fulfill(json({ data: applicationStatus(), meta: meta() })),
  )
  await page.route(
    `**/api/v1/catalog/applications/${applicationID}/runtime/topology?**`,
    (route) => route.fulfill(json({ data: topology(), meta: meta() })),
  )
}

function application() {
  return {
    id: applicationID,
    organizationId: 'organization-1',
    projectId: 'project-1',
    environmentId: 'environment-1',
    name: 'payment-api',
    argocdNamespace: 'argocd',
    argocdApplicationName: 'payment-api-prod',
    argocdProject: 'payment',
    destinationServer: 'https://kubernetes.default.svc',
    destinationName: '',
    destinationNamespace: 'payment',
    sourceRepositoryUrl: 'https://example.invalid/gitops.git',
    sourceTargetRevision: 'main',
    sourcePath: 'production/payment-api',
    active: true,
    version: 1,
  }
}

function applicationStatus() {
  return {
    applicationId: applicationID,
    syncStatus: 'Synced',
    healthStatus: 'Healthy',
    operationPhase: 'Succeeded',
    resolvedRevision: 'abc123',
    automatedSync: false,
    candidatePresent: false,
    onboardingStatus: 'Managed',
    onboardingVersion: 1,
    driftReasons: [],
    observedAt: new Date().toISOString(),
  }
}

function topology() {
  const resource = (id: string, kind: string) => ({
    id,
    group: 'apps',
    version: 'v1',
    kind,
    namespace: 'payment',
    name: id,
    healthStatus: 'Healthy',
    healthMessage: '',
    orphaned: false,
    images: [],
    info: [],
    ingress: [],
    externalUrls: [],
  })
  return {
    applicationId: applicationID,
    view: 'resources',
    observedAt: new Date().toISOString(),
    nodes: [
      resource('deployment', 'Deployment'),
      resource('replica-set', 'ReplicaSet'),
      resource('pod', 'Pod'),
      resource('service', 'Service'),
    ],
    edges: [
      {
        id: 'resource:deployment:replica-set',
        source: 'deployment',
        target: 'replica-set',
        kind: 'resource',
      },
      {
        id: 'resource:replica-set:pod',
        source: 'replica-set',
        target: 'pod',
        kind: 'resource',
      },
    ],
    warnings: [],
    partial: false,
  }
}

function meta() {
  return { requestId: 'e2e', timestamp: new Date().toISOString() }
}

function json(value: unknown) {
  return {
    status: 200,
    contentType: 'application/json',
    body: JSON.stringify(value),
  }
}
