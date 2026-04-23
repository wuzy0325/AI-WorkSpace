[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)]
    [ValidatePattern("^[a-z][a-z0-9-]+$")]
    [string]$Name
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

$workspaceRoot = (Resolve-Path (Join-Path $PSScriptRoot ".." )).Path
$projectRoot = Join-Path $workspaceRoot (Join-Path "projects" $Name)

if (Test-Path -Path $projectRoot) {
    Write-Error "Project already exists: projects/$Name"
    exit 1
}

$directories = @(
    "apps/desktop-wails/frontend",
    "apps/desktop-wails/backend",
    "services/api-go/cmd",
    "services/api-go/internal/core",
    "services/api-go/internal/usecase",
    "services/api-go/internal/ports",
    "services/api-go/internal/adapters/hardware",
    "services/api-go/internal/adapters/db",
    "services/api-go/internal/adapters/mq",
    "contracts/openapi",
    "contracts/proto",
    "deploy/dev",
    "deploy/staging",
    "deploy/prod",
    "tests/integration",
    "tests/hil"
)

New-Item -Path $projectRoot -ItemType Directory -Force | Out-Null

foreach ($relativeDir in $directories) {
    $normalized = $relativeDir -replace "/", [System.IO.Path]::DirectorySeparatorChar
    $target = Join-Path $projectRoot $normalized
    New-Item -Path $target -ItemType Directory -Force | Out-Null
}

$readmePath = Join-Path $projectRoot "README.md"
$readmeContent = @"
# $Name

## Scope

Describe what this project owns and what it does not own.

## Folder Notes

- `apps/desktop-wails/frontend`: Vue 3 desktop frontend (Wails).
- `apps/desktop-wails/backend`: Wails Go app host and bindings.
- `services/api-go`: Go backend service.
- `contracts`: API/protocol contracts.
- `tests/hil`: hardware-in-loop tests.

## Development Rules

- Follow workspace-level rules in `../../AGENTS.md`.
- Desktop app shell location: `apps/desktop-wails`.
- Keep business rules in `services/api-go/internal/core`.
- Keep hardware implementation in `services/api-go/internal/adapters/hardware` or `shared/device-sdk`.
"@

Set-Content -Path $readmePath -Value $readmeContent -Encoding UTF8

$projectAgentsPath = Join-Path $projectRoot "AGENTS.md"
$projectAgentsContent = @"
# Project Agent Rules

Single source of truth: `../../AGENTS.md`.

## Project Addendum

No project-specific overrides currently.

When adding local rules, keep only deltas here and do not copy workspace rules.
"@

Set-Content -Path $projectAgentsPath -Value $projectAgentsContent -Encoding UTF8

$projectClaudePath = Join-Path $projectRoot "CLAUDE.md"
$projectClaudeContent = @"
# Project Claude Rules

Single source of truth: `../../CLAUDE.md` and `../../AGENTS.md`.

## Project Addendum

No project-specific overrides currently.

If local constraints are needed later, list only project deltas here.
"@

Set-Content -Path $projectClaudePath -Value $projectClaudeContent -Encoding UTF8

Write-Host "Created project skeleton: projects/$Name"
Write-Host "If you want this project locked by structure validation, update workspace.structure.json."
exit 0
