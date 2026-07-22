# Fixture test for the demo import guard (R-1) in validate-frontend-structure.ps1
# Builds minimal projects in a temp dir and asserts validator exit codes.
# Scenarios:
#   A) production file imports a DEMO ONLY marked module   -> expect exit 1
#   B) production file imports a simulate* basename module -> expect exit 1
#   C) test file imports the same demo module              -> expect exit 0
# ASCII-only comments to avoid PS 5.1 encoding issues on zh-CN systems.
param()

$ErrorActionPreference = 'Stop'
$validator = Join-Path $PSScriptRoot 'validate-frontend-structure.ps1'
$tempRoot = Join-Path ([System.IO.Path]::GetTempPath()) ("demo-import-fixture-" + [System.IO.Path]::GetRandomFileName())

function WriteFile($path, $content) {
  $dir = Split-Path $path -Parent
  if (-not (Test-Path $dir)) { New-Item -ItemType Directory -Path $dir -Force | Out-Null }
  Set-Content -Path $path -Value $content -Encoding ASCII
}

function InvokeValidator($projectDir) {
  $output = & powershell.exe -NoProfile -ExecutionPolicy Bypass -File $validator -ProjectDir $projectDir 2>&1 | Out-String
  return @{ ExitCode = $LASTEXITCODE; Output = $output }
}

$results = @()
try {
  # --- Scenario A: production file imports a DEMO ONLY module -> expect exit 1
  $srcA = Join-Path $tempRoot 'case-violation-marker\src'
  WriteFile (Join-Path $srcA 'main.ts') "import { runConsumer } from './consumer'`nrunConsumer()"
  WriteFile (Join-Path $srcA 'App.vue') "<template><div /></template>"
  WriteFile (Join-Path $srcA 'demo\demoTraversal.ts') "// DEMO ONLY - not for production use`nexport function demoRun(): number { return 1 }"
  WriteFile (Join-Path $srcA 'consumer.ts') "import { demoRun } from './demo/demoTraversal'`nexport function runConsumer(): number { return demoRun() }"
  $a = InvokeValidator $srcA
  $aPass = ($a.ExitCode -eq 1) -and ($a.Output -match 'demo-only')
  $results += @{ Name = 'violation: production imports DEMO ONLY module'; Pass = $aPass; Detail = "exit=$($a.ExitCode)" }

  # --- Scenario B: production file imports a simulate* module -> expect exit 1
  $srcB = Join-Path $tempRoot 'case-violation-simulate\src'
  WriteFile (Join-Path $srcB 'main.ts') "import { x } from './consumer'`nexport const y = x"
  WriteFile (Join-Path $srcB 'App.vue') "<template><div /></template>"
  WriteFile (Join-Path $srcB 'demo\simulateRun.ts') "export function fakeRun(): number { return 42 }"
  WriteFile (Join-Path $srcB 'consumer.ts') "import { fakeRun } from './demo/simulateRun'`nexport const x = fakeRun()"
  $b = InvokeValidator $srcB
  $bPass = ($b.ExitCode -eq 1) -and ($b.Output -match 'simulate\*')
  $results += @{ Name = 'violation: production imports simulate* module'; Pass = $bPass; Detail = "exit=$($b.ExitCode)" }

  # --- Scenario C: test file imports the same demo module -> expect exit 0
  $srcC = Join-Path $tempRoot 'case-compliant-test\src'
  WriteFile (Join-Path $srcC 'main.ts') "export const ready = true"
  WriteFile (Join-Path $srcC 'App.vue') "<template><div /></template>"
  WriteFile (Join-Path $srcC 'demo\demoTraversal.ts') "// DEMO ONLY - not for production use`nexport function demoRun(): number { return 1 }"
  WriteFile (Join-Path $srcC '__tests__\demoTraversal.test.ts') "import { demoRun } from '../demo/demoTraversal'`nexport const t = demoRun()"
  $c = InvokeValidator $srcC
  $cPass = ($c.ExitCode -eq 0)
  $results += @{ Name = 'compliant: test file imports demo module'; Pass = $cPass; Detail = "exit=$($c.ExitCode)" }
} finally {
  if (Test-Path $tempRoot) { Remove-Item $tempRoot -Recurse -Force -ErrorAction SilentlyContinue }
}

$failed = @($results | Where-Object { -not $_.Pass })
foreach ($r in $results) {
  $mark = if ($r.Pass) { 'PASS' } else { 'FAIL' }
  Write-Host "[$mark] $($r.Name) ($($r.Detail))"
}
if ($failed.Count -gt 0) {
  Write-Host "`nFIXTURE RESULT: FAIL ($($failed.Count) of $($results.Count) scenarios failed)" -ForegroundColor Red
  exit 1
}
Write-Host "`nFIXTURE RESULT: PASS ($($results.Count) scenarios)" -ForegroundColor Green
exit 0
