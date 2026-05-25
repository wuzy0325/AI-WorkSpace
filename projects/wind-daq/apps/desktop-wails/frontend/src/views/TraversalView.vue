<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useI18nStore } from '@stores/i18nStore'
import { useFeedbackStore } from '@stores/feedbackStore'
import { traversalApi } from '@api/deviceApi'
import { useDeviceStore } from '@stores/deviceStore'

withDefaults(defineProps<{ embedded?: boolean }>(), { embedded: false })

const i18n = useI18nStore()
const feedback = useFeedbackStore()
const deviceStore = useDeviceStore()

const busy = ref(false)
const error = ref('')
const state = ref('idle')
const currentPoint = ref(0)
const totalPoints = ref(0)
const showSettings = ref(false)
const activeWorkspaceTab = ref<'preview' | 'visualize'>('preview')

const pathConfig = ref({
  xStart: 0, xEnd: 100, xStep: 10,
  yStart: 0, yEnd: 100, yStep: 10,
  zStart: 0, zEnd: 50, zStep: 10,
  deviceId: 'sim-1',
  channels: '0,1,2,3',
})
const generatedPath = ref<{ x: number; y: number; z: number }[]>([])

// 实时监控数据
const monitorData = ref({
  alpha: '--',
  beta: '--',
  attackAngle: '--',
  sideslipAngle: '--',
  mach: '--',
  velocity: '--',
  p0: '--',
  ps: '--',
  validity: '--',
})

// 实时压力数据
const pressureData = ref({
  p1: '--',
  p2: '--',
  p3: '--',
  p4: '--',
  p5: '--',
  patm: '--',
  tatm: '--',
})

const running = computed(() => state.value === 'running')
const paused = computed(() => state.value === 'paused')
const completed = computed(() => state.value === 'idle' && currentPoint.value > 0 && currentPoint.value >= totalPoints.value)
const canStart = computed(() => !running && !paused && generatedPath.value.length > 0)
const canPause = computed(() => running)
const canResume = computed(() => paused)

const statusText = computed(() => {
  if (running) return 'RUNNING'
  if (paused) return 'PAUSED'
  if (completed) return 'DONE'
  if (error.value) return 'ERROR'
  return 'IDLE'
})

const statusDotClass = computed(() => {
  if (running) return 'bg-emerald-500 shadow-[0_0_6px_#10b981]'
  if (paused) return 'bg-amber-500 animate-pulse'
  if (completed) return 'bg-blue-500 shadow-[0_0_6px_#3b82f6]'
  if (error.value) return 'bg-rose-500 shadow-[0_0_6px_#f43f5e]'
  return 'bg-slate-400'
})

const progressSummary = computed(() => `${currentPoint.value} / ${totalPoints.value}`)

async function run(action: () => Promise<void>) {
  busy.value = true; error.value = ''
  try { await action() } catch (err) { error.value = String(err); feedback.pushToast(String(err), 'error') }
  finally { busy.value = false }
}

async function refreshStatus() {
  try {
    const s = await traversalApi.status()
    state.value = s.state; currentPoint.value = s.currentPoint; totalPoints.value = s.totalPoints
  } catch { /* offline */ }
}

async function startTest() {
  await run(async () => {
    const taskId = `trav-${Date.now()}`
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
    feedback.pushToast('遍历任务已启动', 'success')
  })
}

async function pauseTest() { await run(async () => { await traversalApi.pause(); await refreshStatus(); feedback.pushToast('已暂停', 'info') }) }
async function resumeTest() { await run(async () => { await traversalApi.resume(); await refreshStatus(); feedback.pushToast('已恢复', 'success') }) }
async function stopTest() { await run(async () => { await traversalApi.stop(); await refreshStatus(); feedback.pushToast('已停止', 'info') }) }
async function runCurrentPoint() {
  await run(async () => {
    await traversalApi.runPoint(); await refreshStatus()
    if (completed.value) feedback.pushToast('遍历完成！', 'success')
  })
}

function openSettings() { showSettings.value = true }

onMounted(refreshStatus)
</script>

