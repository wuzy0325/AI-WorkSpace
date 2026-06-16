<script setup lang="ts">
import type { ScanResult } from '@bridge/deviceBridge'
import { Wifi, Monitor, Loader2, Plus } from '@lucide/vue'

defineProps<{
  results: ScanResult[]
  scanning: boolean
}>()

const emit = defineEmits<{
  (e: 'add', result: ScanResult): void
}>()
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
        <button
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
