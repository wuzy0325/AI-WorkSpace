import type { Page } from '@playwright/test'
import type { MockDeviceProfile, MockScanResult } from './fixtures/deviceFixtures'

/** Mock 采集间隔（毫秒） */
const MOCK_ACQUISITION_INTERVAL_MS = 200
/** Mock 通道数 */
const CHANNEL_COUNT = 16

export interface MockState {
  profiles: MockDeviceProfile[]
  statusMap: Record<string, string>
  scanResults: MockScanResult[]
  recording: { active: boolean; snapshotCount: number; outputDir: string | null }
  logFile: { active: boolean; outputDir: string | null }
  eventHandlers: Record<string, ((...args: unknown[]) => void)[]>
  /** If set, Connect will reject with this error */
  connectError: string | null
  /** If set, StartAcquisition will reject with this error */
  startAcquisitionError: string | null
}

/** 默认扫描结果工厂函数，每次返回新实例避免共享引用 */
function createDefaultScanResults(): MockScanResult[] {
  return [
    { id: 't1603_scan_1', name: 'T1603-1', address: '192.168.1.10', port: 9000, macAddress: 'AA:BB:CC:DD:EE:01' },
  ]
}

export function defaultMockState(): MockState {
  return {
    profiles: [],
    statusMap: {},
    scanResults: createDefaultScanResults(),
    recording: { active: false, snapshotCount: 0, outputDir: null },
    logFile: { active: false, outputDir: null },
    eventHandlers: {},
    connectError: null,
    startAcquisitionError: null,
  }
}

