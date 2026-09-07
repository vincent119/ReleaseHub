import { Form, Modal, Select } from 'antd'
import { useTranslation } from 'react-i18next'

import type { Connection } from '@xyflow/react'

interface Props {
  open: boolean
  nodeKeys: string[]
  onClose: () => void
  onSubmit: (value: Connection) => void
}

interface Fields {
  source: string
  target: string
}

export function DependencyModal({ open, nodeKeys, onClose, onSubmit }: Props) {
  const { t } = useTranslation()
  const [form] = Form.useForm<Fields>()
  const options = nodeKeys.map((value) => ({ value, label: value }))
  return (
    <Modal
      open={open}
      title={t('plans.dependency.title')}
      okText={t('plans.dependency.add')}
      cancelText={t('plans.actions.cancel')}
      onCancel={onClose}
      afterClose={() => form.resetFields()}
      onOk={() =>
        void form.validateFields().then(({ source, target }) => {
          onSubmit({ source, target, sourceHandle: null, targetHandle: null })
          onClose()
        })
      }
    >
      <Form form={form} layout="vertical">
        <Form.Item
          name="source"
          label={t('plans.fields.from')}
          rules={[{ required: true }]}
        >
          <Select options={options} />
        </Form.Item>
        <Form.Item
          name="target"
          label={t('plans.fields.to')}
          dependencies={['source']}
          rules={[
            { required: true },
            ({ getFieldValue }) => ({
              validator: (_, value) =>
                value !== getFieldValue('source')
                  ? Promise.resolve()
                  : Promise.reject(
                      new Error(t('plans.validation.selfDependency')),
                    ),
            }),
          ]}
        >
          <Select options={options} />
        </Form.Item>
      </Form>
    </Modal>
  )
}
