import { expect, test, type Page } from '@playwright/test'

test.beforeEach(async ({ page }) => {
  await page.addInitScript(() => {
    localStorage.setItem('releasehub.language', 'en')
    localStorage.setItem('releasehub.theme', 'light')
  })
  await mockShell(page)
  await mockAccessReads(page)
})

test('keeps the active resource in the URL and shows one contextual create action', async ({
  page,
}) => {
  await page.goto('/access?tab=groups')

  await expect(page).toHaveURL(/\/access\?tab=groups$/)
  await expect(page.getByRole('tab')).toHaveCount(6)
  await expect(page.getByRole('button', { name: 'Create Group' })).toBeVisible()
  await expect(page.getByRole('button', { name: 'Create user' })).toHaveCount(0)

  await page.getByRole('tab', { name: 'Users' }).click()
  await expect(page).toHaveURL(/\/access\?tab=users$/)
  await expect(page.getByRole('button', { name: 'Create user' })).toBeVisible()
  await expect(page.getByRole('button', { name: 'Disable' })).toHaveCount(0)
})

test('creates a local user without sending the confirmation password', async ({
  page,
}) => {
  let requestBody: Record<string, unknown> | undefined
  await page.context().addCookies([
    {
      name: 'releasehub_csrf',
      value: 'access-csrf-token',
      url: 'http://127.0.0.1:4173',
    },
  ])
  await page.route('**/api/v1/access/users**', async (route) => {
    if (route.request().method() === 'POST') {
      requestBody = route.request().postDataJSON()
      return route.fulfill({
        status: 201,
        ...json({
          data: {
            id: '019c1230-0000-7000-8000-000000000002',
            username: 'operator',
            disabled: false,
            allowedActions: ['disable'],
          },
          meta: meta(),
        }),
      })
    }
    return route.fulfill(accessList([]))
  })
  await page.goto('/access')

  await page.getByRole('button', { name: 'Create user' }).click()
  const dialog = page.getByRole('dialog', { name: 'Create local user' })
  await dialog.getByLabel('Username').fill('operator')
  await dialog
    .getByLabel('Initial password', { exact: true })
    .fill('initial-password')
  await dialog.getByLabel('Confirm initial password').fill('initial-password')
  await dialog.getByRole('button', { name: 'Save' }).click()

  await expect(dialog).toBeHidden()
  expect(requestBody).toEqual({
    username: 'operator',
    initialPassword: 'initial-password',
  })
})

async function mockShell(page: Page) {
  await page.route('**/api/v1/auth/session', (route) =>
    route.fulfill(
      json({
        data: {
          userId: 'access-manager',
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

async function mockAccessReads(page: Page) {
  await page.route('**/api/v1/access/capabilities', (route) =>
    route.fulfill(
      json({
        data: {
          collections: [
            { key: 'users', visible: true, canCreate: true },
            { key: 'groups', visible: true, canCreate: true },
            { key: 'roles', visible: true, canCreate: true },
            { key: 'memberships', visible: true, canCreate: false },
            { key: 'bindings', visible: true, canCreate: true },
            { key: 'denies', visible: true, canCreate: true },
          ],
          permissions: [],
        },
        meta: meta(),
      }),
    ),
  )
  await page.route('**/api/v1/access/users**', (route) =>
    route.fulfill(
      accessList([
        {
          id: '019c1230-0000-7000-8000-000000000001',
          username: 'vincent',
          disabled: false,
          allowedActions: [],
        },
      ]),
    ),
  )
  for (const resource of [
    'groups',
    'roles',
    'memberships',
    'bindings',
    'denies',
  ]) {
    await page.route(`**/api/v1/access/${resource}**`, (route) =>
      route.fulfill(accessList([])),
    )
  }
  await page.route('**/api/v1/catalog/resource-tree', (route) =>
    route.fulfill(json({ data: [], meta: meta() })),
  )
}

function accessList(data: unknown[]) {
  return json({
    data,
    meta: { ...meta(), hasMore: false },
  })
}

function json(body: unknown) {
  return { contentType: 'application/json', body: JSON.stringify(body) }
}

function meta() {
  return { requestId: 'access-e2e', timestamp: new Date().toISOString() }
}
