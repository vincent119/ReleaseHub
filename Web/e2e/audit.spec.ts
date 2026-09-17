import { expect, test, type Page } from '@playwright/test'

for (const theme of ['light', 'dark'] as const) {
  test(`${theme} 主題 Audit Trail 可透過鍵盤查看安全明細`, async ({ page }) => {
    await page.setViewportSize({ width: 1440, height: 900 })
    await prepareAudit(page, theme)
    await page.goto('/audit')

    await expect(page.locator('html')).toHaveAttribute('data-theme', theme)
    await expect(page.getByRole('heading', { name: '稽核紀錄' })).toBeVisible()
    await expect(page.getByRole('link', { name: '稽核紀錄' })).toBeVisible()
    await page.getByLabel('操作名稱').fill('workflow')
    await expect(
      page
        .locator('.ant-select-dropdown:visible .ant-select-item-option-content')
        .filter({ hasText: 'workflow.version.published' }),
    ).toBeVisible()
    await expect(page.getByRole('table')).toContainText(
      'workflow.version.published',
    )
    await expect(page.getByText('safe-value')).toHaveCount(0)

    const detail = page.getByRole('button', { name: '查看' })
    await detail.focus()
    await expect(detail).toBeFocused()
    await page.keyboard.press('Enter')

    const drawer = page.getByRole('dialog', { name: '稽核事件明細' })
    await expect(drawer).toBeVisible()
    await expect(drawer).toContainText('safe-value')
    await expect(drawer).toContainText('部分 Metadata 已遮罩或截斷。')
    expect(await hasHorizontalOverflow(page)).toBe(false)
  })
}

test('窄螢幕改用事件卡片且明細 Drawer 不超出 viewport', async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 })
  await prepareAudit(page, 'dark')
  await page.goto('/audit')

  await expect(page.getByRole('table')).toBeHidden()
  const eventCard = page.getByRole('button', {
    name: /workflow\.version\.published/,
  })
  await expect(eventCard).toBeVisible()
  await eventCard.click()

  const drawer = page.getByRole('dialog', { name: '稽核事件明細' })
  await expect(drawer).toBeVisible()
  await expect
    .poll(async () => (await drawer.boundingBox())?.width)
    .toBeLessThanOrEqual(390)
  expect(await hasHorizontalOverflow(page)).toBe(false)
})

async function prepareAudit(page: Page, theme: 'light' | 'dark') {
  await page.addInitScript((resolvedTheme) => {
    localStorage.setItem('releasehub.language', 'zh-TW')
    localStorage.setItem('releasehub.theme', resolvedTheme)
  }, theme)
  await page.route('**/api/v1/auth/session', (route) =>
    route.fulfill(
      json({
        data: {
          userId: '019d0000-0000-7000-8000-000000000001',
          username: 'auditor',
          mustChangePassword: false,
          passwordChangeAvailable: true,
        },
        meta: meta(),
      }),
    ),
  )
  await page.route('**/api/v1/audit/capabilities', (route) =>
    route.fulfill(
      json({
        data: {
          visible: true,
          scopeRoots: [
            {
              kind: 'project',
              id: '019d0000-0000-7000-8000-000000000010',
              label: 'Project A',
            },
          ],
        },
        meta: meta(),
      }),
    ),
  )
  await page.route(/\/api\/v1\/audit\/events\/[^/?]+$/, (route) =>
    route.fulfill(
      json({
        data: {
          ...auditEvent,
          metadata: { result: 'safe-value' },
          metadataTruncated: true,
        },
        meta: meta(),
      }),
    ),
  )
  await page.route('**/api/v1/audit/filter-options?*', (route) => {
    const field = new URL(route.request().url()).searchParams.get('field')
    const data =
      field === 'action'
        ? [
            {
              value: 'workflow.version.published',
              label: 'workflow.version.published',
            },
          ]
        : []
    return route.fulfill(json({ data, meta: meta() }))
  })
  await page.route('**/api/v1/audit/events?*', (route) =>
    route.fulfill(
      json({
        data: [auditEvent],
        meta: { ...meta(), hasMore: false },
      }),
    ),
  )
}

const auditEvent = {
  id: '019d0000-0000-7000-8000-000000000020',
  occurredAt: '2026-09-17T01:00:00Z',
  actor: { kind: 'user', displayName: 'auditor' },
  action: 'workflow.version.published',
  resource: { type: 'release_workflow', id: 'workflow-1' },
  scope: { kind: 'project', resolution: 'resolved' },
  requestId: 'audit-e2e-request',
  hasMetadata: true,
}

function json(body: unknown) {
  return { contentType: 'application/json', body: JSON.stringify(body) }
}

function meta() {
  return { requestId: 'audit-e2e', timestamp: new Date().toISOString() }
}

async function hasHorizontalOverflow(page: Page) {
  return page.evaluate(
    () => document.documentElement.scrollWidth > window.innerWidth,
  )
}
