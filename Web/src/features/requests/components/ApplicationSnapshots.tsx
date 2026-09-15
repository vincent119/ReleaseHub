import { Card, Collapse, Descriptions, Space, Tag, Typography } from 'antd'
import { useTranslation } from 'react-i18next'

import type { DeploymentRequestApplicationSnapshot } from '@/generated/model'
import { SemanticList, SemanticListItemContent } from '@/shared/list'

import styles from '../RequestDetailPage.module.css'

export function ApplicationSnapshots({
  applications,
}: {
  applications: DeploymentRequestApplicationSnapshot[]
}) {
  const { t } = useTranslation()
  return (
    <Card
      title={t('requestDetail.applications.title')}
      className={styles.wideCard}
    >
      <Collapse
        destroyOnHidden
        items={applications.map((application) => ({
          key: application.id,
          label: `${application.order + 1}. ${application.applicationKey}`,
          extra: <Tag>{application.targetRevision}</Tag>,
          children: <ApplicationSnapshot value={application} />,
        }))}
      />
    </Card>
  )
}

function ApplicationSnapshot({
  value,
}: {
  value: DeploymentRequestApplicationSnapshot
}) {
  const { t } = useTranslation()
  return (
    <Space orientation="vertical" size="middle" style={{ width: '100%' }}>
      <Descriptions column={{ xs: 1, md: 2 }} size="small">
        <Descriptions.Item label={t('requestDetail.fields.liveRevision')}>
          {value.liveRevision}
        </Descriptions.Item>
        <Descriptions.Item label={t('requestDetail.fields.targetRevision')}>
          {value.targetRevision}
        </Descriptions.Item>
        <Descriptions.Item label={t('requestDetail.fields.manifestHash')}>
          {value.manifestHash}
        </Descriptions.Item>
        <Descriptions.Item label={t('requestDetail.fields.diffHash')}>
          {value.diffHash}
        </Descriptions.Item>
      </Descriptions>
      <SemanticList
        items={value.images}
        rowKey={(image, index) => `${image.digest}-${index}`}
        compact
        header={
          <Typography.Text strong>
            {t('requestDetail.images.title')}
          </Typography.Text>
        }
        renderItem={(image) => (
          <SemanticListItemContent
            title={`${image.repository}:${image.tag}`}
            description={image.digest}
          />
        )}
      />
      <Collapse
        destroyOnHidden
        items={[
          {
            key: 'diff',
            label: t('requestDetail.diff.show'),
            children: (
              <code className={styles.code}>
                {JSON.stringify(value.diff, null, 2)}
              </code>
            ),
          },
        ]}
      />
    </Space>
  )
}
