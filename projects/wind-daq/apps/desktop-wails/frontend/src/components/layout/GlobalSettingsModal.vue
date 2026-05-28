<script setup lang="ts">
import { computed, nextTick, ref, watch } from 'vue'
import { storeToRefs } from 'pinia'
import { useFeedbackStore } from '@stores/feedbackStore'
import { useI18nStore } from '@stores/i18nStore'
import { useStorageStore, type StorageSettings } from '@stores/storageStore'
import { deviceApi, storageApi } from '@api/deviceApi'
import { wailsApi } from '@api/wails-adapter'
import UiButton from '@components/ui/UiButton.vue'
import UiToggle from '@components/ui/UiToggle.vue'

const props = defineProps<{ open: boolean }>()
const emit = defineEmits<{
  (e: 'close'): void
  (e: 'update:open', value: boolean): void
}>()

const i18n = useI18nStore()
const feedback = useFeedbackStore()
const storageStore = useStorageStore()
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
const rotationEnabled = ref(false)
const rotationDurationMinutes = ref(30)
const rotationSizeMb = ref(100)
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
    await storageStore.loadSettings()
    applySettings(storageStore.settings)

    try {
      const status = await storageApi.status()
      if (status.outputDir) baseDirectory.value = status.outputDir
    } catch {
      // 录制状态对打开设置是可选的
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

function applySettings(s: StorageSettings): void {
  baseDirectory.value = s.baseDirectory || 'data/recordings'
  filePrefix.value = s.filePrefix || 'run'
  autoStart.value = s.autoStartOnAcquisition
  durationEnabled.value = !!s.stopConditions.maxDurationMs
  durationMinutes.value = s.stopConditions.maxDurationMs
    ? Math.max(1, Math.round(s.stopConditions.maxDurationMs / 60000))
    : 10
  sizeEnabled.value = !!s.stopConditions.maxFileSizeBytes
  sizeMb.value = s.stopConditions.maxFileSizeBytes
    ? Math.max(1, Math.round(s.stopConditions.maxFileSizeBytes / 1048576))
    : 100
  countEnabled.value = !!s.stopConditions.maxRecordCount
  recordCount.value = s.stopConditions.maxRecordCount || 1000000
  const fr = s.fileRotation ?? { enabled: false, maxFileSizeBytes: 104857600, maxDurationMs: 1800000 }
  rotationEnabled.value = fr.enabled
  rotationDurationMinutes.value = Math.max(1, Math.round(fr.maxDurationMs / 1000 / 60))
  rotationSizeMb.value = Math.max(1, Math.round(fr.maxFileSizeBytes / (1024 * 1024)))
}

function currentSettings(): StorageSettings {
  const stopConditions: StorageSettings['stopConditions'] = {}
  if (durationEnabled.value) stopConditions.maxDurationMs = durationMinutes.value * 60000
  if (sizeEnabled.value) stopConditions.maxFileSizeBytes = sizeMb.value * 1048576
  if (countEnabled.value) stopConditions.maxRecordCount = recordCount.value

  const fileRotation = {
    enabled: rotationEnabled.value,
    maxFileSizeBytes: rotationSizeMb.value * 1024 * 1024,
    maxDurationMs: rotationDurationMinutes.value * 60 * 1000,
  }

  return {
    baseDirectory: baseDirectory.value.trim(),
    filePrefix: filePrefix.value.trim(),
    autoStartOnAcquisition: autoStart.value,
    stopConditions,
    fileRotation,
  }
}

function validate(): boolean {
  if (!baseDirectory.value.trim()) validationError.value = '保存目录不能为空'
  else if (!filePrefix.value.trim()) validationError.value = '文件前缀不能为空'
  else if (durationEnabled.value && (durationMinutes.value < 1 || durationMinutes.value > 1440)) validationError.value = '定时停止范围为 1 到 1440 分钟'
  else if (sizeEnabled.value && (sizeMb.value < 1 || sizeMb.value > 10000)) validationError.value = '文件大小范围为 1 到 10000 MB'
  else if (countEnabled.value && (recordCount.value < 1 || recordCount.value > 100000000)) validationError.value = '记录数范围为 1 到 100000000'
  else if (rotationEnabled.value && (rotationDurationMinutes.value < 1 || rotationDurationMinutes.value > 1440)) validationError.value = '滚动时长范围为 1 到 1440 分钟'
  else if (rotationEnabled.value && (rotationSizeMb.value < 1 || rotationSizeMb.value > 10000)) validationError.value = '滚动大小范围为 1 到 10000 MB'
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
  saving.value = true
  let saved = false
  try {
    if (!validate()) {
      feedback.pushToast(validationError.value || '设置无效，请检查输入', 'warning')
      return
    }

    await storageStore.saveSettings(currentSettings())
    if (refreshRate.value !== originalRefreshRate.value) {
      await deviceApi.setPublishRate(refreshRate.value)
      originalRefreshRate.value = refreshRate.value
    }
    feedback.pushToast('设置已保存', 'success')
    saved = true
  } catch {
    feedback.pushToast('保存失败，请重试', 'error')
  } finally {
    saving.value = false
  }
  if (saved) onClose()
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
            <!-- 头部 -->
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

            <!-- 内容区 -->
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
                <!-- 数据保存 -->
                <section class="form-section">
                  <h3 class="section-title">
                    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5">
                      <path d="M3 7v10a2 2 0 002 2h14a2 2 0 002-2V9a2 2 0 00-2-2h-6l-2-2H5a2 2 0 00-2 2z" />
                    </svg>
                    数据保存
                  </h3>

                  <div class="field-row">
                    <label class="field-label">保存目录</label>
                    <div class="directory-field">
                      <input v-model="baseDirectory" class="directory-field__input" placeholder="data/recordings" />
                      <UiButton variant="secondary" size="sm" @click="handlePickDirectory">选择</UiButton>
                    </div>
                  </div>

                  <div class="field-row">
                    <label class="field-label">文件前缀</label>
                    <input v-model="filePrefix" class="text-field" placeholder="run" />
                  </div>

                  <label class="toggle-row">
                    <UiToggle v-model="autoStart" />
                    <span>开始采集时自动开始记录</span>
                  </label>
                </section>

                <div class="section-divider" />

                <!-- 自动停止条件 -->
                <section class="form-section">
                  <h3 class="section-title">
                    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5">
                      <path d="M12 8v4l3 3m6-3a9 9 0 11-18 0 9 9 0 0118 0z" />
                    </svg>
                    自动停止条件
                    <span v-if="enabledConditionsCount > 0" class="section-badge">{{ enabledConditionsCount }} 项</span>
                  </h3>

                  <div class="conditions-list">
                    <!-- 定时停止 -->
                    <div class="condition-row" :class="{ 'condition-row--active': durationEnabled }">
                      <div class="condition-row__main">
                        <UiToggle v-model="durationEnabled" />
                        <span class="condition-row__label">定时停止</span>
                      </div>
                      <div v-if="durationEnabled" class="condition-row__input">
                        <input v-model.number="durationMinutes" class="number-field" type="number" min="1" max="1440" />
                        <span class="unit">分钟</span>
                      </div>
                    </div>

                    <!-- 按文件大小停止 -->
                    <div class="condition-row" :class="{ 'condition-row--active': sizeEnabled }">
                      <div class="condition-row__main">
                        <UiToggle v-model="sizeEnabled" />
                        <span class="condition-row__label">按文件大小停止</span>
                      </div>
                      <div v-if="sizeEnabled" class="condition-row__input">
                        <input v-model.number="sizeMb" class="number-field" type="number" min="1" max="10000" />
                        <span class="unit">MB</span>
                      </div>
                    </div>

                    <!-- 按记录数停止 -->
                    <div class="condition-row" :class="{ 'condition-row--active': countEnabled }">
                      <div class="condition-row__main">
                        <UiToggle v-model="countEnabled" />
                        <span class="condition-row__label">按记录数停止</span>
                      </div>
                      <div v-if="countEnabled" class="condition-row__input">
                        <input v-model.number="recordCount" class="number-field" type="number" min="1" max="100000000" />
                        <span class="unit">条</span>
                      </div>
                    </div>
                  </div>
                </section>

                <div class="section-divider" />

                <!-- 文件滚动保存 -->
                <section class="form-section">
                  <h3 class="section-title">
                    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5">
                      <path d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15" />
                    </svg>
                    文件滚动保存
                    <UiToggle v-model="rotationEnabled" />
                  </h3>

                  <div v-if="rotationEnabled" class="conditions-list">
                    <div class="condition-row condition-row--active">
                      <span class="condition-row__label">滚动时长</span>
                      <div class="condition-row__input">
                        <input v-model.number="rotationDurationMinutes" class="number-field" type="number" min="1" max="1440" />
                        <span class="unit">分钟</span>
                      </div>
                    </div>
                    <div class="condition-row condition-row--active">
                      <span class="condition-row__label">滚动大小</span>
                      <div class="condition-row__input">
                        <input v-model.number="rotationSizeMb" class="number-field" type="number" min="1" max="10000" />
                        <span class="unit">MB</span>
                      </div>
                    </div>
                  </div>
                </section>

                <div class="section-divider" />

                <!-- 刷新率 -->
                <section class="form-section">
                  <h3 class="section-title">
                    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5">
                      <path d="M13 10V3L4 14h7v7l9-11h-7z" />
                    </svg>
                    刷新率
                  </h3>
                  <div class="refresh-rate-row">
                    <div class="refresh-rate__slider-wrap">
                      <input v-model.number="refreshRate" type="range" min="1" max="20" step="1" class="refresh-rate__slider" />
                      <div class="refresh-rate__ticks">
                        <span>1Hz</span>
                        <span class="refresh-rate__tick--recommended">10Hz 推荐</span>
                        <span>20Hz</span>
                      </div>
                    </div>
                    <div class="refresh-rate__value">
                      <input v-model.number="refreshRate" type="number" min="1" max="20" class="refresh-rate__input" />
                      <span class="unit">Hz</span>
                    </div>
                  </div>
                </section>

                <p v-if="validationError" class="validation-error">{{ validationError }}</p>
              </div>
            </div>

            <!-- 底部 -->
            <footer class="modal-footer">
              <p class="modal-footer__hint">保存后对当前桌面会话生效</p>
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
/* ========== 模态框容器 ========== */
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
  max-width: 28rem;
  max-height: calc(100vh - var(--space-8));
  display: flex;
  flex-direction: column;
  border-radius: var(--radius-xl);
  background: var(--bg-panel);
  border: 1px solid var(--border-default);
  box-shadow: 0 0 0 1px rgba(255, 255, 255, 0.05) inset, 0 25px 50px -12px rgba(0, 0, 0, 0.5);
  outline: none;
}

/* ========== 头部 ========== */
.modal-header {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  gap: var(--space-4);
  padding: var(--space-4) var(--space-5);
  background: var(--bg-panel-strong);
  border-bottom: 1px solid var(--border-default);
  border-radius: var(--radius-xl) var(--radius-xl) 0 0;
}

.modal-header__title {
  font-size: var(--font-size-base);
  font-weight: var(--font-weight-semibold);
  color: var(--text-primary);
  line-height: 1.3;
}

.modal-header__subtitle {
  margin-top: 2px;
  font-size: var(--font-size-xs);
  color: var(--text-muted);
}

/* ========== 内容区 ========== */
.modal-body {
  flex: 1;
  overflow-y: auto;
  padding: var(--space-3) var(--space-5) var(--space-4);
}

.loading-state,
.error-state {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: var(--space-3);
  padding: var(--space-12) var(--space-4);
  text-align: center;
}

.loading-state__spinner {
  width: 32px;
  height: 32px;
  border: 2px solid var(--border-default);
  border-top-color: var(--accent-primary);
  border-radius: 50%;
  animation: spin 0.8s linear infinite;
}

@keyframes spin {
  to { transform: rotate(360deg); }
}

.error-state__icon {
  width: 40px;
  height: 40px;
  color: var(--accent-danger);
}

.loading-state__text,
.error-state__text {
  font-size: var(--font-size-sm);
  color: var(--text-muted);
}

/* ========== 表单区域 ========== */
.settings-form {
  display: flex;
  flex-direction: column;
}

.form-section {
  padding: var(--space-3) 0;
}

.section-title {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  margin-bottom: var(--space-3);
  font-size: var(--font-size-sm);
  font-weight: var(--font-weight-semibold);
  color: var(--text-primary);
}

.section-title svg {
  width: 16px;
  height: 16px;
  color: var(--accent-primary);
  flex-shrink: 0;
}

.section-badge {
  margin-left: auto;
  font-size: 10px;
  font-weight: var(--font-weight-medium);
  color: var(--accent-success);
  background: color-mix(in srgb, var(--accent-success) 12%, transparent);
  padding: 1px var(--space-2);
  border-radius: var(--radius-pill);
}

.section-divider {
  height: 1px;
  background: var(--border-default);
  margin: 0 calc(-1 * var(--space-5));
}

/* ========== 字段行 ========== */
.field-row {
  display: flex;
  flex-direction: column;
  gap: var(--space-1-5);
  margin-bottom: var(--space-3);
}

.field-row:last-child {
  margin-bottom: 0;
}

.field-label {
  font-size: var(--font-size-xs);
  font-weight: var(--font-weight-medium);
  color: var(--text-secondary);
}

.directory-field {
  display: flex;
  gap: var(--space-2);
}

.directory-field__input,
.text-field,
.number-field {
  border: 1px solid var(--border-default);
  border-radius: var(--radius-md);
  background: var(--bg-app);
  color: var(--text-secondary);
  outline: none;
  font-size: var(--font-size-sm);
}

.directory-field__input,
.text-field {
  flex: 1;
  padding: var(--space-2) var(--space-3);
  font-family: var(--font-mono, ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace);
}

.number-field {
  width: 5rem;
  padding: var(--space-1-5) var(--space-2);
  text-align: right;
}

.toggle-row {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  font-size: var(--font-size-sm);
  color: var(--text-secondary);
  cursor: pointer;
}

.unit {
  font-size: var(--font-size-xs);
  color: var(--text-muted);
  white-space: nowrap;
}

/* ========== 条件列表（紧凑行式布局） ========== */
.conditions-list {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
}

.condition-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--space-3);
  padding: var(--space-2) var(--space-3);
  border-radius: var(--radius-md);
  border: 1px solid var(--border-default);
  background: var(--bg-app);
  transition: all var(--motion-fast) var(--easing-standard);
}

