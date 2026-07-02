import { defineStore } from 'pinia'
import { ref } from 'vue'
import * as bridge from '@bridge/recordingBridge'
import type { RecordingSession } from '@bridge/recordingBridge'

export const useRecordingStore = defineStore('recording', () => {
  const isRecording = ref(false)
  const outputDir = ref('')
  const filePrefix = ref('')
  const snapshotCount = ref(0)
  const droppedCount = ref(0)
  const fileCount = ref(0)
  const currentFile = ref('')
  const lastError = ref('')

  function handleRecordingEvent(session: RecordingSession): void {
    // status: 0=Idle 1=Active 2=Stopping。Stopping 仍视为录制中（drain 队列未完成）
    isRecording.value = session.status === 1 || session.status === 2
    snapshotCount.value = session.snapshotCount ?? 0
    droppedCount.value = session.droppedCount ?? 0
    fileCount.value = session.fileCount ?? 0
    lastError.value = session.lastError ?? ''
    if (session.outputDir) {
      outputDir.value = session.outputDir
    }
    if (session.filePrefix) {
      filePrefix.value = session.filePrefix
    }
    // 当前文件路径：session.currentFile 缺省时保留历史值（例如停止录制后仍显示最后写入的文件）
    if (session.currentFile) {
      currentFile.value = session.currentFile
    }
  }

  function startListening(): void {
    bridge.onRecordingStatus(handleRecordingEvent)
  }

  function stopListening(): void {
    bridge.offRecordingStatus()
  }

  async function startRecording(
    dir: string,
    prefix: string,
  ): Promise<void> {
    await bridge.startRecording(dir, prefix)
    isRecording.value = true
    outputDir.value = dir
    filePrefix.value = prefix
    snapshotCount.value = 0
    droppedCount.value = 0
    fileCount.value = 0
    // currentFile 由后端 emit 的 recording-status 事件携带，启动后立即清空旧值
    // 以避免显示上一次会话的文件路径
    currentFile.value = ''
    lastError.value = ''
    // 主动拉取一次状态，确保后端已同步创建首文件后立即在前端展示完整路径
    await refreshStatus()
  }

  async function stopRecording(): Promise<void> {
    await bridge.stopRecording()
    isRecording.value = false
  }

  async function refreshStatus(): Promise<void> {
    const session = await bridge.getRecordingStatus()
    handleRecordingEvent(session)
  }

  return {
    isRecording, outputDir, filePrefix,
    snapshotCount, droppedCount, fileCount, currentFile, lastError,
    startListening, stopListening,
    startRecording, stopRecording, refreshStatus,
  }
})
