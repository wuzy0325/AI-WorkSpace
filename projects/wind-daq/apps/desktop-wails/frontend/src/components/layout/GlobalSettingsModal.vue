<script setup lang="ts">
import { computed, nextTick, ref, watch } from 'vue'
import { storeToRefs } from 'pinia'
import { useFeedbackStore } from '@stores/feedbackStore'
import { useI18nStore } from '@stores/i18nStore'
import { deviceApi, storageApi } from '@api/deviceApi'
import { wailsApi } from '@api/wails-adapter'
import UiButton from '@components/ui/UiButton.vue'
import UiToggle from '@components/ui/UiToggle.vue'

interface LocalStorageSettings {
  baseDirectory: string
  filePrefix: string
  autoStartOnAcquisition: boolean
  stopConditions: {
    maxDurationMs?: number
    maxFileSizeBytes?: number
    maxRecordCount?: number
  }
}

const STORAGE_KEY = 'wind-daq.global-settings'

const props = defineProps<{ open: boolean }>()
const emit = defineEmits<{
  (e: 'close'): void
  (e: 'update:open', value: boolean): void
}>()

const i18n = useI18nStore()
const feedback = useFeedbackStore()
const { t } = storeToRefs(i18n)
const modalRef = ref<HTMLElement | null>(null)

const loading = ref(false)
const saving = ref(false)
const loadError = ref(false)
const baseDirectory = ref('data/recordings')
const filePrefix = ref('run')
const autoStart = ref(false)
const durationEnabled = ref(false)
const durationMinutes = ref(10)
const sizeEnabled = ref(false)
const sizeMb = ref(100)
const countEnabled = ref(false)
const recordCount = ref(1000000)
const refreshRate = ref(20)
const originalRefreshRate = ref(20)
const validationError = ref('')

const isVisible = computed({
  get: () => props.open,
  set: (value) => emit('update:open', value),
})

const enabledConditionsCount = computed(() =>
  [durationEnabled.value, sizeEnabled.value, countEnabled.value].filter(Boolean).length,
)

watch(
  () => props.open,
  (open) => {
    if (open) {
      void loadSettings()
      nextTick(() => modalRef.value?.focus())
    }
  },
)

async function loadSettings(): Promise<void> {
  loading.value = true
  loadError.value = false
  validationError.value = ''

  try {
    const saved = readLocalSettings()
    applySettings(saved)

    try {
      const status = await storageApi.status()
      if (status.outputDir) baseDirectory.value = status.outputDir
    } catch {
      // Recording status is optional for opening settings.
    }

    try {
      const hz = await deviceApi.getPublishRate()
      refreshRate.value = Math.round(hz)
      originalRefreshRate.value = Math.round(hz)
    } catch {
      refreshRate.value = originalRefreshRate.value
    }
  } catch {
    loadError.value = true
  } finally {
    loading.value = false
  }
}

function readLocalSettings(): LocalStorageSettings {
  try {
    const raw = window.localStorage.getItem(STORAGE_KEY)
    if (raw) return JSON.parse(raw) as LocalStorageSettings
  } catch {
    // Ignore corrupt local settings and fall back to defaults.
  }
  return {
    baseDirectory: 'data/recordings',
    filePrefix: 'run',
    autoStartOnAcquisition: false,
    stopConditions: {},
  }
}

function applySettings(settings: LocalStorageSettings): void {
  baseDirectory.value = settings.baseDirectory || 'data/recordings'
  filePrefix.value = settings.filePrefix || 'run'
  autoStart.value = settings.autoStartOnAcquisition
  durationEnabled.value = !!settings.stopConditions.maxDurationMs
  durationMinutes.value = settings.stopConditions.maxDurationMs
    ? Math.max(1, Math.round(settings.stopConditions.maxDurationMs / 60000))
    : 10
  sizeEnabled.value = !!settings.stopConditions.maxFileSizeBytes
  sizeMb.value = settings.stopConditions.maxFileSizeBytes
    ? Math.max(1, Math.round(settings.stopConditions.maxFileSizeBytes / 1048576))
    : 100
  countEnabled.value = !!settings.stopConditions.maxRecordCount
  recordCount.value = settings.stopConditions.maxRecordCount || 1000000
}

