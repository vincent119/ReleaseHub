import { Handle, Position, type NodeProps } from '@xyflow/react'
import {
  ApiOutlined,
  AppstoreOutlined,
  CloudServerOutlined,
  DatabaseOutlined,
  DeploymentUnitOutlined,
  SafetyCertificateOutlined,
} from '@ant-design/icons'
import { Button, Typography } from 'antd'
import type { ReactNode } from 'react'

import type { RuntimeResourceFlowNode } from '../model/runtimeGraph'
import styles from './RuntimeTopologyPanel.module.css'

export function RuntimeNode({ data }: NodeProps<RuntimeResourceFlowNode>) {
  return (
    <Button
      data-runtime-node-id={data.resource.id}
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
        <Typography.Text strong ellipsis>
          {data.resource.name}
        </Typography.Text>
        <span className={styles.nodeMeta}>
          <span className={styles.kindChip}>{data.resource.kind}</span>
          <Typography.Text type="secondary" ellipsis>
            {data.resource.healthStatus || '—'}
          </Typography.Text>
        </span>
      </span>
      <Handle type="source" position={Position.Right} />
    </Button>
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
