<script setup lang="ts">
import { inject } from 'vue'

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
        <input v-model="model.name" class="config-field__input" placeholder="控制器名称" />
      </div>
      <div class="config-field">
        <label class="config-field__label">
          类型
          <span class="field-hint" @mouseenter="tooltip('选择运动控制器的硬件型号', $event)" @mouseleave="hideTooltip">?</span>
        </label>
        <select v-model="model.type" class="config-field__select">
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
        <input v-model="model.address" class="config-field__input" placeholder="127.0.0.1" />
      </div>
      <div class="config-field">
        <label class="config-field__label">
          端口
          <span class="field-hint" @mouseenter="tooltip('控制器的通信端口号', $event)" @mouseleave="hideTooltip">?</span>
        </label>
        <input v-model.number="model.port" type="number" class="config-field__input" min="1" max="65535" />
      </div>
      <div class="config-field config-field--toggle">
        <label class="config-field__label">
          自动连接
          <span class="field-hint" @mouseenter="tooltip('程序启动时自动连接此控制器', $event)" @mouseleave="hideTooltip">?</span>
        </label>
        <label class="toggle-switch">
          <input type="checkbox" v-model="model.autoConnect" />
          <span class="toggle-switch__track">
            <span class="toggle-switch__thumb"></span>
          </span>
        </label>
      </div>
    </div>
  </div>
</template>

<style scoped>
.config-section {
  margin-bottom: 1rem;
}
.config-section__title {
  font-size: 0.75rem;
  font-weight: 700;
  color: var(--text-muted);
  letter-spacing: 0.05em;
  text-transform: uppercase;
  margin-bottom: 0.625rem;
}
.conn-grid {
  display: grid;
  grid-template-columns: repeat(2, 1fr);
  gap: 0.5rem 0.625rem;
}
.config-field {
  display: flex;
  flex-direction: column;
  gap: 0.25rem;
}
.config-field--toggle {
  grid-column: span 2;
  flex-direction: row;
  align-items: center;
  justify-content: space-between;
  padding: 0.25rem 0;
}
.config-field__label {
  font-size: 0.7rem;
  font-weight: 600;
  color: var(--text-muted);
}
.config-field__input {
  height: 2rem;
  padding: 0 0.625rem;
  border-radius: 0.25rem;
  border: 1px solid var(--border-default);
  background: var(--bg-panel-strong);
  color: var(--text-primary);
  font-size: 0.8rem;
  outline: none;
  transition: border-color 0.2s ease, box-shadow 0.2s ease;
}
.config-field__input:focus {
  border-color: var(--accent-success);
  box-shadow: 0 0 0 2px color-mix(in srgb, var(--accent-success) 20%, transparent);
}
.config-field__select {
  height: 2rem;
  padding: 0 0.625rem;
  border-radius: 0.25rem;
  border: 1px solid var(--border-default);
  background: var(--bg-panel-strong);
  color: var(--text-primary);
  font-size: 0.8rem;
  outline: none;
  cursor: pointer;
  transition: border-color 0.2s ease, box-shadow 0.2s ease;
}
.config-field__select:focus {
  border-color: var(--accent-success);
  box-shadow: 0 0 0 2px color-mix(in srgb, var(--accent-success) 20%, transparent);
}
.toggle-switch {
  display: inline-flex;
  align-items: center;
  cursor: pointer;
}
.toggle-switch input {
  display: none;
}
.toggle-switch__track {
  width: 2.25rem;
  height: 1.125rem;
  border-radius: 9999px;
  background: var(--border-strong);
  position: relative;
  transition: background 0.2s ease;
}
.toggle-switch input:checked + .toggle-switch__track {
  background: var(--accent-success);
}
.toggle-switch__thumb {
  position: absolute;
  left: 2px;
  top: 2px;
  width: calc(1.125rem - 4px);
  height: calc(1.125rem - 4px);
  border-radius: 50%;
  background: white;
  transition: transform 0.2s ease;
}
.toggle-switch input:checked + .toggle-switch__track .toggle-switch__thumb {
  transform: translateX(1.125rem);
}
</style>