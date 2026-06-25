// Recording Bridge —— Wails v3 版
//
// 通过生成的 RecordingService 绑定调用 Go 侧方法；事件改用 @wailsio/runtime。
import { Events } from '@wailsio/runtime'
import {
  StartRecording,
  StopRecording,
  GetRecordingStatus,
  PickDirectory,
} from '../../bindings/daq-t1603/backend/recordingservice'

export interface RecordingSession {
  id: string
  outputDir: string
  filePrefix: string
  startTimeMs: number
  snapshotCount: number
  status: number
}

export function startRecording(outputDir: string, filePrefix: string): Promise<void> {
  return StartRecording(outputDir, filePrefix) as Promise<void>
}

export function stopRecording(): Promise<void> {
  return StopRecording() as Promise<void>
}

export function getRecordingStatus(): Promise<RecordingSession> {
  return GetRecordingStatus() as any
}

export function pickDirectory(): Promise<string> {
  return PickDirectory() as Promise<string>
}

let recordingStatusUnsubscribe: (() => void) | null = null

export function onRecordingStatus(handler: (session: RecordingSession) => void): void {
  offRecordingStatus()
  recordingStatusUnsubscribe = Events.On('daq:recording-status', (event: { data: RecordingSession }) => {
    handler(event.data)
  })
}

export function offRecordingStatus(): void {
  if (recordingStatusUnsubscribe) {
    recordingStatusUnsubscribe()
    recordingStatusUnsubscribe = null
  }
}
