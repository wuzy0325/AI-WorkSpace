import { describe, it, expect, beforeEach, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { useDeviceStore } from '@stores/deviceStore'
import { deviceApi } from '@api/deviceApi'

describe('deviceStore', () => {
  beforeEach(() => {
    vi.restoreAllMocks()
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

  it('updates latest but does not append duplicate timestamps to history', () => {
    const store = useDeviceStore()

    store.pushSnapshot({
      deviceId: 'sim-1',
      timestamp: 123,
      channels: [1.5],
      channelIndices: [0],
    })
    store.pushSnapshot({
      deviceId: 'sim-1',
      timestamp: 123,
      channels: [2.5],
      channelIndices: [0],
    })
    store.pushSnapshot({
      deviceId: 'sim-1',
      timestamp: 124,
      channels: [2.5],
      channelIndices: [0],
    })

    expect(store.latestFor('sim-1')?.channels[0]).toBe(2.5)
    expect(store.historyFor('sim-1')).toHaveLength(2)
    expect(store.historyFor('sim-1').map((snapshot) => snapshot.timestamp)).toEqual([123, 124])
  })

  it('does not restart acquisition state from a stale snapshot', () => {
    const store = useDeviceStore()
    store.updateStatus('sim-1', {
      id: 'sim-1',
      name: 'Simulated',
      type: 'SIMULATED',
      connection: 'Connected',
      acquiring: false,
    })

    store.pushSnapshot({
      deviceId: 'sim-1',
      timestamp: 123,
      channels: [1.5],
      channelIndices: [0],
    })

    expect(store.acquiringFor('sim-1')).toBe(false)
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

  it('normalizes profiles with null channels during refresh', async () => {
    const store = useDeviceStore()
    vi.spyOn(deviceApi, 'getProfiles').mockResolvedValueOnce([
      {
        id: 'legacy-1',
        name: 'Legacy Device',
        type: 'SIMULATED',
        samplingRate: 20,
        channels: null,
      } as never,
    ])

    await store.refreshProfiles()

    expect(store.profiles).toHaveLength(1)
    expect(store.profiles[0].channels).toEqual([])
    expect(() => store.selectDevice('legacy-1')).not.toThrow()
  })

  it('keeps profile list stable when starting all acquisitions', async () => {
    const store = useDeviceStore()
    store.profiles = [{
      id: 'daq-1',
      name: 'DAQ 1',
      type: 'SIMULATED',
      samplingRate: 20,
      channels: [],
    }]
    store.updateStatus('daq-1', {
      id: 'daq-1',
      name: 'DAQ 1',
      type: 'SIMULATED',
      connection: 'Connected',
      acquiring: false,
    })

    const getProfiles = vi.spyOn(deviceApi, 'getProfiles').mockResolvedValue([{
      id: 'unexpected',
      name: 'Unexpected Device',
      type: 'SIMULATED',
      samplingRate: 20,
      channels: [],
    }])
    vi.spyOn(deviceApi, 'startAcquisition').mockResolvedValue({ success: true })
    vi.spyOn(deviceApi, 'getStatus').mockResolvedValue({
      id: 'daq-1',
      name: 'DAQ 1',
      type: 'SIMULATED',
      connection: 'Connected',
      acquiring: true,
    })
    vi.spyOn(deviceApi, 'subscribeToDevice').mockImplementation(() => undefined)

    await store.startAllAcquisitions()

    expect(getProfiles).not.toHaveBeenCalled()
    expect(store.profiles.map((profile) => profile.id)).toEqual(['daq-1'])
    expect(store.acquiringFor('daq-1')).toBe(true)
  })
})
