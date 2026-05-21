import { describe, it, expect, beforeEach } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { useDeviceStore } from '@stores/deviceStore'

describe('deviceStore', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
  })

  it('initializes with no profiles', () => {
    const store = useDeviceStore()
    expect(store.profiles).toEqual([])
  })

  it('selects and deselects a device', () => {
    const store = useDeviceStore()
    store.selectDevice('sim-1')
    expect(store.selectedDeviceId).toBe('sim-1')
    store.selectDevice(null)
    expect(store.selectedDeviceId).toBeNull()
  })

  it('pushes a snapshot and makes it available via latestFor', () => {
    const store = useDeviceStore()
    store.pushSnapshot({
      deviceId: 'sim-1',
      timestamp: 123,
      channels: [1.5, 2.5],
      channelIndices: [0, 1],
    })
    expect(store.latestSnapshots.length).toBe(1)
    expect(store.latestFor('sim-1')?.channels[0]).toBe(1.5)
  })

  it('keeps history buffer within capacity', () => {
    const store = useDeviceStore()
    const payload = {
      deviceId: 'sim-1',
      timestamp: 0,
      channels: [1],
      channelIndices: [0],
    }
    for (let i = 0; i < 300; i++) {
      store.pushSnapshot({ ...payload, timestamp: i })
    }
    expect(store.historyFor('sim-1').length).toBeLessThanOrEqual(256)
  })
})
