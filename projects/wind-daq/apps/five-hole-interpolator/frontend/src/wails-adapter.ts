import * as WailsApp from '../wailsjs/go/backend/App'
import { backend } from '../wailsjs/go/models'

// ==================== 类型导出 ====================
export type GenericResponse = backend.GenericResponse
export type PrbFileInfo = backend.PrbFileInfo
export type PrbValidRange = backend.PrbValidRange
export type LoadPrbResult = backend.LoadPrbResult
export type InterpolationInput = backend.InterpolationInput
export type InterpolationResult = backend.InterpolationResult

// ==================== Wails 可用性检测 ====================
export function isWailsAvailable(): boolean {
  return typeof window !== 'undefined' && !!(window as any).go?.backend?.App
}

// ==================== 辅助函数 ====================

// Wails 多返回值处理：Go 返回 (GenericResponse, T) 时，
// Wails 实际返回两个独立的 Promise 结果，第一个是 GenericResponse，第二个是 T
// 需要分别判断类型来提取
function isGenericResponse(obj: any): obj is GenericResponse {
  return obj && typeof obj === 'object' && 'success' in obj
}

// 从 Wails 的联合类型返回值中分离出 GenericResponse 和数据
// Wails 对多返回值的处理：返回值可能是按顺序的，第一个是 response
function splitMultiReturn<T>(raw: any): [GenericResponse, T | null] {
  // 如果返回的是数组（某些 Wails 版本）
  if (Array.isArray(raw)) {
    const resp = raw[0] as GenericResponse
    const data = raw.length > 1 ? (raw[1] as T) : null
    return [resp, data]
  }
  // 如果返回的是单个 GenericResponse（表示失败）
  if (isGenericResponse(raw) && !raw.success) {
    return [raw, null]
  }
  // 如果返回的是单个对象且包含 success: true，可能是 response 本身
  if (isGenericResponse(raw) && raw.success) {
    return [raw, null]
  }
  // 其他情况
  return [{ success: true } as GenericResponse, raw as T]
}

// ==================== API 封装 ====================
export const api = {
  async loadPrbFiles(): Promise<[GenericResponse, LoadPrbResult | null]> {
    try {
      const result = await WailsApp.LoadPrbFiles()
      return splitMultiReturn<LoadPrbResult>(result)
    } catch (e) {
      return [{ success: false, error: String(e) } as GenericResponse, null]
    }
  },

  async isPrbLoaded(): Promise<boolean> {
    try {
      return await WailsApp.IsPrbLoaded()
    } catch {
      return false
    }
  },

  async getPrbFiles(): Promise<PrbFileInfo[]> {
    try {
      return await WailsApp.GetPrbFiles()
    } catch {
      return []
    }
  },

  async batchCalculate(datas: InterpolationInput[]): Promise<[GenericResponse, InterpolationResult[]]> {
    try {
      const result = await WailsApp.BatchCalculate(datas)
      const [resp, results] = splitMultiReturn<InterpolationResult[]>(result)
      return [resp, results ?? []]
    } catch (e) {
      return [{ success: false, error: String(e) } as GenericResponse, []]
    }
  },

  async calculate(input: InterpolationInput): Promise<[GenericResponse, InterpolationResult | null]> {
    try {
      const result = await WailsApp.Calculate(input)
      return splitMultiReturn<InterpolationResult>(result)
    } catch (e) {
      return [{ success: false, error: String(e) } as GenericResponse, null]
    }
  },

  async importCsvData(): Promise<[GenericResponse, InterpolationInput[]]> {
    try {
      const result = await WailsApp.ImportCsvData()
      const [resp, data] = splitMultiReturn<InterpolationInput[]>(result)
      return [resp, data ?? []]
    } catch (e) {
      return [{ success: false, error: String(e) } as GenericResponse, []]
    }
  },
}
