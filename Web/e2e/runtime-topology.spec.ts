import { expect, test, type Page } from '@playwright/test'

const applicationID = '019c1230-0000-7000-8000-000000000010'

for (const warning of [
  { code: 'node_limit', text: '資源超過 500 個節點，畫面僅顯示有界結果。' },
  { code: 'edge_limit', text: '關係超過 1,000 條，畫面僅顯示有界結果。' },
  {
    code: 'network_evidence_unresolved',
    text: '部分 Argo CD 網路關係證據無法安全解析。',
  },
]) {
  test(`Application 保留非空 ${warning.code} 子圖並可重新整理恢復`, async ({
    page,
  }) => {
    await preparePage(page, 'dark')
    let recovered = false
    await page.route(
      `**/api/v1/catalog/applications/${applicationID}/runtime/topology?**`,
      (route) => {
        const value = topology()
        return route.fulfill(
          json({
            data: {
              ...value,
              nodes: recovered ? value.nodes : value.nodes.slice(0, 3),
              observedAt: recovered
                ? '2026-01-01T01:00:00Z'
                : '2026-01-01T00:00:00Z',
              view:
                warning.code === 'network_evidence_unresolved'
                  ? 'network'
                  : 'resources',
              edges: value.edges.map((edge) => ({
                ...edge,
                kind:
                  warning.code === 'network_evidence_unresolved'
                    ? 'network'
                    : 'resource',
              })),
              warnings: recovered ? [] : [warning.code],
              partial: !recovered,
            },
            meta: meta(),
          }),
        )
      },
    )
    await page.goto(`/applications/${applicationID}`)
    await page.getByRole('tab', { name: '資源拓撲' }).click()
    if (warning.code === 'network_evidence_unresolved')
      await page.getByText('網路拓撲', { exact: true }).click()
    await expect(page.getByRole('alert')).toContainText(warning.text)
    const canvas = page.getByLabel('Application 即時資源拓撲')
    await expect(canvas.locator('.react-flow__node-runtime')).toHaveCount(3)
    await expect(
      canvas.locator('.react-flow__edge:not(.runtime-presentation-edge)'),
    ).toHaveCount(2)
    await expect(
      page.getByRole('heading', { name: 'payment-api', exact: true }),
    ).toBeVisible()
    const summary = page
      .locator('[aria-live="polite"]')
      .filter({ hasText: '觀測時間' })
    await expect(summary).toContainText('此視圖 3／回傳 3 個資源')
    const before = await summary.textContent()
    recovered = true
    await page.getByRole('button', { name: '重新整理', exact: true }).click()
    await expect(page.getByRole('alert')).toHaveCount(0)
    await expect(canvas.locator('.react-flow__node-runtime')).toHaveCount(
      warning.code === 'network_evidence_unresolved' ? 3 : 4,
    )
    await expect(summary).toContainText(
      warning.code === 'network_evidence_unresolved'
        ? '此視圖 3／回傳 4 個資源'
        : '此視圖 4／回傳 4 個資源',
    )
    await expect(summary).not.toHaveText(before ?? '')
    await page.getByRole('tab', { name: '狀態總覽', exact: true }).click()
    await expect(page.getByText('Synced', { exact: true })).toBeVisible()
    await expect(page.getByText('Succeeded', { exact: true })).toBeVisible()
  })
}

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
      expect(zoom).toBeGreaterThan(0)
      expect(zoom).toBeLessThanOrEqual(1)
      await expectGraphWithinCanvas(canvas)

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

