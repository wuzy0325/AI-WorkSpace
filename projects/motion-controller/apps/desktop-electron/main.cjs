// Motion Controller Electron 主进程入口（Win7 兼容版）
//
// 职责：
//   1. spawn Go 后端 motion-controller-backend.exe（监听 127.0.0.1:16888）
//   2. 等待后端健康检查通过后，加载主窗口（http://127.0.0.1:16888）
//   3. 退出时主动 kill 后端进程，触发 Go 优雅关闭
//
// 安全约束（与 wind-daq / probe-interpolator Win7 基线一致）：
//   - contextIsolation: true —— 渲染进程与 preload 隔离
//   - sandbox: true —— preload 也运行在沙箱内
//   - nodeIntegration: false —— 渲染进程不能直接 require Node 模块
//   - webSecurity: true —— 保持同源策略
//
// 与 probe-interpolator Win7 版的差异：
//   - 端口 16888（probe-interpolator 用 18183）
//   - 窗口固定 1350x740 不可调整大小（与 trunk 主分支 Wails 窗口配置一致）
//   - preload 不暴露任何 IPC（motion-controller 后端无文件选择对话框需求，
//     运动控制器配置存于 %AppData%/motion-controller/motion-profiles.json，由后端自管理）
//   - 应用标题"运动控制器"

const { app, BrowserWindow, dialog } = require('electron')
const { spawn } = require('node:child_process')
const http = require('node:http')
const path = require('node:path')

// 后端 HTTP server 地址：与 apps/desktop-wails/main.go listenAddr 一致。
// 端口 16888：与 wind-daq（8900/8901）/ daq-t1603（18181）/ daq-p1604（18182）/ probe-interpolator（18183）区分。
const backendURL = 'http://127.0.0.1:16888'
let backendProcess = null
let mainWindow = null

// 后端 exe 路径：
//   - 打包后：app.asar.unpacked 之外，由 extraResources 复制到 resources/backend/
//   - 开发期：desktop-electron/backend/ 下，由 build-backend.ps1 生成
function backendPath() {
  if (app.isPackaged) {
    return path.join(process.resourcesPath, 'backend', 'motion-controller-backend.exe')
  }
  return path.join(__dirname, 'backend', 'motion-controller-backend.exe')
}

// spawn Go 后端子进程：
//   - windowsHide: 隐藏控制台窗口，避免桌面应用出现黑框
//   - stdio: 'ignore': 后端日志由 slog 写入 stderr，无需转发到 Electron stdout
function startBackend() {
  backendProcess = spawn(backendPath(), [], {
    cwd: path.dirname(backendPath()),
    windowsHide: true,
    stdio: 'ignore',
  })
  backendProcess.once('error', (error) => {
    dialog.showErrorBox('运动控制器后端启动失败', error.message)
    app.quit()
  })
  backendProcess.once('exit', (code) => {
    backendProcess = null
    // 非零退出且非主动关闭时，提示用户并退出
    if (!app.isQuitting && code !== 0) {
      dialog.showErrorBox('运动控制器后端异常退出', `退出代码: ${code}`)
      app.quit()
    }
  })
}

// 轮询后端 /api/health 端点，确认 Go server 已就绪。
// motion-controller 启动需加载 motion-profiles.json + 初始化硬件工厂，10 秒上限足够覆盖冷启动。
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

// 应用图标：开发态指向 desktop-wails/appicon.ico（与 electron-builder win.icon 同源）。
// 打包后 BrowserWindow 不指定 icon，Electron 自动从 .exe 提取内嵌图标。
function getAppIcon() {
  if (app.isPackaged) {
    return undefined
  }
  return path.join(__dirname, '..', 'desktop-wails', 'appicon.ico')
}

function createWindow() {
  mainWindow = new BrowserWindow({
    title: '运动控制器',
    width: 1350,
    height: 740,
    minWidth: 1350,
    minHeight: 740,
    // resizable: false 与 trunk 主分支 Wails 窗口 DisableResize: true 对齐
    resizable: false,
    maximizable: false,
    show: false,
    icon: getAppIcon(),
    webPreferences: {
      preload: path.join(__dirname, 'preload.cjs'),
      contextIsolation: true,
      nodeIntegration: false,
      sandbox: true,
      webSecurity: true,
    },
  })
  mainWindow.removeMenu()
  // 等渲染进程首次完成布局后再 show，避免白屏闪烁
  mainWindow.once('ready-to-show', () => mainWindow.show())
  // 加载 Go 后端嵌入的前端静态资源（http.FileServer 注册在 mux "/"）
  mainWindow.loadURL(backendURL)
}

app.whenReady().then(async () => {
  startBackend()
  try {
    await waitForBackend()
    createWindow()
  } catch (error) {
    dialog.showErrorBox('运动控制器启动失败', error.message)
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
