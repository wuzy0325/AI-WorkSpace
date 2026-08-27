import { mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { createRouter, createWebHistory } from 'vue-router'

import CalibrationView from '../CalibrationView.vue'

// Mock fetch to avoid URL parsing errors in jsdom
globalThis.fetch = (async () => new Response(JSON.stringify({ data: {} }), { status: 200, headers: { 'Content-Type': 'application/json' } })) as typeof fetch

// Mock EventSource for jsdom environment
class MockEventSource {
  onmessage: ((event: MessageEvent) => void) | null = null
  onerror: ((event: Event) => void) | null = null
  close() {}
  addEventListener() {}
  removeEventListener() {}
  dispatchEvent() { return true }
  readonly readyState = 0
  readonly url = ''
  readonly withCredentials = false
  readonly CONNECTING = 0
  readonly OPEN = 1
  readonly CLOSED = 2
}

// @ts-expect-error - jsdom doesn't have EventSource
globalThis.EventSource = MockEventSource

// Stub el-table-column to avoid scoped slot destructuring errors in test
const ElTableStub = {
  template: '<div class="el-table-stub"><slot /></div>'
}
const ElTableColumnStub = {
  template: '<span />'
}

describe('CalibrationView', () => {
  it('renders calibration controls and selector', async () => {
    setActivePinia(createPinia())

    const router = createRouter({
      history: createWebHistory(),
      routes: [{ path: '/', component: { template: '<div />' } }]
    })
    await router.push('/')

    const wrapper = mount(CalibrationView, {
      global: {
        plugins: [router],
        stubs: {
          RouterLink: {
            template: '<a><slot /></a>'
          },
          CalibrationDeviceDrawer: {
            template: '<div>CalibrationDeviceDrawerStub</div>'
          },
          PressDevicePanel: {
            template: '<div>PressDeviceStub</div>'
          },
          ChannelMatrix: {
            template: '<div>ChannelMatrixStub</div>'
          },
          ProgressIndicator: {
            template: '<div>ProgressStub</div>'
          },
          ElTable: ElTableStub,
          ElTableColumn: ElTableColumnStub
        }
      }
    })

    expect(wrapper.text()).toContain('标定工作台')
    expect(wrapper.text()).toContain('CalibrationDeviceDrawerStub')
    expect(wrapper.text()).toContain('PressDeviceStub')
    expect(wrapper.text()).toContain('压力点设置')
    expect(wrapper.text()).toContain('采集数据')
  })
})
