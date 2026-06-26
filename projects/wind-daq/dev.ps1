param(
    [ValidateSet("prod", "dev", "browser")]
    [string]$Mode = "prod"
)

$root = Split-Path -Parent $MyInvocation.MyCommand.Path

switch ($Mode) {
    "prod" {
        Write-Host "=== 生产模式：构建并启动桌面应用 ===" -ForegroundColor Cyan
        Push-Location "$root/apps/desktop-wails/frontend"
        npm run build | Out-Null
        Pop-Location
        Push-Location "$root/apps/desktop-wails"
        go build -ldflags="-H windowsgui" -o build/bin/wind-daq.exe .
        Pop-Location
        Write-Host "启动桌面应用..." -ForegroundColor Green
        Push-Location "$root/apps/desktop-wails"
        build/bin/wind-daq.exe
        Pop-Location
    }
    "dev" {
        Write-Host "=== 开发模式：启动 Vite 热更新 + 桌面应用 ===" -ForegroundColor Cyan
        Write-Host "启动 Vite 开发服务器 (端口 9245)..." -ForegroundColor Yellow
        Push-Location "$root/apps/desktop-wails/frontend"
        $viteJob = Start-Job -ScriptBlock {
            Set-Location "$using:root/apps/desktop-wails/frontend"
            npm run dev -- --host 127.0.0.1
        }
        Pop-Location
        Start-Sleep 4
        Write-Host "构建并启动桌面应用（热更新模式）..." -ForegroundColor Green
        Push-Location "$root/apps/desktop-wails"
        $env:FRONTEND_DEVSERVER_URL = 'http://127.0.0.1:9245'
        go run .
        Pop-Location
        Stop-Job $viteJob -ErrorAction SilentlyContinue
        Remove-Job $viteJob -ErrorAction SilentlyContinue
    }
    "browser" {
        Write-Host "=== 浏览器模式：启动 Vite + 浏览器 ===" -ForegroundColor Cyan
        Write-Host "启动 Vite 开发服务器 (端口 9245)..." -ForegroundColor Yellow
        Push-Location "$root/apps/desktop-wails/frontend"
        Start-Process powershell -ArgumentList "-NoExit", "npm run dev -- --host 127.0.0.1"
        Pop-Location
        Start-Sleep 3
        Write-Host "打开浏览器 http://127.0.0.1:9245" -ForegroundColor Green
        Start-Process "http://127.0.0.1:9245"
    }
}
