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
} from 'antd'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useSearchParams } from 'react-router'

import {
  createAccessBinding,
  createAccessDeny,
  createAccessGroup,
  createAccessRole,
  createAccessUser,
  disableAccessGroup,
  disableAccessRole,
  disableAccessUser,
  revokeAccessBinding,
  revokeAccessDeny,
  revokeAccessMembership,
  useGetAccessCapabilities,
  useGetAccessScopeOptions,
  useGetCatalogResourceTree,
  useGetSystemStatus,
  useListAccessBindings,
  useListAccessDenies,
  useListAccessGroups,
  useListAccessMemberships,
  useListAccessRoles,
  useListAccessUsers,
} from '@/generated/api'
import type {
  AccessBinding,
  AccessDeny,
  AccessGroup,
  AccessMembership,
  AccessPermission,
  AccessRole,
  AccessUser,
  CatalogOrganizationNode,
} from '@/generated/model'
import { useFeedback } from '@/shared/feedback/useFeedback'
import { GroupMembersModal } from './GroupMembersModal'
import {
  scopePayload,
  scopeResourceOptions,
  type ScopeKind,
} from './scopeResources'

type TabKey =
  'users' | 'groups' | 'roles' | 'memberships' | 'bindings' | 'denies'
type Action = 'user' | 'group' | 'role' | 'binding' | 'deny'
type ManagedGroup = { id: string; name: string; allowedActions: string[] }

interface Fields {
  username?: string
  password?: string
  passwordConfirm?: string
  ownerKind?: 'platform' | 'organization' | 'project'
  ownerId?: string
  name?: string
  oidcViewerOnly?: boolean
  permissions?: string[]
  groupId?: string
  userId?: string
  roleId?: string
  permission?: string
  scopeKind?: ScopeKind
  scopeId?: string
}

const tabs: TabKey[] = [
  'users',
  'groups',
  'roles',
  'memberships',
  'bindings',
  'denies',
]

