<script setup lang="ts">
import { computed, defineAsyncComponent, ref, onMounted, onBeforeUnmount } from 'vue'
import { storeToRefs } from 'pinia'
import { wailsApi, isWailsAvailable } from '@api/wails-adapter'
import type { AppRailNavItem } from '@components/layout/AppRailNav.vue'
import AppRailNav from '@components/layout/AppRailNav.vue'
import GlobalSettingsModal from '@components/layout/GlobalSettingsModal.vue'
import MainTopBar from '@components/layout/MainTopBar.vue'
import MainBottomBar from '@components/layout/MainBottomBar.vue'
import DeviceSidebar from '@components/main/DeviceSidebar.vue'
import DeviceOverviewPanel from '@components/main/DeviceOverviewPanel.vue'
import DeviceDetailPanel from '@components/main/DeviceDetailPanel.vue'
import MainView from '@views/MainView.vue'
// 按需异步加载子页面：这些视图通过 v-if 在 activePage 变化时挂载，
// 默认进入 dashboard 不会触发，把它们的代码体积从首屏 chunk 移除。
// TraversalView 体积最大（含 echarts + 4 个可视化视图），尤其需要 lazy。
const CalibrationView = defineAsyncComponent(() => import('@views/CalibrationView.vue'))
const TraversalView = defineAsyncComponent(() => import('@views/TraversalView.vue'))
const LogViewer = defineAsyncComponent(() => import('@views/LogViewer.vue'))
// DeviceManagementDrawer 体积较大（1430+ 行、含设备配置表单、通道编辑器、扫描结果列表），
// 仅在用户点击"打开设备管理"时才需要，默认 v-model:open=false 不渲染内部内容。
// 把它转为异步加载有两个好处：
//   1) 主仪表盘首屏 chunk 减小约 85KB（SFC 源码体积）；
//   2) dev 模式下 Wails AssetServer 代理超大 SFC 时可能触发 Resp.AppendHeader 错误，
//      该错误若发生在首屏同步 import 链上会阻断主画面渲染（白屏）。
//      改为异步后，加载失败只影响 drawer 本身，主画面照常可用。
const DeviceManagementDrawer = defineAsyncComponent(() => import('@components/device/DeviceManagementDrawer.vue'))
// 探针校准许可证对话框：仅在用户点击「探针校准」且未解锁时才需要，
// 异步加载避免将解锁逻辑打进首屏 chunk。
const CalibrationLicenseDialog = defineAsyncComponent(() => import('@components/calibration/CalibrationLicenseDialog.vue'))
import { useDeviceStore } from '@stores/deviceStore'
import { useI18nStore } from '@stores/i18nStore'
import { useFeedbackStore } from '@stores/feedbackStore'
import { useStorageStore } from '@stores/storageStore'
import { useCalibrationLicenseStore } from '@stores/calibrationLicenseStore'
import { storageApi, deviceApi } from '@api/deviceApi'
import UiAlert from '@components/ui/UiAlert.vue'
import UiEmptyState from '@components/ui/UiEmptyState.vue'
import UiLoadingState from '@components/ui/UiLoadingState.vue'
import UiButton from '@components/ui/UiButton.vue'

type MainShellPage = 'dashboard' | 'calibration' | 'traversal' | 'log'

const deviceStore = useDeviceStore()
const i18n = useI18nStore()
const feedbackStore = useFeedbackStore()
const storageStore = useStorageStore()
const licenseStore = useCalibrationLicenseStore()
const { t } = storeToRefs(i18n)

