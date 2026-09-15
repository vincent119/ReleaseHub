/// <reference types="node" />

import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'

import { describe, expect, it } from 'vitest'

const paletteCss = readFileSync(
  resolve(
    process.cwd(),
    'src/features/workflows/components/WorkflowGraphEditor.module.css',
  ),
  'utf8',
)
const globalCss = readFileSync(
  resolve(process.cwd(), 'src/shared/styles/tokens.css'),
  'utf8',
)
const graphCss = readFileSync(
  resolve(
    process.cwd(),
    'src/features/workflows/components/WorkflowGraphEditor.module.css',
  ),
  'utf8',
)
const nodeCss = readFileSync(
  resolve(
    process.cwd(),
    'src/features/workflows/components/WorkflowStateNode.module.css',
  ),
  'utf8',
)

const paletteTokens = [
  '--workflow-workspace-bg',
  '--workflow-surface',
  '--workflow-surface-elevated',
  '--workflow-toolbar-bg',
  '--workflow-node-bg',
  '--workflow-border',
  '--workflow-border-strong',
  '--workflow-text-primary',
  '--workflow-text-secondary',
  '--workflow-grid',
  '--workflow-edge',
  '--workflow-edge-selected',
  '--workflow-selected',
  '--workflow-selected-soft',
  '--workflow-accent',
  '--workflow-accent-soft',
  '--workflow-start',
  '--workflow-start-surface',
  '--workflow-review',
  '--workflow-review-surface',
  '--workflow-manual',
  '--workflow-manual-surface',
  '--workflow-deployment',
  '--workflow-deployment-surface',
  '--workflow-terminal',
  '--workflow-terminal-surface',
  '--workflow-terminal-success',
  '--workflow-terminal-success-surface',
  '--workflow-terminal-failure',
  '--workflow-terminal-failure-surface',
  '--workflow-ambient',
  '--workflow-card-shadow',
  '--workflow-panel-shadow',
] as const

const inheritedMappings = new Map([
  ['--workflow-workspace-bg', '--rh-color-page'],
  ['--workflow-surface', '--rh-color-surface'],
  ['--workflow-surface-elevated', '--rh-color-surface-elevated'],
  ['--workflow-toolbar-bg', '--rh-color-surface-elevated'],
  ['--workflow-node-bg', '--rh-color-surface-elevated'],
  ['--workflow-border', '--rh-color-border'],
  ['--workflow-border-strong', '--rh-color-border-strong'],
  ['--workflow-text-primary', '--rh-color-text-primary'],
  ['--workflow-text-secondary', '--rh-color-text-secondary'],
  ['--workflow-grid', '--rh-color-border'],
  ['--workflow-edge', '--rh-color-border-strong'],
  ['--workflow-edge-selected', '--rh-color-focus'],
  ['--workflow-selected', '--rh-color-focus'],
  ['--workflow-selected-soft', '--rh-color-focus-soft'],
  ['--workflow-accent', '--rh-color-primary'],
  ['--workflow-accent-soft', '--rh-color-primary-soft'],
  ['--workflow-start', '--rh-color-primary'],
  ['--workflow-ambient', '--rh-ambient-page'],
  ['--workflow-card-shadow', '--rh-shadow-card'],
  ['--workflow-panel-shadow', '--rh-shadow-floating'],
])

