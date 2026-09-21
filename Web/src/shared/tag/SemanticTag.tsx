import { Tag, type TagProps } from 'antd'

import styles from './SemanticTag.module.css'

export type SemanticTagTone =
  'neutral' | 'success' | 'warning' | 'error' | 'info'

export type SemanticTagProps = Omit<TagProps, 'color'> & {
  tone?: SemanticTagTone
}

export function SemanticTag({
  tone = 'neutral',
  className,
  ...props
}: SemanticTagProps) {
  return (
    <Tag
      {...props}
      className={[styles.root, styles[tone], className]
        .filter(Boolean)
        .join(' ')}
    />
  )
}
