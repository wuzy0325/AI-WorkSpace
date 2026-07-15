/**
 * 校准模块类型定义
 */

import type {
  MotionSafetyConfig,
  MotionSafetyFailure,
  TraversalErrorCode,
} from './traversal'

/**
 * 运动安全相关类型 re-export（与后端 core/calibration 引用 core/traversal 同构）。
 *
 * 设计动机：校准模块的运动安全机制从遍历模块移植，类型定义完全一致；
 * 通过 re-export 让消费方从 `@shared/types/calibration` 一处导入，
 * 避免 UI 组件同时引用两个 type 模块。
 */
export type {
  MotionSafetyConfig,
  MotionSafetyFailure,
  MotionSafetyVerdict,
} from './traversal'
export {
  DEFAULT_MOTION_SAFETY,
  isMotionSafetyEmergency,
  isMotionSafetyFailure,
} from './traversal'

/** 校准类型 */
export type CalibrationType = 'five-hole' | 'three-hole' | 'total-pressure' | 'total-temperature'

/** 校准状态 */
export type CalibrationStatus = 'idle' | 'configuring' | 'running' | 'paused' | 'completed' | 'error'

/** 通道引用（设备ID + 通道索引） */
export interface ChannelRef {
  deviceId: string
  channelIndex: number
}

/**
 * 探针/风洞测点的"语义角色"
 */
export type ProbeChannelRole =
  | 'threeHole.p1'
  | 'threeHole.p2'
  | 'threeHole.p3'
  | 'threeHole.pAtm'
  | 'threeHole.tAtm'
  | 'threeHole.pTotal'
  | 'threeHole.pStatic'
  | 'fiveHole.p1'
  | 'fiveHole.p2'
  | 'fiveHole.p3'
  | 'fiveHole.p4'
  | 'fiveHole.p5'
  | 'fiveHole.pAtm'
  | 'fiveHole.tAtm'
  | 'fiveHole.pTotal'
  | 'fiveHole.pTunnelStatic'
  | 'fiveHole.tTunnel'
  | 'totalPressure.pAtm'
  | 'totalPressure.tAtm'
  | 'totalPressure.pTunnelTotal'
  | 'totalPressure.pTunnelStatic'
  | 'totalPressure.tTunnel'
  | 'totalPressure.pProbeTotal'
  | 'totalTemperature.tTotal'
  | 'totalTemperature.tStatic'
  | 'totalTemperature.tAtm'

/** 测点配置 */
export interface ProbeChannelConfig {
  name: string
  role?: ProbeChannelRole
  channel: ChannelRef
  enabled: boolean
  precision?: number
}

export interface CalibrationDerivedValuePrecision {
  machNumber?: number
  velocity?: number
}

/** 运动轴配置 */
export interface MotionAxisConfig {
  name: string
  controllerId: string
  axis: string
}

/** 校准点位 */
export interface CalibrationPoint {
  id: number
  coordinates: Record<string, number>
}

/** 球罐判定门控配置 */
export interface SphereTankGateConfig {
  enabled: boolean
  waitTimeSec: number
  /** 球罐判定总超时（秒），<=0 时使用默认 300 秒 */
  timeoutSec?: number
  stableTimeChannel: ChannelRef
}

/** 采样批量读取策略 */
export interface AcquisitionSamplingConfig {
  batchTimeoutMs?: number
  batchPollIntervalMs?: number
  batchMaxAgeMs?: number
}

/** 五孔探针点位布局配置 */
export interface FiveHolePointLayout {
  alphaMin: number
  alphaMax: number
  alphaStep: number
  betaMin: number
  betaMax: number
  betaStep: number
  /** 蛇形走位：奇数行反向遍历 α；默认 false 为逐行 raster 扫描 */
  serpentine?: boolean
}

/** 三孔探针点位布局配置 */
export interface ThreeHolePointLayout {
  thetaMin: number
  thetaMax: number
  thetaStep: number
}

/** 三孔探针原始数据
 *
 * 孔序约定（与 shared/algorithms/go/threehole/interpolation 对齐）：
 *   P1 = 侧孔1（左孔）
 *   P2 = 中心孔
 *   P3 = 侧孔2（右孔）
 *   ΔP = 2·P2 - P1 - P3
 */
export interface ThreeHoleRawData {
  p1: number
  p2: number
  p3: number
  pAtm: number
  tAtm: number
  pTotal?: number
  pStatic?: number
}

/** 三孔探针系数（与插值器 PRB 文件列对齐）
 *
 * 工程命名：Kb(Kβ) 角度系数 / K0 总压系数 / Kv 速度系数
 * MachNumber/Velocity 为实时气动参数（可选）：需 PTotal + PStatic + PAtm + TAtm 齐全时计算
 */
export interface ThreeHoleCoefficients {
  Kb: number
  K0: number
  Kv: number
  machNumber?: number
  velocity?: number
}

/** 三孔探针数据点 */
export interface ThreeHoleDataPoint {
  pointId: number
  coordinates: Record<string, number>
  rawData: ThreeHoleRawData
  coefficients: ThreeHoleCoefficients
  sampleCount: number
  stdDev?: number
  startTime: number
  endTime: number
}

