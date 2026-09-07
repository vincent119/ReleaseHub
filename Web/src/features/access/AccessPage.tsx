import {
  Alert,
  Button,
  Card,
  Checkbox,
  Form,
  Input,
  Modal,
  Popconfirm,
  Select,
  Space,
  Table,
  Tabs,
  Tag,
  Typography,
  message,
} from 'antd'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import {
  createAccessBinding,
  createAccessDeny,
  createAccessGroup,
  createAccessMembership,
  createAccessRole,
  disableAccessGroup,
  disableAccessUser,
  useGetAccessManagementSnapshot,
  useGetCatalogResourceTree,
} from '@/generated/api'
import type {
  AccessBinding,
  AccessDeny,
  AccessGroup,
  AccessManagementSnapshot,
  AccessRole,
  AccessUser,
  CatalogOrganizationNode,
} from '@/generated/model'

type Action = 'group' | 'role' | 'membership' | 'binding' | 'deny'
type ScopeKind = 'project' | 'environment' | 'application'
interface Fields {
  ownerKind?: 'platform' | 'organization' | 'project'
  ownerId?: string
  name?: string
  oidcViewerOnly?: boolean
  permissions?: string[]
  groupId?: string
  userId?: string
  roleId?: string
  permission?: string
  organizationId?: string
  projectId?: string
  scopeKind?: ScopeKind
  environmentId?: string
  applicationId?: string
}

