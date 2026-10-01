import { expect, test, type Locator, type Page } from '@playwright/test'

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

test('角色綁定送出中鎖定欄位、阻止重送與關閉，成功後清空', async ({ page }) => {
  await mockBindingReads(page)
  let requests = 0
  let releaseRequest: (() => void) | undefined
  const pending = new Promise<void>((resolve) => (releaseRequest = resolve))
  await page.route('**/api/v1/access/bindings/batch', async (route) => {
    requests += 1
    await pending
    await route.fulfill({ status: 201, ...accessList([]) })
  })
  try {
    await page.goto('/access?tab=bindings')
    await page.getByRole('button', { name: 'Create binding' }).click()
    const dialog = page.getByRole('dialog')
    await fillBindingWithKeyboard(dialog)
    await dialog.getByRole('button', { name: 'Save', exact: true }).click()
    await expect.poll(() => requests).toBe(1)
    for (const label of ['Scope', 'Scope resource', 'Group', 'Role'])
      await expect(dialog.getByLabel(label, { exact: true })).toBeDisabled()
    await dialog.getByRole('button', { name: /Save/ }).click()
    await dialog.getByRole('button', { name: 'Cancel' }).click()
    await page.keyboard.press('Escape')
    await expect(dialog).toBeVisible()
    expect(requests).toBe(1)
    releaseRequest?.()
    await expect(dialog).toBeHidden()
    await page.getByRole('button', { name: 'Create binding' }).click()
    await expect(
      dialog
        .locator('.ant-select-selection-item')
        .filter({ hasText: 'viewer' }),
    ).toHaveCount(0)
    await expect(dialog.getByLabel('Role', { exact: true })).toBeEnabled()
  } finally {
    releaseRequest?.()
  }
})

for (const theme of ['light', 'dark'] as const) {
  test(`角色綁定在 ${theme} 主題的 320px 畫面可用鍵盤多選並送出`, async ({
    page,
  }, testInfo) => {
    await page.setViewportSize({ width: 320, height: 760 })
    await page.addInitScript(
      (value) => localStorage.setItem('releasehub.theme', value),
      theme,
    )
    await mockBindingReads(page)
    let body: Record<string, unknown> | undefined
    let csrf: string | undefined
    let requests = 0
    await page.route('**/api/v1/access/bindings/batch', async (route) => {
      body = route.request().postDataJSON()
      csrf = route.request().headers()['x-csrf-token']
      requests += 1
      await route.fulfill({ status: 201, ...accessList([]) })
    })
    await page.goto('/access?tab=bindings')
    await page.getByRole('button', { name: 'Create binding' }).click()
    const dialog = page.getByRole('dialog')
    await fillBindingWithKeyboard(dialog)
    await expect(
      dialog
        .locator('.ant-select-selection-item')
        .filter({ hasText: 'viewer' }),
    ).toBeVisible()
    await expect(
      dialog
        .locator('.ant-select-selection-item')
        .filter({ hasText: 'deployer' }),
    ).toBeVisible()
    const bounds = await dialog.boundingBox()
    if (!bounds) throw new Error('角色綁定對話框沒有可見的邊界')
    expect(bounds.x).toBeGreaterThanOrEqual(0)
    expect(bounds.x + bounds.width).toBeLessThanOrEqual(320)
    expect(
      await dialog.evaluate(
        (element) => element.scrollWidth <= element.clientWidth,
      ),
    ).toBe(true)
    const colors = await dialog
      .locator('.ant-modal-container')
      .evaluate((element) => {
        const style = getComputedStyle(element)
        const probe = document.createElement('span')
        probe.style.backgroundColor = 'var(--rh-color-surface-elevated)'
        probe.style.color = 'var(--rh-color-text-primary)'
        element.append(probe)
        const expected = getComputedStyle(probe)
        const result = {
          background: style.backgroundColor,
          color: style.color,
          expectedBackground: expected.backgroundColor,
          expectedColor: expected.color,
        }
        probe.remove()
        return result
      })
    await expect(page.locator('html')).toHaveAttribute('data-theme', theme)
    expect(colors.background).toBe(colors.expectedBackground)
    expect(colors.color).toBe(colors.expectedColor)
    expect(colors.color).not.toBe(colors.background)
    await page.screenshot({
      path: testInfo.outputPath(`binding-${theme}-320.png`),
    })
    await dialog.getByRole('button', { name: 'Save', exact: true }).focus()
    await page.keyboard.press('Enter')
    await expect(dialog).toBeHidden()
    expect(requests).toBe(1)
    expect(csrf).toBe('access-csrf-token')
    expect(body).toEqual({
      scopeKind: 'project',
      organizationId: bindingIds.organization,
      projectId: bindingIds.project,
      groupId: bindingIds.group,
      roleIds: [bindingIds.viewer, bindingIds.deployer],
    })
  })
}

