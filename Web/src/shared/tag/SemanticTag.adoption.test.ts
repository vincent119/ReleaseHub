/// <reference types="node" />

import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'

import { expect, it } from 'vitest'

const migratedFiles = [
  'src/features/resources/ResourcesPage.tsx',
  'src/features/access/AccessPage.tsx',
  'src/features/audit/AuditPage.tsx',
  'src/features/notifications/NotificationCenter.tsx',
  'src/features/access/GroupMembersModal.tsx',
  'src/features/requests/RequestsPage.tsx',
  'src/features/requests/components/ApplicationSnapshots.tsx',
  'src/features/requests/components/DeploymentHistoryPanel.tsx',
  'src/features/workflows/components/ReviewDecisionPanel.tsx',
  'src/features/plans/components/DeploymentSchedulePanel.tsx',
] as const

it.each(migratedFiles)('%s has no uncontrolled Ant Design Tag', (file) => {
  const source = readFileSync(resolve(process.cwd(), file), 'utf8')
  const openings = [...source.matchAll(/<Tag\b([^>]*)>/g)]

  for (const opening of openings) {
    const attributes = opening[1]
    const usesPresetColor = /\bcolor=/.test(attributes)
    const isExistingAccessStatus =
      file === 'src/features/access/AccessPage.tsx' &&
      attributes.includes('styles.statusActive') &&
      attributes.includes('styles.statusInactive')

    expect(usesPresetColor || isExistingAccessStatus, opening[0]).toBe(true)
    expect(opening[1], opening[0]).not.toMatch(/\bcolor=["']default["']/)
  }
})
