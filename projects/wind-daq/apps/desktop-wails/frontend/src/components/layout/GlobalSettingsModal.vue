<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { storeToRefs } from 'pinia'
import { useFeedbackStore } from '@stores/feedbackStore'
import { useI18nStore } from '@stores/i18nStore'
import { useStorageStore, type StorageSettings } from '@stores/storageStore'
import { deviceApi, storageApi } from '@api/deviceApi'
import { wailsApi } from '@api/wails-adapter'
import UiButton from '@components/ui/UiButton.vue'
import UiSpin from '@components/ui/UiSpin.vue'
import UiStatusBadge from '@components/ui/UiStatusBadge.vue'
import UiAlert from '@components/ui/UiAlert.vue'
import UiPanel from '@components/ui/UiPanel.vue'
import UiInput from '@components/ui/UiInput.vue'
import UiInputNumber from '@components/ui/UiInputNumber.vue'
import UiSlider from '@components/ui/UiSlider.vue'
import UiToggle from '@components/ui/UiToggle.vue'
import UiErrorState from '@components/ui/UiErrorState.vue'
import UiDialog from '@components/ui/UiDialog.vue'
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
const validationErrors = ref<Record<string, string>>({})
const validationError = computed(() => Object.values(validationErrors.value).find(Boolean) || '')

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

