import { expect, test, type Locator, type Page } from '@playwright/test'

import type {
  CatalogApplication,
  CatalogOrganizationNode,
  DeploymentPlan,
  DeploymentPlanVersion,
} from '../src/generated/model'

const longName = '長名稱搜尋專案-跨服務發布與治理-'.repeat(6)
const organizationA = 'organization-a'
const organizationB = 'organization-b'
const projectA = 'project-0'
const projectB = 'project-1'
const duplicateProject = 'project-other'
const environmentA = 'environment-0'
const environmentB = 'environment-1'
const globalA = 'global-0'
const routes = ['requests', 'plans', 'applications'] as const
type ScopePage = (typeof routes)[number]
type Theme = 'light' | 'dark'

declare global {
  interface Window {
    scopeRuntimeErrors: string[]
  }
}

test.beforeEach(async ({ page }) => {
  await page.addInitScript(() => {
    const errors: string[] = []
    Object.assign(window, { scopeRuntimeErrors: errors })
    // 原生尺寸通知不帶 Error 物件，僅監聽 pageerror 會漏報。
    window.addEventListener('error', (event) => errors.push(event.message))
  })
})

test.afterEach(async ({ page }) => {
  const errors = await page.evaluate(async () => {
    // 只在流程完成後收集待送達通知，不插入操作間的固定等待。
    await new Promise<void>((resolve) =>
      requestAnimationFrame(() => requestAnimationFrame(() => resolve())),
    )
    return window.scopeRuntimeErrors
  })
  expect(errors).toEqual([])
})

for (const path of routes) {
  for (const theme of ['light', 'dark'] as const) {
    for (const width of [320, 768, 1440]) {
      test(`${path} ${theme} ${width}px：分層搜尋、長名稱、鍵盤、主題與不溢位`, async ({
        page,
      }, testInfo) => {
        await page.setViewportSize({ width, height: 900 })
        await prepare(page, theme)
        await page.goto(`/${path}`)
        const organization = page.getByRole('combobox', {
          name: '組織',
          exact: true,
        })
        const project = page.getByRole('combobox', {
          name: 'Project',
          exact: true,
        })
        const environment = page.getByRole('combobox', {
          name: environmentLabel(path),
          exact: true,
        })
        await expect(organization).toBeVisible()
        await expect(project).toBeDisabled()
        await expect(environment).toBeDisabled()
        await expect(page.locator('html')).toHaveAttribute('data-theme', theme)
        await assertNoOverflow(page)
        const widths = await scopeWidths([organization, project, environment])
        await expect(
          page.getByRole('button', { name: /查看 .* 完整名稱/ }),
        ).toHaveCount(0)

        await tabTo(page, organization)
        await assertFocus(organization)
        await organization.press('ArrowDown')
        await expect(page.getByRole('listbox')).toBeVisible()
        await organization.press('Enter')
        await expect(project).toBeEnabled()
        await project.focus()
        await assertFocus(project)
        await project.press('ArrowDown')
        await expect(page.getByRole('option')).toHaveCount(50)
        await assertPopup(page)
        await project.fill('不存在的專案搜尋')
        await expect(page.getByText('沒有符合搜尋條件的選項。')).toBeVisible()
        await project.press('Escape')
        await expect(project).toHaveAttribute('aria-expanded', 'false')
        expect(await scopeWidths([organization, project, environment])).toEqual(
          widths,
        )
        await expect(project).toBeFocused()

        await project.fill('長名稱搜尋')
        const option = page.getByRole('option', { name: longName, exact: true })
        await expect(option).toBeVisible()
        await expect(page.getByRole('option')).toHaveCount(1)
        await assertPopup(page)
        await assertReadable(option)
        await project.press('ArrowDown')
        await project.press('Enter')
        await expect(environment).toBeEnabled()
        await expect(project).toHaveAttribute('aria-expanded', 'false')
        expect(await scopeWidths([organization, project, environment])).toEqual(
          widths,
        )
        await project.press('Tab')
        const nameButton = page.getByRole('button', {
          name: '查看 Project 完整名稱',
          exact: true,
        })
        // Ant Design 的清除按鈕也可由鍵盤抵達，不依賴固定 Tab 次數。
        await tabTo(page, nameButton)
        await expect(nameButton).toHaveCSS('outline-style', 'solid')
        await expect(nameButton).not.toHaveCSS('outline-width', '0px')
        await nameButton.press('Enter')
        const fullName = page
          .locator('[data-scope-full-name]')
          .filter({ hasText: longName })
        await expect(fullName).toBeVisible()
        await expect(fullName).toHaveText(longName)
        await assertInsideViewport(fullName, width)
        await assertNoOverflow(page)
        await nameButton.press('Enter')
        await expect(fullName).toBeHidden()
        await environment.focus()
        await environment.press('ArrowDown')
        await environment.press('ArrowDown')
        await environment.press('Enter')
        await expect(environment).toHaveAttribute('aria-expanded', 'false')
        expect(await scopeWidths([organization, project, environment])).toEqual(
          widths,
        )
        await assertNoOverflow(page)

        if (width === 320) {
          const boxes = await Promise.all(
            [organization, project, environment].map((input) =>
              input.evaluate((element) => {
                const box = element
                  .closest('.ant-select')!
                  .getBoundingClientRect()
                return { top: box.top, width: box.width }
              }),
            ),
          )
          expect(boxes[1].top).toBeGreaterThan(boxes[0].top)
          expect(boxes[2].top).toBeGreaterThan(boxes[1].top)
          expect(boxes.every((box) => box.width >= 180)).toBe(true)
        }
        await page.screenshot({
          path: testInfo.outputPath('scope.png'),
          fullPage: true,
        })
        await organization
          .locator(
            'xpath=ancestor::div[contains(concat(" ", normalize-space(@class), " "), " ant-select ")][1]',
          )
          .getByRole('button', { name: 'Clear', exact: true })
          .click()
        await expect(project).toBeDisabled()
        await expect(environment).toBeDisabled()
        expect(await scopeWidths([organization, project, environment])).toEqual(
          widths,
        )
        await expect(
          page.getByRole('button', { name: /查看 .* 完整名稱/ }),
        ).toHaveCount(0)
      })
    }
  }
}

