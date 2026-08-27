import { beforeEach, describe, expect, it, vi } from 'vitest'

import {
  getMeasurementParamsConfig,
  saveMeasurementParamsConfig,
  type MeasurementParamsPayload
} from '../measurement'
import * as clientApi from '../client'
import { ControlMode, PressureMode } from '@/types/calibration'

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

describe('measurement api config endpoints', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('loads measurement params from dedicated endpoint', async () => {
    const payload: MeasurementParamsPayload = {
      minPressure: 0,
      maxPressure: 100,
      pointCount: 6,
      precision: 2,
      averageCount: 1,
      stableDurationMs: 5000,
      precisionLevel: 0.05,
      pressureMode: PressureMode.Single,
      controlMode: ControlMode.Auto
    }

    vi.mocked(clientApi.requestJSON).mockResolvedValue({ data: payload })

    const result = await getMeasurementParamsConfig()

    expect(result).toEqual(payload)
    expect(clientApi.requestJSON).toHaveBeenCalledWith('/config/measurement')
  })

  it('saves measurement params to dedicated endpoint', async () => {
    const payload: MeasurementParamsPayload = {
      minPressure: 10,
      maxPressure: 200,
      pointCount: 8,
      precision: 3,
      averageCount: 2,
      stableDurationMs: 7000,
      precisionLevel: 0.1,
      pressureMode: PressureMode.RoundTrip,
      controlMode: ControlMode.Manual
    }

    vi.mocked(clientApi.requestJSON).mockResolvedValue({ data: { status: 'ok' } })

    await saveMeasurementParamsConfig(payload)

    expect(clientApi.requestJSON).toHaveBeenCalledWith('/config/measurement', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(payload)
    })
  })
})
