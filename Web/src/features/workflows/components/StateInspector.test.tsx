import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { App as AntdApp } from 'antd'
import { I18nextProvider } from 'react-i18next'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import type { ReleaseWorkflowState } from '@/generated/model'
import i18n from '@/shared/i18n/config'

import { StateInspector } from './StateInspector'

describe('StateInspector Review options', () => {
  afterEach(cleanup)

  beforeEach(async () => {
    await i18n.changeLanguage('zh-TW')
  })

  it('lists readable users and Roles and stores their IDs', () => {
    const onChange = vi.fn()
    renderInspector(onChange)

    fireEvent.mouseDown(screen.getByLabelText('指定使用者'))
    expect(
      screen.queryByText('disabled-reviewer（目前不可指派）'),
    ).not.toBeInTheDocument()
    fireEvent.click(screen.getByText('release-reviewer'))
    expect(onChange).toHaveBeenCalledWith(
      expect.objectContaining({
        reviewPolicy: expect.objectContaining({ userIds: ['user-1'] }),
      }),
    )

    fireEvent.mouseDown(screen.getByLabelText('指定 Role'))
    fireEvent.click(screen.getByText('release_approver · platform'))
    expect(onChange).toHaveBeenCalledWith(
      expect.objectContaining({
        reviewPolicy: expect.objectContaining({ roleIds: ['role-1'] }),
      }),
    )
  })

  it('does not offer unavailable entries unless the draft already references them', () => {
    renderInspector(vi.fn(), {
      ...reviewState,
      reviewPolicy: {
        ...reviewState.reviewPolicy!,
        userIds: ['user-disabled'],
      },
    })

    expect(
      screen.getByText('disabled-reviewer（目前不可指派）'),
    ).toBeInTheDocument()
  })
})

const reviewState: ReleaseWorkflowState = {
  key: 'review',
  name: 'Review',
  type: 'Review',
  reviewPolicy: {
    type: 'AnyApprover',
    requiredApprovals: 1,
    allowSelfReview: false,
    userIds: [],
    roleIds: [],
  },
}

function renderInspector(
  onChange: (state: ReleaseWorkflowState) => void,
  state: ReleaseWorkflowState = reviewState,
) {
  return render(
    <AntdApp>
      <I18nextProvider i18n={i18n}>
        <StateInspector
          state={state}
          initial={false}
          reviewOptions={{
            users: [
              { id: 'user-1', username: 'release-reviewer', assignable: true },
              {
                id: 'user-disabled',
                username: 'disabled-reviewer',
                assignable: false,
              },
            ],
            roles: [
              {
                id: 'role-1',
                name: 'release_approver',
                ownerKind: 'platform',
                assignable: true,
              },
            ],
          }}
          onChange={onChange}
          onMakeInitial={vi.fn()}
          onRemove={vi.fn()}
        />
      </I18nextProvider>
    </AntdApp>,
  )
}
