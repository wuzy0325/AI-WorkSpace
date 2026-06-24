<script setup lang="ts">
import { computed, reactive, onMounted, watch, ref } from 'vue';
import { NButton } from 'naive-ui';
import { useMotionStore } from '@stores/motionStore';
import { useFeedbackStore } from '@stores/feedbackStore';
import type { MotionControllerProfile, AxisConfig, AxisName, PositionSource } from '@shared/types/motion';
import {
  DEFAULT_AXIS_NAMES,
  DEFAULT_STEPS_PER_REV,
  DEFAULT_MICRO_STEPS,
  DEFAULT_LEAD,
  DEFAULT_GEAR_RATIO,
  DEFAULT_MAX_SPEED,
  DEFAULT_ENCODER_SCALE,
  DEFAULT_ENCODER_COMPENSATION_TOLERANCE,
  DEFAULT_ENCODER_COMPENSATION_MAX_CYCLES,
  DEFAULT_ENCODER_COMPENSATION_SETTLE_MS,
  DEFAULT_ENCODER_COMPENSATION_MIN_STEP,
  DEFAULT_ENCODER_COMPENSATION_TIMEOUT_MS,
  createDefaultAxis,
  createDefaultEncoderCompensation,
  normalizeAxisForEditing,
  normalizePositive,
} from './motionConfigEditor';
import ProfileSidebar from './ProfileSidebar.vue';
import AxisConfigCard from './AxisConfigCard.vue';

const props = defineProps<{ open: boolean; currentId?: string | null }>();
const emit = defineEmits<{ (e: 'close'): void }>();

const motion = useMotionStore();
const feedback = useFeedbackStore();

/** 补偿超时验证的缓冲时间(ms)，用于计算最小超时 = maxCycles × settleMs + 缓冲 */
const COMPENSATION_TIMEOUT_BUFFER_MS = 500;

const editing = reactive<MotionControllerProfile>({
  id: '',
  name: '',
  type: 'SIMULATED-MC',
  address: '127.0.0.1',
  port: 9000,
  autoConnect: false,
  axes: DEFAULT_AXIS_NAMES.map((name) => normalizeAxisForEditing(createDefaultAxis(name))),
});

const isEdit = computed(() => !!editing.id);
const validationErrors = ref<string[]>([]);
const isCreatingNew = ref(false);

function defaultPortForType(type: string): number {
  if (type === 'B140-MC') return 23;
  if (type === 'WTNMC4A-MC') return 5000;
  return 9000;
}

function newProfile(): void {
  isCreatingNew.value = true;
  validationErrors.value = [];
  editing.id = '';
  editing.name = '模拟控制器';
  editing.type = 'SIMULATED-MC';
  editing.address = '127.0.0.1';
  editing.port = 9000;
  editing.autoConnect = false;
  editing.axes = DEFAULT_AXIS_NAMES.map((name) => normalizeAxisForEditing(createDefaultAxis(name)));
}

function editProfile(src: MotionControllerProfile): void {
  isCreatingNew.value = false;
  validationErrors.value = [];
  editing.id = src.id;
  editing.name = src.name;
  editing.type = src.type;
  editing.address = src.address;
  editing.port = src.port;
  editing.autoConnect = src.autoConnect;
  editing.axes = src.axes.map((a) => normalizeAxisForEditing(a));
}

