# Three-Hole Interpolator

> **⚠️ DEPRECATED (2026-07-20)** — 本项目已被 [`projects/probe-interpolator`](../probe-interpolator) 取代。
>
> 新项目 `probe-interpolator` v0.1.0+ 将 5 孔 / 3 孔 / 7 孔探针插值整合为单一桌面程序，
> 提供统一 UI 与共享算法包。本项目仅保留代码与历史 release 制品，不再发布新版本或修复缺陷。
> 请迁移至 `probe-interpolator` 进行后续开发与使用。

Standalone Wails desktop app for three-hole probe PRB interpolation.

Extracted from `ThreeHoleProbeApp` (C# WPF) and ported to Go + Vue 3.

## Scope

This project owns the user-facing desktop tool: file selection, CSV import,
batch calculation, help documents, and packaging. The reusable interpolation
algorithm lives in `shared/algorithms/go/threehole`.

## Development Commands

```powershell
# 桌面应用开发（热更新）
cd projects/three-hole-interpolator/apps/desktop-wails
go run github.com/wailsapp/wails/v3/cmd/wails3 dev
```

```powershell
# Go 测试（桌面应用）
cd projects/three-hole-interpolator/apps/desktop-wails
go test ./...
```

```powershell
# Go 测试（共享算法）
cd shared/algorithms/go/threehole
go test ./...
```

```powershell
# 前端构建
cd projects/three-hole-interpolator/apps/desktop-wails/frontend
npm run build
```

## Release Commands（对外交付打包）

```powershell
cd projects/three-hole-interpolator/apps/desktop-wails
$env:GOWORK="off"
go build -tags production -trimpath -buildvcs=false -ldflags="-w -s -H windowsgui" -o build/bin/three-hole-interpolator.exe .
```

生产构建使用 `-tags production`，且必须设置 `GOWORK=off` 以隔离工作空间中
wails/v2 的间接引用导致运行时报"correct build tags"错误。
详见 `docs/decisions/ADR-004-wails-v3-production-build.md`。

## Boundaries

- Keep Wails bindings and file dialogs in `apps/desktop-wails/backend`.
- Keep Vue UI in `apps/desktop-wails/frontend`.
- Keep interpolation algorithms in `shared/algorithms/go/threehole`.
- Do not depend on `projects/wind-daq` or `projects/five-hole-interpolator` internal packages.