const activePage = ref<MainShellPage>('dashboard')
const appVersion = ref('0.1.0')
const showDeviceDrawer = ref(false)
const showSettings = ref(false)
// 探针校准许可证对话框：点击「探针校准」入口且未解锁时弹出。
// 已解锁（licenseStore.isUnlocked=true）时不会显示，直接放行。
const showLicenseDialog = ref(false)
const viewMode = ref<'overview' | 'chart' | 'table' | 'both'>('both')
const isRecording = ref(false)
// 录制状态扩展字段（来自后端 Status()，便于 UI 展示与错误反馈）
// outputDir/currentFile 在 sink Stop 后仍由后端保留，前端据此在状态栏
// 显示"上次保存路径"，便于用户停止后定位文件位置。
const recordingStatus = ref<{
  outputDir?: string
  currentFile?: string
  fileSize?: number
  fileCount?: number
  recordCount?: number
  durationMs?: number
  droppedCount?: number
  lastError?: string
}>({})
const busy = ref(false)
const error = ref('')
const initialLoading = ref(true)
let unsubscribeDeviceSnapshots: (() => void) | null = null
// 录制状态轮询句柄；高频采集中也只 2s 轮询一次，避免给本地 HTTP 增加负担
let recordingStatusTimer: ReturnType<typeof setInterval> | null = null
// 上一次已上报的 lastError，避免同一错误反复 toast
let lastReportedError = ''

// 采集状态：委托 store 层判断（任一设备正在采集即为 true）
const acquiring = computed(() => deviceStore.isAnyAcquiring)

const railItems = computed<AppRailNavItem[]>(() => [
  { id: 'dashboard', label: t.value.dashboardHome, icon: 'IO', active: activePage.value === 'dashboard' },
  // locked 跟随解锁状态响应式变化：解锁后角标自动消失，无需刷新页面
  { id: 'calibration', label: t.value.probeCalibration, icon: 'CP', active: activePage.value === 'calibration', locked: !licenseStore.isUnlocked },
  { id: 'traversal', label: t.value.traversalTest, icon: 'TR', active: activePage.value === 'traversal' },
  { id: 'log', label: t.value.logViewer || 'Logs', icon: 'LG', active: activePage.value === 'log' }
])

// 底部 external 项：运动控制器作为独立窗口入口，放在侧边栏最底部
const railFooterItems = computed<AppRailNavItem[]>(() => [
  { id: 'motion', label: t.value.motionControl, icon: 'AX', external: true }
])

const VALID_MAIN_PAGES = new Set(['dashboard', 'calibration', 'traversal', 'log'])

function handleRailSelect(id: string): void {
  if (!VALID_MAIN_PAGES.has(id)) return
  // 探针校准是付费模块：未解锁时不直接进入，先弹出验证码对话框。
  // 已解锁（localStorage 持久化）则直接放行，无感进入。
  if (id === 'calibration' && !licenseStore.isUnlocked) {
    showLicenseDialog.value = true
    return
  }
  activePage.value = id as MainShellPage
}

// 许可证对话框：验证码正确，已解锁——放行进入探针校准画面
function handleLicenseUnlocked(): void {
  activePage.value = 'calibration'
  feedbackStore.pushToast(t.value.calLicenseUnlockedSuccess || '探针校准模块已解锁', 'success')
}

// 许可证对话框：用户取消或关闭——保持原页面，不跳转
function handleLicenseCancel(): void {
  // 不切换 activePage，用户停留在当前页面
}

// 独立窗口启动中状态，避免重复点击
const motionLaunching = ref(false)

// 处理 external 导航项：启动运动控制器独立窗口
async function handleOpenExternal(id: string): Promise<void> {
  if (id !== 'motion') return
  if (!isWailsAvailable()) {
    feedbackStore.pushToast(t.value.app_independentWindowNotSupported, 'error')
    return
  }
  if (motionLaunching.value) return
  motionLaunching.value = true
  try {
    const res = await wailsApi.app.openMotionWindow()
    if (!res.Success) {
      feedbackStore.pushToast(t.value.app_openIndependentWindowFailed + ': ' + (res.Error || t.value.unknownError), 'error')
    }
  } catch (e) {
    feedbackStore.pushToast(t.value.app_openIndependentWindowException + ': ' + String(e), 'error')
  } finally {
    setTimeout(() => { motionLaunching.value = false }, 1000)
  }
}

