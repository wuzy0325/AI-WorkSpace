param(
  [Parameter(Mandatory=$true)]
  [string]$Path,
  [switch]$Recurse,
  [int]$Budget = 0
)

# ============================================================
# AI context token estimation script
# Estimates token count for .md / .ts / .vue / .go files
# Formula: token ~= chars * 0.5 (mixed CN/EN average)
#   - Chinese: 1 char ~= 0.6 token
#   - English: 1 word ~= 1.3 token
#   - Code:    1 line ~= 5-10 token
#   - Mixed:   chars * 0.5, error within +-15%
#
# Usage:
#   powershell -File scripts/estimate-token.ps1 -Path AGENTS.md
#   powershell -File scripts/estimate-token.ps1 -Path docs/runbooks -Recurse
#   powershell -File scripts/estimate-token.ps1 -Path AGENTS.md -Budget 2000
#
# Exit codes: 0=all OK, 1=over budget, 2=invalid path
# ============================================================

if (-not (Test-Path $Path)) {
  Write-Host "ERROR: Path not found: $Path" -ForegroundColor Red
  exit 2
}

# Budget lookup (aligned with ai-context-loading.zh-CN.md section 3)
function Get-FileBudget([string]$fileName) {
  if ($fileName -eq 'AGENTS.md') { return 2000 }
  if ($fileName -eq 'CLAUDE.md') { return 4000 }
  if ($fileName -eq 'workspace-engineering-rules.zh-CN.md') { return 2000 }
  if ($fileName -eq 'code-standards.zh-CN.md') { return 8000 }
  if ($fileName -eq 'frontend-ai-rules.zh-CN.md') { return 8000 }
  if ($fileName -eq 'frontend-ai-rules-foundation.zh-CN.md') { return 8000 }
  if ($fileName -eq 'frontend-ai-rules-state.zh-CN.md') { return 8000 }
  if ($fileName -eq 'frontend-ai-rules-quality.zh-CN.md') { return 8000 }
  if ($fileName -eq 'frontend-ai-rules-deploy.zh-CN.md') { return 8000 }
  if ($fileName -eq 'frontend-directory-rules.zh-CN.md') { return 8000 }
  if ($fileName -eq 'development-rules.md') { return 8000 }
  if ($fileName -eq 'workspace-directory-rules.zh-CN.md') { return 8000 }
  if ($fileName -eq 'module-design.md') { return 8000 }
  if ($fileName -eq 'project-variants.md') { return 8000 }
  if ($fileName -eq 'ai-context-loading.zh-CN.md') { return 8000 }
  if ($fileName -eq 'ai-document-responsibility-matrix.zh-CN.md') { return 8000 }
  if ($fileName -eq 'ai-task-context-map.zh-CN.md') { return 8000 }
  if ($fileName -eq 'release-versioning.zh-CN.md') { return 8000 }
  return 0
}

# Collect target files
$files = @()
if (Test-Path $Path -PathType Leaf) {
  $files += Get-Item $Path
} else {
  if ($Recurse) {
    $files = @(Get-ChildItem $Path -Recurse -File -ErrorAction SilentlyContinue | Where-Object { ($_.Extension -eq '.md') -or ($_.Extension -eq '.ts') -or ($_.Extension -eq '.vue') -or ($_.Extension -eq '.go') -or ($_.Extension -eq '.js') -or ($_.Extension -eq '.json') -or ($_.Extension -eq '.ps1') -or ($_.Extension -eq '.yaml') -or ($_.Extension -eq '.yml') })
  } else {
    $files = @(Get-ChildItem $Path -File -ErrorAction SilentlyContinue | Where-Object { ($_.Extension -eq '.md') -or ($_.Extension -eq '.ts') -or ($_.Extension -eq '.vue') -or ($_.Extension -eq '.go') -or ($_.Extension -eq '.js') -or ($_.Extension -eq '.json') -or ($_.Extension -eq '.ps1') -or ($_.Extension -eq '.yaml') -or ($_.Extension -eq '.yml') })
  }
}

if ($files.Count -eq 0) {
  Write-Host "No supported files found in: $Path" -ForegroundColor Yellow
  exit 2
}

# Estimate tokens per file
$totalTokens = 0
$overBudgetCount = 0
$rootLen = (Get-Location).Path.Length + 1

foreach ($f in $files) {
  $content = Get-Content $f.FullName -Raw -ErrorAction SilentlyContinue
  if (-not $content) { continue }

  # Strip auto-injected tool sections (e.g. GitNexus) — counted as on-demand L3, not L1 startup budget
  $stripped = $content
  $gnStart = $stripped.IndexOf('<!-- gitnexus:start -->')
  $gnEnd = $stripped.IndexOf('<!-- gitnexus:end -->')
  if (($gnStart -ge 0) -and ($gnEnd -gt $gnStart)) {
    $stripped = $stripped.Substring(0, $gnStart) + $stripped.Substring($gnEnd + '<!-- gitnexus:end -->'.Length)
  }

  $charCount = $stripped.Length
  $estimatedTokens = [math]::Round($charCount * 0.5)

  $fileBudget = 0
  if ($Budget -gt 0) {
    $fileBudget = $Budget
  } elseif (($f.Name -eq 'AGENTS.md') -and ($f.FullName -like '*projects*')) {
    # Project-level AGENTS.md gets 3000 budget (before generic AGENTS.md=2000)
    $fileBudget = 3000
  } else {
    $fileBudget = Get-FileBudget $f.Name
  }

  $status = 'N/A'
  $overBy = 0
  if ($fileBudget -gt 0) {
    if ($estimatedTokens -gt $fileBudget) {
      $status = 'OVER'
      $overBy = $estimatedTokens - $fileBudget
      $overBudgetCount = $overBudgetCount + 1
    } else {
      $status = 'OK'
    }
  }

  $color = 'White'
  if ($status -eq 'OVER') { $color = 'Red' }
  elseif ($status -eq 'OK') { $color = 'Green' }
  elseif ($status -eq 'N/A') { $color = 'DarkGray' }

  $relPath = $f.FullName.Substring($rootLen)
  $budgetStr = 'N/A'
  if ($fileBudget -gt 0) { $budgetStr = "$fileBudget" }
  $overStr = ''
  if ($overBy -gt 0) { $overStr = " (+$overBy)" }

  $line = "{0,-60} {1,8} chars  {2,7} tokens  budget={3,-6} {4,-3}{5}" -f $relPath, $charCount, $estimatedTokens, $budgetStr, $status, $overStr
  Write-Host $line -ForegroundColor $color

  $totalTokens = $totalTokens + $estimatedTokens
}

Write-Host ""
Write-Host "Total: $($files.Count) files, $totalTokens tokens estimated" -ForegroundColor Cyan

if ($overBudgetCount -gt 0) {
  Write-Host "$overBudgetCount file(s) OVER budget" -ForegroundColor Red
  exit 1
} else {
  Write-Host "All files within budget" -ForegroundColor Green
  exit 0
}
