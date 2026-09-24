import { expect, test } from '@playwright/test'

test('舊版 lazy route chunk 失效後保留 App shell 且不循環重新載入', async ({
  page,
}) => {
  await page.addInitScript(() => {
    localStorage.setItem('releasehub.language', 'zh-TW')
  })
  await page.route('**/api/v1/auth/session', (route) =>
    route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        data: { id: 'user-1', username: 'vincent' },
        meta: { requestId: 'test', timestamp: '2026-09-24T00:00:00Z' },
      }),
    }),
  )
  await page.route('**/api/v1/notifications**', (route) =>
    route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ data: [], meta: {} }),
    }),
  )
  let failedChunkRequests = 0
  await page.route('**/src/features/requests/index.ts*', (route) => {
    failedChunkRequests += 1
    return route.fulfill({
      status: 404,
      contentType: 'application/javascript',
      body: '',
    })
  })
  let navigations = 0
  page.on('request', (request) => {
    if (request.isNavigationRequest()) navigations += 1
  })

  await page.goto('/')
  await page.getByRole('link', { name: 'Deployment Requests' }).click()

  await expect(page.getByText('ReleaseHub 已更新')).toBeVisible()
  await expect(page.getByRole('button', { name: '重新載入' })).toBeVisible()
  await expect(
    page.getByRole('link', { name: 'Deployment Requests' }),
  ).toBeVisible()
  expect(failedChunkRequests).toBeGreaterThanOrEqual(2)
  expect(navigations).toBe(2)

  await page.waitForTimeout(1200)
  expect(navigations).toBe(2)
})
