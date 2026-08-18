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
import { getTraversalDisplayedAxes } from '@shared/types/traversal'
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
  // label 使用遍历方向名（cfg.name: 'X' | 'Y' | 'Z' | 'U'）而非物理轴号（cfg.axis），
  // 因为操作员脑内模型是"遍历方向"，物理轴号属于配置细节。布点形状（矩形/弧形）不影响方向命名。
  // 不写死单位（mm/°），避免平移台/旋转台混用时误导（历史 bug：原"当前点位"section 写死 ° 单位）。
  //
  // 显示行数按布点模式过滤（getTraversalDisplayedAxes，与配置屏同一真相源）：
  // line 仅 X，rectangle/sector 为 X/Y，custom 为 X/Y/Z/U——未参与运动的轴不显示，
  // 避免 line 模式下 Z/U 行恒显示 "--" 造成"轴掉线"的误读。
  // 方向名 → 本地化轴标签的映射用 Record 而非三元，扩展第五轴时仅改映射表。
  const axisDirectionLabel: Record<'X' | 'Y' | 'Z' | 'U', () => string> = {
    X: () => t.value.currentPointX,
    Y: () => t.value.currentPointY,
    Z: () => t.value.currentPointZ,
    U: () => t.value.currentPointU
  }
  const axisPositions = computed<AxisPositionDatum[]>(() => {
    const config = currentConfig.value
    if (!config) return []
    // pattern 缺失（旧配置/异常数据）时回退 rectangle 两轴视图，与旧固定两轴运行屏行为一致
    const axes = getTraversalDisplayedAxes(config.layout?.pattern ?? 'rectangle', config.channels.motionAxes ?? [])
    return axes.map((cfg) => {
      const status = motionStore.statusById(cfg.controllerId)
      const axisStatus = status?.axes.find((a) => a.name === cfg.axis)
      return {
        label: axisDirectionLabel[cfg.name]?.() ?? cfg.name,
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
    const config = currentConfig.value
    const motionAxes = config
      ? getTraversalDisplayedAxes(config.layout?.pattern ?? 'rectangle', config.channels.motionAxes ?? [])
      : []

    // 与后端 CheckPreconditions / RunCurrentPoint 保持同一回退语义：
    //   1. 收集所有 status 的 ID 集合（不区分 Connected）——"已知控制器"集合；
    //   2. 若所有非空 controllerId 都不匹配任何已知控制器（典型场景：旧配置保存了
    //      别名 sim-motion-1 / 控制器名 / 旧 UUID），统一回退到「按轴名匹配」；
    //   3. 否则保持严格 ID 绑定——部分有效 ID 时不回退，未匹配的 binding 视为断开。
    // 前端不能直接做本地严格 ID 检查作为硬门禁，否则旧配置即使后端可解析也会被禁用启动。
    // 注意：用「已知控制器」而非「已连接控制器」判断回退——避免用户显式绑定了一个
    // disconnected 控制器时被静默回退到其他已连接控制器（与后端 resolveMotionAxes 一致）。
    const statuses = motionStore.statusList
    const knownIds = new Set(statuses.map((s) => s.id))
    const nonEmptyIds = motionAxes
      .map((ax) => ax.controllerId?.trim())
      .filter((id): id is string => Boolean(id))
    const anyMatched = nonEmptyIds.some((id) => knownIds.has(id))
    const allUnmatched = nonEmptyIds.length > 0 && !anyMatched
    // resolveMotionAxes 的回退结果：回退时把 controllerId 清空，让后续按 axis 名匹配
    const effectiveAxes = motionAxes.map((ax) => ({
      ...ax,
      controllerId: allUnmatched ? '' : ax.controllerId?.trim() ?? ''
    }))

    let state: PositionerConnectionState = 'unconfigured'
    if (motionAxes.length > 0) {
      // 复刻 validateMotionAxisConnections：每个 binding 必须能在已连接控制器中
      // 找到匹配（ID 匹配或回退后按 axis 名匹配），全部通过才视为已连接。
      const allConnected = effectiveAxes.every((binding) => {
        const axisName = binding.axis
        return statuses.some((s) => {
          if (!s.connected || s.emergencyStopped) return false
          if (binding.controllerId && s.id !== binding.controllerId) return false
          return s.axes.some((a) => a.name === axisName)
        })
      })
      state = allConnected ? 'connected' : 'disconnected'
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
      // "已连接"语义与后端 DeviceManager.IsConnected 对齐：非 Disconnected 且非 Error 即视为已连接。
      // 不能用 === 'Connected' 严格判断——adapter 在 StartAcquisition 时会把 Connection
      // 改为 'Acquiring'，若严格要求 'Connected' 会导致设备正在采集时被误判为 disconnected，
      // 侧边栏状态停留在"已连接"而无法切换到"采集中"。
      const allConnected = deviceIds.every((deviceId) => {
        const conn = deviceStore.statusFor(deviceId)
        return conn !== 'Disconnected' && conn !== 'Error'
      })
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
