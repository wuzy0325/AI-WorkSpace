import { ElMessage } from 'element-plus'
import { useCalibrationStore } from '@/stores/calibration'
import { CalibrationMessages as M } from '@/stores/calibration/messages'
import type { ActionResult } from '@/types/api'

type MessageValue = string | ((...args: never[]) => string)

export function useCalibrationUI() {
  const store = useCalibrationStore()

  function resolveMsg(error?: string, detail?: string): string {
    if (!error) return detail || '操作失败'
    const msg = M[error as keyof typeof M] as MessageValue | undefined
    if (typeof msg === 'function') return msg(detail as never)
    return (msg as string) || error
  }

  async function handleResult(action: () => Promise<ActionResult>, successMsg: string) {
    const result = await action()
    if (result.ok) {
      if (successMsg) ElMessage.success(successMsg)
    } else {
      ElMessage.error(resolveMsg(result.error, result.detail))
    }
  }

  const startCalibration = async () => handleResult(() => store.startCalibration(), M.START_OK)
  const pauseCalibration = async () => handleResult(() => store.pauseCalibration(), M.PAUSE_OK)
  const resumeCalibration = async () => handleResult(() => store.resumeCalibration(), M.RESUME_OK)
  const stopCalibration = async () => handleResult(() => store.stopCalibration(), M.STOP_OK)
  const fitData = async () => handleResult(() => store.fitData(), M.FIT_OK)
  const endCalibration = async () => handleResult(() => store.endCalibration(), M.RESET_OK)
  const resetCollection = async () => handleResult(async () => store.resetCollection(), M.COLLECT_RESET)
  const connectDevice1604 = async (ids: string | string[]) => handleResult(() => store.connectDevice1604(ids), '')
  const disconnectDevice1604 = async (ids: string | string[]) => handleResult(() => store.disconnectDevice1604(ids), '')
  const connectPressDevice = async (id: string) => handleResult(() => store.connectPressDevice(id), '')
  const disconnectPressDevice = async (id: string) => handleResult(() => store.disconnectPressDevice(id), '')
  const resolveAlarm = async (decision: 'continue' | 'skip' | 'recollect' | 'stop') => handleResult(() => store.resolveAlarm(decision), M.ALARM_RESOLVED)
  const skipDevice = async (deviceId: string, reason: string) => handleResult(() => store.skipDevice(deviceId, reason), M.SKIP_DEVICE_OK)

  return {
    startCalibration, pauseCalibration, resumeCalibration, stopCalibration,
    fitData, endCalibration, resetCollection,
    connectDevice1604, disconnectDevice1604, connectPressDevice, disconnectPressDevice,
    resolveAlarm, skipDevice,
  }
}