describe.each([
  ['light', '.editor', ':root'],
  [
    'dark',
    ":global(:root[data-theme='dark']) .editor",
    ":root[data-theme='dark']",
  ],
] as const)(
  '%s Workflow component palette',
  (theme, selector, globalSelector) => {
    const palette = getRule(paletteCss, selector)
    const globals = getRule(globalCss, globalSelector)

    it('declares every component token and inherits ReleaseHub semantics', () => {
      for (const token of paletteTokens)
        expect(readToken(palette, token), token).not.toBe('')
      for (const [component, semantic] of inheritedMappings)
        expect(readToken(palette, component)).toBe(`var(${semantic})`)
    })

    it('meets text, node boundary, edge and selection contrast targets', () => {
      const canvas = resolvedToken(palette, globals, '--workflow-workspace-bg')
      for (const [accentToken, surfaceToken] of [
        ...statePaletteTokens,
        ...terminalOutcomePaletteTokens,
      ]) {
        expect(
          contrastRatio(
            resolvedToken(palette, globals, accentToken),
            resolvedToken(palette, globals, surfaceToken),
          ),
          `${theme} ${accentToken} on ${surfaceToken}`,
        ).toBeGreaterThanOrEqual(4.5)
      }
      const nodeBackground = resolvedToken(
        palette,
        globals,
        '--workflow-node-bg',
      )
      expect(
        contrastRatio(
          resolvedToken(palette, globals, '--workflow-border-strong'),
          nodeBackground,
        ),
      ).toBeGreaterThanOrEqual(3)
      expect(
        contrastRatio(
          resolvedToken(palette, globals, '--workflow-edge'),
          canvas,
        ),
      ).toBeGreaterThanOrEqual(3)
      expect(
        contrastRatio(
          resolvedToken(palette, globals, '--workflow-selected'),
          nodeBackground,
        ),
      ).toBeGreaterThanOrEqual(3)
    })
  },
)

it('uses separate Light and Dark Review and ManualAction accents', () => {
  const light = getRule(paletteCss, '.editor')
  const dark = getRule(paletteCss, ":global(:root[data-theme='dark']) .editor")
  expect(readToken(light, '--workflow-review')).toBe('#8a5a00')
  expect(readToken(dark, '--workflow-review')).toBe('#fbbf24')
  expect(readToken(light, '--workflow-manual')).toBe('#6235d5')
  expect(readToken(dark, '--workflow-manual')).toBe('#c4b5fd')
})

it('keeps outcome accents aligned with each theme while preserving AA text contrast', () => {
  const light = getRule(paletteCss, '.editor')
  const dark = getRule(paletteCss, ":global(:root[data-theme='dark']) .editor")
  expect(readToken(light, '--workflow-terminal-success')).toBe('#0b7451')
  expect(readToken(light, '--workflow-terminal-failure')).toBe('#b42318')
  expect(readToken(dark, '--workflow-terminal-success')).toBe(
    'var(--rh-color-success)',
  )
  expect(readToken(dark, '--workflow-terminal-failure')).toBe('#fb7185')
})

it.each([
  ['Start', '--workflow-start', '--workflow-start-surface'],
  ['Review', '--workflow-review', '--workflow-review-surface'],
  ['ManualAction', '--workflow-manual', '--workflow-manual-surface'],
  ['Deployment', '--workflow-deployment', '--workflow-deployment-surface'],
  ['Terminal', '--workflow-terminal', '--workflow-terminal-surface'],
] as const)(
  'maps %s to its semantic node accent and surface',
  (type, accent, surface) => {
    const rule = getRule(nodeCss, `.node[data-state-type='${type}']`)
    expect(readToken(rule, '--workflow-node-accent')).toBe(`var(${accent})`)
    expect(readToken(rule, '--workflow-node-surface')).toBe(`var(${surface})`)
  },
)

it.each([
  [
    'success',
    '--workflow-terminal-success',
    '--workflow-terminal-success-surface',
  ],
  [
    'failure',
    '--workflow-terminal-failure',
    '--workflow-terminal-failure-surface',
  ],
] as const)(
  'maps Terminal %s outcome to its semantic accent and surface',
  (outcome, accent, surface) => {
    const rule = getRule(nodeCss, `.node[data-terminal-outcome='${outcome}']`)
    expect(readToken(rule, '--workflow-node-accent')).toBe(`var(${accent})`)
    expect(readToken(rule, '--workflow-node-surface')).toBe(`var(${surface})`)
  },
)

