<script setup lang="ts">
import type { MotionControllerProfile } from '@shared/types/motion'
import { useMotionStore } from '@stores/motionStore'

defineProps<{
  profiles: MotionControllerProfile[]
  activeId: string
}>()

const emit = defineEmits<{
  select: [id: string]
  add: []
  delete: [id: string]
}>()

const motion = useMotionStore()
</script>

<template>
  <aside class="config-sidebar">
    <div class="config-sidebar__header">
      <h3 class="config-sidebar__title">控制器配置</h3>
      <button class="config-sidebar__add-btn" @click="emit('add')" title="新建控制器">
        <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
          <path d="M12 5v14M5 12h14"/>
        </svg>
      </button>
    </div>

    <div class="config-sidebar__list">
      <button
        v-for="p in profiles"
        :key="p.id"
        type="button"
        class="config-sidebar__item"
        :class="{ 'config-sidebar__item--active': activeId === p.id }"
        @click="emit('select', p.id)"
      >
        <div class="config-sidebar__item-main">
          <span
            class="config-sidebar__status"
            :class="motion.statusById(p.id)?.connected ? 'config-sidebar__status--connected' : 'config-sidebar__status--disconnected'"
            :title="motion.statusById(p.id)?.connected ? '已连接' : '未连接'"
          />
          <div class="config-sidebar__item-content">
            <span class="config-sidebar__item-name">{{ p.name }}</span>
            <span class="config-sidebar__item-meta">{{ p.type }} · {{ p.address }}:{{ p.port }}</span>
          </div>
        </div>
      </button>

      <div v-if="profiles.length === 0" class="config-sidebar__empty">
        <svg class="config-sidebar__empty-icon" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5">
          <path d="M12 2v4"/><path d="m16.2 7.8 2.9-2.9"/><path d="M18 12h4"/><path d="m16.2 16.2 2.9 2.9"/><path d="M12 18v4"/><path d="m4.9 19.1 2.9-2.9"/><path d="M2 12h4"/><path d="m4.9 4.9 2.9 2.9"/>
        </svg>
        <p>暂无控制器配置</p>
      </div>
    </div>
  </aside>
</template>

<style scoped>
.config-sidebar {
  width: 16rem;
  flex-shrink: 0;
  border-right: 1px solid var(--border-default);
  background: var(--bg-panel-strong);
  display: flex;
  flex-direction: column;
}

.config-sidebar__header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0.875rem 1rem;
  border-bottom: 1px solid var(--border-default);
}
.config-sidebar__title {
  font-size: var(--font-size-xs);
  font-weight: 700;
  color: var(--text-muted);
  letter-spacing: 0.05em;
  text-transform: uppercase;
}
.config-sidebar__add-btn {
  width: 1.625rem;
  height: 1.625rem;
  display: flex;
  align-items: center;
  justify-content: center;
  border-radius: var(--radius-sm);
  border: 1px solid rgba(255, 255, 255, 0.1);
  background: rgba(0, 0, 0, 0.15);
  color: var(--text-muted);
  cursor: pointer;
  transition: all 0.2s ease;
}
.config-sidebar__add-btn:hover {
  color: var(--accent-primary);
  background: rgba(16, 185, 129, 0.15);
  border-color: rgba(16, 185, 129, 0.3);
}
:root[data-theme='light'] .config-sidebar__add-btn {
  border-color: rgba(0, 0, 0, 0.1);
  background: rgba(0, 0, 0, 0.04);
}

.config-sidebar__list {
  flex: 1;
  overflow-y: auto;
  padding: 0.5rem;
  display: flex;
  flex-direction: column;
  gap: 0.375rem;
}
.config-sidebar__list::-webkit-scrollbar {
  width: 4px;
}
.config-sidebar__list::-webkit-scrollbar-track {
  background: transparent;
}
.config-sidebar__list::-webkit-scrollbar-thumb {
  background: var(--border-default);
  border-radius: 2px;
}

.config-sidebar__item {
  display: block;
  width: 100%;
  padding: 0.625rem 0.75rem;
  border-radius: var(--radius-sm);
  background: transparent;
  border: 1px solid transparent;
  text-align: left;
  cursor: pointer;
  transition: all 0.2s ease;
}
.config-sidebar__item:hover {
  background: var(--bg-panel);
  border-color: var(--border-default);
  transform: translateX(2px);
}
.config-sidebar__item--active {
  background: color-mix(in srgb, var(--accent-primary) 8%, transparent) !important;
  border-color: color-mix(in srgb, var(--accent-primary) 25%, transparent) !important;
}

.config-sidebar__item-main {
  display: flex;
  align-items: center;
  gap: 0.625rem;
}
.config-sidebar__status {
  width: 0.5rem;
  height: 0.5rem;
  border-radius: 50%;
  flex-shrink: 0;
  box-shadow: 0 0 0 2px var(--bg-panel-strong);
}
.config-sidebar__status--connected {
  background: var(--accent-success);
  box-shadow: 0 0 0 2px var(--bg-panel-strong), 0 0 6px var(--accent-success);
}
.config-sidebar__status--disconnected {
  background: var(--text-muted);
}

.config-sidebar__item-content {
  display: flex;
  flex-direction: column;
  min-width: 0;
  flex: 1;
}
.config-sidebar__item-name {
  font-size: var(--font-size-sm);
  font-weight: 600;
  color: var(--text-primary);
  line-height: 1.4;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.config-sidebar__item--active .config-sidebar__item-name {
  color: var(--accent-primary);
}
.config-sidebar__item-meta {
  font-size: var(--font-size-2xs);
  color: var(--text-muted);
  margin-top: 0.125rem;
  font-weight: 500;
  letter-spacing: 0.02em;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.config-sidebar__empty {
  padding: 2rem 1rem;
  text-align: center;
  color: var(--text-muted);
  font-size: var(--font-size-xs);
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 0.5rem;
}
.config-sidebar__empty-icon {
  opacity: 0.5;
}
</style>
