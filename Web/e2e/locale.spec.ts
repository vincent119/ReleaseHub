import { expect, test, type Page } from '@playwright/test'

const requestId = '019c1230-0000-7000-8000-000000000301'

test('Ant Design built-in copy follows the ReleaseHub language preference', async ({
  page,
}) => {
  await page.addInitScript(() => {
    localStorage.setItem('releasehub.language', 'en')
    localStorage.setItem('releasehub.theme', 'light')
    class TestEventSource {
      onmessage = null
      addEventListener() {}
      close() {}
    }
    Object.defineProperty(window, 'EventSource', { value: TestEventSource })
  })
  await mockApplication(page)

  await page.goto(`/requests/${requestId}`)
  const englishEmpty = page
    .locator('.ant-empty-description')
    .filter({ hasText: /^No data$/ })
  const traditionalChineseEmpty = page
    .locator('.ant-empty-description')
    .filter({ hasText: /^暫無資料$/ })
  await expect(englishEmpty).toHaveCount(2)

  await page
    .getByRole('button', { name: 'Open account menu for vincent' })
    .click()
  await page.getByRole('menuitem', { name: 'Theme settings' }).click()
  await page.getByText('繁體中文', { exact: true }).click()
  await expect(page.locator('html')).toHaveAttribute('lang', 'zh-TW')
  await expect(traditionalChineseEmpty).toHaveCount(2)
  await expect(englishEmpty).toHaveCount(0)

  await page.getByText('English', { exact: true }).click()
  await expect(page.locator('html')).toHaveAttribute('lang', 'en')
  await expect(englishEmpty).toHaveCount(2)
})

async function mockApplication(page: Page) {
  await page.route('**/api/v1/auth/session', (route) =>
    route.fulfill(
      json({
        data: {
          userId: '019c1230-0000-7000-8000-000000000302',
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
  await page.route('**/api/v1/release-workflows**', (route) =>
    route.fulfill(json({ data: [], meta: meta() })),
  )
  await page.route('**/api/v1/deployment-plans**', (route) =>
    route.fulfill(json({ data: [], meta: meta() })),
  )
  await page.route('**/api/v1/deployment-history**', (route) =>
    route.fulfill(json({ data: [], meta: meta() })),
  )
  await page.route(`**/api/v1/deployment-requests/${requestId}`, (route) =>
    route.fulfill(
      json({
        data: {
          id: '019c1230-0000-7000-8000-000000000303',
          requestId,
          organizationId: '019c1230-0000-7000-8000-000000000304',
          projectId: '019c1230-0000-7000-8000-000000000305',
          environmentId: '019c1230-0000-7000-8000-000000000306',
          versionNumber: 1,
          status: 'PendingReview',
          classification: 'Standard',
          fingerprint: 'locale-test',
          workflowVersionId: '019c1230-0000-7000-8000-000000000307',
          planVersionId: '019c1230-0000-7000-8000-000000000308',
          title: 'Locale verification',
          changeDescription: '',
          issueUrl: '',
          lockVersion: 1,
          capabilities: [],
          reviews: [],
          applications: [],
          createdAt: '2026-09-09T00:00:00Z',
        },
        meta: meta(),
      }),
    ),
  )
}

function json(body: unknown) {
  return { contentType: 'application/json', body: JSON.stringify(body) }
}

function meta() {
  return { requestId: 'locale-e2e', timestamp: new Date().toISOString() }
}
