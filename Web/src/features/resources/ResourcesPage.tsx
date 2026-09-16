import {
  Alert,
  Button,
  Card,
  Collapse,
  Empty,
  Flex,
  Form,
  Input,
  Modal,
  Select,
  Space,
  Spin,
  Tag,
  Typography,
} from 'antd'
import { useState } from 'react'
import { Link } from 'react-router'
import { useTranslation } from 'react-i18next'

import {
  createCatalogEnvironment,
  createCatalogEnvironmentLabelMapping,
  createCatalogOrganization,
  createCatalogProject,
  deleteCatalogOrganization,
  updateCatalogOrganization,
  useGetCatalogResourceTree,
  useGetSystemStatus,
} from '@/generated/api'
import type {
  CatalogOrganizationNode,
  CatalogProjectNode,
} from '@/generated/model'
import { parseAPIErrorResponse, type APIErrorView } from '@/shared/api/apiError'
import { useFeedback } from '@/shared/feedback/useFeedback'
import { SemanticList, SemanticListItemContent } from '@/shared/list'

type ResourceAction =
  | { kind: 'organization' }
  | { kind: 'rename'; organization: CatalogOrganizationNode }
  | { kind: 'project'; organization: CatalogOrganizationNode }
  | {
      kind: 'environment'
      organization: CatalogOrganizationNode
      project: CatalogProjectNode
    }
  | {
      kind: 'mapping'
      organization: CatalogOrganizationNode
      project: CatalogProjectNode
    }

interface ResourceFields {
  name?: string
  type?: 'Development' | 'Testing' | 'Staging' | 'Production'
  environmentId?: string
  labelKey?: string
  labelValue?: string
}

