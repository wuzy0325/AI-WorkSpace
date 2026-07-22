export type ProbeReferencePoint = { x: number; y: number; z: number }

const point = (x: number, y: number, z: number): ProbeReferencePoint => ({ x, y, z })
const radians = (degrees: number) => degrees * Math.PI / 180

export function createProbeReferenceGeometry(alphaDegrees: number, betaDegrees: number) {
  const alpha = radians(alphaDegrees)
  const beta = radians(betaDegrees)
  const alphaProjection = point(Math.cos(alpha), 0, Math.sin(alpha))
  const flow = point(
    alphaProjection.x * Math.cos(beta),
    -Math.sin(beta),
    alphaProjection.z * Math.cos(beta),
  )

  return { flow, alphaProjection }
}
