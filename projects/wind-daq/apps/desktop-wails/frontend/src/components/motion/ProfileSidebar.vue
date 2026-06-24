<script setup lang="ts">
import type { MotionControllerProfile } from '@shared/types/motion'
import { useMotionStore } from '@stores/motionStore'

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
    /** 新建草稿当前的地址，用于占位项副标题显示 */
    draftAddress?: string
    /** 新建草稿当前的端口，用于占位项副标题显示 */
    draftPort?: number
  }>(),
  {
    creating: false,
    draftName: '',
    draftType: '',
    draftAddress: '',
    draftPort: 0,
  },
)

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
      <!-- 新建草稿占位项：仅在新建态显示，作为列表中唯一的高亮活动项 -->
      <div
        v-if="creating"
        class="config-sidebar__item config-sidebar__item--draft config-sidebar__item--active"
        role="status"
        aria-label="正在新建控制器"
      >
        <div class="config-sidebar__item-main">
          <span class="config-sidebar__draft-icon" aria-hidden="true">
            <svg width="10" height="10" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="3" stroke-linecap="round" stroke-linejoin="round">
              <line x1="12" y1="5" x2="12" y2="19" />
              <line x1="5" y1="12" x2="19" y2="12" />
            </svg>
          </span>
          <div class="config-sidebar__item-content">
            <span class="config-sidebar__item-name config-sidebar__item-name--draft">
              {{ draftName?.trim() || '未命名控制器' }}
            </span>
            <span class="config-sidebar__item-meta config-sidebar__item-meta--draft">
              <span class="config-sidebar__draft-dot"></span>
              新建中 · {{ draftType || '待选类型' }}<template v-if="draftAddress"> · {{ draftAddress }}<template v-if="draftPort">:{{ draftPort }}</template></template>
            </span>
          </div>
        </div>
      </div>

      <button
        v-for="p in profiles"
        :key="p.id"
        type="button"
        class="config-sidebar__item"
        :class="{
          'config-sidebar__item--active': !creating && activeId === p.id,
          'config-sidebar__item--dimmed': creating,
        }"
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

      <div v-if="profiles.length === 0 && !creating" class="config-sidebar__empty">
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

/* ============================================================
   新建草稿占位项 —— 让用户在侧边栏直接"看见"正在新建的设备
   ============================================================ */
.config-sidebar__item--draft {
  position: relative;
  cursor: default;
  border-style: dashed !important;
  border-color: color-mix(in srgb, var(--accent-primary) 55%, transparent) !important;
  background: color-mix(in srgb, var(--accent-primary) 10%, transparent) !important;
  box-shadow:
    0 0 0 1px color-mix(in srgb, var(--accent-primary) 18%, transparent),
    0 2px 12px -2px color-mix(in srgb, var(--accent-primary) 25%, transparent);
  animation: draft-item-in 0.25s ease-out;
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
  background: var(--accent-primary);
}

.config-sidebar__item--draft:hover {
  transform: none;
}

.config-sidebar__draft-icon {
  width: 0.5rem;
  height: 0.5rem;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
  border-radius: 50%;
  background: var(--accent-primary);
  color: var(--color-brand-foreground, #fff);
  box-shadow: 0 0 0 2px var(--bg-panel-strong), 0 0 8px color-mix(in srgb, var(--accent-primary) 60%, transparent);
}

.config-sidebar__item-name--draft {
  color: var(--accent-primary) !important;
  font-style: italic;
}

.config-sidebar__item-meta--draft {
  display: inline-flex;
  align-items: center;
  gap: 0.25rem;
  color: var(--accent-primary) !important;
  font-weight: 600;
}

.config-sidebar__draft-dot {
  width: 5px;
  height: 5px;
  border-radius: 50%;
  background: var(--accent-primary);
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
