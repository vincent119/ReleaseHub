import { Card, Tag, Typography } from 'antd'
import { useTranslation } from 'react-i18next'

import type {
  DeploymentExecution,
  DeploymentRequestVersion,
  ReleaseWorkflowVersion,
} from '@/generated/model'

import {
  nodeStatusColor,
  requestStatusColor,
  requestStatusLabel,
} from '../model/presentation'
import styles from '../RequestDetailPage.module.css'

interface Props {
  request: DeploymentRequestVersion
  workflow?: ReleaseWorkflowVersion
  execution?: DeploymentExecution
  executionPending: boolean
  executionUnavailable: boolean
}

export function RequestProgressSummary({
  request,
  workflow,
  execution,
  executionPending,
  executionUnavailable,
}: Props) {
  const { t } = useTranslation()
  const currentState = workflow?.document.states.find(
    (state) => state.key === request.workflowStateKey,
  )
  const nodeByApplication = new Map(
    execution?.nodes.map((node) => [node.applicationId, node]),
  )
  const nextAction = findNextAction(request, workflow, execution)

  return (
    <Card title={t('requestDetail.progress.title')}>
      <div className={styles.progressSummary}>
        <section className={styles.progressMetric}>
          <Typography.Text type="secondary">
            {t('requestDetail.progress.workflowStage')}
          </Typography.Text>
          <strong className={styles.progressValue}>
            {currentState?.name ??
              request.workflowStateKey ??
              t('requestDetail.values.pending')}
          </strong>
          <span>
            {t('requestDetail.progress.requestStatus')}{' '}
            <Tag color={requestStatusColor(request.status)}>
              {requestStatusLabel(request.status)}
            </Tag>
          </span>
        </section>
        <section className={styles.progressMetric}>
          <Typography.Text type="secondary">
            {t('requestDetail.progress.executionStatus')}
          </Typography.Text>
          {executionUnavailable ? (
            <strong className={styles.progressValue} role="alert">
              {t('requestDetail.execution.unavailable')}
            </strong>
          ) : execution ? (
            <Tag color={requestStatusColor(execution.status)}>
              {requestStatusLabel(execution.status)}
            </Tag>
          ) : (
            <strong className={styles.progressValue}>
              {request.executionId && executionPending
                ? t('requestDetail.progress.loadingExecution')
                : t('requestDetail.execution.pending')}
            </strong>
          )}
        </section>
        <section className={styles.progressMetric}>
          <Typography.Text type="secondary">
            {t('requestDetail.progress.nextAction')}
          </Typography.Text>
          {nextAction ? (
            <a className={styles.progressValue} href={`#${nextAction.target}`}>
              {t(nextAction.label)}
            </a>
          ) : (
            <strong className={styles.progressValue}>
              {t('requestDetail.progress.noAction')}
            </strong>
          )}
        </section>
      </div>
      <div className={styles.progressApplications}>
        <Typography.Text strong>
          {t('requestDetail.progress.affectedApplications', {
            count: request.applications.length,
          })}
        </Typography.Text>
        <ul className={styles.progressApplicationList}>
          {[...request.applications]
            .sort((left, right) => left.order - right.order)
            .map((application) => {
              const node = nodeByApplication.get(application.applicationId)
              return (
                <li key={application.id}>
                  <a
                    href={
                      execution
                        ? '#request-application-evidence'
                        : '#request-application-snapshots'
                    }
                  >
                    {application.applicationKey}
                  </a>
                  {node && (
                    <Tag color={nodeStatusColor(node.status)}>
                      {node.status}
                    </Tag>
                  )}
                </li>
              )
            })}
        </ul>
      </div>
    </Card>
  )
}

function findNextAction(
  request: DeploymentRequestVersion,
  workflow?: ReleaseWorkflowVersion,
  execution?: DeploymentExecution,
) {
  const capability = (key: string) => request.capabilities.includes(key)
  const pendingReview = request.reviews.some(
    (review) =>
      review.status === 'Pending' || review.status === 'ReassignmentRequired',
  )
  if (
    pendingReview &&
    (capability('deployment_request.review') ||
      capability('deployment_request.reassign'))
  )
    return { target: 'request-review', label: 'requestDetail.progress.review' }
  const manualTransition = workflow?.document.transitions.some(
    (transition) =>
      transition.trigger === 'Manual' &&
      transition.from === request.workflowStateKey &&
      capability(transition.permission),
  )
  if (manualTransition)
    return {
      target: 'request-transitions',
      label: 'requestDetail.progress.transition',
    }
  if (
    capability('deployment_request.retry') &&
    execution?.nodes.some((node) => node.status === 'Failed')
  )
    return {
      target: 'request-application-evidence',
      label: 'requestDetail.progress.retry',
    }
  if (
    capability('deployment_request.terminate') &&
    execution &&
    ['Queued', 'Preflight', 'Running'].includes(execution.status)
  )
    return {
      target: 'request-application-evidence',
      label: 'requestDetail.progress.terminate',
    }
  if (
    capability('deployment_request.unlock') &&
    execution?.status === 'Terminated'
  )
    return {
      target: 'request-application-evidence',
      label: 'requestDetail.progress.unlock',
    }
  return undefined
}
