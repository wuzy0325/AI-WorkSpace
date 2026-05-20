<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { api, defaultSimulatedProfile, type DataPayload, type DeviceProfile, type DeviceStatus } from './api'

const profile = ref<DeviceProfile>(defaultSimulatedProfile())
const status = ref<DeviceStatus | null>(null)
const latest = ref<DataPayload | null>(null)
const busy = ref(false)
const error = ref('')
let pollTimer: number | undefined

const acquiring = computed(() => status.value?.acquiring ?? false)
const connectionLabel = computed(() => status.value?.connection ?? 'Disconnected')
const timestampLabel = computed(() => {
  if (!latest.value?.timestamp) return '--'
  return new Date(latest.value.timestamp).toLocaleTimeString()
})

async function ensureProfile() {
  const profiles = await api.getProfiles()
  const existing = profiles.find((item) => item.id === profile.value.id)
  if (existing) {
    profile.value = existing
    return
  }
  await api.upsertProfile(profile.value)
}

async function connectAndStart() {
  await run(async () => {
    await ensureProfile()
    await api.connect(profile.value.id)
    await api.startAcquisition(profile.value.id)
    await refresh()
    startPolling()
  })
}

async function stop() {
  await run(async () => {
    await api.stopAcquisition(profile.value.id)
    await refresh()
  })
}

async function refresh() {
  try {
    status.value = await api.getStatus(profile.value.id)
  } catch {
    status.value = null
  }
  latest.value = await api.getLatest(profile.value.id)
}

async function run(action: () => Promise<void>) {
  busy.value = true
  error.value = ''
  try {
    await action()
  } catch (err) {
    error.value = err instanceof Error ? err.message : String(err)
  } finally {
    busy.value = false
  }
}

function startPolling() {
  if (pollTimer !== undefined) return
  pollTimer = window.setInterval(() => {
    void refresh()
  }, 250)
}

onMounted(() => {
  void run(async () => {
    await ensureProfile()
    await refresh()
  })
})

onUnmounted(() => {
  if (pollTimer !== undefined) window.clearInterval(pollTimer)
})
</script>

<template>
  <main class="app-shell">
    <header class="top-bar">
      <div class="brand-mark">WDAQ</div>
      <div>
        <p class="eyebrow">Wind Tunnel Acquisition</p>
        <h1>Wind-DAQ Rebuild Console</h1>
      </div>
      <div class="status-pill" :class="{ live: acquiring }">
        <span class="status-dot" />
        {{ acquiring ? 'Acquiring' : connectionLabel }}
      </div>
    </header>

    <section class="workbench">
      <aside class="side-rail">
        <span class="rail-item active">DAQ</span>
        <span class="rail-item">MOT</span>
        <span class="rail-item">CAL</span>
        <span class="rail-item">TRV</span>
      </aside>

      <section class="canvas">
        <div class="hero-panel">
          <div>
            <p class="eyebrow">No-hardware validation path</p>
            <h2>Simulated acquisition loop</h2>
            <p class="muted">Profiles, connect, acquisition and latest data are served by the new Go backend.</p>
          </div>
          <div class="actions">
            <button class="primary" :disabled="busy" @click="connectAndStart">Connect + Start</button>
            <button class="secondary" :disabled="busy" @click="stop">Stop</button>
          </div>
        </div>

        <p v-if="error" class="error-text">{{ error }}</p>

        <div class="grid">
          <article class="metric-card">
            <span>Device</span>
            <strong>{{ profile.name }}</strong>
            <small>{{ profile.type }} / {{ profile.samplingRate }} Hz</small>
          </article>
          <article class="metric-card">
            <span>Connection</span>
            <strong>{{ connectionLabel }}</strong>
            <small>{{ status?.id ?? profile.id }}</small>
          </article>
          <article class="metric-card">
            <span>Last sample</span>
            <strong>{{ timestampLabel }}</strong>
            <small>{{ latest?.channels.length ?? 0 }} active channels</small>
          </article>
        </div>

        <section class="channel-panel">
          <div class="panel-heading">
            <p class="eyebrow">Realtime channels</p>
            <h3>Latest payload</h3>
          </div>
          <div class="channels">
            <div v-for="channel in profile.channels" :key="channel.index" class="channel-card">
              <span>{{ channel.name }}</span>
              <strong>
                {{ latest?.channels[channel.index]?.toFixed(channel.precision) ?? '--' }}
              </strong>
              <small>{{ channel.unit }}</small>
            </div>
          </div>
        </section>
      </section>
    </section>
  </main>
</template>
