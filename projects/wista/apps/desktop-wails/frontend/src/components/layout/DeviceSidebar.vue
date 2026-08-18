<script setup lang="ts">
import { computed, ref } from 'vue'
import { useDeviceStore } from '@stores/deviceStore'
import { useI18nStore } from '@stores/i18nStore'
import {
  Activity, Trash2, Network, Loader2,
  CircleDot, Zap, AlertTriangle, ChevronRight, Search
} from '@lucide/vue'

const emit = defineEmits<{
  (e: 'scan'): void
}>()

const deviceStore = useDeviceStore()
const i18n = useI18nStore()
const pendingDeleteId = ref<string | null>(null)
const pendingDeleteName = ref('')

const sorted = computed(() =>
  [...deviceStore.profiles].sort(
    (a, b) => (b.createdAt ?? 0) - (a.createdAt ?? 0)
  )
)

function handleDelete(id: string, name: string, event: Event): void {
  event.stopPropagation()
  pendingDeleteId.value = id
  pendingDeleteName.value = name || i18n.t('sidebar.unnamedDevice')
}

function closeDeleteDialog(): void {
  pendingDeleteId.value = null
  pendingDeleteName.value = ''
}

async function confirmDelete(): Promise<void> {
  if (!pendingDeleteId.value) return
  const id = pendingDeleteId.value
  closeDeleteDialog()
  await deviceStore.removeProfile(id)
}

function statusIcon(status: string, acquiring: boolean) {
  if (acquiring) return Zap
  if (status === 'Stopping') return Loader2
  if (status === 'Connected') return Network
  if (status === 'Connecting') return Loader2
  if (status === 'Error') return AlertTriangle
  return Network
}

function statusClass(status: string, acquiring: boolean): string {
  if (acquiring) return 'device__status--acquiring'
  if (status === 'Starting' || status === 'Stopping') return 'device__status--connecting'
  if (status === 'Connected') return 'device__status--connected'
  if (status === 'Connecting') return 'device__status--connecting'
  if (status === 'Error') return 'device__status--error'
  return 'device__status--disconnected'
}

function statusLabel(status: string, acquiring: boolean): string {
  if (acquiring) return i18n.t('status.acquiring')
  if (status === 'Starting') return i18n.t('status.starting')
  if (status === 'Stopping') return i18n.t('status.stopping')
  if (status === 'Connected') return i18n.t('status.connected')
  if (status === 'Connecting') return i18n.t('status.connecting')
  if (status === 'Error') return i18n.t('status.error')
  return i18n.t('status.disconnected')
}
</script>

