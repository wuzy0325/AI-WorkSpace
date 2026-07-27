// adapters/seven-hole-helpers.ts 提供 7 孔校准文件批量选择的前端分配逻辑。
//
// 与 wind-daq 遍历测试的 shared/types/traversal.ts 中的同名函数保持一致，
// 便于未来提取到 shared/frontend 共享目录；当前为避免影响 wind-daq 而在本项目内独立维护。
//
// 后端 LoadSevenHolePrbFiles / LoadSevenHoleCalibrationCsvFiles 接收"已分配好"的
// inner + outer[6] 路径——分配逻辑（按文件名识别 sector）在前端完成，这样：
//   1. 后端无需关心文件名约定（PRB "1.prb"~"7.prb" vs CSV "小角度区/大角度N区"）
//   2. 前端可在 UI 上同步展示"哪些文件未被识别"，由用户手动补选
//   3. PRB 与 CSV 两条路径共用同一组 helper，行为可对照测试

import type { SevenHolePrbFileInfo } from './seven-hole'

/** 七孔批量导入的文件名→槽位分配结果 */
export interface SevenHoleFileAssignment {
  innerFile: SevenHolePrbFileInfo | null
  outerFiles: Map<number, SevenHolePrbFileInfo>
  /** 无法按规范命名分配的文件（由用户逐槽手动选择） */
  unmatched: string[]
}

/** 七孔批量导入格式探测结果 */
export type SevenHoleBatchFormat = 'prb' | 'calibration-csv' | 'mixed' | 'empty'

/**
 * detectSevenHoleBatchFormat 检测批量选择的文件格式：
 * - 全 .prb → 'prb'
 * - 全 .csv → 'calibration-csv'
 * - 混合或为空 → 'mixed' / 'empty'
 *
 * 大小写不敏感（用户可能从不同系统拷贝文件）。
 */
export function detectSevenHoleBatchFormat(paths: string[]): SevenHoleBatchFormat {
  if (paths.length === 0) return 'empty'
  const allPrb = paths.every((p) => /\.prb$/i.test(p))
  if (allPrb) return 'prb'
  const allCsv = paths.every((p) => /\.csv$/i.test(p))
  if (allCsv) return 'calibration-csv'
  return 'mixed'
}

/**
 * assignSevenHoleFilesByName 按文件名把批量选择的 .prb 分配到七孔槽位：
 *   - 7.prb → 内区（Sector=7）
 *   - 1.prb~6.prb → 扇区 1..6（Sector=n）
 *
 * 大小写不敏感，仅认规范命名；同一槽位重复命中时后来者覆盖；
 * 不匹配的文件列入 unmatched，由前端 UI 提示用户手动选择槽位。
 *
 * pointCount 在前端分配阶段不可知（需后端读取 PRB 内容才能确定），
 * 这里统一置 0，后端 LoadSevenHolePrbFiles 返回后会用真实值回填槽位。
 */
export function assignSevenHoleFilesByName(paths: string[]): SevenHoleFileAssignment {
  const outerFiles = new Map<number, SevenHolePrbFileInfo>()
  const unmatched: string[] = []
  let innerFile: SevenHolePrbFileInfo | null = null
  for (const path of paths) {
    const fileName = path.split(/[\\/]/).pop() ?? path
    const m = /^(\d+)\.prb$/i.exec(fileName)
    if (m) {
      const n = Number(m[1])
      if (n === 7) {
        innerFile = { filePath: path, fileName, sector: 7, pointCount: 0 }
        continue
      }
      if (n >= 1 && n <= 6) {
        outerFiles.set(n, { filePath: path, fileName, sector: n, pointCount: 0 })
        continue
      }
    }
    unmatched.push(path)
  }
  return { innerFile, outerFiles, unmatched }
}

/**
 * assignSevenHoleCsvFilesByName 按文件名把批量选择的七孔校准 CSV 分配到槽位：
 *   - 文件名含"(小角度区)" → 内区（Sector=7）
 *   - 文件名含"(大角度N区)"（N=1..6）→ 扇区 N
 *
 * 校准 CSV 由标定软件导出，文件名约定形如 "W532...(小角度区).csv" /
 * "W532...(大角度3区).csv"。用括号包裹关键字锚定，避免误匹配
 * "备份-小角度区-2024.csv" 等非标定文件名。
 *
 * 不匹配的文件列入 unmatched，由前端 UI 提示用户手动选择槽位。
 *
 * pointCount 在前端分配阶段不可知（需后端解析 CSV 后才能确定），
 * 这里统一置 0，后端 LoadSevenHoleCalibrationCsvFiles 返回后会用真实值回填槽位。
 */
export function assignSevenHoleCsvFilesByName(paths: string[]): SevenHoleFileAssignment {
  const outerFiles = new Map<number, SevenHolePrbFileInfo>()
  const unmatched: string[] = []
  let innerFile: SevenHolePrbFileInfo | null = null
  for (const path of paths) {
    const fileName = path.split(/[\\/]/).pop() ?? path
    // 用括号锚定关键字，避免 "备份-小角度区-2024.csv" 等非标定文件名误匹配。
    if (fileName.includes('(小角度区)')) {
      innerFile = { filePath: path, fileName, sector: 7, pointCount: 0 }
      continue
    }
    const m = /\(大角度([1-6])区\)/.exec(fileName)
    if (m) {
      const n = Number(m[1])
      outerFiles.set(n, { filePath: path, fileName, sector: n, pointCount: 0 })
      continue
    }
    unmatched.push(path)
  }
  return { innerFile, outerFiles, unmatched }
}

/**
 * fileNameOf 从完整路径中提取 basename，用于 UI 展示。
 * 同时支持 Windows 反斜杠与 POSIX 正斜杠分隔符。
 */
export function fileNameOf(path: string): string {
  return path.split(/[\\/]/).pop() ?? path
}