<template>
  <div class="flex h-full flex-col text-[color:var(--text-primary)]">
    <!-- Top Toolbar -->
    <div class="flex shrink-0 items-center justify-between border-b border-slate-200 bg-white px-5 py-3 dark:border-slate-800 dark:bg-slate-900">
      <div class="flex items-center gap-3">
        <div class="flex h-9 w-9 items-center justify-center rounded-lg bg-blue-500 text-white font-bold text-sm">T</div>
        <div>
          <h1 class="text-sm font-bold text-slate-900 dark:text-slate-100">遍历测试</h1>
          <div class="flex items-center gap-2 mt-0.5">
            <span class="flex h-1.5 w-1.5 rounded-full" :class="statusDotClass"></span>
            <p class="text-xs text-slate-400">{{ statusText }} · 预设轨迹自动采集</p>
          </div>
        </div>
      </div>

      <div class="flex flex-wrap items-center gap-2">
        <div class="flex items-center gap-3 rounded-lg border border-slate-200 bg-slate-50 px-3 py-1.5 dark:border-slate-700 dark:bg-slate-800/50">
          <div class="text-xs text-slate-500">进度</div>
          <div class="font-mono text-sm font-semibold text-blue-500">{{ progressSummary }}</div>
          <div class="h-4 w-px bg-slate-300 dark:bg-slate-600"></div>
          <div class="text-xs text-slate-500">SVG 预览</div>
          <div class="max-w-[80px] truncate font-mono text-xs text-slate-600 dark:text-slate-300">{{ generatedPath.length || 0 }} 点</div>
        </div>

        <button
          @click="openSettings"
          class="flex h-9 items-center gap-2 rounded-lg border border-slate-200 bg-white px-3 text-xs font-medium text-slate-600 transition-all hover:bg-slate-50 active:scale-95 dark:border-slate-700 dark:bg-slate-800 dark:text-slate-300"
        >
          <svg class="h-4 w-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M12.22 2h-.44a2 2 0 0 0-2 2v.18a2 2 0 0 1-1 1.73l-.43.25a2 2 0 0 1-2 0l-.15-.08a2 2 0 0 0-2.73.73l-.22.38a2 2 0 0 0 .73 2.73l.15.1a2 2 0 0 1 1 1.72v.51a2 2 0 0 1-1 1.74l-.15.09a2 2 0 0 0-.73 2.73l.22.38a2 2 0 0 0 2.73.73l.15-.08a2 2 0 0 1 2 0l.43.25a2 2 0 0 1 1 1.73V20a2 2 0 0 0 2 2h.44a2 2 0 0 0 2-2v-.18a2 2 0 0 1 1-1.73l.43-.25a2 2 0 0 1 2 0l.15.08a2 2 0 0 0 2.73-.73l.22-.39a2 2 0 0 0-.73-2.73l-.15-.08a2 2 0 0 1-1-1.74v-.5a2 2 0 0 1 1-1.74l.15-.09a2 2 0 0 0 .73-2.73l-.22-.38a2 2 0 0 0-2.73-.73l-.15.08a2 2 0 0 1-2 0l-.43-.25a2 2 0 0 1-1-1.73V4a2 2 0 0 0-2-2z"/><circle cx="12" cy="12" r="3"/></svg>
          配置
        </button>

        <div class="w-px h-5 bg-slate-200 dark:bg-slate-700"></div>

        <div class="flex items-center gap-2">
          <button
            v-if="canStart"
            class="flex h-9 items-center gap-2 rounded-lg bg-blue-500 px-4 text-xs font-semibold text-white transition-all hover:bg-blue-600 active:scale-95 disabled:opacity-40"
            :disabled="busy"
            @click="startTest"
          >
            <svg class="h-4 w-4 fill-current" viewBox="0 0 24 24"><polygon points="5 3 19 12 5 21 5 3"/></svg>
            启动
          </button>
          <template v-else-if="canPause">
            <button class="flex h-9 items-center gap-2 rounded-lg bg-amber-500 px-3 text-xs font-semibold text-white transition-all hover:bg-amber-600 active:scale-95" @click="pauseTest" :disabled="busy">
              <svg class="h-4 w-4 fill-current" viewBox="0 0 24 24"><rect x="6" y="4" width="4" height="16"/><rect x="14" y="4" width="4" height="16"/></svg>
              暂停
            </button>
            <button class="flex h-9 items-center gap-2 rounded-lg bg-rose-500 px-3 text-xs font-semibold text-white transition-all hover:bg-rose-600 active:scale-95" @click="stopTest" :disabled="busy">
              <svg class="h-4 w-4 fill-current" viewBox="0 0 24 24"><rect x="6" y="6" width="12" height="12"/></svg>
              停止
            </button>
          </template>
          <template v-else-if="canResume">
            <button class="flex h-9 items-center gap-2 rounded-lg bg-blue-500 px-3 text-xs font-semibold text-white transition-all hover:bg-blue-600 active:scale-95" @click="resumeTest" :disabled="busy">
              <svg class="h-4 w-4 fill-current" viewBox="0 0 24 24"><polygon points="5 3 19 12 5 21 5 3"/></svg>
              恢复
            </button>
            <button class="flex h-9 items-center gap-2 rounded-lg bg-rose-500 px-3 text-xs font-semibold text-white transition-all hover:bg-rose-600 active:scale-95" @click="stopTest" :disabled="busy">
              <svg class="h-4 w-4 fill-current" viewBox="0 0 24 24"><rect x="6" y="6" width="12" height="12"/></svg>
              停止
            </button>
          </template>
        </div>
      </div>
    </div>

    <!-- Main Workspace -->
    <div class="flex-1 overflow-hidden p-5">
      <div class="grid h-full grid-cols-[280px_1fr] gap-4">
        <!-- Sidebar -->
        <aside class="flex min-h-0 flex-col gap-3 overflow-hidden">
          <section class="rounded-xl border border-slate-200 bg-white p-3 dark:border-slate-800 dark:bg-slate-900">
            <div class="mb-2">
              <h2 class="text-sm font-semibold">遍历配置</h2>
              <p class="text-xs text-slate-400">网格参数与设备</p>
            </div>

            <div class="mb-2 rounded-lg bg-slate-50 p-2.5 dark:bg-slate-800">
              <div class="text-[10px] text-slate-400 mb-2 font-semibold">网格范围 (mm)</div>
              <div class="space-y-1.5">
                <div v-for="axis in ['X', 'Y', 'Z']" :key="axis" class="flex items-center gap-2">
                  <span class="text-xs font-bold text-slate-500 w-4">{{ axis }}</span>
                  <input v-model.number="pathConfig[`${axis.toLowerCase()}Start` as keyof typeof pathConfig]" type="number" class="w-full rounded border border-slate-200 bg-white px-2 py-1 text-xs dark:border-slate-700 dark:bg-slate-800 dark:text-slate-200" placeholder="起始" :disabled="running || paused" />
                  <span class="text-xs text-slate-400">~</span>
                  <input v-model.number="pathConfig[`${axis.toLowerCase()}End` as keyof typeof pathConfig]" type="number" class="w-full rounded border border-slate-200 bg-white px-2 py-1 text-xs dark:border-slate-700 dark:bg-slate-800 dark:text-slate-200" placeholder="结束" :disabled="running || paused" />
                  <input v-model.number="pathConfig[`${axis.toLowerCase()}Step` as keyof typeof pathConfig]" type="number" class="w-12 rounded border border-slate-200 bg-white px-1 py-1 text-xs dark:border-slate-700 dark:bg-slate-800 dark:text-slate-200" placeholder="步长" :disabled="running || paused" />
                </div>
              </div>
            </div>

            <div class="rounded-lg bg-slate-50 p-2.5 dark:bg-slate-800">
              <div class="text-[10px] text-slate-400 mb-1.5 font-semibold">设备与通道</div>
              <div class="space-y-1.5">
                <select v-model="pathConfig.deviceId" class="w-full rounded border border-slate-200 bg-white px-2 py-1 text-xs dark:border-slate-700 dark:bg-slate-800 dark:text-slate-200" :disabled="running || paused">
                  <option v-for="d in deviceStore.profiles" :key="d.id" :value="d.id">{{ d.name }}</option>
                </select>
                <input v-model="pathConfig.channels" type="text" class="w-full rounded border border-slate-200 bg-white px-2 py-1 text-xs dark:border-slate-700 dark:bg-slate-800 dark:text-slate-200" placeholder="通道: 0,1,2,3" :disabled="running || paused" />
              </div>
            </div>
          </section>

          <!-- 监控面板 -->
          <section class="rounded-xl border border-slate-200 bg-white p-3 dark:border-slate-800 dark:bg-slate-900">
            <div class="mb-2 flex items-center justify-between">
              <div>
                <h3 class="text-sm font-semibold">监控</h3>
                <p class="text-xs text-slate-400">实时计算</p>
              </div>
              <div class="flex items-center gap-2">
                <span class="flex h-2 w-2 rounded-full bg-rose-500"></span>
                <span class="text-[10px] text-slate-500">采集中</span>
                <span class="flex h-2 w-2 rounded-full bg-emerald-500"></span>
                <span class="text-[10px] text-slate-500">位移机构</span>
              </div>
            </div>
            <div class="space-y-2">
              <!-- 当前点 -->
              <div class="rounded-lg bg-slate-50 p-2 dark:bg-slate-800">
                <div class="text-[10px] text-slate-400 mb-1">当前点</div>
                <div class="grid grid-cols-2 gap-2">
                  <div class="flex items-center justify-between">
                    <span class="text-xs text-slate-500">α:</span>
                    <span class="font-mono text-xs font-semibold text-slate-700">{{ monitorData.alpha }}</span>
                  </div>
                  <div class="flex items-center justify-between">
                    <span class="text-xs text-slate-500">β:</span>
                    <span class="font-mono text-xs font-semibold text-slate-700">{{ monitorData.beta }}</span>
                  </div>
                </div>
              </div>
              <!-- 参数网格 -->
              <div class="grid grid-cols-2 gap-2">
                <div class="flex items-center justify-between rounded-lg bg-slate-50 p-2 dark:bg-slate-800">
                  <span class="text-xs text-slate-500">攻角</span>
                  <span class="font-mono text-xs font-semibold text-slate-700">{{ monitorData.attackAngle }}</span>
                </div>
                <div class="flex items-center justify-between rounded-lg bg-slate-50 p-2 dark:bg-slate-800">
                  <span class="text-xs text-slate-500">侧滑角</span>
                  <span class="font-mono text-xs font-semibold text-slate-700">{{ monitorData.sideslipAngle }}</span>
                </div>
                <div class="flex items-center justify-between rounded-lg bg-slate-50 p-2 dark:bg-slate-800">
                  <span class="text-xs text-slate-500">马赫数</span>
                  <span class="font-mono text-xs font-semibold text-slate-700">{{ monitorData.mach }}</span>
                </div>
                <div class="flex items-center justify-between rounded-lg bg-slate-50 p-2 dark:bg-slate-800">
                  <span class="text-xs text-slate-500">速度</span>
                  <span class="font-mono text-xs font-semibold text-slate-700">{{ monitorData.velocity }}</span>
                </div>
                <div class="flex items-center justify-between rounded-lg bg-slate-50 p-2 dark:bg-slate-800">
                  <span class="text-xs text-slate-500">P0</span>
                  <span class="font-mono text-xs font-semibold text-slate-700">{{ monitorData.p0 }}</span>
                </div>
                <div class="flex items-center justify-between rounded-lg bg-slate-50 p-2 dark:bg-slate-800">
                  <span class="text-xs text-slate-500">Ps</span>
                  <span class="font-mono text-xs font-semibold text-slate-700">{{ monitorData.ps }}</span>
                </div>
              </div>
              <div class="flex items-center justify-between rounded-lg bg-slate-50 p-2 dark:bg-slate-800">
                <span class="text-xs text-slate-500">有效性</span>
                <span class="font-mono text-xs font-semibold text-slate-700">{{ monitorData.validity }}</span>
              </div>
            </div>
          </section>

          <!-- 实时压力数据面板 -->
          <section class="rounded-xl border border-slate-200 bg-white p-3 dark:border-slate-800 dark:bg-slate-900">
            <div class="mb-2">
              <h3 class="text-sm font-semibold">实时压力数据</h3>
              <p class="text-xs text-slate-400">原始通道数据</p>
            </div>
            <div class="grid grid-cols-2 gap-2">
              <div class="flex items-center justify-between rounded-lg border border-slate-200 bg-white px-2 py-1.5 dark:border-slate-700 dark:bg-slate-800">
                <span class="text-xs font-medium text-slate-600">P1</span>
                <span class="font-mono text-xs font-semibold text-blue-500">{{ pressureData.p1 }}</span>
              </div>
              <div class="flex items-center justify-between rounded-lg border border-slate-200 bg-white px-2 py-1.5 dark:border-slate-700 dark:bg-slate-800">
                <span class="text-xs font-medium text-slate-600">P2</span>
                <span class="font-mono text-xs font-semibold text-blue-500">{{ pressureData.p2 }}</span>
              </div>
              <div class="flex items-center justify-between rounded-lg border border-slate-200 bg-white px-2 py-1.5 dark:border-slate-700 dark:bg-slate-800">
                <span class="text-xs font-medium text-slate-600">P3</span>
                <span class="font-mono text-xs font-semibold text-blue-500">{{ pressureData.p3 }}</span>
              </div>
              <div class="flex items-center justify-between rounded-lg border border-slate-200 bg-white px-2 py-1.5 dark:border-slate-700 dark:bg-slate-800">
                <span class="text-xs font-medium text-slate-600">P4</span>
                <span class="font-mono text-xs font-semibold text-blue-500">{{ pressureData.p4 }}</span>
              </div>
              <div class="flex items-center justify-between rounded-lg border border-slate-200 bg-white px-2 py-1.5 dark:border-slate-700 dark:bg-slate-800">
                <span class="text-xs font-medium text-slate-600">P5</span>
                <span class="font-mono text-xs font-semibold text-blue-500">{{ pressureData.p5 }}</span>
              </div>
              <div class="flex items-center justify-between rounded-lg border border-slate-200 bg-white px-2 py-1.5 dark:border-slate-700 dark:bg-slate-800">
                <span class="text-xs font-medium text-slate-600">Patm</span>
                <span class="font-mono text-xs font-semibold text-blue-500">{{ pressureData.patm }}</span>
              </div>
              <div class="flex items-center justify-between rounded-lg border border-slate-200 bg-white px-2 py-1.5 dark:border-slate-700 dark:bg-slate-800">
                <span class="text-xs font-medium text-slate-600">Tatm</span>
                <span class="font-mono text-xs font-semibold text-blue-500">{{ pressureData.tatm }}</span>
              </div>
            </div>
          </section>

          <section class="rounded-xl border border-slate-200 bg-white p-3 dark:border-slate-800 dark:bg-slate-900">
            <div class="mb-2">
              <h3 class="text-sm font-semibold">执行状态</h3>
              <p class="text-xs text-slate-400">实时点位进度</p>
            </div>
            <div class="mb-2 rounded-lg bg-slate-50 p-2.5 dark:bg-slate-800">
              <div class="mb-1 text-[10px] text-slate-400">当前点位</div>
              <div class="font-mono text-sm font-semibold text-blue-500">{{ running || paused ? `${currentPoint + 1} / ${totalPoints}` : '--' }}</div>
            </div>
            <div class="grid grid-cols-2 gap-1.5">
              <div class="flex items-center justify-between rounded-lg bg-slate-50 p-2 dark:bg-slate-800">
                <span class="text-xs text-slate-500">状态</span>
                <span class="font-mono text-xs font-semibold" :class="running ? 'text-emerald-500' : paused ? 'text-amber-500' : 'text-slate-400'">{{ statusText }}</span>
              </div>
              <div class="flex items-center justify-between rounded-lg bg-slate-50 p-2 dark:bg-slate-800">
                <span class="text-xs text-slate-500">采集</span>
                <button v-if="running" class="text-xs font-semibold text-blue-500 hover:text-blue-700" :disabled="busy" @click="runCurrentPoint">运行</button>
                <span v-else class="font-mono text-xs text-slate-400">--</span>
              </div>
            </div>
          </section>
        </aside>

        <!-- Main Content -->
        <div class="flex min-h-0 flex-1 flex-col overflow-hidden">
          <div class="min-h-0 flex-1">
            <section class="h-full flex flex-col rounded-xl border border-slate-200 bg-white p-4 dark:border-slate-800 dark:bg-slate-900">
              <div class="mb-3 flex flex-wrap items-center justify-between gap-3">
                <div>
                  <h3 class="text-sm font-semibold">{{ activeWorkspaceTab === 'preview' ? '点位预览' : '可视化' }}</h3>
                  <p class="text-xs text-slate-400">{{ activeWorkspaceTab === 'preview' ? '路径拓扑图' : '需集成 ECharts 扩展' }}</p>
                </div>
                <div class="flex rounded-lg border border-slate-200 bg-slate-50 p-1 dark:border-slate-700 dark:bg-slate-800">
                  <button
                    class="rounded-md px-3 py-1.5 text-xs font-semibold transition-colors"
                    :class="activeWorkspaceTab === 'preview' ? 'bg-blue-500 text-white' : 'text-slate-500 hover:text-slate-700 dark:hover:text-slate-200'"
                    @click="activeWorkspaceTab = 'preview'"
                  >点位预览</button>
                  <button
                    class="rounded-md px-3 py-1.5 text-xs font-semibold transition-colors"
                    :class="activeWorkspaceTab === 'visualize' ? 'bg-blue-500 text-white' : 'text-slate-500 hover:text-slate-700 dark:hover:text-slate-200'"
                    @click="activeWorkspaceTab = 'visualize'"
                  >可视化</button>
                </div>
              </div>

              <div class="flex-1 overflow-hidden rounded-xl border border-slate-200 bg-slate-50 dark:border-slate-700 dark:bg-slate-800 relative">
                <template v-if="activeWorkspaceTab === 'preview'">
                  <div v-if="generatedPath.length > 0" class="h-full flex flex-col p-4">
                    <!-- 图例 -->
                    <div class="flex items-center justify-end gap-3 mb-2">
                      <div class="flex items-center gap-1">
                        <span class="h-2 w-2 rounded-full bg-blue-500"></span>
                        <span class="text-[10px] text-slate-500">移动</span>
                      </div>
                      <div class="flex items-center gap-1">
                        <span class="h-2 w-2 rounded-full bg-amber-400"></span>
                        <span class="text-[10px] text-slate-500">稳定</span>
                      </div>
                      <div class="flex items-center gap-1">
                        <span class="h-2 w-2 rounded-full bg-emerald-500"></span>
                        <span class="text-[10px] text-slate-500">采集</span>
                      </div>
                      <div class="flex items-center gap-1">
                        <span class="h-2 w-2 rounded-full bg-purple-500"></span>
                        <span class="text-[10px] text-slate-500">完成</span>
                      </div>
                      <div class="flex items-center gap-1">
                        <span class="h-2 w-2 rounded-full bg-slate-300"></span>
                        <span class="text-[10px] text-slate-500">未测</span>
                      </div>
                    </div>
                    <!-- SVG 画布 -->
                    <div class="flex-1 relative">
                      <svg :viewBox="`${pathConfig.xStart - 5} ${pathConfig.yStart - 5} ${Math.max(pathConfig.xEnd - pathConfig.xStart + 10, 1)} ${Math.max(pathConfig.yEnd - pathConfig.yStart + 10, 1)}`" class="w-full h-full" preserveAspectRatio="xMidYMid meet">
                        <!-- 背景网格 -->
                        <defs>
                          <pattern :id="'grid-' + pathConfig.xStep" :width="pathConfig.xStep" :height="pathConfig.yStep" patternUnits="userSpaceOnUse">
                            <path :d="`M ${pathConfig.xStep} 0 L 0 0 0 ${pathConfig.yStep}`" fill="none" stroke="rgba(148,163,184,0.15)" stroke-width="0.5"/>
                          </pattern>
                        </defs>
                        <rect :x="pathConfig.xStart" :y="pathConfig.yStart" :width="pathConfig.xEnd - pathConfig.xStart" :height="pathConfig.yEnd - pathConfig.yStart" :fill="`url(#grid-${pathConfig.xStep})`" />

                        <!-- 主网格线 -->
                        <line v-for="x in Math.floor((pathConfig.xEnd - pathConfig.xStart) / pathConfig.xStep) + 1" :key="'vx'+x"
                          :x1="pathConfig.xStart + (x - 1) * pathConfig.xStep" :y1="pathConfig.yStart"
                          :x2="pathConfig.xStart + (x - 1) * pathConfig.xStep" :y2="pathConfig.yEnd"
                          stroke="rgba(148,163,184,0.25)" stroke-width="0.5"
                        />
                        <line v-for="y in Math.floor((pathConfig.yEnd - pathConfig.yStart) / pathConfig.yStep) + 1" :key="'vy'+y"
                          :x1="pathConfig.xStart" :y1="pathConfig.yStart + (y - 1) * pathConfig.yStep"
                          :x2="pathConfig.xEnd" :y2="pathConfig.yStart + (y - 1) * pathConfig.yStep"
                          stroke="rgba(148,163,184,0.25)" stroke-width="0.5"
                        />

                        <!-- 中心十字线 -->
                        <line :x1="(pathConfig.xStart + pathConfig.xEnd) / 2" :y1="pathConfig.yStart" :x2="(pathConfig.xStart + pathConfig.xEnd) / 2" :y2="pathConfig.yEnd" stroke="rgba(59,130,246,0.3)" stroke-width="1" />
                        <line :x1="pathConfig.xStart" :y1="(pathConfig.yStart + pathConfig.yEnd) / 2" :x2="pathConfig.xEnd" :y2="(pathConfig.yStart + pathConfig.yEnd) / 2" stroke="rgba(59,130,246,0.3)" stroke-width="1" />

                        <!-- 连接路径线 -->
                        <line v-for="(pt, i) in generatedPath.slice(0, -1)" :key="'l'+i"
                          :x1="pt.x" :y1="pt.y"
                          :x2="generatedPath[i+1].x" :y2="generatedPath[i+1].y"
                          stroke="rgba(148,163,184,0.3)" stroke-width="0.8"
                        />

                        <!-- 点位圆圈 -->
                        <circle v-for="(pt, i) in generatedPath" :key="i"
                          :cx="pt.x" :cy="pt.y" r="1.8"
                          :fill="i < currentPoint ? '#10b981' : i === currentPoint && running ? '#3b82f6' : 'rgba(148,163,184,0.5)'"
                          :stroke="i === currentPoint && running ? '#3b82f6' : 'none'"
                          stroke-width="0.5"
                        />

                        <!-- 当前点位高亮 -->
                        <circle v-if="running && currentPoint < generatedPath.length"
                          :cx="generatedPath[currentPoint]?.x" :cy="generatedPath[currentPoint]?.y" r="4"
                          fill="none" stroke="#3b82f6" stroke-width="1"
                          class="animate-pulse"
                        />
                      </svg>
                    </div>
                    <!-- 坐标轴标签 -->
                    <div class="flex items-center justify-between mt-1 px-1">
                      <span class="text-[10px] text-slate-400">X: {{ pathConfig.xStart }} ~ {{ pathConfig.xEnd }}</span>
                      <span class="text-[10px] text-slate-400">Y: {{ pathConfig.yStart }} ~ {{ pathConfig.yEnd }}</span>
                    </div>
                  </div>
                  <div v-else class="flex h-full w-full flex-col items-center justify-center gap-3 text-center">
                    <div class="text-4xl opacity-30">📐</div>
                    <div class="text-xs text-slate-400">未配置路径</div>
                    <p class="text-[10px] text-slate-400 px-8">在左侧边栏设定网格参数后点击「启动」生成遍历路径</p>
                  </div>
                </template>
                <div v-else class="h-full flex items-center justify-center">
                  <p class="text-xs text-slate-400">可视化（Heatmap / Vector / CrossSection）需额外 ECharts 组件集成</p>
                </div>
              </div>
            </section>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>
