import { InfoCircleOutlined } from '@ant-design/icons'
import { Button, Popover, Select } from 'antd'
import { useId } from 'react'
import { useTranslation } from 'react-i18next'

import type { CatalogOrganizationNode } from '@/generated/model'

import styles from './ResourceScopeSelector.module.css'
import {
  resolveResourceScope,
  updateResourceScope,
  type ResourceScopeField,
  type ResourceScopeValue,
} from './resourceScope'

export interface ResourceScopeSelectorProps {
  organizations: CatalogOrganizationNode[]
  value?: ResourceScopeValue
  onChange: (value: ResourceScopeValue) => void
  loading?: boolean
  disabled?: boolean
  environmentMode?: 'required' | 'optional'
}

interface ScopeOption {
  value: string
  label: string
}

interface ScopeFieldProps {
  label: string
  nameLabel: string
  placeholder: string
  options: ScopeOption[]
  value?: string
  selectedName?: string
  disabled: boolean
  loading: boolean
  required?: boolean
  hint?: string
  onChange: (id: string | undefined) => void
}

function ScopeField({
  label,
  nameLabel,
  placeholder,
  options,
  value,
  selectedName,
  disabled,
  loading,
  required = false,
  hint,
  onChange,
}: ScopeFieldProps) {
  const id = useId()
  const { t } = useTranslation()
  return (
    <div className={styles.field}>
      <label htmlFor={id} className={styles.label}>
        {label}
      </label>
      <div className={styles.control}>
        <Select<string | undefined, ScopeOption>
          id={id}
          aria-required={required}
          aria-describedby={hint ? `${id}-hint` : undefined}
          className={styles.select}
          classNames={{ popup: { root: styles.popup } }}
          showSearch={{ optionFilterProp: 'label' }}
          allowClear
          virtual={false}
          value={value}
          options={options}
          disabled={disabled}
          loading={loading}
          placeholder={placeholder}
          notFoundContent={t('scopeFilter.noMatches')}
          onChange={onChange}
        />
        <div
          className={styles.nameSlot}
          aria-hidden={!selectedName || undefined}
        >
          {selectedName && (
            <Popover
              trigger="click"
              content={
                <span className={styles.fullName} data-scope-full-name>
                  {selectedName}
                </span>
              }
            >
              <Button
                type="text"
                className={styles.nameButton}
                icon={<InfoCircleOutlined />}
                aria-label={t('scopeFilter.fullName', { field: nameLabel })}
              />
            </Popover>
          )}
        </div>
      </div>
      {hint && (
        <span id={`${id}-hint`} className={styles.hint}>
          {hint}
        </span>
      )}
    </div>
  )
}

export function ResourceScopeSelector({
  organizations,
  value,
  onChange,
  loading = false,
  disabled = false,
  environmentMode = 'required',
}: ResourceScopeSelectorProps) {
  const { t } = useTranslation()
  const scope = resolveResourceScope(organizations, value)
  const unavailable = disabled || loading
  const change = (field: ResourceScopeField) => (id: string | undefined) => {
    onChange(updateResourceScope(organizations, value, field, id))
  }
  const projectHint = loading
    ? t('scopeFilter.loading')
    : organizations.length === 0
      ? t('scopeFilter.noOrganizations')
      : !scope.organization
        ? t('scopeFilter.chooseOrganization')
        : scope.organization.projects.length === 0
          ? t('scopeFilter.noProjects')
          : undefined
  const environmentHint = loading
    ? undefined
    : !scope.project
      ? t('scopeFilter.chooseProject')
      : scope.project.environments.length === 0
        ? t('scopeFilter.noEnvironments')
        : undefined

  return (
    <div className={styles.selector}>
      {organizations.length > 1 && (
        <ScopeField
          label={t('scopeFilter.organization')}
          nameLabel={t('scopeFilter.organization')}
          placeholder={t('scopeFilter.organizationPlaceholder')}
          options={organizations.map((item) => ({
            value: item.id,
            label: item.name,
          }))}
          value={scope.value.organizationId}
          selectedName={scope.organization?.name}
          disabled={unavailable}
          loading={loading}
          onChange={change('organizationId')}
        />
      )}
      <ScopeField
        label={t('scopeFilter.project')}
        nameLabel={t('scopeFilter.project')}
        placeholder={t('scopeFilter.projectPlaceholder')}
        options={(scope.organization?.projects ?? []).map((item) => ({
          value: item.id,
          label: item.name,
        }))}
        value={scope.value.projectId}
        selectedName={scope.project?.name}
        disabled={
          unavailable ||
          !scope.organization ||
          scope.organization.projects.length === 0
        }
        loading={loading}
        hint={projectHint}
        onChange={change('projectId')}
      />
      <ScopeField
        label={t(
          environmentMode === 'required'
            ? 'scopeFilter.environmentRequired'
            : 'scopeFilter.environmentOptional',
        )}
        nameLabel={t('scopeFilter.environment')}
        placeholder={t('scopeFilter.environmentPlaceholder')}
        options={(scope.project?.environments ?? []).map((item) => ({
          value: item.id,
          label: item.name,
        }))}
        value={scope.value.environmentId}
        selectedName={scope.environment?.name}
        disabled={
          unavailable ||
          !scope.project ||
          scope.project.environments.length === 0
        }
        loading={loading}
        required={environmentMode === 'required'}
        hint={environmentHint}
        onChange={change('environmentId')}
      />
    </div>
  )
}