/** 五孔探针原始数据 */
export interface FiveHoleRawData {
  p1: number
  p2: number
  p3: number
  p4: number
  p5: number
  pAtm: number
  tAtm: number
  pTotal?: number
  pStatic?: number
}

/** 五孔探针系数 */
export interface FiveHoleCoefficients {
  Kalpha: number
  Kbeta: number
  CPT: number
  CPS: number
  machNumber?: number
}

/** 总压探针点位布局配置 */
export interface TotalPressurePointLayout {
  alphaMin: number
  alphaMax: number
  alphaStep: number
}

/** 总压探针原始数据 */
export interface TotalPressureRawData {
  pAtm: number
  tAtm: number
  pTunnelTotal: number
  pTunnelStatic: number
  tTunnel: number
  pProbeTotal: number
}

/** 总压探针系数 */
export interface TotalPressureCoefficients {
  CPT: number
  error: number
  /** 马赫数（可选，需风洞总压/静压/大气压/温度齐全；与后端 *float64 nil 语义一致） */
  machNumber?: number
  /** 速度 m/s（可选，需风洞总压/静压/大气压/温度齐全；与后端 *float64 nil 语义一致） */
  velocity?: number
}

/** 总压探针数据点 */
export interface TotalPressureDataPoint {
  pointId: number
  alpha: number
  rawData: TotalPressureRawData
  coefficients: TotalPressureCoefficients
  sampleCount: number
  stdDev?: number
  startTime: number
  endTime: number
}

/** 校准数据点（完整记录） */
export interface CalibrationDataPoint {
  pointId: number
  coordinates: Record<string, number>
  rawData: FiveHoleRawData
  coefficients: FiveHoleCoefficients
  sampleCount: number
  stdDev?: number
  startTime: number
  endTime: number
  rawDeviceChannels?: Record<string, number[]>
}

/** 校准数据点（进度/状态通用联合类型） */
export type CalibrationAnyDataPoint =
  | CalibrationDataPoint
  | ThreeHoleDataPoint
  | TotalPressureDataPoint
  | TotalTemperatureCalibrationPoint

/**
 * 校准错误码（与后端 traversal.ErrorCode 一一对应）。
 *
 * 设计决策：校准模块复用遍历模块的错误码集合，避免重复定义；后端通过
 * traversal.ErrorCodeFor(verdict) 返回的字符串值与 TraversalErrorCode 联合类型保持一致。
 * 前端据此区分告警级别（普通停止类 vs 急停类），驱动告警卡片配色与是否需要人工复位提示。
 */
export type CalibrationErrorCode = TraversalErrorCode

/** 校准配置 */
export interface CalibrationConfig {
  taskId?: string
  type: CalibrationType
  name: string
  probeChannels: ProbeChannelConfig[]
  motionAxes: MotionAxisConfig[]
  points: CalibrationPoint[]
  dwellTimeMs: number
  samplesPerPoint: number
  // 后端 csv_writer 约定 SavePath 必须是含 .csv 扩展名的完整文件路径。
  // 前端持久化时同时写入 saveFileName（仅文件名），加载时优先使用 saveFileName
  // 还原 UI 的"目录 + 文件名"分离展示，避免每次加载都从完整路径反拆文件名。
  savePath: string
  saveFileName?: string
  stopOnError?: boolean
  sphereTankGate?: SphereTankGateConfig
  acquisitionSampling?: AcquisitionSamplingConfig
  fiveHoleLayout?: FiveHolePointLayout
  threeHoleLayout?: ThreeHolePointLayout
  totalPressureLayout?: TotalPressurePointLayout
  totalTemperatureLayout?: TotalTemperaturePointLayout
  totalTemperatureConfig?: Omit<TotalTemperatureCalibrationConfig, 'type' | 'name' | 'savePath' | 'samplesPerPoint'>
  derivedValuePrecision?: CalibrationDerivedValuePrecision
  /** 界面实时数据刷新频率（Hz），影响压力/角度等 UI 更新节奏。缺省由 store 默认值决定。 */
  uiRefreshHz?: number
  /**
   * 运动安全配置（可选，与后端 core/calibration.Config.MotionSafety 对齐）。
   *
   * 留空时后端使用 traversal.DefaultMotionSafety() 兜底（arrivalTolerance=0.2、
   * criticalDeviationLimit=5.0 等）。配置非法（如 criticalDeviationLimit <
   * arrivalTolerance）会在 Start 阶段被 validateCalibrationMotionSafetyConfig 拒绝。
   *
   * 前端配置面板复用遍历模块的 MotionSafetyPanel.vue（Task 9 迁移到共享组件目录），
   * 通过 v-model 双向绑定到此字段。
   */
  motionSafety?: MotionSafetyConfig
}

