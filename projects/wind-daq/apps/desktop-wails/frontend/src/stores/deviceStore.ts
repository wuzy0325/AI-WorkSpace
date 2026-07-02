import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import type { DeviceProfile, DeviceStatus, DataPayload, DSA3217ScanConfig, ChannelConfig } from '@api/types'
import { deviceApi } from '@api/deviceApi'

const MAX_HISTORY_POINTS = 256
const MAX_CHART_CHANNELS = 16

export const useDeviceStore = defineStore('devices', () => {
  const profiles = ref<DeviceProfile[]>([])
  const latestSnapshots = ref<DataPayload[]>([])
  const historyBuffers = ref<Map<string, DataPayload[]>>(new Map())
  const selectedDeviceId = ref<string | null>(null)
  const tareOffsets = ref<Map<string, Record<number, number>>>(new Map())
  const chartSelectedIndices = ref<Map<string, Set<number>>>(new Map())
  const deviceStatuses = ref<Map<string, DeviceStatus>>(new Map())
  const profilesLoading = ref(false)
  const profilesLoadError = ref<string | null>(null)

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
  const historyFor = (id: string): DataPayload[] => historyBuffers.value.get(id) ?? []

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

    const idx = latestSnapshots.value.findIndex((s) => s.deviceId === normalized.deviceId)
    if (idx >= 0) {
      latestSnapshots.value[idx] = normalized
    } else {
      latestSnapshots.value.push(normalized)
    }

    let buffer = historyBuffers.value.get(normalized.deviceId)
    if (!buffer) {
      buffer = []
      historyBuffers.value.set(normalized.deviceId, buffer)
    }
    const last = buffer[buffer.length - 1]
    if (last?.timestamp === normalized.timestamp) return

    buffer.push(normalized)
    if (buffer.length > MAX_HISTORY_POINTS) buffer.shift()
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
    syncSnapshotSubscriptions()

    return () => {
      snapshotAttachCount = Math.max(0, snapshotAttachCount - 1)
      if (snapshotAttachCount === 0 && unsubscribeSnapshot) {
        unsubscribeSnapshot()
        unsubscribeSnapshot = null
        cleanupSnapshotSubscriptions()
      }
    }
  }

  const formatValue = (id: string, channelIndex: number, rawValue: number): string => {
    const value = applyDisplayTare(id, channelIndex, rawValue)
    if (!Number.isFinite(value)) return ''
    return value.toFixed(getChannelPrecision(id, channelIndex))
  }

  const applyDisplayTare = (id: string, channelIndex: number, rawValue: number): number => {
    const offset = tareOffsets.value.get(id)?.[channelIndex] ?? 0
    return rawValue - offset
  }

  function setTare(id: string, channelIndex: number, value: number) {
    let deviceTares = tareOffsets.value.get(id)
    if (!deviceTares) {
      deviceTares = {}
      tareOffsets.value.set(id, deviceTares)
    }
    deviceTares[channelIndex] = value
  }

  function tareAllEnabled(id: string) {
    const profile = profiles.value.find((p) => p.id === id)
    if (!profile) return
    const snapshot = latestSnapshots.value.find((s) => s.deviceId === id)
    if (!snapshot) return
    const indices = Array.isArray(snapshot.channelIndices) ? snapshot.channelIndices : []
    const channels = Array.isArray(snapshot.channels) ? snapshot.channels : []

    const profileChannels = Array.isArray(profile.channels) ? profile.channels : []
    profileChannels.forEach((channel) => {
      if (!channel.enabled) return
      const pos = indices.indexOf(channel.index)
      if (pos >= 0) {
        setTare(id, channel.index, channels[pos])
      }
    })
  }

  function getOffset(id: string, channelIndex: number): number {
    return tareOffsets.value.get(id)?.[channelIndex] ?? 0
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
   *   - DAQ-T-1603 / DAQ-P-1064Pre / DSA3217：无此类通道
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
    attachStatusListener,
    latestFor,
    historyFor,
    formatValue,
    applyDisplayTare,
    setTare,
    tareAllEnabled,
    getOffset,
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
    stopAcquisition,
    getDsa3217ScanConfig,
    applyDsa3217ScanConfig,
  }
})
