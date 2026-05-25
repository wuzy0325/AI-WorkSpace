<script setup lang="ts">
import { computed, reactive, onMounted, watch } from 'vue';
import { useMotionStore } from '@stores/motionStore';
import { useI18nStore } from '@stores/i18nStore';
import { useFeedbackStore } from '@stores/feedbackStore';
import type { MotionControllerProfile, AxisName, AxisConfig, PositionSource } from '@shared/types/motion';

const props = defineProps<{
  open: boolean;
  currentId?: string | null;
}>();

const emit = defineEmits<{ (e: 'close'): void }>();

const motion = useMotionStore();
const i18n = useI18nStore();
const feedback = useFeedbackStore();

const DEFAULT_AXIS_NAMES: AxisName[] = ['X', 'Y', 'Z', 'U'];

const editing = reactive<MotionControllerProfile>({
  id: '', name: '', type: 'SIMULATED-MC', address: '127.0.0.1', port: 9000, autoConnect: false,
  axes: DEFAULT_AXIS_NAMES.map((name) => ({
    name, enabled: true, kind: name === 'U' ? 'ROTARY' as const : 'LINEAR' as const,
    maxSpeed: 10, stepsPerRev: name === 'U' ? 360 : 200,
    microSteps: 8, lead: 2, gearRatio: 1,
    positionSource: 'register', encoderScale: 1,
    encoderCompensation: { enabled: false, tolerance: 0.01, maxCycles: 10, settleMs: 100, minStep: 0.001, timeoutMs: 5000 },
  })),
});

const isEdit = computed(() => !!editing.id);

const axis0EncComp = computed({
  get: () => editing.axes[0]?.encoderCompensation ?? { enabled: false, tolerance: 0.01, maxCycles: 10, settleMs: 100, minStep: 0.001, timeoutMs: 5000 },
  set: (v) => { if (editing.axes[0]) editing.axes[0].encoderCompensation = v; }
});

function newProfile(): void {
  editing.id = '';
  editing.name = '新控制器';
  editing.type = 'SIMULATED-MC';
  editing.address = '127.0.0.1';
  editing.port = 9000;
  editing.autoConnect = false;
  editing.axes = DEFAULT_AXIS_NAMES.map((name) => ({
    name, enabled: true, kind: name === 'U' ? 'ROTARY' as const : 'LINEAR' as const,
    maxSpeed: 10, stepsPerRev: name === 'U' ? 360 : 200,
    microSteps: 8, lead: 2, gearRatio: 1,
    positionSource: 'register', encoderScale: 1,
    encoderCompensation: { enabled: false, tolerance: 0.01, maxCycles: 10, settleMs: 100, minStep: 0.001, timeoutMs: 5000 },
  }));
}

function editProfile(src: MotionControllerProfile): void {
  editing.id = src.id;
  editing.name = src.name;
  editing.type = src.type;
  editing.address = src.address;
  editing.port = src.port;
  editing.autoConnect = src.autoConnect;
  editing.axes = src.axes.map((a) => ({
    ...a,
    stepsPerRev: a.stepsPerRev ?? (a.name === 'U' ? 360 : 200),
    microSteps: a.microSteps ?? 8,
    lead: a.lead ?? 2,
    gearRatio: a.gearRatio ?? 1,
    positionSource: a.positionSource ?? 'register',
    encoderScale: a.encoderScale ?? 1,
    encoderCompensation: a.encoderCompensation ?? { enabled: false, tolerance: 0.01, maxCycles: 10, settleMs: 100, minStep: 0.001, timeoutMs: 5000 },
  }));
}

async function save(): Promise<void> {
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
      kind: a.kind ?? (a.name === 'U' ? 'ROTARY' as const : 'LINEAR' as const),
      maxSpeed: a.maxSpeed,
      minLimit: a.minLimit,
      maxLimit: a.maxLimit,
      stepsPerRev: a.stepsPerRev,
      microSteps: a.microSteps,
      lead: a.lead,
      gearRatio: a.gearRatio,
      inverted: a.inverted,
      encoderInverted: a.encoderInverted,
      positionSource: a.positionSource,
      encoderScale: a.encoderScale,
      encoderCompensation: a.encoderCompensation,
    })),
  };
  await motion.upsertProfile(profile);
  feedback.pushToast('控制器配置已保存', 'success');
  emit('close');
}

