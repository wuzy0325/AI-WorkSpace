<#
.SYNOPSIS
  安装 Go 1.20.14（最后兼容 Win7 的 Go 版本）到独立目录，不污染系统 Go 安装。

.DESCRIPTION
  下载 go1.20.14.windows-amd64.zip 到 C:\go-versions\go1.20.14
  使用方式：在需要 Go 1.20.14 的终端执行
    $env:GOROOT = "C:\go-versions\go1.20.14"
    $env:PATH = "$env:GOROOT\bin;$env:PATH"
    go version
#>

[CmdletBinding()]
param(
    [string]$InstallDir = "C:\go-versions\go1.20.14",
    [string]$Version = "1.20.14"
)

$ErrorActionPreference = "Stop"

$sources = @(
    "https://golang.google.cn/dl/go$Version.windows-amd64.zip",
    "https://go.dev/dl/go$Version.windows-amd64.zip",
    "https://mirrors.aliyun.com/golang/go$Version.windows-amd64.zip"
)

$zip = "$env:TEMP\go$Version.windows-amd64.zip"

if (Test-Path "$InstallDir\bin\go.exe") {
    Write-Host ('[skip] Go ' + $Version + ' already installed at ' + $InstallDir) -ForegroundColor Yellow
    & "$InstallDir\bin\go.exe" version
    return
}

$downloaded = $false
foreach ($url in $sources) {
    Write-Host ('[try ] download ' + $url) -ForegroundColor Cyan
    try {
        Invoke-WebRequest -Uri $url -OutFile $zip -UseBasicParsing -TimeoutSec 60
        $downloaded = $true
        Write-Host '[ok   ] downloaded' -ForegroundColor Green
        break
    } catch {
        Write-Host ('[fail] ' + $_.Exception.Message) -ForegroundColor Yellow
    }
}
if (-not $downloaded) {
    throw 'all mirrors failed'
}

$actualSha = (Get-FileHash $zip -Algorithm SHA256).Hash
Write-Host ('[info] SHA256 = ' + $actualSha)

Write-Host ('[run ] extract to ' + $InstallDir) -ForegroundColor Cyan
$tempExtract = "$env:TEMP\go$Version-extract"
if (Test-Path $tempExtract) { Remove-Item -Recurse -Force $tempExtract }
Expand-Archive -Path $zip -DestinationPath $tempExtract -Force

if (-not (Test-Path $InstallDir)) {
    New-Item -ItemType Directory -Path $InstallDir -Force | Out-Null
}
Copy-Item -Path "$tempExtract\go\*" -Destination $InstallDir -Recurse -Force
Remove-Item -Recurse -Force $tempExtract
Remove-Item $zip

Write-Host '[run ] verify install' -ForegroundColor Cyan
$env:GOROOT = $InstallDir
$env:PATH = "$InstallDir\bin;$env:PATH"
& "$InstallDir\bin\go.exe" version

Write-Host ''
Write-Host ('Go ' + $Version + ' installed at: ' + $InstallDir) -ForegroundColor Green
Write-Host ''
Write-Host 'usage in new terminal:'
Write-Host '  $env:GOROOT = "C:\go-versions\go1.20.14"'
Write-Host '  $env:PATH = "$env:GOROOT\bin;$env:PATH"'
Write-Host '  go version'