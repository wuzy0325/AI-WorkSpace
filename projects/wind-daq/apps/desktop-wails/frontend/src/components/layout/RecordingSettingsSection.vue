<script setup lang="ts">
import { computed, ref } from 'vue'
import { useFeedbackStore } from '@stores/feedbackStore'
import { useI18nStore } from '@stores/i18nStore'
import {
  useStorageStore,
  type StorageSettings,
} from '@stores/storageStore'
import { storageApi } from '@api/deviceApi'
import { isWailsAvailable, wailsApi } from '@api/wails-adapter'
import { request } from '@api/http-client'
import UiButton from '@components/ui/UiButton.vue'
import UiStatusBadge from '@components/ui/UiStatusBadge.vue'
import UiPanel from '@components/ui/UiPanel.vue'
import UiInput from '@components/ui/UiInput.vue'
import UiInputNumber from '@components/ui/UiInputNumber.vue'
import UiToggle from '@components/ui/UiToggle.vue'
import UiFormField from '@components/ui/UiFormField.vue'
import {
  CheckCircle,
  Clock,
  FileText,
  Folder,
  HardDrive,
  ChevronDown,
  ChevronRight,
  Database,
} from '@lucide/vue'

/**
 * sink 调优参数（持久化在 storage.json，由装配层 NewSinkFromConfig 读取）。
 * 与业务级 StorageSettings（storage-settings.json）分离，避免双轨配置冲突。
 * 修改后需重启应用生效（sink 在装配时一次性创建，运行时不重建）。
 */
interface SinkTuningParams {
  format: 'csv' | 'binary'
  queueCapacity: number
  bufferSize: number
  flushIntervalMs: number
  syncIntervalSec: number
}

/** sink 调优参数的默认值（与后端 csv_sink.go 默认常量对齐） */
const DEFAULT_SINK_TUNING: SinkTuningParams = {
  format: 'csv',
  queueCapacity: 32768,
  bufferSize: 1048576, // 1 MB
  flushIntervalMs: 100,
  syncIntervalSec: 2,
}

const SINK_TUNING_CONFIG_KEY = 'storage'
const SINK_TUNING_MIN = {
  queueCapacity: 1024,
  bufferSize: 4096,
  flushIntervalMs: 10,
  syncIntervalSec: 1,
}
const SINK_TUNING_MAX = {
  queueCapacity: 1048576,
  bufferSize: 16777216, // 16 MB
  flushIntervalMs: 5000,
  syncIntervalSec: 60,
}

const feedback = useFeedbackStore()
const i18n = useI18nStore()
const storageStore = useStorageStore()

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

// sink 调优参数（持久化在 storage.json，与业务级 StorageSettings 分离）。
// 注意：这些参数在 sink 装配时读取，运行时不重建 sink，故修改后需重启应用生效。
const sinkTuning = ref<SinkTuningParams>({ ...DEFAULT_SINK_TUNING })
// 加载时保存的 sink 调优参数快照，作为 save 时 dirty 比较基线，
// 用于让父组件精确判断本次保存是否需要提示"重启生效"。
const originalSinkTuning = ref<SinkTuningParams>({ ...DEFAULT_SINK_TUNING })
const advancedExpanded = ref(false)

/** 字段级校验错误记录 */
const validationErrors = ref<Record<string, string>>({})

/** sink 调优参数动态提示：将 {min}/{max} 占位符替换为实际数值 */
const queueCapacityHint = computed(() =>
  i18n.t.set_queueCapacityHint
    .replace('{min}', String(SINK_TUNING_MIN.queueCapacity))
    .replace('{max}', String(SINK_TUNING_MAX.queueCapacity))
)
const bufferSizeHint = computed(() =>
  i18n.t.set_bufferSizeHint
    .replace('{min}', String(SINK_TUNING_MIN.bufferSize))
    .replace('{max}', String(SINK_TUNING_MAX.bufferSize))
)
const flushIntervalHint = computed(() =>
  i18n.t.set_flushIntervalHint
    .replace('{min}', String(SINK_TUNING_MIN.flushIntervalMs))
    .replace('{max}', String(SINK_TUNING_MAX.flushIntervalMs))
)
const syncIntervalHint = computed(() =>
  i18n.t.set_syncIntervalHint
    .replace('{min}', String(SINK_TUNING_MIN.syncIntervalSec))
    .replace('{max}', String(SINK_TUNING_MAX.syncIntervalSec))
)

