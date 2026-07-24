// adapters/five-hole.ts 是 5 孔探针工作区与 Go 后端的桥接层（Win7 HTTP 版）。
//
// 与原 Wails 版差异：
//   - 移除 Wails bindings 依赖，改用 fetch 调用 http://127.0.0.1:18183/api/five/*
//   - 文件选择对话框改由 Electron IPC 处理（electronBridge.pickFiles / pickFile）
//   - 类型从 backend/models.ts 改为本地手写 interface（与 backend/five_hole_types.go 对齐）
//   - 用户取消文件选择时短路返回，不发起后端调用
//
// 暴露的 api.* 方法签名与原 Wails 版完全一致，FiveHoleWorkspace.vue 无需改动。

import { get, post } from '../bridge/httpClient'
import { pickFile, pickFiles, PRB_FILTERS, CSV_FILTERS } from '../bridge/electronBridge'
import { GenericResponse, isWailsAvailable } from './common'

// ==================== 类型定义（与 backend/five_hole_types.go 对齐）====================

export interface PrbValidRange {
  alphaMin: number
  alphaMax: number
  betaMin: number
  betaMax: number
  machMin: number
  machMax: number
}

export interface PrbFileInfo {
  filePath: string
  fileName: string
  machNumber: number
  validRange: PrbValidRange
}

export interface LoadPrbResult {
  files: PrbFileInfo[]
  machRange: number[]
  warnings: string[]
}

export interface FiveHoleInterpolationInput {
  P1: number
  P2: number
  P3: number
  P4: number
  P5: number
  Patm: number
  Tatm: number
  pressureMode: string  // "gauge" | "absolute"
}

export interface FiveHoleInterpolationResult {
  alpha: number
  beta: number
  machNumber: number
  V: number
  Vx: number
  Vy: number
  Vz: number
  velocity: number
  cas: number
  sat: number
  dynamicPressure: number
  density: number
  P0: number
  Ps: number
  isValid: boolean
  warning?: string
}

// 后端 Response 结构（与 backend/five_hole_types.go 各 Response 类型对应）。
// HTTP 信封解包后拿到的是这些 Response 对象本身。
interface LoadPrbResponse {
  success: boolean
  error?: string
  data?: LoadPrbResult
}
interface MachRangeResponse {
  success: boolean
  error?: string
  data?: number[]
}
interface CalculateResponse {
  success: boolean
  error?: string
  data?: FiveHoleInterpolationResult
}
interface BatchCalculateResponse {
  success: boolean
  error?: string
  data?: (FiveHoleInterpolationResult | null)[]
}
interface ImportCsvDataResponse {
  success: boolean
  error?: string
  data?: FiveHoleInterpolationInput[]
}

// 后端 /api/five/is-loaded 返回的 {isLoaded: bool} 信封内 data 字段
interface IsLoadedResponse {
  isLoaded: boolean
}

// GenericResponse 与 isWailsAvailable re-export 保持组件 import 路径不变
export type { GenericResponse }
export { isWailsAvailable }

// ==================== API 封装 ====================
// 每个方法都 catch 异常转为统一 [GenericResponse, Data] 元组返回，
// 让组件无需 try/catch 即可处理错误。

export const api = {
  /**
   * loadPrbFiles 弹出多选文件对话框，让用户选多个 5 孔 .prb 校准文件并加载。
   * 返回 [错误信息, 加载结果]；加载结果为 null 表示失败或取消。
   * 用户取消时短路返回，不发起后端调用。
   */
  async loadPrbFiles(): Promise<[GenericResponse, LoadPrbResult | null]> {
    let filePaths: string[]
    try {
      filePaths = await pickFiles({ title: '选择 5 孔 .prb 校准文件', filters: PRB_FILTERS })
    } catch (e) {
      return [{ success: false, error: String(e) } as GenericResponse, null]
    }
    if (filePaths.length === 0) {
      return [{ success: false, error: '已取消选择文件' } as GenericResponse, null]
    }
    try {
      const resp = await post<LoadPrbResponse>('/api/five/load-prb', { filePaths })
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
      const resp = await get<IsLoadedResponse>('/api/five/is-loaded')
      return resp.isLoaded
    } catch {
      return false
    }
  },

  /**
   * getPrbFiles 返回已加载的 .prb 文件元信息列表。
   */
  async getPrbFiles(): Promise<PrbFileInfo[]> {
    try {
      return await get<PrbFileInfo[]>('/api/five/prb-files')
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
      const resp = await get<MachRangeResponse>('/api/five/mach-range')
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
  async calculate(input: FiveHoleInterpolationInput): Promise<[GenericResponse, FiveHoleInterpolationResult | null]> {
    try {
      const resp = await post<CalculateResponse>('/api/five/calculate', input)
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
  async batchCalculate(datas: FiveHoleInterpolationInput[]): Promise<[GenericResponse, FiveHoleInterpolationResult[]]> {
    try {
      const resp = await post<BatchCalculateResponse>('/api/five/batch-calculate', { datas })
      if (!resp.success) {
        // 即使部分失败，data 仍可能有有效结果，组件应优先用 data
        return [{ success: false, error: resp.error } as GenericResponse, (resp.data ?? []).filter((item): item is FiveHoleInterpolationResult => item !== null)]
      }
      return [{ success: true } as GenericResponse, (resp.data ?? []).filter((item): item is FiveHoleInterpolationResult => item !== null)]
    } catch (e) {
      return [{ success: false, error: String(e) } as GenericResponse, []]
    }
  },

  /**
   * importCsvData 弹出单选文件对话框，解析 5 孔数据 CSV 为输入数组。
   * 用户取消时短路返回，不发起后端调用。
   */
  async importCsvData(): Promise<[GenericResponse, FiveHoleInterpolationInput[]]> {
    let filePath: string
    try {
      filePath = await pickFile({ title: '选择 5 孔数据 CSV 文件', filters: CSV_FILTERS })
    } catch (e) {
      return [{ success: false, error: String(e) } as GenericResponse, []]
    }
    if (filePath === '') {
      return [{ success: false, error: '已取消选择文件' } as GenericResponse, []]
    }
    try {
      const resp = await post<ImportCsvDataResponse>('/api/five/import-csv', { filePath })
      if (!resp.success) {
        return [{ success: false, error: resp.error } as GenericResponse, []]
      }
      return [{ success: true } as GenericResponse, resp.data ?? []]
    } catch (e) {
      return [{ success: false, error: String(e) } as GenericResponse, []]
    }
  },

  /**
   * openHelpDoc 用系统默认程序打开 5 孔用户说明书 HTML。
   * 后端 OpenHelpDoc 返回 error，HTTP 失败时 httpClient 抛 Error，本层 catch 转为 GenericResponse。
   */
  async openHelpDoc(): Promise<GenericResponse> {
    try {
      await post('/api/five/help-doc')
      return { success: true } as GenericResponse
    } catch (e) {
      return { success: false, error: String(e) } as GenericResponse
    }
  },
}
