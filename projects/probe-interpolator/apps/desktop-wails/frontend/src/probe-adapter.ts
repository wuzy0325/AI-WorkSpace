// probe-adapter.ts 是前端调用 Go 后端"探针选择"相关方法的封装层（Win7 HTTP 版）。
//
// 与原 wails-adapter.ts 的差异：
//   - 移除 Wails bindings 依赖，改用 fetch 调用 http://127.0.0.1:18183/api/probe/*
//   - 类型从 backend/models.ts 改为本地手写 interface（与 backend/probe_selector.go 对齐）
//   - 错误处理：fetch 失败由 httpClient 抛 Error，本层 catch 后转为空数组/空字符串保守返回
//
// 暴露的方法签名与原 wails-adapter.ts 完全一致，App.vue / ProbeSelectPage.vue 无需改动。

import { get, post } from './bridge/httpClient'

/** 探针类型字符串字面量（与后端 ProbeKindThree/Five/Seven 对齐） */
export type ProbeKind = 'three' | 'five' | 'seven'

/** 探针元信息（与 backend/probe_selector.go ProbeInfo 对应） */
export interface ProbeInfo {
  kind: ProbeKind
  name: string
  description: string
  holes: number
}

/** 探针类型常量，便于组件 switch / 字面量比较 */
export const PROBE_KIND = {
  THREE: 'three' as ProbeKind,
  FIVE: 'five' as ProbeKind,
  SEVEN: 'seven' as ProbeKind,
}

/** 后端 /api/probe/active GET 返回的 {kind: string} 信封内 data 字段形状 */
interface ActiveProbeResponse {
  kind: string
}

/**
 * GetAvailableProbes 返回启动选择页要展示的探针列表。
 * 失败时返回空数组（保守默认，让用户至少能看到空白页而非崩溃）。
 */
export async function GetAvailableProbes(): Promise<ProbeInfo[]> {
  try {
    return await get<ProbeInfo[]>('/api/probe/available')
  } catch {
    return []
  }
}

/**
 * SetActiveProbe 设置当前会话的探针类型。
 * v0.1.1 起允许覆盖式更新：用户从工作区返回欢迎页后可再次选择其他探针类型。
 * 失败时由 httpClient 抛 Error，调用方（ProbeSelectPage.vue）需 try/catch。
 */
export async function SetActiveProbe(kind: ProbeKind): Promise<void> {
  await post('/api/probe/active', { kind })
}

/**
 * ClearActiveProbe 清空当前会话的探针类型选择。
 * 配合前端"返回欢迎页"按钮：调用后 GetActiveProbe 返回空字符串。
 * 注意：本方法不清理各探针 service 的 .prb / 输入状态，用户再次进入同一探针时可恢复。
 */
export async function ClearActiveProbe(): Promise<void> {
  await post('/api/probe/clear')
}

/**
 * GetActiveProbe 返回当前会话已选择的探针类型。
 * 未选择时返回空字符串（v0.1.1 起不再抛错），前端按空字符串判定留在选择页。
 * 失败时也返回空字符串（保守默认，避免阻塞欢迎页加载）。
 */
export async function GetActiveProbe(): Promise<ProbeKind> {
  try {
    const resp = await get<ActiveProbeResponse>('/api/probe/active')
    return (resp.kind ?? '') as ProbeKind
  } catch {
    return '' as ProbeKind
  }
}
