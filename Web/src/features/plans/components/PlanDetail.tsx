import { Button, Card, Empty, Select, Space, Tag } from 'antd'
import { useTranslation } from 'react-i18next'

import type {
  DefinitionLifecycle,
  DeploymentPlan,
  DeploymentPlanVersion,
} from '@/generated/model'

import styles from '../PlansPage.module.css'
import { PlanGraphEditor } from './PlanGraphEditor'

interface Props {
  plan?: DeploymentPlan
  version?: DeploymentPlanVersion
  versionID?: string
  submitting: boolean
  onVersion: (id: string) => void
  onNewVersion: () => void
  onLifecycle: (value: 'Published' | 'Disabled') => void
}

export function PlanDetail(props: Props) {
  const { t } = useTranslation()
  if (!props.plan || !props.version)
    return (
      <Card>
        <Empty description={t('plans.list.select')} />
      </Card>
    )
  return (
    <Card title={props.plan.name} extra={<PlanActions {...props} />}>
      <Space orientation="vertical" className={styles.detail}>
        <Tag color={lifecycleColor(props.version.lifecycle)}>
          {props.version.lifecycle}
        </Tag>
        <PlanGraphEditor
          key={props.version.id}
          initialDocument={props.version.document}
          readOnly
        />
      </Space>
    </Card>
  )
}

function PlanActions(props: Props) {
  const { t } = useTranslation()
  const plan = props.plan!
  const version = props.version!
  return (
    <Space>
      <Select
        value={props.versionID ?? version.id}
        options={plan.versions.map((item) => ({
          value: item.id,
          label: `v${item.versionNumber} · ${item.lifecycle}`,
        }))}
        onChange={props.onVersion}
      />
      <Button onClick={props.onNewVersion}>
        {t('plans.actions.newVersion')}
      </Button>
      {version.lifecycle === 'Draft' && (
        <Button
          type="primary"
          loading={props.submitting}
          onClick={() => props.onLifecycle('Published')}
        >
          {t('plans.actions.publish')}
        </Button>
      )}
      {version.lifecycle === 'Published' && (
        <Button
          danger
          loading={props.submitting}
          onClick={() => props.onLifecycle('Disabled')}
        >
          {t('plans.actions.disable')}
        </Button>
      )}
    </Space>
  )
}

function lifecycleColor(value: DefinitionLifecycle) {
  return value === 'Published'
    ? 'green'
    : value === 'Disabled'
      ? 'default'
      : 'blue'
}
