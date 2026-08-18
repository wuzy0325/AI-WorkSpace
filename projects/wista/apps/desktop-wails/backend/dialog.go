package backend

import "errors"

// ErrDialogNotSupported 表示后端不再处理目录选择对话框。
//
// Win7 分支采用 Electron 作为前端壳，原生目录选择由 Electron 主进程
// 通过 BrowserWindow.showOpenDialog 处理，前端直接拿到路径后通过
// HTTP API 调用后端。后端保留该方法仅为兼容原有 Service 接口签名。
var ErrDialogNotSupported = errors.New("dialog not supported in win7 build, use electron IPC instead")