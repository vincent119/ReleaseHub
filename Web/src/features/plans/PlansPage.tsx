import { PlusOutlined } from '@ant-design/icons'
import { useQueryClient } from '@tanstack/react-query'
import { Alert, Button, Empty, Flex, Space, Typography } from 'antd'
import { useEffect, useReducer, useState } from 'react'
import { useTranslation } from 'react-i18next'

import {
  changeDeploymentPlanVersionLifecycle,
  createDeploymentPlan,
  createDeploymentPlanVersion,
  getGetAuthSessionQueryKey,
  useGetCatalogResourceTree,
  useListDeploymentPlans,
  useListReleaseWorkflows,
} from '@/generated/api'
import type { DeploymentPlan, DeploymentPlanVersion } from '@/generated/model'
import { parseAPIErrorResponse, type APIErrorView } from '@/shared/api/apiError'
import definitionStyles from '@/shared/definition/DefinitionWorkspace.module.css'
import { useFeedback } from '@/shared/feedback/useFeedback'
import { resolveResourceScope, type ResourceScopeValue } from '@/shared/scope'

import { DeploymentBindingPanel } from './components/DeploymentBindingPanel'
import { DeploymentSchedulePanel } from './components/DeploymentSchedulePanel'
import { PlanDetail } from './components/PlanDetail'
import {
  PlanEditorWorkspace,
  type PlanEditorValue,
} from './components/PlanEditorWorkspace'
import { PlanList } from './components/PlanList'
import {
  PlanScopeSelector,
  type PlanScopeChoice,
} from './components/PlanScopeSelector'
import { emptyPlanDocument } from './model/planGraph'
import styles from './PlansPage.module.css'

const emptyID = '00000000-0000-0000-0000-000000000000'
interface EditorState {
  mode: 'create' | 'version'
  plan?: DeploymentPlan
  initial: PlanEditorValue
}
type PlanMutationOperation = 'create' | 'version' | 'lifecycle'

interface ScopeState {
  choice?: ResourceScopeValue
  invalidated: boolean
  selection: { planID?: string; versionID?: string }
}

type ScopeAction =
  | { type: 'choose' | 'invalidate'; value: ResourceScopeValue }
  | { type: 'plan' | 'version'; id: string }

function scopeReducer(state: ScopeState, action: ScopeAction): ScopeState {
  switch (action.type) {
    case 'choose':
    case 'invalidate':
      return {
        choice: action.value,
        invalidated: action.type === 'invalidate',
        selection: {},
      }
    case 'plan':
      return { ...state, selection: { planID: action.id } }
    case 'version':
      return {
        ...state,
        selection: { ...state.selection, versionID: action.id },
      }
  }
}