.condition-row--active {
  background: color-mix(in srgb, var(--accent-success) 4%, var(--bg-app));
  border-color: color-mix(in srgb, var(--accent-success) 20%, var(--border-default));
}

.condition-row__main {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  flex: 1;
  min-width: 0;
}

.condition-row__label {
  font-size: var(--font-size-sm);
  color: var(--text-secondary);
}

.condition-row__input {
  display: flex;
  align-items: center;
  gap: var(--space-1-5);
  flex-shrink: 0;
}

/* ========== 刷新率（紧凑行式布局） ========== */
.refresh-rate-row {
  display: flex;
  align-items: center;
  gap: var(--space-4);
}

.refresh-rate__slider-wrap {
  flex: 1;
  display: flex;
  flex-direction: column;
  gap: var(--space-1);
}

.refresh-rate__slider {
  width: 100%;
  height: 4px;
  appearance: none;
  background: var(--border-strong);
  border-radius: var(--radius-pill);
  cursor: pointer;
}

.refresh-rate__slider::-webkit-slider-thumb {
  appearance: none;
  width: 16px;
  height: 16px;
  background: var(--accent-primary);
  border: 2px solid var(--bg-panel);
  border-radius: 50%;
  box-shadow: 0 1px 4px rgba(0, 0, 0, 0.3);
}

