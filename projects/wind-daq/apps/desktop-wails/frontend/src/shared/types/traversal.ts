/**
 * 五孔移位测试模块类型定义
 */

import type { ProbeChannelConfig, ProbeChannelRole } from './calibration'
export type { ProbeChannelConfig } from './calibration'

/** 布点模式类型 */
export type TraversalPattern = 'line' | 'rectangle' | 'sector' | 'custom'

/** 步长区段配置 */
export interface StepSegment {
  start: number
  end: number
  step: number
}

/** 遍历测试点坐标（4 轴：X/Y/Z/U，与后端 traversal.Point 对齐）。
 *  z/u 为必填：旧配置加载时由 applySavedLayout 显式补 0，避免 undefined 与后端 float64(0) 语义不一致。 */
export interface TraversalPoint {
  x: number
  y: number
  z: number
  u: number
}

export interface TraversalProbeChannelPreset {
  name: string
  role?: ProbeChannelRole
  defaultChannelIndex: number
  required: boolean
  enabledByDefault: boolean
}

export const TRAVERSAL_PROBE_CHANNEL_PRESETS: readonly TraversalProbeChannelPreset[] = [
  { name: 'P1', role: 'fiveHole.p1', defaultChannelIndex: 0, required: true, enabledByDefault: true },
  { name: 'P2', role: 'fiveHole.p2', defaultChannelIndex: 1, required: true, enabledByDefault: true },
  { name: 'P3', role: 'fiveHole.p3', defaultChannelIndex: 2, required: true, enabledByDefault: true },
  { name: 'P4', role: 'fiveHole.p4', defaultChannelIndex: 3, required: true, enabledByDefault: true },
  { name: 'P5', role: 'fiveHole.p5', defaultChannelIndex: 4, required: true, enabledByDefault: true },
  { name: 'Patm', role: 'fiveHole.pAtm', defaultChannelIndex: 16, required: true, enabledByDefault: true },
  { name: 'Tatm', role: 'fiveHole.tAtm', defaultChannelIndex: 17, required: true, enabledByDefault: true }
] as const

/**
 * 遍历测试测点压力值的默认小数位数。
 * 与校准模块 (calibrationPrecision.ts) 的 DEFAULT_CALIBRATION_PROBE_PRECISION 保持一致，
 * 此处独立定义以避免 types 文件依赖 API 层。
 */
export const TRAVERSAL_DEFAULT_PROBE_PRECISION = 3 as const

export function createDefaultTraversalProbeChannels(): ProbeChannelConfig[] {
  return TRAVERSAL_PROBE_CHANNEL_PRESETS.map((preset) => ({
    name: preset.name,
    role: preset.role,
    channel: { deviceId: '', channelIndex: preset.defaultChannelIndex },
    enabled: preset.enabledByDefault,
    // 为每个测点设置默认精度，用户可在硬件配置步骤中逐通道调整
    precision: TRAVERSAL_DEFAULT_PROBE_PRECISION
  }))
}

export function isTraversalRequiredProbeChannel(role?: ProbeChannelRole, name?: string): boolean {
  return TRAVERSAL_PROBE_CHANNEL_PRESETS.some((preset) => {
    if (role && preset.role === role) {
      return preset.required
    }

    return name !== undefined && preset.name === name && preset.required
  })
}

export function isTraversalConfigurableProbeChannel(role?: ProbeChannelRole, name?: string): boolean {
  return TRAVERSAL_PROBE_CHANNEL_PRESETS.some((preset) => {
    if (role && preset.role === role) {
      return true
    }

    return name !== undefined && preset.name === name
  })
}

/** 走线主轴：控制矩形/线型布局的物理走线方向 */
export type TraversalPrimaryAxis = 'x' | 'y'

/** 布局配置 */
export interface TraversalLayout {
  pattern: TraversalPattern
  snakeOrder?: boolean // 蛇形遍历：偶数行正常，奇数行反转，减少回程时间
  /**
   * 走线主轴：仅 line / rectangle 布局消费，扇形/自定义不消费。
   *   - 'x'：先沿 X 方向走完一条线再切换 Y
   *   - 'y'：先沿 Y 方向走完一条线再切换 X（原始行为）
   *   - undefined / 空字符串：保旧行为（先走 Y），用于升级前保存的 profile 兼容
   * 大小写不敏感、去空白。详见 gridPointsFromAxes。
   */
  primaryAxis?: TraversalPrimaryAxis

  line?: {
    startX: number
    startY: number
    endX: number
    endY: number
    xStepSegments: StepSegment[]
    yStepSegments: StepSegment[]
  }

