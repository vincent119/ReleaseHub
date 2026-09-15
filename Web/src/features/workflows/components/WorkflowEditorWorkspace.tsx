import { ArrowLeftOutlined } from '@ant-design/icons'
import { Alert, Button, Flex, Form, Input, Typography } from 'antd'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { useGetReleaseWorkflowReviewOptions } from '@/generated/api'
import type { ReleaseWorkflowDocument } from '@/generated/model'

import { WorkflowGraphEditor } from './WorkflowGraphEditor'
import styles from './WorkflowEditorWorkspace.module.css'

export interface WorkflowEditorValue {
  name: string
  description: string
  document: ReleaseWorkflowDocument
}

interface Props {
  mode: 'create' | 'copy' | 'version'
  title: string
  initial: WorkflowEditorValue
  submitting: boolean
  onClose: () => void
  onSubmit: (value: WorkflowEditorValue) => void
}

export function WorkflowEditorWorkspace({
  mode,
  title,
  initial,
  submitting,
  onClose,
  onSubmit,
}: Props) {
  const { t } = useTranslation()
  const [form] =
    Form.useForm<Pick<WorkflowEditorValue, 'name' | 'description'>>()
  const [document, setDocument] = useState(initial.document)
  const reviewOptionsQuery = useGetReleaseWorkflowReviewOptions()
  const reviewOptions =
    reviewOptionsQuery.data?.status === 200
      ? reviewOptionsQuery.data.data.data
      : undefined
  const reviewOptionsError =
    reviewOptionsQuery.isError ||
    Boolean(reviewOptionsQuery.data && reviewOptionsQuery.data.status !== 200)
  const validation = useMemo(() => validateDocument(document, t), [document, t])
  const submit = async () => {
    const fields = await form.validateFields()
    if (!validation) onSubmit({ ...fields, document })
  }

  return (
    <section
      className={styles.workspace}
      aria-labelledby="workflow-editor-title"
    >
      <header className={styles.header}>
        <div className={styles.headingGroup}>
          <Button
            type="text"
            icon={<ArrowLeftOutlined />}
            className={styles.back}
            aria-label={t('workflows.editor.back')}
            onClick={onClose}
          >
            {t('workflows.editor.back')}
          </Button>
          <div className={styles.headingCopy}>
            <Typography.Text className={styles.eyebrow}>
              {t(`workflows.editor.${mode}Eyebrow`)}
            </Typography.Text>
            <Typography.Title
              id="workflow-editor-title"
              level={2}
              className={styles.title}
            >
              {title}
            </Typography.Title>
            <Typography.Paragraph
              type="secondary"
              className={styles.description}
            >
              {t(`workflows.editor.${mode}Description`)}
            </Typography.Paragraph>
          </div>
        </div>
        <Flex className={styles.actions} gap="small" wrap>
          <Button onClick={onClose}>{t('workflows.actions.cancel')}</Button>
          <Button
            type="primary"
            loading={submitting}
            disabled={Boolean(validation) || submitting}
            onClick={() => void submit()}
          >
            {t('workflows.actions.save')}
          </Button>
        </Flex>
      </header>

      <div className={styles.body}>
        <section
          className={styles.metadata}
          aria-labelledby="workflow-editor-details-title"
        >
          <Typography.Title
            id="workflow-editor-details-title"
            level={4}
            className={styles.metadataTitle}
          >
            {t('workflows.editor.detailsTitle')}
          </Typography.Title>
          <Form
            form={form}
            layout="vertical"
            className={styles.form}
            initialValues={{
              name: initial.name,
              description: initial.description,
            }}
          >
            <Form.Item
              name="name"
              label={t('workflows.fields.workflowName')}
              rules={[
                { required: true, message: t('workflows.validation.name') },
              ]}
            >
              <Input maxLength={128} />
            </Form.Item>
            <Form.Item
              name="description"
              label={t('workflows.fields.description')}
            >
              <Input.TextArea
                maxLength={4096}
                autoSize={{ minRows: 2, maxRows: 5 }}
              />
            </Form.Item>
          </Form>
        </section>

        {validation && (
          <Alert
            className={styles.validation}
            type="warning"
            showIcon
            title={validation}
          />
        )}

        <section
          className={styles.graphRegion}
          aria-label={t('workflows.editor.workspace')}
        >
          <WorkflowGraphEditor
            initialDocument={initial.document}
            reviewOptions={reviewOptions}
            reviewOptionsLoading={reviewOptionsQuery.isPending}
            reviewOptionsError={reviewOptionsError}
            onChange={setDocument}
          />
        </section>
      </div>
    </section>
  )
}

function validateDocument(
  document: ReleaseWorkflowDocument,
  t: (key: string) => string,
): string | undefined {
  if (
    !document.initialState ||
    !document.states.some((state) => state.key === document.initialState)
  )
    return t('workflows.validation.initialState')
  const keys = document.states.map((state) => state.key)
  if (new Set(keys).size !== keys.length)
    return t('workflows.validation.duplicateState')
  const review = document.states.find(
    (state) =>
      state.type === 'Review' &&
      !state.reviewPolicy?.userIds.length &&
      !state.reviewPolicy?.roleIds.length,
  )
  if (review) return t('workflows.validation.reviewAssignment')
  return undefined
}
