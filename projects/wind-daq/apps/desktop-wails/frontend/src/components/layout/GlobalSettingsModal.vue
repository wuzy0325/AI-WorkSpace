<script setup lang="ts">
import { computed, type Component, nextTick, ref, watch } from 'vue'
import { useFeedbackStore } from '@stores/feedbackStore'
import { useI18nStore } from '@stores/i18nStore'
import { useStorageStore } from '@stores/storageStore'
import UiButton from '@components/ui/UiButton.vue'
import UiSpin from '@components/ui/UiSpin.vue'
import UiErrorState from '@components/ui/UiErrorState.vue'
import UiDialog from '@components/ui/UiDialog.vue'
import DisplaySettingsSection from './DisplaySettingsSection.vue'
import RecordingSettingsSection from './RecordingSettingsSection.vue'
import { FileText, Monitor, RotateCcw, Save, X } from '@lucide/vue'

/** 设置分组标签页类型（按需扩展） */
type SettingsTab = 'display' | 'recording'

const props = defineProps<{ open: boolean }>()
const emit = defineEmits<{
  (e: 'close'): void
  (e: 'update:open', value: boolean): void
}>()

const feedback = useFeedbackStore()
const i18n = useI18nStore()
const storageStore = useStorageStore()

const loading = ref(false)
const saving = ref(false)
const loadError = ref(false)
const activeTab = ref<SettingsTab>('display')

const displayRef = ref<InstanceType<typeof DisplaySettingsSection>>()
const recordingRef = ref<InstanceType<typeof RecordingSettingsSection>>()

const isVisible = computed({
  get: () => props.open,
  set: (value) => emit('update:open', value),
})

/** Tab 配置列表（按需扩展新的分组；label 跟随全局语言切换） */
const TABS = computed<{ key: SettingsTab; label: string; icon: Component }[]>(() => [
  { key: 'display', label: i18n.t.set_tabDisplay, icon: Monitor },
  { key: 'recording', label: i18n.t.set_tabRecording, icon: FileText },
])

watch(() => props.open, (open) => { if (open) void loadSettings() })

async function loadSettings(): Promise<void> {
  loading.value = true
  loadError.value = false
  // storageStore.loadSettings 内部已 catch 错误并写入 storageStore.loadError（不抛出），
  // 这里读 loadError 判断是否失败——否则配置加载失败（如 JSON 损坏）会被静默吞掉，
  // 弹窗显示默认值却无任何错误提示。
  await storageStore.loadSettings()
  if (storageStore.loadError) {
    loadError.value = true
    console.error('[GlobalSettings] 配置加载失败:', storageStore.loadError)
    loading.value = false
    return
  }
  // 必须先结束 loading 再调用子组件 load()：模板中 v-if/v-else 在 loading 期间不渲染
  // DisplaySettingsSection / RecordingSettingsSection，displayRef/recordingRef 为 undefined。
  // 这里用显式检查而非 ?. —— 子组件未挂载属于程序错误（如模板漏绑 ref / 时序错乱），
  // 必须显式暴露（走 loadError 让用户看到"加载失败"），不能用 ?. 静默跳过，
  // 否则会导致重开弹窗时子组件始终显示本地 ref 初始值却无任何报错。
  loading.value = false
  await nextTick()
  const display = displayRef.value
  const recording = recordingRef.value
  if (!display || !recording) {
    loadError.value = true
    console.error('[GlobalSettings] 设置子组件未挂载：displayRef/recordingRef 为空，检查模板 ref 绑定')
    return
  }
  const settings = storageStore.settings
  await Promise.all([
    display.load(settings),
    recording.load(settings),
  ])
}

function onClose(): void {
  if (saving.value) return
  isVisible.value = false
  emit('close')
}

/** 恢复默认设置（两区段各自重置） */
function onReset(): void {
  const display = displayRef.value
  const recording = recordingRef.value
  if (!display || !recording) {
    // ref 缺失属程序错误，必须显式反馈而非 ?. 静默跳过
    feedback.pushToast(i18n.t.set_panelNotReady, 'error')
    console.error('[GlobalSettings] onReset: 子组件未挂载')
    return
  }
  display.reset()
  recording.reset()
  feedback.pushToast(i18n.t.set_defaultsRestored, 'info')
}