export function ResourcesPage() {
  const { t } = useTranslation()
  const feedback = useFeedback()
  const [form] = Form.useForm<ResourceFields>()
  const [action, setAction] = useState<ResourceAction>()
  const [submitting, setSubmitting] = useState(false)
  const [deleteTarget, setDeleteTarget] = useState<CatalogOrganizationNode>()
  const [deleteSubmitting, setDeleteSubmitting] = useState(false)
  const resources = useGetCatalogResourceTree()
  const system = useGetSystemStatus()

  if (resources.isPending || system.isPending)
    return (
      <main>
        <Spin size="large" description={t('resources.loading')} />
      </main>
    )
  if (
    resources.isError ||
    resources.data?.status !== 200 ||
    system.isError ||
    system.data?.status !== 200
  ) {
    return (
      <Alert
        type="error"
        showIcon
        title={t('resources.error.title')}
        description={t('resources.error.description')}
      />
    )
  }

  const organizations = resources.data.data.data
  const singleTenant = system.data.data.data.tenancyMode === 'single'
  const open = (next: ResourceAction) => {
    form.resetFields()
    if (next.kind === 'rename') {
      form.setFieldsValue({ name: next.organization.name })
    }
    setAction(next)
  }
  const close = () => {
    if (!submitting) setAction(undefined)
  }
  const submit = async (fields: ResourceFields) => {
    if (!action) return
    const csrfToken = browserCookie('releasehub_csrf')
    if (!csrfToken) {
      feedback.error(t('resources.mutation.error'))
      return
    }
    setSubmitting(true)
    try {
      let response: { status: number; data?: unknown }
      const options = { headers: { 'X-CSRF-Token': csrfToken } }
      if (action.kind === 'organization') {
        response = await createCatalogOrganization(
          { name: fields.name ?? '' },
          options,
        )
      } else if (action.kind === 'rename') {
        response = await updateCatalogOrganization(
          action.organization.id,
          {
            name: fields.name?.trim() ?? '',
            version: action.organization.version,
          },
          options,
        )
      } else if (action.kind === 'project') {
        response = await createCatalogProject(
          action.organization.id,
          { name: fields.name ?? '' },
          options,
        )
      } else if (action.kind === 'environment') {
        response = await createCatalogEnvironment(
          action.organization.id,
          action.project.id,
          { name: fields.name ?? '', type: fields.type ?? 'Development' },
          options,
        )
      } else {
        response = await createCatalogEnvironmentLabelMapping(
          action.organization.id,
          action.project.id,
          {
            environmentId: fields.environmentId ?? '',
            labelKey: fields.labelKey ?? '',
            labelValue: fields.labelValue ?? '',
          },
          options,
        )
      }
      const expectedStatus = action.kind === 'rename' ? 200 : 201
      if (response.status !== expectedStatus) {
        feedback.error(
          resourceMutationError(parseAPIErrorResponse(response), t),
        )
        return
      }
      setAction(undefined)
      feedback.success(
        t(
          action.kind === 'rename'
            ? 'resources.mutation.renamed'
            : 'resources.mutation.success',
        ),
      )
      await resources.refetch()
    } catch {
      feedback.error(t('resources.mutation.error'))
    } finally {
      setSubmitting(false)
    }
  }
  const confirmDelete = async () => {
    if (!deleteTarget) return
    const csrfToken = browserCookie('releasehub_csrf')
    if (!csrfToken) {
      feedback.error(t('resources.mutation.error'))
      return
    }
    setDeleteSubmitting(true)
    try {
      const response = await deleteCatalogOrganization(
        deleteTarget.id,
        { expectedVersion: deleteTarget.version },
        { headers: { 'X-CSRF-Token': csrfToken } },
      )
      if (response.status !== 204) {
        feedback.error(
          resourceMutationError(parseAPIErrorResponse(response), t),
        )
        return
      }
      setDeleteTarget(undefined)
      feedback.success(t('resources.mutation.deleted'))
      await resources.refetch()
    } catch {
      feedback.error(t('resources.mutation.error'))
    } finally {
      setDeleteSubmitting(false)
    }
  }

  return (
    <Space orientation="vertical" size="large" style={{ width: '100%' }}>
      <Flex justify="space-between" align="start" gap="middle">
        <div>
          <Typography.Title level={2}>{t('resources.title')}</Typography.Title>
          <Typography.Paragraph type="secondary">
            {t('resources.description')}
          </Typography.Paragraph>
        </div>
        {resources.data.data.canCreateOrganization && !singleTenant && (
          <Button type="primary" onClick={() => open({ kind: 'organization' })}>
            {t('resources.actions.createOrganization')}
          </Button>
        )}
      </Flex>
      {organizations.length === 0 ? (
        <Empty description={t('resources.empty')} />
      ) : (
        organizations.map((organization) =>
          singleTenant ? (
            <Card
              key={organization.id}
              title={`${t('resources.workspace')}: ${organization.name}`}
              extra={
                organization.canRename ? (
                  <Button
                    onClick={() => open({ kind: 'rename', organization })}
                  >
                    {t('resources.actions.renameWorkspace')}
                  </Button>
                ) : undefined
              }
            >
              <ProjectList organization={organization} open={open} />
            </Card>
          ) : (
            <Card
              key={organization.id}
              title={`${t('resources.organization')}: ${organization.name}`}
              extra={
                organization.canRename || organization.canDelete ? (
                  <Space wrap>
                    {organization.canRename && (
                      <Button
                        onClick={() => open({ kind: 'rename', organization })}
                      >
                        {t('resources.actions.renameOrganization')}
                      </Button>
                    )}
                    {organization.canDelete && (
                      <Button
                        danger
                        onClick={() => setDeleteTarget(organization)}
                      >
                        {t('resources.actions.deleteOrganization')}
                      </Button>
                    )}
                  </Space>
                ) : undefined
              }
            >
              <ProjectList organization={organization} open={open} />
            </Card>
          ),
        )
      )}
      <ResourceModal
        action={action}
        form={form}
        submitting={submitting}
        close={close}
        submit={submit}
      />
      <Modal
        open={Boolean(deleteTarget)}
        title={t('resources.deleteModal.title')}
        okText={t('resources.deleteModal.confirm')}
        cancelText={t('resources.modal.cancel')}
        okButtonProps={{ danger: true }}
        confirmLoading={deleteSubmitting}
        onCancel={() => {
          if (!deleteSubmitting) setDeleteTarget(undefined)
        }}
        onOk={() => void confirmDelete()}
        destroyOnHidden
      >
        <Typography.Paragraph>
          {t('resources.deleteModal.description', {
            name: deleteTarget?.name ?? '',
          })}
        </Typography.Paragraph>
      </Modal>
    </Space>
  )
}

function ProjectList({
  organization,
  open,
}: {
  organization: CatalogOrganizationNode
  open: (action: ResourceAction) => void
}) {
  const { t } = useTranslation()
  return (
    <Space orientation="vertical" size="middle" style={{ width: '100%' }}>
      {organization.canCreateProject && (
        <Button onClick={() => open({ kind: 'project', organization })}>
          {t('resources.actions.createProject')}
        </Button>
      )}
      {organization.projects.length === 0 ? (
        <Empty description={t('resources.noProjects')} />
      ) : (
        <Collapse
          items={organization.projects.map((project) => ({
            key: project.id,
            label: `${t('resources.project')}: ${project.name}`,
            children: (
              <EnvironmentList
                organization={organization}
                project={project}
                open={open}
              />
            ),
          }))}
        />
      )}
    </Space>
  )
}