for (const path of routes) {
  test(`${path}：單組織省略選單並保留各頁查詢 gate 與 Global ID`, async ({
    page,
  }) => {
    const state = await prepare(page, 'light', { singleOrganization: true })
    await page.goto(`/${path}`)
    await expect(
      page.getByRole('combobox', { name: '組織', exact: true }),
    ).toHaveCount(0)
    await expect(
      page.getByRole('combobox', { name: 'Project', exact: true }),
    ).toBeEnabled()
    expect(state.requests).toEqual([])
    expect(state.plans).toEqual([])
    if (path === 'applications')
      await expectApplicationList(page, [
        '應用 A',
        '應用 Global',
        '應用 B',
        '另一組織應用',
      ])
    await choose(page, 'Project', longName)
    if (path === 'requests') {
      expect(state.requests).toEqual([])
      await expect(
        page.getByText('請先選擇要查看的 Environment。'),
      ).toBeVisible()
    }
    if (path === 'plans')
      await expect.poll(() => state.plans).toEqual([{ projectId: projectA }])
    await choose(page, environmentLabel(path), 'Global')
    if (path === 'requests') {
      await expect
        .poll(() => state.requests)
        .toEqual([
          {
            organizationId: organizationA,
            projectId: projectA,
            environmentId: globalA,
            limit: '20',
          },
        ])
      await expect(
        page.getByRole('link', { name: '申請 Global' }),
      ).toBeVisible()
    }
    if (path === 'plans') {
      expect(state.plans).toEqual([{ projectId: projectA }])
      await expect.poll(() => state.schedules).toContain(globalA)
      await expect
        .poll(() => state.bindings)
        .toContainEqual({
          organizationId: organizationA,
          projectId: projectA,
          environmentId: globalA,
        })
    }
    if (path === 'applications') {
      await expectApplicationList(page, ['應用 Global'])
      await page.getByRole('button', { name: '清除篩選', exact: true }).click()
      await expectApplicationList(page, [
        '應用 A',
        '應用 Global',
        '應用 B',
        '另一組織應用',
      ])
      expect(state.visibleQueries.length).toBeGreaterThan(0)
      expect(state.visibleQueries.every((query) => query === '')).toBe(true)
    }
  })

  test(`${path}：切換組織隔離同名 Project，Project 切換清除舊環境與結果`, async ({
    page,
  }) => {
    const state = await prepare(page, 'dark')
    await page.goto(`/${path}`)
    await choose(page, '組織', '第一組織')
    await choose(page, 'Project', longName)
    await choose(page, environmentLabel(path), 'Production')
    if (path === 'requests')
      await expect(page.getByRole('link', { name: '申請 A' })).toBeVisible()
    if (path === 'plans')
      await expect(page.getByRole('button', { name: /Plan A/ })).toBeVisible()
    if (path === 'applications') await expectApplicationList(page, ['應用 A'])
    await choose(page, 'Project', '另一個專案')
    const environment = page.getByRole('combobox', {
      name: environmentLabel(path),
      exact: true,
    })
    await expect(environment).toHaveValue('')
    if (path === 'requests') {
      await expect(page.getByRole('link', { name: '申請 A' })).toHaveCount(0)
      expect(state.requests).toHaveLength(1)
    }
    if (path === 'plans') {
      await expect(page.getByRole('button', { name: /Plan A/ })).toHaveCount(0)
      await expect(page.getByRole('button', { name: /Plan B/ })).toBeVisible()
    }
    if (path === 'applications') await expectApplicationList(page, ['應用 B'])
    await choose(page, '組織', '另一組織')
    await expect(environment).toBeDisabled()
    await choose(page, 'Project', longName)
    await choose(page, environmentLabel(path), 'Production')
    if (path === 'requests')
      await expect
        .poll(() => state.requests.at(-1))
        .toEqual({
          organizationId: organizationB,
          projectId: duplicateProject,
          environmentId: 'environment-other',
          limit: '20',
        })
    if (path === 'plans')
      await expect
        .poll(() => state.plans.at(-1))
        .toEqual({ projectId: duplicateProject })
    if (path === 'applications')
      await expectApplicationList(page, ['另一組織應用'])
  })

  test(`${path}：頁內切換主題時範圍、選取與焦點樣式不遺失`, async ({
    page,
  }) => {
    await prepare(page, 'light', { singleOrganization: true })
    await page.goto(`/${path}`)
    await choose(page, 'Project', longName)
    await page.getByRole('button', { name: '開啟 tester 的帳號選單' }).click()
    await page.getByRole('menuitem', { name: '主題設定', exact: true }).click()
    await page.getByRole('combobox', { name: '主題', exact: true }).click()
    await page
      .locator('.ant-select-dropdown:visible')
      .getByText('深色', { exact: true })
      .click()
    await expect(page.locator('html')).toHaveAttribute('data-theme', 'dark')
    await page.getByRole('button', { name: '開啟 tester 的帳號選單' }).click()
    await page.getByRole('combobox', { name: 'Project', exact: true }).focus()
    await assertFocus(
      page.getByRole('combobox', { name: 'Project', exact: true }),
    )
    await expect(
      page.getByRole('button', { name: '查看 Project 完整名稱', exact: true }),
    ).toBeVisible()
    await expect(
      page.getByRole('combobox', { name: environmentLabel(path), exact: true }),
    ).toBeEnabled()
  })
}

