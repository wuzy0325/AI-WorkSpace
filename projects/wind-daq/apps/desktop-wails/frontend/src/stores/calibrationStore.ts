import { defineStore } from 'pinia'
import { ref, computed, watch } from 'vue'
import type {
  CalibrationConfig,
  CalibrationType,
  CalibrationTaskStatus,
  CalibrationCompleteEvent,
  CalibrationDataPoint,
  CalibrationAnyDataPoint,
  FiveHoleCoefficients,
} from '@shared/types/calibration'
import { calibrationApi } from '@api/calibrationApi'
import { wailsApi, isWailsAvailable } from '@api/wails-adapter'

export interface RealtimePressures {
  P1: number
  P2: number
  P3: number
  P4: number
  P5: number
  Patm: number
  Tatm: number
  P0?: number
  Ps?: number
  Ttunnel?: number
  // 总压探针核心通道：探针总压不参与 Ma/V 计算（仅风洞总压/静压参与），
  // 但需在侧边栏显示，单独字段供 TotalPressureMain 读取，与三孔从 store 读通道值的模式一致。
  PprobeTotal?: number
}

export interface CalculatedPhysics {
  machNumber?: number
  velocity?: number
}

// 大气数据计算常数（与后端 AtmosphericDataCalculator 保持一致）
const ATM_GAMMA = 1.4       // 空气绝热指数
const ATM_C_COEFF = 20.047  // 声速计算系数
const ATM_RECOVERY = 0.9    // 温度传感器恢复系数

/**
 * 根据实时压力计算气动参数（马赫数、流速）。
 * 公式与后端 AtmosphericDataCalculator / formulas.go 一致，用于 UI 实时显示，非校准算法。
 *
 * 关键：风洞总压/静压通道通常以大气压为参考点输出差压（表压），
 *   后端公式约定 Pt_abs = P0 + Patm, Ps_abs = Ps + Patm。
 *   若 Patm 缺失或为 0，实时 UI 使用标准大气压兜底，避免差压通道导致整块空白。
 *
 * 马赫数: Ma = sqrt((2/(γ-1)) * ((Pt_abs/Ps_abs)^((γ-1)/γ) - 1))
 * 静温:   SAT = TAT / (1 + 0.2 * r * Ma^2)   （TAT 取风洞温度，需转开尔文）
 * 流速:   V = Ma * 20.047 * sqrt(SAT)
 *
 * 当 P0/Ps/Ttunnel 任一缺失或非法时返回 null（UI 显示 "--"）。
 */
function calculateAtmosphericPhysics(p: RealtimePressures): CalculatedPhysics | null {
  const ptGauge = p.P0
  const psGauge = p.Ps
  // 风洞温度通道单位为 ℃，需转换为开尔文
  const tatK = p.Ttunnel === undefined ? undefined : p.Ttunnel + 273.15
  // 大气压：与后端 formulas.go 口径一致——Patm 缺失或 <=0 时不计算 Ma/V，
  // 返回 null 让 UI 显示 "--"、CSV 写空。避免前端兜底标准大气压而后端置 nil
  // 导致"UI 显示数值、CSV 对应列为空"的不一致（§22: pAtm 为必需通道）。
  const patm = p.Patm

  if (ptGauge === undefined || psGauge === undefined || tatK === undefined || patm === undefined) return null
  if (!Number.isFinite(ptGauge) || !Number.isFinite(psGauge) || !Number.isFinite(tatK) || !Number.isFinite(patm)) return null
  if (patm <= 0) return null

  const ptAbs = ptGauge + patm
  const psAbs = psGauge + patm

  if (psAbs <= 0 || ptAbs < psAbs || tatK <= 0) return null
  if (ptAbs === psAbs) return { machNumber: 0, velocity: 0 }

  const ratio = ptAbs / psAbs
  const ma = Math.sqrt((2 / (ATM_GAMMA - 1)) * (Math.pow(ratio, (ATM_GAMMA - 1) / ATM_GAMMA) - 1))
  if (!Number.isFinite(ma) || ma < 0) return null

  const sat = tatK / (1 + ((ATM_GAMMA - 1) / 2) * ATM_RECOVERY * ma * ma)
  if (!Number.isFinite(sat) || sat <= 0) return null

  const velocity = ma * ATM_C_COEFF * Math.sqrt(sat)
  if (!Number.isFinite(velocity)) return null

  return { machNumber: ma, velocity }
}

