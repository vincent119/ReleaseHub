import { expect, test, type Page } from '@playwright/test'

test.beforeEach(async ({ page }) => {
  await page.addInitScript(() => {
    localStorage.setItem('releasehub.language', 'en')
    localStorage.setItem('releasehub.theme', 'light')
  })
  await mockShell(page)
})

test('shows read-only account information and separate preference settings', async ({
  page,
}) => {
  await page.setViewportSize({ width: 1440, height: 900 })
  await page.goto('/')

  const trigger = page.getByRole('button', {
    name: 'Open account menu for vincent',
  })
  await expectNotificationIconSize(page)
  await trigger.focus()
  await page.keyboard.press('Enter')
  await expect(
    page.getByRole('menuitem', { name: 'Personal settings' }),
  ).toBeVisible()
  await expect(
    page.locator('.ant-dropdown-menu').getByRole('menuitem'),
  ).toHaveText(['Personal settings', 'Language', 'Theme settings', 'Sign out'])
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
  await expect(personal.getByLabel('Current password')).toBeVisible()
  await expect(
    personal.getByLabel('New password', { exact: true }),
  ).toBeVisible()
  await expect(personal.getByLabel('Confirm new password')).toBeVisible()
  await expect(
    personal.getByRole('button', { name: 'Change password' }),
  ).toHaveCount(0)
  await expect(personal.getByRole('button', { name: 'Confirm' })).toBeVisible()
  await page.getByRole('button', { name: 'Close' }).click()
  await expect(trigger).toBeFocused()

  await trigger.click()
  await page.getByRole('menuitem', { name: 'Language' }).click()
  const languageSelect = page.getByRole('combobox', { name: 'Language' })
  const languagePopover = page.locator('.ant-popover-container')
  await expect(languageSelect).toBeVisible()
  await expect(languageSelect.locator('..')).toHaveText('English')
  await expect(
    languagePopover.getByText('Language', { exact: true }),
  ).toHaveCount(1)
  await expect(languageSelect.locator('../..')).toHaveCSS('width', '288px')
  await expect(page.getByRole('combobox', { name: 'Theme' })).toHaveCount(0)
  await page.keyboard.press('Escape')

  await trigger.click()
  await page.getByRole('menuitem', { name: 'Theme settings' }).click()
  const themeSelect = page.getByRole('combobox', { name: 'Theme' })
  await expect(themeSelect).toBeVisible()
  await expect(themeSelect.locator('..')).toHaveText('Light')
  await expect(page.getByText('Theme', { exact: true })).toHaveCount(0)
  await expect(page.getByRole('combobox', { name: 'Language' })).toHaveCount(0)
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
  await expectNotificationIconSize(page)
  await expect(trigger).toBeVisible()
  await expect(trigger.getByText('vincent')).toBeHidden()
  await trigger.click()
  await page.getByRole('menuitem', { name: 'Theme settings' }).click()

  const dialog = page.getByRole('dialog', { name: 'Theme settings' })
  await expect(dialog).toBeVisible()
  await expect(dialog.getByText('Theme', { exact: true })).toHaveCount(0)
  const mobileSelect = dialog.locator('.ant-select')
  const widths = await mobileSelect.evaluate((select) => ({
    select: select.getBoundingClientRect().width,
    controls: select.parentElement?.getBoundingClientRect().width ?? 0,
  }))
  expect(widths.select).toBeCloseTo(widths.controls, 0)
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
  await page.getByRole('combobox', { name: 'Theme' }).click()
  await page.getByText('Dark', { exact: true }).click()
  await expect(page.locator('html')).toHaveAttribute('data-theme', 'dark')
  await expectNotificationIconSize(page)

  await trigger.click()
  await page.getByRole('menuitem', { name: 'Theme settings' }).click()
  await expect(
    page.getByRole('combobox', { name: 'Theme' }).locator('..'),
  ).toHaveText('Dark')
  const popover = page.locator('.ant-popover-container')
  await expect(popover).toBeVisible()
  await expect(popover).toHaveCSS('background-color', 'rgb(17, 24, 39)')

  await page.keyboard.press('Escape')
  await trigger.click()
  const dropdown = page.locator('.ant-dropdown-menu')
  await expect(dropdown).toBeVisible()
  await expect(dropdown).toHaveCSS('background-color', 'rgb(17, 24, 39)')
})

test('changes a local password and returns to sign in after session revocation', async ({
  page,
}) => {
  const csrfToken = 'password-change-csrf-token'
  let authenticated = true
  let requestBody: unknown
  let requestCSRFHeader = ''
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
              passwordChangeAvailable: true,
            },
            meta: meta(),
          })
        : { status: 401, ...json({ error: { code: 'unauthorized' } }) },
    ),
  )
  await page.route('**/api/v1/auth/password', async (route) => {
    requestBody = route.request().postDataJSON()
    requestCSRFHeader = route.request().headers()['x-csrf-token'] ?? ''
    authenticated = false
    return route.fulfill({ status: 204 })
  })
  await page.goto('/')

  await page
    .getByRole('button', { name: 'Open account menu for vincent' })
    .click()
  await page.getByRole('menuitem', { name: 'Personal settings' }).click()
  await page.getByLabel('Current password').fill('current-password')
  await page.getByLabel('New password', { exact: true }).fill('new-password')
  await page.getByLabel('Confirm new password').fill('new-password')
  await page
    .getByRole('dialog', { name: 'Personal settings' })
    .getByRole('button', { name: 'Confirm' })
    .click()

  await expect(
    page.getByRole('heading', { name: 'Welcome back' }),
  ).toBeVisible()
  expect(requestBody).toEqual({
    currentPassword: 'current-password',
    newPassword: 'new-password',
  })
  expect(requestCSRFHeader).toBe(csrfToken)
})