async function remove(id: string): Promise<void> {
  await motion.deleteProfile(id);
  feedback.pushToast('控制器已删除', 'info');
  emit('close');
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
  newProfile();
}

onMounted(async () => {
  await motion.refreshProfiles();
  ensureEditingOnOpen();
});

watch(() => props.open, (v) => {
  if (v) ensureEditingOnOpen();
});

function getAxisThemeClass(axisName: AxisName): string {
  const themeMap: Record<AxisName, string> = {
    X: 'axis-x-theme',
    Y: 'axis-y-theme',
    Z: 'axis-z-theme',
    U: 'axis-u-theme',
  };
  return themeMap[axisName] || '';
}

function toggleLocale() {
  i18n.setLocale(i18n.locale === 'zh' ? 'en' : 'zh');
}

function getAxisUnit(axis: AxisConfig): string {
  return axis.kind === 'ROTARY' ? 'deg' : 'mm';
}

function getStepsPerMm(axis: AxisConfig): number {
  if (axis.kind === 'ROTARY') return 0;
  const stepsPerRev = axis.stepsPerRev || 200;
  const microSteps = axis.microSteps || 1;
  const lead = axis.lead || 2;
  const gearRatio = axis.gearRatio || 1;
  return (stepsPerRev * microSteps * gearRatio) / lead;
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
      <div v-show="open" class="config-overlay" @click="emit('close')">
        <Transition
          enter-active-class="transition ease-out duration-300"
          enter-from-class="opacity-0 scale-95 translate-y-4"
          enter-to-class="opacity-100 scale-100 translate-y-0"
          leave-active-class="transition ease-in duration-200"
          leave-from-class="opacity-100 scale-100 translate-y-0"
          leave-to-class="opacity-0 scale-95 translate-y-4"
        >
          <div v-show="open" class="config-panel" @click.stop>
            <header class="config-panel__header">
              <div class="config-panel__header-left">
                <div class="flex items-center gap-3">
                  <div class="h-10 w-10 flex items-center justify-center rounded-lg bg-gradient-to-br from-[color:var(--accent-primary)]/20 to-[color:var(--accent-primary)]/5 border border-[color:var(--accent-primary)]/20">
                    <svg class="w-5 h-5 text-[color:var(--accent-primary)]" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                      <path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 0 1 0 2.83 2 2 0 0 1-2.83 0l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-2 2 2 2 0 0 1-2-2v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 0 1-2.83 0 2 2 0 0 1 0-2.83l.06-.06a1.65 1.65 0 0 0 .33-1.82 1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1-2-2 2 2 0 0 1 2-2h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 0 1 0-2.83 2 2 0 0 1 2.83 0l.06.06a1.65 1.65 0 0 0 1.82.33H9a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 2-2 2 2 0 0 1 2 2v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 0 1 2.83 0 2 2 0 0 1 0 2.83l-.06.06a1.65 1.65 0 0 0-.33 1.82V9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 2 2 2 2 0 0 1-2 2h-.09a1.65 1.65 0 0 0-1.51 1z"/>
                    </svg>
                  </div>
                  <div>
                    <h2 class="config-panel__title">{{ isEdit ? editing.name : '新建控制器' }}</h2>
                    <p class="config-panel__subtitle">{{ isEdit ? '编辑现有控制器配置' : '创建新的运动控制器配置' }}</p>
                  </div>
                </div>
              </div>
              <div class="config-panel__header-right">
                <button
                  class="locale-toggle-btn"
                  @click="toggleLocale"
                  title="切换语言 / Switch Language"
                >
                  {{ i18n.locale === 'zh' ? 'EN' : '中文' }}
                </button>
                <button class="config-panel__close" @click="emit('close')">
                  <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                    <path d="M18 6L6 18M6 6l12 12"/>
                  </svg>
                </button>
              </div>
            </header>

            <div class="config-panel__body">
              <aside class="config-sidebar">
                <div class="config-sidebar__header">
                  <h3 class="config-sidebar__title">{{ i18n.t.deviceList }}</h3>
                  <button class="config-sidebar__add-btn" @click="newProfile">
                    <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                      <path d="M12 5v14M5 12h14"/>
                    </svg>
                  </button>
                </div>
                <div class="config-sidebar__list">
                  <button
                    v-for="p in motion.profiles"
                    :key="p.id"
                    @click="editProfile(p)"
                    class="config-sidebar__item"
                    :class="{ 'config-sidebar__item--active': editing.id === p.id }"
                  >
                    <span class="config-sidebar__item-name">{{ p.name }}</span>
                    <span class="config-sidebar__item-type">{{ p.type }}</span>
                  </button>
                  <div v-if="motion.profiles.length === 0" class="config-sidebar__empty">
                    <p>{{ i18n.t.noControllerConfig }}</p>
                  </div>
                </div>
              </aside>

              <main class="config-content">
                <div class="config-section">
                  <h3 class="config-section__title">基础设置</h3>
                  <div class="config-grid">
                    <div class="config-field">
                      <label class="config-field__label">名称</label>
                      <input v-model="editing.name" class="config-field__input" placeholder="控制器名称" />
                    </div>
                    <div class="config-field">
                      <label class="config-field__label">类型</label>
                      <select v-model="editing.type" class="config-field__select">
                        <option value="SIMULATED-MC">模拟控制器</option>
                        <option value="B140-MC">B140 控制器</option>
                        <option value="WTNMC4A-MC">WTNMC4A 控制器</option>
                      </select>
                    </div>
                    <div class="config-field">
                      <label class="config-field__label">地址</label>
                      <input v-model="editing.address" class="config-field__input" placeholder="127.0.0.1" />
                    </div>
                    <div class="config-field">
                      <label class="config-field__label">端口</label>
                      <input v-model.number="editing.port" type="number" class="config-field__input config-field__input--short" min="1" max="65535" />
                    </div>
                    <div class="config-field config-field--toggle">
                      <label class="config-field__label">自动连接</label>
                      <label class="toggle-switch">
                        <input type="checkbox" v-model="editing.autoConnect" />
                        <span class="toggle-switch__track">
                          <span class="toggle-switch__thumb"></span>
                        </span>
                      </label>
                    </div>
                  </div>
                </div>

                <div class="config-section">
                  <h3 class="config-section__title">轴配置矩阵</h3>
                  <div class="axis-matrix">
                    <div
                      v-for="(axis, index) in editing.axes"
                      :key="axis.name"
                      class="axis-card"
                      :class="[getAxisThemeClass(axis.name), { 'axis-card--disabled': !axis.enabled }]"
                      :style="{ '--grid-col': (index % 2) + 1, '--grid-row': Math.floor(index / 2) + 1 }"
                    >
                      <div class="axis-card__header">
                        <div class="axis-card__badge">{{ axis.name }}</div>
                        <label class="axis-card__toggle">
                          <input type="checkbox" v-model="axis.enabled" />
                          <span>{{ axis.enabled ? '启用' : '禁用' }}</span>
                        </label>
                      </div>

                      <div class="axis-card__body">
                        <div class="axis-card__group">
                          <label class="axis-card__label">轴类型</label>
                          <select v-model="axis.kind" class="axis-card__select" :disabled="!axis.enabled">
                            <option value="LINEAR">直线轴</option>
                            <option value="ROTARY">旋转轴</option>
                          </select>
                        </div>

                        <div class="axis-card__group">
                          <label class="axis-card__label">每转步数</label>
                          <input v-model.number="axis.stepsPerRev" type="number" class="axis-card__input" :disabled="!axis.enabled" min="1" />
                        </div>

                        <div class="axis-card__group">
                          <label class="axis-card__label">微步</label>
                          <select v-model="axis.microSteps" class="axis-card__select" :disabled="!axis.enabled">
                            <option :value="1">1x</option>
                            <option :value="2">2x</option>
                            <option :value="4">4x</option>
                            <option :value="8">8x</option>
                            <option :value="16">16x</option>
                            <option :value="32">32x</option>
                          </select>
                        </div>

                        <div class="axis-card__group">
                          <label class="axis-card__label">导程 (mm)</label>
                          <input v-model.number="axis.lead" type="number" class="axis-card__input" :disabled="!axis.enabled || axis.kind === 'ROTARY'" min="0.1" step="0.1" />
                        </div>

                        <div class="axis-card__group">
                          <label class="axis-card__label">齿轮比</label>
                          <input v-model.number="axis.gearRatio" type="number" class="axis-card__input" :disabled="!axis.enabled" min="0.1" step="0.1" />
                        </div>

                        <div class="axis-card__group">
                          <label class="axis-card__label">最大速度</label>
                          <input v-model.number="axis.maxSpeed" type="number" class="axis-card__input" :disabled="!axis.enabled" min="1" />
                        </div>

                        <div class="axis-card__group">
                          <label class="axis-card__label">位置源</label>
                          <select v-model="axis.positionSource" class="axis-card__select" :disabled="!axis.enabled">
                            <option value="register">寄存器</option>
                            <option value="encoder">编码器</option>
                          </select>
                        </div>

                        <div class="axis-card__group">
                          <label class="axis-card__label">方向反转</label>
                          <label class="mini-toggle">
                            <input type="checkbox" v-model="axis.inverted" :disabled="!axis.enabled" />
                            <span class="mini-toggle__track">
                              <span class="mini-toggle__thumb"></span>
                            </span>
                          </label>
                        </div>

                        <div class="axis-card__info">
                          <span class="axis-card__info-label">步/mm:</span>
                          <span class="axis-card__info-value">{{ getStepsPerMm(axis).toFixed(2) }}</span>
                        </div>
                      </div>
                    </div>
                  </div>
                </div>

                <div class="config-section">
                  <h3 class="config-section__title">编码器补偿</h3>
                  <div class="encoder-compensation">
                    <div class="encoder-compensation__header">
                      <label class="encoder-compensation__toggle">
                        <input type="checkbox" v-model="axis0EncComp.enabled" />
                        <span>启用编码器补偿</span>
                      </label>
                    </div>
                    <div class="encoder-compensation__fields" v-if="axis0EncComp.enabled">
                      <div class="encoder-compensation__row">
                        <div class="encoder-compensation__field">
                          <label>容差</label>
                          <input v-model.number="axis0EncComp.tolerance" type="number" class="encoder-compensation__input" step="0.001" min="0" />
                        </div>
                        <div class="encoder-compensation__field">
                          <label>最大周期</label>
                          <input v-model.number="axis0EncComp.maxCycles" type="number" class="encoder-compensation__input" min="1" />
                        </div>
                        <div class="encoder-compensation__field">
                          <label>稳定时间 (ms)</label>
                          <input v-model.number="axis0EncComp.settleMs" type="number" class="encoder-compensation__input" min="10" />
                        </div>
                        <div class="encoder-compensation__field">
                          <label>最小步长</label>
                          <input v-model.number="axis0EncComp.minStep" type="number" class="encoder-compensation__input" step="0.0001" min="0" />
                        </div>
                        <div class="encoder-compensation__field">
                          <label>超时时间 (ms)</label>
                          <input v-model.number="axis0EncComp.timeoutMs" type="number" class="encoder-compensation__input" min="100" />
                        </div>
                      </div>
                    </div>
                  </div>
                </div>
              </main>
            </div>

            <footer class="config-panel__footer">
              <div class="config-panel__footer-left">
                <button v-if="isEdit" class="config-panel__delete-btn" @click="remove(editing.id)">删除</button>
                <button class="config-panel__new-btn" @click="newProfile">新建</button>
              </div>
              <div class="config-panel__footer-right">
                <button class="config-panel__cancel-btn" @click="emit('close')">取消</button>
                <button class="config-panel__save-btn" @click="save">保存</button>
              </div>
            </footer>
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
  max-height: 750px;
  display: flex;
  flex-direction: column;
  border-radius: 1rem;
  background: rgba(30, 41, 59, 0.95);
  border: 1px solid rgba(255, 255, 255, 0.1);
  box-shadow: 0 32px 80px -24px rgba(0, 0, 0, 0.5);
  outline: none;
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
  border-bottom: 1px solid rgba(255, 255, 255, 0.08);
}

