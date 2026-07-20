<script setup lang="ts">
// MotionSidebar — 左侧控制器列表侧边栏
// 从 MotionControlPanel 抽出。负责展示控制器列表、连接状态、地址、新增按钮。

import type { MotionControllerProfile, MotionControllerStatus } from '@shared/types/motion'
import { useI18nStore } from '@stores/i18nStore'

defineProps<{
  /** 全部控制器配置 */
  profiles: MotionControllerProfile[]
  /** 全部控制器状态（用于查 connected 状态） */
  statusList: MotionControllerStatus[]
  /** 当前选中的控制器 ID */
  selectedId: string | null
}>()

const emit = defineEmits<{
  (e: 'select', id: string): void
  (e: 'open-config'): void
}>()

const i18n = useI18nStore()
</script>

<template>
  <aside class="motion-sidebar" data-test="motion-panel-surface">
    <div class="motion-sidebar__header">{{ i18n.t.motionController }}</div>

    <!-- 空状态 -->
    <div v-if="profiles.length === 0" class="motion-sidebar__empty">
      <p class="motion-sidebar__empty-title">{{ i18n.t.noControllerConfig }}</p>
      <p class="motion-sidebar__empty-desc">{{ i18n.t.clickConfigToAdd }}</p>
    </div>

    <!-- 控制器列表 -->
    <div v-else class="motion-sidebar__list custom-scrollbar">
      <button
        v-for="p in profiles"
        :key="p.id"
        @click="emit('select', p.id)"
        class="motion-sidebar__item"
        :class="{ 'motion-sidebar__item--active': selectedId === p.id }"
        :aria-current="selectedId === p.id ? 'true' : undefined"
      >
        <span
          class="motion-sidebar__dot"
          :class="statusList.find((s) => s.id === p.id)?.connected ? 'ok' : 'off'"
          aria-hidden="true"
        ></span>
        <div class="motion-sidebar__item-main">
          <div class="motion-sidebar__item-name" :title="p.name">{{ p.name }}</div>
          <div class="motion-sidebar__item-sub">
            {{ statusList.find((s) => s.id === p.id)?.connected ? i18n.t.connected : i18n.t.disconnected }}
            · {{ p.address }}:{{ p.port }}
          </div>
        </div>
      </button>
    </div>

    <div class="motion-sidebar__foot">
      <button class="motion-sidebar__config-btn" @click="emit('open-config')">
        <svg class="motion-sidebar__config-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
          <circle cx="12" cy="12" r="3"/>
          <path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 0 1 0 2.83 2 2 0 0 1-2.83 0l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-2 2 2 2 0 0 1-2-2v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 0 1-2.83 0 2 2 0 0 1 0-2.83l.06-.06a1.65 1.65 0 0 0 .33-1.82 1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1-2-2 2 2 0 0 1 2-2h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 0 1 0-2.83 2 2 0 0 1 2.83 0l.06.06a1.65 1.65 0 0 0 1.82.33H9a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 2-2 2 2 0 0 1 2 2v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 0 1 2.83 0 2 2 0 0 1 0 2.83l-.06.06a1.65 1.65 0 0 0-.33 1.82V9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 2 2 2 2 0 0 1-2 2h-.09a1.65 1.65 0 0 0-1.51 1z"/>
        </svg>
        <span>{{ i18n.t.config }}</span>
      </button>
    </div>
  </aside>
</template>

<style scoped>
/* ============================================================
   侧边栏容器
   ============================================================ */
.motion-sidebar {
  width: 208px;
  flex: none;
  background: var(--bg-panel);
  border-right: 1px solid var(--border-default);
  display: flex;
  flex-direction: column;
}

.motion-sidebar__header {
  font-size: 10px;
  letter-spacing: 1px;
  text-transform: uppercase;
  color: var(--text-muted);
  font-weight: 700;
  padding: 14px 14px 8px;
}

/* ============================================================
   空状态
   ============================================================ */
.motion-sidebar__empty {
  display: flex;
  flex-direction: column;
  gap: 4px;
  padding: 32px 12px;
  text-align: center;
}

.motion-sidebar__empty-title {
  font-size: 13px;
  font-weight: 700;
  color: var(--text-primary);
}

.motion-sidebar__empty-desc {
  font-size: 11px;
  color: var(--text-muted);
  line-height: 1.5;
}

/* ============================================================
   控制器列表
   ============================================================ */
.motion-sidebar__list {
  flex: 1;
  overflow: auto;
  padding: 0 8px;
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.motion-sidebar__item {
  display: flex;
  align-items: center;
  gap: 9px;
  padding: 9px 10px;
  border-radius: 4px;
  cursor: pointer;
  border: 1px solid transparent;
  background: transparent;
  transition: all 0.12s ease;
  text-align: left;
  width: 100%;
}

.motion-sidebar__item:hover {
  background: var(--bg-panel-strong);
}

.motion-sidebar__item--active {
  background: var(--bg-panel-strong);
  border-color: var(--accent-primary);
  box-shadow: inset 2px 0 0 var(--accent-primary);
}

.motion-sidebar__dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  flex: none;
}

.motion-sidebar__dot.ok {
  background: var(--accent-success);
  box-shadow: 0 0 6px var(--accent-success);
}

.motion-sidebar__dot.off {
  background: var(--text-muted);
  opacity: 0.5;
}

.motion-sidebar__item-main {
  min-width: 0;
}

.motion-sidebar__item-name {
  font-size: 13px;
  font-weight: 600;
  color: var(--text-primary);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.motion-sidebar__item-sub {
  font-size: 10.5px;
  color: var(--text-muted);
  margin-top: 1px;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

/* ============================================================
   底部配置按钮
   ============================================================ */
.motion-sidebar__foot {
  padding: 10px 12px;
  border-top: 1px solid var(--border-default);
}

.motion-sidebar__config-btn {
  width: 100%;
  padding: 8px;
  border-radius: 4px;
  background: var(--bg-panel-strong);
  border: 1px solid var(--border-default);
  color: var(--text-muted);
  font-size: 12px;
  font-weight: 600;
  cursor: pointer;
  transition: all 0.12s ease;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 6px;
}

.motion-sidebar__config-btn:hover {
  color: var(--text-primary);
  border-color: var(--accent-primary);
}

.motion-sidebar__config-btn:active {
  transform: scale(0.97);
}

.motion-sidebar__config-icon {
  width: 12px;
  height: 12px;
}

/* ============================================================
   prefers-reduced-motion
   ============================================================ */
@media (prefers-reduced-motion: reduce) {
  .motion-sidebar__config-btn:active {
    transform: none;
  }
  .motion-sidebar__item,
  .motion-sidebar__config-btn {
    transition: none;
  }
}
</style>
