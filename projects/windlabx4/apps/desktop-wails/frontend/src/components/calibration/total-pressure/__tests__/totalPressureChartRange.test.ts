import { describe, it, expect } from 'vitest'
import {
  resolveTotalPressureYRange,
  TOTAL_PRESSURE_Y_DEFAULT_MIN,
  TOTAL_PRESSURE_Y_DEFAULT_MAX,
} from '../totalPressureChartRange'

describe('resolveTotalPressureYRange', () => {
  it('returns the default window [0.99, 1.01] when there is no data', () => {
    expect(resolveTotalPressureYRange([], null)).toEqual({
      min: TOTAL_PRESSURE_Y_DEFAULT_MIN,
      max: TOTAL_PRESSURE_Y_DEFAULT_MAX,
    })
  })

  it('keeps the exact default window when all data points fall inside it (no padding)', () => {
    expect(resolveTotalPressureYRange([0.995, 1.002, 1.005], null)).toEqual({
      min: TOTAL_PRESSURE_Y_DEFAULT_MIN,
      max: TOTAL_PRESSURE_Y_DEFAULT_MAX,
    })
  })

  it('expands the window with 10% padding when data falls below the default min', () => {
    const result = resolveTotalPressureYRange([0.985, 1.0, 1.005], null)
    // 数据最低 0.985，超出默认窗口 0.99 → 范围 [0.985 - pad, 1.01 + pad]
    const pad = (1.01 - 0.985) * 0.1
    expect(result.min).toBeCloseTo(0.985 - pad, 6)
    expect(result.max).toBeCloseTo(1.01 + pad, 6)
  })

  it('expands the window with 10% padding when data rises above the default max', () => {
    const result = resolveTotalPressureYRange([0.995, 1.015], null)
    const pad = (1.015 - 0.99) * 0.1
    expect(result.min).toBeCloseTo(0.99 - pad, 6)
    expect(result.max).toBeCloseTo(1.015 + pad, 6)
  })

  it('honors a valid manual override and skips padding', () => {
    expect(resolveTotalPressureYRange([0.985, 1.015], { min: 0.95, max: 1.05 })).toEqual({
      min: 0.95,
      max: 1.05,
    })
  })

  it('falls back to auto mode when the override is invalid (min >= max)', () => {
    expect(resolveTotalPressureYRange([], { min: 1.01, max: 0.99 })).toEqual({
      min: TOTAL_PRESSURE_Y_DEFAULT_MIN,
      max: TOTAL_PRESSURE_Y_DEFAULT_MAX,
    })
  })

  it('falls back to auto mode when the override contains a non-finite number', () => {
    expect(resolveTotalPressureYRange([], { min: Number.NaN, max: 1.01 })).toEqual({
      min: TOTAL_PRESSURE_Y_DEFAULT_MIN,
      max: TOTAL_PRESSURE_Y_DEFAULT_MAX,
    })
  })
})