import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { describe, expect, it } from 'vitest'

// 遍历等待设备恢复采集横幅（spec-traversal-acquisition-stop）源码契约：
// TraversalMain 体量大、依赖多，沿用仓库对 TraversalMain 的 source-contract 测试风格
// （TraversalMainStartGate.contract.test.ts），行为层由 traversalStore 等待字段测试
// + DualProbeRow 挂载测试覆盖（两组件共享同一段文案逻辑）。
describe('TraversalMain 等待横幅（spec-traversal-acquisition-stop）', () => {
  const source = readFileSync(resolve(__dirname, '../TraversalMain.vue'), 'utf8')
  const bannerBlock = source.slice(
    source.indexOf('// 等待设备恢复采集文案'),
    source.indexOf('// 实时插值节流')
  )

  it('暂停时隐藏横幅（横幅被暂停 UI 取代）', () => {
    expect(bannerBlock).toContain('status.status === \'paused\'')
    expect(bannerBlock).toContain('if (!status?.waitingForAcquisition || status.status === \'paused\') return \'\'')
  })

  it('主导设备按 reconnect_required > stopped 优先级选择', () => {
    expect(bannerBlock).toContain("devices.find((d) => d.state === 'reconnect_required') ?? devices[0]")
  })

  it('复用 1s ticker（now）计算并展示已等待时长', () => {
    expect(bannerBlock).toContain('waitingForAcquisitionSinceMs')
    expect(bannerBlock).toContain('now.value')
    expect(bannerBlock).toContain('travWaitingSince')
    expect(bannerBlock).toContain('Math.floor((now.value - since) / 1000)')
  })

  it('横幅模板以 waitingAcquisitionText 为显示条件', () => {
    const template = source.slice(source.indexOf('<template>'))
    const vif = template.indexOf('v-if="waitingAcquisitionText"')
    expect(vif).toBeGreaterThan(-1)
    expect(template.slice(vif, vif + 300)).toContain('trav-wait-banner')
  })
})
