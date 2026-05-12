# Wind-DAQ Agent Rules

Single source of truth: `../../AGENTS.md`.

## Project Commands

```powershell
# From workspace root
Set-Location .\projects\wind-daq\services\api-rs

# Build
cargo check

# Format & lint
cargo fmt --check
cargo clippy --all-targets -- -D warnings

# Run
cargo run --bin wind-daq-api
```

## Pre-submit Checklist

1. `Set-Location .\projects\wind-daq\services\api-rs; cargo check` — must pass
2. `Set-Location .\projects\wind-daq\services\api-rs; cargo fmt --check` — must pass
3. Verify docs/STRUCTURE.md matches actual disk layout
