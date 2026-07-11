import { computed, onUnmounted, type ComputedRef } from 'vue'
import type { DeviceProfile } from '@api/types'
import { useDeviceStore } from '@stores/deviceStore'
import { useFeedbackStore } from '@stores/feedbackStore'
import { useI18nStore } from '@stores/i18nStore'

export function useDeviceZeroCalibration(profile: ComputedRef<DeviceProfile | undefined>) {
  const deviceStore = useDeviceStore()
  const feedbackStore = useFeedbackStore()
  const i18n = useI18nStore()
  const operation = computed(() => profile.value ? deviceStore.calibrationOperationFor(profile.value.id) : undefined)
  const isCalibrating = computed(() => operation.value?.state === 'running')

  async function run(channelIndex?: number): Promise<void> {
    const id = profile.value?.id
    if (!id) {
      // 防御：设备切换瞬间 profile 可能为 undefined，原本静默返回会让用户以为按钮失灵。
      feedbackStore.pushToast(i18n.t.emptyDeviceSelectionTitle || '未选择设备', 'warning')
      return
    }
    try {
      await deviceStore.calibrate(id, channelIndex)
      // 区分单通道 vs 全通道校零的成功文案
      const msg = channelIndex == null
        ? (i18n.t.tareAllChannelsComplete || '全部通道校零完成')
        : (i18n.t.tareComplete || '校零完成')
      feedbackStore.pushToast(msg, 'success')
    } catch (error) {
      const current = deviceStore.calibrationOperationFor(id)
      if (current?.state === 'cancelled') {
        // 取消校零补充 info 级反馈，让用户知道取消已生效（原先静默无任何提示）。
        feedbackStore.pushToast(i18n.t.tareCancelled || '校零已取消', 'info')
        return
      }
      feedbackStore.pushToast(error instanceof Error ? error.message : String(error), 'error')
    }
  }

  async function setEnabled(channelIndex: number, enabled: boolean): Promise<void> {
    const id = profile.value?.id
    if (!id) return
    try {
      await deviceStore.setCalibrationEnabled(id, channelIndex, enabled)
    } catch (error) {
      feedbackStore.pushToast(error instanceof Error ? error.message : String(error), 'error')
    }
  }

  function cancel(): void {
    if (profile.value) deviceStore.cancelCalibration(profile.value.id)
  }

  // 组件卸载时取消正在进行的校零，避免 HTTP 连接残留 5 秒后才被后端 deadline 终止。
  // 此前未清理会导致组件重建后无法立即重新校零（store 层 calibrationControllers 仍持有旧 controller）。
  onUnmounted(() => {
    if (profile.value) deviceStore.cancelCalibration(profile.value.id)
  })

  return { operation, isCalibrating, run, setEnabled, cancel }
}
