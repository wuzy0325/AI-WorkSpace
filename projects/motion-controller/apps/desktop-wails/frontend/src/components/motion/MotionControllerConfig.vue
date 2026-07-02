<script setup lang="ts">
import { computed, reactive, onMounted, watch, ref, provide } from 'vue'
import { useMotionStore } from '@stores/motionStore'
import { useFeedbackStore } from '@stores/feedbackStore'
import type { MotionControllerProfile, AxisConfig, AxisEncoderCompensationConfig } from '@shared/types/motion'

import UiButton from '@components/ui/UiButton.vue'
import UiSelect, { type UiSelectOption } from '@components/ui/UiSelect.vue'
import UiInput from '@components/ui/UiInput.vue'
import UiToggle from '@components/ui/UiToggle.vue'

import { DEFAULT_AXIS_NAMES, createDefaultAxis, defaultEncComp, validateEncoderCompensation, normalizePositive, DEFAULT_ENCODER_SCALE } from './motionConfigEditor'
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

function defaultPortForType(type: string): number {
  if (type === 'B140-MC') return 23
  if (type === 'WTNMC4A-MC') return 5000
  return DEFAULT_PORT
}

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

const IPV4_REGEX = /^(?:(?:25[0-5]|2[0-4]\d|[01]?\d\d?)\.){3}(?:25[0-5]|2[0-4]\d|[01]?\d\d?)$/
const MAX_NAME_LENGTH = 64

const fieldErrors = computed<FieldErrors>(() => {
  const errors: FieldErrors = {}
  // name is required
  if (!editing.name.trim()) {
    errors.name = '控制器名称不能为空'
  } else if (editing.name.trim().length > MAX_NAME_LENGTH) {
    errors.name = `名称不能超过${MAX_NAME_LENGTH}个字符`
  }
  // name must be unique
  const duplicate = motion.profiles.find(p => p.name.trim() === editing.name.trim() && p.id !== editing.id)
  if (duplicate) {
    errors.name = '已存在同名控制器'
  }
  // address validation
  const trimmedAddr = editing.address.trim()
  if (!trimmedAddr) {
    errors.address = 'IP 地址不能为空'
  } else if (!IPV4_REGEX.test(trimmedAddr) && !/^[a-zA-Z0-9]([a-zA-Z0-9.-]*[a-zA-Z0-9])?$/.test(trimmedAddr)) {
    errors.address = '请输入有效的 IP 地址或主机名'
  }
  // port range check
  if (!Number.isFinite(editing.port) || editing.port < 1 || editing.port > MAX_PORT) {
    errors.port = `端口范围 1-${MAX_PORT}`
  }
  return errors
})

// 编码器补偿 error 级告警：与 wind-daq 行为对齐，保存时阻断。
// 仅收集 severity==='error'（物理不可能，如容差小于编码器分辨率）；warning 级由
// EncoderCompensationEditor 内联展示，不阻断保存。
const compensationErrors = computed<string[]>(() => {
  const errors: string[] = []
  for (const axis of editing.axes) {
    if (axis.positionSource !== 'encoder') continue
    const comp = axis.encoderCompensation
    if (!comp?.enabled) continue
    const warns = validateEncoderCompensation({ ...comp, enabled: true }, axis)
    for (const w of warns) {
      if (w.severity === 'error') errors.push(`轴 ${axis.name}：${w.message}`)
    }
  }
  return errors
})

const validationErrorCount = computed(() =>
  Object.values(fieldErrors.value).filter(Boolean).length + compensationErrors.value.length
)

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
    enabled: true,
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
        name: a.name, enabled: true, kind: a.kind ?? (a.name === 'U' ? 'ROTARY' as const : 'LINEAR' as const),
        maxSpeed: a.maxSpeed, minLimit: a.minLimit, maxLimit: a.maxLimit,
        stepsPerRev: a.stepsPerRev, microSteps: a.microSteps,
        lead: a.lead, gearRatio: a.gearRatio,
        inverted: a.inverted, encoderInverted: a.encoderInverted,
        positionSource: a.positionSource, encoderScale: normalizePositive(a.encoderScale, DEFAULT_ENCODER_SCALE),
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

