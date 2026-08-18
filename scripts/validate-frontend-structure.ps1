param(
  [string]$ProjectDir = "",
  [switch]$CheckFileSize
)

if (-not $ProjectDir) {
  Write-Host "Usage: validate-frontend-structure.ps1 -ProjectDir <frontend/src> [-CheckFileSize]"
  Write-Host "Example: validate-frontend-structure.ps1 -ProjectDir projects/windlabx4/apps/desktop-wails/frontend/src -CheckFileSize"
  exit 1
}

$src = Resolve-Path $ProjectDir -ErrorAction SilentlyContinue
if (-not $src) {
  Write-Host "ERROR: Path not found: $ProjectDir" -ForegroundColor Red
  exit 1
}
# Coerce PathInfo to string so .Length works correctly in PS 5.1 (PathInfo.Length is not the path length)
$src = $src.Path

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
  # Business keywords kept as ASCII-only to avoid PS 5.1 encoding issues on zh-CN systems
  $businessKeywords = @("device", "calibration", "traversal", "acquisition")
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

# ============================================================
# Demo import guard (R-1)
# Production source files must not import demo-only modules. A module
# counts as demo-only when its file head (first 5 lines) contains the
# 'DEMO ONLY' marker, or its basename matches simulate*. Test files,
# mock directories, and files that are themselves demo-only are exempt.
# Note: comments in English to avoid PS 5.1 encoding issues on zh-CN systems
# ============================================================
function ResolveImportTarget($importerDir, $spec) {
  # Resolve relative paths and common '@' aliases to a file on disk.
  # Returns $null for bare package imports and unresolvable specifiers.
  $base = $null
  if ($spec.StartsWith('.')) {
    $base = Join-Path $importerDir $spec
  } elseif ($spec -match '^@/(.+)$') {
    $base = Join-Path $src $matches[1]
  } elseif ($spec -match '^@([\w-]+)/(.+)$') {
    # Heuristic: '@components/x' style aliases map to src/<alias>/x
    $base = Join-Path $src (Join-Path $matches[1] $matches[2])
  } else {
    return $null
  }
  $candidates = @($base, "$base.ts", "$base.vue", "$base.js", "$base.d.ts", "$base/index.ts", "$base/index.vue", "$base/index.js")
  foreach ($c in $candidates) {
    if (Test-Path $c) { return [System.IO.Path]::GetFullPath($c) }
  }
  return $null
}

function GetDemoOnlyReason($targetPath) {
  # Returns a reason string when the target file is demo-only, otherwise $null
  $targetName = [System.IO.Path]::GetFileName($targetPath)
  if ($targetName -like 'simulate*') { return "target file name matches simulate*" }
  $head = @(Get-Content $targetPath -TotalCount 5 -ErrorAction SilentlyContinue)
  foreach ($line in $head) {
    if ($line -match 'DEMO ONLY') { return "target file head contains DEMO ONLY marker" }
  }
  return $null
}

$allSourceFiles = @(Get-ChildItem $src -Recurse -Include "*.ts", "*.vue" -ErrorAction SilentlyContinue |
  Where-Object { $_.FullName -notlike '*\node_modules\*' -and $_.FullName -notlike '*\bindings\*' })

