<script setup lang="ts">
import { ref, computed, shallowRef, onMounted, watch, defineAsyncComponent, type Component } from 'vue'
import type { CalibrationType } from '@shared/types/calibration'
import CalibrationHome from './CalibrationHome.vue'
import UiDialog from '@components/ui/UiDialog.vue'
import { useCalibrationStore } from '@stores/calibrationStore'
import { useFeedbackStore } from '@stores/feedbackStore'
import { useI18nStore } from '@stores/i18nStore'

const settingsComponents: Record<string, Component> = {
  'five-hole': defineAsyncComponent(() => import('./five-hole/FiveHoleSettings.vue')),
  'three-hole': defineAsyncComponent(() => import('./three-hole/ThreeHoleSettings.vue')),
  'total-pressure': defineAsyncComponent(() => import('./total-pressure/TotalPressureSettings.vue')),
  'total-temperature': defineAsyncComponent(() => import('./total-temperature/TotalTemperatureSettings.vue')),
  'seven-hole': defineAsyncComponent(() => import('./seven-hole/SevenHoleSettings.vue')),
}

const mainComponents: Record<string, Component> = {
  'five-hole': defineAsyncComponent(() => import('./five-hole/FiveHoleMain.vue')),
  'three-hole': defineAsyncComponent(() => import('./three-hole/ThreeHoleMain.vue')),
  'total-pressure': defineAsyncComponent(() => import('./total-pressure/TotalPressureMain.vue')),
  'total-temperature': defineAsyncComponent(() => import('./total-temperature/TotalTemperatureMain.vue')),
  'seven-hole': defineAsyncComponent(() => import('./seven-hole/SevenHoleMain.vue')),
}

const calibrationStore = useCalibrationStore()
const feedbackStore = useFeedbackStore()
const i18n = useI18nStore()

// spec I10 / Task 6：落点由后端 status 决定，禁止落点闪烁、禁止空闲态记忆类型。
// currentView 初始为 null —— onMounted 期间显示 loading 占位或错误视图，recovery 完成后再决定落点，
// 避免挂载瞬间渲染 CalibrationHome 又立刻被替换成 Main 产生闪烁。
const currentView = shallowRef<Component | null>(null)
const showSettings = ref(false)
const activeCalibrationType = ref<CalibrationType | null>(null)
const currentMainRef = ref<{ reloadSavedConfig?: () => Promise<void> | void } | null>(null)
// 恢复失败后重试锁：防止用户短时间内多次点击重试按钮
const isRetrying = ref(false)

const currentSettings = computed(() =>
  activeCalibrationType.value ? settingsComponents[activeCalibrationType.value] : null,
)

// 探针类型 → 本地化名称映射：用于跨类型切换确认框 {from}/{to} 占位符替换
// 复用 i18n 中已有的 ch_xxxName key，避免新增重复文案
function getCalibrationTypeName(type: CalibrationType): string {
  switch (type) {
    case 'five-hole': return i18n.t.ch_fiveHoleName
    case 'three-hole': return i18n.t.ch_threeHoleName
    case 'total-pressure': return i18n.t.ch_totalPressureName
    case 'total-temperature': return i18n.t.ch_totalTemperatureName
    case 'seven-hole': return i18n.t.ch_sevenHoleName
  }
}

