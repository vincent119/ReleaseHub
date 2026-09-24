const reloadKeyPrefix = 'releasehub:route-chunk-reload:'

export function isChunkLoadError(error: unknown): boolean {
  if (!(error instanceof Error)) return false

  const message = error.message
  return (
    /Failed to fetch dynamically imported module/i.test(message) ||
    /error loading dynamically imported module/i.test(message) ||
    /Importing a module script failed/i.test(message) ||
    /Unable to preload CSS for /i.test(message) ||
    (error.name === 'ChunkLoadError' &&
      /Loading (?:CSS )?chunk .+ failed/i.test(message))
  )
}

export function reloadOnceForBuild(
  buildIdentity: string,
  storage: Pick<Storage, 'getItem' | 'setItem'>,
  reload: () => void,
): boolean {
  const key = `${reloadKeyPrefix}${buildIdentity}`

  try {
    if (storage.getItem(key) !== null) return false
    // 若儲存失敗就不重新載入，避免隱私模式下形成無限循環。
    storage.setItem(key, '1')
  } catch {
    return false
  }

  reload()
  return true
}