export function AccessPage() {
  const { t } = useTranslation()
  const feedback = useFeedback()
  const [searchParams, setSearchParams] = useSearchParams()
  const requestedTab = searchParams.get('tab') as TabKey | null
  const activeTab =
    requestedTab && tabs.includes(requestedTab) ? requestedTab : 'users'
  const [form] = Form.useForm<Fields>()
  const [action, setAction] = useState<Action>()
  const [memberGroup, setMemberGroup] = useState<ManagedGroup>()
  const [submitting, setSubmitting] = useState(false)
  const [search, setSearch] = useState('')
  const [status, setStatus] = useState<'all' | 'active' | 'inactive'>('all')
  const [cursors, setCursors] = useState<Partial<Record<TabKey, string>>>({})
  const [cursorHistory, setCursorHistory] = useState<Record<TabKey, string[]>>({
    users: [],
    groups: [],
    roles: [],
    memberships: [],
    bindings: [],
    denies: [],
  })

  const capabilities = useGetAccessCapabilities()
  const users = useListAccessUsers({
    limit: 20,
    status,
    search,
    cursor: cursors.users,
  })
  const groups = useListAccessGroups({
    limit: 20,
    status,
    search,
    cursor: cursors.groups,
  })
  const roles = useListAccessRoles({
    limit: 20,
    status,
    search,
    cursor: cursors.roles,
  })
  const memberships = useListAccessMemberships({
    limit: 20,
    status,
    cursor: cursors.memberships,
  })
  const bindings = useListAccessBindings({
    limit: 20,
    status,
    cursor: cursors.bindings,
  })
  const denies = useListAccessDenies({
    limit: 20,
    status,
    cursor: cursors.denies,
  })
  const resources = useGetCatalogResourceTree()
  const system = useGetSystemStatus()

  const capabilityData =
    capabilities.data?.status === 200
      ? capabilities.data.data.data.collections
      : []
  const permissionData =
    capabilities.data?.status === 200
      ? capabilities.data.data.data.permissions
      : []
  const userData = users.data?.status === 200 ? users.data.data.data : []
  const groupData = groups.data?.status === 200 ? groups.data.data.data : []
  const roleData = roles.data?.status === 200 ? roles.data.data.data : []
  const membershipData =
    memberships.data?.status === 200 ? memberships.data.data.data : []
  const bindingData =
    bindings.data?.status === 200 ? bindings.data.data.data : []
  const denyData = denies.data?.status === 200 ? denies.data.data.data : []
  const pageMeta = {
    users: users.data?.status === 200 ? users.data.data.meta : undefined,
    groups: groups.data?.status === 200 ? groups.data.data.meta : undefined,
    roles: roles.data?.status === 200 ? roles.data.data.meta : undefined,
    memberships:
      memberships.data?.status === 200 ? memberships.data.data.meta : undefined,
    bindings:
      bindings.data?.status === 200 ? bindings.data.data.meta : undefined,
    denies: denies.data?.status === 200 ? denies.data.data.meta : undefined,
  }[activeTab]
  const organizationData =
    resources.data?.status === 200 ? resources.data.data.data : []
  const singleTenant =
    system.data?.status === 200 &&
    system.data.data.data.tenancyMode === 'single'
  const capability = new Map(capabilityData.map((item) => [item.key, item]))
  const visibleTabs = tabs.filter(
    (key) => capability.get(key)?.visible !== false,
  )
  const canManagePlatform = capability.get('users')?.canCreate === true
  const activeQuery = { users, groups, roles, memberships, bindings, denies }[
    activeTab
  ]
  const loading = activeQuery.isPending
  const activeQueryFailed =
    activeQuery.isError ||
    (activeQuery.data !== undefined && activeQuery.data.status !== 200)

  if (
    capabilities.isError ||
    (capabilities.data && capabilities.data.status !== 200)
  ) {
    return (
      <Alert
        type="warning"
        showIcon
        title={t('access.unavailable.title')}
        description={t('access.unavailable.description')}
      />
    )
  }

  const refetch = async () => {
    await Promise.all([
      capabilities.refetch(),
      users.refetch(),
      groups.refetch(),
      roles.refetch(),
      memberships.refetch(),
      bindings.refetch(),
      denies.refetch(),
      resources.refetch(),
    ])
  }
  const open = (next: Action, initial?: Fields) => {
    form.resetFields()
    form.setFieldsValue(initial ?? {})
    setAction(next)
  }
  const mutate = async (
    work: (options: RequestInit) => Promise<{ status: number }>,
    closeModal = true,
  ): Promise<boolean> => {
    const csrf = browserCookie('releasehub_csrf')
    if (!csrf) {
      feedback.error(t('access.mutation.error'))
      return false
    }
    setSubmitting(true)
    try {
      const response = await work({ headers: { 'X-CSRF-Token': csrf } })
      if (response.status !== 201 && response.status !== 204) {
        const code = mutationErrorCode(response)
        let errorKey = 'access.mutation.error'
        if (response.status === 401)
          errorKey = 'access.mutation.unauthenticated'
        else if (response.status === 400) errorKey = 'access.mutation.invalid'
        else if (response.status === 404)
          errorKey = 'access.mutation.notAuthorized'
        else if (code === 'ACCESS_SELF_DISABLE_FORBIDDEN')
          errorKey = 'access.mutation.selfDisable'
        else if (code === 'ACCESS_LAST_PLATFORM_MANAGER')
          errorKey = 'access.mutation.lastManager'
        else if (code === 'ACCESS_RESOURCE_PROTECTED')
          errorKey = 'access.mutation.protected'
        else if (response.status === 409) errorKey = 'access.mutation.conflict'
        feedback.error(t(errorKey))
        return false
      }
      if (closeModal) {
        setAction(undefined)
        form.resetFields()
      }
      feedback.success(t('access.mutation.success'))
      await refetch()
      return true
    } catch {
      feedback.error(t('access.mutation.error'))
      return false
    } finally {
      setSubmitting(false)
    }
  }
  const submit = async (fields: Fields) => {
    if (action === 'user')
      await mutate((options) =>
        createAccessUser(
          { username: fields.username!, initialPassword: fields.password! },
          options,
        ),
      )
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
    if (action === 'binding')
      await mutate((options) =>
        createAccessBinding(
          {
            ...scopePayload(fields, organizationData),
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
            ...scopePayload(fields, organizationData),
            groupId: fields.groupId!,
            permission: fields.permission!,
          },
          options,
        ),
      )
  }
  const destructive = (
    kind: 'user' | 'group' | 'role' | 'membership' | 'binding' | 'deny',
    id: string,
  ) =>
    void mutate((options) => {
      if (kind === 'user') return disableAccessUser(id, options)
      if (kind === 'group') return disableAccessGroup(id, options)
      if (kind === 'role') return disableAccessRole(id, options)
      if (kind === 'membership') return revokeAccessMembership(id, options)
      if (kind === 'binding') return revokeAccessBinding(id, options)
      return revokeAccessDeny(id, options)
    })
  const currentCapability = capability.get(activeTab)
  const primaryAction = currentCapability?.canCreate
    ? createActionForTab(activeTab)
    : undefined
  const resetCurrentPage = () => {
    setCursors((value) => ({ ...value, [activeTab]: undefined }))
    setCursorHistory((value) => ({ ...value, [activeTab]: [] }))
  }
  const nextPage = () => {
    if (!pageMeta?.nextCursor) return
    setCursorHistory((value) => ({
      ...value,
      [activeTab]: [...value[activeTab], cursors[activeTab] ?? ''],
    }))
    setCursors((value) => ({ ...value, [activeTab]: pageMeta.nextCursor }))
  }
  const previousPage = () => {
    const history = cursorHistory[activeTab]
    if (history.length === 0) return
    const previous = history[history.length - 1]
    setCursorHistory((value) => ({
      ...value,
      [activeTab]: history.slice(0, -1),
    }))
    setCursors((value) => ({ ...value, [activeTab]: previous || undefined }))
  }

  return (
    <Space orientation="vertical" size="large" style={{ width: '100%' }}>
      <div>
        <Typography.Title level={2}>{t('access.title')}</Typography.Title>
        <Typography.Paragraph type="secondary">
          {t('access.description')}
        </Typography.Paragraph>
      </div>
      <Card>
        <div
          style={{
            display: 'flex',
            justifyContent: 'space-between',
            gap: 16,
            alignItems: 'center',
            marginBottom: 16,
          }}
        >
          <div>
            <Typography.Title level={4} style={{ margin: 0 }}>
              {t(`access.sections.${activeTab}.title`)}
            </Typography.Title>
            <Typography.Text type="secondary">
              {t(`access.sections.${activeTab}.description`)}
            </Typography.Text>
          </div>
          {primaryAction && (
            <Button type="primary" onClick={() => open(primaryAction)}>
              {t(
                `access.actions.${primaryAction === 'user' ? 'createUser' : `create${capitalize(primaryAction)}`}`,
              )}
            </Button>
          )}
        </div>
        <Space wrap style={{ marginBottom: 12 }}>
          {(['users', 'groups', 'roles'] as TabKey[]).includes(activeTab) && (
            <Input.Search
              allowClear
              value={search}
              placeholder={t('access.filters.search')}
              onChange={(event) => {
                setSearch(event.target.value)
                resetCurrentPage()
              }}
              style={{ width: 280 }}
            />
          )}
          <Select
            value={status}
            onChange={(value) => {
              setStatus(value)
              resetCurrentPage()
            }}
            options={(['all', 'active', 'inactive'] as const).map((value) => ({
              value,
              label: t(`access.filters.${value}`),
            }))}
            style={{ width: 140 }}
          />
        </Space>
        <Tabs
          activeKey={activeTab}
          onChange={(key) => {
            setSearch('')
            setStatus('all')
            setCursors((value) => ({ ...value, [key]: undefined }))
            setCursorHistory((value) => ({ ...value, [key]: [] }))
            setSearchParams({ tab: key })
          }}
          items={visibleTabs.map((key) => ({
            key,
            label: t(`access.tabs.${key}`),
            children:
              key === activeTab && activeQueryFailed ? (
                <Alert
                  type="error"
                  showIcon
                  title={t('access.listError.title')}
                  description={t('access.listError.description')}
                  action={
                    <Button onClick={() => void activeQuery.refetch()}>
                      {t('access.listError.retry')}
                    </Button>
                  }
                />
              ) : (
                renderTable(key, {
                  loading,
                  users: userData,
                  groups: groupData,
                  roles: roleData,
                  memberships: membershipData,
                  bindings: bindingData,
                  denies: denyData,
                  t,
                  open,
                  manageMembers: setMemberGroup,
                  destructive,
                })
              ),
          }))}
        />
        <div style={{ display: 'flex', justifyContent: 'flex-end', gap: 8 }}>
          <Button
            disabled={cursorHistory[activeTab].length === 0}
            onClick={previousPage}
          >
            {t('access.pagination.previous')}
          </Button>
          <Button disabled={!pageMeta?.hasMore} onClick={nextPage}>
            {t('access.pagination.next')}
          </Button>
        </div>
      </Card>
      <AccessModal
        action={action}
        form={form}
        submitting={submitting}
        canManagePlatform={canManagePlatform}
        permissions={permissionData}
        organizations={organizationData}
        singleTenant={singleTenant}
        close={() => !submitting && setAction(undefined)}
        submit={submit}
      />
      {memberGroup && (
        <GroupMembersModal
          group={memberGroup}
          submitting={submitting}
          mutate={mutate}
          close={() => !submitting && setMemberGroup(undefined)}
        />
      )}
    </Space>
  )
}