:root[data-theme='light'] .config-panel__header {
  border-bottom: 1px solid rgba(0, 0, 0, 0.05);
}

.config-panel__header-left,
.config-panel__header-right {
  display: flex;
  align-items: center;
  gap: 1rem;
}

.config-panel__title {
  font-size: 1.1rem;
  font-weight: 700;
  color: #e2e8f0;
}

:root[data-theme='light'] .config-panel__title {
  color: #0f172a;
}

.config-panel__subtitle {
  margin-top: 0.25rem;
  font-size: 0.75rem;
  color: #64748b;
}

.config-panel__close {
  width: 2rem;
  height: 2rem;
  display: flex;
  align-items: center;
  justify-content: center;
  border-radius: 0.5rem;
  color: #64748b;
  background: rgba(0, 0, 0, 0.2);
  transition: all 0.2s ease;
}

.config-panel__close:hover {
  color: #10b981;
  background: rgba(16, 185, 129, 0.15);
}

.locale-toggle-btn {
  padding: 0.375rem 0.75rem;
  border-radius: 0.375rem;
  border: 1px solid rgba(255, 255, 255, 0.1);
  background: rgba(0, 0, 0, 0.15);
  color: #94a3b8;
  font-size: 0.7rem;
  font-weight: 700;
  letter-spacing: 0.05em;
  text-transform: uppercase;
  transition: all 0.2s ease;
}