test('does not show local password controls for an OIDC session', async ({
  page,
}) => {
  await page.unroute('**/api/v1/auth/session')
  await page.route('**/api/v1/auth/session', (route) =>
    route.fulfill(
      json({
        data: {
          userId: 'user-123',
          username: 'vincent',
          mustChangePassword: false,
          passwordChangeAvailable: false,
        },
        meta: meta(),
      }),
    ),
  )
  await page.goto('/')

  await page
    .getByRole('button', { name: 'Open account menu for vincent' })
    .click()
  await page.getByRole('menuitem', { name: 'Personal settings' }).click()

  const personal = page.getByRole('dialog', { name: 'Personal settings' })
  await expect(
    personal.getByRole('button', { name: 'Change password' }),
  ).toHaveCount(0)
  await expect(personal.getByRole('button', { name: 'Confirm' })).toHaveCount(0)
  await expect(personal.getByRole('textbox')).toHaveCount(0)
})

test('keeps the password form open when the current password is rejected', async ({
  page,
}) => {
  await page.route('**/api/v1/auth/password', (route) =>
    route.fulfill({ status: 400, ...json({ error: { code: 'invalid' } }) }),
  )
  await page.goto('/')

  await page
    .getByRole('button', { name: 'Open account menu for vincent' })
    .click()
  await page.getByRole('menuitem', { name: 'Personal settings' }).click()
  await page.getByLabel('Current password').fill('incorrect-password')
  await page.getByLabel('New password', { exact: true }).fill('new-password')
  await page.getByLabel('Confirm new password').fill('new-password')
  const personalDialog = page.getByRole('dialog', {
    name: 'Personal settings',
  })
  await personalDialog.getByRole('button', { name: 'Confirm' }).click()

  await expect(personalDialog).toBeVisible()
  await expect(
    personalDialog.getByText(
      'The password could not be changed. Check the current password and try again.',
    ),
  ).toBeVisible()
  await expect(page.getByLabel('Current password')).toHaveValue(
    'incorrect-password',
  )
})

test('keeps the inline password form usable on a narrow dark viewport', async ({
  page,
}) => {
  const longUserId = '766bdf1f-b573-4db4-9d77-76b25e42dc44'
  await page.setViewportSize({ width: 390, height: 760 })
  await page.addInitScript(() => {
    localStorage.setItem('releasehub.theme', 'dark')
  })
  await page.unroute('**/api/v1/auth/session')
  await page.route('**/api/v1/auth/session', (route) =>
    route.fulfill(
      json({
        data: {
          userId: longUserId,
          username: 'vincent',
          mustChangePassword: false,
          passwordChangeAvailable: true,
        },
        meta: meta(),
      }),
    ),
  )
  await page.goto('/')

  await page
    .getByRole('button', { name: 'Open account menu for vincent' })
    .click()
  await page.getByRole('menuitem', { name: 'Personal settings' }).click()
  const personalDialog = page.getByRole('dialog', {
    name: 'Personal settings',
  })

  await expect(page.getByRole('dialog')).toHaveCount(1)
  await expect(page.getByLabel('Current password')).toBeFocused()
  await expect(personalDialog).toContainText(longUserId)
  await expect(
    personalDialog.getByRole('button', { name: 'Cancel' }),
  ).toBeVisible()
  const bounds = await personalDialog.evaluate((element) => {
    const rect = element.getBoundingClientRect()
    return { left: rect.left, right: rect.right, viewport: window.innerWidth }
  })
  expect(bounds.left).toBeGreaterThanOrEqual(0)
  expect(bounds.right).toBeLessThanOrEqual(bounds.viewport)

  await page.getByLabel('Current password').fill('not-submitted')
  await personalDialog.getByRole('button', { name: 'Cancel' }).click()
  await expect(personalDialog).toHaveCount(0)

  await page
    .getByRole('button', { name: 'Open account menu for vincent' })
    .click()
  await page.getByRole('menuitem', { name: 'Personal settings' }).click()
  await expect(page.getByLabel('Current password')).toHaveValue('')
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
              passwordChangeAvailable: true,
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
          passwordChangeAvailable: true,
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

async function expectNotificationIconSize(page: Page) {
  const notificationIcon = page
    .getByRole('button', { name: 'Open notifications' })
    .locator('.anticon-bell')
  await expect(notificationIcon).toHaveCSS('font-size', '18px')
}
