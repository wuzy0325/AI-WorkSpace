import { computed, ref } from 'vue'
import { defineStore } from 'pinia'
import { useTraversalStore } from './traversalStore'
import { useDualTraversalStore } from './dualTraversalStore'
import { useFeedbackStore } from './feedbackStore'
import { useI18nStore } from './i18nStore'
import type { ProbeId, TraversalMode } from '@shared/types/traversal'

/**
 * 遍历测试模式状态（single / dual）全局 store。
 *
 * 设计原因：模式切换入口从 TraversalView 顶部按钮迁移到主导航「遍历测试」子菜单后，
 * MainDashboardView（入口）与 TraversalView（渲染分支）都需要访问同一份 mode 状态。
 * 提升到独立 store 避免 prop drilling，也保证活动检测（traversalStore + dualTraversalStore）
 * 在导航入口就能拦截，用户无需进入遍历页面才看到 toast。
 *
 * 持久化：localStorage 键 WindLabX4.traversal.mode，默认 single（与既有行为一致）。
 * 活动检测：任一 session running/moving/stabilizing/acquiring/saving/paused/isStarting
 *   时禁止切换（spec FR1），切换前清理旧模式订阅与 timer。
 */
const MODE_STORAGE_KEY = 'WindLabX4.traversal.mode'

export const useTraversalModeStore = defineStore('traversalMode', () => {
  const traversalStore = useTraversalStore()
  const dualTraversalStore = useDualTraversalStore()
  const feedbackStore = useFeedbackStore()
  const i18n = useI18nStore()

  const mode = ref<TraversalMode>(loadMode())

  function loadMode(): TraversalMode {
    try {
      const v = localStorage.getItem(MODE_STORAGE_KEY)
      return v === 'dual' ? 'dual' : 'single'
    } catch {
      // localStorage 不可用（隐私模式等）时回退到 single
      return 'single'
    }
  }

  function persistMode(value: TraversalMode): void {
    try {
      localStorage.setItem(MODE_STORAGE_KEY, value)
    } catch {
      // 持久化失败不阻断模式切换；下次启动回退到 single
    }
  }

  // 活动态判定：任一 session running/moving/stabilizing/acquiring/saving/paused
  // 时禁止切换模式（spec FR1）。
  //   - single 模式：traversalStore.canStop 覆盖 isActive 或 paused，
  //     isStarting 也算"活动"避免启动中切换造成脏状态
  //   - dual 模式：dualTraversalStore.anyActive 覆盖两路所有活动状态
  const singleActive = computed(() => traversalStore.canStop || traversalStore.isStarting)
  const dualActive = computed(() => dualTraversalStore.anyActive)

  const modeSwitchDisabled = computed(() => {
    if (mode.value === 'single') return singleActive.value
    return dualActive.value
  })

  const modeSwitchDisabledReason = computed(() => {
    if (!modeSwitchDisabled.value) return ''
    return i18n.t.traversalModeSwitchDisabled
  })

  /**
   * 切换模式：先确认当前模式所有 session 已终态并清理，再切换。
   * spec FR1：模式切换不直接删除活动 manager，终态由 registry 清理；
   * 但前端必须取消旧模式的所有订阅与 timer，避免泄漏。
   *
   * @returns true 表示切换成功（或已是目标模式），false 表示被活动检测拒绝
   */
  async function switchMode(next: TraversalMode): Promise<boolean> {
    if (next === mode.value) return true
    if (modeSwitchDisabled.value) {
      feedbackStore.pushToast(
        modeSwitchDisabledReason.value || i18n.t.traversalModeSwitchDisabled,
        'warning',
      )
      return false
    }
    // 切换前清理旧模式（spec FR1：取消订阅/timer，不主动 reset 状态）
    if (mode.value === 'dual') {
      // 离开 dual 模式：两路 session 通过 close 清理订阅/timer；
      // 再 reset 确保下一次进入 dual 时是干净状态。
      const closed = await closeDualSessions()
      if (!closed) return false
      dualTraversalStore.reset('probe1')
      dualTraversalStore.reset('probe2')
    }
    // single 模式无显式 close：traversalStore 的事件订阅/timer 在组件卸载时清理。
    // 切换模式会卸载 TraversalMain，触发 onBeforeUnmount 自动 reset。
    mode.value = next
    persistMode(next)
    return true
  }

  /**
   * 离开遍历视图时清理 dual 模式订阅/timer（spec FR1）。
   * 由 TraversalView.onBeforeUnmount 调用。
   */
  function cleanupOnLeave(): void {
    if (mode.value === 'dual') {
      dualTraversalStore.cleanupLocal('probe1')
      dualTraversalStore.cleanupLocal('probe2')
    }
  }

  async function closeDualSessions(): Promise<boolean> {
    const results = await Promise.allSettled([
      dualTraversalStore.close('probe1'),
      dualTraversalStore.close('probe2'),
    ])
    const failed = results.some((result) => result.status === 'rejected' || !result.value)
    if (!failed) return true
    // I-21 修复：部分失败时已关闭的 probe 状态已被 close() 内部清理（status=null 等），
    // 但失败的 probe 仍保留终态可重试。原代码用户重试时会陷入死循环——已关闭的 probe
    // 再调 close() 返回 false（registry 返回 already_closed 类错误），失败的 probe
    // 重试时仍可能失败。改进：对失败的 probe 显式调 cleanupLocal 释放本地资源（订阅/设备），
    // 让用户重试时 close() 是无副作用的幂等动作；同时把错误聚合到 toast。
    const probes: readonly ProbeId[] = ['probe1', 'probe2'] as const
    const details = probes
      .map((probeId, index) => {
        const result = results[index]
        if (result?.status === 'rejected') {
          // rejected 时清理本地资源，避免下次重试 close 时订阅泄漏。
          dualTraversalStore.cleanupLocal(probeId)
          return `${probeId}: ${String((result as PromiseRejectedResult).reason)}`
        }
        if (result?.status === 'fulfilled' && !result.value) {
          // close 返回 false：后端拒绝（如 already_running），保留可重试状态但清理本地资源。
          dualTraversalStore.cleanupLocal(probeId)
          return `${probeId}: ${dualTraversalStore.sessions[probeId].error ?? 'close failed'}`
        }
        return null
      })
      .filter(Boolean)
      .join('; ')
    feedbackStore.pushToast(`${i18n.t.dualCloseFailed}${details ? `: ${details}` : ''}`, 'error')
    return false
  }

  return {
    mode,
    singleActive,
    dualActive,
    modeSwitchDisabled,
    modeSwitchDisabledReason,
    switchMode,
    cleanupOnLeave,
  }
})
