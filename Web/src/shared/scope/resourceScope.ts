import type {
  CatalogEnvironmentNode,
  CatalogOrganizationNode,
  CatalogProjectNode,
} from '@/generated/model'

export interface ResourceScopeValue {
  organizationId?: string
  projectId?: string
  environmentId?: string
}

export type ResourceScopeField = keyof ResourceScopeValue

interface ResolvedResourceScope {
  organization?: CatalogOrganizationNode
  project?: CatalogProjectNode
  environment?: CatalogEnvironmentNode
  value: ResourceScopeValue
}

export function resolveResourceScope(
  organizations: CatalogOrganizationNode[],
  value: ResourceScopeValue = {},
): ResolvedResourceScope {
  // 明確指定但已失效的組織不可被另一個組織悄悄取代。
  const organization = value.organizationId
    ? organizations.find((item) => item.id === value.organizationId)
    : organizations.length === 1
      ? organizations[0]
      : undefined
  const project = organization?.projects.find(
    (item) => item.id === value.projectId,
  )
  const environment = project?.environments.find(
    (item) => item.id === value.environmentId,
  )
  const resolved: ResourceScopeValue = {}
  if (organization) resolved.organizationId = organization.id
  if (project) resolved.projectId = project.id
  if (environment) resolved.environmentId = environment.id
  return { organization, project, environment, value: resolved }
}

export function updateResourceScope(
  organizations: CatalogOrganizationNode[],
  current: ResourceScopeValue | undefined,
  field: ResourceScopeField,
  id: string | undefined,
): ResourceScopeValue {
  const scope = resolveResourceScope(organizations, current)
  if (field === 'organizationId') {
    const organization = organizations.find((item) => item.id === id)
    return organization ? { organizationId: organization.id } : {}
  }
  const next: ResourceScopeValue = {}
  if (scope.organization) next.organizationId = scope.organization.id
  if (field === 'projectId') {
    const project = scope.organization?.projects.find((item) => item.id === id)
    if (project) next.projectId = project.id
    return next
  }
  if (scope.project) next.projectId = scope.project.id
  const environment = scope.project?.environments.find((item) => item.id === id)
  if (environment) next.environmentId = environment.id
  return next
}
