# check-nsi-encoding.ps1 — NSIS 安装脚本（project.nsi）编码防劣化检查
#
# 背景：makensis 对无 BOM 文件按系统 ANSI 代码页（中文 Windows 为 GBK）解析，
# 文件一旦含非 ASCII 字节且没有 BOM，能否构建取决于打包机器的代码页——
# UTF-8 无 BOM 直接报 "Bad text encoding"，GBK 无 BOM 换台机器就乱码。
# WindLabX4 的 project.nsi 历史上被反复改坏（UTF-8 BOM → UTF-16 → GBK → UTF-8 无 BOM），
# AGENTS.md 的 NSIS 规则只是流程约束，没有技术强制，本脚本提供强制卡点。
#
# 规则（对每个 git 跟踪的 project.nsi）：
#   1. UTF-8 BOM（EF BB BF）→ 校验全文必须是合法 UTF-8，否则失败
#   2. UTF-16 BOM（FF FE / FE FF）→ 通过（NSIS 3.x 支持），但警告建议归一为 UTF-8 BOM
#   3. 无 BOM 且含非 ASCII 字节 → 失败（编码不确定状态，makensis 行为依赖代码页）
#   4. 无 BOM 且纯 ASCII → 通过
#
# 修复方式：为文件补上 UTF-8 BOM 并确保内容为合法 UTF-8。
# 用法：powershell -ExecutionPolicy Bypass -File scripts/check-nsi-encoding.ps1 [-Quiet]
# 由 .githooks/pre-commit 在提交前调用；失败会阻止提交。

param(
    [switch]$Quiet
)

$ErrorActionPreference = "Stop"

# 定位仓库根目录（git ls-files 的路径相对仓库根，与调用方工作目录无关）
$repoRoot = git rev-parse --show-toplevel 2>$null
if ($LASTEXITCODE -ne 0 -or -not $repoRoot) {
    Write-Host "[check-nsi-encoding] 不在 git 仓库内，跳过检查" -ForegroundColor Yellow
    exit 0
}
Set-Location $repoRoot

# 枚举所有 git 跟踪的 project.nsi
$nsiFiles = git ls-files "*project.nsi" 2>$null
if ($LASTEXITCODE -ne 0) {
    Write-Host "[check-nsi-encoding] git ls-files 失败，跳过检查" -ForegroundColor Yellow
    exit 0
}
if (-not $nsiFiles) {
    if (-not $Quiet) { Write-Host "[check-nsi-encoding] 未找到 project.nsi，跳过" }
    exit 0
}

$failures = @()
$warnings = @()

foreach ($file in $nsiFiles) {
    if (-not (Test-Path $file)) { continue }
    $bytes = [System.IO.File]::ReadAllBytes($file)
    if ($bytes.Length -lt 3) { continue }

    $hasUtf8Bom = ($bytes[0] -eq 0xEF -and $bytes[1] -eq 0xBB -and $bytes[2] -eq 0xBF)
    $hasUtf16Bom = ($bytes[0] -eq 0xFF -and $bytes[1] -eq 0xFE) -or
                   ($bytes[0] -eq 0xFE -and $bytes[1] -eq 0xFF)

    if ($hasUtf8Bom) {
        # 必须是合法 UTF-8
        try {
            $utf8 = New-Object System.Text.UTF8Encoding($false, $true)
            [void]$utf8.GetString($bytes, 3, $bytes.Length - 3)
        } catch {
            $failures += "$file ：有 UTF-8 BOM 但内容不是合法 UTF-8（可能混入 GBK 字节）"
        }
    } elseif ($hasUtf16Bom) {
        $warnings += "$file ：UTF-16 编码（NSIS 可构建，但建议归一为 UTF-8 BOM）"
    } else {
        # 无 BOM：含非 ASCII 字节即编码不确定状态
        $hasHighBytes = $false
        foreach ($b in $bytes) {
            if ($b -ge 0x80) { $hasHighBytes = $true; break }
        }
        if ($hasHighBytes) {
            $failures += "$file ：无 BOM 且含非 ASCII 字节——makensis 将按系统代码页解析，构建结果依赖打包机器。请转为 UTF-8 并补 BOM（EF BB BF）"
        }
    }
}

foreach ($w in $warnings) {
    Write-Host "[check-nsi-encoding] 警告: $w" -ForegroundColor Yellow
}

if ($failures.Count -gt 0) {
    Write-Host ""
    Write-Host "[check-nsi-encoding] NSIS 脚本编码检查失败：" -ForegroundColor Red
    foreach ($f in $failures) {
        Write-Host "  - $f" -ForegroundColor Red
    }
    Write-Host ""
    Write-Host "规则依据：projects/windlabx4/AGENTS.md 的 NSIS Installer Hard Rules。" -ForegroundColor Red
    exit 1
}

if (-not $Quiet) {
    Write-Host "[check-nsi-encoding] OK: $($nsiFiles.Count) 个 project.nsi 编码合规"
}
exit 0
