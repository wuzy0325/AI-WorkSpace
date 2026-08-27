import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import type { ActionResult } from '@/types/api'
import {
  setCalibrationConfig,
  generatePressurePoints as apiGeneratePoints,
  getPressurePoints as apiGetPoints,
  updatePointTargetPressure as apiUpdateTargetPressure,
  pressurize as apiPressurize,
  collectDataByDevice as apiCollectData
} from "@/api/calibration"
import type { FittingResultDTO } from "@/types/calibration"
import { ControlMode } from "@/types/calibration"
import type { PressurePoint } from './types'

export type { PressurePoint } from './types'

const POINTS_KEY = 'cal1604_pressure_points'

function loadSavedPoints(): PressurePoint[] {
  try {
    const raw = localStorage.getItem(POINTS_KEY)
    if (raw) {
      const saved = JSON.parse(raw) as PressurePoint[]
      return saved.map(p => ({ ...p, status: 'pending' as const, collectedData: undefined, collectedByDevice: undefined, actualPressure: undefined }))
    }
  } catch { /* ignore */ }
  return []
}

function savePoints(points: PressurePoint[]) {
  try {
    localStorage.setItem(POINTS_KEY, JSON.stringify(points))
  } catch { /* ignore */ }
}

function clearSavedPoints() {
  try {
    localStorage.removeItem(POINTS_KEY)
  } catch { /* ignore */ }
}

export const usePressurePointStore = defineStore('pressurePoint', () => {
  // State
  const pressurePoints = ref<PressurePoint[]>(loadSavedPoints())
  const fittingResult = ref<FittingResultDTO | null>(null)

  // Getters
  const hasCollectedData = computed(() =>
    pressurePoints.value.some(p => p.status === 'completed')
  )

  // Actions
  // 生成压力点（标定模块使用）
  const generatePressurePoints = async (opts?: {
    controlMode?: string
    pressureMode?: string
    channels?: number[]
    params?: { points: number; averageCount: number; minValue: number; maxValue: number; stableTime: number; precision: number; precisionLevel: string }
    silent?: boolean
  }): Promise<ActionResult> => {
    try {
      const channels = opts?.channels ?? []
      const params = opts?.params

      await setCalibrationConfig({
        channels,
        pressurePoints: params?.points ?? 2,
        averageCount: params?.averageCount ?? 5,
        minPressure: params?.minValue ?? 0,
        maxPressure: params?.maxValue ?? 100,
        stableWaitMs: (params?.stableTime ?? 3) * 1000,
        controlMode: (opts?.controlMode as ControlMode) || undefined,
        precision: params?.precision ?? 2,
        precisionLevel: Number(params?.precisionLevel) || 0.05
      })

      const points = await apiGeneratePoints()
      pressurePoints.value = points.map(p => ({
        id: `point-${p.index}`,
        index: p.index,
        targetPressure: p.targetPressure,
        status: p.status as PressurePoint['status'],
        collectedData: p.collectedData,
        collectedByDevice: p.collectedByDevice,
        actualPressure: p.actualPressure
      }))

      savePoints(pressurePoints.value)
      return { ok: true }
    } catch (error) {
      console.error('生成压力点失败:', error)
      return { ok: false, error: 'GENERATE_FAILED', detail: String(error) }
    }
  }

  // 添加压力点
  const addPressurePoint = (point: Omit<PressurePoint, 'id'>) => {
    pressurePoints.value.push({
      ...point,
      id: crypto.randomUUID()
    })
  }

  // 更新压力点目标压力（仅 pending 状态的点允许修改）
  const updateTargetPressure = async (pointId: string, targetPressure: number): Promise<ActionResult> => {
    const point = pressurePoints.value.find(p => p.id === pointId)
    if (!point) return { ok: false, error: 'MISSING_POINT', detail: '压力点未找到' }
    if (point.status !== 'pending') return { ok: false, error: 'NOT_PENDING', detail: '仅待执行状态的压力点可修改目标压力' }

    try {
      await apiUpdateTargetPressure(point.index, targetPressure)
      point.targetPressure = targetPressure
      savePoints(pressurePoints.value)
      return { ok: true }
    } catch (error) {
      console.error('更新目标压力失败:', error)
      return { ok: false, error: 'UPDATE_FAILED', detail: String(error) }
    }
  }

  // 删除压力点
  const removePressurePoint = (index: number) => {
    pressurePoints.value.splice(index, 1)
  }

  // 更新压力点状态
  const updatePointStatus = (pointId: string, status: PressurePoint['status']) => {
    const point = pressurePoints.value.find(p => p.id === pointId)
    if (point) {
      point.status = status
    }
  }

  // 打压
  const pressurize = async (pointId: string): Promise<ActionResult> => {
    const point = pressurePoints.value.find(p => p.id === pointId)
    if (!point) return { ok: false, error: 'MISSING_POINT', detail: '压力点未找到' }

    try {
      point.status = 'pressurizing'
      await apiPressurize(point.index)

      // 打压完成后刷新压力点状态
      const points = await apiGetPoints()
      const updatedPoint = points.find(p => p.index === point.index)
      if (updatedPoint) {
        point.status = updatedPoint.status as PressurePoint['status']
        point.actualPressure = updatedPoint.actualPressure
      } else {
        point.status = 'stabilizing'
      }

      return { ok: true }
    } catch (error) {
      console.error('打压失败:', error)
      point.status = 'error'
      return { ok: false, error: 'PRESSURIZE_FAILED', detail: String(error) }
    }
  }

  // 采集数据
  const collectData = async (pointId: string): Promise<ActionResult> => {
    const point = pressurePoints.value.find(p => p.id === pointId)
    if (!point) return { ok: false, error: 'MISSING_POINT', detail: '压力点未找到' }

    // manual mode: pending auto-confirms on collect
    if (point.status === 'pending') {
      point.status = 'stabilizing'
    }

    try {
      point.status = 'collecting'
      // 多设备采集：devices 为每台设备通道数据，data 为首个设备数据（兼容旧字段）
      const { data, devices } = await apiCollectData(point.index)

      point.collectedData = data
      if (devices && Object.keys(devices).length > 0) {
        point.collectedByDevice = Object.fromEntries(
          Object.entries(devices).map(([deviceId, collected]) => [
            deviceId,
            { deviceId, collected, status: 'completed' as const }
          ])
        )
      }
      point.status = 'completed'

      return { ok: true }
    } catch (error) {
      console.error('采集数据失败:', error)
      point.status = 'error'
      const detail = error instanceof Error ? error.message : String(error)
      return { ok: false, error: 'COLLECT_FAILED', detail: `采集数据失败: ${detail}` }
    }
  }

  // 重置采集数据（标定模块专用，仅重置测点状态，保留配置）
  const resetCollection = () => {
    pressurePoints.value = pressurePoints.value.map(p => ({
      ...p,
      status: 'pending' as PressurePoint['status'],
      collectedData: undefined,
      collectedByDevice: undefined,
      actualPressure: undefined
    }))
  }

  // 清空压力点
  const clearPoints = () => {
    pressurePoints.value = []
    clearSavedPoints()
  }

  return {
    // State
    pressurePoints,
    fittingResult,
    // Getters
    hasCollectedData,
    // Actions
    generatePressurePoints,
    addPressurePoint,
    updateTargetPressure,
    removePressurePoint,
    updatePointStatus,
    pressurize,
    collectData,
    resetCollection,
    clearPoints
  }
})
