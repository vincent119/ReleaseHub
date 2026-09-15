import { PlusOutlined } from '@ant-design/icons'
import { Background, Controls, MiniMap, ReactFlow } from '@xyflow/react'
import '@xyflow/react/dist/style.css'
import { Alert, Button, Empty, Flex, InputNumber, Typography } from 'antd'
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'

import type { DeploymentPlanDocument } from '@/generated/model'
import frameStyles from '@/shared/graph/GraphEditorFrame.module.css'
import {
  definitionFitViewOptions,
  definitionZoomRange,
} from '@/shared/graph/graphViewport'

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
    <div className={`${frameStyles.editor} ${styles.editor}`}>
      <Flex
        className={`${frameStyles.toolbar} ${styles.toolbar}`}
        role="toolbar"
        aria-label={t('plans.editor.toolbar')}
      >
        <div className={frameStyles.toolbarGroup}>
          <Typography.Text className={frameStyles.toolbarLabel}>
            {t('plans.editor.structureGroup')}
          </Typography.Text>
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
        </div>
        <div className={frameStyles.toolbarDivider} aria-hidden="true" />
        <div className={frameStyles.toolbarGroup}>
          <Typography.Text className={frameStyles.toolbarLabel}>
            {t('plans.editor.executionGroup')}
          </Typography.Text>
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
        </div>
        {hasCycle(document) && (
          <Alert type="error" showIcon title={t('plans.validation.cycle')} />
        )}
      </Flex>
      <div
        className={`${frameStyles.canvas} ${styles.canvas}`}
        aria-label={t('plans.editor.canvas')}
      >
        <ReactFlow
          nodes={graph.nodes}
          edges={graph.edges}
          fitView
          fitViewOptions={definitionFitViewOptions}
          minZoom={definitionZoomRange.min}
          maxZoom={definitionZoomRange.max}
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
          onPaneClick={() => {
            setSelectedNode(undefined)
            setSelectedEdge(undefined)
          }}
        >
          <Background />
          <MiniMap className={frameStyles.miniMap} pannable zoomable />
          <Controls />
        </ReactFlow>
      </div>
      <aside
        className={`${frameStyles.inspector} ${styles.inspector}`}
        aria-label={t('plans.editor.inspectorTitle')}
      >
        <div className={frameStyles.inspectorHeader}>
          <Typography.Text className={frameStyles.inspectorEyebrow}>
            {t('plans.editor.inspectorTitle')}
          </Typography.Text>
          <Typography.Title level={4} className={frameStyles.inspectorTitle}>
            {node
              ? node.data.node.applicationKey
              : edge
                ? `${edge.data.edge.from} → ${edge.data.edge.to}`
                : t('plans.editor.nothingSelected')}
          </Typography.Title>
        </div>
        <div className={frameStyles.inspectorBody}>
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
            <div className={frameStyles.inspectorEmpty}>
              <Empty
                image={Empty.PRESENTED_IMAGE_SIMPLE}
                description={t('plans.editor.selectElement')}
              />
            </div>
          )}
        </div>
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
