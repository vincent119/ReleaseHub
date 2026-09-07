import { Alert, Button, Drawer, Form, Input, Space } from 'antd'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import type { ReleaseWorkflowDocument } from '@/generated/model'

import { WorkflowGraphEditor } from './WorkflowGraphEditor'

export interface WorkflowEditorValue {
  name: string
  description: string
  document: ReleaseWorkflowDocument
}

interface Props {
  open: boolean
  title: string
  initial: WorkflowEditorValue
  submitting: boolean
  onClose: () => void
  onSubmit: (value: WorkflowEditorValue) => void
}

export function WorkflowEditorDrawer({
  open,
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
  const validation = useMemo(() => validateDocument(document, t), [document, t])
  const submit = async () => {
    const fields = await form.validateFields()
    if (!validation) onSubmit({ ...fields, document })
  }
  return (
    <Drawer
      open={open}
      width="min(96vw, 92rem)"
      title={title}
      destroyOnHidden
      onClose={onClose}
      extra={
        <Space>
          <Button onClick={onClose}>{t('workflows.actions.cancel')}</Button>
          <Button
            type="primary"
            loading={submitting}
            disabled={Boolean(validation)}
            onClick={() => void submit()}
          >
            {t('workflows.actions.save')}
          </Button>
        </Space>
      }
    >
      <Form
        form={form}
        layout="vertical"
        initialValues={{ name: initial.name, description: initial.description }}
      >
        <Form.Item
          name="name"
          label={t('workflows.fields.workflowName')}
          rules={[{ required: true, message: t('workflows.validation.name') }]}
        >
          <Input maxLength={128} />
        </Form.Item>
        <Form.Item name="description" label={t('workflows.fields.description')}>
          <Input.TextArea
            maxLength={4096}
            autoSize={{ minRows: 2, maxRows: 5 }}
          />
        </Form.Item>
      </Form>
      {validation && <Alert type="warning" showIcon message={validation} />}
      <WorkflowGraphEditor
        initialDocument={initial.document}
        onChange={setDocument}
      />
    </Drawer>
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
