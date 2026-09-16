import { DeleteOutlined, PlusOutlined } from '@ant-design/icons'
import { Button, Empty, Select, Space, Typography } from 'antd'
import { useEffect } from 'react'
import { useTranslation } from 'react-i18next'

import type {
  ReleaseWorkflowDocument,
  ReleaseWorkflowReviewOptions,
} from '@/generated/model'
import { DefinitionGraphCanvas } from '@/shared/graph/DefinitionGraphCanvas'
import { GraphEditorFrame } from '@/shared/graph/GraphEditorFrame'
import frameStyles from '@/shared/graph/GraphEditorFrame.module.css'
import { useGraphSelection } from '@/shared/graph/useGraphSelection'

import { useWorkflowEditor } from '../hooks/useWorkflowEditor'
import { StateInspector } from './StateInspector'
import { TransitionInspector } from './TransitionInspector'
import styles from './WorkflowGraphEditor.module.css'
import { WorkflowStateNode } from './WorkflowStateNode'

const nodeTypes = { workflowState: WorkflowStateNode }

interface Props {
  initialDocument: ReleaseWorkflowDocument
  reviewOptions?: ReleaseWorkflowReviewOptions
  reviewOptionsLoading?: boolean
  reviewOptionsError?: boolean
  onChange?: (document: ReleaseWorkflowDocument) => void
  readOnly?: boolean
}

export function WorkflowGraphEditor({
  initialDocument,
  reviewOptions,
  reviewOptionsLoading = false,
  reviewOptionsError = false,
  onChange,
  readOnly = false,
}: Props) {
  const { t } = useTranslation()
  const { graph, document, dispatch } = useWorkflowEditor(initialDocument)
  const selection = useGraphSelection()
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
      toolbarLabel={t('workflows.editor.toolbar')}
      canvasLabel={t('workflows.editor.canvas')}
      inspectorLabel={t('workflows.editor.inspectorTitle')}
      selectionTitle={
        node
          ? node.data.state.name
          : edge
            ? edge.data.transition.key
            : t('workflows.editor.nothingSelected')
      }
      toolbar={
        <>
          <div className={frameStyles.toolbarGroup}>
            <Typography.Text className={frameStyles.toolbarLabel}>
              {t('workflows.editor.structureGroup')}
            </Typography.Text>
            <Button
              icon={<PlusOutlined />}
              disabled={readOnly}
              onClick={() => emit({ type: 'addState' })}
            >
              {t('workflows.actions.addState')}
            </Button>
          </div>
          <div className={frameStyles.toolbarDivider} aria-hidden="true" />
          <div className={frameStyles.toolbarGroup}>
            <Typography.Text className={frameStyles.toolbarLabel}>
              {t('workflows.editor.startingPointGroup')}
            </Typography.Text>
            <Select
              className={styles.initialSelect}
              aria-label={t('workflows.fields.initialState')}
              value={graph.initialState}
              disabled={readOnly}
              options={graph.nodes.map((item) => ({
                value: item.id,
                label: item.data.state.name,
              }))}
              onChange={(id) => emit({ type: 'initial', id })}
            />
          </div>
        </>
      }
      canvas={
        <DefinitionGraphCanvas
          nodes={graph.nodes}
          edges={graph.edges}
          nodeTypes={nodeTypes}
          onNodesChange={(changes) => emit({ type: 'nodes', changes })}
          onEdgesChange={(changes) => emit({ type: 'edges', changes })}
          onConnect={(connection) => emit({ type: 'connect', connection })}
          readOnly={readOnly}
          miniMapClassName={frameStyles.miniMap}
          backgroundProps={{ gap: 20, size: 1 }}
          onSelectNode={selection.selectNode}
          onSelectEdge={selection.selectEdge}
          onClearSelection={selection.clearSelection}
        />
      }
      inspector={
        <>
          {node && !readOnly && (
            <StateInspector
              state={node.data.state}
              initial={graph.initialState === node.id}
              reviewOptions={reviewOptions}
              reviewOptionsLoading={reviewOptionsLoading}
              reviewOptionsError={reviewOptionsError}
              onMakeInitial={() => emit({ type: 'initial', id: node.id })}
              onChange={(state) => {
                emit({ type: 'updateState', id: node.id, state })
                selection.selectNode(state.key)
              }}
              onRemove={() => {
                emit({ type: 'removeState', id: node.id })
                selection.clearSelection()
              }}
            />
          )}
          {edge && !readOnly && (
            <TransitionInspector
              transition={edge.data.transition}
              states={graph.nodes.map((item) => item.id)}
              onChange={(transition) => {
                emit({ type: 'updateTransition', id: edge.id, transition })
                selection.selectEdge(transition.key)
              }}
              onRemove={() => {
                emit({ type: 'removeTransition', id: edge.id })
                selection.clearSelection()
              }}
            />
          )}
          {(!node || readOnly) && (!edge || readOnly) && (
            <div className={frameStyles.inspectorEmpty}>
              <Empty
                image={Empty.PRESENTED_IMAGE_SIMPLE}
                description={t('workflows.editor.selectElement')}
              />
            </div>
          )}
          {!readOnly && (node || edge) && (
            <Space className={styles.draftHint} align="start">
              <DeleteOutlined />
              <Typography.Text type="secondary">
                {t('workflows.editor.deleteHint')}
              </Typography.Text>
            </Space>
          )}
        </>
      }
    />
  )
}
