# 复制 DAQ-P-1603 依赖的 WTNDAQ16H_64.dll 到构建输出目录。
#
# 设计动机：原先这段逻辑内联在 Taskfile.yml 的 PowerShell -Command 字符串中，
# wails3 task 在 Windows 上会把 YAML 里的 `$$` 转义符替换成进程 PID
# （例如 14108），导致 PowerShell 报 "Missing argument in parameter list"
# 语法错误。抽到独立脚本文件可彻底规避 `$$` 转义与 YAML 引号嵌套问题，
# 同时便于本地独立测试与 CI 复用。
#
# 路径解析优先级：
#   1. 环境变量 WTNDAQ16H_DLL_PATHS（分号分隔，按顺序尝试，CI 可覆盖）
#   2. 默认值：WindLabX 仓库 → ART 驱动目录
# 未找到任何 DLL 不阻止构建（exit 0），仅打印警告。
# 拷贝失败（权限/占用）静默跳过，不阻断构建流程。
#
# 用法：powershell -ExecutionPolicy Bypass -File scripts/copy-dll.ps1
# 可选参数 -Dest 指定目标目录（默认 build/bin）。

[CmdletBinding()]
param(
    [string]$Dest = 'build\bin'
)

$ErrorActionPreference = 'SilentlyContinue'

# 默认搜索路径（环境变量优先，无则回退到开发环境固定路径）
$envPaths = $env:WTNDAQ16H_DLL_PATHS
if ($envPaths) {
    $paths = $envPaths -split ';'
} else {
    $paths = @(
        'D:\SVN\SoftWare\trunk\WindLabX\data\WTNDAQ16H_64.dll',
        'C:\ART\WTNDAQ16H\Driver\INF\WTNDAQ16H_64.dll'
    )
}

$copied = $false
foreach ($p in $paths) {
    if ([string]::IsNullOrWhiteSpace($p)) { continue }
    if (Test-Path $p) {
        Copy-Item $p $Dest -Force -ErrorAction SilentlyContinue
        if ($?) {
            Write-Host "[copy-dll] copied: $p -> $Dest"
            $copied = $true
        }
    }
}

if (-not $copied) {
    Write-Warning "[copy-dll] WTNDAQ16H_64.dll not found in any search path; build will continue without it."
}

# 始终 exit 0：DLL 缺失不阻断构建（运行时再报错更易定位）
exit 0
