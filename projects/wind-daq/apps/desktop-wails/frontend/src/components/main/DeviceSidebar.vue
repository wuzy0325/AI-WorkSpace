<script setup lang="ts">
import { CheckCircle2 } from '@lucide/vue'
import { useDeviceStore } from '@stores/deviceStore'

const props = defineProps<{
  t?: Record<string, string>
  statusColor?: (status: string) => string
  statusLabel?: (status: string) => string
}>()

const emit = defineEmits<{
  (e: 'open-manage'): void
}>()

const deviceStore = useDeviceStore()

function statusGlowClass(profileId: string): string {
  const status = deviceStore.statusFor(profileId)
  if (deviceStore.acquiringFor(profileId)) return 'device-sidebar__status--acquiring'
  if (status === 'Connected') return 'device-sidebar__status--connected'
  if (status === 'Connecting') return 'device-sidebar__status--connecting'
  if (status === 'Error') return 'device-sidebar__status--error'
  return 'device-sidebar__status--disconnected'
}

function statusTextClass(profileId: string): string {
  if (deviceStore.acquiringFor(profileId)) return 'device-status--acquiring'
  const status = deviceStore.statusFor(profileId)
  if (status === 'Connected') return 'device-status--connected'
  if (status === 'Connecting') return 'device-status--connecting'
  if (status === 'Error') return 'device-status--error'
  return 'device-status--disconnected'
}

function displayStatusLabel(profileId: string): string {
  if (deviceStore.acquiringFor(profileId)) return props.t?.acquiring || '采集中'
  const status = deviceStore.statusFor(profileId)
  if (status === 'Connected') return props.t?.connectedState || 'Connected'
  if (status === 'Connecting') return props.t?.connectingState || 'Connecting'
  if (status === 'Error') return props.t?.warningState || 'Warning'
  return props.t?.disconnectedState || 'Disconnected'
}
</script>

<template>
  <aside
    data-test="device-sidebar-shell"
    class="device-sidebar"
  >
    <!-- Header -->
    <div class="device-sidebar__header">
      <span class="device-sidebar__title">{{ t?.deviceList || '设备列表' }}</span>
      <button
        class="device-sidebar__manage-btn"
        @click="emit('open-manage')"
        :title="t?.manage || '管理设备'"
      >
        {{ t?.manage || '管理' }}
      </button>
    </div>

    <!-- Device List -->
    <div data-test="device-sidebar-list" class="device-sidebar__list no-scrollbar">
      <div v-if="!deviceStore.profiles || deviceStore.profiles.length === 0" class="device-sidebar__empty">
        {{ t?.noDevices || '暂无设备' }}
      </div>

      <template v-else>
        <button
          v-for="p in deviceStore.profiles"
          :key="p.id"
          data-test="device-sidebar-item"
          class="device-sidebar__item"
          :class="{
            'device-sidebar__item--active': deviceStore.selectedDeviceId === p.id,
            'device-sidebar__item--error': deviceStore.statusFor(p.id) === 'Error'
          }"
          @click="deviceStore.selectDevice(p.id)"
        >
          <div class="device-sidebar__item-header">
            <span class="device-sidebar__item-name">{{ p.name || t?.unnamed || '未命名' }}</span>
            <div
              class="device-sidebar__status-dot"
              :class="statusGlowClass(p.id)"
            />
          </div>
          <div
            data-test="device-sidebar-status-text"
            class="device-sidebar__item-status"
            :class="statusTextClass(p.id)"
          >
            <span v-if="deviceStore.acquiringFor(p.id)" class="w-3 h-3 mr-1 rounded-[2px] device-status--acquiring-icon"></span>
            <CheckCircle2 v-else-if="deviceStore.statusFor(p.id) === 'Connected'" class="w-3 h-3 mr-1" />
            {{ displayStatusLabel(p.id) }}
          </div>
        </button>
      </template>
    </div>
  </aside>
</template>

<style scoped>
.device-sidebar {
  width: clamp(220px, 24vw, var(--layout-sidebar-width, 244px));
  height: 100%;
  flex-shrink: 0;
  display: flex;
  flex-direction: column;
  overflow: hidden;
  border-right: 1px solid rgba(255, 255, 255, 0.05);
  background: color-mix(in srgb, var(--bg-panel) 96%, transparent);
}

:root[data-theme='light'] .device-sidebar {
  border-right: 1px solid rgba(0, 0, 0, 0.05);
}

