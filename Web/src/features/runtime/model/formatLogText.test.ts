import { describe, expect, it } from 'vitest'

import { formatLogText } from './formatLogText'

describe('formatLogText', () => {
  it.each([
    ['', ''],
    [
      '中文 plain text {"value":1}\n\tat file.go:10',
      '中文 plain text {"value":1}\n\tat file.go:10',
    ],
    ['\u001b[34mINFO\u001b[0m accepted', 'INFO accepted'],
    ['a\u009b38;2;1;2;3mcolor\u009b0m', 'acolor'],
    ['a\u001b[2Jb\u001b[1;5Hc\u001b[?25ld', 'abcd'],
    ['a\u001b(Bb\u001b7c\u001b8d', 'abcd'],
    [
      '\u001b]8;;https://example.invalid\u001b\\label\u001b]8;;\u001b\\',
      'label',
    ],
    ['a\u001b]0;title\u0007b', 'ab'],
    ['a\u009d0;title\u009cb', 'ab'],
    [
      'a\u001bPpayload\u001b\\b\u001bXpayload\u001b\\c\u001b^payload\u001b\\d\u001b_payload\u001b\\e',
      'abcde',
    ],
    [
      'a\u0090payload\u009cb\u0098payload\u009cc\u009epayload\u009cd\u009fpayload\u009ce',
      'abcde',
    ],
    ['first\bsecond\rthird', 'first\\u0008second\\u000dthird'],
    ['a\r\nb', 'a\\u000d\nb'],
    ['a\u202eb\u2066c\u2069d', 'a\\u202eb\\u2066c\\u2069d'],
    ['literal \\u001b[34m remains', 'literal \\u001b[34m remains'],
    ['unfinished \u001b[31;', 'unfinished \\u001b[31;'],
    [
      'unfinished \u001b]0;keep this\nnext line',
      'unfinished \\u001b]0;keep this\nnext line',
    ],
    ['trailing \u001b', 'trailing \\u001b'],
    ['\u001b[31\nkeep', '\\u001b[31\nkeep'],
    ['\u001b[31\u001b[32mkeep', '\\u001b[31keep'],
  ])('produces safe readable text for case %#', (source, expected) => {
    expect(formatLogText(source)).toBe(expected)
  })

  it.each([
    '',
    'a\n\tb',
    '\u001b[34mINFO\u001b[0m',
    'literal \\u001b[34m "quote"',
    String.fromCharCode(...Array.from({ length: 160 }, (_, index) => index)),
    '中文\u202e\u2066\u2069\ud800',
  ])(
    'retains the exact original through JSON string decoding for case %#',
    (source) => {
      const raw = formatLogText(source, 'raw')
      expect(JSON.parse(raw)).toBe(source)
      for (const character of raw) {
        const code = character.charCodeAt(0)
        expect(code >= 32 && (code < 127 || code > 159)).toBe(true)
      }
    },
  )

  it('handles the existing payload bound and adversarial incomplete sequences without truncation', () => {
    const plain = 'a'.repeat(1024 * 1024)
    expect(formatLogText(plain)).toBe(plain)
    expect(JSON.parse(formatLogText(plain, 'raw'))).toBe(plain)
    const incomplete = '\u001b]'.repeat(100_000)
    expect(formatLogText(incomplete)).toBe('\\u001b]'.repeat(100_000))
    const colors = '\u001b[34mtext\u001b[0m'.repeat(30_000)
    expect(formatLogText(colors)).toBe('text'.repeat(30_000))
  })
})
