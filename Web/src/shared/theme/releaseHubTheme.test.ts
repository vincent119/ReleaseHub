import { theme } from 'antd'
import { describe, expect, it } from 'vitest'

import { createReleaseHubTheme, releaseHubSeedColors } from './releaseHubTheme'

describe.each(['light', 'dark'] as const)('%s ReleaseHub theme', (mode) => {
  it('maps semantic CSS variables into Ant Design tokens', () => {
    const config = createReleaseHubTheme(mode)

    expect(config.algorithm).toBe(
      mode === 'dark' ? theme.darkAlgorithm : theme.defaultAlgorithm,
    )
    expect(config.token).toMatchObject({
      colorBgLayout: 'var(--rh-color-page)',
      colorBgContainer: 'var(--rh-color-surface)',
      colorBgElevated: 'var(--rh-color-surface-elevated)',
      colorFillSecondary: 'var(--rh-color-surface-soft)',
      colorBorderSecondary: 'var(--rh-color-border)',
      colorText: 'var(--rh-color-text-primary)',
      colorTextSecondary: 'var(--rh-color-text-secondary)',
      colorPrimary: releaseHubSeedColors[mode].primary,
      colorPrimaryBorder: 'var(--rh-color-primary)',
      controlOutline: 'var(--rh-color-focus-soft)',
    })
    expect(config.components).toMatchObject({
      Button: {
        defaultBg: 'var(--rh-color-surface-elevated)',
        defaultHoverBg: 'var(--rh-color-surface-soft)',
      },
      Input: {
        activeBg: 'var(--rh-color-surface)',
        addonBg: 'var(--rh-color-surface-soft)',
      },
      Menu: {
        itemSelectedBg: 'var(--rh-color-primary-soft)',
      },
      Segmented: {
        trackBg: 'var(--rh-color-surface-soft)',
      },
      Table: {
        rowSelectedBg: 'var(--rh-color-primary-soft)',
      },
    })
  })
})

it('keeps the compact control scale explicit', () => {
  expect(createReleaseHubTheme('light').token).toMatchObject({
    controlHeightSM: 28,
    controlHeight: 36,
    controlHeightLG: 40,
    borderRadius: 8,
    borderRadiusLG: 12,
  })
})
