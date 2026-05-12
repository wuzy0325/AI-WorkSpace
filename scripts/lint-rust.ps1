# lint-rust.ps1
# Run Rust formatting and compile checks for the workspace.
[CmdletBinding()]
param()

$ErrorActionPreference = "Stop"
$root = Resolve-Path "$PSScriptRoot/.."

Push-Location $root
try {
    cargo fmt --all -- --check
    cargo check --workspace
}
finally {
    Pop-Location
}
