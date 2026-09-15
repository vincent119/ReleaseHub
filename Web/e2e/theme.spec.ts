import { expect, test, type Page } from '@playwright/test'

test.use({ viewport: { width: 1440, height: 900 } })

test.describe('ReleaseHub themes', () => {
  test('dark theme applies shared surfaces and concise navigation', async ({
    page,
  }) => {
    await prepareTheme(page, 'dark')
    await page.goto('/')

    await expectSemanticTheme(page, {
      pageToken: '#090b12',
      surfaceToken: '#0f172a',
      pageBackground: 'rgb(9, 11, 18)',
      ambient: true,
    })
    await expectBrandMark(page)
    await expectConciseNavigation(page)
  })

  test('light theme provides the same semantic contract without ambience', async ({
    page,
  }) => {
    await prepareTheme(page, 'light')
    await page.goto('/')

    await expectSemanticTheme(page, {
      pageToken: '#f6f8fb',
      surfaceToken: '#ffffff',
      pageBackground: 'rgb(246, 248, 251)',
      ambient: false,
    })
    await expectBrandMark(page)
    await expectConciseNavigation(page)
  })

  test('dark theme remains usable below the desktop sidebar breakpoint', async ({
    page,
  }) => {
    await page.setViewportSize({ width: 900, height: 800 })
    await prepareTheme(page, 'dark')
    await page.goto('/')

    await expect(
      page.getByRole('heading', { name: 'Platform overview' }),
    ).toBeVisible()
    expect(
      await page.evaluate(
        () => document.documentElement.scrollWidth <= window.innerWidth,
      ),
    ).toBe(true)
    await expect(page.locator('.ant-layout-sider')).toHaveCSS(
      'max-width',
      '0px',
    )
  })
})

async function prepareTheme(page: Page, theme: 'light' | 'dark') {
  await page.addInitScript((resolvedTheme) => {
    localStorage.setItem('releasehub.language', 'en')
    localStorage.setItem('releasehub.theme', resolvedTheme)
  }, theme)
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

async function expectSemanticTheme(
  page: Page,
  expected: {
    pageToken: string
    surfaceToken: string
    pageBackground: string
    ambient: boolean
  },
) {
  await page.locator('.ant-layout-content').waitFor()
  const styles = await page.evaluate(() => {
    const root = getComputedStyle(document.documentElement)
    const content = getComputedStyle(
      document.querySelector('.ant-layout-content')!,
    )
    const header = getComputedStyle(
      document.querySelector('.ant-layout-header')!,
    )
    return {
      pageToken: root.getPropertyValue('--rh-color-page').trim(),
      surfaceToken: root.getPropertyValue('--rh-color-surface').trim(),
      ambient: root.getPropertyValue('--rh-ambient-page').trim(),
      contentBackground: content.backgroundColor,
      headerBackground: header.backgroundColor,
    }
  })

  expect(styles.pageToken).toBe(expected.pageToken)
  expect(styles.surfaceToken).toBe(expected.surfaceToken)
  expect(styles.contentBackground).toBe(expected.pageBackground)
  expect(styles.headerBackground).not.toBe('rgb(0, 0, 0)')
  expect(styles.headerBackground).not.toBe('rgb(255, 255, 255, 0)')
  if (expected.ambient) expect(styles.ambient).toContain('radial-gradient')
  else expect(styles.ambient).toBe('none')
}

async function expectConciseNavigation(page: Page) {
  const sidebar = page.locator('.ant-layout-sider')
  const expectations = [
    ['Applications', 'Applications'],
    ['Deployment Requests', 'Requests'],
    ['Release Workflows', 'Workflows'],
    ['Deployment Plans', 'Plans'],
    ['Access management', 'Access'],
  ] as const

  for (const [accessibleName, visibleLabel] of expectations) {
    const link = sidebar.getByRole('link', { name: accessibleName })
    await expect(link).toHaveText(visibleLabel)
    expect(
      await link.evaluate(
        (element) => element.scrollWidth <= element.clientWidth,
      ),
    ).toBe(true)
  }
}

async function expectBrandMark(page: Page) {
  const sidebar = page.locator('.ant-layout-sider')
  const mark = sidebar.getByRole('img', { name: 'ReleaseHub' })
  const brandName = sidebar.getByText('ReleaseHub')

  await expect(mark).toBeVisible()
  await expect(brandName).toBeVisible()
  await expect(mark).toHaveCSS('width', '40px')
  await expect(mark).toHaveCSS('height', '40px')
  await expect(page.locator('link[rel="icon"]')).toHaveAttribute(
    'href',
    '/favicon.ico',
  )

  const markSource = await mark.getAttribute('src')
  expect(markSource).toContain('data:image/svg+xml')
  expect(decodeURIComponent(markSource ?? '')).toContain('#02C874')

  const positions = await Promise.all([
    mark.boundingBox(),
    brandName.boundingBox(),
  ])
  expect(positions[0]).not.toBeNull()
  expect(positions[1]).not.toBeNull()
  expect(positions[0]!.x + positions[0]!.width).toBeLessThanOrEqual(
    positions[1]!.x,
  )
  expect(
    Math.abs(
      positions[0]!.y +
        positions[0]!.height / 2 -
        (positions[1]!.y + positions[1]!.height / 2),
    ),
  ).toBeLessThanOrEqual(1)
}

function json(body: unknown) {
  return { contentType: 'application/json', body: JSON.stringify(body) }
}

function meta() {
  return { requestId: 'theme-e2e', timestamp: new Date().toISOString() }
}
