import { Button, Card, Empty, Form, Input, Modal, Space } from 'antd'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import {
  decideDeploymentReview,
  reassignDeploymentReview,
} from '@/generated/api'
import type {
  DeploymentRequestVersion,
  DeploymentReviewTask,
} from '@/generated/model'
import { useFeedback } from '@/shared/feedback/useFeedback'
import { SemanticList, SemanticListItemContent } from '@/shared/list'
import { SemanticTag } from '@/shared/tag/SemanticTag'

import {
  ReviewReassignmentModal,
  type ReviewReassignmentValue,
} from './ReviewReassignmentModal'

interface Props {
  request: DeploymentRequestVersion
  onUpdated: (request: DeploymentRequestVersion) => void
  canReview?: boolean
  canReassign?: boolean
}

export function ReviewDecisionPanel({
  request,
  onUpdated,
  canReview = true,
  canReassign = false,
}: Props) {
  const { t } = useTranslation()
  const feedback = useFeedback()
  const [rejecting, setRejecting] = useState<DeploymentReviewTask>()
  const [reassigning, setReassigning] = useState<DeploymentReviewTask>()
  const [submitting, setSubmitting] = useState(false)
  const pending = request.reviews.filter(
    (review) =>
      review.status === 'Pending' || review.status === 'ReassignmentRequired',
  )
  const decide = async (
    review: DeploymentReviewTask,
    decision: 'Approve' | 'Reject',
    reason?: string,
  ) => {
    const csrf = browserCookie('releasehub_csrf')
    if (!csrf) return void feedback.error(t('workflows.review.error'))
    setSubmitting(true)
    try {
      const response = await decideDeploymentReview(
        request.requestId,
        request.id,
        review.id,
        { decision, reason, expectedVersion: request.lockVersion },
        {
          headers: {
            'X-CSRF-Token': csrf,
            'Idempotency-Key': crypto.randomUUID(),
          },
        },
      )
      if (response.status !== 200)
        return void feedback.error(t('workflows.review.rejected'))
      onUpdated(response.data.data)
      setRejecting(undefined)
      feedback.success(t('workflows.review.saved'))
    } catch {
      feedback.error(t('workflows.review.error'))
    } finally {
      setSubmitting(false)
    }
  }
  const reassign = async (value: ReviewReassignmentValue) => {
    if (!reassigning) return
    const csrf = browserCookie('releasehub_csrf')
    if (!csrf) return void feedback.error(t('workflows.review.error'))
    setSubmitting(true)
    try {
      const response = await reassignDeploymentReview(
        request.requestId,
        request.id,
        reassigning.id,
        { ...value, expectedVersion: request.lockVersion },
        {
          headers: {
            'X-CSRF-Token': csrf,
            'Idempotency-Key': crypto.randomUUID(),
          },
        },
      )
      if (response.status !== 200)
        return void feedback.error(t('workflows.review.rejected'))
      onUpdated(response.data.data)
      setReassigning(undefined)
      feedback.success(t('workflows.review.reassigned'))
    } catch {
      feedback.error(t('workflows.review.error'))
    } finally {
      setSubmitting(false)
    }
  }
  return (
    <Card title={t('workflows.review.title')}>
      {pending.length === 0 ? (
        <Empty
          image={Empty.PRESENTED_IMAGE_SIMPLE}
          description={t('workflows.review.empty')}
        />
      ) : (
        <SemanticList
          items={pending}
          rowKey="id"
          renderItem={(review) => (
            <SemanticListItemContent
              actions={[
                ...(canReview
                  ? [
                      <Button
                        key="approve"
                        type="primary"
                        loading={submitting}
                        onClick={() => void decide(review, 'Approve')}
                      >
                        {t('workflows.review.approve')}
                      </Button>,
                      <Button
                        key="reject"
                        danger
                        disabled={submitting}
                        onClick={() => setRejecting(review)}
                      >
                        {t('workflows.review.reject')}
                      </Button>,
                    ]
                  : []),
                ...(canReassign
                  ? [
                      <Button
                        key="reassign"
                        disabled={submitting}
                        onClick={() => setReassigning(review)}
                      >
                        {t('workflows.review.reassign')}
                      </Button>,
                    ]
                  : []),
              ]}
              title={`${review.stateKey} · ${t('workflows.review.stage', { stage: review.stageNumber })}`}
              description={
                <Space>
                  <SemanticTag>{review.policyType}</SemanticTag>
                  <span>
                    {t('workflows.review.required', {
                      count: review.requiredApprovals,
                    })}
                  </span>
                </Space>
              }
            />
          )}
        />
      )}
      <RejectModal
        review={rejecting}
        submitting={submitting}
        onCancel={() => setRejecting(undefined)}
        onSubmit={(reason) => rejecting && decide(rejecting, 'Reject', reason)}
      />
      <ReviewReassignmentModal
        open={Boolean(reassigning)}
        submitting={submitting}
        onCancel={() => setReassigning(undefined)}
        onSubmit={(value) => void reassign(value)}
      />
    </Card>
  )
}

function RejectModal({
  review,
  submitting,
  onCancel,
  onSubmit,
}: {
  review?: DeploymentReviewTask
  submitting: boolean
  onCancel: () => void
  onSubmit: (reason: string) => void
}) {
  const { t } = useTranslation()
  const [form] = Form.useForm<{ reason: string }>()
  return (
    <Modal
      open={Boolean(review)}
      title={t('workflows.review.rejectTitle')}
      confirmLoading={submitting}
      okText={t('workflows.review.confirmReject')}
      cancelText={t('workflows.actions.cancel')}
      onCancel={onCancel}
      onOk={() => {
        void form
          .validateFields()
          .then(({ reason }) => onSubmit(reason))
          .catch(() => undefined)
      }}
    >
      <Form form={form} layout="vertical">
        <Form.Item
          name="reason"
          label={t('workflows.review.reason')}
          rules={[
            { required: true, message: t('workflows.validation.rejectReason') },
          ]}
        >
          <Input.TextArea maxLength={10000} />
        </Form.Item>
      </Form>
    </Modal>
  )
}

function browserCookie(name: string): string | undefined {
  return document.cookie
    .split('; ')
    .find((value) => value.startsWith(`${name}=`))
    ?.slice(name.length + 1)
}
