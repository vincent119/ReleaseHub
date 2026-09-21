import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/react'
import { I18nextProvider } from 'react-i18next'
import { MemoryRouter } from 'react-router'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import i18n from '@/shared/i18n/config'
import tagStyles from '@/shared/tag/SemanticTag.module.css'

import { NotificationCenter } from './NotificationCenter'
import styles from './NotificationCenter.module.css'

const api = vi.hoisted(() => ({
  list: vi.fn(),
  mark: vi.fn(),
  markAll: vi.fn(),
  refetch: vi.fn(),
}))

vi.mock('@/generated/api', () => ({
  useListNotifications: api.list,
  markNotificationRead: api.mark,
  markAllNotificationsRead: api.markAll,
  getListNotificationsQueryKey: () => ['/api/v1/notifications'],
}))

vi.mock('./useNotificationEvents', () => ({ useNotificationEvents: vi.fn() }))

describe('NotificationCenter', () => {
  beforeEach(async () => {
    vi.clearAllMocks()
    await i18n.changeLanguage('zh-TW')
    api.refetch.mockResolvedValue(undefined)
    api.mark.mockResolvedValue({ status: 204 })
    api.markAll.mockResolvedValue({ status: 204 })
    api.list.mockReturnValue({
      isPending: false,
      isFetching: false,
      isError: false,
      refetch: api.refetch,
      data: {
        status: 200,
        data: {
          data: [
            {
              id: 'notification-1',
              eventType: 'deployment.request.created',
              resourceType: 'deployment_request',
              resourceId: 'request-1',
              occurredAt: '2026-09-07T00:00:00Z',
              read: false,
              restricted: false,
            },
          ],
        },
      },
    })
  })

  afterEach(cleanup)

  it('uses the enlarged outlined bell without changing the trigger semantics', () => {
    renderCenter()

    const trigger = screen.getByRole('button', { name: '開啟通知' })
    expect(trigger.querySelector('.anticon-bell')).toHaveClass(styles.icon)
    expect(screen.getByText('1')).toBeInTheDocument()
  })

  it('shows unread history, resource link, and marks an item read', async () => {
    render(
      <I18nextProvider i18n={i18n}>
        <MemoryRouter>
          <NotificationCenter />
        </MemoryRouter>
      </I18nextProvider>,
    )
    fireEvent.click(screen.getByRole('button', { name: '開啟通知' }))
    expect(screen.getByText('已建立 Deployment Request')).toBeVisible()
    expect(screen.getByRole('link', { name: '查看相關資源' })).toHaveAttribute(
      'href',
      '/requests/request-1',
    )
    fireEvent.click(screen.getByRole('button', { name: '標記已讀' }))
    await waitFor(() =>
      expect(api.mark).toHaveBeenCalledWith(
        'notification-1',
        expect.any(Object),
      ),
    )
  })

  it('marks all notifications read', async () => {
    renderCenter()
    fireEvent.click(screen.getByRole('button', { name: '開啟通知' }))
    fireEvent.click(screen.getByRole('button', { name: '全部標記已讀' }))
    await waitFor(() => expect(api.markAll).toHaveBeenCalled())
  })

  it('does not link to a resource when notification details are restricted', () => {
    api.list.mockReturnValue({
      isPending: false,
      isFetching: false,
      isError: false,
      refetch: api.refetch,
      data: {
        status: 200,
        data: {
          data: [
            {
              id: 'notification-2',
              eventType: 'deployment.notification.restricted',
              resourceType: 'deployment_request',
              resourceId: 'request-2',
              occurredAt: '2026-09-07T00:00:00Z',
              read: false,
              restricted: true,
            },
          ],
        },
      },
    })
    renderCenter()
    fireEvent.click(screen.getByRole('button', { name: '開啟通知' }))
    expect(screen.getByText('內容已遮蔽')).toHaveClass(tagStyles.neutral)
    expect(screen.queryByRole('link', { name: '查看相關資源' })).toBeNull()
  })
})

function renderCenter() {
  return render(
    <I18nextProvider i18n={i18n}>
      <MemoryRouter>
        <NotificationCenter />
      </MemoryRouter>
    </I18nextProvider>,
  )
}
