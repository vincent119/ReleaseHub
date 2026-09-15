/// <reference types="node" />

import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'

import { describe, expect, it } from 'vitest'

const tokensCss = readFileSync(
  resolve(process.cwd(), 'src/shared/styles/tokens.css'),
  'utf8',
)
const feedbackCss = readFileSync(
  resolve(process.cwd(), 'src/shared/feedback/feedback.module.css'),
  'utf8',
)

const types = ['success', 'info', 'warning', 'error'] as const

const palettes = {
  light: {
    selector: ':root',
    backgrounds: ['#e2f6ec', '#e4efff', '#fff1cf', '#ffe5e8'],
  },
  dark: {
    selector: ":root[data-theme='dark']",
    backgrounds: ['#123b31', '#162f52', '#3b2c13', '#451f2a'],
  },
} as const

describe.each(Object.entries(palettes))('%s feedback palette', (_, palette) => {
  it('defines distinct opaque component backgrounds', () => {
    const declarations = getRule(tokensCss, palette.selector)
    const backgrounds = types.map((type) =>
      readToken(declarations, `--rh-feedback-${type}-bg`),
    )

    expect(backgrounds).toEqual(palette.backgrounds)
    expect(new Set(backgrounds)).toHaveLength(types.length)
    expect(backgrounds).not.toContain(
      readToken(declarations, '--rh-color-surface-elevated'),
    )
    for (const background of backgrounds) {
      expect(background).toMatch(/^#[\da-f]{6}$/i)
    }
  })

  it('keeps status and secondary text readable on every background', () => {
    const declarations = getRule(tokensCss, palette.selector)
    const secondaryText = readToken(declarations, '--rh-color-text-secondary')

    for (const type of types) {
      const background = readToken(declarations, `--rh-feedback-${type}-bg`)
      const statusColor = readToken(declarations, `--rh-color-${type}`)
      expect(
        contrastRatio(secondaryText, background),
        type,
      ).toBeGreaterThanOrEqual(4.5)
      expect(
        contrastRatio(statusColor, background),
        type,
      ).toBeGreaterThanOrEqual(3)
    }
  })
})

it.each(types)('maps %s feedback to its component background token', (type) => {
  const declarations = getRule(
    feedbackCss,
    `.feedback.${type}:global(.ant-notification-notice-${type})`,
  )
  expect(readProperty(declarations, 'background')).toBe(
    `var(--rh-feedback-${type}-bg)`,
  )
})

function getRule(css: string, selector: string): string {
  const escaped = selector.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
  const match = css.match(new RegExp(`${escaped}\\s*\\{([\\s\\S]*?)\\}`))
  expect(match, `${selector} rule`).not.toBeNull()
  return match?.[1] ?? ''
}

function readToken(declarations: string, token: string): string {
  return readProperty(declarations, token)
}

function readProperty(declarations: string, property: string): string {
  const escaped = property.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
  const match = declarations.match(new RegExp(`${escaped}:\\s*([^;]+);`))
  return match?.[1].trim() ?? ''
}

function contrastRatio(foreground: string, background: string): number {
  const lighter = Math.max(luminance(foreground), luminance(background))
  const darker = Math.min(luminance(foreground), luminance(background))
  return (lighter + 0.05) / (darker + 0.05)
}

function luminance(hex: string): number {
  const channels = hex
    .slice(1)
    .match(/.{2}/g)!
    .map((value) => Number.parseInt(value, 16) / 255)
    .map((value) =>
      value <= 0.04045 ? value / 12.92 : Math.pow((value + 0.055) / 1.055, 2.4),
    )
  return channels[0] * 0.2126 + channels[1] * 0.7152 + channels[2] * 0.0722
}