<template>
  <aside class="sidebar">
    <div class="sidebar__header">
      <h2 class="sidebar__title">{{ i18n.t('sidebar.deviceList') }}</h2>
      <div class="sidebar__header-actions">
        <span class="sidebar__count" data-testid="sidebar-count">{{ sorted.length }}</span>
        <button
          class="sidebar__scan-btn"
          :title="i18n.t('sidebar.scanDevices')"
          :disabled="deviceStore.isScanning"
          @click="emit('scan')"
        >
          <Search class="sidebar__scan-icon" />
        </button>
      </div>
    </div>

    <div v-if="sorted.length === 0" class="sidebar__empty">
      <div class="sidebar__empty-illu">
        <Activity class="sidebar__empty-icon" />
      </div>
      <p class="sidebar__empty-text">{{ i18n.t('sidebar.noDevices') }}</p>
      <p class="sidebar__empty-hint">{{ i18n.t('sidebar.addHint') }}</p>
    </div>

    <ul v-else class="sidebar__list" data-testid="sidebar-list">
      <li
        v-for="(p, idx) in sorted"
        :key="p.id"
        class="sidebar__item"
        data-testid="sidebar-item"
        :class="{
          'sidebar__item--selected': deviceStore.selectedId === p.id,
        }"
        :style="{ animationDelay: `${idx * 40}ms` }"
        @click="deviceStore.selectDevice(p.id)"
      >
        <div class="device">
          <div class="device__icon">
            <Activity class="device__icon-svg" />
          </div>
          <div class="device__info">
            <span class="device__name">{{ p.name || i18n.t('sidebar.unnamed') }}</span>
            <span class="device__addr mono">{{ p.address }}:{{ p.port }}</span>
          </div>
          <div class="device__right">
            <div class="device__status" :class="statusClass(deviceStore.statusFor(p.id), deviceStore.acquiringFor(p.id))">
              <component
                :is="statusIcon(deviceStore.statusFor(p.id), deviceStore.acquiringFor(p.id))"
                class="device__status-icon"
                :class="{ 'device__status-icon--spin': deviceStore.statusFor(p.id) === 'Connecting' || deviceStore.statusFor(p.id) === 'Starting' || deviceStore.statusFor(p.id) === 'Stopping' }"
              />
              <span class="device__status-text">{{ statusLabel(deviceStore.statusFor(p.id), deviceStore.acquiringFor(p.id)) }}</span>
            </div>
            <button
              class="device__delete"
              :title="i18n.t('sidebar.deleteDevice')"
              @click="handleDelete(p.id, p.name || i18n.t('sidebar.unnamedDevice'), $event)"
            >
              <Trash2 class="device__delete-icon" />
            </button>
            <ChevronRight class="device__chevron" />
          </div>
        </div>

        <!-- 选中指示器 -->
        <div v-if="deviceStore.selectedId === p.id" class="sidebar__indicator"></div>
      </li>
    </ul>

    <Teleport to="body">
      <Transition name="modal">
        <div v-if="pendingDeleteId" class="modal-overlay" @click.self="closeDeleteDialog">
          <div class="modal-panel modal-panel--compact">
            <div class="dialog">
              <div class="dialog__header">
                <h3 class="dialog__title">{{ i18n.t('sidebar.confirmDeleteTitle') }}</h3>
                <p class="dialog__subtitle">{{ i18n.t('sidebar.confirmDeleteSubtitle') }}</p>
              </div>
              <div class="dialog__body">
                <p class="dialog__text">{{ i18n.t('sidebar.confirmDeleteText', { name: pendingDeleteName }) }}</p>
              </div>
              <div class="dialog__actions">
                <button class="dialog__btn dialog__btn--secondary" @click="closeDeleteDialog">{{ i18n.t('common.cancel') }}</button>
                <button class="dialog__btn dialog__btn--danger" @click="confirmDelete">{{ i18n.t('common.delete') }}</button>
              </div>
            </div>
          </div>
        </div>
      </Transition>
    </Teleport>
  </aside>
</template>

<style scoped>
.sidebar {
  width: var(--layout-sidebar-width);
  min-width: var(--layout-sidebar-width);
  background: var(--sidebar-bg);
  backdrop-filter: blur(14px) saturate(140%);
  -webkit-backdrop-filter: blur(14px) saturate(140%);
  border-right: 1px solid var(--border-default);
  display: flex;
  flex-direction: column;
  overflow: hidden;
}

.modal-overlay {
  position: fixed;
  inset: 0;
  z-index: 100;
  display: flex;
  align-items: center;
  justify-content: center;
  background: var(--bg-overlay);
  backdrop-filter: blur(8px);
  -webkit-backdrop-filter: blur(8px);
}

.modal-panel {
  width: 100%;
  display: flex;
  flex-direction: column;
}

.modal-panel--compact {
  max-width: 24rem;
}

.modal-enter-active {
  transition: opacity var(--motion-base) var(--easing-standard);
}

.modal-leave-active {
  transition: opacity var(--motion-fast) var(--easing-exit);
}

.modal-enter-from,
.modal-leave-to {
  opacity: 0;
}

.modal-enter-active .modal-panel,
.modal-leave-active .modal-panel {
  transition: transform var(--motion-base) var(--easing-emphasis),
              opacity var(--motion-base) var(--easing-standard);
}

.modal-enter-from .modal-panel {
  opacity: 0;
  transform: scale(0.96) translateY(12px);
}

.modal-leave-to .modal-panel {
  opacity: 0;
  transform: scale(0.98) translateY(4px);
}

.dialog {
  background: var(--bg-panel);
  border: 1px solid var(--border-default);
  border-radius: var(--radius-xl);
  box-shadow: var(--shadow-lg);
  overflow: hidden;
  display: flex;
  flex-direction: column;
}