const enabledConditionsCount = () =>
  [durationEnabled.value, sizeEnabled.value, countEnabled.value].filter(Boolean).length

/** 加载业务级 StorageSettings（由父组件 storageStore.loadSettings 后传入）+ sink 调优参数 */
async function load(settings: StorageSettings): Promise<void> {
  validationErrors.value = {}
  applySettings(settings)
  try {
    const status = await storageApi.status()
    if (status.outputDir) baseDirectory.value = status.outputDir
  } catch { /* ok */ }
  await loadSinkTuning()
}

/** 从 storage 配置键加载 sink 调优参数，缺失字段回退到默认值 */
async function loadSinkTuning(): Promise<void> {
  try {
    let data: Partial<SinkTuningParams> | null = null
    if (isWailsAvailable()) {
      const res = await wailsApi.config.load<Partial<SinkTuningParams>>(SINK_TUNING_CONFIG_KEY)
      if (res.success && res.data) data = res.data
    } else {
      const res = await request<{ success: boolean; data?: Partial<SinkTuningParams> }>(`/api/config/${SINK_TUNING_CONFIG_KEY}`)
      if (res.success && res.data) data = res.data
    }
    if (data) {
      sinkTuning.value = {
        format: data.format === 'binary' ? 'binary' : 'csv',
        queueCapacity: clampSinkTuning(data.queueCapacity, 'queueCapacity'),
        bufferSize: clampSinkTuning(data.bufferSize, 'bufferSize'),
        flushIntervalMs: clampSinkTuning(data.flushIntervalMs, 'flushIntervalMs'),
        syncIntervalSec: clampSinkTuning(data.syncIntervalSec, 'syncIntervalSec'),
      }
    } else {
      sinkTuning.value = { ...DEFAULT_SINK_TUNING }
    }
  } catch {
    // 读取失败不阻塞设置面板：保留默认值即可
    sinkTuning.value = { ...DEFAULT_SINK_TUNING }
  }
  // 同步 dirty 基线快照，确保 save 时的"是否修改"判断以本次加载值为准
  originalSinkTuning.value = { ...sinkTuning.value }
}

/** 浅比较两个 sink 调优参数对象是否相等（所有字段均为基础类型，浅比较足够） */
function isSinkTuningEqual(a: SinkTuningParams, b: SinkTuningParams): boolean {
  return a.format === b.format
    && a.queueCapacity === b.queueCapacity
    && a.bufferSize === b.bufferSize
    && a.flushIntervalMs === b.flushIntervalMs
    && a.syncIntervalSec === b.syncIntervalSec
}

/** 限制 sink 调优参数到合法范围；非法或缺失时回退到默认值 */
function clampSinkTuning(value: number | undefined, field: keyof typeof SINK_TUNING_MIN): number {
  const min = SINK_TUNING_MIN[field]
  const max = SINK_TUNING_MAX[field]
  const def = DEFAULT_SINK_TUNING[field]
  if (typeof value !== 'number' || !Number.isFinite(value)) return def as number
  return Math.max(min, Math.min(max, Math.round(value)))
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

/** 构建业务级 StorageSettings 的录制相关字段。
 *  waveformBufferSize / refreshRateHz 属于"界面"区段，由父组件从 DisplaySettingsSection 取值后合入。 */
function buildRecordingSettings(waveformBufferSize: number, refreshRateHz: number): StorageSettings {
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
    waveformBufferSize,
    refreshRateHz,
  }
}

