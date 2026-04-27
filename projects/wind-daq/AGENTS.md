# Wind-DAQ Agent Rules

Single source of truth: `../../AGENTS.md`.

## Project Commands

```powershell
# Build
go build -buildvcs=false ./...
cd services/api-go

# Format & lint
gofmt -l .          # check formatting
gofmt -w .          # fix formatting
go vet ./...        # static analysis

# Run
go run ./cmd/server/main.go
```

## Pre-submit Checklist

1. `go build -buildvcs=false ./...` — must pass
2. `gofmt -l .` — must show no output
3. Verify docs/STRUCTURE.md matches actual disk layout
