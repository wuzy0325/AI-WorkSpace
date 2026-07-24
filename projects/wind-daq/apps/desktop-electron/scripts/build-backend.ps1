$ErrorActionPreference = 'Stop'

# 构建 wind-daq Win7 后端二进制：
#   1. 先 npm run build 前端到 desktop-wails/frontend/dist
#   2. 用 Go 1.20.14 编译 desktop-wails 为单一二进制（内嵌前端资源）
#   3. 输出到 desktop-electron/backend/wind-daq-backend.exe
# 与 daq-t1603 的 build-backend.ps1 保持一致的步骤与参数。

$electronRoot = Split-Path -Parent $PSScriptRoot
$wailsRoot = Join-Path (Split-Path -Parent $electronRoot) 'desktop-wails'
$frontendRoot = Join-Path $wailsRoot 'frontend'
$backendOutput = Join-Path $electronRoot 'backend'
$goRoot = 'C:\go-versions\go1.20.14'

if (-not (Test-Path -LiteralPath (Join-Path $goRoot 'bin\go.exe'))) {
    throw "Go 1.20.14 not found at $goRoot"
}

# 1. 构建前端
Push-Location $frontendRoot
try {
    npm run build
} finally {
    Pop-Location
}

if (-not (Test-Path -LiteralPath $backendOutput)) {
    New-Item -ItemType Directory -Path $backendOutput | Out-Null
}

# 2. 构建 Go 后端（Go 1.20.14 + 关闭 cgo + trimpath + windowsgui）
$env:GOROOT = $goRoot
$env:PATH = "$goRoot\bin;$env:PATH"
$env:GOWORK = 'off'
$env:CGO_ENABLED = '0'
$env:GOOS = 'windows'
$env:GOARCH = 'amd64'

Push-Location $wailsRoot
try {
    & (Join-Path $goRoot 'bin\go.exe') build -buildvcs=false -trimpath -ldflags '-s -w -H=windowsgui' -o (Join-Path $backendOutput 'wind-daq-backend.exe') .
    if ($LASTEXITCODE -ne 0) {
        throw "Go backend build failed with exit code $LASTEXITCODE"
    }
} finally {
    Pop-Location
}
