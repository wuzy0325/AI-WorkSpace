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
import { useDeviceStore } from '@stores/deviceStore'
import { useI18nStore } from '@stores/i18nStore'
import { useFeedbackStore } from '@stores/feedbackStore'
import { useStorageStore } from '@stores/storageStore'
import { deviceApi, storageApi } from '@api/deviceApi'
import { subscribeDaqStream } from '@api/sse-client'
import UiAlert from '@components/ui/UiAlert.vue'
import UiEmptyState from '@components/ui/UiEmptyState.vue'
import UiLoadingState from '@components/ui/UiLoadingState.vue'
import UiButton from '@components/ui/UiButton.vue'
import { Activity, Wifi, LineChart } from '@lucide/vue'

type MainShellPage = 'dashboard' | 'calibration' | 'traversal' | 'log'

const deviceStore = useDeviceStore()
const i18n = useI18nStore()
const feedbackStore = useFeedbackStore()
const storageStore = useStorageStore()
const { t, locale } = storeToRefs(i18n)

const activePage = ref<MainShellPage>('dashboard')
const appVersion = ref('0.1.0')
const showDeviceDrawer = ref(false)
const showSettings = ref(false)
const viewMode = ref<'overview' | 'chart' | 'table' | 'both'>('both')
const isRecording = ref(false)
const recordingOutputDir = ref('data/recordings')
const recordingFilePrefix = ref('run')
const busy = ref(false)
const error = ref('')
const initialLoading = ref(true)
let sseSub: { unsubscribe: () => void } | null = null
let wailsUnsub: (() => void) | null = null
let unsubscribeDeviceSnapshots: (() => void) | null = null

// 采集状态：委托 store 层判断（任一设备正在采集即为 true）
const acquiring = computed(() => deviceStore.isAnyAcquiring)

const railItems = computed<AppRailNavItem[]>(() => [
  { id: 'dashboard', label: t.value.dashboardHome, icon: 'IO', active: activePage.value === 'dashboard' },
  { id: 'calibration', label: t.value.probeCalibration, icon: 'CP', active: activePage.value === 'calibration' },
  { id: 'traversal', label: t.value.traversalTest, icon: 'TR', active: activePage.value === 'traversal' },
  { id: 'log', label: t.value.logViewer || 'Logs', icon: 'LG', active: activePage.value === 'log' }
])

// 底部 external 项：运动控制器作为独立窗口入口，放在侧边栏最底部
const railFooterItems = computed<AppRailNavItem[]>(() => [
  { id: 'motion', label: t.value.motionControl, icon: 'AX', external: true }
])

const VALID_MAIN_PAGES = new Set(['dashboard', 'calibration', 'traversal', 'log'])

function handleRailSelect(id: string): void {
  if (VALID_MAIN_PAGES.has(id)) {
    activePage.value = id as MainShellPage
  }
}

// 独立窗口启动中状态，避免重复点击
const motionLaunching = ref(false)

