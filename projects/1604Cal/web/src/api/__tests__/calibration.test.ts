import { beforeEach, describe, expect, it, vi } from 'vitest'

import { resolveAlarm } from '../calibration'
import * as clientApi from '../client'

const { mockRequestJSON } = vi.hoisted(() => ({
  mockRequestJSON: vi.fn()
}))
vi.mock('../client', () => ({
  requestJSON: mockRequestJSON,
  apiGet: <T>(path: string): Promise<T> =>
    mockRequestJSON(path).then((r: { data: T }) => r.data),
  apiPost: <T>(path: string, body?: unknown): Promise<T> =>
    mockRequestJSON(path, {
      method: 'POST',
      headers: body !== undefined ? { 'Content-Type': 'application/json' } : undefined,
      body: body !== undefined ? JSON.stringify(body) : undefined
    }).then((r: { data: T }) => r.data)
}))

describe('calibration api resolveAlarm', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(clientApi.requestJSON).mockResolvedValue({ data: { status: 'ok' } })
  })

  it('posts recollect decision to resolve-alarm endpoint', async () => {
    await resolveAlarm('recollect')

    expect(clientApi.requestJSON).toHaveBeenCalledWith('/calibration/resolve-alarm', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ decision: 'recollect' })
    })
  })

  it('posts skip decision to resolve-alarm endpoint', async () => {
    await resolveAlarm('skip')

    expect(clientApi.requestJSON).toHaveBeenCalledWith('/calibration/resolve-alarm', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ decision: 'skip' })
    })
  })
})
