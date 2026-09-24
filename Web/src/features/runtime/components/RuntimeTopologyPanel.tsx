import {
  Background,
  Controls,
  MiniMap,
  ReactFlow,
  type Viewport,
} from '@xyflow/react'
import '@xyflow/react/dist/style.css'
import {
  Alert,
  Button,
  Empty,
  Flex,
  Segmented,
  Space,
  Spin,
  Typography,
} from 'antd'
import { useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { useGetCatalogApplicationRuntimeTopology } from '@/generated/api'
import type { RuntimeTopologyView } from '@/generated/model'
import { useThemePreference } from '@/shared/theme/useThemePreference'

import {
  runtimeFitViewOptions,
  runtimeTopologyToGraph,
} from '../model/runtimeGraph'
import { ApplicationRootNode } from './ApplicationRootNode'
import { RuntimeNode } from './RuntimeNode'
import { RuntimeResourceDrawer } from './RuntimeResourceDrawer'
import styles from './RuntimeTopologyPanel.module.css'

const nodeTypes = { runtime: RuntimeNode, application: ApplicationRootNode }

interface Props {
  applicationId: string
  applicationName?: string
  active?: boolean
  visible?: boolean
  currentLiveState?: boolean
}

export function RuntimeTopologyPanel({
  applicationId,
  applicationName,
  active = false,
  visible = true,
  currentLiveState = false,
}: Props) {
  const { t } = useTranslation()
  const { resolvedTheme } = useThemePreference()
  const [view, setView] = useState<RuntimeTopologyView>('resources')
  const [selected, setSelected] = useState<string>()
  const [viewports, setViewports] = useState<Record<string, Viewport>>({})
  const viewportKey = `${applicationId}:${view}`
  const savedViewport = viewports[viewportKey]
  const topology = useGetCatalogApplicationRuntimeTopology(
    applicationId,
    { view },
    {
      query: {
        enabled: Boolean(applicationId) && visible,
        refetchInterval: active && visible ? 5000 : false,
        refetchIntervalInBackground: false,
      },
    },
  )
  const value =
    topology.data?.status === 200 ? topology.data.data.data : undefined
  const graph = useMemo(
    () =>
      value
        ? runtimeTopologyToGraph(value, active, applicationName)
        : { nodes: [], edges: [] },
    [active, applicationName, value],
  )
  const resource = value?.nodes.find((node) => node.id === selected)
  const closeDrawer = () => {
    const selectedID = selected
    setSelected(undefined)
    window.requestAnimationFrame(() => {
      document
        .querySelectorAll<HTMLElement>('[data-runtime-node-id]')
        .forEach((node) => {
          if (node.dataset.runtimeNodeId === selectedID) node.focus()
        })
    })
  }
  useEffect(() => {
    const select = (event: Event) =>
      setSelected((event as CustomEvent<string>).detail)
    window.addEventListener('releasehub:runtime-node', select)
    return () => window.removeEventListener('releasehub:runtime-node', select)
  }, [])

  return (
    <Space orientation="vertical" size="middle" className={styles.panel}>
      <Flex justify="space-between" align="center" gap="middle" wrap>
        <Space>
          <Segmented
            value={view}
            onChange={(next) => setView(next as RuntimeTopologyView)}
            options={[
              {
                value: 'resources',
                label: t('runtimeTopology.views.resources'),
              },
              { value: 'network', label: t('runtimeTopology.views.network') },
            ]}
          />
          {active && (
            <Typography.Text type="secondary">
              {t('runtimeTopology.live')}
            </Typography.Text>
          )}
          {currentLiveState && (
            <Typography.Text type="secondary">
              {t('runtimeTopology.current')}
            </Typography.Text>
          )}
          {value && (
            <Typography.Text type="secondary" aria-live="polite">
              {t('runtimeTopology.observation', {
                count: value.nodes.length,
                time: new Date(value.observedAt).toLocaleString(),
              })}
            </Typography.Text>
          )}
        </Space>
        <Button
          loading={topology.isFetching}
          onClick={() => void topology.refetch()}
        >
          {t('runtimeTopology.refresh')}
        </Button>
      </Flex>
      {topology.isError || (topology.data && topology.data.status !== 200) ? (
        <Alert type="error" showIcon title={t('runtimeTopology.unavailable')} />
      ) : topology.isPending ? (
        <div className={styles.center}>
          <Spin />
        </div>
      ) : (
        <>
          {value?.warnings.map((warning) => (
            <Alert
              key={warning}
              type="warning"
              showIcon
              title={t(`runtimeTopology.warnings.${warning}`)}
            />
          ))}
          {graph.nodes.length === 0 ? (
            <Empty description={t('runtimeTopology.empty')} />
          ) : (
            <>
              <div
                className={styles.legend}
                aria-label={t('runtimeTopology.legend.title')}
              >
                {view === 'resources' && (
                  <span>
                    <span
                      className={styles.presentationLine}
                      aria-hidden="true"
                    />
                    {t('runtimeTopology.legend.presentation')}
                  </span>
                )}
                <span>
                  <span className={styles.evidenceLine} aria-hidden="true" />
                  {t('runtimeTopology.legend.evidence')}
                </span>
              </div>
              <div
                className={styles.canvas}
                aria-label={t('runtimeTopology.title')}
              >
                <ReactFlow
                  key={viewportKey}
                  nodes={graph.nodes}
                  edges={graph.edges}
                  nodeTypes={nodeTypes}
                  colorMode={resolvedTheme}
                  fitView={!savedViewport}
                  fitViewOptions={runtimeFitViewOptions}
                  defaultViewport={savedViewport}
                  onMoveEnd={(_, viewport) =>
                    setViewports((current) => ({
                      ...current,
                      [viewportKey]: viewport,
                    }))
                  }
                  minZoom={runtimeFitViewOptions.minZoom}
                  maxZoom={1.6}
                  nodesDraggable={false}
                  nodesConnectable={false}
                  elementsSelectable
                >
                  <Background />
                  <MiniMap pannable zoomable />
                  <Controls showInteractive={false} />
                </ReactFlow>
              </div>
            </>
          )}
        </>
      )}
      <RuntimeResourceDrawer
        applicationId={applicationId}
        resource={resource}
        onClose={closeDrawer}
      />
    </Space>
  )
}
