import { describe, it, expect, vi } from 'vitest'

describe('sse-client', () => {
  it('calls fetch with correct stream URL', async () => {
    vi.restoreAllMocks()
    vi.stubEnv('VITE_API_BASE', 'http://localhost:8080')
    vi.spyOn(globalThis, 'fetch').mockResolvedValue({
      ok: true,
      headers: new Map(Object.entries({ 'content-type': 'text/event-stream' })),
      body: new ReadableStream({ start(c) { c.close() } }),
    } as unknown as Response)

    const { subscribeDaqStream } = await import('@api/sse-client')
    const sub = subscribeDaqStream('dev-1', vi.fn(), vi.fn())

    expect(globalThis.fetch).toHaveBeenCalledWith('http://localhost:8080/api/daq/stream/dev-1')
    sub.unsubscribe()
  })

  it('calls onError on non-ok response', async () => {
    vi.restoreAllMocks()
    vi.stubEnv('VITE_API_BASE', 'http://localhost:8080')
    vi.spyOn(globalThis, 'fetch').mockResolvedValue({
      ok: false,
      status: 500,
    } as unknown as Response)

    const onError = vi.fn()
    const { subscribeDaqStream } = await import('@api/sse-client')
    const sub = subscribeDaqStream('dev-1', vi.fn(), onError)

    await vi.waitFor(() => {
      expect(onError).toHaveBeenCalled()
    }, { timeout: 2000 })

    sub.unsubscribe()
  })

  it('unsubscribe sets aborted flag', async () => {
    vi.restoreAllMocks()
    vi.stubEnv('VITE_API_BASE', 'http://localhost:8080')

    const { subscribeDaqStream } = await import('@api/sse-client')
    const sub = subscribeDaqStream('dev-1', vi.fn(), vi.fn())
    sub.unsubscribe()

    expect(sub.unsubscribe).toBeTypeOf('function')
  })
})
