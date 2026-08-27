import { ref } from 'vue'
import { ElMessage } from 'element-plus'
import { normalizeValveStatus, type ValveState } from '@/types/valve'

/**
 * useValveControl 抽离两个 1604 面板里几乎相同的「推阀门 + 回读 + toast」逻辑。
 *
 * - 防抖：pending 标志阻止并发点击
 * - 写后回读：使用指数退避（200ms / 500ms / 1000ms 三轮），任一轮回读 = target
 *   即提前结束，照顾不同机械阀门的切换延迟，又避免快阀也固定等满 1.7s。
 * - 用户反馈：success / warning（状态未知或回读不一致）/ error（含 N09 设备拒绝原文）
 *
 * 业务模块（calibration / measurement）提供 store-side 接口即可复用，
 * 不需要在每个面板里复制 30 行交互代码。
 */
export interface ValveStore {
  /** 阀门当前状态（store 中是字符串 ref，组件按需访问其 .value） */
  valveStatus: string
  setValveStatus: (status: string) => Promise<{ ok: boolean; error?: string; detail?: string }>
  refreshValveStatus?: () => Promise<void>
}

export interface UseValveControlOptions {
  /** 用于在 toast 中区分「标定 / 计量」语境（仅影响中文文案） */
  scenario: 'calibration' | 'measurement'
  /** 回读退避间隔（毫秒）。默认 [200, 500, 1000]，单测可注入更短间隔 */
  readbackBackoffMs?: number[]
}

const DEFAULT_READBACK_BACKOFF_MS = [200, 500, 1000]

function sleep(ms: number): Promise<void> {
  return new Promise(resolve => setTimeout(resolve, ms))
}

export function useValveControl(store: ValveStore, options: UseValveControlOptions) {
  const valvePending = ref(false)
  const backoff = options.readbackBackoffMs ?? DEFAULT_READBACK_BACKOFF_MS

  async function setValve(target: ValveState): Promise<void> {
    if (target !== 'calibration' && target !== 'measurement') return
    if (valvePending.value) return
    valvePending.value = true
    try {
      const result = await store.setValveStatus(target)
      if (!result.ok) {
        ElMessage.error(result.detail || '阀门切换失败')
        return
      }
      // 指数退避回读：命令应答 "A" 仅代表设备已接收；机械阀门切换有延迟，
      // 用 200/500/1000ms 三轮兜底，一旦回读 = target 立即结束。
      let actual: ValveState = ''
      for (let i = 0; i < backoff.length; i++) {
        await sleep(backoff[i])
        await store.refreshValveStatus?.()
        actual = normalizeValveStatus(store.valveStatus)
        if (actual === target) break
      }
      if (actual === target) {
        ElMessage.success(target === 'calibration' ? '阀门已切换到校准模式' : '阀门已切换到测量模式')
      } else if (actual === '' || actual === 'unknown') {
        ElMessage.warning('已下发指令，但阀门状态未知，请确认设备')
      } else {
        ElMessage.warning(
          `阀门当前仍为${actual === 'calibration' ? '校准' : '测量'}模式，请检查设备`
        )
      }
    } catch (err) {
      ElMessage.error(err instanceof Error ? err.message : '阀门切换失败')
    } finally {
      valvePending.value = false
    }
  }

  return { valvePending, setValve, scenario: options.scenario }
}