.locale-toggle-btn:hover {
  color: #fff;
  background: rgba(255, 255, 255, 0.1);
}

.config-panel__body {
  flex: 1;
  display: flex;
  overflow: hidden;
}

.config-sidebar {
  width: 16rem;
  flex-shrink: 0;
  border-right: 1px solid rgba(255, 255, 255, 0.08);
  display: flex;
  flex-direction: column;
}

:root[data-theme='light'] .config-sidebar {
  border-right: 1px solid rgba(0, 0, 0, 0.05);
}

.config-sidebar__header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0.875rem 1rem;
  border-bottom: 1px solid rgba(255, 255, 255, 0.06);
}

.config-sidebar__title {
  font-size: 0.75rem;
  font-weight: 700;
  color: #94a3b8;
  letter-spacing: 0.05em;
  text-transform: uppercase;
}

.config-sidebar__add-btn {
  width: 1.75rem;
  height: 1.75rem;
  display: flex;
  align-items: center;
  justify-content: center;
  border-radius: 0.375rem;
  color: #10b981;
  background: rgba(16, 185, 129, 0.1);
  border: 1px solid rgba(16, 185, 129, 0.2);
  transition: all 0.2s ease;
}

.config-sidebar__add-btn:hover {
  background: rgba(16, 185, 129, 0.2);
}