type TableContext = {
  loading: boolean
  users: AccessUser[]
  groups: AccessGroup[]
  roles: AccessRole[]
  memberships: AccessMembership[]
  bindings: AccessBinding[]
  denies: AccessDeny[]
  t: ReturnType<typeof useTranslation>['t']
  open: (action: Action, fields?: Fields) => void
  manageMembers: (group: ManagedGroup) => void
  destructive: (
    kind: 'user' | 'group' | 'role' | 'membership' | 'binding' | 'deny',
    id: string,
  ) => void
}

function renderTable(key: TabKey, context: TableContext) {
  const actionColumn = <T extends { id: string; allowedActions: string[] }>(
    kind: 'user' | 'group' | 'role' | 'membership' | 'binding' | 'deny',
    describe: (item: T) => string,
  ) => ({
    title: context.t('access.columns.actions'),
    render: (_: unknown, item: T) => (
      <Space>
        {kind === 'group' &&
          item.allowedActions.includes('viewMemberships') && (
            <Button
              size="small"
              onClick={() =>
                context.manageMembers({
                  id: item.id,
                  name: describe(item),
                  allowedActions: item.allowedActions,
                })
              }
            >
              {context.t('access.actions.manageMembers')}
            </Button>
          )}
        {(item.allowedActions.includes('disable') ||
          item.allowedActions.includes('revoke')) && (
          <ActionConfirm
            revoke={item.allowedActions.includes('revoke')}
            record={describe(item)}
            onConfirm={() => context.destructive(kind, item.id)}
          />
        )}
      </Space>
    ),
  })
  if (key === 'users')
    return (
      <Table
        rowKey="id"
        loading={context.loading}
        dataSource={context.users}
        pagination={false}
        locale={{ emptyText: context.t('access.empty') }}
        columns={[
          {
            title: context.t('access.columns.username'),
            dataIndex: 'username',
          },
          {
            title: context.t('access.columns.status'),
            dataIndex: 'disabled',
            render: (disabled: boolean) => <StatusTag active={!disabled} />,
          },
          actionColumn<AccessUser>('user', (item) => item.username),
        ]}
      />
    )
  if (key === 'groups')
    return (
      <Table
        rowKey="id"
        loading={context.loading}
        dataSource={context.groups}
        pagination={false}
        locale={{ emptyText: context.t('access.empty') }}
        columns={[
          { title: context.t('access.columns.name'), dataIndex: 'name' },
          { title: context.t('access.columns.owner'), dataIndex: 'ownerKind' },
          {
            title: context.t('access.columns.status'),
            dataIndex: 'disabled',
            render: (disabled: boolean) => <StatusTag active={!disabled} />,
          },
          actionColumn<AccessGroup>('group', (item) => item.name),
        ]}
      />
    )
  if (key === 'roles')
    return (
      <Table
        rowKey="id"
        loading={context.loading}
        dataSource={context.roles}
        pagination={false}
        locale={{ emptyText: context.t('access.empty') }}
        columns={[
          { title: context.t('access.columns.name'), dataIndex: 'name' },
          { title: context.t('access.columns.owner'), dataIndex: 'ownerKind' },
          {
            title: context.t('access.columns.permissions'),
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
            title: context.t('access.columns.status'),
            dataIndex: 'active',
            render: (active: boolean) => <StatusTag active={active} />,
          },
          actionColumn<AccessRole>('role', (item) => item.name),
        ]}
      />
    )
  if (key === 'memberships')
    return (
      <Table
        rowKey="id"
        loading={context.loading}
        dataSource={context.memberships}
        pagination={false}
        locale={{ emptyText: context.t('access.empty') }}
        columns={[
          {
            title: context.t('access.columns.group'),
            dataIndex: 'groupName',
          },
          {
            title: context.t('access.columns.username'),
            dataIndex: 'username',
          },
          { title: context.t('access.columns.source'), dataIndex: 'source' },
          {
            title: context.t('access.columns.status'),
            dataIndex: 'active',
            render: (active: boolean) => <StatusTag active={active} />,
          },
          actionColumn<AccessMembership>(
            'membership',
            (item) => `${item.username} → ${item.groupName}`,
          ),
        ]}
      />
    )
  if (key === 'bindings')
    return (
      <Table
        rowKey="id"
        loading={context.loading}
        dataSource={context.bindings}
        pagination={false}
        locale={{ emptyText: context.t('access.empty') }}
        columns={[
          {
            title: context.t('access.columns.group'),
            dataIndex: 'groupName',
          },
          {
            title: context.t('access.columns.role'),
            dataIndex: 'roleName',
          },
          { title: context.t('access.columns.scope'), dataIndex: 'scopeKind' },
          {
            title: context.t('access.columns.status'),
            dataIndex: 'active',
            render: (active: boolean) => <StatusTag active={active} />,
          },
          actionColumn<AccessBinding>(
            'binding',
            (item) =>
              `${item.groupName} → ${item.roleName} (${item.scopeKind})`,
          ),
        ]}
      />
    )
  return (
    <Table
      rowKey="id"
      loading={context.loading}
      dataSource={context.denies}
      pagination={false}
      locale={{ emptyText: context.t('access.empty') }}
      columns={[
        {
          title: context.t('access.columns.group'),
          dataIndex: 'groupName',
        },
        {
          title: context.t('access.columns.permission'),
          dataIndex: 'permission',
        },
        { title: context.t('access.columns.scope'), dataIndex: 'scopeKind' },
        {
          title: context.t('access.columns.status'),
          dataIndex: 'active',
          render: (active: boolean) => <StatusTag active={active} />,
        },
        actionColumn<AccessDeny>(
          'deny',
          (item) =>
            `${item.groupName} · ${item.permission} (${item.scopeKind})`,
        ),
      ]}
    />
  )
}

