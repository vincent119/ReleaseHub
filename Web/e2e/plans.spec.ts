import { expect, test, type Page } from '@playwright/test'

const ids = {
  organization: '019c1230-0000-7000-8000-000000000101',
  project: '019c1230-0000-7000-8000-000000000102',
  environment: '019c1230-0000-7000-8000-000000000103',
  plan: '019c1230-0000-7000-8000-000000000104',
  planVersion: '019c1230-0000-7000-8000-000000000105',
  workflow: '019c1230-0000-7000-8000-000000000106',
  workflowVersion: '019c1230-0000-7000-8000-000000000107',
}

test('建立 A/C 到 B 的 Plan 並綁定已發布版本', async ({ context, page }) => {
  await context.addCookies([
    {
      name: 'releasehub_csrf',
      value: 'csrf-token',
      domain: '127.0.0.1',
      path: '/',
    },
  ])
  await page.addInitScript(() =>
    localStorage.setItem('releasehub.language', 'zh-TW'),
  )
  await mockSession(page)
  await mockResources(page)
  await mockWorkflows(page)
  let plan = null as Record<string, unknown> | null
  let bindingBody = null as Record<string, unknown> | null
  await page.route('**/api/v1/deployment-plans**', async (route) => {
    const request = route.request()
    const url = new URL(request.url())
    if (request.method() === 'GET')
      return route.fulfill(json({ data: plan ? [plan] : [], meta: meta() }))
    if (url.pathname.endsWith('/lifecycle')) {
      const version = (plan!.versions as Record<string, unknown>[])[0]
      version.lifecycle = 'Published'
      version.lockVersion = 2
      return route.fulfill(json({ data: version, meta: meta() }))
    }
    const body = request.postDataJSON()
    plan = planResponse(body)
    return route.fulfill({ ...json({ data: plan, meta: meta() }), status: 201 })
  })
  await page.route('**/api/v1/deployment-bindings**', async (route) => {
    if (route.request().method() === 'GET')
      return route.fulfill(json({ data: null, meta: meta() }))
    bindingBody = route.request().postDataJSON()
    return route.fulfill(
      json({
        data: { id: 'binding-1', ...bindingBody, version: 1 },
        meta: meta(),
      }),
    )
  })

  await page.goto('/plans')
  await page.getByLabel('選擇 Project').click()
  await page.getByText('Organization A / Project A').click()
  await page.getByRole('button', { name: '建立 Plan' }).click()
  await page.getByRole('button', { name: '新增 Application' }).click()
  await page.getByRole('button', { name: '新增 Application' }).click()
  await addDependency(page, 'application_1', 'application_2')
  await addDependency(page, 'application_3', 'application_2')
  await page.getByRole('button', { name: '儲存 Plan' }).click()
  await expect(page.getByRole('button', { name: '發布版本' })).toBeVisible()

  const document = (plan!.versions as Record<string, unknown>[])[0]
    .document as Record<string, unknown>
  expect(document.edges).toEqual([
    {
      from: 'application_1',
      to: 'application_2',
      condition: 'UpstreamSucceeded',
    },
    {
      from: 'application_3',
      to: 'application_2',
      condition: 'UpstreamSucceeded',
    },
  ])
  await page.getByRole('button', { name: '發布版本' }).click()
  await page.getByLabel('選擇 Environment').click()
  await page.getByText('production', { exact: true }).click()
  await page.getByLabel('Release Workflow Version').click()
  await page.getByText('Production approval · v1').click()
  await page.getByLabel('Deployment Plan Version').click()
  await page.getByText('Production plan · v1').click()
  await page.getByRole('button', { name: '儲存 Binding' }).click()
  await expect
    .poll(() => bindingBody)
    .toMatchObject({
      organizationId: ids.organization,
      projectId: ids.project,
      environmentId: ids.environment,
      workflowVersionId: ids.workflowVersion,
      planVersionId: ids.planVersion,
      expectedVersion: 0,
    })
})

async function addDependency(page: Page, source: string, target: string) {
  await page.getByRole('button', { name: '新增相依關係' }).click()
  const dialog = page.getByRole('dialog', {
    name: '新增 Application 相依關係',
  })
  await dialog.getByLabel('上游 Node').click()
  await page.getByText(source, { exact: true }).last().click()
  await dialog.getByLabel('下游 Node').click()
  await page.getByText(target, { exact: true }).last().click()
  await dialog.getByRole('button', { name: '新增相依關係' }).click()
  await expect(dialog).toBeHidden()
}

async function mockSession(page: Page) {
  await page.route('**/api/v1/auth/session', (route) =>
    route.fulfill(
      json({
        data: {
          id: '019c1230-0000-7000-8000-000000000001',
          username: 'vincent',
        },
        meta: meta(),
      }),
    ),
  )
}

async function mockResources(page: Page) {
  await page.route('**/api/v1/catalog/resource-tree', (route) =>
    route.fulfill(
      json({
        data: [
          {
            id: ids.organization,
            name: 'Organization A',
            canCreateProject: false,
            projects: [
              {
                id: ids.project,
                name: 'Project A',
                canManage: true,
                environments: [
                  {
                    id: ids.environment,
                    name: 'production',
                    type: 'Production',
                    applications: [],
                  },
                ],
              },
            ],
          },
        ],
        canCreateOrganization: false,
        meta: meta(),
      }),
    ),
  )
}

async function mockWorkflows(page: Page) {
  await page.route('**/api/v1/release-workflows', (route) =>
    route.fulfill(
      json({
        data: [
          {
            id: ids.workflow,
            name: 'Production approval',
            description: '',
            active: true,
            versions: [
              {
                id: ids.workflowVersion,
                workflowId: ids.workflow,
                versionNumber: 1,
                lifecycle: 'Published',
                lockVersion: 2,
                createdAt: '2026-09-07T00:00:00Z',
                document: {
                  initialState: 'done',
                  states: [{ key: 'done', name: 'Done', type: 'Terminal' }],
                  transitions: [],
                },
              },
            ],
          },
        ],
        meta: meta(),
      }),
    ),
  )
}

function planResponse(body: Record<string, unknown>) {
  return {
    id: ids.plan,
    ownerKind: body.ownerKind,
    ownerProjectId: body.ownerProjectId,
    name: body.name,
    description: body.description,
    active: true,
    versions: [
      {
        id: ids.planVersion,
        planId: ids.plan,
        versionNumber: 1,
        lifecycle: 'Draft',
        document: body.document,
        lockVersion: 1,
        createdAt: '2026-09-07T00:00:00Z',
      },
    ],
  }
}

function json(body: unknown) {
  return { contentType: 'application/json', body: JSON.stringify(body) }
}

function meta() {
  return { requestId: 'e2e', timestamp: new Date().toISOString() }
}
