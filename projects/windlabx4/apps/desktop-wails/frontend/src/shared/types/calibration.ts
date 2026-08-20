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
export type CalibrationType = 'five-hole' | 'three-hole' | 'total-pressure' | 'total-temperature' | 'seven-hole'

/** 校准状态 */
// 'stopped'：用户主动 stop 后的终态（spec Decision #4 / I7）——保留已采数据可导出，与 idle 区分
export type CalibrationStatus = 'idle' | 'configuring' | 'running' | 'paused' | 'completed' | 'error' | 'stopped'

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
  // 七孔探针 11 个必需角色（spec §6.1）：
  //   - sevenHole.p1~p7：7 个压力孔（外围 6 孔 P1~P6 + 中心孔 P7）
  //   - sevenHole.pTotal：风洞参考总压（K0/Ks/Ma 公式分母来源）
  //   - sevenHole.pTunnelStatic：风洞参考静压（Ks/Ma 公式分母来源）
  //   - sevenHole.pAtm：大气压力（A→C 边界转换用）
  //   - sevenHole.tAtm：大气温度（静温/真空速计算用）
  | 'sevenHole.p1'
  | 'sevenHole.p2'
  | 'sevenHole.p3'
  | 'sevenHole.p4'
  | 'sevenHole.p5'
  | 'sevenHole.p6'
  | 'sevenHole.p7'
  | 'sevenHole.pTotal'
  | 'sevenHole.pTunnelStatic'
  | 'sevenHole.pAtm'
  | 'sevenHole.tAtm'

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
  /** 球罐压力通道引用，仅用于前端实时显示当前球罐压力值，不参与闸门判定逻辑 */
  pressureChannel?: ChannelRef
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

// ==================== 七孔探针校准类型（spec Task 18） ====================
//
// 与后端 internal/core/calibration/seven_hole.go 同名结构对齐：
//   - SevenHoleMode：校准模式枚举（完整 / 数据集）
//   - SevenHoleConfig：点位生成参数（α/β/θ/φ 范围与步长），透传到后端 GenerateSevenHolePoints
//   - SevenHolePreviewResult：点位预览结果（点位列表 + 内/外区点数聚合）
//   - SevenHoleRawData / SevenHoleCoefficients / SevenHoleDataPoint：实时数据与结果数据点
//
// 字段语义参考 spec §3.1 / §4.1 / §4.2 / §6.2，及后端 seven_hole.go 同名结构注释。

/** 七孔校准模式（spec §3.1）
 * - 'full'：完整模式，外区 θ/φ 由用户配置范围+步长生成（默认 715 点 = 169 内 + 546 外）
 * - 'dataset'：数据集模式，外区 θ 硬编码 {30°,35°,40°,45°}，φ 按扇区独立配置（481 点 = 169 内 + 312 外）
 */
export type SevenHoleMode = 'full' | 'dataset'

/** 七孔点位生成配置（与后端 calibration.SevenHoleConfig 对齐）
 *
 * 字段语义（后端 seven_hole.go L486-510）：
 *   - mode：校准模式（full / dataset），决定外区 θ 与 φ 的取值策略
 *   - innerAlphaMin/Max/Step：内区 α 范围与步长（度），完整/数据集模式都使用此配置
 *   - innerBetaMin/Max/Step：内区 β 范围与步长（度）
 *   - outerThetaMin/Max/Step：外区 θ 范围与步长（度），仅完整模式生效；数据集模式忽略并使用硬编码 {30°,35°,40°,45°}
 *   - outerPhiMin/Max/Step：外区 φ 范围与步长（度），仅完整模式生效；数据集模式按扇区独立配置
 *   - serpentine：是否启用蛇形走位（true 时奇数行 α/θ 反向）
 */
export interface SevenHoleConfig {
  mode: SevenHoleMode
  innerAlphaMin: number
  innerAlphaMax: number
  innerAlphaStep: number
  innerBetaMin: number
  innerBetaMax: number
  innerBetaStep: number
  outerThetaMin: number
  outerThetaMax: number
  outerThetaStep: number
  outerPhiMin: number
  outerPhiMax: number
  outerPhiStep: number
  serpentine: boolean
}

/** 七孔点位预览结果（与后端 calibration.SevenHolePreviewResult 对齐）
 *
 * 用于"配置向导"实时显示总点数（如 715 点 = 169 内区 + 546 外区），
 * 帮助用户在启动校准前确认点位规模与预计耗时。
 */
export interface SevenHolePreviewResult {
  /** 完整点位列表（含内区+外区，按蛇形/数据集顺序排列） */
  points: CalibrationPoint[]
  /** 总点数 = innerCount + outerCount */
  totalCount: number
  /** 内区点数（α-β 网格点数） */
  innerCount: number
  /** 外区点数（6 扇区 × θ × φ 网格点数） */
  outerCount: number
}

/** 七孔探针原始数据（与后端 SevenHoleRawData 对齐）
 *
 * 压力基准（spec §2.1）：
 *   - P1~P7、pTotal、pStatic 全部为表压（A 基准，相对环境大气压，可正可负）
 *   - 系数计算直接使用表压（B 基准 = A 基准，因压差比同基准等价）
 *   - 仅马赫数计算入口处把 pTotal/pStatic 转绝压（A→C 边界，仅转换一次）
 *
 * 指针字段语义：pTotal/pStatic/tTunnel 为可选（缺失时 K0/Ks/Ma/V 跳过计算），
 * 与五孔/三孔/总压的 *float64 nil 语义一致。
 */
