$ErrorActionPreference = 'Stop'

# wind-daq 开发态热加载启动脚本
#
# 并行启动三个进程：
#   1. Go 后端（go run，监听 127.0.0.1:8900）—— 后端代码改动需 Ctrl+C 重启本脚本
#   2. Vite dev server（HMR，监听 127.0.0.1:9246）—— 前端 Vue/TS 改动自动热刷新
#   3. Electron（加载 Vite dev server，自动打开 DevTools）
#
# 任一进程退出时全部清理。Electron 关闭后整个脚本退出。
#
# 使用：npm run dev
# 前端热加载自动生效；后端改动需 Ctrl+C 后重新 npm run dev。

$electronRoot = Split-Path -Parent $PSScriptRoot
$wailsRoot = Join-Path (Split-Path -Parent $electronRoot) 'desktop-wails'
$frontendRoot = Join-Path $wailsRoot 'frontend'
$goRoot = 'C:\go-versions\go1.20.14'

if (-not (Test-Path -LiteralPath (Join-Path $goRoot 'bin\go.exe'))) {
    throw "Go 1.20.14 not found at $goRoot"
}

# 准备 Go 环境（与 build-backend.ps1 一致）
$env:GOROOT = $goRoot
$env:PATH = "$goRoot\bin;$env:PATH"
$env:GOWORK = 'off'
$env:CGO_ENABLED = '0'

# 告知 Electron 加载 Vite dev server 并跳过 spawn 后端
$env:ELECTRON_DEV_URL = 'http://127.0.0.1:9246'
$env:ELECTRON_SKIP_BACKEND = '1'

$processes = @()

function Stop-AllChildren {
    # 反向遍历：先杀后启动的（Electron），再杀 Vite 和后端
    for ($i = $processes.Count - 1; $i -ge 0; $i--) {
        $p = $processes[$i]
        if ($p -and -not $p.HasExited) {
            try {
                # Kill 整个进程树（包括 go run 启动的子进程）
                taskkill /PID $p.Id /T /F 2>$null | Out-Null
                if (-not $p.HasExited) { $p.Kill() }
            } catch {}
        }
    }
}

# Ctrl+C 或正常退出时清理所有子进程
$null = Register-EngineEvent PowerShell.Exiting -Action { Stop-AllChildren }

try {
    # 1. 启动 Go 后端（go run，监听 8900）
    Write-Host '[dev] 启动 Go 后端 (8900)...' -ForegroundColor Cyan
    $backendProc = Start-Process -FilePath (Join-Path $goRoot 'bin\go.exe') `
        -ArgumentList @('run', '.') `
        -WorkingDirectory $wailsRoot `
        -PassThru -NoNewWindow `
        -RedirectStandardOutput "$env:TEMP\wind-daq-backend.out.log" `
        -RedirectStandardError "$env:TEMP\wind-daq-backend.err.log"
    $processes += $backendProc

    # 2. 启动 Vite dev server（HMR，监听 9246）
    Write-Host '[dev] 启动 Vite dev server (9246)...' -ForegroundColor Cyan
    $viteProc = Start-Process -FilePath 'npm.cmd' `
        -ArgumentList @('run', 'dev', '--', '--host', '127.0.0.1') `
        -WorkingDirectory $frontendRoot `
        -PassThru -NoNewWindow
    $processes += $viteProc

    # 3. 等待 Vite dev server 就绪（轮询 9246）
    Write-Host '[dev] 等待 Vite dev server 就绪...' -ForegroundColor Yellow
    $viteReady = $false
    for ($i = 0; $i -lt 40; $i++) {
        Start-Sleep -Milliseconds 500
        try {
            $resp = Invoke-WebRequest -Uri 'http://127.0.0.1:9246' -UseBasicParsing -TimeoutSec 2
            if ($resp.StatusCode -eq 200) {
                $viteReady = $true
                break
            }
        } catch {}
    }
    if (-not $viteReady) {
        throw 'Vite dev server 20 秒内未就绪'
    }
    Write-Host '[dev] Vite dev server 已就绪' -ForegroundColor Green

    # 4. 等待 Go 后端健康检查通过
    Write-Host '[dev] 等待 Go 后端健康检查...' -ForegroundColor Yellow
    $backendReady = $false
    for ($i = 0; $i -lt 40; $i++) {
        Start-Sleep -Milliseconds 500
        try {
            $resp = Invoke-WebRequest -Uri 'http://127.0.0.1:8900/api/health' -UseBasicParsing -TimeoutSec 2
            if ($resp.StatusCode -eq 200) {
                $backendReady = $true
                break
            }
        } catch {}
    }
    if (-not $backendReady) {
        throw 'Go 后端 20 秒内未就绪，请查看日志: $env:TEMP\wind-daq-backend.err.log'
    }
    Write-Host '[dev] Go 后端已就绪' -ForegroundColor Green

    # 5. 启动 Electron（加载 Vite dev server）
    Write-Host '[dev] 启动 Electron...' -ForegroundColor Cyan
    $electronExe = Join-Path $electronRoot 'node_modules\electron\dist\electron.exe'
    if (-not (Test-Path -LiteralPath $electronExe)) {
        throw "Electron not found at $electronExe，请先 npm install"
    }
    $electronProc = Start-Process -FilePath $electronExe `
        -ArgumentList @('.') `
        -WorkingDirectory $electronRoot `
        -PassThru
    $processes += $electronProc

    Write-Host ''
    Write-Host '[dev] 热加载环境已启动：' -ForegroundColor Green
    Write-Host '  - 前端 HMR: http://127.0.0.1:9246 (Vue/TS 改动自动刷新)' -ForegroundColor Gray
    Write-Host '  - 后端 API: http://127.0.0.1:8900 (Go 改动需 Ctrl+C 重启)' -ForegroundColor Gray
    Write-Host '  - 后端日志: $env:TEMP\wind-daq-backend.err.log' -ForegroundColor Gray
    Write-Host ''
    Write-Host '[dev] 关闭 Electron 窗口或 Ctrl+C 退出全部进程' -ForegroundColor Yellow
    Write-Host ''

    # 等待 Electron 退出（主进程），退出后清理后端和 Vite
    $electronProc.WaitForExit()
    Write-Host '[dev] Electron 已退出，清理子进程...' -ForegroundColor Cyan
}
finally {
    Stop-AllChildren
}
