import {
  ApartmentOutlined,
  CopyOutlined,
  DeleteOutlined,
  PlusOutlined,
} from '@ant-design/icons'
import {
  Alert,
  Button,
  Card,
  Empty,
  Flex,
  Input,
  List,
  Modal,
  Select,
  Space,
  Tag,
  Tooltip,
  Typography,
} from 'antd'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import {
  changeReleaseWorkflowVersionLifecycle,
  createReleaseWorkflow,
  createReleaseWorkflowVersion,
  deleteReleaseWorkflow,
  useListReleaseWorkflows,
} from '@/generated/api'
import type {
  DefinitionLifecycle,
  ReleaseWorkflow,
  ReleaseWorkflowVersion,
} from '@/generated/model'
import { useFeedback } from '@/shared/feedback/useFeedback'
import definitionStyles from '@/shared/definition/DefinitionWorkspace.module.css'

import {
  WorkflowEditorWorkspace,
  type WorkflowEditorValue,
} from './components/WorkflowEditorWorkspace'
import { WorkflowGraphEditor } from './components/WorkflowGraphEditor'
import { productionApprovalTemplate } from './model/workflowGraph'
import styles from './WorkflowsPage.module.css'

interface EditorState {
  mode: 'create' | 'copy' | 'version'
  workflow?: ReleaseWorkflow
  initial: WorkflowEditorValue
}

interface WorkflowMutationResponse {
  status: number
  data: unknown
}

type WorkflowConflictContext = 'create' | 'version'