function currentSettings(): LocalStorageSettings {
  const stopConditions: LocalStorageSettings['stopConditions'] = {}
  if (durationEnabled.value) stopConditions.maxDurationMs = durationMinutes.value * 60000
  if (sizeEnabled.value) stopConditions.maxFileSizeBytes = sizeMb.value * 1048576
  if (countEnabled.value) stopConditions.maxRecordCount = recordCount.value

  return {
    baseDirectory: baseDirectory.value.trim(),
    filePrefix: filePrefix.value.trim(),
    autoStartOnAcquisition: autoStart.value,
    stopConditions,
  }
}

function validate(): boolean {
  if (!baseDirectory.value.trim()) validationError.value = '保存目录不能为空'
  else if (!filePrefix.value.trim()) validationError.value = '文件前缀不能为空'
  else if (durationEnabled.value && (durationMinutes.value < 1 || durationMinutes.value > 1440)) validationError.value = '定时停止范围为 1 到 1440 分钟'
  else if (sizeEnabled.value && (sizeMb.value < 1 || sizeMb.value > 10000)) validationError.value = '文件大小范围为 1 到 10000 MB'
  else if (countEnabled.value && (recordCount.value < 1 || recordCount.value > 100000000)) validationError.value = '记录数范围为 1 到 100000000'
  else if (refreshRate.value < 1 || refreshRate.value > 20) validationError.value = '刷新率范围为 1 到 20 Hz'
  else validationError.value = ''

  return !validationError.value
}

function formatNumber(num: number): string {
  if (num >= 1000000) return (num / 1000000).toFixed(1) + 'M'
  if (num >= 1000) return (num / 1000).toFixed(1) + 'K'
  return String(num)
}

function onClose(): void {
  if (saving.value) return
  isVisible.value = false
  emit('close')
}

async function handlePickDirectory(): Promise<void> {
  try {
    const dir = await wailsApi.app.pickDirectory()
    if (dir) {
      baseDirectory.value = dir
    }
  } catch {
    feedback.pushToast('选择目录失败', 'error')
  }
}

