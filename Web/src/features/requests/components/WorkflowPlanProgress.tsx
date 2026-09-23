import { Card, Col, Empty, Row, Space, Steps, Tag, Typography } from 'antd'
import { useTranslation } from 'react-i18next'

import type {
  DeploymentExecution,
  DeploymentPlan,
  DeploymentPlanVersion,
  DeploymentRequestVersion,
  DeploymentRequestStatus,
  ReleaseWorkflow,
  ReleaseWorkflowVersion,
} from '@/generated/model'
import { SemanticList, SemanticListItemContent } from '@/shared/list'

import { nodeStatusColor } from '../model/presentation'
import { buildWorkflowProgress } from '../model/workflowProgress'

interface Props {
  request: DeploymentRequestVersion
  workflows: ReleaseWorkflow[]
  plans: DeploymentPlan[]
  execution?: DeploymentExecution
}

export function WorkflowPlanProgress({
  request,
  workflows,
  plans,
  execution,
}: Props) {
  const workflow = workflowVersion(workflows, request.workflowVersionId)
  const plan = planVersion(plans, request.planVersionId)
  return (
    <Row gutter={[16, 16]}>
      <Col xs={24} xl={10}>
        <WorkflowProgress
          version={workflow}
          current={request.workflowStateKey}
          requestStatus={request.status}
        />
      </Col>
      <Col xs={24} xl={14}>
        <PlanProgress version={plan} execution={execution} />
      </Col>
    </Row>
  )
}

export function WorkflowProgress({
  version,
  current,
  requestStatus,
}: {
  version?: ReleaseWorkflowVersion
  current?: string
  requestStatus: DeploymentRequestStatus
}) {
  const { t } = useTranslation()
  if (!version)
    return (
      <Card title={t('requestDetail.workflow.title')}>
        <Empty />
      </Card>
    )
  const { stages, currentIndex } = buildWorkflowProgress(
    version.document,
    current,
    {
      resultTitle: t('requestDetail.workflow.resultTitle'),
      resultPending: t('requestDetail.workflow.resultPending'),
    },
  )
  return (
    <Card title={t('requestDetail.workflow.title')}>
      <Steps
        orientation="vertical"
        size="small"
        current={currentIndex}
        status={workflowStepStatus(requestStatus)}
        items={stages.map((stage) => ({
          title: stage.title,
          content: stage.content,
        }))}
      />
    </Card>
  )
}

function workflowStepStatus(status: DeploymentRequestStatus) {
  if (status === 'Failed') return 'error'
  if (status === 'Succeeded') return 'finish'
  return 'process'
}

function PlanProgress({
  version,
  execution,
}: {
  version?: DeploymentPlanVersion
  execution?: DeploymentExecution
}) {
  const { t } = useTranslation()
  if (!version)
    return (
      <Card title={t('requestDetail.plan.title')}>
        <Empty />
      </Card>
    )
  const statuses = new Map(execution?.nodes.map((node) => [node.nodeKey, node]))
  return (
    <Card title={t('requestDetail.plan.title')}>
      <SemanticList
        items={[...version.document.nodes].sort((a, b) => a.order - b.order)}
        rowKey="key"
        renderItem={(node) => {
          const runtime = statuses.get(node.key)
          const upstream = version.document.edges.filter(
            (edge) => edge.to === node.key,
          )
          return (
            <SemanticListItemContent
              extra={
                <Tag color={nodeStatusColor(runtime?.status ?? 'Waiting')}>
                  {runtime?.status ?? 'Waiting'}
                </Tag>
              }
              title={`${node.order + 1}. ${node.applicationKey}`}
              description={
                <Space orientation="vertical" size={0}>
                  <Typography.Text type="secondary">
                    {upstream.length
                      ? t('requestDetail.plan.dependsOn', {
                          nodes: upstream.map((edge) => edge.from).join(', '),
                        })
                      : t('requestDetail.plan.independent')}
                  </Typography.Text>
                  {runtime?.healthStatus && (
                    <Typography.Text>
                      {runtime.syncStatus} · {runtime.healthStatus}
                    </Typography.Text>
                  )}
                </Space>
              }
            />
          )
        }}
      />
    </Card>
  )
}

function workflowVersion(values: ReleaseWorkflow[], id: string) {
  return values
    .flatMap((value) => value.versions)
    .find((version) => version.id === id)
}

function planVersion(values: DeploymentPlan[], id: string) {
  return values
    .flatMap((value) => value.versions)
    .find((version) => version.id === id)
}