test('network view shows only evidenced endpoints and keeps the full resource view', async ({
  page,
}) => {
  const ids = [
    'status-webhooks-ingress',
    'status-webhooks-service',
    'status-webhooks-deploy-b9cc74fc5-54kq9',
    'status-webhooks-deploy-b9cc74fc5-64kq9',
  ]
  await page.setViewportSize({ width: 1440, height: 900 })
  await preparePage(page, 'light')
  await page.route(
    `**/api/v1/catalog/applications/${applicationID}/runtime/topology?**`,
    (route) => {
      const base = topology()
      const resource = base.nodes[0]
      const nodes = [
        ...ids.map((id, index) => ({
          ...resource,
          id,
          name: id,
          kind: index === 0 ? 'Ingress' : index === 1 ? 'Service' : 'Pod',
        })),
        ...Array.from({ length: 11 }, (_, index) => ({
          ...resource,
          id: `unrelated-${index}`,
          name: `unrelated-${index}`,
        })),
      ]
      const view = new URL(route.request().url()).searchParams.get('view')
      return route.fulfill(
        json({
          data: {
            ...base,
            view,
            nodes,
            edges:
              view === 'network'
                ? [
                    {
                      id: 'entry',
                      source: ids[0],
                      target: ids[1],
                      kind: 'network',
                    },
                    {
                      id: 'pod-1',
                      source: ids[1],
                      target: ids[2],
                      kind: 'network',
                    },
                    {
                      id: 'pod-2',
                      source: ids[1],
                      target: ids[3],
                      kind: 'network',
                    },
                  ]
                : [],
          },
          meta: meta(),
        }),
      )
    },
  )
  await page.goto(`/applications/${applicationID}`)
  await page.getByRole('tab', { name: '資源拓撲' }).click()
  const canvas = page.getByLabel('Application 即時資源拓撲')
  await expect(canvas.locator('.react-flow__node-runtime')).toHaveCount(15)
  await expectGraphWithinCanvas(canvas)
  await page.getByText('網路拓撲', { exact: true }).click()
  await expect(canvas.locator('.react-flow__node-runtime')).toHaveCount(4)
  await expect(canvas.locator('.react-flow__edge')).toHaveCount(3)
  await expect(
    page.locator('[aria-live="polite"]').filter({ hasText: '觀測時間' }),
  ).toContainText('此視圖 4／回傳 15 個資源')
  await expectGraphWithinCanvas(canvas)
  const firstPod = canvas.getByRole('button', { name: `Pod ${ids[2]}` })
  const secondPod = canvas.getByRole('button', { name: `Pod ${ids[3]}` })
  await expect(firstPod).toContainText('54kq9')
  await expect(secondPod).toContainText('64kq9')
  expect(
    await firstPod
      .locator('[class*="nodeName"]')
      .evaluate((element) => element.scrollWidth <= element.clientWidth),
  ).toBe(true)
  await expect(firstPod).toHaveAttribute('data-runtime-node-id', ids[2])
  await firstPod.click()
  await expect(page.getByRole('dialog')).toContainText(ids[2])
  await page.getByRole('dialog').getByRole('button', { name: '關閉' }).click()
  await page.getByText('資源階層', { exact: true }).click()
  await expect(canvas.locator('.react-flow__node-runtime')).toHaveCount(15)
})

test('large overview fits all nodes, and reading zoom can be restored after resize', async ({
  page,
}) => {
  await page.setViewportSize({ width: 900, height: 700 })
  await preparePage(page, 'dark')
  await page.route(
    `**/api/v1/catalog/applications/${applicationID}/runtime/topology?**`,
    (route) => {
      const base = topology()
      const resource = base.nodes[0]
      return route.fulfill(
        json({
          data: {
            ...base,
            nodes: Array.from({ length: 100 }, (_, index) => ({
              ...resource,
              id: `resource-${index}`,
              name: `resource-${index}`,
            })),
            edges: [],
          },
          meta: meta(),
        }),
      )
    },
  )
  await page.goto(`/applications/${applicationID}`)
  await page.getByRole('tab', { name: '資源拓撲' }).click()
  const canvas = page.getByLabel('Application 即時資源拓撲')
  await expect(canvas.locator('.react-flow__node-runtime')).toHaveCount(100)
  await expectGraphWithinCanvas(canvas)
  await page.getByRole('button', { name: '閱讀比例' }).click()
  await expect
    .poll(() =>
      canvas
        .locator('.react-flow__viewport')
        .evaluate(
          (element) =>
            new DOMMatrixReadOnly(getComputedStyle(element).transform).a,
        ),
    )
    .toBeCloseTo(1, 2)
  await page.setViewportSize({ width: 320, height: 568 })
  await page.getByRole('button', { name: '全圖總覽' }).click()
  await expectGraphWithinCanvas(canvas)
})

async function expectGraphWithinCanvas(canvas: ReturnType<Page['getByLabel']>) {
  await expect
    .poll(() =>
      canvas.evaluate((element) => {
        const bounds = element.getBoundingClientRect()
        const nodes = [...element.querySelectorAll('.react-flow__node')]
        return (
          nodes.length > 0 &&
          nodes.every((node) => {
            const rect = node.getBoundingClientRect()
            return (
              rect.left >= bounds.left + 4 &&
              rect.right <= bounds.right - 4 &&
              rect.top >= bounds.top + 4 &&
              rect.bottom <= bounds.bottom - 4
            )
          })
        )
      }),
    )
    .toBe(true)
}

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