.config-sidebar__list {
  flex: 1;
  overflow-y: auto;
  padding: 0.5rem;
}

.config-sidebar__item {
  display: flex;
  flex-direction: column;
  padding: 0.625rem 0.75rem;
  margin-bottom: 0.25rem;
  border-radius: 0.375rem;
  background: rgba(0, 0, 0, 0.1);
  border: 1px solid transparent;
  cursor: pointer;
  transition: all 0.15s ease;
}

.config-sidebar__item:hover {
  background: rgba(255, 255, 255, 0.05);
}

.config-sidebar__item--active {
  background: rgba(16, 185, 129, 0.1) !important;
  border-color: rgba(16, 185, 129, 0.3) !important;
}

.config-sidebar__item-name {
  font-size: 0.8rem;
  font-weight: 600;
  color: #e2e8f0;
}

:root[data-theme='light'] .config-sidebar__item-name {
  color: #0f172a;
}

.config-sidebar__item-type {
  font-size: 0.625rem;
  color: #64748b;
  margin-top: 0.125rem;
}

.config-sidebar__empty {
  padding: 2rem 1rem;
  text-align: center;
  color: #64748b;
  font-size: 0.75rem;
}

.config-content {
  flex: 1;
  overflow-y: auto;
  padding: 1rem;
}

