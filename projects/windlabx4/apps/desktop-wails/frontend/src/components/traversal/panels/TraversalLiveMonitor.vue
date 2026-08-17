<script setup lang="ts">
/**
 * 遍历测试左侧栏：统一卡片风格，紧凑无滚动。
 *
 * 结构（从顶到底）：
 *   - 目标点 + 实际位置（合并为单行大字卡片）
 *   - 实时插值计算（α/β/Ma/V/P0/Ps）
 *   - 实时压力数据（P1-P5/Patm/Tatm）
 *   - 底部信息条：CSV路径 + 验证警告 + 硬件状态
 *
 * 所有数据项统一使用 bg-panel-strong 圆角卡片，
 * 标签左/数值右，数值字号 20px+，整体填满侧边栏不溢出。
 */
import { computed } from 'vue'
import { Activity, AlertTriangle, Cpu, FileText, Wind } from '@lucide/vue'
import type { PressureItem } from '@composables/useTraversalRealtimeData'
import type { InterpolationResult, MotionSafetyFailure, MotionSafetyVerdict, TraversalCoordPoint, TraversalPattern } from '@shared/types/traversal'
import { getMotionSafetyVerdictLabel, getTraversalDisplayedAxisNames, isMotionSafetyEmergency } from '@shared/types/traversal'

interface ConnectionDisplay {
  label: string
  dotColor: string
  dotGlow: string
  textColor: string
}

interface ActualPosition {
  label: string
  position: number | undefined
  moving: boolean
}

// 目标点坐标允许 null：line 模式 Y 轴标记为 NaN 后后端序列化为 null，
// 表示该轴不参与遍历运动。前端必须在 toFixed 前做 number 类型守卫，
// 否则 null.toFixed 抛 TypeError 导致整个侧边栏组件崩溃（只剩布点图）。
type TargetCoord = number | null

const props = defineProps<{
  // 后端兼容字段仍名为 alpha/beta，实际语义是遍历逻辑目标 X/Y，不是插值结果攻角/侧滑角；
  // z/u 为逻辑目标 Z/U（仅 custom 模式有实际值，其余模式为 null 或缺失）。
  targetPoint: TraversalCoordPoint | undefined
  /** 布点模式：决定目标点面板显示的轴数（line=1、rectangle/sector=2、custom=4）。
   *  undefined（配置未加载）时回退 X/Y 两轴，与旧固定两轴视图行为一致。 */
  pattern: TraversalPattern | undefined
  actualPositions: ActualPosition[]
  machNumber: number | undefined
  velocity: number | undefined
  csvSavePath: string
  lastError: string
  validationWarnings: readonly string[] | undefined
  /** 非致命运行警告（当前唯一来源：回零失败，数据已采完仅提示） */
  warning: string | undefined
  /** 运动安全故障现场快照（仅 lastErrorCode 为运动安全错误码时存在） */
  motionSafetyFailure: MotionSafetyFailure | null | undefined

  acquisitionConnection: ConnectionDisplay
  positionerConnection: ConnectionDisplay
  pressureItems: PressureItem[]
  realtimeResult: InterpolationResult | null
  /**
   * PRB/CSV 插值数据集是否已加载到后端插值器。
   * 用于区分"PRB 未加载"与"插值无效"两种 isValid=false 场景:
   *   - isValid=false + 未加载 → 配置层问题,提示用户导入 PRB(橙色)
   *   - isValid=false + 已加载 → 数据层问题,本点压力异常(红色)
   * 真相源为 traversalStore.hasLoadedInterpolator,后端校验失败时同步置 false。
   */
  hasLoadedInterpolator: boolean

  labels: {
    target: string
    targetXDirection: string
    targetYDirection: string
    targetZDirection: string
    targetUDirection: string
    actual: string
    mach: string
    velocity: string
    realtimeCalculation: string
    /** PRB 未加载时实时插值卡片的状态条文案 */
    interpolationNotLoaded: string
    /** 插值结果无效时实时插值卡片的状态条文案 */
    interpolationInvalid: string
    /** PRB 已加载但还未采到第一帧数据时的状态条文案 */
    interpolationWaitingData: string
    realtimePressureData: string
    alpha: string
    beta: string
    csvPath: string
    validationWarnings: string
    /** 回零失败警告条文案 */
    returnToOriginWarning: string
    hardwareStatus: string
    acquisitionDevice: string
    positionerDevice: string
    moving: string
    /** 运动安全故障告警卡片文案 */
    motionSafetyAlert: string
    motionSafetyAlertEmergency: string
    motionSafetyAxis: string
    motionSafetyTarget: string
    motionSafetyActual: string
    motionSafetyDeviation: string
    motionSafetyPointIndex: string
    motionSafetyController: string
    motionSafetyVerdictOk: string
    motionSafetyVerdictArrived: string
    motionSafetyVerdictDeviation: string
    motionSafetyVerdictCriticalDeviation: string
    motionSafetyVerdictLimitTriggered: string
    motionSafetyVerdictNoProgress: string
    motionSafetyVerdictOvershoot: string
    motionSafetyVerdictStatusUnavailable: string
  }
}>()

