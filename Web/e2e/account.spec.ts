import { expect, test, type Page } from '@playwright/test'

test.beforeEach(async ({ page }) => {
  await page.addInitScript(() => {
    localStorage.setItem('releasehub.language', 'en')
    localStorage.setItem('releasehub.theme', 'light')
  })
  await mockShell(page)
})

test('shows read-only account information and desktop theme settings', async ({
  page,
}) => {
  await page.setViewportSize({ width: 1440, height: 900 })
  await page.goto('/')

  const trigger = page.getByRole('button', {
    name: 'Open account menu for vincent',
  })
  await trigger.focus()
  await page.keyboard.press('Enter')
  await expect(
    page.getByRole('menuitem', { name: 'Personal settings' }),
  ).toBeVisible()
  await page.keyboard.press('Escape')
  await expect(
    page.getByRole('menuitem', { name: 'Personal settings' }),
  ).toBeHidden()
  await expect(trigger).toBeFocused()

  await trigger.click()
  await page.getByRole('menuitem', { name: 'Personal settings' }).click()

  const personal = page.getByRole('dialog', { name: 'Personal settings' })
  await expect(personal).toContainText('vincent')
  await expect(personal).toContainText('user-123')
  await expect(personal.getByRole('textbox')).toHaveCount(0)
  await page.getByRole('button', { name: 'Close' }).click()
  await expect(trigger).toBeFocused()

  await trigger.click()
  await page.getByRole('menuitem', { name: 'Theme settings' }).click()
  await expect(page.getByText('System', { exact: true })).toBeVisible()
  await expect(
    page.getByRole('dialog', { name: 'Theme settings' }),
  ).toHaveCount(0)
})

test('uses a Modal and an avatar-only trigger on a small screen', async ({
  page,
}) => {
  await page.setViewportSize({ width: 700, height: 800 })
  await page.goto('/')

  const trigger = page.getByRole('button', {
    name: 'Open account menu for vincent',
  })
  await expect(trigger).toBeVisible()
  await expect(trigger.getByText('vincent')).toBeHidden()
  await trigger.click()
  await page.getByRole('menuitem', { name: 'Theme settings' }).click()

  await expect(
    page.getByRole('dialog', { name: 'Theme settings' }),
  ).toBeVisible()
})

test('keeps desktop account overlays on the dark elevated surface', async ({
  page,
}) => {
  await page.setViewportSize({ width: 1440, height: 900 })
  await page.goto('/')

  const trigger = page.getByRole('button', {
    name: 'Open account menu for vincent',
  })
  await trigger.click()
  await page.getByRole('menuitem', { name: 'Theme settings' }).click()
  await page.getByText('Dark', { exact: true }).click()
  await expect(page.locator('html')).toHaveAttribute('data-theme', 'dark')

  await trigger.click()
  await page.getByRole('menuitem', { name: 'Theme settings' }).click()
  const popover = page.locator('.ant-popover-container')
  await expect(popover).toBeVisible()
  await expect(popover).toHaveCSS('background-color', 'rgb(17, 24, 39)')

  await page.keyboard.press('Escape')
  await trigger.click()
  const dropdown = page.locator('.ant-dropdown-menu')
  await expect(dropdown).toBeVisible()
  await expect(dropdown).toHaveCSS('background-color', 'rgb(17, 24, 39)')
})

test('calls the logout API once and leaves the authenticated shell', async ({
  page,
}) => {
  const csrfToken = 'logout-csrf-token'
  let authenticated = true
  let logoutCalls = 0
  let logoutCSRFHeader = ''
  await page.context().addCookies([
    {
      name: 'releasehub_csrf',
      value: csrfToken,
      url: 'http://127.0.0.1:4173',
    },
  ])
  await page.unroute('**/api/v1/auth/session')
  await page.route('**/api/v1/auth/session', (route) =>
    route.fulfill(
      authenticated
        ? json({
            data: {
              userId: 'user-123',
              username: 'vincent',
              mustChangePassword: false,
            },
            meta: meta(),
          })
        : { status: 401, ...json({ error: { code: 'unauthorized' } }) },
    ),
  )
  await page.route('**/api/v1/auth/logout', (route) => {
    logoutCalls += 1
    logoutCSRFHeader = route.request().headers()['x-csrf-token'] ?? ''
    authenticated = false
    return route.fulfill(json({ data: { loggedOut: true }, meta: meta() }))
  })
  await page.goto('/')

  await page
    .getByRole('button', { name: 'Open account menu for vincent' })
    .click()
  await page.getByRole('menuitem', { name: 'Sign out' }).click()

  await expect(
    page.getByRole('heading', { name: 'Welcome back' }),
  ).toBeVisible()
  await expect(page.getByLabel('Username')).toHaveValue('')
  await expect(page.getByLabel('Password')).toHaveValue('')
  expect(logoutCalls).toBe(1)
  expect(logoutCSRFHeader).toBe(csrfToken)
})

async function mockShell(page: Page) {
  await page.route('**/api/v1/auth/session', (route) =>
    route.fulfill(
      json({
        data: {
          userId: 'user-123',
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

function json(body: unknown) {
  return { contentType: 'application/json', body: JSON.stringify(body) }
}

function meta() {
  return { requestId: 'account-e2e', timestamp: new Date().toISOString() }
}