function validateEncoderCompensation(): string[] {
  const errors: string[] = [];

  for (const axis of editing.axes) {
    if (!axis.enabled || axis.positionSource !== 'encoder') continue;

    const comp = axis.encoderCompensation;
    if (!comp?.enabled) continue;

    const tolerance = comp.tolerance ?? DEFAULT_ENCODER_COMPENSATION_TOLERANCE;
    const minStep = comp.minStep ?? DEFAULT_ENCODER_COMPENSATION_MIN_STEP;
    const maxCycles = comp.maxCycles ?? DEFAULT_ENCODER_COMPENSATION_MAX_CYCLES;
    const settleMs = comp.settleMs ?? DEFAULT_ENCODER_COMPENSATION_SETTLE_MS;
    const timeoutMs = comp.timeoutMs ?? DEFAULT_ENCODER_COMPENSATION_TIMEOUT_MS;

    // 关键约束：最小步长必须小于容差
    if (minStep >= tolerance) {
      errors.push(
        `轴 ${axis.name}：最小白 (${minStep}) 必须小于容差 (${tolerance})`
      );
    }

    // 建议：最小步长应小于电机一个微步的位移
    const stepsPerRev = axis.stepsPerRev ?? DEFAULT_STEPS_PER_REV;
    const microSteps = axis.microSteps ?? DEFAULT_MICRO_STEPS;
    const stepsPerMicrostep = (360 / stepsPerRev) * microSteps;

    let microstepSize: number;
    if (axis.kind === 'ROTARY') {
      const gearRatio = axis.gearRatio ?? DEFAULT_GEAR_RATIO;
      microstepSize = 360 / (stepsPerMicrostep * gearRatio);
    } else {
      const lead = axis.lead ?? DEFAULT_LEAD;
      microstepSize = lead / stepsPerMicrostep;
    }

    if (minStep > microstepSize) {
      errors.push(
        `轴 ${axis.name}：最小白 (${minStep}) 大于单个微步位移 (${microstepSize.toFixed(4)})，建议调小`
      );
    }

    // 超时时间应足够完成所有补偿循环
    const minTimeout = maxCycles * settleMs + COMPENSATION_TIMEOUT_BUFFER_MS;
    if (timeoutMs < minTimeout) {
      errors.push(
        `轴 ${axis.name}：超时时间 (${timeoutMs}ms) 应不小于 ${minTimeout}ms`
      );
    }
  }

  return errors;
}

async function save(): Promise<void> {
  const errors = validateEncoderCompensation();
  if (errors.length > 0) {
    validationErrors.value = errors;
    return;
  }
  validationErrors.value = [];

  const profile: MotionControllerProfile = {
    id: editing.id || (crypto.randomUUID?.() ?? `${Date.now()}-${Math.random().toString(36).slice(2, 8)}`),
    name: editing.name.trim() || '新控制器',
    type: editing.type,
    address: editing.address.trim() || '127.0.0.1',
    port: Number.isFinite(editing.port) ? editing.port : 9000,
    autoConnect: editing.autoConnect,
    axes: editing.axes.map((a) => ({
      name: a.name,
      enabled: a.enabled,
      kind: a.kind ?? (a.name === 'U' ? 'ROTARY' : 'LINEAR'),
      maxSpeed: normalizePositive(a.maxSpeed, DEFAULT_MAX_SPEED),
      minLimit: a.minLimit,
      maxLimit: a.maxLimit,
      inverted: a.inverted ?? false,
      encoderInverted: a.encoderInverted ?? a.inverted ?? false,
      stepsPerRev: normalizePositive(a.stepsPerRev, DEFAULT_STEPS_PER_REV),
      microSteps: normalizePositive(a.microSteps, DEFAULT_MICRO_STEPS),
      lead: normalizePositive(a.lead, DEFAULT_LEAD),
      gearRatio: normalizePositive(a.gearRatio, DEFAULT_GEAR_RATIO),
      positionSource: (a.positionSource ?? 'register') as PositionSource,
      encoderScale: normalizePositive(a.encoderScale, DEFAULT_ENCODER_SCALE),
      encoderCompensation: {
        enabled: a.encoderCompensation?.enabled ?? false,
        tolerance: normalizePositive(a.encoderCompensation?.tolerance, DEFAULT_ENCODER_COMPENSATION_TOLERANCE),
        maxCycles: normalizePositive(a.encoderCompensation?.maxCycles, DEFAULT_ENCODER_COMPENSATION_MAX_CYCLES),
        settleMs: normalizePositive(a.encoderCompensation?.settleMs, DEFAULT_ENCODER_COMPENSATION_SETTLE_MS),
        minStep: normalizePositive(a.encoderCompensation?.minStep, DEFAULT_ENCODER_COMPENSATION_MIN_STEP),
        timeoutMs: normalizePositive(a.encoderCompensation?.timeoutMs, DEFAULT_ENCODER_COMPENSATION_TIMEOUT_MS),
      },
    })),
  };

  try {
    await motion.upsertProfile(profile);
  } catch (e) {
    feedback.pushToast(`保存配置失败: ${e instanceof Error ? e.message : '未知错误'}`, 'error');
    return;
  }
  feedback.pushToast('控制器配置已保存', 'success');
  emit('close');
}

