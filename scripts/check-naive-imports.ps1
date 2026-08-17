param(
  [string]$ProjectDir = "projects/windlabx4/apps/desktop-wails/frontend/src"
)

$root = Join-Path (Split-Path $PSScriptRoot -Parent) $ProjectDir
$uiDir = Join-Path $root "components/ui"
$spikesDir = Join-Path $root "spikes"
$hasError = $false

# Whitelist:
# - App.vue: type-only import for theme overrides
# - spikes/: experimental code
# - Known files using NSlider/NSpace/NResult without Ui* wrapper
$exceptions = @(
  (Join-Path $root "App.vue"),
  (Join-Path $root "spikes"),
  (Join-Path $root "components\layout\GlobalSettingsModal.vue"),
  (Join-Path $root "components\traversal\ProbeReferenceCard.vue"),
  (Join-Path $root "components\traversal\TraversalHardwareStep.vue"),
  (Join-Path $root "components\traversal\TraversalLayoutStep.vue")
)
Get-ChildItem -Recurse -Filter "*.vue" $root | Where-Object {
  $path = $_.FullName
  $path -notlike "$uiDir\*" -and -not ($exceptions | Where-Object { $path -like "$_*" })
} | ForEach-Object {
  $content = Get-Content $_.FullName -Raw
  if ($content -match "from\s+['""]naive-ui['""]") {
    Write-Host "ERROR: $($_.FullName.Replace($root,'')) imports naive-ui directly" -ForegroundColor Red
    $hasError = $true
  }
}

if ($hasError) {
  Write-Host "`nFeature files must use Ui* wrappers, not direct naive-ui imports." -ForegroundColor Red
  exit 1
}

Write-Host "OK: no direct naive-ui imports in feature files." -ForegroundColor Green
exit 0
