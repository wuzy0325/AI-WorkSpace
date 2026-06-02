<script setup lang="ts">
import { computed } from 'vue'
import { useDeviceStore } from '@stores/deviceStore'
import {
  Activity, Trash2, Wifi, WifiOff, Loader2,
  CircleDot, Zap, AlertTriangle, ChevronRight
} from '@lucide/vue'

const deviceStore = useDeviceStore()

const sorted = computed(() =>
  [...deviceStore.profiles].sort(
    (a, b) => (b.createdAt ?? 0) - (a.createdAt ?? 0)
  )
)

async function handleDelete(id: string, event: Event): Promise<void> {
  event.stopPropagation()
  await deviceStore.removeProfile(id)
}

function statusIcon(status: string, acquiring: boolean) {
  if (acquiring) return Zap
  if (status === 'Connected') return Wifi
  if (status === 'Connecting') return Loader2
  if (status === 'Error') return AlertTriangle
  return WifiOff
}

function statusClass(status: string, acquiring: boolean): string {
  if (acquiring) return 'device__status--acquiring'
  if (status === 'Connected') return 'device__status--connected'
  if (status === 'Connecting') return 'device__status--connecting'
  if (status === 'Error') return 'device__status--error'
  return 'device__status--disconnected'
}

function statusLabel(status: string, acquiring: boolean): string {
  if (acquiring) return '采集中'
  if (status === 'Connected') return '已连接'
  if (status === 'Connecting') return '连接中'
  if (status === 'Error') return '错误'
  return '未连接'
}
</script>

<template>
  <aside class="sidebar">
    <div class="sidebar__header">
      <h2 class="sidebar__title">设备列表</h2>
      <span class="sidebar__count">{{ sorted.length }}</span>
    </div>

    <div v-if="sorted.length === 0" class="sidebar__empty">
      <div class="sidebar__empty-illu">
        <Activity class="sidebar__empty-icon" />
      </div>
      <p class="sidebar__empty-text">暂无设备</p>
      <p class="sidebar__empty-hint">点击顶栏 + 添加 T1603</p>
    </div>

    <ul v-else class="sidebar__list">
      <li
        v-for="(p, idx) in sorted"
        :key="p.id"
        class="sidebar__item"
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
            <span class="device__name">{{ p.name || '未命名' }}</span>
            <span class="device__addr mono">{{ p.address }}:{{ p.port }}</span>
          </div>
          <div class="device__right">
            <div class="device__status" :class="statusClass(deviceStore.statusFor(p.id), deviceStore.acquiringFor(p.id))">
              <component
                :is="statusIcon(deviceStore.statusFor(p.id), deviceStore.acquiringFor(p.id))"
                class="device__status-icon"
                :class="{ 'device__status-icon--spin': deviceStore.statusFor(p.id) === 'Connecting' }"
              />
              <span class="device__status-text">{{ statusLabel(deviceStore.statusFor(p.id), deviceStore.acquiringFor(p.id)) }}</span>
            </div>
            <button
              class="device__delete"
              title="删除设备"
              @click="handleDelete(p.id, $event)"
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
