<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useI18nStore } from '@stores/i18nStore'
import { useFeedbackStore } from '@stores/feedbackStore'
import { traversalApi } from '@api/deviceApi'
import { useDeviceStore } from '@stores/deviceStore'

const i18n = useI18nStore()
const feedback = useFeedbackStore()
const deviceStore = useDeviceStore()

type Tab = 'config' | 'execute' | 'visualize'
type TraversalMode = 'hil' | 'layout' | 'prb'

const activeTab = ref<Tab>('config')
const busy = ref(false)
const error = ref('')
const state = ref('idle')
const currentPoint = ref(0)
const totalPoints = ref(0)
const lastTaskId = ref('')

const pathConfig = ref({
  xStart: 0, xEnd: 100, xStep: 10,
  yStart: 0, yEnd: 100, yStep: 10,
  zStart: 0, zEnd: 50, zStep: 10,
  deviceId: 'sim-1',
  channels: '0,1,2,3',
  mode: 'hil' as TraversalMode,
})

const generatedPath = ref<{ x: number; y: number; z: number }[]>([])

const running = computed(() => state.value === 'running')
const paused = computed(() => state.value === 'paused')
const completed = computed(() => state.value === 'idle' && currentPoint.value > 0 && currentPoint.value >= totalPoints.value)

const tabs = [
  { id: 'config' as const, label: '路径配置' },
  { id: 'execute' as const, label: '执行控制' },
  { id: 'visualize' as const, label: '可视化' },
]

const modes = [
  { id: 'hil' as const, label: 'HIL (水平网格)' },
  { id: 'layout' as const, label: 'Layout (布局点)' },
  { id: 'prb' as const, label: 'PRB (探针校准)' },
]

async function run(action: () => Promise<void>) {
  busy.value = true; error.value = ''
  try { await action() } catch (err) { error.value = String(err); feedback.pushToast(String(err), 'error') }
  finally { busy.value = false }
}

async function refreshStatus() {
  try {
    const s = await traversalApi.status()
    state.value = s.state
    currentPoint.value = s.currentPoint
    totalPoints.value = s.totalPoints
  } catch { /* offline */ }
}

async function startTraversal() {
  await run(async () => {
    const taskId = `trav-${Date.now()}`
    lastTaskId.value = taskId
    const channels = pathConfig.value.channels.split(',').map(s => parseInt(s.trim())).filter(n => !isNaN(n))
    const path = await traversalApi.generateGrid({
      xStart: pathConfig.value.xStart, xEnd: pathConfig.value.xEnd, xStep: pathConfig.value.xStep,
      yStart: pathConfig.value.yStart, yEnd: pathConfig.value.yEnd, yStep: pathConfig.value.yStep,
      zStart: pathConfig.value.zStart,
    })
    generatedPath.value = path
    await traversalApi.start(taskId, pathConfig.value.deviceId, channels, path)
    await refreshStatus()
    totalPoints.value = path.length
    activeTab.value = 'execute'
    feedback.pushToast('遍历任务已启动', 'success')
  })
}

async function pauseTraversal() { await run(async () => { await traversalApi.pause(); await refreshStatus(); feedback.pushToast('已暂停', 'info') }) }
async function resumeTraversal() { await run(async () => { await traversalApi.resume(); await refreshStatus(); feedback.pushToast('已恢复', 'success') }) }
async function stopTraversal() { await run(async () => { await traversalApi.stop(); await refreshStatus(); feedback.pushToast('已停止', 'info') }) }
async function runCurrentPoint() {
  await run(async () => {
    await traversalApi.runPoint()
    await refreshStatus()
    if (completed.value) {
      feedback.pushToast('遍历完成！', 'success')
    }
  })
}

onMounted(refreshStatus)
</script>

