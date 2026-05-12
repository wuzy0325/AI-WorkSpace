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

function Get-RelativePath {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Path
    )

    return $Path.Substring($workspaceRoot.Length + 1)
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

# ---- 2. Rust Architecture Boundary Validation ----

function Test-RustFileOutsideComments {
    param(
        [Parameter(Mandatory = $true)]
        [string]$FileContent,
        [Parameter(Mandatory = $true)]
        [string]$Pattern
    )

    foreach ($line in $FileContent -split "`n") {
        $trimmed = $line.Trim()
        if ($trimmed.StartsWith("//") -or $trimmed.StartsWith("#![") -or $trimmed.StartsWith("#[")) {
            continue
        }

        if ($trimmed -match $Pattern) {
            return $true
        }
    }
    return $false
}

$projectDirs = Get-ChildItem -Path (Join-Path $workspaceRoot "projects") -Directory -ErrorAction SilentlyContinue
foreach ($projectDir in $projectDirs) {
    $tauriRoot = Join-Path $projectDir.FullName "apps/desktop-tauri/src-tauri"
    if (Test-Path -Path $tauriRoot -PathType Container) {
        $tauriFiles = Get-ChildItem -Path $tauriRoot -Filter "*.rs" -Recurse -ErrorAction SilentlyContinue
        foreach ($f in $tauriFiles) {
            $content = Get-Content -Path $f.FullName -Raw -ErrorAction SilentlyContinue
            if (-not $content) { continue }

            if (Test-RustFileOutsideComments -FileContent $content -Pattern '::(core|usecase|ports|adapters)\b') {
                $relPath = Get-RelativePath -Path $f.FullName
                $errors.Add("ARCH: Tauri shell imports backend internals: $relPath (use commands/API bridge only)")
            }
        }
    }

    $apiRoot = Join-Path $projectDir.FullName "services/api-rs"
    if (-not (Test-Path -Path $apiRoot -PathType Container)) {
        continue
    }

    $srcRoot = Join-Path $apiRoot "src"

    $usecaseFiles = Get-ChildItem -Path (Join-Path $srcRoot "usecase") -Filter "*.rs" -Recurse -ErrorAction SilentlyContinue
    foreach ($f in $usecaseFiles) {
        $content = Get-Content -Path $f.FullName -Raw -ErrorAction SilentlyContinue
        if (-not $content) { continue }

        if (Test-RustFileOutsideComments -FileContent $content -Pattern 'crate::adapters|super::super::adapters') {
            $relPath = Get-RelativePath -Path $f.FullName
            $errors.Add("ARCH: Rust usecase depends on adapter: $relPath")
        }
    }

    $coreFiles = Get-ChildItem -Path (Join-Path $srcRoot "core") -Filter "*.rs" -Recurse -ErrorAction SilentlyContinue
    foreach ($f in $coreFiles) {
        $content = Get-Content -Path $f.FullName -Raw -ErrorAction SilentlyContinue
        if (-not $content) { continue }

        $coreForbiddenPatterns = @(
            'crate::adapters',
            'crate::ports',
            '\bstd::fs\b',
            '\bstd::net\b',
            '\btokio::net\b',
            '\bserialport::',
            '\baxum::',
            '\bactix_',
            '\brocket::'
        )

        foreach ($pattern in $coreForbiddenPatterns) {
            if (Test-RustFileOutsideComments -FileContent $content -Pattern $pattern) {
                $relPath = Get-RelativePath -Path $f.FullName
                $errors.Add("ARCH: Rust core uses forbidden dependency: $relPath (matches: $pattern)")
            }
        }
    }

    $portsFiles = Get-ChildItem -Path (Join-Path $srcRoot "ports") -Filter "*.rs" -Recurse -ErrorAction SilentlyContinue
    foreach ($f in $portsFiles) {
        $content = Get-Content -Path $f.FullName -Raw -ErrorAction SilentlyContinue
        if (-not $content) { continue }
        $relPath = Get-RelativePath -Path $f.FullName

        if (Test-RustFileOutsideComments -FileContent $content -Pattern '^\s*(pub\s+)?fn\s+\w+') {
            $errors.Add("ARCH: Rust ports has function implementation: $relPath (ports must define traits only)")
        }
    }
}

# 2b. programs must not import projects/*/services/* private internals
$programsDirs = Get-ChildItem -Path (Join-Path $workspaceRoot "programs") -Directory -ErrorAction SilentlyContinue
foreach ($progDir in $programsDirs) {
    $progFiles = Get-ChildItem -Path $progDir.FullName -Include "*.go","*.rs" -File -Recurse -ErrorAction SilentlyContinue
    foreach ($f in $progFiles) {
        $content = Get-Content -Path $f.FullName -Raw -ErrorAction SilentlyContinue
        if (-not $content) { continue }

        if ($content -match 'projects/.*/services/.*/(internal|src/(core|usecase|ports|adapters))') {
            $relPath = Get-RelativePath -Path $f.FullName
            $errors.Add("ARCH: program imports project private service code: $relPath")
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