export interface TimeInfo {
  elapsedTime: number
  estimatedRemaining: number
}

export const useCalibrationStore = defineStore('calibration', () => {
  const status = ref<CalibrationTaskStatus | null>(null)
  const isRunning = ref(false)
  const isPaused = ref(false)
  const completeEvent = ref<CalibrationCompleteEvent | null>(null)
  const dataPoints = ref<CalibrationAnyDataPoint[]>([])
  const realtimePressures = ref<RealtimePressures | null>(null)
  const calculatedPhysics = ref<CalculatedPhysics | null>(null)
  const timeInfo = ref<TimeInfo | null>(null)
  // 默认 5Hz：兼顾实时性与性能。CalibrationConfig.uiRefreshHz 在 startCalibration 中同步覆盖。
  const uiRefreshHz = ref(5)
  const uiRefreshIntervalMs = computed(() => 1000 / uiRefreshHz.value)

  // 状态轮询定时器：必须放在 store 内部（而非模块级），
  // 否则多实例 / HMR 重载时旧 timer 不会随 store dispose 而清理，导致泄漏与重复轮询。
  let statusPollingTimer: ReturnType<typeof setInterval> | null = null

  // 画面活跃性计数：驱动 polling 频率切换（spec I4）
  // - count > 0：至少一个校准 Main 可见，polling 用 uiRefreshHz（默认 5Hz）
  // - count == 0：全部切走，polling 降到 1Hz 心跳，让"切走期间完成/失败/停止"仍能被前端捕获
  // acquire/release 仅控制频率，不清理会话状态（status/dataPoints/completeEvent），参见 I1/I3
  const activeViewCount = ref(0)

  // 恢复中状态：recoveryFromBackend 期间为 true，UI 显示 loading 占位（spec I8 / Recovery UX）
  const isRecovering = ref(false)
  // 恢复失败原因：null 表示无错误。失败时保留旧 store 状态，UI 显示错误条 + 可重试
  const recoveryError = ref<string | null>(null)
  // 上次 recovery 完成时刻（ms 时间戳）：调用方用于"2s 内跳过二次 recovery"判定（spec Decision #14）
  const lastRecoveryAt = ref(0)

  // 已用时 tick 定时器：固定 1Hz，独立于 statusPolling 频率。
  // 已用时显示精度到秒，1Hz 足够；不必跟随 uiRefreshHz（5Hz~60Hz）浪费主线程。
  let elapsedTickTimer: ReturnType<typeof setInterval> | null = null

  // 暂停累计时长（ms）：用于从总 elapsed 中扣除暂停段，让"已用时"在暂停期间真正停住。
  // lastPauseAt 记录本次暂停开始时刻，resume 时累加到 pausedAccumulatedMs。
  // 这两个字段不进 ref：不需要响应式，只在 tick 计算时读取。
  let pausedAccumulatedMs = 0
  let lastPauseAt: number | null = null

  // 压力数据节流控制
  let lastPressureUpdateAt = 0
  let pendingPressureUpdate: RealtimePressures | null = null
  let pressureThrottleTimer: ReturnType<typeof setTimeout> | null = null

  function cancelPressureThrottle(): void {
    if (pressureThrottleTimer !== null) {
      clearTimeout(pressureThrottleTimer)
      pressureThrottleTimer = null
    }
    pendingPressureUpdate = null
  }

  function flushPendingPressureIfReady(): void {
    if (pendingPressureUpdate === null) return

    if (pressureThrottleTimer !== null) {
      clearTimeout(pressureThrottleTimer)
      pressureThrottleTimer = null
    }

    const now = Date.now()
    const intervalMs = Math.round(uiRefreshIntervalMs.value)

    const applyPressureUpdate = (pressures: RealtimePressures) => {
      realtimePressures.value = pressures
      // 同步计算气动参数（马赫数、流速），公式与后端 AtmosphericDataCalculator 一致：
      //   Ma = sqrt((2/(γ-1)) * ((Pt/Ps)^((γ-1)/γ) - 1))
      //   SAT = TAT / (1 + 0.2 * r * Ma^2)   （TAT 取风洞温度，开尔文）
      //   V   = Ma * 20.047 * sqrt(SAT)
      // 这是实时显示用的标准大气数据计算，非校准算法。
      calculatedPhysics.value = calculateAtmosphericPhysics(pressures)
    }

    if (now - lastPressureUpdateAt >= intervalMs) {
      applyPressureUpdate(pendingPressureUpdate)
      pendingPressureUpdate = null
      lastPressureUpdateAt = now
    } else {
      const delay = intervalMs - (now - lastPressureUpdateAt)
      pressureThrottleTimer = setTimeout(() => {
        if (pendingPressureUpdate !== null) {
          applyPressureUpdate(pendingPressureUpdate)
          pendingPressureUpdate = null
        }
        lastPressureUpdateAt = Date.now()
        pressureThrottleTimer = null
      }, delay)
    }
  }

  function setUiRefreshHz(hz: number) {
    uiRefreshHz.value = Math.max(1, Math.min(60, hz))
  }

  function updateRealtimePressures(pressures: RealtimePressures) {
    pendingPressureUpdate = pressures
    flushPendingPressureIfReady()
  }

  // 开始轮询校准状态
  // 参数 intervalMs：显式指定轮询间隔（用于 acquire/release 切换频率）。
  //   默认 undefined 时按 activeViewCount 自动选择：count>0 用 uiRefreshHz，count==0 用 1Hz 心跳。
  //   spec I4：禁止并发写竞态，调用方需先 stopStatusPolling() 再 start。
  function startStatusPolling(intervalMs?: number) {
    stopStatusPolling()
    const interval = intervalMs ?? (activeViewCount.value > 0 ? uiRefreshIntervalMs.value : 1000)
    statusPollingTimer = setInterval(async () => {
      if (!isWailsAvailable()) return
      try {
        const calStatus = await wailsApi.calibration.status()
        if (calStatus) {
          // 更新本地状态
          updateStatusFromBackend(calStatus)
        }
      } catch (err) {
        console.error('轮询校准状态失败:', err)
      }
    }, Math.round(interval))
  }

  // 停止轮询
  function stopStatusPolling() {
    if (statusPollingTimer) {
      clearInterval(statusPollingTimer)
      statusPollingTimer = null
    }
  }

  // 启动已用时 tick：每秒刷新 timeInfo，让 UI "已用时"控件真正动起来。
  // 与 statusPolling 同启停，但暂停时 tick 仍保留运行（只是 updateTimeInfo 内部会跳过 elapsed 累加），
  // 这样恢复运行后无需重启 timer，elapsed 立即恢复递增。
  function startElapsedTick() {
    stopElapsedTick()
    elapsedTickTimer = setInterval(() => {
      updateTimeInfo()
    }, 1000)
  }

  function stopElapsedTick() {
    if (elapsedTickTimer) {
      clearInterval(elapsedTickTimer)
      elapsedTickTimer = null
    }
  }

  // 画面活跃性 acquire：引用计数+1。从 0→1 时升频（spec I4 / Recovery UX）：
  //   - 仅 running/paused 态需要高频 polling + elapsedTick（终态/idle 无需轮询）
  //   - acquire 不动 status/dataPoints/completeEvent，仅控制 polling 频率
  function acquireView() {
    const wasZero = activeViewCount.value === 0
    activeViewCount.value++
    if (wasZero && (isRunning.value || isPaused.value)) {
      if (isWailsAvailable()) {
        startStatusPolling(Math.round(uiRefreshIntervalMs.value))
      }
      startElapsedTick()
    }
  }

  // 画面活跃性 release：引用计数-1（下限 0）。1→0 时降频（spec I4 / I6）：
  //   - polling 降到 1Hz 心跳（仅 running/paused 态保留，捕获切走期间终态写入 store）
  //   - elapsedTick 停止（无画面无需刷新时间显示）
  //   - 不清空会话状态（spec I1/I3/I7）
  function releaseView() {
    if (activeViewCount.value > 0) {
      activeViewCount.value--
    }
    if (activeViewCount.value === 0) {
      if ((isRunning.value || isPaused.value) && isWailsAvailable()) {
        startStatusPolling(1000)
      }
      stopElapsedTick()
    }
  }

  // recovery 完成后由调用方调用，依当前 store 状态重启 polling（spec Recovery UX 第 5 步）：
  //   - running/paused && activeViewCount > 0：高频 uiRefreshHz（至少一个 Main 可见）
  //   - running/paused && activeViewCount == 0：1Hz 心跳（全部切走，捕获终态写入）
  //   - 其他（终态/idle）：不动——updateStatusFromBackend 终态分支已 stop polling
  // 为什么需要这个：recoveryFromBackend 内部 stopStatusPolling 防竞态但不重启，
  //   调用方必须显式依后端返回状态设频，避免 polling 一直停着。
  function restartPollingForCurrentState() {
    if (!isRunning.value && !isPaused.value) return
    if (!isWailsAvailable()) return
    const interval = activeViewCount.value > 0 ? uiRefreshIntervalMs.value : 1000
    startStatusPolling(Math.round(interval))
  }

  // 基于 status.startTime + pausedAccumulatedMs 计算 elapsedTime 与 estimatedRemaining。
  // 暂停态下直接跳过更新（保留上次 timeInfo），让"已用时"在暂停期间冻结。
  function updateTimeInfo() {
    const startTime = status.value?.startTime
    if (!startTime || startTime <= 0) return
    // 暂停期间不更新 elapsed，避免 UI 数字继续前进
    if (lastPauseAt !== null) return

    const now = Date.now()
    const elapsedTime = Math.max(0, now - startTime - pausedAccumulatedMs)

    // 估算剩余时间：用已完成的平均单点耗时 × 剩余点数。
    // 仅在已完成点数 > 0 且总点数 > 已完成点数时有效，否则置 0 让 UI 显示 "--:--"。
    const completed = status.value?.completedPoints ?? 0
    const total = status.value?.totalPoints ?? 0
    let estimatedRemaining = 0
    if (completed > 0 && elapsedTime > 0 && total > completed) {
      const avgPerPoint = elapsedTime / completed
      estimatedRemaining = Math.round(avgPerPoint * (total - completed))
    }

    timeInfo.value = { elapsedTime, estimatedRemaining }
  }

  // 从后端状态更新本地状态
  function updateStatusFromBackend(calStatus: any) {
    const state = calStatus.state ?? calStatus.State
    const taskId = calStatus.taskId ?? calStatus.TaskID ?? calStatus.TaskId ?? ''
    const type = calStatus.type ?? calStatus.Type ?? 'five-hole'
    const totalPoints = calStatus.totalPoints ?? calStatus.TotalPoints ?? 0
    // completedPoints 只读后端 completedPoints 字段，不再回退到 currentPoint：
    // 后端 CurrentPoint 语义已改为"当前正在处理的点索引"（currentPointIdx，循环顶部推进），
    // 不再等于 CompletedPoints。回退会导致 completedPoints 多 1，进度条/百分比计算错误。
    const completedPoints = calStatus.completedPoints ?? calStatus.CompletedPoints ?? 0
    // currentPointIndex：后端 autoEngine.GetCurrentPointIndex()，循环顶部推进（早于 moveToPoint）。
    // 前端 progressInfo 优先用此索引查 config.points 得到"目标点"，让目标角度先于实际角度变化。
    // 后端 autoEngine 为 nil（未启动/总温手动模式）时缺失，progressInfo 回退到 completedPoints。
    const currentPointIndex = calStatus.currentPoint
    const backendDataPoints = calStatus.dataPoints ?? calStatus.DataPoints
    const progress = calStatus.progress ?? calStatus.Progress ?? (totalPoints > 0 ? (completedPoints / totalPoints) * 100 : 0)
    // 读取后端返回的启动时间戳（calibration.Status.StartTime，JSON 字段 startTime）。
    // 这是 elapsed 计时的基准——比前端本地记 Date.now() 更准：用户切换页面/刷新后仍能基于后端真实启动时刻恢复已用时。
    const startTime = calStatus.startTime ?? calStatus.StartTime
    const backendPausedDurationMs = calStatus.pausedDurationMs ?? calStatus.PausedDurationMs
    // 运动安全错误码 + 故障现场快照（与 core/calibration.Status.LastErrorCode /
    // MotionSafetyFailure 对齐）。Wails binding 重新生成前 PascalCase 字段缺失，需做 fallback。
    // 这两个字段决定前端告警卡片是否渲染、Start 是否阻塞——必须在每次轮询时刷新，
    // 否则故障恢复后卡片不会自动消失（后端在故障恢复时会清空 MotionSafetyFailure）。
    const lastErrorCode = calStatus.lastErrorCode ?? calStatus.LastErrorCode ?? undefined
    const motionSafetyFailure = calStatus.motionSafetyFailure ?? calStatus.MotionSafetyFailure ?? null
    const mappedState = mapCalibrationState(state)

    if (!status.value) {
      // 初始化状态
      status.value = {
        taskId,
        type,
        status: mappedState,
        totalPoints,
        completedPoints,
        progress,
        startTime: typeof startTime === 'number' && startTime > 0 ? startTime : undefined,
        dataPoints: Array.isArray(backendDataPoints) ? backendDataPoints : [],
        currentSample: calStatus.currentSample ?? calStatus.CurrentSample ?? 0,
        samplesPerPoint: calStatus.samplesPerPoint ?? calStatus.SamplesPerPoint ?? 0,
        currentPointIndex: typeof currentPointIndex === 'number' ? currentPointIndex : undefined,
        lastErrorCode,
        motionSafetyFailure,
      }
    } else {
      status.value.status = mappedState
      status.value.completedPoints = completedPoints
      status.value.totalPoints = totalPoints
      status.value.progress = progress
      status.value.currentPointIndex = typeof currentPointIndex === 'number' ? currentPointIndex : undefined
      // 任务身份同步：每次后端快照都更新 taskId 和 type。
      //   场景：任务由另一个窗口、HTTP 客户端或后端生命周期替换（stop+start 新任务）时，
      //   前端 store 仍持有旧 taskId/type，会继续用旧任务 ID 停止/导出，并可能打开错误类型的 Main。
      //   后端快照是身份真值来源，必须每次覆盖本地。
      status.value.taskId = taskId
      status.value.type = type
      // 后端在 Start 时才写入 StartTime；首次轮询可能还没拿到，需要每次都尝试补齐
      if (typeof startTime === 'number' && startTime > 0) {
        status.value.startTime = startTime
      }
      if (Array.isArray(backendDataPoints)) {
        status.value.dataPoints = backendDataPoints
      }
      // 采样进度透传：仅 Running 态需要实时采样进度，非 Running 态强制清零避免残留旧值
      if (mappedState === 'running' || state === 'running') {
        status.value.currentSample = calStatus.currentSample ?? calStatus.CurrentSample ?? 0
        status.value.samplesPerPoint = calStatus.samplesPerPoint ?? calStatus.SamplesPerPoint ?? 0
      } else {
        status.value.currentSample = 0
        status.value.samplesPerPoint = 0
      }
      // 运动安全错误码 + 故障现场快照每次轮询刷新：
      //   - 故障发生时后端写入，前端展示告警卡片
      //   - 故障恢复后后端清空（MotionSafetyFailure = nil），前端卡片自动消失
      // 不做"只在 error 态保留"的特殊处理，因为后端在 paused/stopped 态也可能保留快照供操作员复盘
      status.value.lastErrorCode = lastErrorCode
      status.value.motionSafetyFailure = motionSafetyFailure
    }

    if (Array.isArray(backendDataPoints)) {
      dataPoints.value = backendDataPoints
    }

    // 更新运行状态
    isRunning.value = state === 'running'
    isPaused.value = state === 'paused'

    if (typeof backendPausedDurationMs === 'number' && backendPausedDurationMs >= 0) {
      pausedAccumulatedMs = backendPausedDurationMs
    }

    // 运行/暂停态都需要 tick：暂停时 tick 内部会跳过 elapsed 累加，但保留 timer
    // 以便 resume 后立即恢复递增，无需重启 timer。
    if (state === 'running' || state === 'paused') {
      if (status.value?.startTime) {
        if (state === 'running' && lastPauseAt !== null) {
          // 后端从 paused 切回 running，但本地 lastPauseAt 未清——说明 resume 路径未走 store
          // （例如页面刷新后后端已是 running）：补一次累计避免 elapsed 偏大
          if (typeof backendPausedDurationMs !== 'number') {
            pausedAccumulatedMs += Date.now() - lastPauseAt
          }
          lastPauseAt = null
        }
        if (state === 'paused' && (lastPauseAt === null || typeof backendPausedDurationMs === 'number')) {
          // 后端 pausedDurationMs 包含当前暂停段，因此每个状态快照都能重建准确 elapsed；
          // lastPauseAt 只负责在两次快照之间冻结本地 tick。
          timeInfo.value = {
            elapsedTime: Math.max(0, Date.now() - status.value.startTime - pausedAccumulatedMs),
            estimatedRemaining: 0,
          }
          lastPauseAt = Date.now()
        }
        updateTimeInfo()
        startElapsedTick()
      }
    }

    // 检查是否完成
    if (state === 'completed' || state === 'error' || state === 'stopped') {
      if (status.value) {
        // 完成时刷新一次最终 elapsed，让 UI 显示真实总耗时。
        // 关键约束：终态快照若提供后端完整 pausedDurationMs，已在 L388 赋值给
        // pausedAccumulatedMs，此处不得再累加 Date.now() - lastPauseAt——
        // 否则最后一个暂停段会被重复结算，导致 elapsed 偏小（paused 时间被多算）。
        // 场景：paused 快照设置 lastPauseAt 后，下一帧直接变为 completed，
        //   旧代码在 422 累加 Date.now() - lastPauseAt，但后端 pausedDurationMs
        //   已包含该段，造成重复累计。
        if (typeof backendPausedDurationMs !== 'number') {
          // 后端未提供暂停时长：结算本地暂停段（兼容旧后端）
          if (lastPauseAt !== null) {
            pausedAccumulatedMs += Date.now() - lastPauseAt
            lastPauseAt = null
          }
        } else {
          // 后端已提供完整暂停时长：仅清 lastPauseAt，不再累加本地段
          lastPauseAt = null
        }
        updateTimeInfo()
        completeEvent.value = {
          taskId: status.value.taskId || `cal-${Date.now()}`,
          success: state === 'completed',
          totalPoints: status.value.totalPoints,
          duration: timeInfo.value?.elapsedTime ?? 0,
          error: calStatus.lastError ?? calStatus.LastError,
        }
        isRunning.value = false
        isPaused.value = false
        stopStatusPolling()
        stopElapsedTick()
      }
    }
  }

  // 从后端拉取一次完整 status 兜底，用于画面切回 / 再次进入时恢复状态（spec I2 / Decision #2）。
  //
  // 为什么：组件 remount 后本地 store 可能为空或过期，不能信任本地时效性；
  //   必须以后端为准。recovery 仅同步状态字段，不重启 polling——
  //   polling 频率由调用方依返回的 status 状态决定（running/paused 升 5Hz，否则 1Hz 心跳）。
  //
  // 失败时保留旧 store 状态（不 reset），让 UI 显示错误条 + 可重试（spec Recovery UX / I8）。
  //
  // @returns 映射后的前端任务状态；后端无任务（idle）时返回当时 store 中的 status（可能为 null）
  async function recoveryFromBackend(): Promise<void> {
    isRecovering.value = true
    recoveryError.value = null
    // 停掉 polling 避免与本次 recovery 并发写 status.value（spec I4 / Risks：竞态保护）
    stopStatusPolling()
    try {
      // wails 模式直接走 binding；http 模式调 calibrationApi.status() 兜底（API 名为 status，非 getStatus）
      const calStatus = isWailsAvailable()
        ? await wailsApi.calibration.status()
        : await calibrationApi.status()
      if (calStatus) {
        // 复用 updateStatusFromBackend 同步 status/dataPoints/completeEvent/timeInfo 等
        updateStatusFromBackend(calStatus)
      }
      lastRecoveryAt.value = Date.now()
    } catch (err: unknown) {
      // 失败：保留旧 store 状态（不 reset），让 UI 显示错误条 + 可重试
      const msg = err instanceof Error ? err.message : '恢复校准状态失败'
      recoveryError.value = msg
    } finally {
      isRecovering.value = false
    }
  }

  // 映射后端状态字符串到前端状态
  function mapCalibrationState(state: string): any {
    switch (state) {
      case 'running': return 'running'
      case 'paused': return 'paused'
      case 'completed': return 'completed'
      case 'error': return 'error'
      // spec Decision #4 / I7：stop 后 status='stopped'，与 idle 区分（保留数据可导出）
      case 'stopped': return 'stopped'
      default: return 'idle'
    }
  }

  watch(uiRefreshHz, () => {
    flushPendingPressureIfReady()
    if (isRunning.value && isWailsAvailable()) {
      startStatusPolling()
    }
  })

  // 内部会话清空：仅用于"开始新任务"场景（spec Decision #3 / I1）。
  // 不暴露给外部——unmount / stop 不应调，避免抹掉上一趟结果。
  // 注意：不调 stopStatusPolling，由 startCalibration 后续 startStatusPolling 重启。
  function resetSession() {
    cancelPressureThrottle()
    stopElapsedTick()
    pausedAccumulatedMs = 0
    lastPauseAt = null
    status.value = null
    isRunning.value = false
    isPaused.value = false
    completeEvent.value = null
    dataPoints.value = []
    realtimePressures.value = null
    calculatedPhysics.value = null
    timeInfo.value = null
  }

  async function startCalibration(config: CalibrationConfig) {
    const taskId = config.taskId || `cal-${Date.now()}`
    const configToStart: CalibrationConfig = { ...config, taskId }
    // 启动校准时同步配置中持久化的刷新频率，确保用户在设置对话框中的选择立即生效
    if (typeof config.uiRefreshHz === 'number' && Number.isFinite(config.uiRefreshHz)) {
      setUiRefreshHz(config.uiRefreshHz)
    }
    const wails = isWailsAvailable()
    if (wails) {
      const res = await wailsApi.calibration.start(configToStart)
      if (!res.Success) throw new Error(res.Error || '启动校准失败')
    } else {
      const res = await calibrationApi.startCalibration(configToStart)
      if (!res.success) throw new Error(res.error || '启动校准失败')
    }
    // spec Decision #3 / I1：后端 start 成功后才清旧会话；失败保留上一趟 stopped 结果，
    // 避免"start 失败把上趟数据抹了"。resetSession 不停 polling，下面 startStatusPolling 会重启。
    resetSession()
    isRunning.value = true
    isPaused.value = false
    // 启动时刻前端先记录一份本地 startTime 作为兜底：后端 status 首次轮询返回前，UI 已能用上 elapsed。
    // 后端返回真实 StartTime 后会在 updateStatusFromBackend 中覆盖，误差通常 < 一次轮询间隔（200ms）。
    const localStartTime = Date.now()
    status.value = {
      taskId,
      type: configToStart.type,
      status: 'running',
      totalPoints: configToStart.points.length,
      completedPoints: 0,
      progress: 0,
      startTime: localStartTime,
      dataPoints: [],
    }
    updateTimeInfo()
    if (wails) {
      startStatusPolling()
    }
    // 已用时 tick 在 wails / http 两种模式下都需要启动：
    // http 模式没有 statusPolling，但 UI "已用时"控件同样需要每秒刷新。
    startElapsedTick()
  }

  async function pause() {
    if (!status.value) return
    if (isWailsAvailable()) {
      const res = await wailsApi.calibration.pause()
      if (!res.Success) {
        throw new Error(res.Error || '暂停校准失败')
      }
    } else {
      await calibrationApi.pauseCalibration(status.value.taskId)
    }
    isPaused.value = true
    // 记录暂停开始时刻，tick 据此冻结 elapsed；resume 时累加到 pausedAccumulatedMs
    if (lastPauseAt === null) {
      lastPauseAt = Date.now()
    }
    if (status.value) status.value.status = 'paused'
  }

  async function resume() {
    if (!status.value) return
    if (isWailsAvailable()) {
      const res = await wailsApi.calibration.resume()
      if (!res.Success) {
        throw new Error(res.Error || '恢复校准失败')
      }
    } else {
      await calibrationApi.resumeCalibration(status.value.taskId)
    }
    isPaused.value = false
    // 把暂停段累加到 pausedAccumulatedMs，让 elapsed 从恢复时刻继续递增
    if (lastPauseAt !== null) {
      pausedAccumulatedMs += Date.now() - lastPauseAt
      lastPauseAt = null
    }
    if (status.value) status.value.status = 'running'
    // 立即刷新一次，避免 UI 等到下一个 tick 才更新
    updateTimeInfo()
  }

  async function stop() {
    if (!status.value) return
    if (isWailsAvailable()) {
      const res = await wailsApi.calibration.stop()
      if (!res.Success) {
        throw new Error(res.Error || '停止校准失败')
      }
    } else {
      await calibrationApi.stopCalibration(status.value.taskId)
    }
    isRunning.value = false
    isPaused.value = false
    // 停止时同样把暂停段结算到 pausedAccumulatedMs，确保最终 timeInfo 准确
    if (lastPauseAt !== null) {
      pausedAccumulatedMs += Date.now() - lastPauseAt
      lastPauseAt = null
    }
    updateTimeInfo()
    // spec Decision #4 / I7：stop 保留 dataPoints，status='stopped' 与 idle 区分（可导出 / 复盘）
    if (status.value) status.value.status = 'stopped'
    // 定格已用时；继续 poll 一次等后端 stopped 回包（updateStatusFromBackend 终态分支会 stopStatusPolling），
    // 避免高频空转等待。
    stopElapsedTick()
    if (isWailsAvailable()) {
      startStatusPolling(1000)
    }
  }

  async function saveData(savePath: string): Promise<{ success: boolean; filepath?: string; error?: string }> {
    if (!status.value) return { success: false, error: '无活跃任务' }
    return calibrationApi.saveData(status.value.taskId, savePath)
  }

  async function exportReport(): Promise<{ success: boolean; filepath?: string; error?: string }> {
    if (!status.value) return { success: false, error: '无活跃任务' }
    return calibrationApi.exportReport(status.value.taskId)
  }

  function reset() {
    cancelPressureThrottle()
    stopStatusPolling()
    stopElapsedTick()
    pausedAccumulatedMs = 0
    lastPauseAt = null
    status.value = null
    isRunning.value = false
    isPaused.value = false
    completeEvent.value = null
    dataPoints.value = []
    realtimePressures.value = null
    calculatedPhysics.value = null
    timeInfo.value = null
  }

  return {
    status,
    isRunning,
    isPaused,
    completeEvent,
    dataPoints,
    realtimePressures,
    calculatedPhysics,
    timeInfo,
    uiRefreshHz,
    uiRefreshIntervalMs,
    activeViewCount,
    isRecovering,
    recoveryError,
    lastRecoveryAt,
    setUiRefreshHz,
    updateRealtimePressures,
    acquireView,
    releaseView,
    recoveryFromBackend,
    restartPollingForCurrentState,
    startCalibration,
    pause,
    resume,
    stop,
    saveData,
    exportReport,
    reset,
  }
})
