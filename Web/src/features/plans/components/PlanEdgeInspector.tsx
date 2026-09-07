import { Button, Card, Form, Select } from 'antd'
import { useTranslation } from 'react-i18next'

import type { DeploymentPlanEdge } from '@/generated/model'

interface Props {
  edge: DeploymentPlanEdge
  nodeKeys: string[]
  onChange: (value: DeploymentPlanEdge) => void
  onRemove: () => void
}

export function PlanEdgeInspector({
  edge,
  nodeKeys,
  onChange,
  onRemove,
}: Props) {
  const { t } = useTranslation()
  const options = nodeKeys.map((value) => ({ value, label: value }))
  return (
    <Card title={t('plans.inspector.edge')} size="small">
      <Form layout="vertical">
        <Form.Item label={t('plans.fields.from')} required>
          <Select
            value={edge.from}
            options={options}
            onChange={(from) => onChange({ ...edge, from })}
          />
        </Form.Item>
        <Form.Item label={t('plans.fields.to')} required>
          <Select
            value={edge.to}
            options={options}
            onChange={(to) => onChange({ ...edge, to })}
          />
        </Form.Item>
        <Form.Item label={t('plans.fields.condition')} required>
          <Select
            value={edge.condition}
            options={[
              { value: 'UpstreamSucceeded', label: 'UpstreamSucceeded' },
              { value: 'PrerequisiteHealthy', label: 'PrerequisiteHealthy' },
            ]}
            onChange={(condition) => onChange({ ...edge, condition })}
          />
        </Form.Item>
        <Button danger onClick={onRemove}>
          {t('plans.actions.removeEdge')}
        </Button>
      </Form>
    </Card>
  )
}
