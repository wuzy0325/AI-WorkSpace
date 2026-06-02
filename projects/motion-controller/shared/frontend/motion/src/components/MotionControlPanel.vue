<script setup lang="ts">
/**
 * 运动控制面板组件
 * 共享模块 - 各项目可自定义样式
 */
import { onMounted, onBeforeUnmount, computed, reactive, ref, watch } from 'vue';
import { useMotionStore } from '../stores';
import { motionZh, motionEn } from '../i18n';
import type { AxisName } from '../types/motion';

const props = defineProps<{
  locale?: 'zh' | 'en';
}>();

const motion = useMotionStore();
const selectedId = ref<string | null>(null);
const showConfig = ref(false);

const t = computed(() => props.locale === 'en' ? motionEn : motionZh);

type AxisHistoryMap = Map<AxisName, number[]>;
const axisHistory = ref<Map<string, AxisHistoryMap>>(new Map());
const MAX_HISTORY = 50;

interface AxisLocalState {
  targetPosition: number;
  step: number;
  expandedPanel: 'monitor' | 'jog' | 'move' | null;
}

type AxisLocalStateMap = Record<AxisName, AxisLocalState>;
const axisLocalState = reactive<Record<string, AxisLocalStateMap>>({});

function ensureAxisLocalState(controllerId: string, axisName: AxisName): AxisLocalState {
  if (!axisLocalState[controllerId]) {
    axisLocalState[controllerId] = {
      X: { targetPosition: 0, step: 1, expandedPanel: 'monitor' },
      Y: { targetPosition: 0, step: 1, expandedPanel: 'monitor' },
      Z: { targetPosition: 0, step: 1, expandedPanel: 'monitor' },
      U: { targetPosition: 0, step: 1, expandedPanel: 'monitor' }
    };
  }
  return axisLocalState[controllerId][axisName];
}

function togglePanel(controllerId: string, axisName: AxisName, panel: 'monitor' | 'jog' | 'move'): void {
  const state = ensureAxisLocalState(controllerId, axisName);
  state.expandedPanel = state.expandedPanel === panel ? null : panel;
}

type AxisZeroOffsetMap = Record<AxisName, number>;
const axisZeroOffset = reactive<Record<string, AxisZeroOffsetMap>>({});

function ensureZeroOffsetMap(controllerId: string): AxisZeroOffsetMap {
  if (!axisZeroOffset[controllerId]) {
    axisZeroOffset[controllerId] = { X: 0, Y: 0, Z: 0, U: 0 };
  }
  return axisZeroOffset[controllerId];
}

function getZeroOffset(controllerId: string, axisName: AxisName): number {
  return ensureZeroOffsetMap(controllerId)[axisName] ?? 0;
}

function setZeroOffset(controllerId: string, axisName: AxisName, value: number): void {
  ensureZeroOffsetMap(controllerId)[axisName] = value;
}

const currentStatus = computed(() =>
  selectedId.value ? motion.statusById(selectedId.value) : undefined
);

const currentProfile = computed(() =>
  selectedId.value ? motion.profiles.find((p) => p.id === selectedId.value) : undefined
);

const controllerConnected = computed(() => Boolean(currentStatus.value?.connected));
const axes = computed(() => currentStatus.value?.axes ?? []);

function getAxisKind(axisName: AxisName): 'LINEAR' | 'ROTARY' {
  if (!currentProfile.value) return 'LINEAR';
  const axisConfig = currentProfile.value.axes.find((a) => a.name === axisName);
  return axisConfig?.kind ?? 'LINEAR';
}

function getAxisUnit(axisName: AxisName): string {
  return getAxisKind(axisName) === 'ROTARY' ? '°' : 'mm';
}

function selectController(id: string): void {
  selectedId.value = id;
}

async function handleConnect(): Promise<void> {
  if (!selectedId.value) return;
  await motion.connect(selectedId.value);
}

async function handleDisconnect(): Promise<void> {
  if (!selectedId.value) return;
  await motion.disconnect(selectedId.value);
}