.dialog__header {
  padding: 1.2rem 1.25rem 0.9rem;
  display: flex;
  flex-direction: column;
  gap: 0.25rem;
  border-bottom: 1px solid var(--divider-color);
}

.dialog__title {
  font-size: var(--font-size-md);
  font-weight: 800;
  color: var(--text-primary);
}

.dialog__subtitle {
  font-size: var(--font-size-xs);
  color: var(--text-muted);
}

.dialog__body {
  padding: 1rem 1.25rem;
}

.dialog__text {
  font-size: var(--font-size-sm);
  color: var(--text-secondary);
  line-height: 1.6;
}

.dialog__actions {
  display: flex;
  justify-content: flex-end;
  gap: 0.65rem;
  padding: 0 1.25rem 1.1rem;
}

.dialog__btn {
  min-width: 4.8rem;
  height: 2.15rem;
  padding: 0 0.9rem;
  border-radius: var(--radius-md);
  font-size: var(--font-size-sm);
  font-weight: 700;
  transition: all var(--motion-fast) var(--easing-standard);
}

.dialog__btn--secondary {
  background: var(--btn-bg);
  border: 1px solid var(--border-default);
  color: var(--text-secondary);
}

.dialog__btn--secondary:hover {
  background: var(--btn-bg-hover);
  color: var(--text-primary);
}

.dialog__btn--danger {
  background: var(--danger-muted);
  border: 1px solid var(--danger-border);
  color: var(--danger);
}

.dialog__btn--danger:hover {
  background: color-mix(in srgb, var(--danger-muted) 160%, transparent);
}

.sidebar__header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 1rem 1.1rem;
  border-bottom: 1px solid var(--divider-color);
  flex-shrink: 0;
}

.sidebar__title {
  font-size: var(--font-size-sm);
  font-weight: 800;
  color: var(--text-primary);
  letter-spacing: 0.02em;
}

.sidebar__count {
  font-size: var(--font-size-xs);
  font-weight: 700;
  color: var(--text-muted);
  background: var(--btn-bg);
  padding: 0.15rem 0.5rem;
  border-radius: var(--radius-pill);
  min-width: 1.5rem;
  text-align: center;
}

.sidebar__header-actions {
  display: flex;
  align-items: center;
  gap: 0.35rem;
}

.sidebar__scan-btn {
  width: 26px;
  height: 26px;
  display: flex;
  align-items: center;
  justify-content: center;
  border-radius: var(--radius-sm);
  background: var(--btn-bg);
  border: 1px solid var(--border-default);
  color: var(--text-muted);
  cursor: pointer;
  transition: all var(--motion-fast) var(--easing-standard);
}

.sidebar__scan-btn:hover:not(:disabled) {
  color: var(--accent);
  border-color: var(--accent-border);
  background: var(--accent-soft);
}

.sidebar__scan-btn:disabled {
  opacity: 0.4;
  cursor: not-allowed;
}

.sidebar__scan-icon {
  width: 14px;
  height: 14px;
}

/* 空状态 */
.sidebar__empty {
  flex: 1;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 0.6rem;
  padding: 2rem 1rem;
  color: var(--text-muted);
}

.sidebar__empty-illu {
  width: 56px;
  height: 56px;
  border-radius: var(--radius-lg);
  background: var(--btn-bg);
  border: 1px solid var(--border-default);
  display: flex;
  align-items: center;
  justify-content: center;
  margin-bottom: 0.5rem;
}

.sidebar__empty-icon {
  width: 24px;
  height: 24px;
  color: var(--text-muted);
  opacity: 0.6;
}

.sidebar__empty-text {
  font-size: var(--font-size-sm);
  font-weight: 700;
  color: var(--text-secondary);
}

.sidebar__empty-hint {
  font-size: var(--font-size-xs);
  color: var(--text-muted);
  text-align: center;
}

/* 设备列表 */
.sidebar__list {
  list-style: none;
  overflow-y: auto;
  padding: 0.5rem;
  display: flex;
  flex-direction: column;
  gap: 0.25rem;
}

.sidebar__item {
  position: relative;
  border-radius: var(--radius-md);
  cursor: pointer;
  transition: all var(--motion-fast) var(--easing-standard);
  animation: sidebar-item-enter var(--motion-base) var(--easing-emphasis) both;
}