async function remove(id: string): Promise<void> {
  try {
    await motion.deleteProfile(id);
    feedback.pushToast('控制器已删除', 'info');
    emit('close');
  } catch (e) {
    feedback.pushToast(`删除失败: ${e instanceof Error ? e.message : '未知错误'}`, 'error');
  }
}

function ensureEditingOnOpen(): void {
  if (!props.open) return;
  if (props.currentId) {
    const target = motion.profiles.find((p) => p.id === props.currentId);
    if (target) {
      editProfile(target);
      return;
    }
  }
  if (!editing.id && motion.profiles.length > 0) {
    editProfile(motion.profiles[0]);
    return;
  }
  newProfile();
}

onMounted(async () => {
  await motion.refreshProfiles();
  ensureEditingOnOpen();
});

watch(() => props.open, (v) => {
  if (v) ensureEditingOnOpen();
  else isCreatingNew.value = false;
});

watch(() => editing.type, (type, oldType) => {
  if (!oldType || type === oldType) return;
  if (editing.port === defaultPortForType(oldType)) {
    editing.port = defaultPortForType(type);
  }
});

function onAxisUpdate(index: number, axis: AxisConfig): void {
  // 必须使用 splice 触发 Vue 响应式更新，直接按索引赋值不会被追踪
  editing.axes.splice(index, 1, axis);
}

const axisIndices = computed(() => editing.axes.map((_, i) => i));

