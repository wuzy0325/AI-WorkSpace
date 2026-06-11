<script setup lang="ts">
import { computed, reactive, onMounted, watch, ref, provide } from 'vue'
import { useMotionStore } from '@stores/motionStore'
import { useFeedbackStore } from '@stores/feedbackStore'
import type { MotionControllerProfile, AxisConfig, AxisEncoderCompensationConfig } from '@shared/types/motion'

import UiButton from '@components/ui/UiButton.vue'
import UiSelect, { type UiSelectOption } from '@components/ui/UiSelect.vue'
import UiInput from '@components/ui/UiInput.vue'
import UiToggle from '@components/ui/UiToggle.vue'

import { DEFAULT_AXIS_NAMES, createDefaultAxis, defaultEncComp } from './motionConfigEditor'
import ProfileSidebar from './ProfileSidebar.vue'
import AxisConfigCard from './AxisConfigCard.vue'
import EncoderCompensationEditor from './EncoderCompensationEditor.vue'

const props = defineProps<{ open: boolean; currentId?: string | null }>()
const emit = defineEmits<{
  (e: 'close'): void
  (e: 'saved', id: string): void
}>()

const motion = useMotionStore()
const feedback = useFeedbackStore()

/* -- Tooltip system -- */
const activeTooltip = ref<{ text: string; x: number; y: number } | null>(null)

function showTooltip(text: string, event: MouseEvent): void {
  activeTooltip.value = { text, x: event.clientX, y: event.clientY - 32 }
}
function hideTooltip(): void {
  activeTooltip.value = null
}
provide<(text: string, event: MouseEvent) => void>('showTooltip', showTooltip)
provide<() => void>('hideTooltip', hideTooltip)

/* -- 控制器默认值常量 -- */
const DEFAULT_NAME = '新控制器'
const DEFAULT_TYPE = 'SIMULATED-MC'
const DEFAULT_ADDRESS = '127.0.0.1'
const DEFAULT_PORT = 5176
const MAX_PORT = 65535

/* -- Form state -- */
const editing = reactive<MotionControllerProfile>({
  id: '', name: '', type: DEFAULT_TYPE, address: DEFAULT_ADDRESS, port: DEFAULT_PORT, autoConnect: false,
  axes: DEFAULT_AXIS_NAMES.map((name) => createDefaultAxis(name)),
})

const isEdit = computed(() => !!editing.id)

/* -- Creating-new indicator -- */
const isCreatingNew = ref(false)

/* -- Saving state -- */
const saving = ref(false)

/* -- Controller type options -- */
const controllerTypeOptions = computed<UiSelectOption[]>(() => [
  { value: DEFAULT_TYPE, label: '模拟控制器' },
  { value: 'B140-MC', label: 'B140 控制器' },
  { value: 'WTNMC4A-MC', label: 'WTNMC4A 控制器' },
])

/* -- Form validation -- */
type FieldErrors = {
  name?: string
  address?: string
  port?: string
}

const fieldErrors = computed<FieldErrors>(() => {
  const errors: FieldErrors = {}
  // name is required
  if (!editing.name.trim()) {
    errors.name = '控制器名称不能为空'
  }
  // name must be unique
  const duplicate = motion.profiles.find(p => p.name.trim() === editing.name.trim() && p.id !== editing.id)
  if (duplicate) {
    errors.name = '已存在同名控制器'
  }
  // address is required
  if (!editing.address.trim()) {
    errors.address = 'IP 地址不能为空'
  }
  // port range check
  if (!Number.isFinite(editing.port) || editing.port < 1 || editing.port > MAX_PORT) {
    errors.port = `端口范围 1-${MAX_PORT}`
  }
  return errors
})

const validationErrorCount = computed(() => Object.values(fieldErrors.value).filter(Boolean).length)

/* -- Dirty detection -- */
const initialDraftSnapshot = ref('')

/** Serialize current form state as a snapshot string. */
function snapshotDraft(): string {
  return JSON.stringify({
    name: editing.name,
    type: editing.type,
    address: editing.address,
    port: editing.port,
    autoConnect: editing.autoConnect,
    axes: editing.axes,
  })
}

/** Whether the form differs from the initial snapshot. */
const isDirty = ref(false)

