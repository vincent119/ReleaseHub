import { useQueryClient } from '@tanstack/react-query'
import { Alert, Button, Form, Input } from 'antd'
import { useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { changeLocalPassword, getGetAuthSessionQueryKey } from '@/generated/api'

import styles from './AccountMenu.module.css'

interface ChangePasswordFormProps {
  csrfToken?: string
  onCancel: () => void
  onSuccess: () => void
  onSubmittingChange: (submitting: boolean) => void
}

interface ChangePasswordFields {
  currentPassword: string
  newPassword: string
  confirmPassword: string
}

export function ChangePasswordForm({
  csrfToken,
  onCancel,
  onSuccess,
  onSubmittingChange,
}: ChangePasswordFormProps) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [form] = Form.useForm<ChangePasswordFields>()
  const submittingRef = useRef(false)
  const [submitting, setSubmitting] = useState(false)
  const [requestFailed, setRequestFailed] = useState(false)

  const setSubmissionState = (nextSubmitting: boolean) => {
    submittingRef.current = nextSubmitting
    setSubmitting(nextSubmitting)
    onSubmittingChange(nextSubmitting)
  }

  const cancel = () => {
    if (submittingRef.current) return
    form.resetFields()
    setRequestFailed(false)
    onCancel()
  }

  const submit = async (values: ChangePasswordFields) => {
    if (submittingRef.current) return
    setSubmissionState(true)
    setRequestFailed(false)
    try {
      if (!csrfToken) throw new Error('CSRF token unavailable')
      const response = await changeLocalPassword(
        {
          currentPassword: values.currentPassword,
          newPassword: values.newPassword,
        },
        { headers: { 'X-CSRF-Token': csrfToken } },
      )
      if (response.status !== 204) throw new Error('password change rejected')

      form.resetFields()
      onSuccess()
      await queryClient.invalidateQueries({
        queryKey: getGetAuthSessionQueryKey(),
      })
    } catch {
      setRequestFailed(true)
    } finally {
      setSubmissionState(false)
    }
  }

  const byteLengthRule = (minimum: number, message: string) => ({
    validator(_: unknown, value?: string) {
      if (!value) return Promise.resolve()
      const length = new TextEncoder().encode(value).length
      if (length >= minimum && length <= 72) return Promise.resolve()
      return Promise.reject(new Error(message))
    },
  })

  return (
    <Form
      form={form}
      layout="vertical"
      className={styles.passwordForm}
      onFinish={(values) => void submit(values)}
    >
      {requestFailed && (
        <Alert
          className={styles.passwordError}
          type="error"
          showIcon
          title={t('account.password.error')}
        />
      )}
      <Form.Item
        name="currentPassword"
        label={t('account.password.current')}
        rules={[
          { required: true },
          byteLengthRule(1, t('account.password.currentLengthError')),
        ]}
      >
        <Input.Password autoFocus autoComplete="current-password" />
      </Form.Item>
      <Form.Item
        name="newPassword"
        label={t('account.password.new')}
        rules={[
          { required: true },
          byteLengthRule(8, t('account.password.lengthError')),
        ]}
      >
        <Input.Password autoComplete="new-password" />
      </Form.Item>
      <Form.Item
        name="confirmPassword"
        label={t('account.password.confirm')}
        dependencies={['newPassword']}
        rules={[
          { required: true },
          ({ getFieldValue }) => ({
            validator(_, value) {
              if (!value || getFieldValue('newPassword') === value)
                return Promise.resolve()
              return Promise.reject(new Error(t('account.password.mismatch')))
            },
          }),
        ]}
      >
        <Input.Password autoComplete="new-password" />
      </Form.Item>
      <div className={styles.passwordActions}>
        <Button disabled={submitting} onClick={cancel}>
          {t('account.password.cancel')}
        </Button>
        <Button
          type="primary"
          htmlType="submit"
          loading={submitting}
          disabled={submitting}
        >
          {t('account.password.submit')}
        </Button>
      </div>
    </Form>
  )
}
