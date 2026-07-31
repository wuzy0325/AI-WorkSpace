import { mount } from '@vue/test-utils'
import { defineComponent, nextTick, ref } from 'vue'
import { describe, expect, it, vi } from 'vitest'

import DualTraversalSettings from '../DualTraversalSettings.vue'
import TraversalHardwareStep from '../../TraversalHardwareStep.vue'
import TraversalPrbStep from '../../TraversalPrbStep.vue'
import type { TraversalTestConfig } from '@shared/types/traversal'

const loadedConfigs = vi.hoisted(() => ({
  probe1: null as TraversalTestConfig | null,
  probe2: null as TraversalTestConfig | null,
}))

const dualStore = vi.hoisted(() => ({
  sessions: {
    probe1: { config: null as TraversalTestConfig | null, error: null as string | null },
    probe2: { config: null as TraversalTestConfig | null, error: null as string | null },
  },
  loadConfig: vi.fn(async (probeId: 'probe1' | 'probe2') => {
    await Promise.resolve()
    dualStore.sessions[probeId].config = loadedConfigs[probeId]
  }),
  saveConfig: vi.fn(),
  importPrbFile: vi.fn(),
  importMultiPrbFiles: vi.fn(),
  importCalibrationCsvFile: vi.fn(),
  importSevenHolePrbFiles: vi.fn(),
  importSevenHoleCalibrationCsvFiles: vi.fn(),
  clearInterpolator: vi.fn(),
}))

function deferred<T>() {
  let resolve!: (value: T) => void
  const promise = new Promise<T>((done) => { resolve = done })
  return { promise, resolve }
}

vi.mock('@stores/dualTraversalStore', () => ({ useDualTraversalStore: () => dualStore }))
vi.mock('@stores/i18nStore', () => ({
  useI18nStore: () => ({ t: new Proxy({}, { get: (_target, key) => typeof key === 'symbol' ? undefined : key }) }),
}))
vi.mock('@stores/feedbackStore', () => ({
  useFeedbackStore: () => ({ pushToast: vi.fn(), confirm: vi.fn() }),
}))
vi.mock('@stores/deviceStore', () => ({
  useDeviceStore: () => ({
    profiles: [],
    refreshProfiles: vi.fn(),
    statusFor: vi.fn(() => 'Disconnected'),
    acquiringFor: vi.fn(() => false),
  }),
}))
vi.mock('@stores/motionStore', () => ({
  useMotionStore: () => ({ profiles: [], refreshProfiles: vi.fn() }),
}))
vi.mock('@stores/storageStore', () => ({
  useStorageStore: () => ({ settings: { baseDirectory: '' }, loadSettings: vi.fn(), pickDirectory: vi.fn() }),
}))

function configWithChannel(deviceId: string): TraversalTestConfig {
  return {
    name: deviceId,
    probeType: 'five-hole',
    layout: {
      pattern: 'line',
      line: {
        startX: 10,
        startY: 0,
        endX: 0,
        endY: 0,
        xStepSegments: [{ start: 10, end: 0, step: 2 }],
        yStepSegments: [],
      },
    },
    channels: {
      probeChannels: [{ name: 'P1', enabled: true, channel: { deviceId, channelIndex: 0 } }],
      motionAxes: [],
    },
    dwellTimeMs: 100,
    samplesPerPoint: 1,
    prbFile: null,
    savePath: 'D:/out',
    saveFileName: 'result.csv',
    saveOptions: {
      savePointId: true,
      saveTimestamp: true,
      saveRawPressure: true,
      saveCalculatedResult: true,
    },
  }
}