.refresh-rate__ticks {
  display: flex;
  justify-content: space-between;
  font-size: 10px;
  color: var(--text-muted);
}

.refresh-rate__tick--recommended {
  color: var(--accent-success);
  font-weight: var(--font-weight-medium);
}

.refresh-rate__value {
  display: flex;
  align-items: baseline;
  gap: var(--space-1);
  min-width: 4rem;
  justify-content: flex-end;
  padding-left: var(--space-3);
  border-left: 1px solid var(--border-default);
}

.refresh-rate__input {
  width: 3.5rem;
  padding: 0 var(--space-1-5);
  font-size: var(--font-size-lg);
  font-weight: var(--font-weight-bold);
  color: var(--accent-primary);
  text-align: right;
  border: 1px solid var(--border-default);
  border-radius: var(--radius-md);
  background: var(--bg-app);
  outline: none;
}

/* ========== 验证错误 ========== */
.validation-error {
  margin-top: var(--space-3);
  padding: var(--space-2) var(--space-3);
  border-radius: var(--radius-md);
  background: color-mix(in srgb, var(--accent-danger) 8%, transparent);
  color: var(--accent-danger);
  font-size: var(--font-size-sm);
  border: 1px solid color-mix(in srgb, var(--accent-danger) 15%, transparent);
}

/* ========== 底部 ========== */
.modal-footer {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--space-4);
  padding: var(--space-3) var(--space-5);
  background: var(--bg-panel-strong);
  border-top: 1px solid var(--border-default);
  border-radius: 0 0 var(--radius-xl) var(--radius-xl);
}

.modal-footer__hint {
  font-size: var(--font-size-xs);
  color: var(--text-muted);
}

.modal-footer__actions {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  flex-shrink: 0;
}

/* ========== 滚动条 ========== */
.custom-scrollbar::-webkit-scrollbar {
  width: 5px;
}

.custom-scrollbar::-webkit-scrollbar-thumb {
  background: var(--border-strong);
  border-radius: var(--radius-pill);
}

.custom-scrollbar::-webkit-scrollbar-thumb:hover {
  background: var(--accent-primary);
}
</style>
