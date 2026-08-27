/**
 * 分批计量流程编排 Composable
 *
 * 协调量程录入 → 自动分组 → 核对码确认 → 逐批加压 → 合并报告 的完整流程。
 * 作为 UI 组件与 store 之间的中介，封装跨组件状态流转。
 *
 * 错误处理策略：
 *  - 调用 store action 后检查返回的 ok 标志
 *  - 失败时通过 ElMessage.error 向用户提示，避免 Unhandled Promise Rejection
 *  - 核对码校验失败时通过 dialogRef.value.setError() 在弹窗内联显示错误
 */

import { ref, computed, reactive, shallowRef } from 'vue'
import { ElMessage } from 'element-plus'
import { useBatchMeasurementStore } from '@/stores/batchMeasurement'
import { generateBatchReport } from '@/api/batchMeasurement'
import type { ChannelRange, BatchGroup, VerificationResult } from '@/types/batch'
// 注：.vue 默认导出是组件实例类型，这里用 default import 拿到组件类型，
// 再用 InstanceType<typeof> 获取组件实例类型用于 ref。
import BatchVerificationDialog from '@/components/measurement/BatchVerificationDialog.vue'

/** 流程阶段 */
export type BatchPhase =
  | 'range-input'    // 量程录入
  | 'group-view'     // 分组确认
  | 'batch-running'  // 批次执行中
  | 'completed'      // 全部完成
  | 'report'         // 报告预览

/** 报告数据结构 */
export interface BatchReportData {
  filename: string
  batches: BatchGroup[]
}

