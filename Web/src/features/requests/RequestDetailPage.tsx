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
import { RequestProgressSummary } from './components/RequestProgressSummary'
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
      <Alert type="error" showIcon title={t('requestDetail.unavailable')} />
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
  const pendingReview = request.reviews.some(
    (review) =>
      review.status === 'Pending' || review.status === 'ReassignmentRequired',
  )
  const actionableReview =
    pendingReview &&
    (request.capabilities.includes('deployment_request.review') ||
      request.capabilities.includes('deployment_request.reassign'))
  const reviewPanel = pendingReview && (
    <div id="request-review" className={styles.sectionAnchor} tabIndex={-1}>
      <ReviewDecisionPanel
        request={request}
        canReview={request.capabilities.includes('deployment_request.review')}
        canReassign={request.capabilities.includes(
          'deployment_request.reassign',
        )}
        onUpdated={() => void refresh()}
      />
    </div>
  )
  return (
    <Space orientation="vertical" size="large" className={styles.page}>
      <Breadcrumb
        items={[
          { title: <Link to="/requests">{t('requests.title')}</Link> },
          {
            title: (
              <span className={styles.breadcrumbCurrent} title={request.title}>
                {request.title}
              </span>
            ),
          },
        ]}
      />
      <Flex align="center" gap="middle" wrap>
        <Typography.Title
          level={2}
          className={styles.requestTitle}
          title={request.title}
        >
          {request.title}
        </Typography.Title>
        <Tag color={requestStatusColor(request.status)}>
          {requestStatusLabel(request.status)}
        </Tag>
        {request.classification === 'ForwardRollback' && (
          <Tag color="purple">Forward Rollback</Tag>
        )}
      </Flex>
      <RequestProgressSummary
        request={request}
        workflow={workflow}
        execution={executionValue}
        executionPending={execution.isPending}
        executionUnavailable={
          execution.isError ||
          Boolean(execution.data && execution.data.status !== 200)
        }
      />
      {workflow?.document.transitions.some(
        (transition) =>
          transition.trigger === 'Manual' &&
          transition.from === request.workflowStateKey &&
          request.capabilities.includes(transition.permission),
      ) && (
        <div
          id="request-transitions"
          className={styles.sectionAnchor}
          tabIndex={-1}
        >
          <TransitionPanel
            request={request}
            workflow={workflow}
            onUpdated={refresh}
          />
        </div>
      )}
      {actionableReview && reviewPanel}
      <WorkflowPlanProgress
        request={request}
        workflows={workflowValues}
        plans={planValues}
        execution={executionValue}
      />
      <div
        id="request-application-evidence"
        className={styles.sectionAnchor}
        tabIndex={-1}
      >
        <ExecutionPanel
          request={request}
          execution={executionValue}
          unavailable={
            execution.isError ||
            Boolean(execution.data && execution.data.status !== 200)
          }
          onUpdated={refresh}
        />
      </div>
      <RequestOverview request={request} onUpdated={refresh} />
      {!actionableReview && reviewPanel}
      <div
        id="request-application-snapshots"
        className={styles.sectionAnchor}
        tabIndex={-1}
      >
        <ApplicationSnapshots applications={request.applications} />
      </div>
      <DeploymentHistoryPanel
        projectId={request.projectId}
        environmentId={request.environmentId}
        enabled={request.capabilities.includes('deployment_history.view')}
      />
    </Space>
  )
}
