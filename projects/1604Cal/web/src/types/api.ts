export interface HealthResponse {
  status: string
}

export interface ApiResponse<T> {
  success: boolean
  code?: string
  message?: string
  data: T
}

export interface StreamEventPayload {
  type: string
  data?: unknown
}

export interface ActionResult {
  ok: boolean
  error?: string
  detail?: string
}
