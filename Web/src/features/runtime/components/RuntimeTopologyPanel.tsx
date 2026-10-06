import {
  Background,
  Controls,
  MiniMap,
  ReactFlow,
  type ReactFlowInstance,
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
  runtimeDisplayTopology,
  runtimeLayeredTopologyToGraph,
  runtimeTopologyToGraph,
  type RuntimeLayeredGraph,
  type RuntimeFlowEdge,
  type RuntimeFlowNode,
} from '../model/runtimeGraph'
import { ApplicationRootNode } from './ApplicationRootNode'
import { RuntimeNode } from './RuntimeNode'
import { RuntimeGroupNode } from './RuntimeGroupNode'
import { RuntimeResourceDialog } from './RuntimeResourceDialog'
import styles from './RuntimeTopologyPanel.module.css'

const nodeTypes = {
  runtime: RuntimeNode,
  application: ApplicationRootNode,
  'runtime-group': RuntimeGroupNode,
}

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
  const [resourceModes, setResourceModes] = useState<
    Record<string, 'layered' | 'all'>
  >({})
  const [expandedGroups, setExpandedGroups] = useState<
    Record<string, string[]>
  >({})
  const resourceMode = resourceModes[applicationId] ?? 'layered'
  const expandedGroupIds = useMemo(
    () => expandedGroups[applicationId] ?? [],
    [applicationId, expandedGroups],
  )
  const [selected, setSelected] = useState<string>()
  const panelRef = useRef<HTMLDivElement>(null)
  const canvasRef = useRef<HTMLDivElement>(null)
  const [canvasWidth, setCanvasWidth] = useState<number | undefined>(() =>
    window.innerWidth < 600 ? window.innerWidth : undefined,
  )
  const lastFittedWidthRef = useRef<number | undefined>(undefined)
  const compact = canvasWidth !== undefined && canvasWidth < 600
  const selectedNodeRef = useRef<string | undefined>(undefined)
  const groupFocusRef = useRef<
    { applicationId: string; id: string } | undefined
  >(undefined)
  const flowRef = useRef<ReactFlowInstance<
    RuntimeFlowNode,
    RuntimeFlowEdge
  > | null>(null)
  const [viewports, setViewports] = useState<Record<string, Viewport>>({})
  const [readingViewports, setReadingViewports] = useState<
    Record<string, boolean>
  >({})
  const viewportKey = `${applicationId}:${view}:${view === 'resources' ? resourceMode : 'network'}:${compact ? 'compact' : 'standard'}`
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
  const displayTopology = useMemo(
    () => (value ? runtimeDisplayTopology(value) : undefined),
    [value],
  )
  const graph = useMemo<RuntimeLayeredGraph>(() => {
    if (!displayTopology)
      return {
        nodes: [],
        edges: [],
        groups: [],
        visibleResourceCount: 0,
        groupedResourceCount: 0,
        groupingAvailable: false,
      }
    if (view === 'resources' && resourceMode === 'layered')
      return runtimeLayeredTopologyToGraph(
        displayTopology,
        new Set(expandedGroupIds),
        active,
        applicationName,
        compact,
      )
    return {
      ...runtimeTopologyToGraph(displayTopology, active, applicationName),
      groups: [],
      visibleResourceCount: displayTopology.nodes.length,
      groupedResourceCount: 0,
      groupingAvailable: false,
    }
  }, [
    active,
    applicationName,
    compact,
    displayTopology,
    expandedGroupIds,
    resourceMode,
    view,
  ])
  const displayCount = displayTopology?.nodes.length ?? 0
  useEffect(() => {
    const canvas = canvasRef.current
    if (!canvas) return
    if (canvas.clientWidth > 0) setCanvasWidth(canvas.clientWidth)
    if (typeof ResizeObserver === 'undefined') return
    const observer = new ResizeObserver(([entry]) => {
      if (entry.contentRect.width > 0) setCanvasWidth(entry.contentRect.width)
    })
    observer.observe(canvas)
    return () => observer.disconnect()
  }, [graph.nodes.length, topology.isPending, view])
  useEffect(() => {
    if (
      canvasWidth === undefined ||
      canvasWidth === lastFittedWidthRef.current ||
      graph.nodes.length === 0
    )
      return
    lastFittedWidthRef.current = canvasWidth
    if (readingViewports[viewportKey]) return
    let pendingFrame = 0
    const firstFrame = window.requestAnimationFrame(() => {
      pendingFrame = window.requestAnimationFrame(() => {
        void flowRef.current?.fitView(
          view === 'resources' &&
            resourceMode === 'layered' &&
            graph.groupingAvailable
            ? { ...runtimeFitViewOptions, padding: 0.04, minZoom: 1 }
            : runtimeFitViewOptions,
        )
      })
    })
    return () => {
      window.cancelAnimationFrame(firstFrame)
      if (pendingFrame) window.cancelAnimationFrame(pendingFrame)
    }
  }, [
    canvasWidth,
    graph.groupingAvailable,
    graph.nodes.length,
    resourceMode,
    view,
    viewportKey,
    readingViewports,
  ])
  const networkHasUnlinkedEntrance = useMemo(() => {
    if (view !== 'network' || !displayTopology) return false
    const linkedIds = new Set(
      displayTopology.edges.flatMap((edge) => [edge.source, edge.target]),
    )
    return displayTopology.nodes.some((node) => !linkedIds.has(node.id))
  }, [displayTopology, view])
  const resource = displayTopology?.nodes.some((node) => node.id === selected)
    ? value?.nodes.find((node) => node.id === selected)
    : undefined
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
  useEffect(() => {
    const expand = (event: Event) => {
      const groupId = (event as CustomEvent<string>).detail
      const group = graph.groups.find((item) => item.groupId === groupId)
      if (!group || expandedGroupIds.includes(groupId)) return
      groupFocusRef.current = { applicationId, id: group.memberIds[0] }
      setReadingViewports((current) => ({ ...current, [viewportKey]: true }))
      setExpandedGroups((current) => ({
        ...current,
        [applicationId]: [...(current[applicationId] ?? []), groupId],
      }))
    }
    window.addEventListener('releasehub:runtime-group', expand)
    return () => window.removeEventListener('releasehub:runtime-group', expand)
  }, [applicationId, expandedGroupIds, graph.groups, viewportKey])
  useEffect(() => {
    groupFocusRef.current = undefined
  }, [applicationId, view])
  useEffect(() => {
    const pending = groupFocusRef.current
    if (
      !pending ||
      pending.applicationId !== applicationId ||
      !graph.nodes.some((node) => node.id === pending.id)
    )
      return
    groupFocusRef.current = undefined
    window.requestAnimationFrame(() => {
      const node = Array.from(
        panelRef.current?.querySelectorAll<HTMLElement>(
          '[data-runtime-node-id], [data-runtime-group-id]',
        ) ?? [],
      ).find(
        (element) =>
          element.dataset.runtimeNodeId === pending.id ||
          element.dataset.runtimeGroupId === pending.id,
      )
      node?.focus({ preventScroll: true })
      void flowRef.current?.fitView({
        nodes: [{ id: pending.id }],
        minZoom: 1,
        maxZoom: 1,
        duration: 200,
      })
    })
  }, [applicationId, graph.nodes])

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
            onChange={(next) => {
              setSelected(undefined)
              setView(next as RuntimeTopologyView)
            }}
            options={[
              {
                value: 'resources',
                label: t('runtimeTopology.views.resources'),
              },
              { value: 'network', label: t('runtimeTopology.views.network') },
            ]}
          />
          {view === 'resources' && (
            <Segmented
              value={resourceMode}
              onChange={(mode) =>
                setResourceModes((current) => ({
                  ...current,
                  [applicationId]: mode as 'layered' | 'all',
                }))
              }
              options={[
                { value: 'layered', label: t('runtimeTopology.modes.layered') },
                { value: 'all', label: t('runtimeTopology.modes.all') },
              ]}
            />
          )}
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
                visible: displayCount,
                time: new Date(value.observedAt).toLocaleString(),
              })}
            </Typography.Text>
          )}
        </Space>
        <Space wrap>
          {graph.nodes.length > 0 && (
            <>
              <Button
                onClick={() => {
                  setReadingViewports((current) => ({
                    ...current,
                    [viewportKey]: false,
                  }))
                  void flowRef.current?.fitView(runtimeFitViewOptions)
                }}
              >
                {t('runtimeTopology.fitView')}
              </Button>
              <Button
                onClick={() => {
                  setReadingViewports((current) => ({
                    ...current,
                    [viewportKey]: true,
                  }))
                  void flowRef.current?.zoomTo(1)
                }}
              >
                {t('runtimeTopology.readableZoom')}
              </Button>
            </>
          )}
          <Button
            data-runtime-refresh
            loading={topology.isFetching}
            onClick={() => void topology.refetch()}
          >
            {t('runtimeTopology.refresh')}
          </Button>
        </Space>
      </Flex>
      {topology.isError || (topology.data && topology.data.status !== 200) ? (
        <Alert type="error" showIcon title={t('runtimeTopology.unavailable')} />
      ) : topology.isPending ? (
        <div className={styles.center}>
          <Spin />
        </div>
      ) : (
        <>
          {view === 'resources' &&
            resourceMode === 'layered' &&
            graph.groupingAvailable && (
              <Flex gap="middle" wrap align="center">
                <Typography.Text type="secondary" aria-live="polite">
                  {t('runtimeTopology.group.summary', {
                    visible: graph.visibleResourceCount,
                    grouped: graph.groupedResourceCount,
                    groups: graph.groups.filter(
                      (group) => !expandedGroupIds.includes(group.groupId),
                    ).length,
                  })}
                </Typography.Text>
                {graph.groups
                  .filter((group) => expandedGroupIds.includes(group.groupId))
                  .map((group) => (
                    <Button
                      key={group.groupId}
                      size="small"
                      onClick={() => {
                        groupFocusRef.current = {
                          applicationId,
                          id: group.groupId,
                        }
                        setExpandedGroups((current) => ({
                          ...current,
                          [applicationId]: (
                            current[applicationId] ?? []
                          ).filter((id) => id !== group.groupId),
                        }))
                        setReadingViewports((current) => ({
                          ...current,
                          [viewportKey]: false,
                        }))
                      }}
                    >
                      {t('runtimeTopology.group.collapse', {
                        kind: group.compactOverview
                          ? t('runtimeTopology.group.compact', {
                              count: group.memberIds.length,
                            })
                          : (group.kind ?? t('runtimeTopology.group.other')),
                      })}
                    </Button>
                  ))}
              </Flex>
            )}
          {value?.warnings.map((warning) => (
            <Alert
              key={warning}
              type="warning"
              showIcon
              title={t(`runtimeTopology.warnings.${warning}`)}
            />
          ))}
          {networkHasUnlinkedEntrance && (
            <Alert
              type="info"
              showIcon
              title={t('runtimeTopology.unlinkedEntrance')}
            />
          )}
          {((resourceMode === 'all' && view === 'resources') ||
            (savedViewport && savedViewport.zoom < 0.75)) && (
            <Typography.Text type="secondary">
              {t('runtimeTopology.overviewHint')}
            </Typography.Text>
          )}
          {view === 'resources' &&
            resourceMode === 'layered' &&
            compact &&
            graph.groupedResourceCount > 0 && (
              <Typography.Text type="secondary">
                {t('runtimeTopology.compactHint')}
              </Typography.Text>
            )}
          {readingViewports[viewportKey] && (
            <Typography.Text type="secondary">
              {t('runtimeTopology.readingHint')}
            </Typography.Text>
          )}
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
                ref={canvasRef}
                className={styles.canvas}
                aria-label={t('runtimeTopology.title')}
                style={
                  compact && graph.groupedResourceCount > 0
                    ? { height: Math.max(420, graph.nodes.length * 94 + 24) }
                    : undefined
                }
              >
                <ReactFlow
                  key={viewportKey}
                  nodes={graph.nodes}
                  edges={graph.edges}
                  nodeTypes={nodeTypes}
                  colorMode={resolvedTheme}
                  defaultMarkerColor="var(--rh-color-border-strong)"
                  fitView={!savedViewport}
                  fitViewOptions={
                    view === 'resources' &&
                    resourceMode === 'layered' &&
                    graph.groupingAvailable
                      ? { ...runtimeFitViewOptions, padding: 0.04, minZoom: 1 }
                      : runtimeFitViewOptions
                  }
                  defaultViewport={savedViewport}
                  onInit={(instance) => {
                    flowRef.current = instance
                  }}
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
                  {!compact && <MiniMap pannable zoomable />}
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
