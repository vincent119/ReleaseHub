import {
  Alert,
  Button,
  Card,
  Form,
  Input,
  InputNumber,
  Select,
  Switch,
} from 'antd'
import { useTranslation } from 'react-i18next'

import type {
  ReleaseWorkflowState,
  ReleaseWorkflowReviewOptions,
  WorkflowReviewPolicy,
} from '@/generated/model'

interface Props {
  state: ReleaseWorkflowState
  initial: boolean
  reviewOptions?: ReleaseWorkflowReviewOptions
  reviewOptionsLoading?: boolean
  reviewOptionsError?: boolean
  onChange: (state: ReleaseWorkflowState) => void
  onMakeInitial: () => void
  onRemove: () => void
}

export function StateInspector({
  state,
  initial,
  reviewOptions,
  reviewOptionsLoading = false,
  reviewOptionsError = false,
  onChange,
  onMakeInitial,
  onRemove,
}: Props) {
  const { t } = useTranslation()
  const update = (
    _: Partial<ReleaseWorkflowState>,
    values: ReleaseWorkflowState,
  ) => {
    const next = { ...values }
    if (next.type !== 'Review') delete next.reviewPolicy
    if (next.type === 'Review' && !next.reviewPolicy)
      next.reviewPolicy = emptyPolicy()
    onChange(next)
  }
  return (
    <Card title={t('workflows.inspector.state')} size="small">
      <Form
        key={state.key}
        layout="vertical"
        initialValues={state}
        onValuesChange={update}
      >
        <Form.Item
          name="key"
          label={t('workflows.fields.key')}
          rules={[{ required: true }, { pattern: /^[a-z][a-z0-9_-]*$/ }]}
        >
          <Input />
        </Form.Item>
        <Form.Item
          name="name"
          label={t('workflows.fields.name')}
          rules={[{ required: true }]}
        >
          <Input />
        </Form.Item>
        <Form.Item
          name="type"
          label={t('workflows.fields.type')}
          rules={[{ required: true }]}
        >
          <Select
            options={[
              'Start',
              'Review',
              'ManualAction',
              'Deployment',
              'Terminal',
            ].map(option)}
          />
        </Form.Item>
        {state.type === 'Review' && (
          <ReviewPolicyFields
            policy={state.reviewPolicy ?? emptyPolicy()}
            options={reviewOptions}
            loading={reviewOptionsLoading}
            error={reviewOptionsError}
          />
        )}
      </Form>
      <Button block disabled={initial} onClick={onMakeInitial}>
        {initial
          ? t('workflows.actions.initialState')
          : t('workflows.actions.makeInitial')}
      </Button>
      <Button block danger className="rh-danger-action" onClick={onRemove}>
        {t('workflows.actions.removeState')}
      </Button>
    </Card>
  )
}

interface ReviewPolicyFieldsProps {
  policy: WorkflowReviewPolicy
  options?: ReleaseWorkflowReviewOptions
  loading: boolean
  error: boolean
}

function ReviewPolicyFields({
  policy,
  options,
  loading,
  error,
}: ReviewPolicyFieldsProps) {
  const { t } = useTranslation()
  const users = reviewUserOptions(options, policy.userIds, t)
  const roles = reviewRoleOptions(options, policy.roleIds, t)
  return (
    <>
      <Form.Item
        name={['reviewPolicy', 'type']}
        label={t('workflows.fields.reviewPolicy')}
        rules={[{ required: true }]}
      >
        <Select
          options={[
            'AnyApprover',
            'MinimumApprovals',
            'AllApprovers',
            'RoleMinimumOne',
            'SequentialStages',
          ].map(option)}
        />
      </Form.Item>
      <Form.Item
        name={['reviewPolicy', 'requiredApprovals']}
        label={t('workflows.fields.requiredApprovals')}
        rules={[{ required: true }]}
      >
        <InputNumber min={1} precision={0} style={{ width: '100%' }} />
      </Form.Item>
      <Form.Item
        name={['reviewPolicy', 'allowSelfReview']}
        label={t('workflows.fields.allowSelfReview')}
        valuePropName="checked"
      >
        <Switch />
      </Form.Item>
      <Form.Item
        name={['reviewPolicy', 'userIds']}
        label={t('workflows.fields.userIds')}
      >
        <Select
          mode="multiple"
          showSearch
          optionFilterProp="label"
          options={users}
          loading={loading}
          status={error ? 'error' : undefined}
          placeholder={t('workflows.reviewOptions.userPlaceholder')}
          notFoundContent={reviewOptionsEmptyState(loading, error, t)}
        />
      </Form.Item>
      <Form.Item
        name={['reviewPolicy', 'roleIds']}
        label={t('workflows.fields.roleIds')}
      >
        <Select
          mode="multiple"
          showSearch
          optionFilterProp="label"
          options={roles}
          loading={loading}
          status={error ? 'error' : undefined}
          placeholder={t('workflows.reviewOptions.rolePlaceholder')}
          notFoundContent={reviewOptionsEmptyState(loading, error, t)}
        />
      </Form.Item>
      {error && (
        <Alert
          type="error"
          showIcon
          title={t('workflows.reviewOptions.error')}
        />
      )}
    </>
  )
}

type Translation = (key: string, options?: Record<string, unknown>) => string

function reviewUserOptions(
  options: ReleaseWorkflowReviewOptions | undefined,
  selected: string[],
  t: Translation,
) {
  const values = (options?.users ?? [])
    .filter((item) => item.assignable || selected.includes(item.id))
    .map((item) => ({
      value: item.id,
      label: item.assignable
        ? item.username
        : t('workflows.reviewOptions.unavailable', { name: item.username }),
    }))
  return includeUnknownSelections(values, selected, t)
}

function reviewRoleOptions(
  options: ReleaseWorkflowReviewOptions | undefined,
  selected: string[],
  t: Translation,
) {
  const values = (options?.roles ?? [])
    .filter((item) => item.assignable || selected.includes(item.id))
    .map((item) => {
      const name = t('workflows.reviewOptions.roleLabel', {
        name: item.name,
        owner: item.ownerKind,
      })
      return {
        value: item.id,
        label: item.assignable
          ? name
          : t('workflows.reviewOptions.unavailable', { name }),
      }
    })
  return includeUnknownSelections(values, selected, t)
}

function includeUnknownSelections(
  values: { value: string; label: string }[],
  selected: string[],
  t: Translation,
) {
  const known = new Set(values.map((item) => item.value))
  return values.concat(
    selected
      .filter((id) => !known.has(id))
      .map((id) => ({
        value: id,
        label: t('workflows.reviewOptions.unavailable', { name: id }),
      })),
  )
}

function reviewOptionsEmptyState(
  loading: boolean,
  error: boolean,
  t: Translation,
) {
  if (loading) return t('workflows.reviewOptions.loading')
  if (error) return t('workflows.reviewOptions.error')
  return t('workflows.reviewOptions.empty')
}

function emptyPolicy(): WorkflowReviewPolicy {
  return {
    type: 'AnyApprover',
    requiredApprovals: 1,
    allowSelfReview: false,
    userIds: [],
    roleIds: [],
  }
}

function option(value: string) {
  return { value, label: value }
}
