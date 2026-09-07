import { expect, test, type Page } from '@playwright/test'

const applicationID = '019c1230-0000-7000-8000-000000000010'

test.beforeEach(async ({ page }) => {
  await page.addInitScript(() =>
    localStorage.setItem('releasehub.language', 'zh-TW'),
  )
})

test('未登入使用者只會看到登入提示', async ({ page }) => {
  await page.route('**/api/v1/auth/session', (route) =>
    route.fulfill({ status: 401, contentType: 'application/json', body: '{}' }),
  )
  await page.goto('/access')
  await expect(page.getByText('需要登入')).toBeVisible()
  await expect(page.locator('a[href="/api/v1/auth/login"]')).toBeVisible()
})

test('授權使用者可檢視 onboarding 與設定漂移並送出驗證', async ({
  context,
  page,
}) => {
  await context.addCookies([
    {
      name: 'releasehub_csrf',
      value: 'csrf-token',
      domain: '127.0.0.1',
      path: '/',
    },
  ])
  await mockSession(page)
  await page.route(`**/api/v1/catalog/applications/${applicationID}`, (route) =>
    route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ data: application(), meta: meta() }),
    }),
  )
  await page.route(
    `**/api/v1/catalog/applications/${applicationID}/status`,
    (route) =>
      route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ data: applicationStatus(), meta: meta() }),
      }),
  )
  let dryRunCalled = false
  await page.route(
    `**/api/v1/catalog/applications/${applicationID}/onboarding/dry-run`,
    (route) => {
      dryRunCalled = route.request().headers()['x-csrf-token'] === 'csrf-token'
      return route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          data: {
            applicationId: applicationID,
            status: 'AwaitingConfirmation',
            version: 3,
            automatedSyncObserved: true,
            checks: [],
          },
          meta: meta(),
        }),
      })
    },
  )
  await page.goto(`/applications/${applicationID}`)
  await expect(page.getByText('偵測到設定漂移')).toBeVisible()
  await expect(page.getByText('automated_sync_enabled')).toBeVisible()
  await page.getByRole('button', { name: '驗證 onboarding' }).click()
  await expect.poll(() => dryRunCalled).toBe(true)
})

test('無平台權限時 candidate queue 不揭露資料', async ({ page }) => {
  await mockSession(page)
  await page.route('**/api/v1/argocd/candidates', (route) =>
    route.fulfill({ status: 404, contentType: 'application/json', body: '{}' }),
  )
  await page.goto('/candidates')
  await expect(page.getByText('找不到候選佇列')).toBeVisible()
})

async function mockSession(page: Page) {
  await page.route('**/api/v1/auth/session', (route) =>
    route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        data: {
          id: '019c1230-0000-7000-8000-000000000001',
          username: 'vincent',
        },
        meta: meta(),
      }),
    }),
  )
}

function application() {
  return {
    id: applicationID,
    organizationId: '019c1230-0000-7000-8000-000000000002',
    projectId: '019c1230-0000-7000-8000-000000000003',
    environmentId: '019c1230-0000-7000-8000-000000000004',
    name: 'payment-api',
    argocdNamespace: 'argocd',
    argocdApplicationName: 'payment-api-prod',
    argocdProject: 'payment',
    destinationServer: 'https://kubernetes.default.svc',
    destinationName: '',
    destinationNamespace: 'payment',
    sourceRepositoryUrl: 'https://example.invalid/gitops.git',
    sourceTargetRevision: 'main',
    sourcePath: 'production/payment-api',
    active: true,
    version: 1,
  }
}

function applicationStatus() {
  return {
    applicationId: applicationID,
    syncStatus: 'OutOfSync',
    healthStatus: 'Healthy',
    operationPhase: '',
    resolvedRevision: 'abc123',
    automatedSync: true,
    candidatePresent: true,
    onboardingStatus: 'AwaitingConfirmation',
    onboardingVersion: 2,
    driftReasons: ['automated_sync_enabled'],
    observedAt: new Date().toISOString(),
  }
}

function meta() {
  return { requestId: 'e2e', timestamp: new Date().toISOString() }
}
