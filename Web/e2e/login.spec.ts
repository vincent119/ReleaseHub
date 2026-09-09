import { expect, test, type Page } from '@playwright/test'

test.beforeEach(async ({ page }) => {
  await page.addInitScript(() => {
    localStorage.setItem('releasehub.language', 'en')
    localStorage.setItem('releasehub.theme', 'light')
  })
  await page.route('**/api/v1/auth/session', (route) =>
    route.fulfill({
      status: 401,
      contentType: 'application/json',
      body: JSON.stringify({ error: { code: 'SESSION_REQUIRED' } }),
    }),
  )
})

test('shows the enterprise sign-in only when OIDC is enabled', async ({
  page,
}) => {
  await mockSystemStatus(page, true)
  await page.setViewportSize({ width: 1440, height: 900 })
  await page.goto('/')

  await expect(
    page.getByRole('heading', { name: 'Welcome back' }),
  ).toBeVisible()
  await expect(page.getByAltText('ReleaseHub brand mark')).toBeVisible()
  await expect(
    page.getByRole('link', { name: 'Sign in with enterprise account' }),
  ).toHaveAttribute('href', '/api/v1/auth/login')
  await expect(
    page.getByRole('button', { name: 'Sign in with password' }),
  ).toBeVisible()

  const brand = await page
    .getByRole('region', { name: 'ReleaseHub' })
    .boundingBox()
  const form = await page
    .getByRole('region', { name: 'Welcome back' })
    .boundingBox()
  expect(brand).not.toBeNull()
  expect(form).not.toBeNull()
  expect(brand!.x).toBeLessThan(form!.x)
  expect(Math.abs(brand!.y - form!.y)).toBeLessThan(2)
})

test('keeps local sign-in available when OIDC is disabled or unknown', async ({
  page,
}) => {
  await mockSystemStatus(page, false)
  await page.goto('/')

  await expect(
    page.getByRole('link', { name: 'Sign in with enterprise account' }),
  ).toHaveCount(0)
  await expect(page.getByLabel('Username')).toHaveValue('')
  await expect(page.getByLabel('Password')).toBeEditable()

  await page.route('**/api/v1/system/status', (route) =>
    route.fulfill({ status: 503, contentType: 'application/json', body: '{}' }),
  )
  await page.reload()
  await expect(
    page.getByRole('button', { name: 'Sign in with password' }),
  ).toBeVisible()
  await expect(page.getByLabel('Username')).toHaveValue('')
  await expect(page.getByLabel('Password')).toHaveValue('')
  await expect(
    page.getByRole('link', { name: 'Sign in with enterprise account' }),
  ).toHaveCount(0)
})

test('uses a single-column dark layout without horizontal overflow on mobile', async ({
  page,
}) => {
  await page.addInitScript(() => {
    localStorage.setItem('releasehub.theme', 'dark')
  })
  await mockSystemStatus(page, true)
  await page.setViewportSize({ width: 320, height: 720 })
  await page.goto('/')

  await expect(page.locator('html')).toHaveAttribute('data-theme', 'dark')
  const brand = await page
    .getByRole('region', { name: 'ReleaseHub' })
    .boundingBox()
  const form = await page
    .getByRole('region', { name: 'Welcome back' })
    .boundingBox()
  expect(brand).not.toBeNull()
  expect(form).not.toBeNull()
  expect(form!.y).toBeGreaterThanOrEqual(brand!.y + brand!.height - 2)
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= innerWidth,
    ),
  ).toBe(true)
})

async function mockSystemStatus(page: Page, oidcEnabled: boolean) {
  await page.route('**/api/v1/system/status', (route) =>
    route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({
        data: {
          name: 'ReleaseHub',
          version: 'test',
          tenancyMode: 'single',
          oidcEnabled,
        },
        meta: {
          requestId: 'login-e2e',
          timestamp: new Date().toISOString(),
        },
      }),
    }),
  )
}