test('resource dialog opens by keyboard and restores node focus and viewport on Escape', async ({
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
  const dialog = page.getByRole('dialog')
  await dialog.getByRole('button', { name: '關閉', exact: true }).focus()
  for (const key of ['Tab', 'Shift+Tab']) {
    for (let index = 0; index < 12; index += 1) {
      await page.keyboard.press(key)
      expect(
        await dialog.evaluate((element) =>
          element.contains(document.activeElement),
        ),
      ).toBe(true)
    }
  }
  await page.keyboard.press('Escape')
  await expect(page.getByRole('dialog')).toHaveCount(0)
  await expect(node).toBeFocused()
  await expect(viewport).toHaveAttribute('style', before ?? '')

  await page.keyboard.press('Space')
  await expect(page.getByRole('dialog')).toBeVisible()
})

test('resource dialog restores safe focus when the selected resource disappears', async ({
  page,
}) => {
  await preparePage(page, 'dark')
  let releaseRefresh: (() => void) | undefined
  const refreshed = new Promise<void>((resolve) => {
    releaseRefresh = resolve
  })
  let calls = 0
  let blockRefresh = false
  await page.route(
    `**/api/v1/catalog/applications/${applicationID}/runtime/topology?**`,
    async (route) => {
      const data = topology()
      calls += 1
      if (blockRefresh) {
        await refreshed
        data.nodes = data.nodes.filter((node) => node.id !== 'pod')
        data.edges = data.edges.filter((edge) => edge.target !== 'pod')
      }
      await route.fulfill(json({ data, meta: meta() }))
    },
  )
  await page.goto(`/applications/${applicationID}`)
  await page.getByRole('tab', { name: '資源拓撲' }).click()
  const refresh = page.locator('[data-runtime-refresh]')
  const node = page.locator('[data-runtime-node-id="pod"]')
  await expect(node).toBeVisible()
  const initialCalls = calls
  blockRefresh = true
  await refresh.click()
  await expect.poll(() => calls).toBe(initialCalls + 1)
  try {
    await node.click()
    await expect(page.getByRole('dialog')).toBeVisible()
  } finally {
    releaseRefresh?.()
  }
  await expect(page.getByRole('dialog')).toHaveCount(0)
  await expect(refresh).toBeFocused()
  await expect(page.locator('body')).not.toHaveCSS('overflow-y', 'hidden')
})

for (const theme of ['light', 'dark'] as const) {
  for (const viewport of [
    { width: 1440, height: 900 },
    { width: 900, height: 600 },
    { width: 390, height: 700 },
    { width: 740, height: 360 },
  ]) {
    test(`${theme} resource dialog uses the available space at ${viewport.width}x${viewport.height}`, async ({
      page,
    }) => {
      await page.setViewportSize(viewport)
      await page.emulateMedia({ reducedMotion: 'reduce' })
      await preparePage(page, theme)
      await page.route(/\/runtime\/resources\/detail(?:\?|$)/, (route) =>
        route.fulfill(
          json({
            data: {
              manifest: Array.from(
                { length: 100 },
                (_, i) => `field${i}: ${'value'.repeat(100)}`,
              ).join('\n'),
              resource: {},
            },
            meta: meta(),
          }),
        ),
      )
      await page.goto(`/applications/${applicationID}`)
      await page.getByRole('tab', { name: '資源拓撲' }).click()
      const canvas = page.getByLabel('Application 即時資源拓撲')
      const node = canvas.getByRole('button', { name: /pod/ }).first()
      await node.focus()
      await page.keyboard.press('Enter')
      const dialog = page.getByRole('dialog', { name: 'pod', exact: true })
      await expect(dialog).toBeVisible()
      const full = viewport.width <= 768
      await expect
        .poll(async () => Math.round((await dialog.boundingBox())?.width ?? 0))
        .toBe(Math.round(viewport.width * (full ? 1 : 0.9)))
      await expect
        .poll(async () => Math.round((await dialog.boundingBox())?.height ?? 0))
        .toBe(Math.round(viewport.height * (full ? 1 : 0.9)))
      const bounds = (await dialog.boundingBox())!
      expect(bounds.x).toBeCloseTo(full ? 0 : viewport.width * 0.05, 0)
      expect(bounds.y).toBeCloseTo(full ? 0 : viewport.height * 0.05, 0)
      await expect(page.locator('body')).toHaveCSS('overflow-y', 'hidden')
      await dialog.getByRole('tab', { name: '即時 Manifest' }).click()
      const pane = dialog.getByRole('tabpanel')
      await expect(pane).toContainText('field99:')
      const tabsBefore = await dialog.getByRole('tablist').boundingBox()
      await pane.evaluate((element) => {
        element.scrollTop = element.scrollHeight
      })
      expect(
        await pane.evaluate((element) => element.scrollTop),
      ).toBeGreaterThan(0)
      expect(await dialog.getByRole('tablist').boundingBox()).toEqual(
        tabsBefore,
      )
      expect(
        await dialog.evaluate(
          (element) => element.scrollWidth <= element.clientWidth,
        ),
      ).toBe(true)
      await expect(dialog.locator('.ant-modal-container')).toHaveCSS(
        'background-color',
        theme === 'dark' ? 'rgb(17, 24, 39)' : 'rgb(255, 255, 255)',
      )
      await page.screenshot({
        path: `/tmp/releasehub-dialog-${theme}-${viewport.width}.png`,
      })
      await dialog.getByRole('button', { name: '關閉', exact: true }).click()
      await expect(dialog).toHaveCount(0)
      await expect(node).toBeFocused()
      await expect(page.locator('body')).not.toHaveCSS('overflow-y', 'hidden')
    })
  }
}

test('Manifest formats JSON without changing large integers or exposing hidden metadata by default', async ({
  page,
}) => {
  await preparePage(page, 'dark')
  const source =
    '{"kind":"Pod","metadata":{"managedFields":[{"manager":"synthetic-controller"}],"annotations":{"example":"keep"}},"spec":{"generation":90071992547409931234}}'
  let detailCalls = 0
  await page.route(/\/runtime\/resources\/detail(?:\?|$)/, (route) => {
    detailCalls += 1
    return route.fulfill(
      json({ data: { manifest: source, resource: {} }, meta: meta() }),
    )
  })
  await page.goto(`/applications/${applicationID}`)
  await page.getByRole('tab', { name: '資源拓撲' }).click()
  const node = page
    .getByLabel('Application 即時資源拓撲')
    .getByRole('button', { name: /pod/ })
    .first()
  await node.click()
  const dialog = page.getByRole('dialog')
  await dialog.getByRole('tab', { name: '即時 Manifest' }).click()
  const content = dialog.locator('pre')
  await expect(content).toContainText('\n  "kind": "Pod",\n')
  await expect(content).not.toContainText('synthetic-controller')
  await expect(content).toContainText('90071992547409931234')
  await dialog.getByRole('checkbox', { name: '顯示 managedFields' }).check()
  await expect(content).toContainText('synthetic-controller')
  await dialog.getByText('完整原文', { exact: true }).click()
  await expect(dialog.getByRole('radio', { name: '完整原文' })).toBeChecked()
  await expect(content).toHaveText(source)
  expect(detailCalls).toBe(1)
  await page.keyboard.press('Escape')
  await expect(node).toBeFocused()
})

for (const theme of ['light', 'dark'] as const) {
  for (const width of [900, 1440]) {
    test(`${theme} Manifest supports horizontal scrolling and keyboard wrapping at ${width}px`, async ({
      page,
    }) => {
      await page.setViewportSize({ width, height: 900 })
      await preparePage(page, theme)
      const source = JSON.stringify({
        apiVersion: 'v1',
        kind: 'Pod',
        metadata: {
          name: 'synthetic-pod',
          managedFields: [{ manager: 'synthetic-controller' }],
        },
        spec: {
          containers: [
            { name: 'api', image: `example.invalid/api:${'v'.repeat(300)}` },
          ],
        },
        status: { phase: 'Running' },
      })
      let calls = 0
      await page.route(/\/runtime\/resources\/detail(?:\?|$)/, (route) => {
        calls += 1
        return route.fulfill(
          json({ data: { manifest: source, resource: {} }, meta: meta() }),
        )
      })
      await page.goto(`/applications/${applicationID}`)
      await page.getByRole('tab', { name: '資源拓撲' }).click()
      const node = page.locator('[data-runtime-node-id="pod"]')
      await node.click()
      const dialog = page.getByRole('dialog')
      await dialog.getByRole('tab', { name: '即時 Manifest' }).click()
      const code = dialog.getByLabel('Manifest 內容', { exact: true })
      await expect(code).toContainText('"phase": "Running"')
      await expect(code).toHaveCSS('white-space', 'pre')
      await expect(code).toHaveCSS(
        'background-color',
        theme === 'dark' ? 'rgb(9, 11, 18)' : 'rgb(246, 248, 251)',
      )
      await expect(code).toHaveCSS(
        'color',
        theme === 'dark' ? 'rgb(248, 250, 252)' : 'rgb(23, 32, 51)',
      )
      await code.focus()
      await page.keyboard.press('Shift+Tab')
      await page.keyboard.press('Tab')
      await expect(code).toBeFocused()
      await expect(code).toHaveCSS('outline-style', 'solid')
      await code.press('ArrowRight')
      await expect
        .poll(() => code.evaluate((element) => element.scrollLeft))
        .toBeGreaterThan(0)
      expect(
        await dialog.evaluate(
          (element) => element.scrollWidth <= element.clientWidth,
        ),
      ).toBe(true)
      const wrap = dialog.getByRole('checkbox', { name: '自動換行' })
      await wrap.focus()
      await page.keyboard.press('Space')
      await expect(wrap).toBeChecked()
      await expect(code).toHaveCSS('white-space', 'pre-wrap')
      expect(
        await code.evaluate(
          (element) => element.scrollWidth <= element.clientWidth,
        ),
      ).toBe(true)
      expect(calls).toBe(1)
      await page.screenshot({
        path: `/tmp/releasehub-manifest-${theme}-${width}.png`,
      })
      await page.keyboard.press('Escape')
      await expect(node).toBeFocused()
    })
  }
}

test('Manifest API failure stays local and the summary remains available', async ({
  page,
}) => {
  await preparePage(page, 'dark')
  await page.route(/\/runtime\/resources\/detail(?:\?|$)/, (route) =>
    route.fulfill({ ...json({ error: {} }), status: 503 }),
  )
  await page.goto(`/applications/${applicationID}`)
  await page.getByRole('tab', { name: '資源拓撲' }).click()
  await page.locator('[data-runtime-node-id="pod"]').click()
  const dialog = page.getByRole('dialog')
  await dialog.getByRole('tab', { name: '即時 Manifest' }).click()
  await expect(dialog.getByRole('alert')).toContainText(
    '無法取得 Application 即時資源。',
  )
  await expect(dialog.getByText('目前沒有 Manifest 內容。')).toHaveCount(0)
  await dialog.getByRole('tab', { name: '摘要' }).click()
  await expect(dialog.getByText('Healthy')).toBeVisible()
})

test('Logs separates API timestamps and preserves entry boundaries while cleaning ANSI', async ({
  page,
}) => {
  await preparePage(page, 'dark')
  const entries = [
    {
      timestamp: '2026-01-02T03:04:05Z',
      content:
        '\u001b[34mINFO\u001b[0m 2026-01-02T11:04:05+08:00 {"value":1}\n\tat example.go:10',
      podName: 'pod',
    },
    { timestamp: '2026-01-01T03:04:05Z', content: '', podName: 'pod' },
  ]
  let calls = 0
  await page.route(/\/runtime\/pods\/logs(?:\?|$)/, (route) => {
    calls += 1
    return route.fulfill(json({ data: entries, meta: meta() }))
  })
  await page.goto(`/applications/${applicationID}`)
  await page.getByRole('tab', { name: '資源拓撲' }).click()
  const node = page.locator('[data-runtime-node-id="pod"]')
  await node.click()
  const dialog = page.getByRole('dialog')
  await dialog.getByRole('tab', { name: '日誌' }).click()
  await expect(dialog.locator('pre').first()).not.toContainText('\u001b[34m')
  const rows = dialog
    .getByRole('table', { name: '日誌記錄' })
    .locator('tbody tr')
  await expect(rows).toHaveCount(2)
  await expect(rows.nth(0).getByRole('cell').nth(0)).toHaveText(
    entries[0].timestamp,
  )
  await expect(rows.nth(0).locator('pre')).toHaveText(
    'INFO 2026-01-02T11:04:05+08:00 {"value":1}\n\tat example.go:10',
  )
  await expect(rows.nth(1).locator('pre')).toHaveText('')
  await dialog.getByText('原文（跳脫）', { exact: true }).click()
  await expect(rows.nth(0).locator('pre')).toHaveText(
    JSON.stringify(entries[0].content),
  )
  expect(calls).toBe(1)
  await page.keyboard.press('Escape')
  await expect(node).toBeFocused()
})

for (const theme of ['light', 'dark'] as const) {
  for (const width of [900, 1440]) {
    test(`${theme} Logs supports keyboard wrapping and time visibility at ${width}px`, async ({
      page,
    }) => {
      await page.setViewportSize({ width, height: 900 })
      await preparePage(page, theme)
      const entries = [
        {
          timestamp: '2026-01-02T03:04:05.123456Z',
          content: `\u001b[34mINFO\u001b[0m 2026-01-02T11:04:05+08:00 synthetic request ${'value'.repeat(80)}`,
          podName: 'pod',
        },
        {
          timestamp: '2026-01-02T03:04:06Z',
          content: 'example error\n\tat example.go:10\n\tat worker.go:20',
          podName: 'pod',
        },
        {
          timestamp: '2026-01-02T03:04:07Z',
          content:
            '\u001b]8;;https://example.invalid\u0007safe label\u001b]8;;\u0007 <b>plain text</b>',
          podName: 'pod',
        },
      ]
      let calls = 0
      await page.route(/\/runtime\/pods\/logs(?:\?|$)/, (route) => {
        calls += 1
        return route.fulfill(json({ data: entries, meta: meta() }))
      })
      await page.goto(`/applications/${applicationID}`)
      await page.getByRole('tab', { name: '資源拓撲' }).click()
      const node = page.locator('[data-runtime-node-id="pod"]')
      await node.click()
      const dialog = page.getByRole('dialog')
      await dialog.getByRole('tab', { name: '日誌' }).click()
      const content = dialog.getByRole('region', {
        name: '日誌內容',
        exact: true,
      })
      await expect(content.locator('tbody tr')).toHaveCount(3)
      await expect(content.locator('pre').first()).toHaveCSS(
        'white-space',
        'pre',
      )
      await expect(content).toHaveCSS(
        'background-color',
        theme === 'dark' ? 'rgb(9, 11, 18)' : 'rgb(246, 248, 251)',
      )
      await expect(content).toHaveCSS(
        'color',
        theme === 'dark' ? 'rgb(248, 250, 252)' : 'rgb(23, 32, 51)',
      )
      await content.focus()
      await page.keyboard.press('Shift+Tab')
      await page.keyboard.press('Tab')
      await expect(content).toBeFocused()
      await expect(content).toHaveCSS('outline-style', 'solid')
      await content.press('ArrowRight')
      await expect
        .poll(() => content.evaluate((element) => element.scrollLeft))
        .toBeGreaterThan(0)
      expect(
        await dialog.evaluate(
          (element) => element.scrollWidth <= element.clientWidth,
        ),
      ).toBe(true)
      const wrap = dialog.getByRole('checkbox', { name: '自動換行' })
      await wrap.focus()
      await page.keyboard.press('Space')
      await expect(content.locator('pre').first()).toHaveCSS(
        'white-space',
        'pre-wrap',
      )
      expect(
        await content.evaluate(
          (element) => element.scrollWidth <= element.clientWidth,
        ),
      ).toBe(true)
      await page.screenshot({
        path: `/tmp/releasehub-logs-${theme}-${width}.png`,
      })
      await dialog.getByRole('checkbox', { name: '顯示 API 時間' }).uncheck()
      await expect(
        content.getByRole('columnheader', { name: 'API 時間', exact: true }),
      ).toHaveCount(0)
      await expect(content.locator('pre').first()).toContainText(
        '2026-01-02T11:04:05+08:00',
      )
      await expect(content.locator('a, b')).toHaveCount(0)
      await dialog.getByText('原文（跳脫）', { exact: true }).click()
      await expect(content.locator('pre').first()).toHaveText(
        JSON.stringify(entries[0].content),
      )
      await dialog.getByRole('checkbox', { name: '顯示 API 時間' }).check()
      await expect(
        content.locator('tbody tr').first().getByRole('cell').first(),
      ).toHaveText(JSON.stringify(entries[0].timestamp))
      expect(calls).toBe(1)
      await page.keyboard.press('Escape')
      await expect(node).toBeFocused()
    })
  }
}

test('Logs API failure stays local instead of appearing as empty logs', async ({
  page,
}) => {
  await preparePage(page, 'dark')
  await page.route(/\/runtime\/pods\/logs(?:\?|$)/, (route) =>
    route.fulfill({ ...json({ error: {} }), status: 503 }),
  )
  await page.goto(`/applications/${applicationID}`)
  await page.getByRole('tab', { name: '資源拓撲' }).click()
  await page.locator('[data-runtime-node-id="pod"]').click()
  const dialog = page.getByRole('dialog')
  await dialog.getByRole('tab', { name: '日誌' }).click()
  await expect(dialog.getByRole('alert')).toContainText(
    '無法取得 Application 即時資源。',
  )
  await expect(dialog.getByText('目前沒有日誌記錄。')).toHaveCount(0)
  await dialog.getByRole('tab', { name: '摘要' }).click()
  await expect(dialog.getByText('Healthy')).toBeVisible()
})

test('resource dialog tab failure stays local and non-Pod nodes never request logs', async ({
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

  await drawer.getByRole('tab', { name: '日誌' }).click()
  await expect.poll(() => requests.logs).toBe(1)
  await expect(drawer.getByRole('tabpanel', { name: '日誌' })).toContainText(
    '目前沒有日誌記錄。',
  )

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
  expect(requests.logs).toBe(1)
})

test('runtime theme switches in place with readable status and observation', async ({
  page,
}) => {
  await preparePage(page, 'light')
  await page.goto(`/applications/${applicationID}`)
  await page.getByRole('tab', { name: '資源拓撲' }).click()
  const canvas = page.getByLabel('Application 即時資源拓撲')
  const node = canvas.locator('[data-runtime-node-id="deployment"]')
  const observation = page
    .locator('[aria-live="polite"]')
    .filter({ hasText: '觀測時間' })
  await expect(observation).toHaveCSS('color', 'rgb(82, 96, 116)')
  await expect(node.getByText('Healthy')).toHaveCSS('color', 'rgb(82, 96, 116)')
  await expect(canvas.locator('.react-flow__minimap-node')).toHaveCount(5)
  await canvas.locator('.react-flow__controls-zoomin').click()
  const viewport = canvas.locator('.react-flow__viewport')
  const before = await viewport.getAttribute('style')
  const documentHandle = await page.evaluateHandle(() => document)
  for (const mode of ['dark', 'light', 'system'] as const) {
    await page.getByRole('button', { name: '開啟 vincent 的帳號選單' }).click()
    await page.getByRole('menuitem', { name: '主題設定' }).click()
    await page.getByRole('combobox', { name: '主題', exact: true }).click()
    await page
      .getByText({ dark: '深色', light: '淺色', system: '系統' }[mode], {
        exact: true,
      })
      .last()
      .click()
    await page.keyboard.press('Escape')
    await page.mouse.click(350, 100)
    const themes = mode === 'system' ? (['dark', 'light'] as const) : [mode]
    for (const theme of themes) {
      if (mode === 'system') await page.emulateMedia({ colorScheme: theme })
      const dark = theme === 'dark'
      const secondary = dark ? 'rgb(203, 213, 225)' : 'rgb(82, 96, 116)'
      const border = dark ? 'rgb(100, 116, 139)' : 'rgb(123, 141, 165)'
      await expect(page.locator('html')).toHaveAttribute('data-theme', theme)
      await expect(observation).toHaveCSS('color', secondary)
      const observationContrast = await observation.evaluate((element) => {
        const probe = document.createElement('span')
        probe.style.color = 'var(--rh-color-page)'
        element.appendChild(probe)
        const luminance = (color: string) => {
          const [r, g, b] = (color.match(/[\d.]+/g) ?? [])
            .slice(0, 3)
            .map(Number)
            .map((value) => {
              const c = value / 255
              return c <= 0.04045 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4
            })
          return r * 0.2126 + g * 0.7152 + b * 0.0722
        }
        const foreground = luminance(getComputedStyle(element).color)
        const background = luminance(getComputedStyle(probe).color)
        probe.remove()
        return (
          (Math.max(foreground, background) + 0.05) /
          (Math.min(foreground, background) + 0.05)
        )
      })
      expect(observationContrast).toBeGreaterThanOrEqual(4.5)
      await expect(node.getByText('Healthy')).toHaveCSS('color', secondary)
      await expect(node).toHaveCSS(
        'background-color',
        dark ? 'rgb(18, 59, 49)' : 'rgb(226, 246, 236)',
      )
      await expect(node).toHaveCSS(
        'border-top-color',
        dark ? 'rgb(52, 211, 153)' : 'rgb(20, 132, 94)',
      )
      await expect(canvas.locator('.react-flow__edge-path').last()).toHaveCSS(
        'stroke',
        border,
      )
      await expect(
        canvas.locator('.react-flow__arrowhead polyline').first(),
      ).toHaveCSS('fill', border)
      await expect(
        canvas.locator('.react-flow__minimap-node').first(),
      ).toHaveCSS('stroke', dark ? 'rgb(129, 140, 248)' : 'rgb(79, 70, 229)')
      await expect(
        canvas.locator('.react-flow__minimap-node').first(),
      ).toHaveCSS(
        'fill',
        dark ? 'rgba(99, 102, 241, 0.18)' : 'rgb(238, 242, 255)',
      )
      await expect(canvas.locator('.react-flow__minimap-mask')).toHaveCSS(
        'fill',
        dark ? 'rgba(0, 0, 0, 0.68)' : 'rgba(15, 23, 42, 0.48)',
      )
      await expect(canvas.locator('.react-flow__background')).toHaveCSS(
        'background-color',
        dark ? 'rgb(9, 11, 18)' : 'rgb(246, 248, 251)',
      )
      await expect(
        canvas.locator('.react-flow__controls-button').first(),
      ).toHaveCSS('color', dark ? 'rgb(248, 250, 252)' : 'rgb(23, 32, 51)')
      await expect(viewport).toHaveAttribute('style', before ?? '')
      expect(
        await page.evaluate(
          (original) => original === document,
          documentHandle,
        ),
      ).toBe(true)
    }
  }
})

for (const theme of ['light', 'dark'] as const) {
  test(`${theme} runtime health palette follows workflow visual hierarchy`, async ({
    page,
  }) => {
    await page.setViewportSize({ width: 1440, height: 1100 })
    await preparePage(page, theme)
    const statuses = [
      'Healthy',
      'Progressing',
      'Missing',
      'Suspended',
      'Degraded',
      'Unknown',
      '',
    ]
    const tones = [
      'success',
      'info',
      'warning',
      'warning',
      'error',
      'neutral',
      'neutral',
    ]
    await page.route(
      `**/api/v1/catalog/applications/${applicationID}/runtime/topology?**`,
      (route) => {
        const data = topology()
        data.nodes = statuses.map((healthStatus, index) => ({
          ...data.nodes[0],
          id: `node-${index}`,
          name: `node-${index}`,
          healthStatus,
        }))
        data.edges = []
        return route.fulfill(json({ data, meta: meta() }))
      },
    )
    await page.goto(`/applications/${applicationID}`)
    await page.getByRole('tab', { name: '資源拓撲' }).click()
    const canvas = page.getByLabel('Application 即時資源拓撲')
    for (const [index, tone] of tones.entries()) {
      const node = canvas.locator(`[data-runtime-node-id="node-${index}"]`)
      await expect(node).toHaveAttribute(
        'data-health',
        statuses[index] || 'Unknown',
      )
      const measurements = await node.evaluate((element, tone) => {
        const style = getComputedStyle(element)
        const probe = document.createElement('span')
        element.appendChild(probe)
        const resolve = (token: string) => {
          probe.style.color = `var(${token})`
          return getComputedStyle(probe).color
        }
        const expectedSurface = resolve(
          tone === 'neutral'
            ? '--rh-color-surface'
            : `--rh-feedback-${tone}-bg`,
        )
        const expectedAccent = resolve(
          tone === 'neutral'
            ? '--rh-color-text-secondary'
            : `--rh-color-${tone}`,
        )
        const luminance = (color: string) => {
          const [r, g, b] = (color.match(/[\d.]+/g) ?? [])
            .slice(0, 3)
            .map(Number)
            .map((value) => {
              const c = value / 255
              return c <= 0.04045 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4
            })
          return r * 0.2126 + g * 0.7152 + b * 0.0722
        }
        const status = element.querySelector('.ant-typography-secondary')!
        const foreground = luminance(getComputedStyle(status).color)
        const background = luminance(style.backgroundColor)
        probe.remove()
        return {
          background: style.backgroundColor,
          accent: style.borderTopColor,
          borderWidth: style.borderTopWidth,
          expectedSurface,
          expectedAccent,
          contrast:
            (Math.max(foreground, background) + 0.05) /
            (Math.min(foreground, background) + 0.05),
        }
      }, tone)
      expect(measurements.background).toBe(measurements.expectedSurface)
      expect(measurements.accent).toBe(measurements.expectedAccent)
      expect(measurements.borderWidth).toBe('3px')
      expect(measurements.contrast).toBeGreaterThanOrEqual(4.5)
    }
    await page.screenshot({
      path: `/tmp/releasehub-runtime-${theme}.png`,
      fullPage: true,
    })
  })
}

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
