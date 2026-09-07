import { Card, Flex, Select } from 'antd'
import { useTranslation } from 'react-i18next'

import type { CatalogOrganizationNode } from '@/generated/model'

export interface PlanScopeChoice {
  organizationId: string
  projectId: string
  environmentId?: string
}

interface Props {
  organizations: CatalogOrganizationNode[]
  scope?: PlanScopeChoice
  onChange: (value: PlanScopeChoice) => void
}

export function PlanScopeSelector({ organizations, scope, onChange }: Props) {
  const { t } = useTranslation()
  const projects = organizations.flatMap((organization) =>
    organization.projects.map((project) => ({
      value: `${organization.id}:${project.id}`,
      label: `${organization.name} / ${project.name}`,
      organizationId: organization.id,
      project,
    })),
  )
  const selected = projects.find((item) => item.project.id === scope?.projectId)
  return (
    <Card>
      <Flex gap="middle" wrap>
        <Select
          aria-label={t('plans.scope.project')}
          placeholder={t('plans.scope.project')}
          value={selected?.value}
          options={projects.map(({ value, label }) => ({ value, label }))}
          onChange={(value) => {
            const item = projects.find((project) => project.value === value)!
            onChange({
              organizationId: item.organizationId,
              projectId: item.project.id,
            })
          }}
        />
        <Select
          aria-label={t('plans.scope.environment')}
          placeholder={t('plans.scope.environment')}
          disabled={!selected}
          value={scope?.environmentId}
          options={selected?.project.environments.map((environment) => ({
            value: environment.id,
            label: environment.name,
          }))}
          onChange={(environmentId) =>
            scope && onChange({ ...scope, environmentId })
          }
        />
      </Flex>
    </Card>
  )
}