  rectangle?: {
    xMin: number
    xMax: number
    xStepSegments: StepSegment[]
    yMin: number
    yMax: number
    yStepSegments: StepSegment[]
  }

  sector?: {
    centerX: number
    centerY: number
    radiusMin: number
    radiusMax: number
    radialStepSegments: StepSegment[]
    angleStart: number
    angleEnd: number
    angularStepSegments: StepSegment[]
  }

  custom?: {
    points: Array<{ x: number; y: number; z: number; u: number }>
  }
}

export function getTraversalStepValues(start: number, end: number, segments: StepSegment[] = []): number[] {
  // finite 校验：start/end 为 undefined/NaN 时直接返回空数组。
  // 若不拦截，Math.max(seg.start, undefined)=NaN 会让循环不执行，
  // 随后 fallback 分支 push(undefined)，最终导致 PointsPreview bounds 塌缩成 NaN、画布空白。
  if (!Number.isFinite(start) || !Number.isFinite(end)) {
    return []
  }

  const values: number[] = []
  const sortedSegments = [...segments].sort((a, b) => a.start - b.start)

  for (const segment of sortedSegments) {
    const { start: segmentStart, end: segmentEnd, step } = segment
    if (step <= 0) {
      continue
    }

    const actualStart = Math.max(segmentStart, start)
    const actualEnd = Math.min(segmentEnd, end)

    // 使用 1e-9 容差避免浮点数精度问题导致末端点被遗漏
    // 必须与后端 path.go 的 StepValues 函数保持一致
    for (let value = actualStart; value <= actualEnd + 1e-9; value += step) {
      if (!values.includes(value)) {
        values.push(value)
      }
    }
  }

  if (values.length === 0 && start !== end) {
    values.push(start, end)
  } else if (values.length === 0) {
    values.push(start)
  }

  return values.sort((a, b) => a - b)
}

/**
 * 从步进区段派生范围 [min, max]。
 *
 * 用于 rectangle/sector 布局的 xMin/xMax、yMin/yMax、radiusMin/Max、angleStart/End 兜底：
 * 当持久化 profile 缺这些字段（旧版本）或 TraversalLayoutStep 副作用未执行时，
 * 从 segments 的 start/end 实时派生，避免 undefined 传播到 getTraversalStepValues 导致画布塌缩。
 *
 * 空 segments 时返回 { min: 0, max: 0 }，与 TraversalLayoutStep 的 computedRectangleRange 旧逻辑一致。
 */
export function deriveRangeFromSegments(segments: StepSegment[] = []): { min: number; max: number } {
  if (segments.length === 0) {
    return { min: 0, max: 0 }
  }
  return {
    min: Math.min(...segments.map((s) => s.start)),
    max: Math.max(...segments.map((s) => s.end))
  }
}

/** 矩形布局的范围由分段输入唯一决定，避免旧范围裁剪新分段后退化成单行或单列。 */
export function normalizeTraversalLayoutRanges(layout: TraversalLayout): TraversalLayout {
  if (layout.pattern !== 'rectangle' || !layout.rectangle) {
    return layout
  }

  const normalizeAxis = (segments: StepSegment[]): StepSegment[] => {
    const range = deriveRangeFromSegments(segments)
    if (range.max > range.min) return segments

    const step = segments.find((segment) => segment.step > 0)?.step ?? 5
    return [{ start: -30, end: 30, step }]
  }

  const xStepSegments = normalizeAxis(layout.rectangle.xStepSegments)
  const yStepSegments = normalizeAxis(layout.rectangle.yStepSegments)
  const xRange = deriveRangeFromSegments(xStepSegments)
  const yRange = deriveRangeFromSegments(yStepSegments)
  return {
    ...layout,
    rectangle: {
      ...layout.rectangle,
      xMin: xRange.min,
      xMax: xRange.max,
      xStepSegments,
      yMin: yRange.min,
      yMax: yRange.max,
      yStepSegments
    }
  }
}

/**
 * 网格布点（支持蛇形遍历与走线主轴选择）
 *
 * primaryAxis 取值（归一化：大小写不敏感、去空白）：
 *   - 'x'：外层 Y、内层 X，每条线沿 X 方向走，切换时跳到下一行 Y
 *   - 'y'：外层 X、内层 Y，每条线沿 Y 方向走（原始行为）
 *   - undefined / 空字符串 / 其他未识别值：保旧行为（先走 Y），用于升级前保存的 profile 兼容
 *
 * 蛇形反转方向跟随主轴：主轴是“长程走线方向”，反转主轴可避免回程空跑。
 * 必须与后端 path.go 的 GridPointsFromAxesOrdered / GridPointsFromAxesSnakeOrdered 保持一致。
 *
 * 实现上 'x' 分支复用 gridPointsFromAxesY（交换 xs/ys）后转置坐标，避免重复双重循环逻辑，
 * 与后端 swapPoints 模式对齐。
 */
