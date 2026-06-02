// ⚠️ 架构边界注意：此文件包含数据插值/聚合逻辑（热力图数据构建）。
// 纯展示数据转换可保留在前端，但插值计算应确保在后端完成。
// 当前实现仅做数据格式转换（后端已计算插值结果），符合前端展示职责。

import { computed, type ComputedRef, type Ref, unref } from 'vue'
import type { TraversalDataPoint } from '@shared/types/traversal'
import type { HeatmapCell, VisualizationParam } from '../types'
import { getParamValue } from '../types'

function uniqueSorted(values: number[]): number[] {
  return Array.from(new Set(values)).sort((a, b) => a - b)
}

export function useHeatmapData(
  dataPoints: Ref<TraversalDataPoint[]> | ComputedRef<TraversalDataPoint[]>,
  param: Ref<VisualizationParam> | ComputedRef<VisualizationParam>
) {
  const validPoints = computed(() => unref(dataPoints).filter((point) => point.interpolationResult.isValid))

  const alphaValues = computed(() => uniqueSorted(validPoints.value.map((point) => point.coordinates.alpha)))
  const betaValues = computed(() => uniqueSorted(validPoints.value.map((point) => point.coordinates.beta)))

  const heatmapData = computed<HeatmapCell[]>(() => {
    const currentParam = unref(param)
    const alphaIndexByValue = new Map(alphaValues.value.map((value, index) => [value, index]))
    const betaIndexByValue = new Map(betaValues.value.map((value, index) => [value, index]))

    return validPoints.value.flatMap((point) => {
      const value = getParamValue(point.interpolationResult, currentParam)
      const alphaIndex = alphaIndexByValue.get(point.coordinates.alpha)
      const betaIndex = betaIndexByValue.get(point.coordinates.beta)

      if (value === null || alphaIndex === undefined || betaIndex === undefined) {
        return []
      }

      return [{
        value: [alphaIndex, betaIndex, value],
        alpha: point.coordinates.alpha,
        beta: point.coordinates.beta
      }]
    })
  })

  const valueRange = computed<[number, number]>(() => {
    const values = heatmapData.value.map((cell) => cell.value[2])
    if (values.length === 0) return [0, 1]

    const min = Math.min(...values)
    const max = Math.max(...values)
    return min === max ? [min - 1, max + 1] : [min, max]
  })

  return { validPoints, alphaValues, betaValues, heatmapData, valueRange }
}