export function WorkflowsPage() {
  const { t } = useTranslation()
  const feedback = useFeedback()
  const query = useListReleaseWorkflows()
  const [workflowID, setWorkflowID] = useState<string | null>()
  const [versionID, setVersionID] = useState<string>()
  const [editor, setEditor] = useState<EditorState>()
  const [deleteOpen, setDeleteOpen] = useState(false)
  const [deleteConfirmation, setDeleteConfirmation] = useState('')
  const [submitting, setSubmitting] = useState(false)
  const workflows = query.data?.status === 200 ? query.data.data.data : []
  const workflow =
    workflowID === null
      ? undefined
      : (workflows.find((item) => item.id === workflowID) ?? workflows[0])
  const version = selectedVersion(workflow, versionID)
  const mutate = async (
    operation: (options: RequestInit) => Promise<WorkflowMutationResponse>,
    conflictContext?: WorkflowConflictContext,
  ) => {
    const csrf = browserCookie('releasehub_csrf')
    if (!csrf) return void feedback.error(t('workflows.mutation.error'))
    setSubmitting(true)
    try {
      const response = await operation({ headers: { 'X-CSRF-Token': csrf } })
      if (
        response.status === 409 &&
        conflictContext === 'create' &&
        responseErrorCode(response.data) === 'WORKFLOW_NAME_CONFLICT'
      )
        return void feedback.error(t('workflows.mutation.nameConflict'))
      if (response.status === 409 && conflictContext === 'version')
        return void feedback.error(t('workflows.mutation.versionConflict'))
      if (response.status < 200 || response.status >= 300)
        return void feedback.error(t('workflows.mutation.rejected'))
      setEditor(undefined)
      feedback.success(t('workflows.mutation.saved'))
      await query.refetch()
    } catch {
      feedback.error(t('workflows.mutation.error'))
    } finally {
      setSubmitting(false)
    }
  }
  const save = (value: WorkflowEditorValue) =>
    editor?.mode !== 'version'
      ? mutate((options) => createReleaseWorkflow(value, options), 'create')
      : mutate(
          (options) =>
            createReleaseWorkflowVersion(
              editor!.workflow!.id,
              {
                expectedVersion: latestVersionNumber(editor!.workflow!),
                document: value.document,
              },
              options,
            ),
          'version',
        )
  const removeWorkflow = async () => {
    if (!workflow) return
    const csrf = browserCookie('releasehub_csrf')
    if (!csrf) return void feedback.error(t('workflows.delete.error'))
    setSubmitting(true)
    try {
      const response = await deleteReleaseWorkflow(
        workflow.id,
        { expectedVersion: latestVersionNumber(workflow) },
        { headers: { 'X-CSRF-Token': csrf } },
      )
      if (response.status === 409)
        return void feedback.error(t('workflows.delete.conflict'))
      if (response.status !== 204)
        return void feedback.error(t('workflows.delete.error'))
      setWorkflowID(null)
      setVersionID(undefined)
      setDeleteOpen(false)
      setDeleteConfirmation('')
      feedback.success(t('workflows.delete.success'))
      await query.refetch()
    } catch {
      feedback.error(t('workflows.delete.error'))
    } finally {
      setSubmitting(false)
    }
  }
  if (query.isError || (query.data && query.data.status !== 200))
    return (
      <Alert
        type="warning"
        showIcon
        message={t('workflows.unavailable.title')}
        description={t('workflows.unavailable.description')}
      />
    )
  if (editor)
    return (
      <WorkflowEditorWorkspace
        mode={editor.mode}
        title={
          editor.mode === 'create'
            ? t('workflows.editor.createTitle')
            : editor.mode === 'copy'
              ? t('workflows.editor.copyTitle')
              : t('workflows.editor.versionTitle')
        }
        initial={editor.initial}
        submitting={submitting}
        onClose={() => setEditor(undefined)}
        onSubmit={(value) => void save(value)}
      />
    )
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
      <div className={definitionStyles.workspace}>
        <Card
          className={definitionStyles.listCard}
          title={t('workflows.list.title')}
          loading={query.isPending}
        >
          {workflows.length === 0 ? (
            <Empty description={t('workflows.list.empty')} />
          ) : (
            <List
              dataSource={workflows}
              renderItem={(item) => (
                <List.Item
                  className={`${definitionStyles.listItem} ${item.id === workflow?.id ? definitionStyles.selected : ''}`}
                  role="button"
                  tabIndex={0}
                  aria-current={item.id === workflow?.id ? 'page' : undefined}
                  onClick={() => {
                    setWorkflowID(item.id)
                    setVersionID(undefined)
                  }}
                  onKeyDown={(event) => {
                    if (event.key !== 'Enter' && event.key !== ' ') return
                    event.preventDefault()
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
          onCopy={() =>
            workflow &&
            version &&
            setEditor(
              copyEditor(
                workflow,
                version,
                t('workflows.copy.defaultName', { name: workflow.name }),
              ),
            )
          }
          onDelete={() => {
            setDeleteConfirmation('')
            setDeleteOpen(true)
          }}
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
      <Modal
        open={deleteOpen && Boolean(workflow)}
        title={t('workflows.delete.title')}
        okText={t('workflows.delete.confirm')}
        cancelText={t('workflows.actions.cancel')}
        okButtonProps={{
          danger: true,
          disabled: deleteConfirmation !== workflow?.name,
        }}
        confirmLoading={submitting}
        destroyOnHidden
        onCancel={() => {
          setDeleteOpen(false)
          setDeleteConfirmation('')
        }}
        onOk={() => void removeWorkflow()}
      >
        <Space orientation="vertical" size="middle" className={styles.detail}>
          <Alert
            type="warning"
            showIcon
            title={t('workflows.delete.warning')}
            description={t('workflows.delete.impact')}
          />
          <Typography.Text>
            {t('workflows.delete.instruction', { name: workflow?.name })}
          </Typography.Text>
          <Input
            value={deleteConfirmation}
            aria-label={t('workflows.delete.confirmationLabel')}
            placeholder={workflow?.name}
            onChange={(event) => setDeleteConfirmation(event.target.value)}
          />
        </Space>
      </Modal>
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
  onCopy,
  onDelete,
  onLifecycle,
}: {
  workflow?: ReleaseWorkflow
  version?: ReleaseWorkflowVersion
  versionID?: string
  setVersionID: (id: string) => void
  submitting: boolean
  onNewVersion: () => void
  onCopy: () => void
  onDelete: () => void
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
      className={definitionStyles.detailCard}
      title={
        <div className={definitionStyles.detailHeading}>
          <span className={definitionStyles.detailTitle}>{workflow.name}</span>
          <span className={definitionStyles.detailMeta}>
            <Tag color={lifecycleColor(version.lifecycle)}>
              {version.lifecycle}
            </Tag>
            <span>v{version.versionNumber}</span>
          </span>
        </div>
      }
      extra={
        <Flex className={definitionStyles.actions}>
          <div className={definitionStyles.actionGroup}>
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
            <Button icon={<CopyOutlined />} onClick={onCopy}>
              {t('workflows.actions.copy')}
            </Button>
          </div>
          <div className={definitionStyles.actionGroup}>
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
          </div>
          <div className={definitionStyles.dangerGroup}>
            <Tooltip
              title={
                isDraftOnly(workflow)
                  ? undefined
                  : t('workflows.delete.publishedReason')
              }
            >
              <span>
                <Button
                  danger
                  icon={<DeleteOutlined />}
                  disabled={!isDraftOnly(workflow)}
                  loading={submitting}
                  onClick={onDelete}
                >
                  {t('workflows.actions.delete')}
                </Button>
              </span>
            </Tooltip>
          </div>
        </Flex>
      }
    >
      <div className={definitionStyles.detail}>
        <WorkflowGraphEditor
          key={version.id}
          initialDocument={version.document}
          readOnly
        />
      </div>
    </Card>
  )
}

function selectedVersion(workflow?: ReleaseWorkflow, versionID?: string) {
  return (
    workflow?.versions.find((item) => item.id === versionID) ??
    workflow?.versions.at(-1)
  )
}

function latestVersionNumber(workflow: ReleaseWorkflow) {
  return workflow.versions.at(-1)?.versionNumber ?? 1
}

function responseErrorCode(data: unknown) {
  if (typeof data !== 'object' || data === null || !('code' in data))
    return undefined
  return typeof data.code === 'string' ? data.code : undefined
}

function isDraftOnly(workflow: ReleaseWorkflow) {
  return (
    workflow.versions.length > 0 &&
    workflow.versions.every((item) => item.lifecycle === 'Draft')
  )
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

function copyEditor(
  workflow: ReleaseWorkflow,
  version: ReleaseWorkflowVersion,
  name: string,
): EditorState {
  return {
    mode: 'copy',
    initial: {
      name,
      description: workflow.description,
      document: structuredClone(version.document),
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
