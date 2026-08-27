# 启动脚本 - 同时启动后端和前端
# 使用方式: .\start-dev.ps1

# 设置控制台编码为 UTF-8，解决中文乱码问题
[Console]::OutputEncoding = [System.Text.Encoding]::UTF8
$OutputEncoding = [System.Text.Encoding]::UTF8
chcp 65001 > $null 2>&1

Write-Host "正在启动 1604 统一校准系统..." -ForegroundColor Green
Write-Host ""

# 获取脚本所在目录
$scriptPath = $PSScriptRoot
if (-not $scriptPath) {
    $scriptPath = Split-Path -Parent $MyInvocation.MyCommand.Path
}
if (-not $scriptPath) {
    $scriptPath = Get-Location
}
Set-Location $scriptPath

# 启动后端
Write-Host "[1/2] 启动 Go API 服务 (端口 18080)..." -ForegroundColor Cyan
$backendJob = Start-Job -ScriptBlock {
    param($path)
    Set-Location $path
    go run ./cmd/server
} -ArgumentList $scriptPath

Start-Sleep -Seconds 2

# 检查后端是否成功启动
$backendLog = Receive-Job -Job $backendJob -Keep
if ($backendLog -match "listening") {
    Write-Host "✓ 后端启动成功" -ForegroundColor Green
} else {
    Write-Host "✗ 后端启动可能有问题，查看日志:" -ForegroundColor Yellow
    Receive-Job -Job $backendJob | Select-Object -Last 10
}

# 启动前端
Write-Host ""
Write-Host "[2/2] 启动 Vue 前端服务 (端口 5173)..." -ForegroundColor Cyan
Set-Location "$scriptPath\web"
$frontendProcess = Start-Process -FilePath "npm" -ArgumentList "run", "dev" -PassThru -WindowStyle Normal

Start-Sleep -Seconds 3

# 显示信息
Write-Host ""
Write-Host "==========================================" -ForegroundColor Green
Write-Host "系统已启动！" -ForegroundColor Green
Write-Host ""
Write-Host "后端 API: http://localhost:18080" -ForegroundColor Yellow
Write-Host "前端页面: http://localhost:5173" -ForegroundColor Yellow
Write-Host ""
Write-Host "关闭此窗口即可停止所有服务" -ForegroundColor Cyan
Write-Host "==========================================" -ForegroundColor Green
Write-Host ""

# 保持脚本运行
Write-Host "按 Enter 键停止服务..."
Read-Host

# 清理
Write-Host "正在停止服务..."
Stop-Job -Job $backendJob
Remove-Job -Job $backendJob
Stop-Process -Id $frontendProcess.Id -Force -ErrorAction SilentlyContinue
Write-Host "服务已停止" -ForegroundColor Green