export function PlansPage() {
  const { t } = useTranslation()
  const feedback = useFeedback()
  const queryClient = useQueryClient()
  const resources = useGetCatalogResourceTree()
  const workflows = useListReleaseWorkflows()
  const [{ choice, invalidated: scopeInvalidated, selection }, dispatchScope] =
    useReducer(scopeReducer, { invalidated: false, selection: {} })
  const [editor, setEditor] = useState<EditorState>()
  const [submitting, setSubmitting] = useState(false)
  const organizations =
    resources.data?.status === 200 ? resources.data.data.data : []
  const catalogAvailable =
    !resources.isPending && !resources.isError && resources.data?.status === 200
  const resolved = resolveResourceScope(organizations, choice).value
  // 同次 render 清除失效 scope 與選取，避免先提交舊 Plan 或版本的畫面。
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
    dispatchScope({ type: 'invalidate', value: resolved })
    setEditor(undefined)
  }
  const scope: PlanScopeChoice | undefined =
    catalogAvailable && resolved.organizationId && resolved.projectId
      ? {
          organizationId: resolved.organizationId,
          projectId: resolved.projectId,
          environmentId: resolved.environmentId,
        }
      : undefined
  const plansQuery = useListDeploymentPlans(
    { projectId: scope?.projectId ?? emptyID },
    { query: { enabled: Boolean(scope?.projectId) } },
  )
  const { planID, versionID } = selection
  const planListStatus = plansQuery.data?.status
  const planListAvailable =
    Boolean(scope) &&
    planListStatus === 200 &&
    !plansQuery.isError &&
    !plansQuery.isPending &&
    !plansQuery.isPlaceholderData
  const planListUnavailable = Boolean(
    scope &&
    (plansQuery.isError ||
      (planListStatus !== undefined && planListStatus !== 200)),
  )
  const plans =
    planListAvailable && plansQuery.data?.status === 200
      ? plansQuery.data.data.data
      : []
  const plan = plans.find((item) => item.id === planID) ?? plans[0]
  const version = selectedVersion(plan, versionID)
  useEffect(() => {
    if (planListStatus !== 401) return
    void queryClient.invalidateQueries({
      queryKey: getGetAuthSessionQueryKey(),
      exact: true,
    })
  }, [planListStatus, queryClient])
  const mutate = async (
    mutationOperation: PlanMutationOperation,
    operation: (
      options: RequestInit,
    ) => Promise<{ status: number; data?: unknown }>,
  ) => {
    const csrf = browserCookie('releasehub_csrf')
    if (!csrf) return void feedback.error(t('plans.mutation.error'))
    setSubmitting(true)
    try {
      const response = await operation({ headers: { 'X-CSRF-Token': csrf } })
      if (response.status < 200 || response.status >= 300) {
        const error = parseAPIErrorResponse(response)
        if (error.status === 401)
          await queryClient.invalidateQueries({
            queryKey: getGetAuthSessionQueryKey(),
            exact: true,
          })
        return void feedback.error(
          t(planMutationErrorKey(mutationOperation, error)),
        )
      }
      setEditor(undefined)
      feedback.success(t('plans.mutation.saved'))
      await plansQuery.refetch()
    } catch {
      feedback.error(t('plans.mutation.error'))
    } finally {
      setSubmitting(false)
    }
  }
  const save = (value: PlanEditorValue) => {
    if (!scope || !editor) return
    if (editor.mode === 'create') {
      return mutate('create', (options) =>
        createDeploymentPlan(
          {
            ownerKind: value.ownerKind,
            ownerProjectId:
              value.ownerKind === 'project' ? scope.projectId : undefined,
            name: value.name,
            description: value.description,
            document: value.document,
          },
          options,
        ),
      )
    }
    return mutate('version', (options) =>
      createDeploymentPlanVersion(
        editor.plan!.id,
        {
          expectedVersion: latestVersionNumber(editor.plan!),
          document: value.document,
        },
        options,
      ),
    )
  }
  if (
    resources.isError ||
    workflows.isError ||
    (resources.data && resources.data.status !== 200) ||
    (workflows.data && workflows.data.status !== 200)
  )
    return <Alert type="error" showIcon title={t('plans.unavailable')} />
  if (editor && scope)
    return (
      <PlanEditorWorkspace
        title={
          editor.mode === 'create'
            ? t('plans.editor.createTitle')
            : t('plans.editor.versionTitle')
        }
        initial={editor.initial}
        creating={editor.mode === 'create'}
        submitting={submitting}
        onClose={() => setEditor(undefined)}
        onSubmit={(value) => void save(value)}
      />
    )
  const workflowValues =
    workflows.data?.status === 200 ? workflows.data.data.data : []
  return (
    <Space orientation="vertical" size="large" className={styles.page}>
      <Flex
        className={styles.pageHeader}
        justify="space-between"
        align="start"
        gap="middle"
        wrap
      >
        <div>
          <Typography.Title level={2}>{t('plans.title')}</Typography.Title>
          <Typography.Paragraph type="secondary">
            {t('plans.description')}
          </Typography.Paragraph>
        </div>
        <Button
          type="primary"
          icon={<PlusOutlined />}
          disabled={!scope || !planListAvailable}
          onClick={() => setEditor(createEditor(t))}
        >
          {t('plans.actions.create')}
        </Button>
      </Flex>
      <PlanScopeSelector
        organizations={organizations}
        scope={choice}
        loading={resources.isPending}
        onChange={(value) => {
          dispatchScope({ type: 'choose', value })
        }}
      />
      {scopeInvalidated && (
        <Alert type="warning" showIcon title={t('plans.scope.invalidated')} />
      )}
      {planListUnavailable ? (
        <Alert type="error" showIcon title={t('plans.unavailable')} />
      ) : !scope ? (
        <Empty description={t('plans.scope.select')} />
      ) : (
        <>
          <div className={definitionStyles.workspace}>
            <PlanList
              plans={plans}
              selected={plan?.id}
              loading={plansQuery.isPending || plansQuery.isPlaceholderData}
              onSelect={(id) => {
                dispatchScope({ type: 'plan', id })
              }}
            />
            <PlanDetail
              plan={plan}
              version={version}
              versionID={versionID}
              submitting={submitting}
              onVersion={(id) => dispatchScope({ type: 'version', id })}
              onNewVersion={() =>
                plan && version && setEditor(versionEditor(plan, version))
              }
              onLifecycle={(lifecycle) =>
                plan &&
                version &&
                void mutate('lifecycle', (options) =>
                  changeDeploymentPlanVersionLifecycle(
                    plan.id,
                    version.id,
                    {
                      lifecycle,
                      expectedVersion: version.lockVersion,
                    },
                    options,
                  ),
                )
              }
            />
          </div>
          {scope.environmentId && planListAvailable && (
            <>
              <DeploymentBindingPanel
                key={`${scope.organizationId}:${scope.projectId}:${scope.environmentId}`}
                organizationId={scope.organizationId}
                projectId={scope.projectId}
                environmentId={scope.environmentId}
                workflows={workflowValues}
                plans={plans}
              />
              <DeploymentSchedulePanel
                key={scope.environmentId}
                environmentId={scope.environmentId}
              />
            </>
          )}
        </>
      )}
    </Space>
  )
}

