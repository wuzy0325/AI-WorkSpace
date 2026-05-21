<script setup lang="ts">
import { CheckCircle2 } from '@lucide/vue'
import { useDeviceStore } from '@stores/deviceStore'

const deviceStore = useDeviceStore()
const emit = defineEmits<{ (e: 'openManage'): void }>()
</script>

<template>
  <aside class="device-sidebar">
    <div class="device-sidebar__head">
      <span>设备列表</span>
      <button @click="emit('openManage')">管理</button>
    </div>
    <div class="device-sidebar__list">
      <div v-if="!deviceStore.profiles.length" class="device-sidebar__empty">
        暂无设备
      </div>
        <button
          v-for="p in deviceStore.profiles"
          :key="p.id"
          class="device-sidebar__item"
          :class="{ active: deviceStore.selectedDeviceId === p.id }"
          @click="deviceStore.selectDevice(p.id)"
        >
          <div>
            <strong>{{ p.name }}</strong>
            <small>{{ p.type }} · {{ p.channels?.length ?? 0 }} CH</small>
          </div>
          <div class="device-sidebar__status">
            <span class="device-sidebar__status-icon">
              <CheckCircle2 v-if="deviceStore.acquiringFor(p.id)" class="w-3 h-3 text-emerald-500" />
              <span v-else class="device-sidebar__dot" />
            </span>
            <span class="device-sidebar__status-text" :class="{ live: deviceStore.acquiringFor(p.id) }">
              {{ deviceStore.acquiringFor(p.id) ? 'ACQ' : 'OFF' }}
            </span>
          </div>
        </button>
    </div>
  </aside>
</template>

<style scoped>
.device-sidebar {
  width: var(--layout-sidebar-width, 220px);
  flex-shrink: 0;
  display: flex;
  flex-direction: column;
  border-right: 1px solid rgba(255, 255, 255, 0.05);
  background: color-mix(in srgb, var(--bg-panel) 96%, transparent);
}

.device-sidebar__head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: var(--space-4);
  border-bottom: 1px solid rgba(255, 255, 255, 0.05);
}

.device-sidebar__head span {
  color: var(--text-muted);
  font-size: 0.78rem;
  font-weight: 800;
  letter-spacing: 0.1em;
}

.device-sidebar__head button {
  padding: 0.3rem 0.7rem;
  border-radius: 0.5rem;
  background: rgba(255, 255, 255, 0.05);
  color: var(--text-muted);
  font-size: 0.75rem;
  font-weight: 700;
}

.device-sidebar__list {
  flex: 1;
  overflow-y: auto;
  padding: var(--space-3);
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
}

.device-sidebar__empty {
  padding: 2rem 1rem;
  text-align: center;
  color: var(--text-muted);
  font-size: 0.75rem;
}

.device-sidebar__item {
  padding: 0.75rem;
  display: flex;
  align-items: center;
  justify-content: space-between;
  border-radius: 0.75rem;
  background: rgba(30, 41, 59, 0.4);
  color: var(--text-primary);
  text-align: left;
  transition: all 0.2s ease;
}

.device-sidebar__item.active {
  border: 1px solid var(--accent-success);
  border-color: color-mix(in srgb, var(--accent-success) 30%, transparent);
  background: color-mix(in srgb, var(--accent-success) 8%, transparent);
}

.device-sidebar__item strong {
  display: block;
  font-size: 0.95rem;
}

.device-sidebar__item small {
  display: block;
  margin-top: 0.25rem;
  color: var(--text-muted);
  font-size: 0.72rem;
  font-weight: 700;
}

.device-sidebar__status {
  display: flex;
  align-items: center;
  gap: 0.35rem;
}

.device-sidebar__status-icon {
  display: flex;
  align-items: center;
}

.device-sidebar__status-text {
  font-size: 0.6rem;
  font-weight: 800;
  color: var(--text-muted);
  letter-spacing: 0.05em;
}

.device-sidebar__status-text.live {
  color: var(--accent-success);
}

.device-sidebar__dot {
  width: 7px;
  height: 7px;
  border-radius: 50%;
  background: var(--text-muted);
}

.device-sidebar__dot.live {
  background: var(--accent-success);
  box-shadow: 0 0 12px var(--accent-success);
}
</style>
