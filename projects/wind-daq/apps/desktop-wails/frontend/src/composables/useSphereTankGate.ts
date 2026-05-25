import { computed, onBeforeUnmount, onMounted, ref, watch, type Ref } from 'vue'
import { calibrationApi } from '@api/calibrationApi'
import { deviceApi } from '@api/deviceApi'
import type { CalibrationConfig, CalibrationType, ChannelRef, SphereTankGateConfig } from '@shared/types/calibration'

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
  const gateEnabled = ref(false)
  const waitTimeSec = ref(3)
  const stableTimeSec = ref<number | null>(null)
  const isSaving = ref(false)

  const isActive = computed(() => gateEnabled.value && stableTimeSec.value !== null)

  const statusText = computed(() => {
    if (!gateEnabled.value) return '未启用'
    if (stableTimeSec.value === null) return '暂无数据'
    if (stableTimeSec.value >= waitTimeSec.value) return '稳定'
    return '等待中'
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

  function updateStableTimeFromSnapshots(config: CalibrationConfig, snapshots: import('@api/types').DataPayload[]): void {
    const gate = config.sphereTankGate
    if (!gate) {
      stableTimeSec.value = null
      return
    }
    const payload = snapshots.find((s) => s.deviceId === gate.stableTimeChannel.deviceId)
    if (!payload) {
      stableTimeSec.value = null
      return
    }
    const idx = payload.channelIndices.indexOf(gate.stableTimeChannel.channelIndex)
    if (idx < 0) {
      stableTimeSec.value = null
      return
    }
    const raw = payload.channels[idx]
    stableTimeSec.value = typeof raw === 'number' && Number.isFinite(raw) && raw >= 0 ? raw : null
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
        throw new Error(saveRes.error || '保存球罐判定配置失败')
      }

      const runtimeRes = await calibrationApi.updateSphereTankGate(gate)
      if (!runtimeRes.success) {
        throw new Error(runtimeRes.error || '更新运行中球罐判定配置失败')
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
    unsubscribe = deviceApi.onSnapshot((payload: import('@api/types').DataPayload) => {
      const config = options.config.value
      if (!config) return
      const snapshots = Array.isArray(payload) ? payload : [payload]
      updateStableTimeFromSnapshots(config, snapshots)
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
    statusText,
    isActive,
    isSaving,
    saveGate,
  }
}
