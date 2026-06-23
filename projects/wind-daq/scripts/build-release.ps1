param(
    [switch]$WithInstaller
)

$ErrorActionPreference = 'Stop'

# --- Resolve paths ---
$ScriptDir = $PSScriptRoot
$ProjectDir = Split-Path -Parent $ScriptDir                              # projects/wind-daq/
$WorkspaceRoot = Split-Path -Parent (Split-Path -Parent $ProjectDir)     # AI-Workspace/
$WailsDir = Join-Path $ProjectDir 'apps\desktop-wails'
$DLLName = 'WTNMC4A_64.dll'
$DLLDir = Join-Path $WorkspaceRoot 'shared\device-sdk\WTNMC4A\Driver\INF\Win32&Win64\amd64'
$DLLSource = Join-Path $DLLDir $DLLName
$OutputExe = Join-Path $WailsDir 'build\bin\wind-daq.exe'

# --- Validate DLL source ---
if (-not (Test-Path -LiteralPath $DLLSource)) {
    Write-Error "WTNMC4A DLL not found at: $DLLSource"
    exit 1
}

# --- Step 1: Build Wails app ---
Write-Host '>>> Building Wind-DAQ...' -ForegroundColor Cyan
Push-Location $WailsDir
try {
    if ($WithInstaller) {
        # Pre-populate the installer directory so our project.nsi takes effect
        $null = New-Item -ItemType Directory -Path (Join-Path $WailsDir 'build\windows\installer') -Force
        & wails build -windowsnsis
    } else {
        & wails build
    }
    if ($LASTEXITCODE -ne 0) {
        throw "wails build failed (exit $LASTEXITCODE)"
    }
    if (-not (Test-Path -LiteralPath $OutputExe)) {
        throw "Expected output exe not found: $OutputExe"
    }
} finally {
    Pop-Location
}

# --- Step 2: Copy DLL alongside the exe (portable build) ---
Write-Host ">>> Copying $DLLName to build/bin/ ..." -ForegroundColor Cyan
Copy-Item -LiteralPath $DLLSource -Destination (Join-Path (Join-Path $WailsDir 'build\bin') $DLLName) -Force

# --- Step 3: NSIS installer — also copy into installer resources ---
if ($WithInstaller) {
    $InstallerDir = Join-Path $WailsDir 'build\windows\installer'
    Write-Host ">>> Copying $DLLName to $InstallerDir ..." -ForegroundColor Cyan
    Copy-Item -LiteralPath $DLLSource -Destination (Join-Path $InstallerDir $DLLName) -Force
}

# --- Summary ---
Write-Host "<<< Build complete." -ForegroundColor Green
Write-Host "    Exe: $OutputExe"
Write-Host "    DLL: $(Join-Path (Split-Path -Parent $OutputExe) $DLLName)"
if ($WithInstaller) {
    $Installer = Get-ChildItem (Join-Path $WailsDir 'build\bin\*installer*') -ErrorAction SilentlyContinue | Select-Object -First 1
    if ($Installer) {
        Write-Host "    Installer: $($Installer.FullName)"
    }
}
