import {
  Button,
  Card,
  Form,
  Input,
  Modal,
  Result,
  Select,
  Space,
  Spin,
  Table,
  Typography,
} from 'antd'
import type { TableProps } from 'antd'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import {
  assignArgoCDCandidate,
  useListArgoCDCandidateAssignmentScopes,
  useListArgoCDCandidates,
} from '@/generated/api'
import type { ArgoCDCandidate } from '@/generated/model'
import { useFeedback } from '@/shared/feedback/useFeedback'

interface AssignmentFields {
  scopeId: string
  applicationName: string
  sourceIndex: string
}

export function CandidatesPage() {
  const { t } = useTranslation()
  const feedback = useFeedback()
  const [form] = Form.useForm<AssignmentFields>()
  const [selected, setSelected] = useState<ArgoCDCandidate>()
  const [submitting, setSubmitting] = useState(false)
  const candidates = useListArgoCDCandidates()
  const scopes = useListArgoCDCandidateAssignmentScopes({
    query: { enabled: Boolean(selected) },
  })

  const openAssignment = (candidate: ArgoCDCandidate) => {
    setSelected(candidate)
    form.setFieldsValue({
      applicationName: candidate.argocdApplicationName,
      scopeId: undefined,
      sourceIndex: candidate.sources.length === 1 ? '0' : undefined,
    })
  }

  const closeAssignment = () => {
    if (submitting) return
    setSelected(undefined)
    form.resetFields()
  }

  const submitAssignment = async (fields: AssignmentFields) => {
    if (!selected || scopes.data?.status !== 200) return
    const scope = scopes.data.data.data.find(
      (value) => value.environmentId === fields.scopeId,
    )
    const source = selected.sources[Number(fields.sourceIndex)]
    const csrfToken = browserCookie('releasehub_csrf')
    if (!scope || !source || !csrfToken) {
      feedback.error(t('candidates.assignment.error'))
      return
    }

    setSubmitting(true)
    try {
      const response = await assignArgoCDCandidate(
        selected.id,
        {
          expectedVersion: selected.version,
          organizationId: scope.organizationId,
          projectId: scope.projectId,
          environmentId: scope.environmentId,
          applicationName: fields.applicationName,
          source,
        },
        { headers: { 'X-CSRF-Token': csrfToken } },
      )
      if (response.status !== 201) {
        feedback.error(
          response.status === 409
            ? t('candidates.assignment.conflict')
            : t('candidates.assignment.error'),
        )
        return
      }
      feedback.success(t('candidates.assignment.success'))
      setSelected(undefined)
      form.resetFields()
      await candidates.refetch()
    } catch {
      feedback.error(t('candidates.assignment.error'))
    } finally {
      setSubmitting(false)
    }
  }

  const columns: TableProps<ArgoCDCandidate>['columns'] = [
    {
      title: t('candidates.columns.application'),
      dataIndex: 'argocdApplicationName',
      key: 'application',
      ellipsis: true,
    },
    {
      title: t('candidates.columns.namespace'),
      dataIndex: 'argocdNamespace',
      key: 'namespace',
    },
    {
      title: t('candidates.columns.project'),
      dataIndex: 'argocdProject',
      key: 'project',
    },
    {
      title: t('candidates.columns.sync'),
      dataIndex: 'syncStatus',
      key: 'sync',
    },
    {
      title: t('candidates.columns.health'),
      dataIndex: 'healthStatus',
      key: 'health',
    },
    {
      title: t('candidates.columns.lastSeen'),
      dataIndex: 'lastSeenAt',
      key: 'lastSeen',
    },
    {
      title: t('candidates.columns.actions'),
      key: 'actions',
      render: (_, candidate) => (
        <Button onClick={() => openAssignment(candidate)}>
          {t('candidates.assignment.action')}
        </Button>
      ),
    },
  ]

  if (candidates.isPending)
    return (
      <main>
        <Spin size="large" description={t('candidates.loading')} />
      </main>
    )
  if (candidates.isError || candidates.data?.status !== 200)
    return (
      <Result
        status="404"
        title={t('candidates.unavailable.title')}
        subTitle={t('candidates.unavailable.description')}
      />
    )

  const scopeOptions =
    scopes.data?.status === 200
      ? scopes.data.data.data.map((scope) => ({
          value: scope.environmentId,
          label: `${scope.organizationName} / ${scope.projectName} / ${scope.environmentName}`,
        }))
      : []
  const sourceOptions =
    selected?.sources.map((source, index) => ({
      value: String(index),
      label: `${source.repositoryUrl} · ${source.targetRevision} · ${source.path}`,
    })) ?? []

  return (
    <Space orientation="vertical" size="large" style={{ width: '100%' }}>
      <div>
        <Typography.Title level={2}>{t('candidates.title')}</Typography.Title>
        <Typography.Paragraph type="secondary">
          {t('candidates.description')}
        </Typography.Paragraph>
      </div>
      <Card>
        <Table<ArgoCDCandidate>
          rowKey="id"
          columns={columns}
          dataSource={candidates.data.data.data}
          pagination={false}
          locale={{ emptyText: t('candidates.empty') }}
        />
      </Card>
      <Modal
        open={Boolean(selected)}
        title={t('candidates.assignment.title')}
        okText={t('candidates.assignment.submit')}
        cancelText={t('candidates.assignment.cancel')}
        confirmLoading={submitting}
        okButtonProps={{
          disabled:
            scopes.isPending ||
            scopes.isError ||
            scopeOptions.length === 0 ||
            !browserCookie('releasehub_csrf'),
        }}
        onCancel={closeAssignment}
        onOk={() => form.submit()}
        destroyOnHidden
      >
        {scopes.isError || (scopes.data && scopes.data.status !== 200) ? (
          <Result
            status="warning"
            title={t('candidates.assignment.scopesUnavailable')}
          />
        ) : (
          <Form
            form={form}
            layout="vertical"
            onFinish={(fields) => void submitAssignment(fields)}
          >
            <Form.Item
              label={t('candidates.assignment.scope')}
              name="scopeId"
              rules={[
                {
                  required: true,
                  message: t('candidates.assignment.scopeRequired'),
                },
              ]}
            >
              <Select
                loading={scopes.isPending}
                options={scopeOptions}
                showSearch
                optionFilterProp="label"
              />
            </Form.Item>
            <Form.Item
              label={t('candidates.assignment.applicationName')}
              name="applicationName"
              rules={[
                {
                  required: true,
                  whitespace: true,
                  max: 128,
                  message: t('candidates.assignment.applicationNameRequired'),
                },
              ]}
            >
              <Input />
            </Form.Item>
            <Form.Item
              label={t('candidates.assignment.source')}
              name="sourceIndex"
              rules={[
                {
                  required: true,
                  message: t('candidates.assignment.sourceRequired'),
                },
              ]}
            >
              <Select options={sourceOptions} />
            </Form.Item>
          </Form>
        )}
      </Modal>
    </Space>
  )
}

function browserCookie(name: string): string | undefined {
  return document.cookie
    .split('; ')
    .find((value) => value.startsWith(`${name}=`))
    ?.slice(name.length + 1)
}
