import { expect, test } from '@playwright/test'

test('returns to a blank sign-in form after the server session deadline', async ({
  page,
}) => {
  await page.addInitScript(() => {
    localStorage.setItem('releasehub.language', 'en')
    localStorage.setItem('releasehub.theme', 'light')
  })

  const idleDeadline = Date.now() + 10_000
  let expiredSessionRequests = 0
  await page.route('**/api/v1/auth/session', (route) => {
    if (Date.now() < idleDeadline) {
      return route.fulfill({
        contentType: 'application/json',
        body: JSON.stringify({
          data: {
            userId: 'user-123',
            username: 'vincent',
            mustChangePassword: false,
            passwordChangeAvailable: true,
            idleExpiresAt: new Date(idleDeadline).toISOString(),
            absoluteExpiresAt: new Date(Date.now() + 60_000).toISOString(),
          },
          meta: {
            requestId: 'session-timeout-e2e',
            timestamp: new Date().toISOString(),
          },
        }),
      })
    }

    expiredSessionRequests += 1
    return route.fulfill({
      status: 401,
      contentType: 'application/json',
      body: JSON.stringify({
        code: 'SESSION_INVALID',
        message: 'Authentication session is invalid',
        requestId: 'session-timeout-e2e',
      }),
    })
  })
  await page.route('**/api/v1/system/status', (route) =>
    route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({
        data: {
          name: 'ReleaseHub',
          version: 'test',
          tenancyMode: 'single',
          oidcEnabled: false,
        },
        meta: {
          requestId: 'session-timeout-e2e',
          timestamp: new Date().toISOString(),
        },
      }),
    }),
  )

  await page.goto('/')
  await expect(page.getByText('vincent')).toBeVisible()
  await expect(page.getByRole('heading', { name: 'Welcome back' })).toBeVisible(
    { timeout: 15_000 },
  )
  await expect(page.getByLabel('Username')).toHaveValue('')
  await expect(page.getByLabel('Password')).toHaveValue('')
  expect(expiredSessionRequests).toBeGreaterThanOrEqual(1)
})

test('returns to sign in when the resource tree observes an invalid session first', async ({
  page,
}) => {
  await page.addInitScript(() => {
    localStorage.setItem('releasehub.language', 'en')
    localStorage.setItem('releasehub.theme', 'light')
  })

  let sessionRequests = 0
  await page.route('**/api/v1/auth/session', (route) => {
    sessionRequests += 1
    if (sessionRequests === 1) {
      return route.fulfill({
        contentType: 'application/json',
        body: JSON.stringify({
          data: {
            userId: 'user-123',
            username: 'vincent',
            mustChangePassword: false,
            passwordChangeAvailable: true,
            idleExpiresAt: new Date(Date.now() + 60_000).toISOString(),
            absoluteExpiresAt: new Date(Date.now() + 120_000).toISOString(),
          },
          meta: {
            requestId: 'resource-tree-session-e2e',
            timestamp: new Date().toISOString(),
          },
        }),
      })
    }

    return route.fulfill({
      status: 401,
      contentType: 'application/json',
      body: JSON.stringify({
        code: 'SESSION_INVALID',
        message: 'Authentication session is invalid',
        requestId: 'resource-tree-session-e2e',
      }),
    })
  })
  await page.route('**/api/v1/catalog/resource-tree', (route) =>
    route.fulfill({
      status: 401,
      contentType: 'application/json',
      body: JSON.stringify({
        code: 'SESSION_INVALID',
        message: 'Authentication session is invalid',
        requestId: 'resource-tree-session-e2e',
      }),
    }),
  )
  await page.route('**/api/v1/system/status', (route) =>
    route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({
        data: {
          name: 'ReleaseHub',
          version: 'test',
          tenancyMode: 'single',
          oidcEnabled: false,
        },
        meta: {
          requestId: 'resource-tree-session-e2e',
          timestamp: new Date().toISOString(),
        },
      }),
    }),
  )

  await page.goto('/resources')
  await expect(
    page.getByRole('heading', { name: 'Welcome back' }),
  ).toBeVisible()
  await expect(page.getByLabel('Username')).toHaveValue('')
  await expect(page.getByLabel('Password')).toHaveValue('')
  await expect(page.getByText('Unable to load resources')).toBeHidden()
  expect(sessionRequests).toBe(2)
})
