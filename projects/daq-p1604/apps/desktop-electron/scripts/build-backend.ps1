$ErrorActionPreference = 'Stop'

# 构建脚本：编译 Go 后端 + 前端 dist + 复制 exe 到 desktop-electron/backend/
#
# 步骤：
#   1. cd 到 frontend 目录执行 npm run build（生成 frontend/dist）
#   2. cd 到 desktop-wails 目录执行 go build（嵌入 frontend/dist 生成 exe）
#   3. exe 输出到 desktop-electron/backend/daq-p1604-backend.exe
#
# 注意事项：
#   - 必须用 Go 1.20.14（最后支持 Win7 的 Go 版本），路径硬编码到 C:\go-versions\go1.20.14
#   - GOWORK=off：避免工作空间中其他 Wails v3 模块的间接依赖冲突
#   - CGO_ENABLED=0：纯 Go 静态链接，避免依赖 mingw
#   - -ldflags '-s -w -H=windowsgui'：剥调试信息 + 标记为 GUI 程序（无控制台窗口）

$electronRoot = Split-Path -Parent $PSScriptRoot
$wailsRoot = Join-Path (Split-Path -Parent $electronRoot) 'desktop-wails'
$frontendRoot = Join-Path $wailsRoot 'frontend'
$backendOutput = Join-Path $electronRoot 'backend'
$goRoot = 'C:\go-versions\go1.20.14'

if (-not (Test-Path -LiteralPath (Join-Path $goRoot 'bin\go.exe'))) {
    throw "Go 1.20.14 not found at $goRoot"
}

# ---- 1. 构建前端 dist ----
Push-Location $frontendRoot
try {
    npm run build
} finally {
    Pop-Location
}

# ---- 2. 准备 backend 输出目录 ----
if (-not (Test-Path -LiteralPath $backendOutput)) {
    New-Item -ItemType Directory -Path $backendOutput | Out-Null
}

# ---- 3. 设置 Go 1.20.14 环境 ----
$env:GOROOT = $goRoot
$env:PATH = "$goRoot\bin;$env:PATH"
$env:GOWORK = 'off'
$env:CGO_ENABLED = '0'
$env:GOOS = 'windows'
$env:GOARCH = 'amd64'

# ---- 4. 编译 Go 后端 ----
Push-Location $wailsRoot
try {
    & (Join-Path $goRoot 'bin\go.exe') build -buildvcs=false -trimpath -ldflags '-s -w -H=windowsgui' -o (Join-Path $backendOutput 'daq-p1604-backend.exe') .
    if ($LASTEXITCODE -ne 0) {
        throw "Go backend build failed with exit code $LASTEXITCODE"
    }
} finally {
    Pop-Location
}