/** Mark the form as dirty. */
function markDirty(): void {
  isDirty.value = true
}

/** Capture current form state as the initial snapshot and reset dirty flag. */
function captureSnapshot(): void {
  initialDraftSnapshot.value = snapshotDraft()
  isDirty.value = false
}

/* -- New profile -- */
function newProfile(): void {
  skipDirtyWatch = true
  editing.id = ''
  editing.name = DEFAULT_NAME
  editing.type = DEFAULT_TYPE
  editing.address = DEFAULT_ADDRESS
  editing.port = DEFAULT_PORT
  editing.autoConnect = false
  editing.axes = DEFAULT_AXIS_NAMES.map((name) => createDefaultAxis(name))
  isCreatingNew.value = true
  captureSnapshot()
  // 下一微任务恢复监听，确保 captureSnapshot 已完成
  queueMicrotask(() => { skipDirtyWatch = false })
}

/* -- Edit profile -- */
function editProfile(src: MotionControllerProfile): void {
  skipDirtyWatch = true
  editing.id = src.id
  editing.name = src.name
  editing.type = src.type
  editing.address = src.address
  editing.port = src.port
  editing.autoConnect = src.autoConnect
  editing.axes = src.axes.map((a) => ({
    ...a,
    enabled: a.enabled ?? true,
    stepsPerRev: a.stepsPerRev ?? 1.8,
    microSteps: a.microSteps ?? 4,
    lead: a.lead ?? 4,
    gearRatio: a.gearRatio ?? 1,
    positionSource: a.positionSource ?? 'register',
    encoderScale: a.encoderScale ?? 0.005,
    encoderCompensation: a.encoderCompensation ?? defaultEncComp(),
  }))
  isCreatingNew.value = false
  captureSnapshot()
  // 下一微任务恢复监听，确保 captureSnapshot 已完成
  queueMicrotask(() => { skipDirtyWatch = false })
}

/* -- Save profile -- */
async function save(): Promise<void> {
  // 验证失败时阻止保存
  if (validationErrorCount.value > 0) return
  saving.value = true
  // 保存期间跳过脏状态监听，避免异步等待期间 watch 干扰
  skipDirtyWatch = true
  try {
    const profile: MotionControllerProfile = {
      id: editing.id || (crypto.randomUUID?.() ?? `${Date.now()}-${Math.random().toString(36).slice(2, 8)}`),
      name: editing.name.trim() || DEFAULT_NAME,
      type: editing.type,
      address: editing.address.trim() || DEFAULT_ADDRESS,
      port: Number.isFinite(editing.port) ? editing.port : DEFAULT_PORT,
      autoConnect: editing.autoConnect,
      axes: editing.axes.map((a) => ({
        name: a.name, enabled: a.enabled, kind: a.kind ?? (a.name === 'U' ? 'ROTARY' as const : 'LINEAR' as const),
        maxSpeed: a.maxSpeed, minLimit: a.minLimit, maxLimit: a.maxLimit,
        stepsPerRev: a.stepsPerRev, microSteps: a.microSteps,
        lead: a.lead, gearRatio: a.gearRatio,
        inverted: a.inverted, encoderInverted: a.encoderInverted,
        positionSource: a.positionSource, encoderScale: a.encoderScale,
        encoderCompensation: a.encoderCompensation,
      })),
    }
    await motion.upsertProfile(profile)
    isCreatingNew.value = false
    captureSnapshot()
    feedback.pushToast('控制器配置已保存', 'success')
    emit('saved', profile.id)
    emit('close')
  } finally {
    saving.value = false
    // 下一微任务恢复监听，确保 captureSnapshot 已完成
    queueMicrotask(() => { skipDirtyWatch = false })
  }
}

/* -- Delete profile -- */
async function remove(id: string): Promise<void> {
  const profile = motion.profiles.find(p => p.id === id)
  const name = profile?.name ?? ''
  const ok = await feedback.confirm(`确定要删除控制器「${name}」吗？此操作不可撤销。`, {
    title: '删除确认',
    confirmText: '删除',
    cancelText: '取消',
  })
  if (!ok) return
  try {
    await motion.deleteProfile(id)
    feedback.pushToast('控制器已删除', 'info')
    emit('close')
  } catch (e) {
    feedback.pushToast(`删除失败: ${e instanceof Error ? e.message : '未知错误'}`, 'error')
  }
}