function gridPointsFromAxes(
  xs: number[],
  ys: number[],
  snakeOrder = false,
  primaryAxis: TraversalPrimaryAxis | string = 'y'
): TraversalPoint[] {
  // 归一化：去空白 + 转小写。仅显式 'x' 才走新逻辑，其他（含 undefined/空/'y'）保旧行为
  const normalized = (primaryAxis ?? '').trim().toLowerCase()
  if (normalized !== 'x') {
    return gridPointsFromAxesY(xs, ys, snakeOrder)
  }
  // 先走 X：交换两轴调用 legacy（外层 Y、内层 X），再转置坐标恢复 (X,Y) 语义
  return swapPoints(gridPointsFromAxesY(ys, xs, snakeOrder))
}

/**
 * gridPointsFromAxesY 是 legacy 实现：外层 X、内层 Y，蛇形反转 Y。
 * 抽成独立函数供 'x' 分支转置复用，避免双重循环逻辑散落两处。
 */
function gridPointsFromAxesY(xs: number[], ys: number[], snakeOrder: boolean): TraversalPoint[] {
  const points: TraversalPoint[] = []
  for (let i = 0; i < xs.length; i++) {
    if (snakeOrder && i % 2 === 1) {
      for (let j = ys.length - 1; j >= 0; j--) {
        // 网格布点不消费 Z/U：固定补 0，与后端 path.go GridPointsFromAxesOrdered 对齐
        points.push({ x: xs[i], y: ys[j], z: 0, u: 0 })
      }
    } else {
      for (const y of ys) {
        points.push({ x: xs[i], y, z: 0, u: 0 })
      }
    }
  }
  return points
}

/** swapPoints 原地转置点集的 X/Y 坐标，与后端 swapPoints 模式对齐。 */
function swapPoints(points: TraversalPoint[]): TraversalPoint[] {
  for (let i = 0; i < points.length; i++) {
    const tmp = points[i].x
    points[i].x = points[i].y
    points[i].y = tmp
  }
  return points
}

/** 扇形布点（支持蛇形遍历） */
function sectorPointsFromRadiiAngles(
  centerX: number,
  centerY: number,
  radii: number[],
  angles: number[],
  snakeOrder = false
): TraversalPoint[] {
  const points: TraversalPoint[] = []
  for (let i = 0; i < radii.length; i++) {
    if (snakeOrder && i % 2 === 1) {
      for (let j = angles.length - 1; j >= 0; j--) {
        const radian = (angles[j] * Math.PI) / 180
        // 扇形布点不消费 Z/U：固定补 0，与后端 path.go SectorPointsFromRadiiAngles 对齐
        points.push({
          x: centerX + radii[i] * Math.cos(radian),
          y: centerY + radii[i] * Math.sin(radian),
          z: 0,
          u: 0
        })
      }
    } else {
      for (const angle of angles) {
        const radian = (angle * Math.PI) / 180
        points.push({
          x: centerX + radii[i] * Math.cos(radian),
          y: centerY + radii[i] * Math.sin(radian),
          z: 0,
          u: 0
        })
      }
    }
  }
  return points
}