test('Plans：切換 Project 清除舊版本與 binding／schedule context', async ({
  page,
}) => {
  const state = await prepare(page, 'dark', {
    singleOrganization: true,
    versionedPlans: true,
  })
  await page.goto('/plans')
  await choose(page, 'Project', longName)
  const actions = page.locator('.ant-card-extra')
  const version = actions.getByRole('combobox')
  await expect(
    actions.getByTitle('v2 · Published', { exact: true }),
  ).toBeVisible()
  await version.click()
  await page
    .locator('.ant-select-dropdown:visible')
    .getByText('v1 · Published', { exact: true })
    .click()
  await expect(
    actions.getByTitle('v1 · Published', { exact: true }),
  ).toBeVisible()
  await choose(page, '環境（選填）', 'Global')
  await expect.poll(() => state.schedules.at(-1)).toBe(globalA)
  await choose(page, 'Project', '另一個專案')
  await expect(
    actions.getByTitle('v2 · Published', { exact: true }),
  ).toBeVisible()
  await expect(
    page.getByText('Environment Binding', { exact: true }),
  ).toHaveCount(0)
  await choose(page, '環境（選填）', 'Production')
  await expect
    .poll(() => state.bindings.at(-1))
    .toEqual({
      organizationId: organizationA,
      projectId: projectB,
      environmentId: environmentB,
    })
  await expect.poll(() => state.schedules.at(-1)).toBe(environmentB)
  await assertPlanGraph(page, `${projectB}-v2`)
})

