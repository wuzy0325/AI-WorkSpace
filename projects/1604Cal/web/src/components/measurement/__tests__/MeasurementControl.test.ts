import { defineComponent } from 'vue'
import { mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'

import MeasurementControl from '../MeasurementControl.vue'
import { useMeasurementStore } from '@/stores/measurement'

const ElIconStub = defineComponent({
  name: 'ElIcon',
  template: '<i><slot /></i>'
})

function mountControl() {
  return mount(MeasurementControl, {
    props: {
      channels: [1, 2],
      isStable: false,
      stableSeconds: 0,
      selectedChannelCount: 2
    },
    global: {
      stubs: {
        ElIcon: ElIconStub
      }
    }
  })
}

describe('MeasurementControl', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
  })

  it('shows pause button enabled while running', () => {
    const store = useMeasurementStore()
    store.measureDeviceIds = ['measure-1']
    store.syncState('pressurizing')

    const wrapper = mountControl()

    const pauseButton = wrapper.findAll('button').find(btn => btn.text().includes('暂停'))
    expect(pauseButton).toBeTruthy()
    expect(pauseButton?.attributes('disabled')).toBeUndefined()
  })

  it('shows export primary button enabled when completed and device bound', () => {
    // 主按钮在 completed 状态文案为「导出报告」，启用即可。
    const store = useMeasurementStore()
    store.measureDeviceIds = ['measure-1']
    store.syncState('completed')

    const wrapper = mountControl()

    const primary = wrapper.findAll('button').find(btn => btn.text().includes('导出报告'))
    expect(primary).toBeTruthy()
    expect(primary?.attributes('disabled')).toBeUndefined()
  })

  it('renders mode toggles and progress', () => {
    const wrapper = mountControl()

    expect(wrapper.text()).toContain('模式')
    expect(wrapper.text()).toContain('打压')
    expect(wrapper.text()).toContain('任务进度')
  })
})
