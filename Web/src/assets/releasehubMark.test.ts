/// <reference types="node" />

import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'

import { describe, expect, it } from 'vitest'

const markPath = resolve(process.cwd(), 'src/assets/releasehub-mark.svg')
const faviconPath = resolve(process.cwd(), 'public/favicon.ico')

describe('ReleaseHub brand assets', () => {
  it('keeps the approved logo composition and color responsibilities', () => {
    const mark = readFileSync(markPath, 'utf8')

    expect(mark).toContain('viewBox="0 0 512 512"')
    expect(mark).toContain('#151A3A')
    expect(mark).toContain('#67E8F9')
    expect(mark).toContain('#818CF8')
    expect(mark).toContain('stroke="#02C874"')
    expect(mark.match(/#02C874/g)).toHaveLength(1)
    expect(mark).not.toContain('#E05AD7')
    expect(mark).not.toContain('#2CC8E8')
  })

  it('contains the expected favicon sizes from the same square mark', () => {
    const favicon = readFileSync(faviconPath)

    expect(favicon.readUInt16LE(0)).toBe(0)
    expect(favicon.readUInt16LE(2)).toBe(1)

    const imageCount = favicon.readUInt16LE(4)
    const sizes = Array.from({ length: imageCount }, (_, index) => {
      const width = favicon[6 + index * 16]
      const height = favicon[7 + index * 16]
      return {
        width: width === 0 ? 256 : width,
        height: height === 0 ? 256 : height,
      }
    })

    expect(sizes.sort((left, right) => left.width - right.width)).toEqual([
      { width: 16, height: 16 },
      { width: 32, height: 32 },
      { width: 48, height: 48 },
      { width: 64, height: 64 },
      { width: 128, height: 128 },
      { width: 256, height: 256 },
    ])
  })
})
