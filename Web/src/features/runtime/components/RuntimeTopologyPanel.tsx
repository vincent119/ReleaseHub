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
import { useEffect, useMemo, useRef, useState } from 'react'
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
import { RuntimeResourceDialog } from './RuntimeResourceDialog'
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
  const panelRef = useRef<HTMLDivElement>(null)
  const selectedNodeRef = useRef<string | undefined>(undefined)
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
  const value = useMemo(() => {
    if (topology.data?.status !== 200) return undefined
    const response = topology.data.data.data
    // 發布切換期間舊版回應可能仍含 null；呈現層不得因此中斷 Request 頁面。
    return {
      ...response,
      nodes: Array.isArray(response.nodes)
        ? response.nodes.map((node) => ({
            ...node,
            images: Array.isArray(node.images) ? node.images : [],
            info: Array.isArray(node.info) ? node.info : [],
            ingress: Array.isArray(node.ingress) ? node.ingress : [],
            externalUrls: Array.isArray(node.externalUrls)
              ? node.externalUrls
              : [],
          }))
        : [],
      edges: Array.isArray(response.edges) ? response.edges : [],
      warnings: Array.isArray(response.warnings) ? response.warnings : [],
    }
  }, [topology.data])
  const graph = useMemo(
    () =>
      value
        ? runtimeTopologyToGraph(value, active, applicationName)
        : { nodes: [], edges: [] },
    [active, applicationName, value],
  )
  const resource = value?.nodes.find((node) => node.id === selected)
  const restoreNodeFocus = () => {
    setSelected(undefined)
    window.requestAnimationFrame(() => {
      const panel = panelRef.current
      const node = Array.from(
        panel?.querySelectorAll<HTMLElement>('[data-runtime-node-id]') ?? [],
      ).find(
        (element) => element.dataset.runtimeNodeId === selectedNodeRef.current,
      )
      // 等視窗解除焦點限制後再恢復；資源消失時仍有可操作的返回位置。
      const target =
        node ?? panel?.querySelector<HTMLElement>('[data-runtime-refresh]')
      target?.focus({ preventScroll: true })
    })
  }
  useEffect(() => {
    const select = (event: Event) => {
      const id = (event as CustomEvent<string>).detail
      selectedNodeRef.current = id
      setSelected(id)
    }
    window.addEventListener('releasehub:runtime-node', select)
    return () => window.removeEventListener('releasehub:runtime-node', select)
  }, [])

  return (
    <Space
      ref={panelRef}
      orientation="vertical"
      size="middle"
      className={styles.panel}
    >
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
          data-runtime-refresh
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
                  defaultMarkerColor="var(--rh-color-border-strong)"
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
      <RuntimeResourceDialog
        applicationId={applicationId}
        resource={resource}
        onClose={() => setSelected(undefined)}
        onAfterClose={restoreNodeFocus}
      />
    </Space>
  )
}
