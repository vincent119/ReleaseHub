import { Alert, Descriptions, Modal, Spin, Table, Tabs, Tooltip } from 'antd'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import {
  useGetCatalogApplicationRuntimePodLogs,
  useGetCatalogApplicationRuntimeResource,
  useListCatalogApplicationRuntimeEvents,
} from '@/generated/api'
import type {
  GetCatalogApplicationRuntimeResourceParams,
  RuntimeResourceNode,
} from '@/generated/model'

import styles from './RuntimeTopologyPanel.module.css'
import dialogStyles from './RuntimeResourceDialog.module.css'

interface Props {
  applicationId: string
  resource?: RuntimeResourceNode
  onClose: () => void
  onAfterClose: () => void
}

export function RuntimeResourceDialog({
  applicationId,
  resource,
  onClose,
  onAfterClose,
}: Props) {
  const { t } = useTranslation()
  const [tab, setTab] = useState('summary')
  const params = resourceParams(resource)
  const detail = useGetCatalogApplicationRuntimeResource(
    applicationId,
    params,
    {
      query: { enabled: Boolean(resource) && tab === 'manifest' },
    },
  )
  const events = useListCatalogApplicationRuntimeEvents(applicationId, params, {
    query: { enabled: Boolean(resource) && tab === 'events' },
  })
  const logs = useGetCatalogApplicationRuntimePodLogs(
    applicationId,
    { ...params, tailLines: 500 },
    { query: { enabled: resource?.kind === 'Pod' && tab === 'logs' } },
  )
  return (
    <Modal
      open={Boolean(resource)}
      title={resource?.name}
      centered
      width="var(--runtime-dialog-width)"
      className={dialogStyles.dialog}
      classNames={{
        container: dialogStyles.container,
        header: dialogStyles.header,
        title: dialogStyles.title,
        body: dialogStyles.body,
      }}
      footer={null}
      focusable={{ trap: true, focusTriggerAfterClose: false }}
      modalRender={(content) => (
        <div
          onKeyDown={(event) => {
            if (event.key !== 'Tab') return
            // 元件庫依賴 focusin 回收焦點；到瀏覽器工具列時不會觸發，需封住首尾邊界。
            const controls = Array.from(
              event.currentTarget.querySelectorAll<HTMLElement>(
                'button, a[href], input, select, textarea, [tabindex]',
              ),
            ).filter(
              (element) =>
                element.tabIndex >= 0 &&
                !element.matches(':disabled') &&
                element.getClientRects().length > 0 &&
                getComputedStyle(element).visibility !== 'hidden',
            )
            const first = controls[0]
            const last = controls.at(-1)
            if (event.shiftKey && document.activeElement === first) {
              event.preventDefault()
              last?.focus()
            } else if (!event.shiftKey && document.activeElement === last) {
              event.preventDefault()
              first?.focus()
            }
          }}
        >
          {content}
        </div>
      )}
      onCancel={() => {
        setTab('summary')
        onClose()
      }}
      afterClose={() => {
        setTab('summary')
        onAfterClose()
      }}
      destroyOnHidden
    >
      {resource && (
        <Tabs
          className={dialogStyles.tabs}
          activeKey={tab}
          onChange={setTab}
          items={[
            {
              key: 'summary',
              label: t('runtimeTopology.tabs.summary'),
              children: <RuntimeSummary resource={resource} />,
            },
            {
              key: 'events',
              label: t('runtimeTopology.tabs.events'),
              children: events.isPending ? (
                <Spin />
              ) : events.data?.status === 200 ? (
                <Table
                  size="small"
                  pagination={false}
                  rowKey={(value) =>
                    `${value.reason}:${value.lastObservedAt ?? value.firstObservedAt ?? ''}`
                  }
                  dataSource={events.data.data.data}
                  columns={[
                    {
                      title: t('runtimeTopology.fields.type'),
                      dataIndex: 'type',
                    },
                    {
                      title: t('runtimeTopology.fields.reason'),
                      dataIndex: 'reason',
                    },
                    {
                      title: t('runtimeTopology.fields.message'),
                      dataIndex: 'message',
                    },
                    {
                      title: t('runtimeTopology.fields.count'),
                      dataIndex: 'count',
                    },
                  ]}
                />
              ) : (
                <Alert
                  type="error"
                  showIcon
                  title={t('runtimeTopology.unavailable')}
                />
              ),
            },
            {
              key: 'logs',
              label:
                resource.kind === 'Pod' ? (
                  t('runtimeTopology.tabs.logs')
                ) : (
                  <Tooltip title={t('runtimeTopology.logsPodOnly')}>
                    <span>{t('runtimeTopology.tabs.logs')}</span>
                  </Tooltip>
                ),
              disabled: resource.kind !== 'Pod',
              children: logs.isPending ? (
                <Spin />
              ) : logs.data?.status === 200 ? (
                <pre className={styles.code}>
                  {logs.data.data.data
                    .map((entry) => `${entry.timestamp} ${entry.content}`)
                    .join('\n')}
                </pre>
              ) : (
                <Alert
                  type="error"
                  showIcon
                  title={t('runtimeTopology.unavailable')}
                />
              ),
            },
            {
              key: 'manifest',
              label: t('runtimeTopology.tabs.manifest'),
              children: detail.isPending ? (
                <Spin />
              ) : detail.data?.status === 200 ? (
                <pre className={styles.code}>
                  {detail.data.data.data.manifest}
                </pre>
              ) : (
                <Alert
                  type="error"
                  showIcon
                  title={t('runtimeTopology.unavailable')}
                />
              ),
            },
          ]}
        />
      )}
    </Modal>
  )
}

function RuntimeSummary({ resource }: { resource: RuntimeResourceNode }) {
  const { t } = useTranslation()
  return (
    <Descriptions column={1} bordered size="small">
      <Descriptions.Item label={t('runtimeTopology.fields.kind')}>
        {resource.kind}
      </Descriptions.Item>
      <Descriptions.Item label={t('runtimeTopology.fields.namespace')}>
        {resource.namespace || '—'}
      </Descriptions.Item>
      <Descriptions.Item label={t('runtimeTopology.fields.health')}>
        {resource.healthStatus || '—'}
      </Descriptions.Item>
      <Descriptions.Item label={t('runtimeTopology.fields.createdAt')}>
        {resource.createdAt ?? '—'}
      </Descriptions.Item>
      <Descriptions.Item label={t('runtimeTopology.fields.images')}>
        {resource.images.length ? resource.images.join(', ') : '—'}
      </Descriptions.Item>
    </Descriptions>
  )
}

function resourceParams(
  resource?: RuntimeResourceNode,
): GetCatalogApplicationRuntimeResourceParams {
  return {
    group: resource?.group,
    version: resource?.version ?? 'v1',
    kind: resource?.kind ?? 'Pod',
    namespace: resource?.namespace,
    name: resource?.name ?? '_',
  }
}
