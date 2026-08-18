import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { describe, expect, it } from 'vitest'

describe('TraversalMain start gate', () => {
  it('disables start when the configured positioner is not connected', () => {
    const source = readFileSync(resolve(__dirname, '../TraversalMain.vue'), 'utf8')
    const startGate = source.slice(
      source.indexOf('const startDisabledReason = computed'),
      source.indexOf('// 实时插值节流')
    )

    expect(startGate).toContain("positionerConnection.value.state")
    expect(startGate).toContain('wf_motionControllerDisconnected')

    const confirmStart = source.slice(
      source.indexOf('async function confirmStartTest'),
      source.indexOf('/** 取消启动确认对话框 */')
    )
    expect(confirmStart).toContain('startDisabledReason.value')
  })
})