<template>
  <div class="traversal-view">
    <div class="traversal-view__head">
      <p class="eyebrow">{{ i18n.t.traversalTest }}</p>
      <h2>{{ i18n.t.traversalTest }}</h2>
    </div>

    <section class="state-panel" :class="{ complete: completed, error: !!error && !running }">
      <div class="state-panel__indicator" />
      <div>
        <h3>{{ completed ? '遍历完成' : (running ? '遍历进行中' : (paused ? '已暂停' : '遍历测试')) }}</h3>
        <p v-if="completed">所有 {{ totalPoints }} 个点位已采集完毕</p>
        <p v-else-if="running">点位: {{ currentPoint + 1 }} / {{ totalPoints }} — 按「采集当前点」记录数据</p>
        <p v-else-if="paused">点击恢复继续遍历</p>
        <p v-else>配置遍历路径参数后启动任务</p>
      </div>
    </section>

    <p v-if="error" class="error-text">{{ error }}</p>

    <div class="traversal-tabs">
      <button
        v-for="t in tabs"
        :key="t.id"
        class="traversal-tab"
        :class="{ active: activeTab === t.id }"
        @click="activeTab = t.id"
      >
        {{ t.label }}
      </button>
    </div>

    <div v-if="activeTab === 'config'" class="traversal-config">
      <div class="traversal-config__mode">
        <h3>遍历模式</h3>
        <div class="traversal-config__mode-btns">
          <button v-for="m in modes" :key="m.id" class="traversal-tab" :class="{ active: pathConfig.mode === m.id }" @click="pathConfig.mode = m.id">{{ m.label }}</button>
        </div>
      </div>
      <div class="traversal-config__misc">
        <div class="traversal-config__field">
          <label>设备</label>
          <select v-model="pathConfig.deviceId">
            <option v-for="d in deviceStore.profiles" :key="d.id" :value="d.id">{{ d.name }}</option>
          </select>
        </div>
        <div class="traversal-config__field">
          <label>通道 (逗号分隔)</label>
          <input v-model="pathConfig.channels" type="text" />
        </div>
      </div>
      <div class="traversal-config__grid">
        <div v-for="axis in ['x', 'y', 'z']" :key="axis" class="traversal-config__axis">
          <h3>{{ axis.toUpperCase() }} 轴</h3>
          <div class="traversal-config__field">
            <label>起始</label>
            <input v-model.number="pathConfig[`${axis}Start` as keyof typeof pathConfig]" type="number" />
          </div>
          <div class="traversal-config__field">
            <label>结束</label>
            <input v-model.number="pathConfig[`${axis}End` as keyof typeof pathConfig]" type="number" />
          </div>
          <div class="traversal-config__field">
            <label>步长</label>
            <input v-model.number="pathConfig[`${axis}Step` as keyof typeof pathConfig]" type="number" step="1" />
          </div>
        </div>
      </div>
      <div class="traversal-config__preview">
        <h3>路径预览 ({{ generatedPath.length || 0 }} 个点)</h3>
        <svg :viewBox="`0 0 200 200`" class="traversal-config__svg">
          <line x1="10" y1="190" x2="190" y2="190" stroke="rgba(255,255,255,0.2)" stroke-width="1" />
          <line x1="10" y1="190" x2="10" y2="10" stroke="rgba(255,255,255,0.2)" stroke-width="1" />
          <circle v-for="(pt, i) in generatedPath.slice(0, 200)" :key="i" :cx="10 + (pt.x / (pathConfig.xEnd || 1)) * 180" :cy="190 - (pt.y / (pathConfig.yEnd || 1)) * 180" r="2" fill="rgba(16,185,129,0.6)" />
        </svg>
        <p v-if="!generatedPath.length" style="color:var(--text-muted);font-size:0.7rem;">启动遍历后自动生成路径</p>
      </div>
      <button class="btn-primary" :disabled="busy || running" @click="startTraversal">
        启动遍历
      </button>
    </div>

    <div v-else-if="activeTab === 'execute'" class="traversal-execute">
      <div v-if="!running && !paused && !completed" class="traversal-placeholder">
        请先在路径配置中设定参数并启动遍历
      </div>
      <div v-else-if="completed" class="traversal-placeholder" style="color: var(--accent-success);">
        ✔ 全部 {{ totalPoints }} 个点位已完成
      </div>
      <template v-else>
        <div class="traversal-progress-bar">
          <div class="traversal-progress-bar__fill" :style="{ width: `${totalPoints > 0 ? (currentPoint / totalPoints) * 100 : 0}%` }" />
        </div>
        <p class="traversal-progress-text">当前点位: {{ currentPoint + 1 }} / {{ totalPoints }}</p>
        <div class="traversal-controls">
          <button v-if="running" class="btn-secondary" :disabled="busy" @click="pauseTraversal">暂停</button>
          <button v-if="paused" class="btn-primary" :disabled="busy" @click="resumeTraversal">恢复</button>
          <button v-if="running || paused" class="btn-danger" :disabled="busy" @click="stopTraversal">停止</button>
          <button v-if="running" class="btn-sm" :disabled="busy" @click="runCurrentPoint">采集当前点</button>
        </div>
      </template>
    </div>

    <div v-else-if="activeTab === 'visualize'" class="traversal-visualize">
      <div class="traversal-placeholder">
        <p>可视化区域将在后续集成 ECharts Heatmap/CrossSection/Vector 后展示</p>
      </div>
    </div>
  </div>
