export interface PrbValidRange {
  alphaMin: number
  alphaMax: number
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

export interface InterpolationInput {
  P1: number
  P2: number
  P3: number
  Patm: number
  Tatm: number
  pressureMode: string
}

export interface InterpolationResult {
  alpha: number
  machNumber: number
  P0: number
  Ps: number
  iterationCount: number
  isValid: boolean
  warning?: string
}

export interface GenericResponse {
  success: boolean
  error?: string
}

function okResponse(): GenericResponse {
  return { success: true }
}

function errResponse(error: string): GenericResponse {
  return { success: false, error }
}

async function fetchAPI<T>(path: string, options?: RequestInit): Promise<T> {
  const resp = await fetch(path, options)
  return resp.json()
}

interface LoadPrbResp {
  success: boolean
  error?: string
  data?: LoadPrbResult
}

interface CalculateResp {
  success: boolean
  error?: string
  data?: InterpolationResult
}

interface BatchCalculateResp {
  success: boolean
  error?: string
  data?: InterpolationResult[]
}

interface ImportCsvResp {
  success: boolean
  error?: string
  data?: InterpolationInput[]
}

export const api = {
  async loadPrbFiles(files: File[]): Promise<[GenericResponse, LoadPrbResult | null]> {
    try {
      const formData = new FormData()
      for (const f of files) {
        formData.append('files', f)
      }
      const result = await fetchAPI<LoadPrbResp>('/api/load-prb', {
        method: 'POST',
        body: formData,
      })
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
      return [errResponse('PRB 文件加载成功但未返回文件信息'), null]
    } catch (e) {
      return [errResponse(String(e)), null]
    }
  },

  async isPrbLoaded(): Promise<boolean> {
    try {
      const result = await fetchAPI<{ loaded: boolean }>('/api/is-prb-loaded')
      return result.loaded
    } catch {
      return false
    }
  },

  async getPrbFiles(): Promise<PrbFileInfo[]> {
    try {
      const result = await fetchAPI<{ files: PrbFileInfo[] }>('/api/prb-files')
      return result.files ?? []
    } catch {
      return []
    }
  },

  async batchCalculate(datas: InterpolationInput[]): Promise<[GenericResponse, InterpolationResult[]]> {
    try {
      const result = await fetchAPI<BatchCalculateResp>('/api/batch-calculate', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(datas),
      })
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
      const result = await fetchAPI<CalculateResp>('/api/calculate', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(input),
      })
      if (!result.success) {
        return [errResponse(result.error ?? '未知错误'), null]
      }
      return [okResponse(), result.data ?? null]
    } catch (e) {
      return [errResponse(String(e)), null]
    }
  },

  async importCsvData(file: File): Promise<[GenericResponse, InterpolationInput[]]> {
    try {
      const formData = new FormData()
      formData.append('file', file)
      const result = await fetchAPI<ImportCsvResp>('/api/import-csv', {
        method: 'POST',
        body: formData,
      })
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
      window.open('/api/help', '_blank')
      return okResponse()
    } catch (e) {
      return errResponse(String(e))
    }
  },
}
