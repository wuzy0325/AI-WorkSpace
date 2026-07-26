import { createPinia, setActivePinia } from 'pinia'
import { computed, ref } from 'vue'
import { describe, expect, it } from 'vitest'

import { useHardwareConnectionStatus } from '../useHardwareConnectionStatus'
import { useMotionStore } from '../../stores/motionStore'
import type { TraversalTestConfig } from '../../shared/types/traversal'

describe('useHardwareConnectionStatus', () => {
  it('矩形遍历只按参与运动的 X/Y 判断位移机构连接状态', () => {
    setActivePinia(createPinia())
    const motionStore = useMotionStore()
    motionStore.statusList = [{
      id: 'connected-controller',
      name: 'B140',
      type: 'B140-MC',
      connected: true,
      emergencyStopped: false,
      // 必须给出 X/Y 轴条目：positionerConnection 复刻 validateMotionAxisConnections，
      // 会在 statuses 中查找 binding.axis；axes 为空时所有 binding 都找不到匹配 → disconnected。
      axes: [
        { name: 'X', position: 0, moving: false, homed: true },
        { name: 'Y', position: 0, moving: false, homed: true }
      ]
    }]
    const config = ref({
      layout: { pattern: 'rectangle' },
      channels: {
        probeChannels: [],
        motionAxes: [
          { name: 'X', controllerId: 'connected-controller', axis: 'X' },
          { name: 'Y', controllerId: 'connected-controller', axis: 'Y' },
          { name: 'Z', controllerId: 'removed-controller', axis: 'Z' },
          { name: 'U', controllerId: 'removed-controller', axis: 'U' }
        ]
      }
    } as unknown as TraversalTestConfig)

    const { positionerConnection } = useHardwareConnectionStatus(computed(() => config.value))

    expect(positionerConnection.value.state).toBe('connected')
  })

  it.each(['X', 'Y'] as const)('%s 控制器离线时位移机构状态为 disconnected', (offlineAxis) => {
    setActivePinia(createPinia())
    const motionStore = useMotionStore()
    motionStore.statusList = [
      {
        id: 'x-controller',
        name: 'X Controller',
        type: 'B140-MC',
        connected: offlineAxis !== 'X',
        emergencyStopped: false,
        axes: [{ name: 'X', position: 0, moving: false, homed: true }]
      },
      {
        id: 'y-controller',
        name: 'Y Controller',
        type: 'B140-MC',
        connected: offlineAxis !== 'Y',
        emergencyStopped: false,
        axes: [{ name: 'Y', position: 0, moving: false, homed: true }]
      }
    ]
    const config = ref({
      layout: { pattern: 'rectangle' },
      channels: {
        probeChannels: [],
        motionAxes: [
          { name: 'X', controllerId: 'x-controller', axis: 'X' },
          { name: 'Y', controllerId: 'y-controller', axis: 'Y' }
        ]
      }
    } as unknown as TraversalTestConfig)

    const { positionerConnection } = useHardwareConnectionStatus(computed(() => config.value))

    expect(positionerConnection.value.state).toBe('disconnected')
  })

  // 与后端 TestCheckPreconditions_MotionAliasFallbackPasses 同一语义：
  // 旧配置保存了别名 / 控制器名 / 旧 UUID，所有 controllerId 都不匹配任何已知控制器，
  // 应回退到按轴名匹配；只要存在一个已连接控制器的同名轴可用，就应判为 connected。
  // 修复前前端用 statusById 严格匹配，会把这种旧配置判定为 disconnected 并禁用启动。
  it('所有 controllerId 都不匹配已知控制器时回退到按轴名匹配（旧别名配置）', () => {
    setActivePinia(createPinia())
    const motionStore = useMotionStore()
    // 后端 profile.ID 是 UUID，前端旧配置保存的是别名 "sim-motion-1"
    motionStore.statusList = [
      {
        id: 'f3c70fdd-0e1c-4d99-86ae-c4c5387e41c2',
        name: '模拟运动控制器',
        type: 'SIMULATED-MC',
        connected: true,
        emergencyStopped: false,
        axes: [
          { name: 'X', position: 0, moving: false, homed: true },
          { name: 'Y', position: 0, moving: false, homed: true }
        ]
      }
    ]
    const config = ref({
      layout: { pattern: 'rectangle' },
      channels: {
        probeChannels: [],
        motionAxes: [
          { name: 'X', controllerId: 'sim-motion-1', axis: 'X' },
          { name: 'Y', controllerId: 'sim-motion-1', axis: 'Y' }
        ]
      }
    } as unknown as TraversalTestConfig)

    const { positionerConnection } = useHardwareConnectionStatus(computed(() => config.value))

    expect(positionerConnection.value.state).toBe('connected')
  })

  // 部分匹配时保持严格绑定：一个 binding 匹配已知控制器、另一个不匹配时，
  // 不触发全局回退；不匹配的 binding 找不到对应轴 → disconnected。
  // 与后端 TestCheckPreconditions_MotionPartialMatchKeepsStrictBinding 同一语义。
  it('部分 controllerId 匹配时不回退，未匹配的 binding 视为 disconnected', () => {
    setActivePinia(createPinia())
    const motionStore = useMotionStore()
    motionStore.statusList = [
      {
        id: 'mc-uuid-1',
        name: '模拟运动控制器',
        type: 'SIMULATED-MC',
        connected: true,
        emergencyStopped: false,
        axes: [{ name: 'X', position: 0, moving: false, homed: true }]
      }
    ]
    const config = ref({
      layout: { pattern: 'rectangle' },
      channels: {
        probeChannels: [],
        motionAxes: [
          { name: 'X', controllerId: 'mc-uuid-1', axis: 'X' },
          { name: 'Y', controllerId: 'sim-motion-1', axis: 'Y' }
        ]
      }
    } as unknown as TraversalTestConfig)

    const { positionerConnection } = useHardwareConnectionStatus(computed(() => config.value))

    expect(positionerConnection.value.state).toBe('disconnected')
  })
})
