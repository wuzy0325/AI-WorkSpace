<script setup lang="ts">
import { computed, ref, onMounted, onBeforeUnmount } from 'vue'
import { storeToRefs } from 'pinia'
import { wailsApi, isWailsAvailable } from '@api/wails-adapter'
import type { AppRailNavItem } from '@components/layout/AppRailNav.vue'
import AppRailNav from '@components/layout/AppRailNav.vue'
import DeviceManagementDrawer from '@components/device/DeviceManagementDrawer.vue'
import GlobalSettingsModal from '@components/layout/GlobalSettingsModal.vue'
import MainTopBar from '@components/layout/MainTopBar.vue'
import MainBottomBar from '@components/layout/MainBottomBar.vue'
import DeviceSidebar from '@components/main/DeviceSidebar.vue'
import DeviceOverviewPanel from '@components/main/DeviceOverviewPanel.vue'
import DeviceDetailPanel from '@components/main/DeviceDetailPanel.vue'
import MainView from '@views/MainView.vue'
import MotionView from '@views/MotionView.vue'
import CalibrationView from '@views/CalibrationView.vue'
import TraversalView from '@views/TraversalView.vue'
import LogViewer from '@views/LogViewer.vue'
import { useDeviceStore } from '@stores/deviceStore'
import { useI18nStore } from '@stores/i18nStore'
import { useFeedbackStore } from '@stores/feedbackStore'
import { useStorageStore } from '@stores/storageStore'
import { deviceApi, storageApi } from '@api/deviceApi'
import { subscribeDaqStream } from '@api/sse-client'

type MainShellPage = 'dashboard' | 'motion' | 'calibration' | 'traversal' | 'log'

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
let sseSub: { unsubscribe: () => void } | null = null
let wailsUnsub: (() => void) | null = null
let unsubscribeDeviceSnapshots: (() => void) | null = null

const acquiring = computed(() => {
  const id = deviceStore.selectedDeviceId
  return id ? deviceStore.acquiringFor(id) : false
})

const railItems = computed<AppRailNavItem[]>(() => [
  { id: 'dashboard', label: t.value.dashboardHome, icon: 'IO', active: activePage.value === 'dashboard' },
  { id: 'motion', label: t.value.motionControl, icon: 'AX', active: activePage.value === 'motion' },
  { id: 'calibration', label: t.value.probeCalibration, icon: 'CP', active: activePage.value === 'calibration' },
  { id: 'traversal', label: t.value.traversalTest, icon: 'TR', active: activePage.value === 'traversal' },
  { id: 'log', label: t.value.logViewer || 'Logs', icon: 'LG', active: activePage.value === 'log' }
])

const VALID_MAIN_PAGES = new Set(['dashboard', 'motion', 'calibration', 'traversal', 'log'])

function handleRailSelect(id: string): void {
  if (VALID_MAIN_PAGES.has(id)) {
    activePage.value = id as MainShellPage
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
    const id = deviceStore.selectedDeviceId ?? 'sim-1'
    await deviceStore.connect(id)
    await deviceStore.startAcquisition(id)
    // 采集启动后主动刷新实例状态，采集按钮立即响应
    await deviceStore.refreshInstances()
    // 检查 autoStartOnAcquisition 配置，自动开始记录
    if (storageStore.settings.autoStartOnAcquisition && !isRecording.value) {
      // 从全局设置同步保存目录和文件前缀，确保使用用户配置的值
      recordingOutputDir.value = storageStore.settings.baseDirectory || 'data/recordings'
      recordingFilePrefix.value = storageStore.settings.filePrefix || 'run'
      await toggleRecording()
    }
  })
}

