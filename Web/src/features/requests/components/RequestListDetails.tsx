import { CopyOutlined, InfoCircleOutlined } from '@ant-design/icons'
import { Button, Descriptions, Modal } from 'antd'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import type { DeploymentRequestSummary } from '@/generated/model'

import styles from '../RequestsPage.module.css'
import { shortRequestID } from '../model/presentation'

const historicalStatuses = new Set([
  'Succeeded',
  'Failed',
  'PartialFailed',
  'Superseded',
  'Terminated',
])

export function RequestListDetails({
  request,
  kind,
}: {
  request: DeploymentRequestSummary
  kind: 'identity' | 'schedule'
}) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  const [copying, setCopying] = useState(false)
  const [copyResult, setCopyResult] = useState<'success' | 'error'>()
  const historical = historicalStatuses.has(request.status)
  const close = () => setOpen(false)
  const copy = async () => {
    setCopying(true)
    setCopyResult(undefined)
    try {
      await navigator.clipboard.writeText(request.id)
      setCopyResult('success')
    } catch {
      setCopyResult('error')
    } finally {
      setCopying(false)
    }
  }
  const scheduleLabel = historical
    ? t('requests.schedule.view')
    : t(`requests.schedule.states.${request.scheduleState}`, {
        defaultValue: request.scheduleState,
      })

  return (
    <>
      <Button
        type="text"
        className={styles.detailsButton}
        icon={kind === 'identity' ? <InfoCircleOutlined /> : undefined}
        aria-label={t(`requests.details.${kind}`, {
          id: shortRequestID(request.id),
        })}
        onClick={() => {
          setCopyResult(undefined)
          setOpen(true)
        }}
      >
        {kind === 'schedule' ? scheduleLabel : undefined}
      </Button>
      <Modal
        open={open}
        title={t('requests.details.title')}
        onCancel={close}
        footer={
          <Button className={styles.detailsButton} onClick={close}>
            {t('requests.details.close')}
          </Button>
        }
        width={640}
        className={styles.detailsModal}
      >
        <Descriptions
          column={1}
          size="small"
          items={[
            {
              key: 'title',
              label: t('requests.columns.title'),
              children: (
                <span className={styles.fullValue}>{request.title}</span>
              ),
            },
            {
              key: 'id',
              label: t('requests.details.id'),
              children: (
                <div className={styles.idDetails}>
                  <code className={styles.fullValue}>{request.id}</code>
                  <Button
                    className={styles.detailsButton}
                    aria-label={t('requests.details.copy')}
                    icon={<CopyOutlined />}
                    loading={copying}
                    onClick={() => void copy()}
                  >
                    {t('requests.details.copy')}
                  </Button>
                  {copyResult && (
                    <span
                      role={copyResult === 'error' ? 'alert' : 'status'}
                      className={styles.subtle}
                    >
                      {t(`requests.details.copy${copyResult}`)}
                    </span>
                  )}
                </div>
              ),
            },
          ]}
        />
        <p className={styles.scheduleNotice}>{t('requests.schedule.notice')}</p>
        <Descriptions
          column={1}
          size="small"
          items={[
            {
              key: 'state',
              label: t('requestDetail.fields.scheduleState'),
              children: t(`requests.schedule.states.${request.scheduleState}`, {
                defaultValue: request.scheduleState,
              }),
            },
            {
              key: 'reason',
              label: t('requests.schedule.reason'),
              children: t(
                `requestDetail.schedule.reasons.${request.scheduleReason}`,
                {
                  defaultValue: request.scheduleReason,
                },
              ),
            },
            ...(request.scheduledFor
              ? [
                  {
                    key: 'requested',
                    label: t('requests.schedule.requested'),
                    children: new Date(request.scheduledFor).toLocaleString(),
                  },
                ]
              : []),
            {
              key: 'next',
              label: t('requests.schedule.next'),
              children: new Date(request.nextEligibleAt).toLocaleString(),
            },
          ]}
        />
      </Modal>
    </>
  )
}
