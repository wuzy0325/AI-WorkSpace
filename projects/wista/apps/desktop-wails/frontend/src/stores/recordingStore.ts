import { defineStore } from 'pinia'
import { ref } from 'vue'
import * as bridge from '@bridge/recordingBridge'
import type { RecordingSession } from '@bridge/recordingBridge'
import { useI18nStore } from '@stores/i18nStore'

export const useRecordingStore = defineStore('recording', () => {
  const i18n = useI18nStore()
  const isRecording = ref(false)
  const outputDir = ref('')
  const filePrefix = ref('')
  const snapshotCount = ref(0)
  /** 录制不可恢复错误（I/O 失败），状态栏展示，开始新会话时清空 */
  const lastError = ref('')
  /** 背压丢帧总数（累计） */
  const droppedCount = ref(0)

  function handleRecordingEvent(session: RecordingSession): void {
    // status: 0=Idle 1=Active 2=Stopping。Stopping 仍视为录制中（drain 队列未完成），
    // 否则 UI 会在用户点 Stop 后立即切回"未录制"，但后端仍在写盘，状态栏会出现短暂不一致。
    isRecording.value = session.status === 1 || session.status === 2
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
    try {
      await bridge.startRecording(dir, prefix)
    } catch (err) {
      // REC-018：录制启动失败（路径不存在/权限不足/磁盘满等）时，
      // 把友好错误消息写入 lastError，让状态栏红色显示给操作员，
      // 而不是让错误冒泡到全局未处理 rejection。
      // 关键词覆盖三类常见 OS 错误消息形态：
      //   - Windows: "The system cannot find the path specified"
      //   - Linux:   "no such file or directory"
      //   - Go:      "open <path>: The system cannot find the path specified."
      // 录制是文件 I/O，不会出现 "device does not exist" 等设备通信错误，无需上下文限定。
      const reason = err instanceof Error ? err.message : String(err)
      // 英文关键词用 \b 单词边界限定，避免 "permission_token" 等非错误场景误判；
      // 中文关键词不用 \b（JS \b 对中文 \w 行为依赖引擎，可能漏判），改为独立分支。
      const friendly = /\b(not found|no such file|cannot find)\b/i.test(reason)
        || /不存在|无法找到/.test(reason)
        ? i18n.t('error.recordingDirNotFound', { dir })
        : /\b(permission|denied)\b/i.test(reason) || /权限/.test(reason)
          ? i18n.t('error.recordingPermissionDenied', { dir })
          : i18n.t('error.recordingStartFailed', { reason })
      lastError.value = friendly
      throw new Error(friendly)
    }
    isRecording.value = true
    outputDir.value = dir
    filePrefix.value = prefix
    snapshotCount.value = 0
    // 重置上次会话的错误与丢帧计数
    lastError.value = ''
    droppedCount.value = 0
  }

  async function stopRecording(): Promise<void> {
    await bridge.stopRecording()
    isRecording.value = false
  }

  async function refreshStatus(): Promise<void> {
    const session = await bridge.getRecordingStatus()
    handleRecordingEvent(session)
  }

  /** 收到录制 fatal 事件：标记错误并停止录制显示 */
  function handleFatalError(error: string): void {
    lastError.value = error
    isRecording.value = false
  }

  /** 收到背压丢帧事件：直接赋值后端累计值（避免多次事件累加偏离真实值） */
  function handleBackpressure(droppedTotal: number): void {
    droppedCount.value = droppedTotal
  }

  /** 清空错误（用户手动清除） */
  function clearError(): void {
    lastError.value = ''
  }

  return {
    isRecording, outputDir, filePrefix, snapshotCount, lastError, droppedCount,
    startListening, stopListening,
    startRecording, stopRecording, refreshStatus,
    handleFatalError, handleBackpressure, clearError,
  }
})