.sidebar__item:hover {
  background: var(--btn-bg-hover);
}

.sidebar__item--selected {
  background: var(--accent-muted);
  border: 1px solid var(--accent-border);
}

.sidebar__item--selected:hover {
  background: var(--accent-muted);
}

.sidebar__indicator {
  position: absolute;
  left: 0;
  top: 50%;
  transform: translateY(-50%);
  width: 3px;
  height: 20px;
  background: var(--accent);
  border-radius: 0 2px 2px 0;
  box-shadow: 0 0 8px var(--accent-glow);
}

/* 设备项内容 */
.device {
  display: flex;
  align-items: center;
  gap: 0.65rem;
  padding: 0.65rem 0.75rem;
  position: relative;
}

.device__icon {
  width: 32px;
  height: 32px;
  border-radius: var(--radius-md);
  background: linear-gradient(135deg, var(--accent) 0%, var(--accent-active) 100%);
  display: flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
  box-shadow: 0 4px 12px var(--accent-glow);
}

.device__icon-svg {
  width: 16px;
  height: 16px;
  color: #ffffff;
}

.device__info {
  display: flex;
  flex-direction: column;
  gap: 0.15rem;
  flex: 1;
  min-width: 0;
}

.device__name {
  font-size: var(--font-size-sm);
  font-weight: 700;
  color: var(--text-primary);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.device__addr {
  font-size: var(--font-size-xs);
  color: var(--text-muted);
  font-weight: 500;
}

.device__right {
  display: flex;
  align-items: center;
  gap: 0.4rem;
  flex-shrink: 0;
}

.device__status {
  display: flex;
  align-items: center;
  gap: 0.3rem;
  padding: 0.25rem 0.5rem;
  border-radius: var(--radius-sm);
  font-size: 0.6rem;
  font-weight: 700;
  letter-spacing: 0.06em;
  text-transform: uppercase;
  background: var(--btn-bg);
  border: 1px solid var(--border-default);
}

.device__status-icon {
  width: 10px;
  height: 10px;
}

.device__status-icon--spin {
  animation: spin 1s linear infinite;
}

.device__status-text {
  display: none;
}

/* 状态变体 */
.device__status--acquiring {
  background: var(--success-muted);
  border-color: var(--accent-border);
  color: var(--accent);
}

.device__status--acquiring .device__status-icon {
  animation: status-pulse 1.2s ease-in-out infinite;
}

.device__status--connected {
  background: var(--success-muted);
  border-color: var(--accent-border);
  color: var(--accent);
}

.device__status--connecting {
  background: var(--warning-muted);
  border-color: var(--warning);
  color: var(--warning);
}

.device__status--error {
  background: var(--danger-muted);
  border-color: var(--danger-border);
  color: var(--danger);
}

.device__status--disconnected {
  color: var(--text-muted);
}

.device__chevron {
  width: 14px;
  height: 14px;
  color: var(--text-muted);
  opacity: 0;
  transform: translateX(-4px);
  transition: all var(--motion-fast) var(--easing-standard);
}

.sidebar__item:hover .device__chevron {
  opacity: 0.6;
  transform: translateX(0);
}

.sidebar__item--selected .device__chevron {
  opacity: 1;
  color: var(--accent);
  transform: translateX(0);
}

.device__delete {
  width: 22px;
  height: 22px;
  display: flex;
  align-items: center;
  justify-content: center;
  border-radius: var(--radius-sm);
  background: transparent;
  border: 1px solid transparent;
  color: var(--text-muted);
  opacity: 0;
  transition: all var(--motion-fast) var(--easing-standard);
  flex-shrink: 0;
}

.sidebar__item:hover .device__delete {
  opacity: 0.6;
}

.device__delete:hover {
  opacity: 1 !important;
  color: var(--danger);
  background: var(--danger-muted);
  border-color: var(--danger-border);
}

.device__delete-icon {
  width: 12px;
  height: 12px;
}

/* 减少动画偏好 */
@media (prefers-reduced-motion: reduce) {
  .sidebar__item {
    animation: none;
  }

  .device__status-icon--spin {
    animation: none;
  }

  .device__status--acquiring .device__status-icon {
    animation: none;
  }

  .device__chevron {
    transition: none;
  }
}
</style>
