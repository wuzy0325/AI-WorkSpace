/**
 * 分批计量 Pinia Store
 *
 * 管理分批计量会话状态：
 *  - 会话 ID
 *  - 16 通道量程配置
 *  - 自动分组结果
 *  - 当前批次索引
 *  - 各批次状态
 *
 * 断点不保留：页面刷新或关闭后状态丢失，重新开始。
 *
 * 错误处理策略：
 *  - action 内部 catch 并记录到 error state，不再 re-throw
 *  - 调用方（composable）通过返回的 ok 标志判断成败，并负责向用户提示
 *  - 这样避免 Unhandled Promise Rejection，同时保留 UI 反馈通道
 */

import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import type { ChannelRange, BatchGroup, VerificationResult } from '@/types/batch'
import * as batchApi from '@/api/batchMeasurement'

/** 通用操作结果，便于调用方判断成败而不必依赖 try/catch。 */
export interface BatchActionResult {
  ok: boolean
  /** 失败时的提示信息（成功时为空字符串） */
  message: string
}

/** 成功结果快捷构造 */
const okResult = (): BatchActionResult => ({ ok: true, message: '' })

/** 失败结果快捷构造 */
const errResult = (e: unknown): BatchActionResult => ({
  ok: false,
  message: e instanceof Error ? e.message : String(e)
})

