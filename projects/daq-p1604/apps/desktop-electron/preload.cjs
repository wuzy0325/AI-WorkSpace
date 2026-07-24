// preload 脚本：在 renderer 进程启动前注入受限的 Electron API。
//
// 安全设计：
//   - contextIsolation: true（main.cjs 中启用）：preload 与 renderer 上下文隔离
//   - nodeIntegration: false：renderer 无法直接 require Node 模块
//   - sandbox: true：preload 也运行在沙箱中，仅可用 Electron 提供的 API
//
// 暴露给 renderer 的 API（window.electronAPI）：
//   - showOpenDialog()：触发主进程的原生目录选择对话框
//     详见 main.cjs 的 ipcMain.handle('dialog:pick-directory', ...)
//   - bridge 层（logBridge.ts / recordingBridge.ts）调用此 API 替代 Wails v3 的 PickDirectory
const { contextBridge, ipcRenderer } = require('electron')

contextBridge.exposeInMainWorld('electronAPI', {
  showOpenDialog: () => ipcRenderer.invoke('dialog:pick-directory'),
})