for (const theme of ['light', 'dark'] as const) {
  for (const width of [320, 768, 1440]) {
    test(`Plans ${theme} ${width}px：版本切換後圖形可見且位於可量測畫布`, async ({
      page,
    }, testInfo) => {
      await page.setViewportSize({ width, height: 900 })
      await prepare(page, theme, {
        singleOrganization: true,
        versionedPlans: true,
      })
      await page.goto('/plans')
      await choose(page, 'Project', longName)
      await assertPlanGraph(page, `${projectA}-v2`)
      const version = page.locator('.ant-card-extra').getByRole('combobox')
      await version.click()
      await page
        .locator('.ant-select-dropdown:visible')
        .getByText('v1 · Published', { exact: true })
        .click()
      await assertPlanGraph(page, `${projectA}-v1`)
      await choose(page, 'Project', '另一個專案')
      await assertPlanGraph(page, `${projectB}-v2`)
      await assertNoOverflow(page)
      await page.screenshot({
        path: testInfo.outputPath('plan-graph.png'),
        fullPage: true,
      })
    })
  }
}

async function assertPlanGraph(page: Page, nodeID: string) {
  const graph = page.locator('.react-flow')
  await expect(graph).toBeVisible()
  await expect(graph.locator('.react-flow__node')).toHaveCount(1)
  const node = graph.locator(`.react-flow__node[data-id="${nodeID}"]`)
  await expect(node).toBeVisible()
  const bounds = await graph.boundingBox()
  expect(bounds?.width).toBeGreaterThan(0)
  expect(bounds?.height).toBeGreaterThan(0)
  await expect
    .poll(() =>
      node.evaluate((element) => {
        const node = element.getBoundingClientRect()
        const canvas = element.closest('.react-flow')!.getBoundingClientRect()
        return (
          node.left >= canvas.left &&
          node.right <= canvas.right &&
          node.top >= canvas.top &&
          node.bottom <= canvas.bottom
        )
      }),
    )
    .toBe(true)
}

async function scopeWidths(inputs: Locator[]) {
  return Promise.all(
    inputs.map((input) =>
      input.evaluate((element) =>
        Math.round(
          element.closest('.ant-select')!.getBoundingClientRect().width,
        ),
      ),
    ),
  )
}

test('Applications：tree 失敗只局部降級，重試可恢復分層篩選', async ({
  page,
}) => {
  const state = await prepare(page, 'dark', { treeUnavailable: true })
  await page.goto('/applications')
  await expect(page.getByText('無法載入範圍選項。')).toBeVisible()
  await expectApplicationList(page, [
    '應用 A',
    '應用 Global',
    '應用 B',
    '另一組織應用',
  ])
  state.treeUnavailable = false
  await page.getByRole('button', { name: '重試範圍選項' }).click()
  await expect(page.getByText('無法載入範圍選項。')).toBeHidden()
  await choose(page, '組織', '第一組織')
  await choose(page, 'Project', longName)
  await expectApplicationList(page, ['應用 A', '應用 Global'])
})

