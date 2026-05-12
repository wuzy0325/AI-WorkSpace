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
$rustIdentifier = $Name -replace "-", "_"
$bundleIdentifier = $Name -replace "-", ""

if (Test-Path -Path $projectRoot) {
    Write-Error "Project already exists: projects/$Name"
    exit 1
}

$directories = @(
    "apps/desktop-tauri/frontend/src",
    "apps/desktop-tauri/src-tauri/src",
    "services/api-rs/src/bin",
    "services/api-rs/src/core",
    "services/api-rs/src/usecase",
    "services/api-rs/src/ports",
    "services/api-rs/src/adapters/hardware",
    "services/api-rs/src/adapters/db",
    "services/api-rs/src/adapters/mq",
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

- `apps/desktop-tauri/frontend`: Vue 3 desktop frontend.
- `apps/desktop-tauri/src-tauri`: Tauri Rust shell and command bridge.
- `services/api-rs`: Rust backend service.
- `contracts`: API/protocol contracts.
- `tests/hil`: hardware-in-loop tests.

## Development Rules

- Follow workspace-level rules in `../../AGENTS.md`.
- Desktop app shell location: `apps/desktop-tauri`.
- Keep business rules in `services/api-rs/src/core`.
- Keep hardware implementation in `services/api-rs/src/adapters/hardware` or `shared/device-sdk`.
"@

Set-Content -Path $readmePath -Value $readmeContent -Encoding UTF8

$desktopRoot = Join-Path $projectRoot "apps/desktop-tauri"
Set-Content -Path (Join-Path $desktopRoot "README.md") -Value "# $Name Desktop`n`nTauri desktop shell. Business logic belongs in ../../services/api-rs." -Encoding UTF8

$packageContent = @"
{
  "name": "$Name-desktop",
  "private": true,
  "version": "0.1.0",
  "type": "module",
  "scripts": {
    "dev": "vite",
    "build": "vite build",
    "tauri": "tauri"
  },
  "dependencies": {
    "@tauri-apps/api": "^2.0.0",
    "vue": "^3.5.0"
  },
  "devDependencies": {
    "@tauri-apps/cli": "^2.0.0",
    "@vitejs/plugin-vue": "^5.0.0",
    "vite": "^5.0.0",
    "typescript": "^5.0.0"
  }
}
"@
Set-Content -Path (Join-Path $desktopRoot "package.json") -Value $packageContent -Encoding UTF8

$indexContent = @"
<!doctype html>
<html lang="en">
  <head>
    <meta charset="UTF-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1.0" />
    <title>$Name</title>
  </head>
  <body>
    <div id="app"></div>
    <script type="module" src="/src/main.ts"></script>
  </body>
</html>
"@
Set-Content -Path (Join-Path $desktopRoot "frontend/index.html") -Value $indexContent -Encoding UTF8

$viteContent = @"
import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'

export default defineConfig({
  plugins: [vue()],
  root: 'frontend',
  clearScreen: false,
  server: {
    port: 1420,
    strictPort: true,
  },
  build: {
    outDir: '../dist',
    emptyOutDir: true,
  },
})
"@
Set-Content -Path (Join-Path $desktopRoot "vite.config.ts") -Value $viteContent -Encoding UTF8

Set-Content -Path (Join-Path $desktopRoot "frontend/src/main.ts") -Value "import { createApp } from 'vue'`n`nimport App from './App.vue'`n`ncreateApp(App).mount('#app')" -Encoding UTF8

$appVueContent = @"
<template>
  <main>$Name desktop placeholder</main>
</template>
"@
Set-Content -Path (Join-Path $desktopRoot "frontend/src/App.vue") -Value $appVueContent -Encoding UTF8

$tauriCargoContent = @"
[package]
name = "$Name-desktop"
version = "0.1.0"
edition = "2021"

[lib]
name = "${rustIdentifier}_desktop_lib"
crate-type = ["staticlib", "cdylib", "rlib"]

[build-dependencies]
tauri-build = { version = "2.0.0", features = [] }

[dependencies]
tauri = { version = "2.0.0", features = [] }
tauri-plugin-opener = "2.0.0"
serde = { version = "1.0", features = ["derive"] }
serde_json = "1.0"
"@
Set-Content -Path (Join-Path $desktopRoot "src-tauri/Cargo.toml") -Value $tauriCargoContent -Encoding UTF8

Set-Content -Path (Join-Path $desktopRoot "src-tauri/build.rs") -Value "fn main() {`n    tauri_build::build()`n}" -Encoding UTF8

$tauriLibContent = @"
#[tauri::command]
fn app_version() -> &'static str {
    env!("CARGO_PKG_VERSION")
}

