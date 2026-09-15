import { Spin } from 'antd'
import type { HTMLAttributes, Key, ReactNode } from 'react'

import styles from './SemanticList.module.css'

type RowKey<T> = keyof T | ((item: T, index: number) => Key)

interface SemanticListProps<T> {
  items: T[]
  rowKey: RowKey<T>
  renderItem: (item: T, index: number) => ReactNode
  itemProps?: (item: T, index: number) => HTMLAttributes<HTMLLIElement>
  header?: ReactNode
  loading?: boolean
  emptyContent?: ReactNode
  className?: string
  compact?: boolean
  ariaLabel?: string
}

interface SemanticListItemContentProps {
  title: ReactNode
  description?: ReactNode
  leading?: ReactNode
  leadingVariant?: 'default' | 'accent'
  truncate?: boolean
  actions?: ReactNode[]
  extra?: ReactNode
}

export function SemanticList<T>({
  items,
  rowKey,
  renderItem,
  itemProps,
  header,
  loading = false,
  emptyContent,
  className,
  compact = false,
  ariaLabel,
}: SemanticListProps<T>) {
  const content = items.length ? (
    <ul
      className={joinClassNames(
        styles.list,
        compact ? styles.compact : undefined,
        className,
      )}
      aria-label={ariaLabel}
      aria-busy={loading}
    >
      {items.map((item, index) => {
        const props = itemProps?.(item, index)
        return (
          <li
            {...props}
            key={resolveRowKey(item, index, rowKey)}
            className={joinClassNames(styles.item, props?.className)}
          >
            {renderItem(item, index)}
          </li>
        )
      })}
    </ul>
  ) : (
    emptyContent
  )

  return (
    <div className={styles.root} aria-busy={loading}>
      {header && <div className={styles.header}>{header}</div>}
      <Spin spinning={loading}>{content}</Spin>
    </div>
  )
}

export function SemanticListItemContent({
  title,
  description,
  leading,
  leadingVariant = 'default',
  truncate = false,
  actions = [],
  extra,
}: SemanticListItemContentProps) {
  return (
    <>
      <div className={styles.main}>
        {leading && (
          <div
            className={joinClassNames(
              styles.leading,
              leadingVariant === 'accent' ? styles.leadingAccent : undefined,
            )}
          >
            {leading}
          </div>
        )}
        <div
          className={joinClassNames(
            styles.metadata,
            truncate ? styles.truncate : undefined,
          )}
        >
          <div className={styles.title}>{title}</div>
          {description !== undefined && (
            <div className={styles.description}>{description}</div>
          )}
        </div>
      </div>
      {extra && <div className={styles.extra}>{extra}</div>}
      {actions.length > 0 && <div className={styles.actions}>{actions}</div>}
    </>
  )
}

function resolveRowKey<T>(item: T, index: number, rowKey: RowKey<T>) {
  return typeof rowKey === 'function'
    ? rowKey(item, index)
    : String(item[rowKey])
}

function joinClassNames(...values: Array<string | undefined>) {
  return values.filter(Boolean).join(' ')
}
