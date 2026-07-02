import { describe, it, expect, beforeEach } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { useCalibrationStore } from '@stores/calibrationStore'

describe('calibrationStore', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
  })

  it('shows zero aerodynamic values when tunnel total and static pressure are equal', () => {
    const store = useCalibrationStore()

    store.updateRealtimePressures({
      P1: 0,
      P2: 0,
      P3: 0,
      P4: 0,
      P5: 0,
      Patm: 0,
      Tatm: 25,
      P0: 0,
      Ps: 0,
      Ttunnel: 25,
    })

    expect(store.calculatedPhysics).toEqual({ machNumber: 0, velocity: 0 })
  })

  it('uses standard atmosphere fallback for live aerodynamic display when Patm channel is zero', () => {
    const store = useCalibrationStore()

    store.updateRealtimePressures({
      P1: 0,
      P2: 0,
      P3: 0,
      P4: 0,
      P5: 0,
      Patm: 0,
      Tatm: 25,
      P0: 1000,
      Ps: 0,
      Ttunnel: 25,
    })

    expect(store.calculatedPhysics?.machNumber).toBeGreaterThan(0)
    expect(store.calculatedPhysics?.velocity).toBeGreaterThan(0)
  })
})
