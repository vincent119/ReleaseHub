import { Button, Card, Empty, Space } from 'antd'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { transitionDeploymentRequest } from '@/generated/api'
import type {
  DeploymentRequestVersion,
  ReleaseWorkflowVersion,
} from '@/generated/model'
import { useFeedback } from '@/shared/feedback/useFeedback'

interface Props {
  request: DeploymentRequestVersion
  workflow?: ReleaseWorkflowVersion
  onUpdated: () => Promise<unknown>
}

export function TransitionPanel({ request, workflow, onUpdated }: Props) {
  const { t } = useTranslation()
  const feedback = useFeedback()
  const [submitting, setSubmitting] = useState<string>()
  const transitions =
    workflow?.document.transitions.filter(
      (transition) =>
        transition.trigger === 'Manual' &&
        transition.from === request.workflowStateKey &&
        request.capabilities.includes(transition.permission),
    ) ?? []
  const execute = async (transitionKey: string) => {
    const csrf = browserCookie('releasehub_csrf')
    if (!csrf) return
    setSubmitting(transitionKey)
    try {
      const response = await transitionDeploymentRequest(
        request.requestId,
        request.id,
        { transitionKey, expectedVersion: request.lockVersion },
        {
          headers: {
            'X-CSRF-Token': csrf,
            'Idempotency-Key': crypto.randomUUID(),
          },
        },
      )
      if (response.status !== 202)
        return void feedback.error(t('requestDetail.transitions.rejected'))
      feedback.success(t('requestDetail.transitions.accepted'))
      await onUpdated()
    } catch {
      feedback.error(t('requestDetail.transitions.error'))
    } finally {
      setSubmitting(undefined)
    }
  }
  return (
    <Card title={t('requestDetail.transitions.title')}>
      {transitions.length ? (
        <Space wrap>
          {transitions.map((transition) => (
            <Button
              key={transition.key}
              type="primary"
              loading={submitting === transition.key}
              disabled={Boolean(submitting) && submitting !== transition.key}
              onClick={() => void execute(transition.key)}
            >
              {transition.key}
            </Button>
          ))}
        </Space>
      ) : (
        <Empty
          image={Empty.PRESENTED_IMAGE_SIMPLE}
          description={t('requestDetail.transitions.empty')}
        />
      )}
    </Card>
  )
}

function browserCookie(name: string): string | undefined {
  return document.cookie
    .split('; ')
    .find((value) => value.startsWith(`${name}=`))
    ?.slice(name.length + 1)
}
