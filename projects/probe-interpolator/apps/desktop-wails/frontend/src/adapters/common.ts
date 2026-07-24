// adapters/common.ts 抽取 5 / 3 / 7 孔适配器中完全相同的公共类型与常量。
//
// 与原版的差异：
//   - isWailsAvailable 改为始终返回 true（Win7 版运行在 Electron 中，HTTP 总是可用）
//     保留导出避免改动 3 个 Workspace.vue 组件的 import 与守卫逻辑
//   - GenericResponse 保留：adapter 仍按 [GenericResponse, Data] 元组返回，组件无需 try/catch

/**
 * GenericResponse 是 adapter 层统一错误响应签名。
 * 各 Response 类型（LoadPrbResponse/CalculateResponse/...）已含 success/error/data 字段，
 * 此接口仅作为通用错误处理的简化签名。
 */
export interface GenericResponse {
  success: boolean
  error?: string
}

/**
 * isWailsAvailable 在 Win7 分支始终返回 true。
 *
 * 历史背景：原 Wails 版用此函数检测是否运行在 Wails WebView 中，
 * 浏览器调试时所有调用会失败 → 组件给出"当前不在 Wails 环境中运行"提示。
 *
 * Win7 版改用 Electron + HTTP，runtime 总是可用，此函数退化为常量 true。
 * 保留导出避免改动 3 个 Workspace.vue 组件的守卫逻辑（FiveHoleWorkspace.vue 等）。
 */
export function isWailsAvailable(): boolean {
  return true
}