export function AccessPage() {
  const { t } = useTranslation()
  const [form] = Form.useForm<Fields>()
  const [action, setAction] = useState<Action>()
  const [submitting, setSubmitting] = useState(false)
  const access = useGetAccessManagementSnapshot()
  const resources = useGetCatalogResourceTree()
  if (access.isError || (access.data && access.data.status !== 200))
    return (
      <Alert
        type="warning"
        showIcon
        message={t('access.unavailable.title')}
        description={t('access.unavailable.description')}
      />
    )

  const value = access.data?.status === 200 ? access.data.data.data : undefined
  const organizations =
    resources.data?.status === 200 ? resources.data.data.data : []
  const managedProjects = new Set(value?.managedProjectIds ?? [])
  const groupNames = new Map(value?.groups.map((item) => [item.id, item.name]))
  const roleNames = new Map(value?.roles.map((item) => [item.id, item.name]))
  const canManageGroup = (group: AccessGroup) =>
    Boolean(
      value?.canManagePlatform ||
      (group.ownerKind === 'project' &&
        group.ownerId &&
        managedProjects.has(group.ownerId)),
    )
  const open = (next: Action, initial?: Fields) => {
    form.resetFields()
    form.setFieldsValue(initial ?? {})
    setAction(next)
  }
  const mutate = async (
    work: (options: RequestInit) => Promise<{ status: number }>,
  ) => {
    const csrf = browserCookie('releasehub_csrf')
    if (!csrf) return void message.error(t('access.mutation.error'))
    setSubmitting(true)
    try {
      const response = await work({ headers: { 'X-CSRF-Token': csrf } })
      if (response.status !== 201 && response.status !== 204)
        return void message.error(
          response.status === 404
            ? t('access.mutation.notAuthorized')
            : t('access.mutation.conflict'),
        )
      setAction(undefined)
      message.success(t('access.mutation.success'))
      await Promise.all([access.refetch(), resources.refetch()])
    } catch {
      message.error(t('access.mutation.error'))
    } finally {
      setSubmitting(false)
    }
  }
  const submit = async (fields: Fields) => {
    if (action === 'group')
      await mutate((options) =>
        createAccessGroup(
          {
            ownerKind: fields.ownerKind!,
            ownerId: fields.ownerId,
            name: fields.name!,
            oidcViewerOnly: fields.oidcViewerOnly ?? false,
          },
          options,
        ),
      )
    if (action === 'role')
      await mutate((options) =>
        createAccessRole(
          {
            ownerKind: fields.ownerKind === 'platform' ? 'platform' : 'project',
            ownerId: fields.ownerId,
            name: fields.name!,
            permissions: fields.permissions ?? [],
          },
          options,
        ),
      )
    if (action === 'membership')
      await mutate((options) =>
        createAccessMembership(
          fields.groupId!,
          { userId: fields.userId! },
          options,
        ),
      )
    if (action === 'binding')
      await mutate((options) =>
        createAccessBinding(
          {
            ...scopePayload(fields),
            groupId: fields.groupId!,
            roleId: fields.roleId!,
          },
          options,
        ),
      )
    if (action === 'deny')
      await mutate((options) =>
        createAccessDeny(
          {
            ...scopePayload(fields),
            groupId: fields.groupId!,
            permission: fields.permission!,
          },
          options,
        ),
      )
  }
  const disable = (kind: 'group' | 'user', id: string) =>
    void mutate((options) =>
      kind === 'group'
        ? disableAccessGroup(id, options)
        : disableAccessUser(id, options),
    )

  return (
    <Space orientation="vertical" size="large" style={{ width: '100%' }}>
      <div>
        <Typography.Title level={2}>{t('access.title')}</Typography.Title>
        <Typography.Paragraph type="secondary">
          {t('access.description')}
        </Typography.Paragraph>
      </div>
      <Space wrap>
        <Button type="primary" onClick={() => open('group')}>
          {t('access.actions.createGroup')}
        </Button>
        <Button onClick={() => open('role')}>
          {t('access.actions.createRole')}
        </Button>
        <Button onClick={() => open('binding')}>
          {t('access.actions.createBinding')}
        </Button>
        <Button onClick={() => open('deny')}>
          {t('access.actions.createDeny')}
        </Button>
      </Space>
      <Card>
        <Tabs
          items={[
            {
              key: 'users',
              label: t('access.tabs.users'),
              children: (
                <Table<AccessUser>
                  rowKey="id"
                  loading={access.isPending}
                  pagination={false}
                  dataSource={value?.users ?? []}
                  locale={{ emptyText: t('access.empty') }}
                  columns={[
                    {
                      title: t('access.columns.username'),
                      dataIndex: 'username',
                    },
                    {
                      title: t('access.columns.status'),
                      dataIndex: 'disabled',
                      render: (disabled: boolean) => (
                        <StatusTag active={!disabled} />
                      ),
                    },
                    ...(value?.canManagePlatform
                      ? [
                          {
                            title: t('access.columns.actions'),
                            render: (_: unknown, user: AccessUser) =>
                              user.disabled ? null : (
                                <DisableConfirm
                                  onConfirm={() => disable('user', user.id)}
                                />
                              ),
                          },
                        ]
                      : []),
                  ]}
                />
              ),
            },
            {
              key: 'groups',
              label: t('access.tabs.groups'),
              children: (
                <Table<AccessGroup>
                  rowKey="id"
                  loading={access.isPending}
                  pagination={false}
                  dataSource={value?.groups ?? []}
                  locale={{ emptyText: t('access.empty') }}
                  columns={[
                    { title: t('access.columns.name'), dataIndex: 'name' },
                    {
                      title: t('access.columns.owner'),
                      dataIndex: 'ownerKind',
                    },
                    {
                      title: t('access.columns.oidc'),
                      dataIndex: 'oidcViewerOnly',
                      render: (enabled: boolean) =>
                        enabled ? t('common.enabled') : t('common.disabled'),
                    },
                    {
                      title: t('access.columns.status'),
                      dataIndex: 'disabled',
                      render: (disabled: boolean) => (
                        <StatusTag active={!disabled} />
                      ),
                    },
                    {
                      title: t('access.columns.actions'),
                      render: (_: unknown, group: AccessGroup) =>
                        canManageGroup(group) && !group.disabled ? (
                          <Space>
                            <Button
                              size="small"
                              onClick={() =>
                                open('membership', { groupId: group.id })
                              }
                            >
                              {t('access.actions.addMember')}
                            </Button>
                            <DisableConfirm
                              onConfirm={() => disable('group', group.id)}
                            />
                          </Space>
                        ) : null,
                    },
                  ]}
                />
              ),
            },
            {
              key: 'roles',
              label: t('access.tabs.roles'),
              children: (
                <Table<AccessRole>
                  rowKey="id"
                  loading={access.isPending}
                  pagination={false}
                  dataSource={value?.roles ?? []}
                  locale={{ emptyText: t('access.empty') }}
                  columns={[
                    { title: t('access.columns.name'), dataIndex: 'name' },
                    {
                      title: t('access.columns.owner'),
                      dataIndex: 'ownerKind',
                    },
                    {
                      title: t('access.columns.permissions'),
                      dataIndex: 'permissions',
                      render: (items: string[]) => (
                        <Space wrap>
                          {items.map((item) => (
                            <Tag key={item}>{item}</Tag>
                          ))}
                        </Space>
                      ),
                    },
                    {
                      title: t('access.columns.status'),
                      dataIndex: 'active',
                      render: (active: boolean) => (
                        <StatusTag active={active} />
                      ),
                    },
                  ]}
                />
              ),
            },
            {
              key: 'bindings',
              label: t('access.tabs.bindings'),
              children: (
                <Table<AccessBinding>
                  rowKey="id"
                  loading={access.isPending}
                  pagination={false}
                  dataSource={value?.bindings ?? []}
                  locale={{ emptyText: t('access.empty') }}
                  columns={[
                    {
                      title: t('access.columns.group'),
                      dataIndex: 'groupId',
                      render: (id: string) => groupNames.get(id) ?? id,
                    },
                    {
                      title: t('access.columns.role'),
                      dataIndex: 'roleId',
                      render: (id: string) => roleNames.get(id) ?? id,
                    },
                    {
                      title: t('access.columns.scope'),
                      render: (_: unknown, item: AccessBinding) =>
                        formatScope(item, organizations),
                    },
                    {
                      title: t('access.columns.status'),
                      dataIndex: 'active',
                      render: (active: boolean) => (
                        <StatusTag active={active} />
                      ),
                    },
                  ]}
                />
              ),
            },
            {
              key: 'denies',
              label: t('access.tabs.denies'),
              children: (
                <Table<AccessDeny>
                  rowKey="id"
                  loading={access.isPending}
                  pagination={false}
                  dataSource={value?.denies ?? []}
                  locale={{ emptyText: t('access.empty') }}
                  columns={[
                    {
                      title: t('access.columns.group'),
                      dataIndex: 'groupId',
                      render: (id: string) => groupNames.get(id) ?? id,
                    },
                    {
                      title: t('access.columns.permission'),
                      dataIndex: 'permission',
                    },
                    {
                      title: t('access.columns.scope'),
                      render: (_: unknown, item: AccessDeny) =>
                        formatScope(item, organizations),
                    },
                    {
                      title: t('access.columns.status'),
                      dataIndex: 'active',
                      render: (active: boolean) => (
                        <StatusTag active={active} />
                      ),
                    },
                  ]}
                />
              ),
            },
          ]}
        />
      </Card>
      <AccessModal
        action={action}
        form={form}
        submitting={submitting}
        value={value}
        organizations={organizations}
        managedProjects={managedProjects}
        close={() => !submitting && setAction(undefined)}
        submit={submit}
      />
    </Space>
  )
}

