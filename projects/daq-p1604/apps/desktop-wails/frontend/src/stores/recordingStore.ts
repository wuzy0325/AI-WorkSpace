import { defineStore } from 'pinia'
import { ref } from 'vue'
import * as bridge from '@bridge/recordingBridge'
import type { RecordingSession } from '@bridge/recordingBridge'

export const useRecordingStore = defineStore('recording', () => {
  const isRecording = ref(false)
  const outputDir = ref('')
  const filePrefix = ref('')
  const snapshotCount = ref(0)

  function handleRecordingEvent(session: RecordingSession): void {
    isRecording.value = session.status === 1
    snapshotCount.value = session.snapshotCount ?? 0
    if (session.outputDir) {
      outputDir.value = session.outputDir
    }
    if (session.filePrefix) {
      filePrefix.value = session.filePrefix
    }
  }

  function startListening(): void {
    bridge.onRecordingStatus(handleRecordingEvent)
  }

  function stopListening(): void {
    bridge.offRecordingStatus()
  }

  async function startRecording(dir: string, prefix: string): Promise<void> {
    await bridge.startRecording(dir, prefix)
    isRecording.value = true
    outputDir.value = dir
    filePrefix.value = prefix
    snapshotCount.value = 0
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
    isRecording, outputDir, filePrefix, snapshotCount,
    startListening, stopListening,
    startRecording, stopRecording, refreshStatus,
  }
})
