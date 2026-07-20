/**
 * 五孔移位测试模块类型定义
 */

import type { ProbeChannelConfig, ProbeChannelRole } from './calibration'
import type { AxisKind, AxisName } from './motion'
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

/**
 * 七孔探针 9 通道预设（spec-seven-hole-traversal §2.3）：
 * 外围 6 孔 P1~P6（CH1~CH6）+ 中心孔 P7（CH7）+ 大气压力（CH17）+ 大气温度（CH18）。
 * 角色与 spec-seven-hole-calibration §9.4 共用同一 sevenHole.* 命名空间。
 */
export const SEVEN_HOLE_TRAVERSAL_PROBE_CHANNEL_PRESETS: readonly TraversalProbeChannelPreset[] = [
  { name: 'P1', role: 'sevenHole.p1', defaultChannelIndex: 0, required: true, enabledByDefault: true },
  { name: 'P2', role: 'sevenHole.p2', defaultChannelIndex: 1, required: true, enabledByDefault: true },
  { name: 'P3', role: 'sevenHole.p3', defaultChannelIndex: 2, required: true, enabledByDefault: true },
  { name: 'P4', role: 'sevenHole.p4', defaultChannelIndex: 3, required: true, enabledByDefault: true },
  { name: 'P5', role: 'sevenHole.p5', defaultChannelIndex: 4, required: true, enabledByDefault: true },
  { name: 'P6', role: 'sevenHole.p6', defaultChannelIndex: 5, required: true, enabledByDefault: true },
  { name: 'P7', role: 'sevenHole.p7', defaultChannelIndex: 6, required: true, enabledByDefault: true },
  { name: 'Patm', role: 'sevenHole.pAtm', defaultChannelIndex: 16, required: true, enabledByDefault: true },
  { name: 'Tatm', role: 'sevenHole.tAtm', defaultChannelIndex: 17, required: true, enabledByDefault: true }
] as const

/** 七孔遍历默认通道（与 createDefaultTraversalProbeChannels 同构） */
export function createSevenHoleTraversalProbeChannels(): ProbeChannelConfig[] {
  return SEVEN_HOLE_TRAVERSAL_PROBE_CHANNEL_PRESETS.map((preset) => ({
    name: preset.name,
    role: preset.role,
    channel: { deviceId: '', channelIndex: preset.defaultChannelIndex },
    enabled: preset.enabledByDefault,
    precision: TRAVERSAL_DEFAULT_PROBE_PRECISION
  }))
}

/**
 * 通道绑定的唯一键：设备 + 硬件通道号。
 * 不同设备的通道号允许重复（各设备 profile 均从 0 编号），
 * 只有「同一设备同一通道」被多个探针绑定才是冲突。
 */
export function channelBindingKey(ch: ProbeChannelConfig): string {
  return `${ch.channel.deviceId} ${ch.channel.channelIndex}`
}

/**
 * 检测启用通道中重复的物理通道绑定（同一设备同一通道号），返回冲突绑定键集合。
 *
 * 仅检测 enabled 通道：未启用通道不参与后端采样，重复无影响。
 * channelIndex < 0 视为未分配，跳过检测。
 *
 * 与后端 traversal_config.go 的 ParseConfig 重复检测策略对齐：
 * 后端按「设备+通道号」判定重复并直接返回错误不启动测试，因此前端需在保存前阻断。
 * 跨设备绑定（如五孔在设备 A、大气压/温度在设备 B）通道号相同不算冲突。
 * 此函数作为前后端共享真相源，避免 TraversalHardwareStep.vue 的视觉提示
 * 与 TraversalSettings.vue 的 isStepValid 阻断逻辑使用两份独立实现。
 */
export function findDuplicateChannelBindings(channels: ProbeChannelConfig[]): Set<string> {
  const counts = new Map<string, number>()
  for (const ch of channels) {
    if (!ch.enabled || ch.channel.channelIndex == null || ch.channel.channelIndex < 0) continue
    const key = channelBindingKey(ch)
    counts.set(key, (counts.get(key) ?? 0) + 1)
  }
  const dupes = new Set<string>()
  for (const [key, count] of counts) {
    if (count > 1) dupes.add(key)
  }
  return dupes
}

/** 是否存在重复通道绑定——findDuplicateChannelBindings 的布尔快捷形式 */
export function hasDuplicateChannel(channels: ProbeChannelConfig[]): boolean {
  return findDuplicateChannelBindings(channels).size > 0
}

/** 遍历通道预设全集（五孔 7 项 + 七孔 9 项），供 required/configurable 判定共用 */
const ALL_TRAVERSAL_PROBE_CHANNEL_PRESETS: readonly TraversalProbeChannelPreset[] = [
  ...TRAVERSAL_PROBE_CHANNEL_PRESETS,
  ...SEVEN_HOLE_TRAVERSAL_PROBE_CHANNEL_PRESETS
]

export function isTraversalRequiredProbeChannel(role?: ProbeChannelRole, name?: string): boolean {
  return ALL_TRAVERSAL_PROBE_CHANNEL_PRESETS.some((preset) => {
    if (role && preset.role === role) {
      return preset.required
    }

    return name !== undefined && preset.name === name && preset.required
  })
}

