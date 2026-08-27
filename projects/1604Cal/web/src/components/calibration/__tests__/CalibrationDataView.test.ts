/* eslint-disable @typescript-eslint/no-explicit-any */
/* eslint-disable vue/one-component-per-file */
import { computed, defineComponent, h, inject, provide, ref } from 'vue'
import { mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it } from 'vitest'

import CalibrationDataView from '../CalibrationDataView.vue'
import { useCalibrationStore } from '@/stores/calibration'
import { usePressurePointStore } from '@/stores/calibration/pressurePoints'
import { ControlMode } from '@/types/calibration'

type RowData = Record<string, unknown>

const tableRowsKey = Symbol('tableRows')

const ElTableStub = defineComponent({
  name: 'ElTable',
  props: {
    data: {
      type: Array,
      default: () => []
    }
  },
  setup(props: any, { slots }: any) {
    provide(tableRowsKey, computed(() => props.data as RowData[]))
    return () => h('div', { class: 'el-table-stub' }, slots.default ? slots.default() : [])
  }
})

const ElTableColumnStub = defineComponent({
  name: 'ElTableColumn',
  props: {
    label: {
      type: String,
      default: ''
    },
    prop: {
      type: String,
      default: ''
    }
  },
  setup(props: any, { slots }: any) {
    const rows = inject<any>(tableRowsKey, ref<RowData[]>([]))

    return () => h('div', { class: 'el-table-column-stub', 'data-label': props.label }, [
      h('div', { class: 'column-label' }, props.label),
      ...rows.value.map((row: RowData, index: number) => h(
        'div',
        { class: 'column-row', 'data-row-index': String(index) },
        slots.default
          ? slots.default({ row, $index: index })
          : props.prop
            ? String(row[props.prop] ?? '')
            : ''
      ))
    ])
  }
})

const ElButtonStub = defineComponent({
  name: 'ElButton',
  props: {
    disabled: {
      type: Boolean,
      default: false
    }
  },
  emits: ['click'],
  template: '<button :disabled="disabled" @click="$emit(\'click\')"><slot /></button>'
})

describe('CalibrationDataView', () => {
  beforeEach(() => {
    setActivePinia(createPinia())

    const calibrationStore = useCalibrationStore()
    const pressurePointStore = usePressurePointStore()

    calibrationStore.selectedChannels = [1, 2]
    pressurePointStore.pressurePoints = [
      {
        id: 'point-1',
        index: 1,
        targetPressure: 10,
        status: 'stabilizing',
        collectedData: [10.12, 10.34],
        actualPressure: 10.2
      },
      {
        id: 'point-2',
        index: 2,
        targetPressure: 20,
        status: 'completed',
        collectedData: [20.12, 20.34],
        actualPressure: 20.2
      }
    ]
  })

  it('renders separate point-operation and collected-data tables', () => {
    const wrapper = mount(CalibrationDataView, {
      global: {
        stubs: {
          ElTable: ElTableStub,
          ElTableColumn: ElTableColumnStub,
          ElButton: ElButtonStub,
          ElTag: { template: '<span><slot /></span>' },
          ElIcon: { template: '<i><slot /></i>' }
        }
      }
    })

    expect(wrapper.find('[data-testid="pressure-point-operation-table"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="collected-data-table"]').exists()).toBe(true)
    expect(wrapper.text()).toContain('压力点设置')
    expect(wrapper.text()).toContain('采集数据')
  })

it('before start, manual mode keeps confirm disabled and hides pressurize action', () => {
    const calibrationStore = useCalibrationStore()
    const pressurePointStore = usePressurePointStore()

    pressurePointStore.pressurePoints = [
      {
        id: 'point-1',
        index: 1,
        targetPressure: 10,
        status: 'pending',
        collectedData: undefined,
        actualPressure: undefined
      }
    ]

    ;(calibrationStore as any).controlMode = ControlMode.Manual

    const wrapper = mount(CalibrationDataView, {
      global: {
        stubs: {
          ElTable: ElTableStub,
          ElTableColumn: ElTableColumnStub,
          ElButton: ElButtonStub,
          ElTag: { template: '<span><slot /></span>' },
          ElIcon: { template: '<i><slot /></i>' }
        }
      }
    })

    expect(wrapper.text()).not.toContain('打压')
    expect(wrapper.text()).toContain('待采集')
  })

  it('after start, manual mode allows collect directly for stabilizing point', () => {
    const calibrationStore = useCalibrationStore()
    const pressurePointStore = usePressurePointStore()

    pressurePointStore.pressurePoints = [
      {
        id: 'point-1',
        index: 1,
        targetPressure: 10,
        status: 'stabilizing',
        collectedData: undefined,
        actualPressure: undefined
      }
    ]

    ;(calibrationStore as any).controlMode = ControlMode.Manual
    calibrationStore.sessionState = 'ready'

    const wrapper = mount(CalibrationDataView, {
      global: {
        stubs: {
          ElTable: ElTableStub,
          ElTableColumn: ElTableColumnStub,
          ElButton: ElButtonStub,
          ElTag: { template: '<span><slot /></span>' },
          ElIcon: { template: '<i><slot /></i>' }
        }
      }
    })

    const collectButton = wrapper.get('[data-testid="collect-btn-point-1"]')
    expect(collectButton.attributes('disabled')).toBeUndefined()
  })

  it('keeps collect enabled for completed point without retry action', () => {
    const calibrationStore = useCalibrationStore()
    ;(calibrationStore as any).controlMode = ControlMode.Manual
    calibrationStore.sessionState = 'ready'

    const wrapper = mount(CalibrationDataView, {
      global: {
        stubs: {
          ElTable: ElTableStub,
          ElTableColumn: ElTableColumnStub,
          ElButton: ElButtonStub,
          ElTag: { template: '<span><slot /></span>' },
          ElIcon: { template: '<i><slot /></i>' }
        }
      }
    })

    const collectButton = wrapper.get('[data-testid="collect-btn-point-2"]')
    expect(collectButton.attributes('disabled')).toBeUndefined()
    expect(wrapper.text()).not.toContain('重采')
  })
})
