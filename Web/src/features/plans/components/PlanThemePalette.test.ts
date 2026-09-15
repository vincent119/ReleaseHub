/// <reference types="node" />

import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'

import { describe, expect, it } from 'vitest'

const paletteCss = readFileSync(
  resolve(
    process.cwd(),
    'src/features/plans/components/PlanGraphEditor.module.css',
  ),
  'utf8',
)
const graphCss = readFileSync(
  resolve(
    process.cwd(),
    'src/features/plans/components/PlanGraphEditor.module.css',
  ),
  'utf8',
)
const workflowCss = readFileSync(
  resolve(
    process.cwd(),
    'src/features/workflows/components/WorkflowGraphEditor.module.css',
  ),
  'utf8',
)

const paletteTokens = [
  '--plan-workspace-bg',
  '--plan-surface',
  '--plan-surface-elevated',
  '--plan-toolbar-bg',
  '--plan-canvas-bg',
  '--plan-node-bg',
  '--plan-stage-0-accent',
  '--plan-stage-0-surface',
  '--plan-stage-1-accent',
  '--plan-stage-1-surface',
  '--plan-stage-2-accent',
  '--plan-stage-2-surface',
  '--plan-stage-3-accent',
  '--plan-stage-3-surface',
  '--plan-stage-4-accent',
  '--plan-stage-4-surface',
  '--plan-border',
  '--plan-border-strong',
  '--plan-text-primary',
  '--plan-text-secondary',
  '--plan-grid',
  '--plan-edge',
  '--plan-edge-selected',
  '--plan-selected',
  '--plan-selected-soft',
  '--plan-accent',
  '--plan-accent-soft',
  '--plan-ambient',
  '--plan-card-shadow',
  '--plan-panel-shadow',
] as const

const semanticMappings = new Map([
  ['--plan-workspace-bg', '--rh-color-page'],
  ['--plan-surface', '--rh-color-surface'],
  ['--plan-surface-elevated', '--rh-color-surface-elevated'],
  ['--plan-toolbar-bg', '--rh-color-surface-elevated'],
  ['--plan-canvas-bg', '--rh-color-page'],
  ['--plan-border', '--rh-color-border'],
  ['--plan-border-strong', '--rh-color-border-strong'],
  ['--plan-text-primary', '--rh-color-text-primary'],
  ['--plan-text-secondary', '--rh-color-text-secondary'],
  ['--plan-grid', '--rh-color-border'],
  ['--plan-edge', '--rh-color-border-strong'],
  ['--plan-edge-selected', '--rh-color-focus'],
  ['--plan-selected', '--rh-color-focus'],
  ['--plan-selected-soft', '--rh-color-focus-soft'],
  ['--plan-accent', '--rh-color-primary'],
  ['--plan-accent-soft', '--rh-color-primary-soft'],
  ['--plan-ambient', '--rh-ambient-page'],
  ['--plan-card-shadow', '--rh-shadow-card'],
  ['--plan-panel-shadow', '--rh-shadow-floating'],
])

const workflowStageMappings = new Map([
  ['--plan-stage-0-accent', '--workflow-start'],
  ['--plan-stage-0-surface', '--workflow-start-surface'],
  ['--plan-stage-1-accent', '--workflow-review'],
  ['--plan-stage-1-surface', '--workflow-review-surface'],
  ['--plan-stage-2-accent', '--workflow-manual'],
  ['--plan-stage-2-surface', '--workflow-manual-surface'],
  ['--plan-stage-3-accent', '--workflow-deployment'],
  ['--plan-stage-3-surface', '--workflow-deployment-surface'],
  ['--plan-stage-4-accent', '--workflow-terminal-success'],
  ['--plan-stage-4-surface', '--workflow-terminal-success-surface'],
])

describe.each([
  ['light', '.editor', '.editor'],
  [
    'dark',
    ":global(:root[data-theme='dark']) .editor",
    ":global(:root[data-theme='dark']) .editor",
  ],
] as const)(
  '%s Plan component palette',
  (_theme, selector, workflowSelector) => {
    const palette = getRule(paletteCss, selector)
    const workflow = getRule(workflowCss, workflowSelector)

    it('declares a complete component palette from ReleaseHub semantics', () => {
      for (const token of paletteTokens)
        expect(readToken(palette, token), token).not.toBe('')
      for (const [component, semantic] of semanticMappings)
        expect(readToken(palette, component)).toBe(`var(${semantic})`)
    })

    it('matches the Workflow canvas hierarchy and Start node surface', () => {
      expect(readToken(palette, '--plan-workspace-bg')).toBe(
        readToken(workflow, '--workflow-workspace-bg'),
      )
      expect(readToken(palette, '--plan-toolbar-bg')).toBe(
        readToken(workflow, '--workflow-toolbar-bg'),
      )
      expect(readToken(palette, '--plan-node-bg')).toBe(
        readToken(workflow, '--workflow-start-surface'),
      )
      expect(readToken(palette, '--plan-selected')).toBe(
        readToken(workflow, '--workflow-selected'),
      )
      for (const [planToken, workflowToken] of workflowStageMappings)
        expect(readToken(palette, planToken)).toBe(
          readToken(workflow, workflowToken),
        )
    })
  },
)

it('maps every Plan Graph visual layer to component tokens', () => {
  expect(getRule(paletteCss, '.editor')).toContain(
    'background: var(--plan-surface',
  )
  expect(getRule(graphCss, '.toolbar')).toContain('var(--plan-toolbar-bg')
  expect(getRule(graphCss, '.canvas')).toContain('var(--plan-canvas-bg')
  expect(
    getRule(graphCss, '.canvas :global(.react-flow__node-default)'),
  ).toContain('var(--plan-node-surface')
  for (const tone of [1, 2, 3, 4]) {
    const rule = getRule(
      graphCss,
      `.canvas :global(.react-flow__node-default.plan-stage-${tone})`,
    )
    expect(rule).toContain(`var(--plan-stage-${tone}-accent)`)
    expect(rule).toContain(`var(--plan-stage-${tone}-surface)`)
  }
  expect(
    getRule(graphCss, '.canvas :global(.react-flow__edge-path)'),
  ).toContain('var(--plan-edge')
  expect(
    getRule(graphCss, '.canvas :global(.react-flow__node-default.selected)'),
  ).toContain('var(--plan-selected')
  expect(getRule(graphCss, '.inspector')).toContain('var(--plan-surface')
})

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
