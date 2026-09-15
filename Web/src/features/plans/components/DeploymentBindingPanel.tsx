import { Alert, Button, Card, Form, Select, Typography } from 'antd'
import { useTranslation } from 'react-i18next'

import {
  bindDeploymentDefinitions,
  useGetDeploymentBinding,
} from '@/generated/api'
import type { DeploymentPlan, ReleaseWorkflow } from '@/generated/model'
import { useFeedback } from '@/shared/feedback/useFeedback'

interface Props {
  organizationId: string
  projectId: string
  environmentId: string
  workflows: ReleaseWorkflow[]
  plans: DeploymentPlan[]
}

interface BindingFields {
  workflowVersionId: string
  planVersionId: string
}

export function DeploymentBindingPanel(props: Props) {
  const { t } = useTranslation()
  const feedback = useFeedback()
  const [form] = Form.useForm<BindingFields>()
  const query = useGetDeploymentBinding(scopeParams(props))
  const binding = query.data?.status === 200 ? query.data.data.data : null
  const submit = async (fields: BindingFields) => {
    const csrf = browserCookie('releasehub_csrf')
    if (!csrf) return void feedback.error(t('plans.binding.error'))
    const response = await bindDeploymentDefinitions(
      {
        ...scopeParams(props),
        ...fields,
        expectedVersion: binding?.version ?? 0,
      },
      { headers: { 'X-CSRF-Token': csrf } },
    )
    if (response.status !== 200)
      return void feedback.error(t('plans.binding.rejected'))
    feedback.success(t('plans.binding.saved'))
    await query.refetch()
  }
  if (query.isError || (query.data && query.data.status !== 200))
    return (
      <Alert type="warning" showIcon title={t('plans.binding.unavailable')} />
    )
  return (
    <Card title={t('plans.binding.title')} loading={query.isPending}>
      <Typography.Paragraph type="secondary">
        {binding
          ? t('plans.binding.version', { version: binding.version })
          : t('plans.binding.unbound')}
      </Typography.Paragraph>
      <Form
        key={`${binding?.id ?? 'unbound'}:${binding?.version ?? 0}`}
        form={form}
        layout="vertical"
        initialValues={binding ?? undefined}
        onFinish={(fields) => void submit(fields)}
      >
        <Form.Item
          name="workflowVersionId"
          label={t('plans.binding.workflow')}
          rules={[{ required: true }]}
        >
          <Select options={publishedWorkflowOptions(props.workflows)} />
        </Form.Item>
        <Form.Item
          name="planVersionId"
          label={t('plans.binding.plan')}
          rules={[{ required: true }]}
        >
          <Select options={publishedPlanOptions(props.plans)} />
        </Form.Item>
        <Button type="primary" htmlType="submit">
          {t('plans.binding.save')}
        </Button>
      </Form>
    </Card>
  )
}

function scopeParams(props: Props) {
  return {
    organizationId: props.organizationId,
    projectId: props.projectId,
    environmentId: props.environmentId,
  }
}

function publishedWorkflowOptions(values: ReleaseWorkflow[]) {
  return values.flatMap((workflow) =>
    workflow.versions
      .filter((version) => version.lifecycle === 'Published')
      .map((version) => ({
        value: version.id,
        label: `${workflow.name} · v${version.versionNumber}`,
      })),
  )
}

function publishedPlanOptions(values: DeploymentPlan[]) {
  return values.flatMap((plan) =>
    plan.versions
      .filter((version) => version.lifecycle === 'Published')
      .map((version) => ({
        value: version.id,
        label: `${plan.name} · v${version.versionNumber}`,
      })),
  )
}

function browserCookie(name: string): string | undefined {
  return document.cookie
    .split('; ')
    .find((value) => value.startsWith(`${name}=`))
    ?.slice(name.length + 1)
}