export interface SevenHoleRawData {
  p1: number
  p2: number
  p3: number
  p4: number
  p5: number
  p6: number
  p7: number
  pAtm: number
  tAtm: number
  pTotal?: number
  pStatic?: number
  tTunnel?: number
}

/** 七孔探针系数（与后端 SevenHoleCoefficients 对齐）
 *
 * 内区系数（P7 最大时，spec §4.1 公式 1-8）：
 *   - Kalpha / Kbeta：方向系数
 *   - K0 / Ks：总压/静压系数
 *
 * 外区系数（Pn 最大时 n∈{1..6}，spec §4.2 公式 9-12）：
 *   - Ktheta / Kphi：方向系数（Kphi 不取绝对值，spec §4.3）
 *   - K0Outer / KsOuter：外区总压/静压系数
 *
 * 马赫数/速度（可选，需 pTotal/pStatic/pAtm 齐全）：
 *   - machNumber / velocity
 */
export interface SevenHoleCoefficients {
  // 内区系数
  Kalpha: number
  Kbeta: number
  K0: number
  Ks: number
  // 外区系数
  Ktheta: number
  Kphi: number
  K0Outer: number
  KsOuter: number
  // 气动参数（可选）
  machNumber?: number
  velocity?: number
}

/** 七孔探针数据点（与后端 SevenHoleDataPoint 对齐） */
export interface SevenHoleDataPoint {
  pointId: number
  /** 双坐标（spec §3.4）：logicalCoordinates=θ/φ（外区）或 α/β（内区），motionCoordinates=α/β（运动下发） */
  coordinates: Record<string, number>
  /** 运动下发坐标（α/β 顺序，spec Task 10） */
  motionCoordinates?: Record<string, number>
  /** 当前分区（"inner" / "outer"） */
  region: string
  /** 当前扇区编号（内区=7，外区=1..6） */
  sector: number
  /** 边界点标记（空串=非边界，"P7-Pn"或"Pn-Pm"=并列边界） */
  boundaryFlag: string
  rawData: SevenHoleRawData
  coefficients: SevenHoleCoefficients
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
  | SevenHoleDataPoint

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
  /** 七孔探针点位布局配置（type='seven-hole' 时必填） */
  sevenHoleLayout?: SevenHoleConfig
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

/**
 * 实时物理量快照（与后端 core/calibration.LivePhysics 对齐，spec Task 13）。
 *
 * 设计动机：前端 5Hz 轮询 status 时需展示当前马赫数/速度，但既有 currentStatus 只持久化
 * 校准业务字段（点号/进度/数据点），物理量需基于实时通道数据即时计算。后端在 Status() 调用
 * 时即时计算并返回此字段，绝不写入 currentStatus（避免 stale 残留与 writer/CSV 污染）。
 *
 * 三态语义（与后端 *float64 指针语义一致）：
 *   - 字段省略（undefined）：缺失——必需通道未配置/读取失败/物理非法如 Pt < Ps，UI 显示 "--"
 *   - 字段值为 0：有效零——Pt == Ps 即零流量（Task 12），UI 显示格式化的 0
 *   - 字段值为正数：正常计算值
 *
 * 整体 livePhysics 省略（undefined）表示类型不支持实时物理量（总温）或未启动校准。
 * 整体存在但字段省略表示类型支持但运行期读取失败（与"整体省略"区分）。
 *
 * 关键不变量：前端不得用 truthiness 判断 `livePhysics.machNumber`——0 是有效零，
 * 必须用 `!== undefined` 区分缺失与零值，否则零流量场景 UI 误显示 "--"。
 */
export interface LivePhysics {
  /** 马赫数（缺失 undefined / 有效零 0 / 正常 正数） */
  machNumber?: number
  /** 真空速 m/s（缺失 undefined / 有效零 0 / 正常 正数） */
  velocity?: number
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
  pausedDurationMs?: number
  estimatedTimeRemaining?: number
  lastError?: string
  /**
   * 实时物理量快照（spec Task 13/14）。
   *
   * 后端在 Status() 调用时即时计算并返回，绝不持久化到 currentStatus。
   * 前端 store 应直接映射此字段到 calculatedPhysics，不再本地用 ATM_GAMMA 等公式计算（Task 15 删除前端公式）。
   *
   * 整体 undefined：类型不支持（总温）或未启动校准。
   * 整体存在但 machNumber/velocity 字段 undefined：类型支持但通道读取失败。
   * 字段值为 0：有效零流量（Pt == Ps），UI 必须显示格式化的 0 而非 "--"。
   */
  livePhysics?: LivePhysics
  /** Instantaneous five-hole coefficients for display; not a completed point result. */
  liveFiveHoleCoefficients?: FiveHoleCoefficients
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

/**
 * 校准 Main 组件对外暴露的契约接口。
 *
 * 用途：CalibrationWindow.handleSettingsSaved 通过 currentMainRef.reloadSavedConfig()
 *   在 Settings 保存后触发对应 Main 重新加载配置——否则 currentConfig 保留挂载时旧值，
 *   canStartCalibration 不会重算，UI 一直提示"未配置"，必须切走再切回才生效。
 *
 * 强制约束：四个 Main 组件（FiveHoleMain / ThreeHoleMain / TotalPressureMain /
 *   TotalTemperatureMain）必须通过 defineExpose 暴露本接口，
 *   由 calibrationMainExpose.contract.ts 在编译期断言，缺暴露时 vue-tsc 报错。
 */
export interface CalibrationMainExpose {
  reloadSavedConfig: () => Promise<void> | void
}
