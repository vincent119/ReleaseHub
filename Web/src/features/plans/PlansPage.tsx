import { PlusOutlined } from '@ant-design/icons'
import { Alert, Button, Empty, Flex, Space, Typography, message } from 'antd'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import {
  changeDeploymentPlanVersionLifecycle,
  createDeploymentPlan,
  createDeploymentPlanVersion,
  useGetCatalogResourceTree,
  useListDeploymentPlans,
  useListReleaseWorkflows,
} from '@/generated/api'
import type { DeploymentPlan, DeploymentPlanVersion } from '@/generated/model'

import { DeploymentBindingPanel } from './components/DeploymentBindingPanel'
import { PlanDetail } from './components/PlanDetail'
import {
  PlanEditorDrawer,
  type PlanEditorValue,
} from './components/PlanEditorDrawer'
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

export function PlansPage() {
  const { t } = useTranslation()
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
  const plans = plansQuery.data?.status === 200 ? plansQuery.data.data.data : []
  const plan = plans.find((item) => item.id === planID) ?? plans[0]
  const version = selectedVersion(plan, versionID)
  const mutate = async (
    operation: (options: RequestInit) => Promise<{ status: number }>,
  ) => {
    const csrf = browserCookie('releasehub_csrf')
    if (!csrf) return void message.error(t('plans.mutation.error'))
    setSubmitting(true)
    try {
      const response = await operation({ headers: { 'X-CSRF-Token': csrf } })
      if (response.status < 200 || response.status >= 300)
        return void message.error(t('plans.mutation.rejected'))
      setEditor(undefined)
      message.success(t('plans.mutation.saved'))
      await plansQuery.refetch()
    } catch {
      message.error(t('plans.mutation.error'))
    } finally {
      setSubmitting(false)
    }
  }
  const save = (value: PlanEditorValue) => {
    if (!scope || !editor) return
    if (editor.mode === 'create') {
      return mutate((options) =>
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
    return mutate((options) =>
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
    return <Alert type="error" showIcon message={t('plans.unavailable')} />
  const organizations =
    resources.data?.status === 200 ? resources.data.data.data : []
  const workflowValues =
    workflows.data?.status === 200 ? workflows.data.data.data : []
  return (
    <Space orientation="vertical" size="large" className={styles.page}>
      <Flex justify="space-between" align="start" gap="middle" wrap>
        <div>
          <Typography.Title level={2}>{t('plans.title')}</Typography.Title>
          <Typography.Paragraph type="secondary">
            {t('plans.description')}
          </Typography.Paragraph>
        </div>
        <Button
          type="primary"
          icon={<PlusOutlined />}
          disabled={!scope}
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
      {!scope ? (
        <Empty description={t('plans.scope.select')} />
      ) : (
        <>
          <div className={styles.workspace}>
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
                void mutate((options) =>
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
            <DeploymentBindingPanel
              organizationId={scope.organizationId}
              projectId={scope.projectId}
              environmentId={scope.environmentId}
              workflows={workflowValues}
              plans={plans}
            />
          )}
        </>
      )}
      {editor && (
        <PlanEditorDrawer
          open
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
