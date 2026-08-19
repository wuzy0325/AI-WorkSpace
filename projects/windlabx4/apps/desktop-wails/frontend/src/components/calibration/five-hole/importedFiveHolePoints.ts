import type { CalibrationPoint } from '@shared/types/calibration'

interface ImportedFiveHolePoints {
  fileName: string
  points: CalibrationPoint[]
}

let importedPoints: ImportedFiveHolePoints | null = null

function clonePoints(points: CalibrationPoint[]): CalibrationPoint[] {
  return points.map((point) => ({
    id: point.id,
    coordinates: { ...point.coordinates },
  }))
}

export function setImportedFiveHolePoints(fileName: string, points: CalibrationPoint[]): void {
  importedPoints = { fileName, points: clonePoints(points) }
}

export function getImportedFiveHolePoints(): ImportedFiveHolePoints | null {
  if (!importedPoints) return null
  return { fileName: importedPoints.fileName, points: clonePoints(importedPoints.points) }
}

export function clearImportedFiveHolePoints(): void {
  importedPoints = null
}
