import { PlusOutlined } from '@ant-design/icons'
import { useQueryClient } from '@tanstack/react-query'
import { Alert, Button, Empty, Flex, Space, Typography } from 'antd'
import { useEffect, useState } from 'react'
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

export function PlansPage() {
  const { t } = useTranslation()
  const feedback = useFeedback()
  const queryClient = useQueryClient()
  const resources = useGetCatalogResourceTree()
  const workflows = useListReleaseWorkflows()
  const [scope, setScope] = useState<PlanScopeChoice>()
  const plansQuery = useListDeploymentPlans(
    { projectId: scope?.projectId ?? emptyID },
    { query: { enabled: Boolean(scope?.projectId) } },
  )
  const [planID, setPlanID] = useState<string>()
  const [versionID, setVersionID] = useState<string>()
  const [editor, setEditor] = useState<EditorState>()
  const [submitting, setSubmitting] = useState(false)
  const planListStatus = plansQuery.data?.status
  const planListAvailable = planListStatus === 200 && !plansQuery.isError
  const planListUnavailable = Boolean(
    scope &&
    (plansQuery.isError ||
      (planListStatus !== undefined && planListStatus !== 200)),
  )
  const plans = plansQuery.data?.status === 200 ? plansQuery.data.data.data : []
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
  if (resources.isError || workflows.isError)
    return <Alert type="error" showIcon title={t('plans.unavailable')} />
  if (editor)
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
  const organizations =
    resources.data?.status === 200 ? resources.data.data.data : []
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
        scope={scope}
        onChange={(value) => {
          setScope(value)
          setPlanID(undefined)
          setVersionID(undefined)
        }}
      />
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
              loading={plansQuery.isPending}
              onSelect={(id) => {
                setPlanID(id)
                setVersionID(undefined)
              }}
            />
            <PlanDetail
              plan={plan}
              version={version}
              versionID={versionID}
              submitting={submitting}
              onVersion={setVersionID}
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
          {scope.environmentId && (
            <>
              <DeploymentBindingPanel
                organizationId={scope.organizationId}
                projectId={scope.projectId}
                environmentId={scope.environmentId}
                workflows={workflowValues}
                plans={plans}
              />
              <DeploymentSchedulePanel environmentId={scope.environmentId} />
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
