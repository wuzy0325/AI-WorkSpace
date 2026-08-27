import type { SessionStateDTO, ReportTemplateDTO, CalibrationConfigDTO, PressurePointDTO, FittingResultDTO } from '@/types/calibration'
import { apiGet, apiPost, apiPut } from './client'

export async function fetchSessionState(): Promise<SessionStateDTO> {
  return apiGet<SessionStateDTO>('/sessions/state')
}

export async function triggerSessionAction(action: 'start' | 'pause' | 'resume' | 'stop'): Promise<SessionStateDTO> {
  return apiPost<SessionStateDTO>(`/sessions/${action}`)
}

export async function selectReportTemplate(points: number, mode: 'single' | 'return'): Promise<ReportTemplateDTO> {
  return apiGet<ReportTemplateDTO>(`/reports/templates/select?points=${points}&mode=${mode}`)
}

/** 导出标定报告；多设备时每台设备各生成一个文件，返回全部路径 */
export async function exportCalibrationReport(outputPath: string): Promise<string[]> {
  const resp = await apiPost<{ path: string; paths?: string[] }>('/reports/export', { outputPath })
  return resp.paths?.length ? resp.paths : [resp.path]
}

// ---------------------------------------------------------------------------
// 校准流程 API
// ---------------------------------------------------------------------------

/** 设置校准配置 */
export async function setCalibrationConfig(config: CalibrationConfigDTO): Promise<void> {
  await apiPost('/calibration/config', config)
}

/** 设置采集通道 */
export async function setCalibrationChannels(channels: number[]): Promise<number[]> {
  return (await apiPost<{ channels: number[] }>('/calibration/channels', { channels })).channels
}

/** 获取当前通道配置 */
export async function getCalibrationChannels(): Promise<number[]> {
  return (await apiGet<{ channels: number[] }>('/calibration/channels/list')).channels
}

/** 生成压力点 */
export async function generatePressurePoints(): Promise<PressurePointDTO[]> {
  return apiPost<PressurePointDTO[]>('/calibration/points/generate')
}

/** 获取压力点列表 */
export async function getPressurePoints(): Promise<PressurePointDTO[]> {
  return apiGet<PressurePointDTO[]>('/calibration/points')
}

/** 更新指定压力点的目标压力 */
export async function updatePointTargetPressure(pointIndex: number, targetPressure: number): Promise<void> {
  await apiPut(`/calibration/points/${pointIndex}/target-pressure`, { targetPressure })
}

/** 执行打压 */
export async function pressurize(pointIndex: number): Promise<void> {
  await apiPost('/calibration/pressurize', { pointIndex })
}

/** 采集数据（首个绑定设备，兼容单设备场景） */
export async function collectData(pointIndex: number): Promise<number[]> {
  return (await apiPost<{ data: number[] }>('/calibration/collect', { pointIndex })).data
}

/** 采集数据（多设备维度结果：deviceID -> 通道数据；data 为首个设备数据） */
export async function collectDataByDevice(
  pointIndex: number
): Promise<{ data: number[]; devices: Record<string, number[]> }> {
  return apiPost<{ data: number[]; devices: Record<string, number[]> }>('/calibration/collect', { pointIndex })
}

/** 永久跳过指定计量设备（从本批次剩余压力点移除） */
export async function skipCalibrationDevice(deviceId: string, reason: string): Promise<void> {
  await apiPost('/calibration/skip-device', { deviceId, reason })
}

/** 执行拟合 */
export async function fitData(): Promise<FittingResultDTO> {
  return apiPost<FittingResultDTO>('/calibration/fit')
}

/** 确认报警决策（continue/skip/recollect/stop） */
export async function resolveAlarm(decision: 'continue' | 'skip' | 'recollect' | 'stop'): Promise<void> {
  await apiPost('/calibration/resolve-alarm', { decision })
}

/** 重试指定压力点 */
export async function retryPoint(pointIndex: number): Promise<void> {
  await apiPost('/calibration/retry-point', { pointIndex })
}

// ---------------------------------------------------------------------------
// 配置持久化 API
// ---------------------------------------------------------------------------

export interface CalibrationParamsPayload {
  minPressure: number
  maxPressure: number
  pointCount: number
  precision: number
  averageCount: number
  stableDurationMs: number
  precisionLevel: number
  pressureMode: string
  controlMode: string
}

export interface AlarmConfigPayload {
  enabled: boolean
  precisionThreshold: number
  soundEnabled: boolean
  confirmOnAlarm: boolean
  enabledChannels: number[]
}

/** 获取持久化校准参数配置 */
export async function getCalibrationParamsConfig(): Promise<CalibrationParamsPayload> {
  return apiGet<CalibrationParamsPayload>('/config/calibration')
}

/** 保存校准参数配置 */
export async function saveCalibrationParamsConfig(params: CalibrationParamsPayload): Promise<void> {
  await apiPost('/config/calibration', params)
}

/** 获取持久化报警配置 */
export async function getAlarmConfig(): Promise<AlarmConfigPayload> {
  return apiGet<AlarmConfigPayload>('/config/alarm')
}

/** 保存报警配置 */
export async function saveAlarmConfig(config: AlarmConfigPayload): Promise<void> {
  await apiPost('/config/alarm', config)
}
