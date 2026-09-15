import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { I18nextProvider } from 'react-i18next'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import type { CatalogOrganizationNode } from '@/generated/model'
import i18n from '@/shared/i18n/config'

import styles from './PlanScopeSelector.module.css'
import { PlanScopeSelector, type PlanScopeChoice } from './PlanScopeSelector'

describe('PlanScopeSelector', () => {
  beforeEach(async () => {
    await i18n.changeLanguage('zh-TW')
    Object.defineProperty(window, 'matchMedia', {
      configurable: true,
      value: () => ({
        matches: false,
        addEventListener: () => undefined,
        removeEventListener: () => undefined,
      }),
    })
  })

  afterEach(cleanup)

  it('applies the shared width rule and disables Environment initially', () => {
    renderSelector()

    const project = screen.getByLabelText('選擇 Project')
    const environment = screen.getByLabelText('選擇 Environment')

    expect(project.closest('.ant-select')).toHaveClass(styles.scopeSelect)
    expect(environment.closest('.ant-select')).toHaveClass(styles.scopeSelect)
    expect(environment).toBeDisabled()
  })

  it('returns the existing organization and project selection contract', () => {
    const onChange = vi.fn()
    renderSelector({ onChange })

    fireEvent.mouseDown(screen.getByLabelText('選擇 Project'))
    const option = screen.getByTitle('default / Payment platform')
    expect(option).toHaveAttribute('title', 'default / Payment platform')
    fireEvent.click(option)

    expect(onChange).toHaveBeenCalledWith({
      organizationId: 'organization-1',
      projectId: 'project-1',
    })
  })

  it('returns Environment without changing the selected parent scope', () => {
    const onChange = vi.fn()
    renderSelector({
      scope: {
        organizationId: 'organization-1',
        projectId: 'project-1',
      },
      onChange,
    })

    fireEvent.mouseDown(screen.getByLabelText('選擇 Environment'))
    fireEvent.click(screen.getByTitle('Production'))

    expect(onChange).toHaveBeenCalledWith({
      organizationId: 'organization-1',
      projectId: 'project-1',
      environmentId: 'environment-1',
    })
  })
})

function renderSelector({
  scope,
  onChange = vi.fn(),
}: {
  scope?: {
    organizationId: string
    projectId: string
    environmentId?: string
  }
  onChange?: (value: PlanScopeChoice) => void
} = {}) {
  return render(
    <I18nextProvider i18n={i18n}>
      <PlanScopeSelector
        organizations={organizations}
        scope={scope}
        onChange={onChange}
      />
    </I18nextProvider>,
  )
}

const organizations: CatalogOrganizationNode[] = [
  {
    id: 'organization-1',
    name: 'default',
    version: 1,
    isDefault: true,
    canRename: true,
    canDelete: false,
    canCreateProject: true,
    projects: [
      {
        id: 'project-1',
        name: 'Payment platform',
        canManage: true,
        environments: [
          {
            id: 'environment-1',
            name: 'Production',
            type: 'Production',
            applications: [],
          },
        ],
      },
    ],
  },
]