function setViewMode(mode: 'overview' | 'chart' | 'table' | 'both'): void {
  viewMode.value = mode
}

async function ensureProfile() {
  await deviceStore.refreshProfiles()
  if (!deviceStore.selectedDeviceId && deviceStore.profiles.length) {
    deviceStore.selectDevice(deviceStore.profiles[0].id)
  }
}

// 启动时自动连接：遍历所有设备配置，对 autoConnect=true 的设备并行发起连接。
// 单个设备连接失败不阻塞其他设备，错误记录到控制台供排查。
// 注意：此处通过 deviceStore.connect 走完整的 store → api → wails binding → usecase → adapter 链路，
// 不直接访问硬件，符合分层约束。
async function autoConnectProfiles() {
  const targets = deviceStore.profiles.filter((p) => p.autoConnect)
  if (targets.length === 0) return
  const results = await Promise.allSettled(
    targets.map((p) => deviceStore.connect(p.id)),
  )
  results.forEach((result, idx) => {
    if (result.status === 'rejected') {
      const profile = targets[idx]
      console.warn(
        `[autoConnect] 设备 "${profile.name}"(${profile.id}) 启动自动连接失败:`,
        result.reason instanceof Error ? result.reason.message : String(result.reason),
      )
    }
  })
}

async function run(action: () => Promise<void>) {
  busy.value = true
  error.value = ''
  try {
    await action()
  } catch (err) {
    error.value = err instanceof Error ? err.message : String(err)
    feedbackStore.pushToast(error.value, 'error')
  } finally {
    busy.value = false
  }
}

async function start(): Promise<void> {
  await run(async () => {
    await ensureProfile()
    // 采集编排逻辑委托给 store 层
    await deviceStore.startAllAcquisitions()
    // autoStart 逻辑已移至后端 DeviceStartAcquisition：
    // 后端读取 storage-settings 配置，若 autoStartOnAcquisition=true
    // 且当前未在录制则自动调用 StorageRecorder.Start。
    // 前端只通过 refreshStorageStatus 轮询观察 recording 状态。
  })
}

async function stop(): Promise<void> {
  await run(async () => {
    // 停止采集编排逻辑委托给 store 层
    await deviceStore.stopAllAcquisitions()
    // 如果记录正在运行，同步停止记录
    if (isRecording.value) {
      await toggleRecording()
    }
  })
}

// refreshStorageStatus 拉取后端 Status() 并同步本地状态。
// 同时检测：
//   - recording 从 true -> false（sink 自停止）：弹出提示
//   - lastError 出现新值：弹出错误 toast
async function refreshStorageStatus(): Promise<void> {
  try {
    const status = await storageApi.status()
    const wasRecording = isRecording.value
    isRecording.value = status.recording
    recordingStatus.value = {
      outputDir: status.outputDir,
      currentFile: status.currentFile,
      fileSize: status.fileSize,
      fileCount: status.fileCount,
      recordCount: status.recordCount,
      durationMs: status.durationMs,
      droppedCount: status.droppedCount,
      lastError: status.lastError,
    }
    // 检测 sink 自停止：上次还在录制，这次状态显示已停止
    if (wasRecording && !status.recording) {
      const reason = status.lastError
        ? `${t.value.app_recordingStoppedWithError}${status.lastError}`
        : t.value.app_recordingStoppedAuto
      feedbackStore.pushToast(reason, status.lastError ? 'error' : 'info')
    }
    // 检测新增错误（仍在录制但出现 I/O 错误，例如队列丢弃告警）
    if (status.lastError && status.lastError !== lastReportedError) {
      lastReportedError = status.lastError
      feedbackStore.pushToast(`${t.value.app_recordingError}${status.lastError}`, 'error')
    } else if (!status.lastError) {
      // 错误已恢复，重置去重标记
      lastReportedError = ''
    }
  } catch {
    // Storage status is non-critical for the main shell.
  }
}

