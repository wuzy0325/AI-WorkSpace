// wind-daq Electron 主进程入口
//
// 职责：
//   1. spawn Go 后端 wind-daq-backend.exe（监听 127.0.0.1:8900）
//   2. 等待后端健康检查通过后，加载主窗口（http://127.0.0.1:8900）
//   3. 通过 IPC 处理文件对话框（pickDirectory）和运动控制器独立窗口（openMotionWindow）
//
// 安全约束（与 daq-t1603 Win7 基线一致）：
//   - contextIsolation: true —— 渲染进程与 preload 隔离
//   - sandbox: true —— preload 也运行在沙箱内
//   - nodeIntegration: false —— 渲染进程不能直接 require Node 模块
//   - webSecurity: true —— 保持同源策略
//
// 注意：motion-only 子进程也是同一份 wind-daq-backend.exe，通过 --motion-only flag 启动，
// 监听独立端口 8901，避免与主进程 8900 冲突。

const { app, BrowserWindow, dialog, ipcMain } = require('electron')
const { spawn } = require('node:child_process')
const http = require('node:http')
const path = require('node:path')

// 主进程地址（与 apps/desktop-wails/main.go listenAddr 对应）
const mainBackendURL = 'http://127.0.0.1:8900'
// motion-only 子进程地址（独立端口避免冲突）
const motionBackendURL = 'http://127.0.0.1:8901'

// 开发态热加载支持：
//   ELECTRON_DEV_URL —— 指定后主窗口加载该 URL（通常是 Vite dev server http://127.0.0.1:9246），
//                        而非内嵌静态资源。前端 Vue/TS 改动通过 Vite HMR 实时刷新。
//   ELECTRON_SKIP_BACKEND —— 指定非空值时跳过 spawn 后端进程，由开发脚本独立启动 Go 后端，
//                             便于后端单独重启不影响前端 HMR 状态。
const devFrontendURL = process.env.ELECTRON_DEV_URL || ''
const skipMainBackend = Boolean(process.env.ELECTRON_SKIP_BACKEND)

// 应用图标：开发态指向 desktop-wails/appicon.ico（与 electron-builder win.icon 同源）。
// 打包后 BrowserWindow 不指定 icon，Electron 自动从 .exe 提取内嵌图标。
function getAppIcon() {
  if (app.isPackaged) {
    return undefined
  }
  return path.join(__dirname, '..', 'desktop-wails', 'appicon.ico')
}

let mainBackendProcess = null
let mainWindow = null
let motionWindow = null
let motionBackendProcess = null

// 后端二进制路径：
// - 开发态：desktop-electron/backend/wind-daq-backend.exe（由 build-backend.ps1 生成）
// - 打包后：resources/backend/wind-daq-backend.exe
function backendPath() {
  if (app.isPackaged) {
    return path.join(process.resourcesPath, 'backend', 'wind-daq-backend.exe')
  }
  return path.join(__dirname, 'backend', 'wind-daq-backend.exe')
}

// spawn Go 后端进程
function startMainBackend() {
  const exe = backendPath()
  mainBackendProcess = spawn(exe, [], {
    cwd: path.dirname(exe),
    windowsHide: true,
    stdio: 'ignore',
  })
  mainBackendProcess.once('error', (error) => {
    dialog.showErrorBox('Wind DAQ 后端启动失败', error.message)
    app.quit()
  })
  mainBackendProcess.once('exit', (code) => {
    mainBackendProcess = null
    if (!app.isQuitting && code !== 0) {
      dialog.showErrorBox('Wind DAQ 后端异常退出', `退出代码: ${code}`)
      app.quit()
    }
  })
}