const controllerTypeOptions = [
  { value: 'SIMULATED-MC', label: '模拟控制器' },
  { value: 'B140-MC', label: 'B140 控制器' },
  { value: 'WTNMC4A-MC', label: 'WTNMC4A 控制器' },
];
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
      <div v-show="open" class="config-overlay" @click="emit('close')">
        <Transition
          enter-active-class="transition ease-out duration-300"
          enter-from-class="opacity-0 scale-95 translate-y-4"
          enter-to-class="opacity-100 scale-100 translate-y-0"
          leave-active-class="transition ease-in duration-200"
          leave-from-class="opacity-100 scale-100 translate-y-0"
          leave-to-class="opacity-0 scale-95 translate-y-4"
        >
          <div v-show="open" class="config-panel" :class="{ 'config-panel--creating': isCreatingNew }" @click.stop>
            <!-- Header -->
            <header class="config-panel__header" :class="{ 'config-panel__header--creating': isCreatingNew }">
              <div class="flex items-center gap-3">
                <div class="config-panel__icon" :class="{ 'config-panel__icon--creating': isCreatingNew }">
                  <!-- 新建态：加号图标；编辑态：齿轮图标 -->
                  <svg v-if="isCreatingNew" class="w-5 h-5" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round">
                    <circle cx="12" cy="12" r="10"/>
                    <line x1="12" y1="8" x2="12" y2="16"/>
                    <line x1="8" y1="12" x2="16" y2="12"/>
                  </svg>
                  <svg v-else class="w-5 h-5" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                    <circle cx="12" cy="12" r="3"/>
                    <path d="M12 2v2M12 20v2M4.93 4.93l1.41 1.41M17.66 17.66l1.41 1.41M2 12h2M20 12h2M6.34 17.66l-1.41 1.41M19.07 4.93l-1.41 1.41"/>
                  </svg>
                </div>
                <div class="config-panel__title-block">
                  <div class="config-panel__title-row">
                    <h2 class="config-panel__title">{{ isEdit ? editing.name : '新建运动控制器' }}</h2>
                    <span v-if="isCreatingNew" class="creation-badge">
                      <span class="creation-badge__dot"></span>
                      新建中 · 尚未保存
                    </span>
                  </div>
                  <p class="config-panel__subtitle">
                    <template v-if="isCreatingNew">填写下方表单并点击「创建控制器」以新增一条配置</template>
                    <template v-else>编辑现有控制器配置</template>
                  </p>
                </div>
              </div>
              <NButton quaternary size="small" class="config-panel__close" @click="emit('close')">
                <template #icon>
                  <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                    <path d="M18 6L6 18M6 6l12 12"/>
                  </svg>
                </template>
              </NButton>
            </header>

            <!-- Main Content -->
            <div class="config-panel__body">
              <ProfileSidebar
                :profiles="motion.profiles"
                :active-id="editing.id"
                :creating="isCreatingNew"
                :draft-name="editing.name"
                :draft-type="editing.type"
                :draft-address="editing.address"
                :draft-port="editing.port"
                @select="(id: string) => { const p = motion.profiles.find(x => x.id === id); if (p) editProfile(p); }"
                @add="newProfile"
              />

              <main class="config-content">
                <!-- Basic Info -->
                <section class="config-section config-section--boxed">
                  <h3 class="config-section__title">
                    <svg class="w-4 h-4 inline-block mr-1.5 -mt-0.5" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M5 12h14"/><path d="m12 5 7 7-7 7"/></svg>
                    通信设置
                  </h3>
                  <div class="basic-info-grid">
                    <div class="basic-info-field">
                      <label class="basic-info-field__label">名称</label>
                      <input v-model="editing.name" type="text" placeholder="控制器名称" class="input-compact" />
                    </div>
                    <div class="basic-info-field">
                      <label class="basic-info-field__label">类型</label>
                      <select v-model="editing.type" class="input-compact input-compact--select">
                        <option v-for="opt in controllerTypeOptions" :key="opt.value" :value="opt.value">{{ opt.label }}</option>
                      </select>
                    </div>
                    <div class="basic-info-field basic-info-field--wide">
                      <label class="basic-info-field__label">地址</label>
                      <div class="flex gap-2">
                        <input v-model="editing.address" type="text" placeholder="127.0.0.1" class="input-compact flex-1" />
                        <input v-model.number="editing.port" type="number" placeholder="9000" min="1" max="65535" class="input-compact w-20 text-center" />
                      </div>
                    </div>
                    <div class="basic-info-field">
                      <label class="basic-info-field__label">自动连接</label>
                      <label class="auto-connect-toggle">
                        <input v-model="editing.autoConnect" type="checkbox" />
                        <span class="auto-connect-toggle__track">
                          <span class="auto-connect-toggle__thumb"></span>
                        </span>
                        <span class="auto-connect-toggle__label">{{ editing.autoConnect ? '启用' : '禁用' }}</span>
                      </label>
                    </div>
                  </div>
                </section>

                <!-- Axis Matrix -->
                <div class="config-section flex flex-col">
                  <h3 class="config-section__title">
                    <svg class="w-4 h-4 inline-block mr-1.5 -mt-0.5" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M12 2v4"/><path d="m16.2 7.8 2.9-2.9"/><path d="M18 12h4"/><path d="m16.2 16.2 2.9 2.9"/><path d="M12 18v4"/><path d="m4.9 19.1 2.9-2.9"/><path d="M2 12h4"/><path d="m4.9 4.9 2.9 2.9"/></svg>
                    轴配置
                    <span class="section-subtitle">配置每个轴的机械和电气参数</span>
                  </h3>
                  <div class="axis-matrix">
                    <AxisConfigCard
                      v-for="idx in axisIndices"
                      :key="editing.axes[idx].name"
                      :axis="editing.axes[idx]"
                      :index="idx"
                      :controller-type="editing.type"
                      @update="onAxisUpdate"
                    />
                  </div>
                </div>
              </main>
            </div>

            <!-- Footer -->
            <footer class="config-panel__footer" :class="{ 'config-panel__footer--creating': isCreatingNew }">
              <div class="config-panel__footer-left">
                <span class="config-status" :class="isCreatingNew ? 'config-status--new' : 'config-status--edit'">
                  <span v-if="isCreatingNew" class="config-status__dot"></span>
                  {{ isCreatingNew ? '新建配置 · 保存后生效' : '编辑现有配置' }}
                </span>
              </div>
              <div class="config-panel__footer-right">
                <NButton v-if="isEdit" type="error" size="tiny" class="config-panel__delete-btn" @click="remove(editing.id)">删除</NButton>
                <NButton v-if="isEdit" type="primary" size="tiny" class="config-panel__new-btn" @click="newProfile">新建</NButton>
                <NButton quaternary size="tiny" class="config-panel__cancel-btn" @click="emit('close')">取消</NButton>
                <NButton type="primary" size="tiny" class="config-panel__save-btn" @click="save">
                  {{ isCreatingNew ? '创建控制器' : '保存' }}
                </NButton>
              </div>
            </footer>

            <!-- 验证错误提示 -->
            <div v-if="validationErrors.length > 0" class="validation-errors">
              <div class="validation-errors__title">参数验证失败</div>
              <ul class="validation-errors__list">
                <li v-for="(err, idx) in validationErrors" :key="idx" class="validation-errors__item">
                  <span class="validation-errors__dot">•</span>
                  <span>{{ err }}</span>
                </li>
              </ul>
            </div>
          </div>
        </Transition>
      </div>
    </Transition>
  </Teleport>
