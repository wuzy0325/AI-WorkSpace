import { describe, expect, it } from 'vitest'
import { createProbeReferenceGeometry } from '../probeReferenceGeometry'

function angleBetween(a: { x: number; y: number; z: number }, b: { x: number; y: number; z: number }) {
  const dot = a.x * b.x + a.y * b.y + a.z * b.z
  const lengthA = Math.hypot(a.x, a.y, a.z)
  const lengthB = Math.hypot(b.x, b.y, b.z)
  return Math.acos(dot / (lengthA * lengthB)) * 180 / Math.PI
}

describe('createProbeReferenceGeometry', () => {
  it('uses beta as the angle between the inflow and its X-Z projection', () => {
    const geometry = createProbeReferenceGeometry(25, 20)

    expect(angleBetween(geometry.flow, geometry.alphaProjection)).toBeCloseTo(20, 10)
  })

  it('preserves alpha as the inflow projection angle in the X-Z plane', () => {
    const geometry = createProbeReferenceGeometry(-35, 40)
    const projectedAlpha = Math.atan2(geometry.alphaProjection.z, geometry.alphaProjection.x) * 180 / Math.PI

    expect(projectedAlpha).toBeCloseTo(-35, 10)
  })
})