const emit = defineEmits<{
  /**
   * PRB 未加载状态下用户点击状态条时触发。
   * 父组件打开 TraversalSettings 对话框,引导用户导入 PRB/CSV。
   * 仅在 interpStatus === 'prb-missing' 时点击有效,避免插值无效时误跳配置。
   */
  'navigate-to-prb': []
}>()

/** 安全格式化坐标：null/undefined/NaN 显示为 '--'，避免 toFixed 崩溃 */
function formatCoord(v: TargetCoord | undefined, digits = 1): string {
  if (typeof v !== 'number' || Number.isNaN(v)) return '--'
  return v.toFixed(digits)
}

// 目标点/实际位置面板显示的轴：按布点模式动态生成（line → X；rectangle/sector → X/Y；
// custom → X/Y/Z/U），与配置屏 TraversalLayoutStep 共用 shared 层同一真相源。
const displayedAxisNames = computed(() => getTraversalDisplayedAxisNames(props.pattern ?? 'rectangle'))

// 轴名 → 目标点字段映射：与后端 status.currentPointCoordinates 对齐（alpha=X、beta=Y、z=Z、u=U）。
// 用 Record 映射代替 switch，扩展第五轴时仅改映射表（与 TraversalLayoutStep directionLabelKey 同模式）。
const targetRows = computed(() => {
  const valueFor: Record<'X' | 'Y' | 'Z' | 'U', TargetCoord | undefined> = {
    X: props.targetPoint?.alpha,
    Y: props.targetPoint?.beta,
    Z: props.targetPoint?.z,
    U: props.targetPoint?.u
  }
  const labelFor: Record<'X' | 'Y' | 'Z' | 'U', string> = {
    X: props.labels.targetXDirection,
    Y: props.labels.targetYDirection,
    Z: props.labels.targetZDirection,
    U: props.labels.targetUDirection
  }
  return displayedAxisNames.value.map((name) => ({ name, label: labelFor[name], value: valueFor[name] }))
})

// 网格列数：单轴（line）独占一行，多轴两列换行（custom 4 轴排成 2×2），
// 目标位置与实际位置两段结构保持完全对称。
const positionGridClass = computed(() =>
  displayedAxisNames.value.length > 1 ? 'grid grid-cols-2 gap-2' : 'grid grid-cols-1 gap-2'
)

/**
 * verdict → 本地化文案映射（响应式）。
 *
 * 以 computed 暴露给模板，使语言切换后 verdict 标签即时刷新。
 * 实际的 verdict 查表与缺省回退由共享函数 getMotionSafetyVerdictLabel 统一维护，
 * 避免与 MotionSafetyAlertCard 各持一份 switch 实现导致行为分叉。
 */
const verdictLabels = computed<Partial<Record<MotionSafetyVerdict, string>>>(() => ({
  ok: props.labels.motionSafetyVerdictOk,
  arrived: props.labels.motionSafetyVerdictArrived,
  deviation: props.labels.motionSafetyVerdictDeviation,
  critical_deviation: props.labels.motionSafetyVerdictCriticalDeviation,
  limit_triggered: props.labels.motionSafetyVerdictLimitTriggered,
  no_progress: props.labels.motionSafetyVerdictNoProgress,
  overshoot: props.labels.motionSafetyVerdictOvershoot,
  status_unavailable: props.labels.motionSafetyVerdictStatusUnavailable
}))

