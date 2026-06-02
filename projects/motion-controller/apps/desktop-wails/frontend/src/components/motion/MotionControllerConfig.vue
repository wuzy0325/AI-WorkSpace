<script setup lang="ts">
import { computed, reactive, onMounted, watch, ref, provide } from 'vue';
import { useMotionStore } from '@stores/motionStore';
import { useFeedbackStore } from '@stores/feedbackStore';
import type { MotionControllerProfile } from '@shared/types/motion';

import { DEFAULT_AXIS_NAMES, createDefaultAxis, defaultEncComp } from './motionConfigEditor';
import ProfileSidebar from './ProfileSidebar.vue';
import ConnectionConfigEditor from './ConnectionConfigEditor.vue';
import AxisConfigCard from './AxisConfigCard.vue';
import EncoderCompensationEditor from './EncoderCompensationEditor.vue';

const props = defineProps<{ open: boolean; currentId?: string | null }>();
const emit = defineEmits<{
  (e: 'close'): void
  (e: 'saved', id: string): void
}>();

const motion = useMotionStore();
const feedback = useFeedbackStore();

const activeTooltip = ref<{ text: string; x: number; y: number } | null>(null);

function showTooltip(text: string, event: MouseEvent): void {
  activeTooltip.value = { text, x: event.clientX, y: event.clientY - 32 };
}
function hideTooltip(): void {
  activeTooltip.value = null;
}
provide<(text: string, event: MouseEvent) => void>('showTooltip', showTooltip);
provide<() => void>('hideTooltip', hideTooltip);

const editing = reactive<MotionControllerProfile>({
  id: '', name: '', type: 'SIMULATED-MC', address: '127.0.0.1', port: 5176, autoConnect: false,
  axes: DEFAULT_AXIS_NAMES.map((name) => createDefaultAxis(name)),
});

const isEdit = computed(() => !!editing.id);

function newProfile(): void {
  editing.id = '';
  editing.name = '新控制器';
  editing.type = 'SIMULATED-MC';
  editing.address = '127.0.0.1';
  editing.port = 5176;
  editing.autoConnect = false;
  editing.axes = DEFAULT_AXIS_NAMES.map((name) => createDefaultAxis(name));
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
    enabled: a.enabled ?? true,
    stepsPerRev: a.stepsPerRev ?? 1.8,
    microSteps: a.microSteps ?? 4,
    lead: a.lead ?? 4,
    gearRatio: a.gearRatio ?? 1,
    positionSource: a.positionSource ?? 'register',
    encoderScale: a.encoderScale ?? 0.005,
    encoderCompensation: a.encoderCompensation ?? defaultEncComp(),
  }));
}

async function save(): Promise<void> {
  const profile: MotionControllerProfile = {
    id: editing.id || (crypto.randomUUID?.() ?? `${Date.now()}-${Math.random().toString(36).slice(2, 8)}`),
    name: editing.name.trim() || '新控制器',
    type: editing.type,
    address: editing.address.trim() || '127.0.0.1',
    port: Number.isFinite(editing.port) ? editing.port : 5176,
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
  };
  await motion.upsertProfile(profile);
  feedback.pushToast('控制器配置已保存', 'success');
  emit('saved', profile.id);
  emit('close');
}

async function remove(id: string): Promise<void> {
  if (!window.confirm('确定要删除此控制器配置吗？此操作不可撤销。')) return
  await motion.deleteProfile(id)
  feedback.pushToast('控制器已删除', 'info')
  emit('close')
}

function ensureEditingOnOpen(): void {
  if (!props.open) return;
  if (props.currentId) {
    const target = motion.profiles.find((p) => p.id === props.currentId);
    if (target) { editProfile(target); return; }
  }
  newProfile();
}

onMounted(async () => {
  await motion.refreshProfiles();
  ensureEditingOnOpen();
});

watch(() => props.open, (v) => { if (v) ensureEditingOnOpen(); });

