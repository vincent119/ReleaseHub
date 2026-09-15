import assert from 'node:assert/strict'
import { randomBytes } from 'node:crypto'
import { mkdirSync, mkdtempSync, rmSync, writeFileSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import test from 'node:test'

import { analyzeBundle } from './check-bundle-budget.mjs'

test('計算 entry 與遞迴同步 imports，不包含 dynamic imports', () => {
  const fixture = createFixture({
    'src/main.tsx': {
      file: 'assets/main.js',
      isEntry: true,
      imports: ['_shared.js'],
      dynamicImports: ['src/features/plans.tsx'],
    },
    '_shared.js': { file: 'assets/shared.js' },
    'src/features/plans.tsx': {
      file: 'assets/plans.js',
      isDynamicEntry: true,
    },
  })

  try {
    writeAsset(fixture, 'main.js', Buffer.from('main entry'))
    writeAsset(fixture, 'shared.js', Buffer.from('shared module'))
    writeAsset(fixture, 'plans.js', Buffer.from('lazy plans route'))

    const result = analyzeBundle(fixture, {
      maxChunkBytes: 500_000,
      maxEntryGzipBytes: 370_000,
    })

    assert.deepEqual(result.entries[0].files.sort(), [
      'assets/main.js',
      'assets/shared.js',
    ])
    assert.equal(result.violations.length, 0)
  } finally {
    rmSync(fixture, { recursive: true, force: true })
  }
})

test('回報單一 chunk 與 entry gzip 超標', () => {
  const fixture = createFixture({
    'src/main.tsx': { file: 'assets/main.js', isEntry: true },
  })

  try {
    writeAsset(fixture, 'main.js', randomBytes(510_000))

    const result = analyzeBundle(fixture, {
      maxChunkBytes: 500_000,
      maxEntryGzipBytes: 370_000,
    })

    assert.equal(result.violations.length, 2)
    assert.match(result.violations[0], /單一 chunk 預算/)
    assert.match(result.violations[1], /entry 預算/)
  } finally {
    rmSync(fixture, { recursive: true, force: true })
  }
})

function createFixture(manifest) {
  const fixture = mkdtempSync(join(tmpdir(), 'releasehub-bundle-budget-'))
  mkdirSync(join(fixture, '.vite'), { recursive: true })
  mkdirSync(join(fixture, 'assets'), { recursive: true })
  writeFileSync(
    join(fixture, '.vite', 'manifest.json'),
    JSON.stringify(manifest),
  )
  return fixture
}

function writeAsset(fixture, name, content) {
  writeFileSync(join(fixture, 'assets', name), content)
}
