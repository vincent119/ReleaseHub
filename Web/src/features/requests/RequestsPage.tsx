import { Alert, Flex, Space, Typography } from 'antd'
import { useTranslation } from 'react-i18next'
import { useReducer, useState } from 'react'

import { useGetCatalogResourceTree } from '@/generated/api'
import { resolveResourceScope, type ResourceScopeValue } from '@/shared/scope'

import {
  RequestScopeSelector,
  type RequestScope,
} from './components/RequestScopeSelector'
import { RequestList } from './components/RequestList'
import { RequestListFilters } from './components/RequestListFilters'
import {
  initialRequestListCriteria,
  requestListCriteriaReducer,
} from './model/listQuery'
import styles from './RequestsPage.module.css'

export function RequestsPage() {
  const { t } = useTranslation()
  const resources = useGetCatalogResourceTree()
  const [choice, setChoice] = useState<ResourceScopeValue>()
  const [scopeInvalidated, setScopeInvalidated] = useState(false)
  const [criteria, dispatchCriteria] = useReducer(
    requestListCriteriaReducer,
    initialRequestListCriteria,
  )
  const organizations =
    resources.data?.status === 200 ? resources.data.data.data : []
  const catalogAvailable =
    !resources.isPending && !resources.isError && resources.data?.status === 200
  const resolved = resolveResourceScope(organizations, choice).value
  // Catalog 移除資源時清除儲存的舊 ID，避免它在後續回應中再次生效。
  if (
    catalogAvailable &&
    choice &&
    ((choice.organizationId !== undefined &&
      choice.organizationId !== resolved.organizationId) ||
      (choice.projectId !== undefined &&
        choice.projectId !== resolved.projectId) ||
      (choice.environmentId !== undefined &&
        choice.environmentId !== resolved.environmentId))
  ) {
    setChoice(resolved)
    setScopeInvalidated(true)
  }
  const scope: RequestScope | undefined =
    catalogAvailable &&
    resolved.organizationId &&
    resolved.projectId &&
    resolved.environmentId
      ? {
          organizationId: resolved.organizationId,
          projectId: resolved.projectId,
          environmentId: resolved.environmentId,
        }
      : undefined
  if (resources.isError || (resources.data && resources.data.status !== 200)) {
    return <Alert type="error" showIcon title={t('requests.unavailable')} />
  }
  return (
    <Space orientation="vertical" size="large" className={styles.page}>
      <Flex justify="space-between" align="start" gap="middle" wrap>
        <div>
          <Typography.Title level={2}>{t('requests.title')}</Typography.Title>
          <Typography.Paragraph type="secondary">
            {t('requests.description')}
          </Typography.Paragraph>
        </div>
      </Flex>
      <section
        className={styles.filters}
        aria-label={t('requests.scope.label')}
      >
        <RequestScopeSelector
          organizations={organizations}
          value={choice}
          loading={resources.isPending}
          onChange={(value) => {
            setChoice(value)
            setScopeInvalidated(false)
          }}
        />
        <RequestListFilters
          criteria={criteria}
          disabled={!scope}
          dispatch={dispatchCriteria}
        />
      </section>
      {scopeInvalidated && (
        <Alert
          type="warning"
          showIcon
          title={t('requests.scope.invalidated')}
        />
      )}
      <RequestList scope={scope} criteria={criteria} />
    </Space>
  )
}
