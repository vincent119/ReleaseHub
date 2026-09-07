import { ApartmentOutlined, PlusOutlined } from '@ant-design/icons'
import {
  Alert,
  Button,
  Card,
  Empty,
  Flex,
  List,
  Select,
  Space,
  Tag,
  Typography,
  message,
} from 'antd'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import {
  changeReleaseWorkflowVersionLifecycle,
  createReleaseWorkflow,
  createReleaseWorkflowVersion,
  useListReleaseWorkflows,
} from '@/generated/api'
import type {
  DefinitionLifecycle,
  ReleaseWorkflow,
  ReleaseWorkflowVersion,
} from '@/generated/model'

import {
  WorkflowEditorDrawer,
  type WorkflowEditorValue,
} from './components/WorkflowEditorDrawer'
import { WorkflowGraphEditor } from './components/WorkflowGraphEditor'
import { productionApprovalTemplate } from './model/workflowGraph'
import styles from './WorkflowsPage.module.css'

interface EditorState {
  mode: 'create' | 'version'
  workflow?: ReleaseWorkflow
  initial: WorkflowEditorValue
}

export function WorkflowsPage() {
  const { t } = useTranslation()
  const query = useListReleaseWorkflows()
  const [workflowID, setWorkflowID] = useState<string>()
  const [versionID, setVersionID] = useState<string>()
  const [editor, setEditor] = useState<EditorState>()
  const [submitting, setSubmitting] = useState(false)
  const workflows = query.data?.status === 200 ? query.data.data.data : []
  const workflow =
    workflows.find((item) => item.id === workflowID) ?? workflows[0]
  const version = selectedVersion(workflow, versionID)
  const mutate = async (
    operation: (options: RequestInit) => Promise<{ status: number }>,
  ) => {
    const csrf = browserCookie('releasehub_csrf')
    if (!csrf) return void message.error(t('workflows.mutation.error'))
    setSubmitting(true)
    try {
      const response = await operation({ headers: { 'X-CSRF-Token': csrf } })
      if (response.status < 200 || response.status >= 300)
        return void message.error(t('workflows.mutation.rejected'))
      setEditor(undefined)
      message.success(t('workflows.mutation.saved'))
      await query.refetch()
    } catch {
      message.error(t('workflows.mutation.error'))
    } finally {
      setSubmitting(false)
    }
  }
  const save = (value: WorkflowEditorValue) =>
    editor?.mode === 'create'
      ? mutate((options) => createReleaseWorkflow(value, options))
      : mutate((options) =>
          createReleaseWorkflowVersion(
            editor!.workflow!.id,
            {
              expectedVersion: latestLock(editor!.workflow!),
              document: value.document,
            },
            options,
          ),
        )
  if (query.isError || (query.data && query.data.status !== 200))
    return (
      <Alert
        type="warning"
        showIcon
        message={t('workflows.unavailable.title')}
        description={t('workflows.unavailable.description')}
      />
    )
  return (
    <Space orientation="vertical" size="large" className={styles.page}>
      <Flex justify="space-between" align="start" gap="middle" wrap>
        <div>
          <Typography.Title level={2}>{t('workflows.title')}</Typography.Title>
          <Typography.Paragraph type="secondary">
            {t('workflows.description')}
          </Typography.Paragraph>
        </div>
        <Button
          type="primary"
          icon={<PlusOutlined />}
          onClick={() => setEditor(createEditor(t))}
        >
          {t('workflows.actions.create')}
        </Button>
      </Flex>
      <div className={styles.workspace}>
        <Card title={t('workflows.list.title')} loading={query.isPending}>
          {workflows.length === 0 ? (
            <Empty description={t('workflows.list.empty')} />
          ) : (
            <List
              dataSource={workflows}
              renderItem={(item) => (
                <List.Item
                  className={
                    item.id === workflow?.id ? styles.selected : undefined
                  }
                  onClick={() => {
                    setWorkflowID(item.id)
                    setVersionID(undefined)
                  }}
                >
                  <List.Item.Meta
                    avatar={<ApartmentOutlined />}
                    title={item.name}
                    description={item.description}
                  />
                </List.Item>
              )}
            />
          )}
        </Card>
        <WorkflowDetail
          workflow={workflow}
          version={version}
          versionID={versionID}
          setVersionID={setVersionID}
          submitting={submitting}
          onNewVersion={() =>
            workflow && version && setEditor(versionEditor(workflow, version))
          }
          onLifecycle={(lifecycle) =>
            workflow &&
            version &&
            void mutate((options) =>
              changeReleaseWorkflowVersionLifecycle(
                workflow.id,
                version.id,
                { lifecycle, expectedVersion: version.lockVersion },
                options,
              ),
            )
          }
        />
      </div>
      {editor && (
        <WorkflowEditorDrawer
          open
          title={
            editor.mode === 'create'
              ? t('workflows.editor.createTitle')
              : t('workflows.editor.versionTitle')
          }
          initial={editor.initial}
          submitting={submitting}
          onClose={() => setEditor(undefined)}
          onSubmit={(value) => void save(value)}
        />
      )}
    </Space>
  )
}

