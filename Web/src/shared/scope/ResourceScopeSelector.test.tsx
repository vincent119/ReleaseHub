import { act, cleanup, fireEvent, render, screen } from '@testing-library/react'
import { ConfigProvider } from 'antd'
import { useState } from 'react'
import { I18nextProvider } from 'react-i18next'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import type { CatalogOrganizationNode } from '@/generated/model'
import i18n from '@/shared/i18n/config'

import { ResourceScopeSelector } from './ResourceScopeSelector'
import styles from './ResourceScopeSelector.module.css'
import type { ResourceScopeValue } from './resourceScope'
import { multipleScopeFixture, scopeFixture } from './scopeFixtures'

beforeEach(async () => {
  await i18n.changeLanguage('zh-TW')
})

afterEach(async () => {
  await act(async () => cleanup())
})

function renderSelector({
  organizations = scopeFixture(),
  value,
  onChange = vi.fn(),
  loading = false,
  disabled = false,
  environmentMode = 'required',
}: {
  organizations?: CatalogOrganizationNode[]
  value?: ResourceScopeValue
  onChange?: (value: ResourceScopeValue) => void
  loading?: boolean
  disabled?: boolean
  environmentMode?: 'required' | 'optional'
} = {}) {
  return render(
    <I18nextProvider i18n={i18n}>
      <ConfigProvider theme={{ token: { motion: false } }}>
        <ResourceScopeSelector
          organizations={organizations}
          value={value}
          onChange={onChange}
          loading={loading}
          disabled={disabled}
          environmentMode={environmentMode}
        />
      </ConfigProvider>
    </I18nextProvider>,
  )
}

async function choose(field: string, option: string) {
  fireEvent.mouseDown(screen.getByRole('combobox', { name: field }))
  fireEvent.click(
    await screen.findByText(option, {
      selector: '.ant-select-item-option-content',
    }),
  )
}

