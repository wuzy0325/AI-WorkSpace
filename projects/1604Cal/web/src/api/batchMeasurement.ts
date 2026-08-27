/**
 * 分批计量 API 封装
 *
 * 对接后端 /api/v1/batch/* 端点，提供：
 *  - 创建会话
 *  - 核对码校验
 *  - 批次启动/完成/重置
 *  - 查询会话状态
 *  - 合并报告
 */

import { apiGet, apiPost, apiDelete } from './client'
import type {
  ChannelRange,
  BatchGroup,
  VerificationResult,
  BatchSession
} from '@/types/batch'

/** 创建会话请求体 */
interface CreateSessionRequest {
  channelRanges: ChannelRange[]
  batches: BatchGroup[]
}

/** 创建会话响应 */
interface CreateSessionResponse {
  sessionId: string
}

/** 报告合并请求 */
interface BatchReportRequest {
  batches: BatchGroup[]
  points: number
  mode: string
}

/** 报告合并响应 */
interface BatchReportResponse {
  reportTemplate: { filename: string }
  batches: BatchGroup[]
}

/** 创建分批计量会话 */
export async function createBatchSession(
  channelRanges: ChannelRange[],
  batches: BatchGroup[]
): Promise<string> {
  const result = await apiPost<CreateSessionResponse>(
    '/batch/sessions',
    { channelRanges, batches } as CreateSessionRequest
  )
  return result.sessionId
}

/** 查询分批计量会话状态 */
export async function getBatchSession(sessionId: string): Promise<BatchSession> {
  return apiGet<BatchSession>(`/batch/sessions/${sessionId}`)
}

/** 删除分批计量会话（释放后端内存，幂等：删除不存在的会话也成功） */
export async function deleteBatchSession(sessionId: string): Promise<void> {
  await apiDelete<void>(`/batch/sessions/${sessionId}`)
}

/** 核对码校验 */
export async function verifyBatch(
  sessionId: string,
  batchId: string,
  verificationCode: string
): Promise<VerificationResult> {
  return apiPost<VerificationResult>(
    `/batch/sessions/${sessionId}/batches/${batchId}/verify`,
    { verificationCode }
  )
}

/** 启动批次（前置条件：已通过核对码校验） */
export async function startBatch(sessionId: string, batchId: string): Promise<void> {
  await apiPost(`/batch/sessions/${sessionId}/batches/${batchId}/start`)
}

/** 标记批次完成 */
export async function completeBatch(sessionId: string, batchId: string): Promise<void> {
  await apiPost(`/batch/sessions/${sessionId}/batches/${batchId}/complete`)
}

/** 回退重跑批次（清空采集数据，需重新校验） */
export async function resetBatch(sessionId: string, batchId: string): Promise<void> {
  await apiPost(`/batch/sessions/${sessionId}/batches/${batchId}/reset`)
}

/** 生成合并报告（一期复用现有标定报告模板） */
export async function generateBatchReport(
  batches: BatchGroup[],
  points: number,
  mode: string
): Promise<BatchReportResponse> {
  return apiPost<BatchReportResponse>('/batch/report', {
    batches,
    points,
    mode
  } as BatchReportRequest)
}
