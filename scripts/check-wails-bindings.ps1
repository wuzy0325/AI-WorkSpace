[CmdletBinding()]
param(
    # 指定要检查的项目名（如 WindLabX4 / wispa / wista），不传则检查所有 Wails 项目
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

# ---- FNV-1a 32 位哈希（与 Wails v3 internal/hash.Fnv 完全一致）----
# Wails v3 的绑定 ID 是 FNV(FQN) 的 32 位无符号值，其中
#   FQN = "<module 包路径>.<接收器类型名>.<方法名>"
# （见 wails/v3/internal/hash/fnv.go 与 pkg/application/bindings.go）。
# 任何影响 FQN 的变更（模块改名、方法增删、类型改名）都会改变 ID。
# 只有比对 ID 哈希才能发现"方法名一致但 ID 漂移"的绑定错位，
# 纯方法名对比对此类问题完全失灵——这是三项目绑定 ID 漂移漏网的根本原因。
function Get-BindingId {
    param([Parameter(Mandatory)][string]$Fqn)
    # FNV_offset_basis = 2166136261，FNV_prime = 16777619（乘后取模 2^32）
    [uint64]$hash = 2166136261
    foreach ($byteVal in [System.Text.Encoding]::UTF8.GetBytes($Fqn)) {
        $hash = $hash -bxor [uint64]$byteVal
        $hash = ($hash * 16777619) % 4294967296
    }
    return $hash
}

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

# ---- 2. 提取 Go backend exported 方法名 + 期望绑定 ID ----
# 匹配形如：func (a *App) MethodName(  或  func (s *DeviceService) MethodName(
function Get-BackendMethods {
    param(
        [string]$BackendDir,
        # go.mod 的 module 声明（module 包路径），用于构造 FQN 计算绑定 ID
        [string]$Module
    )

    $methods = New-Object System.Collections.Generic.HashSet[string]
    $methodFiles = @{}
    # 期望绑定 ID 表：方法名 -> 期望 ID 列表（同一方法名可能分布在多个类型上）
    $methodIds = @{}

    Get-ChildItem -Path $BackendDir -Filter "*.go" -File | Where-Object {
        # 排除 _test.go
        -not $_.Name.EndsWith("_test.go")
    } | ForEach-Object {
        $content = Get-Content $_.FullName -Raw
        # 匹配指针接收器的 exported 方法（大写开头）
        # group1 = 接收器变量名，group2 = 接收器类型名，group3 = 方法名
        $matches = [regex]::Matches($content, 'func\s+\((\w+)\s+\*(\w+)\)\s+([A-Z]\w*)\s*\(')
        foreach ($m in $matches) {
            $methodName = $m.Groups[3].Value
            $typeName = $m.Groups[2].Value
            # 跳过 Wails lifecycle 方法（不暴露给前端）
            if ($methodName -in $wailsLifecycleMethods) {
                continue
            }
            [void]$methods.Add($methodName)
            $methodFiles[$methodName] = $_.Name

            # 用 FQN 计算期望绑定 ID。Wails v3 使用 FNV(包导入路径.类型.方法名)。
            # backend 类型都位于 module 下的 backend 包，故完整导入路径 = <module>/backend。
            # FQN 作为字符串哈希，因此必须与 binding JS 中 $Call.ByID(N) 一致。
            if (-not [string]::IsNullOrEmpty($Module)) {
                $fqn = "$Module/backend.$typeName.$methodName"
                $id = Get-BindingId -Fqn $fqn
                if (-not $methodIds.ContainsKey($methodName)) {
                    $methodIds[$methodName] = New-Object System.Collections.Generic.List[string]
                }
                $holder = $methodIds[$methodName]
                # 去重：同一方法名多个类型时只保留不同 ID
                $idStr = [string]$id
                if ($idStr -notin $holder) {
                    $holder.Add($idStr)
                }
            }
        }
    }

    return @{
        Methods = $methods
        Files = $methodFiles
        # 方法名 -> 期望绑定 ID 集合（string 列表）
        Ids = $methodIds
    }
}

# ---- 3. 提取 binding JS/TS export function 名 + 实际绑定 ID ----
function Get-BindingExports {
    param([string]$BindingDir)

    $exports = New-Object System.Collections.Generic.HashSet[string]
    $exportFiles = @{}
    # 实际绑定 ID 表：方法名 -> 实际 ID 列表
    $exportIds = @{}

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
        $fnRegex = [regex]'export\s+function\s+([A-Z]\w*)\s*\('
        $fnMatches = $fnRegex.Matches($content)
        for ($i = 0; $i -lt $fnMatches.Count; $i++) {
            $exportName = $fnMatches[$i].Groups[1].Value
            [void]$exports.Add($exportName)
            $exportFiles[$exportName] = $_.FullName.Replace($BindingDir, "")

            # 取该 export function 的函数体（到下一个 export function 或文件结尾），
            # 在函数体内找首个 $Call.ByID(<数字>) 作为实际绑定 ID
            $start = $fnMatches[$i].Index
            $end = if ($i + 1 -lt $fnMatches.Count) { $fnMatches[$i + 1].Index } else { $content.Length }
            $block = $content.Substring($start, $end - $start)
            $idMatch = [regex]::Match($block, '\$Call\.ByID\(\s*(\d+)')
            if ($idMatch.Success) {
                $idStr = $idMatch.Groups[1].Value
                if (-not $exportIds.ContainsKey($exportName)) {
                    $exportIds[$exportName] = New-Object System.Collections.Generic.List[string]
                }
                $holder = $exportIds[$exportName]
                if ($idStr -notin $holder) {
                    $holder.Add($idStr)
                }
            }
        }
    }

    return @{
        Exports = $exports
        Files = $exportFiles
        # 方法名 -> 实际绑定 ID 集合（string 列表）
        Ids = $exportIds
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

    # 读取 go.mod module 声明，用于构造 backend 方法的 FQN 计算期望绑定 ID
    $module = ""
    $goModPath = Join-Path $wailsDir "go.mod"
    if (Test-Path $goModPath) {
        $moduleLine = Select-String -Path $goModPath -Pattern '^module ' | Select-Object -First 1
        if ($moduleLine) {
            $module = ($moduleLine.Line -replace '^module\s+', '').Trim()
        }
    }

    # 5.1 提取方法
    $backendResult = Get-BackendMethods -BackendDir $backendDir -Module $module
    $bindingResult = Get-BindingExports -BindingDir $bindingDir

    $backendMethods = $backendResult.Methods
    $bindingExports = $bindingResult.Exports

    if (-not $Quiet) {
        Write-Host "  backend exported 方法数: $($backendMethods.Count)"
        Write-Host "  binding export function 数: $($bindingExports.Count)"
    }

    # 5.2 缺失检查：backend 有但 binding 没有
    $missingInBinding = @($backendMethods | Where-Object { $_ -notin $bindingExports })
    foreach ($m in $missingInBinding) {
        $sourceFile = $backendResult.Files[$m]
        $errors.Add("[$proj] backend 方法 '$m' (定义在 $sourceFile) 在 binding 中缺失——需运行 'wails3 generate bindings'")
    }

    # 5.3 多余检查：binding 有但 backend 没有（可能是改了名但没重新生成）
    $extraInBinding = @($bindingExports | Where-Object { $_ -notin $backendMethods })
    foreach ($m in $extraInBinding) {
        $bindingFile = $bindingResult.Files[$m]
        $warnings.Add("[$proj] binding export '$m' (在 $bindingFile) 在 backend 中已不存在——可能是改名后未重新生成 binding")
    }

    # 5.4 ID 漂移检查：后端期望绑定 ID vs binding JS 实际 $Call.ByID
    # 这是防止"方法名一致但 ID 漂移"的关键检查。Wails v3 绑定 ID 是 FNV(FQN) 的哈希，
    # 方法名对比无法发现模块改名后 bindings 未重新生成导致的 ID 错位。
    # 只对方法名在双方都存在的方法比对，避免重复报缺失/多余错误。
    $idDrift = 0
    if (-not [string]::IsNullOrEmpty($module)) {
        $backendIds = $backendResult.Ids
        $bindingIds = $bindingResult.Ids
        foreach ($mName in $backendMethods) {
            if ($mName -notin $bindingExports) {
                continue  # 缺失已在 5.2 报告
            }
            $expected = $backendIds[$mName]
            $actual = $bindingIds[$mName]
            if (-not $expected -or -not $actual) {
                continue  # 无法提取 ID 的方法（正常不出现，跳过）
            }
            # 期望与实际任何一项匹配即认为一致（同名方法可能分布在多个类型上）
            $match = $false
            foreach ($e in $expected) {
                if ($e -in $actual) { $match = $true; break }
            }
            if (-not $match) {
                $sourceFile = $backendResult.Files[$mName]
                $bindingFile = $bindingResult.Files[$mName]
                $errMsg = "[$proj] 绑定 ID 漂移: 方法 '$mName'（backend 定义于 $sourceFile）——binding($bindingFile) 实际 ID $($actual -join ',') 与后端期望 ID $($expected -join ',') 不匹配，疑似模块改名/方法变更后未运行 'wails3 generate bindings'"
                $errors.Add($errMsg)
                $idDrift++
            }
        }
    }

    # 5.5 stale 时间戳检查
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

    # 方法层面是否一致：缺失和多余都为 0 时，backend 与 binding 的方法集合完全匹配。
    # 此时 stale 时间戳差异只能来自函数体内部变更（不影响 binding），降级为 info；
    # 方法层面不一致时，stale 保持 warning（可能是改名/新增后未重新生成）。
    $methodConsistent = ($missingInBinding.Count -eq 0) -and ($extraInBinding.Count -eq 0)

    if ($backendProdNewest -and $bindingNewest) {
        $diffMinutes = ($backendProdNewest - $bindingNewest).TotalMinutes
        if ($diffMinutes -gt $StaleMinutes) {
            $msg = "[$proj] backend 生产代码 ($backendProdNewestFile @ $($backendProdNewest.ToString('yyyy-MM-dd HH:mm'))) 比 binding 最新文件 ($bindingNewestFile @ $($bindingNewest.ToString('yyyy-MM-dd HH:mm'))) 新 $([math]::Round($diffMinutes, 1)) 分钟"
            if ($methodConsistent) {
                # 方法层面一致：函数体内部变更不影响 binding，降级为 info（不计入 warnings，不阻断提交）
                if (-not $Quiet) {
                    Write-Host "  INFO: $msg——方法签名未变（函数体内部变更），binding 无需重新生成" -ForegroundColor DarkGray
                }
            } else {
                # 方法层面不一致：可能是改名/新增后未重新生成，保持 warning
                $warnings.Add("$msg——方法签名有变更，需运行 'wails3 generate bindings'")
            }
        }
        if (-not $Quiet) {
            if ($diffMinutes -gt $StaleMinutes) {
                $status = if ($methodConsistent) { "STALE（方法一致）" } else { "STALE 风险" }
                $color = if ($methodConsistent) { "DarkGray" } else { "Yellow" }
            } elseif ($diffMinutes -gt 0) {
                $status = "略新"; $color = "DarkGray"
            } else {
                $status = "OK"; $color = "Green"
            }
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