async function move(axis: AxisName): Promise<void> {
  if (!selectedId.value || !controllerConnected.value) return;
  const state = ensureAxisLocalState(selectedId.value, axis);
  const offset = getZeroOffset(selectedId.value, axis);
  const absoluteTarget = offset + state.targetPosition;
  await motion.moveTo(selectedId.value, axis, absoluteTarget);
}

async function jogAxis(axis: AxisName, direction: 'forward' | 'reverse'): Promise<void> {
  if (!selectedId.value || !controllerConnected.value) return;
  const axisConfig = currentProfile.value?.axes.find((a) => a.name === axis);
  const maxSpeed = axisConfig?.maxSpeed ?? 10;
  await motion.jog(selectedId.value, axis, direction, maxSpeed);
}

async function stop(axis?: AxisName): Promise<void> {
  if (!selectedId.value || !controllerConnected.value) return;
  await motion.stop(selectedId.value, axis);
}

async function emergencyStop(): Promise<void> {
  if (!selectedId.value || !controllerConnected.value) return;
  await motion.emergencyStop(selectedId.value);
}

async function adjustByStep(axis: AxisName, direction: 'forward' | 'reverse'): Promise<void> {
  if (!selectedId.value || !controllerConnected.value || !currentStatus.value) return;
  const state = ensureAxisLocalState(selectedId.value, axis);
  const axisStatus = currentStatus.value.axes.find((a) => a.name === axis);
  if (!axisStatus) return;
  const delta = direction === 'forward' ? state.step : -state.step;
  await motion.moveBy(selectedId.value, axis, delta);
}

async function setZero(axis: AxisName): Promise<void> {
  if (!selectedId.value || !controllerConnected.value) return;
  await motion.definePosition(selectedId.value, axis, 0);
  setZeroOffset(selectedId.value, axis, 0);
  const state = ensureAxisLocalState(selectedId.value, axis);
  state.targetPosition = 0;
}

function appendHistory(controllerId: string, axisName: AxisName, position: number): void {
  let ctrlMap = axisHistory.value.get(controllerId);
  if (!ctrlMap) {
    ctrlMap = new Map();
    axisHistory.value.set(controllerId, ctrlMap);
  }
  let arr = ctrlMap.get(axisName);
  if (!arr) {
    arr = [];
    ctrlMap.set(axisName, arr);
  }
  const offset = getZeroOffset(controllerId, axisName);
  arr.push(position - offset);
  if (arr.length > MAX_HISTORY) {
    arr.splice(0, arr.length - MAX_HISTORY);
  }
}

function getAxisHistory(axisName: AxisName): number[] {
  if (!selectedId.value) return [];
  const ctrlMap = axisHistory.value.get(selectedId.value);
  if (!ctrlMap) return [];
  return ctrlMap.get(axisName) ?? [];
}

function historyBarStyle(axisName: AxisName): Record<string, string> {
  const data = getAxisHistory(axisName);
  if (data.length === 0) return {};
  const min = data.reduce((a, b) => Math.min(a, b), Infinity);
  const max = data.reduce((a, b) => Math.max(a, b), -Infinity);
  if (!Number.isFinite(min) || !Number.isFinite(max) || min === max) return {};
  const segments: string[] = [];
  const n = data.length;
  for (let i = 0; i < n; i += 1) {
    const t = n === 1 ? 0 : (i / (n - 1)) * 100;
    const ratio = (data[i] - min) / (max - min);
    const hue = 210 - ratio * 90;
    segments.push(`hsl(${hue} 80% 60%) ${t.toFixed(1)}%`);
  }
  return { backgroundImage: `linear-gradient(to right, ${segments.join(', ')})` };
}

let unsubscribeStatus: (() => void) | null = null;

function handleKeydown(e: KeyboardEvent): void {
  if (e.key === 'Escape') {
    e.preventDefault();
    emergencyStop();
  }
}

