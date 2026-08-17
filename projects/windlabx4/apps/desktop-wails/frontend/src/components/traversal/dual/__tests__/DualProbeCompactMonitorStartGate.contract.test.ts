import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { describe, expect, it } from 'vitest'

const monitorSource = readFileSync(resolve(__dirname, '../DualProbeCompactMonitor.vue'), 'utf8')

describe('DualProbeCompactMonitor start gate', () => {
  it('disables start when the configured acquisition device is not acquiring', () => {
    const startGate = monitorSource.slice(
      monitorSource.indexOf('const startDisabledReason = computed'),
      monitorSource.indexOf('const canStart = computed'),
    )

    expect(startGate).toContain('positionerConnection.value.state')
    expect(startGate).toContain('acquisitionConnection.value.state')
    expect(startGate).toContain('wf_acquisitionDeviceNotAcquiring')
  })

  it('keeps canStart gated on the disabled reason and surfaces it as button tooltip', () => {
    const canStart = monitorSource.slice(
      monitorSource.indexOf('const canStart = computed'),
      monitorSource.indexOf('const canPause = computed'),
    )
    expect(canStart).toContain('startDisabledReason.value')

    expect(monitorSource).toContain(':title="startDisabledReason || undefined"')
  })
})