</template>

<style scoped>
.traversal-view {
  padding: var(--space-4);
  min-height: 0;
  overflow-y: auto;
}

.traversal-view__head {
  margin-bottom: var(--space-4);
}

.traversal-view__head h2 {
  margin: 0;
  font-size: 1.35rem;
}

.state-panel.complete {
  background: rgba(16, 185, 129, 0.08);
  border-color: rgba(16, 185, 129, 0.15);
  color: #10b981;
}

.state-panel {
  display: flex;
  align-items: flex-start;
  gap: var(--space-3);
  margin-bottom: var(--space-4);
  padding: var(--space-3);
  border-radius: 0.5rem;
  background: rgba(245, 158, 11, 0.08);
  border: 1px solid rgba(245, 158, 11, 0.15);
  color: #f59e0b;
  font-size: 0.75rem;
}

.state-panel__indicator {
  width: 0.6rem;
  height: 0.6rem;
  margin-top: 0.25rem;
  border-radius: 999px;
  background: currentColor;
  box-shadow: 0 0 12px currentColor;
  flex-shrink: 0;
}

.state-panel h3, .state-panel p { margin: 0; }
.state-panel p { margin-top: 0.25rem; color: var(--text-muted); line-height: 1.5; }

.error-text {
  margin-bottom: var(--space-3);
  color: var(--accent-danger);
  font: 700 0.75rem/1.4 var(--font-family-mono, monospace);
}

.traversal-tabs {
  display: flex;
  gap: 0.25rem;
  margin-bottom: var(--space-4);
  padding: 0.25rem;
  border-radius: 0.5rem;
  background: rgba(0, 0, 0, 0.2);
}

.traversal-tab {
  padding: 0.4rem 1rem;
  border-radius: 0.375rem;
  background: transparent;
  color: var(--text-muted);
  font-size: 0.8rem;
  font-weight: 700;
  flex: 1;
}

.traversal-tab.active {
  color: #f8fbff;
  background: rgba(16, 185, 129, 0.16);
}

.traversal-config__mode,
.traversal-config__misc {
  display: flex;
  gap: var(--space-4);
  align-items: flex-start;
  margin-bottom: var(--space-4);
  padding: var(--space-4);
  border-radius: 0.75rem;
  border: 1px solid rgba(255, 255, 255, 0.08);
  background: rgba(30, 41, 59, 0.4);
}

.traversal-config__mode h3,
.traversal-config__misc > .traversal-config__field { margin: 0; }

.traversal-config__mode-btns {
  display: flex;
  gap: 0.25rem;
  margin-top: var(--space-2);
}

