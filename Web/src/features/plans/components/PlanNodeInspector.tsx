import { Button, Card, Form, Input, InputNumber, Select } from 'antd'
import { useTranslation } from 'react-i18next'

import type { DeploymentPlanNode } from '@/generated/model'

interface Props {
  node: DeploymentPlanNode
  onChange: (value: DeploymentPlanNode) => void
  onRemove: () => void
}

export function PlanNodeInspector({ node, onChange, onRemove }: Props) {
  const { t } = useTranslation()
  const change = <K extends keyof DeploymentPlanNode>(
    key: K,
    value: DeploymentPlanNode[K],
  ) => onChange({ ...node, [key]: value })
  const changeCondition = (
    key: keyof DeploymentPlanNode['successCondition'],
    value: string[] | number | undefined,
  ) =>
    onChange({
      ...node,
      successCondition: { ...node.successCondition, [key]: value },
    })
  return (
    <Card title={t('plans.inspector.node')} size="small">
      <Form layout="vertical">
        <Form.Item label={t('plans.fields.key')} required>
          <Input
            value={node.key}
            onChange={(event) => change('key', event.target.value)}
          />
        </Form.Item>
        <Form.Item label={t('plans.fields.applicationKey')} required>
          <Input
            value={node.applicationKey}
            onChange={(event) => change('applicationKey', event.target.value)}
          />
        </Form.Item>
        <Form.Item label={t('plans.fields.order')} required>
          <InputNumber
            min={0}
            value={node.order}
            onChange={(value) => change('order', value ?? 0)}
          />
        </Form.Item>
        <Form.Item label={t('plans.fields.syncStatuses')} required>
          <Select
            mode="tags"
            value={node.successCondition.syncStatuses}
            onChange={(value) => changeCondition('syncStatuses', value)}
          />
        </Form.Item>
        <Form.Item label={t('plans.fields.healthStatuses')} required>
          <Select
            mode="tags"
            value={node.successCondition.healthStatuses}
            onChange={(value) => changeCondition('healthStatuses', value)}
          />
        </Form.Item>
        <Form.Item label={t('plans.fields.stabilizationSeconds')}>
          <InputNumber
            min={0}
            value={node.successCondition.stabilizationSeconds}
            onChange={(value) =>
              changeCondition('stabilizationSeconds', value ?? 0)
            }
          />
        </Form.Item>
        <Form.Item label={t('plans.fields.timeoutSeconds')}>
          <InputNumber
            min={1}
            value={node.successCondition.timeoutSeconds}
            onChange={(value) =>
              changeCondition('timeoutSeconds', value ?? undefined)
            }
          />
        </Form.Item>
        <Button danger onClick={onRemove}>
          {t('plans.actions.removeNode')}
        </Button>
      </Form>
    </Card>
  )
}
