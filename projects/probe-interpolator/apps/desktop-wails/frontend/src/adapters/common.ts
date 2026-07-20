// adapters/common.ts 抽取 5 / 3 / 7 孔适配器中完全相同的公共代码：
//   - GenericResponse 接口（统一错误响应签名）
//   - isWailsAvailable 检测函数
//
// 历史背景：3 个适配器各自重复定义了这两段代码，code-review P3 抽出。
// 注意：bindings 目录由 wails3 generate bindings 自动生成，不要手动修改；
//       本文件是手写的适配层，与 bindings 解耦。

// GenericResponse 不在 Wails 生成的 models.js 中，手动定义。
// 各 Response 类型（LoadPrbResponse/CalculateResponse/...）已含 success/error/data 字段，
// 此接口仅作为通用错误处理的简化签名。
export interface GenericResponse {
  success: boolean
  error?: string
}

// isWailsAvailable 检测当前是否运行在 Wails WebView 中。
// 开发时用浏览器调试，所有调用会失败 → 组件应给出友好提示而非崩溃。
export function isWailsAvailable(): boolean {
  return typeof window !== 'undefined' && !!(window as any).chrome?.webview
}