function onAxisUpdate(index: number, axis: any): void {
  editing.axes[index] = axis;
}

function onUpdateEncComp(index: number, value: any): void {
  editing.axes[index].encoderCompensation = value;
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
                <h2 class="config-panel__title">{{ isEdit ? editing.name : '新建控制器' }}</h2>
                <p class="config-panel__subtitle">{{ isEdit ? '编辑现有控制器配置' : '创建新的运动控制器配置' }}</p>
              </div>
              <div class="config-panel__header-right">
                <button class="config-panel__close" @click="emit('close')">
                  <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                    <path d="M18 6L6 18M6 6l12 12"/>
                  </svg>
                </button>
              </div>
            </header>

            <div class="config-panel__body">
              <ProfileSidebar
                :profiles="motion.profiles"
                :active-id="editing.id"
                @select="(id: string) => { const p = motion.profiles.find(x => x.id === id); if (p) editProfile(p); }"
                @add="newProfile"
              />

              <main class="config-content">
                <ConnectionConfigEditor v-model="editing" />

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

                <EncoderCompensationEditor
                  :axes="editing.axes"
                  @update-enc-comp="onUpdateEncComp"
                />
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
  max-width: 960px;
  max-height: 640px;
  display: flex;
  flex-direction: column;
  border-radius: 0.75rem;
  background: color-mix(in srgb, var(--bg-panel) 95%, transparent);
  border: 1px solid var(--border-default);
  box-shadow: 0 24px 64px -20px rgba(0, 0, 0, 0.5);
  outline: none;
}

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
.config-panel__title {
  font-size: 1rem;
  font-weight: 700;
  color: var(--text-primary);
}

.config-panel__subtitle {
  font-size: 0.7rem;
  color: var(--text-muted);
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
.config-panel__body {
  flex: 1;
  display: flex;
  overflow: hidden;
}
.config-content {
  flex: 1;
  overflow-y: auto;
  padding: 0.75rem;
}
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
.axis-matrix {
  display: grid;
  grid-template-columns: repeat(2, 1fr);
  gap: 0.625rem;
}
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
.config-panel__delete-btn,
.config-panel__new-btn {
  padding: 0.375rem 0.875rem;
  border-radius: 0.375rem;
  font-size: 0.75rem;
  font-weight: 600;
  transition: all 0.2s ease;
}
.config-panel__delete-btn {
  color: var(--accent-danger);
  background: color-mix(in srgb, var(--accent-danger) 10%, transparent);
  border: 1px solid color-mix(in srgb, var(--accent-danger) 20%, transparent);
}
.config-panel__delete-btn:hover {
  background: color-mix(in srgb, var(--accent-danger) 20%, transparent);
}
.config-panel__new-btn {
  color: var(--accent-success);
  background: color-mix(in srgb, var(--accent-success) 10%, transparent);
  border: 1px solid color-mix(in srgb, var(--accent-success) 20%, transparent);
}
.config-panel__new-btn:hover {
  background: color-mix(in srgb, var(--accent-success) 20%, transparent);
}
.config-panel__cancel-btn,
.config-panel__save-btn {
  padding: 0.375rem 1rem;
  border-radius: 0.375rem;
  font-size: 0.75rem;
  font-weight: 600;
  transition: all 0.2s ease;
}
.config-panel__cancel-btn {
  color: var(--text-muted);
  background: var(--bg-panel-strong);
  border: 1px solid var(--border-default);
}
.config-panel__cancel-btn:hover {
  color: var(--text-primary);
  background: var(--bg-canvas);
}
.config-panel__save-btn {
  color: #fff;
  background: linear-gradient(135deg, var(--accent-success), color-mix(in srgb, var(--accent-success) 80%, black));
  border: none;
}
.config-panel__save-btn:hover {
  box-shadow: 0 4px 16px color-mix(in srgb, var(--accent-success) 30%, transparent);
}
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
