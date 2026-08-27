import { describe, expect, it } from 'vitest'

import router from '../index'

describe('router module entry routes', () => {
  it('registers module hub and device management routes', () => {
    expect(router.hasRoute('module-hub')).toBe(true)
    expect(router.hasRoute('module-device-management')).toBe(true)
    expect(router.hasRoute('module-measurement')).toBe(true)
    expect(router.hasRoute('module-calibration')).toBe(true)
  })

  it('maps primary paths to unified module routes', () => {
    expect(router.resolve('/').name).toBe('module-hub')
    expect(router.resolve('/device-management').name).toBe('module-device-management')
    expect(router.resolve('/measurement').name).toBe('module-measurement')
    expect(router.resolve('/calibration').name).toBe('module-calibration')
  })
})
