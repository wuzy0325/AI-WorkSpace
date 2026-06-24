<script setup lang="ts">
import type { MotionControllerProfile } from '@shared/types/motion'

withDefaults(
  defineProps<{
    profiles: MotionControllerProfile[]
    activeId: string
    /** 是否正处于新建态（草稿尚未保存） */
    creating?: boolean
    /** 新建草稿当前的控制器名称，用于侧边栏占位项的实时显示 */
    draftName?: string
    /** 新建草稿当前的控制器类型，用于占位项副标题显示 */
    draftType?: string
  }>(),
  {
    creating: false,
    draftName: '',
    draftType: '',
  },
)

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
      <!-- 新建草稿占位项：仅在新建态显示，作为列表中唯一的高亮活动项 -->
      <div
        v-if="creating"
        class="config-sidebar__item config-sidebar__item--draft config-sidebar__item--active"
        role="status"
        aria-label="正在新建控制器"
      >
        <div class="config-sidebar__item-row">
          <div class="config-sidebar__item-icon config-sidebar__item-icon--draft">
            <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round">
              <circle cx="12" cy="12" r="10" />
              <line x1="12" y1="8" x2="12" y2="16" />
              <line x1="8" y1="12" x2="16" y2="12" />
            </svg>
          </div>
          <div class="config-sidebar__item-content">
            <span class="config-sidebar__item-name config-sidebar__item-name--draft">
              {{ draftName?.trim() || '未命名控制器' }}
            </span>
            <span class="config-sidebar__item-type config-sidebar__item-type--draft">
              <span class="config-sidebar__draft-dot"></span>
              新建中 · {{ draftType || '待选类型' }}
            </span>
          </div>
        </div>
      </div>

      <button
        v-for="p in profiles"
        :key="p.id"
        @click="emit('select', p.id)"
        class="config-sidebar__item"
        :class="{ 'config-sidebar__item--active': !creating && activeId === p.id, 'config-sidebar__item--dimmed': creating }"
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
      <div v-if="profiles.length === 0 && !creating" class="config-sidebar__empty">
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

/* ============================================================
   新建草稿占位项 —— 让用户在侧边栏直接"看见"正在新建的设备
   ============================================================ */
.config-sidebar__item--draft {
  position: relative;
  cursor: default;
  border-style: dashed !important;
  border-color: color-mix(in srgb, var(--accent-success) 55%, transparent) !important;
  background: color-mix(in srgb, var(--accent-success) 10%, transparent) !important;
  box-shadow: 0 0 0 1px color-mix(in srgb, var(--accent-success) 18%, transparent),
              0 2px 12px -2px color-mix(in srgb, var(--accent-success) 25%, transparent) !important;
  animation: draft-item-in 0.25s var(--easing-standard, ease-out);
}

/* 草稿项左侧竖向高亮条 */
.config-sidebar__item--draft::before {
  content: '';
  position: absolute;
  left: -1px;
  top: 6px;
  bottom: 6px;
  width: 3px;
  border-radius: 2px;
  background: var(--accent-success);
}

.config-sidebar__item--draft:hover {
  transform: none;
}

.config-sidebar__item-icon--draft {
  background: color-mix(in srgb, var(--accent-success) 25%, transparent) !important;
  color: var(--accent-success) !important;
  box-shadow: 0 0 8px -2px color-mix(in srgb, var(--accent-success) 70%, transparent);
}

.config-sidebar__item-name--draft {
  color: var(--accent-success) !important;
  font-style: italic;
}

.config-sidebar__item-type--draft {
  display: inline-flex;
  align-items: center;
  gap: 0.25rem;
  color: var(--accent-success) !important;
  font-weight: 600;
}

.config-sidebar__draft-dot {
  width: 5px;
  height: 5px;
  border-radius: 50%;
  background: var(--accent-success);
  animation: draft-pulse 1.5s ease-in-out infinite;
  flex-shrink: 0;
}

/* 新建态下其余已存在控制器淡化，凸显草稿项 */
.config-sidebar__item--dimmed {
  opacity: 0.55;
}
.config-sidebar__item--dimmed:hover {
  opacity: 1;
}

@keyframes draft-item-in {
  from {
    opacity: 0;
    transform: translateY(-4px);
  }
  to {
    opacity: 1;
    transform: translateY(0);
  }
}

@keyframes draft-pulse {
  0%, 100% { opacity: 1; transform: scale(1); }
  50% { opacity: 0.4; transform: scale(0.7); }
}
</style>