// buildRecordingConfig 从 storageStore.settings 构造完整 RecordingConfig。
// 路径解析由后端 StorageStartRecording 统一完成（前端只传用户输入的原始路径）。
function buildRecordingConfig() {
  const s = storageStore.settings
  return {
    outputDir: s.baseDirectory || 'data/recordings',
    filePrefix: s.filePrefix || 'run',
    stopConditions: { ...s.stopConditions },
    fileRotation: { ...s.fileRotation },
    autoStartOnAcquisition: s.autoStartOnAcquisition,
  }
}

async function toggleRecording(): Promise<void> {
  try {
    if (isRecording.value) {
      // 先置 false 再 await stop()：避免 2s 轮询 refreshStorageStatus 在 stop()
      // 进行中触发"wasRecording=true && status.recording=false"，误报"录制已停止（达到自动停止条件）"。
      isRecording.value = false
      await storageApi.stop()
      feedbackStore.pushToast(t.value.stoppedRecording, 'success')
      return
    }

    // 传递完整 config：stopConditions/fileRotation 由 sink writerLoop 评估
    await storageApi.start(buildRecordingConfig())
    isRecording.value = true
    lastReportedError = ''
    feedbackStore.pushToast(t.value.startedRecording, 'success')
    // 立即拉取一次状态，确保 UI 反映后端真实状态
    await refreshStorageStatus()
  } catch (err) {
    const message = err instanceof Error ? err.message : String(err)
    // 停止分支已先把 isRecording 置 false；若 stop() 失败需回滚，否则 UI 与后端状态错位，
    // 且下方的消息分支判断会误走"启动记录失败"。
    // 此处用操作意图区分消息，不依赖 isRecording.value 的中间态。
    const wasStopping = !isRecording.value
    if (wasStopping) {
      // stop() 失败：恢复录制状态标记，后端可能仍在录制
      isRecording.value = true
    }
    feedbackStore.pushToast(
      wasStopping
        ? t.value.failedToStopRecording + ': ' + message
        : t.value.failedToStartRecording + ': ' + message,
      'error',
    )
  }
}

onMounted(async () => {
  try {
    const result = await wailsApi.app.getVersion()
    appVersion.value = result.version
  } catch {
    appVersion.value = '0.1.0-dev'
  }
  await run(ensureProfile)
  unsubscribeDeviceSnapshots = deviceStore.attachStatusListener()
  await storageStore.loadSettings()
  // 启动时把持久化的刷新率下发后端 AcquisitionHub（后端重启后恒为默认 20Hz），
  // 并同步前端轮询间隔。失败不阻塞主流程——退回默认 20Hz 仍可用。
  try {
    await deviceApi.setPublishRate(storageStore.settings.refreshRateHz)
  } catch {
    // ignore：刷新率恢复失败不影响其他功能
  }
  await refreshStorageStatus()
  initialLoading.value = false
  // 注册键盘快捷键
  window.addEventListener('keydown', handleKeydown)
  // 启动时对所有 autoConnect=true 的设备并行发起连接，失败不阻塞主流程
  void autoConnectProfiles()
  // 启动录制状态轮询：2s 间隔平衡响应性与 HTTP 开销。
  // 用于检测后端 sink 自停止（达到条件或 I/O 错误）并反馈到 UI。
  recordingStatusTimer = setInterval(() => { void refreshStorageStatus() }, 2000)
})

onBeforeUnmount(() => {
  unsubscribeDeviceSnapshots?.()
  unsubscribeDeviceSnapshots = null
  window.removeEventListener('keydown', handleKeydown)
  if (recordingStatusTimer !== null) {
    clearInterval(recordingStatusTimer)
    recordingStatusTimer = null
  }
})

