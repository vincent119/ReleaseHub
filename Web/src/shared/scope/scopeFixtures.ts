import type { CatalogOrganizationNode } from '@/generated/model'

export function scopeFixture(projectCount = 2): CatalogOrganizationNode[] {
  return [
    {
      id: 'organization-a',
      name: '測試組織',
      version: 1,
      isDefault: true,
      canRename: false,
      canDelete: false,
      canCreateProject: false,
      projects: Array.from({ length: projectCount }, (_, index) => ({
        id: `project-${index}`,
        name: `測試專案 ${index}`,
        canManage: false,
        environments: [
          {
            id: `environment-${index}`,
            name: 'Production',
            type: 'Production',
            applications: [],
          },
          {
            id: `global-${index}`,
            name: 'Global',
            type: 'Production',
            applications: [],
          },
        ],
      })),
    },
  ]
}

export function multipleScopeFixture(): CatalogOrganizationNode[] {
  const first = scopeFixture()[0]
  return [
    first,
    {
      ...first,
      id: 'organization-b',
      name: '另一組織',
      isDefault: false,
      projects: [{ ...first.projects[0], id: 'project-b', environments: [] }],
    },
  ]
}
