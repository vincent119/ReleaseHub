import { Card, Empty, Typography } from 'antd'
import { useTranslation } from 'react-i18next'

import { useListDeploymentHistory } from '@/generated/api'
import { SemanticList, SemanticListItemContent } from '@/shared/list'
import { SemanticTag } from '@/shared/tag/SemanticTag'

interface Props {
  projectId: string
  environmentId: string
  enabled: boolean
}

export function DeploymentHistoryPanel({
  projectId,
  environmentId,
  enabled,
}: Props) {
  const { t } = useTranslation()
  const history = useListDeploymentHistory(
    { projectId, environmentId, limit: 20 },
    { query: { enabled } },
  )
  const values = history.data?.status === 200 ? history.data.data.data : []
  return (
    <Card
      title={t('requestDetail.history.title')}
      loading={history.isPending && enabled}
    >
      {history.isError || (history.data && history.data.status !== 200) ? (
        <Typography.Text type="secondary">
          {t('requestDetail.history.unavailable')}
        </Typography.Text>
      ) : values.length ? (
        <SemanticList
          items={values}
          rowKey="requestVersionId"
          renderItem={(item) => (
            <SemanticListItemContent
              extra={<SemanticTag>{item.classification}</SemanticTag>}
              title={new Date(item.completedAt).toLocaleString()}
              description={t('requestDetail.history.applicationCount', {
                count: item.applications.length,
              })}
            />
          )}
        />
      ) : (
        <Empty
          image={Empty.PRESENTED_IMAGE_SIMPLE}
          description={t('requestDetail.history.empty')}
        />
      )}
    </Card>
  )
}