describe('DualTraversalSettings', () => {
  it('loadConfig 完成后将持久化配置复制到对应 probe draft', async () => {
    loadedConfigs.probe1 = configWithChannel('persisted-probe1')
    loadedConfigs.probe2 = configWithChannel('persisted-probe2')
    const wrapper = mount(DualTraversalSettings, {
      props: { show: true, probeId: 'probe1' },
      global: {
        stubs: {
          UiDialog: defineComponent({ inheritAttrs: false, template: '<div><slot name="header"/><slot/><slot name="footer"/></div>' }),
          UiSteps: defineComponent({ inheritAttrs: false, template: '<div><slot/></div>' }),
          UiStep: defineComponent({ inheritAttrs: false, template: '<div />' }),
          MotionSafetyPanel: defineComponent({ inheritAttrs: false, template: '<div />' }),
          PointsPreview: defineComponent({ inheritAttrs: false, template: '<div />' }),
        },
      },
    })

    const hardware = wrapper.findComponent(TraversalHardwareStep)
    await vi.waitFor(() => {
      expect(dualStore.loadConfig).toHaveBeenCalledTimes(2)
      expect(hardware.props('probeChannels')[0].channel.deviceId).toBe('persisted-probe1')
    })
    await nextTick()
    loadedConfigs.probe1.channels.probeChannels[0]!.channel.deviceId = 'mutated-store'
    expect(hardware.props('probeChannels')[0].channel.deviceId).toBe('persisted-probe1')
  })

  it('PRB operations route through the active probe tab', async () => {
    loadedConfigs.probe1 = configWithChannel('probe1-device')
    loadedConfigs.probe2 = configWithChannel('probe2-device')
    const wrapper = mount(DualTraversalSettings, {
      props: { show: true, probeId: 'probe1' },
      global: {
        stubs: {
          UiDialog: defineComponent({ inheritAttrs: false, template: '<div><slot name="header"/><slot/><slot name="footer"/></div>' }),
          UiSteps: defineComponent({ inheritAttrs: false, template: '<div><slot/></div>' }),
          UiStep: defineComponent({ inheritAttrs: false, template: '<div />' }),
          MotionSafetyPanel: defineComponent({ inheritAttrs: false, template: '<div />' }),
          PointsPreview: defineComponent({ inheritAttrs: false, template: '<div />' }),
          TraversalPrbStep: defineComponent({
            inheritAttrs: false,
            props: ['operations'],
            template: '<div class="prb-step-stub" />',
          }),
        },
      },
    })
    await vi.waitFor(() => {
      expect(wrapper.findComponent(TraversalHardwareStep).props('probeChannels')[0].channel.deviceId).toBe('probe1-device')
    })

    await wrapper.findAll('.footer-right button').at(-1)!.trigger('click')
    const probe1Operations = wrapper.findComponent(TraversalPrbStep).props('operations')!
    await probe1Operations.importPrbFile('D:/probe1.prb')
    expect(dualStore.importPrbFile).toHaveBeenLastCalledWith('probe1', 'D:/probe1.prb')

    await wrapper.findAll('.dual-settings-tab')[1]!.trigger('click')
    await wrapper.findAll('.footer-right button').at(-1)!.trigger('click')
    const probe2Operations = wrapper.findComponent(TraversalPrbStep).props('operations')!
    await probe2Operations.importSevenHoleCalibrationCsvFiles('D:/7.csv', ['D:/1.csv'])
    expect(dualStore.importSevenHoleCalibrationCsvFiles).toHaveBeenLastCalledWith(
      'probe2',
      'D:/7.csv',
      ['D:/1.csv'],
    )
  })

  it('deferred probe1 import disables tab switching and never mutates probe2 draft', async () => {
    loadedConfigs.probe1 = configWithChannel('probe1-device')
    loadedConfigs.probe2 = configWithChannel('probe2-device')
    const pendingImport = deferred<{ filePath: string }>()
    dualStore.importPrbFile.mockReturnValueOnce(pendingImport.promise)
    const PrbStepStub = defineComponent({
      inheritAttrs: false,
      props: ['operations', 'prbFile'],
      emits: ['update:prbFile'],
      setup(props, { emit }) {
        const importing = ref(false)
        const runImport = async () => {
          importing.value = true
          try {
            const imported = await props.operations.importPrbFile('D:/probe1.prb')
            if (imported) emit('update:prbFile', imported)
          } finally {
            importing.value = false
          }
        }
        return { importing, runImport }
      },
      template: '<button class="deferred-import" @click="runImport">{{ importing }}</button>',
    })
    const wrapper = mount(DualTraversalSettings, {
      props: { show: true, probeId: 'probe1' },
      global: {
        stubs: {
          UiDialog: defineComponent({ inheritAttrs: false, template: '<div><slot name="header"/><slot/><slot name="footer"/></div>' }),
          UiSteps: defineComponent({ inheritAttrs: false, template: '<div><slot/></div>' }),
          UiStep: defineComponent({ inheritAttrs: false, template: '<div />' }),
          MotionSafetyPanel: defineComponent({ inheritAttrs: false, template: '<div />' }),
          PointsPreview: defineComponent({ inheritAttrs: false, template: '<div />' }),
          TraversalPrbStep: PrbStepStub,
        },
      },
    })
    await vi.waitFor(() => {
      expect(wrapper.findComponent(TraversalHardwareStep).props('probeChannels')[0].channel.deviceId).toBe('probe1-device')
    })
    await wrapper.findAll('.footer-right button').at(-1)!.trigger('click')

    await wrapper.find('.deferred-import').trigger('click')
    await vi.waitFor(() => expect(dualStore.importPrbFile).toHaveBeenCalledWith('probe1', 'D:/probe1.prb'))
    const probe2Tab = wrapper.findAll<HTMLButtonElement>('.dual-settings-tab')[1]!
    expect(probe2Tab.element.disabled).toBe(true)
    await probe2Tab.trigger('click')
    expect(wrapper.findAll('.dual-settings-tab')[0]!.attributes('aria-selected')).toBe('true')

    pendingImport.resolve({ filePath: 'D:/probe1.prb' })
    await vi.waitFor(() => expect(wrapper.findComponent(PrbStepStub).props('prbFile')).toEqual({ filePath: 'D:/probe1.prb' }))
    await probe2Tab.trigger('click')
    expect(wrapper.findAll('.dual-settings-tab')[1]!.attributes('aria-selected')).toBe('true')
    await wrapper.findAll('.footer-right button').at(-1)!.trigger('click')
    expect(wrapper.findComponent(PrbStepStub).props('prbFile')).toBeNull()
  })

  it('initializes rectangle and sector configs when switching layout patterns', async () => {
    const probe1 = configWithChannel('probe1-device')
    probe1.prbFile = {
      filePath: 'D:/probe1.prb',
      fileName: 'probe1.prb',
      loadedAt: 0,
      validRange: { alphaMin: -30, alphaMax: 30, betaMin: -30, betaMax: 30, machMin: 0, machMax: 1 },
    }
    probe1.channels.motionAxes = [
      { name: 'X', controllerId: 'mc-x', axis: 'X' },
      { name: 'Y', controllerId: 'mc-y', axis: 'Y' },
    ]
    loadedConfigs.probe1 = probe1
    loadedConfigs.probe2 = configWithChannel('probe2-device')

    const LayoutStepStub = defineComponent({
      inheritAttrs: false,
      props: ['pattern', 'rectangleConfig', 'sectorConfig'],
      emits: ['update:pattern'],
      template: `
        <div class="layout-step-stub">
          <button class="select-rectangle" @click="$emit('update:pattern', 'rectangle')">rectangle</button>
          <button class="select-sector" @click="$emit('update:pattern', 'sector')">sector</button>
        </div>
      `,
    })
    const wrapper = mount(DualTraversalSettings, {
      props: { show: true, probeId: 'probe1' },
      global: {
        stubs: {
          UiDialog: defineComponent({ inheritAttrs: false, template: '<div><slot name="header"/><slot/><slot name="footer"/></div>' }),
          UiSteps: defineComponent({ inheritAttrs: false, template: '<div><slot/></div>' }),
          UiStep: defineComponent({ inheritAttrs: false, template: '<div />' }),
          MotionSafetyPanel: defineComponent({ inheritAttrs: false, template: '<div />' }),
          PointsPreview: defineComponent({ inheritAttrs: false, template: '<div />' }),
          TraversalPrbStep: defineComponent({ inheritAttrs: false, template: '<div />' }),
          TraversalLayoutStep: LayoutStepStub,
        },
      },
    })
    await vi.waitFor(() => {
      expect(wrapper.findComponent(TraversalHardwareStep).props('probeChannels')[0].channel.deviceId).toBe('probe1-device')
    })

    await wrapper.findAll('.footer-right button').at(-1)!.trigger('click')
    await wrapper.findAll('.footer-right button').at(-1)!.trigger('click')
    const layoutStep = wrapper.findComponent(LayoutStepStub)

    await layoutStep.find('.select-rectangle').trigger('click')
    expect(layoutStep.props('rectangleConfig')).toEqual({
      xMin: -30,
      xMax: 30,
      xStepSegments: [{ start: -30, end: 30, step: 5 }],
      yMin: -30,
      yMax: 30,
      yStepSegments: [{ start: -30, end: 30, step: 5 }],
    })

    await layoutStep.find('.select-sector').trigger('click')
    expect(layoutStep.props('sectorConfig')).toEqual({
      centerX: 0,
      centerY: 0,
      radiusMin: 100,
      radiusMax: 300,
      radialStepSegments: [{ start: 100, end: 300, step: 50 }],
      angleStart: -30,
      angleEnd: 30,
      angularStepSegments: [{ start: -30, end: 30, step: 5 }],
    })
  })
})
