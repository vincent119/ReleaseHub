import { describe, expect, it } from 'vitest'

import { formatManifest } from './formatManifest'

describe('formatManifest', () => {
  it('indents objects and arrays without changing token order or spelling', () => {
    const source = '{"z":[],"a":{},"items":[true,null,"text",{"count":2}]}'
    expect(formatManifest(source)).toEqual({
      status: 'formatted',
      text: JSON.stringify(JSON.parse(source), null, 2),
      hasManagedFields: false,
    })
  })

  it('preserves large integers, negative zero, fractional precision and exponents', () => {
    const tokens = [
      '900719925474099312345678',
      '-900719925474099312345678',
      '-0',
      '0.123456789012345678901',
      '1.2300e+999',
    ]
    const result = formatManifest(`[${tokens.join(',')}]`)
    expect(result.status).toBe('formatted')
    expect(result.text).toBe(
      `[\n${tokens.map((token) => `  ${token}`).join(',\n')}\n]`,
    )
  })

  it('preserves escaped strings and duplicate keys without prototype mutation', () => {
    const source = String.raw`{"a":"quote\" slash\\ tab\t line\n \u4f60","a":2,"__proto__":{"example":true},"10":1,"2":2}`
    const result = formatManifest(source)
    expect(result.status).toBe('formatted')
    expect(result.text).toContain(
      String.raw`"a": "quote\" slash\\ tab\t line\n \u4f60"`,
    )
    expect(result.text.match(/"a":/g)).toHaveLength(2)
    expect(result.text.indexOf('"10"')).toBeLessThan(result.text.indexOf('"2"'))
    expect(result.text).toContain('"__proto__": {')
    expect(Object.prototype).not.toHaveProperty('example')
  })

  it('hides only root metadata.managedFields while retaining all other data', () => {
    const source =
      '{"metadata":{"managedFields":[1],"annotations":{"managedFields":"annotation"}},"spec":{"metadata":{"managedFields":"business"}},"status":{"ready":true},"managedFields":"root"}'
    const result = formatManifest(source)
    expect(result.hasManagedFields).toBe(true)
    expect(JSON.parse(result.text)).toEqual({
      metadata: { annotations: { managedFields: 'annotation' } },
      spec: { metadata: { managedFields: 'business' } },
      status: { ready: true },
      managedFields: 'root',
    })
    expect(formatManifest(source, true).text).toBe(
      JSON.stringify(JSON.parse(source), null, 2),
    )
  })

  it('recognizes escaped root keys, removes all duplicate managedFields, and retains empty metadata', () => {
    const source = String.raw`{"meta\u0064ata":{"managed\u0046ields":[],"managedFields":null}}`
    expect(formatManifest(source)).toEqual({
      status: 'formatted',
      text: String.raw`{` + '\n  "meta\\u0064ata": {}\n}',
      hasManagedFields: true,
    })
    expect(formatManifest(source, true).text).toContain(
      String.raw`"managed\u0046ields": []`,
    )
  })

  it('does not filter resources nested inside a root array', () => {
    const result = formatManifest('[{"metadata":{"managedFields":[1]}}]')
    expect(result.hasManagedFields).toBe(false)
    expect(result.text).toContain('"managedFields"')
  })

  it.each(['{}', '[]', 'null', 'true', 'false', '123', '"text"'])(
    'accepts valid JSON %s',
    (source) => {
      expect(formatManifest(source).text).toBe(source)
      expect(formatManifest(source).status).toBe('formatted')
    },
  )

  it.each(['', ' \n\t '])('returns an explicit empty state', (source) => {
    expect(formatManifest(source)).toMatchObject({
      status: 'empty',
      text: source,
    })
  })

  it.each([
    'kind: Pod',
    '{',
    '[1,]',
    '{"x":1,}',
    '{"x" 1}',
    '{x:1}',
    '01',
    '+1',
    'NaN',
    '1.',
    '[true false]',
    'true false',
    String.raw`"\x41"`,
    '"line\nbreak"',
    '{"x":}',
    '\uFEFF{}',
  ])('keeps invalid or non-JSON input unchanged: %s', (source) => {
    expect(formatManifest(source)).toEqual({
      status: 'invalid',
      text: source,
      hasManagedFields: false,
    })
  })

  it.each([
    ' '.repeat(1024 * 1024 + 1),
    '['.repeat(66) + '0' + ']'.repeat(66),
    '[' + '0,'.repeat(50_000) + '0]',
    '['.repeat(60) + '0,'.repeat(35_000) + '0' + ']'.repeat(60),
  ])(
    'falls back without truncation when a processing limit is reached',
    (source) => {
      expect(formatManifest(source)).toEqual({
        status: 'limited',
        text: source,
        hasManagedFields: false,
      })
    },
  )
})
