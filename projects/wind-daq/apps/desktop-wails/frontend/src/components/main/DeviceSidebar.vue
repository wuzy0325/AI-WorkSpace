<script setup lang="ts">
import { CheckCircle2 } from '@lucide/vue'
import UiButton from '@components/ui/UiButton.vue'
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
          type="button"
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
            <span v-if="deviceStore.acquiringFor(p.id)" class="device-status--acquiring-icon"></span>
            <CheckCircle2 v-else-if="deviceStore.statusFor(p.id) === 'Connected'" class="device-sidebar__status-icon" />
            {{ displayStatusLabel(p.id) }}
          </div>
        </button>
      </template>
    </div>
  </aside>
</template>

<style scoped>
.device-sidebar {
  width: clamp(196px, 20vw, var(--layout-sidebar-width, 244px));
  height: 100%;
  flex-shrink: 0;
  display: flex;
  flex-direction: column;
  overflow: hidden;
  border-right: 1px solid color-mix(in srgb, var(--border-default) 50%, transparent);
  background: color-mix(in srgb, var(--bg-panel) 96%, transparent);
}

:root[data-theme='light'] .device-sidebar {
  border-right: 1px solid color-mix(in srgb, var(--border-default) 50%, transparent);
}

.device-sidebar__header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: var(--space-2) var(--space-3);
  border-bottom: 1px solid color-mix(in srgb, var(--border-default) 50%, transparent);
}

:root[data-theme='light'] .device-sidebar__header {
  border-bottom: 1px solid color-mix(in srgb, var(--border-default) 50%, transparent);
}

.device-sidebar__title {
  font-size: var(--font-size-sm);
  font-weight: 700;
  color: var(--text-secondary);
  letter-spacing: 0.1em;
  text-transform: uppercase;
}

:root[data-theme='light'] .device-sidebar__title {
  color: var(--text-secondary);
}

.device-sidebar__manage-btn {
  padding: var(--space-0-5) var(--space-2);
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: var(--font-size-xs);
  font-weight: 600;
  color: var(--text-secondary);
  background: color-mix(in srgb, var(--bg-panel-strong) 80%, transparent);
  border: 1px solid color-mix(in srgb, var(--border-default) 60%, transparent);
  border-radius: var(--radius-md);
  cursor: pointer;
  transition: all 0.2s ease;
}

:root[data-theme='light'] .device-sidebar__manage-btn {
  color: var(--text-secondary);
  background: color-mix(in srgb, var(--bg-panel-strong) 80%, transparent);
  border: 1px solid color-mix(in srgb, var(--border-default) 60%, transparent);
}

.device-sidebar__manage-btn:hover {
  color: var(--accent-primary);
  background: color-mix(in srgb, var(--accent-primary) 15%, transparent);
  border-color: color-mix(in srgb, var(--accent-primary) 30%, transparent);
}

.device-sidebar__list {
  flex: 1;
  overflow-y: auto;
  padding: var(--space-2);
  display: flex;
  flex-direction: column;
  gap: var(--space-1);
}

.device-sidebar__empty {
  padding: var(--space-4) var(--space-2);
  text-align: center;
  font-size: var(--font-size-xs);
  color: var(--text-muted);
}

.device-sidebar__item {
  width: 100%;
  text-align: left;
  padding: var(--space-2);
  background: color-mix(in srgb, var(--bg-panel-strong) 60%, transparent);
  border: 1px solid transparent;
  border-radius: var(--radius-md);
  cursor: pointer;
  transition: all 0.2s ease;
  display: flex;
  flex-direction: column;
  gap: var(--space-1);
  align-items: stretch;
  justify-content: flex-start;
  height: auto;
}

/* 无障碍焦点样式：确保键盘导航时可见 */
.device-sidebar__item:focus-visible {
  outline: none;
  box-shadow: 0 0 0 2px var(--focus-ring), 0 0 0 4px var(--focus-ring-soft);
}

:root[data-theme='light'] .device-sidebar__item {
  background: color-mix(in srgb, var(--bg-panel-strong) 60%, transparent);
}

.device-sidebar__item:hover {
  background: color-mix(in srgb, var(--bg-panel-strong) 80%, transparent);
  border-color: color-mix(in srgb, var(--border-default) 80%, transparent);
}

:root[data-theme='light'] .device-sidebar__item:hover {
  background: color-mix(in srgb, var(--bg-panel-strong) 80%, transparent);
  border-color: color-mix(in srgb, var(--border-default) 60%, transparent);
}

.device-sidebar__item--active {
  background: color-mix(in srgb, var(--accent-primary) 8%, transparent);
  border-color: color-mix(in srgb, var(--accent-primary) 25%, transparent);
}

.device-sidebar__item--error {
  border-color: color-mix(in srgb, var(--accent-danger) 20%, transparent);
}

.device-sidebar__item--error:hover {
  border-color: color-mix(in srgb, var(--accent-danger) 40%, transparent);
}

.device-sidebar__item-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--space-2);
  min-width: 0;
}

.device-sidebar__item-name {
  font-size: var(--font-size-sm);
  font-weight: 700;
  color: var(--text-primary);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  flex: 1;
  min-width: 0;
}

:root[data-theme='light'] .device-sidebar__item-name {
  color: var(--text-primary);
}

.device-sidebar__status-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  flex-shrink: 0;
}

.device-sidebar__status--acquiring {
  background: var(--accent-primary);
  box-shadow: 0 0 12px var(--accent-primary);
  animation: breathe 1.5s ease-in-out infinite;
}

.device-sidebar__status--connected {
  background: var(--accent-primary);
  box-shadow: 0 0 8px var(--accent-primary);
}

.device-sidebar__status--connecting {
  background: var(--accent-warning);
  box-shadow: 0 0 8px color-mix(in srgb, var(--accent-warning) 50%, transparent);
  animation: breathe 0.8s ease-in-out infinite;
}

.device-sidebar__status--error {
  background: var(--accent-danger);
  box-shadow: 0 0 8px color-mix(in srgb, var(--accent-danger) 60%, transparent);
}

.device-sidebar__status--disconnected {
  background: var(--text-muted);
}

@keyframes breathe {
  0%, 100% { opacity: 1; transform: scale(1); }
  50% { opacity: 0.4; transform: scale(0.7); }
}

.device-sidebar__item-status.device-status--acquiring {
  color: var(--accent-primary);
}

.device-status--acquiring-icon {
  display: inline-block;
  width: var(--font-size-xs);
  height: var(--font-size-xs);
  border-radius: 2px;
  background: var(--accent-primary);
  animation: breathe 1.5s ease-in-out infinite;
}

.device-sidebar__item-status.device-status--connected {
  color: var(--accent-primary);
}

.device-sidebar__item-status.device-status--error {
  color: var(--accent-danger);
  text-transform: uppercase;
  letter-spacing: 0.06em;
}

.device-sidebar__item-status.device-status--connecting {
  color: var(--accent-warning-core);
}

.device-sidebar__item-status.device-status--disconnected {
  color: var(--text-muted);
}



.device-sidebar__item-status {
  display: flex;
  align-items: center;
  gap: var(--space-1);
  font-size: var(--font-size-xs);
  font-weight: 600;
  letter-spacing: 0.02em;
}

.device-sidebar__status-icon {
  width: 0.75rem;
  height: 0.75rem;
  flex-shrink: 0;
}
</style>
