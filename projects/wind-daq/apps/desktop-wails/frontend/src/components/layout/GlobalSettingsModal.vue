<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { storeToRefs } from 'pinia'
import { useFeedbackStore } from '@stores/feedbackStore'
import { useI18nStore } from '@stores/i18nStore'
import { useStorageStore, type StorageSettings } from '@stores/storageStore'
import { deviceApi, storageApi } from '@api/deviceApi'
import { wailsApi } from '@api/wails-adapter'
import {
  NAlert,
  NButton,
  NCard,
  NInput,
  NInputNumber,
  NModal,
  NResult,
  NSlider,
  NSpace,
  NSpin,
  NSwitch,
  NTag,
  NText,
} from 'naive-ui'
import {
  CheckCircle,
  Clock,
  FileText,
  Folder,
  HardDrive,
  RefreshCw,
  Save,
  X,
} from '@lucide/vue'

const props = defineProps<{ open: boolean }>()
const emit = defineEmits<{
  (e: 'close'): void
  (e: 'update:open', value: boolean): void
}>()

const feedback = useFeedbackStore()
const storageStore = useStorageStore()

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

watch(() => props.open, (open) => { if (open) void loadSettings() })

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

function validate(): boolean {
  const v = validationError
  if (!baseDirectory.value.trim()) v.value = '保存目录不能为空'
  else if (!filePrefix.value.trim()) v.value = '文件前缀不能为空'
  else if (durationEnabled.value && (durationMinutes.value < 1 || durationMinutes.value > 1440))
    v.value = '定时停止范围为 1 到 1440 分钟'
  else if (sizeEnabled.value && (sizeMb.value < 1 || sizeMb.value > 10000))
    v.value = '文件大小范围为 1 到 10000 MB'
  else if (countEnabled.value && (recordCount.value < 1 || recordCount.value > 100000000))
    v.value = '记录数范围为 1 到 100000000'
  else if (rotationEnabled.value && (rotationDurationMinutes.value < 1 || rotationDurationMinutes.value > 1440))
    v.value = '滚动时长范围为 1 到 1440 分钟'
  else if (rotationEnabled.value && (rotationSizeMb.value < 1 || rotationSizeMb.value > 10000))
    v.value = '滚动大小范围为 1 到 10000 MB'
  else if (refreshRate.value < 1 || refreshRate.value > 20)
    v.value = '刷新率范围为 1 到 20 Hz'
  else v.value = ''
  return !v.value
}

function onClose(): void {
  if (saving.value) return
  isVisible.value = false
  emit('close')
}

async function handlePickDirectory(): Promise<void> {
  try {
    const dir = await wailsApi.app.pickDirectory()
    if (dir) baseDirectory.value = dir
  } catch {
    feedback.pushToast('选择目录失败', 'error')
  }
}

