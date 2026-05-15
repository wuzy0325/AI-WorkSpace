# lint-go.ps1
# Run Go lint checks: gofmt + build
$ErrorActionPreference = "Stop"
$root = Resolve-Path "$PSScriptRoot/.."

# Find all Go service directories
$services = Get-ChildItem "$root/projects/*/services/api-go" -Directory
$programs = Get-ChildItem "$root/programs/*" -Directory

$targets = @()
foreach ($s in $services) { $targets += $s.FullName }
foreach ($p in $programs) {
    if (Test-Path "$p/go.mod") { $targets += $p.FullName }
}

$hasError = $false
foreach ($dir in $targets) {
    Write-Host "=== Checking $dir ===" -ForegroundColor Cyan

    # gofmt check
    $unformatted = gofmt -l $dir
    if ($unformatted) {
        Write-Host "  gofmt: FAIL (unformatted files)" -ForegroundColor Red
        $unformatted | ForEach-Object { Write-Host "    $_" -ForegroundColor Red }
        $hasError = $true
    } else {
        Write-Host "  gofmt: PASS" -ForegroundColor Green
    }

    # go build
    Push-Location $dir
    $output = go build -buildvcs=false ./... 2>&1
    if ($LASTEXITCODE -ne 0) {
        Write-Host "  build: FAIL" -ForegroundColor Red
        $output | ForEach-Object { Write-Host "    $_" -ForegroundColor Red }
        $hasError = $true
    } else {
        Write-Host "  build: PASS" -ForegroundColor Green
    }
    Pop-Location
}

if ($hasError) { exit 1 } else { Write-Host "All checks passed" -ForegroundColor Green }
