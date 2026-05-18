<script setup lang="ts">
import { computed } from 'vue'
import type { Status, UiSampleFrame } from '../api/wails'

const props = defineProps<{ status: Status; frame: UiSampleFrame }>()

const metrics = computed(() => [
  { label: 'Batches', value: props.status.batchCount.toLocaleString() },
  { label: 'Samples', value: props.status.sampleCount.toLocaleString() },
  { label: 'Rate', value: `${props.status.sampleRateHz} Hz` },
  { label: 'Seq', value: props.frame.sequenceStart.toLocaleString() },
])

const channelColors = ['var(--ch0)', 'var(--ch1)', 'var(--ch2)', 'var(--ch3)']
</script>

<template>
  <div class="metrics-row">
    <div v-for="m in metrics" :key="m.label" class="metric-card">
      <div class="metric-label">{{ m.label }}</div>
      <div class="metric-value">{{ m.value }}</div>
    </div>

    <!-- Per-channel latest values -->
    <div
      v-for="(ch, i) in props.status.latestValues"
      :key="'ch' + i"
      class="metric-card channel-card"
    >
      <div class="metric-label">CH{{ i }}</div>
      <div class="metric-value" :style="{ color: channelColors[i] }">
        {{ ch >= 0 ? '+' : '' }}{{ ch.toFixed(4) }}
      </div>
    </div>
  </div>
</template>

<style scoped>
.metrics-row {
  display: flex;
  gap: 8px;
  flex-shrink: 0;
  flex-wrap: wrap;
}

.metric-card {
  flex: 1;
  min-width: 120px;
  background: var(--bg-panel);
  border: 1px solid var(--border-default);
  border-radius: 3px;
  padding: 10px 14px;
  text-align: center;
}

.metric-label {
  font-size: 10px;
  font-weight: 600;
  text-transform: uppercase;
  letter-spacing: 0.08em;
  color: var(--text-muted);
  margin-bottom: 4px;
}

.metric-value {
  font-family: var(--font-mono);
  font-size: 18px;
  font-weight: 800;
  color: var(--text-primary);
  font-variant-numeric: tabular-nums;
}

.channel-card .metric-value {
  font-size: 16px;
}
</style>