test('Requests：延遲回傳的舊 scope 不能取代新 scope 結果', async ({ page }) => {
  const state = await prepare(page, 'light', { singleOrganization: true })
  let release: () => void = () => {}
  const delayed = new Promise<void>((resolve) => {
    release = resolve
  })
  state.delayRequestA = delayed
  await page.goto('/requests')
  await choose(page, 'Project', longName)
  await choose(page, '環境（必填）', 'Production')
  await expect.poll(() => state.requests.length).toBe(1)
  await choose(page, 'Project', '另一個專案')
  await choose(page, '環境（必填）', 'Production')
  await expect(page.getByRole('link', { name: '申請 B' })).toBeVisible()
  release()
  await expect.poll(() => state.delayedResponded).toBe(true)
  await expect(page.getByRole('link', { name: '申請 B' })).toBeVisible()
  await expect(page.getByRole('link', { name: '申請 A' })).toHaveCount(0)
})

function environmentLabel(path: ScopePage) {
  return path === 'requests' ? '環境（必填）' : '環境（選填）'
}

async function choose(page: Page, label: string, name: string) {
  await page.getByRole('combobox', { name: label, exact: true }).click()
  await page.getByRole('option', { name, exact: true }).click()
}

async function tabTo(page: Page, target: Locator) {
  for (let index = 0; index < 30; index++) {
    if (await target.evaluate((element) => document.activeElement === element))
      return
    await page.keyboard.press('Tab')
  }
  await expect(target).toBeFocused()
}

async function assertFocus(input: Locator) {
  await expect(input).toBeFocused()
  const focus = await input.evaluate((element) => {
    const control = element.closest('.ant-select')!
    const style = getComputedStyle(control)
    return {
      focused: control.classList.contains('ant-select-focused'),
      shadow: style.boxShadow,
      border: style.borderColor,
    }
  })
  expect(focus.focused).toBe(true)
  expect(focus.shadow).not.toBe('none')
}

async function assertNoOverflow(page: Page) {
  const dimensions = await page.evaluate(() => ({
    width: document.documentElement.clientWidth,
    scroll: document.documentElement.scrollWidth,
  }))
  expect(dimensions.scroll).toBeLessThanOrEqual(dimensions.width + 1)
  for (const input of await page.getByRole('combobox').all()) {
    if (!(await input.isVisible())) continue
    const box = await input.evaluate((element) => {
      const rect = element.closest('.ant-select')!.getBoundingClientRect()
      return { left: rect.left, right: rect.right }
    })
    expect(box.left).toBeGreaterThanOrEqual(0)
    expect(box.right).toBeLessThanOrEqual(dimensions.width + 1)
  }
}

async function assertInsideViewport(locator: Locator, width: number) {
  const box = await locator.boundingBox()
  expect(box).not.toBeNull()
  expect(box!.x).toBeGreaterThanOrEqual(0)
  expect(box!.x + box!.width).toBeLessThanOrEqual(width + 1)
}

async function assertPopup(page: Page) {
  const popup = page.locator('.ant-select-dropdown:visible')
  await expect(popup).toBeVisible()
  await assertInsideViewport(popup, page.viewportSize()!.width)
  const overflow = await popup.evaluate(
    (element) => element.scrollWidth - element.clientWidth,
  )
  expect(overflow).toBeLessThanOrEqual(1)
}

async function assertReadable(option: Locator) {
  const colors = await option.evaluate((element) => ({
    text: getComputedStyle(element).color,
    background: getComputedStyle(element.closest('.ant-select-dropdown')!)
      .backgroundColor,
  }))
  function luminance(color: string) {
    const channels = color
      .match(/[\d.]+/g)!
      .slice(0, 3)
      .map((value) => {
        const channel = Number(value) / 255
        return channel <= 0.04045
          ? channel / 12.92
          : ((channel + 0.055) / 1.055) ** 2.4
      })
    return channels[0] * 0.2126 + channels[1] * 0.7152 + channels[2] * 0.0722
  }
  const values = [luminance(colors.text), luminance(colors.background)].sort(
    (a, b) => b - a,
  )
  expect((values[0] + 0.05) / (values[1] + 0.05)).toBeGreaterThanOrEqual(4.5)
}

