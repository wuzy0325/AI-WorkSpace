# Five-Hole Interpolator

> **⚠️ DEPRECATED (2026-07-20)** — 本项目已被 [`projects/probe-interpolator`](../probe-interpolator) 取代。
>
> 新项目 `probe-interpolator` v0.1.0+ 将 5 孔 / 3 孔 / 7 孔探针插值整合为单一桌面程序，
> 提供统一 UI 与共享算法包。本项目仅保留代码与历史 release 制品，不再发布新版本或修复缺陷。
> 请迁移至 `probe-interpolator` 进行后续开发与使用。

Standalone Wails desktop app for five-hole probe PRB interpolation.

## Scope

This project owns the user-facing desktop tool: file selection, CSV import,
batch calculation, help documents, and packaging. Reusable interpolation
algorithms live in `shared/algorithms/go/fivehole` and are shared with
`projects/wind-daq`.

## Development Commands

```powershell
# 桌面应用开发（热更新）
cd projects/five-hole-interpolator/apps/desktop-wails
go run github.com/wailsapp/wails/v3/cmd/wails3 dev
```

```powershell
# Go 测试
cd projects/five-hole-interpolator/apps/desktop-wails
go test ./...
```

```powershell
# 前端构建
cd projects/five-hole-interpolator/apps/desktop-wails/frontend
npm run build
```

## Release Commands（对外交付打包）

```powershell
cd projects/five-hole-interpolator/apps/desktop-wails
task release        # 清理旧产物 → 构建前端 → 生产模式 Go 二进制
```

生产构建使用 `-tags production -trimpath -ldflags="-w -s -H windowsgui"`，
详见 `docs/decisions/ADR-004-wails-v3-production-build.md`。

## Boundaries

- Keep Wails bindings and file dialogs in `apps/desktop-wails/backend`.
- Keep Vue UI in `apps/desktop-wails/frontend`.
- Keep interpolation algorithms in `shared/algorithms/go/fivehole`.
- Do not depend on `projects/wind-daq/services/api-go`.
