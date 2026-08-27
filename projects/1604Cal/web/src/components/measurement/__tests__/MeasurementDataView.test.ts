import { defineComponent } from 'vue'
import { mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'

import MeasurementDataView from '../MeasurementDataView.vue'

const ElIconStub = defineComponent({
  name: 'ElIcon',
  template: '<i><slot /></i>'
})

const ElTableStub = defineComponent({
  name: 'ElTable',
  props: ['data'],
  template: '<div class="table-stub" :data-row-count="data.length"><slot /></div>'
})

const ElTableColumnStub = defineComponent({
  name: 'ElTableColumn',
  template: '<div><slot :row="{ actualPressure: \'10.100\', channelValues: { 1: \'10.100\', 2: \'10.200\' }, collectTime: \'10:00:01\' }" /></div>'
})

const ElEmptyStub = defineComponent({
  name: 'ElEmpty',
  template: '<div><slot /></div>'
})

describe('MeasurementDataView', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
  })

  it('renders raw sampling rows even when no pressure targets are configured', () => {
    const wrapper = mount(MeasurementDataView, {
      props: {
        rows: [
          {
            timestamp: '2026-04-23T10:00:01Z',
            channels: { '1': 10.1, '2': 10.2 }
          }
        ],
        channels: [1, 2],
        targets: [],
        averageCount: 1,
        state: 'collecting'
      },
      global: {
        stubs: {
          ElIcon: ElIconStub,
          ElTable: ElTableStub,
          ElTableColumn: ElTableColumnStub,
          ElEmpty: ElEmptyStub
        }
      }
    })

    expect(wrapper.text()).toContain('1 条采样')
    expect(wrapper.text()).toContain('10.100')
    expect(wrapper.text()).toContain('10.200')
    expect(wrapper.text()).not.toContain('请先配置参数并生成压力表')
  })

  it('renders only a bounded window for long-running sampling', () => {
    const rows = Array.from({ length: 1000 }, (_, index) => ({
      timestamp: new Date(index * 1000).toISOString(),
      channels: { '1': index }
    }))
    const wrapper = mount(MeasurementDataView, {
      props: { rows, channels: [1], targets: [], averageCount: 1, state: 'collecting' },
      global: {
        stubs: {
          ElIcon: ElIconStub,
          ElTable: ElTableStub,
          ElTableColumn: ElTableColumnStub,
          ElEmpty: ElEmptyStub
        }
      }
    })

    expect(wrapper.text()).toContain('1000 条采样')
    expect(wrapper.find('.sample-section .table-stub').attributes('data-row-count')).toBe('100')
  })
})
