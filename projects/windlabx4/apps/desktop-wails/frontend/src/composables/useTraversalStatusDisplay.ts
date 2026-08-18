// 遍历测试状态文本与点样式
//
// 把 TraversalMain.vue 中"根据 traversalStore 状态推导显示文案/点样式"的纯映射逻辑
// 抽到 composable。语义上它属于 traversal 视图层，但与 store 无写入耦合，
// 所以放在 composables/ 而不是 store 内部。
//
// isStartRequestPending 是 TraversalMain 用来防重入启动的本地 ref，作为参数传入，
// 让 composable 不感知组件具体怎么管理这个标志。
import { computed, type Ref } from 'vue'
import { useTraversalStore } from '@stores/traversalStore'
import { useI18nStore } from '@stores/i18nStore'

/**
 * 状态点配色：与 traversalStore.statusType 一一对应。
 * 子状态（moving/stabilizing/acquiring/saving）共用 running 的绿色，
 * 因为它们都属于 "正在跑" 的语义。
 */
const STATUS_DOT_CLASS: Record<string, string> = {
  running: 'bg-emerald-500 shadow-[0_0_6px_#10b981]',
  moving: 'bg-emerald-500 shadow-[0_0_6px_#10b981]',
  stabilizing: 'bg-emerald-500 shadow-[0_0_6px_#10b981]',
  acquiring: 'bg-emerald-500 shadow-[0_0_6px_#10b981]',
  saving: 'bg-emerald-500 shadow-[0_0_6px_#10b981]',
  paused: 'bg-amber-500 animate-pulse',
  completed: 'bg-blue-500 shadow-[0_0_6px_#3b82f6]',
  error: 'bg-rose-500 shadow-[0_0_6px_#f43f5e]',
  stopped: 'bg-amber-500',
  unknown: 'bg-rose-500 shadow-[0_0_6px_#f43f5e]',
  idle: 'bg-slate-400'
}

/**
 * 遍历测试状态显示
 *
 * @param isStartRequestPending 启动按钮防重入标志，true 时强制显示 "Starting..."
 */
export function useTraversalStatusDisplay(isStartRequestPending: Ref<boolean>) {
  const traversalStore = useTraversalStore()
  const i18n = useI18nStore()
  const t = computed(() => i18n.t)

  // 状态文本：优先显示子状态（moving/stabilizing/acquiring/saving），其次显示主状态
  const statusText = computed(() => {
    const phase = traversalStore.status?.currentPointPhase
    // 启动中或运行中：根据子阶段细化文案
    if (isStartRequestPending.value || traversalStore.isStarting) {
      return t.value.statusStarting
    }
    if (traversalStore.canPause) {
      switch (phase) {
        case 'moving':
          return t.value.statusMoving
        case 'stabilizing':
          return t.value.statusStabilizing
        case 'acquiring':
          return t.value.statusAcquiring
        case 'saving':
          return t.value.statusSaving
        default:
          return t.value.statusRunning
      }
    }
    switch (traversalStore.statusType) {
      case 'paused':
        return t.value.statusPaused
      case 'completed':
        return t.value.statusDone
      case 'error':
        return t.value.statusError
      case 'stopped':
        return t.value.statusStopped
      case 'unknown':
        return t.value.statusUnknown
      default:
        return t.value.statusIdle
    }
  })

  // 状态点样式
  const statusDotClass = computed(
    () => STATUS_DOT_CLASS[traversalStore.statusType] ?? STATUS_DOT_CLASS.idle
  )

  return {
    statusText,
    statusDotClass
  }
}
