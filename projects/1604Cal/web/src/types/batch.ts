/**
 * 分批计量相关类型定义
 *
 * 用于 16 通道多量程压力传感器的分批计量流程：
 * 1. 操作员录入 16 通道量程 → 自动分组 → 核对码确认 → 逐批加压 → 合并报告
 * 2. 量程现场录入，不预存台账
 * 3. 同量程通道归为一批，每批切换标准器后需输入核对码确认
 */

/** 量程单位，默认 MPa */
export type RangeUnit = 'MPa' | 'kPa' | 'bar' | 'psi'

/** 单通道量程录入 */
export interface ChannelRange {
  /** 通道编号 1-16 */
  channelId: number
  /** 量程下限（支持负值，用于负压量程如 -0.1~0.1 MPa） */
  rangeMin: number
  /** 量程上限（必须严格大于 rangeMin，作为分批标识与核对码依据） */
  rangeMax: number
  /** 量程单位 */
  rangeUnit: RangeUnit
  /** 是否跳过（一期固定 false，预留字段） */
  skipped: boolean
}

/** 批次状态。后端只有 pending/running/completed 三态，前端保持一致。 */
export type BatchStatus = 'pending' | 'running' | 'completed'

/** 批次分组 */
export interface BatchGroup {
  /** 批次唯一标识，如 "batch-1" */
  batchId: string
  /** 批次序号 1-based */
  batchIndex: number
  /** 该批次量程下限（与所有通道 rangeMin 一致） */
  rangeMin: number
  /** 该批次量程上限（与所有通道 rangeMax 一致；核对码依据） */
  rangeMax: number
  /** 该批次量程单位 */
  rangeUnit: RangeUnit
  /** 该批次包含的通道列表 */
  channels: ChannelRange[]
  /** 批次状态 */
  status: BatchStatus
  /** 已采集的数据：channelId -> 每个加压点的采集值 */
  collectedData?: Record<number, number[]>
  /** 已完成的加压点列表 */
  pressurePoints?: number[]
}

/** 分批计量会话 */
export interface BatchSession {
  /** 16 通道量程配置 */
  channelRanges: ChannelRange[]
  /** 自动分组结果 */
  batches: BatchGroup[]
  /** 当前正在执行的批次索引（-1 表示未开始） */
  currentBatchIndex: number
}

/** 核对码校验请求 */
export interface VerificationRequest {
  /** 批次 ID */
  batchId: string
  /** 操作员输入的核对码（量程数值字符串） */
  verificationCode: string
}

/** 核对码校验结果 */
export interface VerificationResult {
  /** 是否校验通过 */
  valid: boolean
  /** 失败时的提示信息 */
  message: string
}

/** 批次配置请求 */
export interface BatchConfigRequest {
  /** 16 通道量程配置 */
  channelRanges: ChannelRange[]
  /** 自动分组结果 */
  batches: BatchGroup[]
}

/** 批次配置响应 */
export interface BatchConfigResponse {
  /** 批次会话 ID */
  sessionId: string
}

/** 批次状态查询响应 */
export interface BatchStatusResponse {
  /** 会话状态 */
  state: string
  /** 加压点列表 */
  pressurePoints: number[]
  /** 已采集的数据 */
  collectedData: Record<number, number[]>
}

/** 报告合并请求 */
export interface BatchReportRequest {
  /** 所有批次数据 */
  batches: BatchGroup[]
}

/** 报告合并响应 */
export interface BatchReportResponse {
  /** 报告模板信息 */
  reportTemplate: {
    /** 报告文件名 */
    filename: string
  }
}

/**
 * 量程单位选项（用于下拉选择）
 */
export const RANGE_UNIT_OPTIONS: { value: RangeUnit; label: string }[] = [
  { value: 'MPa', label: 'MPa' },
  { value: 'kPa', label: 'kPa' },
  { value: 'bar', label: 'bar' },
  { value: 'psi', label: 'psi' }
]

/**
 * 默认量程单位
 */
export const DEFAULT_RANGE_UNIT: RangeUnit = 'MPa'

/**
 * 通道总数（固定 16）
 */
export const TOTAL_CHANNELS = 16
