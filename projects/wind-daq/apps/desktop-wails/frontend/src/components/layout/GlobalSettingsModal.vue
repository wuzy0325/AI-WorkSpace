<script setup lang="ts">
import { computed, type Component, ref, watch } from 'vue'
import { storeToRefs } from 'pinia'
import { useFeedbackStore } from '@stores/feedbackStore'
import { useI18nStore } from '@stores/i18nStore'
import { useStorageStore, type StorageSettings, DEFAULT_SETTINGS } from '@stores/storageStore'
import { useThemeStore } from '@stores/themeStore'
import { deviceApi, storageApi } from '@api/deviceApi'
import { wailsApi } from '@api/wails-adapter'
import UiButton from '@components/ui/UiButton.vue'
import UiSpin from '@components/ui/UiSpin.vue'
import UiStatusBadge from '@components/ui/UiStatusBadge.vue'
import UiPanel from '@components/ui/UiPanel.vue'
import UiInput from '@components/ui/UiInput.vue'
import UiInputNumber from '@components/ui/UiInputNumber.vue'
import UiSlider from '@components/ui/UiSlider.vue'
import UiToggle from '@components/ui/UiToggle.vue'
import UiErrorState from '@components/ui/UiErrorState.vue'
import UiDialog from '@components/ui/UiDialog.vue'
import UiFormField from '@components/ui/UiFormField.vue'
import {
  CheckCircle,
  Clock,
  FileText,
  Folder,
  HardDrive,
  RefreshCw,
  RotateCcw,
  Save,
  Sun,
  Moon,
  X,
  Monitor,
  Globe,
  Zap,
} from '@lucide/vue'

/** 设置分组标签页类型 */
type SettingsTab = 'appearance' | 'storage' | 'acquisition' | 'advanced'

const props = defineProps<{ open: boolean }>()
const emit = defineEmits<{
  (e: 'close'): void
  (e: 'update:open', value: boolean): void
}>()

const feedback = useFeedbackStore()
const storageStore = useStorageStore()
const themeStore = useThemeStore()
const i18nStore = useI18nStore()
const { theme } = storeToRefs(themeStore)
const { locale } = storeToRefs(i18nStore)

const loading = ref(false)
const saving = ref(false)
const loadError = ref(false)
const activeTab = ref<SettingsTab>('appearance')

// 表单数据状态
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

/** 字段级校验错误记录 */
const validationErrors = ref<Record<string, string>>({})

const isVisible = computed({
  get: () => props.open,
  set: (value) => emit('update:open', value),
})

const enabledConditionsCount = computed(() =>
  [durationEnabled.value, sizeEnabled.value, countEnabled.value].filter(Boolean).length,
)

/** Tab 配置列表（静态常量，无需响应式重新创建） */
const TABS: { key: SettingsTab; label: string; icon: Component }[] = [
  { key: 'appearance', label: '外观', icon: Monitor },
  { key: 'storage', label: '存储', icon: HardDrive },
  { key: 'acquisition', label: '采集', icon: Zap },
  { key: 'advanced', label: '高级', icon: RefreshCw },
]

watch(() => props.open, (open) => { if (open) void loadSettings() })

