import { Card } from 'antd'

import type { CatalogOrganizationNode } from '@/generated/model'
import { ResourceScopeSelector, type ResourceScopeValue } from '@/shared/scope'

import styles from './PlanScopeSelector.module.css'

export interface PlanScopeChoice {
  organizationId: string
  projectId: string
  environmentId?: string
}

interface Props {
  organizations: CatalogOrganizationNode[]
  scope?: ResourceScopeValue
  onChange: (value: ResourceScopeValue) => void
  loading?: boolean
  disabled?: boolean
}

export function PlanScopeSelector({
  organizations,
  scope,
  onChange,
  loading,
  disabled,
}: Props) {
  return (
    <Card className={styles.scopeCard}>
      <ResourceScopeSelector
        organizations={organizations}
        value={scope}
        onChange={onChange}
        environmentMode="optional"
        loading={loading}
        disabled={disabled}
      />
    </Card>
  )
}
