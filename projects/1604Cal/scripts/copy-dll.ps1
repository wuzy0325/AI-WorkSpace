# 复制 DAQ-P-1603 依赖的 WTNDAQ16H_64.dll 到构建输出目录。
#
# 设计动机：1604Cal 的 DAQ-P-1603 驱动通过 vendor DLL（WTNDAQ16H_64.dll）
# 建立 TCP 连接。若 DLL 未随可执行文件发布，驱动 Connect() 会返回
# "WTNDAQ16H DLL not initialized"，表现即为"能 ping 通但无法连接"。
# 本脚本确保每次构建后 DLL 都在 build\bin（exe 同目录），与 WindLabX4 对齐。
#
# 路径解析优先级：
#   1. 环境变量 WTNDAQ16H_DLL_PATHS（分号分隔，按顺序尝试，CI 可覆盖）
#   2. 默认值：WindLabX4 构建输出 / 已安装驱动目录 / SVN 旧路径
# 未找到任一 DLL 不阻止构建（exit 0），仅打印警告（运行时再报错更易定位）。
#
# 用法：powershell -ExecutionPolicy Bypass -File scripts/copy-dll.ps1
# 可选参数 -Dest 指定目标目录（默认 build\bin）。

[CmdletBinding()]
param(
    [string]$Dest = 'build\bin'
)

$ErrorActionPreference = 'SilentlyContinue'

# 默认搜索路径（环境变量优先，无则回退到已知候选位置）
$envPaths = $env:WTNDAQ16H_DLL_PATHS
if ($envPaths) {
    $paths = $envPaths -split ';'
} else {
    $paths = @(
        '..\AI-Workspace\projects\windlabx4\apps\desktop-wails\build\bin\WTNDAQ16H_64.dll',
        'C:\Program Files\wind-daq\wind-daq\WTNDAQ16H_64.dll',
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
            break
        }
    }
}

if (-not $copied) {
    Write-Warning "[copy-dll] WTNDAQ16H_64.dll not found in any search path; build will continue without it."
}

# 始终 exit 0：DLL 缺失不阻断构建（运行时再报错更易定位）。
exit 0
