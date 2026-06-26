import {
  StartRecording,
  StopRecording,
  GetRecordingStatus,
  PickDirectory,
} from '../../bindings/daq-p1604/backend/app'
import { Events } from '@wailsio/runtime'

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
  return PickDirectory() as any
}

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

let recordingStatusUnsubscribe: (() => void) | null = null
