import { PlusOutlined } from '@ant-design/icons'
import { Alert, Button, Empty, InputNumber, Typography } from 'antd'
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'

import type { DeploymentPlanDocument } from '@/generated/model'
import { DefinitionGraphCanvas } from '@/shared/graph/DefinitionGraphCanvas'
import { GraphEditorFrame } from '@/shared/graph/GraphEditorFrame'
import frameStyles from '@/shared/graph/GraphEditorFrame.module.css'
import { useGraphSelection } from '@/shared/graph/useGraphSelection'

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
  const selection = useGraphSelection()
  const [addingDependency, setAddingDependency] = useState(false)
  useEffect(() => onChange?.(document), [document, onChange])
  const emit = (action: Parameters<typeof dispatch>[0]) => dispatch(action)
  const node = graph.nodes.find((item) => item.id === selection.selectedNodeID)
  const edge = graph.edges.find((item) => item.id === selection.selectedEdgeID)
  return (
    <GraphEditorFrame
      className={styles.editor}
      toolbarClassName={styles.toolbar}
      canvasClassName={styles.canvas}
      inspectorClassName={styles.inspector}
      toolbarLabel={t('plans.editor.toolbar')}
      canvasLabel={t('plans.editor.canvas')}
      inspectorLabel={t('plans.editor.inspectorTitle')}
      selectionTitle={
        node
          ? node.data.node.applicationKey
          : edge
            ? `${edge.data.edge.from} → ${edge.data.edge.to}`
            : t('plans.editor.nothingSelected')
      }
      toolbar={
        <>
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
        </>
      }
      canvas={
        <DefinitionGraphCanvas
          nodes={graph.nodes}
          edges={graph.edges}
          onNodesChange={(changes) => emit({ type: 'nodes', changes })}
          onEdgesChange={(changes) => emit({ type: 'edges', changes })}
          onConnect={(connection) => emit({ type: 'connect', connection })}
          onNodeDragStop={() => emit({ type: 'reorder' })}
          readOnly={readOnly}
          miniMapClassName={frameStyles.miniMap}
          onSelectNode={selection.selectNode}
          onSelectEdge={selection.selectEdge}
          onClearSelection={selection.clearSelection}
        />
      }
      inspector={
        <>
          {node && !readOnly && (
            <PlanNodeInspector
              node={node.data.node}
              onChange={(value) => {
                emit({ type: 'updateNode', id: node.id, node: value })
                selection.selectNode(value.key)
              }}
              onRemove={() => {
                emit({ type: 'removeNode', id: node.id })
                selection.clearSelection()
              }}
            />
          )}
          {edge && !readOnly && (
            <PlanEdgeInspector
              edge={edge.data.edge}
              nodeKeys={graph.nodes.map((item) => item.id)}
              onChange={(value) => {
                emit({ type: 'updateEdge', id: edge.id, edge: value })
                selection.selectEdge(`${value.from}::${value.to}`)
              }}
              onRemove={() => {
                emit({ type: 'removeEdge', id: edge.id })
                selection.clearSelection()
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
        </>
      }
    >
      <DependencyModal
        open={addingDependency}
        nodeKeys={graph.nodes.map((item) => item.id)}
        onClose={() => setAddingDependency(false)}
        onSubmit={(connection) => emit({ type: 'connect', connection })}
      />
    </GraphEditorFrame>
  )
}
