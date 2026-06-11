<script setup lang="ts">
import type { MotionControllerProfile } from '@shared/types/motion'

defineProps<{
  profiles: MotionControllerProfile[]
  activeId: string
}>()

const emit = defineEmits<{
  select: [id: string]
  add: []
  delete: [id: string]
}>()
</script>

<template>
  <aside class="config-sidebar">
    <div class="config-sidebar__header">
      <h3 class="config-sidebar__title">控制器配置</h3>
      <button class="config-sidebar__add-btn" @click="emit('add')">
        <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
          <path d="M12 5v14M5 12h14"/>
        </svg>
      </button>
    </div>
    <div class="config-sidebar__list">
      <button
        v-for="p in profiles"
        :key="p.id"
        @click="emit('select', p.id)"
        class="config-sidebar__item"
        :class="{ 'config-sidebar__item--active': activeId === p.id }"
      >
        <div class="config-sidebar__item-row">
          <div class="config-sidebar__item-icon">
            <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
              <path d="M12 2v4"/><path d="m16.2 7.8 2.9-2.9"/><path d="M18 12h4"/><path d="m16.2 16.2 2.9 2.9"/><path d="M12 18v4"/><path d="m4.9 19.1 2.9-2.9"/><path d="M2 12h4"/><path d="m4.9 4.9 2.9 2.9"/>
            </svg>
          </div>
          <div class="config-sidebar__item-content">
            <span class="config-sidebar__item-name">{{ p.name }}</span>
            <span class="config-sidebar__item-type">{{ p.type }}</span>
          </div>
        </div>
      </button>
      <div v-if="profiles.length === 0" class="config-sidebar__empty">
        <p>暂无控制器配置</p>
      </div>
    </div>
  </aside>
</template>

<style scoped>
/* ============================================================
   配置侧边栏
   ============================================================ */
.config-sidebar {
  display: flex;
  flex-direction: column;
  overflow: hidden;
}

/* ============================================================
   侧边栏头部
   ============================================================ */
.config-sidebar__header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: var(--space-3);
  border-bottom: 1px solid var(--border-default);
}

.config-sidebar__title {
  font-size: 0.6875rem;
  font-weight: 700;
  color: var(--text-muted);
  letter-spacing: 0.06em;
  text-transform: uppercase;
}

.config-sidebar__add-btn {
  width: 1.5rem;
  height: 1.5rem;
  display: flex;
  align-items: center;
  justify-content: center;
  border-radius: var(--radius-sm);
  color: var(--accent-success);
  background: color-mix(in srgb, var(--accent-success) 10%, transparent);
  border: 1px solid color-mix(in srgb, var(--accent-success) 20%, transparent);
  cursor: pointer;
  transition: all var(--motion-fast) var(--easing-standard);
}

.config-sidebar__add-btn:hover {
  background: color-mix(in srgb, var(--accent-success) 20%, transparent);
}

/* ============================================================
   配置列表
   ============================================================ */
.config-sidebar__list {
  flex: 1;
  overflow-y: auto;
  padding: var(--space-2);
  display: flex;
  flex-direction: column;
  gap: var(--space-1);
}

.config-sidebar__item {
  display: block;
  width: 100%;
  padding: var(--space-2) var(--space-2-5);
  border-radius: var(--radius-md);
  background: var(--bg-panel-strong);
  border: 1px solid var(--border-default);
  cursor: pointer;
  transition: all var(--motion-fast) var(--easing-standard);
  text-align: left;
}

.config-sidebar__item:hover {
  background: var(--bg-panel);
  border-color: var(--border-strong);
  transform: translateX(2px);
}

.config-sidebar__item--active {
  background: color-mix(in srgb, var(--accent-success) 12%, transparent) !important;
  border-color: color-mix(in srgb, var(--accent-success) 40%, transparent) !important;
  box-shadow: 0 0 0 1px color-mix(in srgb, var(--accent-success) 15%, transparent), 0 2px 8px color-mix(in srgb, var(--accent-success) 10%, transparent);
}

.config-sidebar__item-row {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  width: 100%;
}

.config-sidebar__item-icon {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 1.5rem;
  height: 1.5rem;
  border-radius: var(--radius-sm);
  background: var(--bg-panel-strong);
  color: var(--text-muted);
  flex-shrink: 0;
  transition: all var(--motion-fast) var(--easing-standard);
}

.config-sidebar__item:hover .config-sidebar__item-icon {
  background: var(--border-strong);
  color: var(--text-primary);
}

.config-sidebar__item--active .config-sidebar__item-icon {
  background: color-mix(in srgb, var(--accent-success) 20%, transparent) !important;
  color: var(--accent-success) !important;
}

.config-sidebar__item-content {
  display: flex;
  flex-direction: column;
  min-width: 0;
  gap: 0;
}

.config-sidebar__item-name {
  font-size: 0.8125rem;
  font-weight: 600;
  color: var(--text-primary);
  line-height: 1.3;
}

.config-sidebar__item-type {
  font-size: 0.625rem;
  color: var(--text-muted);
  font-weight: 500;
  letter-spacing: 0.03em;
}

.config-sidebar__empty {
  padding: var(--space-6) var(--space-4);
  text-align: center;
  color: var(--text-muted);
  font-size: 0.75rem;
}
</style>
