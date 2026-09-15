import { existsSync, readFileSync, readdirSync } from 'node:fs'
import { gzipSync } from 'node:zlib'
import { join, relative, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

export const DEFAULT_BUNDLE_BUDGET = Object.freeze({
  maxChunkBytes: 500_000,
  maxEntryGzipBytes: 370_000,
})

export function analyzeBundle(distDirectory, budget = DEFAULT_BUNDLE_BUDGET) {
  const distPath = resolve(distDirectory)
  const manifestPath = join(distPath, '.vite', 'manifest.json')

  if (!existsSync(manifestPath)) {
    throw new Error(`找不到 Vite manifest：${manifestPath}`)
  }

  const manifest = JSON.parse(readFileSync(manifestPath, 'utf8'))
  const chunks = collectJavaScriptFiles(distPath).map((filePath) => {
    const content = readFileSync(filePath)
    return {
      file: relative(distPath, filePath),
      bytes: content.byteLength,
      gzipBytes: gzipSync(content).byteLength,
    }
  })
  const chunkByFile = new Map(chunks.map((chunk) => [chunk.file, chunk]))
  const entries = Object.entries(manifest)
    .filter(([, value]) => value.isEntry)
    .map(([manifestKey]) => {
      const files = collectSynchronousEntryFiles(manifest, manifestKey)
      const missingFiles = files.filter((file) => !chunkByFile.has(file))

      if (missingFiles.length > 0) {
        throw new Error(
          `Vite manifest 指向不存在的 JavaScript 產物：${missingFiles.join(', ')}`,
        )
      }

      return {
        entry: manifestKey,
        files,
        gzipBytes: files.reduce(
          (total, file) => total + chunkByFile.get(file).gzipBytes,
          0,
        ),
      }
    })

  if (entries.length === 0) {
    throw new Error('Vite manifest 沒有 application entry')
  }

  const violations = [
    ...chunks
      .filter((chunk) => chunk.bytes >= budget.maxChunkBytes)
      .map(
        (chunk) =>
          `${chunk.file} 為 ${formatKilobytes(chunk.bytes)}，超過單一 chunk 預算 ${formatKilobytes(budget.maxChunkBytes)}`,
      ),
    ...entries
      .filter((entry) => entry.gzipBytes > budget.maxEntryGzipBytes)
      .map(
        (entry) =>
          `${entry.entry} 的同步 JavaScript gzip 合計為 ${formatKilobytes(entry.gzipBytes)}，超過 entry 預算 ${formatKilobytes(budget.maxEntryGzipBytes)}`,
      ),
  ]

  return { budget, chunks, entries, violations }
}

export function formatBundleReport(result) {
  const chunkLines = [...result.chunks]
    .sort((left, right) => right.bytes - left.bytes)
    .map(
      (chunk) =>
        `  ${chunk.file}: ${formatKilobytes(chunk.bytes)}，gzip ${formatKilobytes(chunk.gzipBytes)}`,
    )
  const entryLines = result.entries.map(
    (entry) =>
      `  ${entry.entry}: ${formatKilobytes(entry.gzipBytes)} gzip，共 ${entry.files.length} 個同步 chunk`,
  )

  return [
    'JavaScript bundle：',
    ...chunkLines,
    'Entry 同步載入：',
    ...entryLines,
  ].join('\n')
}

function collectSynchronousEntryFiles(manifest, entryKey) {
  const visitedKeys = new Set()
  const files = new Set()

  function visit(key) {
    if (visitedKeys.has(key)) return
    visitedKeys.add(key)

    const chunk = manifest[key]
    if (!chunk) {
      throw new Error(`Vite manifest 缺少同步 import：${key}`)
    }
    if (typeof chunk.file === 'string' && chunk.file.endsWith('.js')) {
      files.add(chunk.file)
    }
    for (const importedKey of chunk.imports ?? []) visit(importedKey)
  }

  visit(entryKey)
  return [...files]
}

function collectJavaScriptFiles(directory) {
  return readdirSync(directory, { withFileTypes: true }).flatMap((entry) => {
    const path = join(directory, entry.name)
    if (entry.isDirectory()) return collectJavaScriptFiles(path)
    return entry.isFile() && entry.name.endsWith('.js') ? [path] : []
  })
}

function formatKilobytes(bytes) {
  return `${(bytes / 1000).toFixed(2)} kB`
}

function runCli() {
  const distDirectory = resolve(process.argv[2] ?? 'dist')
  const result = analyzeBundle(distDirectory)
  console.log(formatBundleReport(result))

  if (result.violations.length > 0) {
    console.error('\nBundle 預算檢查失敗：')
    for (const violation of result.violations) console.error(`  - ${violation}`)
    process.exitCode = 1
  }
}

const currentFile = fileURLToPath(import.meta.url)
const invokedFile = process.argv[1] ? resolve(process.argv[1]) : undefined
if (invokedFile === currentFile) runCli()
