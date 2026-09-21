import { Link, type LinkProps } from 'react-router'

import styles from './ThemedLink.module.css'

export function ThemedLink({ className, ...props }: LinkProps) {
  return (
    <Link
      {...props}
      className={[styles.link, className].filter(Boolean).join(' ')}
    />
  )
}