async function onSave(): Promise<void> {
  if (!validate()) return

  saving.value = true
  try {
    window.localStorage.setItem(STORAGE_KEY, JSON.stringify(currentSettings()))
    if (refreshRate.value !== originalRefreshRate.value) {
      await deviceApi.setPublishRate(refreshRate.value)
      originalRefreshRate.value = refreshRate.value
    }
    feedback.pushToast('设置已保存', 'success')
    onClose()
  } catch {
    feedback.pushToast('保存失败，请重试', 'error')
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <Teleport to="body">
    <Transition
      enter-active-class="transition ease-out duration-200"
      enter-from-class="opacity-0"
      enter-to-class="opacity-100"
      leave-active-class="transition ease-in duration-150"
      leave-from-class="opacity-100"
      leave-to-class="opacity-0"
    >
      <div v-show="isVisible" class="modal-overlay" @click="onClose">
        <Transition
          enter-active-class="transition ease-out duration-300"
          enter-from-class="opacity-0 scale-95 translate-y-4"
          enter-to-class="opacity-100 scale-100 translate-y-0"
          leave-active-class="transition ease-in duration-200"
          leave-from-class="opacity-100 scale-100 translate-y-0"
          leave-to-class="opacity-0 scale-95 translate-y-4"
        >
          <div
            v-show="isVisible"
            ref="modalRef"
            class="modal-container"
            tabindex="-1"
            @click.stop
            @keydown.esc="onClose"
          >
            <header class="modal-header">
              <div class="modal-header__content">
                <h2 class="modal-header__title">全局设置</h2>
                <p class="modal-header__subtitle">数据保存、自动停止条件和实时刷新频率</p>
              </div>
              <UiButton variant="ghost" size="sm" :disabled="saving" aria-label="关闭" @click="onClose">
                <svg class="w-4 h-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                  <path d="M18 6L6 18M6 6l12 12" />
                </svg>
              </UiButton>
            </header>

            <div class="modal-body custom-scrollbar">
              <div v-if="loading" class="loading-state">
                <div class="loading-state__spinner" />
                <p class="loading-state__text">正在加载设置...</p>
              </div>

              <div v-else-if="loadError" class="error-state">
                <div class="error-state__icon">
                  <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                    <circle cx="12" cy="12" r="10" />
                    <line x1="12" y1="8" x2="12" y2="12" />
                    <line x1="12" y1="16" x2="12.01" y2="16" />
                  </svg>
                </div>
                <p class="error-state__text">设置加载失败</p>
                <UiButton variant="secondary" size="sm" @click="loadSettings">重试</UiButton>
              </div>

              <div v-else class="settings-form">
                <section class="form-section">
                  <div class="section-header">
                    <div class="section-header__icon">
                      <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5">
                        <path d="M3 7v10a2 2 0 002 2h14a2 2 0 002-2V9a2 2 0 00-2-2h-6l-2-2H5a2 2 0 00-2 2z" />
                      </svg>
                    </div>
                    <div class="section-header__text">
                      <h3 class="section-header__title">数据保存设置</h3>
                      <p class="section-header__subtitle">设置默认保存目录、文件前缀和录制策略</p>
                    </div>
                  </div>

                  <div class="section-content">
                    <div class="field-group">
                      <label class="field-label">保存目录</label>
                      <div class="directory-field">
                        <input v-model="baseDirectory" class="directory-field__input" placeholder="data/recordings" />
                        <UiButton variant="secondary" size="sm" @click="handlePickDirectory">选择目录</UiButton>
                      </div>
                      <p class="field-hint">点击选择目录按钮或直接输入路径。</p>
                    </div>

                    <div class="field-group">
                      <label class="field-label">文件前缀</label>
                      <input v-model="filePrefix" class="text-field" placeholder="run" />
                    </div>

                    <label class="toggle-row">
                      <UiToggle v-model="autoStart" />
                      <span>开始采集时自动开始记录</span>
                    </label>

                    <div class="stop-conditions">
                      <div class="stop-conditions__header">
                        <span class="stop-conditions__label">自动停止条件</span>
                        <span v-if="enabledConditionsCount > 0" class="stop-conditions__badge">
                          {{ enabledConditionsCount }} 项已启用
                        </span>
                      </div>

                      <div class="stop-conditions__items">
                        <div class="condition-item" :class="{ 'condition-item--enabled': durationEnabled }">
                          <div class="condition-item__header" @click="durationEnabled = !durationEnabled">
                            <div class="condition-item__toggle" @click.stop><UiToggle v-model="durationEnabled" /></div>
                            <div class="condition-item__label">定时停止</div>
                            <div v-if="durationEnabled" class="condition-item__value">{{ durationMinutes }} 分钟</div>
                            <svg class="condition-item__chevron" :class="{ 'condition-item__chevron--open': durationEnabled }" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                              <path d="M6 9l6 6 6-6" />
                            </svg>
                          </div>
                          <div v-if="durationEnabled" class="condition-item__content">
                            <div class="condition-item__input-row">
                              <input v-model.number="durationMinutes" class="number-field" type="number" min="1" max="1440" />
                              <span class="condition-item__unit">分钟</span>
                            </div>
                            <p class="field-hint">达到指定时长后自动停止录制。</p>
                          </div>
                        </div>

                        <div class="condition-item" :class="{ 'condition-item--enabled': sizeEnabled }">
                          <div class="condition-item__header" @click="sizeEnabled = !sizeEnabled">
                            <div class="condition-item__toggle" @click.stop><UiToggle v-model="sizeEnabled" /></div>
                            <div class="condition-item__label">按文件大小停止</div>
                            <div v-if="sizeEnabled" class="condition-item__value">{{ sizeMb }} MB</div>
                            <svg class="condition-item__chevron" :class="{ 'condition-item__chevron--open': sizeEnabled }" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                              <path d="M6 9l6 6 6-6" />
                            </svg>
                          </div>
                          <div v-if="sizeEnabled" class="condition-item__content">
                            <div class="condition-item__input-row">
                              <input v-model.number="sizeMb" class="number-field" type="number" min="1" max="10000" />
                              <span class="condition-item__unit">MB</span>
                            </div>
                            <p class="field-hint">当前文件达到大小上限后停止录制。</p>
                          </div>
                        </div>

                        <div class="condition-item" :class="{ 'condition-item--enabled': countEnabled }">
                          <div class="condition-item__header" @click="countEnabled = !countEnabled">
                            <div class="condition-item__toggle" @click.stop><UiToggle v-model="countEnabled" /></div>
                            <div class="condition-item__label">按记录数停止</div>
                            <div v-if="countEnabled" class="condition-item__value">{{ formatNumber(recordCount) }} 条</div>
                            <svg class="condition-item__chevron" :class="{ 'condition-item__chevron--open': countEnabled }" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                              <path d="M6 9l6 6 6-6" />
                            </svg>
                          </div>
                          <div v-if="countEnabled" class="condition-item__content">
                            <div class="condition-item__input-row">
                              <input v-model.number="recordCount" class="number-field" type="number" min="1" max="100000000" />
                              <span class="condition-item__unit">条</span>
                            </div>
                            <p class="field-hint">达到指定记录数后自动停止录制。</p>
                          </div>
                        </div>
                      </div>
                      <p class="field-hint">启用后，任一条件满足时录制将自动停止。</p>
                    </div>
                  </div>
                </section>

                <div class="section-divider" />

                <section class="form-section form-section--compact">
                  <div class="section-header">
                    <div class="section-header__icon">
                      <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5">
                        <path d="M13 10V3L4 14h7v7l9-11h-7z" />
                      </svg>
                    </div>
                    <div class="section-header__text">
                      <h3 class="section-header__title">刷新率设置</h3>
                      <p class="section-header__subtitle">控制前端实时数据发布频率</p>
                    </div>
                  </div>

                  <div class="section-content">
                    <div class="refresh-rate-control">
                      <div class="refresh-rate-control__slider-wrap">
                        <input v-model.number="refreshRate" type="range" min="1" max="20" step="1" class="refresh-rate-control__slider" />
                        <div class="refresh-rate-control__ticks">
                          <span>1Hz</span>
                          <span class="refresh-rate-control__tick--recommended">10Hz 推荐</span>
                          <span>20Hz</span>
                        </div>
                      </div>
                      <div class="refresh-rate-control__value">
                        <input v-model.number="refreshRate" type="number" min="1" max="20" class="refresh-rate-control__input" />
                        <span class="refresh-rate-control__unit">Hz</span>
                      </div>
                    </div>
                    <p class="field-hint">较高刷新率会增加界面绘制和数据传输压力。</p>
                  </div>
                </section>

                <p v-if="validationError" class="validation-error">{{ validationError }}</p>
              </div>
            </div>

            <footer class="modal-footer">
              <p class="modal-footer__hint">保存后对当前桌面会话生效。</p>
              <div class="modal-footer__actions">
                <UiButton variant="secondary" size="sm" :disabled="saving" @click="onClose">取消</UiButton>
                <UiButton variant="primary" size="sm" :disabled="saving || loading" @click="onSave">
                  {{ saving ? '保存中...' : '保存设置' }}
                </UiButton>
              </div>
            </footer>
          </div>
        </Transition>
      </div>
    </Transition>
  </Teleport>
</template>

<style scoped>
.modal-overlay {
  position: fixed;
  inset: 0;
  z-index: 200;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: var(--space-4);
  background: color-mix(in srgb, var(--bg-app) 70%, transparent);
  backdrop-filter: blur(12px);
}

.modal-container {
  width: 100%;
  max-width: 40rem;
  max-height: calc(100vh - var(--space-8));
  display: flex;
  flex-direction: column;
  border-radius: var(--radius-xl);
  background: var(--bg-panel);
  border: 1px solid var(--border-default);
  box-shadow: 0 0 0 1px rgba(255, 255, 255, 0.05) inset, 0 25px 50px -12px rgba(0, 0, 0, 0.5);
  outline: none;
}

.modal-header,
.modal-footer {
  display: flex;
  justify-content: space-between;
  gap: var(--space-4);
  padding: var(--space-5) var(--space-6);
  background: var(--bg-panel-strong);
}

.modal-header {
  align-items: flex-start;
  border-bottom: 1px solid var(--border-default);
  border-radius: var(--radius-xl) var(--radius-xl) 0 0;
}

.modal-header__title {
  font-size: var(--font-size-lg);
  font-weight: var(--font-weight-semibold);
  color: var(--text-primary);
  line-height: 1.2;
}

.modal-header__subtitle {
  margin-top: var(--space-1);
  font-size: var(--font-size-sm);
  color: var(--text-muted);
}

.modal-body {
  flex: 1;
  overflow-y: auto;
  padding: var(--space-2) var(--space-6) var(--space-6);
}

.loading-state,
.error-state {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: var(--space-4);
  padding: var(--space-16) var(--space-6);
  text-align: center;
}

.loading-state__spinner {
  width: 40px;
  height: 40px;
  border: 3px solid var(--border-default);
  border-top-color: var(--accent-primary);
  border-radius: 50%;
  animation: spin 0.8s linear infinite;
}

@keyframes spin {
  to { transform: rotate(360deg); }
}

.error-state__icon {
  width: 48px;
  height: 48px;
  color: var(--accent-danger);
}

.settings-form {
  display: flex;
  flex-direction: column;
}

.form-section {
  padding: var(--space-5) 0;
}

.form-section--compact {
  padding: var(--space-4) 0;
}

.section-header {
  display: flex;
  align-items: flex-start;
  gap: var(--space-3);
  margin-bottom: var(--space-5);
}

.section-header__icon {
  flex-shrink: 0;
  width: 36px;
  height: 36px;
  display: flex;
  align-items: center;
  justify-content: center;
  border-radius: var(--radius-md);
  background: color-mix(in srgb, var(--accent-primary) 15%, transparent);
  color: var(--accent-primary);
}

.section-header__icon svg {
  width: 20px;
  height: 20px;
}

.section-header__text {
  flex: 1;
  min-width: 0;
  padding-top: var(--space-1);
}

.section-header__title {
  font-size: var(--font-size-base);
  font-weight: var(--font-weight-semibold);
  color: var(--text-primary);
}

.section-header__subtitle,
.field-hint,
.modal-footer__hint,
.loading-state__text,
.error-state__text {
  font-size: var(--font-size-xs);
  color: var(--text-muted);
  line-height: 1.4;
}

.section-divider {
  height: 1px;
  background: var(--border-default);
  margin: 0 calc(-1 * var(--space-6));
}

.section-content {
  display: flex;
  flex-direction: column;
  gap: var(--space-5);
  padding-left: calc(36px + var(--space-3));
}

.field-group {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
}

.field-label {
  font-size: var(--font-size-sm);
  font-weight: var(--font-weight-medium);
  color: var(--text-secondary);
}

.directory-field__input,
.text-field,
.number-field,
.refresh-rate-control__input {
  border: 1px solid var(--border-default);
  border-radius: var(--radius-md);
  background: var(--bg-app);
  color: var(--text-secondary);
  outline: none;
}

.directory-field__input,
.text-field {
  width: 100%;
  padding: var(--space-2-5) var(--space-3);
  font-size: var(--font-size-sm);
  font-family: var(--font-mono, ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace);
}

.toggle-row {
  display: flex;
  align-items: center;
  gap: var(--space-3);
  font-size: var(--font-size-sm);
  color: var(--text-secondary);
}

.stop-conditions__header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: var(--space-3);
}

.stop-conditions__label {
  font-size: var(--font-size-xs);
  font-weight: var(--font-weight-semibold);
  color: var(--text-secondary);
  text-transform: uppercase;
  letter-spacing: 0.06em;
}

.stop-conditions__badge {
  font-size: var(--font-size-xs);
  font-weight: var(--font-weight-medium);
  color: var(--accent-success);
  background: color-mix(in srgb, var(--accent-success) 12%, transparent);
  padding: var(--space-0-5) var(--space-2);
  border-radius: var(--radius-pill);
}

.stop-conditions__items {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
}

.condition-item {
  background: var(--bg-app);
  border: 1px solid var(--border-default);
  border-radius: var(--radius-md);
  overflow: hidden;
  transition: all var(--motion-fast) var(--easing-standard);
}

.condition-item--enabled {
  background: color-mix(in srgb, var(--accent-success) 5%, var(--bg-app));
  border-color: color-mix(in srgb, var(--accent-success) 25%, var(--border-default));
}

.condition-item__header {
  display: flex;
  align-items: center;
  gap: var(--space-3);
  padding: var(--space-3);
  cursor: pointer;
}

.condition-item__label {
  flex: 1;
  font-size: var(--font-size-sm);
  font-weight: var(--font-weight-medium);
  color: var(--text-secondary);
}

.condition-item__value {
  font-size: var(--font-size-sm);
  font-weight: var(--font-weight-semibold);
  color: var(--accent-success);
}

.condition-item__chevron {
  width: 16px;
  height: 16px;
  color: var(--text-muted);
  transition: transform var(--motion-fast) var(--easing-standard);
}

.condition-item__chevron--open {
  transform: rotate(180deg);
}

.condition-item__content {
  padding: 0 var(--space-3) var(--space-3) calc(38px + var(--space-3));
  border-top: 1px solid var(--border-default);
  background: var(--bg-panel);
}

.condition-item__input-row {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  padding-top: var(--space-3);
}

.number-field {
  width: 7rem;
  padding: var(--space-2) var(--space-3);
}

.condition-item__unit {
  font-size: var(--font-size-sm);
  color: var(--text-muted);
}

.refresh-rate-control {
  display: flex;
  align-items: center;
  gap: var(--space-6);
  padding: var(--space-4);
  background: var(--bg-app);
  border: 1px solid var(--border-default);
  border-radius: var(--radius-lg);
}

.refresh-rate-control__slider-wrap {
  flex: 1;
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
}

.refresh-rate-control__slider {
  width: 100%;
  height: 6px;
  appearance: none;
  background: var(--border-strong);
  border-radius: var(--radius-pill);
  cursor: pointer;
}

.refresh-rate-control__slider::-webkit-slider-thumb {
  appearance: none;
  width: 20px;
  height: 20px;
  background: var(--accent-primary);
  border: 3px solid var(--bg-panel);
  border-radius: 50%;
  box-shadow: 0 2px 8px rgba(0, 0, 0, 0.3);
}

.refresh-rate-control__ticks {
  display: flex;
  justify-content: space-between;
  font-size: 10px;
  color: var(--text-muted);
  text-transform: uppercase;
  letter-spacing: 0.05em;
}

.refresh-rate-control__tick--recommended {
  color: var(--accent-success);
  font-weight: var(--font-weight-medium);
}

.refresh-rate-control__value {
  display: flex;
  align-items: baseline;
  gap: var(--space-1);
  min-width: 5rem;
  justify-content: flex-end;
  padding-left: var(--space-4);
  border-left: 1px solid var(--border-default);
}

.refresh-rate-control__input {
  width: 4.5rem;
  padding: 0 var(--space-2);
  font-size: 1.5rem;
  font-weight: var(--font-weight-bold);
  color: var(--accent-primary);
  text-align: right;
}

.refresh-rate-control__unit {
  font-size: var(--font-size-sm);
  color: var(--text-muted);
}

.validation-error {
  margin: var(--space-2) 0 0 calc(36px + var(--space-3));
  color: var(--accent-danger);
  font-size: var(--font-size-sm);
}

.modal-footer {
  align-items: center;
  border-top: 1px solid var(--border-default);
  border-radius: 0 0 var(--radius-xl) var(--radius-xl);
}

.modal-footer__actions {
  display: flex;
  align-items: center;
  gap: var(--space-3);
  flex-shrink: 0;
}

.custom-scrollbar::-webkit-scrollbar {
  width: 6px;
}

.custom-scrollbar::-webkit-scrollbar-thumb {
  background: var(--border-strong);
  border-radius: var(--radius-pill);
}

.custom-scrollbar::-webkit-scrollbar-thumb:hover {
  background: var(--accent-primary);
}
</style>
