import {
  AuditOutlined,
  FilterOutlined,
  ReloadOutlined,
} from '@ant-design/icons'
import {
  Alert,
  AutoComplete,
  Button,
  Card,
  Descriptions,
  Drawer,
  Empty,
  Flex,
  Form,
  Input,
  Select,
  Skeleton,
  Space,
  Table,
  Tag,
  Typography,
} from 'antd'
import type { TableProps } from 'antd'
import { useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import {
  getGetAuditCapabilitiesQueryKey,
  getGetAuditEventQueryKey,
  getListAuditFilterOptionsQueryKey,
  getListAuditEventsQueryKey,
  useGetAuditCapabilities,
  useGetAuditEvent,
  useListAuditFilterOptions,
  useListAuditEvents,
} from '@/generated/api'
import type {
  AuditEventSummary,
  AuditFilterOptionField,
  AuditScopeRoot,
  ListAuditEventsParams,
} from '@/generated/model'

import styles from './AuditPage.module.css'

type FilterValues = {
  action?: string
  resourceType?: string
  actor?: string
  requestId?: string
  occurredFrom?: string
  occurredTo?: string
}

export function AuditPage({ principalID }: { principalID: string }) {
  const { t } = useTranslation()
  const capabilities = useGetAuditCapabilities({
    query: {
      queryKey: [...getGetAuditCapabilitiesQueryKey(), principalID],
    },
  })
  const capability =
    capabilities.data?.status === 200 ? capabilities.data.data.data : undefined
  const [selectedScopeKey, setSelectedScopeKey] = useState<string>()
  const [filters, setFilters] = useState<FilterValues>({})
  const [cursors, setCursors] = useState<string[]>([''])
  const [pageIndex, setPageIndex] = useState(0)
  const [selectedID, setSelectedID] = useState<string>()

  const scope =
    capability?.scopeRoots.find(
      (candidate) => scopeKey(candidate) === selectedScopeKey,
    ) ?? capability?.scopeRoots[0]

  const params = useMemo<ListAuditEventsParams>(
    () => ({
      scopeKind: scope?.kind ?? 'platform',
      scopeId: scope?.id,
      action: clean(filters.action),
      resourceType: clean(filters.resourceType),
      actor: clean(filters.actor),
      requestId: clean(filters.requestId),
      occurredFrom: toISOString(filters.occurredFrom),
      occurredTo: toISOString(filters.occurredTo),
      cursor: cursors[pageIndex] || undefined,
      limit: 20,
    }),
    [cursors, filters, pageIndex, scope],
  )
  const events = useListAuditEvents(params, {
    query: {
      enabled: Boolean(capability?.visible && scope),
      queryKey: [...getListAuditEventsQueryKey(params), principalID],
    },
  })
  const values = events.data?.status === 200 ? events.data.data.data : []
  const meta = events.data?.status === 200 ? events.data.data.meta : undefined
  const detail = useGetAuditEvent(selectedID ?? '', {
    query: {
      enabled: Boolean(selectedID),
      queryKey: [...getGetAuditEventQueryKey(selectedID ?? ''), principalID],
    },
  })

  const resetPage = () => {
    setCursors([''])
    setPageIndex(0)
  }
  const applyFilters = (value: FilterValues) => {
    setFilters(value)
    resetPage()
  }
  const changeScope = (value: string) => {
    setSelectedScopeKey(value)
    resetPage()
  }
  const nextPage = () => {
    if (!meta?.nextCursor) return
    setCursors((current) => {
      const next = current.slice(0, pageIndex + 1)
      next.push(meta.nextCursor ?? '')
      return next
    })
    setPageIndex((current) => current + 1)
  }
  const refresh = () => {
    if (pageIndex > 0) {
      resetPage()
      return
    }
    void events.refetch()
  }

  if (capabilities.isPending) return <Skeleton active paragraph={{ rows: 8 }} />
  if (
    capabilities.isError ||
    capabilities.data?.status !== 200 ||
    !capability?.visible
  )
    return (
      <Alert
        type="error"
        showIcon
        title={t('audit.unavailable.title')}
        description={t('audit.unavailable.description')}
      />
    )

  const queryFailed =
    events.isError || (events.data && events.data.status !== 200)

  return (
    <Space orientation="vertical" size="large" className={styles.page}>
      <Flex justify="space-between" align="start" gap="middle" wrap>
        <div>
          <Typography.Title level={2}>{t('audit.title')}</Typography.Title>
          <Typography.Paragraph type="secondary">
            {t('audit.description')}
          </Typography.Paragraph>
        </div>
        <Button
          aria-label={t('audit.actions.refresh')}
          icon={<ReloadOutlined />}
          loading={events.isFetching}
          onClick={refresh}
        >
          {t('audit.actions.refresh')}
        </Button>
      </Flex>

      <Card className={styles.filterCard}>
        <Form<FilterValues>
          layout="vertical"
          initialValues={filters}
          onFinish={applyFilters}
        >
          <div className={styles.filterGrid}>
            <Form.Item label={t('audit.filters.scope')}>
              <Select
                aria-label={t('audit.filters.scope')}
                value={scope ? scopeKey(scope) : undefined}
                options={capability.scopeRoots.map((root) => ({
                  value: scopeKey(root),
                  label: root.label,
                }))}
                onChange={changeScope}
              />
            </Form.Item>
            <Form.Item name="action" label={t('audit.filters.action')}>
              <AuditFilterAutocomplete
                ariaLabel={t('audit.filters.action')}
                field="action"
                scope={scope}
                occurredFrom={filters.occurredFrom}
                occurredTo={filters.occurredTo}
                principalID={principalID}
              />
            </Form.Item>
            <Form.Item
              name="resourceType"
              label={t('audit.filters.resourceType')}
            >
              <AuditFilterAutocomplete
                ariaLabel={t('audit.filters.resourceType')}
                field="resourceType"
                scope={scope}
                occurredFrom={filters.occurredFrom}
                occurredTo={filters.occurredTo}
                principalID={principalID}
              />
            </Form.Item>
            <Form.Item name="actor" label={t('audit.filters.actor')}>
              <AuditFilterAutocomplete
                ariaLabel={t('audit.filters.actor')}
                field="actor"
                scope={scope}
                occurredFrom={filters.occurredFrom}
                occurredTo={filters.occurredTo}
                principalID={principalID}
              />
            </Form.Item>
            <Form.Item name="requestId" label={t('audit.filters.requestId')}>
              <Input allowClear maxLength={255} />
            </Form.Item>
            <Form.Item
              name="occurredFrom"
              label={t('audit.filters.occurredFrom')}
            >
              <Input type="datetime-local" />
            </Form.Item>
            <Form.Item name="occurredTo" label={t('audit.filters.occurredTo')}>
              <Input type="datetime-local" />
            </Form.Item>
          </div>
          <Flex justify="end">
            <Button
              aria-label={t('audit.actions.apply')}
              type="primary"
              htmlType="submit"
              icon={<FilterOutlined />}
            >
              {t('audit.actions.apply')}
            </Button>
          </Flex>
        </Form>
      </Card>

      {queryFailed ? (
        <Alert
          type="error"
          showIcon
          title={t('audit.queryError.title')}
          description={t('audit.queryError.description')}
          action={
            <Button
              aria-label={t('audit.actions.retry')}
              onClick={() => void events.refetch()}
            >
              {t('audit.actions.retry')}
            </Button>
          }
        />
      ) : (
        <Card className={styles.resultsCard}>
          <div className={styles.desktopTable}>
            <Table<AuditEventSummary>
              rowKey="id"
              columns={auditColumns(t, setSelectedID)}
              dataSource={values}
              loading={events.isPending}
              pagination={false}
              locale={{ emptyText: auditEmpty(t, filters) }}
              scroll={{ x: 1040 }}
            />
          </div>
          <div className={styles.mobileCards} aria-live="polite">
            {events.isPending ? (
              <Skeleton active />
            ) : values.length ? (
              values.map((event) => (
                <button
                  type="button"
                  key={event.id}
                  className={styles.eventCard}
                  onClick={() => setSelectedID(event.id)}
                >
                  <Flex justify="space-between" gap="small">
                    <strong>{event.action}</strong>
                    <Tag>{event.scope.kind}</Tag>
                  </Flex>
                  <span>{event.resource.type}</span>
                  <span>{new Date(event.occurredAt).toLocaleString()}</span>
                </button>
              ))
            ) : (
              <Empty description={auditEmpty(t, filters)} />
            )}
          </div>
          <Flex justify="space-between" align="center" className={styles.pager}>
            <Button
              disabled={pageIndex === 0}
              onClick={() => setPageIndex((current) => current - 1)}
            >
              {t('audit.actions.previous')}
            </Button>
            <Typography.Text type="secondary">
              {t('audit.page', { page: pageIndex + 1 })}
            </Typography.Text>
            <Button disabled={!meta?.hasMore} onClick={nextPage}>
              {t('audit.actions.next')}
            </Button>
          </Flex>
        </Card>
      )}

      <AuditDetailDrawer
        open={Boolean(selectedID)}
        loading={detail.isPending}
        failed={detail.isError || (detail.data && detail.data.status !== 200)}
        value={detail.data?.status === 200 ? detail.data.data.data : undefined}
        onClose={() => setSelectedID(undefined)}
      />
    </Space>
  )
}

function AuditFilterAutocomplete({
  id,
  value,
  onChange,
  ariaLabel,
  field,
  scope,
  occurredFrom,
  occurredTo,
  principalID,
}: {
  id?: string
  value?: string
  onChange?: (value?: string) => void
  ariaLabel: string
  field: AuditFilterOptionField
  scope?: AuditScopeRoot
  occurredFrom?: string
  occurredTo?: string
  principalID: string
}) {
  const [search, setSearch] = useState('')
  const [debouncedSearch, setDebouncedSearch] = useState('')
  const [dropdownFocused, setDropdownFocused] = useState(false)

  useEffect(() => {
    const timeout = window.setTimeout(
      () => setDebouncedSearch(search.trim()),
      300,
    )
    return () => window.clearTimeout(timeout)
  }, [search])

  const params = {
    scopeKind: scope?.kind ?? 'platform',
    scopeId: scope?.id,
    occurredFrom: toISOString(occurredFrom),
    occurredTo: toISOString(occurredTo),
    field,
    search: clean(debouncedSearch),
    limit: 20,
  }
  const optionsQuery = useListAuditFilterOptions(params, {
    query: {
      enabled: Boolean(scope),
      queryKey: [...getListAuditFilterOptionsQueryKey(params), principalID],
      staleTime: 30_000,
    },
  })
  const options =
    optionsQuery.data?.status === 200 ? optionsQuery.data.data.data : []

  return (
    <AutoComplete
      id={id}
      aria-label={ariaLabel}
      allowClear
      value={value}
      options={options}
      open={dropdownFocused && options.length > 0}
      filterOption={false}
      onFocus={() => setDropdownFocused(true)}
      onBlur={() => setDropdownFocused(false)}
      onSearch={(next) => {
        setDropdownFocused(true)
        setSearch(next)
      }}
      onSelect={() => setDropdownFocused(false)}
      onKeyDown={(event) => {
        if (event.key === 'Escape') setDropdownFocused(false)
      }}
      onChange={(next) => {
        setSearch(next ?? '')
        onChange?.(next || undefined)
      }}
    />
  )
}

function AuditDetailDrawer({
  open,
  loading,
  failed,
  value,
  onClose,
}: {
  open: boolean
  loading: boolean
  failed: boolean | undefined
  value?: import('@/generated/model').AuditEventDetail
  onClose: () => void
}) {
  const { t } = useTranslation()
  return (
    <Drawer
      open={open}
      size="large"
      rootClassName={styles.detailDrawer}
      title={t('audit.detail.title')}
      onClose={onClose}
    >
      {loading ? (
        <Skeleton active />
      ) : failed || !value ? (
        <Alert type="error" showIcon title={t('audit.detail.unavailable')} />
      ) : (
        <Space orientation="vertical" size="large" className={styles.detail}>
          <Descriptions column={1} bordered size="small">
            <Descriptions.Item label={t('audit.columns.time')}>
              {new Date(value.occurredAt).toLocaleString()}
            </Descriptions.Item>
            <Descriptions.Item label={t('audit.columns.actor')}>
              {actorLabel(value)}
            </Descriptions.Item>
            <Descriptions.Item label={t('audit.columns.action')}>
              {value.action}
            </Descriptions.Item>
            <Descriptions.Item label={t('audit.columns.resource')}>
              {value.resource.type} · {value.resource.id}
            </Descriptions.Item>
            <Descriptions.Item label={t('audit.columns.scope')}>
              {value.scope.kind}
            </Descriptions.Item>
            <Descriptions.Item label={t('audit.columns.requestId')}>
              {value.requestId ?? t('audit.values.notSet')}
            </Descriptions.Item>
          </Descriptions>
          <div>
            <Typography.Title level={5}>
              {t('audit.detail.metadata')}
            </Typography.Title>
            {value.metadataTruncated && (
              <Alert
                type="warning"
                showIcon
                title={t('audit.detail.truncated')}
                className={styles.metadataWarning}
              />
            )}
            <pre className={styles.metadata} tabIndex={0}>
              {JSON.stringify(value.metadata, null, 2)}
            </pre>
          </div>
        </Space>
      )}
    </Drawer>
  )
}

function auditColumns(
  t: ReturnType<typeof useTranslation>['t'],
  select: (id: string) => void,
): TableProps<AuditEventSummary>['columns'] {
  return [
    {
      title: t('audit.columns.time'),
      dataIndex: 'occurredAt',
      width: 190,
      render: (value: string) => new Date(value).toLocaleString(),
    },
    {
      title: t('audit.columns.actor'),
      dataIndex: 'actor',
      width: 160,
      render: (_, value) => actorLabel(value),
    },
    { title: t('audit.columns.action'), dataIndex: 'action', ellipsis: true },
    {
      title: t('audit.columns.resource'),
      dataIndex: 'resource',
      ellipsis: true,
      render: (_, value) => `${value.resource.type} · ${value.resource.id}`,
    },
    {
      title: t('audit.columns.scope'),
      dataIndex: 'scope',
      width: 130,
      render: (_, value) => <Tag>{value.scope.kind}</Tag>,
    },
    {
      title: t('audit.columns.details'),
      key: 'details',
      width: 110,
      render: (_, value) => (
        <Button
          aria-label={t('audit.actions.view')}
          type="link"
          icon={<AuditOutlined />}
          onClick={() => select(value.id)}
        >
          {t('audit.actions.view')}
        </Button>
      ),
    },
  ]
}

function actorLabel(value: AuditEventSummary) {
  return value.actor.displayName ?? value.actor.kind
}

function scopeKey(scope: AuditScopeRoot) {
  return `${scope.kind}:${scope.id ?? 'platform'}`
}

function clean(value?: string) {
  return value?.trim() || undefined
}

function toISOString(value?: string) {
  return value ? new Date(value).toISOString() : undefined
}

function auditEmpty(
  t: ReturnType<typeof useTranslation>['t'],
  filters: FilterValues,
) {
  return Object.values(filters).some(Boolean)
    ? t('audit.empty.filtered')
    : t('audit.empty.scope')
}
