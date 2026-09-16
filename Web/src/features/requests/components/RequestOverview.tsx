import { Button, Card, Descriptions, Form, Input, Modal, Tag } from 'antd'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { updateDeploymentRequestVersionMetadata } from '@/generated/api'
import type { DeploymentRequestVersion } from '@/generated/model'
import { useFeedback } from '@/shared/feedback/useFeedback'

import { requestStatusColor, requestStatusLabel } from '../model/presentation'

interface Props {
  request: DeploymentRequestVersion
  onUpdated: () => Promise<unknown>
}

interface MetadataForm {
  changeDescription?: string
  issueUrl?: string
  scheduledFor?: string
}

export function RequestOverview({ request, onUpdated }: Props) {
  const { t } = useTranslation()
  const feedback = useFeedback()
  const [editing, setEditing] = useState(false)
  const [submitting, setSubmitting] = useState(false)
  const [form] = Form.useForm<MetadataForm>()
  const canUpdate = request.capabilities.includes('deployment_request.update')
  const submit = async () => {
    const values = await form.validateFields()
    const csrf = browserCookie('releasehub_csrf')
    if (!csrf) return
    setSubmitting(true)
    try {
      const response = await updateDeploymentRequestVersionMetadata(
        request.requestId,
        request.id,
        { ...metadataInput(values), expectedVersion: request.lockVersion },
        { headers: { 'X-CSRF-Token': csrf } },
      )
      if (response.status === 200) {
        setEditing(false)
        await onUpdated()
        feedback.success(t('requestDetail.metadata.saved'))
      } else {
        feedback.error(t('requestDetail.metadata.rejected'))
      }
    } catch {
      feedback.error(t('requestDetail.metadata.error'))
    } finally {
      setSubmitting(false)
    }
  }
  return (
    <Card
      title={t('requestDetail.overview.title')}
      extra={
        canUpdate ? (
          <Button
            onClick={() => {
              form.setFieldsValue(metadataValues(request))
              setEditing(true)
            }}
          >
            {t('requestDetail.actions.editMetadata')}
          </Button>
        ) : null
      }
    >
      <Descriptions column={{ xs: 1, sm: 2, lg: 3 }}>
        <Descriptions.Item label={t('requestDetail.fields.status')}>
          <Tag color={requestStatusColor(request.status)}>
            {requestStatusLabel(request.status)}
          </Tag>
        </Descriptions.Item>
        <Descriptions.Item label={t('requestDetail.fields.classification')}>
          {request.classification}
        </Descriptions.Item>
        <Descriptions.Item label={t('requestDetail.fields.version')}>
          {request.versionNumber}
        </Descriptions.Item>
        <Descriptions.Item label={t('requestDetail.fields.workflowState')}>
          {request.workflowStateKey ?? t('requestDetail.values.pending')}
        </Descriptions.Item>
        <Descriptions.Item label={t('requestDetail.fields.scheduledFor')}>
          {request.scheduledFor
            ? new Date(request.scheduledFor).toLocaleString()
            : t('requestDetail.values.notSet')}
        </Descriptions.Item>
        <Descriptions.Item label={t('requestDetail.fields.scheduleState')}>
          <Tag color={request.scheduleState === 'Ready' ? 'green' : 'gold'}>
            {t(`requestDetail.schedule.states.${request.scheduleState}`)}
          </Tag>
          {t(`requestDetail.schedule.reasons.${request.scheduleReason}`)}
        </Descriptions.Item>
        <Descriptions.Item label={t('requestDetail.fields.nextEligibleAt')}>
          {new Date(request.nextEligibleAt).toLocaleString()}
        </Descriptions.Item>
        <Descriptions.Item label={t('requestDetail.fields.issue')}>
          {request.issueUrl ? (
            <a href={request.issueUrl} target="_blank" rel="noreferrer">
              {request.issueUrl}
            </a>
          ) : (
            t('requestDetail.values.notSet')
          )}
        </Descriptions.Item>
        <Descriptions.Item
          label={t('requestDetail.fields.changeDescription')}
          span="filled"
        >
          {request.changeDescription || t('requestDetail.values.notSet')}
        </Descriptions.Item>
      </Descriptions>
      <Modal
        open={editing}
        title={t('requestDetail.metadata.title')}
        okText={t('requestDetail.metadata.save')}
        cancelText={t('requestDetail.metadata.cancel')}
        confirmLoading={submitting}
        onCancel={() => setEditing(false)}
        onOk={() => void submit()}
      >
        <Form form={form} layout="vertical">
          <Form.Item
            name="changeDescription"
            label={t('requestDetail.fields.changeDescription')}
          >
            <Input.TextArea maxLength={10000} />
          </Form.Item>
          <Form.Item
            name="issueUrl"
            label={t('requestDetail.fields.issue')}
            rules={[{ type: 'url' }]}
          >
            <Input />
          </Form.Item>
          <Form.Item
            name="scheduledFor"
            label={t('requestDetail.fields.scheduledFor')}
          >
            <Input type="datetime-local" />
          </Form.Item>
        </Form>
      </Modal>
    </Card>
  )
}

function metadataValues(request: DeploymentRequestVersion) {
  return {
    changeDescription: request.changeDescription,
    issueUrl: request.issueUrl,
    scheduledFor: request.scheduledFor?.slice(0, 16),
  }
}

function metadataInput(values: MetadataForm) {
  return {
    changeDescription: values.changeDescription || undefined,
    issueUrl: values.issueUrl || undefined,
    scheduledFor: values.scheduledFor
      ? new Date(values.scheduledFor).toISOString()
      : undefined,
  }
}

function browserCookie(name: string): string | undefined {
  return document.cookie
    .split('; ')
    .find((value) => value.startsWith(`${name}=`))
    ?.slice(name.length + 1)
}
