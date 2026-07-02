import {
  StartRecording,
  StartRecordingWithConfig,
  StopRecording,
  GetRecordingStatus,
  PickDirectory,
} from '../../bindings/daq-p1604/backend/app'
import { Events } from '@wailsio/runtime'

// 文件滚动条件（任一满足即滚动到新文件，0 表示不限制）
export interface FileRotation {
  maxSizeBytes?: number
  maxDurationMs?: number
  maxRecordCount?: number
}

// 自动停止条件（任一满足即停止整个录制，0 表示不限制）
export interface StopConditions {
  maxDurationMs?: number
  maxFileSizeBytes?: number
  maxRecordCount?: number
}

export interface RecordingSession {
  id: string
  outputDir: string
  filePrefix: string
  startTimeMs: number
  snapshotCount: number   // 已写入快照数
  droppedCount: number    // 队列满时丢弃的快照数
  fileCount: number       // 已创建的文件数（含滚动）
  currentFile?: string    // 当前正在写入的文件完整路径（文件滚动时更新）
  lastError?: string      // 最近一次错误信息
  status: number          // 0=Idle 1=Active 2=Stopping
}

export function startRecording(
  outputDir: string,
  filePrefix: string,
): Promise<void> {
  return StartRecording(outputDir, filePrefix) as Promise<void>
}

export function startRecordingWithConfig(
  outputDir: string,
  filePrefix: string,
  rotation: FileRotation,
  stopCond: StopConditions,
): Promise<void> {
  return StartRecordingWithConfig(outputDir, filePrefix, rotation as any, stopCond as any) as Promise<void>
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