/** 校准任务状态 */
export interface CalibrationTaskStatus {
  taskId: string
  type: CalibrationType
  status: CalibrationStatus
  totalPoints: number
  completedPoints: number
  progress: number
  startTime?: number
  estimatedTimeRemaining?: number
  lastError?: string
  /**
   * 结构化错误码（与后端 core/calibration.Status.LastErrorCode 对齐）。
   *
   * 后端在 failWithCode 调用时写入；空字符串表示无结构化错误码（旧路径 fallback）。
   * 前端据此区分告警级别（急停类 vs 普通停止类），决定告警卡片颜色与是否提示"需人工复位"。
   * 当 lastErrorCode 为急停类（CRITICAL_POSITION_DEVIATION / LIMIT_SWITCH_TRIGGERED /
   * MOTION_STATUS_UNAVAILABLE / EMERGENCY_STOP_FAILED）时，前端必须阻塞后续 Start 操作，
   * 强制要求操作员点击"复位"按钮才能继续。
   */
  lastErrorCode?: CalibrationErrorCode
  /**
   * 运动安全故障现场快照（与后端 core/calibration.Status.MotionSafetyFailure 对齐）。
   *
   * 后端在检测到运动安全故障时立即构造并写入 Status；failWithCode 会清空此字段，
   * 之后由 recordMotionSafetyFailure 重新写入（保证快照与错误码同源同时刻）。
   *
   * 前端展示：
   *   - null / undefined：无故障（正常状态或非运动安全错误路径）
   *   - 非 null：展示告警卡片，含 controllerId / axis / verdict / target / actual / pointIndex
   */
  motionSafetyFailure?: MotionSafetyFailure | null
  dataPoints: CalibrationAnyDataPoint[]
  /** 当前点已采样本数（1..samplesPerPoint），0 表示未开始/已完成 */
  currentSample?: number
  /** 当前点总采样数，UI 据此显示"当前点采样 i/N"子进度 */
  samplesPerPoint?: number
  /** 当前正在处理的点索引（后端 currentPointIdx，循环顶部推进，早于 moveToPoint）。
   *  前端 progressInfo 优先用此索引查 config.points 得到"目标点"，
   *  让目标角度先于实际角度变化。后端 autoEngine 为 nil 时此字段缺失，回退到 completedPoints。 */
  currentPointIndex?: number
}

export interface CalibrationModuleResult {
  taskId: string
  type: CalibrationType
  config: CalibrationConfig
  dataPoints: CalibrationAnyDataPoint[]
}

export type CalibrationWindowTag = 'main' | 'calibration' | 'traversal'

/** 校准进度更新事件 */
export interface CalibrationProgressEvent {
  taskId: string
  windowTag?: CalibrationWindowTag
  currentPoint: CalibrationPoint
  completedPoints: number
  totalPoints: number
  latestData?: CalibrationAnyDataPoint
  timestamp: number
}

/** 实时校准数据事件 */
export interface CalibrationRealtimeEvent {
  taskId: string
  windowTag?: CalibrationWindowTag
  type: CalibrationType
  point?: CalibrationPoint
  fiveHoleRaw?: FiveHoleRawData
  fiveHoleCoefficients?: FiveHoleCoefficients
  threeHoleRaw?: ThreeHoleRawData
  threeHoleCoefficients?: ThreeHoleCoefficients
  totalTemperatureState?: TotalTemperatureCalibrationState
  timestamp: number
}

/** 校准完成事件 */
export interface CalibrationCompleteEvent {
  taskId: string
  windowTag?: CalibrationWindowTag
  success: boolean
  filePath?: string
  error?: string
  duration: number
  totalPoints: number
  successPoints?: number
}

/** 总温探针校准配置 */
export interface TotalTemperatureCalibrationConfig {
  type: 'total-temperature'
  name: string
  probeChannels: {
    testProbe: ChannelRef
    standardProbe: ChannelRef
    totalPressure: ChannelRef
    staticPressure: ChannelRef
    atmosphericPressure: ChannelRef
    atmosphericTemperature: ChannelRef
  }
  targetMachNumbers: number[]
  machRange?: { min: number; max: number; step: number }
  machTolerance?: number
  stabilityCriteria: {
    sampleCount: number
    maxStdDev: number
    sampleInterval: number
  }
  samplesPerPoint: number
  sampleInterval: number
  savePath: string
  enableFitting?: boolean
  stopOnError?: boolean
  sphereTankGate?: SphereTankGateConfig
}

/** 总温探针点位布局配置 */
export interface TotalTemperaturePointLayout {
  machMin: number
  machMax: number
  machStep: number
}

/** 总温探针校准点位数据 */
export interface TotalTemperatureCalibrationPoint {
  id: number
  targetMachNumber: number
  actualMachNumber: number
  testProbeTemp: number
  standardProbeTemp: number
  recoveryCoefficient: number
  totalPressure: number
  staticPressure: number
  atmosphericPressure: number
  atmosphericTemperature: number
  ambientTemp: number
  samples: number[]
  standardSamples: number[]
  stdDev: number
  timestamp: number
  sampleCount: number
}

/** 总温探针校准状态 */
export interface TotalTemperatureCalibrationState {
  status: 'idle' | 'waiting' | 'acquiring' | 'completed'
  currentIndex: number
  currentMachNumber: number
  targetMachNumber: number
  isNearTarget: boolean
  temperatureStable: boolean
  points: TotalTemperatureCalibrationPoint[]
  startTime: number
}
