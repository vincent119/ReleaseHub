import { Button, Card, Empty, Flex, Select, Tag, Tooltip } from 'antd'
import { useTranslation } from 'react-i18next'

import type {
  DefinitionLifecycle,
  DeploymentPlan,
  DeploymentPlanVersion,
} from '@/generated/model'
import definitionStyles from '@/shared/definition/DefinitionWorkspace.module.css'

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
    <Card
      className={definitionStyles.detailCard}
      title={
        <div className={definitionStyles.detailHeading}>
          <span className={definitionStyles.detailTitle}>
            {props.plan.name}
          </span>
          <span className={definitionStyles.detailMeta}>
            <Tag color={lifecycleColor(props.version.lifecycle)}>
              {props.version.lifecycle}
            </Tag>
            <span>v{props.version.versionNumber}</span>
          </span>
        </div>
      }
      extra={<PlanActions {...props} />}
    >
      <div className={definitionStyles.detail}>
        <PlanGraphEditor
          key={props.version.id}
          initialDocument={props.version.document}
          readOnly
        />
      </div>
    </Card>
  )
}

function PlanActions(props: Props) {
  const { t } = useTranslation()
  const plan = props.plan!
  const version = props.version!
  const hasDraft = plan.versions.some((item) => item.lifecycle === 'Draft')
  return (
    <Flex className={definitionStyles.actions}>
      <div className={definitionStyles.actionGroup}>
        <Select
          value={props.versionID ?? version.id}
          options={plan.versions.map((item) => ({
            value: item.id,
            label: `v${item.versionNumber} · ${item.lifecycle}`,
          }))}
          onChange={props.onVersion}
        />
        <Tooltip
          title={hasDraft ? t('plans.actions.newVersionDraftHint') : undefined}
        >
          <span>
            <Button disabled={hasDraft} onClick={props.onNewVersion}>
              {t('plans.actions.newVersion')}
            </Button>
          </span>
        </Tooltip>
      </div>
      <div className={definitionStyles.actionGroup}>
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
      </div>
    </Flex>
  )
}

function lifecycleColor(value: DefinitionLifecycle) {
  return value === 'Published'
    ? 'green'
    : value === 'Disabled'
      ? 'default'
      : 'blue'
}
