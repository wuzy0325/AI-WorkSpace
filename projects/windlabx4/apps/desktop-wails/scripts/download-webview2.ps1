# Download WebView2 Runtime offline installer (MicrosoftEdgeWebView2RuntimeInstallerX64.exe).
#
# Motivation: In network-restricted environments (e.g. Russian systems under sanctions),
# NSIS installer's online WebView2 bootstrapper download often fails, leaving WindLabX4
# unable to start after installation (log shows "no webview2 found").
# This script downloads the offline installer to the installer.exe output directory.
# When distributing, copy installer.exe and the offline installer together to target machines;
# NSIS will auto-detect the offline installer in $EXEDIR and use it, no internet required.
#
# Offline installer vs online bootstrapper:
#   - Online bootstrapper (~2MB): downloads actual Runtime (~150MB) at install time, fails on restricted networks
#   - Offline installer (~150MB): contains full Runtime, installs without internet
#
# Usage: powershell -ExecutionPolicy Bypass -File scripts/download-webview2.ps1
# Optional param -Dest to specify target dir (default build\bin, same as installer.exe output dir).
#
# Distribution flow:
#   1. Run `wails3 task release` to generate installer.exe
#   2. Run this script to download WebView2 offline installer to build\bin\
#   3. Copy build\bin\WindLabX4-*.exe and build\bin\MicrosoftEdgeWebView2RuntimeInstallerX64.exe
#      together to target machine
#   4. Run installer.exe on target machine, NSIS auto-detects offline installer and silently installs WebView2

[CmdletBinding()]
param(
    [string]$Dest = 'build\bin'
)

$ErrorActionPreference = 'Stop'

# WebView2 Runtime offline installer official download URL (X64)
# Source: https://developer.microsoft.com/en-us/microsoft-edge/webview/
# NOTE: LinkID=2124701 = X64 offline installer (MicrosoftEdgeWebView2RuntimeInstallerX64.exe)
#       LinkID=2124703 = online bootstrapper (MicrosoftEdgeWebview2Setup.exe, ~2MB, used by NSIS online fallback)
#       LinkID=2124702 is BROKEN (redirects to Linux RPM package as of 2026-07)
$downloadUrl = 'https://go.microsoft.com/fwlink/p/?LinkId=2124701'
$fileName = 'MicrosoftEdgeWebView2RuntimeInstallerX64.exe'

# Ensure dest dir exists
if (-not (Test-Path $Dest)) {
    New-Item -ItemType Directory -Path $Dest -Force | Out-Null
    Write-Host "[download-webview2] created dest dir: $Dest"
}

$destPath = Join-Path $Dest $fileName

# Skip download if already exists (unless user deleted it)
if (Test-Path $destPath) {
    $existingSize = (Get-Item $destPath).Length
    if ($existingSize -gt 100MB) {
        $existingSizeMB = [math]::Round($existingSize/1MB, 1)
        Write-Host "[download-webview2] already exists (${existingSizeMB} MB), skip: $destPath"
        Write-Host "[download-webview2] delete it if you want to re-download."
        exit 0
    }
    Write-Host "[download-webview2] existing file too small ($existingSize bytes), re-downloading..."
}

Write-Host "[download-webview2] downloading WebView2 Runtime offline installer..."
Write-Host "[download-webview2] URL: $downloadUrl"
Write-Host "[download-webview2] Dest: $destPath"

# Use .NET WebClient for download (more stable than Invoke-WebRequest for large files)
$webClient = New-Object System.Net.WebClient
$webClient.Headers.Add('User-Agent', 'WindLabX4-Installer-Script/1.0')

try {
    $webClient.DownloadFile($downloadUrl, $destPath)
} catch {
    Write-Error "[download-webview2] download failed: $($_.Exception.Message)"
    Write-Host ""
    Write-Host "[download-webview2] Please download offline installer manually:" -ForegroundColor Yellow
    Write-Host "  URL: https://developer.microsoft.com/en-us/microsoft-edge/webview/" -ForegroundColor Yellow
    Write-Host "  Download the 'Standalone' version and rename to $fileName" -ForegroundColor Yellow
    Write-Host "  Place at: $Dest" -ForegroundColor Yellow
    exit 1
} finally {
    $webClient.Dispose()
}

# Verify download result
if (Test-Path $destPath) {
    $fileSize = (Get-Item $destPath).Length
    $sizeMB = [math]::Round($fileSize/1MB, 1)
    if ($fileSize -lt 100MB) {
        Write-Warning "[download-webview2] downloaded file too small (${sizeMB} MB), may be corrupted."
        exit 1
    }
    Write-Host "[download-webview2] download OK: $destPath (${sizeMB} MB)" -ForegroundColor Green
    Write-Host ""
    Write-Host "[download-webview2] Distribution: copy this file with installer.exe to target machine," -ForegroundColor Cyan
    Write-Host "[download-webview2] NSIS installer will auto-detect the offline installer and silently install WebView2." -ForegroundColor Cyan
    exit 0
} else {
    Write-Error "[download-webview2] download failed: file not found after download."
    exit 1
}