export const useBatchMeasurementStore = defineStore('batchMeasurement', () => {
  // ---- State ----

  /** 会话 ID（后端创建会话后返回） */
  const sessionId = ref<string>('')

  /** 16 通道量程配置 */
  const channelRanges = ref<ChannelRange[]>([])

  /** 自动分组结果 */
  const batches = ref<BatchGroup[]>([])

  /** 当前执行中的批次索引（0-based，-1 表示未开始） */
  const currentBatchIndex = ref<number>(-1)

  /** 加载状态 */
  const loading = ref<boolean>(false)

  /** 错误信息 */
  const error = ref<string>('')

  // ---- Getters ----

  /** 是否已创建会话 */
  const hasSession = computed<boolean>(() => sessionId.value !== '')

  /** 所有批次是否均已完成 */
  const allBatchesCompleted = computed<boolean>(() => {
    if (batches.value.length === 0) return false
    return batches.value.every((b) => b.status === 'completed')
  })

  /** 已完成批次数 */
  const completedCount = computed<number>(() => {
    return batches.value.filter((b) => b.status === 'completed').length
  })

  /** 当前批次（无则 null） */
  const currentBatch = computed<BatchGroup | null>(() => {
    if (currentBatchIndex.value < 0 || currentBatchIndex.value >= batches.value.length) {
      return null
    }
    return batches.value[currentBatchIndex.value]
  })

  // ---- Actions ----

  /** 设置通道量程配置（由 BatchRangeInput 触发） */
  const setChannelRanges = (ranges: ChannelRange[]): void => {
    channelRanges.value = [...ranges]
  }

  /** 设置分组结果（由 BatchGroupView 触发） */
  const setBatches = (groups: BatchGroup[]): void => {
    batches.value = groups.map((g) => ({ ...g }))
    currentBatchIndex.value = -1
  }

  /** 创建会话（提交到后端）。
   *
   * 成功时返回 okResult；失败时记录 error 并返回 errResult，不抛错。
   */
  const createSession = async (): Promise<BatchActionResult> => {
    loading.value = true
    error.value = ''
    try {
      const id = await batchApi.createBatchSession(channelRanges.value, batches.value)
      sessionId.value = id
      return okResult()
    } catch (e) {
      error.value = errResult(e).message
      return errResult(e)
    } finally {
      loading.value = false
    }
  }

  /** 核对码校验。
   *
   * 后端校验失败（valid=false）也返回 okResult，仅在调用 API 抛错时返回 errResult。
   * 调用方需检查返回的 VerificationResult.valid 字段。
   * 校验失败时的 message 通过 VerificationResult.message 传递。
   */
  const verifyBatch = async (
    batchId: string,
    code: string
  ): Promise<{ ok: boolean; result?: VerificationResult; message?: string }> => {
    loading.value = true
    error.value = ''
    try {
      const result = await batchApi.verifyBatch(sessionId.value, batchId, code)
      return { ok: true, result }
    } catch (e) {
      const message = errResult(e).message
      error.value = message
      return { ok: false, message }
    } finally {
      loading.value = false
    }
  }

  /** 启动批次（设置当前批次索引，状态由后端管理） */
  const startBatch = async (batchId: string): Promise<BatchActionResult> => {
    loading.value = true
    error.value = ''
    try {
      await batchApi.startBatch(sessionId.value, batchId)
      const idx = batches.value.findIndex((b) => b.batchId === batchId)
      if (idx >= 0) {
        batches.value[idx].status = 'running'
        currentBatchIndex.value = idx
      }
      return okResult()
    } catch (e) {
      error.value = errResult(e).message
      return errResult(e)
    } finally {
      loading.value = false
    }
  }

  /** 标记批次完成 */
  const completeBatch = async (batchId: string): Promise<BatchActionResult> => {
    loading.value = true
    error.value = ''
    try {
      await batchApi.completeBatch(sessionId.value, batchId)
      const idx = batches.value.findIndex((b) => b.batchId === batchId)
      if (idx >= 0) {
        batches.value[idx].status = 'completed'
      }
      return okResult()
    } catch (e) {
      error.value = errResult(e).message
      return errResult(e)
    } finally {
      loading.value = false
    }
  }

  /** 回退重跑批次 */
  const resetBatch = async (batchId: string): Promise<BatchActionResult> => {
    loading.value = true
    error.value = ''
    try {
      await batchApi.resetBatch(sessionId.value, batchId)
      const idx = batches.value.findIndex((b) => b.batchId === batchId)
      if (idx >= 0) {
        batches.value[idx].status = 'pending'
        batches.value[idx].collectedData = undefined
        batches.value[idx].pressurePoints = undefined
        currentBatchIndex.value = idx
      }
      return okResult()
    } catch (e) {
      error.value = errResult(e).message
      return errResult(e)
    } finally {
      loading.value = false
    }
  }

  /** 切换当前查看的批次（仅切换视图，不改变执行状态） */
  const selectBatch = (batchId: string): void => {
    const idx = batches.value.findIndex((b) => b.batchId === batchId)
    if (idx >= 0) {
      currentBatchIndex.value = idx
    }
  }

  /** 进入下一批次（当前批次完成后调用）。
   *
   * 跳过已 completed 的批次，定位到下一个 pending 批次。
   * 若没有 pending 批次，currentBatchIndex 保持不变（由上层判定 allBatchesCompleted）。
   * 这样回退重跑场景下不会对已完成批次再次弹核对码弹窗。
   */
  const moveToNextBatch = (): void => {
    for (let i = currentBatchIndex.value + 1; i < batches.value.length; i++) {
      if (batches.value[i].status === 'pending') {
        currentBatchIndex.value = i
        return
      }
    }
    // 没有 pending 批次：保持当前索引，由上层判定 allBatchesCompleted
  }

  /** 重置整个会话（清空所有状态）。
   *
   * 同时通知后端删除会话以释放内存，避免长期运行后 sessions map 持续累积。
   * 后端删除是幂等的，前端无需关心会话是否已存在。
   * 网络失败不阻塞前端状态清理：本地状态优先重置，后端残留可由后续清理。
   */
  const resetSession = async (): Promise<void> => {
    const id = sessionId.value
    sessionId.value = ''
    channelRanges.value = []
    batches.value = []
    currentBatchIndex.value = -1
    error.value = ''
    if (id) {
      try {
        await batchApi.deleteBatchSession(id)
      } catch {
        // 后端删除失败不阻塞前端重置：本地状态已清空，后端残留由后续清理或进程重启回收
      }
    }
  }

  return {
    // state
    sessionId,
    channelRanges,
    batches,
    currentBatchIndex,
    loading,
    error,
    // getters
    hasSession,
    allBatchesCompleted,
    completedCount,
    currentBatch,
    // actions
    setChannelRanges,
    setBatches,
    createSession,
    verifyBatch,
    startBatch,
    completeBatch,
    resetBatch,
    selectBatch,
    moveToNextBatch,
    resetSession
  }
})
