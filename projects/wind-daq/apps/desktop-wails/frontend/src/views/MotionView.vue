<script setup lang="ts">
import { ref, onMounted, onBeforeUnmount } from 'vue'
import { useI18nStore } from '@stores/i18nStore'
import { useFeedbackStore } from '@stores/feedbackStore'
import UiButton from '@components/ui/UiButton.vue'
import { motionApi, type MotionStatus } from '@api/deviceApi'

const i18n = useI18nStore()
const feedback = useFeedbackStore()

const connected = ref(false)
const busy = ref(false)
const error = ref('')
const jogVelocity = ref(1)
const moveTarget = ref(10)
const axes = ref<MotionStatus['axes']>([])
const emergencyStopped = ref(false)

let pollTimer: ReturnType<typeof setInterval> | undefined

async function run(action: () => Promise<void>) {
  busy.value = true
  error.value = ''
  try {
    await action()
  } catch (err) {
    error.value = String(err)
    feedback.pushToast(String(err), 'error')
  } finally {
    busy.value = false
  }
}

async function pollStatus() {
  try {
    const status = await motionApi.status()
    connected.value = status.connected
    axes.value = status.axes
  } catch {
    // keep last state
  }
}

async function handleConnect() {
  await run(async () => {
    await motionApi.connect()
    connected.value = true
    feedback.pushToast('运动控制器已连接', 'success')
    pollTimer = setInterval(pollStatus, 2000)
    await pollStatus()
  })
}

async function handleDisconnect() {
  await run(async () => {
    if (pollTimer) { clearInterval(pollTimer); pollTimer = undefined }
    await motionApi.disconnect()
    connected.value = false
    feedback.pushToast('运动控制器已断开', 'info')
  })
}

async function handleMoveTo() {
  await run(async () => {
    await motionApi.moveTo('X', moveTarget.value)
    feedback.pushToast('MoveTo X=' + moveTarget.value, 'success')
    await pollStatus()
  })
}

async function handleJog(axis: string) {
  await run(async () => {
    await motionApi.jog(axis, jogVelocity.value)
    feedback.pushToast('Jog ' + axis, 'success')
    await pollStatus()
  })
}

async function handleStop() {
  await run(async () => { await motionApi.stop(); await pollStatus() })
}

async function handleEmergencyStop() {
  await run(async () => {
    await motionApi.emergencyStop()
    emergencyStopped.value = true
    feedback.pushToast('紧急停止已触发', 'error')
  })
}

async function handleHome() {
  await run(async () => { await motionApi.home(); await pollStatus() })
}

onMounted(async () => {
  try { const s = await motionApi.status(); connected.value = s.connected; axes.value = s.axes } catch { /* offline */ }
})

onBeforeUnmount(() => {
  if (pollTimer) clearInterval(pollTimer)
})
</script>

<template>
  <div class="motion-view">
    <div class="motion-view__head">
      <p class="eyebrow">{{ i18n.t.motionControl }}</p>
      <h2>{{ i18n.t.motionControl }}</h2>
    </div>

    <p v-if="error" class="error-text">{{ error }}</p>

    <section class="state-panel" :class="{ 'state-panel--ready': connected }">
      <div class="state-panel__indicator" />
      <div>
        <h3>{{ connected ? '运动控制器已连接' : '运动控制器未连接' }}</h3>
        <p>{{ connected ? '后端 SimulatedMotionController 已就绪，可通过 API 控制。' : '点击连接以启用运动控制功能。' }}</p>
      </div>
    </section>

    <div class="motion-view__grid">
      <section class="motion-panel">
        <h3>控制器</h3>
        <div class="motion-panel__status">
          <span class="motion-status" :class="{ connected }">
            {{ connected ? 'Connected' : 'Disconnected' }}
          </span>
        </div>
        <div class="motion-panel__actions">
          <UiButton variant="primary" :disabled="busy || connected" @click="handleConnect">{{ i18n.t.connect }}</UiButton>
          <UiButton variant="danger" :disabled="busy || !connected" @click="handleDisconnect">{{ i18n.t.disconnect }}</UiButton>
        </div>
      </section>

      <section class="motion-panel">
        <h3>轴状态</h3>
        <div class="motion-axes">
          <div v-for="axis in axes" :key="axis.name" class="motion-axis">
            <span class="motion-axis__name">{{ axis.name }}</span>
            <span class="motion-axis__pos">{{ axis.position.toFixed(2) }}</span>
            <span class="motion-axis__status" :class="{ moving: axis.moving, homed: axis.homed }">
              {{ axis.moving ? 'MOV' : axis.homed ? 'OK' : '--' }}
            </span>
          </div>
          <p v-if="!axes.length" class="motion-hint">未连接或无数据</p>
        </div>
      </section>

      <section class="motion-panel">
        <h3>Jog 运动</h3>
        <div class="motion-panel__field">
          <label>Velocity</label>
          <input v-model.number="jogVelocity" type="number" step="0.1" min="0.1" />
        </div>
        <div class="motion-panel__actions">
          <UiButton v-for="axis in ['X', 'Y', 'Z']" :key="axis" variant="secondary" size="sm" :disabled="!connected || busy" @click="handleJog(axis)">{{ axis }}+</UiButton>
        </div>
      </section>

      <section class="motion-panel">
        <h3>MoveTo</h3>
        <div class="motion-panel__field">
          <label>Target X</label>
          <input v-model.number="moveTarget" type="number" step="0.5" />
        </div>
        <UiButton variant="primary" :disabled="!connected || busy" @click="handleMoveTo">Move To {{ moveTarget }}</UiButton>
      </section>

      <section class="motion-panel">
        <h3>控制</h3>
        <div class="motion-panel__actions">
          <UiButton variant="secondary" :disabled="!connected || busy" @click="handleHome">Home</UiButton>
          <UiButton variant="danger" :disabled="!connected || busy" @click="handleStop">Stop</UiButton>
        </div>
      </section>

      <section class="motion-panel">
        <h3>急停</h3>
        <UiButton variant="danger" size="lg" :disabled="!connected" @click="handleEmergencyStop">EMERGENCY STOP</UiButton>
      </section>
    </div>

    <div v-if="connected" class="motion-view__footnote">
      使用 Go HTTP API（simulated motion controller），每 2s 自动轮询状态。
    </div>
  </div>
