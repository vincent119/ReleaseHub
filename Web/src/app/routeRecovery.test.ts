import { describe, expect, it, vi } from 'vitest'

import { isChunkLoadError, reloadOnceForBuild } from './routeRecovery'

describe('route chunk recovery', () => {
  it.each([
    new TypeError(
      'Failed to fetch dynamically imported module: /assets/route.js',
    ),
    new TypeError('error loading dynamically imported module'),
    new TypeError('Importing a module script failed.'),
    new Error('Unable to preload CSS for /assets/route.css'),
    Object.assign(new Error('Loading chunk 42 failed.'), {
      name: 'ChunkLoadError',
    }),
  ])('recognizes a stale chunk failure: %s', (error) => {
    expect(isChunkLoadError(error)).toBe(true)
  })

  it.each([
    new Error('Failed to fetch /api/v1/requests'),
    new Error('Unauthorized'),
    new Error('Component render failed'),
    new Error('Loading chunk 42 failed.'),
    'Failed to fetch dynamically imported module',
  ])('does not classify unrelated failures as a release: %s', (error) => {
    expect(isChunkLoadError(error)).toBe(false)
  })

  it('reloads only once for each build identity', () => {
    const values = new Map<string, string>()
    const storage = {
      getItem: (key: string) => values.get(key) ?? null,
      setItem: (key: string, value: string) => void values.set(key, value),
    }
    const reload = vi.fn()

    expect(reloadOnceForBuild('/assets/app-old.js', storage, reload)).toBe(true)
    expect(reloadOnceForBuild('/assets/app-old.js', storage, reload)).toBe(
      false,
    )
    expect(reloadOnceForBuild('/assets/app-new.js', storage, reload)).toBe(true)
    expect(reload).toHaveBeenCalledTimes(2)
  })

  it('does not reload when storage cannot persist the guard', () => {
    const reload = vi.fn()
    const storage = {
      getItem: () => null,
      setItem: () => {
        throw new Error('Storage unavailable')
      },
    }

    expect(reloadOnceForBuild('/assets/app.js', storage, reload)).toBe(false)
    expect(reload).not.toHaveBeenCalled()
  })
})