// spec Task 6：挂载时先 recovery，再根据后端 status 决定落点。
//   - 有任务（running/paused/completed/error/stopped）→ 对应 Main
//   - 无任务（idle / status 为 null）→ CalibrationHome
// 恢复失败时保持 currentView=null（错误视图），禁止进入 Home 触发新的校准操作。
onMounted(async () => {
  // isWailsAvailable 检查放在 store 内部 recoveryFromBackend；http 模式同样走 calibrationApi.status 兜底。
  // 这里不读 lastRecoveryAt 跳过 —— CalibrationWindow 是模块入口，每次挂载都需要 fresh recovery。
  // (Window + Main 双重 recovery 的去重由 useCalibrationWorkflow.onMounted 内的 2s 判定处理。)
  try {
    await calibrationStore.recoveryFromBackend()
  } catch (err) {
    // recoveryFromBackend 内部已捕获并写入 recoveryError，这里兜底防 unhandled rejection
    console.error('CalibrationWindow recovery failed:', err)
  }

  const status = calibrationStore.status
  // 落点决策：
  //   recoveryError 非空 → currentView=null，渲染错误视图（含重试按钮）
  //   status 非空 + 非 idle + 类型有对应 Main → 进 Main
  //   否则 → 进 Home
  if (calibrationStore.recoveryError) {
    // 恢复失败：不渲染任何可操作视图，用户需先重试或解决问题
    activeCalibrationType.value = null
    currentView.value = null
  } else if (
    status &&
    status.status !== 'idle' &&
    status.type &&
    mainComponents[status.type]
  ) {
    activeCalibrationType.value = status.type
    currentView.value = mainComponents[status.type]
  } else {
    activeCalibrationType.value = null
    currentView.value = CalibrationHome
  }
})

// 重试恢复：清错误 → 调 recoveryFromBackend → 按结果重新落点
//
// 关键约束：recoveryFromBackend 内部吞掉异常并设置 recoveryError（不抛 throw），
// 因此无需 try/catch——必须在 await 后显式检查 recoveryError：
//   - 非空 → 保持 currentView=null，渲染错误视图（含重试按钮）
//   - 否则 → 按 status 落点
// 若跳过此检查，旧 status 会被用来设置 currentView，模板 v-if 优先渲染该视图，
// 隐藏恢复错误，用户看不到重试按钮，连续两次失败也无法暴露。
async function retryRecovery() {
  if (isRetrying.value) return
  isRetrying.value = true
  try {
    calibrationStore.recoveryError = null
    await calibrationStore.recoveryFromBackend()
    // recoveryFromBackend 内部捕获异常并写入 recoveryError，不会 throw；
    // 必须在此处显式检查，否则会用旧 status 设置 currentView 隐藏恢复错误
    if (calibrationStore.recoveryError) {
      activeCalibrationType.value = null
      currentView.value = null
      return
    }
    const status = calibrationStore.status
    if (
      status &&
      status.status !== 'idle' &&
      status.type &&
      mainComponents[status.type]
    ) {
      activeCalibrationType.value = status.type
      currentView.value = mainComponents[status.type]
    } else {
      activeCalibrationType.value = null
      currentView.value = CalibrationHome
    }
  } finally {
    isRetrying.value = false
  }
}

async function handleSelectCalibration(type: CalibrationType) {
  // spec Task 7：跨类型切换拦截
  //   - 仅 running/paused 态 + 目标类型 !== 当前任务类型 → 弹确认框
  //   - 同类型卡片重复点击（如五孔 Main → Home → 再点五孔）不拦截，直接进 Main
  //   - completed/error/stopped/idle 态直接切换，不弹框
  //   - handleBack 不走此路径，不拦截、不 stop（任务继续后台 1Hz 心跳）
  if (
    (calibrationStore.isRunning || calibrationStore.isPaused) &&
    calibrationStore.status?.type &&
    type !== calibrationStore.status.type
  ) {
    const fromName = getCalibrationTypeName(calibrationStore.status.type)
    const toName = getCalibrationTypeName(type)
    const message = i18n.t.wf_switchTypeConfirm
      .replace('{from}', fromName)
      .replace('{to}', toName)
    const accepted = await feedbackStore.confirm(message, {
      title: i18n.t.wf_switchTypeTitle,
      confirmText: i18n.t.wf_stopAndSwitch,
      cancelText: i18n.t.cancel,
    })
    if (!accepted) {
      // 取消：留在当前画面，任务继续
      return
    }
    // 确认：先 stop 再切换。stop 失败时重新查询后端状态——若原任务仍在运行则阻断切换，
    // 避免用户以为任务已停但实际上还在后台运行，导致在错误界面操作运行中的任务。
    try {
      await calibrationStore.stop()
    } catch (err) {
      console.error('Failed to stop calibration before switching type:', err)
      feedbackStore.pushToast(
        i18n.t.wf_stopCalibrationFailed + ': ' + (err instanceof Error ? err.message : String(err)),
        'error',
      )
      // stop 失败：重新查询后端状态确认原任务是否仍在运行
      await calibrationStore.recoveryFromBackend()
      const originalTask = calibrationStore.status
      if (originalTask && (calibrationStore.isRunning || calibrationStore.isPaused)) {
        // 原任务仍在运行——阻断切换，保持在当前画面
        return
      }
      // 原任务已自行结束（recovery 显示 idle/completed/stopped）——允许切换
    }
  }

  // 跨类型切换：清掉上一趟会话残留状态。
  // 根因：completed/error/stopped 终态下 store.status / dataPoints / timeInfo 仍保留上一类型的数据，
  //   而三孔/总压/总温 Main 的 progressInfo、formattedTimeInfo、completedPoints 直接读这些全局 ref，
  //   不 reset 会把五孔的进度/已用时间/数据点带到三孔画面（同类型切换不 reset，保留结果供导出）。
  // 注意：后端 status 不动——后端 completed 任务作为历史保留，下次进 Main 仍可看到；
  //   这里只清前端 store 的会话快照，recovery 下次进入时会按后端真实状态重新同步。
  const previousType = calibrationStore.status?.type
  if (previousType && previousType !== type) {
    calibrationStore.reset()
  }

  activeCalibrationType.value = type
  currentView.value = mainComponents[type]
}

