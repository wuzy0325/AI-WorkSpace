// adapters/seven-hole.ts 是 7 孔探针工作区与 Wails 后端的桥接层。
// 作用：
//   1. 隔离 bindings 目录路径细节（Wails v3 按 module path 生成嵌套目录）
//   2. 统一错误处理（Wails 抛出的 error 转 string）
//   3. 暴露强类型签名供 SevenHoleWorkspace.vue 使用
//
// 与 5 孔 / 3 孔 adapter 的关键差异：
//   - 7 孔无 PressureMode 字段（spec §1.1 强制表压输入）
//   - 7 孔无 MachRange 概念，改用 ValidRange（Alpha/Beta 范围，Mach 恒为 0）
//   - loadPrbFiles 一次加载 7 个文件（1.prb..7.prb），返回 SevenHoleLoadPrbResult
//
// 注意：bindings 目录由 `wails3 generate bindings` 自动生成，不要手动修改。

import * as WailsApp from '../../bindings/probe-interpolator/apps/desktop-wails/backend/app'
import * as models from '../../bindings/probe-interpolator/apps/desktop-wails/backend/models'
import { GenericResponse, isWailsAvailable } from './common'

// ==================== 类型 re-export ====================
export type SevenHolePrbFileInfo = models.SevenHolePrbFileInfo
export type SevenHolePrbValidRange = models.SevenHolePrbValidRange
export type SevenHoleLoadPrbResult = models.SevenHoleLoadPrbResult
export type SevenHoleInterpolationInput = models.SevenHoleInterpolationInput
export type SevenHoleInterpolationResult = models.SevenHoleInterpolationResult

// GenericResponse 与 isWailsAvailable 已迁移到 ./common，此处 re-export 保持组件 import 路径不变。
// isolatedModules 模式下 type 重导出必须用 export type，与 value 分开。
export type { GenericResponse }
export { isWailsAvailable }

// ==================== API 封装 ====================
// 每个方法都 catch 异常转为统一 [GenericResponse, Data] 元组返回，
// 让组件无需 try/catch 即可处理错误。

export const api = {
  /**
   * loadPrbFiles 弹出多选文件对话框，让用户选 7 个 .prb 校准文件（1.prb..7.prb）并加载。
   * 返回 [错误信息, 加载结果]；加载结果为 null 表示失败或取消。
   * 文件名 basename 必须为 "1".."7" 之一，否则后端返回错误。
   */
  async loadPrbFiles(): Promise<[GenericResponse, SevenHoleLoadPrbResult | null]> {
    try {
      const resp = await WailsApp.LoadSevenHolePrbFiles()
      if (!resp.success) {
        return [{ success: false, error: resp.error } as GenericResponse, null]
      }
      return [{ success: true } as GenericResponse, resp.data ?? null]
    } catch (e) {
      return [{ success: false, error: String(e) } as GenericResponse, null]
    }
  },

  /**
   * isPrbLoaded 查询 7 个 .prb 是否已全部加载。异常时返回 false（保守默认）。
   */
  async isPrbLoaded(): Promise<boolean> {
    try {
      return await WailsApp.IsSevenHolePrbLoaded()
    } catch {
      return false
    }
  },

  /**
   * getPrbFiles 返回已加载的 7 个 .prb 文件元信息列表（内区 + 外区 1..6 顺序）。
   */
  async getPrbFiles(): Promise<SevenHolePrbFileInfo[]> {
    try {
      return await WailsApp.GetSevenHolePrbFiles()
    } catch {
      return []
    }
  },

  /**
   * getValidRange 返回内区网格的角度覆盖范围（±30°）。
   * 注意：MachMin/MachMax 恒为 0，仅供 UI 展示，不得用于事后有效性拒绝（spec §2.2）。
   * 未加载时返回 success=false。
   */
  async getValidRange(): Promise<[GenericResponse, SevenHolePrbValidRange | null]> {
    try {
      const resp = await WailsApp.GetSevenHoleValidRange()
      if (!resp.success) {
        return [{ success: false, error: resp.error } as GenericResponse, null]
      }
      return [{ success: true } as GenericResponse, resp.data ?? null]
    } catch (e) {
      return [{ success: false, error: String(e) } as GenericResponse, null]
    }
  },

  /**
   * calculate 单点计算，返回 [错误信息, 计算结果]。
   * 输入所有 P1..P7 必须为表压（gauge），spec §1.1 强制要求。
   */
  async calculate(input: SevenHoleInterpolationInput): Promise<[GenericResponse, SevenHoleInterpolationResult | null]> {
    try {
      const resp = await WailsApp.CalculateSevenHole(input)
      if (!resp.success) {
        return [{ success: false, error: resp.error } as GenericResponse, null]
      }
      return [{ success: true } as GenericResponse, resp.data ?? null]
    } catch (e) {
      return [{ success: false, error: String(e) } as GenericResponse, null]
    }
  },

  /**
   * batchCalculate 批量计算，部分失败时 success=false 但 data 数组仍返回每行结果。
   * 过滤掉 null（理论上后端总返回非 nil 标记，但运行时防御）。
   */
  async batchCalculate(inputs: SevenHoleInterpolationInput[]): Promise<[GenericResponse, SevenHoleInterpolationResult[]]> {
    try {
      const resp = await WailsApp.BatchCalculateSevenHole(inputs)
      if (!resp.success) {
        // 即使部分失败，data 仍可能有有效结果，组件应优先用 data
        return [{ success: false, error: resp.error } as GenericResponse, (resp.data ?? []).filter((item): item is SevenHoleInterpolationResult => item !== null)]
      }
      return [{ success: true } as GenericResponse, (resp.data ?? []).filter((item): item is SevenHoleInterpolationResult => item !== null)]
    } catch (e) {
      return [{ success: false, error: String(e) } as GenericResponse, []]
    }
  },

  /**
   * importCsvData 弹出文件选择对话框，解析 7 孔数据 CSV 为输入数组。
   * 与 5 孔 / 3 孔不同：7 孔 CSV 必含 P1-P7 + Patm + Tatm 共 9 列，全部必需。
   */
  async importCsvData(): Promise<[GenericResponse, SevenHoleInterpolationInput[]]> {
    try {
      const resp = await WailsApp.ImportSevenHoleCsvData()
      if (!resp.success) {
        return [{ success: false, error: resp.error } as GenericResponse, []]
      }
      return [{ success: true } as GenericResponse, resp.data ?? []]
    } catch (e) {
      return [{ success: false, error: String(e) } as GenericResponse, []]
    }
  },

  /**
   * openHelpDoc 用系统默认程序打开 7 孔用户说明书 HTML。
   */
  async openHelpDoc(): Promise<GenericResponse> {
    try {
      await WailsApp.OpenSevenHoleHelpDoc()
      return { success: true } as GenericResponse
    } catch (e) {
      return { success: false, error: String(e) } as GenericResponse
    }
  },
}