.traversal-config__preview {
  margin-bottom: var(--space-4);
  padding: var(--space-4);
  border-radius: 0.75rem;
  border: 1px solid rgba(255, 255, 255, 0.08);
  background: rgba(30, 41, 59, 0.4);
}

.traversal-config__preview h3 {
  margin: 0 0 var(--space-2);
  font-size: 0.85rem;
  font-weight: 800;
  color: var(--text-muted);
  text-transform: uppercase;
  letter-spacing: 0.08em;
}

.traversal-config__svg {
  width: 100%;
  max-height: 180px;
  background: rgba(0, 0, 0, 0.2);
  border-radius: 0.4rem;
}

.traversal-config__grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(220px, 1fr));
  gap: var(--space-4);
  margin-bottom: var(--space-4);
}

.traversal-config__axis {
  padding: var(--space-4);
  border-radius: 0.75rem;
  border: 1px solid rgba(255, 255, 255, 0.08);
  background: rgba(30, 41, 59, 0.4);
}

.traversal-config__axis h3 {
  margin: 0 0 var(--space-3);
  font-size: 0.85rem;
  font-weight: 800;
  color: var(--text-muted);
  text-transform: uppercase;
  letter-spacing: 0.08em;
}

.traversal-config__field {
  margin-bottom: var(--space-3);
}

.traversal-config__field label {
  display: block;
  margin-bottom: 0.25rem;
  font-size: 0.7rem;
  font-weight: 700;
  color: var(--text-muted);
}

.traversal-config__field input {
  width: 100%;
  padding: 0.4rem 0.6rem;
  border-radius: 0.35rem;
  border: 1px solid var(--border-default);
  background: rgba(0, 0, 0, 0.2);
  color: var(--text-primary);
  font-size: 0.85rem;
}

.traversal-execute {
  padding: var(--space-4);
  border-radius: 0.75rem;
  border: 1px solid rgba(255, 255, 255, 0.08);
  background: rgba(30, 41, 59, 0.4);
}

.traversal-placeholder {
  min-height: 200px;
  display: flex;
  align-items: center;
  justify-content: center;
  color: var(--text-muted);
  font-size: 0.8rem;
}

.traversal-progress-bar {
  height: 8px;
  border-radius: 999px;
  background: rgba(148, 163, 184, 0.1);
  overflow: hidden;
  margin-bottom: var(--space-3);
}

.traversal-progress-bar__fill {
  height: 100%;
  border-radius: 999px;
  background: var(--accent-success);
  transition: width 0.3s ease;
}

.traversal-progress-text {
  margin: 0 0 var(--space-3);
  font: 700 0.8rem/1 var(--font-family-mono, monospace);
  color: var(--text-secondary);
}

.traversal-controls {
  display: flex;
  gap: 0.5rem;
}

.traversal-visualize {
  padding: var(--space-4);
  border-radius: 0.75rem;
  border: 1px solid rgba(255, 255, 255, 0.08);
  background: rgba(30, 41, 59, 0.4);
  min-height: 300px;
}

.btn-primary, .btn-secondary, .btn-danger, .btn-sm {
  min-height: 32px;
  padding: 0 0.9rem;
  border-radius: 0.4rem;
  font-size: 0.8rem;
  font-weight: 700;
}

.btn-primary { background: var(--accent-success); color: #f8fbff; }
.btn-secondary { background: rgba(245, 158, 11, 0.12); color: #f59e0b; border: 1px solid rgba(245, 158, 11, 0.25); }
.btn-danger { background: rgba(244, 63, 94, 0.12); color: var(--accent-danger); border: 1px solid rgba(244, 63, 94, 0.25); }
.btn-sm { min-height: 28px; padding: 0 0.6rem; font-size: 0.7rem; background: rgba(255, 255, 255, 0.06); color: var(--text-secondary); border: 1px solid rgba(255, 255, 255, 0.1); }
</style>
