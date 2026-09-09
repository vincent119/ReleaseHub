import { expect, test, type Locator, type Page } from '@playwright/test'

test.beforeEach(async ({ page }) => {
  await mockShell(page)
})

test('collapses, persists, navigates, and expands on desktop', async ({
  page,
}) => {
  await page.setViewportSize({ width: 1440, height: 900 })
  await preparePreferences(page, 'en', 'light', null)
  await page.goto('/')

  const sidebar = page.locator('.ant-layout-sider')
  const header = page.locator('header.ant-layout-header')
  const logo = sidebar.getByRole('img', { name: 'ReleaseHub' })
  const collapse = sidebar.getByRole('button', { name: 'Collapse sidebar' })

  await expect(sidebar).toHaveCSS('width', '200px')
  await expect(header.locator('..')).toHaveCSS(
    'background-color',
    'rgb(246, 248, 251)',
  )
  await expectDesktopHeaderSeam(page, sidebar)
  await expect(logo).toBeVisible()
  await collapse.focus()
  await page.keyboard.press('Enter')

  await expect(sidebar).toHaveCSS('width', '80px')
  await expectDesktopHeaderSeam(page, sidebar)
  await expect(logo).toBeVisible()
  await expect(sidebar.getByText('ReleaseHub', { exact: true })).toHaveCount(0)
  await expect(
    sidebar.getByRole('button', { name: 'Expand sidebar' }),
  ).toBeFocused()
  await expect
    .poll(() =>
      page.evaluate(() => localStorage.getItem('releasehub.sidebar.collapsed')),
    )
    .toBe('true')
  expect(await hasHorizontalOverflow(page)).toBe(false)

  const requests = sidebar.getByRole('link', {
    name: 'Deployment Requests',
  })
  await expect(requests).toBeVisible()
  await requests.hover()
  await expect(page.getByRole('tooltip')).toContainText('Requests')
  await requests.click()
  await expect(page).toHaveURL(/\/requests$/)

  await page.reload()
  await expect(sidebar).toHaveCSS('width', '80px')
  const expand = sidebar.getByRole('button', { name: 'Expand sidebar' })
  await expand.focus()
  await page.keyboard.press('Space')

  await expect(sidebar).toHaveCSS('width', '200px')
  await expect(sidebar.getByText('ReleaseHub', { exact: true })).toBeVisible()
  await expect
    .poll(() =>
      page.evaluate(() => localStorage.getItem('releasehub.sidebar.collapsed')),
    )
    .toBe('false')
})

test('keeps the desktop preference across the responsive breakpoint', async ({
  page,
}) => {
  await page.setViewportSize({ width: 1440, height: 900 })
  await preparePreferences(page, 'en', 'dark', true)
  await page.goto('/')

  const sidebar = page.locator('.ant-layout-sider')
  await expect(sidebar).toHaveCSS('width', '80px')

  await page.setViewportSize({ width: 900, height: 800 })
  await expect(sidebar).toHaveCSS('max-width', '0px')
  await expectMobileHeaderSeam(page)
  await expect(
    sidebar.getByRole('button', { name: 'Expand sidebar' }),
  ).toHaveCount(0)
  expect(await hasHorizontalOverflow(page)).toBe(false)

  await page.setViewportSize({ width: 1440, height: 900 })
  await expect(sidebar).toHaveCSS('width', '80px')
  await expectDesktopHeaderSeam(page, sidebar)
  await expect(
    sidebar.getByRole('button', { name: 'Expand sidebar' }),
  ).toBeVisible()
  expect(
    await page.evaluate(() =>
      localStorage.getItem('releasehub.sidebar.collapsed'),
    ),
  ).toBe('true')
})

test('uses localized controls and shared dark theme styling', async ({
  page,
}) => {
  await page.setViewportSize({ width: 1440, height: 900 })
  await preparePreferences(page, 'zh-TW', 'dark', false)
  await page.goto('/')

  const sidebar = page.locator('.ant-layout-sider')
  const header = page.locator('header.ant-layout-header')
  const collapse = sidebar.getByRole('button', { name: '收合側邊欄' })
  await expect(collapse).toBeVisible()
  await expect(sidebar).toHaveCSS('background-color', 'rgb(15, 23, 42)')
  await expect(header.locator('..')).toHaveCSS(
    'background-color',
    'rgb(9, 11, 18)',
  )
  await expectDesktopHeaderSeam(page, sidebar)

  await collapse.click()

  await expect(
    sidebar.getByRole('button', { name: '展開側邊欄' }),
  ).toBeVisible()
  await expect(page.locator('html')).toHaveAttribute('data-theme', 'dark')
})

async function preparePreferences(
  page: Page,
  language: 'en' | 'zh-TW',
  theme: 'light' | 'dark',
  collapsed: boolean | null,
) {
  await page.addInitScript(
    ({ activeLanguage, activeTheme, sidebarCollapsed }) => {
      if (sessionStorage.getItem('releasehub.sidebar.e2e-initialized')) return

      localStorage.setItem('releasehub.language', activeLanguage)
      localStorage.setItem('releasehub.theme', activeTheme)
      if (sidebarCollapsed === null) {
        localStorage.removeItem('releasehub.sidebar.collapsed')
      } else {
        localStorage.setItem(
          'releasehub.sidebar.collapsed',
          String(sidebarCollapsed),
        )
      }
      sessionStorage.setItem('releasehub.sidebar.e2e-initialized', 'true')
    },
    {
      activeLanguage: language,
      activeTheme: theme,
      sidebarCollapsed: collapsed,
    },
  )
}

async function mockShell(page: Page) {
  await page.route('**/api/v1/auth/session', (route) =>
    route.fulfill(
      json({
        data: {
          userId: 'sidebar-user',
          username: 'vincent',
          mustChangePassword: false,
        },
        meta: meta(),
      }),
    ),
  )
  await page.route('**/api/v1/notifications**', (route) =>
    route.fulfill(json({ data: [], meta: meta() })),
  )
}

async function hasHorizontalOverflow(page: Page) {
  return page.evaluate(
    () => document.documentElement.scrollWidth > window.innerWidth,
  )
}

async function expectDesktopHeaderSeam(page: Page, sidebar: Locator) {
  const header = page.locator('header.ant-layout-header')
  await expect(header).toHaveCSS('margin-left', '3px')
  await expect(header).toHaveCSS('border-bottom-left-radius', '12px')
  await expect
    .poll(async () => {
      const [sidebarBox, headerBox] = await Promise.all([
        sidebar.boundingBox(),
        header.boundingBox(),
      ])
      if (!sidebarBox || !headerBox) return null
      return headerBox.x - (sidebarBox.x + sidebarBox.width)
    })
    .toBe(3)
  expect(await hasHorizontalOverflow(page)).toBe(false)
}

async function expectMobileHeaderSeam(page: Page) {
  const header = page.locator('header.ant-layout-header')
  await expect(header).toHaveCSS('margin-left', '0px')
  await expect(header).toHaveCSS('border-bottom-left-radius', '0px')
  await expect.poll(async () => (await header.boundingBox())?.x ?? null).toBe(0)
}

function json(body: unknown) {
  return { contentType: 'application/json', body: JSON.stringify(body) }
}

function meta() {
  return { requestId: 'sidebar-e2e', timestamp: new Date().toISOString() }
}