onMounted(async () => {
  await motion.refreshProfiles();
  await motion.refreshStatus();
  if (motion.profiles.length > 0) {
    selectedId.value = motion.profiles[0].id;
  }
  unsubscribeStatus = motion.attachStatusListener();
  window.addEventListener('keydown', handleKeydown);
});

onBeforeUnmount(() => {
  if (unsubscribeStatus) unsubscribeStatus();
  window.removeEventListener('keydown', handleKeydown);
});

watch(
  () => currentStatus.value,
  (status) => {
    if (!status || !selectedId.value) return;
    for (const axis of status.axes) {
      appendHistory(selectedId.value, axis.name, axis.position);
    }
  },
  { deep: true }
);
</script>

<template>
  <div class="motion-control-panel">
    <!-- 控制器选择列表 -->
    <aside class="controller-list">
      <div class="controller-list-header">
        <span>{{ t.motionController }}</span>
        <button @click="showConfig = true">{{ t.config }}</button>
      </div>

      <div v-if="motion.profiles.length === 0" class="no-config">
        <p>{{ t.noControllerConfig }}</p>
        <p>{{ t.clickConfigToAdd }}</p>
      </div>

      <div v-else class="controller-items">
        <button
          v-for="p in motion.profiles"
          :key="p.id"
          @click="selectController(p.id)"
          :class="{ active: selectedId === p.id }"
        >
          <span class="status-dot" :class="motion.statusById(p.id)?.connected ? 'connected' : ''" />
          <span>{{ p.name }}</span>
          <span>{{ motion.statusById(p.id)?.connected ? t.connected : t.disconnected }}</span>
        </button>
      </div>
    </aside>

    <!-- 控制面板主体 -->
    <section class="control-panel">
      <header class="panel-header">
        <div class="controller-info">
          <h2>{{ selectedId ? motion.profiles.find(p => p.id === selectedId)?.name || t.selectController : t.selectController }}</h2>
          <span>{{ currentStatus?.connected ? t.systemOnline : t.systemOffline }}</span>
        </div>
        <div class="control-buttons">
          <button @click="handleConnect" :disabled="!selectedId || currentStatus?.connected">{{ t.connectBtn }}</button>
          <button @click="handleDisconnect" :disabled="!selectedId || !currentStatus?.connected">{{ t.disconnectBtn }}</button>
          <button @click="stop()" :disabled="!selectedId || !currentStatus?.connected">{{ t.stopAll }}</button>
          <button class="estop" @click="emergencyStop" :disabled="!selectedId">{{ t.eStop }}</button>
        </div>
      </header>

      <!-- 轴卡片 -->
      <div v-if="axes.length === 0" class="no-axes">
        <p>{{ t.noAxesConfigured }}</p>
        <button @click="showConfig = true">{{ t.openConfig }}</button>
      </div>

      <div v-else class="axis-grid">
        <div v-for="axis in axes" :key="axis.name" class="axis-card">
          <div class="axis-header">
            <span class="axis-name">{{ axis.name }}</span>
            <span :class="axis.moving ? 'moving' : ''">{{ axis.moving ? t.moving : t.idle }}</span>
          </div>

          <div class="position-display">
            <span class="position-value">{{ (axis.position - getZeroOffset(selectedId as string, axis.name as AxisName)).toFixed(2) }}</span>
            <span class="position-unit">{{ getAxisUnit(axis.name as AxisName) }}</span>
          </div>

          <div class="axis-tabs">
            <button @click="togglePanel(selectedId as string, axis.name as AxisName, 'monitor')">{{ t.monitor }}</button>
            <button @click="togglePanel(selectedId as string, axis.name as AxisName, 'jog')" :disabled="!controllerConnected">{{ t.jog }}</button>
            <button @click="togglePanel(selectedId as string, axis.name as AxisName, 'move')" :disabled="!controllerConnected">{{ t.move }}</button>
          </div>

          <!-- Monitor Panel -->
          <div v-if="ensureAxisLocalState(selectedId as string, axis.name as AxisName).expandedPanel === 'monitor'" class="panel-content">
            <div class="status-row">
              <span>{{ t.status }}</span>
              <span>{{ axis.moving ? t.moving : t.idle }}</span>
            </div>
            <div class="status-row">
              <span>{{ t.homeStatus }}</span>
              <span>{{ axis.homed ? t.homed : t.notHomed }}</span>
            </div>
            <div class="button-row">
              <button @click="setZero(axis.name as AxisName)" :disabled="axis.moving || !controllerConnected">{{ t.setZero }}</button>
              <button @click="stop(axis.name as AxisName)" :disabled="!axis.moving || !controllerConnected">{{ t.stop }}</button>
            </div>
          </div>

          <!-- Jog Panel -->
          <div v-if="ensureAxisLocalState(selectedId as string, axis.name as AxisName).expandedPanel === 'jog'" class="panel-content">
            <div class="step-control">
              <span>{{ t.jogStep }}: {{ ensureAxisLocalState(selectedId as string, axis.name as AxisName).step }}</span>
              <div class="step-buttons">
                <button @click="adjustByStep(axis.name as AxisName, 'reverse')" :disabled="axis.moving || !controllerConnected">−</button>
                <input v-model.number="ensureAxisLocalState(selectedId as string, axis.name as AxisName).step" type="number" />
                <button @click="adjustByStep(axis.name as AxisName, 'forward')" :disabled="axis.moving || !controllerConnected">+</button>
              </div>
            </div>
          </div>

          <!-- Move Panel -->
          <div v-if="ensureAxisLocalState(selectedId as string, axis.name as AxisName).expandedPanel === 'move'" class="panel-content">
            <div class="move-control">
              <input v-model.number="ensureAxisLocalState(selectedId as string, axis.name as AxisName).targetPosition" type="number" :disabled="axis.moving || !controllerConnected" />
              <button @click="move(axis.name as AxisName)" :disabled="axis.moving || !controllerConnected">{{ t.move }}</button>
            </div>
          </div>

          <!-- 历史条 -->
          <div class="history-bar" :style="historyBarStyle(axis.name as AxisName)"></div>
        </div>
      </div>
    </section>
  </div>
