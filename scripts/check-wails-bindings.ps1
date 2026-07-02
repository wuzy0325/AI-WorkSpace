[CmdletBinding()]
param(
    # 指定要检查的项目名（如 wind-daq / daq-p1604 / daq-t1603），不传则检查所有 Wails 项目
    [string[]]$Projects = @(),
    # stale 阈值（分钟）：backend 比 binding 新超过此值则警告，默认 60 分钟
    [int]$StaleMinutes = 60,
    [switch]$Quiet
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

# 工作区根目录
$workspaceRoot = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path
$projectsRoot = Join-Path $workspaceRoot "projects"

# 错误与警告收集
$errors = New-Object System.Collections.Generic.List[string]
$warnings = New-Object System.Collections.Generic.List[string]

# Wails v3 Service 接口的 lifecycle 方法——不暴露给前端，binding 不会生成
# 详见 application.Service interface
$wailsLifecycleMethods = @(
    "ServiceName",
    "ServiceStartup",
    "ServiceShutdown",
    "OnStartup",
    "OnShutdown",
    "OnDomReady"
)

# ---- 1. 发现所有 Wails 项目 ----
$allWailsProjects = @()
if (Test-Path $projectsRoot) {
    Get-ChildItem -Path $projectsRoot -Directory | ForEach-Object {
        $wailsDir = Join-Path $_.FullName "apps\desktop-wails"
        $backendDir = Join-Path $wailsDir "backend"
        $bindingDir = Join-Path $wailsDir "frontend\bindings"
        # 只有同时存在 backend 和 bindings 的才算 Wails 项目
        if ((Test-Path $backendDir) -and (Test-Path $bindingDir)) {
            $allWailsProjects += $_.Name
        }
    }
}

if ($allWailsProjects.Count -eq 0) {
    Write-Host "WARN: 未发现任何 Wails 项目（projects/*/apps/desktop-wails/{backend,frontend/bindings}）" -ForegroundColor Yellow
    exit 0
}

# 过滤目标项目
if ($Projects.Count -gt 0) {
    $targetProjects = $Projects | Where-Object { $_ -in $allWailsProjects }
    $missing = $Projects | Where-Object { $_ -notin $allWailsProjects }
    foreach ($m in $missing) {
        $errors.Add("指定的项目不存在或非 Wails 项目: $m")
    }
} else {
    $targetProjects = $allWailsProjects
}

if (-not $Quiet) {
    Write-Host "=== Wails Binding 一致性检查 ===" -ForegroundColor Cyan
    Write-Host "目标项目: $($targetProjects -join ', ')"
    Write-Host ""
}

# ---- 2. 提取 Go backend exported 方法名 ----
# 匹配形如：func (a *App) MethodName(  或  func (s *DeviceService) MethodName(
function Get-BackendMethods {
    param([string]$BackendDir)

    $methods = New-Object System.Collections.Generic.HashSet[string]
    $methodFiles = @{}

    Get-ChildItem -Path $BackendDir -Filter "*.go" -File | Where-Object {
        # 排除 _test.go
        -not $_.Name.EndsWith("_test.go")
    } | ForEach-Object {
        $content = Get-Content $_.FullName -Raw
        # 匹配指针接收器的 exported 方法（大写开头）
        # 形如：func (a *App) MethodName(
        # 形如：func (s *DeviceService) MethodName(
        $matches = [regex]::Matches($content, 'func\s+\(\w+\s+\*\w+\)\s+([A-Z]\w*)\s*\(')
        foreach ($m in $matches) {
            $methodName = $m.Groups[1].Value
            # 跳过 Wails lifecycle 方法（不暴露给前端）
            if ($methodName -in $wailsLifecycleMethods) {
                continue
            }
            [void]$methods.Add($methodName)
            $methodFiles[$methodName] = $_.Name
        }
    }

    return @{
        Methods = $methods
        Files = $methodFiles
    }
}

# ---- 3. 提取 binding JS/TS export function 名 ----
function Get-BindingExports {
    param([string]$BindingDir)

    $exports = New-Object System.Collections.Generic.HashSet[string]
    $exportFiles = @{}

    # 扫描所有 .js 和 .ts 文件（v3 binding 在 frontend/bindings/<project>/backend/*.ts 或 .js）
    Get-ChildItem -Path $BindingDir -Recurse -File | Where-Object {
        $_.Extension -in @(".js", ".ts") -and
        # 排除 .d.ts（声明文件）和 test 文件
        -not $_.Name.EndsWith(".d.ts") -and
        -not $_.Name.EndsWith(".test.ts") -and
        -not $_.Name.EndsWith(".test.js")
    } | ForEach-Object {
        $content = Get-Content $_.FullName -Raw
        # 匹配 export function MethodName(
        $matches = [regex]::Matches($content, 'export\s+function\s+([A-Z]\w*)\s*\(')
        foreach ($m in $matches) {
            $exportName = $m.Groups[1].Value
            [void]$exports.Add($exportName)
            $exportFiles[$exportName] = $_.FullName.Replace($BindingDir, "")
        }
    }

    return @{
        Exports = $exports
        Files = $exportFiles
    }
}

# ---- 4. 时间戳 stale 风险检查 ----
function Get-NewestMtime {
    param([string]$Dir, [string[]]$Filters = @("*.go"))

    $newest = $null
    $newestFile = ""
    foreach ($filter in $Filters) {
        Get-ChildItem -Path $Dir -Recurse -File -Filter $filter -ErrorAction SilentlyContinue | ForEach-Object {
            if ($newest -eq $null -or $_.LastWriteTime -gt $newest) {
                $newest = $_.LastWriteTime
                $newestFile = $_.Name
            }
        }
    }
    return @{
        Time = $newest
        File = $newestFile
    }
}

# ---- 5. 主检查循环 ----
foreach ($proj in $targetProjects) {
    if (-not $Quiet) {
        Write-Host "[$proj]" -ForegroundColor Cyan
    }

    $wailsDir = Join-Path $projectsRoot "$proj\apps\desktop-wails"
    $backendDir = Join-Path $wailsDir "backend"
    $bindingDir = Join-Path $wailsDir "frontend\bindings"

    # 5.1 提取方法
    $backendResult = Get-BackendMethods -BackendDir $backendDir
    $bindingResult = Get-BindingExports -BindingDir $bindingDir

    $backendMethods = $backendResult.Methods
    $bindingExports = $bindingResult.Exports

    if (-not $Quiet) {
        Write-Host "  backend exported 方法数: $($backendMethods.Count)"
        Write-Host "  binding export function 数: $($bindingExports.Count)"
    }

    # 5.2 缺失检查：backend 有但 binding 没有
    $missingInBinding = $backendMethods | Where-Object { $_ -notin $bindingExports }
    foreach ($m in $missingInBinding) {
        $sourceFile = $backendResult.Files[$m]
        $errors.Add("[$proj] backend 方法 '$m' (定义在 $sourceFile) 在 binding 中缺失——需运行 'wails3 generate bindings'")
    }

    # 5.3 多余检查：binding 有但 backend 没有（可能是改了名但没重新生成）
    $extraInBinding = $bindingExports | Where-Object { $_ -notin $backendMethods }
    foreach ($m in $extraInBinding) {
        $bindingFile = $bindingResult.Files[$m]
        $warnings.Add("[$proj] binding export '$m' (在 $bindingFile) 在 backend 中已不存在——可能是改名后未重新生成 binding")
    }

    # 5.4 stale 时间戳检查
    $backendNewest = Get-NewestMtime -Dir $backendDir -Filters @("*.go")
    # 排除 _test.go 的影响——单独再算一次仅生产代码
    $backendProdNewest = $null
    $backendProdNewestFile = ""
    Get-ChildItem -Path $backendDir -Filter "*.go" -File | Where-Object {
        -not $_.Name.EndsWith("_test.go")
    } | ForEach-Object {
        if ($backendProdNewest -eq $null -or $_.LastWriteTime -gt $backendProdNewest) {
            $backendProdNewest = $_.LastWriteTime
            $backendProdNewestFile = $_.Name
        }
    }

    $bindingNewest = $null
    $bindingNewestFile = ""
    Get-ChildItem -Path $bindingDir -Recurse -File | Where-Object {
        $_.Extension -in @(".js", ".ts", ".d.ts")
    } | ForEach-Object {
        if ($bindingNewest -eq $null -or $_.LastWriteTime -gt $bindingNewest) {
            $bindingNewest = $_.LastWriteTime
            $bindingNewestFile = $_.Name
        }
    }

    if ($backendProdNewest -and $bindingNewest) {
        $diffMinutes = ($backendProdNewest - $bindingNewest).TotalMinutes
        if ($diffMinutes -gt $StaleMinutes) {
            $warnings.Add("[$proj] backend 生产代码 ($backendProdNewestFile @ $($backendProdNewest.ToString('yyyy-MM-dd HH:mm'))) 比 binding 最新文件 ($bindingNewestFile @ $($bindingNewest.ToString('yyyy-MM-dd HH:mm'))) 新 $([math]::Round($diffMinutes, 1)) 分钟——若改了方法签名需运行 'wails3 generate bindings'")
        }
        if (-not $Quiet) {
            $status = if ($diffMinutes -gt $StaleMinutes) { "STALE 风险" } elseif ($diffMinutes -gt 0) { "略新" } else { "OK" }
            $color = if ($diffMinutes -gt $StaleMinutes) { "Yellow" } elseif ($diffMinutes -gt 0) { "DarkGray" } else { "Green" }
            Write-Host "  时间戳对比: $status (backend $([math]::Round($diffMinutes, 1)) 分钟 vs binding)" -ForegroundColor $color
        }
    }

    if (-not $Quiet) {
        Write-Host ""
    }
}

# ---- 6. 输出结果 ----
if ($warnings.Count -gt 0) {
    Write-Host "=== 警告 ($($warnings.Count)) ===" -ForegroundColor Yellow
    foreach ($w in $warnings) {
        Write-Host "  WARN: $w" -ForegroundColor Yellow
    }
    Write-Host ""
}

if ($errors.Count -gt 0) {
    Write-Host "=== 错误 ($($errors.Count)) ===" -ForegroundColor Red
    foreach ($e in $errors) {
        Write-Host "  FAIL: $e" -ForegroundColor Red
    }
    Write-Host ""
    Write-Host "FAIL: 发现 binding 不一致，请运行 'wails3 generate bindings' 重新生成" -ForegroundColor Red
    exit 1
}

if ($warnings.Count -gt 0) {
    Write-Host "WARN: 存在 stale 风险或多余 binding，建议检查后重新生成" -ForegroundColor Yellow
    exit 0
}

Write-Host "OK: 所有项目 binding 一致" -ForegroundColor Green
exit 0
