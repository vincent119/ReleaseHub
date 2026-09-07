import { Button, Card, Form, Input, Select } from 'antd'
import { useTranslation } from 'react-i18next'

import type {
  ReleaseWorkflowTransition,
  WorkflowCondition,
} from '@/generated/model'

interface Props {
  transition: ReleaseWorkflowTransition
  states: string[]
  onChange: (transition: ReleaseWorkflowTransition) => void
  onRemove: () => void
}

export function TransitionInspector({
  transition,
  states,
  onChange,
  onRemove,
}: Props) {
  const { t } = useTranslation()
  return (
    <Card title={t('workflows.inspector.transition')} size="small">
      <Form
        key={transition.key}
        layout="vertical"
        initialValues={transition}
        onValuesChange={(_, values) => onChange(values)}
      >
        <Form.Item
          name="key"
          label={t('workflows.fields.key')}
          rules={[{ required: true }, { pattern: /^[a-z][a-z0-9_-]*$/ }]}
        >
          <Input />
        </Form.Item>
        <Form.Item
          name="from"
          label={t('workflows.fields.from')}
          rules={[{ required: true }]}
        >
          <Select options={states.map(option)} />
        </Form.Item>
        <Form.Item
          name="to"
          label={t('workflows.fields.to')}
          rules={[{ required: true }]}
        >
          <Select options={states.map(option)} />
        </Form.Item>
        <Form.Item
          name="trigger"
          label={t('workflows.fields.trigger')}
          rules={[{ required: true }]}
        >
          <Select
            options={[
              'Automatic',
              'Manual',
              'ReviewSatisfied',
              'DeploymentResult',
            ].map(option)}
          />
        </Form.Item>
        <Form.Item
          name="permission"
          label={t('workflows.fields.permission')}
          rules={[
            { required: true },
            { pattern: /^[a-z][a-z0-9_]*(\.[a-z][a-z0-9_]*)+$/ },
          ]}
        >
          <Input />
        </Form.Item>
        <Form.List name="conditions">
          {(fields, { add, remove }) => (
            <>
              {fields.map((field) => (
                <ConditionFields
                  key={field.key}
                  name={field.name}
                  remove={() => remove(field.name)}
                />
              ))}
              <Button block onClick={() => add(emptyCondition())}>
                {t('workflows.actions.addCondition')}
              </Button>
            </>
          )}
        </Form.List>
      </Form>
      <Button block danger className="rh-danger-action" onClick={onRemove}>
        {t('workflows.actions.removeTransition')}
      </Button>
    </Card>
  )
}

function ConditionFields({
  name,
  remove,
}: {
  name: number
  remove: () => void
}) {
  const { t } = useTranslation()
  return (
    <Card size="small">
      <Form.Item
        name={[name, 'fact']}
        label={t('workflows.fields.fact')}
        rules={[{ required: true }]}
      >
        <Select
          options={[
            'review.status',
            'deployment.status',
            'deployment.syncStatus',
            'deployment.healthStatus',
            'request.classification',
          ].map(option)}
        />
      </Form.Item>
      <Form.Item
        name={[name, 'operator']}
        label={t('workflows.fields.operator')}
        rules={[{ required: true }]}
      >
        <Select options={['Equals', 'NotEquals', 'In', 'NotIn'].map(option)} />
      </Form.Item>
      <Form.Item
        name={[name, 'value']}
        label={t('workflows.fields.value')}
        rules={[{ required: true }]}
      >
        <Select mode="tags" tokenSeparators={[',']} />
      </Form.Item>
      <Button danger onClick={remove}>
        {t('workflows.actions.removeCondition')}
      </Button>
    </Card>
  )
}

function emptyCondition(): WorkflowCondition {
  return { fact: 'review.status', operator: 'Equals', value: [] }
}

function option(value: string) {
  return { value, label: value }
}