$demoImportHits = @()
foreach ($f in $allSourceFiles) {
  $rel = $f.FullName.Substring($src.Length + 1)

  # Exempt importers: tests, mock directories, and self-marked demo files
  if ($rel -match '__tests__') { continue }
  if ($f.Name -match '\.(test|spec)\.') { continue }
  if ($f.FullName -match '\\__mocks__\\|\\mocks?\\') { continue }
  $ownHead = @(Get-Content $f.FullName -TotalCount 5 -ErrorAction SilentlyContinue)
  $isDemoFile = $false
  foreach ($line in $ownHead) { if ($line -match 'DEMO ONLY') { $isDemoFile = $true; break } }
  if ($isDemoFile) { continue }

  $content = Get-Content $f.FullName -Raw -ErrorAction SilentlyContinue
  if (-not $content) { continue }

  # Collect import specifiers: static import/export-from, side-effect import, literal dynamic import()
  $specs = @{}
  foreach ($m in [regex]::Matches($content, "(?:import|export)\s[^'""]*?\bfrom\s*['""]([^'""]+)['""]")) {
    $specs[$m.Groups[1].Value] = $true
  }
  foreach ($m in [regex]::Matches($content, "(?:^|\n)\s*import\s*['""]([^'""]+)['""]")) {
    $specs[$m.Groups[1].Value] = $true
  }
  foreach ($m in [regex]::Matches($content, "\bimport\s*\(\s*['""]([^'""]+)['""]\s*\)")) {
    $specs[$m.Groups[1].Value] = $true
  }

  foreach ($spec in $specs.Keys) {
    $target = ResolveImportTarget $f.Directory.FullName $spec
    if (-not $target) { continue }
    $reason = GetDemoOnlyReason $target
    if ($reason) {
      if ($target.StartsWith($src, [System.StringComparison]::OrdinalIgnoreCase)) {
        $targetRel = $target.Substring($src.Length + 1)
      } else {
        $targetRel = $target
      }
      $demoImportHits += "$rel imports '$spec' -> $targetRel ($reason)"
    }
  }
}
if ($demoImportHits.Count -gt 0) {
  $issues += "Production files must not import demo-only modules (DEMO ONLY marker or simulate* file):"
  foreach ($h in $demoImportHits) { $issues += "  - $h" }
}