// 轮询健康检查端点，等待后端 HTTP server 就绪
function waitForBackend(url, timeoutMs = 15000) {
  const deadline = Date.now() + timeoutMs
  return new Promise((resolve, reject) => {
    const probe = () => {
      const request = http.get(`${url}/api/health`, (response) => {
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
        reject(new Error('15 秒内未能连接后端服务'))
        return
      }
      setTimeout(probe, 250)
    }
    probe()
  })
}

function createMainWindow() {
  mainWindow = new BrowserWindow({
    title: '风洞数据采集',
    width: 1600,
    height: 900,
    minWidth: 1280,
    minHeight: 720,
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
  mainWindow.once('ready-to-show', () => mainWindow.show())
  // 开发态加载 Vite dev server（HMR），生产态加载后端内嵌静态资源
  if (devFrontendURL) {
    mainWindow.loadURL(devFrontendURL)
    // 开发态自动打开 DevTools 便于调试
    mainWindow.webContents.once('did-finish-load', () => {
      mainWindow.webContents.openDevTools({ mode: 'right' })
    })
  } else {
    mainWindow.loadURL(mainBackendURL)
  }

  // 主窗口关闭时，连同 motion 子窗口和子进程一起清理
  mainWindow.on('closed', () => {
    mainWindow = null
    closeMotionWindow()
  })
}

// spawn motion-only 子进程并打开独立窗口
// 流程：
//   1. spawn wind-daq-backend.exe --motion-only --parent-pid=<主进程 PID>
//   2. 等待 8901 健康检查通过
//   3. 创建第二个 BrowserWindow 加载 http://127.0.0.1:8901
async function openMotionWindow() {
  if (motionWindow) {
    // 已有 motion 窗口，直接聚焦
    if (motionWindow.isMinimized()) motionWindow.restore()
    motionWindow.focus()
    return
  }
  if (motionBackendProcess) {
    // 子进程已存在但窗口被关闭，重新打开窗口
    await waitForBackend(motionBackendURL, 8000).catch(() => {})
    createMotionWindow()
    return
  }

  const exe = backendPath()
  motionBackendProcess = spawn(exe, ['--motion-only', `--parent-pid=${process.pid}`], {
    cwd: path.dirname(exe),
    windowsHide: true,
    stdio: 'ignore',
  })
  motionBackendProcess.once('error', (error) => {
    dialog.showErrorBox('运动控制器后端启动失败', error.message)
    motionBackendProcess = null
  })
  motionBackendProcess.once('exit', () => {
    motionBackendProcess = null
    // 子进程退出时关闭 motion 窗口
    closeMotionWindow()
  })

  try {
    await waitForBackend(motionBackendURL, 15000)
  } catch (error) {
    dialog.showErrorBox('运动控制器后端启动失败', error.message)
    if (motionBackendProcess) {
      motionBackendProcess.kill()
      motionBackendProcess = null
    }
    return
  }
  createMotionWindow()
}

function createMotionWindow() {
  motionWindow = new BrowserWindow({
    title: '运动控制器',
    width: 1280,
    height: 800,
    minWidth: 1024,
    minHeight: 600,
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
  motionWindow.removeMenu()
  motionWindow.once('ready-to-show', () => motionWindow.show())
  // 前端 SPA 用 hash history（createWebHashHistory），路由 /#/motion 对应 MotionView。
  // 若只加载根路径 http://127.0.0.1:8901，SPA 默认进 / 路由（MainDashboardView），
  // 用户看到的 motion 窗口会显示主界面而非运动控制面板。
  motionWindow.loadURL(`${motionBackendURL}/#/motion`)
  motionWindow.on('closed', () => {
    motionWindow = null
  })
}

function closeMotionWindow() {
  if (motionWindow) {
    motionWindow.close()
    motionWindow = null
  }
  if (motionBackendProcess) {
    motionBackendProcess.kill()
    motionBackendProcess = null
  }
}

// IPC：渲染进程通过 window.electronAPI.showOpenDialog() 调用，
// 返回用户选择的目录路径，取消则返回空字符串
ipcMain.handle('dialog:pick-directory', async () => {
  const result = await dialog.showOpenDialog(mainWindow, {
    properties: ['openDirectory', 'createDirectory'],
  })
  return result.canceled ? '' : result.filePaths[0]
})

// IPC：渲染进程通过 window.electronAPI.openMotionWindow() 调用，
// 触发 motion-only 子进程启动并打开独立窗口
ipcMain.handle('app:open-motion-window', async () => {
  await openMotionWindow()
  return true
})

app.whenReady().then(async () => {
  // 开发态跳过后端 spawn（由 dev 脚本独立启动 Go 后端），但仍等待健康检查通过
  if (!skipMainBackend) {
    startMainBackend()
  }
  try {
    await waitForBackend(mainBackendURL)
    createMainWindow()
  } catch (error) {
    dialog.showErrorBox('Wind DAQ 启动失败', error.message)
    app.quit()
  }
})

app.on('window-all-closed', () => app.quit())

app.on('before-quit', () => {
  app.isQuitting = true
  closeMotionWindow()
  if (mainBackendProcess) {
    mainBackendProcess.kill()
    mainBackendProcess = null
  }
})