async function onSave(): Promise<void> {
  saving.value = true
  let saved = false
  try {
    if (!validate()) {
      feedback.pushToast(validationError.value || '设置无效', 'warning')
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
  <NModal
    v-model:show="isVisible"
    preset="card"
    :style="{ maxWidth: '32rem' }"
    title="全局设置"
    :bordered="false"
    :mask-closable="false"
    @close="onClose"
  >
    <template #header>
      <div class="modal-head">
        <div>
          <NText tag="div" depth="1" style="font-size:15px;font-weight:600">全局设置</NText>
          <NText depth="3" style="font-size:11px;margin-top:2px">数据保存、自动停止条件和实时刷新频率</NText>
        </div>
        <NButton quaternary circle size="small" @click="onClose"><template #icon><X :size="14" /></template></NButton>
      </div>
    </template>

    <NSpin v-if="loading" size="small" style="display:flex;justify-content:center;padding:40px 0" />
    <NResult v-else-if="loadError" status="error" title="设置加载失败" description="请检查后端连接" size="small">
      <template #footer><NButton size="small" @click="loadSettings">重试</NButton></template>
    </NResult>

    <div v-else class="settings-body">
      <NCard size="small" :bordered="true" :segmented="false" class="form-card">
        <template #header>
          <div class="card-head">
            <FileText :size="15" />
            <NText depth="1" style="font-size:12px;font-weight:600">数据保存</NText>
          </div>
        </template>
        <NSpace vertical size="small">
          <div class="field-row">
            <NText depth="3" style="font-size:11px">保存目录</NText>
            <NSpace>
              <NInput v-model:value="baseDirectory" placeholder="data/recordings" size="small" style="flex:1;min-width:220px" />
              <NButton size="small" @click="handlePickDirectory">
                <template #icon><Folder :size="14" /></template>选择
              </NButton>
            </NSpace>
          </div>
          <div class="field-row">
            <NText depth="3" style="font-size:11px">文件前缀</NText>
            <NInput v-model:value="filePrefix" placeholder="run" size="small" />
          </div>
          <div class="auto-start-row">
            <NSwitch v-model:value="autoStart" size="small" />
            <NText depth="2" style="font-size:12px">开始采集时自动开始记录</NText>
          </div>
        </NSpace>
      </NCard>

      <NCard size="small" :bordered="true" class="form-card">
        <template #header>
          <div class="card-head">
            <Clock :size="15" />
            <NText depth="1" style="font-size:12px;font-weight:600">自动停止条件</NText>
            <NTag v-if="enabledConditionsCount > 0" size="tiny" type="success" round style="margin-left:auto">
              {{ enabledConditionsCount }} 项
            </NTag>
          </div>
        </template>
        <div class="conditions-list">
          <div
            class="condition-row"
            :class="{ 'condition-row--on': durationEnabled }"
            @click="durationEnabled = !durationEnabled"
          >
            <div class="condition-row__main">
              <CheckCircle v-if="durationEnabled" :size="14" class="icon-check" />
              <div v-else class="icon-circle" />
              <NText :depth="durationEnabled ? 1 : 3" style="font-size:12px">定时停止</NText>
            </div>
            <div v-if="durationEnabled" class="condition-row__input" @click.stop>
              <NInputNumber v-model:value="durationMinutes" :min="1" :max="1440" size="tiny" style="width:80px" />
              <NText depth="3" style="font-size:11px">分钟</NText>
            </div>
          </div>
          <div
            class="condition-row"
            :class="{ 'condition-row--on': sizeEnabled }"
            @click="sizeEnabled = !sizeEnabled"
          >
            <div class="condition-row__main">
              <CheckCircle v-if="sizeEnabled" :size="14" class="icon-check" />
              <div v-else class="icon-circle" />
              <NText :depth="sizeEnabled ? 1 : 3" style="font-size:12px">按文件大小停止</NText>
            </div>
            <div v-if="sizeEnabled" class="condition-row__input" @click.stop>
              <NInputNumber v-model:value="sizeMb" :min="1" :max="10000" size="tiny" style="width:80px" />
              <NText depth="3" style="font-size:11px">MB</NText>
            </div>
          </div>
          <div
            class="condition-row"
            :class="{ 'condition-row--on': countEnabled }"
            @click="countEnabled = !countEnabled"
          >
            <div class="condition-row__main">
              <CheckCircle v-if="countEnabled" :size="14" class="icon-check" />
              <div v-else class="icon-circle" />
              <NText :depth="countEnabled ? 1 : 3" style="font-size:12px">按记录数停止</NText>
            </div>
            <div v-if="countEnabled" class="condition-row__input" @click.stop>
              <NInputNumber v-model:value="recordCount" :min="1" :max="100000000" size="tiny" style="width:96px" />
              <NText depth="3" style="font-size:11px">条</NText>
            </div>
          </div>
        </div>
      </NCard>

      <NCard size="small" :bordered="true" class="form-card">
        <template #header>
          <div class="card-head">
            <HardDrive :size="15" />
            <NText depth="1" style="font-size:12px;font-weight:600">文件滚动保存</NText>
            <NSwitch v-model:value="rotationEnabled" size="small" style="margin-left:auto" />
          </div>
        </template>
        <div v-if="rotationEnabled" class="conditions-list">
          <div class="condition-row condition-row--on">
            <NText depth="2" style="font-size:12px">滚动时长</NText>
            <div class="condition-row__input">
              <NInputNumber v-model:value="rotationDurationMinutes" :min="1" :max="1440" size="tiny" style="width:80px" />
              <NText depth="3" style="font-size:11px">分钟</NText>
            </div>
          </div>
          <div class="condition-row condition-row--on">
            <NText depth="2" style="font-size:12px">滚动大小</NText>
            <div class="condition-row__input">
              <NInputNumber v-model:value="rotationSizeMb" :min="1" :max="10000" size="tiny" style="width:80px" />
              <NText depth="3" style="font-size:11px">MB</NText>
            </div>
          </div>
        </div>
        <div v-else class="empty-hint">
          <NText depth="3" style="font-size:11px">启用后可在采集时长或大小达到阈值时自动滚动到新文件</NText>
        </div>
      </NCard>

      <NCard size="small" :bordered="true" class="form-card">
        <template #header>
          <div class="card-head">
            <RefreshCw :size="15" />
            <NText depth="1" style="font-size:12px;font-weight:600">刷新率</NText>
          </div>
        </template>
        <div class="refresh-row">
          <div class="refresh-slider">
            <NSlider v-model:value="refreshRate" :min="1" :max="20" :step="1" />
            <div class="refresh-labels">
              <NText depth="3" style="font-size:10px">1 Hz</NText>
              <NText :depth="refreshRate >= 5 && refreshRate <= 15 ? 1 : 3" style="font-size:10px;font-weight:500">推荐 5–15 Hz</NText>
              <NText depth="3" style="font-size:10px">20 Hz</NText>
            </div>
          </div>
          <div class="refresh-value">
            <NInputNumber v-model:value="refreshRate" :min="1" :max="20" size="small" style="width:72px" />
            <NText depth="3" style="font-size:11px">Hz</NText>
          </div>
        </div>
      </NCard>

      <NAlert v-if="validationError" type="warning" :bordered="false" closable @close="validationError = ''">
        {{ validationError }}
      </NAlert>
    </div>

    <template #footer>
      <div class="modal-foot">
        <NText depth="3" style="font-size:11px">保存后对当前桌面会话生效</NText>
        <NSpace size="small">
          <NButton size="small" :disabled="saving" @click="onClose">取消</NButton>
          <NButton size="small" type="primary" :loading="saving" :disabled="loading" @click="onSave">
            <template #icon><Save :size="14" /></template>保存设置
          </NButton>
        </NSpace>
      </div>
    </template>
  </NModal>
</template>

<style scoped>
.modal-head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 12px;
  width: 100%;
}

.settings-body {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.form-card {
  font-size: 12px;
}

.card-head {
  display: flex;
  align-items: center;
  gap: 6px;
}

.field-row {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.auto-start-row {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 6px 8px;
  border-radius: 4px;
  border: 1px solid var(--border-default);
  background: var(--bg-app);
}

.conditions-list {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.condition-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  padding: 8px 10px;
  border-radius: 6px;
  border: 1px solid var(--border-default);
  background: var(--bg-app);
  cursor: pointer;
  transition: all 150ms ease;
}

.condition-row:hover {
  border-color: color-mix(in srgb, var(--accent-primary) 30%, var(--border-default));
  background: color-mix(in srgb, var(--accent-primary) 3%, var(--bg-app));
}

.condition-row--on {
  border-color: color-mix(in srgb, var(--accent-success) 30%, var(--border-default));
  background: color-mix(in srgb, var(--accent-success) 5%, var(--bg-app));
}

.condition-row--on:hover {
  border-color: color-mix(in srgb, var(--accent-success) 50%, var(--border-default));
  background: color-mix(in srgb, var(--accent-success) 8%, var(--bg-app));
}

.condition-row__main {
  display: flex;
  align-items: center;
  gap: 8px;
  flex: 1;
  min-width: 0;
}

.condition-row__input {
  display: flex;
  align-items: center;
  gap: 4px;
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

.empty-hint {
  padding: 4px 0;
}

.refresh-row {
  display: flex;
  align-items: center;
  gap: 16px;
}

.refresh-slider {
  flex: 1;
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.refresh-labels {
  display: flex;
  justify-content: space-between;
}

.refresh-value {
  display: flex;
  align-items: center;
  gap: 4px;
  flex-shrink: 0;
}

.modal-foot {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  width: 100%;
}
</style>