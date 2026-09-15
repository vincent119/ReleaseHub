import { ApartmentOutlined } from '@ant-design/icons'
import { Card, Empty } from 'antd'
import { useTranslation } from 'react-i18next'

import type { DeploymentPlan } from '@/generated/model'
import definitionStyles from '@/shared/definition/DefinitionWorkspace.module.css'
import { SemanticList, SemanticListItemContent } from '@/shared/list'

interface Props {
  plans: DeploymentPlan[]
  selected?: string
  loading: boolean
  onSelect: (id: string) => void
}

export function PlanList({ plans, selected, loading, onSelect }: Props) {
  const { t } = useTranslation()
  return (
    <Card
      className={definitionStyles.listCard}
      title={t('plans.list.title')}
      loading={loading}
    >
      {plans.length === 0 ? (
        <Empty description={t('plans.list.empty')} />
      ) : (
        <SemanticList
          items={plans}
          rowKey="id"
          itemProps={(item) => ({
            className: `${definitionStyles.listItem} ${item.id === selected ? definitionStyles.selected : ''}`,
            role: 'button',
            tabIndex: 0,
            'aria-current': item.id === selected ? 'page' : undefined,
            onClick: () => onSelect(item.id),
            onKeyDown: (event) => {
              if (event.key !== 'Enter' && event.key !== ' ') return
              event.preventDefault()
              onSelect(item.id)
            },
          })}
          renderItem={(item) => (
            <SemanticListItemContent
              leading={<ApartmentOutlined />}
              leadingVariant="accent"
              truncate
              title={item.name}
              description={item.description}
            />
          )}
        />
      )}
    </Card>
  )
}
