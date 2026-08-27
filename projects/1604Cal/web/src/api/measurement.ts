import { apiGet, apiPost } from './client'
import type { DevicePointDataDTO } from '@/types/calibration'

export interface CollectedRow {
  timestamp: string
  channels: Record<string, number>
}

export interface MeasurementDataResponse {
  rows: CollectedRow[]
  total: number
}

export interface MeasurementParamsPayload {
  minPressure: number
  maxPressure: number
  pointCount: number
  precision: number
  averageCount: number
  stableDurationMs: number
  precisionLevel: number
  pressureMode: string
  controlMode: string
  customPoints?: number[]
}

/** 获取计量状态 */
export async function fetchMeasurementState(): Promise<string> {
  return (await apiGet<{ state: string }>('/measurement/state')).state
}

/** 启动计量采集 */
export async function startMeasurement(channels: number[]): Promise<string> {
  return (await apiPost<{ state: string }>('/measurement/start', { channels })).state
}

/** 暂停计量采集 */
export async function pauseMeasurement(): Promise<string> {
  return (await apiPost<{ state: string }>('/measurement/pause')).state
}

/** 停止计量采集 */
export async function stopMeasurement(): Promise<string> {
  return (await apiPost<{ state: string }>('/measurement/stop')).state
}

/** 获取采集数据 */
export async function fetchMeasurementData(): Promise<MeasurementDataResponse> {
  return apiGet<MeasurementDataResponse>('/measurement/data')
}

/** 导出 xlsx 报告（模板方式）；多设备时每台设备各生成一个文件，返回全部路径 */
export async function exportMeasurementReport(outputPath: string): Promise<string[]> {
  const resp = await apiPost<{ path: string; paths?: string[] }>('/measurement/export', { outputPath })
  return resp.paths?.length ? resp.paths : [resp.path]
}

/** 获取计量模块参数配置 */
export async function getMeasurementParamsConfig(): Promise<MeasurementParamsPayload> {
  return apiGet<MeasurementParamsPayload>('/config/measurement')
}

/** 保存计量模块参数配置 */
export async function saveMeasurementParamsConfig(params: MeasurementParamsPayload): Promise<void> {
  await apiPost('/config/measurement', params)
}

export interface MeasurementPoint {
  id: string
  index: number
  targetPressure: number
  direction: string
  status: string
  collectedData?: number[]
  collectedByDevice?: Record<string, DevicePointDataDTO>
  actualPressure?: number
  collectTime?: string
  errorMessage?: string
}

export interface MeasurementAlarmConfig {
  enabled: boolean
  enabledChannels: number[]
  confirmOnAlarm: boolean
  soundEnabled: boolean
}

export interface MeasurementAlarm {
  pointId: string
  /** 触发报警的计量设备（多设备场景定位故障设备；单设备可能为空） */
  deviceId?: string
  targetPressure: number
  actualPressure: number
  threshold: number
  maxDeviation: number
  overLimitChannels: number[]
}

/** 生成压力点 */
export async function generateMeasurementPoints(): Promise<MeasurementPoint[]> {
  return apiPost<MeasurementPoint[]>('/measurement/points/generate')
}

/** 获取压力点列表 */
export async function fetchMeasurementPoints(): Promise<MeasurementPoint[]> {
  return apiGet<MeasurementPoint[]>('/measurement/points')
}

/** 获取计量报警配置 */
export async function getMeasurementAlarmConfig(): Promise<MeasurementAlarmConfig> {
  return apiGet<MeasurementAlarmConfig>('/config/measurement-alarm')
}

/** 保存计量报警配置 */
export async function saveMeasurementAlarmConfig(config: MeasurementAlarmConfig): Promise<void> {
  await apiPost('/config/measurement-alarm', config)
}

/** 检查是否有待处理的报警（挂起时附带报警详情，页面刷新后恢复弹窗用） */
export async function checkMeasurementAlarmPending(): Promise<{ pending: boolean; alarm?: MeasurementAlarm | null }> {
  return apiGet<{ pending: boolean; alarm?: MeasurementAlarm | null }>('/measurement/alarm/pending')
}

/** 查询稳定超时是否挂起（后端阻塞等待决策时，页面刷新恢复弹窗用） */
export async function fetchStabilityTimeoutPending(): Promise<{ pending: boolean; pointIndex: number }> {
  return apiGet<{ pending: boolean; pointIndex: number }>('/measurement/stability-timeout/pending')
}

/** 报警决策：与后端 workflow.AlarmDecision* 常量保持一致 */
export type AlarmDecision = 'continue' | 'recollect' | 'skip' | 'stop'

/** 处理报警 */
export async function resolveMeasurementAlarm(decision: AlarmDecision): Promise<void> {
  await apiPost('/measurement/alarm/resolve', { decision })
}

/** 永久跳过指定计量设备（从本批次剩余压力点移除） */
export async function skipMeasurementDevice(deviceId: string, reason: string): Promise<void> {
  await apiPost('/measurement/skip-device', { deviceId, reason })
}

/** 自动按点采集（逐点打压→稳定→采集） */
export async function autoCollectMeasurement(): Promise<string> {
  return (await apiPost<{ state: string }>('/measurement/auto-collect')).state
}

/** 手动模式启动工作流（仅进入 ready 状态，不启动实时采样） */
export async function manualStartMeasurement(channels: number[]): Promise<string> {
  return (await apiPost<{ state: string }>('/measurement/manual-start', { channels })).state
}

/** 手动打压指定测点 */
export async function manualPressurizeMeasurement(pointIndex: number): Promise<string> {
  return (await apiPost<{ state: string }>('/measurement/manual-pressurize', { pointIndex })).state
}

/** 手动采集指定测点 */
export async function manualCollectMeasurement(pointIndex: number): Promise<string> {
  return (await apiPost<{ state: string }>('/measurement/manual-collect', { pointIndex })).state
}

/** 稳定超时决定：继续等待或跳过当前点 */
export async function resolveStabilityTimeout(decision: 'continue' | 'skip'): Promise<void> {
  await apiPost('/measurement/stability-timeout/resolve', { decision })
}
