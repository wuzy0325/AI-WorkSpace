import { describe, it, expect } from 'vitest'
import { parseLogSseChunk } from '@api/logSseClient'

describe('logSseClient', () => {
  it('parses log events split across stream chunks', () => {
    const entries: unknown[] = []
    const state = { buffer: '', event: '', data: '' }

    parseLogSseChunk(state, 'event: log\n', (entry) => entries.push(entry))
    parseLogSseChunk(state, 'data: {"id":1,"timestamp":"2026-06-30T00:00:00Z",', (entry) => entries.push(entry))
    parseLogSseChunk(state, '"level":"info","source":"app","message":"ready"}\n\n', (entry) => entries.push(entry))

    expect(entries).toEqual([
      {
        id: 1,
        timestamp: '2026-06-30T00:00:00Z',
        level: 'info',
        source: 'app',
        message: 'ready',
      },
    ])
  })
})
