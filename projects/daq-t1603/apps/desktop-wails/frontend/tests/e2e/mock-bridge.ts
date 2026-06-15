import type { Page } from '@playwright/test'
import type { MockDeviceProfile, MockScanResult } from './fixtures/deviceFixtures'

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

const DEFAULT_PROFILES: MockDeviceProfile[] = []
const DEFAULT_SCAN_RESULTS: MockScanResult[] = [
  { id: 't1603_scan_1', name: 'T1603-1', address: '192.168.1.10', port: 9000, macAddress: 'AA:BB:CC:DD:EE:01' },
]

export function defaultMockState(): MockState {
  return {
    profiles: DEFAULT_PROFILES,
    statusMap: {},
    scanResults: DEFAULT_SCAN_RESULTS,
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
      let snapshotInterval = null;
      let snapshotCounter = 0;

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
              if (snapshotInterval) {
                clearInterval(snapshotInterval);
                snapshotInterval = null;
              }
              return Promise.resolve();
            },
            StartAcquisition: (id) => {
              if (state.startAcquisitionError) {
                const err = state.startAcquisitionError;
                state.startAcquisitionError = null;
                return Promise.reject(new Error(err));
              }
              state.statusMap[id] = 'Acquiring';
              fireEvent('daq:device-status', { deviceId: id, status: 'Acquiring' });
              snapshotCounter = 0;
              snapshotInterval = setInterval(() => {
                const values = Array.from({ length: 16 }, () => +(20 + Math.random() * 10).toFixed(2));
                const snap = {
                  deviceId: id,
                  timestamp: Date.now(),
                  hardwareTimestamp: Date.now() * 1000,
                  values,
                  unit: '°C',
                };
                fireEvent('daq:payload', snap);
                snapshotCounter++;
              }, 200);
              return Promise.resolve();
            },
            StopAcquisition: (id) => {
              state.statusMap[id] = 'Connected';
              fireEvent('daq:device-status', { deviceId: id, status: 'Connected' });
              if (snapshotInterval) {
                clearInterval(snapshotInterval);
                snapshotInterval = null;
              }
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
              if (!profile) return Promise.resolve(false);
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

      function registerHandler(eventName, callback) {
        if (!handlers[eventName]) handlers[eventName] = [];
        handlers[eventName].push(callback);
      }

      window.runtime = {
        EventsOn: registerHandler,
        EventsOnMultiple: (eventName, callback, maxCallbacks) => {
          registerHandler(eventName, callback);
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
        var values = Array.from({ length: 16 }, function() { return +(20 + Math.random() * 10).toFixed(2); });
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
