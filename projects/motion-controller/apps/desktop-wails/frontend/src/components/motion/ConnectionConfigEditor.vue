<script setup lang="ts">
import { inject } from 'vue'
import UiToggle from '@components/ui/UiToggle.vue'

const model = defineModel<{
  name: string
  type: string
  address: string
  port: number
  autoConnect: boolean
}>({ required: true })

const tooltip = inject<(text: string, event: MouseEvent) => void>('showTooltip', () => {})
const hideTooltip = inject<() => void>('hideTooltip', () => {})
</script>

<template>
  <div class="config-section">
    <h3 class="config-section__title">
      <svg class="w-3.5 h-3.5 inline-block mr-1 -mt-0.5" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M5 12h14"/><path d="m12 5 7 7-7 7"/></svg>
      通信设置
    </h3>
    <div class="conn-grid">
      <div class="config-field">
        <label class="config-field__label">
          名称
          <span class="field-hint" @mouseenter="tooltip('控制器的显示名称，用于区分多个控制器', $event)" @mouseleave="hideTooltip">?</span>
        </label>
        <input v-model="model.name" class="config-field__input config-input" placeholder="控制器名称" />
      </div>
      <div class="config-field">
        <label class="config-field__label">
          类型
          <span class="field-hint" @mouseenter="tooltip('选择运动控制器的硬件型号', $event)" @mouseleave="hideTooltip">?</span>
        </label>
        <select v-model="model.type" class="config-field__select config-select">
          <option value="SIMULATED-MC">模拟控制器</option>
          <option value="B140-MC">B140 控制器</option>
          <option value="WTNMC4A-MC">WTNMC4A 控制器</option>
        </select>
      </div>
      <div class="config-field">
        <label class="config-field__label">
          地址
          <span class="field-hint" @mouseenter="tooltip('控制器的 IP 地址或主机名', $event)" @mouseleave="hideTooltip">?</span>
        </label>
        <input v-model="model.address" class="config-field__input config-input" placeholder="127.0.0.1" />
      </div>
      <div class="config-field">
        <label class="config-field__label">
          端口
          <span class="field-hint" @mouseenter="tooltip('控制器的通信端口号', $event)" @mouseleave="hideTooltip">?</span>
        </label>
        <input v-model.number="model.port" type="number" class="config-field__input config-input" min="1" max="65535" />
      </div>
      <div class="config-field config-field--toggle">
        <label class="config-field__label">
          自动连接
          <span class="field-hint" @mouseenter="tooltip('程序启动时自动连接此控制器', $event)" @mouseleave="hideTooltip">?</span>
        </label>
        <UiToggle v-model="model.autoConnect" />
      </div>
    </div>
  </div>
</template>

<style scoped>
/* ============================================================
   通信设置区域
   ============================================================ */
.config-section {
  margin-bottom: var(--space-4);
}

.config-section__title {
  font-size: 0.75rem;
  font-weight: 700;
  color: var(--text-muted);
  letter-spacing: 0.06em;
  text-transform: uppercase;
  margin-bottom: var(--space-3);
}

.conn-grid {
  display: grid;
  grid-template-columns: repeat(2, 1fr);
  gap: var(--space-2) var(--space-3);
}

.config-field {
  display: flex;
  flex-direction: column;
  gap: var(--space-1);
}

.config-field--toggle {
  grid-column: span 2;
  flex-direction: row;
  align-items: center;
  justify-content: space-between;
  padding: var(--space-1) 0;
}

.config-field__label {
  font-size: 0.6875rem;
  font-weight: 600;
  color: var(--text-muted);
}

/* 输入框和选择框 — 基础样式继承全局 .config-input / .config-select */
.config-field__input,
.config-field__select {
  /* 仅补充组件特有样式 */
}

.config-field__select {
  cursor: pointer;
}

.config-field__input:focus,
.config-field__select:focus {
  border-color: var(--accent-primary);
  box-shadow: 0 0 0 2px var(--accent-primary-muted);
}
</style>