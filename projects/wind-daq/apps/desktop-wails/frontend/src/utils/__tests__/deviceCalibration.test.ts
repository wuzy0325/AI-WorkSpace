import { describe, expect, it } from 'vitest'
import { isChannelCalibrationEnabled } from '@utils/deviceCalibration'

describe('isChannelCalibrationEnabled', () => {
  it('disables DAQ-P-1603 channels whose calibration application is disabled', () => {
    expect(isChannelCalibrationEnabled('DAQ-P-1603', false)).toBe(false)
    expect(isChannelCalibrationEnabled('DAQ-P-1603', true)).toBe(true)
  })

  it('does not apply the DAQ-P-1603 flag to other device types', () => {
    expect(isChannelCalibrationEnabled('DAQ-P-1604', false)).toBe(true)
    expect(isChannelCalibrationEnabled('DSA3217', false)).toBe(true)
  })
})
