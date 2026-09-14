import { DeleteOutlined, PlusOutlined } from '@ant-design/icons'
import { Background, Controls, MiniMap, ReactFlow } from '@xyflow/react'
import '@xyflow/react/dist/style.css'
import { Button, Empty, Flex, Select, Space, Typography } from 'antd'
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'

import type {
  ReleaseWorkflowDocument,
  ReleaseWorkflowReviewOptions,
} from '@/generated/model'

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
  const [selectedNode, setSelectedNode] = useState<string>()
  const [selectedEdge, setSelectedEdge] = useState<string>()
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
          onClick={() => emit({ type: 'addState' })}
        >
          {t('workflows.actions.addState')}
        </Button>
        <Typography.Text>{t('workflows.fields.initialState')}</Typography.Text>
        <Select
          value={graph.initialState}
          options={graph.nodes.map((item) => ({
            value: item.id,
            label: item.data.state.name,
          }))}
          onChange={(id) => emit({ type: 'initial', id })}
        />
      </Flex>
      <div className={styles.canvas} aria-label={t('workflows.editor.canvas')}>
        <ReactFlow
          nodes={graph.nodes}
          edges={graph.edges}
          nodeTypes={nodeTypes}
          fitView
          fitViewOptions={{ padding: 0.2 }}
          minZoom={0.35}
          maxZoom={1.5}
          onNodesChange={(changes) => emit({ type: 'nodes', changes })}
          onEdgesChange={(changes) => emit({ type: 'edges', changes })}
          onConnect={(connection) => emit({ type: 'connect', connection })}
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
          <Background gap={20} size={1} />
          <MiniMap pannable zoomable />
          <Controls />
        </ReactFlow>
      </div>
      <aside className={styles.inspector}>
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
              setSelectedNode(state.key)
            }}
            onRemove={() => {
              emit({ type: 'removeState', id: node.id })
              setSelectedNode(undefined)
            }}
          />
        )}
        {edge && !readOnly && (
          <TransitionInspector
            transition={edge.data.transition}
            states={graph.nodes.map((item) => item.id)}
            onChange={(transition) => {
              emit({ type: 'updateTransition', id: edge.id, transition })
              setSelectedEdge(transition.key)
            }}
            onRemove={() => {
              emit({ type: 'removeTransition', id: edge.id })
              setSelectedEdge(undefined)
            }}
          />
        )}
        {(!node || readOnly) && (!edge || readOnly) && (
          <Empty
            image={Empty.PRESENTED_IMAGE_SIMPLE}
            description={t('workflows.editor.selectElement')}
          />
        )}
        {!readOnly && (node || edge) && (
          <Space>
            <DeleteOutlined />
            <Typography.Text type="secondary">
              {t('workflows.editor.deleteHint')}
            </Typography.Text>
          </Space>
        )}
      </aside>
    </div>
  )
}
