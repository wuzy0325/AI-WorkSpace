import {
  StartRecording,
  StopRecording,
  GetRecordingStatus,
  PickDirectory,
} from '../../wailsjs/go/backend/App'
import { EventsOn, EventsOff } from '../../wailsjs/runtime/runtime'

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

export function onRecordingStatus(handler: (session: RecordingSession) => void): void {
  EventsOn('daq:recording-status', handler)
}

export function offRecordingStatus(): void {
  EventsOff('daq:recording-status')
}
