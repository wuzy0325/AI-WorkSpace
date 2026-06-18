<#
.SYNOPSIS
    Cross-project hexagonal architecture import-direction validator.
    Does not require golangci-lint.

.DESCRIPTION
    Scans every Go file in the workspace and enforces the CLAUDE.md
    "Hard Constraints" + "Constraint Clarifications" rules:

      1. core/         no byte I/O, no frameworks, no hardware drivers,
                       no adapters, no usecase imports.
                       (format-description structs are allowed; only byte
                       I/O packages are denied)
      2. ports/        interface definitions only (no os/net/serial/framework,
                       no adapters, no struct methods)
      3. usecase/      no adapters/* imports, no hardware drivers, no file I/O,
                       no UI frameworks. Composition root is exempt:
                       pkg/appcontext, cmd, apps/desktop-wails/backend.
      4. adapters/hardware/  no domain-algorithm keywords (protocol translation only)
      5. shared/algorithms/  no device dependencies, no I/O
      6. shared/       no projects/*/internal/* imports
      7. programs/     no projects/*/internal/* imports

    Complements .golangci.yml: depguard handles stdlib package checks;
    this script handles cross-project adapters path globbing (each project
    has a different module path prefix, which depguard cannot unify).

.PARAMETER Quiet
    Suppress pass/fail summary; print violations only.

.EXAMPLE
    powershell -File .\scripts\validate-import-direction.ps1
#>
[CmdletBinding()]
param(
    [switch]$Quiet
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

$workspaceRoot = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path
$errors = New-Object System.Collections.Generic.List[string]
$warnings = New-Object System.Collections.Generic.List[string]
$scannedFiles = 0

# ---- Helper: extract import paths from Go source ----
function Get-GoImports {
    param([string]$FileContent)
    $imports = New-Object System.Collections.Generic.List[string]
    # Lines are Trim()'d before matching, so patterns must NOT require leading whitespace.
    $groupPattern = '^(?:\w+\s+)?"([^"]+)"'
    $singlePattern = '^import\s+(?:\w+\s+)?"([^"]+)"'
    $inGroup = $false
    foreach ($line in $FileContent -split "`n") {
        $trimmed = $line.Trim()
        if ($trimmed -match '^import\s*\(') { $inGroup = $true; continue }
        if ($inGroup -and $trimmed -eq ')') { $inGroup = $false; continue }
        if ($inGroup -and $trimmed -match $groupPattern) {
            $imports.Add($matches[1])
        } elseif (-not $inGroup -and $trimmed -match $singlePattern) {
            $imports.Add($matches[1])
        }
    }
    return $imports
}

function Test-GoTestFile {
    param([string]$FileName)
    return $FileName -match '_test\.go$'
}

function Get-RelativePath {
    param([string]$FullPath)
    return $FullPath.Substring($workspaceRoot.Length + 1)
}

# ---- Collect all Go files (skip vendor / node_modules / .git / .wails) ----
$goFiles = Get-ChildItem -Path $workspaceRoot -Filter "*.go" -Recurse -File -ErrorAction SilentlyContinue |
    Where-Object {
        $_.FullName -notmatch '[\\/](vendor|node_modules|\.git|\.wails)[\\/]'
    }

# ---- Composition root whitelist ----
# Per CLAUDE.md "Constraint Clarifications" rule 2: only these locations may
# import ports + adapters + usecase simultaneously.
$compositionRootPatterns = @(
    '[\\/]pkg[\\/]appcontext[\\/]',
    '[\\/]pkg[\\/]wiring[\\/]',
    '[\\/]pkg[\\/]apiserver[\\/]',
    '[\\/]internal[\\/]bootstrap[\\/]',
    '[\\/]cmd[\\/]',
    '[\\/]apps[\\/]desktop-wails[\\/]backend[\\/]',
    '[\\/]apps[\\/]desktop-wails[\\/]main\.go$',
    '[\\/]services[\\/]api-go[\\/]pkg[\\/]'
)

function Test-IsCompositionRoot {
    param([string]$RelativePath)
    foreach ($pattern in $compositionRootPatterns) {
        if ($RelativePath -match $pattern) { return $true }
    }
    return $false
}

# ---- Per-file rule checks ----
foreach ($f in $goFiles) {
    $scannedFiles++
    $relPath = Get-RelativePath -FullPath $f.FullName
    $content = Get-Content -Path $f.FullName -Raw -ErrorAction SilentlyContinue
    if (-not $content) { continue }
    $imports = Get-GoImports -FileContent $content
    $isTest = Test-GoTestFile -FileName $f.Name
    $isCompRoot = Test-IsCompositionRoot -RelativePath $relPath

    # Rule 1: core/ — no byte I/O, no frameworks, no hardware, no adapters/usecase
    if ($relPath -match '[\\/]core[\\/]') {
        $forbidden = @(
            @{ Pattern = '^os$';           Msg = 'core/ no file I/O (format structs OK, byte I/O goes to adapters)' },
            @{ Pattern = '^io$';            Msg = 'core/ no I/O' },
            @{ Pattern = '^io/fs$';         Msg = 'core/ no filesystem ops' },
            @{ Pattern = '^net$';           Msg = 'core/ no network I/O' },
            @{ Pattern = '^net/http$';      Msg = 'core/ no HTTP' },
            @{ Pattern = '^encoding/csv$';  Msg = 'core/ no CSV byte writer; define CsvSchema struct only' },
            @{ Pattern = '^encoding/json$'; Msg = 'core/ no JSON serialization I/O; structs only' },
            @{ Pattern = '^encoding/xml$';  Msg = 'core/ no XML serialization I/O' },
            @{ Pattern = 'github\.com/wailsapp';        Msg = 'core/ no UI framework' },
            @{ Pattern = 'github\.com/tarm/serial';     Msg = 'core/ no hardware driver' },
            @{ Pattern = 'github\.com/jacobsa/go-serial'; Msg = 'core/ no hardware driver' },
            @{ Pattern = '/adapters/';      Msg = 'core/ must not depend on adapters' },
            @{ Pattern = '/usecase/';       Msg = 'core/ must not depend on usecase' }
        )
        foreach ($imp in $imports) {
            foreach ($rule in $forbidden) {
                if ($imp -match $rule.Pattern) {
                    $errors.Add("CORE:    $relPath -> $imp  ($($rule.Msg))")
                }
            }
        }
    }

    # Rule 2: ports/ — interface definitions only
    if ($relPath -match '[\\/]ports[\\/]' -and -not $isTest) {
        $forbidden = @(
            @{ Pattern = '^os$';  Msg = 'ports/ interfaces only' },
            @{ Pattern = '^io$';  Msg = 'ports/ interfaces only' },
            @{ Pattern = '^net$'; Msg = 'ports/ interfaces only' },
            @{ Pattern = 'github\.com/tarm/serial'; Msg = 'ports/ no hardware driver' },
            @{ Pattern = 'github\.com/wailsapp';    Msg = 'ports/ no UI framework' },
            @{ Pattern = '/adapters/';              Msg = 'ports/ must not depend on adapters' }
        )
        foreach ($imp in $imports) {
            foreach ($rule in $forbidden) {
                if ($imp -match $rule.Pattern) {
                    $errors.Add("PORTS:   $relPath -> $imp  ($($rule.Msg))")
                }
            }
        }
        # Detect struct method definitions (ports must be interface-only)
        if ($content -match 'func\s+\([^)]+\)\s+\w+\(') {
            $errors.Add("PORTS:   $relPath has struct method definition (ports: interfaces only)")
        }
    }

    # Rule 3: usecase/ — no adapters, no hardware, no file I/O, no UI framework
    # Composition root is exempt (it wires adapters into usecase).
    # Test files are exempt for os/adapters: tests need real I/O to verify
    # side effects (e.g. checkpoint files on disk) and may assemble adapters
    # to construct the system under test.
    if ($relPath -match '[\\/]usecase[\\/]' -and -not $isCompRoot) {
        $forbidden = @(
            @{ Pattern = '/adapters/';              Msg = 'usecase/ must not import adapters; wire in pkg/appcontext or cmd' },
            @{ Pattern = 'github\.com/tarm/serial'; Msg = 'usecase/ must use ports for hardware' },
            @{ Pattern = 'github\.com/jacobsa/go-serial'; Msg = 'usecase/ must use ports for hardware' },
            @{ Pattern = 'github\.com/wailsapp';    Msg = 'usecase/ no UI framework' },
            @{ Pattern = '^os$';                    Msg = 'usecase/ no file I/O; use ports interface' },
            @{ Pattern = '^io$';                    Msg = 'usecase/ no I/O; use ports interface' }
        )
        foreach ($imp in $imports) {
            foreach ($rule in $forbidden) {
                # Test files may use os and import adapters to assemble SUT
                if ($isTest -and ($imp -match '^os$' -or $imp -match '^io$' -or $imp -match '/adapters/')) {
                    continue
                }
                if ($imp -match $rule.Pattern) {
                    $errors.Add("USECASE: $relPath -> $imp  ($($rule.Msg))")
                }
            }
        }
    }

    # Rule 4: adapters/hardware/ — protocol translation only, no domain logic
    if ($relPath -match '[\\/]adapters[\\/]hardware[\\/]') {
        $domainKeywords = @(
            'CalculateMach', 'FiveHoleProbe', 'ThreeHoleProbe',
            'CalibrationCoefficient', 'InterpolateFlow'
        )
        foreach ($kw in $domainKeywords) {
            if ($content -match $kw) {
                $warnings.Add("ADAPTER-HW: $relPath contains domain keyword '$kw' (adapters/hardware: protocol translation only)")
            }
        }
    }

    # Rule 5: shared/algorithms/ — device-agnostic
    # Test files are exempt for os/net: real-data tests may load fixtures from disk.
    if ($relPath -match '[\\/]shared[\\/]algorithms[\\/]') {
        $forbidden = @(
            @{ Pattern = 'github\.com/tarm/serial';       Msg = 'algorithms must be device-agnostic' },
            @{ Pattern = 'github\.com/jacobsa/go-serial'; Msg = 'algorithms must be device-agnostic' },
            @{ Pattern = '/shared/device-sdk/';           Msg = 'algorithms must not depend on device-sdk' },
            @{ Pattern = '^os$';  Msg = 'algorithms: no I/O' },
            @{ Pattern = '^net$'; Msg = 'algorithms: no network' }
        )
        foreach ($imp in $imports) {
            foreach ($rule in $forbidden) {
                if ($isTest -and ($imp -match '^os$' -or $imp -match '^net$')) {
                    continue
                }
                if ($imp -match $rule.Pattern) {
                    $errors.Add("SHARED-ALGO: $relPath -> $imp  ($($rule.Msg))")
                }
            }
        }
    }

    # Rule 6: shared/ — no projects/*/internal/* imports
    if ($relPath -match '[\\/]shared[\\/]') {
        foreach ($imp in $imports) {
            if ($imp -match 'projects/.*/internal/') {
                $errors.Add("SHARED:  $relPath -> $imp  (shared/ must not depend on project internal)")
            }
        }
    }

    # Rule 7: programs/ — no projects/*/internal/* imports
    if ($relPath -match '[\\/]programs[\\/]') {
        foreach ($imp in $imports) {
            if ($imp -match 'projects/.*/internal/') {
                $errors.Add("PROGRAMS: $relPath -> $imp  (programs/ depends on shared/* only)")
            }
        }
    }
}

# ---- Output ----
if (-not $Quiet) {
    Write-Host "Import-direction scan: $scannedFiles Go files checked." -ForegroundColor Cyan
}

if ($warnings.Count -gt 0) {
    Write-Host ""
    Write-Host "Warnings (review recommended):" -ForegroundColor Yellow
    foreach ($w in $warnings) { Write-Host "  - $w" -ForegroundColor Yellow }
}

if ($errors.Count -gt 0) {
    Write-Host ""
    Write-Host "Import-direction check FAILED ($($errors.Count) violation(s)):" -ForegroundColor Red
    foreach ($e in $errors) { Write-Host "  - $e" -ForegroundColor Red }
    Write-Host ""
    Write-Host "See CLAUDE.md 'Hard Constraints' + 'Constraint Clarifications'." -ForegroundColor Cyan
    exit 1
}

if (-not $Quiet) {
    Write-Host "Import-direction check passed." -ForegroundColor Green
}
exit 0
