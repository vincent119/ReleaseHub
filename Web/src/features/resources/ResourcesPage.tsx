import {
  Alert,
  Button,
  Card,
  Collapse,
  Empty,
  Flex,
  Form,
  Input,
  List,
  Modal,
  Select,
  Space,
  Spin,
  Tag,
  Typography,
  message,
} from 'antd'
import { useState } from 'react'
import { Link } from 'react-router'
import { useTranslation } from 'react-i18next'

import {
  createCatalogEnvironment,
  createCatalogEnvironmentLabelMapping,
  createCatalogOrganization,
  createCatalogProject,
  useGetCatalogResourceTree,
  useGetSystemStatus,
} from '@/generated/api'
import type {
  CatalogOrganizationNode,
  CatalogProjectNode,
} from '@/generated/model'

type ResourceAction =
  | { kind: 'organization' }
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
  const [form] = Form.useForm<ResourceFields>()
  const [action, setAction] = useState<ResourceAction>()
  const [submitting, setSubmitting] = useState(false)
  const resources = useGetCatalogResourceTree()
  const system = useGetSystemStatus()

  if (resources.isPending || system.isPending)
    return (
      <main>
        <Spin size="large" tip={t('resources.loading')} />
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
        message={t('resources.error.title')}
        description={t('resources.error.description')}
      />
    )
  }

  const organizations = resources.data.data.data
  const singleTenant = system.data.data.data.tenancyMode === 'single'
  const open = (next: ResourceAction) => {
    form.resetFields()
    setAction(next)
  }
  const close = () => {
    if (!submitting) setAction(undefined)
  }
  const submit = async (fields: ResourceFields) => {
    if (!action) return
    const csrfToken = browserCookie('releasehub_csrf')
    if (!csrfToken) {
      message.error(t('resources.mutation.error'))
      return
    }
    setSubmitting(true)
    try {
      let status: number
      const options = { headers: { 'X-CSRF-Token': csrfToken } }
      if (action.kind === 'organization') {
        status = (
          await createCatalogOrganization({ name: fields.name ?? '' }, options)
        ).status
      } else if (action.kind === 'project') {
        status = (
          await createCatalogProject(
            action.organization.id,
            { name: fields.name ?? '' },
            options,
          )
        ).status
      } else if (action.kind === 'environment') {
        status = (
          await createCatalogEnvironment(
            action.organization.id,
            action.project.id,
            { name: fields.name ?? '', type: fields.type ?? 'Development' },
            options,
          )
        ).status
      } else {
        status = (
          await createCatalogEnvironmentLabelMapping(
            action.organization.id,
            action.project.id,
            {
              environmentId: fields.environmentId ?? '',
              labelKey: fields.labelKey ?? '',
              labelValue: fields.labelValue ?? '',
            },
            options,
          )
        ).status
      }
      if (status !== 201) {
        message.error(
          status === 404
            ? t('resources.mutation.notAuthorized')
            : t('resources.mutation.conflict'),
        )
        return
      }
      setAction(undefined)
      message.success(t('resources.mutation.success'))
      await resources.refetch()
    } catch {
      message.error(t('resources.mutation.error'))
    } finally {
      setSubmitting(false)
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
            <ProjectList
              key={organization.id}
              organization={organization}
              open={open}
            />
          ) : (
            <Card
              key={organization.id}
              title={`${t('resources.organization')}: ${organization.name}`}
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
            <List
              dataSource={environment.applications}
              locale={{ emptyText: t('resources.noApplications') }}
              renderItem={(application) => (
                <List.Item>
                  <List.Item.Meta
                    title={
                      <Link to={`/applications/${application.id}`}>
                        {application.name}
                      </Link>
                    }
                    description={`${application.argocdNamespace}/${application.argocdApplicationName}`}
                  />
                </List.Item>
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
      okText={t('resources.modal.submit')}
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

function browserCookie(name: string): string | undefined {
  return document.cookie
    .split('; ')
    .find((value) => value.startsWith(`${name}=`))
    ?.slice(name.length + 1)
}
