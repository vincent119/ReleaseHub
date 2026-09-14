import { App as AntdApp, Button } from 'antd'
import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { I18nextProvider } from 'react-i18next'
import { afterEach, beforeEach, describe, expect, it } from 'vitest'

import i18n from '@/shared/i18n/config'

import {
  createFeedbackOptions,
  type FeedbackType,
  useFeedback,
} from './useFeedback'

const cases = [
  ['success', 4, 'status', false, '操作成功'],
  ['info', 4, 'status', false, '資訊'],
  ['warning', 7, 'status', undefined, '注意'],
  ['error', 0, 'alert', undefined, '操作失敗'],
] as const

describe('useFeedback', () => {
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

  it.each(cases)(
    'creates %s notification semantics',
    (type, duration, role, closeIcon) => {
      const options = createFeedbackOptions(type, 'Title', 'Description')

      expect(options).toMatchObject({
        type,
        title: 'Title',
        description: 'Description',
        duration,
        role,
        closeIcon,
        placement: 'topRight',
      })
      expect(options.className).toContain('feedback')
    },
  )

  it.each(cases)(
    'renders the localized %s notification card',
    async (type, _duration, role, _closeIcon, title) => {
      render(
        <I18nextProvider i18n={i18n}>
          <AntdApp>
            <FeedbackProbe type={type} />
          </AntdApp>
        </I18nextProvider>,
      )

      fireEvent.click(screen.getByRole('button', { name: '顯示通知' }))

      const notice = await screen.findByRole(role)
      expect(notice).toHaveTextContent(title)
      expect(notice).toHaveTextContent('詳細內容')
    },
  )
})

function FeedbackProbe({ type }: { type: FeedbackType }) {
  const feedback = useFeedback()
  return <Button onClick={() => feedback[type]('詳細內容')}>顯示通知</Button>
}
