import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { reactive, ref } from 'vue'
import { mount } from '@vue/test-utils'
import type { ProbeId, TraversalTestStatus } from '@shared/types/traversal'

import DualProbeRow from '../DualProbeRow.vue'

// 可变的会话容器：storeToRefs mock 用 reactive 包装，测试中直接改 state.sessions
// 的字段即可驱动组件 computed 重新求值（probe1/probe2 两路独立）。
const state = vi.hoisted(() => ({
  sessions: {
    probe1: { status: null as TraversalTestStatus | null, config: null },
    probe2: { status: null as TraversalTestStatus | null, config: null },
  },
}))

vi.mock('pinia', async (importOriginal) => ({
  ...(await importOriginal<typeof import('pinia')>()),
  storeToRefs: () => ({ sessions: ref(reactive(state.sessions)) }),
}))
vi.mock('@stores/dualTraversalStore', () => ({ useDualTraversalStore: () => state }))
vi.mock('@stores/i18nStore', () => ({
  useI18nStore: () => ({
    t: {
      probe1Label: 'Probe 1',
      probe2Label: 'Probe 2',
      travWaitingAcquisition: '等待设备恢复采集',
      travWaitingAcquisitionDevice: '等待设备 {name} 恢复采集',
      travWaitingReconnect: '设备 {name} 已断开，请重新连接并启动采集',
      travWaitingCount: '等 {count} 台设备',
      travWaitingSince: '已等待 {duration}s',
      realtimePressureData: '实时压力数据',
      pleaseConfigureLayout: '请先配置布局',
      pointsPreview: '点位预览',
    },
  }),
}))

const BASE_TIME = new Date('2026-01-01T12:00:00Z')

function waitingStatus(overrides: Partial<TraversalTestStatus> = {}): TraversalTestStatus {
  return {
    taskId: 'probe-task',
    state: 'running',
    status: 'running',
    totalPoints: 5,
    completedPoints: 0,
    progress: 0,
    waitingForAcquisition: true,
    waitingDevices: [{ name: 'dev-1', state: 'stopped' }],
    waitingForAcquisitionSinceMs: Date.now() - 5000,
    ...overrides,
  }
}

function mountRow(probeId: ProbeId = 'probe1') {
  return mount(DualProbeRow, {
    props: { probeId },
    global: { stubs: { DualProbeCompactMonitor: true, PointsPreview: true } },
  })
}

function bannerText(wrapper: ReturnType<typeof mountRow>): string {
  return wrapper.find('.dual-row__wait-banner').text()
}

beforeEach(() => {
  vi.useFakeTimers()
  vi.setSystemTime(BASE_TIME)
})

afterEach(() => {
  state.sessions.probe1.status = null
  state.sessions.probe2.status = null
  vi.useRealTimers()
})

describe('DualProbeRow 等待设备恢复采集横幅（spec-traversal-acquisition-stop）', () => {
  it('stopped 设备显示"等待设备恢复采集"文案与已等待时长', () => {
    state.sessions.probe1.status = waitingStatus()
    const wrapper = mountRow('probe1')

    expect(wrapper.find('.dual-row__wait-banner').exists()).toBe(true)
    expect(bannerText(wrapper)).toContain('等待设备 dev-1 恢复采集')
    expect(bannerText(wrapper)).toContain('已等待 5s')
  })

  it('已等待时长随 1s ticker 更新，不依赖 status 轮询', async () => {
    state.sessions.probe1.status = waitingStatus()
    const wrapper = mountRow('probe1')
    expect(bannerText(wrapper)).toContain('已等待 5s')

    vi.advanceTimersByTime(3000)
    await wrapper.vm.$nextTick()
    expect(bannerText(wrapper)).toContain('已等待 8s')

    wrapper.unmount()
  })

  it('reconnect_required 设备优先级高于 stopped 设备', () => {
    state.sessions.probe1.status = waitingStatus({
      waitingDevices: [
        { name: 'dev-a', state: 'stopped' },
        { name: 'dev-b', state: 'reconnect_required' },
      ],
    })
    const wrapper = mountRow('probe1')

    expect(bannerText(wrapper)).toContain('设备 dev-b 已断开，请重新连接并启动采集')
    expect(bannerText(wrapper)).toContain('等 2 台设备')
  })

  it('paused 状态隐藏横幅（横幅被暂停 UI 取代）', () => {
    state.sessions.probe1.status = waitingStatus({ status: 'paused', state: 'paused' })
    const wrapper = mountRow('probe1')

    expect(wrapper.find('.dual-row__wait-banner').exists()).toBe(false)
  })

  it('后端清空等待字段（waitingForAcquisition=false）后横幅消失', () => {
    state.sessions.probe1.status = waitingStatus({ waitingForAcquisition: false })
    const wrapper = mountRow('probe1')

    expect(wrapper.find('.dual-row__wait-banner').exists()).toBe(false)
  })

  it('probe1/probe2 等待状态相互独立', () => {
    state.sessions.probe1.status = waitingStatus()
    state.sessions.probe2.status = waitingStatus({ waitingForAcquisition: false })

    const wrapper1 = mountRow('probe1')
    const wrapper2 = mountRow('probe2')

    expect(wrapper1.find('.dual-row__wait-banner').exists()).toBe(true)
    expect(wrapper2.find('.dual-row__wait-banner').exists()).toBe(false)
  })
})
