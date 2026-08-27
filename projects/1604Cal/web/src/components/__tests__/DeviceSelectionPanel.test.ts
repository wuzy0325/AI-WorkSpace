import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import DeviceSelectionPanel from '../device/DeviceSelectionPanel.vue'
import * as apiClient from '@/api/device'

vi.mock('@/api/device', () => ({
  fetchDevices: vi.fn()
}))

describe('DeviceSelectionPanel', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.clearAllMocks()

    vi.mocked(apiClient.fetchDevices).mockResolvedValue([
      {
        id: 'm1',
        name: 'measure-1',
        type: 'measure',
        model: 'WTN1604',
        host: '192.168.1.10',
        port: 9000,
        unit: 'kPa',
        status: 'connected'
      },
      {
        id: 'p1',
        name: 'pressure-1',
        type: 'pressure',
        model: 'ConST 820',
        host: '192.168.1.11',
        port: 7001,
        unit: 'kPa',
        status: 'connected'
      }
    ])
  })

  it('loads device options and allows selecting for module', async () => {
    const wrapper = mount(DeviceSelectionPanel, {
      props: {
        moduleKey: 'measurement'
      }
    })

    await flushPromises()

    // 计量设备为多选 checkbox 列表，打压设备为下拉 select。
    const checkboxes = wrapper.findAll('input[type="checkbox"]')
    expect(checkboxes.length).toBeGreaterThanOrEqual(1)

    const selects = wrapper.findAll('select')
    expect(selects.length).toBeGreaterThanOrEqual(1)

    await checkboxes[0].setValue()
    await selects[0].setValue('p1')

    expect(wrapper.text()).toContain('measure-1')
    expect(wrapper.text()).toContain('pressure-1')
  })
})
