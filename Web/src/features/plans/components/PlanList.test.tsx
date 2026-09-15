import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { App as AntdApp } from 'antd'
import { I18nextProvider } from 'react-i18next'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import type { DeploymentPlan } from '@/generated/model'
import i18n from '@/shared/i18n/config'

import { PlanList } from './PlanList'

describe('PlanList', () => {
  beforeEach(async () => i18n.changeLanguage('zh-TW'))
  afterEach(cleanup)

  it('supports keyboard selection and exposes the current item', () => {
    const onSelect = vi.fn()
    render(
      <AntdApp>
        <I18nextProvider i18n={i18n}>
          <PlanList
            plans={[plan('plan-1', 'Production'), plan('plan-2', 'Staging')]}
            selected="plan-1"
            loading={false}
            onSelect={onSelect}
          />
        </I18nextProvider>
      </AntdApp>,
    )

    const current = screen.getByRole('button', {
      name: /Production Production deployment flow/,
    })
    const staging = screen.getByRole('button', {
      name: /Staging Staging deployment flow/,
    })
    expect(current).toHaveAttribute('aria-current', 'page')
    fireEvent.keyDown(staging, { key: ' ' })
    expect(onSelect).toHaveBeenCalledWith('plan-2')
  })
})

function plan(id: string, name: string): DeploymentPlan {
  return {
    id,
    ownerKind: 'project',
    ownerProjectId: 'project-1',
    name,
    description: `${name} deployment flow`,
    active: true,
    versions: [],
  }
}
