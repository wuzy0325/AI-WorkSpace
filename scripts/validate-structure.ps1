[CmdletBinding()]
param(
    [switch]$Quiet
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

$workspaceRoot = (Resolve-Path (Join-Path $PSScriptRoot ".." )).Path
$configPath = Join-Path $workspaceRoot "workspace.structure.json"

if (-not (Test-Path -Path $configPath -PathType Leaf)) {
    Write-Error "Missing workspace structure config: workspace.structure.json"
    exit 1
}

$config = Get-Content -Path $configPath -Raw | ConvertFrom-Json
$errors = New-Object System.Collections.Generic.List[string]

function Resolve-WorkspacePath {
    param(
        [Parameter(Mandatory = $true)]
        [string]$RelativePath
    )

    $normalized = $relativePath -replace "/", [System.IO.Path]::DirectorySeparatorChar
    return Join-Path $workspaceRoot $normalized
}

# ---- 1. Directory & File existence checks ----
foreach ($requiredDir in $config.requiredDirectories) {
    $path = Resolve-WorkspacePath -RelativePath $requiredDir
    if (-not (Test-Path -Path $path -PathType Container)) {
        $errors.Add("Missing required directory: $requiredDir")
    }
}

foreach ($requiredFile in $config.requiredFiles) {
    $path = Resolve-WorkspacePath -RelativePath $requiredFile
    if (-not (Test-Path -Path $path -PathType Leaf)) {
        $errors.Add("Missing required file: $requiredFile")
    }
}

if (-not $config.allowUnknownTopLevelEntries) {
    $allowed = @($config.allowedTopLevelEntries)
    $topLevelEntries = Get-ChildItem -Path $workspaceRoot -Force | Select-Object -ExpandProperty Name

    foreach ($entry in $topLevelEntries) {
        if ($entry -notin $allowed) {
            $errors.Add("Unexpected top-level entry: $entry")
        }
    }
}

# ---- 2. Go Architecture Import Validation ----

function Get-GoImports {
    param([string]$FileContent)
    $imports = New-Object System.Collections.Generic.List[string]

    # Match simple imports like "pkg/path"
    $pattern = '^\s+"([^"]+)"'

    # Match grouped imports inside ()
    $inGroup = $false
    foreach ($line in $FileContent -split "`n") {
        $trimmed = $line.Trim()
        if ($trimmed -match '^import\s*\(') { $inGroup = $true; continue }
        if ($inGroup -and $trimmed -eq ')') { $inGroup = $false; continue }

        if ($trimmed -match $pattern) {
            $imports.Add($matches[1])
        }
    }
    return $imports
}

# 2a. usecase must not import internal/adapters
$usecaseFiles = Get-ChildItem -Path (Join-Path $workspaceRoot "projects/wind-daq/services/api-go/internal/usecase") -Filter "*.go" -Recurse -ErrorAction SilentlyContinue
foreach ($f in $usecaseFiles) {
    $content = Get-Content -Path $f.FullName -Raw -ErrorAction SilentlyContinue
    if (-not $content) { continue }
    $imports = Get-GoImports -FileContent $content
    foreach ($imp in $imports) {
        if ($imp -match 'internal/adapters/') {
            $relPath = $f.FullName.Substring($workspaceRoot.Length + 1)
            $errors.Add("ARCH: usecase imports adapter: $relPath -> $imp")
        }
    }
}

# 2b. core must not import hardware, file I/O, network, or framework packages
$coreFiles = Get-ChildItem -Path (Join-Path $workspaceRoot "projects/wind-daq/services/api-go/internal/core") -Filter "*.go" -Recurse -ErrorAction SilentlyContinue
$forbiddenCorePatterns = @(
    'adapters/',
    'os\.',
    'net\.',
    'io/ioutil',
    'serial',
    'github.com/',
    'gopkg\.in/',
    'google\.golang\.org/'
)
foreach ($f in $coreFiles) {
    $content = Get-Content -Path $f.FullName -Raw -ErrorAction SilentlyContinue
    if (-not $content) { continue }
    $imports = Get-GoImports -FileContent $content
    foreach ($imp in $imports) {
        foreach ($pattern in $forbiddenCorePatterns) {
            if ($imp -match $pattern) {
                $relPath = $f.FullName.Substring($workspaceRoot.Length + 1)
                $errors.Add("ARCH: core imports forbidden package: $relPath -> $imp (matches: $pattern)")
            }
        }
    }
}

# 2c. ports must not contain struct method definitions
$portsFiles = Get-ChildItem -Path (Join-Path $workspaceRoot "projects/wind-daq/services/api-go/internal/ports") -Filter "*.go" -Recurse -ErrorAction SilentlyContinue
foreach ($f in $portsFiles) {
    $content = Get-Content -Path $f.FullName -Raw -ErrorAction SilentlyContinue
    if (-not $content) { continue }
    # Check for func \(receiver\) method patterns
    if ($content -match 'func\s+\([^)]+\)\s+\w+\(') {
        $relPath = $f.FullName.Substring($workspaceRoot.Length + 1)
        $errors.Add("ARCH: ports has struct method: $relPath (ports must only define interfaces)")
    }
}

# 2d. programs must not import projects/*/internal/*
$programsDirs = Get-ChildItem -Path (Join-Path $workspaceRoot "programs") -Directory -ErrorAction SilentlyContinue
foreach ($progDir in $programsDirs) {
    $progFiles = Get-ChildItem -Path $progDir.FullName -Filter "*.go" -Recurse -ErrorAction SilentlyContinue
    foreach ($f in $progFiles) {
        $content = Get-Content -Path $f.FullName -Raw -ErrorAction SilentlyContinue
        if (-not $content) { continue }
        $imports = Get-GoImports -FileContent $content
        foreach ($imp in $imports) {
            if ($imp -match 'projects/.*/internal/') {
                $relPath = $f.FullName.Substring($workspaceRoot.Length + 1)
                $errors.Add("ARCH: program imports project internal: $relPath -> $imp")
            }
        }
    }
}

# ---- 2e. Go file line count check (code-standards.zh-CN.md §一) ----
# 单个非测试 .go 文件不得超过 500 行；超出且不在豁免清单的，视为结构违规。
# 豁免清单用于过渡期管理预存超长文件，新增超长文件不得加入清单。

$maxGoFileLines = 500
$waiverPath = Join-Path $PSScriptRoot "go-file-waivers.txt"
# 用 .NET API 显式 UTF-8 读取，避免 PowerShell 5.x 默认 GBK 解码导致中文注释乱码与路径错位
$goFileWaivers = New-Object System.Collections.Generic.HashSet[string]([System.StringComparer]::OrdinalIgnoreCase)
if (Test-Path -Path $waiverPath -PathType Leaf) {
    $waiverLines = [System.IO.File]::ReadAllLines($waiverPath, [System.Text.Encoding]::UTF8)
    foreach ($wl in $waiverLines) {
        $trimmed = $wl.Trim()
        if ($trimmed -and -not $trimmed.StartsWith('#')) {
            # 统一用正斜杠比较，避免 Windows 反斜杠差异
            $null = $goFileWaivers.Add(($trimmed -replace '\\', '/'))
        }
    }
}

# 扫描 projects/ 下所有 .go 文件，排除测试文件与生成产物目录
$goFiles = Get-ChildItem -Path (Join-Path $workspaceRoot "projects") -Filter "*.go" -Recurse -ErrorAction SilentlyContinue |
    Where-Object {
        $_.FullName -notmatch '\\(node_modules|vendor|\.git|build|dist)\\' -and
        $_.Name -notmatch '_test\.go$'
    }

foreach ($f in $goFiles) {
    $lineCount = (Get-Content -Path $f.FullName -ErrorAction SilentlyContinue | Measure-Object -Line).Lines
    if ($lineCount -le $maxGoFileLines) { continue }

    # 相对路径并统一为正斜杠，便于与豁免清单比对
    $relPath = $f.FullName.Substring($workspaceRoot.Length + 1) -replace '\\', '/'
    if ($goFileWaivers.Contains($relPath)) { continue }

    $errors.Add("SIZE: Go file exceeds $maxGoFileLines lines: $relPath ($lineCount lines, see docs/runbooks/code-standards.zh-CN.md section 1)")
}

# ---- 3. Results ----
if ($errors.Count -gt 0) {
    if (-not $Quiet) {
        Write-Host "Workspace structure check failed." -ForegroundColor Red
        foreach ($errorLine in $errors) {
            Write-Host " - $errorLine" -ForegroundColor Red
        }
        Write-Host ""
        Write-Host "If this change is intentional, update workspace.structure.json first."
    }
    exit 1
}

if (-not $Quiet) {
    Write-Host "Workspace structure check passed." -ForegroundColor Green
}

exit 0
