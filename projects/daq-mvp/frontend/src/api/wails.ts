// Type definitions matching Go backend structs.
// These are the bridge types for Wails bindings + Events.

export interface DeviceInfo {
  id: string
  name: string
  channels: number
}

export interface Status {
  state: 'idle' | 'running' | 'unknown'
  sampleRateHz: number
  batchCount: number
  sampleCount: number
  latestValues: number[]
}

export interface RuntimeStats {
  batchesEmitted: number
  droppedFrames: number
  uptimeMs: number
}

export interface UiSampleFrame {
  deviceId: string
  sequenceStart: number
  sampleCount: number
  channels: number[]
  latestValues: number[]
  samplesPerChannel: number
  hostTimestampMs: number
}

// Wails v2 generated bindings are available at window.go.main.App.
// Events are at window.runtime.EventsOn / EventsOff.
declare global {
  interface Window {
    go: {
      main: {
        App: {
          StartAcquisition(): Promise<void>
          StopAcquisition(): Promise<void>
          GetStatus(): Promise<Status>
          GetDevices(): Promise<DeviceInfo[]>
          GetRuntimeStats(): Promise<RuntimeStats>
        }
      }
    }
    runtime: {
      EventsOn(event: string, cb: (...args: any[]) => void): void
      EventsOff(event: string): void
    }
  }
}