function selectedVersion(plan?: DeploymentPlan, versionID?: string) {
  return (
    plan?.versions.find((item) => item.id === versionID) ??
    plan?.versions.at(-1)
  )
}

function latestVersionNumber(plan: DeploymentPlan) {
  return plan.versions.at(-1)?.versionNumber ?? 1
}

function createEditor(t: (key: string) => string): EditorState {
  return {
    mode: 'create',
    initial: {
      ownerKind: 'project',
      name: t('plans.template.name'),
      description: t('plans.template.description'),
      document: emptyPlanDocument(),
    },
  }
}

function versionEditor(
  plan: DeploymentPlan,
  version: DeploymentPlanVersion,
): EditorState {
  return {
    mode: 'version',
    plan,
    initial: {
      ownerKind: plan.ownerKind,
      name: plan.name,
      description: plan.description,
      document: version.document,
    },
  }
}

function browserCookie(name: string): string | undefined {
  return document.cookie
    .split('; ')
    .find((value) => value.startsWith(`${name}=`))
    ?.slice(name.length + 1)
}

function planMutationErrorKey(
  operation: PlanMutationOperation,
  error: APIErrorView,
) {
  if (error.status === 401) return 'plans.mutation.unauthenticated'
  if (error.status === 404) return 'plans.mutation.notFound'
  if (error.category === 'conflict')
    return operation === 'create'
      ? 'plans.mutation.nameConflict'
      : 'plans.mutation.versionConflict'
  if (error.status === 422 && operation === 'lifecycle')
    return 'plans.mutation.invalidLifecycle'
  return 'plans.mutation.error'
}
