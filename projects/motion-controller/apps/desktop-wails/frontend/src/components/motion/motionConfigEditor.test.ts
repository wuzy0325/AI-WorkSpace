import { describe, expect, it } from 'vitest'

import {
  normalizeAxisForMotionController,
  applyAxisKindDefaults,
  DEFAULT_GEAR_RATIO,
  DEFAULT_LEAD,
  DEFAULT_ROTARY_GEAR_RATIO,
  type AxisConfig,
} from './motionConfigEditor'

describe('normalizeAxisForMotionController', () => {
  it('uses the project default for legacy simulated profiles', () => {
    const axis = normalizeAxisForMotionController({ name: 'X', enabled: true, kind: 'LINEAR' }, 'SIMULATED-MC')

    expect(axis.maxSpeed).toBe(10)
  })

  it('uses the hardware-safe default for legacy hardware profiles', () => {
    const axis = normalizeAxisForMotionController({ name: 'X', enabled: true, kind: 'LINEAR' }, 'B140-MC')

    expect(axis.maxSpeed).toBe(4)
  })

  it('preserves an explicitly configured speed', () => {
    const axis = normalizeAxisForMotionController({ name: 'X', enabled: true, kind: 'LINEAR', maxSpeed: 2 }, 'B140-MC')

    expect(axis.maxSpeed).toBe(2)
  })

  // 旋转轴默认传动比 180（产品出厂常用减速比），直线轴默认 1。
  // 覆盖共享模块 defaultGearRatioForKind 的行为，避免后续修改默认值时静默漂移。
  it('fills gearRatio=180 for ROTARY axis when missing', () => {
    const axis = normalizeAxisForMotionController({ name: 'U', enabled: true, kind: 'ROTARY' }, 'B140-MC')

    expect(axis.gearRatio).toBe(DEFAULT_ROTARY_GEAR_RATIO)
    expect(axis.gearRatio).toBe(180)
  })

  it('fills gearRatio=1 for LINEAR axis when missing', () => {
    const axis = normalizeAxisForMotionController({ name: 'X', enabled: true, kind: 'LINEAR' }, 'B140-MC')

    expect(axis.gearRatio).toBe(DEFAULT_GEAR_RATIO)
    expect(axis.gearRatio).toBe(1)
  })

  it('preserves an explicitly configured gearRatio', () => {
    const axis = normalizeAxisForMotionController({ name: 'U', enabled: true, kind: 'ROTARY', gearRatio: 50 }, 'B140-MC')

    expect(axis.gearRatio).toBe(50)
  })
})

// 轴类型切换联动规则（AxisConfigCard.vue 共用纯函数）：
// 切 ROTARY 仅当 gearRatio 仍为默认 1 时填 180；切 LINEAR 仅当 lead 为空时填 4。
describe('applyAxisKindDefaults', () => {
  it('fills gearRatio=180 when switching to ROTARY with default gearRatio', () => {
    const input: AxisConfig = { name: 'X', enabled: true, kind: 'LINEAR', gearRatio: DEFAULT_GEAR_RATIO }
    const axis = applyAxisKindDefaults(input, 'ROTARY')

    expect(axis.kind).toBe('ROTARY')
    expect(axis.gearRatio).toBe(DEFAULT_ROTARY_GEAR_RATIO)
  })

  it('fills gearRatio=180 when switching to ROTARY with gearRatio missing', () => {
    const input: AxisConfig = { name: 'X', enabled: true, kind: 'LINEAR' }
    const axis = applyAxisKindDefaults(input, 'ROTARY')

    expect(axis.gearRatio).toBe(DEFAULT_ROTARY_GEAR_RATIO)
  })

  it('preserves a customized gearRatio when switching to ROTARY', () => {
    const input: AxisConfig = { name: 'X', enabled: true, kind: 'LINEAR', gearRatio: 50 }
    const axis = applyAxisKindDefaults(input, 'ROTARY')

    expect(axis.gearRatio).toBe(50)
  })

  it('fills lead=4 when switching to LINEAR with lead missing', () => {
    const input: AxisConfig = { name: 'U', enabled: true, kind: 'ROTARY', gearRatio: 180 }
    const axis = applyAxisKindDefaults(input, 'LINEAR')

    expect(axis.kind).toBe('LINEAR')
    expect(axis.lead).toBe(DEFAULT_LEAD)
  })

  it('preserves an existing lead when switching to LINEAR', () => {
    const input: AxisConfig = { name: 'U', enabled: true, kind: 'ROTARY', lead: 10 }
    const axis = applyAxisKindDefaults(input, 'LINEAR')

    expect(axis.lead).toBe(10)
  })

  it('does not mutate the input axis', () => {
    const input = { name: 'X' as const, enabled: true, kind: 'LINEAR' as const, gearRatio: 1 }
    applyAxisKindDefaults(input, 'ROTARY')

    expect(input.kind).toBe('LINEAR')
    expect(input.gearRatio).toBe(1)
  })
})
