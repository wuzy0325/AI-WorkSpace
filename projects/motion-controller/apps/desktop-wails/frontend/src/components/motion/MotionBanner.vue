<script setup lang="ts">
// MotionBanner — 急停 / 错误横幅
// 从 MotionControlPanel 抽出，便于复用与单独测试。

import { useI18nStore } from '@stores/i18nStore'

defineProps<{
  /** 急停已触发 */
  emergencyStopped: boolean
  /** 控制器最近的错误信息（空字符串表示无错误） */
  lastError: string
}>()

const emit = defineEmits<{
  (e: 'reset-emergency-stop'): void
  (e: 'clear-error'): void
}>()

const i18n = useI18nStore()
</script>

<template>
  <!-- 急停横幅 -->
  <div v-if="emergencyStopped" class="motion-banner motion-banner--estop" role="alert">
    <svg class="motion-banner__icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
      <path d="M10.29 3.86 1.82 18a2 2 0 0 0 1.71 3h16.94a2 2 0 0 0 1.71-3L13.71 3.86a2 2 0 0 0-3.42 0z"/>
      <line x1="12" y1="9" x2="12" y2="13"/>
      <line x1="12" y1="17" x2="12.01" y2="17"/>
    </svg>
    <span class="motion-banner__text">
      {{ i18n.t.eStopActive }} — {{ i18n.t.eStopResetHint }}
    </span>
    <button
      class="motion-banner__action"
      :aria-label="i18n.t.eStopReset"
      @click="emit('reset-emergency-stop')"
    >
      {{ i18n.t.eStopReset }}
    </button>
  </div>

  <!-- 错误横幅 -->
  <div v-else-if="lastError" class="motion-banner motion-banner--error" role="alert">
    <svg class="motion-banner__icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
      <circle cx="12" cy="12" r="10"/>
      <line x1="12" y1="8" x2="12" y2="12"/>
      <line x1="12" y1="16" x2="12.01" y2="16"/>
    </svg>
    <div class="motion-banner__body">
      <span class="motion-banner__title">{{ i18n.t.controllerAlarm }}</span>
      <span class="motion-banner__msg">{{ lastError }}</span>
    </div>
    <button
      class="motion-banner__close"
      :aria-label="i18n.t.clearError"
      :title="i18n.t.close"
      @click="emit('clear-error')"
    >
      <svg class="motion-banner__close-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
        <line x1="18" y1="6" x2="6" y2="18"/>
        <line x1="6" y1="6" x2="18" y2="18"/>
      </svg>
    </button>
  </div>
</template>

<style scoped>
/* ============================================================
   横幅容器
   ============================================================ */
.motion-banner {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 10px 16px;
  font-size: 13px;
  flex-shrink: 0;
}

/* ============================================================
   急停横幅
   ============================================================ */
.motion-banner--estop {
  background: linear-gradient(
    90deg,
    var(--accent-danger-banner-from),
    var(--accent-danger-banner-to)
  );
  color: var(--color-on-danger);
  font-weight: 700;
  border-bottom: 1px solid var(--accent-danger-banner-border);
}

.motion-banner--estop .motion-banner__action {
  margin-left: auto;
  background: var(--accent-danger-banner-action-bg);
  border: 1px solid var(--accent-danger-banner-border);
  color: var(--accent-danger-banner-action-fg);
  padding: 5px 12px;
  border-radius: 3px;
  font-weight: 600;
  cursor: pointer;
  font-size: 12px;
  transition: background 0.15s ease;
}

.motion-banner--estop .motion-banner__action:hover {
  background: var(--accent-danger-banner-action-bg-hover);
}

.motion-banner--estop .motion-banner__action:active {
  transform: scale(0.96);
}

/* ============================================================
   错误横幅
   ============================================================ */
.motion-banner--error {
  background: color-mix(in srgb, var(--accent-danger) 12%, transparent);
  border-bottom: 1px solid color-mix(in srgb, var(--accent-danger) 35%, transparent);
  color: var(--accent-danger);
}

.motion-banner__body {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 1px;
}

.motion-banner__title {
  font-size: 10px;
  font-weight: 700;
  text-transform: uppercase;
  letter-spacing: 0.06em;
  color: var(--accent-danger);
}

.motion-banner__msg {
  font-size: 12.5px;
  font-weight: 600;
  color: var(--text-primary);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.motion-banner__close {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 22px;
  height: 22px;
  border-radius: 3px;
  color: var(--accent-danger);
  background: transparent;
  border: none;
  cursor: pointer;
  transition: background 0.15s ease;
  flex-shrink: 0;
}

.motion-banner__close:hover {
  background: color-mix(in srgb, var(--accent-danger) 15%, transparent);
}

/* ============================================================
   通用元素
   ============================================================ */
.motion-banner__icon {
  width: 16px;
  height: 16px;
  flex-shrink: 0;
}

.motion-banner__close-icon {
  width: 12px;
  height: 12px;
}

/* ============================================================
   prefers-reduced-motion：禁用所有 transform/opacity 位移动画
   ============================================================ */
@media (prefers-reduced-motion: reduce) {
  .motion-banner__action:active,
  .motion-banner__close:active {
    transform: none;
  }
}
</style>