</template>

<style scoped>
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
.config-panel {
  width: 100%;
  max-width: 1100px;
  max-height: calc(100vh - var(--space-8));
  height: calc(100vh - var(--space-8));
  display: flex;
  flex-direction: column;
  border-radius: 1rem;
  background: var(--bg-panel);
  border: 1px solid var(--border-default);
  box-shadow: 0 32px 80px -24px rgba(0, 0, 0, 0.5);
  overflow: hidden;
  transition: border-color 0.25s ease, box-shadow 0.25s ease;
}

/* 新建态：弹窗整体加 success 主题色描边和外发光 */
.config-panel--creating {
  border-color: color-mix(in srgb, var(--accent-primary) 55%, var(--border-default));
  box-shadow:
    0 32px 80px -24px rgba(0, 0, 0, 0.5),
    0 0 0 1px color-mix(in srgb, var(--accent-primary) 35%, transparent),
    0 12px 40px -8px color-mix(in srgb, var(--accent-primary) 30%, transparent);
}
:root[data-theme='light'] .config-panel {
  background: rgba(255, 255, 255, 0.98);
  border: 1px solid rgba(0, 0, 0, 0.1);
}
.config-panel__header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 1rem 1.25rem;
  border-bottom: 1px solid var(--border-default);
  background: var(--bg-panel-strong);
  position: relative;
  transition: background 0.25s ease;
}

/* 新建态：头部主题色渐变背景 + 顶部高亮条 */
.config-panel__header--creating {
  background: linear-gradient(
    180deg,
    color-mix(in srgb, var(--accent-primary) 18%, var(--bg-panel-strong)) 0%,
    var(--bg-panel-strong) 100%
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
    var(--accent-primary) 20%,
    var(--accent-primary) 80%,
    transparent 100%
  );
}

.config-panel__title-block {
  display: flex;
  flex-direction: column;
  gap: 0.25rem;
}

.config-panel__title-row {
  display: flex;
  align-items: center;
  gap: 0.625rem;
}

/* 新建状态徽章 */
.creation-badge {
  display: inline-flex;
  align-items: center;
  gap: 0.25rem;
  padding: 0.125rem 0.5rem;
  border-radius: 9999px;
  font-size: 0.625rem;
  font-weight: 700;
  letter-spacing: 0.04em;
  color: var(--accent-primary);
  background: color-mix(in srgb, var(--accent-primary) 12%, transparent);
  border: 1px solid color-mix(in srgb, var(--accent-primary) 28%, transparent);
}

.creation-badge__dot {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: var(--accent-primary);
  animation: motion-cfg-pulse-dot 1.5s ease-in-out infinite;
}

@keyframes motion-cfg-pulse-dot {
  0%, 100% { opacity: 1; transform: scale(1); }
  50% { opacity: 0.4; transform: scale(0.7); }
}
.config-panel__icon {
  width: 2.5rem;
  height: 2.5rem;
  display: flex;
  align-items: center;
  justify-content: center;
  border-radius: 0.5rem;
  background: color-mix(in srgb, var(--accent-info) 10%, transparent);
  color: var(--accent-info);
  transition: background 0.25s ease, color 0.25s ease, box-shadow 0.25s ease;
}