export function getTraversalLayoutPoints(layout?: TraversalLayout): TraversalPoint[] {
  if (!layout) {
    return []
  }

  const snake = layout.snakeOrder ?? false
  // 走线主轴：undefined / 空字符串走 legacy（先走 Y），保旧行为兼容升级前 profile。
  // 显式 'x' 才走新逻辑（先走 X）；'y' 等价 legacy。gridPointsFromAxes 内部已归一化。
  const primaryAxis = layout.primaryAxis

  switch (layout.pattern) {
    case 'line': {
      if (!layout.line) {
        return []
      }

      const { startX, startY, endX, endY, xStepSegments, yStepSegments } = layout.line
      const xSteps = getTraversalStepValues(startX, endX, xStepSegments)
      const ySteps = getTraversalStepValues(startY, endY, yStepSegments)

      if (xSteps.length === 0 && ySteps.length === 0) {
        // line 布局退化场景：仅起止点重合，Z/U 补 0
        return [{ x: startX, y: startY, z: 0, u: 0 }]
      }
      if (ySteps.length === 0) {
        return xSteps.map((x) => ({ x, y: startY, z: 0, u: 0 }))
      }
      if (xSteps.length === 0) {
        return ySteps.map((y) => ({ x: startX, y, z: 0, u: 0 }))
      }
      return gridPointsFromAxes(xSteps, ySteps, snake, primaryAxis)
    }

    case 'rectangle': {
      if (!layout.rectangle) {
        return []
      }

      const rectangle = normalizeTraversalLayoutRanges(layout).rectangle!
      const { xStepSegments, yStepSegments } = rectangle
      // 矩形编辑器只暴露分段输入，因此分段是实际布点范围的单一数据源。
      // 不能优先使用持久化的 xMin/xMax/yMin/yMax，否则编辑分段后旧范围会裁剪新点集。
      const xRange = deriveRangeFromSegments(xStepSegments)
      const yRange = deriveRangeFromSegments(yStepSegments)
      const xSteps = getTraversalStepValues(xRange.min, xRange.max, xStepSegments)
      const ySteps = getTraversalStepValues(yRange.min, yRange.max, yStepSegments)
      return gridPointsFromAxes(xSteps, ySteps, snake, primaryAxis)
    }

    case 'sector': {
      if (!layout.sector) {
        return []
      }

      const {
        centerX,
        centerY,
        radiusMin,
        radiusMax,
        radialStepSegments,
        angleStart,
        angleEnd,
        angularStepSegments
      } = layout.sector
      const radii = getTraversalStepValues(radiusMin, radiusMax, radialStepSegments)
      const angles = getTraversalStepValues(angleStart, angleEnd, angularStepSegments)
      // 扇形布局不消费 primaryAxis，保持原“外层半径、内层角度”语义
      return sectorPointsFromRadiiAngles(centerX, centerY, radii, angles, snake)
    }

    case 'custom': {
      return layout.custom?.points ? [...layout.custom.points] : []
    }
  }

  return []
}

export function getTraversalLayoutPointCount(layout?: TraversalLayout): number {
  return getTraversalLayoutPoints(layout).length
}

/** 运动轴配置（X/Y 到α/β映射） */
export interface TraversalMotionAxisConfig {
  name: 'X' | 'Y'
  controllerId: string
  axis: 'X' | 'Y' | 'Z' | 'U'
  angleMapping?: {
    type: 'alpha' | 'beta'
    offset?: number
    scale?: number
  }
}

/** PRB 文件信息 */
export interface PrbFileInfo {
  filePath: string
  fileName: string
  loadedAt: number
  validRange: {
    alphaMin: number
    alphaMax: number
    betaMin: number
    betaMax: number
    machMin: number
    machMax: number
  }
  machNumber?: number // 从文件名解析或用户指定的马赫数
}

/** 多PRB插值模式 */
export type MultiPrbInterpolationMode = 'nearest' | 'linear'

/** 多PRB文件配置 */
export interface MultiPrbConfig {
  files: PrbFileInfo[]
  machNumbers: number[]
  interpolationMode: MultiPrbInterpolationMode
}

export interface TraversalRawPressure {
  P1: number
  P2: number
  P3: number
  P4: number
  P5: number
  Patm: number
  Tatm: number
  P0?: number
  Ps?: number
}

export type TraversalInterpolationInput = Pick<
  TraversalRawPressure,
  'P1' | 'P2' | 'P3' | 'P4' | 'P5' | 'Patm' | 'Tatm'
>

/** 插值计算结果 */
export interface InterpolationResult {
  alpha: number
  beta: number
  machNumber: number
  velocity: number
  dynamicPressure: number
  density: number
  P0?: number
  Ps?: number
  isValid: boolean
  warning?: string
}

/** 插值算法类型 */
export type InterpolationAlgorithm = 'old' | 'new'

/** CSV 校准数据文件信息（新算法使用） */
export interface CalibrationCsvFileInfo {
  filePath: string
  fileName: string
  loadedAt: number
  validRange: {
    alphaMin: number
    alphaMax: number
    betaMin: number
    betaMax: number
  }
  pointCount: number
}

export interface TraversalSaveOptions {
  savePointId: boolean
  saveTimestamp: boolean
  saveRawPressure: boolean
  saveCalculatedResult: boolean
  customFields?: Record<string, boolean>
}

