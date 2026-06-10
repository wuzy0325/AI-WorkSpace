<script setup lang="ts">
import { inject } from 'vue'
import UiCheckbox from '@components/ui/UiCheckbox.vue'
import UiInput from '@components/ui/UiInput.vue'
import UiInputNumber from '@components/ui/UiInputNumber.vue'
import UiSelect from '@components/ui/UiSelect.vue'

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
      <svg class="w-4 h-4 inline-block mr-1.5 -mt-0.5" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M5 12h14"/><path d="m12 5 7 7-7 7"/></svg>
      通信设置
    </h3>
    <div class="config-grid">
      <div class="config-field">
        <label class="config-field__label">
          名称
          <span class="field-hint" @mouseenter="tooltip('控制器的显示名称，用于区分多个控制器', $event)" @mouseleave="hideTooltip">?</span>
        </label>
        <UiInput v-model="model.name" placeholder="控制器名称" />
      </div>
      <div class="config-field">
        <label class="config-field__label">
          类型
          <span class="field-hint" @mouseenter="tooltip('选择运动控制器的硬件型号', $event)" @mouseleave="hideTooltip">?</span>
        </label>
        <UiSelect v-model="model.type" :options="[{value:'SIMULATED-MC',label:'模拟控制器'},{value:'B140-MC',label:'B140 控制器'},{value:'WTNMC4A-MC',label:'WTNMC4A 控制器'}]" />
      </div>
      <div class="config-field">
        <label class="config-field__label">
          地址
          <span class="field-hint" @mouseenter="tooltip('控制器的 IP 地址或主机名', $event)" @mouseleave="hideTooltip">?</span>
        </label>
        <UiInput v-model="model.address" placeholder="127.0.0.1" />
      </div>
      <div class="config-field">
        <label class="config-field__label">
          端口
          <span class="field-hint" @mouseenter="tooltip('控制器的通信端口号', $event)" @mouseleave="hideTooltip">?</span>
        </label>
        <UiInputNumber v-model="model.port" class="input-width-80" :min="1" :max="65535" />
      </div>
      <div class="config-field config-field--toggle">
        <label class="config-field__label">
          自动连接
          <span class="field-hint" @mouseenter="tooltip('程序启动时自动连接此控制器', $event)" @mouseleave="hideTooltip">?</span>
        </label>
        <label class="toggle-switch">
          <UiCheckbox v-model:checked="model.autoConnect" />
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
  margin-bottom: 1.5rem;
}
.config-section__title {
  font-size: 0.8rem;
  font-weight: 700;
  color: var(--color-text-secondary);
  letter-spacing: 0.05em;
  text-transform: uppercase;
  margin-bottom: 0.875rem;
}
.config-grid {
  display: grid;
  grid-template-columns: repeat(2, 1fr);
  gap: var(--space-3);
}
.config-field {
  display: flex;
  flex-direction: column;
  gap: 0.375rem;
}
.config-field--toggle {
  flex-direction: row;
  align-items: center;
  justify-content: space-between;
}
.config-field__label {
  font-size: var(--text-sm);
  font-weight: 600;
  color: var(--color-text-muted);
}
.config-field__input {
  height: 2.25rem;
  padding: 0 var(--space-3);
  border-radius: var(--radius-lg);
  border: 1px solid rgba(255, 255, 255, 0.1);
  background: rgba(0, 0, 0, 0.15);
  color: var(--color-text-primary);
  font-size: 0.85rem;
  outline: none;
  transition: border-color 0.2s ease, box-shadow 0.2s ease;
}
:root[data-theme='light'] .config-field__input {
  border: 1px solid rgba(0, 0, 0, 0.1);
  background: rgba(0, 0, 0, 0.04);
  color: var(--color-text-primary);
}
.config-field__input:focus {
  border-color: var(--color-success);
  box-shadow: 0 0 0 3px rgba(16, 185, 129, 0.2);
}
.config-field__input--short {
  max-width: 8rem;
}
.config-field__select {
  height: 2.25rem;
  padding: 0 var(--space-3);
  border-radius: var(--radius-lg);
  border: 1px solid rgba(255, 255, 255, 0.1);
  background: rgba(0, 0, 0, 0.15);
  color: var(--color-text-primary);
  font-size: 0.85rem;
  outline: none;
  cursor: pointer;
  transition: border-color 0.2s ease, box-shadow 0.2s ease;
}
:root[data-theme='light'] .config-field__select {
  border: 1px solid rgba(0, 0, 0, 0.1);
  background: rgba(0, 0, 0, 0.04);
  color: var(--color-text-primary);
}
.config-field__select:focus {
  border-color: var(--color-success);
  box-shadow: 0 0 0 3px rgba(16, 185, 129, 0.2);
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
  width: 2.5rem;
  height: var(--space-5);
  border-radius: 9999px;
  background: rgba(0, 0, 0, 0.3);
  position: relative;
  transition: background 0.2s ease;
}
.toggle-switch input:checked + .toggle-switch__track {
  background: var(--color-success);
}
.toggle-switch__thumb {
  position: absolute;
  left: 2px;
  top: 2px;
  width: calc(1.25rem - 4px);
  height: calc(1.25rem - 4px);
  border-radius: 50%;
  background: white;
  transition: transform 0.2s ease;
}
.toggle-switch input:checked + .toggle-switch__track .toggle-switch__thumb {
  transform: translateX(1.25rem);
}

.input-width-80 {
  width: 80px;
}
</style>