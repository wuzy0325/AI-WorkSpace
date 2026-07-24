// preload 脚本：在 renderer 进程启动前注入受限的 Electron API。
//
// 安全设计：
//   - contextIsolation: true（main.cjs 中启用）：preload 与 renderer 上下文隔离
//   - nodeIntegration: false：renderer 无法直接 require Node 模块
//   - sandbox: true：preload 也运行在沙箱中，仅可用 Electron 提供的 API
//
// 暴露给 renderer 的 API（window.electronAPI）：
//   - pickFile(opts?)：触发主进程原生单选文件对话框，返回选中文件路径或空串
//   - pickFiles(opts?)：触发主进程原生多选文件对话框，返回路径数组或空数组
//
// 与 daq-p1604 preload 的差异：
//   - daq-p1604 只暴露 showOpenDialog（目录选择，用于录制/日志路径）
//   - probe-interpolator 暴露 pickFile / pickFiles（文件选择，用于 .prb / CSV 加载）
//   - opts 经 IPC 透传给主进程，支持 title 与 filters 字段
const { contextBridge, ipcRenderer } = require('electron')

contextBridge.exposeInMainWorld('electronAPI', {
  pickFile: (opts) => ipcRenderer.invoke('dialog:pick-file', opts),
  pickFiles: (opts) => ipcRenderer.invoke('dialog:pick-files', opts),
})