function WorkflowDetail({
  workflow,
  version,
  versionID,
  setVersionID,
  submitting,
  onNewVersion,
  onLifecycle,
}: {
  workflow?: ReleaseWorkflow
  version?: ReleaseWorkflowVersion
  versionID?: string
  setVersionID: (id: string) => void
  submitting: boolean
  onNewVersion: () => void
  onLifecycle: (value: 'Published' | 'Disabled') => void
}) {
  const { t } = useTranslation()
  if (!workflow || !version)
    return (
      <Card>
        <Empty description={t('workflows.list.select')} />
      </Card>
    )
  return (
    <Card
      title={workflow.name}
      extra={
        <Space>
          <Select
            value={versionID ?? version.id}
            options={workflow.versions.map((item) => ({
              value: item.id,
              label: `v${item.versionNumber} · ${item.lifecycle}`,
            }))}
            onChange={setVersionID}
          />
          <Button onClick={onNewVersion}>
            {t('workflows.actions.newVersion')}
          </Button>
          {version.lifecycle === 'Draft' && (
            <Button
              type="primary"
              loading={submitting}
              onClick={() => onLifecycle('Published')}
            >
              {t('workflows.actions.publish')}
            </Button>
          )}
          {version.lifecycle === 'Published' && (
            <Button
              danger
              loading={submitting}
              onClick={() => onLifecycle('Disabled')}
            >
              {t('workflows.actions.disable')}
            </Button>
          )}
        </Space>
      }
    >
      <Space orientation="vertical" className={styles.detail}>
        <Tag color={lifecycleColor(version.lifecycle)}>{version.lifecycle}</Tag>
        <WorkflowGraphEditor
          key={version.id}
          initialDocument={version.document}
          readOnly
        />
      </Space>
    </Card>
  )
}

function selectedVersion(workflow?: ReleaseWorkflow, versionID?: string) {
  return (
    workflow?.versions.find((item) => item.id === versionID) ??
    workflow?.versions.at(-1)
  )
}

function latestLock(workflow: ReleaseWorkflow) {
  return workflow.versions.at(-1)?.lockVersion ?? 1
}

function createEditor(t: (key: string) => string): EditorState {
  return {
    mode: 'create',
    initial: {
      name: t('workflows.template.name'),
      description: t('workflows.template.description'),
      document: productionApprovalTemplate(),
    },
  }
}

function versionEditor(
  workflow: ReleaseWorkflow,
  version: ReleaseWorkflowVersion,
): EditorState {
  return {
    mode: 'version',
    workflow,
    initial: {
      name: workflow.name,
      description: workflow.description,
      document: version.document,
    },
  }
}

function lifecycleColor(value: DefinitionLifecycle) {
  return value === 'Published'
    ? 'green'
    : value === 'Disabled'
      ? 'default'
      : 'blue'
}

function browserCookie(name: string): string | undefined {
  return document.cookie
    .split('; ')
    .find((value) => value.startsWith(`${name}=`))
    ?.slice(name.length + 1)
}
