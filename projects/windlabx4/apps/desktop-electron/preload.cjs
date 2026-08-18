// preload 脚本：在渲染进程加载前注入到 DOM，
// 通过 contextBridge 暴露受限的 IPC 通道给渲染进程。
//
// 仅暴露以下两个能力：
//   1. showOpenDialog() —— 触发原生目录选择对话框
//   2. openMotionWindow() —— 触发运动控制器独立窗口
//
// 其他后端能力均通过 HTTP API（fetch http://127.0.0.1:8900/api/*）实现，
// 不依赖 IPC，保持架构简单。

const { contextBridge, ipcRenderer } = require('electron')

contextBridge.exposeInMainWorld('electronAPI', {
  // 弹出原生目录选择对话框，返回选中的目录绝对路径，取消则返回空字符串
  showOpenDialog: () => ipcRenderer.invoke('dialog:pick-directory'),
  // 启动运动控制器独立窗口（spawn motion-only 子进程 + 创建 BrowserWindow）
  openMotionWindow: () => ipcRenderer.invoke('app:open-motion-window'),
})
