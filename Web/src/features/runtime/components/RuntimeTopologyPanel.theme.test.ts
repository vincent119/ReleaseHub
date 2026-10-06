/// <reference types="node" />

import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'

import { describe, expect, it } from 'vitest'

const css = readFileSync(
  resolve(
    process.cwd(),
    'src/features/runtime/components/RuntimeTopologyPanel.module.css',
  ),
  'utf8',
)

function rule(selector: string): string {
  const escaped = selector.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
  const match = css.match(new RegExp(`${escaped}\\s*\\{([\\s\\S]*?)\\}`))
  expect(match, `${selector} rule`).not.toBeNull()
  return match?.[1] ?? ''
}

describe('runtime node component tokens', () => {
  it('keeps the card and icon on neutral semantic surfaces', () => {
    const node = rule('.node')
    expect(node).toContain('--runtime-node-surface: var(--rh-color-surface)')
    expect(node).toContain('--runtime-node-icon-surface: var(--rh-color-page)')
    expect(rule('.node .nodeIcon')).toContain(
      'background: var(--runtime-node-icon-surface)',
    )
    expect(rule('.kindChip')).toContain('background: var(--rh-color-page)')
    expect(rule('.groupNode')).not.toContain('--runtime-node-surface:')
  })

  it.each(['Healthy', 'Progressing', 'Missing', 'Suspended', 'Degraded'])(
    '%s changes only the accent, not the card surface',
    (health) => {
      const selector =
        health === 'Missing' || health === 'Suspended'
          ? ".node[data-health='Missing'],\n.node[data-health='Suspended']"
          : `.node[data-health='${health}']`
      const declarations = rule(selector)
      expect(declarations).toContain('--runtime-node-accent:')
      expect(declarations).not.toContain('--runtime-node-surface:')
    },
  )
})
