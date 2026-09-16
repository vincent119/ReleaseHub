import {
  CalendarOutlined,
  DeleteOutlined,
  PlusOutlined,
} from '@ant-design/icons'
import {
  Alert,
  Button,
  Card,
  Divider,
  Flex,
  Form,
  Input,
  Select,
  Space,
  Switch,
  Tag,
  Typography,
} from 'antd'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import {
  updateDeploymentSchedule,
  useGetDeploymentSchedule,
} from '@/generated/api'
import type {
  DeploymentSchedule,
  UpdateDeploymentScheduleRequest,
} from '@/generated/model'
import { parseAPIErrorResponse } from '@/shared/api/apiError'
import { useFeedback } from '@/shared/feedback/useFeedback'

import styles from './DeploymentSchedulePanel.module.css'

interface Props {
  environmentId: string
}

interface ScheduleForm {
  enabled: boolean
  timeZone: string
  weeklyWindows: Array<{
    dayOfWeek: number
    startTime: string
    endTime: string
  }>
  blackouts: Array<{ startsAt: string; endsAt: string }>
}

export function DeploymentSchedulePanel({ environmentId }: Props) {
  const { t } = useTranslation()
  const feedback = useFeedback()
  const [form] = Form.useForm<ScheduleForm>()
  const [submitting, setSubmitting] = useState(false)
  const query = useGetDeploymentSchedule(environmentId)
  const schedule = query.data?.status === 200 ? query.data.data.data : undefined

  const submit = async (values: ScheduleForm) => {
    if (!schedule?.canManage) return
    const csrf = browserCookie('releasehub_csrf')
    if (!csrf) return void feedback.error(t('plans.schedule.mutation.error'))
    setSubmitting(true)
    try {
      const response = await updateDeploymentSchedule(
        environmentId,
        scheduleInput(values, schedule.version),
        {
          headers: {
            'X-CSRF-Token': csrf,
            'Idempotency-Key': crypto.randomUUID(),
          },
        },
      )
      if (response.status !== 200) {
        const error = parseAPIErrorResponse(response)
        return void feedback.error(
          t(
            error.status === 409
              ? 'plans.schedule.mutation.conflict'
              : error.status === 422 || error.status === 400
                ? 'plans.schedule.mutation.invalid'
                : 'plans.schedule.mutation.error',
          ),
        )
      }
      feedback.success(t('plans.schedule.mutation.saved'))
      await query.refetch()
    } catch {
      feedback.error(t('plans.schedule.mutation.error'))
    } finally {
      setSubmitting(false)
    }
  }

  if (query.isError || (query.data && query.data.status !== 200)) {
    return (
      <Card title={t('plans.schedule.title')}>
        <Alert type="error" showIcon title={t('plans.schedule.unavailable')} />
      </Card>
    )
  }

  return (
    <Card
      className={styles.card}
      loading={query.isPending}
      title={
        <Flex align="center" gap="small">
          <CalendarOutlined />
          <span>{t('plans.schedule.title')}</span>
        </Flex>
      }
      extra={
        schedule ? (
          <Space>
            <Tag color={schedule.enabled ? 'cyan' : 'default'}>
              {t(
                schedule.enabled
                  ? 'plans.schedule.status.enabled'
                  : 'plans.schedule.status.disabled',
              )}
            </Tag>
            <Typography.Text type="secondary">
              {t('plans.schedule.version', { version: schedule.version })}
            </Typography.Text>
          </Space>
        ) : null
      }
    >
      {schedule && (
        <>
          {!schedule.canManage && (
            <Alert
              className={styles.notice}
              type="info"
              showIcon
              title={t('plans.schedule.readOnly')}
            />
          )}
          <Typography.Paragraph type="secondary">
            {t('plans.schedule.description')}
          </Typography.Paragraph>
          <Form<ScheduleForm>
            key={`${environmentId}:${schedule.version}`}
            form={form}
            layout="vertical"
            disabled={!schedule.canManage}
            initialValues={scheduleValues(schedule)}
            onFinish={(values) => void submit(values)}
          >
            <Flex className={styles.policyHeader} gap="large" wrap>
              <Form.Item
                name="enabled"
                label={t('plans.schedule.fields.enabled')}
                valuePropName="checked"
              >
                <Switch />
              </Form.Item>
              <Form.Item
                className={styles.timeZone}
                name="timeZone"
                label={t('plans.schedule.fields.timeZone')}
                rules={[
                  {
                    required: true,
                    message: t('plans.schedule.validation.timeZone'),
                  },
                  {
                    validator: (_, value: string) =>
                      !value || validTimeZone(value.trim())
                        ? Promise.resolve()
                        : Promise.reject(
                            new Error(t('plans.schedule.validation.timeZone')),
                          ),
                  },
                ]}
              >
                <Input placeholder="Asia/Taipei" maxLength={255} />
              </Form.Item>
            </Flex>

            <Divider titlePlacement="start">
              {t('plans.schedule.weekly.title')}
            </Divider>
            <Typography.Paragraph type="secondary">
              {t('plans.schedule.weekly.description')}
            </Typography.Paragraph>
            <Form.List
              name="weeklyWindows"
              rules={[
                {
                  validator: (_, windows) =>
                    form.getFieldValue('enabled') && !windows?.length
                      ? Promise.reject(
                          new Error(t('plans.schedule.validation.window')),
                        )
                      : Promise.resolve(),
                },
              ]}
            >
              {(fields, { add, remove }, { errors }) => (
                <Space orientation="vertical" className={styles.list}>
                  {fields.map(({ key, ...field }) => (
                    <Flex
                      key={key}
                      className={styles.row}
                      gap="small"
                      align="start"
                      wrap
                    >
                      <Form.Item
                        {...field}
                        name={[field.name, 'dayOfWeek']}
                        label={t('plans.schedule.fields.day')}
                        rules={[{ required: true }]}
                      >
                        <Select
                          className={styles.day}
                          options={dayOptions(t)}
                        />
                      </Form.Item>
                      <Form.Item
                        {...field}
                        name={[field.name, 'startTime']}
                        label={t('plans.schedule.fields.startTime')}
                        rules={[{ required: true }]}
                      >
                        <Input type="time" />
                      </Form.Item>
                      <Form.Item
                        {...field}
                        name={[field.name, 'endTime']}
                        label={t('plans.schedule.fields.endTime')}
                        dependencies={[
                          ['weeklyWindows', field.name, 'startTime'],
                        ]}
                        rules={[
                          { required: true },
                          {
                            validator: (_, endTime: string) => {
                              const startTime = form.getFieldValue([
                                'weeklyWindows',
                                field.name,
                                'startTime',
                              ])
                              return !startTime ||
                                !endTime ||
                                endTime > startTime
                                ? Promise.resolve()
                                : Promise.reject(
                                    new Error(
                                      t('plans.schedule.validation.timeRange'),
                                    ),
                                  )
                            },
                          },
                        ]}
                      >
                        <Input type="time" />
                      </Form.Item>
                      <Button
                        className={styles.remove}
                        danger
                        type="text"
                        icon={<DeleteOutlined />}
                        aria-label={t('plans.schedule.actions.removeWindow')}
                        disabled={!schedule.canManage}
                        onClick={() => remove(field.name)}
                      />
                    </Flex>
                  ))}
                  <Form.ErrorList errors={errors} />
                  <Button
                    className={styles.add}
                    type="dashed"
                    icon={<PlusOutlined />}
                    disabled={!schedule.canManage || fields.length >= 32}
                    onClick={() =>
                      add({
                        dayOfWeek: 1,
                        startTime: '09:00',
                        endTime: '17:00',
                      })
                    }
                  >
                    {t('plans.schedule.actions.addWindow')}
                  </Button>
                </Space>
              )}
            </Form.List>

            <Divider titlePlacement="start">
              {t('plans.schedule.blackouts.title')}
            </Divider>
            <Typography.Paragraph type="secondary">
              {t('plans.schedule.blackouts.description')}
            </Typography.Paragraph>
            <Form.List name="blackouts">
              {(fields, { add, remove }) => (
                <Space orientation="vertical" className={styles.list}>
                  {fields.map(({ key, ...field }) => (
                    <Flex
                      key={key}
                      className={styles.row}
                      gap="small"
                      align="start"
                      wrap
                    >
                      <Form.Item
                        {...field}
                        name={[field.name, 'startsAt']}
                        label={t('plans.schedule.fields.startsAt')}
                        rules={[{ required: true }]}
                      >
                        <Input type="datetime-local" />
                      </Form.Item>
                      <Form.Item
                        {...field}
                        name={[field.name, 'endsAt']}
                        label={t('plans.schedule.fields.endsAt')}
                        rules={[
                          { required: true },
                          {
                            validator: (_, endsAt: string) => {
                              const startsAt = form.getFieldValue([
                                'blackouts',
                                field.name,
                                'startsAt',
                              ])
                              return !startsAt || !endsAt || endsAt > startsAt
                                ? Promise.resolve()
                                : Promise.reject(
                                    new Error(
                                      t('plans.schedule.validation.timeRange'),
                                    ),
                                  )
                            },
                          },
                        ]}
                      >
                        <Input type="datetime-local" />
                      </Form.Item>
                      <Button
                        className={styles.remove}
                        danger
                        type="text"
                        icon={<DeleteOutlined />}
                        aria-label={t('plans.schedule.actions.removeBlackout')}
                        disabled={!schedule.canManage}
                        onClick={() => remove(field.name)}
                      />
                    </Flex>
                  ))}
                  <Button
                    className={styles.add}
                    type="dashed"
                    icon={<PlusOutlined />}
                    disabled={!schedule.canManage || fields.length >= 64}
                    onClick={() => add({ startsAt: '', endsAt: '' })}
                  >
                    {t('plans.schedule.actions.addBlackout')}
                  </Button>
                </Space>
              )}
            </Form.List>

            {schedule.canManage && (
              <Flex className={styles.actions} justify="end">
                <Button type="primary" htmlType="submit" loading={submitting}>
                  {t('plans.schedule.actions.save')}
                </Button>
              </Flex>
            )}
          </Form>
        </>
      )}
    </Card>
  )
}