.config-section {
  margin-bottom: 1.5rem;
}

.config-section__title {
  font-size: 0.8rem;
  font-weight: 700;
  color: #94a3b8;
  letter-spacing: 0.05em;
  text-transform: uppercase;
  margin-bottom: 0.875rem;
}

.config-grid {
  display: grid;
  grid-template-columns: repeat(2, 1fr);
  gap: 0.75rem;
}

.config-field {
  display: flex;
  flex-direction: column;
  gap: 0.375rem;
}

.config-field--toggle {
  flex-direction: row;
  align-items: center;
  justify-content: space-between;
}

.config-field__label {
  font-size: 0.75rem;
  font-weight: 600;
  color: #64748b;
}

.config-field__input {
  height: 2.25rem;
  padding: 0 0.75rem;
  border-radius: 0.375rem;
  border: 1px solid rgba(255, 255, 255, 0.1);
  background: rgba(0, 0, 0, 0.15);
  color: #e2e8f0;
  font-size: 0.85rem;
  outline: none;
  transition: border-color 0.2s ease, box-shadow 0.2s ease;
}

:root[data-theme='light'] .config-field__input {
  border: 1px solid rgba(0, 0, 0, 0.1);
  background: rgba(0, 0, 0, 0.04);
  color: #0f172a;
}

.config-field__input:focus {
  border-color: #10b981;
  box-shadow: 0 0 0 3px rgba(16, 185, 129, 0.2);
}

.config-field__input--short {
  max-width: 8rem;
}

.config-field__select {
  height: 2.25rem;
  padding: 0 0.75rem;
  border-radius: 0.375rem;
  border: 1px solid rgba(255, 255, 255, 0.1);
  background: rgba(0, 0, 0, 0.15);
  color: #e2e8f0;
  font-size: 0.85rem;
  outline: none;
  cursor: pointer;
  transition: border-color 0.2s ease, box-shadow 0.2s ease;
}

:root[data-theme='light'] .config-field__select {
  border: 1px solid rgba(0, 0, 0, 0.1);
  background: rgba(0, 0, 0, 0.04);
  color: #0f172a;
}

.config-field__select:focus {
  border-color: #10b981;
  box-shadow: 0 0 0 3px rgba(16, 185, 129, 0.2);
}

.toggle-switch {
  display: inline-flex;
  align-items: center;
  cursor: pointer;
}

.toggle-switch input {
  display: none;
}

.toggle-switch__track {
  width: 2.5rem;
  height: 1.25rem;
  border-radius: 9999px;
  background: rgba(0, 0, 0, 0.3);
  position: relative;
  transition: background 0.2s ease;
}

.toggle-switch input:checked + .toggle-switch__track {
  background: #10b981;
}

.toggle-switch__thumb {
  position: absolute;
  left: 2px;
  top: 2px;
  width: calc(1.25rem - 4px);
  height: calc(1.25rem - 4px);
  border-radius: 50%;
  background: white;
  transition: transform 0.2s ease;
}

.toggle-switch input:checked + .toggle-switch__track .toggle-switch__thumb {
  transform: translateX(1.25rem);
}

.axis-matrix {
  display: grid;
  grid-template-columns: repeat(2, 1fr);
  gap: 0.875rem;
}

.axis-card {
  border-radius: 0.5rem;
  border: 1px solid rgba(255, 255, 255, 0.08);
  background: rgba(0, 0, 0, 0.1);
  overflow: hidden;
  transition: all 0.2s ease;
}

:root[data-theme='light'] .axis-card {
  border: 1px solid rgba(0, 0, 0, 0.06);
  background: rgba(0, 0, 0, 0.02);
}

