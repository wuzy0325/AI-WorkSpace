// 总压校准系数（CPT 等）曲线 Y 轴范围解析（纯函数，便于单测）。
//
// 产品决策：总压校准系数理想值接近 1，自动模式（无用户手动 override）下
// Y 轴默认窗口为 [0.99, 1.01]，让操作员直接聚焦系数在 1 附近的微小变化。
// 数据点超出该窗口时自动扩展范围以容纳散点，避免点被裁剪出图外。

export const TOTAL_PRESSURE_Y_DEFAULT_MIN = 0.99
export const TOTAL_PRESSURE_Y_DEFAULT_MAX = 1.01

export interface YRange {
  min: number
  max: number
}

/** Y 轴范围手动覆盖：用户在图表 Tab 输入 min/max 后传入，直接采用，跳过默认窗口。 */
export interface YRangeOverride {
  min: number
  max: number
}

/**
 * 解析 Y 轴范围：
 * - 手动 override 有效（min < max 且均为有限数）时直接采用，不加 padding（用户已自定边界）。
 * - 否则为自动模式：基础窗口 [0.99, 1.01]，数据超出时扩展并加 10% padding。
 */
export function resolveTotalPressureYRange(
  yValues: number[],
  yRangeOverride: YRangeOverride | null | undefined,
): YRange {
  if (
    yRangeOverride &&
    Number.isFinite(yRangeOverride.min) &&
    Number.isFinite(yRangeOverride.max) &&
    yRangeOverride.min < yRangeOverride.max
  ) {
    return { min: yRangeOverride.min, max: yRangeOverride.max }
  }

  let yMin = TOTAL_PRESSURE_Y_DEFAULT_MIN
  let yMax = TOTAL_PRESSURE_Y_DEFAULT_MAX
  if (yValues.length > 0) {
    yMin = Math.min(TOTAL_PRESSURE_Y_DEFAULT_MIN, ...yValues)
    yMax = Math.max(TOTAL_PRESSURE_Y_DEFAULT_MAX, ...yValues)
    if (yMin === yMax) {
      yMin -= 1
      yMax += 1
    }
  }

  // 数据全落在默认窗口内时不加 padding，保持精确 0.99~1.01；
  // 数据超出窗口时才按比例加 10% 边距，保证散点不贴边。
  if (yMin < TOTAL_PRESSURE_Y_DEFAULT_MIN || yMax > TOTAL_PRESSURE_Y_DEFAULT_MAX) {
    const yPad = (yMax - yMin) * 0.1
    yMin -= yPad
    yMax += yPad
  }
  return { min: yMin, max: yMax }
}