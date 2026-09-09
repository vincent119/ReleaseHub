import { expect, test, type Page } from '@playwright/test'

test.use({ viewport: { width: 1440, height: 900 } })

for (const theme of ['light', 'dark'] as const) {
  test(`${theme} 主題 Workflow 編輯器保有單一焦點與可讀節點`, async ({
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

    await page.goto('/workflows')
    await page.getByRole('button', { name: '建立 Workflow' }).click()

    const dialog = page.getByRole('dialog', { name: '建立 Release Workflow' })
    await expect(dialog).toBeVisible()
    await expect
      .poll(async () => (await dialog.boundingBox())?.x)
      .toBeLessThanOrEqual(1)
    await expect
      .poll(async () => (await dialog.boundingBox())?.width)
      .toBeGreaterThanOrEqual(1438)

    const nodes = page.locator(
      '.react-flow__node-workflowState [data-state-type]',
    )
    await expect(nodes).toHaveCount(5)
    await expect(nodes.filter({ hasText: 'Pending Review' })).toBeVisible()
    await expect(nodes.filter({ hasText: 'Deploying' })).toBeVisible()
    await expect(nodes.filter({ hasText: 'Succeeded' })).toBeVisible()
    await expect(nodes.filter({ hasText: 'Failed' })).toBeVisible()

    const colors = await nodes.first().evaluate((element) => {
      const style = getComputedStyle(element)
      return { color: style.color, background: style.backgroundColor }
    })
    expect(colors.color).not.toBe(colors.background)
    expect(colors.background).not.toBe('rgb(0, 0, 0)')
    if (theme === 'dark') {
      expect(colors.background).not.toBe('rgb(255, 255, 255)')
    }

    const drawerBackground = await dialog
      .locator('.ant-drawer-body')
      .evaluate((element) => getComputedStyle(element).backgroundImage)
    if (theme === 'dark') {
      expect(drawerBackground).toContain('radial-gradient')
    } else {
      expect(drawerBackground).toBe('none')
    }

    await nodes.filter({ hasText: 'Pending Review' }).click()
    await expect(page.getByText('狀態設定', { exact: true })).toBeVisible()
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
        },
        meta: meta(),
      }),
    ),
  )
}

function json(body: unknown) {
  return { contentType: 'application/json', body: JSON.stringify(body) }
}

function meta() {
  return { requestId: 'e2e', timestamp: new Date().toISOString() }
}