it.each([
  ['light', '.editor'],
  ['dark', ":global(:root[data-theme='dark']) .editor"],
] as const)('%s state surfaces are distinct', (_theme, selector) => {
  const palette = getRule(paletteCss, selector)
  const surfaces = statePaletteTokens.map(([, token]) =>
    readToken(palette, token),
  )
  expect(new Set(surfaces).size).toBe(statePaletteTokens.length)
})

it.each([
  ['light', '.editor'],
  ['dark', ":global(:root[data-theme='dark']) .editor"],
] as const)(
  '%s default workflow surfaces keep visible color separation',
  (_theme, selector) => {
    const palette = getRule(paletteCss, selector)
    const surfaces = [
      '--workflow-review-surface',
      '--workflow-manual-surface',
      '--workflow-deployment-surface',
      '--workflow-terminal-success-surface',
      '--workflow-terminal-failure-surface',
    ].map((token) => readToken(palette, token))

    for (let left = 0; left < surfaces.length; left += 1)
      for (let right = left + 1; right < surfaces.length; right += 1)
        expect(
          rgbDistance(surfaces[left], surfaces[right]),
          `${surfaces[left]} compared with ${surfaces[right]}`,
        ).toBeGreaterThanOrEqual(18)
  },
)

it('uses the node accent consistently without replacing selection semantics', () => {
  expect(getRule(nodeCss, '.type')).toContain(
    'color: var(--workflow-node-accent)',
  )
  expect(getRule(nodeCss, '.node')).toContain(
    'border-top: 3px solid var(--workflow-node-accent)',
  )
  expect(getRule(nodeCss, '.node')).toContain(
    'background: var(--workflow-node-surface)',
  )
  expect(getRule(nodeCss, '.node :global(.react-flow__handle)')).toContain(
    'background: var(--workflow-node-accent)',
  )
  const selected = getRule(nodeCss, '.selected')
  expect(selected).toContain('var(--workflow-selected')
  expect(selected).not.toContain('var(--workflow-node-accent)')
})

const statePaletteTokens = [
  ['--workflow-start', '--workflow-start-surface'],
  ['--workflow-review', '--workflow-review-surface'],
  ['--workflow-manual', '--workflow-manual-surface'],
  ['--workflow-deployment', '--workflow-deployment-surface'],
  ['--workflow-terminal', '--workflow-terminal-surface'],
] as const

const terminalOutcomePaletteTokens = [
  ['--workflow-terminal-success', '--workflow-terminal-success-surface'],
  ['--workflow-terminal-failure', '--workflow-terminal-failure-surface'],
] as const

it('maps graph surfaces and paths to Workflow component tokens', () => {
  expect(getRule(graphCss, '.toolbar')).toContain('var(--workflow-toolbar-bg')
  expect(
    getRule(graphCss, '.canvas :global(.react-flow__background-pattern)'),
  ).toContain('var(--workflow-grid')
  expect(
    getRule(graphCss, '.canvas :global(.react-flow__edge-path)'),
  ).toContain('var(--workflow-edge')
  expect(
    getRule(
      graphCss,
      '.canvas :global(.react-flow__edge.selected .react-flow__edge-path)',
    ),
  ).toContain('var(--workflow-edge-selected')
})

function resolvedToken(
  palette: string,
  globals: string,
  token: string,
): string {
  const value = readToken(palette, token)
  const reference = value.match(/^var\((--[^)]+)\)$/)?.[1]
  return reference ? readToken(globals, reference) : value
}

function getRule(css: string, selector: string): string {
  const escaped = selector.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
  const match = css.match(new RegExp(`${escaped}\\s*\\{([\\s\\S]*?)\\}`))
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

function rgbDistance(left: string, right: string): number {
  const leftChannels = left
    .slice(1)
    .match(/.{2}/g)!
    .map((value) => Number.parseInt(value, 16))
  const rightChannels = right
    .slice(1)
    .match(/.{2}/g)!
    .map((value) => Number.parseInt(value, 16))
  return Math.sqrt(
    leftChannels.reduce(
      (distance, channel, index) =>
        distance + Math.pow(channel - rightChannels[index], 2),
      0,
    ),
  )
}
