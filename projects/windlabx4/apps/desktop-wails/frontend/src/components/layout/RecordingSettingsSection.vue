<script setup lang="ts">
import { ref } from 'vue'
import { useFeedbackStore } from '@stores/feedbackStore'
import { useI18nStore } from '@stores/i18nStore'
import {
  useStorageStore,
  type StorageSettings,
} from '@stores/storageStore'
import { storageApi } from '@api/deviceApi'
import { wailsApi } from '@api/wails-adapter'
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
} from '@lucide/vue'

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

/** 字段级校验错误记录 */
const validationErrors = ref<Record<string, string>>({})

const enabledConditionsCount = () =>
  [durationEnabled.value, sizeEnabled.value, countEnabled.value].filter(Boolean).length

/** 加载业务级 StorageSettings（由父组件 storageStore.loadSettings 后传入） */
async function load(settings: StorageSettings): Promise<void> {
  validationErrors.value = {}
  applySettings(settings)
  try {
    const status = await storageApi.status()
    if (status.outputDir) baseDirectory.value = status.outputDir
  } catch { /* ok */ }
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
 *  historyWindowSec / refreshRateHz 属于"界面"区段，由父组件从 DisplaySettingsSection 取值后合入。 */
function buildRecordingSettings(historyWindowSec: number, refreshRateHz: number): StorageSettings {
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
    historyWindowSec,
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

/** 恢复默认设置（仅录制相关字段；historyWindowSec 由 Display 区段自行重置） */
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
 * 保存业务级 StorageSettings（合入父组件传入的 historyWindowSec / refreshRateHz）。
 * 所有设置即时生效，无需重启。
 *
 * 返回值语义：await saveSettings 成功即视为保存成功返回 true；
 * saveSettings 抛异常时由父组件 try/catch 捕获并提示，本函数无需返回 false。
 * 之前恒返回 false 与签名 Promise<boolean> 语义矛盾，已修正。
 */
async function save(historyWindowSec: number, refreshRateHz: number): Promise<boolean> {
  await storageStore.saveSettings(buildRecordingSettings(historyWindowSec, refreshRateHz))
  return true
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
      <!-- 整改：数据保存字段改为垂直堆叠（标签在上、控件在下），
           与文件滚动保存、自动停止条件卡片的左侧对齐线统一 -->
      <div class="form-fields form-fields--stacked">
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
        <!-- 开关行不再套 UiFormField，直接作为一行使用 toggle-row，保持左对齐 -->
        <div class="toggle-row">
          <span class="toggle-row__label">{{ i18n.t.set_autoStartOnAcquisition }}</span>
          <UiToggle v-model="autoStart" />
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
      <!-- 整改：两个短字段并排，标签在上，与数据保存卡片左侧对齐线统一 -->
      <div v-if="rotationEnabled" class="form-fields form-fields--stacked form-row--inline">
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
