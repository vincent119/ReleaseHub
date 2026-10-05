import { act, cleanup, fireEvent, render, screen } from '@testing-library/react'
import { ConfigProvider } from 'antd'
import { I18nextProvider } from 'react-i18next'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import type { CatalogOrganizationNode } from '@/generated/model'
import i18n from '@/shared/i18n/config'

import styles from './PlanScopeSelector.module.css'
import { PlanScopeSelector } from './PlanScopeSelector'
import type { ResourceScopeValue } from '@/shared/scope'

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

  afterEach(async () => {
    await act(async () => cleanup())
  })

  it('applies the shared width rule and disables Environment initially', () => {
    renderSelector()

    const project = screen.getByLabelText('Project')
    const environment = screen.getByLabelText('環境（選填）')

    expect(document.querySelector(`.${styles.scopeCard}`)).toBeInTheDocument()
    expect(project).toBeEnabled()
    expect(environment).toBeDisabled()
  })

  it('returns the existing organization and project selection contract', async () => {
    const onChange = vi.fn()
    renderSelector({ onChange })

    fireEvent.mouseDown(screen.getByLabelText('Project'))
    const option = await screen.findByText('Payment platform', {
      selector: '.ant-select-item-option-content',
    })
    fireEvent.click(option)

    expect(onChange).toHaveBeenCalledWith({
      organizationId: 'organization-1',
      projectId: 'project-1',
    })
  })

  it('returns Environment without changing the selected parent scope', async () => {
    const onChange = vi.fn()
    renderSelector({
      scope: {
        organizationId: 'organization-1',
        projectId: 'project-1',
      },
      onChange,
    })

    fireEvent.mouseDown(screen.getByLabelText('環境（選填）'))
    fireEvent.click(
      await screen.findByText('Production', {
        selector: '.ant-select-item-option-content',
      }),
    )

    expect(onChange).toHaveBeenCalledWith({
      organizationId: 'organization-1',
      projectId: 'project-1',
      environmentId: 'environment-1',
    })
  })

  it('選擇組織但未選 Project 時仍回報過渡值，避免保留舊 scope', async () => {
    const onChange = vi.fn()
    const other = {
      ...organizations[0],
      id: 'organization-2',
      name: '另一組織',
      projects: [],
    }
    render(
      <I18nextProvider i18n={i18n}>
        <PlanScopeSelector
          organizations={[...organizations, other]}
          onChange={onChange}
        />
      </I18nextProvider>,
    )
    fireEvent.mouseDown(screen.getByLabelText('組織'))
    fireEvent.click(
      await screen.findByText('另一組織', {
        selector: '.ant-select-item-option-content',
      }),
    )
    expect(onChange).toHaveBeenCalledWith({ organizationId: 'organization-2' })
    expect(screen.getByLabelText('Project')).toBeDisabled()
  })
})

function renderSelector({
  scope,
  onChange = vi.fn(),
}: {
  scope?: ResourceScopeValue
  onChange?: (value: ResourceScopeValue) => void
} = {}) {
  return render(
    <I18nextProvider i18n={i18n}>
      <ConfigProvider theme={{ token: { motion: false } }}>
        <PlanScopeSelector
          organizations={organizations}
          scope={scope}
          onChange={onChange}
        />
      </ConfigProvider>
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
