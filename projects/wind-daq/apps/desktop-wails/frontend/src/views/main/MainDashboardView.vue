<script setup lang="ts">
import { computed, ref, onMounted, onBeforeUnmount } from 'vue'
import DeviceDetailPanel from '@components/main/DeviceDetailPanel.vue'
import DeviceOverviewPanel from '@components/main/DeviceOverviewPanel.vue'
import { useDeviceStore } from '@stores/deviceStore'
import { useFeedbackStore } from '@stores/feedbackStore'
import { deviceApi } from '@api/deviceApi'
import { subscribeDaqStream, type SseSubscription } from '@api/sse-client'

type DashboardMode = 'overview' | 'chart' | 'table' | 'both'

const props = withDefaults(
  defineProps<{
    dashboardMode?: DashboardMode
  }>(),
  { dashboardMode: 'both' },
)

const deviceStore = useDeviceStore()
const feedback = useFeedbackStore()

const busy = ref(false)
const error = ref('')
let sseSub: SseSubscription | null = null

const acquiring = computed(() => {
  const id = deviceStore.selectedDeviceId
  return id ? deviceStore.acquiringFor(id) : false
})

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
    feedback.pushToast(error.value, 'error')
  } finally {
    busy.value = false
  }
}

async function connectAndStart() {
  await run(async () => {
    await ensureProfile()
    const id = deviceStore.selectedDeviceId ?? 'sim-1'
    await deviceApi.connect(id)
    await deviceApi.startAcquisition(id)
    await deviceStore.refreshStatusFor(id)
    subscribeStream(id)
  })
}

async function stopAcq() {
  await run(async () => {
    const id = deviceStore.selectedDeviceId ?? 'sim-1'
    await deviceApi.stopAcquisition(id)
    await deviceStore.refreshStatusFor(id)
    unsubscribeStream()
  })
}

function subscribeStream(id: string) {
  unsubscribeStream()
  sseSub = subscribeDaqStream(
    id,
    (payload) => { deviceStore.pushSnapshot(payload) },
    (msg) => {
      if (msg !== 'connected') error.value = msg
    },
  )
}

function unsubscribeStream() {
  if (sseSub) {
    sseSub.unsubscribe()
    sseSub = null
  }
}

onMounted(() => {
  void run(ensureProfile)
})

onBeforeUnmount(() => {
  unsubscribeStream()
})
</script>

<template>
  <div class="dashboard-stage">
    <p v-if="error" class="error-text">{{ error }}</p>

    <DeviceOverviewPanel v-if="props.dashboardMode === 'overview'" />
    <DeviceDetailPanel v-else :mode="props.dashboardMode">
      <template #actions>
        <button class="primary" :disabled="busy" @click="connectAndStart">
          {{ acquiring ? '采集中' : '连接 + 开始' }}
        </button>
        <button class="danger" :disabled="busy || !acquiring" @click="stopAcq">
          停止
        </button>
      </template>
    </DeviceDetailPanel>
  </div>
</template>

<style scoped>
.dashboard-stage {
  display: flex;
  flex-direction: column;
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  padding: var(--space-4);
}

.error-text {
  margin-bottom: var(--space-3);
  color: var(--accent-danger);
  font: 700 0.75rem/1.4 var(--font-family-mono, monospace);
}

.primary,
.danger {
  min-height: 34px;
  padding: 0 0.9rem;
  border-radius: 0.5rem;
  color: #f8fbff;
  font-size: 0.8rem;
  font-weight: 800;
}

.primary {
  background: var(--accent-success);
}

.danger {
  background: color-mix(in srgb, var(--accent-danger) 12%, transparent);
  color: var(--accent-danger);
  border: 1px solid color-mix(in srgb, var(--accent-danger) 25%, transparent);
}
</style>