function handleBack() {
  // spec Task 7：Main 返回 Home 不拦截、不 stop。
  // 仅 running/paused 态任务继续后台 1Hz 心跳（releaseView 已由 composable.onBeforeUnmount 处理降频），
  // Home 卡片显示进行中标识（Task 8）。
  activeCalibrationType.value = null
  showSettings.value = false
  currentView.value = CalibrationHome
}

function handleOpenSettings() {
  showSettings.value = true
}

function handleCloseSettings() {
  showSettings.value = false
}

async function handleSettingsSaved() {
  showSettings.value = false
  await currentMainRef.value?.reloadSavedConfig?.()
}

// 校准完成模态提示框：
// 通过 store 的 completionSignal（仅新鲜完成时自增一次）驱动，覆盖全部探针校准模块。
// 相比直接 watch completeEvent，能避免 recovery / 重进已完成任务时重复弹出。
const showCompletionDialog = ref(false)
const completionMessage = ref('')

watch(
  () => calibrationStore.completionSignal,
  () => {
    const ev = calibrationStore.completeEvent
    if (!ev || !ev.success) return
    // 用 status.type 而非 activeCalibrationType：用户可能已从 Main 返回 Home（后台 1Hz 心跳仍在跑），
    // 此时 activeCalibrationType 为 null，但 status.type 仍保留任务类型，能正确显示提示框文案。
    const typeName = calibrationStore.status?.type ? getCalibrationTypeName(calibrationStore.status.type) : ''
    const durationSec = Math.max(0, Math.round((ev.duration || 0) / 1000))
    const minutes = Math.floor(durationSec / 60)
    const seconds = durationSec % 60
    const duration = `${String(minutes).padStart(2, '0')}:${String(seconds).padStart(2, '0')}`
    completionMessage.value = i18n.t.calCompleteBody
      .replace('{type}', typeName)
      .replace('{total}', String(ev.totalPoints ?? 0))
      .replace('{duration}', duration)
    showCompletionDialog.value = true
  },
)

function closeCompletionDialog() {
  showCompletionDialog.value = false
}

</script>