</template>

<style scoped>
.motion-view {
  padding: var(--space-4);
  min-height: 0;
  overflow-y: auto;
}
.motion-view__head { margin-bottom: var(--space-4); }
.motion-view__head h2 { margin: 0; font-size: 1.35rem; }
.error-text { margin-bottom: var(--space-3); color: var(--accent-danger); font: 700 0.75rem/1.4 var(--font-family-mono, monospace); }
.state-panel {
  display: flex; align-items: flex-start; gap: var(--space-3); margin-bottom: var(--space-4);
  padding: var(--space-3); border-radius: 0.5rem;
  background: rgba(245, 158, 11, 0.08); border: 1px solid rgba(245, 158, 11, 0.15); color: #f59e0b; font-size: 0.75rem;
}
.state-panel--ready { background: rgba(34, 197, 94, 0.08); border-color: rgba(34, 197, 94, 0.15); color: #22c55e; }
.state-panel__indicator { width: 0.6rem; height: 0.6rem; margin-top: 0.25rem; border-radius: 999px; background: currentColor; box-shadow: 0 0 12px currentColor; flex-shrink: 0; }
.state-panel h3, .state-panel p { margin: 0; }
.state-panel p { margin-top: 0.25rem; color: var(--text-muted); line-height: 1.5; }
.motion-view__grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(280px, 1fr)); gap: var(--space-4); }
.motion-panel { padding: var(--space-4); border-radius: 0.75rem; border: 1px solid rgba(255,255,255,0.08); background: rgba(30,41,59,0.4); }
.motion-panel h3 { margin: 0 0 var(--space-3); font-size: 0.85rem; font-weight: 800; color: var(--text-muted); text-transform: uppercase; letter-spacing: 0.08em; }
.motion-panel__status { margin-bottom: var(--space-3); }
.motion-status { display: inline-flex; padding: 0.2rem 0.6rem; border-radius: 999px; font-size: 0.7rem; font-weight: 700; background: rgba(148,163,184,0.1); color: var(--text-muted); }
.motion-status.connected { background: rgba(34,197,94,0.1); color: #22c55e; }
.motion-panel__actions { display: flex; gap: 0.5rem; flex-wrap: wrap; }
.motion-panel__field { margin-bottom: var(--space-3); }
.motion-panel__field label { display: block; margin-bottom: 0.25rem; font-size: 0.7rem; font-weight: 700; color: var(--text-muted); }
.motion-panel__field input { width: 100%; padding: 0.4rem 0.6rem; border-radius: 0.35rem; border: 1px solid var(--border-default); background: rgba(0,0,0,0.2); color: var(--text-primary); font-size: 0.85rem; }
.motion-axes { display: flex; flex-direction: column; gap: var(--space-2); }
.motion-axis { display: flex; align-items: center; gap: var(--space-3); padding: var(--space-2) var(--space-3); border-radius: 0.4rem; background: rgba(0,0,0,0.15); font-family: var(--font-family-mono,monospace); font-size: 0.85rem; }
.motion-axis__name { width: 24px; font-weight: 800; color: var(--accent-primary); }
.motion-axis__pos { flex: 1; text-align: right; font-variant-numeric: tabular-nums; }
.motion-axis__status { width: 36px; text-align: center; font-size: 0.65rem; font-weight: 800; padding: 0.15rem 0.3rem; border-radius: 0.25rem; background: rgba(148,163,184,0.15); color: var(--text-muted); }
.motion-axis__status.moving { background: rgba(245,158,11,0.15); color: #f59e0b; }
.motion-axis__status.homed { background: rgba(34,197,94,0.15); color: #22c55e; }
.motion-hint { margin: 0; color: var(--text-muted); font-size: 0.75rem; }
.motion-view__footnote { margin-top: var(--space-6); padding: var(--space-3); border-radius: 0.5rem; background: rgba(16,185,129,0.08); border: 1px solid rgba(16,185,129,0.15); color: #10b981; font-size: 0.75rem; }
</style>
