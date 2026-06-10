param(
  [string]$ProjectDir = ""
)

if (-not $ProjectDir) {
  Write-Host "Usage: validate-frontend-structure.ps1 -ProjectDir <frontend/src>"
  Write-Host "Example: validate-frontend-structure.ps1 -ProjectDir projects/wind-daq/apps/desktop-wails/frontend/src"
  exit 1
}

$src = Resolve-Path $ProjectDir -ErrorAction SilentlyContinue
if (-not $src) {
  Write-Host "ERROR: Path not found: $ProjectDir" -ForegroundColor Red
  exit 1
}

$issues = @()
$warnings = @()
$info = @()

# Detect which pattern is used
function HasContent($path) {
  return (Test-Path $path) -and ((Get-ChildItem $path -Force -ErrorAction SilentlyContinue).Count -gt 0)
}

$hasPages = HasContent "$src/pages"
$hasViews = HasContent "$src/views"
$hasUiDir = HasContent "$src/components/ui"
$hasLayoutDir = HasContent "$src/components/layout"
$hasStores = HasContent "$src/stores"
$hasTokens = HasContent "$src/styles/tokens"
$hasApi = HasContent "$src/api"
$hasBridge = HasContent "$src/bridge"
$hasMain = Test-Path "$src/main.ts"
$hasApp = Test-Path "$src/App.vue"

# Required checks (skip for tiny projects)
$componentDirs = @(Get-ChildItem "$src/components" -Directory -ErrorAction SilentlyContinue | Where-Object { $_.Name -ne 'ui' -and $_.Name -ne 'layout' -and $_.Name -ne 'feedback' -and $_.Name -ne 'icons' })
$hasDomainComponents = $componentDirs.Count -gt 0

# For projects with domain components or stores, check required directories
$hasRealApp = $hasPages -or $hasViews -or $hasDomainComponents -or $hasStores -or $hasApi
if ($hasRealApp) {
  if (-not $hasPages -and -not $hasViews) {
    $issues += "Missing page directory: use 'pages/' or 'views/'"
  }
  if ($hasPages -and $hasViews) {
    $issues += "Both 'pages/' and 'views/' exist. Choose one."
  }
}

if ($hasDomainComponents) {
  if (-not $hasUiDir) {
    $warnings += "Missing 'components/ui/' directory for base UI primitives"
  }
  if (-not $hasStores) {
    $warnings += "Missing 'stores/' directory for Pinia state"
  }
  if (-not $hasTokens) {
    $warnings += "Missing 'styles/tokens/' directory for design tokens"
  }
}

# Check for Ui* components outside components/ui/
$uiComponentsOutside = @(Get-ChildItem "$src/components" -Recurse -Filter "Ui*.vue" -ErrorAction SilentlyContinue `
  | Where-Object { $_.Directory.Name -ne 'ui' -and $_.Directory.Name -ne 'feedback' })
if ($uiComponentsOutside) {
  $warnings += "Ui* components found outside components/ui/:"
  foreach ($f in $uiComponentsOutside) {
    $rel = $f.FullName.Substring($src.Length + 1)
    $warnings += "  - $rel"
  }
}

# Check api/ and bridge/ conflict
if ($hasApi -and $hasBridge) {
  $issues += "Both 'api/' and 'bridge/' exist. Choose one API facade layer."
}

# Check main.ts and App.vue
if (-not $hasMain) {
  $issues += "Missing 'main.ts' application entry point"
}
if (-not $hasApp) {
  $issues += "Missing 'App.vue' root component"
}

# Check token file naming when tokens exist
if ($hasTokens) {
  $tokenFiles = @(Get-ChildItem "$src/styles/tokens" -Filter "*.css" -ErrorAction SilentlyContinue)
  if ($tokenFiles.Count -eq 0) {
    $warnings += "'styles/tokens/' exists but no CSS token files found"
  }
}

# Heuristic: Ui* components in components/ui/ should not import business stores
$uiDir = "$src/components/ui"
if (Test-Path $uiDir) {
  $uiStoreImport = @(Select-String -Path "$uiDir/*.vue" -Pattern "from ['`"]\.\./\.\./stores/" -ErrorAction SilentlyContinue)
  if ($uiStoreImport) {
    $warnings += "Ui* component(s) in components/ui/ import from stores/ (violates no-business-store dependency rule):"
    foreach ($m in $uiStoreImport) {
      $rel = $m.Path.Substring($src.Length + 1)
      $warnings += "  - $rel"
    }
  }
  $businessKeywords = @("device", "calibration", "traversal", "acquisition", "设备", "采集", "校准", "遍历")
  foreach ($kw in $businessKeywords) {
    $matches = @(Select-String -Path "$uiDir/*.vue" -Pattern "\b$kw\b" -ErrorAction SilentlyContinue)
    if ($matches) {
      $warnings += "'components/ui/' files contain business keyword '$kw' (violates no-business-term rule):"
      foreach ($m in $matches) {
        $rel = $m.Path.Substring($src.Length + 1)
        $warnings += "  - $rel"
      }
    }
  }
}

# Heuristic: stores should not directly import Wails runtime
$storesDir = "$src/stores"
if (Test-Path $storesDir) {
  $wailsImport = @(Select-String -Path "$storesDir/*.ts" -Pattern "wailsjs/runtime|@wailsapp/runtime" -ErrorAction SilentlyContinue)
  if ($wailsImport) {
    $issues += "stores/ must not directly import Wails runtime (use api/bridge instead):"
    foreach ($m in $wailsImport) {
      $rel = $m.Path.Substring($src.Length + 1)
      $issues += "  - $rel"
    }
  }
}

# Report
Write-Host "`n=== Frontend Structure Validation ===" -ForegroundColor Cyan
Write-Host "Target: $src`n"

if ($issues.Count -eq 0 -and $warnings.Count -eq 0) {
  Write-Host "✓ All checks passed" -ForegroundColor Green
  exit 0
}

if ($issues.Count -gt 0) {
  Write-Host "ISSUES (must fix):" -ForegroundColor Red
  foreach ($i in $issues) { Write-Host "  • $i" -ForegroundColor Red }
}

if ($warnings.Count -gt 0) {
  Write-Host "WARNINGS (recommended):" -ForegroundColor Yellow
  foreach ($w in $warnings) { Write-Host "  • $w" -ForegroundColor Yellow }
}

if ($issues.Count -gt 0) {
  exit 1
}
exit 0
