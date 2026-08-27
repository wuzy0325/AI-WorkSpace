import { defineComponent } from 'vue'
import { mount, flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import CalibrationDeviceDrawer from '../CalibrationDeviceDrawer.vue'
import { useDeviceInventoryStore } from '@/stores/device/inventoryStore'
import { useDeviceStore } from '@/stores/deviceStore'

vi.mock('@/api/session', () => ({
  bindDevices: vi.fn(), bindMeasureDevice: vi.fn(), unbindMeasureDevices: vi.fn(),
  readPressure: vi.fn(), readStability: vi.fn(), readMeasureData: vi.fn(), readMeasureDataAllDevices: vi.fn(),
  readValveStatus: vi.fn(), readValveStatusAll: vi.fn().mockResolvedValue({}),
  setValveStatus: vi.fn(), readMeasureUnit: vi.fn(), readMeasureUnitAll: vi.fn().mockResolvedValue({}),
  setMeasureUnit: vi.fn(), setMeasureUnitAll: vi.fn(), readSessionUnitConsistency: vi.fn(),
  readDeviceInfo: vi.fn(), resetDevice: vi.fn(), calibrateZero: vi.fn()
}))
vi.mock('@/api/device', () => ({
  fetchDevices: vi.fn().mockResolvedValue([]),
  upsertDevice: vi.fn(),
  connectDevice: vi.fn(),
  disconnectDevice: vi.fn(),
  deleteDevice: vi.fn(),
  setDeviceStatus: vi.fn(),
  fetchUnitConsistency: vi.fn().mockResolvedValue({ consistent: true, conflicts: [] }),
  fetchDeviceConnectConfig: vi.fn()
}))
vi.mock('@/api/multipress', () => ({
  multipressRegister: vi.fn(), multipressUnregister: vi.fn(), multipressSetPressure: vi.fn(),
  multipressStop: vi.fn(), multipressExhaust: vi.fn(), multipressReadPressure: vi.fn(),
  multipressReadStability: vi.fn(), multipressSetUnit: vi.fn(), multipressListDevices: vi.fn().mockResolvedValue([]),
  multipressStopAll: vi.fn()
}))

// 模拟 el-drawer：挂载后触发 open，驱动组件回读逐台状态
const DrawerStub = defineComponent({
  props: { modelValue: Boolean },
  emits: ['open'],
  mounted() {
    this.$emit('open')
  },
  template: '<div><slot /></div>'
})
const ButtonStub = defineComponent({
  emits: ['click'],
  template: '<button type="button" @click="$emit(\'click\')"><slot /></button>'
})

function mountDrawer() {
  return mount(CalibrationDeviceDrawer, {
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
}

// 通过模块勾选配置预置两台已连接设备（走生产代码的恢复勾选路径）
function seedConnectedDevices() {
  const inventory = useDeviceInventoryStore()
  const moduleSelection = useDeviceStore()
  inventory.measureDevices = [
    { id: 'm1', name: '设备一', model: 'WTN1604', channels: 16, status: 'connected' },
    { id: 'm2', name: '设备二', model: 'WTN1604', channels: 16, status: 'connected' }
  ]
  moduleSelection.setModuleSelection('calibration', { measureDeviceIds: ['m1', 'm2'] })
}

describe('CalibrationDeviceDrawer', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
  })

  it('switches the focused device by clicking a per-device valve row', async () => {
    seedConnectedDevices()
    const wrapper = mountDrawer()
    await flushPromises()

    // 阀门区块为第一个 device-state-list；默认焦点为第一台，点击第二行后切换
    const rows = wrapper.findAll('.device-state-list')[0].findAll('.device-state-row')
    expect(rows).toHaveLength(2)

    let activeRows = wrapper.findAll('.device-state-row.active')
    expect(activeRows).toHaveLength(1)
    expect(activeRows[0].text()).toContain('设备一')

    await rows[1].trigger('click')

    activeRows = wrapper.findAll('.device-state-row.active')
    expect(activeRows).toHaveLength(1)
    expect(activeRows[0].text()).toContain('设备二')
  })

  it('highlights per-device unit differences in red', async () => {
    const { readValveStatusAll, readMeasureUnitAll } = await import('@/api/session')
    vi.mocked(readValveStatusAll).mockResolvedValue({
      m1: { value: 'calibration' },
      m2: { value: 'calibration' }
    })
    vi.mocked(readMeasureUnitAll).mockResolvedValue({
      m1: { value: 'kPa' },
      m2: { value: 'MPa' }
    })

    seedConnectedDevices()
    const wrapper = mountDrawer()
    await flushPromises()
    await flushPromises()

    // 单位区块（第二个 device-state-list）：m2 与基线 kPa 不同，应标红并带"不一致"标注
    const unitSpans = wrapper.findAll('.device-state-list')[1].findAll('.state-value')
    expect(unitSpans).toHaveLength(2)
    expect(unitSpans[0].classes()).not.toContain('error')
    expect(unitSpans[1].classes()).toContain('error')
    expect(unitSpans[1].text()).toContain('不一致')
  })
})
