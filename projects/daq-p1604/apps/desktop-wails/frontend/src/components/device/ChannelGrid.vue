<script setup lang="ts">
import { computed } from 'vue'
import { useDeviceStore } from '@stores/deviceStore'
import ChannelCard from './ChannelCard.vue'

defineProps<{ deviceId: string }>()

const deviceStore = useDeviceStore()

// 全局精度：作为单通道精度未设置时的回退值
const globalPrecision = computed(() => deviceStore.selectedProfile?.p1604Config?.precision ?? 3)

// 18 通道颜色方案
const COLORS = [
  '#3b82f6', '#10b981', '#f59e0b', '#a855f7',
  '#f43f5e', '#06b6d4', '#f97316', '#6366f1',
  '#84cc16', '#14b8a6', '#d946ef', '#0ea5e9',
  '#eab308', '#22c55e', '#ef4444', '#8b5cf6',
  '#ec4899', '#64748b',
]
</script>

<template>
  <div class="grid">
    <template v-for="ch in (deviceStore.selectedProfile?.channels ?? [])" :key="ch.index">
      <ChannelCard
        v-if="ch.enabled"
        :index="ch.index"
        :value="deviceStore.renderedSnapshotMap[deviceId]?.values[ch.index] ?? NaN"
        :unit="ch.unit"
        :color="ch.color || COLORS[ch.index % COLORS.length]"
        :name="ch.name"
        :precision="ch.precision ?? globalPrecision"
        :active="deviceStore.isChartSelected(deviceId, ch.index)"
        @change-color="(color: string) => deviceStore.updateChannel(deviceId, ch.index, { color })"
      />
    </template>
  </div>
</template>

<style scoped>
.grid {
  display: grid;
  grid-template-columns: repeat(9, 1fr);
  gap: 0.45rem;
}

@media (max-width: 1399px) {
  .grid {
    grid-template-columns: repeat(6, 1fr);
  }
}

@media (max-width: 1199px) {
  .grid {
    grid-template-columns: repeat(4, 1fr);
  }
}

@media (max-width: 767px) {
  .grid {
    grid-template-columns: repeat(3, 1fr);
    gap: 0.4rem;
  }
}
</style>
