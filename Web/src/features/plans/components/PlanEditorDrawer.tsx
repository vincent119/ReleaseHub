import { Alert, Button, Drawer, Form, Input, Radio, Space } from 'antd'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import type {
  DeploymentPlanDocument,
  DeploymentPlanOwnerKind,
} from '@/generated/model'

import { PlanGraphEditor } from './PlanGraphEditor'

export interface PlanEditorValue {
  ownerKind: DeploymentPlanOwnerKind
  name: string
  description: string
  document: DeploymentPlanDocument
}

interface Props {
  open: boolean
  title: string
  initial: PlanEditorValue
  creating: boolean
  submitting: boolean
  onClose: () => void
  onSubmit: (value: PlanEditorValue) => void
}

export function PlanEditorDrawer(props: Props) {
  const { t } = useTranslation()
  const [form] = Form.useForm<Omit<PlanEditorValue, 'document'>>()
  const [document, setDocument] = useState(props.initial.document)
  const validation = useMemo(() => validatePlan(document, t), [document, t])
  const submit = async () => {
    const fields = await form.validateFields()
    if (!validation) props.onSubmit({ ...fields, document })
  }
  return (
    <Drawer
      open={props.open}
      width="min(96vw, 92rem)"
      title={props.title}
      destroyOnHidden
      onClose={props.onClose}
      extra={
        <Space>
          <Button onClick={props.onClose}>{t('plans.actions.cancel')}</Button>
          <Button
            type="primary"
            loading={props.submitting}
            disabled={Boolean(validation)}
            onClick={() => void submit()}
          >
            {t('plans.actions.save')}
          </Button>
        </Space>
      }
    >
      <Form form={form} layout="vertical" initialValues={props.initial}>
        {props.creating && (
          <Form.Item name="ownerKind" label={t('plans.fields.ownerKind')}>
            <Radio.Group
              options={[
                { value: 'project', label: t('plans.owner.project') },
                { value: 'platform', label: t('plans.owner.platform') },
              ]}
            />
          </Form.Item>
        )}
        <Form.Item
          name="name"
          label={t('plans.fields.name')}
          rules={[{ required: true, message: t('plans.validation.name') }]}
        >
          <Input maxLength={128} disabled={!props.creating} />
        </Form.Item>
        <Form.Item name="description" label={t('plans.fields.description')}>
          <Input.TextArea
            maxLength={4096}
            autoSize={{ minRows: 2, maxRows: 5 }}
            disabled={!props.creating}
          />
        </Form.Item>
      </Form>
      {validation && <Alert type="error" showIcon message={validation} />}
      <PlanGraphEditor
        initialDocument={props.initial.document}
        onChange={setDocument}
      />
    </Drawer>
  )
}

function validatePlan(
  document: DeploymentPlanDocument,
  t: (key: string) => string,
) {
  if (document.nodes.length === 0) return t('plans.validation.nodeRequired')
  if (
    new Set(document.nodes.map((node) => node.key)).size !==
    document.nodes.length
  )
    return t('plans.validation.duplicateNode')
  if (
    new Set(document.nodes.map((node) => node.applicationKey)).size !==
    document.nodes.length
  )
    return t('plans.validation.duplicateApplication')
  return undefined
}
