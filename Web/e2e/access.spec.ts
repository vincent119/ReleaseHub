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

test('loads Group candidates without search and removes only manual members', async ({
  page,
}) => {
  let candidateURL = ''
  let removedMembership = ''
  await page.context().addCookies([
    {
      name: 'releasehub_csrf',
      value: 'access-csrf-token',
      url: 'http://127.0.0.1:4173',
    },
  ])
  await page.route('**/api/v1/access/groups**', (route) =>
    route.fulfill(
      accessList([
        {
          id: '019c1230-0000-7000-8000-000000000010',
          ownerKind: 'platform',
          name: 'release-managers',
          oidcViewerOnly: false,
          disabled: false,
          allowedActions: ['viewMemberships', 'addMember'],
        },
      ]),
    ),
  )
  await page.route(
    '**/api/v1/access/groups/*/membership-candidates**',
    (route) => {
      candidateURL = route.request().url()
      return route.fulfill(
        accessList([
          {
            id: '019c1230-0000-7000-8000-000000000011',
            username: 'amy',
          },
        ]),
      )
    },
  )
  await page.route('**/api/v1/access/memberships**', (route) =>
    route.fulfill(
      accessList([
        {
          id: 'manual-membership',
          groupId: '019c1230-0000-7000-8000-000000000010',
          groupName: 'release-managers',
          userId: '019c1230-0000-7000-8000-000000000012',
          username: 'manual-user',
          source: 'manual',
          active: true,
          allowedActions: ['revoke'],
        },
        {
          id: 'oidc-membership',
          groupId: '019c1230-0000-7000-8000-000000000010',
          groupName: 'release-managers',
          userId: '019c1230-0000-7000-8000-000000000013',
          username: 'oidc-user',
          source: 'oidc',
          active: true,
          allowedActions: [],
        },
      ]),
    ),
  )
  await page.route(
    '**/api/v1/access/memberships/manual-membership/revoke',
    (route) => {
      if (route.request().method() === 'POST') {
        removedMembership = 'manual-membership'
        return route.fulfill({ status: 204 })
      }
      return route.continue()
    },
  )

  await page.goto('/access?tab=groups')
  await page.getByRole('button', { name: 'Manage members' }).click()
  const dialog = page.getByRole('dialog', { name: 'release-managers members' })

  await expect(dialog.getByText('manual-user')).toBeVisible()
  await expect(dialog.getByText('oidc-user')).toBeVisible()
  await expect(dialog.getByRole('button', { name: 'Remove' })).toHaveCount(1)
  await expect(dialog.getByText('Managed by OIDC')).toBeVisible()
  await dialog.getByRole('combobox').click()
  await expect(page.getByText('amy')).toBeVisible()
  expect(new URL(candidateURL).searchParams.has('query')).toBe(false)

  await dialog.getByRole('button', { name: 'Remove' }).click()
  await page.getByRole('button', { name: 'OK' }).click()
  await expect.poll(() => removedMembership).toBe('manual-membership')
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