/* -- Re-entry guard for async confirm dialogs -- */
const confirming = ref(false)

/** 包装带确认对话框的异步操作，防止快速重复点击导致 confirm 竞态 */
async function withConfirmGuard(fn: () => Promise<void>): Promise<void> {
  if (confirming.value) return
  confirming.value = true
  try {
    await fn()
  } finally {
    confirming.value = false
  }
}

/* -- Close confirm (dirty check) -- */
async function tryClose(): Promise<void> {
  await withConfirmGuard(async () => {
    if (isDirty.value) {
      const ok = await feedback.confirm('当前有未保存的更改，确定要放弃更改并关闭吗？', {
        title: '关闭确认',
        confirmText: '放弃并关闭',
        cancelText: '继续编辑',
        variant: 'primary',
      })
      if (!ok) return
    }
    emit('close')
  })
}

/* -- Sidebar: switch profile (dirty check) -- */
async function onProfileSelect(id: string): Promise<void> {
  await withConfirmGuard(async () => {
    if (isDirty.value) {
      const ok = await feedback.confirm('当前有未保存的更改，切换配置将丢失更改。确定要切换吗？', {
        title: '切换确认',
        confirmText: '放弃并切换',
        cancelText: '继续编辑',
        variant: 'primary',
      })
      if (!ok) return
    }
    const p = motion.profiles.find(x => x.id === id)
    if (p) editProfile(p)
  })
}

/* -- Sidebar: new profile (dirty check) -- */
async function onProfileAdd(): Promise<void> {
  await withConfirmGuard(async () => {
    if (isDirty.value) {
      const ok = await feedback.confirm('当前有未保存的更改，新建配置将丢失更改。确定要新建吗？', {
        title: '新建确认',
        confirmText: '放弃并新建',
        cancelText: '继续编辑',
        variant: 'primary',
      })
      if (!ok) return
    }
    newProfile()
  })
}

/* -- Initialization -- */
function ensureEditingOnOpen(): void {
  if (!props.open) return
  if (props.currentId) {
    const target = motion.profiles.find((p) => p.id === props.currentId)
    if (target) { editProfile(target); return }
  }
  newProfile()
}

onMounted(async () => {
  await motion.refreshProfiles()
  ensureEditingOnOpen()
})

watch(() => props.open, (v) => { if (v) ensureEditingOnOpen() })

// 深度监听 editing 变化，标记脏状态
// 使用 skipDirtyWatch 标志避免在 editProfile/newProfile/save 赋值期间误触发
// 注意：skipDirtyWatch 是模块级变量，若同一页面渲染多个本组件实例会共享状态
// 当前场景下组件仅单实例使用，若需多实例请改为 ref
let skipDirtyWatch = false

watch(editing, () => {
  if (skipDirtyWatch) return
  if (initialDraftSnapshot.value) markDirty()
}, { deep: true })

/* -- Axis update callbacks -- */
function onAxisUpdate(index: number, axis: AxisConfig): void {
  editing.axes[index] = axis
  markDirty()
}

