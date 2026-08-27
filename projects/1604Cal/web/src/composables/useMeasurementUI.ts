import { ElMessage } from 'element-plus'
import { useMeasurementStore } from '@/stores/measurement'
import { MeasurementMessages as M } from '@/stores/measurement/messages'
import type { ActionResult } from '@/types/api'

type MessageValue = string | ((...args: never[]) => string)

export function useMeasurementUI() {
  const store = useMeasurementStore()

  function resolveMsg(error?: string, detail?: string): string {
    if (!error) return detail || '操作失败'
    const msg = M[error as keyof typeof M] as MessageValue | undefined
    if (typeof msg === 'function') return msg(detail as never)
    return (msg as string) || error
  }

  async function handleResult(action: () => Promise<ActionResult>, successMsg: string, errorMap?: Record<string, string>) {
    const result = await action()
    if (result.ok) {
      if (successMsg) ElMessage.success(successMsg)
    } else {
      const errMsg = result.error
        ? (errorMap?.[result.error] ?? resolveMsg(result.error, result.detail))
        : (result.detail || '操作失败')
      ElMessage.error(errMsg)
    }
  }

  const start = (channels: number[]) => handleResult(() => store.start(channels), M.START_OK)
  const manualStart = (channels: number[]) => handleResult(() => store.manualStart(channels), M.MANUAL_READY)
  const pause = async () => handleResult(() => store.pause(), M.PAUSE_OK)
  const stop = async () => handleResult(() => store.stop(), M.STOP_OK)
  const generatePoints = async () => handleResult(() => store.generatePoints(), M.POINTS_GENERATED)
  const autoCollect = async () => handleResult(() => store.autoCollect(), '')
  const manualPressurize = (idx: number) => handleResult(() => store.manualPressurize(idx), '')
  const manualCollect = (idx: number) => handleResult(() => store.manualCollect(idx), '')

  return { start, manualStart, pause, stop, generatePoints, autoCollect, manualPressurize, manualCollect }
}
