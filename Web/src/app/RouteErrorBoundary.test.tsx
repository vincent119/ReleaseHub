import { cleanup, render, screen } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import i18n from '@/shared/i18n/config'

import { RouteErrorBoundary } from './RouteErrorBoundary'

function Throws({ error }: { error: Error }): never {
  throw error
}

describe('RouteErrorBoundary', () => {
  beforeEach(async () => {
    await i18n.changeLanguage('zh-TW')
    vi.spyOn(console, 'error').mockImplementation(() => undefined)
  })

  afterEach(() => {
    cleanup()
    vi.restoreAllMocks()
  })

  it('preserves the surrounding shell and offers reload after a chunk failure', () => {
    const reload = vi.fn()
    const storage = {
      getItem: () => '1',
      setItem: vi.fn(),
    }

    render(
      <>
        <nav>側邊導覽</nav>
        <RouteErrorBoundary
          buildIdentity="old-build"
          storage={storage}
          reload={reload}
        >
          <Throws
            error={
              new TypeError(
                'Failed to fetch dynamically imported module: /assets/old.js',
              )
            }
          />
        </RouteErrorBoundary>
      </>,
    )

    expect(screen.getByText('側邊導覽')).toBeInTheDocument()
    expect(screen.getByText('ReleaseHub 已更新')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '重新載入' })).toBeInTheDocument()
    expect(reload).not.toHaveBeenCalled()
  })

  it('uses a generic fallback without reloading for render errors', () => {
    const reload = vi.fn()

    render(
      <RouteErrorBoundary buildIdentity="build" reload={reload}>
        <Throws error={new Error('Component render failed')} />
      </RouteErrorBoundary>,
    )

    expect(screen.getByText('無法顯示此頁面')).toBeInTheDocument()
    expect(screen.queryByText('ReleaseHub 已更新')).not.toBeInTheDocument()
    expect(reload).not.toHaveBeenCalled()
  })
})