export function mockBridgeScript(mockState: MockState): string {
  return `
    (() => {
      const state = ${JSON.stringify(mockState)};
      const handlers = {};
      // 按设备 ID 管理各自的采集定时器，避免多设备时互相覆盖
      const snapshotIntervals = {};
      const snapshotCounters = {};
      const CHANNEL_COUNT = ${CHANNEL_COUNT};

      window.go = {
        backend: {
          App: {
            GetProfiles: () => Promise.resolve(JSON.parse(JSON.stringify(state.profiles))),
            UpsertProfile: (profile) => {
              const idx = state.profiles.findIndex(p => p.id === profile.id);
              if (idx >= 0) {
                state.profiles[idx] = profile;
              } else {
                state.profiles.push(profile);
              }
              return Promise.resolve();
            },
            DeleteProfile: (id) => {
              state.profiles = state.profiles.filter(p => p.id !== id);
              return Promise.resolve();
            },
            Connect: (id) => {
              if (state.connectError) {
                const err = state.connectError;
                state.connectError = null;
                return Promise.reject(new Error(err));
              }
              state.statusMap[id] = 'Connected';
              fireEvent('daq:device-status', { deviceId: id, status: 'Connected' });
              return Promise.resolve();
            },
            Disconnect: (id) => {
              state.statusMap[id] = 'Disconnected';
              fireEvent('daq:device-status', { deviceId: id, status: 'Disconnected' });
              // 清除该设备的采集定时器
              clearDeviceInterval(id);
              return Promise.resolve();
            },
            StartAcquisition: (id) => {
              if (state.startAcquisitionError) {
                const err = state.startAcquisitionError;
                state.startAcquisitionError = null;
                return Promise.reject(new Error(err));
              }
              // 若该设备已有定时器则先清除
              clearDeviceInterval(id);
              state.statusMap[id] = 'Acquiring';
              fireEvent('daq:device-status', { deviceId: id, status: 'Acquiring' });
              snapshotCounters[id] = 0;
              snapshotIntervals[id] = setInterval(() => {
                const values = Array.from({ length: CHANNEL_COUNT }, () => +(20 + Math.random() * 10).toFixed(2));
                const snap = {
                  deviceId: id,
                  timestamp: Date.now(),
                  hardwareTimestamp: Date.now() * 1000,
                  values,
                  unit: '°C',
                };
                fireEvent('daq:payload', snap);
                snapshotCounters[id]++;
              }, ${MOCK_ACQUISITION_INTERVAL_MS});
              return Promise.resolve();
            },
            StopAcquisition: (id) => {
              state.statusMap[id] = 'Connected';
              fireEvent('daq:device-status', { deviceId: id, status: 'Connected' });
              clearDeviceInterval(id);
              return Promise.resolve();
            },
            ApplyConfig: (id, cfg) => {
              const profile = state.profiles.find(p => p.id === id);
              if (profile) {
                profile.t1603Config = { ...profile.t1603Config, ...cfg };
              }
              return Promise.resolve();
            },
            GetStatus: (id) => {
              const profile = state.profiles.find(p => p.id === id);
              if (!profile) {
                return Promise.resolve({
                  profile: null,
                  status: 0,
                  statusText: 'NotFound',
                  error: 'device not found',
                  connectedAt: 0,
                  acquiringAt: 0,
                  samplingRate: 0,
                });
              }
              return Promise.resolve({
                profile,
                status: state.statusMap[id] === 'Acquiring' ? 3 : state.statusMap[id] === 'Connected' ? 2 : 1,
                statusText: state.statusMap[id] || 'Disconnected',
                error: '',
                connectedAt: Date.now(),
                acquiringAt: state.statusMap[id] === 'Acquiring' ? Date.now() : 0,
                samplingRate: 10,
              });
            },
            ScanDevices: () => Promise.resolve(JSON.parse(JSON.stringify(state.scanResults))),
            PickDirectory: () => Promise.resolve(null),
            StartRecording: (dir, prefix) => {
              state.recording.active = true;
              state.recording.outputDir = dir;
              return Promise.resolve();
            },
            StopRecording: () => {
              state.recording.active = false;
              return Promise.resolve();
            },
            GetRecordingStatus: () => Promise.resolve({
              id: 'rec_1',
              outputDir: state.recording.outputDir || '',
              filePrefix: 'DAQ-T1603',
              startTimeMs: Date.now(),
              snapshotCount: state.recording.snapshotCount,
              status: state.recording.active ? 2 : 1,
            }),
            EmitLog: () => Promise.resolve(),
            StartLogFile: () => Promise.resolve(),
            StopLogFile: () => Promise.resolve(),
            GetLogFileState: () => Promise.resolve({ active: false, outputDir: null }),
          },
        },
      };

      // 清除指定设备的采集定时器
      function clearDeviceInterval(deviceId) {
        if (snapshotIntervals[deviceId]) {
          clearInterval(snapshotIntervals[deviceId]);
          delete snapshotIntervals[deviceId];
        }
        delete snapshotCounters[deviceId];
      }

      function registerHandler(eventName, callback, maxCallbacks) {
        if (!handlers[eventName]) handlers[eventName] = [];
        // 支持 maxCallbacks：回调触发指定次数后自动移除
        if (maxCallbacks && maxCallbacks > 0) {
          let callCount = 0;
          const wrappedCallback = (...args) => {
            callCount++;
            callback(...args);
            if (callCount >= maxCallbacks) {
              const idx = handlers[eventName].indexOf(wrappedCallback);
              if (idx >= 0) handlers[eventName].splice(idx, 1);
            }
          };
          handlers[eventName].push(wrappedCallback);
        } else {
          handlers[eventName].push(callback);
        }
      }

      window.runtime = {
        EventsOn: (eventName, callback) => registerHandler(eventName, callback),
        EventsOnMultiple: (eventName, callback, maxCallbacks) => {
          registerHandler(eventName, callback, maxCallbacks);
        },
        EventsOff: (eventName) => {
          delete handlers[eventName];
        },
        EventsEmit: (eventName, ...args) => {
          fireEvent(eventName, ...args);
        },
        LogPrint: () => {},
        LogDebug: () => {},
        LogInfo: () => {},
        LogWarning: () => {},
        LogError: () => {},
        Environment: () => Promise.resolve({ platform: 'windows', arch: 'amd64' }),
        WindowSetTitle: () => {},
      };

      function fireEvent(eventName, ...args) {
        const hs = handlers[eventName];
        if (hs) {
          hs.forEach(cb => { try { cb(...args); } catch(e) { console.warn('[mock] handler error:', e); } });
        }
      }

      window.__mockGetHandlerCount = function(eventName) {
        return (handlers[eventName] || []).length;
      };

      window.__mockState = state;

      window.__mockFireEvent = fireEvent;

      window.__mockPushSnapshot = function(deviceId) {
        var values = Array.from({ length: CHANNEL_COUNT }, function() { return +(20 + Math.random() * 10).toFixed(2); });
        fireEvent('daq:payload', {
          deviceId: deviceId,
          timestamp: Date.now(),
          hardwareTimestamp: Date.now() * 1000,
          values: values,
          unit: '°C'
        });
      };
    })();
  `
}

export async function setupMockBridge(page: Page, mockState: MockState): Promise<void> {
  await page.context().addInitScript(mockBridgeScript(mockState))
}

export async function triggerPayload(page: Page, deviceId: string, values?: number[]): Promise<void> {
  await page.evaluate(({ deviceId, values }) => {
    const snap = {
      deviceId,
      timestamp: Date.now(),
      hardwareTimestamp: Date.now() * 1000,
      values: values || Array.from({ length: 16 }, () => +(20 + Math.random() * 10).toFixed(2)),
      unit: '°C',
    }
    const fireEvent = (window as any).__mockFireEvent
    if (fireEvent) {
      fireEvent('daq:payload', snap)
    }
  }, { deviceId, values })
}
