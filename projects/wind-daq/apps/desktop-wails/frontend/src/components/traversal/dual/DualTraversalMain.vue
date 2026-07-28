/**
 * ============================================================================
 * DualTraversalMain — 双探针并行遍历主画面（spec FR7 / Task 19）
 * ============================================================================
 *
 * 【功能定位】
 * 双探针模式下的整体布局容器。模式开关由 TraversalView 持有（spec FR1），
 * 本组件仅负责 dual 模式下的双列布局与生命周期管理：
 *   - 顶部 DualStatusBar：两 probe 摘要并排展示
 *   - 中部左右排列两个 DualProbeRow：每列包含紧凑监测 + Tab 详情区
 *
 * 【隔离设计】
 * - 通过 useDualTraversalStore 的 keyed session（key 为 ProbeId 的 Record）
 *   实现 probe1/probe2 完全隔离：一路 reset/失败/unmount 不影响另一路。
 * - 模式切换时由 TraversalView 触发 dualStore.reset(probe1) + reset(probe2)
 *   清理订阅/timer/状态；本组件不直接处理模式切换。
 *
 * 【布局约束】
 * - 1440x900 / 1600x900 桌面固定布局，无移动端 breakpoint（spec FR7）；
 * - 双列不重叠、不溢出；
 * - 每 row 的控制栏与 Warning 固定不滚动，详情区可独立滚动。
 *
 * @module DualTraversalMain
 * @see DualStatusBar — 顶部双列摘要
 * @see DualProbeRow — 单 probe 完整 row（紧凑监测 + Tab 详情）
 * ============================================================================
 */
<script setup lang="ts">
import { computed, onMounted, onUnmounted } from 'vue'
import { storeToRefs } from 'pinia'

import DualStatusBar from './DualStatusBar.vue'
import DualProbeRow from './DualProbeRow.vue'

import { useDualTraversalStore } from '@stores/dualTraversalStore'
import { useI18nStore } from '@stores/i18nStore'
import { useFeedbackStore } from '@stores/feedbackStore'
import type { ProbeId, TraversalTestConfig } from '@shared/types/traversal'

const emit = defineEmits<{
  /** 打开指定 probe 的配置对话框（probe1/probe2 各自独立配置入口） */
  openSettings: [probeId: ProbeId]
  back: []
}>()

const dualStore = useDualTraversalStore()
const i18n = useI18nStore()
const feedbackStore = useFeedbackStore()
const { sessions } = storeToRefs(dualStore)
const t = computed(() => i18n.t)
let mounted = false

// 两 probe 在 onMounted 时分别恢复配置/状态/断点；任一失败不影响另一路。
onMounted(async () => {
  mounted = true
  const probeIds: ProbeId[] = ['probe1', 'probe2']
  // 每路必须先恢复配置，运行态恢复才能按持久化设备绑定重建订阅。
  const results = await Promise.allSettled(probeIds.map(async (probeId) => {
    await dualStore.loadConfig(probeId)
    if (!mounted) return
    await dualStore.recoverRuntime(probeId)
  }))
  if (!mounted) return
  // 加载状态/断点（不阻塞 UI 渲染；缺失配置时跳过）
  await Promise.allSettled([
    dualStore.loadCheckpoint('probe1'),
    dualStore.loadCheckpoint('probe2'),
  ])
  if (!mounted) return
  // 任意 probe 加载失败：toast 提示但不阻断（另一路仍可用）
  results.forEach((res, idx) => {
    if (res.status === 'rejected') {
      const probeId = probeIds[idx]
      const label = probeId === 'probe1' ? t.value.probe1Label : t.value.probe2Label
      feedbackStore.pushToast(
        `${label} ${t.value.dualStartFailed}：${res.reason instanceof Error ? res.reason.message : String(res.reason)}`,
        'warning',
      )
    }
  })
})

onUnmounted(() => {
  mounted = false
  dualStore.cleanupLocal('probe1')
  dualStore.cleanupLocal('probe2')
})

/** 打开指定 probe 的配置对话框 */
function onOpenSettings(probeId: ProbeId): void {
  emit('openSettings', probeId)
}

/** 双控制器冲突检测：probe1 与 probe2 必须绑定不同运动控制器（spec FR1） */
const controllerConflict = computed(() => {
  const cfg1 = sessions.value.probe1.config
  const cfg2 = sessions.value.probe2.config
  if (!cfg1 || !cfg2) return false
  const ids1 = new Set(extractControllerIds(cfg1))
  const ids2 = new Set(extractControllerIds(cfg2))
  for (const id of ids1) {
    if (id && ids2.has(id)) return true
  }
  return false
})

function extractControllerIds(cfg: TraversalTestConfig): string[] {
  const ids: string[] = []
  // TraversalTestConfig.channels.motionAxes 是绑定数组（不是顶层 motionAxes）
  for (const axis of cfg.channels?.motionAxes ?? []) {
    if (axis.controllerId) ids.push(axis.controllerId)
  }
  return ids
}
</script>

<template>
  <div
    class="dual-main"
    :data-test="'dual-traversal-main'"
  >
    <!-- 顶部双列摘要状态栏 -->
    <div class="dual-main__status-bar">
      <DualStatusBar probe-id="probe1" />
      <DualStatusBar probe-id="probe2" />
    </div>

    <!-- 双控制器冲突提示（spec FR1：两个探针必须绑定不同运动控制器） -->
    <div
      v-if="controllerConflict"
      class="dual-main__conflict-banner"
      role="alert"
    >
      <span class="dual-main__conflict-icon" aria-hidden="true">⚠</span>
      <span class="dual-main__conflict-text">{{ t.dualControllerConflict }}</span>
    </div>

    <!-- 主体：左右排列两个 DualProbeRow；每列固定 min-width:0 避免内容撑破 flex 行 -->
    <div class="dual-main__rows">
      <DualProbeRow probe-id="probe1" @open-settings="onOpenSettings" />
      <DualProbeRow probe-id="probe2" @open-settings="onOpenSettings" />
    </div>
  </div>
</template>

<style scoped>
.dual-main {
  display: flex;
  flex-direction: column;
  /* 关键：作为 TraversalView flex 行的子项，必须 flex-1 + width 100% 才能撑满父容器，
     否则默认 flex: 0 1 auto 会导致宽度由内容收缩决定，右侧出现大片空白。 */
  flex: 1 1 0;
  width: 100%;
  height: 100%;
  min-width: 0;
  min-height: 0;
  gap: 8px;
  padding: 8px 12px;
  background: var(--bg-canvas, #f7f7f8);
  color: var(--text-primary, #1f1f1f);
  overflow: hidden;
}

.dual-main__status-bar {
  display: flex;
  gap: 12px;
  flex: 0 0 auto;
}

.dual-main__conflict-banner {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px 12px;
  background: var(--color-warning-bg, #fff8e1);
  color: var(--color-warning, #f0a020);
  border: 1px solid var(--color-warning, #f0a020);
  border-radius: 6px;
  font-size: 13px;
  flex: 0 0 auto;
}

.dual-main__conflict-icon {
  font-weight: 700;
  flex-shrink: 0;
}

.dual-main__conflict-text {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.dual-main__rows {
  display: flex;
  flex-direction: row;
  gap: 12px;
  flex: 1 1 0;
  min-height: 0;
  overflow: hidden;
}
</style>