function onUpdateEncComp(index: number, value: AxisEncoderCompensationConfig): void {
  editing.axes[index].encoderCompensation = value
  markDirty()
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
      <div v-show="open" class="config-overlay" @click="tryClose">
        <Transition
          enter-active-class="transition ease-out duration-300"
          enter-from-class="opacity-0 scale-95 translate-y-4"
          enter-to-class="opacity-100 scale-100 translate-y-0"
          leave-active-class="transition ease-in duration-200"
          leave-from-class="opacity-100 scale-100 translate-y-0"
          leave-to-class="opacity-0 scale-95 translate-y-4"
        >
          <div v-show="open" class="config-panel" @click.stop>
            <!-- -- Header -- -->
            <header class="config-panel__header">
              <div class="config-panel__header-left">
                <div class="config-panel__title-row">
                  <svg class="config-panel__title-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                    <rect x="2" y="2" width="20" height="8" rx="2" /><rect x="2" y="14" width="20" height="8" rx="2" /><circle cx="6" cy="6" r="1" /><circle cx="6" cy="18" r="1" />
                  </svg>
                  <h2 class="config-panel__title">{{ isEdit ? editing.name : '新建控制器' }}</h2>
                  <!-- New-mode pulse indicator -->
                  <span v-if="isCreatingNew" class="creation-badge">
                    <span class="creation-badge__dot"></span>
                    新建中
                  </span>
                </div>
                <p class="config-panel__subtitle">{{ isEdit ? '编辑现有控制器配置' : '创建新的运动控制器配置' }}</p>
              </div>
              <div class="config-panel__header-right">
                <button class="config-panel__close" @click="tryClose">
                  <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                    <path d="M18 6L6 18M6 6l12 12"/>
                  </svg>
                </button>
              </div>
            </header>

            <!-- -- Body -- -->
            <div class="config-panel__body">
              <!-- Sidebar -->
              <div class="sidebar-area">
                <ProfileSidebar
                  :profiles="motion.profiles"
                  :active-id="editing.id"
                  @select="onProfileSelect"
                  @add="onProfileAdd"
                />
                <!-- Sidebar bottom creation indicator -->
                <div v-if="isCreatingNew" class="creation-indicator">
                  <span class="creation-indicator__dot"></span>
                  <span class="creation-indicator__text">新建中</span>
                </div>
              </div>

              <!-- Config content area -->
              <main class="config-content">
                <!-- Validation error banner -->
                <div v-if="validationErrorCount > 0" class="validation-banner">
                  <svg class="validation-banner__icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                    <circle cx="12" cy="12" r="10"/><line x1="12" y1="8" x2="12" y2="12"/><line x1="12" y1="16" x2="12.01" y2="16"/>
                  </svg>
                  <span>{{ validationErrorCount }} 项验证错误需要修正</span>
                </div>

                <!-- Basic info section -->
                <div class="config-section">
                  <h3 class="config-section__title">
                    <svg class="w-4 h-4 inline-block mr-1.5 -mt-0.5" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                      <circle cx="12" cy="12" r="3"/><path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 0 1-2.83 2.83l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-4 0v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 0 1-2.83-2.83l.06-.06A1.65 1.65 0 0 0 4.68 15a1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1 0-4h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 0 1 2.83-2.83l.06.06A1.65 1.65 0 0 0 9 4.68a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 4 0v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 0 1 2.83 2.83l-.06.06A1.65 1.65 0 0 0 19.4 9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 0 4h-.09a1.65 1.65 0 0 0-1.51 1z"/>
                    </svg>
                    基本信息
                  </h3>
                  <div class="form-grid">
                    <!-- Name -->
                    <UiInput
                      v-model="editing.name"
                      label="名称"
                      placeholder="控制器名称"
                      :error="fieldErrors.name"
                    />
                    <!-- Type -->
                    <div class="ui-field">
                      <label class="ui-field__label">类型</label>
                      <UiSelect
                        v-model="editing.type"
                        :options="controllerTypeOptions"
                        compact
                      />
                    </div>
                    <!-- Address -->
                    <UiInput
                      v-model="editing.address"
                      label="地址"
                      placeholder="127.0.0.1"
                      :error="fieldErrors.address"
                    />
                    <!-- Port -->
                    <UiInput
                      v-model="editing.port"
                      type="number"
                      label="端口"
                      :min="1"
                      :max="65535"
                      :step="1"
                      :error="fieldErrors.port"
                    />
                    <!-- Auto-connect toggle -->
                    <div class="config-field config-field--toggle">
                      <label class="config-field__label">自动连接</label>
                      <UiToggle v-model="editing.autoConnect" />
                    </div>
                  </div>
                </div>

                <!-- Axis config section -->
                <div class="config-section">
                  <h3 class="config-section__title">
                    <svg class="w-4 h-4 inline-block mr-1.5 -mt-0.5" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M12 2v4"/><path d="m16.2 7.8 2.9-2.9"/><path d="M18 12h4"/><path d="m16.2 16.2 2.9 2.9"/><path d="M12 18v4"/><path d="m4.9 19.1 2.9-2.9"/><path d="M2 12h4"/><path d="m4.9 4.9 2.9 2.9"/></svg>
                    轴配置
                    <span class="section-subtitle">配置每个轴的机械和电气参数</span>
                  </h3>
                  <div class="axis-matrix">
                    <AxisConfigCard
                      v-for="(axis, index) in editing.axes"
                      :key="axis.name"
                      :axis="axis"
                      :index="index"
                      @update="onAxisUpdate"
                    />
                  </div>
                </div>

                <!-- Encoder compensation -->
                <EncoderCompensationEditor
                  :axes="editing.axes"
                  :controller-type="editing.type"
                  @update-enc-comp="onUpdateEncComp"
                />
              </main>
            </div>

            <!-- -- Footer actions -- -->
            <footer class="config-panel__footer">
              <div class="config-panel__footer-left">
                <UiButton v-if="isEdit" variant="danger" size="sm" @click="remove(editing.id)">
                  删除
                </UiButton>
                <!-- Dirty state indicator -->
                <span v-if="isDirty" class="dirty-indicator">
                  <span class="dirty-indicator__dot"></span>
                  未保存
                </span>
              </div>
              <div class="config-panel__footer-right">
                <UiButton variant="secondary" size="sm" @click="tryClose">
                  取消
                </UiButton>
                <UiButton
                  variant="primary"
                  size="sm"
                  :loading="saving"
                  :disabled="validationErrorCount > 0"
                  @click="save"
                >
                  保存
                </UiButton>
              </div>
            </footer>
          </div>
        </Transition>
      </div>
    </Transition>
  </Teleport>

  <!-- Tooltip overlay -->
  <Teleport to="body">
    <Transition
      enter-active-class="transition ease-out duration-150"
      enter-from-class="opacity-0 scale-95"
      enter-to-class="opacity-100 scale-100"
      leave-active-class="transition ease-in duration-100"
      leave-from-class="opacity-100 scale-100"
      leave-to-class="opacity-0 scale-95"
    >
      <div
        v-if="activeTooltip"
        class="field-tooltip"
        :style="{ left: activeTooltip.x + 'px', top: activeTooltip.y + 'px' }"
      >
        {{ activeTooltip.text }}
      </div>
    </Transition>
  </Teleport>
</template>

<style scoped>
/* -- Overlay -- */
.config-overlay {
  position: fixed;
  inset: 0;
  z-index: 200;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: var(--space-4);
  background: rgba(0, 0, 0, 0.5);
  backdrop-filter: blur(4px);
}

/* -- Panel container -- */
.config-panel {
  width: 100%;
  max-width: 960px;
  max-height: 640px;
  display: flex;
  flex-direction: column;
  border-radius: var(--radius-xl);
  background: color-mix(in srgb, var(--bg-panel) 95%, transparent);
  border: 1px solid var(--border-default);
  box-shadow: 0 24px 64px -20px rgba(0, 0, 0, 0.5);
  outline: none;
}

/* -- Header -- */
.config-panel__header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0.75rem 1rem;
  border-bottom: 1px solid var(--border-default);
}
.config-panel__header-left {
  display: flex;
  flex-direction: column;
  gap: 0.125rem;
}
.config-panel__header-right {
  display: flex;
  align-items: center;
}
.config-panel__title-row {
  display: flex;
  align-items: center;
  gap: 0.5rem;
}
.config-panel__title-icon {
  width: 1rem;
  height: 1rem;
  color: var(--accent-success);
  flex-shrink: 0;
}
.config-panel__title {
  font-size: 1rem;
  font-weight: 700;
  color: var(--text-primary);
}
.config-panel__subtitle {
  font-size: 0.7rem;
  color: var(--text-muted);
  padding-left: 1.5rem;
}
.config-panel__close {
  width: 1.75rem;
  height: 1.75rem;
  display: flex;
  align-items: center;
  justify-content: center;
  border-radius: 0.375rem;
  color: var(--text-muted);
  background: var(--bg-panel-strong);
  transition: all 0.2s ease;
}
.config-panel__close:hover {
  color: var(--accent-success);
  background: color-mix(in srgb, var(--accent-success) 15%, transparent);
}