/** 测试配置 */
export interface TraversalTestConfig {
  name: string
  layout: TraversalLayout
  channels: {
    probeChannels: ProbeChannelConfig[]
    motionAxes: TraversalMotionAxisConfig[]
  }
  prbFile: PrbFileInfo | null // 单PRB模式(向后兼容)
  multiPrb?: MultiPrbConfig // 多PRB模式
  useMultiPrb?: boolean // 是否使用多PRB模式
  interpolationAlgorithm?: InterpolationAlgorithm // 插值算法选择，默认 'old'
  calibrationCsvFile?: CalibrationCsvFileInfo | null // 新算法的CSV校准数据文件
  /** 五孔探针压力类型：'gauge' 表压(默认) | 'absolute' 绝压 */
  pProbePressureType?: 'gauge' | 'absolute'
  dwellTimeMs: number
  samplesPerPoint: number
  savePath: string
  saveFileName: string
  saveOptions: TraversalSaveOptions
  /** 数据验证配置（可选，与 Cursor DAQ 行为一致） */
  validation?: DataValidationConfig
  /** 稳定等待配置（可选，与 Cursor DAQ 行为一致） */
  stabilization?: StabilizationConfig
}

/** 测试数据点 */
export interface TraversalDataPoint {
  pointId: number
  coordinates: { alpha: number; beta: number }
  rawPressure: TraversalRawPressure
  interpolationResult: InterpolationResult
  sampleCount: number
  timestamp: number
  dwellTimeElapsed: number
}

/** 测试状态 */
export type TraversalTerminalStatus = 'completed' | 'error' | 'stopped'

export type TraversalTestStatusType = 'idle' | 'running' | 'paused' | TraversalTerminalStatus

/** 当前点的处理阶段 */
export type TraversalPointPhase = 'moving' | 'stabilizing' | 'acquiring' | 'saving'

export interface TraversalTestStatus {
  taskId: string
  state: string // 后端原始状态（idle/preparing/moving/stabilizing/acquiring/saving/running/paused/stopped/error/completed）
  status: TraversalTestStatusType
  totalPoints: number
  completedPoints: number
  currentPoint?: { alpha: number; beta: number }
  currentPointPhase?: TraversalPointPhase
  latestData?: TraversalDataPoint
  dataPoints?: TraversalDataPoint[]
  progress: number
  startTime?: number
  estimatedRemaining?: number
  lastError?: string
  lastErrorCode?: TraversalErrorCode
  validationWarnings?: string[]
}

export interface TraversalModuleResult {
  taskId: string
  status: TraversalTestStatusType
  completedPoints: number
  totalPoints: number
}

/** 前置条件检查结果 */
export interface StartPreconditions {
  motionControllerConnected: boolean
  prbFileLoaded: boolean
  acquisitionDevicesReady: boolean
}

export interface PreconditionCheckResult {
  allPassed: boolean
  checks: Array<{
    name: string
    passed: boolean
    message?: string
  }>
}

/** 测试进度事件 */
export interface TraversalProgressEvent {
  taskId: string
  completedPoints: number
  totalPoints: number
  currentPoint: { alpha: number; beta: number }
  currentPointPhase?: TraversalPointPhase
  latestData?: TraversalDataPoint
  timestamp: number
}

/** 测试完成事件 */
export interface TraversalCompleteEvent {
  taskId: string
  success: boolean
  status: TraversalTerminalStatus
  filePath?: string
  error?: string
  duration: number
  totalPoints: number
}

/** 测试错误事件 */
export type TraversalErrorCode =
  | 'MOTION_FAILED'
  | 'ACQUISITION_FAILED'
  | 'SAVE_FAILED'
  | 'INTERPOLATION_FAILED'
  | 'UNKNOWN'

export interface TraversalErrorEvent {
  taskId: string
  error: string
  code: TraversalErrorCode
  recoverable: boolean
}

/** 数据验证配置 */
export interface DataValidationConfig {
  enabled: boolean
  pressureRange?: Record<string, { min: number; max: number }>
  spikeDetection?: {
    enabled: boolean
    threshold: number // 百分比阈值
  }
  onInvalid: 'skip' | 'retry' | 'continue'
  retryCount?: number
}

/** 稳定等待配置 */
export interface StabilizationConfig {
  mode: 'fixed' | 'adaptive'
  fixedTimeMs?: number
  adaptive?: {
    maxWaitMs: number
    minWaitMs: number
    stabilityThreshold: number // 百分比变化阈值
    checkIntervalMs: number
    consecutiveChecks: number // 连续稳定检查次数
  }
}

/** 断点恢复信息 */
export interface TraversalCheckpoint {
  taskId: string
  /** 完整测试配置（用于恢复时重建测试上下文，与 Cursor DAQ 行为一致） */
  config?: TraversalTestConfig
  completedPoints: number
  totalPoints: number
  lastPoint?: TraversalPoint
  savePath: string
  createdAt: number
}