// 处理 external 导航项：启动运动控制器独立窗口
async function handleOpenExternal(id: string): Promise<void> {
  if (id !== 'motion') return
  if (!isWailsAvailable()) {
    feedbackStore.pushToast('当前环境不支持独立窗口', 'error')
    return
  }
  if (motionLaunching.value) return
  motionLaunching.value = true
  try {
    const res = await wailsApi.app.openMotionWindow()
    if (!res.Success) {
      feedbackStore.pushToast('启动独立窗口失败: ' + (res.Error || '未知错误'), 'error')
    }
  } catch (e) {
    feedbackStore.pushToast('启动独立窗口异常: ' + String(e), 'error')
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
    // 采集启动后检查 autoStartOnAcquisition 配置，自动开始记录
    if (storageStore.settings.autoStartOnAcquisition && !isRecording.value) {
      recordingOutputDir.value = storageStore.settings.baseDirectory || 'data/recordings'
      recordingFilePrefix.value = storageStore.settings.filePrefix || 'run'
      await toggleRecording()
    }
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

async function refreshStorageStatus(): Promise<void> {
  try {
    const status = await storageApi.status()
    isRecording.value = status.recording
    if (status.outputDir) recordingOutputDir.value = status.outputDir
    // 从全局设置同步文件前缀，确保手动录制也使用用户配置的值
    recordingFilePrefix.value = storageStore.settings.filePrefix || 'run'
  } catch {
    // Storage status is non-critical for the main shell.
  }
}

async function toggleRecording(): Promise<void> {
  try {
    if (isRecording.value) {
      await storageApi.stop()
      isRecording.value = false
      feedbackStore.pushToast(t.value.stoppedRecording || '已停止记录', 'success')
      return
    }

    await storageApi.start(recordingOutputDir.value, recordingFilePrefix.value)
    isRecording.value = true
    feedbackStore.pushToast(t.value.startedRecording || '已开始记录数据', 'success')
  } catch (err) {
    const message = err instanceof Error ? err.message : String(err)
    feedbackStore.pushToast(
      isRecording.value
        ? (t.value.failedToStopRecording || '停止记录失败') + ': ' + message
        : (t.value.failedToStartRecording || '启动记录失败') + ': ' + message,
      'error',
    )
  }
}

let isSubscribing = false

function subscribeStream(id: string) {
  // 防止重复订阅的竞态条件
  if (isSubscribing) return
  isSubscribing = true
  unsubscribeStream()
  if (isWailsAvailable()) {
    // Wails 桌面模式：使用 Wails Events 机制接收采集数据
    void wailsApi.device.subscribeStream(id, true)
    wailsUnsub = wailsApi.device.onPayload((payload) => {
      deviceStore.pushSnapshot({
        deviceId: payload.deviceId ?? '',
        timestamp: payload.timestamp ?? 0,
        channels: Array.isArray(payload.channels) ? payload.channels : [],
        channelIndices: Array.isArray(payload.channelIndices) ? payload.channelIndices : [],
      })
    })
  } else {
    // Web 模式：使用 HTTP SSE
    sseSub = subscribeDaqStream(
      id,
      (payload) => { deviceStore.pushSnapshot(payload) },
      (msg) => {
        if (msg !== 'connected') error.value = msg
      },
    )
  }
  isSubscribing = false
}

function unsubscribeStream() {
  if (sseSub) {
    sseSub.unsubscribe()
    sseSub = null
  }
  if (wailsUnsub) {
    const unsub = wailsUnsub
    wailsUnsub = null
    unsub()
    // 使用当前选中的设备ID取消订阅，避免依赖可能已变化的变量
    const id = deviceStore.selectedDeviceId
    if (id) {
      void wailsApi.device.subscribeStream(id, false)
    }
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
  await refreshStorageStatus()
  initialLoading.value = false
  // 注册键盘快捷键
  window.addEventListener('keydown', handleKeydown)
  // 启动时对所有 autoConnect=true 的设备并行发起连接，失败不阻塞主流程
  void autoConnectProfiles()
})

onBeforeUnmount(() => {
  unsubscribeStream()
  unsubscribeDeviceSnapshots?.()
  unsubscribeDeviceSnapshots = null
  window.removeEventListener('keydown', handleKeydown)
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
        @select="handleRailSelect"
        @open-external="handleOpenExternal"
        @open-settings="showSettings = true"
      />
    </template>

    <template v-if="activePage === 'dashboard'" #sidebar>
      <DeviceSidebar :t="i18n.t" @open-manage="showDeviceDrawer = true" />
    </template>

    <div v-if="activePage === 'dashboard'" class="main-dashboard-stage">
      <UiLoadingState v-if="initialLoading" :loading="true" text="正在加载设备..." />

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
        title="选择一个设备"
        :description="t.selectDevicePrompt || '请从左侧设备列表中选择一台设备开始监控'"
      >
        <template #action>
          <UiButton size="sm" variant="primary" @click="showDeviceDrawer = true">
            {{ t.openDeviceManager || '打开设备管理器' }}
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
      />
    </template>

    <DeviceManagementDrawer v-model:open="showDeviceDrawer" />
    <GlobalSettingsModal v-model:open="showSettings" @close="showSettings = false" />
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
  padding: var(--space-4);
}

:root[data-theme='light'] .main-dashboard-stage {
  background: transparent;
}

.page-fullscreen {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  padding: var(--space-4);
  background: transparent;
  display: flex;
  flex-direction: column;
}
</style>
