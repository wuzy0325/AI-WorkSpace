<script setup lang="ts">
import { computed, reactive, onMounted, watch, ref, provide } from 'vue';
import { NButton } from 'naive-ui';
import { useMotionStore } from '@stores/motionStore';
import { useI18nStore } from '@stores/i18nStore';
import { useFeedbackStore } from '@stores/feedbackStore';
import type { MotionControllerProfile } from '@shared/types/motion';
import { DEFAULT_AXIS_NAMES, createDefaultAxis } from './motionConfigEditor';
import ProfileSidebar from './ProfileSidebar.vue';
import ConnectionConfigEditor from './ConnectionConfigEditor.vue';
import AxisConfigCard from './AxisConfigCard.vue';

const props = defineProps<{ open: boolean; currentId?: string | null }>();
const emit = defineEmits<{ (e: 'close'): void }>();

const motion = useMotionStore();
const i18n = useI18nStore();
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
  id: '', name: '', type: 'SIMULATED-MC', address: '127.0.0.1', port: 9000, autoConnect: false,
  axes: DEFAULT_AXIS_NAMES.map((name) => createDefaultAxis(name) as any),
});

const isEdit = computed(() => !!editing.id);

const connectionConfig = computed({
  get: () => ({
    name: editing.name,
    type: editing.type,
    address: editing.address,
    port: editing.port,
    autoConnect: editing.autoConnect,
  }),
  set: (val) => {
    editing.name = val.name;
    editing.type = val.type;
    editing.address = val.address;
    editing.port = val.port;
    editing.autoConnect = val.autoConnect;
  },
});

function newProfile(): void {
  editing.id = '';
  editing.name = '新控制器';
  editing.type = 'SIMULATED-MC';
  editing.address = '127.0.0.1';
  editing.port = 9000;
  editing.autoConnect = false;
  editing.axes = DEFAULT_AXIS_NAMES.map((name) => createDefaultAxis(name) as any);
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
      name: a.name, enabled: a.enabled, kind: a.kind ?? (a.name === 'U' ? 'ROTARY' as const : 'LINEAR' as const),
      maxSpeed: a.maxSpeed, minLimit: a.minLimit, maxLimit: a.maxLimit,
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
  }

function toggleLocale() {
  i18n.setLocale(i18n.locale === 'zh' ? 'en' : 'zh');
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
                <NButton size="tiny" class="locale-toggle-btn" @click="toggleLocale" title="切换语言 / Switch Language">
                  {{ i18n.locale === 'zh' ? 'EN' : '中文' }}
                </NButton>
                <NButton quaternary size="small" class="config-panel__close" @click="emit('close')">
                  <template #icon>
                    <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                      <path d="M18 6L6 18M6 6l12 12"/>
                    </svg>
                  </template>
                </NButton>
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
                <ConnectionConfigEditor v-model="connectionConfig" />

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
              </main>
            </div>

            <footer class="config-panel__footer">
              <div class="config-panel__footer-left">
                <NButton v-if="isEdit" type="error" size="tiny" class="config-panel__delete-btn" @click="remove(editing.id)">删除</NButton>
                <NButton type="primary" size="tiny" class="config-panel__new-btn" @click="newProfile">新建</NButton>
              </div>
              <div class="config-panel__footer-right">
                <NButton quaternary size="tiny" class="config-panel__cancel-btn" @click="emit('close')">取消</NButton>
                <NButton type="primary" size="tiny" class="config-panel__save-btn" @click="save">保存</NButton>
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
.section-subtitle {
  display: inline;
  font-size: 0.65rem;
  font-weight: 400;
  color: #64748b;
  text-transform: none;
  letter-spacing: normal;
  margin-left: 0.5rem;
}
.axis-matrix {
  display: grid;
  grid-template-columns: repeat(2, 1fr);
  gap: 0.875rem;
}
.config-panel__footer {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 1rem 1.25rem;
  border-top: 1px solid rgba(255, 255, 255, 0.08);
}
:root[data-theme='light'] .config-panel__footer {
  border-top: 1px solid rgba(0, 0, 0, 0.05);
}
.config-panel__footer-left,
.config-panel__footer-right {
  display: flex;
  align-items: center;
  gap: 0.75rem;
}
.config-panel__delete-btn,
.config-panel__new-btn {
  padding: 0.5rem 1rem;
  border-radius: 0.5rem;
  font-size: 0.8rem;
  font-weight: 600;
  transition: all 0.2s ease;
}
.config-panel__delete-btn {
  color: #ef4444;
  background: rgba(239, 68, 68, 0.1);
  border: 1px solid rgba(239, 68, 68, 0.2);
}
.config-panel__delete-btn:hover {
  background: rgba(239, 68, 68, 0.2);
}
.config-panel__new-btn {
  color: #10b981;
  background: rgba(16, 185, 129, 0.1);
  border: 1px solid rgba(16, 185, 129, 0.2);
}
.config-panel__new-btn:hover {
  background: rgba(16, 185, 129, 0.2);
}
.config-panel__cancel-btn,
.config-panel__save-btn {
  padding: 0.5rem 1.25rem;
  border-radius: 0.5rem;
  font-size: 0.8rem;
  font-weight: 600;
  transition: all 0.2s ease;
}
.config-panel__cancel-btn {
  color: #94a3b8;
  background: rgba(0, 0, 0, 0.15);
  border: 1px solid rgba(255, 255, 255, 0.06);
}
.config-panel__cancel-btn:hover {
  color: #e2e8f0;
  background: rgba(0, 0, 0, 0.25);
}
.config-panel__save-btn {
  color: #fff;
  background: linear-gradient(135deg, #10b981, #059669);
  border: none;
}
.config-panel__save-btn:hover {
  box-shadow: 0 4px 16px rgba(16, 185, 129, 0.3);
}
.field-tooltip {
  position: fixed;
  z-index: 9999;
  padding: 0.5rem 0.75rem;
  border-radius: 0.375rem;
  font-size: 0.7rem;
  font-weight: 500;
  color: #f1f5f9;
  background: rgba(15, 23, 42, 0.95);
  border: 1px solid rgba(255, 255, 255, 0.1);
  pointer-events: none;
  white-space: nowrap;
  box-shadow: 0 4px 12px rgba(0, 0, 0, 0.3);
}
:root[data-theme='light'] .field-tooltip {
  color: #0f172a;
  background: rgba(255, 255, 255, 0.98);
  border: 1px solid rgba(0, 0, 0, 0.1);
}
</style>