function EnvironmentList({
  organization,
  project,
  open,
}: {
  organization: CatalogOrganizationNode
  project: CatalogProjectNode
  open: (action: ResourceAction) => void
}) {
  const { t } = useTranslation()
  return (
    <Space orientation="vertical" size="middle" style={{ width: '100%' }}>
      {project.canManage && (
        <Space wrap>
          <Button
            onClick={() => open({ kind: 'environment', organization, project })}
          >
            {t('resources.actions.createEnvironment')}
          </Button>
          <Button
            onClick={() => open({ kind: 'mapping', organization, project })}
          >
            {t('resources.actions.createMapping')}
          </Button>
        </Space>
      )}
      {project.environments.length === 0 ? (
        <Empty description={t('resources.noEnvironments')} />
      ) : (
        project.environments.map((environment) => (
          <Card
            key={environment.id}
            size="small"
            title={environment.name}
            extra={<Tag>{environment.type}</Tag>}
          >
            <SemanticList
              items={environment.applications}
              rowKey="id"
              emptyContent={
                <Empty description={t('resources.noApplications')} />
              }
              renderItem={(application) => (
                <SemanticListItemContent
                  title={
                    <Link to={`/applications/${application.id}`}>
                      {application.name}
                    </Link>
                  }
                  description={`${application.argocdNamespace}/${application.argocdApplicationName}`}
                />
              )}
            />
          </Card>
        ))
      )}
    </Space>
  )
}

function ResourceModal({
  action,
  form,
  submitting,
  close,
  submit,
}: {
  action?: ResourceAction
  form: ReturnType<typeof Form.useForm<ResourceFields>>[0]
  submitting: boolean
  close: () => void
  submit: (fields: ResourceFields) => Promise<void>
}) {
  const { t } = useTranslation()
  const project =
    action && (action.kind === 'environment' || action.kind === 'mapping')
      ? action.project
      : undefined
  return (
    <Modal
      open={Boolean(action)}
      title={action ? t(`resources.modal.${action.kind}`) : ''}
      okText={t(
        action?.kind === 'rename'
          ? 'resources.modal.save'
          : 'resources.modal.submit',
      )}
      cancelText={t('resources.modal.cancel')}
      confirmLoading={submitting}
      onCancel={close}
      onOk={() => form.submit()}
      destroyOnHidden
    >
      <Form
        form={form}
        layout="vertical"
        onFinish={(fields) => void submit(fields)}
      >
        {action?.kind !== 'mapping' && (
          <Form.Item
            name="name"
            label={t('resources.fields.name')}
            rules={[
              {
                required: true,
                whitespace: true,
                max: 128,
                message: t('resources.fields.nameRequired'),
              },
            ]}
          >
            <Input />
          </Form.Item>
        )}
        {action?.kind === 'environment' && (
          <Form.Item
            name="type"
            label={t('resources.fields.type')}
            rules={[
              { required: true, message: t('resources.fields.typeRequired') },
            ]}
          >
            <Select
              options={['Development', 'Testing', 'Staging', 'Production'].map(
                (value) => ({ value, label: value }),
              )}
            />
          </Form.Item>
        )}
        {action?.kind === 'mapping' && (
          <>
            <Form.Item
              name="environmentId"
              label={t('resources.fields.environment')}
              rules={[
                {
                  required: true,
                  message: t('resources.fields.environmentRequired'),
                },
              ]}
            >
              <Select
                options={project?.environments.map((environment) => ({
                  value: environment.id,
                  label: `${environment.name} (${environment.type})`,
                }))}
              />
            </Form.Item>
            <Form.Item
              name="labelKey"
              label={t('resources.fields.labelKey')}
              rules={[
                {
                  required: true,
                  whitespace: true,
                  max: 317,
                  message: t('resources.fields.labelKeyRequired'),
                },
              ]}
            >
              <Input />
            </Form.Item>
            <Form.Item
              name="labelValue"
              label={t('resources.fields.labelValue')}
              rules={[
                { max: 63, message: t('resources.fields.labelValueRequired') },
              ]}
            >
              <Input />
            </Form.Item>
          </>
        )}
      </Form>
    </Modal>
  )
}

function resourceMutationError(
  error: APIErrorView,
  t: (key: string) => string,
) {
  if (error.category === 'validation') return t('resources.mutation.invalid')
  if (error.category === 'not_found')
    return t('resources.mutation.notAuthorized')
  if (error.category === 'conflict') return t('resources.mutation.conflict')
  return t('resources.mutation.error')
}

function browserCookie(name: string): string | undefined {
  return document.cookie
    .split('; ')
    .find((value) => value.startsWith(`${name}=`))
    ?.slice(name.length + 1)
}
