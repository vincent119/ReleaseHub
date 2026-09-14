import { useInfiniteQuery } from '@tanstack/react-query'
import {
  Button,
  Empty,
  Form,
  Modal,
  Popconfirm,
  Select,
  Space,
  Table,
  Tag,
  Typography,
} from 'antd'
import { useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import {
  createAccessMembership,
  listAccessMembershipCandidates,
  listAccessMemberships,
  revokeAccessMembership,
} from '@/generated/api'
import type { AccessMembership } from '@/generated/model'
import { membershipCandidateParams } from './membershipCandidates'

type MutationRunner = (
  work: (options: RequestInit) => Promise<{ status: number }>,
  closeModal?: boolean,
) => Promise<boolean>

export function GroupMembersModal({
  group,
  submitting,
  mutate,
  close,
}: {
  group: { id: string; name: string; allowedActions: string[] }
  submitting: boolean
  mutate: MutationRunner
  close: () => void
}) {
  const { t } = useTranslation()
  const [form] = Form.useForm<{ userId: string }>()
  const [search, setSearch] = useState('')
  const [debouncedSearch, setDebouncedSearch] = useState('')

  useEffect(() => {
    const timeout = window.setTimeout(
      () => setDebouncedSearch(search.trim()),
      250,
    )
    return () => window.clearTimeout(timeout)
  }, [search])

  const candidates = useInfiniteQuery({
    queryKey: ['access', 'membership-candidates', group.id, debouncedSearch],
    initialPageParam: undefined as string | undefined,
    enabled: group.allowedActions.includes('addMember'),
    queryFn: ({ pageParam, signal }) =>
      listAccessMembershipCandidates(
        group.id,
        membershipCandidateParams(debouncedSearch, pageParam),
        { signal },
      ),
    getNextPageParam: (page) =>
      page.status === 200 && page.data.meta.hasMore
        ? page.data.meta.nextCursor
        : undefined,
  })
  const memberships = useInfiniteQuery({
    queryKey: ['access', 'memberships', group.id],
    initialPageParam: undefined as string | undefined,
    queryFn: ({ pageParam, signal }) =>
      listAccessMemberships(
        {
          groupId: group.id,
          status: 'active',
          cursor: pageParam,
          limit: 20,
        },
        { signal },
      ),
    getNextPageParam: (page) =>
      page.status === 200 && page.data.meta.hasMore
        ? page.data.meta.nextCursor
        : undefined,
  })

  const candidateOptions = useMemo(() => {
    const byID = new Map<string, string>()
    for (const page of candidates.data?.pages ?? []) {
      if (page.status !== 200) continue
      for (const item of page.data.data) byID.set(item.id, item.username)
    }
    return Array.from(byID, ([value, label]) => ({ value, label }))
  }, [candidates.data])
  const memberData = useMemo(
    () =>
      (memberships.data?.pages ?? []).flatMap((page) =>
        page.status === 200 ? page.data.data : [],
      ),
    [memberships.data],
  )

  const refresh = async () => {
    await Promise.all([candidates.refetch(), memberships.refetch()])
  }
  const addMember = async ({ userId }: { userId: string }) => {
    const saved = await mutate(
      (options) => createAccessMembership(group.id, { userId }, options),
      false,
    )
    if (!saved) return
    form.resetFields()
    await refresh()
  }
  const removeMember = async (membership: AccessMembership) => {
    const saved = await mutate(
      (options) => revokeAccessMembership(membership.id, options),
      false,
    )
    if (saved) await refresh()
  }

  return (
    <Modal
      open
      title={t('access.groupMembers.title', { group: group.name })}
      footer={null}
      width={720}
      onCancel={close}
      destroyOnHidden
    >
      {group.allowedActions.includes('addMember') && (
        <Form form={form} layout="vertical" onFinish={addMember}>
          <Form.Item
            name="userId"
            label={t('access.groupMembers.addLabel')}
            rules={[{ required: true }]}
          >
            <Select
              showSearch
              filterOption={false}
              searchValue={search}
              placeholder={t('access.groupMembers.candidatePlaceholder')}
              loading={candidates.isFetching && !candidates.isFetchingNextPage}
              options={candidateOptions}
              notFoundContent={
                candidates.isFetching ? undefined : (
                  <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} />
                )
              }
              onSearch={setSearch}
              onPopupScroll={(event) => {
                const target = event.currentTarget
                const nearBottom =
                  target.scrollTop + target.clientHeight >=
                  target.scrollHeight - 24
                if (
                  nearBottom &&
                  candidates.hasNextPage &&
                  !candidates.isFetchingNextPage
                )
                  void candidates.fetchNextPage()
              }}
            />
          </Form.Item>
          <Button type="primary" htmlType="submit" loading={submitting}>
            {t('access.groupMembers.add')}
          </Button>
        </Form>
      )}

      <Typography.Title level={5} style={{ marginTop: 24 }}>
        {t('access.groupMembers.current')}
      </Typography.Title>
      <Table
        rowKey="id"
        size="small"
        loading={memberships.isPending}
        dataSource={memberData}
        pagination={false}
        locale={{ emptyText: t('access.groupMembers.empty') }}
        columns={[
          {
            title: t('access.columns.username'),
            dataIndex: 'username',
          },
          {
            title: t('access.columns.source'),
            dataIndex: 'source',
            render: (source: AccessMembership['source']) => (
              <Tag>
                {source === 'oidc' ? 'OIDC' : t('access.groupMembers.manual')}
              </Tag>
            ),
          },
          {
            title: t('access.columns.actions'),
            render: (_: unknown, item: AccessMembership) =>
              item.allowedActions.includes('revoke') ? (
                <Popconfirm
                  title={t('access.groupMembers.removeConfirm')}
                  onConfirm={() => void removeMember(item)}
                >
                  <Button size="small" danger loading={submitting}>
                    {t('access.groupMembers.remove')}
                  </Button>
                </Popconfirm>
              ) : (
                <Typography.Text type="secondary">
                  {t('access.groupMembers.oidcManaged')}
                </Typography.Text>
              ),
          },
        ]}
      />
      {memberships.hasNextPage && (
        <Space
          style={{ width: '100%', justifyContent: 'center', marginTop: 12 }}
        >
          <Button
            loading={memberships.isFetchingNextPage}
            onClick={() => void memberships.fetchNextPage()}
          >
            {t('access.groupMembers.loadMore')}
          </Button>
        </Space>
      )}
    </Modal>
  )
}