export function isTraversalConfigurableProbeChannel(role?: ProbeChannelRole, name?: string): boolean {
  return ALL_TRAVERSAL_PROBE_CHANNEL_PRESETS.some((preset) => {
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
        // 扇形布点不消费 Z/U：固定补 0。仅前端预览用；后端扇形生产路径已改走 GridPointsFromAxes 相对坐标
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

/** 遍历目标方向到物理运动轴的绑定。
 *
 * name 为逻辑目标名（'X'/'Y'/'Z'/'U'）——对应遍历点 Point 的 X/Y/Z/U 字段。
 * custom 模式允许绑定全部 4 个逻辑目标；line/rectangle/sector 模式仅消费 X/Y
 * （后端 path.go 把 Z/U 标记为 NaN，availableAxisTargets 自动跳过）。
 * axis 为该逻辑目标对应的物理轴名（控制器 profile 中的轴名）。 */
export interface TraversalMotionAxisConfig {
  name: 'X' | 'Y' | 'Z' | 'U'
  controllerId: string
  axis: 'X' | 'Y' | 'Z' | 'U'
  /** @deprecated 仅用于读取旧 profile；攻角和侧滑角由测点压力插值计算。 */
  angleMapping?: {
    type: 'alpha' | 'beta'
    offset?: number
    scale?: number
  }
}

/**
 * 扇形布点对运动轴类型的约束：
 * - X 方向驱动径向（半径）运动 → 必须绑定直线轴（平移台）
 * - Y 方向驱动角度运动 → 必须绑定旋转轴（旋转台）
 * 其余布局（line/rectangle/custom）所有方向都是笛卡尔直线运动，无类型约束，返回 null。
 *
 * target 取 'Z'/'U' 时始终返回 null：扇形布局只消费 X/Y 两个逻辑目标，
 * Z/U 行（custom 模式下存在的绑定）不参与扇形轴类型校验。
 *
 * 与 findTraversalAxisKindIssues / filterTraversalAxisOptions 一起作为
 * TraversalHardwareStep.vue 的选项过滤 + 视觉提示和 TraversalSettings.vue
 * isStepValid 阻断逻辑的共享真相源（与 findDuplicateChannelBindings 同模式）。
 */
export function requiredTraversalAxisKind(pattern: TraversalPattern, target: 'X' | 'Y' | 'Z' | 'U'): AxisKind | null {
  if (pattern !== 'sector') return null
  // 扇形仅约束 X/Y；Z/U 不参与扇形布点（path.go 把扇形点的 Z/U 标记为 NaN）
  if (target !== 'X' && target !== 'Y') return null
  return target === 'X' ? 'LINEAR' : 'ROTARY'
}

/** 轴类型校验所需的控制器 profile 最小结构（MotionControllerProfile 满足此接口）。 */
export interface TraversalAxisKindProfileLike {
  id: string
  axes: ReadonlyArray<{ name: AxisName; kind: AxisKind; enabled?: boolean }>
}

/** 单条不满足布点模式轴类型要求的绑定。actualKind 为 null 表示 profile 中找不到该轴配置。 */
export interface TraversalAxisKindIssue {
  name: 'X' | 'Y' | 'Z' | 'U'
  axis: AxisName
  requiredKind: AxisKind
  actualKind: AxisKind | null
  /** 轴种类匹配但被停用（enabled === false）时置 true；停用轴同样不合规。 */
  disabled?: boolean
}

/**
 * 找出不满足布点模式轴类型要求的运动轴绑定。
 * 仅在 controllerId 已选且 profile 存在时判定；控制器未绑定或 profile 缺失由既有校验
 * （isStepValid 的 controllerId !== ''）兜底，此处跳过避免重复报错。
 * 已停用（enabled === false）的轴视为不合规：filterTraversalAxisOptions 会把停用轴
 * 排除在可选项外，若校验放行则出现"选不了的轴却能通过校验"的矛盾。
 *
 * Z/U 行（custom 模式下存在的绑定）由 requiredTraversalAxisKind 返回 null 跳过，
 * 不参与扇形轴类型校验——扇形布局只消费 X/Y 两个逻辑目标。
 */
export function findTraversalAxisKindIssues(
  motionAxes: TraversalMotionAxisConfig[],
  profiles: ReadonlyArray<TraversalAxisKindProfileLike>,
  pattern: TraversalPattern
): TraversalAxisKindIssue[] {
  const issues: TraversalAxisKindIssue[] = []
  for (const ma of motionAxes) {
    const requiredKind = requiredTraversalAxisKind(pattern, ma.name)
    if (!requiredKind || !ma.controllerId) continue
    const profile = profiles.find((p) => p.id === ma.controllerId)
    if (!profile) continue
    const axisCfg = profile.axes.find((a) => a.name === ma.axis)
    const actualKind = axisCfg?.kind ?? null
    const disabled = axisCfg?.enabled === false
    if (actualKind !== requiredKind || disabled) {
      issues.push({ name: ma.name, axis: ma.axis, requiredKind, actualKind, ...(disabled ? { disabled: true } : {}) })
    }
  }
  return issues
}

/**
 * 指定控制器下满足布点模式轴类型要求的可选物理轴列表。
 * 返回 null 表示不过滤：非扇形布局本就不限制；控制器未选择 / profile 缺失时
 * 无法判定类型，保持显示全部轴，类型不符由 findTraversalAxisKindIssues 视觉提示兜底。
 * 已停用（enabled === false）的轴不作为可选项。
 *
 * target 为 'Z'/'U' 时返回 null：扇形不约束 Z/U，沿 axisOptionsFor 旧路径显示全部 4 轴。
 */
export function filterTraversalAxisOptions(
  profile: TraversalAxisKindProfileLike | null | undefined,
  pattern: TraversalPattern,
  target: 'X' | 'Y' | 'Z' | 'U'
): AxisName[] | null {
  const requiredKind = requiredTraversalAxisKind(pattern, target)
  if (!requiredKind || !profile) return null
  return profile.axes
    .filter((a) => a.enabled !== false && a.kind === requiredKind)
    .map((a) => a.name)
}

/**
 * 运动轴绑定唯一键：控制器 ID + 物理轴名。
 *
 * 同一控制器内一根物理轴只能被一个方向（X/Y/Z/U）绑定——否则两个方向都会向
 * 同一根物理轴下发 MoveTo，最终只剩最后一个方向的目标生效，另一个方向静默丢失。
 * 跨控制器的同名物理轴互不影响（各控制器独立编号），不算冲突。
 *
 * 与 channelBindingKey 同模式：用于检测重复 + 视觉高亮，键格式稳定可序列化。
 */
export function motionAxisBindingKey(ax: TraversalMotionAxisConfig): string {
  return `${ax.controllerId} ${ax.axis}`
}

/**
 * 检测已绑定控制器的运动轴中"同控制器同一物理轴"被多个方向绑定的冲突。
 *
 * 仅检测 controllerId 非空的绑定：
 *   - controllerId 为空表示尚未绑定控制器，没有"同控制器"概念，跳过；
 *   - axis 为空理论上不会发生（UI 强制选 4 轴之一），同样跳过。
 *
 * 跨设备绑定（如 X→控制器A.X、Y→控制器B.X）不算冲突——各控制器物理轴独立编号。
 *
 * 与 findDuplicateChannelBindings 同模式：返回冲突绑定键集合，供
 * TraversalLayoutStep.vue 的视觉高亮与 TraversalSettings.vue isStepValid 阻断共用，
 * 避免两份独立判定实现。
 */
export function findDuplicateMotionAxisBindings(motionAxes: TraversalMotionAxisConfig[]): Set<string> {
  const counts = new Map<string, number>()
  for (const ax of motionAxes) {
    if (!ax.controllerId || !ax.axis) continue
    const key = motionAxisBindingKey(ax)
    counts.set(key, (counts.get(key) ?? 0) + 1)
  }
  const dupes = new Set<string>()
  for (const [key, count] of counts) {
    if (count > 1) dupes.add(key)
  }
  return dupes
}

/** 是否存在重复运动轴绑定——findDuplicateMotionAxisBindings 的布尔快捷形式 */
export function hasDuplicateMotionAxis(motionAxes: TraversalMotionAxisConfig[]): boolean {
  return findDuplicateMotionAxisBindings(motionAxes).size > 0
}

/**
 * 查询"当前方向行之外、同一控制器上已绑定物理轴"的占用映射：物理轴名 → 占用它的方向名列表。
 *
 * 与 findDuplicateMotionAxisBindings 的事后告警互补：重复检测只能在用户选完后红框 + 横幅
 * 拦截，本函数在选项渲染前暴露占用关系，让 TraversalLayoutStep 能在下拉选项 label 上追加
 * "已被 X 方向占用"后缀——用户点开下拉即可预见冲突，而不是选完才被拦截。
 *
 * 判定规则与重复检测保持一致：
 *   - 仅统计 controllerId 与 axis 均已设置的行（未绑定行没有"占用"概念）；
 *   - 仅同控制器内比较（跨控制器同名物理轴独立编号，互不占用）；
 *   - 排除当前行自身（按 name 即方向名识别）——自己绑定的轴不是"被他人占用"，
 *     否则当前选中项会自我标注，误导用户以为与别人冲突。
 *
 * currentRow.controllerId 为空时结果恒为空：当前行尚未选定控制器，无所谓"同控制器占用"。
 * 返回值不含未被占用的轴；同一物理轴被多个其他方向占用时按行顺序全部列出
 * （custom 模式交换两行绑定的中间态会出现，此时两行都被后缀预告，交换即可完成）。
 */
export function findOccupiedMotionAxisDirections(
  motionAxes: TraversalMotionAxisConfig[],
  currentRow: TraversalMotionAxisConfig
): Map<string, Array<'X' | 'Y' | 'Z' | 'U'>> {
  const occupied = new Map<string, Array<'X' | 'Y' | 'Z' | 'U'>>()
  for (const ax of motionAxes) {
    if (ax.name === currentRow.name) continue
    if (!ax.controllerId || !ax.axis) continue
    if (ax.controllerId !== currentRow.controllerId) continue
    const list = occupied.get(ax.axis)
    if (list) list.push(ax.name)
    else occupied.set(ax.axis, [ax.name])
  }
  return occupied
}

/**
 * 布点模式实际参与遍历运动的逻辑轴名列表。
 * - line：仅沿 X 轴布点，Y/Z/U 被后端 path.go markAxesNaN → ['X']
 * - rectangle/sector：仅沿 X/Y 平面布点，Z/U 被 markAxesNaN → ['X', 'Y']
 * - custom：4 轴全部参与（用户填的 Z/U 坐标必须下发）→ ['X', 'Y', 'Z', 'U']
 *
 * 与后端 path.go markAxesNaN 的消费矩阵严格对齐，作为配置屏（TraversalLayoutStep）
 * 与运行屏（TraversalLiveMonitor / useHardwareConnectionStatus）显示轴数的共享真相源。
 */
export function getTraversalDisplayedAxisNames(pattern: TraversalPattern): Array<'X' | 'Y' | 'Z' | 'U'> {
  if (pattern === 'line') return ['X']
  if (pattern === 'custom') return ['X', 'Y', 'Z', 'U']
  return ['X', 'Y']
}

/**
 * 按布点模式过滤"实际参与遍历运动"的运动轴绑定（getTraversalDisplayedAxisNames 的绑定级形式）。
 * 过滤规则与轴名列表一致：line 仅 X，rectangle/sector 为 X/Y，custom 全部 4 轴。
 */
export function getTraversalDisplayedAxes(
  pattern: TraversalPattern,
  motionAxes: TraversalMotionAxisConfig[]
): TraversalMotionAxisConfig[] {
  const names = getTraversalDisplayedAxisNames(pattern)
  return motionAxes.filter((ax) => names.includes(ax.name))
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

// =====================================================================
// 七孔探针：类型、判别配置、展示元数据（spec-seven-hole-traversal §2.3/§6.5）
// =====================================================================

/** 探针类型判别字段：'five-hole'（默认，旧配置缺省等价）/ 'seven-hole' */
export type TraversalProbeType = 'five-hole' | 'seven-hole'

/**
 * 七孔 PRB 文件信息（importSevenHolePrb 响应逐文件项 + 持久化）。
 * 持久化时后端仅消费 filePath；其余字段为前端展示保留。
 */
export interface SevenHolePrbFileInfo {
  filePath: string
  fileName?: string
  /** 7=内区（7.prb），1..6=扇区（n.prb） */
  sector?: number
  /** 169（内区）/ 52（扇区） */
  pointCount?: number
  loadedAt?: number
}

/** 七孔批量导入的文件名→槽位分配结果 */
export interface SevenHoleFileAssignment {
  innerFile: SevenHolePrbFileInfo | null
  outerFiles: Map<number, SevenHolePrbFileInfo>
  /** 无法按规范命名分配的文件（由用户逐槽手动选择） */
  unmatched: string[]
}

/**
 * 按文件名把批量选择的 .prb 分配到七孔槽位：
 * 7.prb → 内区；1.prb~6.prb → 扇区 1..6（大小写不敏感，仅认规范命名）。
 * 同一槽位重复命中时后来者覆盖；不匹配的文件列入 unmatched。
 */
export function assignSevenHoleFilesByName(paths: string[]): SevenHoleFileAssignment {
  const outerFiles = new Map<number, SevenHolePrbFileInfo>()
  const unmatched: string[] = []
  let innerFile: SevenHolePrbFileInfo | null = null
  for (const path of paths) {
    const fileName = path.split(/[\\/]/).pop() ?? path
    const m = /^(\d+)\.prb$/i.exec(fileName)
    if (m) {
      const n = Number(m[1])
      if (n === 7) {
        innerFile = { filePath: path, fileName, sector: 7 }
        continue
      }
      if (n >= 1 && n <= 6) {
        outerFiles.set(n, { filePath: path, fileName, sector: n })
        continue
      }
    }
    unmatched.push(path)
  }
  return { innerFile, outerFiles, unmatched }
}

/** 七孔批量导入格式探测：全 .prb / 全 .csv / 混合 / 空 */
export function detectSevenHoleBatchFormat(paths: string[]): 'prb' | 'calibration-csv' | 'mixed' | 'empty' {
  if (paths.length === 0) return 'empty'
  const allPrb = paths.every((p) => /\.prb$/i.test(p))
  if (allPrb) return 'prb'
  const allCsv = paths.every((p) => /\.csv$/i.test(p))
  if (allCsv) return 'calibration-csv'
  return 'mixed'
}

/**
 * 按文件名把批量选择的七孔校准 CSV 分配到槽位：
 * 含「小角度区」→ 内区；含「大角度N区」（N=1..6）→ 扇区 N（校准导出规范命名）。
 * 不匹配的文件列入 unmatched。
 */
export function assignSevenHoleCsvFilesByName(paths: string[]): SevenHoleFileAssignment {
  const outerFiles = new Map<number, SevenHolePrbFileInfo>()
  const unmatched: string[] = []
  let innerFile: SevenHolePrbFileInfo | null = null
  for (const path of paths) {
    const fileName = path.split(/[\\/]/).pop() ?? path
    if (fileName.includes('小角度区')) {
      innerFile = { filePath: path, fileName, sector: 7 }
      continue
    }
    const m = /大角度([1-6])区/.exec(fileName)
    if (m) {
      const n = Number(m[1])
      outerFiles.set(n, { filePath: path, fileName, sector: n })
      continue
    }
    unmatched.push(path)
  }
  return { innerFile, outerFiles, unmatched }
}

/** 七孔插值配置（判别变体，spec §2.3）：1 个内区文件 + 恰 6 个扇区文件 */
export interface SevenHoleTraversalInterpolationConfig {
  /** prb-set=.prb 文件集；calibration-csv=七孔校准 CSV 文件集（校准导出直接导入） */
  kind: 'seven-hole-prb-set' | 'seven-hole-calibration-csv'
  innerFile: SevenHolePrbFileInfo
  outerFiles: [
    SevenHolePrbFileInfo, SevenHolePrbFileInfo, SevenHolePrbFileInfo,
    SevenHolePrbFileInfo, SevenHolePrbFileInfo, SevenHolePrbFileInfo
  ]
}

/** 七孔数据源格式（向导文件选择与导入动作判别） */
export type SevenHolePrbSource = 'prb' | 'calibration-csv'

/** 七孔 PRB 编辑态（向导内允许空槽位；持久化前必须补全为 SevenHoleTraversalInterpolationConfig） */
export interface SevenHolePrbDraft {
  source: SevenHolePrbSource
  innerFile: SevenHolePrbFileInfo | null
  outerFiles: (SevenHolePrbFileInfo | null)[]
}

/**
 * 五孔插值配置变体：由旧扁平字段（prbFile/multiPrb/useMultiPrb/
 * interpolationAlgorithm/calibrationCsvFile）规范化而来；
 * 优先级与后端恢复链一致：校准 CSV > 多 PRB > 单 PRB > 未配置。
 */
export type FiveHoleTraversalInterpolationConfig =
  | { kind: 'calibration-csv'; file: CalibrationCsvFileInfo }
  | { kind: 'multi-prb'; config: MultiPrbConfig }
  | { kind: 'single-prb'; file: PrbFileInfo }
  | { kind: 'none' }

/**
 * 探针判别配置（spec §2.3 激活变体）：五孔/七孔各自的插值配置在类型层互斥。
 * 持久化 JSON 允许双变体并存（五孔字段 + sevenHolePrb），probeType 标记激活方；
 * 未知 probeType 或激活七孔文件集不齐在 normalizeTraversalProbeConfig 边界报错。
 */
export type TraversalProbeConfig =
  | {
      probeType: 'five-hole'
      probeChannels: ProbeChannelConfig[]
      interpolation: FiveHoleTraversalInterpolationConfig
    }
  | {
      probeType: 'seven-hole'
      probeChannels: ProbeChannelConfig[]
      interpolation: SevenHoleTraversalInterpolationConfig
    }

/** 探针展示元数据（仅标题与 Alpha/Beta 标签键，不含任何计算逻辑，spec §6.5） */
export interface TraversalProbePresentation {
  titleKey: string
  alphaLabelKey: string
  betaLabelKey: string
}

/**
 * 角度语义注册表：五孔 Alpha=攻角/Beta=侧滑角；七孔 Alpha=侧滑角/Beta=迎角
 * （与后端字段名复用但物理含义不同，UI 必须按探针类型查表标注）。
 */
export const TRAVERSAL_PROBE_PRESENTATION: Record<TraversalProbeType, TraversalProbePresentation> = {
  'five-hole': { titleKey: 'fiveHoleTraversalTest', alphaLabelKey: 'angleOfAttack', betaLabelKey: 'sideslipAngle' },
  'seven-hole': { titleKey: 'sevenHoleTraversalTest', alphaLabelKey: 'sideslipAngle', betaLabelKey: 'angleOfAttack' }
} as const

/** 旧扁平五孔字段集合（用于五孔插值配置抽取；字段均可缺省） */
type FiveHoleFlatFields = {
  prbFile?: PrbFileInfo | null
  multiPrb?: MultiPrbConfig
  useMultiPrb?: boolean
  interpolationAlgorithm?: InterpolationAlgorithm
  calibrationCsvFile?: CalibrationCsvFileInfo | null
}

/** 旧扁平五孔字段 → 五孔插值配置变体（优先级：CSV > 多 PRB > 单 PRB > 未配置） */
function fiveHoleInterpolationFromRaw(cfg: FiveHoleFlatFields): FiveHoleTraversalInterpolationConfig {
  if (cfg.interpolationAlgorithm === 'new' && cfg.calibrationCsvFile) {
    return { kind: 'calibration-csv', file: cfg.calibrationCsvFile }
  }
  if (cfg.useMultiPrb && cfg.multiPrb) {
    return { kind: 'multi-prb', config: cfg.multiPrb }
  }
  if (cfg.prbFile) {
    return { kind: 'single-prb', file: cfg.prbFile }
  }
  return { kind: 'none' }
}

/** 七孔文件集边界校验（与后端 normalizeAndValidateProbeType 同契约） */
function normalizeSevenHoleInterpolation(raw: unknown): SevenHoleTraversalInterpolationConfig {
  const prb = (raw ?? {}) as {
    kind?: string
    innerFile?: SevenHolePrbFileInfo
    outerFiles?: (SevenHolePrbFileInfo | null)[]
  }
  if (prb.kind != null && prb.kind !== 'seven-hole-prb-set' && prb.kind !== 'seven-hole-calibration-csv') {
    throw new Error(`七孔插值配置 kind 必须为 'seven-hole-prb-set' 或 'seven-hole-calibration-csv'，实际 ${String(prb.kind)}`)
  }
  if (!prb.innerFile?.filePath) {
    throw new Error('七孔配置缺少内区文件 (sevenHolePrb.innerFile)')
  }
  const outer = prb.outerFiles
  if (!Array.isArray(outer) || outer.length !== 6) {
    throw new Error(`七孔配置扇区文件必须恰为 6 份，实际 ${Array.isArray(outer) ? outer.length : 0} 份`)
  }
  outer.forEach((f, i) => {
    if (!f?.filePath) {
      throw new Error(`七孔配置扇区 ${i + 1} 文件路径为空`)
    }
  })
  return {
    kind: (prb.kind as SevenHoleTraversalInterpolationConfig['kind'] | undefined) ?? 'seven-hole-prb-set',
    innerFile: prb.innerFile,
    outerFiles: outer as SevenHoleTraversalInterpolationConfig['outerFiles']
  }
}

/**
 * normalizeTraversalProbeConfig：把持久化配置 JSON 规范化为「激活」探针判别配置。
 *
 * 双变体语义：五孔字段与 sevenHolePrb 在持久化 JSON 中并存合法，probeType 仅
 * 标记激活方。本函数只抽取激活变体；未激活变体字段留在原始配置对象中由
 * 持久化往返携带，不进入返回的判别配置。
 *
 * 规则：
 *   - 旧扁平五孔 JSON（无 probeType）→ five-hole 变体；
 *   - 未知 probeType → 抛错（不静默降级）；
 *   - 激活七孔时 sevenHolePrb 必须齐全，否则抛错。
 */
export function normalizeTraversalProbeConfig(raw: unknown): TraversalProbeConfig {
  const cfg = (raw ?? {}) as Partial<TraversalTestConfig> & { probeType?: string }
  const probeType = cfg.probeType ?? ''
  switch (probeType) {
    case '':
    case 'five-hole':
      return {
        probeType: 'five-hole',
        probeChannels: cfg.channels?.probeChannels ?? [],
        interpolation: fiveHoleInterpolationFromRaw(cfg)
      }
    case 'seven-hole':
      return {
        probeType: 'seven-hole',
        probeChannels: cfg.channels?.probeChannels ?? [],
        interpolation: normalizeSevenHoleInterpolation(cfg.sevenHolePrb)
      }
    default:
      throw new Error(`未知探针类型: ${probeType}（仅支持 five-hole / seven-hole）`)
  }
}

export interface TraversalRawPressure {
  P1: number
  P2: number
  P3: number
  P4: number
  P5: number
  /** 七孔外围孔 P6/P7（仅 seven-hole；五孔响应中缺失） */
  P6?: number
  P7?: number
  Patm: number
  Tatm: number
  P0?: number
  Ps?: number
}

export type TraversalInterpolationInput = Pick<
  TraversalRawPressure,
  'P1' | 'P2' | 'P3' | 'P4' | 'P5' | 'Patm' | 'Tatm'
>

/** 实时插值请求输入：五孔字段 + 七孔可选 P6/P7（七孔时必填，spec §5.6 超集 DTO） */
export type TraversalRealtimeInput = TraversalInterpolationInput & { P6?: number; P7?: number }

/** 插值计算结果 */
export interface InterpolationResult {
  alpha: number
  beta: number
  machNumber: number
  velocity: number
  dynamicPressure: number
  /** 密度（七孔结果不含此字段；五孔新算法路径才有值） */
  density?: number
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
  /**
   * 探针类型：'five-hole'(默认，旧配置缺省等价) | 'seven-hole'。
   * 驱动插值器选择、通道标签集与输入装配（spec-seven-hole-traversal §2.3）。
   */
  probeType?: TraversalProbeType
  /**
   * 七孔 PRB 文件集（七孔变体数据；与五孔字段并存合法，probeType 标记激活方）。
   */
  sevenHolePrb?: SevenHoleTraversalInterpolationConfig | null
  /**
   * 未激活探针类型的通道绑定（双变体持久化）。
   * 激活通道在 channels.probeChannels；未激活侧通道存于此（后端忽略此字段，
   * 启动遍历只用激活集合）。
   */
  inactiveProbeChannels?: ProbeChannelConfig[]
  dwellTimeMs: number
  samplesPerPoint: number
  savePath: string
  saveFileName: string
  saveOptions: TraversalSaveOptions
  /** 数据验证配置（可选，与 Cursor DAQ 行为一致） */
  validation?: DataValidationConfig
  /** 稳定等待配置（可选，与 Cursor DAQ 行为一致） */
  stabilization?: StabilizationConfig
  /**
   * 运动安全配置（可选）。
   * 为 undefined 时后端使用 DefaultMotionSafety（arrivalTolerance=0.2,
   * criticalDeviationLimit=5.0, noProgressTimeoutMs=2000, progressEpsilon=0.001）。
   * 通过 checkpoint 持久化，Resume 时从 snapshot 还原，不重新读取前端当前配置。
   */
  motionSafety?: MotionSafetyConfig
}

/**
 * 遍历点坐标（API 响应）。
 * alpha/beta 允许 null：line/rectangle/sector 模式通过 markAxesNaN 将未配置轴标记为 NaN，
 * 后端 JSON 序列化为 null。前端消费方必须做 number 类型守卫，禁止直接 toFixed。
 *
 * 兼容字段名 alpha/beta 实际语义是遍历逻辑目标 X/Y（非插值结果攻角/侧滑角）；
 * z/u 为逻辑目标 Z/U，仅 custom 模式有实际值，line/rectangle/sector 为 null。
 * z/u 为可选字段：旧后端不返回，消费方按缺失即不显示处理。
 */
export type TraversalCoordValue = number | null

export interface TraversalCoordPoint {
  alpha: TraversalCoordValue
  beta: TraversalCoordValue
  /** 逻辑目标 Z（后端 currentPointCoordinates.z）；非 custom 模式为 null，旧后端可能缺失 */
  z?: TraversalCoordValue
  /** 逻辑目标 U（后端 currentPointCoordinates.u）；非 custom 模式为 null，旧后端可能缺失 */
  u?: TraversalCoordValue
}

/** 测试数据点 */
export interface TraversalDataPoint {
  pointId: number
  // alpha/beta 允许 null：line 模式 Y 轴 NaN 序列化为 null
  coordinates: TraversalCoordPoint
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
  currentPoint?: TraversalCoordPoint
  currentPointPhase?: TraversalPointPhase
  latestData?: TraversalDataPoint
  dataPoints?: TraversalDataPoint[]
  progress: number
  startTime?: number
  estimatedRemaining?: number
  lastError?: string
  lastErrorCode?: TraversalErrorCode
  validationWarnings?: string[]
  /**
   * 非致命运行警告（当前唯一来源：回零失败）。
   * 数据全部采完后回零失败不判测试失败（status 仍为 completed），
   * 后端在此字段写入提示，前端在侧边栏以 warning 样式展示。
   */
  warning?: string
  /**
   * 实际落盘的 CSV 文件完整路径（撞名 -2/-3 后缀后的真实路径）。
   * 后端 Start/ResumeFromCheckpoint 在 openReliabilityPorts 之后写入：
   * csvPort.Open 在 Create 模式撞名时会自动追加 -2/-3 后缀，
   * 实际路径可能与 config.savePath + saveFileName 拼接的预期路径不同。
   * 侧边栏优先用此字段展示真实文件名；空串表示尚未启动或后端未注入 v2 csvPort，
   * 此时回退到 config 静态拼接的预期路径。
   */
  csvPath?: string
  /**
   * 运动安全故障现场快照（仅 lastErrorCode 为运动安全错误码时存在）。
   * 后端 handleMotionSafetyFailure 写入，前端据此在运行状态栏展示
   * "故障发生在哪个控制器/轴/第几个点，目标 vs 实际" 等关键诊断信息。
   * nil/null 表示无运动安全故障或故障已清除。
   */
  motionSafetyFailure?: MotionSafetyFailure | null
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
  currentPoint: TraversalCoordPoint
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
  // 运动安全错误码（与后端 traversal.ErrorCode 一一对应）
  // 普通停止类：故障不严重，调用 Stop 后可继续操作
  | 'POSITION_DEVIATION'
  | 'MOTION_OVERSHOOT'
  | 'MOTION_NO_PROGRESS'
  | 'MOTION_TIMEOUT'
  // 急停类：故障严重，调用 EmergencyStop 防止撞机
  | 'CRITICAL_POSITION_DEVIATION'
  | 'LIMIT_SWITCH_TRIGGERED'
  | 'MOTION_STATUS_UNAVAILABLE'
  | 'EMERGENCY_STOP_FAILED'

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

/**
 * 运动安全配置（与后端 traversal.MotionSafetyConfig 一一对应）。
 *
 * 所有数值字段为可选——为 undefined 时后端使用 DefaultMotionSafety 中的默认值。
 * 通过指针字段实现"零值即默认"，避免前端必须填全 4 个字段才能保存配置。
 *
 * 字段语义：
 *   - arrivalTolerance: 到位容差（mm/°）。轴停且 |actual-target| ≤ tolerance 视为到位
 *   - criticalDeviationLimit: 严重偏离阈值。轴停且 |actual-target| ≥ limit 触发急停
 *   - noProgressTimeoutMs: 无进展超时（ms）。运动中位置长时间无变化判卡死
 *   - progressEpsilon: 进展判定阈值。位置变化超过该值视为"有进展"
 *   - axisOverrides: 按轴覆盖（key 为轴名 "X"/"Y"/"Z"/"U"）。
 *       覆盖项中未指定的字段继承全局配置。
 */
export interface MotionSafetyConfig {
  arrivalTolerance?: number
  criticalDeviationLimit?: number
  noProgressTimeoutMs?: number
  progressEpsilon?: number
  axisOverrides?: Record<string, MotionSafetyConfig>
}

/**
 * 默认运动安全阈值（与后端 core/traversal.DefaultMotionSafety() 一一对应）。
 *
 * 这里的值是"安全的保守起点"，覆盖大多数设备类型；生产部署前必须通过 HIL
 * 测试根据具体设备精度调整（参见 spec §Confirmed Decisions #8）。
 *
 * 前端用途：
 *   - 配置面板输入框 placeholder 显示具体默认值，让操作员一眼看到留空的生效值
 *   - 按轴覆盖留空时 placeholder 显示"继承全局（或默认）"，必须取此处的兜底值
 *
 * 同步约束：后端 defaultArrivalTolerance/defaultCriticalDeviationLimit/
 * defaultNoProgressTimeoutMs/defaultProgressEpsilon 修改时必须同步更新此处。
 */
export const DEFAULT_MOTION_SAFETY: Required<
  Pick<MotionSafetyConfig, 'arrivalTolerance' | 'criticalDeviationLimit' | 'noProgressTimeoutMs' | 'progressEpsilon'>
> = {
  arrivalTolerance: 0.2,
  criticalDeviationLimit: 5.0,
  noProgressTimeoutMs: 2000,
  progressEpsilon: 0.001,
}

/**
 * 运动安全判定结果（与后端 MotionSafetyVerdict 一一对应）。
 *
 * - 'ok': 正常运动中，继续等待
 * - 'arrived': 已到位（轴停且偏差 ≤ arrivalTolerance）
 * - 'deviation': 超差——轴已停但偏差 > tolerance 且 < criticalDeviationLimit
 * - 'critical_deviation': 严重偏离——轴已停且偏差 ≥ criticalDeviationLimit，需急停
 * - 'limit_triggered': 撞限位——PosLimit 或 NegLimit 触发，需急停
 * - 'no_progress': 运动中无进展——Moving=true 但位置长时间无变化
 * - 'overshoot': 越过目标——Moving=true 且位置已穿越目标位置
 * - 'status_unavailable': 状态不可用——控制器掉线/已急停/目标轴连续缺失，需急停
 */
export type MotionSafetyVerdict =
  | 'ok'
  | 'arrived'
  | 'deviation'
  | 'critical_deviation'
  | 'limit_triggered'
  | 'no_progress'
  | 'overshoot'
  | 'status_unavailable'

/**
 * 运动安全故障快照（与后端 MotionSafetyFailure 一一对应）。
 *
 * 故障发生时由遍历层立即构造并停止运动，包含故障现场的关键信息：
 *   - controllerId/axis: 故障发生的控制器与轴
 *   - verdict: 判定结果（决定停机策略：Stop 或 EmergencyStop）
 *   - target/actual: 故障时刻的目标位置与实际位置
 *   - pointIndex: 故障发生在哪个遍历点（用于日志定位）
 */
export interface MotionSafetyFailure {
  controllerId: string
  controllerName?: string
  axis: string
  verdict: MotionSafetyVerdict
  target: number
  actual: number
  pointIndex: number
}

/** 是否需要急停——前端展示告警时按 verdict 严重级别区分颜色 */
export function isMotionSafetyEmergency(verdict: MotionSafetyVerdict): boolean {
  return verdict === 'critical_deviation' || verdict === 'limit_triggered' || verdict === 'status_unavailable'
}

/** 是否为故障 verdict（非 ok / arrived） */
export function isMotionSafetyFailure(verdict: MotionSafetyVerdict): boolean {
  return verdict !== 'ok' && verdict !== 'arrived'
}

/**
 * 运动安全 verdict → 本地化标签映射（共享函数）。
 *
 * verdict 值与后端 traversal.MotionSafetyVerdict 一一对应（'ok'/'arrived'/'deviation' 等）。
 * ok/arrived 不会出现在故障快照中（仅作为运行中判定结果），缺省 verdict 返回其原值。
 *
 * 抽取为共享函数：校准告警卡片（MotionSafetyAlertCard）与遍历实时监控侧边栏
 * （TraversalLiveMonitor）均通过此函数获取本地化标签，避免两处独立 switch
 * 实现产生行为分叉（新增 verdict 时一处漏改）。
 *
 * @param verdict 运动安全判定结果
 * @param labels verdict → 本地化字符串映射；缺省 verdict 返回其原值
 */
export function getMotionSafetyVerdictLabel(
  verdict: MotionSafetyVerdict,
  labels: Partial<Record<MotionSafetyVerdict, string>>
): string {
  return labels[verdict] ?? verdict
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
