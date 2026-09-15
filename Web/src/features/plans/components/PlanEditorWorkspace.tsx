import { ArrowLeftOutlined } from '@ant-design/icons'
import { Alert, Button, Flex, Form, Input, Radio, Typography } from 'antd'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import type {
  DeploymentPlanDocument,
  DeploymentPlanOwnerKind,
} from '@/generated/model'

import { PlanGraphEditor } from './PlanGraphEditor'
import styles from './PlanEditorWorkspace.module.css'

export interface PlanEditorValue {
  ownerKind: DeploymentPlanOwnerKind
  name: string
  description: string
  document: DeploymentPlanDocument
}

interface Props {
  title: string
  initial: PlanEditorValue
  creating: boolean
  submitting: boolean
  onClose: () => void
  onSubmit: (value: PlanEditorValue) => void
}

export function PlanEditorWorkspace(props: Props) {
  const { t } = useTranslation()
  const [form] = Form.useForm<Omit<PlanEditorValue, 'document'>>()
  const [document, setDocument] = useState(props.initial.document)
  const validation = useMemo(() => validatePlan(document, t), [document, t])
  const submit = async () => {
    const fields = await form.validateFields()
    if (!validation) props.onSubmit({ ...fields, document })
  }

  return (
    <section className={styles.workspace} aria-labelledby="plan-editor-title">
      <header className={styles.header}>
        <div className={styles.headingGroup}>
          <Button
            type="text"
            icon={<ArrowLeftOutlined />}
            className={styles.back}
            aria-label={t('plans.editor.back')}
            onClick={props.onClose}
          >
            {t('plans.editor.back')}
          </Button>
          <div className={styles.headingCopy}>
            <Typography.Text className={styles.eyebrow}>
              {t('plans.editor.eyebrow')}
            </Typography.Text>
            <Typography.Title
              id="plan-editor-title"
              level={2}
              className={styles.title}
            >
              {props.title}
            </Typography.Title>
            <Typography.Paragraph
              type="secondary"
              className={styles.description}
            >
              {t(
                props.creating
                  ? 'plans.editor.createDescription'
                  : 'plans.editor.versionDescription',
              )}
            </Typography.Paragraph>
          </div>
        </div>
        <Flex className={styles.actions} gap="small" wrap>
          <Button onClick={props.onClose}>{t('plans.actions.cancel')}</Button>
          <Button
            type="primary"
            loading={props.submitting}
            disabled={Boolean(validation)}
            onClick={() => void submit()}
          >
            {t('plans.actions.save')}
          </Button>
        </Flex>
      </header>

      <div className={styles.body}>
        <section
          className={styles.metadata}
          aria-labelledby="plan-editor-details-title"
        >
          <Typography.Title
            id="plan-editor-details-title"
            level={4}
            className={styles.metadataTitle}
          >
            {t('plans.editor.detailsTitle')}
          </Typography.Title>
          <Form
            form={form}
            layout="vertical"
            initialValues={props.initial}
            className={styles.form}
          >
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
            <Form.Item
              name="description"
              label={t('plans.fields.description')}
              className={styles.descriptionField}
            >
              <Input.TextArea
                maxLength={4096}
                autoSize={{ minRows: 2, maxRows: 5 }}
                disabled={!props.creating}
              />
            </Form.Item>
          </Form>
        </section>

        {validation && <Alert type="error" showIcon title={validation} />}

        <section
          className={styles.graphRegion}
          aria-label={t('plans.editor.workspace')}
        >
          <PlanGraphEditor
            initialDocument={props.initial.document}
            onChange={setDocument}
          />
        </section>
      </div>
    </section>
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
