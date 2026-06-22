<script setup lang="ts">
import type { ScanResult } from '@bridge/deviceBridge'
import { Wifi, Monitor, Loader2, Plus, Check } from '@lucide/vue'

const props = defineProps<{
  results: ScanResult[]
  scanning: boolean
  /** 判断扫描结果对应设备是否已被添加；未传则视为全部未添加 */
  isAdded?: (result: ScanResult) => boolean
}>()

const emit = defineEmits<{
  (e: 'add', result: ScanResult): void
}>()

/** 安全包装：调用方未传 isAdded 时默认返回 false */
function added(result: ScanResult): boolean {
  return props.isAdded ? props.isAdded(result) : false
}
</script>

<template>
  <div class="scan">
    <div v-if="scanning" class="scan__loading">
      <Loader2 class="scan__spinner" />
      <span>正在扫描...</span>
    </div>

    <div v-else-if="results.length === 0" class="scan__empty">
      <Monitor class="scan__empty-icon" />
      <p>未发现设备</p>
      <p class="scan__empty-hint">请确保设备已开机并连接在同一网络</p>
    </div>

    <ul v-else class="scan__list">
      <li
        v-for="r in results"
        :key="r.id"
        class="scan__item"
        :class="{ 'scan__item--added': added(r) }"
      >
        <div class="scan__item-icon">
          <Wifi class="scan__item-icon-svg" />
        </div>
        <div class="scan__item-info">
          <span class="scan__item-name">{{ r.name }}</span>
          <span class="scan__item-addr mono">{{ r.address }}:{{ r.port }}</span>
          <span v-if="r.serialNumber" class="scan__item-sn mono">{{ r.serialNumber }}</span>
        </div>
        <div class="scan__item-meta">
          <span v-if="r.macAddress" class="scan__item-mac mono">{{ r.macAddress }}</span>
          <span v-if="r.firmwareVersion" class="scan__item-fw mono">FW {{ r.firmwareVersion }}</span>
        </div>
        <!-- 已添加：显示徽章；未添加：显示 + 按钮 -->
        <span v-if="added(r)" class="scan__item-badge" title="该设备已添加">
          <Check class="scan__item-badge-icon" />
          已添加
        </span>
        <button
          v-else
          class="scan__item-add"
          title="添加此设备"
          @click="emit('add', r)"
        >
          <Plus class="scan__item-add-icon" />
        </button>
      </li>
    </ul>

    <p v-if="!scanning && results.length > 0" class="scan__count">
      发现 {{ results.length }} 台设备
    </p>
  </div>
</template>

<style scoped>
.scan {
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
}

.scan__loading {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 0.6rem;
  padding: 2.5rem 1rem;
  color: var(--text-muted);
  font-size: var(--font-size-sm);
  font-weight: 600;
}

.scan__spinner {
  width: 20px;
  height: 20px;
  animation: spin 1s linear infinite;
}

.scan__empty {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 0.4rem;
  padding: 2rem 1rem;
  color: var(--text-muted);
}

.scan__empty-icon {
  width: 32px;
  height: 32px;
  opacity: 0.4;
  margin-bottom: 0.3rem;
}

.scan__empty p {
  font-size: var(--font-size-sm);
  font-weight: 600;
}

.scan__empty-hint {
  font-size: var(--font-size-xs) !important;
  font-weight: 400 !important;
  color: var(--text-muted);
  text-align: center;
}

.scan__list {
  list-style: none;
  display: flex;
  flex-direction: column;
  gap: 0.35rem;
}

.scan__item {
  display: flex;
  align-items: center;
  gap: 0.7rem;
  padding: 0.65rem 0.75rem;
  border-radius: var(--radius-md);
  background: var(--btn-bg);
  border: 1px solid var(--border-default);
  transition: all var(--motion-fast) var(--easing-standard);
}

.scan__item:hover {
  border-color: var(--accent-border);
  background: var(--accent-soft);
}

/* 已添加状态：视觉降权，并屏蔽 hover 高亮，避免误导用户可点击 */
.scan__item--added {
  opacity: 0.7;
}

.scan__item--added:hover {
  border-color: var(--border-default);
  background: var(--btn-bg);
}

/* "已添加"徽章：使用强调色弱化背景，明确告知设备已存在 */
.scan__item-badge {
  display: inline-flex;
  align-items: center;
  gap: 0.25rem;
  padding: 0.25rem 0.55rem;
  border-radius: var(--radius-sm);
  background: var(--accent-soft);
  border: 1px solid var(--accent-border);
  color: var(--accent);
  font-size: var(--font-size-xs);
  font-weight: 600;
  white-space: nowrap;
  flex-shrink: 0;
}

.scan__item-badge-icon {
  width: 12px;
  height: 12px;
}

.scan__item-icon {
  width: 32px;
  height: 32px;
  border-radius: var(--radius-md);
  background: linear-gradient(135deg, var(--accent) 0%, var(--accent-active) 100%);
  display: flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
}

.scan__item-icon-svg {
  width: 16px;
  height: 16px;
  color: #ffffff;
}

.scan__item-info {
  display: flex;
  flex-direction: column;
  gap: 0.1rem;
  flex: 1;
  min-width: 0;
}

.scan__item-name {
  font-size: var(--font-size-sm);
  font-weight: 700;
  color: var(--text-primary);
}

.scan__item-addr {
  font-size: var(--font-size-xs);
  color: var(--text-muted);
  font-weight: 500;
}

.scan__item-sn {
  font-size: 0.6rem;
  color: var(--text-muted);
}

.scan__item-meta {
  display: flex;
  flex-direction: column;
  align-items: flex-end;
  gap: 0.1rem;
  flex-shrink: 0;
}

.scan__item-mac {
  font-size: 0.55rem;
  color: var(--text-muted);
}

.scan__item-fw {
  font-size: 0.55rem;
  color: var(--text-muted);
}

.scan__item-add {
  width: 28px;
  height: 28px;
  display: flex;
  align-items: center;
  justify-content: center;
  border-radius: var(--radius-sm);
  background: transparent;
  border: 1px solid var(--border-default);
  color: var(--text-muted);
  transition: all var(--motion-fast) var(--easing-standard);
  flex-shrink: 0;
}

.scan__item-add:hover {
  color: var(--accent);
  background: var(--accent-muted);
  border-color: var(--accent-border);
}

.scan__item-add-icon {
  width: 14px;
  height: 14px;
}

.scan__count {
  font-size: var(--font-size-xs);
  color: var(--text-muted);
  font-weight: 600;
  text-align: center;
}

@keyframes spin {
  to { transform: rotate(360deg); }
}
</style>