.device-sidebar__header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: var(--space-4);
  border-bottom: 1px solid rgba(255, 255, 255, 0.05);
}

:root[data-theme='light'] .device-sidebar__header {
  border-bottom: 1px solid rgba(0, 0, 0, 0.05);
}

.device-sidebar__title {
  font-size: var(--font-size-sm);
  font-weight: 700;
  color: #64748b;
  letter-spacing: 0.1em;
  text-transform: uppercase;
}

.device-sidebar__manage-btn {
  padding: var(--space-1) var(--space-3);
  display: flex;
  align-items: center;
  justify-content: center;
  border-radius: 0.5rem;
  font-size: var(--font-size-xs);
  font-weight: 600;
  color: #64748b;
  background: rgba(255, 255, 255, 0.05);
  border: 1px solid rgba(255, 255, 255, 0.1);
  transition: all 0.2s ease;
}

:root[data-theme='light'] .device-sidebar__manage-btn {
  background: rgba(0, 0, 0, 0.05);
  border: 1px solid rgba(0, 0, 0, 0.1);
}

.device-sidebar__manage-btn:hover {
  color: #10b981;
  background: rgba(16, 185, 129, 0.15);
  border-color: rgba(16, 185, 129, 0.3);
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
  padding: var(--space-8) var(--space-4);
  text-align: center;
  font-size: var(--font-size-xs);
  color: #64748b;
}

.device-sidebar__item {
  width: 100%;
  text-align: left;
  padding: var(--space-3);
  border-radius: 0.75rem;
  background: rgba(30, 41, 59, 0.4);
  border: 1px solid transparent;
  transition: all 0.2s ease;
  cursor: pointer;
}

:root[data-theme='light'] .device-sidebar__item {
  background: rgba(255, 255, 255, 0.6);
}

.device-sidebar__item:hover {
  background: rgba(30, 41, 59, 0.6);
  border-color: rgba(255, 255, 255, 0.1);
}

:root[data-theme='light'] .device-sidebar__item:hover {
  background: rgba(255, 255, 255, 0.8);
  border-color: rgba(0, 0, 0, 0.05);
}

.device-sidebar__item--active {
  background: rgba(16, 185, 129, 0.08) !important;
  border-color: rgba(16, 185, 129, 0.3) !important;
}

.device-sidebar__item--error {
  border-color: rgba(244, 63, 94, 0.2);
}

.device-sidebar__item--error:hover {
  border-color: rgba(244, 63, 94, 0.4);
}

.device-sidebar__item-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: var(--space-1);
}

.device-sidebar__item-name {
  font-size: var(--font-size-base);
  font-weight: 700;
  color: #e2e8f0;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

:root[data-theme='light'] .device-sidebar__item-name {
  color: #0f172a;
}

.device-sidebar__status-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  flex-shrink: 0;
}

.device-sidebar__status--acquiring {
  background: #10b981;
  box-shadow: 0 0 12px #10b981;
  animation: breathe 1.5s ease-in-out infinite;
}

.device-sidebar__status--connected {
  background: #10b981;
  box-shadow: 0 0 8px #10b981;
}

.device-sidebar__status--connecting {
  background: #f59e0b;
  box-shadow: 0 0 8px #f59e0b;
  animation: breathe 0.8s ease-in-out infinite;
}

.device-sidebar__status--error {
  background: #f43f5e;
  box-shadow: 0 0 8px #f43f5e;
}

.device-sidebar__status--disconnected {
  background: #64748b;
}

@keyframes breathe {
  0%, 100% { opacity: 1; transform: scale(1); }
  50% { opacity: 0.4; transform: scale(0.7); }
}

.device-sidebar__item-status.device-status--acquiring {
  color: #10b981;
}

.device-status--acquiring-icon {
  display: inline-block;
  background: #10b981;
  animation: breathe 1.5s ease-in-out infinite;
}

.device-sidebar__item-status.device-status--connected {
  color: #10b981;
}

.device-sidebar__item-status.device-status--error {
  color: #f43f5e;
  text-transform: uppercase;
  letter-spacing: 0.06em;
}

.device-sidebar__item-status.device-status--connecting {
  color: #f59e0b;
}

.device-sidebar__item-status.device-status--disconnected {
  color: #64748b;
}

.device-sidebar__item-status {
  display: flex;
  align-items: center;
  font-size: var(--font-size-xs);
  font-weight: 700;
  letter-spacing: 0.04em;
}
</style>
