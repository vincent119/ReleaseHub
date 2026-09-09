import {
  AppstoreOutlined,
  ApartmentOutlined,
  DeploymentUnitOutlined,
  FileDoneOutlined,
  NodeIndexOutlined,
  SafetyCertificateOutlined,
  TeamOutlined,
} from '@ant-design/icons'
import { NavLink } from 'react-router'
import type { useTranslation } from 'react-i18next'

export function navigationItems(t: ReturnType<typeof useTranslation>['t']) {
  return [
    {
      key: '/',
      icon: <AppstoreOutlined />,
      label: (
        <NavLink to="/" aria-label={t('overview.title')}>
          {t('navigation.overview')}
        </NavLink>
      ),
    },
    {
      key: '/resources',
      icon: <DeploymentUnitOutlined />,
      label: (
        <NavLink to="/resources" aria-label={t('resources.title')}>
          {t('navigation.resources')}
        </NavLink>
      ),
    },
    {
      key: '/candidates',
      icon: <DeploymentUnitOutlined />,
      label: (
        <NavLink to="/candidates" aria-label={t('candidates.title')}>
          {t('navigation.candidates')}
        </NavLink>
      ),
    },
    {
      key: '/applications',
      icon: <SafetyCertificateOutlined />,
      label: (
        <NavLink to="/applications" aria-label={t('applications.title')}>
          {t('navigation.applications')}
        </NavLink>
      ),
    },
    {
      key: '/requests',
      icon: <FileDoneOutlined />,
      label: (
        <NavLink to="/requests" aria-label={t('requests.title')}>
          {t('navigation.requests')}
        </NavLink>
      ),
    },
    {
      key: '/workflows',
      icon: <ApartmentOutlined />,
      label: (
        <NavLink to="/workflows" aria-label={t('workflows.title')}>
          {t('navigation.workflows')}
        </NavLink>
      ),
    },
    {
      key: '/plans',
      icon: <NodeIndexOutlined />,
      label: (
        <NavLink to="/plans" aria-label={t('plans.title')}>
          {t('navigation.plans')}
        </NavLink>
      ),
    },
    {
      key: '/access',
      icon: <TeamOutlined />,
      label: (
        <NavLink to="/access" aria-label={t('access.title')}>
          {t('navigation.access')}
        </NavLink>
      ),
    },
  ]
}
