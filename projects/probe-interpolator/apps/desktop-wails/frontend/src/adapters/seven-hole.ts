// adapters/seven-hole.ts 是 7 孔探针工作区与 Go 后端的桥接层（Win7 HTTP 版）。
//
// 与原 Wails 版差异：
//   - 移除 Wails bindings 依赖，改用 fetch 调用 http://127.0.0.1:18183/api/seven/*
//   - 文件选择对话框改由 Electron IPC 处理（electronBridge.pickFiles 多选 7 个 .prb）
//   - 类型从 backend/models.ts 改为本地手写 interface（与 backend/seven_hole_types.go 对齐）
//
// 与 5 孔 / 3 孔 adapter 的关键差异：
//   - 7 孔无 PressureMode 字段（spec §1.1 强制表压输入）
//   - 7 孔无 MachRange 概念，改用 ValidRange（Alpha/Beta 范围，Mach 恒为 0）
//   - loadPrbFiles 一次加载 7 个文件（1.prb..7.prb），返回 SevenHoleLoadPrbResult
//   - Alpha=侧滑角、Beta=迎角（与 5 孔语义反转）
//
// 暴露的 api.* 方法签名与原 Wails 版完全一致，SevenHoleWorkspace.vue 无需改动。

import { get, post } from '../bridge/httpClient'
import { pickFile, pickFiles, PRB_FILTERS, CSV_FILTERS } from '../bridge/electronBridge'
import { GenericResponse, isWailsAvailable } from './common'

// ==================== 类型定义（与 backend/seven_hole_types.go 对齐）====================

export interface SevenHolePrbValidRange {
  alphaMin: number
  alphaMax: number
  betaMin: number
  betaMax: number
  machMin: number  // 恒为 0，仅占位
  machMax: number  // 恒为 0，仅占位
}

export interface SevenHolePrbFileInfo {
  filePath: string
  fileName: string
  sector: number  // 0=内区 (7.prb), 1..6=外区扇区 n
}

export interface SevenHoleLoadPrbResult {
  files: SevenHolePrbFileInfo[]
  validRange: SevenHolePrbValidRange
  warnings: string[]
}

export interface SevenHoleInterpolationInput {
  P1: number
  P2: number
  P3: number
  P4: number
  P5: number
  P6: number
  P7: number
  Patm: number
  Tatm: number
}

export interface SevenHoleInterpolationResult {
  alpha: number            // 侧滑角（deg），7 孔语义
  beta: number             // 迎角（deg），7 孔语义
  machNumber: number
  velocity: number
  dynamicPressure: number
  P0: number
  Ps: number
  isValid: boolean
  warning?: string
}

// 后端 Response 结构（与 backend/seven_hole_types.go 各 Response 类型对应）
interface SevenHoleLoadPrbResponse {
  success: boolean
  error?: string
  data?: SevenHoleLoadPrbResult
}
interface SevenHoleValidRangeResponse {
  success: boolean
  error?: string
  data?: SevenHolePrbValidRange
}
interface SevenHoleCalculateResponse {
  success: boolean
  error?: string
  data?: SevenHoleInterpolationResult
}
interface SevenHoleBatchCalculateResponse {
  success: boolean
  error?: string
  data?: (SevenHoleInterpolationResult | null)[]
}
interface SevenHoleImportCsvDataResponse {
  success: boolean
  error?: string
  data?: SevenHoleInterpolationInput[]
}

interface IsLoadedResponse {
  isLoaded: boolean
}

export type { GenericResponse }
export { isWailsAvailable }

// ==================== API 封装 ====================

export const api = {
  /**
   * loadPrbFiles 弹出多选文件对话框，让用户选 7 个 .prb 校准文件（1.prb..7.prb）并加载。
   * 返回 [错误信息, 加载结果]；加载结果为 null 表示失败或取消。
   * 文件名 basename 必须为 "1".."7" 之一，否则后端返回警告并跳过。
   * 用户取消时短路返回，不发起后端调用。
   */
  async loadPrbFiles(): Promise<[GenericResponse, SevenHoleLoadPrbResult | null]> {
    let filePaths: string[]
    try {
      filePaths = await pickFiles({ title: '选择 7 个 .prb 校准文件（1.prb..7.prb）', filters: PRB_FILTERS })
    } catch (e) {
      return [{ success: false, error: String(e) } as GenericResponse, null]
    }
    if (filePaths.length === 0) {
      return [{ success: false, error: '已取消选择文件' } as GenericResponse, null]
    }
    try {
      const resp = await post<SevenHoleLoadPrbResponse>('/api/seven/load-prb', { filePaths })
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
      const resp = await get<IsLoadedResponse>('/api/seven/is-loaded')
      return resp.isLoaded
    } catch {
      return false
    }
  },

  /**
   * getPrbFiles 返回已加载的 7 个 .prb 文件元信息列表（内区 + 外区 1..6 顺序）。
   */
  async getPrbFiles(): Promise<SevenHolePrbFileInfo[]> {
    try {
      return await get<SevenHolePrbFileInfo[]>('/api/seven/prb-files')
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
      const resp = await get<SevenHoleValidRangeResponse>('/api/seven/valid-range')
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
      const resp = await post<SevenHoleCalculateResponse>('/api/seven/calculate', input)
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
   */
  async batchCalculate(inputs: SevenHoleInterpolationInput[]): Promise<[GenericResponse, SevenHoleInterpolationResult[]]> {
    try {
      const resp = await post<SevenHoleBatchCalculateResponse>('/api/seven/batch-calculate', { inputs })
      if (!resp.success) {
        return [{ success: false, error: resp.error } as GenericResponse, (resp.data ?? []).filter((item): item is SevenHoleInterpolationResult => item !== null)]
      }
      return [{ success: true } as GenericResponse, (resp.data ?? []).filter((item): item is SevenHoleInterpolationResult => item !== null)]
    } catch (e) {
      return [{ success: false, error: String(e) } as GenericResponse, []]
    }
  },

  /**
   * importCsvData 弹出单选文件对话框，解析 7 孔数据 CSV 为输入数组。
   * 与 5 孔 / 3 孔不同：7 孔 CSV 必含 P1-P7 + Patm + Tatm 共 9 列，全部必需。
   * 用户取消时短路返回，不发起后端调用。
   */
  async importCsvData(): Promise<[GenericResponse, SevenHoleInterpolationInput[]]> {
    let filePath: string
    try {
      filePath = await pickFile({ title: '选择 7 孔数据 CSV 文件', filters: CSV_FILTERS })
    } catch (e) {
      return [{ success: false, error: String(e) } as GenericResponse, []]
    }
    if (filePath === '') {
      return [{ success: false, error: '已取消选择文件' } as GenericResponse, []]
    }
    try {
      const resp = await post<SevenHoleImportCsvDataResponse>('/api/seven/import-csv', { filePath })
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
      await post('/api/seven/help-doc')
      return { success: true } as GenericResponse
    } catch (e) {
      return { success: false, error: String(e) } as GenericResponse
    }
  },
}
