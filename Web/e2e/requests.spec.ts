import { expect, test, type Page } from '@playwright/test'

import type { DeploymentRequestVersion } from '../src/generated/model'

const ids = {
  organization: '019c1230-0000-7000-8000-000000000201',
  project: '019c1230-0000-7000-8000-000000000202',
  environment: '019c1230-0000-7000-8000-000000000203',
  request: '019c1230-0000-7000-8000-000000000204',
  version: '019c1230-0000-7000-8000-000000000205',
  workflow: '019c1230-0000-7000-8000-000000000206',
  workflowVersion: '019c1230-0000-7000-8000-000000000207',
  plan: '019c1230-0000-7000-8000-000000000208',
  planVersion: '019c1230-0000-7000-8000-000000000209',
  execution: '019c1230-0000-7000-8000-000000000210',
  appA: '019c1230-0000-7000-8000-000000000211',
  appB: '019c1230-0000-7000-8000-000000000212',
}

test.beforeEach(async ({ context, page }) => {
  await context.addCookies([
    {
      name: 'releasehub_csrf',
      value: 'csrf-token',
      domain: '127.0.0.1',
      path: '/',
    },
  ])
  await page.addInitScript(() => {
    localStorage.setItem('releasehub.language', 'zh-TW')
    class TestEventSource {
      onmessage = null
      addEventListener() {}
      close() {}
    }
    Object.defineProperty(window, 'EventSource', { value: TestEventSource })
  })
})

test('自動 Request 經審核與部署後呈現 DAG 及 Partial Failed', async ({
  page,
}) => {
  const state = { request: requestFixture(), retryBody: undefined as unknown }
  await mockApplication(page, state)

  await page.goto('/requests')
  await page.getByLabel('選擇 Request scope').click()
  await page.getByText('Organization A / Project A / production').click()
  await page.getByRole('link', { name: 'Automatic payment deployment' }).click()
  await expect(page.getByRole('button', { name: '核准申請' })).toBeVisible()

  await page.getByRole('button', { name: '核准申請' }).click()
  await page.getByRole('button', { name: 'deploy' }).click()
  await expect(page.getByText('Partial Failed').first()).toBeVisible()
  await expect(page.getByText('等待：app-a')).toBeVisible()
  await expect(page.getByText('Synced · Healthy').first()).toBeVisible()
  await expect(page.getByText('sync failed')).toBeVisible()

  await page.getByLabel('選取 app-b 進行重試').check()
  await page.getByRole('button', { name: '重試選取的失敗項目' }).click()
  await expect
    .poll(() => state.retryBody)
    .toMatchObject({
      applicationIds: [ids.appB],
      expectedVersion: 1,
    })
})

test('Forward Rollback 仍顯示正常審核，Superseded 版本不可操作', async ({
  page,
}) => {
  const request = requestFixture()
  request.classification = 'ForwardRollback'
  const state = { request, retryBody: undefined as unknown }
  await mockApplication(page, state)

  await page.goto(`/requests/${ids.request}`)
  await expect(page.getByText('Forward Rollback').first()).toBeVisible()
  await expect(page.getByRole('button', { name: '核准申請' })).toBeVisible()

  state.request = {
    ...state.request,
    status: 'Superseded',
    classification: 'Standard',
    capabilities: [],
  }
  await page.reload()
  await expect(page.getByText('Superseded').first()).toBeVisible()
  await expect(page.getByRole('button', { name: '核准申請' })).toHaveCount(0)
  await expect(page.getByRole('button', { name: 'deploy' })).toHaveCount(0)
})

async function mockApplication(
  page: Page,
  state: { request: DeploymentRequestVersion; retryBody: unknown },
) {
  await page.route('**/api/v1/auth/session', (route) =>
    route.fulfill(
      json({ data: { id: 'user-1', username: 'vincent' }, meta: meta() }),
    ),
  )
  await page.route('**/api/v1/notifications**', (route) =>
    route.fulfill(json({ data: [], meta: meta() })),
  )
  await page.route('**/api/v1/catalog/resource-tree', (route) =>
    route.fulfill(json({ data: resourceTree(), meta: meta() })),
  )
  await page.route('**/api/v1/release-workflows**', (route) =>
    route.fulfill(json({ data: [workflowFixture()], meta: meta() })),
  )
  await page.route('**/api/v1/deployment-plans**', (route) =>
    route.fulfill(json({ data: [planFixture()], meta: meta() })),
  )
  await page.route('**/api/v1/deployment-history**', (route) =>
    route.fulfill(json({ data: [], meta: meta() })),
  )
  await page.route(
    `**/api/v1/deployment-executions/${ids.execution}`,
    (route) => route.fulfill(json({ data: executionFixture(), meta: meta() })),
  )
  await page.route('**/api/v1/deployment-requests**', async (route) => {
    const request = route.request()
    const url = new URL(request.url())
    if (
      request.method() === 'GET' &&
      url.pathname === '/api/v1/deployment-requests'
    )
      return route.fulfill(
        json({ data: [requestSummary(state.request)], meta: meta() }),
      )
    if (request.method() === 'GET')
      return route.fulfill(json({ data: state.request, meta: meta() }))
    if (url.pathname.endsWith('/decisions')) {
      state.request = approvedRequest(state.request)
      return route.fulfill(json({ data: state.request, meta: meta() }))
    }
    if (url.pathname.endsWith('/transitions')) {
      state.request = deployedRequest(state.request)
      return route.fulfill({
        status: 202,
        ...json({ data: { accepted: true }, meta: meta() }),
      })
    }
    if (url.pathname.endsWith('/retry')) {
      state.retryBody = request.postDataJSON()
      return route.fulfill({
        status: 202,
        ...json({ data: { accepted: true }, meta: meta() }),
      })
    }
    return route.fulfill({ status: 404, ...json({}) })
  })
}

