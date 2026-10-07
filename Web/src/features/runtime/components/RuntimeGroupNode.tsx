import { AppstoreOutlined } from '@ant-design/icons'
import { Handle, Position, type Node, type NodeProps } from '@xyflow/react'
import { Button, Tooltip, Typography } from 'antd'
import { useTranslation } from 'react-i18next'

import type { RuntimeGroupNodeData } from '../model/runtimeGraph'
import styles from './RuntimeTopologyPanel.module.css'

type GroupNode = Node<RuntimeGroupNodeData, 'runtime-group'>

export function RuntimeGroupNode({ data }: NodeProps<GroupNode>) {
  const { t } = useTranslation()
  const title = data.compactOverview
    ? t('runtimeTopology.group.compact', { count: data.memberIds.length })
    : data.kind
      ? t('runtimeTopology.group.kind', {
          kind: data.kind,
          count: data.rootCount,
        })
      : t('runtimeTopology.group.other')
  const kinds = Object.entries(data.kindCounts)
    .map(([kind, count]) => `${kind} ${count}`)
    .join(', ')
  const health = Object.entries(data.healthCounts)
    .sort(([left], [right]) =>
      left === 'Unknown'
        ? -1
        : right === 'Unknown'
          ? 1
          : left.localeCompare(right),
    )
    .map(([status, count]) => `${status} ${count}`)
    .join(', ')
  const visibleTitle = data.compactOverview
    ? t('runtimeTopology.group.shortCompact')
    : (data.kind ?? t('runtimeTopology.group.shortOther'))
  const resourceCount = t('runtimeTopology.group.resources', {
    count: data.memberIds.length,
  })

  return (
    <Tooltip
      title={`${title} · ${resourceCount} · ${kinds} · ${health}`}
      trigger={['hover', 'focus']}
    >
      <Button
        className={`${styles.node} ${styles.groupNode}`}
        data-runtime-group-id={data.groupId}
        aria-label={
          t('runtimeTopology.group.expand', {
            title,
            count: data.memberIds.length,
          }) + ` · ${kinds} · ${health}`
        }
        onClick={() =>
          window.dispatchEvent(
            new CustomEvent('releasehub:runtime-group', {
              detail: data.groupId,
            }),
          )
        }
      >
        <Handle type="target" position={Position.Left} />
        <span className={styles.nodeIcon} aria-hidden="true">
          <AppstoreOutlined />
        </span>
        <span className={styles.nodeBody}>
          <span className={styles.groupNameRow}>
            <span className={styles.nodeName}>{visibleTitle}</span>
            <span className={styles.groupCount} aria-hidden="true">
              × {data.memberIds.length}
            </span>
          </span>
          <span className={styles.nodeMeta}>
            <Typography.Text type="secondary" className={styles.groupHealth}>
              {health}
            </Typography.Text>
          </span>
        </span>
        <Handle type="source" position={Position.Right} />
      </Button>
    </Tooltip>
  )
}