/** 校验指定字段，返回错误信息或空字符串 */
function validateField(field: string): string {
  switch (field) {
    case 'baseDirectory':
      return baseDirectory.value.trim() ? '' : i18n.t.set_baseDirRequired
    case 'filePrefix':
      return filePrefix.value.trim() ? '' : i18n.t.set_filePrefixRequired
    case 'durationMinutes':
      return durationEnabled.value && (durationMinutes.value < 1 || durationMinutes.value > 1440)
        ? i18n.t.set_durationRangeError : ''
    case 'sizeMb':
      return sizeEnabled.value && (sizeMb.value < 1 || sizeMb.value > 10000)
        ? i18n.t.set_sizeRangeError : ''
    case 'recordCount':
      return countEnabled.value && (recordCount.value < 1 || recordCount.value > 100000000)
        ? i18n.t.set_recordCountRangeError : ''
    case 'rotationDurationMinutes':
      return rotationEnabled.value && (rotationDurationMinutes.value < 1 || rotationDurationMinutes.value > 1440)
        ? i18n.t.set_rotationDurationRangeError : ''
    case 'rotationSizeMb':
      return rotationEnabled.value && (rotationSizeMb.value < 1 || rotationSizeMb.value > 10000)
        ? i18n.t.set_rotationSizeRangeError : ''
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

/** 全量校验，返回错误映射（空对象表示通过） */
function validate(): Record<string, string> {
  const fields = [
    'baseDirectory', 'filePrefix', 'durationMinutes', 'sizeMb',
    'recordCount', 'rotationDurationMinutes', 'rotationSizeMb',
  ]
  const errs: Record<string, string> = {}
  for (const field of fields) {
    const error = validateField(field)
    if (error) errs[field] = error
  }
  validationErrors.value = errs
  return errs
}

/** 恢复默认设置（仅录制相关字段；waveformBufferSize 由 Display 区段自行重置） */
function reset(): void {
  baseDirectory.value = 'data/recordings'
  filePrefix.value = 'run'
  autoStart.value = false
  durationEnabled.value = false
  durationMinutes.value = 10
  sizeEnabled.value = false
  sizeMb.value = 100
  countEnabled.value = false
  recordCount.value = 1000000
  rotationEnabled.value = false
  rotationDurationMinutes.value = 30
  rotationSizeMb.value = 100
  sinkTuning.value = { ...DEFAULT_SINK_TUNING }
  // 同步重置 dirty 基线，避免用户先 reset 再保存时被误判为"已修改 sink 调优"
  originalSinkTuning.value = { ...DEFAULT_SINK_TUNING }
  validationErrors.value = {}
}

async function handlePickDirectory(): Promise<void> {
  try {
    const dir = await wailsApi.app.pickDirectory()
    if (dir) {
      baseDirectory.value = dir
      updateFieldError('baseDirectory')
    }
  } catch {
    feedback.pushToast(i18n.t.failedChooseDirectory, 'error')
  }
}

/**
 * 保存业务级 StorageSettings（合入父组件传入的 waveformBufferSize / refreshRateHz）+ sink 调优参数。
 * 返回值：本次保存是否实际修改了 sink 调优参数（用于父组件决定是否提示"重启生效"）。
 * 业务级 StorageSettings 与 refreshRate/waveformBufferSize 都是即时生效，无需重启。
 */
async function save(waveformBufferSize: number, refreshRateHz: number): Promise<boolean> {
  await storageStore.saveSettings(buildRecordingSettings(waveformBufferSize, refreshRateHz))
  const sinkTuningChanged = !isSinkTuningEqual(sinkTuning.value, originalSinkTuning.value)
  await saveSinkTuning()
  // 保存成功后同步 dirty 基线，避免下次保存被重复判为"已修改"
  originalSinkTuning.value = { ...sinkTuning.value }
  return sinkTuningChanged
}

/** 保存 sink 调优参数到 storage 配置键 */
async function saveSinkTuning(): Promise<void> {
  // 保存前再次范围限制，避免 UI 通过隐藏输入绕过校验
  const payload: SinkTuningParams = {
    format: sinkTuning.value.format === 'binary' ? 'binary' : 'csv',
    queueCapacity: clampSinkTuning(sinkTuning.value.queueCapacity, 'queueCapacity'),
    bufferSize: clampSinkTuning(sinkTuning.value.bufferSize, 'bufferSize'),
    flushIntervalMs: clampSinkTuning(sinkTuning.value.flushIntervalMs, 'flushIntervalMs'),
    syncIntervalSec: clampSinkTuning(sinkTuning.value.syncIntervalSec, 'syncIntervalSec'),
  }
  if (isWailsAvailable()) {
    await wailsApi.config.save(SINK_TUNING_CONFIG_KEY, payload)
  } else {
    await request(`/api/config/${SINK_TUNING_CONFIG_KEY}`, {
      method: 'PUT',
      body: JSON.stringify(payload),
    })
  }
  sinkTuning.value = payload
}

defineExpose({ load, save, reset, validate, enabledConditionsCount })
</script>

<template>
  <section
    id="settings-panel-recording"
    role="tabpanel"
    aria-labelledby="settings-tab-recording"
    class="settings-section"
  >
    <UiPanel :segmented="false" class="form-card">
      <template #header>
        <div class="card-head">
          <FileText :size="15" />
          <span class="card-head__title">{{ i18n.t.set_dataSave }}</span>
        </div>
      </template>
      <div class="form-fields">
        <UiFormField
          :label="i18n.t.set_saveDir"
          :error="validationErrors.baseDirectory"
          :hint="i18n.t.set_saveDirHint"
        >
          <div class="input-with-action">
            <UiInput
              v-model="baseDirectory"
              placeholder="data/recordings"
              @blur="updateFieldError('baseDirectory')"
            />
            <UiButton size="md" :aria-label="i18n.t.set_pickSaveDir" data-test="settings-pick-directory" @click="handlePickDirectory">
              <template #icon><Folder :size="14" /></template>{{ i18n.t.set_choose }}
            </UiButton>
          </div>
        </UiFormField>
        <UiFormField
          :label="i18n.t.set_filePrefix"
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
          <span class="toggle-row__label">{{ i18n.t.set_autoStartOnAcquisition }}</span>
        </div>
        <!-- 存储格式选择：csv（文本，可读性强）或 binary（紧凑，性能高） -->
        <UiFormField
          :label="i18n.t.set_storageFormat"
          :hint="i18n.t.set_formatHint"
        >
          <div class="format-switch">
            <button
              type="button"
              class="format-btn"
              :class="{ 'format-btn--active': sinkTuning.format === 'csv' }"
              data-test="settings-format-csv"
              @click="sinkTuning.format = 'csv'"
            >
              <FileText :size="12" />CSV
            </button>
            <button
              type="button"
              class="format-btn"
              :class="{ 'format-btn--active': sinkTuning.format === 'binary' }"
              data-test="settings-format-binary"
              @click="sinkTuning.format = 'binary'"
            >
              <Database :size="12" />Binary
            </button>
          </div>
        </UiFormField>
        <!-- 高级设置：sink 调优参数（队列容量/缓冲大小/flush/sync）。
             这些参数在 sink 装配时读取，运行时不变，故需重启应用生效。 -->
        <div class="advanced-section">
          <button
            type="button"
            class="advanced-toggle"
            :aria-expanded="advancedExpanded"
            data-test="settings-advanced-toggle"
            @click="advancedExpanded = !advancedExpanded"
          >
            <component :is="advancedExpanded ? ChevronDown : ChevronRight" :size="14" />
            <span>{{ i18n.t.set_advancedSettings }}</span>
          </button>
          <div v-if="advancedExpanded" class="advanced-body">
            <UiFormField
              :label="i18n.t.set_queueCapacity"
              :hint="queueCapacityHint"
            >
              <div class="input-with-unit">
                <UiInputNumber
                  v-model="sinkTuning.queueCapacity"
                  :min="SINK_TUNING_MIN.queueCapacity"
                  :max="SINK_TUNING_MAX.queueCapacity"
                  :step="1024"
                  size="small"
                  data-test="settings-sink-queue"
                />
                <span class="input-unit">{{ i18n.t.countUnitLabel }}</span>
              </div>
            </UiFormField>
            <UiFormField
              :label="i18n.t.set_bufferSize"
              :hint="bufferSizeHint"
            >
              <div class="input-with-unit">
                <UiInputNumber
                  v-model="sinkTuning.bufferSize"
                  :min="SINK_TUNING_MIN.bufferSize"
                  :max="SINK_TUNING_MAX.bufferSize"
                  :step="4096"
                  size="small"
                  data-test="settings-sink-buffer"
                />
                <span class="input-unit">B</span>
              </div>
            </UiFormField>
            <UiFormField
              :label="i18n.t.set_flushInterval"
              :hint="flushIntervalHint"
            >
              <div class="input-with-unit">
                <UiInputNumber
                  v-model="sinkTuning.flushIntervalMs"
                  :min="SINK_TUNING_MIN.flushIntervalMs"
                  :max="SINK_TUNING_MAX.flushIntervalMs"
                  :step="10"
                  size="small"
                  data-test="settings-sink-flush"
                />
                <span class="input-unit">ms</span>
              </div>
            </UiFormField>
            <UiFormField
              :label="i18n.t.set_syncInterval"
              :hint="syncIntervalHint"
            >
              <div class="input-with-unit">
                <UiInputNumber
                  v-model="sinkTuning.syncIntervalSec"
                  :min="SINK_TUNING_MIN.syncIntervalSec"
                  :max="SINK_TUNING_MAX.syncIntervalSec"
                  :step="1"
                  size="small"
                  data-test="settings-sink-sync"
                />
                <span class="input-unit">s</span>
              </div>
            </UiFormField>
            <p class="advanced-warn">
              {{ i18n.t.set_advancedWarn }}
            </p>
          </div>
        </div>
      </div>
    </UiPanel>

    <UiPanel class="form-card">
      <template #header>
        <div class="card-head">
          <HardDrive :size="15" />
          <span class="card-head__title">{{ i18n.t.rotationLabel }}</span>
          <UiToggle v-model="rotationEnabled" />
        </div>
      </template>
      <div v-if="rotationEnabled" class="form-fields">
        <UiFormField
          :label="i18n.t.set_rotationDuration"
          :error="validationErrors.rotationDurationMinutes"
        >
          <div class="input-with-unit">
            <UiInputNumber
              v-model="rotationDurationMinutes"
              :min="1"
              :max="1440"
              @blur="updateFieldError('rotationDurationMinutes')"
            />
            <span class="input-unit">{{ i18n.t.set_minutes }}</span>
          </div>
        </UiFormField>
        <UiFormField
          :label="i18n.t.set_rotationSize"
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
        <span>{{ i18n.t.set_rotationEmptyHint }}</span>
      </div>
    </UiPanel>

    <!-- 自动停止条件 -->
    <UiPanel class="form-card">
      <template #header>
        <div class="card-head">
          <Clock :size="15" />
          <span class="card-head__title">{{ i18n.t.set_autoStopConditions }}</span>
          <UiStatusBadge v-if="enabledConditionsCount() > 0" status="connected">
            {{ enabledConditionsCount() }} {{ i18n.t.set_items }}
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
              {{ i18n.t.set_durationStop }}
            </span>
          </div>
          <div v-if="durationEnabled" class="condition-row__input" @click.stop @keydown.enter.stop @keydown.space.stop>
            <UiInputNumber
              v-model="durationMinutes"
              :min="1"
              :max="1440"
              @blur="updateFieldError('durationMinutes')"
            />
            <span class="input-unit">{{ i18n.t.set_minutes }}</span>
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
              {{ i18n.t.set_sizeStop }}
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
              {{ i18n.t.set_countStop }}
            </span>
          </div>
          <div v-if="countEnabled" class="condition-row__input" @click.stop @keydown.enter.stop @keydown.space.stop>
            <UiInputNumber
              v-model="recordCount"
              :min="1"
              :max="100000000"
              @blur="updateFieldError('recordCount')"
            />
            <span class="input-unit">{{ i18n.t.countUnitLabel }}</span>
          </div>
        </div>
        <p v-if="validationErrors.recordCount" class="field-error">{{ validationErrors.recordCount }}</p>
      </div>
      <div class="hint-row">
        <span class="hint-text">{{ i18n.t.set_stopConditionsHint }}</span>
      </div>
    </UiPanel>
  </section>
</template>