/**
 * 实时插值四态判定:正常 / 未采集(no-data) / PRB 未加载 / 插值无效。
 *
 * 设计动机:旧三态在 realtimeResult=null 时强制 'ok',导致两种用户痛点:
 *   1. PRB 已加载但还没采到第一帧 → 显示 "--" 无状态条,用户误以为"已加载=正常",
 *      但其实插值还无法计算。需要 no-data 态告诉用户"等采集开始"。
 *   2. HTTP 400(PRB 未加载)被旧 store 吞成 null → 状态条被吞,半天看不到提示。
 *      现 store 已改为构造 IsValid=false 占位对象,可走 prb-missing/invalid 分支显示。
 *
 * 真相源 store.hasLoadedInterpolator:
 *   - isValid=true                         → ok,显示真实数值(包括 0 度,零迎角合法)
 *   - realtimeResult=null + 已加载          → no-data,蓝色提示"等待采集数据"
 *   - realtimeResult=null + 未加载          → prb-missing,橙色,可点击跳配置
 *   - isValid=false + !hasLoadedInterpolator → prb-missing,橙色,可点击跳配置
 *   - isValid=false + hasLoadedInterpolator  → invalid,红色,tooltip 显示后端 warning
 *
 * 七孔/五孔探针在此逻辑下行为完全一致,无需按 probeType 分支。
 */
type InterpStatus = 'ok' | 'no-data' | 'prb-missing' | 'invalid'
const interpStatus = computed<InterpStatus>(() => {
  // 已有结果对象:按 isValid 区分 ok / prb-missing / invalid
  if (props.realtimeResult) {
    if (props.realtimeResult.isValid) return 'ok'
    return props.hasLoadedInterpolator ? 'invalid' : 'prb-missing'
  }
  // 无结果对象:按是否已加载区分 no-data / prb-missing
  // 已加载但无结果 → 等待采到第一帧数据(开始遍历前或第一个点运动中)
  // 未加载且无结果 → 提示用户先导入 PRB/CSV,避免点击开始遍历后才在采集循环中报错
  return props.hasLoadedInterpolator ? 'no-data' : 'prb-missing'
})

/**
 * 状态条配色:
 *   - 橙色(state-warning)= 配置层问题(用户可解决,如导入 PRB)
 *   - 红色(accent-danger) = 数据层问题(需排查,如压力越界)
 *   - 蓝色(accent-info)   = 等待状态(系统正常但暂无数据,如已加载未采集)
 *
 * 边框使用 box-shadow inset 而非 border:border 会占据 2px 布局空间,
 * 让状态条总高度(13.2px 内容 + 4px py-0.5 + 2px border = 19.2px)超过标题文字
 * 行高(16px),状态条 v-if 显隐时会撑高标题行 3.2px,导致下方数值网格整体抖动。
 * box-shadow inset 不占据布局空间,状态条高度降为 17.2px,配合标题行 min-h-5
 * (20px)保证显隐都不改变垂直布局。
 */
const statusBarStyle = computed(() => {
  if (interpStatus.value === 'prb-missing') {
    return {
      background: 'color-mix(in srgb, var(--state-warning) 12%, transparent)',
      color: 'var(--state-warning)',
      boxShadow: 'inset 0 0 0 1px var(--state-warning)',
    }
  }
  if (interpStatus.value === 'no-data') {
    return {
      background: 'color-mix(in srgb, var(--accent-info) 12%, transparent)',
      color: 'var(--accent-info)',
      boxShadow: 'inset 0 0 0 1px var(--accent-info)',
    }
  }
  return {
    background: 'color-mix(in srgb, var(--accent-danger) 12%, transparent)',
    color: 'var(--accent-danger)',
    boxShadow: 'inset 0 0 0 1px var(--accent-danger)',
  }
})

const statusBarText = computed(() => {
  if (interpStatus.value === 'prb-missing') return props.labels.interpolationNotLoaded
  if (interpStatus.value === 'no-data') return props.labels.interpolationWaitingData
  return props.labels.interpolationInvalid
})

