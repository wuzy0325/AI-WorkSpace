import { describe, expect, it } from 'vitest'
import {
  getTraversalLayoutPoints,
  normalizeTraversalLayoutRanges,
  type TraversalLayout
} from '@shared/types/traversal'

describe('normalizeTraversalLayoutRanges', () => {
  it('uses rectangle segment ranges instead of stale persisted bounds', () => {
    const layout: TraversalLayout = {
      pattern: 'rectangle',
      primaryAxis: 'x',
      rectangle: {
        xMin: -30,
        xMax: 30,
        xStepSegments: [{ start: 30, end: 50, step: 10 }],
        yMin: -30,
        yMax: 30,
        yStepSegments: [{ start: -30, end: 30, step: 30 }]
      }
    }

    const directPoints = getTraversalLayoutPoints(layout)
    const normalized = normalizeTraversalLayoutRanges(layout)
    const points = getTraversalLayoutPoints(normalized)

    expect(normalized.rectangle).toMatchObject({
      xMin: 30,
      xMax: 50,
      yMin: -30,
      yMax: 30
    })
    expect(directPoints).toEqual(points)
    expect(points).toHaveLength(9)
    expect([...new Set(points.map((point) => point.x))]).toEqual([30, 40, 50])
    expect([...new Set(points.map((point) => point.y))]).toEqual([-30, 0, 30])
  })

  it('leaves non-rectangle layouts unchanged', () => {
    const layout: TraversalLayout = {
      pattern: 'custom',
      custom: { points: [{ x: 1, y: 2, z: 3, u: 4 }] }
    }

    expect(normalizeTraversalLayoutRanges(layout)).toBe(layout)
  })

  it('repairs a persisted rectangle that collapsed to one X column', () => {
    const layout: TraversalLayout = {
      pattern: 'rectangle',
      rectangle: {
        xMin: 30,
        xMax: 30,
        xStepSegments: [{ start: 30, end: 30, step: 5 }],
        yMin: -30,
        yMax: 30,
        yStepSegments: [{ start: -30, end: 30, step: 5 }]
      }
    }

    const normalized = normalizeTraversalLayoutRanges(layout)
    const points = getTraversalLayoutPoints(layout)

    expect(normalized.rectangle?.xStepSegments).toEqual([{ start: -30, end: 30, step: 5 }])
    expect(points).toHaveLength(169)
    expect([...new Set(points.map((point) => point.x))]).toHaveLength(13)
    expect([...new Set(points.map((point) => point.y))]).toHaveLength(13)
  })
})
