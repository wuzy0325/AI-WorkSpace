import { computed, onBeforeUnmount, onMounted, ref, watch, type Ref } from 'vue'
import { calibrationApi } from '@api/calibrationApi'
import { deviceApi } from '@api/deviceApi'
import { useI18nStore } from '@stores/i18nStore'
import { findChannelValue } from '@shared/calibrationSnapshotValue'
import type { CalibrationConfig, CalibrationType, ChannelRef, SphereTankGateConfig } from '@shared/types/calibration'
import type { DataPayload } from '@api/types'

interface UseSphereTankGateOptions {
  calibrationType: CalibrationType
  config: { value: CalibrationConfig | null } | Ref<CalibrationConfig | null>
  onError: (message: string) => void
}

function cloneConfig(config: CalibrationConfig): CalibrationConfig {
  return JSON.parse(JSON.stringify(config)) as CalibrationConfig
}

function pickDefaultChannel(config: CalibrationConfig): ChannelRef {
  const firstProbe = config.probeChannels.find((ch) => ch.enabled && ch.channel.deviceId)
  if (firstProbe) {
    return { ...firstProbe.channel }
  }
  return { deviceId: '', channelIndex: 0 }
}

export function useSphereTankGate(options: UseSphereTankGateOptions) {
  const i18n = useI18nStore()
  const gateEnabled = ref(false)
  const waitTimeSec = ref(3)
  const stableTimeSec = ref<number | null>(null)
  // pressureValue 实时球罐压力值（从 pressureChannel 读取），仅用于 UI 显示，不参与闸门判定
  const pressureValue = ref<number | null>(null)
  const isSaving = ref(false)

  const isActive = computed(() => gateEnabled.value && stableTimeSec.value !== null)

  const statusText = computed(() => {
    if (!gateEnabled.value) return i18n.t.wf_sphereGateNotEnabled
    if (stableTimeSec.value === null) return i18n.t.wf_sphereGateNoData
    if (stableTimeSec.value >= waitTimeSec.value) return i18n.t.wf_sphereGateStable
    return i18n.t.wf_sphereGateWaiting
  })

  function ensureGate(config: CalibrationConfig): SphereTankGateConfig {
    if (config.sphereTankGate) {
      return config.sphereTankGate
    }
    return {
      enabled: false,
      waitTimeSec: 3,
      stableTimeChannel: pickDefaultChannel(config),
    }
  }

  function syncFromConfig(config: CalibrationConfig | null): void {
    if (!config) return
    const gate = ensureGate(config)
    gateEnabled.value = gate.enabled
    waitTimeSec.value = gate.waitTimeSec
  }

  // 快照累积缓存：多设备快照是分批推送的（每次只含一个设备的最新数据），
  // 必须用 Map 按 deviceId 累积保留各设备最新快照，才能让 findChannelValue 总能找到目标设备。
  // 与 SevenHoleMain / FiveHoleMain / ThreeHoleMain / TotalPressureMain 的实时订阅模式一致。
  const latestSnapshots = new Map<string, DataPayload>()

  function updateFromSnapshots(config: CalibrationConfig): void {
    const gate = config.sphereTankGate
    if (!gate) {
      // 配置中不存在球罐门控时才清空
      stableTimeSec.value = null
      pressureValue.value = null
      return
    }

    // 稳定时间通道：参与闸门判定
    // findChannelValue 返回 null 表示缓存中尚无该设备/通道数据（首次订阅前），此时保持上次值
    const snapshots = Array.from(latestSnapshots.values())
    const stableRaw = findChannelValue(snapshots, gate.stableTimeChannel.deviceId, gate.stableTimeChannel.channelIndex)
    if (stableRaw !== null && Number.isFinite(stableRaw) && stableRaw >= 0) {
      stableTimeSec.value = stableRaw
    }

    // 压力通道：仅用于前端显示，未配置时清空
    if (!gate.pressureChannel || !gate.pressureChannel.deviceId) {
      pressureValue.value = null
      return
    }
    const pressureRaw = findChannelValue(snapshots, gate.pressureChannel.deviceId, gate.pressureChannel.channelIndex)
    if (pressureRaw !== null && Number.isFinite(pressureRaw)) {
      pressureValue.value = pressureRaw
    }
    // 非数值/未找到时保持上次值，避免压力数字闪烁回退
  }

  async function saveGate(nextEnabled: boolean, nextWaitTimeSec: number): Promise<void> {
    const config = options.config.value
    if (!config) return

    const gate = ensureGate(config)
    gate.enabled = nextEnabled
    gate.waitTimeSec = Math.max(0, nextWaitTimeSec)
    config.sphereTankGate = gate

    isSaving.value = true
    try {
      const snapshot = cloneConfig(config)
      const saveRes = await calibrationApi.saveConfig(options.calibrationType, snapshot)
      if (!saveRes.success) {
        throw new Error(saveRes.error || i18n.t.wf_sphereGateSaveFailed)
      }

      gateEnabled.value = gate.enabled
      waitTimeSec.value = gate.waitTimeSec
    } catch (error) {
      options.onError(error instanceof Error ? error.message : String(error))
    } finally {
      isSaving.value = false
    }
  }

  let unsubscribe: (() => void) | null = null
  onMounted(() => {
    unsubscribe = deviceApi.onSnapshot((payload: DataPayload) => {
      const config = options.config.value
      if (!config) return
      // 累积各设备最新快照：payload 是单设备快照，set 到 Map 后用全部 values 重算
      latestSnapshots.set(payload.deviceId, payload)
      updateFromSnapshots(config)
    })
  })

  onBeforeUnmount(() => {
    if (unsubscribe) {
      unsubscribe()
      unsubscribe = null
    }
  })

  watch(
    () => options.config.value,
    (next) => {
      syncFromConfig(next)
    },
    { immediate: true },
  )

  return {
    gateEnabled,
    waitTimeSec,
    stableTimeSec,
    pressureValue,
    statusText,
    isActive,
    isSaving,
    saveGate,
  }
}