/** 插值无效时 tooltip 显示后端 warning 全文,便于操作员排查(如"压力差值越界"等) */
const statusBarTooltip = computed(() =>
  interpStatus.value === 'invalid' ? (props.realtimeResult?.warning ?? '') : ''
)

/** PRB 未加载时点击跳转配置;插值无效时不响应(用户需排查数据而非配置) */
function onStatusBarClick() {
  if (interpStatus.value === 'prb-missing') emit('navigate-to-prb')
}
</script>

<template>
  <div class="flex h-full w-96 flex-col flex-shrink-0 border-r border-[var(--border-default)] bg-[var(--bg-panel)] overflow-hidden">
    <!-- 目标点 + 实际位置：同一张卡片内上下两段，结构完全对称（两列 + 大字号 + 标签一致） -->
    <section class="flex-shrink-0 border-b border-[var(--border-default)] p-2.5">
      <div class="rounded-xl bg-[var(--bg-panel-strong)] p-3 space-y-2">
        <!-- 目标位置：行数按布点模式动态生成（line=1、rectangle/sector=2、custom=4），与实际位置结构完全一致 -->
        <div :class="positionGridClass">
          <div
            v-for="row in targetRows"
            :key="row.name"
            class="flex flex-col"
          >
            <span class="mb-0.5 text-[10px] text-[var(--text-muted)]">{{ labels.target }} {{ row.label }}</span>
            <span class="font-mono text-xl font-bold tabular-nums text-[var(--accent-info)]">
              {{ formatCoord(row.value) }}
            </span>
          </div>
        </div>
        <!-- 实际位置：列数与目标位置一致，标签/字号/颜色与目标位置对齐，运动中高亮 -->
        <div :class="[positionGridClass, 'border-t border-[var(--border-default)] pt-2']">
          <template v-if="actualPositions.length">
            <div
              v-for="axis in actualPositions"
              :key="axis.label"
              class="flex flex-col"
            >
              <div class="mb-0.5 flex items-center gap-1.5">
                <span class="text-[10px] text-[var(--text-muted)]">{{ labels.actual }} {{ axis.label }}</span>
                <span
                  v-if="axis.moving"
                  class="ml-auto flex items-center gap-1 text-[10px]"
                  :style="{ color: `var(--accent-success)` }"
                >
                  <span class="h-1.5 w-1.5 animate-pulse rounded-full" :style="{ backgroundColor: `var(--accent-success)` }"></span>
                  {{ labels.moving }}
                </span>
              </div>
              <span
                class="font-mono text-xl font-bold tabular-nums"
                :style="{ color: axis.moving ? `var(--accent-success)` : `var(--text-primary)` }"
              >
                {{ typeof axis.position === 'number' ? axis.position.toFixed(1) : '--' }}
              </span>
            </div>
          </template>
          <template v-else>
            <div
              v-for="placeholder in displayedAxisNames"
              :key="placeholder"
              class="flex flex-col"
            >
              <span class="mb-0.5 text-[10px] text-[var(--text-muted)]">{{ labels.actual }} {{ placeholder }}</span>
              <span class="font-mono text-xl font-bold tabular-nums text-[var(--text-primary)]">--</span>
            </div>
          </template>
        </div>
      </div>
    </section>

    <!-- 实时插值计算：统一卡片，紧凑 py-1.5 -->
    <section class="flex-shrink-0 border-b border-[var(--border-default)] p-2.5">
      <div class="mb-1.5 flex items-center gap-1.5 text-xs font-medium text-[var(--text-primary)] min-h-5">
        <Activity class="h-3.5 w-3.5 text-[var(--accent-info)] flex-shrink-0" />
        <span class="flex-shrink-0">{{ labels.realtimeCalculation }}</span>
        <!-- 状态条:四态视觉区分,与标题同行右侧展示
             - 原独立成行会因 v-if 显隐撑动布局,导致下方数值网格整体跳动
             - 移到同行右侧 + 标题行 min-h-5 + 状态条 box-shadow inset:
               状态条显隐时标题行高度恒为 20px,垂直布局稳定,不再抖动
             - PRB 未加载:橙色,光标 pointer,点击跳配置
             - 已加载未采集(no-data):蓝色提示"等待采集数据",无点击
             - 插值无效:红色,hover tooltip 显示后端 warning 全文
             - 正常:不渲染 -->
        <div
          v-if="interpStatus !== 'ok'"
          class="ml-auto flex items-center gap-1 rounded-md px-2 py-0.5 min-w-0"
          :class="{ 'cursor-pointer': interpStatus === 'prb-missing' }"
          :style="statusBarStyle"
          :title="statusBarTooltip"
          @click="onStatusBarClick"
        >
          <AlertTriangle class="h-3 w-3 flex-shrink-0" />
          <span class="text-[11px] font-medium truncate">{{ statusBarText }}</span>
        </div>
      </div>
      <div class="grid grid-cols-2 gap-1.5">
        <!-- 数值显示规则:isValid=false 时强制 value=undefined,模板兜底显示 '--',
             避免 0 度(零迎角真实值)与无效结果(后端填 0)混淆。
             真实 0 度走 isValid=true 路径,显示 "0.00" 正确。 -->
        <div
          v-for="metric in [
            { label: labels.alpha, value: realtimeResult?.isValid ? realtimeResult.alpha?.toFixed(2) : undefined, unit: '°', accent: true },
            { label: labels.beta, value: realtimeResult?.isValid ? realtimeResult.beta?.toFixed(2) : undefined, unit: '°', accent: true },
            { label: labels.mach, value: realtimeResult?.isValid ? realtimeResult.machNumber?.toFixed(3) : undefined, unit: '', accent: true },
            { label: labels.velocity, value: realtimeResult?.isValid ? realtimeResult.velocity?.toFixed(1) : undefined, unit: 'm/s', accent: true },
            { label: 'P0', value: realtimeResult?.isValid ? realtimeResult.P0?.toFixed(2) : undefined, unit: 'Pa', accent: false },
            { label: 'Ps', value: realtimeResult?.isValid ? realtimeResult.Ps?.toFixed(2) : undefined, unit: 'Pa', accent: false },
          ]"
          :key="metric.label"
          class="flex items-baseline justify-between rounded-lg bg-[var(--bg-panel-strong)] px-3 py-1.5 min-w-0"
        >
          <span class="text-[11px] text-[var(--text-muted)]">{{ metric.label }}</span>
          <div class="flex items-baseline gap-1 min-w-0">
            <span
              class="font-mono text-lg font-bold tabular-nums truncate"
              :style="{ color: metric.accent ? 'var(--accent-info)' : 'var(--text-primary)' }"
            >{{ metric.value ?? '--' }}</span>
            <span
              v-if="metric.unit"
              class="text-[10px] font-medium flex-shrink-0 text-[var(--text-muted)]"
            >{{ metric.unit }}</span>
          </div>
        </div>
      </div>
    </section>

    <!-- 实时压力数据：统一卡片，7 项分两列 -->
    <section class="flex-1 min-h-0 overflow-hidden flex flex-col p-2.5">
      <div class="mb-1.5 flex items-center gap-1.5 text-xs font-medium text-[var(--text-primary)]">
        <Wind class="h-3.5 w-3.5 text-[var(--accent-info)]" />
        {{ labels.realtimePressureData }}
      </div>
      <div class="grid grid-cols-2 gap-1.5 content-start">
        <div
          v-for="item in pressureItems"
          :key="item.key"
          class="flex items-baseline justify-between rounded-lg px-3 py-1.5 min-w-0"
          :style="{
            background: item.disabled
              ? 'color-mix(in srgb, var(--bg-panel-strong) 40%, transparent)'
              : 'var(--bg-panel-strong)',
          }"
        >
          <span class="text-[11px] font-medium" :style="{ color: item.disabled ? 'var(--text-muted)' : 'var(--text-secondary)' }">{{ item.label }}</span>
          <div class="flex items-baseline gap-1 min-w-0">
            <span
              class="font-mono text-lg font-bold tabular-nums truncate"
              :style="{ color: item.disabled ? 'var(--text-muted)' : 'var(--text-primary)' }"
            >{{ item.value }}</span>
            <span
              v-if="item.unit"
              class="text-[10px] font-medium flex-shrink-0"
              :style="{ color: item.disabled ? 'var(--text-muted)' : 'var(--text-secondary)' }"
            >{{ item.unit }}</span>
          </div>
        </div>
      </div>
    </section>

    <!-- 底部信息条：CSV路径 + 验证警告 + 错误 + 硬件状态（固定） -->
    <div class="flex-shrink-0 border-t border-[var(--border-default)] p-2.5 space-y-1.5">
      <!-- CSV 路径 -->
      <div
        v-if="csvSavePath"
        class="flex items-center gap-1.5 rounded-md bg-[var(--bg-panel-strong)] px-2.5 py-1"
        :title="csvSavePath"
      >
        <FileText class="h-3 w-3 text-[var(--text-muted)] flex-shrink-0" />
        <span class="text-[10px] text-[var(--text-muted)] flex-shrink-0">{{ labels.csvPath }}</span>
        <span class="text-[10px] text-[var(--text-secondary)] truncate">{{ csvSavePath }}</span>
      </div>

      <!-- 验证警告 -->
      <div
        v-if="validationWarnings?.length"
        class="flex items-center gap-1.5 rounded-md px-2.5 py-1"
        :style="{
          background: 'color-mix(in srgb, var(--state-warning) 10%, transparent)',
        }"
        :title="validationWarnings.join('\n')"
      >
        <AlertTriangle class="h-3 w-3 text-[var(--state-warning)] flex-shrink-0" />
        <span class="text-[10px] text-[var(--state-warning)]">
          {{ labels.validationWarnings.replace('{count}', String(validationWarnings.length)) }}
        </span>
      </div>

      <!-- 回零警告：数据已全部采完，回零失败不判测试失败，仅以 warning 样式提示 -->
      <div
        v-if="warning"
        class="flex items-center gap-1.5 rounded-md px-2.5 py-1"
        :style="{
          background: 'color-mix(in srgb, var(--state-warning) 10%, transparent)',
        }"
        :title="warning"
      >
        <AlertTriangle class="h-3 w-3 text-[var(--state-warning)] flex-shrink-0" />
        <span class="text-[10px] text-[var(--state-warning)] truncate max-w-[220px]">
          {{ labels.returnToOriginWarning }}
        </span>
      </div>

      <!-- 错误信息 -->
      <div
        v-if="lastError"
        class="flex items-center gap-1.5 rounded-md px-2.5 py-1"
        :style="{
          background: 'color-mix(in srgb, var(--accent-danger) 10%, transparent)',
        }"
        :title="lastError"
      >
        <!-- 错误文本：用 CSS truncate 替代 JS 截断魔法值 36。
             CJK 与拉丁字符宽度差异导致固定字符数截断宽度不均；
             max-w-[220px] + truncate 让浏览器按实际渲染宽度截断，
             完整错误信息通过父 div 的 :title tooltip 提供。 -->
        <span class="text-[10px] font-medium truncate max-w-[220px]" :style="{ color: `var(--accent-danger)` }">⚠ {{ lastError }}</span>
      </div>

      <!-- 运动安全故障现场：仅在 motionSafetyFailure 存在时显示。
           急停类（critical_deviation / limit_triggered）用红色高亮强调严重性，
           普通停止类（deviation / overshoot / no_progress）用橙色提示。
           现场信息（控制器/轴/目标/实际/偏差/点号）直接展示，避免操作员从 lastError 字符串解析。 -->
      <div
        v-if="motionSafetyFailure"
        class="rounded-md px-2.5 py-2 space-y-1"
        :style="{
          background: isMotionSafetyEmergency(motionSafetyFailure.verdict)
            ? 'color-mix(in srgb, var(--accent-danger) 14%, transparent)'
            : 'color-mix(in srgb, var(--state-warning) 12%, transparent)',
          border: `1px solid ${isMotionSafetyEmergency(motionSafetyFailure.verdict) ? 'var(--accent-danger)' : 'var(--state-warning)'}`,
        }"
      >
        <div class="flex items-center gap-1.5">
          <AlertTriangle
            class="h-3 w-3 flex-shrink-0"
            :style="{ color: isMotionSafetyEmergency(motionSafetyFailure.verdict) ? 'var(--accent-danger)' : 'var(--state-warning)' }"
          />
          <span
            class="text-[10px] font-bold uppercase tracking-wider"
            :style="{ color: isMotionSafetyEmergency(motionSafetyFailure.verdict) ? 'var(--accent-danger)' : 'var(--state-warning)' }"
          >
            {{ isMotionSafetyEmergency(motionSafetyFailure.verdict) ? labels.motionSafetyAlertEmergency : labels.motionSafetyAlert }}
            · {{ getMotionSafetyVerdictLabel(motionSafetyFailure.verdict, verdictLabels) }}
          </span>
        </div>
        <div class="grid grid-cols-2 gap-x-2 gap-y-0.5 text-[10px]">
          <div class="flex justify-between">
            <span class="text-[var(--text-muted)]">{{ labels.motionSafetyController }}</span>
            <span class="text-[var(--text-secondary)] truncate max-w-[100px]" :title="motionSafetyFailure.controllerId">
              {{ motionSafetyFailure.controllerId || '--' }}
            </span>
          </div>
          <div class="flex justify-between">
            <span class="text-[var(--text-muted)]">{{ labels.motionSafetyAxis }}</span>
            <span class="text-[var(--text-secondary)]">{{ motionSafetyFailure.axis || '--' }}</span>
          </div>
          <div class="flex justify-between">
            <span class="text-[var(--text-muted)]">{{ labels.motionSafetyTarget }}</span>
            <span class="text-[var(--text-secondary)]">{{ motionSafetyFailure.target.toFixed(3) }}</span>
          </div>
          <div class="flex justify-between">
            <span class="text-[var(--text-muted)]">{{ labels.motionSafetyActual }}</span>
            <span class="text-[var(--text-secondary)]">{{ motionSafetyFailure.actual.toFixed(3) }}</span>
          </div>
          <div class="flex justify-between">
            <span class="text-[var(--text-muted)]">{{ labels.motionSafetyDeviation }}</span>
            <span
              class="font-medium"
              :style="{ color: isMotionSafetyEmergency(motionSafetyFailure.verdict) ? 'var(--accent-danger)' : 'var(--state-warning)' }"
            >
              {{ (motionSafetyFailure.actual - motionSafetyFailure.target).toFixed(3) }}
            </span>
          </div>
          <div class="flex justify-between">
            <span class="text-[var(--text-muted)]">{{ labels.motionSafetyPointIndex }}</span>
            <span class="text-[var(--text-secondary)]">{{ motionSafetyFailure.pointIndex }}</span>
          </div>
        </div>
      </div>

      <!-- 硬件状态：明确标注“采集设备/位移机构”，避免两个灯看不出含义 -->
      <div class="rounded-md bg-[var(--bg-panel-strong)] px-2.5 py-1.5">
        <div class="mb-1 flex items-center gap-1.5">
          <Cpu class="h-3 w-3 text-[var(--text-muted)]" />
          <span class="text-[10px] text-[var(--text-muted)]">{{ labels.hardwareStatus }}</span>
        </div>
        <div class="flex items-center justify-between gap-2 text-[10px]">
          <div class="flex items-center gap-1.5">
            <span class="text-[var(--text-secondary)]">{{ labels.acquisitionDevice }}</span>
            <span class="h-1.5 w-1.5 rounded-full" :style="{ background: acquisitionConnection.dotColor, boxShadow: acquisitionConnection.dotGlow }"></span>
            <span :style="{ color: acquisitionConnection.textColor }">{{ acquisitionConnection.label }}</span>
          </div>
          <div class="flex items-center gap-1.5">
            <span class="text-[var(--text-secondary)]">{{ labels.positionerDevice }}</span>
            <span class="h-1.5 w-1.5 rounded-full" :style="{ background: positionerConnection.dotColor, boxShadow: positionerConnection.dotGlow }"></span>
            <span :style="{ color: positionerConnection.textColor }">{{ positionerConnection.label }}</span>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>