async function expectApplicationList(page: Page, names: string[]) {
  const links = page.locator('a[href^="/applications/"]')
  await expect(links).toHaveText(names)
  await expect(
    page.getByRole('link', { name: '僅在 tree 的應用' }),
  ).toHaveCount(0)
}

interface MockOptions {
  singleOrganization?: boolean
  treeUnavailable?: boolean
  versionedPlans?: boolean
}

interface MockState {
  requests: Record<string, string>[]
  plans: Record<string, string>[]
  bindings: Record<string, string>[]
  schedules: string[]
  visibleQueries: string[]
  treeUnavailable: boolean
  delayRequestA?: Promise<void>
  delayedResponded: boolean
}

async function prepare(page: Page, theme: Theme, options: MockOptions = {}) {
  const state: MockState = {
    requests: [],
    plans: [],
    bindings: [],
    schedules: [],
    visibleQueries: [],
    treeUnavailable: options.treeUnavailable ?? false,
    delayedResponded: false,
  }
  await page.addInitScript((value) => {
    localStorage.setItem('releasehub.language', 'zh-TW')
    localStorage.setItem('releasehub.theme', value)
    class TestEventSource {
      onmessage = null
      addEventListener() {}
      close() {}
    }
    Object.defineProperty(window, 'EventSource', { value: TestEventSource })
  }, theme)
  await page.route('**/api/v1/**', async (route) => {
    const url = new URL(route.request().url())
    const path = url.pathname.replace('/api/v1', '')
    const params = Object.fromEntries(url.searchParams)
    if (path === '/auth/session')
      return route.fulfill(
        json({
          userId: 'user-test',
          username: 'tester',
          mustChangePassword: false,
          passwordChangeAvailable: false,
        }),
      )
    if (path === '/audit/capabilities')
      return route.fulfill(json({ visible: false, scopeRoots: [] }))
    if (path === '/notifications' || path === '/release-workflows')
      return route.fulfill(json([]))
    if (path === '/catalog/resource-tree')
      return route.fulfill(
        state.treeUnavailable
          ? { status: 503, contentType: 'application/json', body: '{}' }
          : json(resourceTree(options.singleOrganization)),
      )
    if (path === '/catalog/visible-applications') {
      state.visibleQueries.push(url.search)
      return route.fulfill(json(applications))
    }
    if (path === '/deployment-requests') {
      state.requests.push(params)
      if (params.projectId === projectA && state.delayRequestA) {
        await state.delayRequestA
        state.delayedResponded = true
      }
      const name =
        params.environmentId === globalA
          ? 'Global'
          : params.projectId === projectA
            ? 'A'
            : 'B'
      return route.fulfill(
        json(
          [
            {
              id: `request-${name}`,
              ...params,
              title: `申請 ${name}`,
              status: 'Succeeded',
              classification: 'Standard',
              activeVersionNumber: 1,
              applicationCount: 1,
              scheduleState: 'Ready',
              nextEligibleAt: '2026-10-01T00:00:00Z',
              scheduleReason: 'Ready',
              updatedAt: '2026-10-01T00:00:00Z',
            },
          ],
          true,
        ),
      )
    }
    if (path === '/deployment-plans') {
      state.plans.push(params)
      return route.fulfill(
        json([plan(params.projectId, options.versionedPlans)]),
      )
    }
    if (path === '/deployment-bindings') {
      state.bindings.push(params)
      return route.fulfill(json(null))
    }
    if (path.startsWith('/deployment-schedules/')) {
      const environmentId = path.split('/').at(-1)!
      state.schedules.push(environmentId)
      return route.fulfill(
        json({
          environmentId,
          enabled: false,
          timeZone: 'UTC',
          weeklyWindows: [],
          blackouts: [],
          version: 0,
          canManage: false,
        }),
      )
    }
    return route.fulfill({
      status: 404,
      contentType: 'application/json',
      body: '{}',
    })
  })
  return state
}

