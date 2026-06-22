// 硬件连接状态显示 composable
//
// 把 TraversalMain.vue 中重复的 "运动器/采集设备 连接状态 → 显示" 逻辑提取到 composable，
// 让组件只关心模板与交互。其他遍历相关页面（独立运动控制器窗口、状态卡片等）也可复用。
//
// 暴露：
//   - axisPositions：每个配置的运动轴当前位置/是否运动中
//   - positionerConnection：连接显示模型（state/label/dotClass/textClass）
//   - acquisitionConnection：采集设备连接显示模型，比 positioner 多一个 acquiring 状态
//
// 入参 currentConfig 必须是响应式 ref，本 composable 内部 watch 它。
import { computed, type ComputedRef } from 'vue'
import type { TraversalTestConfig } from '@shared/types/traversal'
import { useDeviceStore } from '@stores/deviceStore'
import { useMotionStore } from '@stores/motionStore'
import { useI18nStore } from '@stores/i18nStore'

// 单根轴的位置展示数据
interface AxisPositionDatum {
  label: string
  position: number | undefined
  moving: boolean
}

// 运动器连接状态：unconfigured 表示尚未配置任何控制器
type PositionerConnectionState = 'connected' | 'disconnected' | 'unconfigured'

// 采集设备连接状态：相对运动器多一个 acquiring（连上且采集中）
type AcquisitionState = 'acquiring' | 'connected' | 'disconnected' | 'unconfigured'

// 连接状态显示模型，模板直接绑定
interface ConnectionDisplay<S extends string> {
  state: S
  label: string
  dotClass: string
  textClass: string
}

// ── 状态 → class 配色映射（模块作用域常量，避免每次 computed 重新构造）─────────────

const POSITIONER_DOT_CLASS: Record<PositionerConnectionState, string> = {
  connected: 'bg-emerald-500 shadow-[0_0_8px_#10b981]',
  disconnected: 'bg-rose-500 shadow-[0_0_8px_#f43f5e]',
  unconfigured: 'bg-slate-400'
}

const POSITIONER_TEXT_CLASS: Record<PositionerConnectionState, string> = {
  connected: 'text-emerald-600 dark:text-emerald-400',
  disconnected: 'text-rose-600 dark:text-rose-400',
  unconfigured: 'text-slate-500 dark:text-slate-400'
}

const ACQUISITION_DOT_CLASS: Record<AcquisitionState, string> = {
  acquiring: 'bg-emerald-500 shadow-[0_0_8px_#10b981]',
  connected: 'bg-amber-400',
  disconnected: 'bg-rose-500 shadow-[0_0_8px_#f43f5e]',
  unconfigured: 'bg-slate-400'
}

const ACQUISITION_TEXT_CLASS: Record<AcquisitionState, string> = {
  acquiring: 'text-emerald-600 dark:text-emerald-400',
  connected: 'text-amber-600 dark:text-amber-400',
  disconnected: 'text-rose-600 dark:text-rose-400',
  unconfigured: 'text-slate-500 dark:text-slate-400'
}

/**
 * 构造连接状态显示模型。dotClasses/textClasses 携带每个 state 对应的 Tailwind class，
 * label 由调用方提供（已本地化）。
 */
function buildDisplay<S extends string>(
  state: S,
  dotClasses: Record<S, string>,
  textClasses: Record<S, string>,
  label: string
): ConnectionDisplay<S> {
  return {
    state,
    label,
    dotClass: dotClasses[state],
    textClass: textClasses[state]
  }
}

/**
 * 硬件连接状态显示
 *
 * @param currentConfig 当前遍历配置（响应式）。取消激活时传入 null 即可。
 */
export function useHardwareConnectionStatus(
  currentConfig: ComputedRef<TraversalTestConfig | null>
) {
  const deviceStore = useDeviceStore()
  const motionStore = useMotionStore()
  const i18n = useI18nStore()
  const t = computed(() => i18n.t)

  // 轴位置：computed 直接产出对象数组，依赖追踪由 Vue 自动完成；
  // 模板对该数组按引用渲染，position/moving 任一变化都会触发更新。
  const axisPositions = computed<AxisPositionDatum[]>(() => {
    const axes = currentConfig.value?.channels.motionAxes ?? []
    return axes.map((cfg) => {
      const status = motionStore.statusById(cfg.controllerId)
      const axisStatus = status?.axes.find((a) => a.name === cfg.axis)
      return {
        label: cfg.axis,
        position: axisStatus?.position,
        moving: axisStatus?.moving ?? false
      }
    })
  })

  function positionerLabel(state: PositionerConnectionState): string {
    return state === 'unconfigured' ? t.value.unconfigured : t.value[state]
  }

  function acquisitionLabel(state: AcquisitionState): string {
    return state === 'unconfigured' ? t.value.unconfigured : t.value[state]
  }

  const positionerConnection = computed(() => {
    const controllerIds = Array.from(
      new Set(
        (currentConfig.value?.channels.motionAxes ?? [])
          .map((axis) => axis.controllerId?.trim())
          .filter((controllerId): controllerId is string => Boolean(controllerId))
      )
    )

    let state: PositionerConnectionState = 'unconfigured'
    if (controllerIds.length > 0) {
      state = controllerIds.every((controllerId) => motionStore.statusById(controllerId)?.connected)
        ? 'connected'
        : 'disconnected'
    }

    return buildDisplay(state, POSITIONER_DOT_CLASS, POSITIONER_TEXT_CLASS, positionerLabel(state))
  })

  const acquisitionConnection = computed(() => {
    const deviceIds = Array.from(
      new Set(
        (currentConfig.value?.channels.probeChannels ?? [])
          .filter((channel) => channel.enabled)
          .map((channel) => channel.channel.deviceId?.trim())
          .filter((deviceId): deviceId is string => Boolean(deviceId))
      )
    )

    let state: AcquisitionState = 'unconfigured'
    if (deviceIds.length > 0) {
      const allConnected = deviceIds.every((deviceId) => deviceStore.statusFor(deviceId) === 'Connected')
      if (!allConnected) {
        state = 'disconnected'
      } else {
        // 全部连接且全部采集中 → acquiring；仅连接 → connected
        const allAcquiring = deviceIds.every((deviceId) => deviceStore.acquiringFor(deviceId))
        state = allAcquiring ? 'acquiring' : 'connected'
      }
    }

    return buildDisplay(state, ACQUISITION_DOT_CLASS, ACQUISITION_TEXT_CLASS, acquisitionLabel(state))
  })

  return {
    axisPositions,
    positionerConnection,
    acquisitionConnection
  }
}
