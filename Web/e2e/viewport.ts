import { expect, type Page } from '@playwright/test'

export async function simulateScrollbarAvailableWidth(page: Page) {
  const viewport = page.viewportSize()
  if (!viewport) throw new Error('測試需要明確的 viewport 尺寸')
  // overlay 捲軸不縮減 viewport；以扣除 15px 的尺寸模擬可用寬度，非真實捲軸驗證。
  await page.setViewportSize({ ...viewport, width: viewport.width - 15 })
  await expect
    .poll(() => page.evaluate(() => document.documentElement.clientWidth))
    .toBe(viewport.width - 15)
}
