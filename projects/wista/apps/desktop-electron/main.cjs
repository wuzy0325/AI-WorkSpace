const { app, BrowserWindow, dialog, ipcMain } = require('electron')
const { spawn } = require('node:child_process')
const http = require('node:http')
const path = require('node:path')

const backendURL = 'http://127.0.0.1:18181'
let backendProcess = null
let mainWindow = null

function backendPath() {
  if (app.isPackaged) {
    return path.join(process.resourcesPath, 'backend', 'wista-backend.exe')
  }
  return path.join(__dirname, 'backend', 'wista-backend.exe')
}

function startBackend() {
  backendProcess = spawn(backendPath(), [], {
    cwd: path.dirname(backendPath()),
    windowsHide: true,
    stdio: 'ignore',
  })
  backendProcess.once('error', (error) => {
    dialog.showErrorBox('WISTA 后端启动失败', error.message)
    app.quit()
  })
  backendProcess.once('exit', (code) => {
    backendProcess = null
    if (!app.isQuitting && code !== 0) {
      dialog.showErrorBox('WISTA 后端异常退出', `退出代码: ${code}`)
      app.quit()
    }
  })
}

function waitForBackend(timeoutMs = 15000) {
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
        reject(new Error('15 秒内未能连接后端服务'))
        return
      }
      setTimeout(probe, 250)
    }
    probe()
  })
}

function createWindow() {
  mainWindow = new BrowserWindow({
    title: 'WISTA 温度采集',
    width: 1600,
    height: 900,
    minWidth: 1280,
    minHeight: 720,
    show: false,
    webPreferences: {
      preload: path.join(__dirname, 'preload.cjs'),
      contextIsolation: true,
      nodeIntegration: false,
      sandbox: true,
    },
  })
  mainWindow.removeMenu()
  mainWindow.once('ready-to-show', () => mainWindow.show())
  mainWindow.loadURL(backendURL)
}

ipcMain.handle('dialog:pick-directory', async () => {
  const result = await dialog.showOpenDialog(mainWindow, {
    properties: ['openDirectory', 'createDirectory'],
  })
  return result.canceled ? '' : result.filePaths[0]
})

app.whenReady().then(async () => {
  startBackend()
  try {
    await waitForBackend()
    createWindow()
  } catch (error) {
    dialog.showErrorBox('WISTA 启动失败', error.message)
    app.quit()
  }
})

app.on('window-all-closed', () => app.quit())

app.on('before-quit', () => {
  app.isQuitting = true
  if (backendProcess) {
    backendProcess.kill()
    backendProcess = null
  }
})
