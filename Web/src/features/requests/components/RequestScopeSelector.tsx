import { Select } from 'antd'
import { useTranslation } from 'react-i18next'

import type { CatalogOrganizationNode } from '@/generated/model'

export interface RequestScope {
  organizationId: string
  projectId: string
  environmentId: string
}

interface Props {
  organizations: CatalogOrganizationNode[]
  value?: RequestScope
  onChange: (scope: RequestScope) => void
}

interface ScopeOption {
  label: string
  value: string
  scope: RequestScope
}

export function RequestScopeSelector({
  organizations,
  value,
  onChange,
}: Props) {
  const { t } = useTranslation()
  const options = scopeOptions(organizations)
  return (
    <Select
      showSearch
      className="request-scope-selector"
      aria-label={t('requests.scope.label')}
      placeholder={t('requests.scope.placeholder')}
      value={value ? scopeKey(value) : undefined}
      options={options}
      optionFilterProp="label"
      onChange={(key) => {
        const selected = options.find((option) => option.value === key)
        if (selected) onChange(selected.scope)
      }}
    />
  )
}

function scopeOptions(organizations: CatalogOrganizationNode[]): ScopeOption[] {
  return organizations.flatMap((organization) =>
    organization.projects.flatMap((project) =>
      project.environments.map((environment) => ({
        label: `${organization.name} / ${project.name} / ${environment.name}`,
        value: scopeKey({
          organizationId: organization.id,
          projectId: project.id,
          environmentId: environment.id,
        }),
        scope: {
          organizationId: organization.id,
          projectId: project.id,
          environmentId: environment.id,
        },
      })),
    ),
  )
}

function scopeKey(scope: RequestScope) {
  return `${scope.organizationId}:${scope.projectId}:${scope.environmentId}`
}
