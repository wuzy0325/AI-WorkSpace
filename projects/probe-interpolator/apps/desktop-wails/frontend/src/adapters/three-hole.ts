// adapters/three-hole.ts 是 3 孔探针工作区与 Wails 后端的桥接层。
// 作用：
//   1. 隔离 bindings 目录路径细节（Wails v3 按 module path 生成嵌套目录）
//   2. 统一错误处理（Wails 抛出的 error 转 string）
//   3. 暴露强类型签名供 ThreeHoleWorkspace.vue 使用
//
// 注意：bindings 目录由 `wails3 generate bindings` 自动生成，不要手动修改。

import * as WailsApp from '../../bindings/probe-interpolator/apps/desktop-wails/backend/app'
import * as models from '../../bindings/probe-interpolator/apps/desktop-wails/backend/models'
import { GenericResponse, isWailsAvailable } from './common'

// ==================== 类型 re-export ====================
export type ThreeHolePrbFileInfo = models.ThreeHolePrbFileInfo
export type ThreeHolePrbValidRange = models.ThreeHolePrbValidRange
export type ThreeHoleLoadPrbResult = models.ThreeHoleLoadPrbResult
export type ThreeHoleInterpolationInput = models.ThreeHoleInterpolationInput
export type ThreeHoleInterpolationResult = models.ThreeHoleInterpolationResult

// GenericResponse 与 isWailsAvailable 已迁移到 ./common，此处 re-export 保持组件 import 路径不变。
// isolatedModules 模式下 type 重导出必须用 export type，与 value 分开。
export type { GenericResponse }
export { isWailsAvailable }

// ==================== API 封装 ====================
// 每个方法都 catch 异常转为统一 [GenericResponse, Data] 元组返回，
// 让组件无需 try/catch 即可处理错误。

export const api = {
  /**
   * loadPrbFiles 弹出文件选择对话框，让用户选 3 孔 .prb 校准文件并加载。
   * 返回 [错误信息, 加载结果]；加载结果为 null 表示失败或取消。
   */
  async loadPrbFiles(): Promise<[GenericResponse, ThreeHoleLoadPrbResult | null]> {
    try {
      const resp = await WailsApp.LoadThreeHolePrbFiles()
      if (!resp.success) {
        return [{ success: false, error: resp.error } as GenericResponse, null]
      }
      return [{ success: true } as GenericResponse, resp.data ?? null]
    } catch (e) {
      return [{ success: false, error: String(e) } as GenericResponse, null]
    }
  },

  /**
   * isPrbLoaded 查询 .prb 是否已加载。异常时返回 false（保守默认）。
   */
  async isPrbLoaded(): Promise<boolean> {
    try {
      return await WailsApp.IsThreeHolePrbLoaded()
    } catch {
      return false
    }
  },

  /**
   * getPrbFiles 返回已加载的 .prb 文件元信息列表。
   */
  async getPrbFiles(): Promise<ThreeHolePrbFileInfo[]> {
    try {
      return await WailsApp.GetThreeHolePrbFiles()
    } catch {
      return []
    }
  },

  /**
   * getMachRange 返回已加载 .prb 的马赫数范围 [min, max]。
   * 未加载时返回 success=false。
   */
  async getMachRange(): Promise<[GenericResponse, number[]]> {
    try {
      const resp = await WailsApp.GetThreeHoleMachRange()
      if (!resp.success) {
        return [{ success: false, error: resp.error } as GenericResponse, []]
      }
      return [{ success: true } as GenericResponse, resp.data ?? []]
    } catch (e) {
      return [{ success: false, error: String(e) } as GenericResponse, []]
    }
  },

  /**
   * calculate 单点计算，返回 [错误信息, 计算结果]。
   */
  async calculate(input: ThreeHoleInterpolationInput): Promise<[GenericResponse, ThreeHoleInterpolationResult | null]> {
    try {
      const resp = await WailsApp.CalculateThreeHole(input)
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
  async batchCalculate(datas: ThreeHoleInterpolationInput[]): Promise<[GenericResponse, ThreeHoleInterpolationResult[]]> {
    try {
      const resp = await WailsApp.BatchCalculateThreeHole(datas)
      if (!resp.success) {
        // 即使部分失败，data 仍可能有有效结果，组件应优先用 data
        return [{ success: false, error: resp.error } as GenericResponse, (resp.data ?? []).filter((item): item is ThreeHoleInterpolationResult => item !== null)]
      }
      return [{ success: true } as GenericResponse, (resp.data ?? []).filter((item): item is ThreeHoleInterpolationResult => item !== null)]
    } catch (e) {
      return [{ success: false, error: String(e) } as GenericResponse, []]
    }
  },

  /**
   * importCsvData 弹出文件选择对话框，解析 3 孔数据 CSV/TXT/DAT 为输入数组。
   * 与 5 孔不同：3 孔要求 CSV 必含 P1/P2/P3/Patm/Tatm 五列。
   */
  async importCsvData(): Promise<[GenericResponse, ThreeHoleInterpolationInput[]]> {
    try {
      const resp = await WailsApp.ImportThreeHoleCsvData()
      if (!resp.success) {
        return [{ success: false, error: resp.error } as GenericResponse, []]
      }
      return [{ success: true } as GenericResponse, resp.data ?? []]
    } catch (e) {
      return [{ success: false, error: String(e) } as GenericResponse, []]
    }
  },

  /**
   * openHelpDoc 用系统默认程序打开 3 孔用户说明书 HTML。
   */
  async openHelpDoc(): Promise<GenericResponse> {
    try {
      await WailsApp.OpenThreeHoleHelpDoc()
      return { success: true } as GenericResponse
    } catch (e) {
      return { success: false, error: String(e) } as GenericResponse
    }
  },
}
