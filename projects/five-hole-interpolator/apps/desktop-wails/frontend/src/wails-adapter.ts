import * as WailsApp from '../bindings/five-hole-interpolator/apps/desktop-wails/backend/app'
import * as backend from '../bindings/five-hole-interpolator/apps/desktop-wails/backend/models'

export type PrbFileInfo = backend.PrbFileInfo
export type PrbValidRange = backend.PrbValidRange
export type LoadPrbResult = backend.LoadPrbResult
export type InterpolationInput = backend.InterpolationInput
export type InterpolationResult = backend.InterpolationResult

// GenericResponse 不在 Wails 生成的 models.ts 中，手动定义
export interface GenericResponse {
  success: boolean
  error?: string
}

export function isWailsAvailable(): boolean {
  return typeof window !== 'undefined' && !!(window as any).chrome?.webview
}

// #4/#20 修复：简化 Wails 适配器
// Wails v2 绑定机制将 Go 方法的单个返回值序列化为 JSON 对象
// 直接使用 Response 对象的 data 字段即可，无需复杂的 splitMultiReturn 逻辑

function getMachRangeFromFiles(files: PrbFileInfo[]): number[] {
  const machNumbers = files.map(file => file.machNumber).filter(Number.isFinite)
  if (machNumbers.length === 0) return []
  return [Math.min(...machNumbers), Math.max(...machNumbers)]
}

function createLoadPrbResult(files: PrbFileInfo[], machRange?: number[], warnings?: string[]): LoadPrbResult {
  return new backend.LoadPrbResult({
    files,
    machRange: machRange ?? getMachRangeFromFiles(files),
    warnings: warnings ?? [],
  })
}

export const api = {
  async loadPrbFiles(): Promise<[GenericResponse, LoadPrbResult | null]> {
    try {
      const result = await WailsApp.LoadPrbFiles()
      // Wails 返回 LoadPrbResponse 对象，直接使用其字段
      if (!result.success) {
        return [{ success: false, error: result.error } as GenericResponse, null]
      }
      if (result.data && Array.isArray(result.data.files)) {
        return [
          { success: true } as GenericResponse,
          createLoadPrbResult(result.data.files, result.data.machRange, result.data.warnings),
        ]
      }

      // 回退：从 GetPrbFiles 获取文件信息
      const files = await WailsApp.GetPrbFiles()
      if (Array.isArray(files) && files.length > 0) {
        return [{ success: true } as GenericResponse, createLoadPrbResult(files)]
      }

      return [{ success: false, error: 'PRB 文件加载成功但未返回文件信息' } as GenericResponse, null]
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
      if (!result.success) {
        return [{ success: false, error: result.error } as GenericResponse, []]
      }
      return [{ success: true } as GenericResponse, (result.data ?? []).filter((item): item is InterpolationResult => item !== null)]
    } catch (e) {
      return [{ success: false, error: String(e) } as GenericResponse, []]
    }
  },

  async calculate(input: InterpolationInput): Promise<[GenericResponse, InterpolationResult | null]> {
    try {
      const result = await WailsApp.Calculate(input)
      if (!result.success) {
        return [{ success: false, error: result.error } as GenericResponse, null]
      }
      return [{ success: true } as GenericResponse, result.data ?? null]
    } catch (e) {
      return [{ success: false, error: String(e) } as GenericResponse, null]
    }
  },

  async importCsvData(): Promise<[GenericResponse, InterpolationInput[]]> {
    try {
      const result = await WailsApp.ImportCsvData()
      if (!result.success) {
        return [{ success: false, error: result.error } as GenericResponse, []]
      }
      return [{ success: true } as GenericResponse, result.data ?? []]
    } catch (e) {
      return [{ success: false, error: String(e) } as GenericResponse, []]
    }
  },

  async openHelpDoc(): Promise<GenericResponse> {
    try {
      await WailsApp.OpenHelpDoc()
      return { success: true } as GenericResponse
    } catch (e) {
      return { success: false, error: String(e) } as GenericResponse
    }
  },
}
