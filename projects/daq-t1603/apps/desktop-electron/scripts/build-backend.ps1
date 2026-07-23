$ErrorActionPreference = 'Stop'

$electronRoot = Split-Path -Parent $PSScriptRoot
$wailsRoot = Join-Path (Split-Path -Parent $electronRoot) 'desktop-wails'
$frontendRoot = Join-Path $wailsRoot 'frontend'
$backendOutput = Join-Path $electronRoot 'backend'
$goRoot = 'C:\go-versions\go1.20.14'

if (-not (Test-Path -LiteralPath (Join-Path $goRoot 'bin\go.exe'))) {
    throw "Go 1.20.14 not found at $goRoot"
}

Push-Location $frontendRoot
try {
    npm run build
} finally {
    Pop-Location
}

if (-not (Test-Path -LiteralPath $backendOutput)) {
    New-Item -ItemType Directory -Path $backendOutput | Out-Null
}

$env:GOROOT = $goRoot
$env:PATH = "$goRoot\bin;$env:PATH"
$env:GOWORK = 'off'
$env:CGO_ENABLED = '0'
$env:GOOS = 'windows'
$env:GOARCH = 'amd64'

Push-Location $wailsRoot
try {
    & (Join-Path $goRoot 'bin\go.exe') build -buildvcs=false -trimpath -ldflags '-s -w -H=windowsgui' -o (Join-Path $backendOutput 'daq-t1603-backend.exe') .
    if ($LASTEXITCODE -ne 0) {
        throw "Go backend build failed with exit code $LASTEXITCODE"
    }
} finally {
    Pop-Location
}
