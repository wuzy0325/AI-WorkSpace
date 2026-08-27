# check-build-chain.ps1 — Wails 桌面项目打包链路完整性检查
#
# 背景：打包链路由多个入口耦合（Taskfile.yml 任务、build/config.yml dev_mode、
# scripts/build-release.ps1），历史上 Taskfile 被重构（如"对齐 wails3 标准结构"）
# 删除/改名任务后，引用方未同步，导致 `task release` / `wails3 build` /
# build-release.ps1 静默失效，直到打包那一刻才暴露（windlabx4 ed810cd 回归）。
#
# 本脚本在提交前强制校验打包链路自洽：
#   1. Taskfile 关键任务存在：release / build-go / build-frontend / windows:build
#      （仅对存在 apps/desktop-wails/Taskfile.yml 的项目）
#   2. build/config.yml 的 dev_mode.executes 中 `wails3 task <name>` 引用的任务
#      必须存在于 Taskfile
#   3. scripts/build-release.ps1（如有）自包含性：若含 `task <name>` /
#      `wails3 task <name>` 引用，则引用的任务必须存在于 Taskfile；
#      若脚本不含 task 引用（自包含构建），视为合规
#   4. scripts/build-release.ps1 的 go build 必须带 `-tags production`
#      （防止交付构建回退到 dev 分支）
#
# 失败时阻止提交；修复违规后重试即可。
# 用法：powershell -ExecutionPolicy Bypass -File scripts/check-build-chain.ps1 [-Quiet]
# 由 .githooks/pre-commit 在提交前调用。

param(
    [switch]$Quiet
)

$ErrorActionPreference = "Stop"

# 定位仓库根目录
$repoRoot = git rev-parse --show-toplevel 2>$null
if ($LASTEXITCODE -ne 0 -or -not $repoRoot) {
    Write-Host "[check-build-chain] 不在 git 仓库内，跳过检查" -ForegroundColor Yellow
    exit 0
}
Set-Location $repoRoot

$failures = @()

# 枚举所有 Wails 项目（apps/desktop-wails 目录）
$wailsProjects = @(git ls-files "*apps/desktop-wails/Taskfile.yml" 2>$null)
if (-not $wailsProjects) {
    if (-not $Quiet) { Write-Host "[check-build-chain] 未找到 Wails 项目，跳过" }
    exit 0
}

# 解析 Taskfile 中的任务名（顶层任务：两个空格缩进 + 任务名 + 冒号）
function Get-TaskfileTasks([string]$taskfile) {
    $names = @()
    foreach ($line in [System.IO.File]::ReadAllLines($taskfile)) {
        if ($line -match '^\s{2}([a-zA-Z0-9][a-zA-Z0-9:_\-]*):\s*$') {
            $names += $Matches[1]
        }
    }
    return $names
}

# 从 config.yml 提取 dev_mode.executes 中的 wails3 task 引用
function Get-ConfigTaskRefs([string]$configFile) {
    $refs = @()
    foreach ($line in [System.IO.File]::ReadAllLines($configFile)) {
        if ($line -match 'wails3\s+task\s+([a-zA-Z0-9:_\-]+)') {
            $refs += $Matches[1]
        }
    }
    return $refs
}

# 从 build-release.ps1 提取 task / wails3 task 引用（排除注释行）
function Get-ScriptTaskRefs([string]$scriptFile) {
    $refs = @()
    foreach ($line in [System.IO.File]::ReadAllLines($scriptFile)) {
        $trimmed = $line.Trim()
        if ($trimmed.StartsWith('#') -or $trimmed.StartsWith('//')) { continue }
        if ($trimmed -match '(?:wails3\s+task|(?:^|\s)task\s+)([a-zA-Z0-9:_\-]+)') {
            $refs += $Matches[1]
        }
    }
    return $refs
}

foreach ($taskfile in $wailsProjects) {
    $projDir = Split-Path (Split-Path (Split-Path $taskfile -Parent) -Parent) -Parent   # projects/<proj>
    $projName = Split-Path $projDir -Leaf
    $wailsDir = Split-Path $taskfile -Parent
    $tag = "[check-build-chain][$projName]"

    # 1. Taskfile 关键任务存在
    $tasks = @(Get-TaskfileTasks $taskfile)
    $required = @('release', 'build-go', 'build-frontend', 'windows:build')
    foreach ($req in $required) {
        if ($tasks -notcontains $req) {
            $failures += "$tag Taskfile 缺少关键任务 '$req' —— $taskfile"
        }
    }

    # 2. config.yml dev_mode 引用的任务存在
    $configFile = Join-Path $wailsDir 'build\config.yml'
    if (Test-Path $configFile) {
        foreach ($ref in @(Get-ConfigTaskRefs $configFile)) {
            if ($tasks -notcontains $ref) {
                $failures += "$tag build/config.yml 引用的任务 '$ref' 在 Taskfile 中不存在 —— $configFile"
            }
        }
    }

    # 3/4. scripts/build-release.ps1 自包含性 + production 标签
    $scriptFile = Join-Path $projDir 'scripts\build-release.ps1'
    if (Test-Path $scriptFile) {
        $refs = @(Get-ScriptTaskRefs $scriptFile)
        foreach ($ref in $refs) {
            if ($tasks -notcontains $ref) {
                $failures += "$tag build-release.ps1 引用任务 '$ref' 在 Taskfile 中不存在 —— $scriptFile"
            }
        }
        # go build 必须带 production 标签
        $hasProduction = Select-String -Path $scriptFile -Pattern 'go\s+build.*-tags\s+production' -Quiet
        if (-not $hasProduction) {
            $failures += "$tag build-release.ps1 的 go build 未带 -tags production —— $scriptFile"
        }
    }
}

if ($failures.Count -gt 0) {
    Write-Host ""
    Write-Host "[check-build-chain] 打包链路完整性检查失败：" -ForegroundColor Red
    foreach ($f in $failures) {
        Write-Host "  - $f" -ForegroundColor Red
    }
    Write-Host ""
    Write-Host "修复方式：" -ForegroundColor Red
    Write-Host "  1. Taskfile 任务被删除/改名时，同步更新 build/config.yml 的 dev_mode 引用" -ForegroundColor Red
    Write-Host "  2. 若调整 release 流程，请保持 release / build-go / build-frontend / windows:build 任务存在" -ForegroundColor Red
    Write-Host "  3. build-release.ps1 若引用 task，确保任务存在；若自包含，go build 必须带 -tags production" -ForegroundColor Red
    exit 1
}

if (-not $Quiet) {
    Write-Host "[check-build-chain] OK: $($wailsProjects.Count) 个 Wails 项目打包链路自洽"
}
exit 0