function json(data: unknown, cursorPage = false) {
  return {
    contentType: 'application/json',
    body: JSON.stringify({
      data,
      meta: {
        requestId: 'scope-e2e',
        timestamp: '2026-10-01T00:00:00Z',
        ...(cursorPage ? { hasMore: false } : {}),
      },
    }),
  }
}

function resourceTree(singleOrganization = false): CatalogOrganizationNode[] {
  const organization: CatalogOrganizationNode = {
    id: organizationA,
    name: '第一組織',
    version: 1,
    isDefault: true,
    canRename: false,
    canDelete: false,
    canCreateProject: false,
    projects: Array.from({ length: 50 }, (_, index) => ({
      id: `project-${index}`,
      name:
        index === 0
          ? longName
          : index === 1
            ? '另一個專案'
            : `搜尋專案 ${index}`,
      canManage: false,
      environments: [
        {
          id: `environment-${index}`,
          name: 'Production',
          type: 'Production',
          applications:
            index === 0
              ? [
                  application(
                    '僅在 tree 的應用',
                    organizationA,
                    projectA,
                    environmentA,
                  ),
                ]
              : [],
        },
        {
          id: `global-${index}`,
          name: 'Global',
          type: 'Production',
          applications: [],
        },
      ],
    })),
  }
  return singleOrganization
    ? [organization]
    : [
        organization,
        {
          ...organization,
          id: organizationB,
          name: '另一組織',
          isDefault: false,
          projects: [
            {
              id: duplicateProject,
              name: longName,
              canManage: false,
              environments: [
                {
                  id: 'environment-other',
                  name: 'Production',
                  type: 'Production',
                  applications: [],
                },
              ],
            },
          ],
        },
      ]
}

function application(
  name: string,
  organizationId: string,
  projectId: string,
  environmentId: string,
): CatalogApplication {
  return {
    id: name,
    name,
    organizationId,
    projectId,
    environmentId,
    argocdNamespace: 'argocd',
    argocdApplicationName: name,
    argocdProject: '與 Catalog 不同的 Project',
    destinationServer: 'https://kubernetes.default.svc',
    destinationNamespace: 'test',
    sourceRepositoryUrl: 'https://example.invalid/test',
    sourceTargetRevision: 'main',
    sourcePath: 'test',
    active: true,
    version: 1,
  }
}

const applications = [
  application('應用 A', organizationA, projectA, environmentA),
  application('應用 Global', organizationA, projectA, globalA),
  application('應用 B', organizationA, projectB, environmentB),
  application(
    '另一組織應用',
    organizationB,
    duplicateProject,
    'environment-other',
  ),
]

function plan(projectId: string, versioned = false): DeploymentPlan {
  return {
    id: `plan-${projectId}`,
    ownerKind: 'project',
    ownerProjectId: projectId,
    name: projectId === projectA ? 'Plan A' : 'Plan B',
    description: '',
    active: true,
    versions: versioned
      ? Array.from({ length: 2 }, (_, index): DeploymentPlanVersion => ({
          id: `${projectId}-version-${index + 1}`,
          planId: `plan-${projectId}`,
          versionNumber: index + 1,
          lifecycle: 'Published',
          lockVersion: 1,
          createdAt: '2026-10-01T00:00:00Z',
          publishedAt: '2026-10-01T00:00:00Z',
          document: {
            nodes: [
              {
                key: `${projectId}-v${index + 1}`,
                applicationKey: 'application_a',
                order: 0,
                successCondition: {
                  syncStatuses: ['Synced'],
                  healthStatuses: ['Healthy'],
                  stabilizationSeconds: 0,
                },
              },
            ],
            edges: [],
          },
        }))
      : [],
  }
}