</template>

<style scoped>
.motion-control-panel {
  display: flex;
  gap: 12px;
  height: 100%;
}

.controller-list {
  width: 220px;
  background: var(--bg-panel);
  border: 1px solid var(--border-default);
  border-radius: 8px;
  padding: 12px;
  flex-shrink: 0;
}

.controller-list-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 8px;
  font-size: 11px;
  font-weight: 600;
  color: var(--text-secondary);
}

.controller-list-header button {
  padding: 4px 8px;
  font-size: 10px;
  border: 1px solid var(--accent-primary);
  border-radius: 4px;
  color: var(--accent-primary);
  background: transparent;
  cursor: pointer;
}

.controller-items button {
  width: 100%;
  padding: 12px;
  margin-bottom: 8px;
  border-radius: 6px;
  background: var(--bg-panel-strong);
  border: 1px solid transparent;
  text-align: left;
  cursor: pointer;
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.controller-items button.active {
  border-color: var(--accent-success);
  background: rgba(34, 197, 94, 0.1);
}

.status-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: var(--text-muted);
}

.status-dot.connected {
  background: var(--accent-success);
  box-shadow: 0 0 8px var(--accent-success);
}

.control-panel {
  flex: 1;
  background: var(--bg-panel);
  border: 1px solid var(--border-default);
  border-radius: 12px;
  display: flex;
  flex-direction: column;
  overflow: hidden;
}

.panel-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 12px 16px;
  border-bottom: 1px solid var(--border-default);
}

.controller-info h2 {
  font-size: 16px;
  font-weight: 700;
  color: var(--text-primary);
}

.controller-info span {
  font-size: 12px;
  color: var(--text-muted);
}

.control-buttons {
  display: flex;
  gap: 8px;
}