function AccessModal({
  action,
  form,
  submitting,
  value,
  organizations,
  managedProjects,
  close,
  submit,
}: {
  action?: Action
  form: ReturnType<typeof Form.useForm<Fields>>[0]
  submitting: boolean
  value?: AccessManagementSnapshot
  organizations: CatalogOrganizationNode[]
  managedProjects: Set<string>
  close: () => void
  submit: (fields: Fields) => Promise<void>
}) {
  const { t } = useTranslation()
  const ownerKind = Form.useWatch('ownerKind', form)
  const organizationID = Form.useWatch('organizationId', form)
  const projectID = Form.useWatch('projectId', form)
  const scopeKind = Form.useWatch('scopeKind', form)
  const environmentID = Form.useWatch('environmentId', form)
  const organization = organizations.find((item) => item.id === organizationID)
  const project = organization?.projects.find((item) => item.id === projectID)
  const environment = project?.environments.find(
    (item) => item.id === environmentID,
  )
  const projectOptions = organizations.flatMap((org) =>
    org.projects
      .filter(
        (item) => value?.canManagePlatform || managedProjects.has(item.id),
      )
      .map((item) => ({ value: item.id, label: `${org.name} / ${item.name}` })),
  )
  const ownerOptions = (
    value?.canManagePlatform
      ? ['platform', 'organization', 'project']
      : ['project']
  ).filter((kind) => action === 'group' || kind !== 'organization')
  return (
    <Modal
      open={Boolean(action)}
      title={action ? t(`access.modal.${action}`) : ''}
      okText={t('access.modal.submit')}
      cancelText={t('access.modal.cancel')}
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
        {(action === 'group' || action === 'role') && (
          <>
            <Form.Item
              name="ownerKind"
              label={t('access.fields.owner')}
              initialValue={value?.canManagePlatform ? 'platform' : 'project'}
              rules={[{ required: true }]}
            >
              <Select
                options={ownerOptions.map((kind) => ({
                  value: kind,
                  label: kind,
                }))}
              />
            </Form.Item>
            {ownerKind === 'organization' && (
              <Form.Item
                name="ownerId"
                label="Organization"
                rules={[{ required: true }]}
              >
                <Select
                  options={organizations.map((item) => ({
                    value: item.id,
                    label: item.name,
                  }))}
                />
              </Form.Item>
            )}
            {ownerKind === 'project' && (
              <Form.Item
                name="ownerId"
                label="Project"
                rules={[{ required: true }]}
              >
                <Select options={projectOptions} />
              </Form.Item>
            )}
            <Form.Item
              name="name"
              label={t('access.fields.name')}
              rules={[{ required: true, whitespace: true, max: 128 }]}
            >
              <Input />
            </Form.Item>
            {action === 'group' && (
              <Form.Item name="oidcViewerOnly" valuePropName="checked">
                <Checkbox>{t('access.fields.oidcViewerOnly')}</Checkbox>
              </Form.Item>
            )}
            {action === 'role' && (
              <Form.Item
                name="permissions"
                label={t('access.fields.permissions')}
                rules={[{ required: true }]}
              >
                <Select
                  mode="multiple"
                  options={value?.permissions
                    .filter(
                      (item) =>
                        ownerKind === 'platform' || item.projectRoleDelegable,
                    )
                    .map((item) => ({ value: item.key, label: item.key }))}
                />
              </Form.Item>
            )}
          </>
        )}
        {action === 'membership' && (
          <>
            <Form.Item name="groupId" hidden>
              <Input />
            </Form.Item>
            <Form.Item
              name="userId"
              label={t('access.fields.user')}
              rules={[{ required: true }]}
            >
              <Select
                showSearch
                optionFilterProp="label"
                options={value?.users
                  .filter((item) => !item.disabled)
                  .map((item) => ({ value: item.id, label: item.username }))}
              />
            </Form.Item>
          </>
        )}
        {(action === 'binding' || action === 'deny') && (
          <>
            <Form.Item
              name="groupId"
              label={t('access.columns.group')}
              rules={[{ required: true }]}
            >
              <Select
                options={value?.groups
                  .filter((item) => !item.disabled)
                  .map((item) => ({ value: item.id, label: item.name }))}
              />
            </Form.Item>
            {action === 'binding' ? (
              <Form.Item
                name="roleId"
                label={t('access.columns.role')}
                rules={[{ required: true }]}
              >
                <Select
                  options={value?.roles
                    .filter((item) => item.active)
                    .map((item) => ({ value: item.id, label: item.name }))}
                />
              </Form.Item>
            ) : (
              <Form.Item
                name="permission"
                label={t('access.columns.permission')}
                rules={[{ required: true }]}
              >
                <Select
                  options={value?.permissions.map((item) => ({
                    value: item.key,
                    label: item.key,
                  }))}
                />
              </Form.Item>
            )}
            <Form.Item
              name="organizationId"
              label="Organization"
              rules={[{ required: true }]}
            >
              <Select
                options={organizations.map((item) => ({
                  value: item.id,
                  label: item.name,
                }))}
                onChange={() =>
                  form.setFieldsValue({
                    projectId: undefined,
                    environmentId: undefined,
                    applicationId: undefined,
                  })
                }
              />
            </Form.Item>
            <Form.Item
              name="projectId"
              label="Project"
              rules={[{ required: true }]}
            >
              <Select
                options={organization?.projects
                  .filter(
                    (item) =>
                      value?.canManagePlatform || managedProjects.has(item.id),
                  )
                  .map((item) => ({ value: item.id, label: item.name }))}
                onChange={() =>
                  form.setFieldsValue({
                    environmentId: undefined,
                    applicationId: undefined,
                  })
                }
              />
            </Form.Item>
            <Form.Item
              name="scopeKind"
              label={t('access.columns.scope')}
              initialValue="project"
              rules={[{ required: true }]}
            >
              <Select
                options={['project', 'environment', 'application'].map(
                  (item) => ({ value: item, label: item }),
                )}
              />
            </Form.Item>
            {(scopeKind === 'environment' || scopeKind === 'application') && (
              <Form.Item
                name="environmentId"
                label="Environment"
                rules={[{ required: true }]}
              >
                <Select
                  options={project?.environments.map((item) => ({
                    value: item.id,
                    label: item.name,
                  }))}
                  onChange={() =>
                    form.setFieldsValue({ applicationId: undefined })
                  }
                />
              </Form.Item>
            )}
            {scopeKind === 'application' && (
              <Form.Item
                name="applicationId"
                label="Application"
                rules={[{ required: true }]}
              >
                <Select
                  options={environment?.applications.map((item) => ({
                    value: item.id,
                    label: item.name,
                  }))}
                />
              </Form.Item>
            )}
          </>
        )}
      </Form>
    </Modal>
  )
}

