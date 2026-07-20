// wails-adapter.ts 是前端调用 Wails 后端方法的统一封装层。
// 作用：
//   1. 隔离 bindings 目录路径细节（Wails v3 按 module path 生成嵌套目录）
//   2. 统一错误处理（Wails 抛出的 error 转 string）
//   3. 暴露强类型签名供组件使用
//
// 注意：bindings 目录由 `wails3 generate bindings` 自动生成，不要手动修改。

import * as WailsApp from '../bindings/probe-interpolator/apps/desktop-wails/backend/app'
import * as models from '../bindings/probe-interpolator/apps/desktop-wails/backend/models'

// 探针类型与元信息类型，从 bindings 模块 re-export
export type ProbeKind = string  // 实际取值: 'three' | 'five' | 'seven'
export type ProbeInfo = models.ProbeInfo

// 探针类型常量，与后端 ProbeKindThree/Five/Seven 对齐
export const PROBE_KIND = {
  THREE: 'three' as ProbeKind,
  FIVE: 'five' as ProbeKind,
  SEVEN: 'seven' as ProbeKind,
}

/**
 * GetAvailableProbes 返回启动选择页要展示的探针列表。
 */
export async function GetAvailableProbes(): Promise<ProbeInfo[]> {
  return await WailsApp.GetAvailableProbes()
}

/**
 * SetActiveProbe 设置当前会话的探针类型。
 * v0.1.1 起允许覆盖式更新：用户从工作区返回欢迎页后可再次选择其他探针类型。
 */
export async function SetActiveProbe(kind: ProbeKind): Promise<void> {
  await WailsApp.SetActiveProbe(kind)
}

/**
 * ClearActiveProbe 清空当前会话的探针类型选择。
 * 配合前端"返回欢迎页"按钮：调用后 GetActiveProbe 返回空字符串。
 * 注意：本方法不清理各探针 service 的 .prb / 输入状态，用户再次进入同一探针时可恢复。
 */
export async function ClearActiveProbe(): Promise<void> {
  await WailsApp.ClearActiveProbe()
}

/**
 * GetActiveProbe 返回当前会话已选择的探针类型。
 * 未选择时返回空字符串（v0.1.1 起不再抛错），前端按空字符串判定留在选择页。
 */
export async function GetActiveProbe(): Promise<ProbeKind> {
  return await WailsApp.GetActiveProbe()
}
