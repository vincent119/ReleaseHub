import { Form, Input, Modal, Select } from 'antd'
import { useTranslation } from 'react-i18next'

export interface ReviewReassignmentValue {
  userIds: string[]
  roleIds: string[]
  reason: string
}

interface Props {
  open: boolean
  submitting: boolean
  onCancel: () => void
  onSubmit: (value: ReviewReassignmentValue) => void
}

export function ReviewReassignmentModal({
  open,
  submitting,
  onCancel,
  onSubmit,
}: Props) {
  const { t } = useTranslation()
  const [form] = Form.useForm<ReviewReassignmentValue>()
  const submit = async () => {
    try {
      const value = await form.validateFields()
      if (value.userIds.length === 0 && value.roleIds.length === 0) {
        form.setFields([
          {
            name: 'userIds',
            errors: [t('workflows.validation.reassignmentTarget')],
          },
        ])
        return
      }
      onSubmit(value)
    } catch {
      return
    }
  }
  return (
    <Modal
      open={open}
      title={t('workflows.review.reassignTitle')}
      confirmLoading={submitting}
      okText={t('workflows.review.confirmReassign')}
      cancelText={t('workflows.actions.cancel')}
      onCancel={onCancel}
      onOk={() => void submit()}
      afterOpenChange={(visible) => !visible && form.resetFields()}
    >
      <Form
        form={form}
        layout="vertical"
        initialValues={{ userIds: [], roleIds: [], reason: '' }}
      >
        <Form.Item name="userIds" label={t('workflows.fields.userIds')}>
          <Select mode="tags" tokenSeparators={[',']} />
        </Form.Item>
        <Form.Item name="roleIds" label={t('workflows.fields.roleIds')}>
          <Select mode="tags" tokenSeparators={[',']} />
        </Form.Item>
        <Form.Item
          name="reason"
          label={t('workflows.review.reassignmentReason')}
          rules={[
            {
              required: true,
              whitespace: true,
              message: t('workflows.validation.reassignmentReason'),
            },
          ]}
        >
          <Input.TextArea maxLength={10000} />
        </Form.Item>
      </Form>
    </Modal>
  )
}
