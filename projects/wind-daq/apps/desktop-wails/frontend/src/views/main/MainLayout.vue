<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { useDeviceStore } from '@stores/deviceStore'
import { useI18nStore } from '@stores/i18nStore'
import { useThemeStore } from '@stores/themeStore'
import { GetVersion } from '../../../wailsjs/go/backend/App'
import AppShell from '@components/layout/AppShell.vue'
import MainTopBar from '@components/layout/MainTopBar.vue'
import AppRailNav from '@components/layout/AppRailNav.vue'
import MainBottomBar from '@components/layout/MainBottomBar.vue'
import DeviceSidebar from '@components/main/DeviceSidebar.vue'
import DeviceManagementDrawer from '@components/device/DeviceManagementDrawer.vue'
import GlobalSettingsModal from '@components/layout/GlobalSettingsModal.vue'
import UiToastHost from '@components/feedback/UiToastHost.vue'
import UiConfirmDialog from '@components/feedback/UiConfirmDialog.vue'

const router = useRouter()
const route = useRoute()
const deviceStore = useDeviceStore()
const i18n = useI18nStore()
const themeStore = useThemeStore()

const showDeviceDrawer = ref(false)
const showSettings = ref(false)
const appVersion = ref('0.1.0')
const dashboardMode = ref<'overview' | 'chart' | 'table' | 'both'>('both')

onMounted(async () => {
  try { appVersion.value = (await GetVersion()).version } catch { appVersion.value = '0.1.0-dev' }
})

const acquiring = computed(() => {
  const id = deviceStore.selectedDeviceId
  return id ? deviceStore.acquiringFor(id) : false
})

const railItems = computed(() => [
  { id: 'dashboard', label: i18n.t.dashboardHome, icon: 'IO', active: route.name === 'dashboard' },
  { id: 'motion', label: i18n.t.motionControl, icon: 'AX', active: route.name === 'motion' },
  { id: 'calibration', label: i18n.t.probeCalibration, icon: 'CP', active: route.name === 'calibration' },
  { id: 'traversal', label: i18n.t.traversalTest, icon: 'TR', active: route.name === 'traversal' },
  { id: 'log', label: i18n.t.logViewer, icon: 'LG', active: route.name === 'log' },
  { id: 'storage', label: '存储', icon: 'ST', active: route.name === 'storage' },
])

const activePage = computed(() => route.name as 'dashboard' | 'motion' | 'calibration' | 'traversal' | 'log' | 'storage')

const routedViewProps = computed(() => (
  route.name === 'dashboard' ? { dashboardMode: dashboardMode.value } : {}
))

function navigateTo(id: string) {
  router.push({ name: id })
}

function setDashboardMode(mode: 'overview' | 'chart' | 'table' | 'both') {
  dashboardMode.value = mode
}
</script>

<template>
  <div class="app-root">
    <AppShell canvas-class="dashboard-ceramic-shell">
      <template #header>
        <MainTopBar
          :version="appVersion"
          :is-acquiring="acquiring"
          :active-page="activePage"
          :view-mode="dashboardMode"
          :locale="i18n.locale"
          :theme="themeStore.theme"
          :labels="i18n.t"
          @toggle-theme="themeStore.toggleTheme()"
          @set-locale="i18n.setLocale($event)"
          @set-view-mode="setDashboardMode"
        />
      </template>

      <template #rail>
        <AppRailNav :items="railItems" @select="navigateTo" @open-settings="showSettings = true" />
      </template>

      <template v-if="route.name === 'dashboard'" #sidebar>
        <DeviceSidebar @open-manage="showDeviceDrawer = true" />
      </template>

      <router-view v-slot="{ Component }">
        <component
          :is="Component"
          v-bind="routedViewProps"
        />
      </router-view>

      <template #statusbar>
        <MainBottomBar
          :is-acquiring="acquiring"
          :total-devices="deviceStore.profiles.length"
        />
      </template>
    </AppShell>

    <DeviceManagementDrawer v-model:open="showDeviceDrawer" />
    <GlobalSettingsModal v-model:open="showSettings" />
    <UiToastHost />
    <UiConfirmDialog />
  </div>
</template>

<style>
.app-root {
  height: 100vh;
  overflow: hidden;
}
</style>
