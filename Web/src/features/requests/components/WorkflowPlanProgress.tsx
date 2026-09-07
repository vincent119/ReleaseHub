import {
  Card,
  Col,
  Empty,
  List,
  Row,
  Space,
  Steps,
  Tag,
  Typography,
} from 'antd'
import { useTranslation } from 'react-i18next'

import type {
  DeploymentExecution,
  DeploymentPlan,
  DeploymentPlanVersion,
  DeploymentRequestVersion,
  ReleaseWorkflow,
  ReleaseWorkflowVersion,
} from '@/generated/model'

import { nodeStatusColor } from '../model/presentation'

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
        />
      </Col>
      <Col xs={24} xl={14}>
        <PlanProgress version={plan} execution={execution} />
      </Col>
    </Row>
  )
}

function WorkflowProgress({
  version,
  current,
}: {
  version?: ReleaseWorkflowVersion
  current?: string
}) {
  const { t } = useTranslation()
  if (!version)
    return (
      <Card title={t('requestDetail.workflow.title')}>
        <Empty />
      </Card>
    )
  const states = version.document.states
  const currentIndex = Math.max(
    0,
    states.findIndex((state) => state.key === current),
  )
  return (
    <Card title={t('requestDetail.workflow.title')}>
      <Steps
        direction="vertical"
        size="small"
        current={currentIndex}
        items={states.map((state) => ({
          title: state.name,
          description: state.type,
        }))}
      />
    </Card>
  )
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
      <List
        dataSource={[...version.document.nodes].sort(
          (a, b) => a.order - b.order,
        )}
        renderItem={(node) => {
          const runtime = statuses.get(node.key)
          const upstream = version.document.edges.filter(
            (edge) => edge.to === node.key,
          )
          return (
            <List.Item
              extra={
                <Tag color={nodeStatusColor(runtime?.status ?? 'Waiting')}>
                  {runtime?.status ?? 'Waiting'}
                </Tag>
              }
            >
              <List.Item.Meta
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
            </List.Item>
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
