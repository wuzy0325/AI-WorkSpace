<script setup lang="ts">
// MotionToolbar — 顶部控制栏
// 显示当前控制器名 + 类型 + 连接/断开/停止/急停按钮。

import { computed } from 'vue'
import type { MotionControllerProfile } from '@shared/types/motion'
import { useI18nStore } from '@stores/i18nStore'

const props = defineProps<{
  /** 全部控制器配置（用于查找当前控制器名/类型） */
  profiles: MotionControllerProfile[]
  /** 当前选中的控制器 ID */
  selectedId: string | null
  /** 当前控制器是否已连接 */
  connected: boolean
}>()

const emit = defineEmits<{
  (e: 'connect'): void
  (e: 'disconnect'): void
  (e: 'stop-all'): void
  (e: 'emergency-stop'): void
}>()

const i18n = useI18nStore()

const currentProfile = computed(() =>
  props.profiles.find((p) => p.id === props.selectedId)
)

const currentName = computed(() =>
  currentProfile.value?.name || i18n.t.selectController
)

const currentType = computed(() => currentProfile.value?.type)
</script>

<template>
  <div class="motion-toolbar">
    <div class="motion-toolbar__current">
      <span class="motion-toolbar__current-label">{{ i18n.t.selectController }}:</span>
      <b class="motion-toolbar__current-name" :title="currentName">{{ currentName }}</b>
      <span v-if="currentType" class="motion-toolbar__current-type">{{ currentType }}</span>
    </div>

    <button
      class="motion-toolbar__btn motion-toolbar__btn--connect"
      :class="{ 'motion-toolbar__btn--connected': connected }"
      @click="connected ? emit('disconnect') : emit('connect')"
      :disabled="!selectedId"
    >
      <span class="motion-toolbar__dot" aria-hidden="true"></span>
      {{ connected ? i18n.t.disconnectBtn : i18n.t.connectBtn }}
    </button>

    <button
      class="motion-toolbar__btn motion-toolbar__btn--stop"
      @click="emit('stop-all')"
      :disabled="!selectedId || !connected"
    >
      {{ i18n.t.stopAll }}
    </button>

    <button
      class="motion-toolbar__btn motion-toolbar__btn--estop"
      @click="emit('emergency-stop')"
      :disabled="!selectedId"
      :title="i18n.t.eStopShortcut"
      :aria-label="i18n.t.eStop"
    >
      {{ i18n.t.eStop }}
    </button>
  </div>
</template>

<style scoped>
/* ============================================================
   控制栏容器
   ============================================================ */
.motion-toolbar {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 12px 16px;
  background: var(--bg-panel);
  border-bottom: 1px solid var(--border-default);
  flex-shrink: 0;
  flex-wrap: wrap;
}

/* ============================================================
   当前控制器信息
   ============================================================ */
.motion-toolbar__current {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 12px;
  color: var(--text-muted);
  min-width: 0;
  flex: 1;
}

.motion-toolbar__current-label {
  color: var(--text-muted);
  white-space: nowrap;
}

.motion-toolbar__current-name {
  color: var(--text-primary);
  font-weight: 600;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.motion-toolbar__current-type {
  font-size: 10px;
  font-weight: 700;
  letter-spacing: 0.05em;
  color: var(--text-muted);
  background: var(--bg-panel-strong);
  border: 1px solid var(--border-default);
  border-radius: 3px;
  padding: 2px 7px;
  white-space: nowrap;
  flex-shrink: 0;
}

/* ============================================================
   控制按钮通用
   ============================================================ */
.motion-toolbar__btn {
  display: inline-flex;
  align-items: center;
  gap: 7px;
  border-radius: 4px;
  padding: 8px 14px;
  font-size: 13px;
  font-weight: 600;
  cursor: pointer;
  border: 1px solid var(--border-default);
  background: var(--bg-panel-strong);
  color: var(--text-primary);
  transition: all 0.15s ease;
  white-space: nowrap;
}

.motion-toolbar__btn:hover:not(:disabled) {
  border-color: var(--accent-primary);
}

.motion-toolbar__btn:active:not(:disabled) {
  transform: scale(0.96);
}

.motion-toolbar__btn:disabled {
  opacity: 0.4;
  cursor: not-allowed;
}

/* ============================================================
   连接按钮
   ============================================================ */
.motion-toolbar__dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: var(--text-muted);
  flex-shrink: 0;
}

.motion-toolbar__btn--connect.motion-toolbar__btn--connected .motion-toolbar__dot {
  background: var(--accent-success);
  box-shadow: 0 0 8px var(--accent-success);
}

/* ============================================================
   停止按钮（警告色）
   ============================================================ */
.motion-toolbar__btn--stop {
  color: var(--accent-warning);
  border-color: color-mix(in srgb, var(--accent-warning) 35%, var(--border-default));
}

.motion-toolbar__btn--stop:hover:not(:disabled) {
  background: color-mix(in srgb, var(--accent-warning) 12%, transparent);
  border-color: var(--accent-warning);
}

/* ============================================================
   急停按钮（危险色 + 脉冲动画）
   ============================================================ */
.motion-toolbar__btn--estop {
  background: var(--accent-danger);
  border-color: var(--accent-danger);
  color: var(--color-on-danger);
  position: relative;
}

.motion-toolbar__btn--estop::after {
  content: '';
  position: absolute;
  inset: -1px;
  border-radius: inherit;
  border: 1px solid var(--accent-danger);
  animation: motion-estop-pulse 1.4s ease-out infinite;
  pointer-events: none;
}

.motion-toolbar__btn--estop:hover:not(:disabled) {
  filter: brightness(1.1);
}

.motion-toolbar__btn--estop:disabled::after {
  animation: none;
}

@keyframes motion-estop-pulse {
  0% { opacity: 0.7; transform: scale(1); }
  100% { opacity: 0; transform: scale(1.18); }
}

/* ============================================================
   prefers-reduced-motion：禁用脉冲与按压位移
   ============================================================ */
@media (prefers-reduced-motion: reduce) {
  .motion-toolbar__btn--estop::after {
    animation: none;
  }
  .motion-toolbar__btn:active:not(:disabled) {
    transform: none;
  }
  .motion-toolbar__btn {
    transition: none;
  }
}
</style>
