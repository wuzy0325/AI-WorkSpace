import { describe, it, expect } from 'vitest'
import { resolveKAlphaKbetaBounds } from '../fiveHoleChartUtils'

describe('resolveKAlphaKbetaBounds', () => {
  it('returns the default symmetric [-1, 1] window in auto mode when there is no data', () => {
    expect(resolveKAlphaKbetaBounds([])).toEqual({
      xMin: -1,
      xMax: 1,
      yMin: -1,
      yMax: 1,
      tickCount: 4,
      isManual: false,
    })
  })

  it('returns symmetric origin-centered bounds in auto mode with data', () => {
    const result = resolveKAlphaKbetaBounds([
      { x: 0.5, y: 0.5 },
      { x: -0.2, y: 0.3 },
    ])
    expect(result.isManual).toBe(false)
    // x: 数据范围 [-0.2, 0.5] + 10% margin + 包含原点 + 对称化 → ±0.57
    expect(result.xMin).toBeCloseTo(-0.57, 6)
    expect(result.xMax).toBeCloseTo(0.57, 6)
    // y: 数据范围 [0.3, 0.5] + 10% margin + 包含原点 + 对称化 → ±0.52
    expect(result.yMin).toBeCloseTo(-0.52, 6)
    expect(result.yMax).toBeCloseTo(0.52, 6)
  })

  it('honors a valid manual override for both axes and skips symmetric/margin', () => {
    const result = resolveKAlphaKbetaBounds(
      [{ x: 0.5, y: 0.5 }],
      { min: -2, max: 3 },
      { min: -1, max: 1 },
    )
    expect(result).toEqual({
      xMin: -2,
      xMax: 3,
      yMin: -1,
      yMax: 1,
      tickCount: 4,
      isManual: true,
    })
  })

  it('falls back to auto mode when the override is invalid (min >= max)', () => {
    const result = resolveKAlphaKbetaBounds([], { min: 2, max: 1 }, { min: -1, max: 1 })
    expect(result.isManual).toBe(false)
    expect(result.xMin).toBe(-1)
    expect(result.xMax).toBe(1)
  })

  it('falls back to auto mode when the override contains a non-finite number', () => {
    const result = resolveKAlphaKbetaBounds([], { min: Number.NaN, max: 1 }, { min: -1, max: 1 })
    expect(result.isManual).toBe(false)
    expect(result.yMin).toBe(-1)
    expect(result.yMax).toBe(1)
  })

  it('falls back to auto mode when only one axis is overridden', () => {
    const result = resolveKAlphaKbetaBounds([], { min: -2, max: 2 }, null)
    expect(result.isManual).toBe(false)
    expect(result.xMin).toBe(-1)
    expect(result.xMax).toBe(1)
  })
})