function scopePayload(fields: Fields) {
  return {
    organizationId: fields.organizationId!,
    projectId: fields.projectId!,
    scopeKind: fields.scopeKind!,
    environmentId: fields.environmentId,
    applicationId: fields.applicationId,
  }
}
function formatScope(
  item: AccessBinding | AccessDeny,
  organizations: CatalogOrganizationNode[],
) {
  const org = organizations.find((value) => value.id === item.organizationId)
  const project = org?.projects.find((value) => value.id === item.projectId)
  const environment = project?.environments.find(
    (value) => value.id === item.environmentId,
  )
  const application = environment?.applications.find(
    (value) => value.id === item.applicationId,
  )
  return (
    [org?.name, project?.name, environment?.name, application?.name]
      .filter(Boolean)
      .join(' / ') || item.scopeKind
  )
}
function DisableConfirm({ onConfirm }: { onConfirm: () => void }) {
  const { t } = useTranslation()
  return (
    <Popconfirm title={t('access.disable.confirm')} onConfirm={onConfirm}>
      <Button size="small" danger>
        {t('access.actions.disable')}
      </Button>
    </Popconfirm>
  )
}
function StatusTag({ active }: { active: boolean }) {
  const { t } = useTranslation()
  return (
    <Tag color={active ? 'green' : 'default'}>
      {active ? t('access.status.active') : t('access.status.inactive')}
    </Tag>
  )
}
function browserCookie(name: string): string | undefined {
  return document.cookie
    .split('; ')
    .find((value) => value.startsWith(`${name}=`))
    ?.slice(name.length + 1)
}