function cancelAndClose(): void {
  emit('close')
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

watch(() => editing.type, (type, oldType) => {
  if (!oldType || type === oldType) return
  if (editing.port === defaultPortForType(oldType)) {
    editing.port = defaultPortForType(type)
  }
})

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

function onUpdateEncoderScale(index: number, value: number): void {
  editing.axes[index].encoderScale = value
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
          <div v-show="open" class="config-panel" :class="{ 'config-panel--creating': isCreatingNew }" @click.stop>
            <!-- 面板头部 -->
            <header class="config-panel__header" :class="{ 'config-panel__header--creating': isCreatingNew }">
              <div class="config-panel__header-left">
                <div class="config-panel__title-row">
                  <!-- 新建态：加号图标；编辑态：控制器图标 -->
                  <svg v-if="isCreatingNew" class="config-panel__title-icon config-panel__title-icon--creating" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round">
                    <circle cx="12" cy="12" r="10" />
                    <line x1="12" y1="8" x2="12" y2="16" />
                    <line x1="8" y1="12" x2="16" y2="12" />
                  </svg>
                  <svg v-else class="config-panel__title-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                    <rect x="2" y="2" width="20" height="8" rx="2" /><rect x="2" y="14" width="20" height="8" rx="2" /><circle cx="6" cy="6" r="1" /><circle cx="6" cy="18" r="1" />
                  </svg>
                  <h2 class="config-panel__title">{{ isEdit ? editing.name : '新建运动控制器' }}</h2>
                  <span v-if="isCreatingNew" class="creation-badge">
                    <span class="creation-badge__dot"></span>
                    新建中 · 尚未保存
                  </span>
                </div>
                <p class="config-panel__subtitle">
                  <template v-if="isCreatingNew">填写下方表单并点击「创建控制器」以新增一条配置</template>
                  <template v-else>编辑现有控制器配置 · ID {{ editing.id.slice(0, 8) }}</template>
                </p>
              </div>
              <button class="config-panel__close" @click="tryClose">
                <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                  <path d="M18 6L6 18M6 6l12 12"/>
                </svg>
              </button>
            </header>

            <!-- 面板主体 -->
            <div class="config-panel__body">
              <!-- 侧边栏 -->
              <div class="sidebar-area">
                <ProfileSidebar
                  :profiles="motion.profiles"
                  :active-id="editing.id"
                  :creating="isCreatingNew"
                  :draft-name="editing.name"
                  :draft-type="editing.type"
                  @select="onProfileSelect"
                  @add="onProfileAdd"
                />
                <div v-if="isCreatingNew" class="creation-indicator">
                  <span class="creation-indicator__dot"></span>
                  <span class="creation-indicator__text">新建中</span>
                </div>
              </div>

              <!-- 配置内容区 -->
              <main class="config-content custom-scrollbar">
                <!-- 验证错误横幅 -->
                <div v-if="validationErrorCount > 0" class="validation-banner">
                  <svg class="validation-banner__icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                    <circle cx="12" cy="12" r="10"/><line x1="12" y1="8" x2="12" y2="12"/><line x1="12" y1="16" x2="12.01" y2="16"/>
                  </svg>
                  <div class="validation-banner__body">
                    <span>{{ validationErrorCount }} 项验证错误需要修正</span>
                    <ul v-if="compensationErrors.length > 0" class="validation-banner__list">
                      <li v-for="(e, i) in compensationErrors" :key="i">{{ e }}</li>
                    </ul>
                  </div>
                </div>

                <!-- 基本信息区域 -->
                <section class="config-section">
                  <h3 class="config-section__title">基本信息</h3>
                  <div class="form-grid">
                    <div class="form-field">
                      <label class="form-field__label">名称</label>
                      <UiInput v-model="editing.name" placeholder="控制器名称" :error="fieldErrors.name" compact />
                    </div>
                    <div class="form-field">
                      <label class="form-field__label">类型</label>
                      <UiSelect v-model="editing.type" :options="controllerTypeOptions" compact />
                    </div>
                    <div class="form-field">
                      <label class="form-field__label">地址</label>
                      <UiInput v-model="editing.address" placeholder="127.0.0.1" :error="fieldErrors.address" compact />
                    </div>
                    <div class="form-field">
                      <label class="form-field__label">端口</label>
                      <UiInput v-model="editing.port" type="number" :min="1" :max="65535" :step="1" :error="fieldErrors.port" compact />
                    </div>
                    <div class="form-field form-field--toggle">
                      <label class="form-field__label">自动连接</label>
                      <UiToggle v-model="editing.autoConnect" />
                    </div>
                  </div>
                </section>

                <!-- 轴配置区域 -->
                <section class="config-section">
                  <h3 class="config-section__title">轴配置</h3>
                  <div class="axis-matrix">
                    <AxisConfigCard
                      v-for="(axis, index) in editing.axes"
                      :key="axis.name"
                      :axis="axis"
                      :index="index"
                      @update="onAxisUpdate"
                    />
                  </div>
                </section>

                <!-- 编码器补偿区域 -->
                <section class="config-section config-section--last">
                  <EncoderCompensationEditor
                    :axes="editing.axes"
                    :controller-type="editing.type"
                    @update-enc-comp="onUpdateEncComp"
                    @update-encoder-scale="onUpdateEncoderScale"
                  />
                </section>
              </main>
            </div>

            <!-- 底部操作栏 -->
            <footer class="config-panel__footer" :class="{ 'config-panel__footer--creating': isCreatingNew }">
              <div class="config-panel__footer-left">
                <UiButton v-if="isEdit" variant="danger" size="sm" @click="remove(editing.id)">
                  删除
                </UiButton>
                <span v-if="isDirty" class="dirty-indicator">
                  <span class="dirty-indicator__dot"></span>
                  未保存
                </span>
              </div>
              <div class="config-panel__footer-right">
                <UiButton variant="secondary" size="sm" @click="cancelAndClose">
                  取消
                </UiButton>
                <UiButton
                  variant="primary"
                  size="sm"
                  :loading="saving"
                  :disabled="validationErrorCount > 0"
                  @click="save"
                >
                  {{ isCreatingNew ? '创建控制器' : '保存' }}
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
/* ============================================================
   遮罩层
   ============================================================ */
.config-overlay {
  position: fixed;
  inset: 0;
  z-index: 200;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: var(--space-5);
  background: rgba(0, 0, 0, 0.5);
  backdrop-filter: blur(4px);
}

/* ============================================================
   配置面板容器
   ============================================================ */
.config-panel {
  width: 100%;
  max-width: 900px;
  max-height: 600px;
  display: flex;
  flex-direction: column;
  border-radius: var(--radius-xl);
  background: var(--bg-panel);
  border: 1px solid var(--border-default);
  box-shadow: var(--shadow-panel);
  outline: none;
  overflow: hidden;
  transition: border-color var(--motion-medium) var(--easing-standard),
              box-shadow var(--motion-medium) var(--easing-standard);
}

/* 新建态：弹窗整体加 success 主题色描边和外发光，强化"正在创建"的感知 */
.config-panel--creating {
  border-color: color-mix(in srgb, var(--accent-success) 55%, var(--border-default));
  box-shadow: var(--shadow-panel),
              0 0 0 1px color-mix(in srgb, var(--accent-success) 35%, transparent),
              0 12px 40px -8px color-mix(in srgb, var(--accent-success) 35%, transparent);
}

/* ============================================================
   面板头部
   ============================================================ */
.config-panel__header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: var(--space-3) var(--space-4);
  border-bottom: 1px solid var(--border-default);
  background: var(--bg-panel);
  flex-shrink: 0;
  position: relative;
  transition: background var(--motion-medium) var(--easing-standard);
}

/* 新建态：头部渐变背景 + 顶部主题色高亮条，营造"全新画布"的氛围 */
.config-panel__header--creating {
  background: linear-gradient(
    180deg,
    color-mix(in srgb, var(--accent-success) 14%, var(--bg-panel)) 0%,
    var(--bg-panel) 100%
  );
}

.config-panel__header--creating::before {
  content: '';
  position: absolute;
  top: 0;
  left: 0;
  right: 0;
  height: 2px;
  background: linear-gradient(
    90deg,
    transparent 0%,
    var(--accent-success) 20%,
    var(--accent-success) 80%,
    transparent 100%
  );
}

.config-panel__title-icon--creating {
  width: 1.125rem;
  height: 1.125rem;
  color: var(--accent-success);
  filter: drop-shadow(0 0 6px color-mix(in srgb, var(--accent-success) 60%, transparent));
}

.config-panel__header-left {
  display: flex;
  flex-direction: column;
  gap: var(--space-0-5);
}

.config-panel__title-row {
  display: flex;
  align-items: center;
  gap: var(--space-2);
}

.config-panel__title-icon {
  width: 1rem;
  height: 1rem;
  color: var(--accent-success);
  flex-shrink: 0;
}

.config-panel__title {
  font-size: 0.875rem;
  font-weight: 700;
  color: var(--text-primary);
}

.config-panel__subtitle {
  font-size: 0.6875rem;
  color: var(--text-muted);
  padding-left: 1.5rem;
}

/* 新建态：副标题加重颜色，作为操作提示 */
.config-panel__header--creating .config-panel__subtitle {
  color: var(--accent-success);
  font-weight: 600;
}

.config-panel__close {
  width: 1.75rem;
  height: 1.75rem;
  display: flex;
  align-items: center;
  justify-content: center;
  border-radius: var(--radius-md);
  color: var(--text-muted);
  background: var(--bg-panel-strong);
  border: 1px solid var(--border-default);
  cursor: pointer;
  transition: all var(--motion-fast) var(--easing-standard);
}

.config-panel__close:hover {
  color: var(--accent-danger);
  background: color-mix(in srgb, var(--accent-danger) 10%, transparent);
  border-color: color-mix(in srgb, var(--accent-danger) 30%, transparent);
}

/* ============================================================
   新建状态徽章
   ============================================================ */
.creation-badge {
  display: inline-flex;
  align-items: center;
  gap: var(--space-1);
  padding: var(--space-0-5) var(--space-2);
  border-radius: var(--radius-pill);
  font-size: 0.625rem;
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

/* ============================================================
   面板主体
   ============================================================ */
.config-panel__body {
  flex: 1;
  display: flex;
  overflow: hidden;
  min-height: 0;
}

/* ============================================================
   侧边栏区域
   ============================================================ */
.sidebar-area {
  display: flex;
  flex-direction: column;
  flex-shrink: 0;
  width: 12rem;
  border-right: 1px solid var(--border-default);
  background: var(--bg-panel-strong);
}

/* 侧边栏底部新建指示器 */
.creation-indicator {
  display: flex;
  align-items: center;
  gap: var(--space-1-5);
  padding: var(--space-2) var(--space-3);
  border-top: 1px dashed color-mix(in srgb, var(--accent-success) 30%, var(--border-default));
  font-size: 0.75rem;
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

/* ============================================================
   配置内容区
   ============================================================ */
.config-content {
  flex: 1;
  overflow-y: auto;
  padding: var(--space-4);
  display: flex;
  flex-direction: column;
  gap: var(--space-5);
}

/* ============================================================
   验证错误横幅
   ============================================================ */
.validation-banner {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  padding: var(--space-2) var(--space-3);
  border-radius: var(--radius-md);
  font-size: 0.75rem;
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

.validation-banner__body {
  display: flex;
  flex-direction: column;
  gap: var(--space-1);
}

.validation-banner__list {
  margin: 0;
  padding-left: var(--space-4);
  font-weight: 500;
  line-height: 1.4;
}

/* ============================================================
   配置区域
   ============================================================ */
.config-section {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
}

.config-section--last {
  margin-bottom: 0;
}

.config-section__title {
  font-size: 0.6875rem;
  font-weight: 700;
  color: var(--text-muted);
  letter-spacing: 0.06em;
  text-transform: uppercase;
  margin: 0;
}

.section-subtitle {
  display: inline;
  font-size: 0.625rem;
  font-weight: 400;
  color: var(--text-muted);
  text-transform: none;
  letter-spacing: normal;
  margin-left: var(--space-2);
}

/* ============================================================
   表单网格
   ============================================================ */
.form-grid {
  display: grid;
  grid-template-columns: repeat(3, 1fr);
  gap: var(--space-3) var(--space-4);
}

@media (min-width: 768px) {
  .form-grid {
    grid-template-columns: repeat(5, 1fr);
  }
}

.form-field {
  display: flex;
  flex-direction: column;
  gap: var(--space-1);
}

.form-field__label {
  font-size: 0.6875rem;
  font-weight: 600;
  color: var(--text-muted);
  white-space: nowrap;
}

.form-field--toggle {
  flex-direction: row;
  align-items: center;
  justify-content: space-between;
  gap: var(--space-2);
}

.form-field--toggle .form-field__label {
  margin: 0;
}

/* ============================================================
   轴配置矩阵
   ============================================================ */
.axis-matrix {
  display: grid;
  grid-template-columns: repeat(1, 1fr);
  gap: var(--space-3);
}

@media (min-width: 640px) {
  .axis-matrix {
    grid-template-columns: repeat(2, 1fr);
  }
}

/* ============================================================
   底部操作栏
   ============================================================ */
.config-panel__footer {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: var(--space-3) var(--space-4);
  border-top: 1px solid var(--border-default);
  background: var(--bg-panel);
  flex-shrink: 0;
  gap: var(--space-3);
  transition: background var(--motion-medium) var(--easing-standard);
}

/* 新建态：底部轻染色，与头部呼应 */
.config-panel__footer--creating {
  background: linear-gradient(
    0deg,
    color-mix(in srgb, var(--accent-success) 8%, var(--bg-panel)) 0%,
    var(--bg-panel) 100%
  );
  border-top-color: color-mix(in srgb, var(--accent-success) 25%, var(--border-default));
}

.config-panel__footer-left,
.config-panel__footer-right {
  display: flex;
  align-items: center;
  gap: var(--space-3);
}

/* ============================================================
   未保存状态指示器
   ============================================================ */
.dirty-indicator {
  display: inline-flex;
  align-items: center;
  gap: var(--space-1-5);
  font-size: 0.75rem;
  font-weight: 600;
  color: var(--accent-danger);
}

.dirty-indicator__dot {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: var(--accent-danger);
  animation: pulse-dot 2s ease-in-out infinite;
}

/* ============================================================
   Tooltip 覆盖层
   ============================================================ */
.field-tooltip {
  position: fixed;
  z-index: 9999;
  padding: var(--space-2) var(--space-3);
  border-radius: var(--radius-md);
  font-size: 0.75rem;
  font-weight: 500;
  color: var(--text-primary);
  background: var(--bg-app);
  border: 1px solid var(--border-default);
  pointer-events: none;
  white-space: nowrap;
  box-shadow: var(--shadow-panel);
}
</style>
