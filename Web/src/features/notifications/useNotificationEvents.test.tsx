import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { act, render } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { useNotificationEvents } from './useNotificationEvents'

class TestEventSource {
  static current?: TestEventSource

  readonly listeners = new Map<string, EventListener>()
  onmessage: (() => void) | null = null
  closed = false

  constructor(readonly url: string) {
    TestEventSource.current = this
  }

  addEventListener(type: string, listener: EventListener) {
    this.listeners.set(type, listener)
  }

  close() {
    this.closed = true
  }

  emit(type: string) {
    this.listeners.get(type)?.(new Event(type))
  }
}

describe('useNotificationEvents', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
    TestEventSource.current = undefined
  })

  it('invalidates notification and deployment queries after an SSE event', async () => {
    vi.stubGlobal('EventSource', TestEventSource)
    const queryClient = new QueryClient()
    const invalidate = vi
      .spyOn(queryClient, 'invalidateQueries')
      .mockResolvedValue(undefined)
    const view = render(
      <QueryClientProvider client={queryClient}>
        <Harness />
      </QueryClientProvider>,
    )

    expect(TestEventSource.current?.url).toBe('/api/v1/notifications/events')
    await act(async () => {
      TestEventSource.current?.emit('deployment.execution.completed')
    })
    expect(invalidate).toHaveBeenCalledTimes(2)

    const source = TestEventSource.current
    view.unmount()
    expect(source?.closed).toBe(true)
  })
})

function Harness() {
  useNotificationEvents(true)
  return null
}