export function useBatchMeasurement() {
  const store = useBatchMeasurementStore()

  /** 当前流程阶段 */
  const phase = ref<BatchPhase>('range-input')

  /** 核对码弹窗是否显示 */
  const verificationDialogVisible = ref<boolean>(false)

  /** 报告生成结果 */
  const reportData = ref<BatchReportData | null>(null)

  /**
   * 核对码弹窗组件引用。
   * 用 shallowRef 避免 Vue 对组件实例做深度响应式代理。
   * 校验失败时通过此引用调用 dialog.setError() 在弹窗内联显示错误。
   */
  const dialogRef = shallowRef<InstanceType<typeof BatchVerificationDialog> | null>(null)

  // ---- Getters ----

  /** 当前批次（用于核对码弹窗显示量程值）。
   *
   * 仅在 batch-running 阶段返回当前批次；其它阶段返回 null。
   * 配合 MeasurementView 的 v-if 守卫，弹窗仅在 batch-running 阶段显示。
   */
  const pendingVerificationBatch = computed<BatchGroup | null>(() => {
    if (phase.value !== 'batch-running') return null
    return store.currentBatch
  })

  /** 是否所有批次完成 */
  const isAllCompleted = computed<boolean>(() => store.allBatchesCompleted)

  // ---- Actions ----

  /** 量程录入确认（BatchRangeInput → 父组件调用） */
  const handleRangeConfirm = (ranges: ChannelRange[]): void => {
    store.setChannelRanges(ranges)
    phase.value = 'group-view'
  }

  /** 分组确认（BatchGroupView → 父组件调用）。
   *
   * 流程：setBatches → createSession → 切换到 batch-running → 弹出第一批核对码。
   * 注意：setBatches 会把 currentBatchIndex 置为 -1，需在 createSession 后
   * 显式 selectBatch(batches[0].batchId) 把 currentBatchIndex 指向第一批，
   * 避免弹窗依赖 batchStore.batches[0] fallback。
   */
  const handleGroupConfirm = async (batches: BatchGroup[]): Promise<void> => {
    store.setBatches(batches)
    const result = await store.createSession()
    if (!result.ok) {
      ElMessage.error(`创建会话失败：${result.message}`)
      return
    }
    // 显式指向第一批，使 pendingVerificationBatch 非 null
    if (batches.length > 0) {
      store.selectBatch(batches[0].batchId)
    }
    phase.value = 'batch-running'
    // 自动进入第一个批次的核对码弹窗
    verificationDialogVisible.value = true
  }

  /** 核对码校验（BatchVerificationDialog → 父组件调用）。
   *
   * 调用 store.verifyBatch：
   *  - API 抛错（网络/认证等）→ ElMessage 提示，弹窗保持开启
   *  - 后端校验失败（valid=false）→ 通过 dialogRef.setError 在弹窗内联显示
   *  - 校验通过 → 关闭弹窗，自动 startBatch
   *
   * 返回 VerificationResult 便于调用方做额外判断。
   */
  const handleVerification = async (batchId: string, code: string): Promise<VerificationResult | null> => {
    const { ok, result, message } = await store.verifyBatch(batchId, code)
    if (!ok || !result) {
      ElMessage.error(`核对码校验失败：${message ?? '未知错误'}`)
      return null
    }
    if (!result.valid) {
      // 后端校验失败：在弹窗内联显示错误，弹窗保持开启
      dialogRef.value?.setError(result.message)
      return result
    }
    // 校验通过：关闭弹窗，自动启动该批次
    verificationDialogVisible.value = false
    const startResult = await store.startBatch(batchId)
    if (!startResult.ok) {
      ElMessage.error(`启动批次失败：${startResult.message}`)
      return result
    }
    return result
  }

  /** 核对码弹窗取消 */
  const handleVerificationCancel = (): void => {
    verificationDialogVisible.value = false
  }

  /** 当前批次完成（由加压流程触发，通常由 MeasurementView 监听 state 变化调用）。
   *
   * 流程：
   *  1. 调用后端 completeBatch 标记批次完成
   *  2. 若所有批次完成 → 切换到 completed 阶段
   *  3. 否则 moveToNextBatch 定位到下一个 pending 批次，弹出核对码弹窗
   */
  const handleBatchComplete = async (batchId: string): Promise<void> => {
    const result = await store.completeBatch(batchId)
    if (!result.ok) {
      ElMessage.error(`标记批次完成失败：${result.message}`)
      return
    }

    if (store.allBatchesCompleted) {
      phase.value = 'completed'
    } else {
      // 进入下一批次，显示核对码弹窗
      store.moveToNextBatch()
      verificationDialogVisible.value = true
    }
  }

  /** 回退重跑批次（BatchProgressBar → 父组件调用）。
   *
   * 流程：
   *  1. 调用后端 resetBatch 清空采集数据
   *  2. 切换 phase 回 batch-running（避免 UI 卡在 completed 阶段）
   *  3. 弹出核对码弹窗
   */
  const handleBatchReset = async (batchId: string): Promise<void> => {
    const result = await store.resetBatch(batchId)
    if (!result.ok) {
      ElMessage.error(`回退重跑失败：${result.message}`)
      return
    }
    // 关键：phase 必须切回 batch-running，否则 UI 仍渲染完成面板
    phase.value = 'batch-running'
    verificationDialogVisible.value = true
  }

  /** 切换查看批次（BatchProgressBar → 父组件调用） */
  const handleBatchSelect = (batchId: string): void => {
    store.selectBatch(batchId)
  }

  /** 生成合并报告 */
  const handleGenerateReport = async (points: number, mode: string): Promise<void> => {
    try {
      const response = await generateBatchReport(store.batches, points, mode)
      reportData.value = {
        filename: response.reportTemplate.filename,
        batches: response.batches
      }
      phase.value = 'report'
    } catch (e) {
      const msg = e instanceof Error ? e.message : String(e)
      ElMessage.error(`生成报告失败：${msg}`)
    }
  }

  /** 设置核对码弹窗组件引用（由父组件的 template ref 同步传入）。 */
  const setDialogRef = (inst: InstanceType<typeof BatchVerificationDialog> | null): void => {
    dialogRef.value = inst
  }

  /** 重置整个流程。
   *
   * await store.resetSession 确保后端会话也同步删除，避免内存累积。
   */
  const handleReset = async (): Promise<void> => {
    await store.resetSession()
    phase.value = 'range-input'
    verificationDialogVisible.value = false
    reportData.value = null
  }

  // 使用 reactive 包裹，使内部 ref 在模板中自动解包
  // 这样 batch.phase / batch.verificationDialogVisible 可直接访问，无需 .value
  return reactive({
    // state
    phase,
    verificationDialogVisible,
    reportData,
    // getters
    pendingVerificationBatch,
    isAllCompleted,
    // actions
    handleRangeConfirm,
    handleGroupConfirm,
    handleVerification,
    handleVerificationCancel,
    handleBatchComplete,
    handleBatchReset,
    handleBatchSelect,
    handleGenerateReport,
    handleReset,
    setDialogRef
  })
}