/**
 * 校准模块类型定义
 */

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

/** 三孔探针原始数据 */
export interface ThreeHoleRawData {
  p1: number
  p2: number
  p3: number
  pAtm: number
  pTotal?: number
}

/** 三孔探针系数 */
export interface ThreeHoleCoefficients {
  K: number
  Cv: number
  Cp: number
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
  machNumber: number
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
  savePath: string
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
}

/** 校准任务状态 */
export interface CalibrationTaskStatus {
  taskId: string
  type: CalibrationType
  status: CalibrationStatus
  totalPoints: number
  completedPoints: number
  currentPoint?: CalibrationPoint
  progress: number
  startTime?: number
  estimatedTimeRemaining?: number
  lastError?: string
  dataPoints: CalibrationAnyDataPoint[]
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
