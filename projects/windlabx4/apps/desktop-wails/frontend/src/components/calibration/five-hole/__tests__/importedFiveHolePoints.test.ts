import { afterEach, describe, expect, it } from 'vitest'
import {
  clearImportedFiveHolePoints,
  getImportedFiveHolePoints,
  setImportedFiveHolePoints,
} from '../importedFiveHolePoints'

describe('imported five-hole point session override', () => {
  afterEach(() => clearImportedFiveHolePoints())

  it('preserves file order and duplicate points', () => {
    const points = [
      { id: 1, coordinates: { α: 5, β: -10 } },
      { id: 2, coordinates: { α: 0, β: 0 } },
      { id: 3, coordinates: { α: 5, β: -10 } },
    ]

    setImportedFiveHolePoints('custom.csv', points)

    expect(getImportedFiveHolePoints()).toEqual({ fileName: 'custom.csv', points })
  })

  it('returns defensive copies', () => {
    setImportedFiveHolePoints('custom.txt', [{ id: 1, coordinates: { α: 5, β: -10 } }])

    const first = getImportedFiveHolePoints()
    first!.points[0]!.coordinates['α'] = 99

    expect(getImportedFiveHolePoints()!.points[0]!.coordinates['α']).toBe(5)
  })
})
