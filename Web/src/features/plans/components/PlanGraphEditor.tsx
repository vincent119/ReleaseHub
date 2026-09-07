import { PlusOutlined } from '@ant-design/icons'
import { Background, Controls, MiniMap, ReactFlow } from '@xyflow/react'
import '@xyflow/react/dist/style.css'
import { Alert, Button, Empty, Flex, InputNumber, Typography } from 'antd'
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'

import type { DeploymentPlanDocument } from '@/generated/model'

import { usePlanEditor } from '../hooks/usePlanEditor'
import { hasCycle } from '../model/planGraph'
import { PlanEdgeInspector } from './PlanEdgeInspector'
import { DependencyModal } from './DependencyModal'
import { PlanNodeInspector } from './PlanNodeInspector'
import styles from './PlanGraphEditor.module.css'

interface Props {
  initialDocument: DeploymentPlanDocument
  onChange?: (document: DeploymentPlanDocument) => void
  readOnly?: boolean
}

export function PlanGraphEditor({
  initialDocument,
  onChange,
  readOnly = false,
}: Props) {
  const { t } = useTranslation()
  const { graph, document, dispatch } = usePlanEditor(initialDocument)
  const [selectedNode, setSelectedNode] = useState<string>()
  const [selectedEdge, setSelectedEdge] = useState<string>()
  const [addingDependency, setAddingDependency] = useState(false)
  useEffect(() => onChange?.(document), [document, onChange])
  const emit = (action: Parameters<typeof dispatch>[0]) => dispatch(action)
  const node = graph.nodes.find((item) => item.id === selectedNode)
  const edge = graph.edges.find((item) => item.id === selectedEdge)
  return (
    <div className={styles.editor}>
      <Flex className={styles.toolbar} gap="small" align="center" wrap>
        <Button
          icon={<PlusOutlined />}
          disabled={readOnly}
          onClick={() => emit({ type: 'addNode' })}
        >
          {t('plans.actions.addNode')}
        </Button>
        <Button
          disabled={readOnly || graph.nodes.length < 2}
          onClick={() => setAddingDependency(true)}
        >
          {t('plans.actions.addDependency')}
        </Button>
        <Typography.Text>{t('plans.fields.maxParallel')}</Typography.Text>
        <InputNumber
          min={1}
          disabled={readOnly}
          value={graph.maxParallel}
          placeholder={t('plans.fields.systemDefault')}
          onChange={(value) =>
            emit({ type: 'maxParallel', value: value ?? undefined })
          }
        />
        {hasCycle(document) && (
          <Alert type="error" showIcon message={t('plans.validation.cycle')} />
        )}
      </Flex>
      <div className={styles.canvas} aria-label={t('plans.editor.canvas')}>
        <ReactFlow
          nodes={graph.nodes}
          edges={graph.edges}
          fitView
          onNodesChange={(changes) => emit({ type: 'nodes', changes })}
          onEdgesChange={(changes) => emit({ type: 'edges', changes })}
          onConnect={(connection) => emit({ type: 'connect', connection })}
          onNodeDragStop={() => emit({ type: 'reorder' })}
          nodesDraggable={!readOnly}
          nodesConnectable={!readOnly}
          edgesReconnectable={!readOnly}
          onNodeClick={(_, value) => {
            setSelectedNode(value.id)
            setSelectedEdge(undefined)
          }}
          onEdgeClick={(_, value) => {
            setSelectedEdge(value.id)
            setSelectedNode(undefined)
          }}
        >
          <Background />
          <MiniMap pannable zoomable />
          <Controls />
        </ReactFlow>
      </div>
      <aside className={styles.inspector}>
        {node && !readOnly && (
          <PlanNodeInspector
            node={node.data.node}
            onChange={(value) => {
              emit({ type: 'updateNode', id: node.id, node: value })
              setSelectedNode(value.key)
            }}
            onRemove={() => {
              emit({ type: 'removeNode', id: node.id })
              setSelectedNode(undefined)
            }}
          />
        )}
        {edge && !readOnly && (
          <PlanEdgeInspector
            edge={edge.data.edge}
            nodeKeys={graph.nodes.map((item) => item.id)}
            onChange={(value) => {
              emit({ type: 'updateEdge', id: edge.id, edge: value })
              setSelectedEdge(`${value.from}::${value.to}`)
            }}
            onRemove={() => {
              emit({ type: 'removeEdge', id: edge.id })
              setSelectedEdge(undefined)
            }}
          />
        )}
        {(!node || readOnly) && (!edge || readOnly) && (
          <Empty
            image={Empty.PRESENTED_IMAGE_SIMPLE}
            description={t('plans.editor.selectElement')}
          />
        )}
      </aside>
      <DependencyModal
        open={addingDependency}
        nodeKeys={graph.nodes.map((item) => item.id)}
        onClose={() => setAddingDependency(false)}
        onSubmit={(connection) => emit({ type: 'connect', connection })}
      />
    </div>
  )
}
