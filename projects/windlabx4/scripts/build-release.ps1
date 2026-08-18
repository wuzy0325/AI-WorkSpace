param(
    [switch]$WithInstaller
)

$ErrorActionPreference = 'Stop'

# --- Resolve paths ---
$ScriptDir = $PSScriptRoot
$ProjectDir = Split-Path -Parent $ScriptDir                              # projects/windlabx4/
$WorkspaceRoot = Split-Path -Parent (Split-Path -Parent $ProjectDir)     # AI-Workspace/
$WailsDir = Join-Path $ProjectDir 'apps\desktop-wails'
$InstallerDir = Join-Path $ProjectDir 'installer'
$DLLName = 'WTNMC4A_64.dll'
$DLLDir = Join-Path $WorkspaceRoot 'shared\device-sdk\WTNMC4A\Driver\INF\Win32&Win64\amd64'
$DLLSource = Join-Path $DLLDir $DLLName
$OutputExe = Join-Path $WailsDir 'build\bin\windlabx4.exe'

# --- Validate DLL source ---
if (-not (Test-Path -LiteralPath $DLLSource)) {
    Write-Error "WTNMC4A DLL not found at: $DLLSource"
    exit 1
}

# --- Step 1: Build WindLabX4 (Wails v3) ---
Write-Host '>>> Building WindLabX4 (wails3 task release)...' -ForegroundColor Cyan
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

    # 复制随附的 NSIS include 文件（wails_tools.nsh 未被 git 跟踪，可直接覆盖）。
    # 注意：不再用 installer/project.nsi 覆盖 build/windows/installer/project.nsi——
    # 该文件被 git 跟踪、且文档规定其为权威版本源（UTF-8 无 BOM）。
    # 一旦用 backup 副本覆盖再转 GBK，会把它持久污染为 GBK，产生 dirty 工作区。
    Write-Host ">>> Copying wails_tools.nsh ..." -ForegroundColor Cyan
    Copy-Item -LiteralPath (Join-Path $InstallerDir 'wails_tools.nsh') -Destination (Join-Path $TargetInstallerDir 'wails_tools.nsh') -Force

    # Generate NSIS installer
    # Note1: NSIS reads script files as ACP (GBK on Chinese systems)。
    #        UTF-8 源在编译前必须先转成 GBK，否则报 "Bad text encoding"。
    # Note2: 转换只作用于临时文件，用完即删，绝不写回被 git 跟踪的
    #        build/windows/installer/project.nsi，保证每次打包后该源文件保持
    #        UTF-8 无 BOM、git 基线不变（消除"打包即 dirty"副作用）。
    $trackedNsi = Join-Path $TargetInstallerDir 'project.nsi'
    $gbkTmpNsi = Join-Path $TargetInstallerDir 'project.nsi.gbk'
    try {
        Write-Host ">>> Converting project.nsi to ANSI (GBK) for NSIS (temp only) ..." -ForegroundColor Cyan
        # 以权威源（被跟踪的 build/.../project.nsi）为唯一内容来源
        $content = Get-Content -Raw -LiteralPath $trackedNsi -Encoding UTF8
        [System.IO.File]::WriteAllText(
            $gbkTmpNsi,
            $content,
            [System.Text.Encoding]::GetEncoding(936)
        )

        Write-Host ">>> Running makensis ..." -ForegroundColor Cyan
        Push-Location $TargetInstallerDir
        try {
            & 'makensis' "-DARG_WAILS_AMD64_BINARY=..\..\bin\windlabx4.exe" 'project.nsi.gbk'
            if ($LASTEXITCODE -ne 0) {
                Write-Warning "makensis returned exit code $LASTEXITCODE; installer may be incomplete."
            }
        } finally {
            Pop-Location
        }
    } finally {
        # 无论成功与否都清理临时 GBK 副本，避免残留
        if (Test-Path -LiteralPath $gbkTmpNsi) {
            Remove-Item -LiteralPath $gbkTmpNsi -Force
        }
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
