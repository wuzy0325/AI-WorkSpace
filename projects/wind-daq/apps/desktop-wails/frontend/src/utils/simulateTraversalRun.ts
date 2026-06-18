// DEMO ONLY — not for production use
/**
 * 遍历测试模拟数据生成器（从 Cursor DAQ 移植）
 *
 * 在五孔校准模拟数据基础上，扩展生成遍历测试所需的完整数据点：
 * 包含原始压力、插值结果（速度/动压/密度等）。
 *
 * 移植来源：Cursor DAQ src/renderer/src/utils/simulateTraversalRun.ts
 */

import type { TraversalDataPoint, InterpolationResult, TraversalRawPressure } from '@shared/types/traversal'
import { generateSimulatedPoint } from './simulateFiveHoleCalibration'

/** 遍历模拟配置 */
export interface TraversalSimulationConfig {
  /** α 方向网格点数 */
  gridAlpha: number
  /** β 方向网格点数 */
  gridBeta: number
  /** α 取值范围 */
  alphaRange: [number, number]
  /** β 取值范围 */
  betaRange: [number, number]
  /** 是否添加随机噪声 */
  noise: boolean
}

/** 默认模拟配置：16×16 网格，α/β 各 ±45° */
export const DEFAULT_SIMULATION_CONFIG: TraversalSimulationConfig = {
  gridAlpha: 16,
  gridBeta: 16,
  alphaRange: [-45, 45],
  betaRange: [-45, 45],
  noise: true
}

/**
 * 根据区间与点数生成均匀网格值
 * @param range [min, max]
 * @param count 点数
 */
export function buildSimulationGridValues(range: [number, number], count: number): number[] {
  const step = count > 1 ? (range[1] - range[0]) / (count - 1) : 0
  const values: number[] = []
  for (let i = 0; i < count; i++) {
    values.push(Number((range[0] + i * step).toFixed(10)))
  }
  return values
}

/**
 * 马赫数转速度（m/s）
 * @param mach 马赫数
 * @param tatmCelsius 大气静温（℃）
 */
export function machToVelocity(mach: number, tatmCelsius: number): number {
  const Tk = tatmCelsius + 273.15
  const speedOfSound = Math.sqrt(1.4 * 287.06 * Tk)
  return mach * speedOfSound
}

/** 计算动压 q = 0.5 * ρ * V² */
function calculateDynamicPressure(velocity: number, density: number): number {
  return 0.5 * density * velocity * velocity
}

/** 计算空气密度 ρ = p / (R * T) */
function calculateDensity(pAtmPa: number, tatmCelsius: number): number {
  const Tk = tatmCelsius + 273.15
  const R = 287.06
  return pAtmPa / (R * Tk)
}

/**
 * 创建一个模拟遍历数据点
 * 复用五孔校准模拟数据，并补充速度/动压/密度等派生量
 */
export function createSimulatedTraversalDataPoint(
  alpha: number,
  beta: number,
  pointId: number,
  noise: boolean
): TraversalDataPoint {
  const calibPoint = generateSimulatedPoint(alpha, beta, pointId, noise)
  const tatm = calibPoint.rawData.tAtm
  const mach = calibPoint.coefficients.machNumber ?? 0
  const velocity = machToVelocity(mach, tatm)
  const pAtmPa = calibPoint.rawData.pAtm * 1000
  const density = calculateDensity(pAtmPa, tatm)
  const dynPressure = calculateDynamicPressure(velocity, density)

  const result: InterpolationResult = {
    alpha,
    beta,
    machNumber: mach,
    velocity,
    dynamicPressure: dynPressure,
    density,
    P0: calibPoint.rawData.pTotal,
    Ps: calibPoint.rawData.pStatic,
    isValid: true
  }

  const rawPressure: TraversalRawPressure = {
    P1: calibPoint.rawData.p1,
    P2: calibPoint.rawData.p2,
    P3: calibPoint.rawData.p3,
    P4: calibPoint.rawData.p4,
    P5: calibPoint.rawData.p5,
    Patm: calibPoint.rawData.pAtm,
    Tatm: calibPoint.rawData.tAtm,
    P0: calibPoint.rawData.pTotal,
    Ps: calibPoint.rawData.pStatic
  }

  return {
    pointId,
    coordinates: { alpha, beta },
    rawPressure,
    interpolationResult: result,
    sampleCount: 10,
    timestamp: Date.now(),
    dwellTimeElapsed: 1000
  }
}

/**
 * 批量生成模拟遍历数据点
 * 与 Traversal 布点预览保持一致：alpha/x 为外层，beta/y 为内层；奇数列启用蛇形反向
 */
export function generateSimulatedTraversalPoints(
  config: TraversalSimulationConfig = DEFAULT_SIMULATION_CONFIG
): TraversalDataPoint[] {
  const alphaValues = buildSimulationGridValues(config.alphaRange, config.gridAlpha)
  const betaValues = buildSimulationGridValues(config.betaRange, config.gridBeta)

  const points: TraversalDataPoint[] = []
  let id = 0

  // 与 Traversal 布点预览保持一致：alpha/x 为外层，beta/y 为内层；奇数列启用蛇形反向
  for (let alphaIndex = 0; alphaIndex < alphaValues.length; alphaIndex++) {
    const alpha = alphaValues[alphaIndex]
    const betas = alphaIndex % 2 === 1 ? [...betaValues].reverse() : betaValues
    for (const beta of betas) {
      points.push(createSimulatedTraversalDataPoint(alpha, beta, id, config.noise))
      id++
    }
  }

  return points
}
