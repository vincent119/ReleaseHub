import {
  Alert,
  Button,
  Card,
  Checkbox,
  Descriptions,
  Form,
  Input,
  List,
  Modal,
  Space,
  Tag,
  Typography,
  message,
} from 'antd'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import {
  retryDeploymentRequest,
  terminateDeploymentRequest,
  unlockDeploymentRequest,
} from '@/generated/api'
import type {
  DeploymentActualStateConfirmation,
  DeploymentExecution,
  DeploymentRequestVersion,
} from '@/generated/model'

import {
  nodeStatusColor,
  requestStatusColor,
  requestStatusLabel,
} from '../model/presentation'

interface Props {
  request: DeploymentRequestVersion
  execution?: DeploymentExecution
  unavailable: boolean
  onUpdated: () => Promise<unknown>
}

type Command = 'terminate' | 'unlock'

export function ExecutionPanel({
  request,
  execution,
  unavailable,
  onUpdated,
}: Props) {
  const { t } = useTranslation()
  const [selected, setSelected] = useState<string[]>([])
  const [command, setCommand] = useState<Command>()
  const [submitting, setSubmitting] = useState(false)
  if (unavailable)
    return (
      <Alert
        type="error"
        showIcon
        message={t('requestDetail.execution.unavailable')}
      />
    )
  if (!execution)
    return (
      <Card title={t('requestDetail.execution.title')}>
        <Alert
          type="info"
          showIcon
          message={t('requestDetail.execution.pending')}
        />
      </Card>
    )
  const failed = execution.nodes.filter((node) => node.status === 'Failed')
  const capability = (key: string) => request.capabilities.includes(key)
  const retry = async () => {
    if (!selected.length) return
    await runCommand(
      setSubmitting,
      t,
      () =>
        retryDeploymentRequest(
          request.requestId,
          request.id,
          { applicationIds: selected, expectedVersion: execution.lockVersion },
          commandOptions(),
        ),
      onUpdated,
    )
  }
  return (
    <Card
      title={t('requestDetail.execution.title')}
      extra={
        <Tag color={requestStatusColor(execution.status)}>
          {requestStatusLabel(execution.status)}
        </Tag>
      }
    >
      <Descriptions size="small" column={{ xs: 1, sm: 3 }}>
        <Descriptions.Item label={t('requestDetail.execution.attempt')}>
          {execution.attempt}
        </Descriptions.Item>
        <Descriptions.Item label={t('requestDetail.execution.trigger')}>
          {execution.triggerKind}
        </Descriptions.Item>
        <Descriptions.Item label={t('requestDetail.execution.id')}>
          {execution.id}
        </Descriptions.Item>
      </Descriptions>
      <List
        dataSource={execution.nodes}
        renderItem={(node) => (
          <List.Item
            extra={
              <Tag color={nodeStatusColor(node.status)}>{node.status}</Tag>
            }
          >
            <Checkbox
              aria-label={t('requestDetail.execution.selectRetry', {
                application: node.nodeKey,
              })}
              disabled={
                node.status !== 'Failed' ||
                !capability('deployment_request.retry')
              }
              checked={selected.includes(node.applicationId)}
              onChange={(event) =>
                setSelected(
                  toggleValue(
                    selected,
                    node.applicationId,
                    event.target.checked,
                  ),
                )
              }
            >
              {node.nodeKey}
            </Checkbox>
            <Space orientation="vertical" size={0}>
              <Typography.Text type="secondary">
                {node.syncStatus} · {node.healthStatus}
              </Typography.Text>
              {node.errorMessage && (
                <Typography.Text type="danger">
                  {node.errorMessage}
                </Typography.Text>
              )}
            </Space>
          </List.Item>
        )}
      />
      <Space wrap>
        {capability('deployment_request.retry') && failed.length > 0 && (
          <Button
            type="primary"
            loading={submitting}
            disabled={!selected.length}
            onClick={() => void retry()}
          >
            {t('requestDetail.actions.retryFailed')}
          </Button>
        )}
        {capability('deployment_request.terminate') &&
          ['Queued', 'Preflight', 'Running'].includes(execution.status) && (
            <Button
              danger
              disabled={submitting}
              onClick={() => setCommand('terminate')}
            >
              {t('requestDetail.actions.terminate')}
            </Button>
          )}
        {capability('deployment_request.unlock') &&
          execution.status === 'Terminated' && (
            <Button
              danger
              disabled={submitting}
              onClick={() => setCommand('unlock')}
            >
              {t('requestDetail.actions.unlock')}
            </Button>
          )}
      </Space>
      <ExecutionCommandModal
        command={command}
        submitting={submitting}
        onCancel={() => setCommand(undefined)}
        onSubmit={(reason) =>
          void executeCommand(
            command,
            reason,
            request,
            execution,
            setSubmitting,
            onUpdated,
            t,
          ).then(() => setCommand(undefined))
        }
      />
    </Card>
  )
}

