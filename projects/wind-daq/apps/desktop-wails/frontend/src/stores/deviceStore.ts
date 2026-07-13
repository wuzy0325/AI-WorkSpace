import { defineStore } from 'pinia'
import { computed, ref, shallowRef } from 'vue'
import type { DeviceProfile, DeviceStatus, DataPayload, DSA3217ScanConfig, ChannelConfig, CalibrationResult } from '@api/types'
import { deviceApi } from '@api/deviceApi'
import { createRingBuffer, type RingBuffer } from '@composables/ringBuffer'
import { useStorageStore, computeHistoryCapacity, HISTORY_CAPACITY_HARD_CAP } from '@stores/storageStore'
import { useI18nStore } from '@stores/i18nStore'

/** 波形图同时显示的最大通道数，超过时不渲染（避免内存/渲染爆炸） */
const MAX_CHART_CHANNELS = 16

/**
 * 波形图渲染节拍（Hz）。
 *
 * 借鉴 daq-p1604 的架构：把"后端推送频率"与"波形图写入频率"解耦。
 *   - 后端按 refreshRateHz 轮询（默认 5Hz），每次都更新 latestSnapshots（通道卡片实时显示）
 *   - 但波形图只在 RENDER_TICK_HZ 频率下消费 pendingSnapshots 中的最新一帧写入 historyBuffers
 *
 * 这样：
 *   - 无论后端推多快，波形图每秒最多新增 RENDER_TICK_HZ 个点，GC 压力可控
 *   - 通道卡片不受影响，仍按后端推送频率实时更新
 *   - 用户配置的时间窗口与刷新率决定环形缓冲容量上限
 *
 * RENDER_TICK_HZ 决定波形图写入频率，是性能护栏，与刷新率上限 REFRESH_RATE_MAX=10Hz 对齐。
 * 时间窗口和刷新率决定环形缓冲容量，最大容量由 storageStore.HISTORY_CAPACITY_HARD_CAP=300 兜底。
 * 高于 10Hz 在 300 点缓冲下仍可能因 ECharts setOption 重算 times/series 导致卡顿。
 */
const RENDER_TICK_HZ = 10
const RENDER_TICK_INTERVAL_MS = Math.round(1000 / RENDER_TICK_HZ)

export interface DeviceCalibrationOperation {
  state: 'running' | 'completed' | 'cancelled' | 'error'
  channelIndex?: number
  elapsedSeconds: number
  sampleCount: number
  results?: CalibrationResult[]
  error?: string
}