function requestFixture(): DeploymentRequestVersion {
  return {
    id: ids.version,
    requestId: ids.request,
    organizationId: ids.organization,
    projectId: ids.project,
    environmentId: ids.environment,
    versionNumber: 1,
    status: 'PendingReview',
    classification: 'Standard',
    fingerprint: 'fingerprint-1',
    workflowVersionId: ids.workflowVersion,
    planVersionId: ids.planVersion,
    title: 'Automatic payment deployment',
    changeDescription: '',
    issueUrl: '',
    lockVersion: 1,
    workflowStateKey: 'review',
    capabilities: ['deployment_request.review'],
    createdAt: '2026-09-07T00:00:00Z',
    reviews: [
      {
        id: 'review-1',
        stateKey: 'review',
        stageNumber: 1,
        policyType: 'AnyApprover',
        requiredApprovals: 1,
        allowSelfReview: false,
        status: 'Pending',
      },
    ],
    applications: [
      applicationSnapshot(ids.appA, 'app-a', 0),
      applicationSnapshot(ids.appB, 'app-b', 1),
    ],
  }
}

function approvedRequest(
  request: DeploymentRequestVersion,
): DeploymentRequestVersion {
  return {
    ...request,
    status: 'Approved',
    lockVersion: 2,
    workflowStateKey: 'approved',
    capabilities: ['deployment_request.deploy'],
    reviews: [{ ...request.reviews[0], status: 'Approved' }],
  }
}

function deployedRequest(
  request: DeploymentRequestVersion,
): DeploymentRequestVersion {
  return {
    ...request,
    status: 'PartialFailed',
    lockVersion: 3,
    workflowStateKey: 'result',
    executionId: ids.execution,
    executionStatus: 'PartialFailed',
    capabilities: ['deployment_request.retry'],
  }
}

function applicationSnapshot(id: string, key: string, order: number) {
  return {
    id: `snapshot-${key}`,
    applicationId: id,
    applicationKey: key,
    liveRevision: 'commit-a',
    targetRevision: 'commit-b',
    targetRevisions: ['commit-b'],
    manifestHash: `manifest-${key}`,
    diffHash: `diff-${key}`,
    order,
    images: [],
    diff: {
      resources: [
        { group: 'apps', kind: 'Deployment', namespace: 'payment', name: key },
      ],
    },
  }
}

function executionFixture() {
  return {
    id: ids.execution,
    requestVersionId: ids.version,
    planVersionId: ids.planVersion,
    attempt: 1,
    status: 'PartialFailed',
    triggerKind: 'Workflow',
    lockVersion: 1,
    createdAt: '2026-09-07T00:00:00Z',
    nodes: [
      executionNode(ids.appA, 'app-a', 'Succeeded', 'Synced', 'Healthy', ''),
      executionNode(
        ids.appB,
        'app-b',
        'Failed',
        'OutOfSync',
        'Degraded',
        'sync failed',
      ),
    ],
  }
}

function executionNode(
  applicationId: string,
  nodeKey: string,
  status: string,
  syncStatus: string,
  healthStatus: string,
  errorMessage: string,
) {
  return {
    id: `node-${nodeKey}`,
    applicationId,
    nodeKey,
    status,
    operationId: `operation-${nodeKey}`,
    syncStatus,
    healthStatus,
    actualRevision: 'commit-b',
    actualImages: [],
    errorCode: errorMessage ? 'SYNC_FAILED' : '',
    errorMessage,
  }
}

function workflowFixture() {
  return {
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
        lockVersion: 1,
        createdAt: '2026-09-07T00:00:00Z',
        document: {
          initialState: 'review',
          states: [
            { key: 'review', name: 'Review', type: 'Review' },
            { key: 'approved', name: 'Approved', type: 'ManualAction' },
            { key: 'result', name: 'Result', type: 'Terminal' },
          ],
          transitions: [
            {
              key: 'deploy',
              from: 'approved',
              to: 'result',
              trigger: 'Manual',
              permission: 'deployment_request.deploy',
              conditions: [],
            },
          ],
        },
      },
    ],
  }
}

function planFixture() {
  return {
    id: ids.plan,
    ownerKind: 'Project',
    ownerProjectId: ids.project,
    name: 'Production plan',
    description: '',
    active: true,
    versions: [
      {
        id: ids.planVersion,
        planId: ids.plan,
        versionNumber: 1,
        lifecycle: 'Published',
        lockVersion: 1,
        createdAt: '2026-09-07T00:00:00Z',
        document: {
          maxParallel: 2,
          nodes: [
            { key: 'app-a', applicationKey: 'app-a', order: 0 },
            { key: 'app-b', applicationKey: 'app-b', order: 1 },
          ],
          edges: [
            { from: 'app-a', to: 'app-b', condition: 'UpstreamSucceeded' },
          ],
        },
      },
    ],
  }
}

function requestSummary(request: DeploymentRequestVersion) {
  return {
    id: ids.request,
    organizationId: ids.organization,
    projectId: ids.project,
    environmentId: ids.environment,
    status: request.status,
    classification: request.classification,
    activeVersionNumber: request.versionNumber,
    title: request.title,
    applicationCount: request.applications.length,
    updatedAt: request.createdAt,
  }
}

function resourceTree() {
  return [
    {
      id: ids.organization,
      name: 'Organization A',
      canCreateProject: false,
      projects: [
        {
          id: ids.project,
          name: 'Project A',
          canManage: false,
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
  ]
}

function json(body: unknown) {
  return { contentType: 'application/json', body: JSON.stringify(body) }
}

function meta() {
  return { requestId: 'e2e', timestamp: new Date().toISOString() }
}
