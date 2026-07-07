import type { FiveHolePointLayout, CalibrationPoint } from '@shared/types/calibration'
import type { MotionControllerStatus } from '@shared/types/motion'
import type { CalibrationConfig } from '@shared/types/calibration'
import { calibrationApi } from '@api/calibrationApi'

function generateFiveHoleSnakePointsLocal(layout: FiveHolePointLayout): CalibrationPoint[] {
  const points: CalibrationPoint[] = []
  let id = 1

  const alphaValues: number[] = []
  for (let a = layout.alphaMin; a <= layout.alphaMax; a += layout.alphaStep) {
    alphaValues.push(Math.round(a * 10) / 10)
  }
  const betaValues: number[] = []
  for (let b = layout.betaMin; b <= layout.betaMax; b += layout.betaStep) {
    betaValues.push(Math.round(b * 10) / 10)
  }

  for (let bi = 0; bi < betaValues.length; bi++) {
    const beta = betaValues[bi]
    // 蛇形走位：奇数行反向遍历 α；默认（raster）每行都从 αMin 升序遍历
    const reverse = layout.serpentine === true && bi % 2 === 1
    const alphas = reverse ? [...alphaValues].reverse() : alphaValues
    for (const alpha of alphas) {
      points.push({
        id: id++,
        coordinates: { α: alpha, β: beta },
      })
    }
  }

  return points
}

export async function generateFiveHoleSnakePoints(layout: FiveHolePointLayout): Promise<CalibrationPoint[]> {
  try {
    const result = await calibrationApi.generateFiveHoleSnakePoints(layout)
    if (result && result.length > 0) {
      return result as CalibrationPoint[]
    }
  } catch {
    // fallback to local
  }
  return generateFiveHoleSnakePointsLocal(layout)
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