async function stop(): Promise<void> {
  await run(async () => {
    const id = deviceStore.selectedDeviceId ?? 'sim-1'
    await deviceStore.stopAcquisition(id)
    // 停止采集后也刷新实例状态，按钮同步更新
    await deviceStore.refreshInstances()
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

function subscribeStream(id: string) {
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
}

function unsubscribeStream() {
  if (sseSub) {
    sseSub.unsubscribe()
    sseSub = null
  }
  if (wailsUnsub) {
    wailsUnsub()
    wailsUnsub = null
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
})

onBeforeUnmount(() => {
  unsubscribeStream()
  unsubscribeDeviceSnapshots?.()
  unsubscribeDeviceSnapshots = null
})
</script>

<template>
  <MainView class="main-dashboard-view">
    <template #header>
      <MainTopBar
        :locale="locale"
        :version="appVersion"
        :is-acquiring="acquiring"
        :active-page="activePage"
        :view-mode="viewMode"
        :t="t"
        @set-locale="i18n.setLocale"
        @set-view-mode="setViewMode"
      />
    </template>

    <template #rail>
      <AppRailNav :items="railItems" @select="handleRailSelect" @open-settings="showSettings = true" />
    </template>

    <template v-if="activePage === 'dashboard'" #sidebar>
      <DeviceSidebar @open-manage="showDeviceDrawer = true" />
    </template>

    <div v-if="activePage === 'dashboard'" class="main-dashboard-stage">
      <p v-if="error" class="error-text">{{ error }}</p>

      <DeviceOverviewPanel v-if="viewMode === 'overview'" />

      <DeviceDetailPanel
        v-else-if="deviceStore.selectedProfile"
        :mode="viewMode === 'both' ? 'both' : viewMode"
      />

      <section
        v-else
        data-test="dashboard-empty-state"
        class="dashboard-empty-state"
      >
        <div
          data-test="dashboard-empty-icon"
          class="dashboard-empty-icon"
        >
          <svg class="h-8 w-8" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5">
            <rect x="4" y="4" width="16" height="16" rx="2"/>
            <path d="M9 9h6v6H9z"/>
            <path d="M9 1v3M15 1v3M9 20v3M15 20v3M20 9h3M20 15h3M1 9h3M1 15h3"/>
          </svg>
        </div>
        <div data-test="dashboard-empty-title" class="dashboard-empty-title">{{ t.emptyDeviceSelectionTitle }}</div>
        <div class="dashboard-empty-desc">{{ t.selectDevicePrompt }}</div>
        <button
          data-test="dashboard-empty-action"
          class="dashboard-empty-btn"
          @click="showDeviceDrawer = true"
        >
          {{ t.openDeviceManager }}
        </button>
      </section>
    </div>

    <!-- 统一全铺布局：所有子页面都直接铺满主内容区，保持与仪表盘一致的视觉体验 -->
    <div v-else class="page-fullscreen">
      <MotionView v-if="activePage === 'motion'" embedded />
      <CalibrationView v-else-if="activePage === 'calibration'" embedded />
      <TraversalView v-else-if="activePage === 'traversal'" embedded />
      <LogViewer v-else-if="activePage === 'log'" embedded />
    </div>

    <template v-if="activePage === 'dashboard'" #statusbar>
      <MainBottomBar
        :is-acquiring="acquiring"
        :is-recording="isRecording"
        :t="t"
        :total-devices="deviceStore.profiles?.length ?? 0"
        @start="start"
        @stop="stop"
        @toggle-recording="toggleRecording"
      />
    </template>

    <DeviceManagementDrawer v-model:open="showDeviceDrawer" />
    <GlobalSettingsModal v-model:open="showSettings" @close="showSettings = false" />
  </MainView>
</template>

<style scoped>
.main-dashboard-view {
  background: radial-gradient(circle at top left, #1e293b 0%, #0f172a 100%);
}

:root[data-theme='light'] .main-dashboard-view {
  background: radial-gradient(circle at top left, #f8fafc 0%, #e2e8f0 100%);
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

/* 统一全铺布局样式：与仪表盘页面保持一致的视觉体验 */
.page-fullscreen {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  padding: var(--space-4);
  background: transparent;
}

.error-text {
  margin-bottom: var(--space-3);
  color: var(--accent-danger);
  font: 700 0.75rem/1.4 var(--font-family-mono, monospace);
}

.dashboard-empty-state {
  display: flex;
  height: 100%;
  min-height: 0;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  border-radius: 1rem;
  border: 1px dashed var(--border-default, #334155);
  background: var(--bg-panel, #172338);
  padding: 1.5rem;
  text-align: center;
}

:root[data-theme='light'] .dashboard-empty-state {
  border: 1px dashed rgba(0, 0, 0, 0.1);
  background: rgba(255, 255, 255, 0.6);
}

.dashboard-empty-icon {
  display: flex;
  height: 4rem;
  width: 4rem;
  align-items: center;
  justify-content: center;
  border-radius: 0.75rem;
  border: 1px solid rgba(255, 255, 255, 0.1);
  background: var(--bg-panel-strong, #1e293b);
  color: rgba(148, 163, 184, 0.8);
}

:root[data-theme='light'] .dashboard-empty-icon {
  border: 1px solid rgba(0, 0, 0, 0.1);
  background: rgba(255, 255, 255, 0.8);
  color: rgba(100, 116, 139, 0.8);
}

.dashboard-empty-title {
  margin-top: 1.5rem;
  font-size: 1rem;
  font-weight: 800;
  letter-spacing: 0.05em;
  color: var(--text-primary);
}

.dashboard-empty-desc {
  margin-top: 0.5rem;
  max-width: 36rem;
  font-size: 0.875rem;
  line-height: 1.5;
  color: var(--text-secondary);
}

.dashboard-empty-btn {
  margin-top: 1.5rem;
  padding: 0.5rem 1.5rem;
  border-radius: 0.5rem;
  background: #10b981;
  color: white;
  font-size: 0.875rem;
  font-weight: 600;
  transition: all 0.2s ease;
}

.dashboard-empty-btn:hover {
  background: #059669;
  transform: translateY(-1px);
}
</style>
