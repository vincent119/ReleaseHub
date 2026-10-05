import { ReloadOutlined } from '@ant-design/icons'
import { Alert, Button, Card, Empty, Table, Typography } from 'antd'
import { useTranslation } from 'react-i18next'

import type { DeploymentRequestSummary } from '@/generated/model'

import type { RequestListCriteria } from '../model/listQuery'
import { useRequestListPage } from '../model/useRequestListPage'
import { requestColumns } from '../model/requestColumns'
import type { RequestScope } from './RequestScopeSelector'
import styles from '../RequestsPage.module.css'

export function RequestList({
  scope,
  criteria,
}: {
  scope: RequestScope | undefined
  criteria: RequestListCriteria
}) {
  const { t } = useTranslation()
  const { requests, position, dispatch } = useRequestListPage(scope, criteria)
  if (!scope) return <Empty description={t('requests.scope.empty')} />
  const busy =
    requests.isPending || requests.isFetching || requests.isPlaceholderData
  const response =
    requests.data?.status === 200 ? requests.data.data : undefined
  const values = response?.data ?? []
  const meta = response?.meta
  const failed =
    requests.isError || (requests.data && requests.data.status !== 200)
  const invalid = requests.data?.status === 400
  const badPage =
    !busy &&
    !failed &&
    (!meta ||
      typeof meta.hasMore !== 'boolean' ||
      (meta.hasMore &&
        (typeof meta.nextCursor !== 'string' ||
          !meta.nextCursor ||
          position.cursors
            .slice(0, position.index + 1)
            .includes(meta.nextCursor))))
  const nextCursor =
    !busy && !failed && !badPage && meta?.hasMore ? meta.nextCursor : undefined
  const filtered = Boolean(criteria.search || criteria.status)
  const refresh = (updated: boolean) => dispatch({ type: 'refresh', updated })
  return (
    <Card className={styles.listCard}>
      <div className={styles.listHeading}>
        <Typography.Text type="secondary">
          {t('requests.query.order')}
        </Typography.Text>
        <Button
          aria-label={t('requests.query.refresh')}
          icon={<ReloadOutlined />}
          loading={busy}
          onClick={() => refresh(true)}
        >
          {t('requests.query.refresh')}
        </Button>
      </div>
      {position.updated && (
        <Alert type="info" showIcon title={t('requests.query.updated')} />
      )}
      {failed || badPage ? (
        <Alert
          type="error"
          showIcon
          title={t(
            invalid
              ? 'requests.query.invalid'
              : badPage
                ? 'requests.query.badPage'
                : 'requests.unavailable',
          )}
          action={
            <Button onClick={() => refresh(false)}>
              {t('requests.query.firstPage')}
            </Button>
          }
        />
      ) : busy ? (
        <div role="status" aria-live="polite">
          <Card loading />
          <Typography.Text>{t('requests.query.loading')}</Typography.Text>
        </div>
      ) : (
        <Table<DeploymentRequestSummary>
          rowKey="id"
          columns={requestColumns(t)}
          dataSource={values}
          pagination={false}
          size="small"
          tableLayout="fixed"
          locale={{
            emptyText: t(
              position.index > 0
                ? 'requests.query.emptyPage'
                : filtered
                  ? 'requests.query.noResults'
                  : 'requests.empty',
            ),
          }}
          scroll={{ x: 1080 }}
        />
      )}
      <nav
        className={styles.pagination}
        aria-label={t('requests.query.pagination')}
      >
        <Typography.Text role="status" aria-live="polite">
          {!busy &&
            !failed &&
            !badPage &&
            t('requests.query.page', {
              number: position.index + 1,
              count: values.length,
            })}
        </Typography.Text>
        <div className={styles.pageActions}>
          <Button
            disabled={busy || position.index === 0}
            title={
              position.index === 0 ? t('requests.query.atFirst') : undefined
            }
            onClick={() => dispatch({ type: 'previous' })}
          >
            {t('requests.query.previous')}
          </Button>
          <Button
            disabled={!nextCursor}
            title={!nextCursor ? t('requests.query.noNext') : undefined}
            onClick={() =>
              nextCursor && dispatch({ type: 'next', cursor: nextCursor })
            }
          >
            {t('requests.query.next')}
          </Button>
        </div>
      </nav>
    </Card>
  )
}