// 键盘快捷键处理：空格键开始/停止采集
function handleKeydown(e: KeyboardEvent) {
  // 忽略输入框内的按键
  const target = e.target as HTMLElement
  if (target && (target.tagName === 'INPUT' || target.tagName === 'TEXTAREA' || target.isContentEditable)) {
    return
  }
  if (e.code === 'Space') {
    e.preventDefault()
    if (acquiring.value) {
      void stop()
    } else {
      void start()
    }
  }
}
</script>

<template>
  <MainView class="main-dashboard-view">
    <template #header>
      <MainTopBar
        :version="appVersion"
        :is-acquiring="acquiring"
        :is-recording="isRecording"
        :active-page="activePage"
        :view-mode="viewMode"
        :t="t"
        @set-view-mode="setViewMode"
        @start="start"
        @stop="stop"
        @toggle-recording="toggleRecording"
      />
    </template>

    <template #rail>
      <AppRailNav
        :items="railItems"
        :footer-items="railFooterItems"
        :t="t"
        @select="handleRailSelect"
        @open-external="handleOpenExternal"
        @open-settings="showSettings = true"
      />
    </template>

    <template v-if="activePage === 'dashboard'" #sidebar>
      <DeviceSidebar :t="i18n.t" @open-manage="showDeviceDrawer = true" />
    </template>

    <div v-if="activePage === 'dashboard'" class="main-dashboard-stage">
      <UiLoadingState v-if="initialLoading" :loading="true" :text="t.app_loadingDevices" />

      <template v-else>
        <UiAlert v-if="error" type="error" :closable="true" class="mb-3" @close="error = ''">
          {{ error }}
        </UiAlert>

        <DeviceOverviewPanel v-if="viewMode === 'overview'" />

        <DeviceDetailPanel
          v-else-if="deviceStore.selectedProfile"
          :mode="viewMode === 'both' ? 'both' : viewMode"
      />

      <UiEmptyState
        v-else
        :title="t.app_selectDeviceTitle"
        :description="t.selectDevicePrompt"
      >
        <template #action>
          <UiButton size="sm" variant="primary" @click="showDeviceDrawer = true">
            {{ t.openDeviceManager }}
          </UiButton>
        </template>
      </UiEmptyState>
      </template>
    </div>

    <!-- 统一全铺布局：所有子页面都直接铺满主内容区，保持与仪表盘一致的视觉体验 -->
    <div v-else class="page-fullscreen">
      <CalibrationView v-if="activePage === 'calibration'" embedded />
      <TraversalView v-else-if="activePage === 'traversal'" embedded />
      <LogViewer v-else-if="activePage === 'log'" embedded />
    </div>

    <template v-if="activePage === 'dashboard'" #statusbar>
      <MainBottomBar
        :is-acquiring="acquiring"
        :t="t"
        :total-devices="deviceStore.profiles?.length ?? 0"
        :is-recording="isRecording"
        :recording-stats="recordingStatus"
      />
    </template>

    <DeviceManagementDrawer v-model:open="showDeviceDrawer" />
    <GlobalSettingsModal v-model:open="showSettings" @close="showSettings = false" />
    <!-- 探针校准付费模块解锁对话框：仅在未解锁时由 handleRailSelect 触发 -->
    <CalibrationLicenseDialog
      v-model:show="showLicenseDialog"
      :t="t"
      @unlocked="handleLicenseUnlocked"
      @cancel="handleLicenseCancel"
    />
  </MainView>
</template>

<style scoped>
.main-dashboard-view {
  background: var(--bg-canvas);
}

:root[data-theme='light'] .main-dashboard-view {
  background: var(--bg-canvas);
}

.main-dashboard-stage {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  padding: var(--space-2);
}

:root[data-theme='light'] .main-dashboard-stage {
  background: transparent;
}

.page-fullscreen {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  padding: var(--space-2);
  background: transparent;
  display: flex;
  flex-direction: column;
}
</style>