/* 新建态：图标改为主题色高亮 */
.config-panel__icon--creating {
  background: color-mix(in srgb, var(--accent-primary) 18%, transparent);
  color: var(--accent-primary);
  box-shadow: 0 0 0 1px color-mix(in srgb, var(--accent-primary) 25%, transparent),
              0 0 16px -4px color-mix(in srgb, var(--accent-primary) 60%, transparent);
}
.config-panel__title {
  font-size: var(--font-size-lg);
  font-weight: 700;
  color: var(--text-primary);
}
.config-panel__subtitle {
  margin-top: 0.25rem;
  font-size: var(--font-size-xs);
  color: var(--text-muted);
}

/* 新建态：副标题加重颜色作为操作提示 */
.config-panel__header--creating .config-panel__subtitle {
  color: var(--accent-primary);
  font-weight: 600;
}
.config-panel__close {
  width: 2rem;
  height: 2rem;
  display: flex;
  align-items: center;
  justify-content: center;
  border-radius: 0.5rem;
  color: var(--text-muted);
  background: rgba(0, 0, 0, 0.2);
  transition: all 0.2s ease;
}
.config-panel__close:hover {
  color: var(--accent-danger);
  background: rgba(239, 68, 68, 0.15);
}

.config-panel__body {
  flex: 1;
  display: flex;
  overflow: hidden;
}
.config-content {
  flex: 1;
  overflow-y: auto;
  padding: 0.625rem 0.875rem;
  display: flex;
  flex-direction: column;
  gap: 0.625rem;
}
.config-content::-webkit-scrollbar {
  width: 4px;
}
.config-content::-webkit-scrollbar-track {
  background: transparent;
}
.config-content::-webkit-scrollbar-thumb {
  background: var(--border-default);
  border-radius: 2px;
}

.config-section {
  margin-bottom: 0;
}
.config-section--boxed {
  padding: 0.625rem 0.875rem;
  border-radius: var(--radius-md);
  background: var(--bg-panel-strong);
  border: 1px solid var(--border-default);
}
.config-section__title {
  font-size: var(--font-size-xs);
  font-weight: 700;
  color: var(--text-muted);
  letter-spacing: 0.05em;
  text-transform: uppercase;
  margin-bottom: 0.5rem;
}
.section-subtitle {
  display: inline;
  font-size: var(--font-size-2xs);
  font-weight: 400;
  color: var(--text-muted);
  text-transform: none;
  letter-spacing: normal;
  margin-left: 0.5rem;
}

.basic-info-grid {
  display: grid;
  grid-template-columns: repeat(12, 1fr);
  gap: 0.75rem;
}
.basic-info-field {
  display: flex;
  flex-direction: column;
  gap: 0.25rem;
}
.basic-info-field:nth-child(1) { grid-column: span 3; }
.basic-info-field:nth-child(2) { grid-column: span 3; }
.basic-info-field--wide { grid-column: span 4; }
.basic-info-field:nth-child(4) { grid-column: span 2; }
.basic-info-field__label {
  font-size: var(--font-size-2xs);
  font-weight: 600;
  color: var(--text-muted);
}

.input-compact {
  height: 1.75rem;
  padding: 0 0.5rem;
  border: 1px solid var(--border-default);
  border-radius: var(--radius-sm);
  background: var(--bg-canvas);
  color: var(--text-primary);
  font-size: var(--font-size-xs);
  outline: none;
  transition: border-color 0.2s ease, box-shadow 0.2s ease;
}
.input-compact:focus {
  border-color: var(--accent-primary);
  box-shadow: 0 0 0 2px var(--focus-ring-soft);
}
.input-compact--select {
  cursor: pointer;
}

.auto-connect-toggle {
  display: inline-flex;
  align-items: center;
  gap: 0.5rem;
  cursor: pointer;
}
.auto-connect-toggle input {
  display: none;
}
.auto-connect-toggle__track {
  width: 2rem;
  height: 1.125rem;
  border-radius: 9999px;
  background: rgba(0, 0, 0, 0.3);
  position: relative;
  transition: background 0.2s ease;
}
.auto-connect-toggle input:checked + .auto-connect-toggle__track {
  background: var(--accent-primary);
}
.auto-connect-toggle__thumb {
  position: absolute;
  left: 2px;
  top: 2px;
  width: calc(1.125rem - 4px);
  height: calc(1.125rem - 4px);
  border-radius: 50%;
  background: white;
  transition: transform 0.2s ease;
}
.auto-connect-toggle input:checked + .auto-connect-toggle__track .auto-connect-toggle__thumb {
  transform: translateX(0.875rem);
}
.auto-connect-toggle__label {
  font-size: var(--font-size-xs);
  color: var(--text-muted);
}

