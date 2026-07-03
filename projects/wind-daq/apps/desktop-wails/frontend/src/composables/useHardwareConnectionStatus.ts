// 硬件连接状态显示 composable
//
// 把 TraversalMain.vue 中重复的 "运动器/采集设备 连接状态 → 显示" 逻辑提取到 composable，
// 让组件只关心模板与交互。其他遍历相关页面（独立运动控制器窗口、状态卡片等）也可复用。
//
// 暴露：
//   - axisPositions：每个配置的运动轴当前位置/是否运动中
//   - positionerConnection：连接显示模型（state/label/dotColor/textColor/dotGlow）
//   - acquisitionConnection：采集设备连接显示模型，比 positioner 多一个 acquiring 状态
//
// 入参 currentConfig 必须是响应式 ref，本 composable 内部 watch 它。
//
// Phase B (2026-06): 颜色值改为 CSS 变量字符串（token），消费方用 :style 绑定。
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

// 连接状态显示模型，模板直接绑定 :style
export interface ConnectionDisplay<S extends string> {
  state: S
  label: string
  /** Dot background color (CSS value). */
  dotColor: string
  /** Optional glow shadow when the state is "live" (connected/acquiring). */
  dotGlow: string
  /** Text color (CSS value). */
  textColor: string
}

// ── 状态 → token 配色映射（模块作用域常量）─────────────

const POSITIONER_DOT_COLOR: Record<PositionerConnectionState, string> = {
  connected: 'var(--state-success)',
  disconnected: 'var(--state-error)',
  unconfigured: 'var(--text-muted)',
}

const POSITIONER_DOT_GLOW: Record<PositionerConnectionState, string> = {
  connected: '0 0 8px color-mix(in srgb, var(--state-success) 60%, transparent)',
  disconnected: '0 0 8px color-mix(in srgb, var(--state-error) 60%, transparent)',
  unconfigured: 'none',
}

const POSITIONER_TEXT_COLOR: Record<PositionerConnectionState, string> = {
  connected: 'var(--state-success)',
  disconnected: 'var(--state-error)',
  unconfigured: 'var(--text-muted)',
}

const ACQUISITION_DOT_COLOR: Record<AcquisitionState, string> = {
  acquiring: 'var(--state-success)',
  connected: 'var(--state-warning)',
  disconnected: 'var(--state-error)',
  unconfigured: 'var(--text-muted)',
}

const ACQUISITION_DOT_GLOW: Record<AcquisitionState, string> = {
  acquiring: '0 0 8px color-mix(in srgb, var(--state-success) 60%, transparent)',
  connected: 'none',
  disconnected: '0 0 8px color-mix(in srgb, var(--state-error) 60%, transparent)',
  unconfigured: 'none',
}

const ACQUISITION_TEXT_COLOR: Record<AcquisitionState, string> = {
  acquiring: 'var(--state-success)',
  connected: 'var(--state-warning)',
  disconnected: 'var(--state-error)',
  unconfigured: 'var(--text-muted)',
}

/**
 * 构造连接状态显示模型。颜色映射通过参数注入，label 由调用方本地化。
 */
function buildDisplay<S extends string>(
  state: S,
  dotColors: Record<S, string>,
  dotGlows: Record<S, string>,
  textColors: Record<S, string>,
  label: string
): ConnectionDisplay<S> {
  return {
    state,
    label,
    dotColor: dotColors[state],
    dotGlow: dotGlows[state],
    textColor: textColors[state],
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
  //
  // label 使用遍历方向名（cfg.name: 'X' | 'Y'）而非物理轴号（cfg.axis: 'X' | 'Y' | 'Z' | 'U'），
  // 因为操作员脑内模型是"遍历方向"，物理轴号属于配置细节。布点形状（矩形/弧形）不影响方向命名。
  // 不写死单位（mm/°），避免平移台/旋转台混用时误导（历史 bug：原"当前点位"section 写死 ° 单位）。
  const axisPositions = computed<AxisPositionDatum[]>(() => {
    const axes = currentConfig.value?.channels.motionAxes ?? []
    return axes.map((cfg) => {
      const status = motionStore.statusById(cfg.controllerId)
      const axisStatus = status?.axes.find((a) => a.name === cfg.axis)
      return {
        label: cfg.name === 'X' ? t.value.currentPointX : t.value.currentPointY,
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

    return buildDisplay(
      state,
      POSITIONER_DOT_COLOR,
      POSITIONER_DOT_GLOW,
      POSITIONER_TEXT_COLOR,
      positionerLabel(state)
    )
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

    return buildDisplay(
      state,
      ACQUISITION_DOT_COLOR,
      ACQUISITION_DOT_GLOW,
      ACQUISITION_TEXT_COLOR,
      acquisitionLabel(state)
    )
  })

  return {
    axisPositions,
    positionerConnection,
    acquisitionConnection
  }
}
