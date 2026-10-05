import type { CatalogOrganizationNode } from '@/generated/model'
import { ResourceScopeSelector, type ResourceScopeValue } from '@/shared/scope'

export interface RequestScope {
  organizationId: string
  projectId: string
  environmentId: string
}

interface Props {
  organizations: CatalogOrganizationNode[]
  value?: ResourceScopeValue
  onChange: (scope: ResourceScopeValue) => void
  loading?: boolean
  disabled?: boolean
}

export function RequestScopeSelector({
  organizations,
  value,
  onChange,
  loading,
  disabled,
}: Props) {
  return (
    <ResourceScopeSelector
      organizations={organizations}
      value={value}
      onChange={onChange}
      loading={loading}
      disabled={disabled}
      environmentMode="required"
    />
  )
}
