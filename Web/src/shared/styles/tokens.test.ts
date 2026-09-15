/// <reference types="node" />

import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'

import { describe, expect, it } from 'vitest'

const tokensCss = readFileSync(
  resolve(process.cwd(), 'src/shared/styles/tokens.css'),
  'utf8',
)

const semanticTokens = [
  '--rh-color-page',
  '--rh-color-surface',
  '--rh-color-surface-elevated',
  '--rh-color-surface-soft',
  '--rh-color-border',
  '--rh-color-border-strong',
  '--rh-color-text-primary',
  '--rh-color-text-secondary',
  '--rh-color-text-disabled',
  '--rh-color-text-inverse',
  '--rh-color-primary',
  '--rh-color-primary-hover',
  '--rh-color-primary-pressed',
  '--rh-color-primary-soft',
  '--rh-color-primary-contrast',
  '--rh-color-secondary',
  '--rh-color-secondary-soft',
  '--rh-color-tertiary',
  '--rh-color-focus',
  '--rh-color-focus-soft',
  '--rh-color-selection',
  '--rh-color-overlay',
  '--rh-color-accent',
  '--rh-color-success',
  '--rh-color-success-soft',
  '--rh-color-warning',
  '--rh-color-warning-soft',
  '--rh-color-error',
  '--rh-color-error-soft',
  '--rh-color-info',
  '--rh-color-info-soft',
  '--rh-ambient-page',
  '--rh-shadow-card',
  '--rh-shadow-floating',
] as const

const feedbackComponentTokens = [
  '--rh-feedback-success-bg',
  '--rh-feedback-info-bg',
  '--rh-feedback-warning-bg',
  '--rh-feedback-error-bg',
] as const

describe.each([
  ['light', ':root'],
  ['dark', ":root[data-theme='dark']"],
] as const)('%s semantic theme tokens', (_, selector) => {
  it('provides every required semantic value', () => {
    const declarations = getRule(selector)
    for (const token of semanticTokens) {
      expect(readToken(declarations, token), token).not.toBe('')
    }
  })

  it('provides every feedback component background', () => {
    const declarations = getRule(selector)
    for (const token of feedbackComponentTokens) {
      expect(readToken(declarations, token), token).not.toBe('')
    }
  })
})

it('keeps decorative dark ambience out of the light theme', () => {
  expect(readToken(getRule(':root'), '--rh-ambient-page')).toBe('none')
  const darkAmbience = readToken(
    getRule(":root[data-theme='dark']"),
    '--rh-ambient-page',
  )
  expect(darkAmbience).toContain('radial-gradient')
  expect(darkAmbience).toContain('/ 6%')
  expect(darkAmbience).not.toContain('204 58 186')
})

it.each([
  ['light primary text', '#172033', '#f6f8fb', 7],
  ['light secondary text', '#526074', '#ffffff', 4.5],
  ['light primary action', '#4f46e5', '#ffffff', 4.5],
  ['light focus indicator', '#4f46e5', '#ffffff', 3],
  ['light secondary', '#0f8f83', '#ffffff', 3],
  ['light strong border', '#7b8da5', '#ffffff', 3],
  ['light success text', '#14845e', '#ffffff', 4.5],
  ['light warning text', '#a15c00', '#ffffff', 4.5],
  ['light error text', '#c73942', '#ffffff', 4.5],
  ['light info text', '#2563eb', '#ffffff', 4.5],
  ['dark primary text', '#f8fafc', '#090b12', 7],
  ['dark secondary text', '#cbd5e1', '#0f172a', 4.5],
  ['dark primary action', '#818cf8', '#0f172a', 4.5],
  ['dark focus indicator', '#a5b4fc', '#0f172a', 3],
  ['dark secondary', '#2dd4bf', '#0f172a', 4.5],
  ['dark strong border', '#64748b', '#0f172a', 3],
  ['dark success text', '#34d399', '#0f172a', 4.5],
  ['dark warning text', '#f59e0b', '#0f172a', 4.5],
  ['dark error text', '#fb7185', '#0f172a', 4.5],
  ['dark info text', '#60a5fa', '#0f172a', 4.5],
])('%s satisfies its contrast target', (_, foreground, background, target) => {
  expect(contrastRatio(foreground, background)).toBeGreaterThanOrEqual(target)
})

function getRule(selector: string): string {
  expect(tokensCss).toContain(selector)
  const escaped = selector.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
  const match = tokensCss.match(new RegExp(`${escaped}\\s*\\{([\\s\\S]*?)\\}`))
  expect(match, `${selector} rule`).not.toBeNull()
  return match?.[1] ?? ''
}

function readToken(declarations: string, token: string): string {
  const match = declarations.match(new RegExp(`${token}:\\s*([^;]+);`))
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
