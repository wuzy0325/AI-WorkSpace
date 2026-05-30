import * as WailsApp from '../wailsjs/go/backend/App'
import { backend } from '../wailsjs/go/models'

export type PrbFileInfo = backend.PrbFileInfo
export type PrbValidRange = backend.PrbValidRange
export type LoadPrbResult = backend.LoadPrbResult
export type InterpolationInput = backend.InterpolationInput
export type InterpolationResult = backend.InterpolationResult

export interface GenericResponse {
  success: boolean
  error?: string
}

export function isWailsAvailable(): boolean {
  return typeof window !== 'undefined' && !!(window as any).go?.backend?.App
}

function okResponse(): GenericResponse {
  return { success: true }
}

function errResponse(error: string): GenericResponse {
  return { success: false, error }
}

export const api = {
  async loadPrbFiles(): Promise<[GenericResponse, LoadPrbResult | null]> {
    try {
      const result = await WailsApp.LoadPrbFiles()
      if (!result.success) {
        return [errResponse(result.error ?? '未知错误'), null]
      }
      if (result.data && Array.isArray(result.data.files)) {
        const loadResult: LoadPrbResult = {
          files: result.data.files,
          machRange: result.data.machRange ?? [],
          warnings: result.data.warnings ?? [],
        }
        return [okResponse(), loadResult]
      }
      const files = await WailsApp.GetPrbFiles()
      if (Array.isArray(files) && files.length > 0) {
        const fallbackResult: LoadPrbResult = { files, machRange: [], warnings: [] }
        return [okResponse(), fallbackResult]
      }
      return [errResponse('PRB 文件加载成功但未返回文件信息'), null]
    } catch (e) {
      return [errResponse(String(e)), null]
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
        return [errResponse(result.error ?? '未知错误'), []]
      }
      return [okResponse(), result.data ?? []]
    } catch (e) {
      return [errResponse(String(e)), []]
    }
  },

  async calculate(input: InterpolationInput): Promise<[GenericResponse, InterpolationResult | null]> {
    try {
      const result = await WailsApp.Calculate(input)
      if (!result.success) {
        return [errResponse(result.error ?? '未知错误'), null]
      }
      return [okResponse(), result.data ?? null]
    } catch (e) {
      return [errResponse(String(e)), null]
    }
  },

  async importCsvData(): Promise<[GenericResponse, InterpolationInput[]]> {
    try {
      const result = await WailsApp.ImportCsvData()
      if (!result.success) {
        return [errResponse(result.error ?? '未知错误'), []]
      }
      return [okResponse(), result.data ?? []]
    } catch (e) {
      return [errResponse(String(e)), []]
    }
  },

  async openHelpDoc(): Promise<GenericResponse> {
    try {
      await WailsApp.OpenHelpDoc()
      return okResponse()
    } catch (e) {
      return errResponse(String(e))
    }
  },
}
