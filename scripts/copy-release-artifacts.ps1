[CmdletBinding()]
param(
    # Project name(s): wind-daq / daq-p1604 / daq-t1603 / motion-controller / five-hole-interpolator / three-hole-interpolator. Required.
    [Parameter(Mandatory = $true)]
    [ValidateNotNullOrEmpty()]
    [string[]]$Project,
    # Optional: explicit version string. If omitted, reads projects/<project>/VERSION.
    [string]$Version,
    # Dry-run mode: print actions without copying.
    [switch]$DryRun,
    # Quiet mode: suppress success output, only report errors.
    [switch]$Quiet
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

# Workspace root: script sits in <workspace>/scripts/, parent is workspace.
$workspaceRoot = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path
$projectsRoot = Join-Path $workspaceRoot "projects"

# Error and result collectors.
$errors = New-Object System.Collections.Generic.List[string]
$copied = New-Object System.Collections.Generic.List[string]

foreach ($proj in $Project) {
    # Tag prefix for log lines.
    $tag = "$proj"

    $projectDir = Join-Path $projectsRoot $proj
    if (-not (Test-Path $projectDir)) {
        $errors.Add("$tag | project dir missing -> $projectDir")
        continue
    }

    # Resolve version: prefer -Version param, else read VERSION file.
    $resolvedVersion = $Version
    if ([string]::IsNullOrWhiteSpace($resolvedVersion)) {
        $versionFile = Join-Path $projectDir "VERSION"
        if (-not (Test-Path $versionFile)) {
            $errors.Add("$tag | VERSION file missing -> $versionFile")
            continue
        }
        $resolvedVersion = (Get-Content $versionFile -Raw).Trim()
        if ([string]::IsNullOrWhiteSpace($resolvedVersion)) {
            $errors.Add("$tag | VERSION file empty -> $versionFile")
            continue
        }
    }

    # Build/bin dir and archive dir.
    $wailsDir = Join-Path $projectDir "apps\desktop-wails"
    $buildBinDir = Join-Path $wailsDir "build\bin"
    $releasesBinDir = Join-Path $projectDir "releases\bin"

    # NSIS installer naming: <project>-<version>-amd64-installer.exe
    $installerName = "$proj-$resolvedVersion-amd64-installer.exe"
    $installerSrc = Join-Path $buildBinDir $installerName
    $installerDst = Join-Path $releasesBinDir $installerName

    if (-not (Test-Path $installerSrc)) {
        $errors.Add("$tag | installer missing -> $installerSrc  run makensis in apps\desktop-wails\windows\nsis first (e.g. makensis -DARG_WAILS_AMD64_BINARY=..\..\build\bin\probe-interpolator.exe project.nsi)")
        continue
    }

    if ($DryRun) {
        $copied.Add("$tag | DRY-RUN  $installerSrc  ->  $installerDst")
        continue
    }

    # Create archive dir if missing.
    if (-not (Test-Path $releasesBinDir)) {
        New-Item -ItemType Directory -Force -Path $releasesBinDir | Out-Null
    }

    # Copy and overwrite.
    Copy-Item -Path $installerSrc -Destination $installerDst -Force
    if (-not $Quiet) {
        $sizeBytes = (Get-Item $installerDst).Length
        $sizeMb = [math]::Round($sizeBytes / 1MB, 2)
        $logMsg = "$tag | archived  $installerName  $sizeMb MB  ->  $releasesBinDir"
        Write-Host $logMsg -ForegroundColor Green
    }
    $copied.Add("$tag | $installerDst")
}

# Summary output.
if ($DryRun -and $copied.Count -gt 0) {
    $count = $copied.Count
    Write-Host ""
    Write-Host "Dry-run preview - $count item(s)" -ForegroundColor Cyan
    $copied | ForEach-Object { Write-Host "  $_" }
}

if ($errors.Count -gt 0) {
    $errCount = $errors.Count
    Write-Host ""
    Write-Host "Errors - $errCount total" -ForegroundColor Red
    $errors | ForEach-Object { Write-Host "  $_" -ForegroundColor Red }
    exit 1
}

exit 0