export const useDeviceStore = defineStore('devices', () => {
  // 国际化：store 抛出的错误消息需随语言切换（如"请先开始采集"），故引入 i18n store。
  // 与 traversalStore 保持一致，避免在调用方重复维护中英文映射。
  const i18n = useI18nStore()
  const profiles = ref<DeviceProfile[]>([])
  const latestSnapshots = ref<DataPayload[]>([])
  const historyBuffers = shallowRef<Map<string, RingBuffer<DataPayload>>>(new Map())
  /** 历史数据版本号：每次 pushSnapshot 成功写入后递增，供 RealtimeChart computed 依赖 */
  const historyVersion = ref<number>(0)
  const selectedDeviceId = ref<string | null>(null)
  const calibrationOperations = ref<Map<string, DeviceCalibrationOperation>>(new Map())
  const calibrationControllers = new Map<string, AbortController>()
  /**
   * 校零采样时长（秒），从后端 /api/v1/calibrationConfig 加载。
   * 前端所有 /5s 显示与 Math.min(5, ...) 上限都引用此值，避免与后端默认时长脱节。
   * 加载失败时回退到 5 秒，保证功能可用。
   */
  const calibrationDurationSec = ref<number>(5)
  let calibrationConfigLoaded = false
  const chartSelectedIndices = ref<Map<string, Set<number>>>(new Map())
  const deviceStatuses = ref<Map<string, DeviceStatus>>(new Map())
  const profilesLoading = ref(false)
  const profilesLoadError = ref<string | null>(null)

  // pendingSnapshots：暂存每台设备最新一帧，等待 renderTick 消费。
  // 后端按用户配置 refreshRateHz 推送（默认 10Hz），每帧覆盖旧 pending（只保留最新），
  // renderTick 以 RENDER_TICK_HZ (10Hz) 消费 → 波形图每秒最多新增 10 个点，
  // 避免 10Hz 全量推送导致的 GC 压力，同时与刷新率对齐防止数据丢失。
  const pendingSnapshots = new Map<string, DataPayload>()
  let renderTickTimer: ReturnType<typeof setInterval> | null = null

  let unsubscribeSnapshot: (() => void) | null = null
  let snapshotAttachCount = 0
  const subscribedDeviceIds = new Set<string>()

  const selectedProfile = computed(() =>
    profiles.value.find((p) => p.id === selectedDeviceId.value),
  )
  const selectedSnapshot = computed(() =>
    latestSnapshots.value.find((s) => s.deviceId === selectedDeviceId.value),
  )

  const statusFor = (id: string): string => deviceStatuses.value.get(id)?.connection ?? 'Disconnected'
  const acquiringFor = (id: string): boolean => deviceStatuses.value.get(id)?.acquiring ?? false
  const latestFor = (id: string): DataPayload | undefined =>
    latestSnapshots.value.find((s) => s.deviceId === id)
  const historyFor = (id: string): DataPayload[] => {
    const ring = historyBuffers.value.get(id)
    return ring ? ring.toArray() as DataPayload[] : []
  }

  function selectDevice(id: string | null) {
    selectedDeviceId.value = id
    if (id) initializeDefaultChartSelection(id)
  }

  /**
   * 兼容迁移：旧版本（device-profiles 单位为 kPa, range 99–106）的持久化配置
   * 在加载时升级为 Pa, range 99000–106000；同时统一 precision 为 2。
   *
   * 设计要点：
   *   - 仅在通道索引 16（CH17 = 大气压）上生效，避免误改其他自定义通道。
   *   - 三重守卫，避免误升级用户主动选择的 kPa 量程或非大气压通道：
   *     1. unit 必须等于 'kPa'（不区分大小写）
   *     2. 通道名称必须暗示这是"大气压"（包含 "Patm" / "大气压" / "atm" 等关键字），
   *        防止用户把 CH17 重新映射为其他 kPa 量程的传感器时被误改。
   *     3. 数值必须落在大气压典型范围 [90, 120] kPa 内，超出范围说明不是大气压量程，
   *        保留原值不动。
   *   - 三个守卫之中任何一个不满足，函数都直接返回；幂等性来自首条守卫（升级后 unit=Pa）。
   *   - 该函数为纯函数（直接修改入参 channel）以便单测覆盖。
   */
  function migrateAtmPressureUnit(profile: DeviceProfile): void {
    if (!Array.isArray(profile.channels) || profile.channels.length < 17) return
    const atm = profile.channels[16]
    if (!atm) return

    // 守卫 1：仅升级单位仍是 kPa 的通道
    const unitNorm = (atm.unit ?? '').toLowerCase()
    if (unitNorm !== 'kpa') return

    // 守卫 2：通道名称必须暗示是大气压通道，避免用户把 CH17 重映射成其他 kPa 量程后被误改
    const name = (atm.name ?? '').toLowerCase()
    const looksLikeAtmospheric =
      name.includes('patm') ||
      name.includes('atm') ||
      name.includes('大气压') ||
      name.includes('atmosphe')
    if (!looksLikeAtmospheric) return

    // 守卫 3：range 落在大气压典型区间 [90, 120] kPa 内，否则保留用户配置
    const min = typeof atm.rangeMin === 'number' ? atm.rangeMin : NaN
    const max = typeof atm.rangeMax === 'number' ? atm.rangeMax : NaN
    const rangeLooksAtmospheric = Number.isFinite(min) && Number.isFinite(max) && min >= 90 && max <= 120
    if (!rangeLooksAtmospheric && Number.isFinite(min) && Number.isFinite(max)) {
      // 用户明确配置了一个非大气压量程，不做迁移
      return
    }

    atm.unit = 'Pa'
    if (Number.isFinite(min)) atm.rangeMin = min * 1000
    if (Number.isFinite(max)) atm.rangeMax = max * 1000
    if (!Number.isFinite(atm.rangeMin)) atm.rangeMin = 99000
    if (!Number.isFinite(atm.rangeMax)) atm.rangeMax = 106000
    atm.precision = 2
  }

  async function refreshProfiles() {
    profilesLoading.value = true
    profilesLoadError.value = null
    try {
      profiles.value = (await deviceApi.getProfiles()).map((profile) => ({
        ...profile,
        channels: Array.isArray(profile.channels) ? profile.channels : [],
      }))
      // 加载完成后做一次单位兼容迁移，保证旧 kPa 配置自动升级为 Pa
      profiles.value.forEach(migrateAtmPressureUnit)
    } catch (err) {
      // 设备配置是持久化资产；一次临时读取失败不能把侧栏已有设备洗空。
      profilesLoadError.value = err instanceof Error ? err.message : String(err)
      console.warn('[deviceStore] refreshProfiles failed, keeping previous profiles:', err)
      throw err
    } finally {
      profilesLoading.value = false
    }
    // 懒加载校零配置（仅一次），失败回退到默认 5 秒不影响主流程。
    if (!calibrationConfigLoaded) {
      calibrationConfigLoaded = true
      deviceApi.getCalibrationConfig().then((cfg) => {
        if (Number.isFinite(cfg.durationSec) && cfg.durationSec > 0) {
          calibrationDurationSec.value = cfg.durationSec
        }
      }).catch((err) => {
        console.warn('[deviceStore] load calibrationConfig failed, fallback to 5s:', err)
      })
    }
    syncSnapshotSubscriptions()
    if (selectedDeviceId.value) initializeDefaultChartSelection(selectedDeviceId.value)
  }

  async function refreshInstances() {
    // refreshProfiles 失败时不能跳过后续状态刷新与订阅同步——否则一次临时
    // profiles 读取失败会让指示灯失效、正在采集的设备不再轮询快照。失败已由
    // refreshProfiles 记入 profilesLoadError 并保留旧 profiles，此处吞掉错误
    // 继续执行后续步骤（调用方若关心 profiles 错误可直接调 refreshProfiles）。
    await refreshProfiles().catch((err) => {
      console.warn('[deviceStore] refreshInstances: refreshProfiles failed, continuing with status refresh:', err)
    })
    // 刷新所有设备的连接和采集状态，确保指示灯能正确显示
    await refreshAllStatuses()
    // 关键：refreshProfiles 末尾的 syncSnapshotSubscriptions 运行时 deviceStatuses
    // 尚未刷新，acquiringFor 一律返回 false，subscribeToDevice 不会被调用。
    // 必须在 refreshAllStatuses 完成后再同步一次订阅，才能让正在采集的设备
    // 启动 HTTP 轮询，触发 onSnapshot 回调，UI 实时数据才会更新。
    syncSnapshotSubscriptions()
  }

  /** 批量刷新所有已知设备的连接和采集状态 */
  async function refreshAllStatuses() {
    const ids = profiles.value.map((p) => p.id)
    if (ids.length === 0) return
    await Promise.allSettled(ids.map((id) => refreshStatusFor(id)))
  }

  async function refreshStatusFor(id: string) {
    try {
      const status = await deviceApi.getStatus(id)
      deviceStatuses.value.set(id, status)
    } catch {
      // Keep the last known status; transient device status reads should not blank the UI.
    }
  }

  function updateStatus(id: string, status: DeviceStatus) {
    deviceStatuses.value.set(id, status)
  }

  /**
   * 将设备快照推送到前端。
   *
   * 职责拆分（借鉴 daq-p1604 架构）：
   *   1. 立即更新 latestSnapshots（通道卡片实时显示，需要低延迟）
   *   2. 暂存到 pendingSnapshots（波形图专用，等 renderTick 节流消费）
   *
   * 不直接写入 historyBuffers——避免后端按 refreshRateHz 推送时波形图每帧重算 + GC 压力。
   * renderTick 以 RENDER_TICK_HZ (10Hz) 频率消费 pending，每设备每 tick 只写 1 个点。
   */
  function pushSnapshot(payload: DataPayload) {
    if (!payload || !payload.deviceId) return
    const normalized: DataPayload = {
      deviceId: payload.deviceId,
      timestamp: payload.timestamp ?? 0,
      channels: Array.isArray(payload.channels) ? payload.channels : [],
      channelIndices: Array.isArray(payload.channelIndices) ? payload.channelIndices : [],
    }

    // 快照只能证明设备有可展示数据；采集状态必须以后端 status 为准。
    // 停止采集后可能仍收到最后一帧缓存，不能用旧快照把 UI 重新标成采集中。
    const existingStatus = deviceStatuses.value.get(normalized.deviceId)
    if (!existingStatus || existingStatus.connection !== 'Connected') {
      deviceStatuses.value.set(normalized.deviceId, {
        id: normalized.deviceId,
        name: existingStatus?.name ?? normalized.deviceId,
        type: existingStatus?.type ?? 'Unknown',
        connection: 'Connected',
        acquiring: existingStatus?.acquiring ?? false,
      })
    }

    // 职责 1：立即更新 latestSnapshots（通道卡片读取，实时性优先）
    const idx = latestSnapshots.value.findIndex((s) => s.deviceId === normalized.deviceId)
    if (idx >= 0) {
      latestSnapshots.value[idx] = normalized
    } else {
      latestSnapshots.value.push(normalized)
    }

    // 职责 2：暂存到 pendingSnapshots，等 renderTick 节流消费
    // 覆盖式写入：若上一帧还没被 renderTick 消费，直接覆盖
    // → renderTick 消费时只拿到最新一帧，避免历史数据堆积
    pendingSnapshots.set(normalized.deviceId, normalized)
  }

  /**
   * 波形图渲染节拍：消费 pendingSnapshots，每设备每 tick 写入 1 个点到 historyBuffers。
   *
   * - 借鉴 daq-p1604 的 renderTick 设计：把"后端推送频率"与"波形图写入频率"解耦
   * - 10Hz：与默认 refreshRateHz 对齐，波形图每秒最多新增 10 个点
   * - 组件层用单个 computed option + watch flush:'post'，避免 setTimeout 节流叠加
   */
  function flushPendingToHistory(): void {
    if (pendingSnapshots.size === 0) return

    const storageStore = useStorageStore()
    // 容量 = 时间窗口 × 刷新率，clamp 到硬上限
    // 用户改"时间窗口"调的是看多久的历史；改"刷新率"调的是波形密度
    // 容量自动随两者变化，不再让用户直接配点数
    const expectedCapacity = computeHistoryCapacity(
      storageStore.settings.historyWindowSec,
      storageStore.settings.refreshRateHz,
    )

    let dirty = false
    const currentMap = historyBuffers.value

    for (const [deviceId, snapshot] of pendingSnapshots) {
      let ring = currentMap.get(deviceId)

      // 容量变化时重建环形缓冲：丢弃旧数据，使用新容量
      if (!ring || ring.capacity !== expectedCapacity) {
        ring = createRingBuffer<DataPayload>(expectedCapacity)
        const newMap = new Map(currentMap)
        newMap.set(deviceId, ring)
        historyBuffers.value = newMap
        // 注意：historyBuffers.value 已替换为 newMap，后续迭代需用新 Map
        // 但这里我们继续用 currentMap 引用——下面 ring.push 写入的是 newMap 中的 ring 对象
        // 因为 createRingBuffer 返回的 ring 是引用，newMap.set(deviceId, ring) 和 currentMap.get(deviceId) 是同一个对象
        // 所以 push 操作会反映到 newMap 中
      }

      // 同 timestamp 去重：不重复写入
      // 用 peekLast() O(1) 访问最新元素，避免 toArray() 在 cap 满后每帧 O(N) 数组重建
      // （此前是 30 秒后卡顿的主要根因之一）
      const last = ring.peekLast()
      if (last?.timestamp === snapshot.timestamp) continue

      ring.push(snapshot)
      dirty = true
    }

    if (dirty) {
      historyVersion.value += 1
    }
    pendingSnapshots.clear()
  }

  /**
   * 启动 renderTick 定时器。重复调用安全（已启动则跳过）。
   * 在 attachStatusListener 时自动启动，detach 时自动停止。
   */
  function startRenderTick(): void {
    if (renderTickTimer !== null) return
    renderTickTimer = setInterval(flushPendingToHistory, RENDER_TICK_INTERVAL_MS)
  }

  function stopRenderTick(): void {
    if (renderTickTimer !== null) {
      clearInterval(renderTickTimer)
      renderTickTimer = null
    }
  }

  // syncSnapshotSubscriptions 根据当前 profiles 列表同步采集数据订阅。
  //
  // 注意：本函数在 refreshProfiles 末尾调用（L104），此时 deviceStatuses 中设备的
  // acquiring 状态可能仍为初始值 false。实际订阅入口在 startAcquisition（L404）——
  // StartAcquisition 成功后直接调用 subscribeToDevice + subscribedDeviceIds.add，
  // 不依赖本函数。本函数仅作为防御性同步：如果 profiles 列表在采集过程中发生变化
  // （新增/删除设备），确保订阅集合与 profiles 保持一致。
  function syncSnapshotSubscriptions() {
    if (!unsubscribeSnapshot) return

    const activeIds = new Set(profiles.value.map((p) => p.id))
    profiles.value.forEach((profile) => {
      if (subscribedDeviceIds.has(profile.id)) return
      if (!acquiringFor(profile.id)) return
      deviceApi.subscribeToDevice(profile.id)
      subscribedDeviceIds.add(profile.id)
    })

    Array.from(subscribedDeviceIds).forEach((id) => {
      if (activeIds.has(id)) return
      deviceApi.unsubscribeFromDevice(id)
      subscribedDeviceIds.delete(id)
    })
  }

  function cleanupSnapshotSubscriptions() {
    subscribedDeviceIds.forEach((id) => {
      deviceApi.unsubscribeFromDevice(id)
    })
    subscribedDeviceIds.clear()
  }

  function attachStatusListener(): () => void {
    snapshotAttachCount += 1

    if (!unsubscribeSnapshot) {
      unsubscribeSnapshot = deviceApi.onSnapshot((payload) => {
        pushSnapshot(payload)
      })
    }
    // 启动 renderTick：以固定频率消费 pendingSnapshots → historyBuffers
    startRenderTick()
    syncSnapshotSubscriptions()

    return () => {
      snapshotAttachCount = Math.max(0, snapshotAttachCount - 1)
      if (snapshotAttachCount === 0 && unsubscribeSnapshot) {
        unsubscribeSnapshot()
        unsubscribeSnapshot = null
        // 最后一次 detach 时停止 renderTick，避免空转
        stopRenderTick()
        // 清空 pending 防止下次 attach 时残留旧数据被立即消费
        pendingSnapshots.clear()
        cleanupSnapshotSubscriptions()
      }
    }
  }

  const formatValue = (id: string, channelIndex: number, rawValue: number): string => {
    if (!Number.isFinite(rawValue)) return ''
    return rawValue.toFixed(getChannelPrecision(id, channelIndex))
  }

  function setCalibrationOperation(id: string, operation: DeviceCalibrationOperation): void {
    calibrationOperations.value = new Map(calibrationOperations.value).set(id, operation)
  }

  function calibrationOperationFor(id: string): DeviceCalibrationOperation | undefined {
    return calibrationOperations.value.get(id)
  }

  async function calibrate(id: string, channelIndex?: number): Promise<CalibrationResult[]> {
    if (!acquiringFor(id)) throw new Error(i18n.t.pleaseStartAcquisitionFirst || '请先开始采集')
    if (calibrationControllers.has(id)) throw new Error(i18n.t.calibrationInProgress || '校零正在进行中')
    const controller = new AbortController()
    calibrationControllers.set(id, controller)
    const startedAt = Date.now()
    const durationSec = calibrationDurationSec.value
    // P2-17：前端总超时兑底。后端已有 fallback deadline（sampleDuration + 2s），
    // 但 HTTP 请求可能因网络代理缓冲、keep-alive 死锁等原因永不返回，
    // 前端必须有自己的超时兜底，否则用户会被卡在校零中状态无法恢复。
    // 超时 = 采样时长 + 5s（比后端 fallback 多 3s，给后端响应留窗口）。
    const totalTimeoutMs = (durationSec + 5) * 1000
    const totalTimeoutTimer = setTimeout(() => {
      // 触发 abort → fetch 抛 AbortError → catch 块标记为 cancelled
      controller.abort()
    }, totalTimeoutMs)
    setCalibrationOperation(id, { state: 'running', channelIndex, elapsedSeconds: 0, sampleCount: 0 })
    let progressRequestRunning = false
    const timer = setInterval(async () => {
      const current = calibrationOperationFor(id)
      if (current?.state === 'running' && !progressRequestRunning) {
        progressRequestRunning = true
        try {
          const progress = await deviceApi.getCalibrationProgress(id)
          const latestOperation = calibrationOperationFor(id)
          if (latestOperation?.state === 'running') {
            setCalibrationOperation(id, {
              ...latestOperation,
              elapsedSeconds: Math.min(durationSec, Math.floor(progress.elapsedMs / 1000)),
              sampleCount: progress.sampleCount,
            })
          }
        } catch {
          const latestOperation = calibrationOperationFor(id)
          if (latestOperation?.state === 'running') {
            setCalibrationOperation(id, { ...latestOperation, elapsedSeconds: Math.min(durationSec, Math.floor((Date.now() - startedAt) / 1000)) })
          }
        } finally {
          progressRequestRunning = false
        }
      }
    }, 250)
    try {
      const results = await deviceApi.calibrate(id, channelIndex, controller.signal)
      await refreshProfiles()
      setCalibrationOperation(id, { state: 'completed', channelIndex, elapsedSeconds: durationSec, sampleCount: results.reduce((sum, result) => sum + result.sampleCount, 0), results })
      return results
    } catch (error) {
      const cancelled = controller.signal.aborted || (error instanceof DOMException && error.name === 'AbortError')
      setCalibrationOperation(id, cancelled
        ? { state: 'cancelled', channelIndex, elapsedSeconds: Math.min(durationSec, Math.floor((Date.now() - startedAt) / 1000)), sampleCount: calibrationOperationFor(id)?.sampleCount ?? 0 }
        : { state: 'error', channelIndex, elapsedSeconds: Math.min(durationSec, Math.floor((Date.now() - startedAt) / 1000)), sampleCount: calibrationOperationFor(id)?.sampleCount ?? 0, error: error instanceof Error ? error.message : String(error) })
      throw error
    } finally {
      clearInterval(timer)
      clearTimeout(totalTimeoutTimer)
      calibrationControllers.delete(id)
    }
  }

  function cancelCalibration(id: string): void {
    calibrationControllers.get(id)?.abort()
  }

  async function clearCalibration(id: string, channelIndex: number): Promise<void> {
    await deviceApi.clearCalibration(id, channelIndex)
    await refreshProfiles()
  }

  async function setCalibrationEnabled(id: string, channelIndex: number, enabled: boolean): Promise<void> {
    await deviceApi.setCalibrationEnabled(id, channelIndex, enabled)
    await refreshProfiles()
  }

  function findChannel(id: string, channelIndex: number) {
    const profile = profiles.value.find((p) => p.id === id)
    if (!Array.isArray(profile?.channels)) return undefined
    return profile.channels.find((c) => c.index === channelIndex)
  }

  function getChannelRange(id: string, channelIndex: number): { min: number; max: number } {
    const channel = findChannel(id, channelIndex)
    const min = channel?.rangeMin ?? -10
    const max = channel?.rangeMax ?? 10
    return min === max ? { min: min - 1, max: max + 1 } : { min, max }
  }

  function getChannelPrecision(id: string, channelIndex: number): number {
    return findChannel(id, channelIndex)?.precision ?? 3
  }

  function toggleChartSelection(id: string, channelIndex: number) {
    let selection = chartSelectedIndices.value.get(id)
    if (!selection) {
      selection = new Set()
      chartSelectedIndices.value.set(id, selection)
    }

    if (selection.has(channelIndex)) {
      selection.delete(channelIndex)
    } else if (selection.size < MAX_CHART_CHANNELS) {
      selection.add(channelIndex)
    }
  }

  function setAllChartSelection(id: string, enabled: boolean) {
    const profile = profiles.value.find((p) => p.id === id)
    if (!profile) return

    if (enabled) {
      const channels = Array.isArray(profile.channels) ? profile.channels : []
      const enabledIndices = channels.filter((ch) => ch.enabled).map((ch) => ch.index)
      chartSelectedIndices.value.set(id, new Set(enabledIndices.slice(0, MAX_CHART_CHANNELS)))
      return
    }

    chartSelectedIndices.value.set(id, new Set())
  }

  function isChartSelected(id: string, channelIndex: number): boolean {
    return chartSelectedIndices.value.get(id)?.has(channelIndex) ?? false
  }

  /**
   * 判定一个通道是否为"大气压力 / 大气温度"专用通道。
   *
   * 用于波形图默认通道选择 —— 这两类通道量程与常规测量通道差异巨大
   * （大气压约 99000~106000 Pa，大气温度约 20~25 ℃），与常规通道同图绘制
   * 会压扁其他通道的有效幅值范围，因此默认不纳入波形图。
   *
   * 识别策略基于通道名称（而非索引），以兼容不同设备类型：
   *   - SIMULATED / DAQ-P-1604：通道 16=大气压、17=大气温度
   *   - DAQ-T-1603 / DAQ-P-1604Pre / DSA3217：无此类通道
   *   - WTN_PXI：通道布局完全不同
   * 命名守卫与 migrateAtmPressureUnit 保持一致，同时支持中英文。
   */
  function isAtmosphericChannel(channel: ChannelConfig): boolean {
    const name = (channel.name ?? '').toLowerCase()
    return (
      name.includes('patm') ||
      name.includes('tatm') ||
      name.includes('大气压') ||
      name.includes('大气温度') ||
      name.includes('atmosphe')
    )
  }

  function initializeDefaultChartSelection(id: string): void {
    const current = chartSelectedIndices.value.get(id)
    if (current && current.size > 0) return

    const profile = profiles.value.find((p) => p.id === id)
    if (!profile) return

    // 默认选中全部 enabled 通道，但排除大气压力 / 大气温度通道
    // （量程差异过大，同图绘制会压扁常规通道波形）。
    // 上限受 MAX_CHART_CHANNELS 约束，防止渲染过载。
    const channels = Array.isArray(profile.channels) ? profile.channels : []
    const defaults = channels
      .filter((channel) => channel.enabled && !isAtmosphericChannel(channel))
      .slice(0, MAX_CHART_CHANNELS)
      .map((channel) => channel.index)

    if (defaults.length > 0) {
      chartSelectedIndices.value.set(id, new Set(defaults))
    }
  }

  async function connect(id: string) {
    // 乐观更新：在调用后端连接 API 之前，先将状态切换为 Connecting，
    // 这样 UI 可以立即显示"连接中"提示，避免按下按钮后无任何反馈。
    const prev = deviceStatuses.value.get(id)
    const profile = profiles.value.find((p) => p.id === id)
    deviceStatuses.value.set(id, {
      id,
      name: prev?.name ?? profile?.name ?? id,
      type: prev?.type ?? profile?.type ?? 'Unknown',
      connection: 'Connecting',
      acquiring: prev?.acquiring ?? false,
      lastError: undefined,
    })
    try {
      await deviceApi.connect(id)
      // 连接调用返回后，再以后端真实状态为准
      try {
        await refreshStatusFor(id)
      } catch (refreshErr) {
        // refreshStatusFor 失败不意味着连接失败——设备已连接成功，
        // 仅状态刷新暂时不可用，下一次 StatusPoll 轮询会自行修复。
        console.warn(`[deviceStore] connect succeeded but refreshStatusFor failed for ${id}:`, refreshErr)
      }
    } catch (err) {
      // 连接失败：回滚到 Error 状态，便于上层 UI 复位按钮并提示
      const current = deviceStatuses.value.get(id)
      deviceStatuses.value.set(id, {
        id,
        name: current?.name ?? profile?.name ?? id,
        type: current?.type ?? profile?.type ?? 'Unknown',
        connection: 'Error',
        acquiring: current?.acquiring ?? false,
        lastError: err instanceof Error ? err.message : String(err),
      })
      throw err
    }
  }

  async function disconnect(id: string) {
    // 调用后端断开。即使后端返回成功，DeviceManager 也会从 devices map 中删除该设备，
    // 之后 GetStatus(id) 会返回 (Status{}, false)，触发 deviceApi.getStatus 抛
    // "设备状态不可用"，使 refreshStatusFor 静默保留旧状态——按钮文字停在"断开"，
    // 用户看起来就是"点击断开没反应"。
    // 修复策略：先调后端，再退订数据流，并显式把本地状态置为 Disconnected，
    // 最后才尝试 refreshStatusFor。即便刷新失败，UI 也能立即反映断开结果。
    try {
      await deviceApi.disconnect(id)
    } finally {
      // 无论后端是否报错，都先停止订阅，避免遗留 SSE/事件回调把状态又拉回 Connected
      deviceApi.unsubscribeFromDevice(id)
      subscribedDeviceIds.delete(id)
    }

    // 乐观写入：将本地状态置为 Disconnected，确保按钮文字与状态条立刻刷新
    const prev = deviceStatuses.value.get(id)
    const profile = profiles.value.find((p) => p.id === id)
    deviceStatuses.value.set(id, {
      id,
      name: prev?.name ?? profile?.name ?? id,
      type: prev?.type ?? profile?.type ?? 'Unknown',
      connection: 'Disconnected',
      acquiring: false,
      lastError: undefined,
    })

    // 仍尝试拉一次后端状态做最终对齐：若后端返回有效状态会覆盖上面的乐观值；
    // 若返回 "设备状态不可用"（断开后的预期分支），refreshStatusFor 的 catch
    // 会保留我们刚写入的 Disconnected，符合预期。
    try {
      await refreshStatusFor(id)
    } catch (refreshErr) {
      console.warn(`[deviceStore] disconnect succeeded but refreshStatusFor failed for ${id}:`, refreshErr)
    }
  }

  async function startAcquisition(id: string): Promise<boolean> {
    const result = await deviceApi.startAcquisition(id)
    if (result.success) {
      deviceApi.subscribeToDevice(id)
      subscribedDeviceIds.add(id)
      await refreshStatusFor(id)
    }
    return result.success
  }

  /**
   * 确保目标设备的数据轮询订阅已建立，但不发送 startAcquisition 命令。
   *
   * 使用场景：遍历测试由后端 ParseAndStartTraversal 自动启动采集后，
   * 前端仅需建立轮询订阅即可接收实时数据，无需重复发送启动命令。
   *
   * 与 startAcquisition 的区别：不调用 deviceApi.startAcquisition（避免冗余
   * 后端命令），但同样执行 subscribeToDevice + subscribedDeviceIds.add，
   * 保证 cleanupSnapshotSubscriptions 在最后一个 listener detach 时能正确
   * 清理轮询定时器，避免资源泄漏。
   *
   * 幂等：subscribeToDevice 内部按 deviceId 去重，subscribedDeviceIds 是
   * Set，重复调用安全。
   */
  function ensureSubscribed(id: string): void {
    if (!id) return
    deviceApi.subscribeToDevice(id)
    subscribedDeviceIds.add(id)
  }

  async function stopAcquisition(id: string): Promise<void> {
    await deviceApi.stopAcquisition(id)
    deviceApi.unsubscribeFromDevice(id)
    subscribedDeviceIds.delete(id)
    await refreshStatusFor(id)
  }

  async function getDsa3217ScanConfig(id: string): Promise<DSA3217ScanConfig | null> {
    try {
      const result = await deviceApi.getDsa3217ScanConfig(id)
      return result?.data ?? null
    } catch {
      return null
    }
  }

  async function applyDsa3217ScanConfig(
    id: string,
    config: { avg: number; period: number },
  ): Promise<DSA3217ScanConfig | null> {
    const result = await deviceApi.applyDsa3217ScanConfig(id, config)
    if (!result.success) {
      throw new Error(result.error || 'Failed to sync DSA3217 scan config')
    }
    return result.data ?? null
  }

  // DAQ-P-1603 配置回读：失败时返回 null，由调用方决定如何提示
  // （打开配置面板时回读失败不阻塞编辑，仅 console.warn）。
  async function getDaqP1603Config(id: string): Promise<DeviceProfile | null> {
    try {
      const result = await deviceApi.getDaqP1603Config(id)
      return result?.data ?? null
    } catch {
      return null
    }
  }

  // DAQ-P-1603 配置应用：同步到硬件并回读验证。
  // 失败时抛错，由 saveDraft 上层 try/catch 捕获并向用户展示错误提示。
  // 成功时返回回读的 profile，调用方可用其更新本地 draft（拿硬件实际值）。
  async function applyDaqP1603Config(
    id: string,
    profile: DeviceProfile,
  ): Promise<DeviceProfile | null> {
    const result = await deviceApi.applyDaqP1603Config(id, profile)
    if (!result.success) {
      throw new Error(result.error || 'Failed to sync DAQ-P-1603 config')
    }
    return result.data ?? null
  }

  // ==================== 全局采集编排 ====================

  const isAnyAcquiring = computed(() =>
    profiles.value.some((p) => acquiringFor(p.id)),
  )

  async function startAllAcquisitions(): Promise<void> {
    const connectedIds = profiles.value
      .filter((p) => statusFor(p.id) === 'Connected')
      .map((p) => p.id)
    const targetIds = connectedIds.length > 0
      ? connectedIds
      : [selectedDeviceId.value ?? 'sim-1']

    // 先把未连接的设备并行连一遍；只对最终处于 Connected 的设备启动采集，
    // 避免对连接失败的设备发出注定失败的 startAcquisition、并误导调用方
    // (例如 MainDashboardView 误判已开始采集而进入录制流程)。
    const needConnect = targetIds.filter((id) => statusFor(id) !== 'Connected')
    if (needConnect.length > 0) {
      const connectResults = await Promise.allSettled(needConnect.map((id) => connect(id)))
      connectResults.forEach((result, idx) => {
        if (result.status === 'rejected') {
          const id = needConnect[idx]
          const profile = profiles.value.find((p) => p.id === id)
          console.warn(`[startAll] 设备 "${profile?.name ?? id}" 连接失败:`, result.reason)
        }
      })
    }

    const startIds = targetIds.filter((id) => statusFor(id) === 'Connected')
    if (startIds.length === 0) {
      console.warn('[startAll] 没有可启动采集的已连接设备，跳过 startAcquisition')
      await refreshAllStatuses()
      return
    }
    const results = await Promise.allSettled(startIds.map((id) => startAcquisition(id)))
    results.forEach((result, idx) => {
      if (result.status === 'rejected') {
        const profile = profiles.value.find((p) => p.id === startIds[idx])
        console.warn(`[startAll] 设备 "${profile?.name ?? startIds[idx]}" 启动采集失败:`, result.reason)
      }
    })
    await refreshAllStatuses()
  }

  async function stopAllAcquisitions(): Promise<void> {
    const acquiringIds = profiles.value
      .filter((p) => acquiringFor(p.id))
      .map((p) => p.id)
    if (acquiringIds.length === 0) return
    const results = await Promise.allSettled(acquiringIds.map((id) => stopAcquisition(id)))
    results.forEach((result, idx) => {
      if (result.status === 'rejected') {
        const profile = profiles.value.find((p) => p.id === acquiringIds[idx])
        console.warn(`[stopAll] 设备 "${profile?.name ?? acquiringIds[idx]}" 停止采集失败:`, result.reason)
      }
    })
    await refreshAllStatuses()
  }

  return {
    profiles,
    latestSnapshots,
    selectedDeviceId,
    selectedProfile,
    selectedSnapshot,
    deviceStatuses,
    profilesLoading,
    profilesLoadError,
    statusFor,
    acquiringFor,
    isAnyAcquiring,
    startAllAcquisitions,
    stopAllAcquisitions,
    selectDevice,
    refreshProfiles,
    refreshInstances,
    refreshAllStatuses,
    refreshStatusFor,
    updateStatus,
    pushSnapshot,
    flushPendingToHistory,
    historyVersion,
    attachStatusListener,
    latestFor,
    historyFor,
    formatValue,
    calibrationOperations,
    calibrationOperationFor,
    calibrationDurationSec,
    calibrate,
    cancelCalibration,
    clearCalibration,
    setCalibrationEnabled,
    getChannelRange,
    getChannelPrecision,
    toggleChartSelection,
    setAllChartSelection,
    isChartSelected,
    isAtmosphericChannel,
    initializeDefaultChartSelection,
    connect,
    disconnect,
    startAcquisition,
    ensureSubscribed,
    stopAcquisition,
    getDsa3217ScanConfig,
    applyDsa3217ScanConfig,
    getDaqP1603Config,
    applyDaqP1603Config,
  }
})