function AccessModal({
  action,
  form,
  submitting,
  canManagePlatform,
  permissions,
  organizations,
  singleTenant,
  close,
  submit,
}: {
  action?: Action
  form: ReturnType<typeof Form.useForm<Fields>>[0]
  submitting: boolean
  canManagePlatform: boolean
  permissions: AccessPermission[]
  organizations: CatalogOrganizationNode[]
  singleTenant: boolean
  close: () => void
  submit: (fields: Fields) => Promise<void>
}) {
  const { t } = useTranslation()
  const ownerKind = Form.useWatch('ownerKind', form)
  const scopeKind = Form.useWatch('scopeKind', form) ?? 'project'
  const selectedScopeID = Form.useWatch('scopeId', form)
  const scopeID =
    scopeKind === 'platform' ? 'platform' : (selectedScopeID ?? '')
  const scopeOptions = useGetAccessScopeOptions(scopeKind, scopeID, {
    query: {
      enabled: (action === 'binding' || action === 'deny') && Boolean(scopeID),
    },
  })
  const options =
    scopeOptions.data?.status === 200 ? scopeOptions.data.data.data : undefined
  const projectOptions = organizations.flatMap((organization) =>
    organization.projects.map((project) => ({
      value: project.id,
      label: `${organization.name} / ${project.name}`,
    })),
  )
  const ownerOptions = (
    canManagePlatform ? ['platform', 'organization', 'project'] : ['project']
  ).filter((kind) => action === 'group' || kind !== 'organization')
  const scopeKinds =
    action === 'deny' || !canManagePlatform
      ? ['project', 'environment', 'application']
      : ['platform', 'project', 'environment', 'application']
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
        {action === 'user' && (
          <>
            <Form.Item
              name="username"
              label={t('access.fields.username')}
              rules={[{ required: true, whitespace: true, max: 128 }]}
            >
              <Input autoComplete="off" />
            </Form.Item>
            <Form.Item
              name="password"
              label={t('access.fields.initialPassword')}
              extra={t('access.fields.initialPasswordHint')}
              rules={[{ required: true, min: 8, max: 72 }]}
            >
              <Input.Password autoComplete="new-password" />
            </Form.Item>
            <Form.Item
              name="passwordConfirm"
              label={t('access.fields.confirmInitialPassword')}
              dependencies={['password']}
              rules={[
                { required: true },
                ({ getFieldValue }) => ({
                  validator(_, value) {
                    if (!value || getFieldValue('password') === value)
                      return Promise.resolve()
                    return Promise.reject(
                      new Error(t('access.fields.passwordMismatch')),
                    )
                  },
                }),
              ]}
            >
              <Input.Password autoComplete="new-password" />
            </Form.Item>
          </>
        )}
        {(action === 'group' || action === 'role') && (
          <>
            <Form.Item
              name="ownerKind"
              label={t('access.fields.owner')}
              initialValue={canManagePlatform ? 'platform' : 'project'}
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
                  options={permissions
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
        {(action === 'binding' || action === 'deny') && (
          <>
            <Form.Item
              name="scopeKind"
              label={t('access.columns.scope')}
              initialValue="project"
              rules={[{ required: true }]}
            >
              <Select
                options={scopeKinds.map((kind) => ({
                  value: kind,
                  label: kind,
                }))}
                onChange={() =>
                  form.setFieldsValue({
                    scopeId: undefined,
                    groupId: undefined,
                    roleId: undefined,
                    permission: undefined,
                  })
                }
              />
            </Form.Item>
            {scopeKind !== 'platform' && (
              <Form.Item
                name="scopeId"
                label={t('access.fields.scopeResource')}
                rules={[{ required: true }]}
              >
                <Select
                  showSearch
                  optionFilterProp="label"
                  options={scopeResourceOptions(
                    scopeKind,
                    organizations,
                    singleTenant,
                  )}
                  onChange={() =>
                    form.setFieldsValue({
                      groupId: undefined,
                      roleId: undefined,
                      permission: undefined,
                    })
                  }
                />
              </Form.Item>
            )}
            <Form.Item
              name="groupId"
              label={t('access.columns.group')}
              rules={[{ required: true }]}
            >
              <Select
                loading={scopeOptions.isFetching}
                options={(options?.groups ?? [])
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
                  loading={scopeOptions.isFetching}
                  options={(options?.roles ?? [])
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
                  loading={scopeOptions.isFetching}
                  options={(options?.permissions ?? []).map((item) => ({
                    value: item.key,
                    label: item.key,
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

function createActionForTab(tab: TabKey): Action | undefined {
  return (
    {
      users: 'user',
      groups: 'group',
      roles: 'role',
      bindings: 'binding',
      denies: 'deny',
    } as Partial<Record<TabKey, Action>>
  )[tab]
}

function capitalize(value: string) {
  return value.charAt(0).toUpperCase() + value.slice(1)
}

function ActionConfirm({
  revoke,
  record,
  onConfirm,
}: {
  revoke: boolean
  record: string
  onConfirm: () => void
}) {
  const { t } = useTranslation()
  return (
    <Popconfirm
      title={t(revoke ? 'access.revoke.confirm' : 'access.disable.confirm')}
      description={t(
        revoke ? 'access.revoke.impact' : 'access.disable.impact',
        { record },
      )}
      onConfirm={onConfirm}
    >
      <Button size="small" danger>
        {t(revoke ? 'access.actions.revoke' : 'access.actions.disable')}
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

function mutationErrorCode(response: { status: number }) {
  const value = response as { data?: { error?: { code?: string } } }
  return value.data?.error?.code
}
