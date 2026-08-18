import { describe, expect, it } from 'vitest'
import { channelUnit, fixedAtmosphericUnit } from '@utils/channelUnit'

describe('fixedAtmosphericUnit', () => {
  it('returns null for non-atmospheric channels and non-1604 devices', () => {
    expect(fixedAtmosphericUnit('DAQ-P-1604', 0)).toBeNull()
    expect(fixedAtmosphericUnit('DAQ-P-1604', 15)).toBeNull()
    expect(fixedAtmosphericUnit('DSA3217', 16)).toBeNull()
    expect(fixedAtmosphericUnit('SIMULATED', 16)).toBeNull()
  })

  it('locks DAQ-P-1604 CH17 to Pa and CH18 to ℃', () => {
    expect(fixedAtmosphericUnit('DAQ-P-1604', 16)).toBe('Pa')
    expect(fixedAtmosphericUnit('DAQ-P-1604', 17)).toBe('℃')
  })

  it('locks DAQ-P-1604Pre CH17 to Pa and CH18 to ℃', () => {
    expect(fixedAtmosphericUnit('DAQ-P-1604Pre', 16)).toBe('Pa')
    expect(fixedAtmosphericUnit('DAQ-P-1604Pre', 17)).toBe('℃')
  })
})

describe('channelUnit', () => {
  it('uses fallback for regular channels', () => {
    expect(channelUnit('DAQ-P-1604', 0, 'kPa')).toBe('kPa')
    expect(channelUnit('DSA3217', 16, 'Pa')).toBe('Pa')
  })

  it('forces fixed units for atmospheric channels regardless of fallback', () => {
    expect(channelUnit('DAQ-P-1604', 16, 'psi')).toBe('Pa')
    expect(channelUnit('DAQ-P-1604', 17, 'psi')).toBe('℃')
    expect(channelUnit('DAQ-P-1604Pre', 16, 'kPa')).toBe('Pa')
    expect(channelUnit('DAQ-P-1604Pre', 17, 'kPa')).toBe('℃')
  })
})
