import { Handle, Position, type NodeProps } from '@xyflow/react'

import type { WorkflowNode } from '../model/workflowGraph'
import styles from './WorkflowStateNode.module.css'

export function WorkflowStateNode({ data, selected }: NodeProps<WorkflowNode>) {
  return (
    <div
      className={`${styles.node} ${selected ? styles.selected : ''}`}
      data-state-type={data.state.type}
    >
      <Handle type="target" position={Position.Left} />
      <span className={styles.type}>{data.state.type}</span>
      <strong className={styles.name}>{data.state.name}</strong>
      <span className={styles.key}>{data.state.key}</span>
      <Handle type="source" position={Position.Right} />
    </div>
  )
}