/* -- Creation badge -- */
.creation-badge {
  display: inline-flex;
  align-items: center;
  gap: 0.25rem;
  padding: 0.125rem 0.5rem;
  border-radius: 9999px;
  font-size: 0.6rem;
  font-weight: 700;
  letter-spacing: 0.04em;
  color: var(--accent-success);
  background: color-mix(in srgb, var(--accent-success) 12%, transparent);
  border: 1px solid color-mix(in srgb, var(--accent-success) 25%, transparent);
}
.creation-badge__dot {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: var(--accent-success);
  animation: pulse-dot 1.5s ease-in-out infinite;
}
@keyframes pulse-dot {
  0%, 100% { opacity: 1; transform: scale(1); }
  50% { opacity: 0.4; transform: scale(0.7); }
}

/* -- Body -- */
.config-panel__body {
  flex: 1;
  display: flex;
  overflow: hidden;
}

/* -- Sidebar area -- */
.sidebar-area {
  display: flex;
  flex-direction: column;
  flex-shrink: 0;
}

/* -- Sidebar bottom creation indicator -- */
.creation-indicator {
  display: flex;
  align-items: center;
  gap: 0.375rem;
  padding: 0.5rem 0.75rem;
  border-right: 1px solid var(--border-default);
  border-top: 1px dashed color-mix(in srgb, var(--accent-success) 30%, var(--border-default));
  font-size: 0.65rem;
  font-weight: 600;
  color: var(--accent-success);
  background: color-mix(in srgb, var(--accent-success) 5%, transparent);
}
.creation-indicator__dot {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: var(--accent-success);
  animation: pulse-dot 1.5s ease-in-out infinite;
}
.creation-indicator__text {
  letter-spacing: 0.03em;
}