<template>
  <div class="flex-1 min-h-0 w-full flex flex-col">
    <!-- spec Task 6：currentView=null 期间显示 loading 占位，避免 Home 闪烁 -->
    <component
      v-if="currentView"
      ref="currentMainRef"
      :is="currentView"
      @select-calibration="handleSelectCalibration"
      @back="handleBack"
      @open-settings="handleOpenSettings"
    />
    <div v-else-if="calibrationStore.recoveryError" class="flex-1 min-h-0 w-full flex items-center justify-center">
      <div class="flex flex-col items-center gap-4 text-center px-6">
        <div class="w-12 h-12 rounded-full bg-[var(--danger-muted)] flex items-center justify-center">
          <svg class="w-6 h-6 text-[var(--accent-danger)]" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 9v2m0 4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z"/></svg>
        </div>
        <h3 class="text-base font-semibold text-[var(--text-primary)]">{{ i18n.t.wf_recoveryFailedTitle }}</h3>
        <p class="text-sm text-[var(--text-muted)] max-w-md">{{ i18n.t.wf_recoveryFailed }}: {{ calibrationStore.recoveryError }}</p>
        <div class="flex gap-3 mt-2">
          <button class="retry-btn" :disabled="isRetrying" @click="retryRecovery">
            <svg v-if="isRetrying" class="animate-spin w-4 h-4 mr-1.5" fill="none" viewBox="0 0 24 24"><circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"/><path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z"/></svg>
            {{ isRetrying ? i18n.t.wf_recovering : i18n.t.set_retry }}
          </button>
        </div>
      </div>
    </div>
    <div v-else class="flex-1 min-h-0 w-full flex items-center justify-center">
      <div class="flex flex-col items-center gap-3">
        <div class="animate-spin w-8 h-8 border-2 border-[var(--accent-primary)] border-t-transparent rounded-full" />
        <p class="text-sm text-[var(--text-muted)]">{{ i18n.t.wf_recovering }}</p>
      </div>
    </div>

    <Suspense v-if="showSettings && currentSettings">
      <component
        :is="currentSettings"
        @close="handleCloseSettings"
        @saved="handleSettingsSaved"
      />
      <template #fallback>
        <div class="fixed inset-0 z-50 flex items-center justify-center bg-black/30">
          <div class="animate-spin w-8 h-8 border-2 border-[var(--accent-primary)] border-t-transparent rounded-full"></div>
        </div>
      </template>
    </Suspense>

    <!-- 校准完成模态提示框：各探针模块校准完成后统一弹出，显著提醒操作员 -->
    <UiDialog
      :show="showCompletionDialog"
      :title="i18n.t.calCompleteTitle"
      width="min(88vw, 420px)"
      closable
      @update:show="closeCompletionDialog"
    >
      <div class="flex items-start gap-4 py-2">
        <div class="flex h-11 w-11 flex-shrink-0 items-center justify-center rounded-full" :style="{ background: 'color-mix(in srgb, var(--accent-success) 15%, transparent)' }">
          <svg class="h-6 w-6" :style="{ color: 'var(--accent-success)' }" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
            <path stroke-linecap="round" stroke-linejoin="round" d="M5 13l4 4L19 7" />
          </svg>
        </div>
        <div class="min-w-0 flex-1">
          <p class="text-sm leading-relaxed" :style="{ color: 'var(--text-primary)' }">{{ completionMessage }}</p>
        </div>
      </div>
      <template #footer>
        <div class="flex justify-end">
          <button class="complete-ok-btn" @click="closeCompletionDialog">{{ i18n.t.calCompleteConfirm }}</button>
        </div>
      </template>
    </UiDialog>
  </div>
</template>

<style scoped>
.retry-btn {
  display: inline-flex;
  align-items: center;
  padding: 0.5rem 1.25rem;
  border-radius: 0.5rem;
  font-size: 0.875rem;
  font-weight: 600;
  border: 1px solid var(--accent-primary);
  color: var(--accent-primary);
  background: color-mix(in srgb, var(--accent-primary) 10%, transparent);
  cursor: pointer;
  transition: background 0.15s, opacity 0.15s;
}
.retry-btn:hover:not(:disabled) {
  background: color-mix(in srgb, var(--accent-primary) 20%, transparent);
}
.retry-btn:disabled {
  opacity: 0.6;
  cursor: not-allowed;
}
.complete-ok-btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  padding: 0.45rem 1.5rem;
  border-radius: 0.5rem;
  font-size: 0.875rem;
  font-weight: 600;
  color: #fff;
  background: var(--accent-success);
  cursor: pointer;
  transition: opacity 0.15s;
}
.complete-ok-btn:hover {
  opacity: 0.88;
}
</style>