function scheduleValues(schedule: DeploymentSchedule): ScheduleForm {
  return {
    enabled: schedule.enabled,
    timeZone: schedule.timeZone,
    weeklyWindows: schedule.weeklyWindows.map((window) => ({
      dayOfWeek: window.dayOfWeek,
      startTime: minuteToTime(window.startMinute),
      endTime: minuteToTime(window.endMinute),
    })),
    blackouts: schedule.blackouts.map((blackout) => ({
      startsAt: dateTimeLocal(blackout.startsAt),
      endsAt: dateTimeLocal(blackout.endsAt),
    })),
  }
}

function scheduleInput(
  values: ScheduleForm,
  expectedVersion: number,
): UpdateDeploymentScheduleRequest {
  return {
    enabled: values.enabled,
    timeZone: values.timeZone.trim(),
    weeklyWindows: values.weeklyWindows.map((window) => ({
      dayOfWeek: window.dayOfWeek,
      startMinute: timeToMinute(window.startTime),
      endMinute: timeToMinute(window.endTime),
    })),
    blackouts: values.blackouts.map((blackout) => ({
      startsAt: new Date(blackout.startsAt).toISOString(),
      endsAt: new Date(blackout.endsAt).toISOString(),
    })),
    expectedVersion,
  }
}

function minuteToTime(value: number) {
  const hours = Math.floor(value / 60)
  const minutes = value % 60
  return `${String(hours).padStart(2, '0')}:${String(minutes).padStart(2, '0')}`
}

function timeToMinute(value: string) {
  const [hours, minutes] = value.split(':').map(Number)
  return hours * 60 + minutes
}

function dateTimeLocal(value: string) {
  const date = new Date(value)
  const local = new Date(date.getTime() - date.getTimezoneOffset() * 60_000)
  return local.toISOString().slice(0, 16)
}

function dayOptions(t: ReturnType<typeof useTranslation>['t']) {
  return Array.from({ length: 7 }, (_, dayOfWeek) => ({
    value: dayOfWeek,
    label: t(`plans.schedule.weekly.days.${dayOfWeek}`),
  }))
}

function validTimeZone(value: string) {
  try {
    new Intl.DateTimeFormat(undefined, { timeZone: value })
    return true
  } catch {
    return false
  }
}

function browserCookie(name: string): string | undefined {
  return document.cookie
    .split('; ')
    .find((value) => value.startsWith(`${name}=`))
    ?.slice(name.length + 1)
}