.axis-matrix {
  display: grid;
  grid-template-columns: repeat(2, 1fr);
  gap: 0.5rem;
}

.config-panel__footer {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0.875rem 1.25rem;
  border-top: 1px solid var(--border-default);
  background: var(--bg-panel-strong);
  transition: background 0.25s ease, border-top-color 0.25s ease;
}

/* 新建态：底部轻染色，与头部呼应 */
.config-panel__footer--creating {
  background: linear-gradient(
    0deg,
    color-mix(in srgb, var(--accent-primary) 10%, var(--bg-panel-strong)) 0%,
    var(--bg-panel-strong) 100%
  );
  border-top-color: color-mix(in srgb, var(--accent-primary) 25%, var(--border-default));
}
.config-panel__footer-left,
.config-panel__footer-right {
  display: flex;
  align-items: center;
  gap: 0.75rem;
}
.config-status {
  display: inline-flex;
  align-items: center;
  gap: 0.375rem;
  font-size: var(--font-size-xs);
  font-weight: 600;
}
.config-status--edit {
  color: var(--accent-primary);
}
.config-status--new {
  color: var(--accent-primary);
}
.config-status__dot {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: var(--accent-primary);
  animation: motion-cfg-pulse-dot 1.5s ease-in-out infinite;
}
.config-panel__delete-btn,
.config-panel__new-btn,
.config-panel__cancel-btn,
.config-panel__save-btn {
  padding: 0.5rem 1rem;
  border-radius: 0.5rem;
  font-size: var(--font-size-sm);
  font-weight: 600;
  transition: all 0.2s ease;
}
.config-panel__delete-btn {
  color: var(--accent-danger);
  background: rgba(239, 68, 68, 0.1);
  border: 1px solid rgba(239, 68, 68, 0.2);
}
.config-panel__delete-btn:hover {
  background: rgba(239, 68, 68, 0.2);
}
.config-panel__new-btn {
  color: var(--accent-primary);
  background: rgba(16, 185, 129, 0.1);
  border: 1px solid rgba(16, 185, 129, 0.2);
}
.config-panel__new-btn:hover {
  background: rgba(16, 185, 129, 0.2);
}
.config-panel__cancel-btn {
  color: var(--text-muted);
  background: rgba(0, 0, 0, 0.15);
  border: 1px solid rgba(255, 255, 255, 0.06);
}
.config-panel__cancel-btn:hover {
  color: var(--text-primary);
  background: rgba(0, 0, 0, 0.25);
}
.config-panel__save-btn {
  color: var(--color-brand-foreground);
  background: linear-gradient(135deg, var(--accent-primary), var(--accent-primary-core-strong));
  border: none;
}
.config-panel__save-btn:hover {
  box-shadow: 0 4px 16px rgba(16, 185, 129, 0.3);
}

.validation-errors {
  padding: 0.75rem 1.25rem;
  background: color-mix(in srgb, var(--accent-danger) 8%, transparent);
  border-top: 1px solid color-mix(in srgb, var(--accent-danger) 20%, transparent);
}
.validation-errors__title {
  font-size: var(--font-size-xs);
  font-weight: 700;
  color: var(--accent-danger);
  margin-bottom: 0.25rem;
}
.validation-errors__list {
  display: flex;
  flex-wrap: wrap;
  gap: 0.25rem 1rem;
  list-style: none;
  padding: 0;
  margin: 0;
}
.validation-errors__item {
  font-size: var(--font-size-2xs);
  color: var(--accent-danger);
  display: flex;
  align-items: flex-start;
  gap: 0.25rem;
}
.validation-errors__dot {
  flex-shrink: 0;
}
</style>
