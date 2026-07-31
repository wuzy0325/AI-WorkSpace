import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { describe, expect, it } from 'vitest'

const monitorSource = readFileSync(resolve(__dirname, '../DualProbeCompactMonitor.vue'), 'utf8')

describe('dual traversal checkpoint recovery', () => {
  it('blocks a new start and exposes resume or discard actions for the keyed checkpoint', () => {
    expect(monitorSource).toContain('!session.value.checkpoint')
    expect(monitorSource).toContain('dual-compact__checkpoint')
    expect(monitorSource).toContain('dualStore.resumeFromCheckpoint(props.probeId, checkpoint.taskId)')
    expect(monitorSource).toContain('dualStore.clearCheckpoint(props.probeId, checkpoint.taskId)')
    expect(monitorSource).toContain(':disabled="isCheckpointPending"')
    expect(monitorSource).toContain('{{ t.travContinueTest }}')
    expect(monitorSource).toContain('{{ t.travAbandon }}')
  })
})
