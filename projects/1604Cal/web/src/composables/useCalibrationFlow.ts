import type { SessionState } from "@/types/calibration"
import { CalibrationStep } from '@/stores/calibration'

// 会话状态到校准步骤的映射
export function sessionStateToStep(state: SessionState): CalibrationStep {
  switch (state) {
    case 'idle':
    case 'stopped':
      return CalibrationStep.DEVICE_CONNECT
    case 'ready':
      return CalibrationStep.CHANNEL_SELECT
    case 'pressurizing':
    case 'stabilizing':
    case 'collecting':
    case 'point_done':
    case 'await_manual_collect':
    case 'await_alarm_resolution':
      return CalibrationStep.DATA_COLLECTION
    case 'fitting':
      return CalibrationStep.DATA_FITTING
    case 'completed':
      return CalibrationStep.COMPLETED
    case 'paused':
    case 'recovering':
    case 'error':
      return CalibrationStep.DATA_COLLECTION
    default:
      return CalibrationStep.DEVICE_CONNECT
  }
}

// 判断会话状态是否为"运行中"
export function isSessionRunning(state: SessionState): boolean {
  return ['pressurizing', 'stabilizing', 'collecting', 'point_done', 'fitting', 'await_manual_collect', 'await_alarm_resolution', 'recovering'].includes(state)
}