.axis-card--disabled {
  opacity: 0.5;
}

.axis-x-theme {
  --axis-hue: var(--axis-x);
  --axis-hue-soft: var(--axis-x-soft);
}

.axis-y-theme {
  --axis-hue: var(--axis-y);
  --axis-hue-soft: var(--axis-y-soft);
}

.axis-z-theme {
  --axis-hue: var(--axis-z);
  --axis-hue-soft: var(--axis-z-soft);
}

.axis-u-theme {
  --axis-hue: var(--axis-u);
  --axis-hue-soft: var(--axis-u-soft);
}

.axis-card__header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0.75rem;
  background: var(--axis-hue-soft);
  border-bottom: 1px solid rgba(255, 255, 255, 0.06);
}

.axis-card__badge {
  width: 2rem;
  height: 2rem;
  display: flex;
  align-items: center;
  justify-content: center;
  border-radius: 0.375rem;
  background: var(--axis-hue);
  color: white;
  font-size: 1rem;
  font-weight: 800;
}

.axis-card__toggle {
  display: flex;
  align-items: center;
  gap: 0.375rem;
  font-size: 0.7rem;
  font-weight: 600;
  color: #64748b;
  cursor: pointer;
}

.axis-card__body {
  padding: 0.75rem;
}

.axis-card__group {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 0.5rem;
}

.axis-card__label {
  font-size: 0.675rem;
  font-weight: 600;
  color: #64748b;
}

.axis-card__input {
  width: 6rem;
  height: 1.75rem;
  padding: 0 0.5rem;
  border-radius: 0.25rem;
  border: 1px solid rgba(255, 255, 255, 0.1);
  background: rgba(0, 0, 0, 0.2);
  color: #e2e8f0;
  font-size: 0.75rem;
  text-align: right;
  outline: none;
  transition: border-color 0.2s ease;
}

:root[data-theme='light'] .axis-card__input {
  border: 1px solid rgba(0, 0, 0, 0.1);
  background: rgba(0, 0, 0, 0.04);
  color: #0f172a;
}

.axis-card__input:focus {
  border-color: var(--axis-hue);
}

.axis-card__input:disabled {
  opacity: 0.4;
  cursor: not-allowed;
}

.axis-card__select {
  width: 6rem;
  height: 1.75rem;
  padding: 0 0.5rem;
  border-radius: 0.25rem;
  border: 1px solid rgba(255, 255, 255, 0.1);
  background: rgba(0, 0, 0, 0.2);
  color: #e2e8f0;
  font-size: 0.7rem;
  outline: none;
  cursor: pointer;
  transition: border-color 0.2s ease;
}

:root[data-theme='light'] .axis-card__select {
  border: 1px solid rgba(0, 0, 0, 0.1);
  background: rgba(0, 0, 0, 0.04);
  color: #0f172a;
}

.axis-card__select:focus {
  border-color: var(--axis-hue);
}

.axis-card__select:disabled {
  opacity: 0.4;
  cursor: not-allowed;
}

.mini-toggle {
  cursor: pointer;
}

.mini-toggle input {
  display: none;
}

.mini-toggle__track {
  width: 1.75rem;
  height: 1rem;
  border-radius: 9999px;
  background: rgba(0, 0, 0, 0.3);
  position: relative;
  transition: background 0.2s ease;
}

.mini-toggle input:checked + .mini-toggle__track {
  background: var(--axis-hue);
}

.mini-toggle__thumb {
  position: absolute;
  left: 2px;
  top: 2px;
  width: calc(1rem - 4px);
  height: calc(1rem - 4px);
  border-radius: 50%;
  background: white;
  transition: transform 0.2s ease;
}

.mini-toggle input:checked + .mini-toggle__track .mini-toggle__thumb {
  transform: translateX(0.75rem);
}

.axis-card__info {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: 0.5rem;
  margin-top: 0.75rem;
  padding-top: 0.5rem;
  border-top: 1px solid rgba(255, 255, 255, 0.06);
}

