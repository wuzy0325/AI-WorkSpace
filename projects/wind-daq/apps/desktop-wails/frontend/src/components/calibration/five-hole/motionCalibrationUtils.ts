import type { FiveHolePointLayout, CalibrationPoint } from '@shared/types/calibration'
import type { MotionControllerStatus } from '@shared/types/motion'
import type { CalibrationConfig } from '@shared/types/calibration'
import { calibrationApi } from '@api/calibrationApi'

// spec Task 11：删除前端本地蛇形算法 + catch fallback。
//   旧实现 `try { api } catch { local }` 让后端错误静默降级到本地公式，
//   违反 spec R-4 "删除前端五孔蛇形布点本地 fallback，后端失败显式反馈"。
//   现统一透传 calibrationApi.generateFiveHoleSnakePoints：
//     - Wails 模式：calibrationApi 内部调 wailsApi.calibration.previewFiveHole binding
//     - HTTP 模式：calibrationApi 内部 POST /api/calibration/fivehole
//   两端共用后端 usecase.PreviewFiveHolePoints，错误透传到 UI 由调用方显示。
//   此函数现在只做"接口形状适配"——把 bare array 元素类型从
//   { id; coordinates: Record<string, number> } 透传为 CalibrationPoint[]，
//   不再做任何点位生成计算。

/**
 * 生成五孔蛇形/raster 校准点位（spec Task 11 后纯后端驱动）。
 *
 * @param layout α/β 范围与步长 + serpentine 开关
 * @returns 后端返回的点位列表（bare array）
 * @throws 后端错误（如步长 ≤ 0、binding 未初始化、HTTP 离线）透传给调用方
 */
export async function generateFiveHoleSnakePoints(layout: FiveHolePointLayout): Promise<CalibrationPoint[]> {
  return await calibrationApi.generateFiveHoleSnakePoints(layout)
}

export function formatFiveHoleActualPosition(
  config: CalibrationConfig | null | undefined,
  statusList: MotionControllerStatus[]
): string {
  if (!config?.motionAxes?.length) return '未配置'
  const parts: string[] = []
  for (const axis of config.motionAxes) {
    if (!axis.controllerId) {
      parts.push(`${axis.name}=未绑定`)
      continue
    }
    const status = statusList.find((s) => s.id === axis.controllerId)
    if (!status) {
      parts.push(`${axis.name}=离线`)
      continue
    }
    const axisStatus = status.axes.find((a) => a.name === axis.axis)
    if (!axisStatus) {
      parts.push(`${axis.name}=无轴`)
      continue
    }
    parts.push(`${axis.name}=${axisStatus.position.toFixed(2)}°`)
  }
  return parts.join(' · ')
}
