import { Button, Card, Form, Input, InputNumber, Select, Switch } from 'antd'
import { useTranslation } from 'react-i18next'

import type {
  ReleaseWorkflowState,
  WorkflowReviewPolicy,
} from '@/generated/model'

interface Props {
  state: ReleaseWorkflowState
  initial: boolean
  onChange: (state: ReleaseWorkflowState) => void
  onMakeInitial: () => void
  onRemove: () => void
}

export function StateInspector({
  state,
  initial,
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
        {state.type === 'Review' && <ReviewPolicyFields />}
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

function ReviewPolicyFields() {
  const { t } = useTranslation()
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
        <Select mode="tags" tokenSeparators={[',']} />
      </Form.Item>
      <Form.Item
        name={['reviewPolicy', 'roleIds']}
        label={t('workflows.fields.roleIds')}
      >
        <Select mode="tags" tokenSeparators={[',']} />
      </Form.Item>
    </>
  )
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
