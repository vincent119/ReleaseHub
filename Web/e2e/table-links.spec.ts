import { expect, test, type Page } from '@playwright/test'

for (const theme of ['light', 'dark'] as const) {
  test(`${theme} theme keeps Application and Request links readable`, async ({
    page,
  }) => {
    await preparePage(page, theme)
    const expectedColor =
      theme === 'light' ? 'rgb(79, 70, 229)' : 'rgb(129, 140, 248)'
    const expectedFocusColor =
      theme === 'light' ? 'rgb(79, 70, 229)' : 'rgb(165, 180, 252)'

    await page.goto('/applications')
    await expectThemedLink(
      page.getByRole('link', { name: 'status-webhooks' }),
      expectedColor,
      expectedFocusColor,
      '/applications/application-1',
    )

    await page.goto('/requests')
    await page.getByLabel('Select Request scope').click()
    await page.getByText('default / status-webhooks / production').click()
    await expectThemedLink(
      page.getByRole('link', { name: 'status-webhooks' }),
      expectedColor,
      expectedFocusColor,
      '/requests/request-1',
    )
  })
}

async function expectThemedLink(
  link: ReturnType<Page['getByRole']>,
  expectedColor: string,
  expectedFocusColor: string,
  expectedPath: string,
) {
  await expect(link).toBeVisible()
  await expect(link).toHaveCSS('color', expectedColor)
  await expect(link).toHaveAttribute('href', expectedPath)
  await link.focus()
  await expect(link).toHaveCSS('outline-color', expectedFocusColor)
  await expect(link).toHaveCSS('outline-style', 'solid')
}

async function preparePage(page: Page, theme: 'light' | 'dark') {
  await page.addInitScript((resolvedTheme) => {
    localStorage.setItem('releasehub.language', 'en')
    localStorage.setItem('releasehub.theme', resolvedTheme)
    class TestEventSource {
      onmessage = null
      addEventListener() {}
      close() {}
    }
    Object.defineProperty(window, 'EventSource', { value: TestEventSource })
  }, theme)
  await page.route('**/api/v1/auth/session', (route) =>
    route.fulfill(
      json({
        data: {
          userId: 'user-1',
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
  await page.route('**/api/v1/catalog/visible-applications', (route) =>
    route.fulfill(
      json({
        data: [
          {
            id: 'application-1',
            name: 'status-webhooks',
            argocdApplicationName: 'status-webhooks',
            argocdProject: 'status-webhooks',
            sourceTargetRevision: 'main',
          },
        ],
        meta: meta(),
      }),
    ),
  )
  await page.route('**/api/v1/catalog/resource-tree', (route) =>
    route.fulfill(json({ data: resourceTree(), meta: meta() })),
  )
  await page.route('**/api/v1/deployment-requests**', (route) =>
    route.fulfill(
      json({
        data: [
          {
            id: 'request-1',
            organizationId: 'organization-1',
            projectId: 'project-1',
            environmentId: 'environment-1',
            status: 'Failed',
            classification: 'Standard',
            activeVersionNumber: 1,
            title: 'status-webhooks',
            applicationCount: 1,
            scheduleState: 'Ready',
            nextEligibleAt: '2026-09-21T08:00:00Z',
            scheduleReason: 'Ready',
            updatedAt: '2026-09-21T08:00:00Z',
          },
        ],
        meta: meta(),
      }),
    ),
  )
}

function resourceTree() {
  return [
    {
      id: 'organization-1',
      name: 'default',
      version: 1,
      isDefault: true,
      canRename: true,
      canDelete: false,
      canCreateProject: true,
      projects: [
        {
          id: 'project-1',
          name: 'status-webhooks',
          canManage: true,
          environments: [
            {
              id: 'environment-1',
              name: 'production',
              type: 'Production',
              applications: [],
            },
          ],
        },
      ],
    },
  ]
}

function json(body: unknown) {
  return { contentType: 'application/json', body: JSON.stringify(body) }
}

function meta() {
  return { requestId: 'table-links-e2e', timestamp: '2026-09-21T08:00:00Z' }
}