.axis-card__info-label {
  font-size: 0.65rem;
  color: #64748b;
}

.axis-card__info-value {
  font-size: 0.75rem;
  font-weight: 600;
  font-family: monospace;
  color: var(--axis-hue);
}

.encoder-compensation {
  border-radius: 0.5rem;
  border: 1px solid rgba(255, 255, 255, 0.08);
  background: rgba(0, 0, 0, 0.1);
  padding: 0.875rem;
}

:root[data-theme='light'] .encoder-compensation {
  border: 1px solid rgba(0, 0, 0, 0.06);
  background: rgba(0, 0, 0, 0.02);
}

.encoder-compensation__header {
  margin-bottom: 0.875rem;
}

.encoder-compensation__toggle {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  font-size: 0.75rem;
  font-weight: 600;
  color: #94a3b8;
  cursor: pointer;
}

.encoder-compensation__fields {
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
}

.encoder-compensation__row {
  display: grid;
  grid-template-columns: repeat(5, 1fr);
  gap: 0.75rem;
}

.encoder-compensation__field {
  display: flex;
  flex-direction: column;
  gap: 0.25rem;
}

.encoder-compensation__field label {
  font-size: 0.675rem;
  font-weight: 600;
  color: #64748b;
}

.encoder-compensation__input {
  height: 1.875rem;
  padding: 0 0.5rem;
  border-radius: 0.25rem;
  border: 1px solid rgba(255, 255, 255, 0.1);
  background: rgba(0, 0, 0, 0.15);
  color: #e2e8f0;
  font-size: 0.75rem;
  outline: none;
  text-align: right;
  transition: border-color 0.2s ease;
}

:root[data-theme='light'] .encoder-compensation__input {
  border: 1px solid rgba(0, 0, 0, 0.1);
  background: rgba(0, 0, 0, 0.04);
  color: #0f172a;
}

.encoder-compensation__input:focus {
  border-color: #8b5cf6;
}

.config-panel__footer {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0.875rem 1.25rem;
  border-top: 1px solid rgba(255, 255, 255, 0.08);
}

:root[data-theme='light'] .config-panel__footer {
  border-top: 1px solid rgba(0, 0, 0, 0.05);
}

.config-panel__footer-left,
.config-panel__footer-right {
  display: flex;
  align-items: center;
  gap: 0.5rem;
}

.config-panel__delete-btn {
  padding: 0.375rem 0.75rem;
  border-radius: 0.375rem;
  border: 1px solid rgba(239, 68, 68, 0.3);
  background: rgba(239, 68, 68, 0.1);
  color: #ef4444;
  font-size: 0.75rem;
  font-weight: 700;
  transition: all 0.2s ease;
}

.config-panel__delete-btn:hover {
  background: rgba(239, 68, 68, 0.2);
}

.config-panel__new-btn {
  padding: 0.375rem 0.75rem;
  border-radius: 0.375rem;
  border: 1px solid rgba(255, 255, 255, 0.1);
  background: rgba(0, 0, 0, 0.15);
  color: #10b981;
  font-size: 0.75rem;
  font-weight: 700;
  transition: all 0.2s ease;
}

.config-panel__new-btn:hover {
  background: rgba(16, 185, 129, 0.15);
  border-color: rgba(16, 185, 129, 0.3);
}

.config-panel__cancel-btn {
  padding: 0.375rem 1rem;
  border-radius: 0.375rem;
  border: 1px solid rgba(255, 255, 255, 0.1);
  background: rgba(0, 0, 0, 0.15);
  color: #94a3b8;
  font-size: 0.75rem;
  font-weight: 700;
  transition: all 0.2s ease;
}

.config-panel__cancel-btn:hover {
  background: rgba(255, 255, 255, 0.05);
}

.config-panel__save-btn {
  padding: 0.375rem 1rem;
  border-radius: 0.375rem;
  border: none;
  background: #10b981;
  color: white;
  font-size: 0.75rem;
  font-weight: 700;
  transition: all 0.2s ease;
}

.config-panel__save-btn:hover {
  background: #059669;
}
</style>