# ============================================================
# -CheckFileSize: quantitative checks for file size and hardcoded colors
# Implements docs/runbooks/frontend-ai-rules.zh-CN.md section 28.1 and 28.2
# Thresholds must stay in sync with the rule doc (single source of truth)
# Note: comments in English to avoid PS 5.1 encoding issues on zh-CN systems
# ============================================================
if ($CheckFileSize) {
  # Threshold constants (kept in sync with the rule doc to avoid drift)
  $VUE_TOTAL_WARN = 500
  $VUE_TOTAL_ERR = 800
  $VUE_SCRIPT_WARN = 200
  $VUE_SCRIPT_ERR = 300
  $VUE_STYLE_WARN = 200
  $VUE_STYLE_ERR = 300
  $VUE_FUNC_WARN = 10
  $VUE_FUNC_ERR = 15
  $STORE_WARN = 400
  $STORE_ERR = 600
  $I18N_WARN = 400
  $I18N_ERR = 600

  # Collect all .vue files (exclude bindings/ and node_modules/)
  $vueFiles = @(Get-ChildItem $src -Recurse -Filter "*.vue" -ErrorAction SilentlyContinue |
    Where-Object { $_.FullName -notlike '*\node_modules\*' -and $_.FullName -notlike '*\bindings\*' })

  # ---------- 1) .vue file size and hardcoded color checks ----------
  foreach ($f in $vueFiles) {
    $rel = $f.FullName.Substring($src.Length + 1)
    $lines = @(Get-Content $f.FullName -ErrorAction SilentlyContinue)
    $total = $lines.Count

    # Locate <script setup> block boundaries
    $scriptStart = -1; $scriptEnd = -1
    for ($i = 0; $i -lt $lines.Count; $i++) {
      if ($scriptStart -lt 0 -and $lines[$i] -match '<script\s+[^>]*setup[^>]*>') {
        $scriptStart = $i; continue
      }
      if ($scriptStart -ge 0 -and $lines[$i] -match '</script>') {
        $scriptEnd = $i; break
      }
    }
    $scriptLines = 0
    if ($scriptStart -ge 0 -and $scriptEnd -gt $scriptStart) {
      $scriptLines = $scriptEnd - $scriptStart - 1
    }

    # Locate <style> block boundaries (scoped or non-scoped)
    $styleStart = -1; $styleEnd = -1
    for ($i = 0; $i -lt $lines.Count; $i++) {
      if ($styleStart -lt 0 -and $lines[$i] -match '<style[^>]*>') {
        $styleStart = $i; continue
      }
      if ($styleStart -ge 0 -and $lines[$i] -match '</style>') {
        $styleEnd = $i; break
      }
    }
    $styleLines = 0
    if ($styleStart -ge 0 -and $styleEnd -gt $styleStart) {
      $styleLines = $styleEnd - $styleStart - 1
    }

    # Count local functions inside <script setup>
    # Excludes imports, store calls, and lifecycle hooks (not business logic)
    $funcCount = 0
    if ($scriptStart -ge 0 -and $scriptEnd -gt $scriptStart) {
      for ($i = $scriptStart + 1; $i -lt $scriptEnd; $i++) {
        $line = $lines[$i]
        if ($line -match '^\s*(function\s+\w+|const\s+\w+\s*=\s*(async\s*)?\(|const\s+\w+\s*=\s*function)') {
          $funcCount++
        }
      }
    }

    # Total line count check
    if ($total -gt $VUE_TOTAL_ERR) {
      $issues += "$rel total $total lines > $VUE_TOTAL_ERR (split required, 28.1)"
    } elseif ($total -gt $VUE_TOTAL_WARN) {
      $warnings += "$rel total $total lines > $VUE_TOTAL_WARN (consider split, 28.1)"
    }

    # Script block line count check
    if ($scriptLines -gt $VUE_SCRIPT_ERR) {
      $issues += "$rel <script setup> $scriptLines lines > $VUE_SCRIPT_ERR (extract composable, 28.1)"
    } elseif ($scriptLines -gt $VUE_SCRIPT_WARN) {
      $warnings += "$rel <script setup> $scriptLines lines > $VUE_SCRIPT_WARN (consider composable, 28.1)"
    }

    # Style block line count check
    if ($styleLines -gt $VUE_STYLE_ERR) {
      $issues += "$rel <style> $styleLines lines > $VUE_STYLE_ERR (extract utility/pattern, 28.1)"
    } elseif ($styleLines -gt $VUE_STYLE_WARN) {
      $warnings += "$rel <style> $styleLines lines > $VUE_STYLE_WARN (consider extract, 28.1)"
    }

    # Local function count check
    if ($funcCount -gt $VUE_FUNC_ERR) {
      $issues += "$rel <script setup> $funcCount local functions > $VUE_FUNC_ERR (extract composable, 28.1)"
    } elseif ($funcCount -gt $VUE_FUNC_WARN) {
      $warnings += "$rel <script setup> $funcCount local functions > $VUE_FUNC_WARN (consider composable, 28.1)"
    }

    # ---------- 2) Hardcoded color checks (style block only) ----------
    if ($styleStart -ge 0 -and $styleEnd -gt $styleStart) {
      $styleContent = ($lines[($styleStart + 1)..($styleEnd - 1)] -join "`n")

      # Hex color literals
      $hexMatches = [regex]::Matches($styleContent, '#[0-9a-fA-F]{3,8}\b')
      if ($hexMatches.Count -gt 0) {
        $issues += "$rel <style> has $($hexMatches.Count) hex color literal(s) (use token, 28.2)"
      }

      # rgba()/rgb() calls
      $rgbaMatches = [regex]::Matches($styleContent, '\brgba?\(')
      if ($rgbaMatches.Count -gt 0) {
        $issues += "$rel <style> has $($rgbaMatches.Count) rgba()/rgb() call(s) (use token, 28.2)"
      }

      # hsl()/hsla() calls
      $hslMatches = [regex]::Matches($styleContent, '\bhsla?\(')
      if ($hslMatches.Count -gt 0) {
        $issues += "$rel <style> has $($hslMatches.Count) hsl()/hsla() call(s) (use token, 28.2)"
      }

      # color-mix() with hex/rgba fallback
      $colorMixFallback = [regex]::Matches($styleContent, 'color-mix\([^)]*(#[0-9a-fA-F]{3,8}|rgba?\()')
      if ($colorMixFallback.Count -gt 0) {
        $issues += "$rel <style> has $($colorMixFallback.Count) color-mix() with hex/rgba fallback (use token only, 28.2)"
      }

      # var(--xxx, #yyy) fallback - token already defined, fallback is redundant
      $varFallback = [regex]::Matches($styleContent, 'var\(--[\w-]+,\s*(#[0-9a-fA-F]{3,8}|rgba?\(|hsla?\()')
      if ($varFallback.Count -gt 0) {
        $issues += "$rel <style> has $($varFallback.Count) var() with color fallback (remove fallback, 28.2)"
      }

      # Fixed blur() values - warning only (some scenarios genuinely need fixed values)
      $blurMatches = [regex]::Matches($styleContent, 'blur\(\d+px\)')
      if ($blurMatches.Count -gt 0) {
        $warnings += "$rel <style> has $($blurMatches.Count) fixed blur() value(s) (use --blur-* token, 28.2)"
      }
    }
  }

  # ---------- 3) store / i18n file line count checks ----------
  $storeFiles = @(Get-ChildItem "$src/stores" -Filter "*.ts" -ErrorAction SilentlyContinue)
  foreach ($f in $storeFiles) {
    $rel = $f.FullName.Substring($src.Length + 1)
    $lineCount = @(Get-Content $f.FullName).Count

    # i18n / dictionary files use separate thresholds
    $isI18n = $f.Name -match 'i18n|locale|dict'
    $warnThreshold = $STORE_WARN
    $errThreshold = $STORE_ERR
    if ($isI18n) {
      $warnThreshold = $I18N_WARN
      $errThreshold = $I18N_ERR
    }

    if ($lineCount -gt $errThreshold) {
      $issues += "$rel $lineCount lines > $errThreshold (split required, 28.1)"
    } elseif ($lineCount -gt $warnThreshold) {
      $warnings += "$rel $lineCount lines > $warnThreshold (consider split, 28.1)"
    }
  }

  # ---------- 4) Missing composables/ directory check ----------
  # Triggered when: store >= 4 OR domain components >= 6
  # Required: src/composables/ OR src/components/<domain>/composables/
  $storeCount = $storeFiles.Count
  $domainComponentFiles = @(Get-ChildItem "$src/components" -Recurse -Filter "*.vue" -ErrorAction SilentlyContinue |
    Where-Object {
      $_.Directory.Parent.Name -eq 'components' -and
      $_.Directory.Name -ne 'ui' -and
      $_.Directory.Name -ne 'layout' -and
      $_.Directory.Name -ne 'feedback' -and
      $_.Directory.Name -ne 'icons' -and
      $_.Directory.Name -ne 'patterns'
    })
  $domainCount = $domainComponentFiles.Count

  $hasComposables = Test-Path "$src/composables"
  $hasDomainComposables = @(Get-ChildItem "$src/components" -Recurse -Directory -Filter "composables" -ErrorAction SilentlyContinue).Count -gt 0

  if (($storeCount -ge 4 -or $domainCount -ge 6) -and -not $hasComposables -and -not $hasDomainComposables) {
    $issues += "Project has $storeCount stores and $domainCount domain components but no composables/ directory (required when store>=4 or domain>=6, see composables section in frontend-directory-rules.zh-CN.md)"
  }

  # ---------- 5) Cross-component duplicate CSS class check ----------
  # Same class name defined in >=2 .vue <style> blocks must be extracted to styles/utilities/
  $classFileMap = @{}
  foreach ($f in $vueFiles) {
    $rel = $f.FullName.Substring($src.Length + 1)
    $lines = @(Get-Content $f.FullName -ErrorAction SilentlyContinue)

    # Find style block
    $styleStart = -1; $styleEnd = -1
    for ($i = 0; $i -lt $lines.Count; $i++) {
      if ($styleStart -lt 0 -and $lines[$i] -match '<style[^>]*>') {
        $styleStart = $i; continue
      }
      if ($styleStart -ge 0 -and $lines[$i] -match '</style>') {
        $styleEnd = $i; break
      }
    }
    if ($styleStart -lt 0) { continue }

    # Extract all .className { definitions
    for ($i = $styleStart + 1; $i -lt $styleEnd; $i++) {
      $classMatches = [regex]::Matches($lines[$i], '\.([a-zA-Z_][\w-]*)\s*\{')
      foreach ($m in $classMatches) {
        $cls = $m.Groups[1].Value
        if (-not $classFileMap.ContainsKey($cls)) {
          $classFileMap[$cls] = @()
        }
        if ($classFileMap[$cls] -notcontains $rel) {
          $classFileMap[$cls] += $rel
        }
      }
    }
  }

  # Report classes defined in >=2 .vue files
  foreach ($kv in $classFileMap.GetEnumerator()) {
    if ($kv.Value.Count -ge 2) {
      $fileList = $kv.Value -join ", "
      $warnings += "CSS class '.$($kv.Key)' defined in $($kv.Value.Count) .vue files: $fileList (extract to styles/utilities/, 28.2)"
    }
  }
}