function validate(): boolean {
  const errs: Record<string, string> = {}
  if (!baseDirectory.value.trim()) errs.baseDirectory = '保存目录不能为空'
  if (!filePrefix.value.trim()) errs.filePrefix = '文件前缀不能为空'
  if (durationEnabled.value && (durationMinutes.value < 1 || durationMinutes.value > 1440))
    errs.durationMinutes = '定时停止范围为 1 到 1440 分钟'
  if (sizeEnabled.value && (sizeMb.value < 1 || sizeMb.value > 10000))
    errs.sizeMb = '文件大小范围为 1 到 10000 MB'
  if (countEnabled.value && (recordCount.value < 1 || recordCount.value > 100000000))
    errs.recordCount = '记录数范围为 1 到 100000000'
  if (rotationEnabled.value && (rotationDurationMinutes.value < 1 || rotationDurationMinutes.value > 1440))
    errs.rotationDurationMinutes = '滚动时长范围为 1 到 1440 分钟'
  if (rotationEnabled.value && (rotationSizeMb.value < 1 || rotationSizeMb.value > 10000))
    errs.rotationSizeMb = '滚动大小范围为 1 到 10000 MB'
  if (refreshRate.value < 1 || refreshRate.value > 20)
    errs.refreshRate = '刷新率范围为 1 到 20 Hz'
  validationErrors.value = errs
  return Object.keys(errs).length === 0
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
  <UiDialog
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
          <div style="font-size:15px;font-weight:600">全局设置</div>
          <span style="font-size:11px;margin-top:2px;color:var(--text-tertiary)">数据保存、自动停止条件和实时刷新频率</span>
        </div>
        <UiButton quaternary circle size="md" @click="onClose"><template #icon><X :size="14" /></template></UiButton>
      </div>
    </template>

    <UiSpin v-if="loading" style="display:flex;justify-content:center;padding:40px 0" />
    <UiErrorState v-else-if="loadError" title="设置加载失败" message="请检查后端连接">
      <template #action><UiButton size="md" @click="loadSettings">重试</UiButton></template>
    </UiErrorState>

    <div v-else class="settings-body">
      <UiPanel :segmented="false" class="form-card">
        <template #header>
          <div class="card-head">
            <FileText :size="15" />
            <span style="font-size:12px;font-weight:600">数据保存</span>
          </div>
        </template>
        <div class="flex flex-col gap-2">
          <div class="field-row">
            <span style="font-size:11px;color:var(--text-tertiary)">保存目录</span>
            <div class="flex gap-2">
              <UiInput v-model="baseDirectory" placeholder="data/recordings" style="flex:1;min-width:220px" />
              <UiButton size="md" @click="handlePickDirectory">
                <template #icon><Folder :size="14" /></template>选择
              </UiButton>
            </div>
          </div>
          <div class="field-row">
            <span style="font-size:11px;color:var(--text-tertiary)">文件前缀</span>
            <UiInput v-model="filePrefix" placeholder="run"  />
          </div>
          <div class="auto-start-row">
            <UiToggle v-model="autoStart" />
            <span style="font-size:12px;color:var(--text-secondary)">开始采集时自动开始记录</span>
          </div>
        </div>
      </UiPanel>

      <UiPanel class="form-card">
        <template #header>
          <div class="card-head">
            <Clock :size="15" />
            <span style="font-size:12px;font-weight:600">自动停止条件</span>
            <UiStatusBadge v-if="enabledConditionsCount > 0" status="connected" style="margin-left:auto">
              {{ enabledConditionsCount }} 项
            </UiStatusBadge>
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
              <span :style="{ color: durationEnabled ? 'var(--text-primary)' : 'var(--text-tertiary)', fontSize: '12px' }">定时停止</span>
            </div>
            <div v-if="durationEnabled" class="condition-row__input" @click.stop>
              <UiInputNumber v-model="durationMinutes" :min="1" :max="1440" style="width:80px" />
              <span style="font-size:11px;color:var(--text-tertiary)">分钟</span>
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
              <span :style="{ color: sizeEnabled ? 'var(--text-primary)' : 'var(--text-tertiary)', fontSize: '12px' }">按文件大小停止</span>
            </div>
            <div v-if="sizeEnabled" class="condition-row__input" @click.stop>
              <UiInputNumber v-model="sizeMb" :min="1" :max="10000" style="width:80px" />
              <span style="font-size:11px;color:var(--text-tertiary)">MB</span>
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
              <span :style="{ color: countEnabled ? 'var(--text-primary)' : 'var(--text-tertiary)', fontSize: '12px' }">按记录数停止</span>
            </div>
            <div v-if="countEnabled" class="condition-row__input" @click.stop>
              <UiInputNumber v-model="recordCount" :min="1" :max="100000000" style="width:96px" />
              <span style="font-size:11px;color:var(--text-tertiary)">条</span>
            </div>
          </div>
        </div>
      </UiPanel>

      <UiPanel class="form-card">
        <template #header>
          <div class="card-head">
            <HardDrive :size="15" />
            <span style="font-size:12px;font-weight:600">文件滚动保存</span>
            <UiToggle v-model="rotationEnabled" style="margin-left:auto" />
          </div>
        </template>
        <div v-if="rotationEnabled" class="conditions-list">
          <div class="condition-row condition-row--on">
            <span style="font-size:12px;color:var(--text-secondary)">滚动时长</span>
            <div class="condition-row__input">
              <UiInputNumber v-model="rotationDurationMinutes" :min="1" :max="1440" style="width:80px" />
              <span style="font-size:11px;color:var(--text-tertiary)">分钟</span>
            </div>
          </div>
          <div class="condition-row condition-row--on">
            <span style="font-size:12px;color:var(--text-secondary)">滚动大小</span>
            <div class="condition-row__input">
              <UiInputNumber v-model="rotationSizeMb" :min="1" :max="10000" style="width:80px" />
              <span style="font-size:11px;color:var(--text-tertiary)">MB</span>
            </div>
          </div>
        </div>
        <div v-else class="empty-hint">
          <span style="font-size:11px;color:var(--text-tertiary)">启用后可在采集时长或大小达到阈值时自动滚动到新文件</span>
        </div>
      </UiPanel>

      <UiPanel class="form-card">
        <template #header>
          <div class="card-head">
            <RefreshCw :size="15" />
            <span style="font-size:12px;font-weight:600">刷新率</span>
          </div>
        </template>
        <div class="refresh-row">
          <div class="refresh-slider">
            <UiSlider v-model="refreshRate" :min="1" :max="20" :step="1" />
            <div class="refresh-labels">
              <span style="font-size:10px;color:var(--text-tertiary)">1 Hz</span>
              <span :style="{ color: refreshRate >= 5 && refreshRate <= 15 ? 'var(--text-primary)' : 'var(--text-tertiary)', fontSize: '10px', fontWeight: '500' }">推荐 5–15 Hz</span>
              <span style="font-size:10px;color:var(--text-tertiary)">20 Hz</span>
            </div>
          </div>
          <div class="refresh-value">
            <UiInputNumber v-model="refreshRate" :min="1" :max="20" size="small" style="width:72px" />
            <span style="font-size:11px;color:var(--text-tertiary)">Hz</span>
          </div>
        </div>
      </UiPanel>

      <UiAlert v-if="validationError" type="warning" closable @close="validationError = ''">
        {{ validationError }}
      </UiAlert>
    </div>

    <template #footer>
      <div class="modal-foot">
        <span style="font-size:11px;color:var(--text-tertiary)">保存后对当前桌面会话生效</span>
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