// preload 脚本：在 renderer 进程启动前注入受限的 Electron API。
//
// 安全设计：
//   - contextIsolation: true（main.cjs 中启用）：preload 与 renderer 上下文隔离
//   - nodeIntegration: false：renderer 无法直接 require Node 模块
//   - sandbox: true：preload 也运行在沙箱中，仅可用 Electron 提供的 API
//
// 与 probe-interpolator preload 的差异：
//   - motion-controller 后端无文件选择对话框需求（运动控制器配置由后端自管理），
//     preload 不暴露任何 IPC，仅作为占位存在
//   - 未来若需要原生对话框（如导出配置文件），可在此处扩展 contextBridge.exposeInMainWorld
const { contextBridge } = require('electron')

// 暴露空对象，保持与 main.cjs preload 配置的一致性，
// 同时为未来扩展（如配置导入导出对话框）预留接口注入点
contextBridge.exposeInMainWorld('electronAPI', {})
