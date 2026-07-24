const { app, BrowserWindow, dialog, ipcMain } = require('electron')
const { spawn } = require('node:child_process')
const http = require('node:http')
const path = require('node:path')

// 后端 HTTP server 地址：与 main.go listenAddr 一致。
// 端口 18183：与 daq-t1603（18181）/ daq-p1604（18182）区分，避免同机多开冲突。
const backendURL = 'http://127.0.0.1:18183'
let backendProcess = null
let mainWindow = null

// 后端 exe 路径：
//   - 打包后：app.asar.unpacked 之外，由 extraResources 复制到 resources/backend/
//   - 开发期：desktop-electron/backend/ 下，由 build-backend.ps1 生成
function backendPath() {
  if (app.isPackaged) {
    return path.join(process.resourcesPath, 'backend', 'probe-interpolator-backend.exe')
  }
  return path.join(__dirname, 'backend', 'probe-interpolator-backend.exe')
}

// 启动 Go 后端子进程：
//   - windowsHide: 隐藏控制台窗口，避免桌面应用出现黑框
//   - stdio: 'ignore': 后端日志已写入 stderr，无需转发到 Electron stdout
function startBackend() {
  backendProcess = spawn(backendPath(), [], {
    cwd: path.dirname(backendPath()),
    windowsHide: true,
    stdio: 'ignore',
  })
  backendProcess.once('error', (error) => {
    dialog.showErrorBox('探针插值器后端启动失败', error.message)
    app.quit()
  })
  backendProcess.once('exit', (code) => {
    backendProcess = null
    // 非零退出且非主动关闭时，提示用户并退出
    if (!app.isQuitting && code !== 0) {
      dialog.showErrorBox('探针插值器后端异常退出', `退出代码: ${code}`)
      app.quit()
    }
  })
}

// 轮询后端 /api/health 端点，确认 Go server 已就绪。
// 探针插值器启动很快（无硬件初始化），10 秒上限足够覆盖冷启动。
function waitForBackend(timeoutMs = 10000) {
  const deadline = Date.now() + timeoutMs
  return new Promise((resolve, reject) => {
    const probe = () => {
      const request = http.get(`${backendURL}/api/health`, (response) => {
        response.resume()
        if (response.statusCode === 200) {
          resolve()
          return
        }
        retry()
      })
      request.setTimeout(1000, () => request.destroy())
      request.on('error', retry)
    }
    const retry = () => {
      if (Date.now() >= deadline) {
        reject(new Error('10 秒内未能连接后端服务'))
        return
      }
      setTimeout(probe, 250)
    }
    probe()
  })
}

function createWindow() {
  mainWindow = new BrowserWindow({
    title: '探针插值器',
    width: 1400,
    height: 900,
    minWidth: 1100,
    minHeight: 700,
    show: false,
    webPreferences: {
      preload: path.join(__dirname, 'preload.cjs'),
      contextIsolation: true,
      nodeIntegration: false,
      sandbox: true,
    },
  })
  mainWindow.removeMenu()
  // 等渲染进程首次完成布局后再 show，避免白屏闪烁
  mainWindow.once('ready-to-show', () => mainWindow.show())
  // 加载 Go 后端嵌入的前端静态资源（http.FileServer 注册在 mux "/"）
  mainWindow.loadURL(backendURL)
}

// IPC：renderer 调用 window.electronAPI.pickFile() 触发原生单选文件对话框。
// 返回选中文件绝对路径；用户取消返回空串，调用方据此中止加载/导入操作。
//
// 参数 opts 形状（与 preload.cjs contextBridge 对应）：
//   { title?: string, filters?: [{ name: string, extensions: string[] }] }
ipcMain.handle('dialog:pick-file', async (_event, opts) => {
  const params = {
    properties: ['openFile'],
  }
  if (opts && typeof opts === 'object') {
    if (typeof opts.title === 'string') {
      params.title = opts.title
    }
    if (Array.isArray(opts.filters)) {
      params.filters = opts.filters
    }
  }
  const result = await dialog.showOpenDialog(mainWindow, params)
  return result.canceled ? '' : result.filePaths[0]
})

// IPC：renderer 调用 window.electronAPI.pickFiles() 触发原生多选文件对话框。
// 返回选中文件绝对路径数组；用户取消返回空数组。
//
// 用于 5 孔 / 7 孔 LoadPrbFiles（一次加载多个 .prb 校准文件）。
ipcMain.handle('dialog:pick-files', async (_event, opts) => {
  const params = {
    properties: ['openFile', 'multiSelections'],
  }
  if (opts && typeof opts === 'object') {
    if (typeof opts.title === 'string') {
      params.title = opts.title
    }
    if (Array.isArray(opts.filters)) {
      params.filters = opts.filters
    }
  }
  const result = await dialog.showOpenDialog(mainWindow, params)
  return result.canceled ? [] : result.filePaths
})

app.whenReady().then(async () => {
  startBackend()
  try {
    await waitForBackend()
    createWindow()
  } catch (error) {
    dialog.showErrorBox('探针插值器启动失败', error.message)
    app.quit()
  }
})

app.on('window-all-closed', () => app.quit())

app.on('before-quit', () => {
  app.isQuitting = true
  // 主动 kill 后端：Go 后端进程终止会触发 srv.Shutdown + app.ServiceShutdown 优雅关闭
  if (backendProcess) {
    backendProcess.kill()
    backendProcess = null
  }
})
