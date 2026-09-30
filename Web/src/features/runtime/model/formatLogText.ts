export type LogDisplayMode = 'readable' | 'raw'

const c1Kinds: Partial<Record<number, string>> = {
  0x9b: '[',
  0x9d: ']',
  0x90: 'P',
  0x98: 'X',
  0x9e: '^',
  0x9f: '_',
}

function escapeControls(source: string, preserveWhitespace: boolean): string {
  const parts: string[] = []
  let start = 0
  for (let index = 0; index < source.length; index += 1) {
    const code = source.charCodeAt(index)
    const control =
      code < 32 ||
      (code >= 127 && code <= 159) ||
      code === 0x200e ||
      code === 0x200f ||
      (code >= 0x202a && code <= 0x202e) ||
      (code >= 0x2066 && code <= 0x2069)
    if (!control || (preserveWhitespace && (code === 9 || code === 10)))
      continue
    parts.push(
      source.slice(start, index),
      `\\u${code.toString(16).padStart(4, '0')}`,
    )
    start = index + 1
  }
  if (start === 0) return source
  parts.push(source.slice(start))
  return parts.join('')
}

function scanSequence(
  source: string,
  start: number,
): { end: number; complete: boolean } | undefined {
  const code = source.charCodeAt(start)
  const escape = code === 27
  const kind = escape ? source[start + 1] : c1Kinds[code]
  if (!escape && !kind) return undefined
  let end = start + (escape ? 2 : 1)
  const between = (index: number, min: number, max: number) => {
    const value = source.charCodeAt(index)
    return value >= min && value <= max
  }
  if (kind === '[') {
    while (between(end, 0x30, 0x3f)) end += 1
    while (between(end, 0x20, 0x2f)) end += 1
    const complete = between(end, 0x40, 0x7e)
    return { end: end + (complete ? 1 : 0), complete }
  }
  if (kind && ']PX^_'.includes(kind)) {
    while (end < source.length) {
      const current = source.charCodeAt(end)
      if (current === 0x9c || (kind === ']' && current === 7)) {
        return { end: end + 1, complete: true }
      }
      if (current === 27 && source[end + 1] === '\\') {
        return { end: end + 2, complete: true }
      }
      end += 1
    }
    // 未結束序列無法安全判定內容邊界，保留可見原文而非吞掉後續訊息。
    return { end, complete: false }
  }
  end = start + 1
  while (between(end, 0x20, 0x2f)) end += 1
  const complete = between(end, 0x30, 0x7e)
  return { end: end + (complete ? 1 : 0), complete }
}

export function formatLogText(
  source: string,
  mode: LogDisplayMode = 'readable',
): string {
  if (mode === 'raw') {
    // JSON字串表示法保留反斜線與換行差異，避免真實控制碼與字面跳脫混淆。
    return escapeControls(JSON.stringify(source), false)
  }
  const parts: string[] = []
  let start = 0
  let index = 0
  while (index < source.length) {
    const sequence = scanSequence(source, index)
    if (!sequence) {
      index += 1
      continue
    }
    if (sequence.complete) {
      parts.push(source.slice(start, index))
      start = sequence.end
    }
    index = sequence.end
  }
  parts.push(source.slice(start))
  // 不模擬退格或覆寫行；一般文字、tab、換行及內嵌時間保持原順序。
  return escapeControls(parts.join(''), true)
}
