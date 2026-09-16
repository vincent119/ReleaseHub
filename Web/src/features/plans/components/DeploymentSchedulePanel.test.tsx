import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/react'
import { App } from 'antd'
import { I18nextProvider } from 'react-i18next'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import i18n from '@/shared/i18n/config'

import { DeploymentSchedulePanel } from './DeploymentSchedulePanel'

const api = vi.hoisted(() => ({
  refetch: vi.fn(),
  update: vi.fn(),
  useSchedule: vi.fn(),
}))
const feedback = vi.hoisted(() => ({ error: vi.fn(), success: vi.fn() }))

vi.mock('@/generated/api', () => ({
  updateDeploymentSchedule: api.update,
  useGetDeploymentSchedule: api.useSchedule,
}))

vi.mock('@/shared/feedback/useFeedback', () => ({
  useFeedback: () => feedback,
}))

describe('DeploymentSchedulePanel', () => {
  beforeEach(async () => {
    await i18n.changeLanguage('zh-TW')
    api.refetch.mockReset()
    api.update.mockReset()
    api.useSchedule.mockReset()
    feedback.error.mockReset()
    feedback.success.mockReset()
    document.cookie = 'releasehub_csrf=csrf-token; path=/'
    vi.stubGlobal('crypto', { randomUUID: () => 'schedule-command-key' })
    api.useSchedule.mockReturnValue(scheduleQuery(scheduleFixture()))
  })

  afterEach(cleanup)

  it('saves formatted inputs with optimistic version and command headers', async () => {
    api.update.mockResolvedValue({ status: 200 })
    renderPanel()
    fireEvent.click(screen.getByRole('button', { name: '儲存排程政策' }))

    await waitFor(() =>
      expect(api.update).toHaveBeenCalledWith(
        'environment-1',
        {
          enabled: true,
          timeZone: 'Asia/Taipei',
          weeklyWindows: [{ dayOfWeek: 1, startMinute: 540, endMinute: 1020 }],
          blackouts: [
            {
              startsAt: '2026-09-22T02:00:00.000Z',
              endsAt: '2026-09-22T03:00:00.000Z',
            },
          ],
          expectedVersion: 4,
        },
        {
          headers: {
            'X-CSRF-Token': 'csrf-token',
            'Idempotency-Key': 'schedule-command-key',
          },
        },
      ),
    )
    expect(api.refetch).toHaveBeenCalledOnce()
  })

  it('keeps stale input and does not refetch after a conflict', async () => {
    api.update.mockResolvedValue({ status: 409, data: { code: 'x' } })
    renderPanel()
    const timeZone = screen.getByLabelText('IANA 時區')
    fireEvent.change(timeZone, { target: { value: 'Europe/Paris' } })
    fireEvent.click(screen.getByRole('button', { name: '儲存排程政策' }))

    await waitFor(() => expect(api.update).toHaveBeenCalledOnce())
    expect(timeZone).toHaveValue('Europe/Paris')
    expect(api.refetch).not.toHaveBeenCalled()
    expect(feedback.error).toHaveBeenCalledWith(
      '排程政策已被其他人更新；目前輸入內容已保留，請核對後再送出。',
    )
  })

  it('rejects a non-IANA time zone before sending a mutation', async () => {
    renderPanel()
    fireEvent.change(screen.getByLabelText('IANA 時區'), {
      target: { value: 'Taipei' },
    })
    fireEvent.click(screen.getByRole('button', { name: '儲存排程政策' }))

    expect(
      await screen.findByText('請輸入 IANA 時區，例如 Asia/Taipei。'),
    ).toBeInTheDocument()
    expect(api.update).not.toHaveBeenCalled()
  })

  it('renders a read-only policy without mutation controls', () => {
    api.useSchedule.mockReturnValue(
      scheduleQuery({ ...scheduleFixture(), canManage: false }),
    )
    renderPanel()

    expect(
      screen.getByText(
        '你可以查看目前政策，但沒有修改這個 Environment 排程的權限。',
      ),
    ).toBeInTheDocument()
    expect(screen.getByRole('switch')).toBeDisabled()
    expect(
      screen.queryByRole('button', { name: '儲存排程政策' }),
    ).not.toBeInTheDocument()
  })

  it('renders an unrestricted empty policy with explicit add actions', () => {
    api.useSchedule.mockReturnValue(
      scheduleQuery({
        ...scheduleFixture(),
        enabled: false,
        weeklyWindows: [],
        blackouts: [],
        version: 0,
      }),
    )
    renderPanel()

    expect(screen.getByText('不限維護時段')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /新增允許時段/ })).toBeEnabled()
    expect(screen.getByRole('button', { name: /新增禁止時段/ })).toBeEnabled()
  })

  it('renders a local loading state while the policy is pending', () => {
    api.useSchedule.mockReturnValue({
      ...scheduleQuery(undefined),
      isPending: true,
    })
    const { container } = renderPanel()

    expect(container.querySelector('.ant-skeleton')).toBeInTheDocument()
    expect(screen.queryByRole('switch')).not.toBeInTheDocument()
  })

  it('shows query errors without presenting an empty editable policy', () => {
    api.useSchedule.mockReturnValue({
      ...scheduleQuery(undefined),
      isError: true,
    })
    renderPanel()

    expect(
      screen.getByText(
        '無法讀取發布排程，或你沒有目前 Environment 的查看權限。',
      ),
    ).toBeInTheDocument()
    expect(screen.queryByRole('switch')).not.toBeInTheDocument()
  })
})

function renderPanel() {
  return render(
    <App>
      <I18nextProvider i18n={i18n}>
        <DeploymentSchedulePanel environmentId="environment-1" />
      </I18nextProvider>
    </App>,
  )
}

function scheduleQuery(data: ReturnType<typeof scheduleFixture> | undefined) {
  return {
    isPending: false,
    isError: false,
    refetch: api.refetch,
    data: data ? { status: 200, data: { data } } : undefined,
  }
}

function scheduleFixture() {
  return {
    environmentId: 'environment-1',
    enabled: true,
    timeZone: 'Asia/Taipei',
    weeklyWindows: [{ dayOfWeek: 1, startMinute: 540, endMinute: 1020 }],
    blackouts: [
      {
        startsAt: '2026-09-22T02:00:00.000Z',
        endsAt: '2026-09-22T03:00:00.000Z',
      },
    ],
    version: 4,
    canManage: true,
  }
}