async function loadSettings(): Promise<void> {
  loading.value = true
  loadError.value = false
  validationErrors.value = {}
  try {
    await storageStore.loadSettings()
    applySettings(storageStore.settings)
    try {
      const status = await storageApi.status()
      if (status.outputDir) baseDirectory.value = status.outputDir
    } catch { /* ok */ }
    try {
      const hz = await deviceApi.getPublishRate()
      refreshRate.value = Math.round(hz)
      originalRefreshRate.value = Math.round(hz)
    } catch { refreshRate.value = originalRefreshRate.value }
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
    ? Math.max(1, Math.round(s.stopConditions.maxDurationMs / 60000)) : 10
  sizeEnabled.value = !!s.stopConditions.maxFileSizeBytes
  sizeMb.value = s.stopConditions.maxFileSizeBytes
    ? Math.max(1, Math.round(s.stopConditions.maxFileSizeBytes / 1048576)) : 100
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
  return {
    baseDirectory: baseDirectory.value.trim(),
    filePrefix: filePrefix.value.trim(),
    autoStartOnAcquisition: autoStart.value,
    stopConditions,
    fileRotation: {
      enabled: rotationEnabled.value,
      maxFileSizeBytes: rotationSizeMb.value * 1024 * 1024,
      maxDurationMs: rotationDurationMinutes.value * 60 * 1000,
    },
  }
}

/** 校验指定字段，返回错误信息或空字符串 */
function validateField(field: string): string {
  switch (field) {
    case 'baseDirectory':
      return baseDirectory.value.trim() ? '' : '保存目录不能为空'
    case 'filePrefix':
      return filePrefix.value.trim() ? '' : '文件前缀不能为空'
    case 'durationMinutes':
      return durationEnabled.value && (durationMinutes.value < 1 || durationMinutes.value > 1440)
        ? '定时停止范围为 1 到 1440 分钟' : ''
    case 'sizeMb':
      return sizeEnabled.value && (sizeMb.value < 1 || sizeMb.value > 10000)
        ? '文件大小范围为 1 到 10000 MB' : ''
    case 'recordCount':
      return countEnabled.value && (recordCount.value < 1 || recordCount.value > 100000000)
        ? '记录数范围为 1 到 100000000' : ''
    case 'rotationDurationMinutes':
      return rotationEnabled.value && (rotationDurationMinutes.value < 1 || rotationDurationMinutes.value > 1440)
        ? '滚动时长范围为 1 到 1440 分钟' : ''
    case 'rotationSizeMb':
      return rotationEnabled.value && (rotationSizeMb.value < 1 || rotationSizeMb.value > 10000)
        ? '滚动大小范围为 1 到 10000 MB' : ''
    case 'refreshRate':
      return refreshRate.value < 1 || refreshRate.value > 20
        ? '刷新率范围为 1 到 20 Hz' : ''
    default:
      return ''
  }
}

/** 更新单个字段的校验状态 */
function updateFieldError(field: string): void {
  const error = validateField(field)
  if (error) {
    validationErrors.value = { ...validationErrors.value, [field]: error }
  } else {
    const { [field]: _, ...rest } = validationErrors.value
    validationErrors.value = rest as Record<string, string>
  }
}

/** 全量校验 */
function validate(): boolean {
  const fields = [
    'baseDirectory', 'filePrefix', 'durationMinutes', 'sizeMb',
    'recordCount', 'rotationDurationMinutes', 'rotationSizeMb', 'refreshRate',
  ]
  const errs: Record<string, string> = {}
  for (const field of fields) {
    const error = validateField(field)
    if (error) errs[field] = error
  }
  validationErrors.value = errs
  return Object.keys(errs).length === 0
}

function onClose(): void {
  if (saving.value) return
  isVisible.value = false
  emit('close')
}

/** 恢复默认设置 */
function onReset(): void {
  applySettings(DEFAULT_SETTINGS)
  refreshRate.value = 20
  originalRefreshRate.value = 20
  validationErrors.value = {}
  feedback.pushToast('已恢复默认设置', 'info')
}

async function handlePickDirectory(): Promise<void> {
  try {
    const dir = await wailsApi.app.pickDirectory()
    if (dir) {
      baseDirectory.value = dir
      updateFieldError('baseDirectory')
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
      const firstError = Object.values(validationErrors.value).find(Boolean) || '设置无效'
      feedback.pushToast(firstError, 'warning')
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
  <UiDialog
    v-model:show="isVisible"
    preset="card"
    :style="{ maxWidth: '48rem', width: '92vw' }"
    title="全局设置"
    :bordered="false"
    :mask-closable="false"
    @close="onClose"
  >
    <template #header>
      <div class="modal-head">
        <div class="modal-head__info">
          <div class="modal-head__title">全局设置</div>
          <span class="modal-head__subtitle">数据保存、自动停止条件和实时刷新频率</span>
        </div>
        <UiButton quaternary circle size="md" @click="onClose">
          <template #icon><X :size="14" /></template>
        </UiButton>
      </div>
    </template>

    <UiSpin v-if="loading" class="loading-wrap" />
    <UiErrorState v-else-if="loadError" title="设置加载失败" message="请检查后端连接">
      <template #action><UiButton size="md" @click="loadSettings">重试</UiButton></template>
    </UiErrorState>

    <div v-else class="settings-layout">
      <!-- 左侧标签导航 -->
      <nav class="settings-tabs" role="tablist" aria-label="设置分组">
        <button
          v-for="tab in TABS"
          :id="`settings-tab-${tab.key}`"
          :key="tab.key"
          class="settings-tab"
          :class="{ 'settings-tab--active': activeTab === tab.key }"
          role="tab"
          :aria-selected="activeTab === tab.key"
          :aria-controls="`settings-panel-${tab.key}`"
          @click="activeTab = tab.key"
        >
          <component :is="tab.icon" :size="16" />
          <span>{{ tab.label }}</span>
        </button>
      </nav>

      <!-- 右侧内容区 -->
      <div class="settings-content">
        <!-- 外观与语言 -->
        <section
          id="settings-panel-appearance"
          role="tabpanel"
          aria-labelledby="settings-tab-appearance"
          v-show="activeTab === 'appearance'"
          :aria-hidden="activeTab !== 'appearance'"
          class="settings-section"
        >
          <UiPanel :segmented="false" class="form-card">
            <template #header>
              <div class="card-head">
                <Monitor :size="15" />
                <span class="card-head__title">外观与语言</span>
              </div>
            </template>
            <div class="form-fields">
              <!-- 主题切换 -->
              <UiFormField label="主题模式">
                <div class="theme-switch">
                  <UiButton
                    size="md"
                    :variant="theme === 'light' ? 'primary' : 'ghost'"
                    aria-label="切换为浅色主题"
                    data-test="settings-theme-light"
                    @click="themeStore.setTheme('light')"
                  >
                    <template #icon><Sun :size="14" /></template>浅色
                  </UiButton>
                  <UiButton
                    size="md"
                    :variant="theme === 'dark' ? 'primary' : 'ghost'"
                    aria-label="切换为深色主题"
                    data-test="settings-theme-dark"
                    @click="themeStore.setTheme('dark')"
                  >
                    <template #icon><Moon :size="14" /></template>深色
                  </UiButton>
                </div>
              </UiFormField>
              <!-- 语言切换 -->
              <UiFormField label="界面语言">
                <div class="locale-switch">
                  <button
                    class="locale-btn"
                    :class="{ 'locale-btn--active': locale === 'zh' }"
                    aria-label="切换为中文界面"
                    data-test="settings-locale-zh"
                    @click="i18nStore.setLocale('zh')"
                  >
                    <Globe :size="12" />中文
                  </button>
                  <button
                    class="locale-btn"
                    :class="{ 'locale-btn--active': locale === 'en' }"
                    aria-label="Switch interface language to English"
                    data-test="settings-locale-en"
                    @click="i18nStore.setLocale('en')"
                  >
                    <Globe :size="12" />English
                  </button>
                </div>
              </UiFormField>
            </div>
          </UiPanel>
        </section>

        <!-- 数据保存 -->
        <section
          id="settings-panel-storage"
          role="tabpanel"
          aria-labelledby="settings-tab-storage"
          v-show="activeTab === 'storage'"
          :aria-hidden="activeTab !== 'storage'"
          class="settings-section"
        >
          <UiPanel :segmented="false" class="form-card">
            <template #header>
              <div class="card-head">
                <FileText :size="15" />
                <span class="card-head__title">数据保存</span>
              </div>
            </template>
            <div class="form-fields">
              <UiFormField
                label="保存目录"
                :error="validationErrors.baseDirectory"
                hint="数据文件将保存到此目录"
              >
                <div class="input-with-action">
                  <UiInput
                    v-model="baseDirectory"
                    placeholder="data/recordings"
                    @blur="updateFieldError('baseDirectory')"
                  />
                  <UiButton size="md" aria-label="选择保存目录" data-test="settings-pick-directory" @click="handlePickDirectory">
                    <template #icon><Folder :size="14" /></template>选择
                  </UiButton>
                </div>
              </UiFormField>
              <UiFormField
                label="文件前缀"
                :error="validationErrors.filePrefix"
              >
                <UiInput
                  v-model="filePrefix"
                  placeholder="run"
                  @blur="updateFieldError('filePrefix')"
                />
              </UiFormField>
              <div class="toggle-row">
                <UiToggle v-model="autoStart" />
                <span class="toggle-row__label">开始采集时自动开始记录</span>
              </div>
            </div>
          </UiPanel>

          <UiPanel class="form-card">
            <template #header>
              <div class="card-head">
                <HardDrive :size="15" />
                <span class="card-head__title">文件滚动保存</span>
                <UiToggle v-model="rotationEnabled" />
              </div>
            </template>
            <div v-if="rotationEnabled" class="form-fields">
              <UiFormField
                label="滚动时长"
                :error="validationErrors.rotationDurationMinutes"
              >
                <div class="input-with-unit">
                  <UiInputNumber
                    v-model="rotationDurationMinutes"
                    :min="1"
                    :max="1440"
                    @blur="updateFieldError('rotationDurationMinutes')"
                  />
                  <span class="input-unit">分钟</span>
                </div>
              </UiFormField>
              <UiFormField
                label="滚动大小"
                :error="validationErrors.rotationSizeMb"
              >
                <div class="input-with-unit">
                  <UiInputNumber
                    v-model="rotationSizeMb"
                    :min="1"
                    :max="10000"
                    @blur="updateFieldError('rotationSizeMb')"
                  />
                  <span class="input-unit">MB</span>
                </div>
              </UiFormField>
            </div>
            <div v-else class="empty-hint">
              <span>启用后，当采集时长或文件大小达到阈值时，自动滚动到新文件继续记录</span>
            </div>
          </UiPanel>
        </section>

        <!-- 自动停止条件 -->
        <section
          id="settings-panel-acquisition"
          role="tabpanel"
          aria-labelledby="settings-tab-acquisition"
          v-show="activeTab === 'acquisition'"
          :aria-hidden="activeTab !== 'acquisition'"
          class="settings-section"
        >
          <UiPanel class="form-card">
            <template #header>
              <div class="card-head">
                <Clock :size="15" />
                <span class="card-head__title">自动停止条件</span>
                <UiStatusBadge v-if="enabledConditionsCount > 0" status="connected">
                  {{ enabledConditionsCount }} 项
                </UiStatusBadge>
              </div>
            </template>
            <div class="conditions-list">
              <div
                class="condition-row"
                :class="{ 'condition-row--on': durationEnabled }"
                role="checkbox"
                :aria-checked="durationEnabled"
                tabindex="0"
                @click="durationEnabled = !durationEnabled"
                @keydown.enter.space.prevent="durationEnabled = !durationEnabled"
              >
                <div class="condition-row__main">
                  <CheckCircle v-if="durationEnabled" :size="14" class="icon-check" />
                  <div v-else class="icon-circle" />
                  <span class="condition-row__label" :class="{ 'condition-row__label--on': durationEnabled }">
                    定时停止
                  </span>
                </div>
                <div v-if="durationEnabled" class="condition-row__input" @click.stop @keydown.enter.stop @keydown.space.stop>
                  <UiInputNumber
                    v-model="durationMinutes"
                    :min="1"
                    :max="1440"
                    @blur="updateFieldError('durationMinutes')"
                  />
                  <span class="input-unit">分钟</span>
                </div>
              </div>
              <p v-if="validationErrors.durationMinutes" class="field-error">{{ validationErrors.durationMinutes }}</p>

              <div
                class="condition-row"
                :class="{ 'condition-row--on': sizeEnabled }"
                role="checkbox"
                :aria-checked="sizeEnabled"
                tabindex="0"
                @click="sizeEnabled = !sizeEnabled"
                @keydown.enter.space.prevent="sizeEnabled = !sizeEnabled"
              >
                <div class="condition-row__main">
                  <CheckCircle v-if="sizeEnabled" :size="14" class="icon-check" />
                  <div v-else class="icon-circle" />
                  <span class="condition-row__label" :class="{ 'condition-row__label--on': sizeEnabled }">
                    按文件大小停止
                  </span>
                </div>
                <div v-if="sizeEnabled" class="condition-row__input" @click.stop @keydown.enter.stop @keydown.space.stop>
                  <UiInputNumber
                    v-model="sizeMb"
                    :min="1"
                    :max="10000"
                    @blur="updateFieldError('sizeMb')"
                  />
                  <span class="input-unit">MB</span>
                </div>
              </div>
              <p v-if="validationErrors.sizeMb" class="field-error">{{ validationErrors.sizeMb }}</p>

              <div
                class="condition-row"
                :class="{ 'condition-row--on': countEnabled }"
                role="checkbox"
                :aria-checked="countEnabled"
                tabindex="0"
                @click="countEnabled = !countEnabled"
                @keydown.enter.space.prevent="countEnabled = !countEnabled"
              >
                <div class="condition-row__main">
                  <CheckCircle v-if="countEnabled" :size="14" class="icon-check" />
                  <div v-else class="icon-circle" />
                  <span class="condition-row__label" :class="{ 'condition-row__label--on': countEnabled }">
                    按记录数停止
                  </span>
                </div>
                <div v-if="countEnabled" class="condition-row__input" @click.stop @keydown.enter.stop @keydown.space.stop>
                  <UiInputNumber
                    v-model="recordCount"
                    :min="1"
                    :max="100000000"
                    @blur="updateFieldError('recordCount')"
                  />
                  <span class="input-unit">条</span>
                </div>
              </div>
              <p v-if="validationErrors.recordCount" class="field-error">{{ validationErrors.recordCount }}</p>
            </div>
            <div class="hint-row">
              <span class="hint-text">满足任一条件即自动停止采集，未启用则不限制</span>
            </div>
          </UiPanel>
        </section>

        <!-- 刷新率 -->
        <section
          id="settings-panel-advanced"
          role="tabpanel"
          aria-labelledby="settings-tab-advanced"
          v-show="activeTab === 'advanced'"
          :aria-hidden="activeTab !== 'advanced'"
          class="settings-section"
        >
          <UiPanel class="form-card">
            <template #header>
              <div class="card-head">
                <RefreshCw :size="15" />
                <span class="card-head__title">刷新率</span>
              </div>
            </template>
            <div class="form-fields">
              <UiFormField
                label="实时数据刷新频率"
                :error="validationErrors.refreshRate"
                hint="较高的刷新率会占用更多系统资源"
              >
                <div class="refresh-row">
                  <div class="refresh-slider">
                    <UiSlider v-model="refreshRate" :min="1" :max="20" :step="1" aria-label="实时数据刷新频率" />
                    <div class="refresh-labels">
                      <span class="refresh-label">1 Hz</span>
                      <span
                        class="refresh-label refresh-label--highlight"
                        :class="{ 'refresh-label--active': refreshRate >= 5 && refreshRate <= 15 }"
                      >推荐 5–15 Hz</span>
                      <span class="refresh-label">20 Hz</span>
                    </div>
                  </div>
                  <div class="refresh-value">
                    <UiInputNumber
                      v-model="refreshRate"
                      :min="1"
                      :max="20"
                      size="small"
                      @blur="updateFieldError('refreshRate')"
                    />
                    <span class="input-unit">Hz</span>
                  </div>
                </div>
              </UiFormField>
            </div>
          </UiPanel>
        </section>
      </div>
    </div>

    <template #footer>
      <div class="modal-foot">
        <div class="modal-foot__left">
          <UiButton size="md" variant="ghost" :disabled="saving" @click="onReset">
            <template #icon><RotateCcw :size="14" /></template>恢复默认
          </UiButton>
          <span class="foot-hint">保存后对当前桌面会话生效</span>
        </div>
        <div class="flex gap-2">
          <UiButton size="md" :disabled="saving" @click="onClose">取消</UiButton>
          <UiButton size="md" variant="primary" :loading="saving" :disabled="loading" @click="onSave">
            <template #icon><Save :size="14" /></template>保存设置
          </UiButton>
        </div>
      </div>
    </template>
  </UiDialog>
</template>

<style scoped>
/* ===== 模态框头部 ===== */
.modal-head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: var(--space-3);
  width: 100%;
}

.modal-head__info {
  display: flex;
  flex-direction: column;
  gap: var(--space-1);
}

.modal-head__title {
  font-size: var(--font-size-lg);
  font-weight: var(--font-weight-bold);
  color: var(--text-primary);
  line-height: var(--line-height-tight);
}

.modal-head__subtitle {
  font-size: var(--font-size-2xs);
  color: var(--text-muted);
  line-height: var(--line-height-base);
}

/* ===== 加载状态 ===== */
.loading-wrap {
  display: flex;
  justify-content: center;
  padding: var(--space-10) 0;
}

/* ===== 左右分栏布局 ===== */
.settings-layout {
  display: flex;
  gap: var(--space-4);
  min-height: 320px;
}

/* 左侧标签导航 */
.settings-tabs {
  display: flex;
  flex-direction: column;
  gap: var(--space-1);
  width: 140px;
  flex-shrink: 0;
  padding-right: var(--space-3);
  border-right: 1px solid var(--border-default);
}

.settings-tab {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  padding: var(--space-2) var(--space-3);
  border-radius: var(--radius-md);
  font-size: var(--font-size-sm);
  font-weight: var(--font-weight-medium);
  color: var(--text-secondary);
  background: transparent;
  border: none;
  cursor: pointer;
  transition: all var(--motion-fast) var(--easing-standard);
  text-align: left;
}

.settings-tab:hover {
  color: var(--text-primary);
  background: var(--bg-panel);
}

.settings-tab:focus-visible {
  outline: none;
  box-shadow: 0 0 0 2px var(--focus-ring-soft), 0 0 0 4px var(--focus-ring);
}

.settings-tab--active {
  color: var(--accent-primary);
  background: var(--accent-primary-muted);
  font-weight: var(--font-weight-semibold);
}

.settings-tab--active:hover {
  background: color-mix(in srgb, var(--accent-primary) 12%, transparent);
}

/* 右侧内容区 */
.settings-content {
  flex: 1;
  min-width: 0;
  overflow-y: auto;
  max-height: 480px;
  padding-right: var(--space-2);
}

.settings-section {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
}

/* ===== 表单卡片 ===== */
.form-card {
  font-size: var(--font-size-sm);
}

.card-head {
  display: flex;
  align-items: center;
  gap: var(--space-2);
}

.card-head__title {
  font-size: var(--font-size-xs);
  font-weight: var(--font-weight-bold);
  color: var(--text-primary);
}

/* ===== 表单字段区域 ===== */
.form-fields {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
}

/* ===== 输入框组合 ===== */
.input-with-action {
  display: flex;
  gap: var(--space-2);
}

.input-with-action :deep(.n-input) {
  flex: 1;
  min-width: 0;
}

.input-with-unit {
  display: flex;
  align-items: center;
  gap: var(--space-2);
}

.input-unit {
  font-size: var(--font-size-xs);
  color: var(--text-muted);
  white-space: nowrap;
}

/* ===== 开关行 ===== */
.toggle-row {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  padding: var(--space-2) var(--space-3);
  border-radius: var(--radius-md);
  border: 1px solid var(--border-default);
  background: var(--bg-app);
}

.toggle-row__label {
  font-size: var(--font-size-sm);
  color: var(--text-secondary);
}

/* ===== 主题切换 ===== */
.theme-switch {
  display: flex;
  gap: var(--space-2);
}

/* ===== 语言切换 ===== */
.locale-switch {
  display: flex;
  align-items: center;
  padding: var(--space-1);
  background: var(--bg-panel-strong);
  border-radius: var(--radius-lg);
  border: 1px solid var(--border-default);
  gap: var(--space-1);
  width: fit-content;
}

.locale-btn {
  display: flex;
  align-items: center;
  gap: var(--space-1);
  padding: var(--space-1) var(--space-3);
  font-size: var(--font-size-2xs);
  font-weight: var(--font-weight-medium);
  color: var(--text-muted);
  border-radius: var(--radius-md);
  transition: all var(--motion-fast) var(--easing-standard);
  border: none;
  background: transparent;
  cursor: pointer;
  letter-spacing: 0.02em;
}

.locale-btn:hover {
  color: var(--text-primary);
  background: var(--bg-panel);
}

.locale-btn:focus-visible {
  outline: none;
  box-shadow: 0 0 0 2px var(--focus-ring);
}

.locale-switch .locale-btn.locale-btn--active {
  background: var(--bg-panel);
  color: var(--text-primary);
  font-weight: var(--font-weight-semibold);
  box-shadow: var(--shadow-panel);
}

.locale-switch .locale-btn.locale-btn--active:hover {
  background: var(--bg-panel-strong);
}

/* ===== 条件行列表 ===== */
.conditions-list {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
}

.condition-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--space-2);
  padding: var(--space-2) var(--space-3);
  border-radius: var(--radius-md);
  border: 1px solid var(--border-default);
  background: var(--bg-app);
  cursor: pointer;
  transition: all var(--motion-fast) var(--easing-standard);
  position: relative;
}

/* 选中态左侧指示条 */
.condition-row::before {
  content: '';
  position: absolute;
  left: 0;
  top: var(--space-2);
  bottom: var(--space-2);
  width: 3px;
  border-radius: 0 var(--radius-pill) var(--radius-pill) 0;
  background: var(--accent-success);
  opacity: 0;
  transition: opacity var(--motion-fast) var(--easing-standard);
}

.condition-row:hover {
  border-color: color-mix(in srgb, var(--accent-primary) 30%, var(--border-default));
  background: color-mix(in srgb, var(--accent-primary) 3%, var(--bg-app));
}

.condition-row:focus-visible {
  outline: none;
  box-shadow: 0 0 0 2px var(--focus-ring-soft), 0 0 0 4px var(--focus-ring);
}

.condition-row--on {
  border-color: color-mix(in srgb, var(--accent-success) 30%, var(--border-default));
  background: color-mix(in srgb, var(--accent-success) 5%, var(--bg-app));
}

.condition-row--on::before {
  opacity: 1;
}

.condition-row--on:hover {
  border-color: color-mix(in srgb, var(--accent-success) 50%, var(--border-default));
  background: color-mix(in srgb, var(--accent-success) 8%, var(--bg-app));
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
  color: var(--text-tertiary);
  transition: color var(--motion-fast) var(--easing-standard);
}

.condition-row__label--on {
  color: var(--text-primary);
  font-weight: var(--font-weight-medium);
}

.condition-row__input {
  display: flex;
  align-items: center;
  gap: var(--space-1);
  flex-shrink: 0;
}

.icon-check {
  color: var(--accent-success);
  flex-shrink: 0;
}

.icon-circle {
  width: 14px;
  height: 14px;
  border-radius: 50%;
  border: 1.5px solid var(--border-strong);
  flex-shrink: 0;
}

/* 字段级错误提示 */
.field-error {
  margin: calc(var(--space-1) * -1) 0 0 var(--space-5);
  font-size: var(--font-size-xs);
  color: var(--accent-danger);
  line-height: var(--line-height-base);
}

/* 提示行 */
.hint-row {
  padding: var(--space-2) 0 0;
}

.hint-text {
  font-size: var(--font-size-xs);
  color: var(--text-muted);
}

/* 空状态提示 */
.empty-hint {
  padding: var(--space-2) 0;
}

.empty-hint span {
  font-size: var(--font-size-xs);
  color: var(--text-muted);
  line-height: var(--line-height-base);
}

/* ===== 刷新率区域 ===== */
.refresh-row {
  display: flex;
  align-items: center;
  gap: var(--space-4);
}

.refresh-slider {
  flex: 1;
  display: flex;
  flex-direction: column;
  gap: var(--space-1);
}

.refresh-labels {
  display: flex;
  justify-content: space-between;
}

.refresh-label {
  font-size: var(--font-size-2xs);
  color: var(--text-muted);
}

.refresh-label--highlight {
  font-weight: var(--font-weight-medium);
}

.refresh-label--active {
  color: var(--accent-primary);
}

.refresh-value {
  display: flex;
  align-items: center;
  gap: var(--space-1);
  flex-shrink: 0;
}

/* ===== 底部操作栏 ===== */
.modal-foot {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--space-3);
  width: 100%;
}

.modal-foot__left {
  display: flex;
  align-items: center;
  gap: var(--space-3);
}

.foot-hint {
  font-size: var(--font-size-xs);
  color: var(--text-muted);
}

/* ===== 响应式适配 ===== */
@media (max-width: 640px) {
  .settings-layout {
    flex-direction: column;
  }

  .settings-tabs {
    flex-direction: row;
    width: 100%;
    padding-right: 0;
    padding-bottom: var(--space-3);
    border-right: none;
    border-bottom: 1px solid var(--border-default);
    overflow-x: auto;
  }

  .settings-tab {
    white-space: nowrap;
  }

  .settings-content {
    max-height: none;
  }
}
</style>
