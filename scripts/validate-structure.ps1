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