async function onSave(): Promise<void> {
  saving.value = true
  let saved = false
  try {
    const display = displayRef.value
    const recording = recordingRef.value
    // ref 缺失属程序错误（如模板漏绑 ref），必须抛错暴露，不能用 ?. 静默跳过。
    // 否则会发生"提示已保存但实际未保存"的静默失败（参见本次 bug）。
    if (!display || !recording) {
      throw new Error('设置子组件未挂载：检查模板 ref 绑定')
    }
    const errs = {
      ...display.validate(),
      ...recording.validate(),
    }
    if (Object.keys(errs).length) {
      const firstError = Object.values(errs).find(Boolean) || i18n.t.set_invalidSettings
      feedback.pushToast(firstError, 'warning')
      return
    }
    // 先持久化（recording.save 合入 waveformBufferSize + refreshRateHz 落盘 + sink 调优），
    // 成功后再 display.save() 下发后端即时生效。顺序不可颠倒：若先下发后持久化，
    // 持久化失败时后端已是新值而配置仍是旧值，重启后被旧配置回滚，产生不一致。
    const waveformBufferSize = display.waveformBufferSize
    const refreshRateHz = display.refreshRate
    const sinkTuningChanged = await recording.save(waveformBufferSize, refreshRateHz)
    await display.save()
    const message = sinkTuningChanged
      ? i18n.t.set_savedWithRestart
      : i18n.t.set_saved
    feedback.pushToast(message, 'success')
    saved = true
  } catch {
    feedback.pushToast(i18n.t.set_saveFailed, 'error')
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
    :style="{ maxWidth: '42rem', width: '92vw' }"
    :title="i18n.t.set_globalSettings"
    :bordered="false"
    :mask-closable="false"
    @close="onClose"
  >
    <template #header>
      <div class="modal-head">
        <div class="modal-head__info">
          <div class="modal-head__title">{{ i18n.t.set_globalSettings }}</div>
          <span class="modal-head__subtitle">{{ i18n.t.set_globalSettingsSubtitle }}</span>
        </div>
        <UiButton quaternary circle size="md" @click="onClose">
          <template #icon><X :size="14" /></template>
        </UiButton>
      </div>
    </template>

    <UiSpin v-if="loading" class="loading-wrap" />
    <UiErrorState v-else-if="loadError" :title="i18n.t.set_loadFailed" :message="i18n.t.set_checkBackend">
      <template #action><UiButton size="md" @click="loadSettings">{{ i18n.t.set_retry }}</UiButton></template>
    </UiErrorState>

    <div v-else class="settings-layout">
      <!-- 左侧标签导航 -->
      <nav class="settings-tabs" role="tablist" :aria-label="i18n.t.set_settingsGroup">
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
        <DisplaySettingsSection ref="displayRef" v-show="activeTab === 'display'" />
        <RecordingSettingsSection ref="recordingRef" v-show="activeTab === 'recording'" />
      </div>
    </div>

    <template #footer>
      <div class="modal-foot">
        <div class="modal-foot__left">
          <UiButton size="md" variant="ghost" :disabled="saving" @click="onReset">
            <template #icon><RotateCcw :size="14" /></template>{{ i18n.t.set_restoreDefaults }}
          </UiButton>
          <span class="foot-hint">{{ i18n.t.set_saveHint }}</span>
        </div>
        <div class="flex gap-2">
          <UiButton size="md" :disabled="saving" @click="onClose">{{ i18n.t.cancel }}</UiButton>
          <UiButton size="md" variant="primary" :loading="saving" :disabled="loading" @click="onSave">
            <template #icon><Save :size="14" /></template>{{ i18n.t.set_saveSettings }}
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

/* ===== 左右分栏布局 ===== — 紧凑密度：分栏间距 8px */
.settings-layout {
  display: flex;
  gap: var(--space-2);
}

/* 左侧标签导航 — 紧凑密度：宽度收窄到 112px */
.settings-tabs {
  display: flex;
  flex-direction: column;
  gap: var(--space-1);
  width: 112px;
  flex-shrink: 0;
  padding-right: var(--space-2);
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

/* 右侧内容区 — 不限制高度，由 UiDialog content-style 控制滚动条 */
.settings-content {
  flex: 1;
  min-width: 0;
}

:deep(.settings-section) {
  display: flex;
  flex-direction: column;
  /* 紧凑密度：卡片间 8px */
  gap: var(--space-2);
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