const bindingIds = {
  organization: '019c1230-0000-7000-8000-000000000010',
  project: '019c1230-0000-7000-8000-000000000011',
  group: '019c1230-0000-7000-8000-000000000020',
  viewer: '019c1230-0000-7000-8000-000000000030',
  deployer: '019c1230-0000-7000-8000-000000000031',
}

async function mockBindingReads(page: Page) {
  await page.context().addCookies([
    {
      name: 'releasehub_csrf',
      value: 'access-csrf-token',
      url: 'http://127.0.0.1:4173',
    },
  ])
  await page.route('**/api/v1/system/status', (route) =>
    route.fulfill(json({ data: { tenancyMode: 'single' }, meta: meta() })),
  )
  await page.route('**/api/v1/catalog/resource-tree', (route) =>
    route.fulfill(
      json({
        data: [
          {
            id: bindingIds.organization,
            name: 'default',
            projects: [
              { id: bindingIds.project, name: 'Payment', environments: [] },
            ],
          },
        ],
        meta: meta(),
      }),
    ),
  )
  await page.route('**/api/v1/access/scopes/*/*/options', (route) =>
    route.fulfill(
      json({
        data: {
          groups: [
            { id: bindingIds.group, name: 'release-managers', disabled: false },
          ],
          roles: [
            { id: bindingIds.viewer, name: 'viewer', active: true },
            { id: bindingIds.deployer, name: 'deployer', active: true },
          ],
          permissions: [],
        },
        meta: meta(),
      }),
    ),
  )
}

async function fillBindingWithKeyboard(dialog: Locator) {
  // Modal 進場結束後才測 Tab 順序，避免首次自動聚焦覆蓋測試的焦點。
  await expect(dialog).not.toHaveClass(/ant-zoom-(appear|enter)/)
  const resource = dialog.getByLabel('Scope resource')
  await resource.focus()
  await resource.press('ArrowDown')
  await expect(
    dialog
      .page()
      .locator('.ant-select-item-option-content')
      .filter({ hasText: 'Payment' }),
  ).toBeVisible()
  await resource.press('ArrowDown')
  await resource.press('Enter')
  await expect(
    dialog
      .locator('.ant-select')
      .filter({ has: dialog.page().getByLabel('Scope resource') }),
  ).toContainText('Payment')
  await expect(resource).toHaveAttribute('aria-expanded', 'false')
  const group = dialog.getByLabel('Group', { exact: true })
  await resource.press('Tab')
  await expect(group).toBeFocused()
  await group.press('ArrowDown')
  await expect(
    dialog
      .page()
      .locator('.ant-select-item-option-content')
      .filter({ hasText: 'release-managers' }),
  ).toBeVisible()
  await group.press('ArrowDown')
  await group.press('Enter')
  await expect(
    dialog
      .locator('.ant-select')
      .filter({ has: dialog.page().getByLabel('Group', { exact: true }) }),
  ).toContainText('release-managers')
  await expect(group).toHaveAttribute('aria-expanded', 'false')
  const role = dialog.getByLabel('Role', { exact: true })
  await group.press('Tab')
  await expect(role).toBeFocused()
  await role.fill('viewer')
  await role.press('ArrowDown')
  await role.press('Enter')
  await role.fill('deployer')
  await role.press('ArrowDown')
  await role.press('Enter')
  await role.press('Escape')
  await expect(role).toHaveAttribute('aria-expanded', 'false')
  await expect(
    dialog.page().locator('.ant-select-dropdown:visible'),
  ).toHaveCount(0)
}

async function mockShell(page: Page) {
  await page.route('**/api/v1/auth/session', (route) =>
    route.fulfill(
      json({
        data: {
          userId: 'access-manager',
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
