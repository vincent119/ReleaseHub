import { Handle, Position, type NodeProps } from '@xyflow/react'
import {
  ApiOutlined,
  AppstoreOutlined,
  CloudServerOutlined,
  DatabaseOutlined,
  DeploymentUnitOutlined,
  SafetyCertificateOutlined,
} from '@ant-design/icons'
import { Button, Tooltip, Typography } from 'antd'
import type { ReactNode } from 'react'

import {
  runtimeDisplayName,
  type RuntimeResourceFlowNode,
} from '../model/runtimeGraph'
import styles from './RuntimeTopologyPanel.module.css'

export function RuntimeNode({ data }: NodeProps<RuntimeResourceFlowNode>) {
  return (
    <Tooltip title={data.resource.name} trigger={['hover', 'focus']}>
      <Button
        aria-label={`${data.resource.kind} ${data.resource.name}`}
        data-runtime-node-id={data.resource.id}
        data-health={data.resource.healthStatus || 'Unknown'}
        className={`${styles.node} ${data.active ? styles.nodeActive : ''}`}
        onClick={() =>
          window.dispatchEvent(
            new CustomEvent('releasehub:runtime-node', {
              detail: data.resource.id,
            }),
          )
        }
      >
        <Handle type="target" position={Position.Left} />
        <span className={styles.nodeIcon} aria-hidden="true">
          {resourceIcon(data.resource.kind)}
        </span>
        <span className={styles.nodeBody}>
          <span className={styles.nodeName}>
            {runtimeDisplayName(data.resource.name)}
          </span>
          <span className={styles.nodeMeta}>
            <span className={styles.kindChip}>{data.resource.kind}</span>
            <Typography.Text type="secondary" ellipsis>
              {data.resource.healthStatus || '—'}
            </Typography.Text>
          </span>
        </span>
        <Handle type="source" position={Position.Right} />
      </Button>
    </Tooltip>
  )
}

function resourceIcon(kind: string): ReactNode {
  if (kind === 'Pod') return <CloudServerOutlined />
  if (kind === 'Service' || kind === 'Ingress') return <ApiOutlined />
  if (kind === 'Deployment' || kind === 'ReplicaSet')
    return <DeploymentUnitOutlined />
  if (kind === 'Secret' || kind === 'ServiceAccount')
    return <SafetyCertificateOutlined />
  if (kind.includes('Volume') || kind.includes('Database'))
    return <DatabaseOutlined />
  return <AppstoreOutlined />
}
