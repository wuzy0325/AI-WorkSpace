# Sync official addyosmani/agent-skills (https://github.com/addyosmani/agent-skills)
# into the three agents used in this workspace:
#   - OpenCode    : global skills dir (~/.config/opencode/skills)
#   - Trae Work   : <ws>/.trae/skills
#   - WorkBuddy   : <ws>/.workbuddy/skills  +  <ws>/.codebuddy/skills
#
# Run:  pwsh scripts/sync-agent-skills.ps1
# Re-run anytime to pull the latest official skills and re-sync all agents.

param(
    [string]$Workspace = (Resolve-Path (Join-Path $PSScriptRoot "..")),
    [string]$RepoCache = (Join-Path $env:LOCALAPPDATA "opencode\agent-skills")
)

$ErrorActionPreference = "Stop"

# 1. Obtain the official skills source (clone once, then git pull to update).
if (-not (Test-Path (Join-Path $RepoCache ".git"))) {
    if (Test-Path $RepoCache) { Remove-Item -LiteralPath $RepoCache -Recurse -Force }
    git clone --depth 1 https://github.com/addyosmani/agent-skills.git $RepoCache
} else {
    git -C $RepoCache pull --ff-only
}

$srcSkills = Join-Path $RepoCache "skills"
if (-not (Test-Path $srcSkills)) { throw "Official skills not found at $srcSkills" }

# 2. Enumerate the exact official skill set (driven by the repo, always correct).
$skillNames = (Get-ChildItem -LiteralPath $srcSkills -Directory).Name
Write-Host "Official skills found: $($skillNames.Count)"

# 3. Define each agent's target skills directory.
$targets = @(
    (Join-Path $env:USERPROFILE ".config\opencode\skills"),          # OpenCode (global)
    (Join-Path $Workspace ".trae\skills"),                          # Trae Work
    (Join-Path $Workspace ".workbuddy\skills"),                     # WorkBuddy
    (Join-Path $Workspace ".codebuddy\skills")                      # CodeBuddy (WorkBuddy family)
)

function Copy-Skill {
    param([string]$Name, [string]$Dest)
    $from = Join-Path $srcSkills $Name
    $to   = Join-Path $Dest $Name
    if (Test-Path $to) { Remove-Item -LiteralPath $to -Recurse -Force }
    Copy-Item -LiteralPath $from -Destination $to -Recurse -Force
}

foreach ($t in $targets) {
    if (-not (Test-Path $t)) { New-Item -ItemType Directory -Path $t -Force | Out-Null }
    foreach ($n in $skillNames) { Copy-Skill -Name $n -Dest $t }
    Write-Host "Synced $($skillNames.Count) skills -> $t"
}

# 4. Trae routing rule (always-apply) so the agent prefers skills by description.
$traeRuleDir = Join-Path $Workspace ".trae\rules"
if (-not (Test-Path $traeRuleDir)) { New-Item -ItemType Directory -Path $traeRuleDir -Force | Out-Null }
$traeRule = Join-Path $traeRuleDir "agent-skills.md"
Set-Content -LiteralPath $traeRule -Value (@"
---
description: Use the addyosmani agent-skills (spec/plan/build/test/review/ship) by matching intent to each skill's description. Always prefer a skill over ad-hoc reasoning.
alwaysApply: true
---

# Agent Skills (addyosmani/agent-skills)

This project ships the official production-grade engineering skills from
https://github.com/addyosmani/agent-skills under `.trae/skills/`.

Rules:
- Before any non-trivial task, compare the request against each skill's
  `description` in `.trae/skills/<name>/SKILL.md` and load the best match.
- DEFINE -> spec-driven-development; PLAN -> planning-and-task-breakdown;
  BUILD -> incremental-implementation + test-driven-development;
  VERIFY -> debugging-and-error-recovery; REVIEW -> code-review-and-quality;
  SHIP -> shipping-and-launch.
- Follow the loaded skill exactly; do not skip required steps or verification.
"@)
Write-Host "Wrote Trae routing rule: $traeRule"

# 5. WorkBuddy/CodeBuddy index (project memory) pointing at the skills.
$cbIndex = Join-Path $Workspace ".codebuddy\CODEBUDDY.md"
$cbDir = Split-Path -Parent $cbIndex
if (-not (Test-Path $cbDir)) { New-Item -ItemType Directory -Path $cbDir -Force | Out-Null }
Set-Content -LiteralPath $cbIndex -Value (@"
# WorkBuddy / CodeBuddy — Agent Skills

The official engineering skills from https://github.com/addyosmani/agent-skills are
available under `.workbuddy/skills/` and `.codebuddy/skills/` (24 skills).

When a task matches a skill's `description`, load and follow that skill
(SKILL.md) instead of improvising. Lifecycle map:
DEFINE->spec-driven-development, PLAN->planning-and-task-breakdown,
BUILD->incremental-implementation + test-driven-development,
VERIFY->debugging-and-error-recovery, REVIEW->code-review-and-quality,
SHIP->shipping-and-launch.
"@)
Write-Host "Wrote WorkBuddy/CodeBuddy index: $cbIndex"

Write-Host "`nDone. All three agents now use the official agent-skills."
