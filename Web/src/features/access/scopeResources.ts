import type { CatalogOrganizationNode } from '@/generated/model'

export type ScopeKind = 'platform' | 'project' | 'environment' | 'application'

export interface ScopeSelection {
  scopeKind?: ScopeKind
  scopeId?: string
}

export function scopeResourceOptions(
  kind: ScopeKind,
  organizations: CatalogOrganizationNode[],
  singleTenant: boolean,
) {
  if (kind === 'project')
    return organizations.flatMap((organization) =>
      organization.projects.map((project) => ({
        value: project.id,
        label: singleTenant
          ? project.name
          : `${organization.name} / ${project.name}`,
      })),
    )
  if (kind === 'environment')
    return organizations.flatMap((organization) =>
      organization.projects.flatMap((project) =>
        project.environments.map((environment) => ({
          value: environment.id,
          label: singleTenant
            ? `${project.name} / ${environment.name}`
            : `${organization.name} / ${project.name} / ${environment.name}`,
        })),
      ),
    )
  if (kind === 'application')
    return organizations.flatMap((organization) =>
      organization.projects.flatMap((project) =>
        project.environments.flatMap((environment) =>
          environment.applications.map((application) => ({
            value: application.id,
            label: singleTenant
              ? `${project.name} / ${environment.name} / ${application.name}`
              : `${organization.name} / ${project.name} / ${environment.name} / ${application.name}`,
          })),
        ),
      ),
    )
  return []
}

export function scopePayload(
  fields: ScopeSelection,
  organizations: CatalogOrganizationNode[],
) {
  if (fields.scopeKind === 'platform') return { scopeKind: 'platform' as const }
  for (const organization of organizations)
    for (const project of organization.projects) {
      if (fields.scopeKind === 'project' && project.id === fields.scopeId)
        return {
          scopeKind: 'project' as const,
          organizationId: organization.id,
          projectId: project.id,
        }
      for (const environment of project.environments) {
        if (
          fields.scopeKind === 'environment' &&
          environment.id === fields.scopeId
        )
          return {
            scopeKind: 'environment' as const,
            organizationId: organization.id,
            projectId: project.id,
            environmentId: environment.id,
          }
        const application = environment.applications.find(
          (item) => item.id === fields.scopeId,
        )
        if (fields.scopeKind === 'application' && application)
          return {
            scopeKind: 'application' as const,
            organizationId: organization.id,
            projectId: project.id,
            environmentId: environment.id,
            applicationId: application.id,
          }
      }
    }
  return { scopeKind: fields.scopeKind ?? ('project' as const) }
}
