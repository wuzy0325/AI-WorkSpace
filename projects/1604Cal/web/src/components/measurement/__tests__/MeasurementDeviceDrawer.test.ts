import { defineComponent } from 'vue'
import { mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import MeasurementDeviceDrawer from '../MeasurementDeviceDrawer.vue'
import { useMeasurementStore } from '@/stores/measurement'
import { useMeasurementDeviceStore } from '@/stores/measurement/deviceStore'

vi.mock('@/api/session', () => ({
  bindDevices: vi.fn(), bindMeasureDevice: vi.fn(), unbindMeasureDevices: vi.fn(), readPressure: vi.fn(), readStability: vi.fn(),
  readMeasureDataAllDevices: vi.fn(), readValveStatus: vi.fn(), readValveStatusAll: vi.fn(),
  setValveStatus: vi.fn(), readMeasureUnit: vi.fn(), readMeasureUnitAll: vi.fn(), setMeasureUnit: vi.fn(),
  setMeasureUnitAll: vi.fn(), readSessionUnitConsistency: vi.fn(), readDeviceInfo: vi.fn(),
  resetDevice: vi.fn(), calibrateZero: vi.fn()
}))
vi.mock('@/api/measurement', () => ({
  fetchMeasurementState: vi.fn(), startMeasurement: vi.fn(), pauseMeasurement: vi.fn(), stopMeasurement: vi.fn(),
  fetchMeasurementData: vi.fn(), generateMeasurementPoints: vi.fn(), saveMeasurementParamsConfig: vi.fn()
}))

const DrawerStub = defineComponent({
  props: { modelValue: Boolean },
  template: '<div><slot /></div>'
})
const ButtonStub = defineComponent({
  emits: ['click'],
  template: '<button type="button" @click="$emit(\'click\')"><slot /></button>'
})

describe('MeasurementDeviceDrawer', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
  })

  it('changes the shared focused device from the per-device state list', async () => {
    const measurement = useMeasurementStore()
    const devices = useMeasurementDeviceStore()
    measurement.measureDeviceIds = ['m1', 'm2']
    measurement.setActiveDevice('m1')
    measurement.valveStatusByDevice = { m1: 'calibration', m2: 'measurement' }
    devices.measureDevices = [
      { id: 'm1', name: '设备一', model: 'WTN1604', channels: 16, status: 'connected' },
      { id: 'm2', name: '设备二', model: 'WTN1604', channels: 16, status: 'connected' }
    ]
    const wrapper = mount(MeasurementDeviceDrawer, {
      props: { modelValue: true },
      global: {
        stubs: {
          ElDrawer: DrawerStub,
          ElButton: ButtonStub,
          ElSelect: true,
          ElOption: true,
          ElCheckboxGroup: true,
          ElCheckbox: true,
          ElEmpty: true
        }
      }
    })

    const rows = wrapper.findAll('.device-state-row')
    await rows[1].trigger('click')

    expect(measurement.activeDeviceId).toBe('m2')
    expect(wrapper.text()).toContain('测量模式')
  })

  it('highlights per-device valve differences in red', async () => {
    const measurement = useMeasurementStore()
    const devices = useMeasurementDeviceStore()
    measurement.measureDeviceIds = ['m1', 'm2']
    measurement.setActiveDevice('m1')
    measurement.valveStatusByDevice = { m1: 'calibration', m2: 'measurement' }
    devices.measureDevices = [
      { id: 'm1', name: '设备一', model: 'WTN1604', channels: 16, status: 'connected' },
      { id: 'm2', name: '设备二', model: 'WTN1604', channels: 16, status: 'connected' }
    ]
    const wrapper = mount(MeasurementDeviceDrawer, {
      props: { modelValue: true },
      global: {
        stubs: {
          ElDrawer: DrawerStub,
          ElButton: ButtonStub,
          ElSelect: true,
          ElOption: true,
          ElCheckboxGroup: true,
          ElCheckbox: true,
          ElEmpty: true
        }
      }
    })

    // 阀门区块为第一个 device-state-list：m2 与基线（校准）值不同，应标红并带"不一致"标注
    const valveSpans = wrapper.findAll('.device-state-list')[0].findAll('.state-value')
    expect(valveSpans).toHaveLength(2)
    expect(valveSpans[0].classes()).not.toContain('error')
    expect(valveSpans[1].classes()).toContain('error')
    expect(valveSpans[1].text()).toContain('不一致')
  })
})