# ============================================================
# -CheckFileSize extended: code quality checks for §18-§28
# Implements docs/runbooks/frontend-ai-rules.zh-CN.md sections 18-28 (after 2026-07-06 reorg)
# Note: comments in English to avoid PS 5.1 encoding issues on zh-CN systems
# ============================================================
if ($CheckFileSize) {
  # Collect all .ts and .vue files (exclude bindings/, node_modules/)
  $tsFiles = @(Get-ChildItem $src -Recurse -Include "*.ts", "*.vue" -ErrorAction SilentlyContinue |
    Where-Object { $_.FullName -notlike '*\node_modules\*' -and $_.FullName -notlike '*\bindings\*' })

  foreach ($f in $tsFiles) {
    $rel = $f.FullName.Substring($src.Length + 1)
    $content = Get-Content $f.FullName -Raw -ErrorAction SilentlyContinue
    if (-not $content) { continue }

    # ---------- §18 TypeScript strictness ----------
    # Detect ": any" type annotations
    $anyMatches = [regex]::Matches($content, ':\s*any\b')
    if ($anyMatches.Count -gt 0) {
      $warnings += "$rel has $($anyMatches.Count) ': any' type annotation(s) (use unknown or specific type, 18)"
    }

    # Detect "as any" assertions
    $asAnyMatches = [regex]::Matches($content, '\bas\s+any\b')
    if ($asAnyMatches.Count -gt 0) {
      $issues += "$rel has $($asAnyMatches.Count) 'as any' assertion(s) (use 'as unknown as T' with comment, 18)"
    }

    # Detect @ts-ignore (forbidden, must use @ts-expect-error)
    $tsIgnoreMatches = [regex]::Matches($content, '@ts-ignore')
    if ($tsIgnoreMatches.Count -gt 0) {
      $issues += "$rel has $($tsIgnoreMatches.Count) @ts-ignore (forbidden, use @ts-expect-error with comment, 18)"
    }

    # ---------- §19 naming conventions ----------
    # .vue files must be PascalCase (use -cmatch for case-sensitive matching in PS)
    if ($f.Extension -eq '.vue') {
      $baseName = [System.IO.Path]::GetFileNameWithoutExtension($f.Name)
      if ($baseName -cnotmatch '^[A-Z][a-zA-Z0-9]*$') {
        $issues += "$rel .vue file name not PascalCase (19)"
      }
    }

    # composable files must start with "use" (case-sensitive)
    if ($f.Directory.Name -eq 'composables' -and $f.Extension -eq '.ts') {
      $baseName = [System.IO.Path]::GetFileNameWithoutExtension($f.Name)
      if ($baseName -cnotmatch '^use[A-Z]') {
        $warnings += "$rel composable file name does not start with 'use' (19)"
      }
    }

    # ---------- §26 lifecycle & resource cleanup ----------
    # Detect addEventListener without removeEventListener in same file
    $addMatches = [regex]::Matches($content, 'addEventListener\(')
    $removeMatches = [regex]::Matches($content, 'removeEventListener\(')
    if ($addMatches.Count -gt 0 -and $removeMatches.Count -eq 0) {
      $warnings += "$rel has $($addMatches.Count) addEventListener but no removeEventListener (check cleanup, 26)"
    }

    # Detect setInterval without clearInterval
    $setIntervalMatches = [regex]::Matches($content, 'setInterval\(')
    $clearIntervalMatches = [regex]::Matches($content, 'clearInterval\(')
    if ($setIntervalMatches.Count -gt 0 -and $clearIntervalMatches.Count -eq 0) {
      $warnings += "$rel has $($setIntervalMatches.Count) setInterval but no clearInterval (check cleanup, 26)"
    }

    # Detect setTimeout (>1s) without clearTimeout - heuristic, may have false positives
    # Only flag if there are many setTimeout calls and zero clearTimeout
    $setTimeoutMatches = [regex]::Matches($content, 'setTimeout\(')
    $clearTimeoutMatches = [regex]::Matches($content, 'clearTimeout\(')
    if ($setTimeoutMatches.Count -ge 5 -and $clearTimeoutMatches.Count -eq 0) {
      $warnings += "$rel has $($setTimeoutMatches.Count) setTimeout but no clearTimeout (check long timers cleanup, 26)"
    }

    # Detect Events.On without off (Wails event subscription cleanup)
    $eventsOnMatches = [regex]::Matches($content, 'Events\.On\(')
    $eventsOffMatches = [regex]::Matches($content, 'Events\.Off\(')
    if ($eventsOnMatches.Count -gt 0 -and $eventsOffMatches.Count -eq 0) {
      $warnings += "$rel has $($eventsOnMatches.Count) Events.On but no Events.Off (check Wails event cleanup, 26)"
    }

    # ---------- §22 error handling ----------
    # Detect empty catch blocks: catch (e) {} or catch { }
    $emptyCatchMatches = [regex]::Matches($content, 'catch\s*(\([^)]*\))?\s*\{\s*\}')
    if ($emptyCatchMatches.Count -gt 0) {
      $issues += "$rel has $($emptyCatchMatches.Count) empty catch block(s) (22)"
    }

    # Detect catch (e: any) - should use unknown
    $catchAnyMatches = [regex]::Matches($content, 'catch\s*\(\s*\w+\s*:\s*any\s*\)')
    if ($catchAnyMatches.Count -gt 0) {
      $issues += "$rel has $($catchAnyMatches.Count) catch(e: any) (use unknown, 22)"
    }

    # Detect throw new Error with empty or generic message
    $throwGenericMatches = [regex]::Matches($content, "throw\s+new\s+Error\(\s*['""]([^'""]{0,5}|error|fail|failed|exception)['""]?\s*\)")
    if ($throwGenericMatches.Count -gt 0) {
      $warnings += "$rel has $($throwGenericMatches.Count) throw new Error with generic/short message (add context, 22)"
    }

    # ---------- §12 reactivity ----------
    # Detect reactive array (anti-pattern for arrays)
    $reactiveArrayMatches = [regex]::Matches($content, 'reactive\s*<\s*\w+\[\]\s*>')
    if ($reactiveArrayMatches.Count -gt 0) {
      $issues += "$rel has $($reactiveArrayMatches.Count) reactive<T[]>(use ref<T[]> instead, 12)"
    }

    # ---------- §24 performance ----------
    # Detect v-for without :key (in .vue files)
    if ($f.Extension -eq '.vue') {
      # Count v-for and :key occurrences (heuristic, may have false positives for nested v-for)
      $vforCount = ([regex]::Matches($content, '\bv-for\b')).Count
      $keyCount = ([regex]::Matches($content, ':key\b')).Count
      if ($vforCount -gt 0 -and $keyCount -lt $vforCount) {
        $issues += "$rel has $vforCount v-for but only $keyCount :key (v-for must have :key, 24)"
      }

      # Detect v-if and v-for on same element
      $vifForMatches = [regex]::Matches($content, '<[^>]*\bv-for\b[^>]*\bv-if\b[^>]*>')
      $vifForMatches2 = [regex]::Matches($content, '<[^>]*\bv-if\b[^>]*\bv-for\b[^>]*>')
      $vifForTotal = $vifForMatches.Count + $vifForMatches2.Count
      if ($vifForTotal -gt 0) {
        $issues += "$rel has $vifForTotal element(s) with both v-if and v-for (split them, 24)"
      }
    }

    # ---------- §25 i18n ----------
    # Detect hardcoded Chinese in template (between > and <)
    if ($f.Extension -eq '.vue') {
      # Match Chinese chars in template text content (between tags)
      $chineseInTemplate = [regex]::Matches($content, '>[^<]*[\u4e00-\u9fff][^<]*<')
      # Exclude comments
      $chineseInTemplateFiltered = 0
      foreach ($m in $chineseInTemplate) {
        if ($m.Value -notmatch '<!--' -and $m.Value -notmatch '-->') {
          $chineseInTemplateFiltered++
        }
      }
      if ($chineseInTemplateFiltered -gt 5) {
        $warnings += "$rel has ~$chineseInTemplateFiltered hardcoded Chinese text in template (use i18n, 25)"
      }
    }

    # ---------- §13 Provide/Inject ----------
    # Detect inject('xxx') with string literal (no InjectionKey)
    $injectStringMatches = [regex]::Matches($content, "inject\(\s*['""]")
    if ($injectStringMatches.Count -gt 0) {
      $issues += "$rel has $($injectStringMatches.Count) inject with string key (use InjectionKey<T>, 13)"
    }
  }

  # ---------- §14 Pinia store dependencies ----------
  # Check store files for cross-store direct state mutation
  $storeFiles2 = @(Get-ChildItem "$src/stores" -Filter "*.ts" -ErrorAction SilentlyContinue)
  foreach ($f in $storeFiles2) {
    $rel = $f.FullName.Substring($src.Length + 1)
    $content = Get-Content $f.FullName -Raw -ErrorAction SilentlyContinue
    if (-not $content) { continue }

    # Detect options-style defineStore (forbidden, must use setup style)
    $optionsStoreMatches = [regex]::Matches($content, 'defineStore\(\s*[''""][^''""]+[''""]\s*,\s*\{')
    if ($optionsStoreMatches.Count -gt 0) {
      $issues += "$rel uses options-style defineStore (use setup style, 14)"
    }
  }

  # ---------- §20 comment tags format ----------
  # Detect TODO/FIXME without owner: must be TODO(owner) format
  foreach ($f in $tsFiles) {
    $rel = $f.FullName.Substring($src.Length + 1)
    $content = Get-Content $f.FullName -Raw -ErrorAction SilentlyContinue
    if (-not $content) { continue }

    # TODO/FIXME without (owner): // TODO: or // FIXME: without parens
    $todoNoOwner = [regex]::Matches($content, '//\s*(TODO|FIXME)\s*:[^(''"]')
    if ($todoNoOwner.Count -gt 0) {
      $warnings += "$rel has $($todoNoOwner.Count) TODO/FIXME without owner (use TODO(owner): format, 20)"
    }

    # ---------- §21 function complexity (heuristic) ----------
    # Detect functions with too many parameters (>6 is error threshold)
    # Match: function name(a, b, c, d, e, f, g) - count commas in signature
    $funcSignatures = [regex]::Matches($content, '(?:function\s+\w+|const\s+\w+\s*=\s*(?:async\s*)?\(|\w+\s*:\s*(?:async\s*)?\()\s*([^)]*)')
    foreach ($sig in $funcSignatures) {
      $params = $sig.Groups[1].Value
      # Count top-level commas (rough heuristic, may have false positives for nested generics)
      $paramCount = ($params -split ',').Count
      if ($paramCount -gt 6) {
        $warnings += "$rel has function with $paramCount parameters (>6, use options object, 21)"
      }
    }

    # ---------- §23 defensive programming ----------
    # Detect non-null assertion operator (!) - forbidden except tests
    if ($rel -notlike '*test*' -and $rel -notlike '*spec*' -and $rel -notlike '*__tests__*') {
      $nonNullAssert = [regex]::Matches($content, '\.\w+!\.')
      # Filter out type-only ! (like Map!.get) - this is a rough heuristic
      if ($nonNullAssert.Count -gt 3) {
        $warnings += "$rel has $($nonNullAssert.Count) non-null assertion(s) '!' (use explicit null check, 23)"
      }
    }

    # Detect JSON.parse without try/catch (rough heuristic: count parse vs try)
    $jsonParseCount = ([regex]::Matches($content, 'JSON\.parse\(')).Count
    if ($jsonParseCount -gt 0) {
      # Check if file has any try block (very rough)
      $tryCount = ([regex]::Matches($content, '\btry\s*\{')).Count
      if ($tryCount -lt $jsonParseCount) {
        $warnings += "$rel has $jsonParseCount JSON.parse but only $tryCount try blocks (wrap in try/catch, 23)"
      }
    }

    # ---------- §9 module boundaries ----------
    # Detect cross-domain imports (components/<domainA>/ imports components/<domainB>/)
    if ($f.FullName -match '\\components\\([^\\]+)\\') {
      $currentDomain = $matches[1]
      if ($currentDomain -ne 'ui' -and $currentDomain -ne 'layout' -and $currentDomain -ne 'feedback' -and $currentDomain -ne 'icons' -and $currentDomain -ne 'patterns') {
        $crossDomainMatches = [regex]::Matches($content, "from\s+['""](@components|\.\.)/([^/'""]+)/")
        foreach ($m in $crossDomainMatches) {
          $targetDomain = $m.Groups[2].Value
          if ($targetDomain -ne $currentDomain -and $targetDomain -ne 'ui' -and $targetDomain -ne 'layout' -and $targetDomain -ne 'feedback' -and $targetDomain -ne 'icons' -and $targetDomain -ne 'patterns') {
            $issues += "$rel (domain: $currentDomain) imports from domain '$targetDomain' (cross-domain import forbidden, 9)"
          }
        }
      }
    }

    # ---------- §35 AI-friendly naming ----------
    # Detect generic variable names (data, info, manager, temp, obj, item) - warning only
    $genericNames = [regex]::Matches($content, '\b(const|let|var)\s+(data|info|manager|temp|obj|item|value|result)\b')
    if ($genericNames.Count -gt 5) {
      $warnings += "$rel has $($genericNames.Count) generic variable name(s) (data/info/manager/temp/obj/item - use specific name, 35)"
    }
  }
}

# Report
Write-Host "`n=== Frontend Structure Validation ===" -ForegroundColor Cyan
Write-Host "Target: $src`n"

if ($issues.Count -eq 0 -and $warnings.Count -eq 0) {
  Write-Host "OK - All checks passed" -ForegroundColor Green
  exit 0
}

if ($issues.Count -gt 0) {
  Write-Host "ISSUES (must fix):" -ForegroundColor Red
  foreach ($i in $issues) { Write-Host "  - $i" -ForegroundColor Red }
}

if ($warnings.Count -gt 0) {
  Write-Host "WARNINGS (recommended):" -ForegroundColor Yellow
  foreach ($w in $warnings) { Write-Host "  - $w" -ForegroundColor Yellow }
}

if ($issues.Count -gt 0) {
  exit 1
}
exit 0
