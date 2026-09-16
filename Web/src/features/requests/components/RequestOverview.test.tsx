import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { App } from 'antd'
import { I18nextProvider } from 'react-i18next'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import type { DeploymentRequestVersion } from '@/generated/model'
import i18n from '@/shared/i18n/config'

import { RequestOverview } from './RequestOverview'

const api = vi.hoisted(() => ({ update: vi.fn() }))

vi.mock('@/generated/api', () => ({
  updateDeploymentRequestVersionMetadata: api.update,
}))

describe('RequestOverview capability handling', () => {
  beforeEach(async () => {
    await i18n.changeLanguage('zh-TW')
    api.update.mockReset()
    document.cookie = 'releasehub_csrf=csrf-token; path=/'
  })

  it('keeps edited input when a stale capability is rejected', async () => {
    api.update.mockResolvedValue({ status: 404 })
    renderOverview(requestFixture(), vi.fn())

    fireEvent.click(screen.getByRole('button', { name: '編輯選填資訊' }))
    const description = screen.getByLabelText('變更說明')
    fireEvent.change(description, {
      target: { value: '保留這段尚未送出的內容' },
    })
    fireEvent.click(screen.getByRole('button', { name: '建立新 Version' }))

    await waitFor(() => expect(api.update).toHaveBeenCalledOnce())
    expect(screen.getByRole('dialog')).toBeInTheDocument()
    expect(description).toHaveValue('保留這段尚未送出的內容')
  })
})

function renderOverview(
  request: DeploymentRequestVersion,
  onUpdated: () => Promise<unknown> | void,
) {
  return render(
    <App>
      <I18nextProvider i18n={i18n}>
        <RequestOverview
          request={request}
          onUpdated={async () => onUpdated()}
        />
      </I18nextProvider>
    </App>,
  )
}

function requestFixture(): DeploymentRequestVersion {
  return {
    id: 'version-1',
    requestId: 'request-1',
    organizationId: 'organization-1',
    projectId: 'project-1',
    environmentId: 'environment-1',
    versionNumber: 2,
    status: 'PendingReview',
    classification: 'Standard',
    fingerprint: 'fingerprint',
    workflowVersionId: 'workflow-version-1',
    planVersionId: 'plan-version-1',
    title: 'Deploy payment',
    changeDescription: '原始內容',
    issueUrl: '',
    lockVersion: 4,
    capabilities: ['deployment_request.update'],
    applications: [],
    reviews: [],
    createdAt: '2026-09-16T00:00:00Z',
  }
}
