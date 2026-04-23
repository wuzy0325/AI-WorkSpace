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

    $normalized = $RelativePath -replace "/", [System.IO.Path]::DirectorySeparatorChar
    return Join-Path $workspaceRoot $normalized
}

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
