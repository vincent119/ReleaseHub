import {
  DownOutlined,
  LogoutOutlined,
  SkinOutlined,
  UserOutlined,
} from '@ant-design/icons'
import { useQueryClient } from '@tanstack/react-query'
import { Avatar, Button, Dropdown, Grid, Space, Spin } from 'antd'
import { useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'

import {
  getGetAuthSessionQueryKey,
  useLogoutAuthSession,
} from '@/generated/api'
import { useFeedback } from '@/shared/feedback/useFeedback'

import styles from './AccountMenu.module.css'
import { PersonalSettings } from './PersonalSettings'
import { ThemeSettings } from './ThemeSettings'

interface AccountMenuProps {
  userId: string
  username: string
  desktopOverride?: boolean
  navigateTo?: (url: string) => void
}

export function AccountMenu({
  userId,
  username,
  desktopOverride,
  navigateTo = (url) => window.location.assign(url),
}: AccountMenuProps) {
  const { t } = useTranslation()
  const feedback = useFeedback()
  const queryClient = useQueryClient()
  const screens = Grid.useBreakpoint()
  const desktop = desktopOverride ?? Boolean(screens.md)
  const triggerRef = useRef<HTMLButtonElement>(null)
  const [menuOpen, setMenuOpen] = useState(false)
  const [personalOpen, setPersonalOpen] = useState(false)
  const [themeOpen, setThemeOpen] = useState(false)
  const csrfToken = browserCookie('releasehub_csrf')
  const logout = useLogoutAuthSession(
    csrfToken
      ? { fetch: { headers: { 'X-CSRF-Token': csrfToken } } }
      : undefined,
  )

  const focusTrigger = () => triggerRef.current?.focus()
  const closeTheme = () => {
    setThemeOpen(false)
    if (desktop) requestAnimationFrame(focusTrigger)
  }
  const signOut = async () => {
    if (logout.isPending) return
    if (!csrfToken) {
      feedback.error(t('account.signOutError'))
      return
    }

    try {
      const response = await logout.mutateAsync()
      if (response.status !== 200 || !response.data.data.loggedOut) {
        feedback.error(t('account.signOutError'))
        return
      }

      const redirectUrl = response.data.data.redirectUrl
      if (redirectUrl) {
        navigateTo(redirectUrl)
        return
      }

      await queryClient.invalidateQueries({
        queryKey: getGetAuthSessionQueryKey(),
      })
    } catch {
      feedback.error(t('account.signOutError'))
    }
  }

  const trigger = (
    <Dropdown
      open={menuOpen}
      trigger={['click']}
      placement="bottomRight"
      onOpenChange={setMenuOpen}
      menu={{
        items: [
          {
            key: 'personal',
            icon: <UserOutlined aria-hidden="true" />,
            label: t('account.personalSettings'),
          },
          {
            key: 'theme',
            icon: <SkinOutlined aria-hidden="true" />,
            label: t('account.themeSettings'),
          },
          { type: 'divider' },
          {
            key: 'logout',
            danger: true,
            disabled: logout.isPending,
            icon: logout.isPending ? (
              <Spin size="small" />
            ) : (
              <LogoutOutlined aria-hidden="true" />
            ),
            label: t('account.signOut'),
          },
        ],
        onClick: ({ key }) => {
          setMenuOpen(false)
          if (key === 'personal') setPersonalOpen(true)
          if (key === 'theme') setThemeOpen(true)
          if (key === 'logout') void signOut()
        },
      }}
    >
      <Button
        ref={triggerRef}
        type="text"
        className={styles.accountTrigger}
        aria-label={t('account.openMenu', { username })}
      >
        <Space size="small">
          <Avatar size="small" className={styles.avatar}>
            {avatarLabel(username)}
          </Avatar>
          <span className={styles.accountUsername}>{username}</span>
          <DownOutlined className={styles.chevron} aria-hidden="true" />
        </Space>
      </Button>
    </Dropdown>
  )

  return (
    <>
      <ThemeSettings
        anchor={trigger}
        desktop={desktop}
        open={themeOpen}
        onClose={closeTheme}
        onAfterClose={focusTrigger}
      />
      <PersonalSettings
        open={personalOpen}
        userId={userId}
        username={username}
        onClose={() => setPersonalOpen(false)}
        onAfterClose={focusTrigger}
      />
    </>
  )
}

function avatarLabel(username: string) {
  return username.trim().charAt(0).toLocaleUpperCase() || <UserOutlined />
}

function browserCookie(name: string): string | undefined {
  return document.cookie
    .split('; ')
    .find((value) => value.startsWith(`${name}=`))
    ?.slice(name.length + 1)
}
