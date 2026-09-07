import { ApartmentOutlined } from '@ant-design/icons'
import { Card, Empty, List } from 'antd'
import { useTranslation } from 'react-i18next'

import type { DeploymentPlan } from '@/generated/model'

import styles from '../PlansPage.module.css'

interface Props {
  plans: DeploymentPlan[]
  selected?: string
  loading: boolean
  onSelect: (id: string) => void
}

export function PlanList({ plans, selected, loading, onSelect }: Props) {
  const { t } = useTranslation()
  return (
    <Card title={t('plans.list.title')} loading={loading}>
      {plans.length === 0 ? (
        <Empty description={t('plans.list.empty')} />
      ) : (
        <List
          dataSource={plans}
          renderItem={(item) => (
            <List.Item
              className={item.id === selected ? styles.selected : undefined}
              onClick={() => onSelect(item.id)}
            >
              <List.Item.Meta
                avatar={<ApartmentOutlined />}
                title={item.name}
                description={item.description}
              />
            </List.Item>
          )}
        />
      )}
    </Card>
  )
}