function ExecutionCommandModal({
  command,
  submitting,
  onCancel,
  onSubmit,
}: {
  command?: Command
  submitting: boolean
  onCancel: () => void
  onSubmit: (reason: string) => void
}) {
  const { t } = useTranslation()
  const [form] = Form.useForm<{ reason: string }>()
  return (
    <Modal
      open={Boolean(command)}
      title={t(`requestDetail.commands.${command}.title`)}
      okText={t(`requestDetail.commands.${command}.confirm`)}
      cancelText={t('requestDetail.metadata.cancel')}
      confirmLoading={submitting}
      closable={!submitting}
      maskClosable={false}
      onCancel={onCancel}
      onOk={() =>
        void form
          .validateFields()
          .then(({ reason }) => onSubmit(reason))
          .catch(() => undefined)
      }
    >
      <Alert
        type="warning"
        showIcon
        message={t(`requestDetail.commands.${command}.warning`)}
      />
      <Form form={form} layout="vertical">
        <Form.Item
          name="reason"
          label={t('requestDetail.commands.reason')}
          rules={[
            {
              required: true,
              message: t('requestDetail.commands.reasonRequired'),
            },
          ]}
        >
          <Input.TextArea maxLength={10000} />
        </Form.Item>
      </Form>
    </Modal>
  )
}

async function executeCommand(
  command: Command | undefined,
  reason: string,
  request: DeploymentRequestVersion,
  execution: DeploymentExecution,
  setSubmitting: (value: boolean) => void,
  onUpdated: () => Promise<unknown>,
  t: (key: string) => string,
) {
  if (!command) return
  const operation =
    command === 'terminate'
      ? () =>
          terminateDeploymentRequest(
            request.requestId,
            request.id,
            { reason, expectedVersion: execution.lockVersion },
            commandOptions(),
          )
      : () =>
          unlockDeploymentRequest(
            request.requestId,
            request.id,
            {
              reason,
              expectedVersion: execution.lockVersion,
              actualStates: actualStates(execution),
            },
            commandOptions(),
          )
  await runCommand(setSubmitting, t, operation, onUpdated)
}

async function runCommand(
  setSubmitting: (value: boolean) => void,
  t: (key: string) => string,
  operation: () => Promise<{ status: number }>,
  onUpdated: () => Promise<unknown>,
) {
  setSubmitting(true)
  try {
    const response = await operation()
    if (response.status !== 202)
      return void message.error(t('requestDetail.commands.rejected'))
    message.success(t('requestDetail.commands.accepted'))
    await onUpdated()
  } catch {
    message.error(t('requestDetail.commands.error'))
  } finally {
    setSubmitting(false)
  }
}

function commandOptions() {
  const csrf = browserCookie('releasehub_csrf') ?? ''
  return {
    headers: { 'X-CSRF-Token': csrf, 'Idempotency-Key': crypto.randomUUID() },
  }
}

function actualStates(
  execution: DeploymentExecution,
): DeploymentActualStateConfirmation[] {
  return execution.nodes.map((node) => ({
    applicationId: node.applicationId,
    revision: node.actualRevision,
    images: node.actualImages,
  }))
}

function toggleValue(values: string[], value: string, checked: boolean) {
  return checked ? [...values, value] : values.filter((item) => item !== value)
}

function browserCookie(name: string): string | undefined {
  return document.cookie
    .split('; ')
    .find((value) => value.startsWith(`${name}=`))
    ?.slice(name.length + 1)
}