pub fn run() {
    tauri::Builder::default()
        .plugin(tauri_plugin_opener::init())
        .invoke_handler(tauri::generate_handler![app_version])
        .run(tauri::generate_context!())
        .expect("failed to run Tauri application");
}
"@
Set-Content -Path (Join-Path $desktopRoot "src-tauri/src/lib.rs") -Value $tauriLibContent -Encoding UTF8
Set-Content -Path (Join-Path $desktopRoot "src-tauri/src/main.rs") -Value "fn main() {`n    ${rustIdentifier}_desktop_lib::run()`n}" -Encoding UTF8

$tauriConfigContent = @"
{
  "`$schema": "https://schema.tauri.app/config/2.0.0",
  "productName": "$Name",
  "version": "0.1.0",
  "identifier": "com.$bundleIdentifier.desktop",
  "build": {
    "beforeDevCommand": "npm run dev",
    "beforeBuildCommand": "npm run build",
    "devUrl": "http://localhost:1420",
    "frontendDist": "../dist"
  },
  "app": {
    "windows": [
      {
        "title": "$Name",
        "width": 1440,
        "height": 900,
        "minWidth": 1280,
        "minHeight": 720
      }
    ]
  }
}
"@
Set-Content -Path (Join-Path $desktopRoot "src-tauri/tauri.conf.json") -Value $tauriConfigContent -Encoding UTF8

$cargoPath = Join-Path $projectRoot "services/api-rs/Cargo.toml"
$cargoContent = @"
[package]
name = "$Name-api"
version = "0.1.0"
edition = "2021"

[lib]
path = "src/lib.rs"

[[bin]]
name = "$Name-api"
path = "src/bin/server.rs"

[dependencies]
"@
Set-Content -Path $cargoPath -Value $cargoContent -Encoding UTF8

$libPath = Join-Path $projectRoot "services/api-rs/src/lib.rs"
Set-Content -Path $libPath -Value "pub mod adapters;`npub mod core;`npub mod ports;`npub mod usecase;" -Encoding UTF8

$serverPath = Join-Path $projectRoot "services/api-rs/src/bin/server.rs"
$serverContent = @"
fn main() {
    println!("$Name-api Rust backend placeholder");
}
"@
Set-Content -Path $serverPath -Value $serverContent -Encoding UTF8

Set-Content -Path (Join-Path $projectRoot "services/api-rs/src/core/mod.rs") -Value "//! Pure domain logic." -Encoding UTF8
Set-Content -Path (Join-Path $projectRoot "services/api-rs/src/usecase/mod.rs") -Value "//! Application orchestration." -Encoding UTF8
Set-Content -Path (Join-Path $projectRoot "services/api-rs/src/ports/mod.rs") -Value "//! Boundary traits." -Encoding UTF8
Set-Content -Path (Join-Path $projectRoot "services/api-rs/src/adapters/mod.rs") -Value "//! Concrete adapters.`n`npub mod hardware;" -Encoding UTF8
Set-Content -Path (Join-Path $projectRoot "services/api-rs/src/adapters/hardware/mod.rs") -Value "//! Hardware protocol translation and I/O implementations." -Encoding UTF8

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
