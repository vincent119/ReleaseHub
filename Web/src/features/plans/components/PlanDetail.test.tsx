import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { App as AntdApp } from 'antd'
import { I18nextProvider } from 'react-i18next'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import type { DeploymentPlan, DeploymentPlanVersion } from '@/generated/model'
import i18n from '@/shared/i18n/config'

import { PlanDetail } from './PlanDetail'

vi.mock('./PlanGraphEditor', () => ({
  PlanGraphEditor: () => <div>Plan graph</div>,
}))

describe('PlanDetail', () => {
  beforeEach(async () => {
    await i18n.changeLanguage('zh-TW')
  })

  afterEach(cleanup)

  it('disables new version while the latest Plan version is Draft', async () => {
    const onNewVersion = vi.fn()
    const published = planVersion('version-1', 1, 'Published')
    const draft = planVersion('version-2', 2, 'Draft')

    renderDetail(planWithVersions(published, draft), published, onNewVersion)

    const button = screen.getByRole('button', { name: '建立新版本' })
    expect(button).toBeDisabled()
    fireEvent.mouseEnter(button.parentElement!)
    expect(
      await screen.findByText('目前已有 Draft，請先發布目前版本。'),
    ).toBeInTheDocument()
    fireEvent.click(button)
    expect(onNewVersion).not.toHaveBeenCalled()
  })

  it.each(['Published', 'Disabled'] as const)(
    'allows a new version when the latest Plan version is %s',
    (lifecycle) => {
      const onNewVersion = vi.fn()
      const version = planVersion('version-1', 1, lifecycle)

      renderDetail(planWithVersions(version), version, onNewVersion)

      const button = screen.getByRole('button', { name: '建立新版本' })
      expect(button).toBeEnabled()
      fireEvent.click(button)
      expect(onNewVersion).toHaveBeenCalledOnce()
    },
  )

  it('places lifecycle metadata in the detail header', () => {
    const version = planVersion('version-1', 1, 'Published')

    renderDetail(planWithVersions(version), version, vi.fn())

    expect(
      screen.getByText('Published').closest('.ant-card-head'),
    ).not.toBeNull()
    expect(
      screen.getByText('Plan graph').closest('.ant-card-body'),
    ).not.toBeNull()
  })
})

function renderDetail(
  plan: DeploymentPlan,
  version: DeploymentPlanVersion,
  onNewVersion: () => void,
) {
  return render(
    <AntdApp>
      <I18nextProvider i18n={i18n}>
        <PlanDetail
          plan={plan}
          version={version}
          submitting={false}
          onVersion={vi.fn()}
          onNewVersion={onNewVersion}
          onLifecycle={vi.fn()}
        />
      </I18nextProvider>
    </AntdApp>,
  )
}

function planWithVersions(
  ...versions: DeploymentPlanVersion[]
): DeploymentPlan {
  return {
    id: 'plan-1',
    ownerKind: 'project',
    ownerProjectId: 'project-1',
    name: 'Production plan',
    description: 'Production flow',
    active: true,
    versions,
  }
}

function planVersion(
  id: string,
  versionNumber: number,
  lifecycle: DeploymentPlanVersion['lifecycle'],
): DeploymentPlanVersion {
  return {
    id,
    planId: 'plan-1',
    versionNumber,
    lifecycle,
    document: { nodes: [], edges: [] },
    lockVersion: 1,
    createdAt: '2026-09-15T00:00:00Z',
  }
}