/* -- Config content area -- */
.config-content {
  flex: 1;
  overflow-y: auto;
  padding: 0.75rem;
}

/* -- Validation banner -- */
.validation-banner {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  padding: 0.5rem 0.75rem;
  margin-bottom: 0.75rem;
  border-radius: 0.375rem;
  font-size: 0.7rem;
  font-weight: 600;
  color: var(--accent-danger);
  background: color-mix(in srgb, var(--accent-danger) 8%, transparent);
  border: 1px solid color-mix(in srgb, var(--accent-danger) 20%, transparent);
}
.validation-banner__icon {
  width: 1rem;
  height: 1rem;
  flex-shrink: 0;
}

/* -- Config section -- */
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
.section-subtitle {
  display: inline;
  font-size: 0.6rem;
  font-weight: 400;
  color: var(--text-muted);
  text-transform: none;
  letter-spacing: normal;
  margin-left: 0.5rem;
}

/* -- Form grid -- */
.form-grid {
  display: grid;
  grid-template-columns: repeat(2, 1fr);
  gap: 0.5rem 0.625rem;
}

/* -- Auto-connect switch -- */
.config-field--toggle {
  grid-column: span 2;
  display: flex;
  flex-direction: row;
  align-items: center;
  justify-content: space-between;
  padding: 0.375rem 0;
}
.config-field__label {
  font-size: 0.7rem;
  font-weight: 600;
  color: var(--text-muted);
}


/* -- Axis matrix -- */
.axis-matrix {
  display: grid;
  grid-template-columns: repeat(2, 1fr);
  gap: 0.625rem;
}

/* -- Footer actions -- */
.config-panel__footer {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0.75rem 1rem;
  border-top: 1px solid var(--border-default);
}
.config-panel__footer-left,
.config-panel__footer-right {
  display: flex;
  align-items: center;
  gap: 0.5rem;
}

/* -- Dirty state indicator -- */
.dirty-indicator {
  display: inline-flex;
  align-items: center;
  gap: 0.3rem;
  font-size: 0.65rem;
  font-weight: 600;
  color: color-mix(in srgb, var(--accent-danger) 80%, var(--text-muted));
}
.dirty-indicator__dot {
  width: 5px;
  height: 5px;
  border-radius: 50%;
  background: var(--accent-danger);
  animation: pulse-dot 2s ease-in-out infinite;
}

/* -- Tooltip overlay -- */
.field-tooltip {
  position: fixed;
  z-index: 9999;
  padding: 0.375rem 0.625rem;
  border-radius: 0.25rem;
  font-size: 0.65rem;
  font-weight: 500;
  color: var(--text-primary);
  background: color-mix(in srgb, var(--bg-app) 95%, transparent);
  border: 1px solid var(--border-default);
  pointer-events: none;
  white-space: nowrap;
  box-shadow: 0 4px 12px rgba(0, 0, 0, 0.3);
}
</style>
