// adapters/three-hole.ts 是 3 孔探针工作区与 Go 后端的桥接层（Win7 HTTP 版）。
//
// 与原 Wails 版差异：
//   - 移除 Wails bindings 依赖，改用 fetch 调用 http://127.0.0.1:18183/api/three/*
//   - 文件选择对话框改由 Electron IPC 处理（electronBridge.pickFile 单选 .prb / pickFile CSV）
//   - 类型从 backend/models.ts 改为本地手写 interface（与 backend/three_hole_types.go 对齐）
//
// 与 5 孔 / 7 孔 adapter 的关键差异：
//   - 3 孔 .prb 单选（一个文件即可，5 孔 / 7 孔需多选）
//   - 3 孔无 Beta 字段（仅一维角度 Alpha）
//   - 3 孔 result 无速度分量字段
//
// 暴露的 api.* 方法签名与原 Wails 版完全一致，ThreeHoleWorkspace.vue 无需改动。

import { get, post } from '../bridge/httpClient'
import { pickFile, PRB_FILTERS, CSV_FILTERS } from '../bridge/electronBridge'
import { GenericResponse, isWailsAvailable } from './common'

// ==================== 类型定义（与 backend/three_hole_types.go 对齐）====================

export interface ThreeHolePrbValidRange {
  alphaMin: number
  alphaMax: number
  machMin: number
  machMax: number
}

export interface ThreeHolePrbFileInfo {
  filePath: string
  fileName: string
  machNumber: number
  validRange: ThreeHolePrbValidRange
}

export interface ThreeHoleLoadPrbResult {
  files: ThreeHolePrbFileInfo[]
  machRange: number[]
  warnings: string[]
}

export interface ThreeHoleInterpolationInput {
  P1: number
  P2: number
  P3: number
  Patm: number
  Tatm: number
  pressureMode: string  // "gauge" | "absolute"
}

export interface ThreeHoleInterpolationResult {
  alpha: number
  machNumber: number
  P0: number
  Ps: number
  iterationCount: number
  isValid: boolean
  warning?: string
}

// 后端 Response 结构（与 backend/three_hole_types.go 各 Response 类型对应）
interface ThreeHoleLoadPrbResponse {
  success: boolean
  error?: string
  data?: ThreeHoleLoadPrbResult
}
interface ThreeHoleMachRangeResponse {
  success: boolean
  error?: string
  data?: number[]
}
interface ThreeHoleCalculateResponse {
  success: boolean
  error?: string
  data?: ThreeHoleInterpolationResult
}
interface ThreeHoleBatchCalculateResponse {
  success: boolean
  error?: string
  data?: (ThreeHoleInterpolationResult | null)[]
}
interface ThreeHoleImportCsvDataResponse {
  success: boolean
  error?: string
  data?: ThreeHoleInterpolationInput[]
}

interface IsLoadedResponse {
  isLoaded: boolean
}

export type { GenericResponse }
export { isWailsAvailable }

// ==================== API 封装 ====================

export const api = {
  /**
   * loadPrbFiles 弹出单选文件对话框，让用户选 3 孔 .prb 校准文件并加载。
   * 返回 [错误信息, 加载结果]；加载结果为 null 表示失败或取消。
   * 用户取消时短路返回，不发起后端调用。
   */
  async loadPrbFiles(): Promise<[GenericResponse, ThreeHoleLoadPrbResult | null]> {
    let filePath: string
    try {
      filePath = await pickFile({ title: '选择 3 孔 .prb 校准文件', filters: PRB_FILTERS })
    } catch (e) {
      return [{ success: false, error: String(e) } as GenericResponse, null]
    }
    if (filePath === '') {
      return [{ success: false, error: '已取消选择文件' } as GenericResponse, null]
    }
    try {
      // 后端 LoadThreeHolePrbFiles 接受 []string，单选文件包装为单元素数组
      const resp = await post<ThreeHoleLoadPrbResponse>('/api/three/load-prb', { filePaths: [filePath] })
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
      const resp = await get<IsLoadedResponse>('/api/three/is-loaded')
      return resp.isLoaded
    } catch {
      return false
    }
  },

  /**
   * getPrbFiles 返回已加载的 .prb 文件元信息列表。
   */
  async getPrbFiles(): Promise<ThreeHolePrbFileInfo[]> {
    try {
      return await get<ThreeHolePrbFileInfo[]>('/api/three/prb-files')
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
      const resp = await get<ThreeHoleMachRangeResponse>('/api/three/mach-range')
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
      const resp = await post<ThreeHoleCalculateResponse>('/api/three/calculate', input)
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
  async batchCalculate(datas: ThreeHoleInterpolationInput[]): Promise<[GenericResponse, ThreeHoleInterpolationResult[]]> {
    try {
      const resp = await post<ThreeHoleBatchCalculateResponse>('/api/three/batch-calculate', { datas })
      if (!resp.success) {
        return [{ success: false, error: resp.error } as GenericResponse, (resp.data ?? []).filter((item): item is ThreeHoleInterpolationResult => item !== null)]
      }
      return [{ success: true } as GenericResponse, (resp.data ?? []).filter((item): item is ThreeHoleInterpolationResult => item !== null)]
    } catch (e) {
      return [{ success: false, error: String(e) } as GenericResponse, []]
    }
  },

  /**
   * importCsvData 弹出单选文件对话框，解析 3 孔数据 CSV/TXT/DAT 为输入数组。
   * 与 5 孔不同：3 孔要求 CSV 必含 P1/P2/P3/Patm/Tatm 五列。
   * 用户取消时短路返回，不发起后端调用。
   */
  async importCsvData(): Promise<[GenericResponse, ThreeHoleInterpolationInput[]]> {
    let filePath: string
    try {
      filePath = await pickFile({ title: '选择 3 孔数据 CSV 文件', filters: CSV_FILTERS })
    } catch (e) {
      return [{ success: false, error: String(e) } as GenericResponse, []]
    }
    if (filePath === '') {
      return [{ success: false, error: '已取消选择文件' } as GenericResponse, []]
    }
    try {
      const resp = await post<ThreeHoleImportCsvDataResponse>('/api/three/import-csv', { filePath })
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
      await post('/api/three/help-doc')
      return { success: true } as GenericResponse
    } catch (e) {
      return { success: false, error: String(e) } as GenericResponse
    }
  },
}
