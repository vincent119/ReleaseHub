import type { ReactNode } from 'react'
import { Typography } from 'antd'

import styles from './GraphEditorFrame.module.css'

interface Props {
  className?: string
  toolbarClassName?: string
  canvasClassName?: string
  inspectorClassName?: string
  toolbarLabel: string
  canvasLabel: string
  inspectorLabel: string
  selectionTitle: ReactNode
  toolbar: ReactNode
  canvas: ReactNode
  inspector: ReactNode
  children?: ReactNode
}

export function GraphEditorFrame({
  className,
  toolbarClassName,
  canvasClassName,
  inspectorClassName,
  toolbarLabel,
  canvasLabel,
  inspectorLabel,
  selectionTitle,
  toolbar,
  canvas,
  inspector,
  children,
}: Props) {
  return (
    <div className={joinClassNames(styles.editor, className)}>
      <div
        className={joinClassNames(styles.toolbar, toolbarClassName)}
        role="toolbar"
        aria-label={toolbarLabel}
      >
        {toolbar}
      </div>
      <div
        className={joinClassNames(styles.canvas, canvasClassName)}
        aria-label={canvasLabel}
      >
        {canvas}
      </div>
      <aside
        className={joinClassNames(styles.inspector, inspectorClassName)}
        aria-label={inspectorLabel}
      >
        <div className={styles.inspectorHeader}>
          <Typography.Text className={styles.inspectorEyebrow}>
            {inspectorLabel}
          </Typography.Text>
          <Typography.Title level={4} className={styles.inspectorTitle}>
            {selectionTitle}
          </Typography.Title>
        </div>
        <div className={styles.inspectorBody}>{inspector}</div>
      </aside>
      {children}
    </div>
  )
}

function joinClassNames(...values: Array<string | undefined>) {
  return values.filter(Boolean).join(' ')
}
