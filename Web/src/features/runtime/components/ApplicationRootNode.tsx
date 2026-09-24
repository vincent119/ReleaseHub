import { AppstoreOutlined } from '@ant-design/icons'
import { Handle, Position, type NodeProps } from '@xyflow/react'
import { useTranslation } from 'react-i18next'

import type { RuntimeFlowNode } from '../model/runtimeGraph'
import styles from './RuntimeTopologyPanel.module.css'

type ApplicationFlowNode = Extract<RuntimeFlowNode, { type: 'application' }>

export function ApplicationRootNode({ data }: NodeProps<ApplicationFlowNode>) {
  const { t } = useTranslation()

  return (
    <div
      className={styles.applicationNode}
      role="group"
      aria-label={`${t('runtimeTopology.applicationRoot')}: ${data.name}`}
    >
      <span className={styles.nodeIcon} aria-hidden="true">
        <AppstoreOutlined />
      </span>
      <span className={styles.nodeBody}>
        <span className={styles.applicationNodeType}>
          {t('runtimeTopology.applicationRoot')}
        </span>
        <span className={styles.applicationNodeName} title={data.name}>
          {data.name}
        </span>
      </span>
      <Handle type="source" position={Position.Right} />
    </div>
  )
}
