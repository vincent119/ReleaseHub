import { Button, Form, Input, Select } from 'antd'
import { useId } from 'react'
import { useTranslation } from 'react-i18next'

import { DeploymentRequestStatus } from '@/generated/model'

import type {
  RequestListCriteria,
  RequestListCriteriaAction,
} from '../model/listQuery'
import { requestStatusLabel } from '../model/presentation'
import styles from '../RequestsPage.module.css'

export function RequestListFilters({
  criteria,
  disabled,
  dispatch,
}: {
  criteria: RequestListCriteria
  disabled: boolean
  dispatch: (action: RequestListCriteriaAction) => void
}) {
  const { t } = useTranslation()
  const [form] = Form.useForm<{ search: string }>()
  const statusID = useId()
  const limitID = useId()
  return (
    <Form
      form={form}
      name="request-list-filters"
      layout="vertical"
      className={styles.queryFilters}
      initialValues={{ search: criteria.search }}
      onFinish={({ search }) => dispatch({ type: 'search', value: search })}
    >
      <Form.Item
        name="search"
        label={t('requests.query.search')}
        rules={[
          {
            validator: (_, value: string) =>
              Array.from((value ?? '').trim()).length <= 255
                ? Promise.resolve()
                : Promise.reject(new Error(t('requests.query.searchTooLong'))),
          },
        ]}
      >
        <Input
          allowClear
          disabled={disabled}
          placeholder={t('requests.query.searchPlaceholder')}
        />
      </Form.Item>
      <Form.Item label={t('requests.query.status')} htmlFor={statusID}>
        <Select
          id={statusID}
          virtual={false}
          aria-label={t('requests.query.status')}
          disabled={disabled}
          value={criteria.status ?? ''}
          options={[
            { value: '', label: t('requests.query.allStatuses') },
            ...Object.values(DeploymentRequestStatus).map((value) => ({
              value,
              label: requestStatusLabel(value),
            })),
          ]}
          onChange={(value: DeploymentRequestStatus | '') =>
            dispatch({ type: 'status', value: value || undefined })
          }
        />
      </Form.Item>
      <Form.Item label={t('requests.query.pageSize')} htmlFor={limitID}>
        <Select<RequestListCriteria['limit']>
          id={limitID}
          virtual={false}
          aria-label={t('requests.query.pageSize')}
          disabled={disabled}
          value={criteria.limit}
          options={([20, 50, 100] as const).map((value) => ({
            value,
            label: t('requests.query.pageSizeOption', { count: value }),
          }))}
          onChange={(value) => dispatch({ type: 'limit', value })}
        />
      </Form.Item>
      <div className={styles.queryActions}>
        <Button type="primary" htmlType="submit" disabled={disabled}>
          {t('requests.query.apply')}
        </Button>
        <Button
          disabled={disabled}
          onClick={() => {
            form.setFieldsValue({ search: '' })
            dispatch({ type: 'clear' })
          }}
        >
          {t('requests.query.clear')}
        </Button>
      </div>
    </Form>
  )
}
