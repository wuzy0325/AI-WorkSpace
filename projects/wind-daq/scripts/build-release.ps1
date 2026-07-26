param(
    [switch]$WithInstaller
)

$ErrorActionPreference = 'Stop'

# --- Resolve paths ---
$ScriptDir = $PSScriptRoot
$ProjectDir = Split-Path -Parent $ScriptDir                              # projects/wind-daq/
$WorkspaceRoot = Split-Path -Parent (Split-Path -Parent $ProjectDir)     # AI-Workspace/
$WailsDir = Join-Path $ProjectDir 'apps\desktop-wails'
$InstallerDir = Join-Path $ProjectDir 'installer'
$DLLName = 'WTNMC4A_64.dll'
$DLLDir = Join-Path $WorkspaceRoot 'shared\device-sdk\WTNMC4A\Driver\INF\Win32&Win64\amd64'
$DLLSource = Join-Path $DLLDir $DLLName
$OutputExe = Join-Path $WailsDir 'build\bin\wind-daq.exe'

# --- Validate DLL source ---
if (-not (Test-Path -LiteralPath $DLLSource)) {
    Write-Error "WTNMC4A DLL not found at: $DLLSource"
    exit 1
}

# --- Step 1: Build Wind-DAQ (Wails v3) ---
Write-Host '>>> Building Wind-DAQ (wails3 task release)...' -ForegroundColor Cyan
Push-Location $WailsDir
try {
    go run github.com/wailsapp/wails/v3/cmd/wails3 task release
    if ($LASTEXITCODE -ne 0) {
        throw "wails3 task release failed (exit $LASTEXITCODE)"
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

# --- Step 3: NSIS installer (optional) ---
if ($WithInstaller) {
    $TargetInstallerDir = Join-Path $WailsDir 'build\windows\installer'
    $null = New-Item -ItemType Directory -Path $TargetInstallerDir -Force

    Write-Host ">>> Copying $DLLName to $TargetInstallerDir ..." -ForegroundColor Cyan
    Copy-Item -LiteralPath $DLLSource -Destination (Join-Path $TargetInstallerDir $DLLName) -Force

    # Copy custom NSIS files into the installer build dir
    Write-Host ">>> Copying custom NSIS files (with language selection) ..." -ForegroundColor Cyan
    Copy-Item -LiteralPath (Join-Path $InstallerDir 'project.nsi') -Destination (Join-Path $TargetInstallerDir 'project.nsi') -Force
    Copy-Item -LiteralPath (Join-Path $InstallerDir 'wails_tools.nsh') -Destination (Join-Path $TargetInstallerDir 'wails_tools.nsh') -Force

    # Generate NSIS installer
    # Note: NSIS reads script files as ACP (GBK on Chinese systems).
    # Convert UTF-8 to GBK before compilation to avoid "Bad text encoding" error.
    Write-Host ">>> Converting project.nsi to ANSI (GBK) for NSIS ..." -ForegroundColor Cyan
    $content = Get-Content -Raw (Join-Path $TargetInstallerDir 'project.nsi') -Encoding UTF8
    [System.IO.File]::WriteAllText(
        (Join-Path $TargetInstallerDir 'project.nsi'),
        $content,
        [System.Text.Encoding]::GetEncoding(936)
    )

    Write-Host ">>> Running makensis ..." -ForegroundColor Cyan
    Push-Location $TargetInstallerDir
    try {
        & 'makensis' "-DARG_WAILS_AMD64_BINARY=..\..\bin\wind-daq.exe" 'project.nsi'
        if ($LASTEXITCODE -ne 0) {
            Write-Warning "makensis returned exit code $LASTEXITCODE; installer may be incomplete."
        }
    } finally {
        Pop-Location
    }
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
