import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { describe, expect, it } from 'vitest'

const mainSource = readFileSync(resolve(__dirname, '../DualTraversalMain.vue'), 'utf8')
const rowSource = readFileSync(resolve(__dirname, '../DualProbeRow.vue'), 'utf8')
const monitorSource = readFileSync(resolve(__dirname, '../DualProbeCompactMonitor.vue'), 'utf8')

describe('dual traversal live-data hierarchy', () => {
  it('does not render the duplicate top status summary', () => {
    expect(mainSource).not.toContain("import DualStatusBar from './DualStatusBar.vue'")
    expect(mainSource).not.toContain('<DualStatusBar')
  })

  it('labels the fixed interpolated metrics as realtime interpolation data', () => {
    expect(monitorSource).toContain('{{ t.realtimeCalculation }}')
  })

  it('renders realtime pressure and point layout together without tabs', () => {
    expect(rowSource).not.toContain('type DetailTab')
    expect(rowSource).not.toContain('role="tablist"')
    expect(rowSource).toContain('dual-row__live-data')
    expect(rowSource).toContain('<PointsPreview')
  })

  it('does not label the atmospheric temperature channel as pressure', () => {
    expect(rowSource).toContain("unit: ch.role?.endsWith('.tAtm') ? '°C' : 'Pa'")
    expect(rowSource).toContain('{{ row.unit }}')
  })
})