.control-buttons button {
  padding: 8px 16px;
  font-size: 12px;
  font-weight: 700;
  border-radius: 6px;
  cursor: pointer;
  border: none;
}

.control-buttons button:first-child {
  background: var(--accent-success);
  color: white;
}

.control-buttons button:nth-child(2) {
  background: var(--bg-panel-strong);
  border: 1px solid var(--border-default);
  color: var(--text-secondary);
}

.control-buttons button:nth-child(3) {
  background: var(--accent-warning);
  color: white;
}

.control-buttons .estop {
  background: var(--accent-danger);
  color: white;
}

.axis-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(200px, 1fr));
  gap: 12px;
  padding: 16px;
  overflow: auto;
  flex: 1;
}

.axis-card {
  background: var(--bg-panel);
  border: 1px solid var(--border-default);
  border-radius: 12px;
  padding: 12px;
  display: flex;
  flex-direction: column;
  gap: 8px;
  position: relative;
}

.axis-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.axis-name {
  width: 32px;
  height: 32px;
  display: flex;
  align-items: center;
  justify-content: center;
  border-radius: 8px;
  background: var(--accent-primary);
  color: white;
  font-size: 16px;
  font-weight: 900;
}

.axis-header span:last-child {
  font-size: 10px;
  padding: 2px 8px;
  border-radius: 10px;
  background: var(--bg-canvas);
}

.axis-header span.moving {
  background: rgba(34, 197, 94, 0.2);
  color: var(--accent-success);
}

.position-display {
  padding: 12px;
  background: var(--bg-canvas);
  border-radius: 8px;
  text-align: center;
}

.position-value {
  font-size: 24px;
  font-weight: 700;
  font-family: monospace;
}

.position-unit {
  font-size: 12px;
  color: var(--text-muted);
  margin-left: 4px;
}

.axis-tabs {
  display: flex;
  gap: 4px;
  padding: 4px;
  background: var(--bg-canvas);
  border-radius: 6px;
}

.axis-tabs button {
  flex: 1;
  padding: 6px 8px;
  font-size: 11px;
  font-weight: 700;
  border: none;
  border-radius: 4px;
  background: transparent;
  color: var(--text-muted);
  cursor: pointer;
}

.axis-tabs button.active {
  background: var(--bg-panel);
  color: var(--text-primary);
}

.panel-content {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.status-row {
  display: flex;
  justify-content: space-between;
  font-size: 10px;
}

.button-row {
  display: flex;
  gap: 8px;
}

.button-row button {
  flex: 1;
  padding: 8px;
  font-size: 12px;
  font-weight: 700;
  border-radius: 6px;
  border: 1px solid var(--border-default);
  background: var(--bg-panel-strong);
  cursor: pointer;
}

.step-control {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.step-control span {
  font-size: 10px;
}

.step-buttons {
  display: flex;
  gap: 4px;
}

.step-buttons button {
  width: 40px;
  height: 40px;
  font-size: 20px;
  border: 1px solid var(--border-default);
  border-radius: 6px;
  background: var(--bg-panel);
  cursor: pointer;
}

.step-buttons input {
  flex: 1;
  text-align: center;
  font-size: 14px;
}

.move-control {
  display: flex;
  gap: 8px;
}

.move-control input {
  flex: 1;
  padding: 8px;
  font-size: 14px;
}

.move-control button {
  padding: 8px 16px;
  font-size: 12px;
  font-weight: 700;
  border-radius: 8px;
  background: var(--accent-primary);
  color: white;
  border: none;
  cursor: pointer;
}

.history-bar {
  height: 4px;
  border-radius: 2px;
  margin-top: auto;
  opacity: 0.6;
}

.no-config, .no-axes {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  padding: 24px;
  color: var(--text-muted);
}

.no-axes button {
  margin-top: 12px;
  padding: 8px 16px;
  border: 1px solid var(--border-default);
  border-radius: 6px;
  background: var(--bg-panel-strong);
  cursor: pointer;
}
</style>