describe('ResourceScopeSelector', () => {
  it('提示區持續預留空間，未選取不產生空白按鈕或焦點', () => {
    const { container, rerender } = renderSelector()
    const slots = () => container.querySelectorAll(`.${styles.nameSlot}`)
    expect(slots()).toHaveLength(2)
    for (const slot of slots()) {
      expect(slot).toHaveAttribute('aria-hidden', 'true')
      expect(slot.querySelector('button, [tabindex]')).toBeNull()
    }
    rerender(
      <I18nextProvider i18n={i18n}>
        <ResourceScopeSelector
          organizations={scopeFixture()}
          value={{ projectId: 'project-0' }}
          onChange={vi.fn()}
        />
      </I18nextProvider>,
    )
    expect(slots()).toHaveLength(2)
    const button = screen.getByRole('button', {
      name: '查看 Project 完整名稱',
    })
    expect(button.parentElement).toHaveClass(styles.nameSlot)
    expect(button.parentElement).not.toHaveAttribute('aria-hidden')
    expect(
      screen.queryByRole('button', { name: '查看 環境 完整名稱' }),
    ).toBeNull()
  })

  it('顯示可見標籤，單組織不重複顯示，未選 Project 時環境停用', () => {
    renderSelector()
    expect(screen.queryByRole('combobox', { name: '組織' })).toBeNull()
    expect(screen.getByLabelText('Project')).toBeEnabled()
    expect(screen.getByLabelText('環境（必填）')).toBeDisabled()
    expect(screen.getByText('請先選擇 Project。')).toBeInTheDocument()
  })

  it('50 個 Project 只列各一筆，可依名稱片段搜尋', async () => {
    const onChange = vi.fn()
    renderSelector({ organizations: scopeFixture(50), onChange })
    const project = screen.getByLabelText('Project')
    fireEvent.mouseDown(project)
    fireEvent.change(project, { target: { value: '49' } })
    expect(
      await screen.findByText('測試專案 49', {
        selector: '.ant-select-item-option-content',
      }),
    ).toBeInTheDocument()
    expect(
      document.querySelectorAll('.ant-select-item-option-content'),
    ).toHaveLength(1)
    fireEvent.click(
      screen.getByText('測試專案 49', {
        selector: '.ant-select-item-option-content',
      }),
    )
    expect(onChange).toHaveBeenCalledWith({
      organizationId: 'organization-a',
      projectId: 'project-49',
    })
  })

  it('多組織先選組織，不將同名 Project 混在一起', async () => {
    const onChange = vi.fn()
    renderSelector({ organizations: multipleScopeFixture(), onChange })
    expect(screen.getByLabelText('Project')).toBeDisabled()
    await choose('組織', '另一組織')
    expect(onChange).toHaveBeenCalledWith({ organizationId: 'organization-b' })
  })

  it('選取 Project 回傳父 ID 並清除舊環境', async () => {
    const onChange = vi.fn()
    renderSelector({
      value: { projectId: 'project-0', environmentId: 'environment-0' },
      onChange,
    })
    await choose('Project', '測試專案 1')
    expect(onChange).toHaveBeenCalledWith({
      organizationId: 'organization-a',
      projectId: 'project-1',
    })
  })

  it('跨組織同名 Project 回傳所選組織的實際 ID', async () => {
    const onChange = vi.fn()
    renderSelector({
      organizations: multipleScopeFixture(),
      value: { organizationId: 'organization-b' },
      onChange,
    })
    await choose('Project', '測試專案 0')
    expect(onChange).toHaveBeenCalledWith({
      organizationId: 'organization-b',
      projectId: 'project-b',
    })
  })

  it('組織沒有 Project 時停用欄位並提供獨立說明', () => {
    const organizations = scopeFixture()
    organizations[0].projects = []
    renderSelector({ organizations })
    expect(screen.getByLabelText('Project')).toBeDisabled()
    expect(screen.getByText('此組織沒有可選擇的 Project。')).toBeInTheDocument()
  })

  it('Project 沒有環境時不造選項，保留 Project 與說明', () => {
    const organizations = scopeFixture()
    organizations[0].projects[0].environments = []
    renderSelector({ organizations, value: { projectId: 'project-0' } })
    expect(screen.getByLabelText('Project')).toBeEnabled()
    expect(screen.getByLabelText('環境（必填）')).toBeDisabled()
    expect(
      screen.getByText('此 Project 沒有可選擇的環境。'),
    ).toBeInTheDocument()
  })

  it('環境只來自所選 Project，Global 不變成全部環境', async () => {
    const onChange = vi.fn()
    renderSelector({ value: { projectId: 'project-0' }, onChange })
    await choose('環境（必填）', 'Global')
    expect(onChange).toHaveBeenCalledWith({
      organizationId: 'organization-a',
      projectId: 'project-0',
      environmentId: 'global-0',
    })
  })

  it('受控值只有呼叫端更新後才改變，沒有自行查詢或選第一筆', async () => {
    const onChange = vi.fn()
    renderSelector({ onChange })
    await choose('Project', '測試專案 0')
    expect(onChange).toHaveBeenCalledOnce()
    expect(screen.getByLabelText('環境（必填）')).toBeDisabled()
  })

  it('在受控頁面切換 Project 後，環境與完整名稱提示同步更新', async () => {
    function Harness() {
      const [value, setValue] = useState<ResourceScopeValue>({
        projectId: 'project-0',
        environmentId: 'environment-0',
      })
      return (
        <I18nextProvider i18n={i18n}>
          <ConfigProvider theme={{ token: { motion: false } }}>
            <ResourceScopeSelector
              organizations={scopeFixture()}
              value={value}
              onChange={setValue}
            />
          </ConfigProvider>
        </I18nextProvider>
      )
    }
    render(<Harness />)
    await choose('Project', '測試專案 1')
    expect(screen.getByLabelText('環境（必填）')).toBeEnabled()
    expect(screen.queryByText('Production')).toBeNull()
    expect(
      screen.getByRole('button', { name: '查看 Project 完整名稱' }),
    ).toBeEnabled()
  })

  it('以可點擊控制取得完整長名稱，不只提供滑鼠 title', async () => {
    const organizations = scopeFixture()
    const name = '這是一個完整名稱很長而且需要以鍵盤與觸控辨識的測試專案'
    organizations[0].projects[0].name = name
    renderSelector({ organizations, value: { projectId: 'project-0' } })
    fireEvent.click(
      screen.getByRole('button', { name: '查看 Project 完整名稱' }),
    )
    expect(
      await screen.findByText(name, { selector: '[data-scope-full-name]' }),
    ).toBeInTheDocument()
  })

  it('搜尋無結果提示不與空 Catalog 混淆', async () => {
    renderSelector()
    fireEvent.mouseDown(screen.getByLabelText('Project'))
    fireEvent.change(screen.getByLabelText('Project'), {
      target: { value: '不存在' },
    })
    expect(
      await screen.findByText('沒有符合搜尋條件的選項。'),
    ).toBeInTheDocument()
  })

  it('空 Catalog 提供說明且不產生 ID 或通知', () => {
    const onChange = vi.fn()
    renderSelector({ organizations: [], onChange })
    expect(screen.getByText('沒有可選擇的組織。')).toBeInTheDocument()
    expect(screen.getByLabelText('Project')).toBeDisabled()
    expect(onChange).not.toHaveBeenCalled()
  })

  it.each(['loading', 'disabled'] as const)(
    '%s 時停用所有選取控制',
    (state) => {
      renderSelector({
        organizations: multipleScopeFixture(),
        value: { organizationId: 'organization-a', projectId: 'project-0' },
        [state]: true,
      })
      for (const field of screen.getAllByRole('combobox')) {
        expect(field).toBeDisabled()
      }
    },
  )

  it('環境可選模式有獨立標籤，不要求先選環境', () => {
    renderSelector({ environmentMode: 'optional' })
    expect(screen.getByLabelText('環境（選填）')).toHaveAttribute(
      'aria-required',
      'false',
    )
  })

  it('同頁兩個 selector 的 labels 與 input ID 不衝突', () => {
    renderSelector()
    renderSelector()
    const ids = screen.getAllByRole('combobox').map((field) => field.id)
    expect(new Set(ids).size).toBe(ids.length)
  })
})
