// adapters/seven-hole.ts 是 7 孔探针工作区与 Wails 后端的桥接层。
// 作用：
//   1. 隔离 bindings 目录路径细节（Wails v3 按 module path 生成嵌套目录）
//   2. 统一错误处理（Wails 抛出的 error 转 string）
//   3. 暴露强类型签名供 SevenHoleWorkspace.vue 使用
//
// 与 5 孔 / 3 孔 adapter 的关键差异：
//   - 7 孔无 PressureMode 字段（spec §1.1 强制表压输入）
//   - 7 孔无 MachRange 概念，改用 ValidRange（Alpha/Beta 范围，Mach 恒为 0）
//   - loadPrbFiles / loadCalibrationCsvFiles 接收"已分配好"的 inner + outer[6] 路径，
//     文件名分配在前端完成（seven-hole-helpers.ts）
//
// 注意：bindings 目录由 `wails3 generate bindings` 自动生成，不要手动修改。
//       修改后端 App 方法签名后必须重新生成 bindings 才能被前端调用。

import * as WailsApp from '../../bindings/probe-interpolator/apps/desktop-wails/backend/app'
import * as models from '../../bindings/probe-interpolator/apps/desktop-wails/backend/models'
import { GenericResponse, isWailsAvailable } from './common'

// ==================== 类型 re-export ====================
export type SevenHolePrbFileInfo = models.SevenHolePrbFileInfo
export type SevenHolePrbValidRange = models.SevenHolePrbValidRange
export type SevenHoleLoadPrbResult = models.SevenHoleLoadPrbResult
export type SevenHoleInterpolationInput = models.SevenHoleInterpolationInput
export type SevenHoleInterpolationResult = models.SevenHoleInterpolationResult
export type SevenHolePickFilesResponse = models.SevenHolePickFilesResponse
export type SevenHoleDataSourceResponse = models.SevenHoleDataSourceResponse

// GenericResponse 与 isWailsAvailable 已迁移到 ./common，此处 re-export 保持组件 import 路径不变。
// isolatedModules 模式下 type 重导出必须用 export type，与 value 分开。
export type { GenericResponse }
export { isWailsAvailable }

/** 七孔数据源类型（与后端 sevenHoleState.dataSource 字段同步） */
export type SevenHoleDataSource = 'prb' | 'calibration-csv' | ''

// ==================== API 封装 ====================
// 每个方法都 catch 异常转为统一 [GenericResponse, Data] 元组返回，
// 让组件无需 try/catch 即可处理错误。

export const api = {
  /**
   * loadPrbFiles 加载已分配好的 7 个 .prb 文件路径（1 个内区 + 6 个扇区）。
   *
   * 文件路径由前端按 basename 分配（assignSevenHoleFilesByName）：
   *   - "7.prb" → innerPath
   *   - "1.prb"~"6.prb" → outerPaths[0..5]
   *
   * 后端不再做 basename 路由——这样前端可在 UI 上同步展示每个槽位的文件名。
   * 返回 [错误信息, 加载结果]；加载结果为 null 表示失败或取消。
   */
  async loadPrbFiles(
    innerPath: string,
    outerPaths: string[],
  ): Promise<[GenericResponse, SevenHoleLoadPrbResult | null]> {
    try {
      const resp = await WailsApp.LoadSevenHolePrbFiles(innerPath, outerPaths)
      if (!resp.success) {
        return [{ success: false, error: resp.error } as GenericResponse, null]
      }
      return [{ success: true } as GenericResponse, resp.data ?? null]
    } catch (e) {
      return [{ success: false, error: String(e) } as GenericResponse, null]
    }
  },

  /**
   * loadCalibrationCsvFiles 加载已分配好的 7 个校准 CSV 文件路径。
   * 与 loadPrbFiles 同结构，区别仅在解析路径走 GBK + 列位置契约（后端处理）。
   * 文件路径由前端按 basename 分配（assignSevenHoleCsvFilesByName）：
   *   - 文件名含"小角度区" → innerPath
   *   - 文件名含"大角度N区" → outerPaths[N-1]
   */
  async loadCalibrationCsvFiles(
    innerPath: string,
    outerPaths: string[],
  ): Promise<[GenericResponse, SevenHoleLoadPrbResult | null]> {
    try {
      const resp = await WailsApp.LoadSevenHoleCalibrationCsvFiles(innerPath, outerPaths)
      if (!resp.success) {
        return [{ success: false, error: resp.error } as GenericResponse, null]
      }
      return [{ success: true } as GenericResponse, resp.data ?? null]
    } catch (e) {
      return [{ success: false, error: String(e) } as GenericResponse, null]
    }
  },

  /**
   * pickFiles 弹出支持 .prb 和 .csv 的多选文件对话框，仅返回用户选中的路径列表。
   * 不解析、不分配槽位——分配逻辑在前端按 basename 完成。
   * 取消选择时返回 Paths=[]（与 Wails 对话框"OK 但无选择"语义一致）。
   */
  async pickFiles(): Promise<[GenericResponse, string[]]> {
    try {
      const resp = await WailsApp.PickSevenHoleFiles()
      if (!resp.success) {
        return [{ success: false, error: resp.error } as GenericResponse, []]
      }
      return [{ success: true } as GenericResponse, resp.paths ?? []]
    } catch (e) {
      return [{ success: false, error: String(e) } as GenericResponse, []]
    }
  },

  /**
   * getDataSource 查询当前已加载的数据源类型。
   * 取值："prb" / "calibration-csv" / ""（未加载）。
   * 前端在初始化或切回 7 孔工作区时调用，用于决定槽位过滤器与展示文案。
   */
  async getDataSource(): Promise<[GenericResponse, SevenHoleDataSource]> {
    try {
      const resp = await WailsApp.GetSevenHoleDataSource()
      if (!resp.success) {
        return [{ success: false, error: resp.error } as GenericResponse, '']
      }
      // 后端 data 字段为 string，可能为 ""；空字符串表示未加载。
      const ds = (resp.data ?? '') as SevenHoleDataSource
      return [{ success: true } as GenericResponse, ds]
    } catch (e) {
      return [{ success: false, error: String(e) } as GenericResponse, '']
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
   * 注意：这是"数据 CSV"（待计算的压力数据），与 loadCalibrationCsvFiles 加载的"校准 CSV"
   * （标定网格点系数）完全不同——后者是 7 份 GBK 编码的标定文件。
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
