<script setup lang="ts">
import type { MotionControllerProfile } from '@shared/types/motion'
import { NButton } from 'naive-ui'

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
      <NButton quaternary size="small" class="config-sidebar__add-btn" @click="emit('add')">
        <template #icon>
          <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <path d="M12 5v14M5 12h14"/>
          </svg>
        </template>
      </NButton>
    </div>
    <div class="config-sidebar__list">
      <NButton
        v-for="p in profiles"
        :key="p.id"
        @click="emit('select', p.id)"
        quaternary
        size="small"
        class="config-sidebar__item"
        :class="{ 'config-sidebar__item--active': activeId === p.id }"
      >
        <div class="config-sidebar__item-row">
          <div class="config-sidebar__item-icon">
            <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
              <path d="M12 2v4"/><path d="m16.2 7.8 2.9-2.9"/><path d="M18 12h4"/><path d="m16.2 16.2 2.9 2.9"/><path d="M12 18v4"/><path d="m4.9 19.1 2.9-2.9"/><path d="M2 12h4"/><path d="m4.9 4.9 2.9 2.9"/>
            </svg>
          </div>
          <div class="config-sidebar__item-content">
            <span class="config-sidebar__item-name">{{ p.name }}</span>
            <span class="config-sidebar__item-type">{{ p.type }}</span>
          </div>
        </div>
      </NButton>
      <div v-if="profiles.length === 0" class="config-sidebar__empty">
        <p>暂无控制器配置</p>
      </div>
    </div>
  </aside>
</template>

<style scoped>
.config-sidebar {
  width: 16rem;
  flex-shrink: 0;
  border-right: 1px solid rgba(255, 255, 255, 0.08);
  display: flex;
  flex-direction: column;
}
:root[data-theme='light'] .config-sidebar {
  border-right: 1px solid rgba(0, 0, 0, 0.05);
}
.config-sidebar__header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0.875rem 1rem;
  border-bottom: 1px solid rgba(255, 255, 255, 0.06);
}
.config-sidebar__title {
  font-size: 0.75rem;
  font-weight: 700;
  color: #94a3b8;
  letter-spacing: 0.05em;
  text-transform: uppercase;
}
.config-sidebar__add-btn {
  width: 1.75rem;
  height: 1.75rem;
  display: flex;
  align-items: center;
  justify-content: center;
  border-radius: 0.375rem;
  color: #10b981;
  background: rgba(16, 185, 129, 0.1);
  border: 1px solid rgba(16, 185, 129, 0.2);
  transition: all 0.2s ease;
}
.config-sidebar__add-btn:hover {
  background: rgba(16, 185, 129, 0.2);
}
.config-sidebar__list {
  flex: 1;
  overflow-y: auto;
  padding: 0.5rem;
  display: flex;
  flex-direction: column;
}
.config-sidebar__item {
  display: block;
  width: 100%;
  padding: 0.75rem 1rem;
  margin-bottom: 0.5rem;
  border-radius: 0.5rem;
  background: rgba(0, 0, 0, 0.08);
  border: 1px solid rgba(255, 255, 255, 0.06);
  cursor: pointer;
  transition: all 0.2s ease;
  text-align: left;
}
.config-sidebar__item:hover {
  background: rgba(255, 255, 255, 0.06);
  border-color: rgba(255, 255, 255, 0.12);
  transform: translateX(2px);
}
.config-sidebar__item--active {
  background: rgba(16, 185, 129, 0.12) !important;
  border-color: rgba(16, 185, 129, 0.4) !important;
  box-shadow: 0 0 0 1px rgba(16, 185, 129, 0.15), 0 2px 8px rgba(16, 185, 129, 0.1);
}
.config-sidebar__item-row {
  display: flex;
  align-items: center;
  gap: 0.625rem;
  width: 100%;
}
.config-sidebar__item-icon {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 2rem;
  height: 2rem;
  border-radius: 0.375rem;
  background: rgba(255, 255, 255, 0.08);
  color: #94a3b8;
  flex-shrink: 0;
  transition: all 0.2s ease;
}
.config-sidebar__item:hover .config-sidebar__item-icon {
  background: rgba(255, 255, 255, 0.12);
  color: #e2e8f0;
}
.config-sidebar__item--active .config-sidebar__item-icon {
  background: rgba(16, 185, 129, 0.2) !important;
  color: #10b981 !important;
}
.config-sidebar__item-content {
  display: flex;
  flex-direction: column;
  min-width: 0;
}
.config-sidebar__item-name {
  font-size: 0.85rem;
  font-weight: 600;
  color: #e2e8f0;
  line-height: 1.4;
}
:root[data-theme='light'] .config-sidebar__item-name {
  color: #0f172a;
}
.config-sidebar__item-type {
  font-size: 0.7rem;
  color: #64748b;
  margin-top: 0.25rem;
  font-weight: 500;
  letter-spacing: 0.02em;
}
.config-sidebar__empty {
  padding: 2rem 1rem;
  text-align: center;
  color: #64748b;
  font-size: 0.75rem;
}
</style>