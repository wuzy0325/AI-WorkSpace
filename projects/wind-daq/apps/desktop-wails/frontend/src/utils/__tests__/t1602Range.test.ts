import { describe, expect, it } from 'vitest'
import {
  T1602_TC_TYPES,
  T1602_DEFAULT_TYPE_CODE,
  t1602RangeForTypeCode,
  getDeviceChannelRange,
} from '@utils/t1602Range'
import type { DeviceProfile } from '@api/types'

describe('t1602RangeForTypeCode', () => {
  it('maps each thermocouple type code to its firmware range', () => {
    for (const type of T1602_TC_TYPES) {
      const range = t1602RangeForTypeCode(type.code)
      expect(range).toEqual({ min: type.min, max: type.max })
    }
  })

  it('falls back to the default T type range for unknown codes', () => {
    expect(t1602RangeForTypeCode(99)).toEqual({ min: 0, max: 1300 })
    expect(t1602RangeForTypeCode(undefined)).toEqual({ min: 0, max: 1300 })
  })

  it('default type code is T (2)', () => {
    expect(T1602_DEFAULT_TYPE_CODE).toBe(2)
  })
})

describe('getDeviceChannelRange', () => {
  it('uses thermocouple type range for DAQ-T-1602 channels', () => {
    const profile: DeviceProfile = {
      id: 't1602',
      name: 'T1602',
      type: 'DAQ-T-1602',
      samplingRate: 5,
      channels: [],
      daqT1602Config: { typeCodes: [0, 1, 2, 3, 4, 5, 6, 7, 2, 2, 2, 2, 2, 2, 2, 2] },
    }
    expect(getDeviceChannelRange(profile, 0)).toEqual({ min: -50, max: 50 }) // J
    expect(getDeviceChannelRange(profile, 3)).toEqual({ min: -200, max: 400 }) // E
    expect(getDeviceChannelRange(profile, 7)).toEqual({ min: 0, max: 1800 }) // N
  })

  it('falls back to default T range for T1602 channel with missing type code', () => {
    const profile: DeviceProfile = {
      id: 't1602',
      name: 'T1602',
      type: 'DAQ-T-1602',
      samplingRate: 5,
      channels: [],
      daqT1602Config: { typeCodes: [] },
    }
    expect(getDeviceChannelRange(profile, 0)).toEqual({ min: 0, max: 1300 })
  })

  it('reads rangeMin/rangeMax for non-T1602 devices', () => {
    const profile: DeviceProfile = {
      id: 'p1604',
      name: 'P1604',
      type: 'DAQ-P-1604',
      samplingRate: 20,
      channels: [{ index: 0, name: 'CH1', enabled: true, unit: 'Pa', precision: 2, rangeMin: -5000, rangeMax: 5000 }],
    }
    expect(getDeviceChannelRange(profile, 0)).toEqual({ min: -5000, max: 5000 })
  })

  it('falls back to ±10 for non-T1602 channel without range', () => {
    const profile: DeviceProfile = {
      id: 'p1604',
      name: 'P1604',
      type: 'DAQ-P-1604',
      samplingRate: 20,
      channels: [{ index: 0, name: 'CH1', enabled: true, unit: 'Pa', precision: 2 }],
    }
    expect(getDeviceChannelRange(profile, 0)).toEqual({ min: -10, max: 10 })
  })

  it('returns ±10 for undefined profile', () => {
    expect(getDeviceChannelRange(undefined, 0)).toEqual({ min: -10, max: 10 })
  })
})
