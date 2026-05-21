<script setup lang="ts">
import { computed } from 'vue'
import { Activity, RadioTower, Gauge, Waves } from '@lucide/vue'
import { useDeviceStore } from '@stores/deviceStore'
import type { DeviceProfile } from '@api/types'

const deviceStore = useDeviceStore()

const devices = computed(() => deviceStore.profiles)
const selectedId = computed(() => deviceStore.selectedDeviceId)

function latestValue(profile: DeviceProfile): string {
  const snapshot = deviceStore.latestFor(profile.id)
  if (!snapshot?.channels.length) return '--'
  return deviceStore.formatValue(profile.id, snapshot.channelIndices[0] ?? 0, snapshot.channels[0])
}

function latestUnit(profile: DeviceProfile): string {
  const channelIndex = deviceStore.latestFor(profile.id)?.channelIndices[0] ?? 0
  return profile.channels.find((ch) => ch.index === channelIndex)?.unit ?? profile.channels[0]?.unit ?? ''
}

function historyCount(id: string): number {
  return deviceStore.historyFor(id).length
}

function selectDevice(id: string) {
  deviceStore.selectDevice(id)
}
</script>

<template>
  <section class="overview-panel">
    <header class="overview-panel__head">
      <div>
        <p class="overview-panel__eyebrow">Fleet Overview</p>
        <h2>设备总览</h2>
      </div>
      <div class="overview-panel__metrics">
        <span><RadioTower class="overview-panel__icon" />{{ devices.length }} devices</span>
        <span><Waves class="overview-panel__icon" />{{ deviceStore.latestSnapshots.length }} live</span>
      </div>
    </header>

    <div v-if="devices.length" class="overview-grid">
      <button
        v-for="profile in devices"
        :key="profile.id"
        type="button"
        class="overview-card"
        :class="{ 'overview-card--active': profile.id === selectedId }"
        @click="selectDevice(profile.id)"
      >
        <div class="overview-card__top">
          <div>
            <p class="overview-card__type">{{ profile.type }}</p>
            <h3>{{ profile.name }}</h3>
          </div>
          <span class="overview-card__status">{{ deviceStore.statusFor(profile.id) }}</span>
        </div>

        <div class="overview-card__value">
          <strong>{{ latestValue(profile) }}</strong>
          <small>{{ latestUnit(profile) }}</small>
        </div>

        <div class="overview-card__meta">
          <span><Gauge class="overview-panel__icon" />{{ profile.samplingRate }} Hz</span>
          <span><Activity class="overview-panel__icon" />{{ historyCount(profile.id) }} samples</span>
          <span>{{ profile.channels.length }} channels</span>
        </div>
      </button>
    </div>

    <div v-else class="overview-empty">
      <RadioTower class="overview-empty__icon" />
      <h3>暂无设备配置</h3>
      <p>打开设备管理添加或扫描 simulated device 后，总览会显示实时状态。</p>
    </div>
  </section>
</template>

<style scoped>
.overview-panel {
  border-radius: 1rem;
  background: rgba(15, 23, 42, 0.55);
  border: 1px solid rgba(255, 255, 255, 0.08);
  box-shadow: 0 25px 50px rgba(0, 0, 0, 0.25);
  overflow: hidden;
}

.overview-panel__head {
  min-height: 76px;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 1rem;
  padding: 1rem 1.25rem;
  border-bottom: 1px solid rgba(255, 255, 255, 0.06);
}

.overview-panel__eyebrow,
.overview-card__type {
  margin: 0 0 0.3rem;
  color: #64748b;
  font-size: 0.65rem;
  font-weight: 800;
  letter-spacing: 0.1em;
  text-transform: uppercase;
}

.overview-panel__head h2,
.overview-card h3 {
  margin: 0;
}

.overview-panel__metrics,
.overview-card__meta {
  display: flex;
  align-items: center;
  gap: 0.75rem;
  color: #94a3b8;
  font: 700 0.72rem/1 var(--font-family-mono, monospace);
}

.overview-panel__metrics span,
.overview-card__meta span {
  display: inline-flex;
  align-items: center;
  gap: 0.3rem;
}

.overview-panel__icon {
  width: 0.9rem;
  height: 0.9rem;
}

.overview-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(260px, 1fr));
  gap: 0.9rem;
  padding: 1rem;
}

.overview-card {
  display: flex;
  flex-direction: column;
  gap: 1rem;
  min-height: 190px;
  padding: 1rem;
  text-align: left;
  border-radius: 0.85rem;
  color: inherit;
  background: rgba(30, 41, 59, 0.5);
  border: 1px solid rgba(255, 255, 255, 0.08);
  transition: border-color 0.2s ease, transform 0.2s ease, background 0.2s ease;
}

.overview-card:hover,
.overview-card--active {
  border-color: color-mix(in srgb, var(--accent-success) 55%, transparent);
  background: rgba(16, 185, 129, 0.08);
  transform: translateY(-1px);
}

.overview-card__top {
  display: flex;
  justify-content: space-between;
  gap: 1rem;
}

.overview-card__status {
  height: fit-content;
  padding: 0.25rem 0.5rem;
  border-radius: 999px;
  color: var(--accent-success);
  background: rgba(16, 185, 129, 0.1);
  font: 800 0.62rem/1 var(--font-family-mono, monospace);
  text-transform: uppercase;
}

.overview-card__value {
  display: flex;
  align-items: baseline;
  gap: 0.45rem;
}

.overview-card__value strong {
  color: var(--accent-success);
  font: 900 2.4rem/1 var(--font-family-mono, monospace);
  text-shadow: 0 0 12px color-mix(in srgb, var(--accent-success) 45%, transparent);
}

.overview-card__value small {
  color: #64748b;
  font-weight: 800;
}

.overview-card__meta {
  flex-wrap: wrap;
  margin-top: auto;
}

.overview-empty {
  min-height: 320px;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 0.5rem;
  padding: 2rem;
  color: #94a3b8;
  text-align: center;
}

.overview-empty h3,
.overview-empty p {
  margin: 0;
}

.overview-empty__icon {
  width: 2.5rem;
  height: 2.5rem;
  color: var(--accent-success);
}
</style>
