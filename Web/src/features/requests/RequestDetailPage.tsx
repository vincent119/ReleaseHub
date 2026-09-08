import { Alert, Breadcrumb, Flex, Skeleton, Space, Tag, Typography } from 'antd'
import { Link, useParams } from 'react-router'
import { useTranslation } from 'react-i18next'

import {
  useGetDeploymentExecution,
  useGetDeploymentRequest,
  useListDeploymentPlans,
  useListReleaseWorkflows,
} from '@/generated/api'
import type { DeploymentPlan, ReleaseWorkflow } from '@/generated/model'
import { ReviewDecisionPanel } from '@/features/workflows'

import { ApplicationSnapshots } from './components/ApplicationSnapshots'
import { DeploymentHistoryPanel } from './components/DeploymentHistoryPanel'
import { ExecutionPanel } from './components/ExecutionPanel'
import { RequestOverview } from './components/RequestOverview'
import { TransitionPanel } from './components/TransitionPanel'
import { WorkflowPlanProgress } from './components/WorkflowPlanProgress'
import { requestStatusColor, requestStatusLabel } from './model/presentation'
import styles from './RequestDetailPage.module.css'

const emptyID = '00000000-0000-0000-0000-000000000000'

export function RequestDetailPage() {
  const { t } = useTranslation()
  const { requestId = '' } = useParams()
  const requestQuery = useGetDeploymentRequest(requestId, {
    query: { enabled: Boolean(requestId) },
  })
  const request =
    requestQuery.data?.status === 200 ? requestQuery.data.data.data : undefined
  const execution = useGetDeploymentExecution(request?.executionId ?? emptyID, {
    query: { enabled: Boolean(request?.executionId), refetchInterval: 3000 },
  })
  const workflows = useListReleaseWorkflows()
  const plans = useListDeploymentPlans(
    { projectId: request?.projectId ?? emptyID },
    { query: { enabled: Boolean(request?.projectId) } },
  )
  if (requestQuery.isPending) return <Skeleton active />
  if (!request || requestQuery.isError) {
    return (
      <Alert type="error" showIcon message={t('requestDetail.unavailable')} />
    )
  }
  const workflowValues: ReleaseWorkflow[] =
    workflows.data?.status === 200 ? workflows.data.data.data : []
  const planValues: DeploymentPlan[] =
    plans.data?.status === 200 ? plans.data.data.data : []
  const executionValue =
    execution.data?.status === 200 ? execution.data.data.data : undefined
  const refresh = async () => {
    await Promise.all([requestQuery.refetch(), execution.refetch()])
  }
  const workflow = workflowValues
    .flatMap((value) => value.versions)
    .find((value) => value.id === request.workflowVersionId)
  return (
    <Space orientation="vertical" size="large" className={styles.page}>
      <Breadcrumb
        items={[
          { title: <Link to="/requests">{t('requests.title')}</Link> },
          { title: request.title },
        ]}
      />
      <Flex align="center" gap="middle" wrap>
        <Typography.Title level={2} style={{ margin: 0 }}>
          {request.title}
        </Typography.Title>
        <Tag color={requestStatusColor(request.status)}>
          {requestStatusLabel(request.status)}
        </Tag>
        {request.classification === 'ForwardRollback' && (
          <Tag color="purple">Forward Rollback</Tag>
        )}
      </Flex>
      <RequestOverview request={request} onUpdated={refresh} />
      <TransitionPanel
        request={request}
        workflow={workflow}
        onUpdated={refresh}
      />
      <ReviewDecisionPanel
        request={request}
        canReview={request.capabilities.includes('deployment_request.review')}
        canReassign={request.capabilities.includes(
          'deployment_request.reassign',
        )}
        onUpdated={() => void refresh()}
      />
      <WorkflowPlanProgress
        request={request}
        workflows={workflowValues}
        plans={planValues}
        execution={executionValue}
      />
      <ExecutionPanel
        request={request}
        execution={executionValue}
        unavailable={
          execution.isError ||
          Boolean(execution.data && execution.data.status !== 200)
        }
        onUpdated={refresh}
      />
      <ApplicationSnapshots applications={request.applications} />
      <DeploymentHistoryPanel
        projectId={request.projectId}
        environmentId={request.environmentId}
        enabled={request.capabilities.includes('deployment_history.view')}
      />
    </Space>
  )
}
