type Value =
  | { kind: 'atom'; raw: string }
  | { kind: 'array'; items: Value[] }
  | { kind: 'object'; entries: { key: string; name: string; value: Value }[] }

export interface FormattedManifest {
  status: 'formatted' | 'empty' | 'invalid' | 'limited'
  text: string
  hasManagedFields: boolean
}

const maxInputLength = 1024 * 1024
const maxOutputLength = 4 * 1024 * 1024
const maxValues = 50_000
const maxDepth = 64

export function formatManifest(
  source: string,
  showManagedFields = false,
): FormattedManifest {
  const fallback = (
    status: FormattedManifest['status'],
  ): FormattedManifest => ({
    status,
    text: source,
    hasManagedFields: false,
  })
  if (source.length > maxInputLength) return fallback('limited')
  if (!source.trim()) return fallback('empty')

  // 不將數字轉成 JavaScript number，避免大整數、負零及指數寫法失真。
  const tokenPattern =
    /"(?:[^"\\]|\\(?:["\\/bfnrt]|u[0-9a-fA-F]{4}))*"|-?(?:0|[1-9]\d*)(?:\.\d+)?(?:[eE][+-]?\d+)?|true|false|null|[{}[\],:]/y
  const whitespace = /[ \t\r\n]*/y
  let offset = 0
  let count = 0
  const next = (): string | undefined => {
    whitespace.lastIndex = offset
    whitespace.exec(source)
    offset = whitespace.lastIndex
    if (offset === source.length) return undefined
    tokenPattern.lastIndex = offset
    const token = tokenPattern.exec(source)
    if (!token) throw new SyntaxError()
    offset = tokenPattern.lastIndex
    if (token[0].startsWith('"')) {
      for (const character of token[0]) {
        if (character.charCodeAt(0) < 32) throw new SyntaxError()
      }
    }
    return token[0]
  }
  const parse = (token: string | undefined, depth: number): Value => {
    if (++count > maxValues || depth > maxDepth) throw new RangeError()
    if (token === '{') {
      const entries: Extract<Value, { kind: 'object' }>['entries'] = []
      let key = next()
      if (key === '}') return { kind: 'object', entries }
      while (key?.startsWith('"')) {
        if (next() !== ':') throw new SyntaxError()
        // 僅解碼屬性名稱做精確路徑比對；顯示仍沿用原始 key token。
        entries.push({
          key,
          name: JSON.parse(key),
          value: parse(next(), depth + 1),
        })
        const separator = next()
        if (separator === '}') return { kind: 'object', entries }
        if (separator !== ',') throw new SyntaxError()
        key = next()
      }
      throw new SyntaxError()
    }
    if (token === '[') {
      const items: Value[] = []
      let item = next()
      if (item === ']') return { kind: 'array', items }
      while (item !== undefined) {
        items.push(parse(item, depth + 1))
        const separator = next()
        if (separator === ']') return { kind: 'array', items }
        if (separator !== ',') throw new SyntaxError()
        item = next()
      }
      throw new SyntaxError()
    }
    if (token === undefined || (token.length === 1 && '{}[],:'.includes(token)))
      throw new SyntaxError()
    return { kind: 'atom', raw: token }
  }

  try {
    const root = parse(next(), 0)
    if (next() !== undefined) throw new SyntaxError()
    const hasManagedFields =
      root.kind === 'object' &&
      root.entries.some(
        (entry) =>
          entry.name === 'metadata' &&
          entry.value.kind === 'object' &&
          entry.value.entries.some((field) => field.name === 'managedFields'),
      )
    const chunks: string[] = []
    let outputLength = 0
    const append = (text: string) => {
      outputLength += text.length
      if (outputLength > maxOutputLength) throw new RangeError()
      chunks.push(text)
    }
    const render = (
      value: Value,
      depth: number,
      rootMetadata = false,
    ): void => {
      if (value.kind === 'atom') {
        append(value.raw)
        return
      }
      const isObject = value.kind === 'object'
      const entries = isObject
        ? value.entries.filter(
            (entry) =>
              showManagedFields ||
              !rootMetadata ||
              entry.name !== 'managedFields',
          )
        : []
      const length = isObject ? entries.length : value.items.length
      append(isObject ? '{' : '[')
      for (let index = 0; index < length; index += 1) {
        append(`${index ? ',' : ''}\n${'  '.repeat(depth + 1)}`)
        if (isObject) {
          const entry = entries[index]
          append(`${entry.key}: `)
          render(
            entry.value,
            depth + 1,
            depth === 0 && entry.name === 'metadata',
          )
        } else {
          render(value.items[index], depth + 1)
        }
      }
      if (length) append(`\n${'  '.repeat(depth)}`)
      append(isObject ? '}' : ']')
    }
    render(root, 0)
    return { status: 'formatted', text: chunks.join(''), hasManagedFields }
  } catch (error) {
    return fallback(error instanceof RangeError ? 'limited' : 'invalid')
  }
}
