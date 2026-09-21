/// <reference types="node" />

import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'

import { describe, expect, it } from 'vitest'

const tagCss = readFileSync(
  resolve(process.cwd(), 'src/shared/tag/SemanticTag.module.css'),
  'utf8',
)
const tokensCss = readFileSync(
  resolve(process.cwd(), 'src/shared/styles/tokens.css'),
  'utf8',
)

it('maps every tone to ReleaseHub semantic tokens', () => {
  expect(rule(tagCss, '.neutral')).toContain(
    'color: var(--rh-color-text-secondary)',
  )
  expect(rule(tagCss, '.neutral')).toContain(
    'background: var(--rh-color-surface-soft)',
  )
  expect(rule(tagCss, '.neutral')).toContain(
    'border-color: var(--rh-color-border)',
  )

  for (const tone of ['success', 'warning', 'error', 'info']) {
    const declarations = rule(tagCss, `.${tone}`)
    expect(declarations).toContain(`color: var(--rh-color-${tone})`)
    expect(declarations).toContain(`background: var(--rh-color-${tone}-soft)`)
    expect(declarations).toContain(`border-color: var(--rh-color-${tone})`)
  }
})

describe.each([
  ['light', ':root'],
  ['dark', ":root[data-theme='dark']"],
] as const)('%s neutral SemanticTag', (_, selector) => {
  it('keeps secondary text readable on the soft surface', () => {
    const declarations = rule(tokensCss, selector)
    const foreground = token(declarations, '--rh-color-text-secondary')
    const background = token(declarations, '--rh-color-surface-soft')

    expect(background.toLowerCase()).not.toBe('#000000')
    expect(contrastRatio(foreground, background)).toBeGreaterThanOrEqual(4.5)
  })
})

function rule(css: string, selector: string): string {
  const escaped = selector.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
  const match = css.match(new RegExp(`${escaped}\\s*\\{([\\s\\S]*?)\\}`))
  expect(match, `${selector} rule`).not.toBeNull()
  return match?.[1] ?? ''
}

function token(declarations: string, name: string): string {
  const match = declarations.match(new RegExp(`${name}:\\s*([^;]+);`))
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
