# ADR-006: daq-t1603 临时摘除工作空间

## Date

2026-07-02

## Status

Accepted（临时，待回归）

## Context

daq-t1603 升级到 Wails v3 alpha.95 后，与工作空间其他仍使用 Wails v2 的项目（motion-controller / five-hole-interpolator / three-hole-interpolator）通过 `wails/v2` 间接依赖产生冲突，导致 `go.work` 内 `go build ./...` 失败。

daq-t1603 当前阶段需要 v3 alpha（用于新增的硬件通信日志、TS binding 等），其他项目暂未升级。在所有项目统一 v3 之前，daq-t1603 必须独立构建。

历史背景：daq-t1603 v0.2.0 release note 提到「wails3 因 go.sum 缺失不可用，改用 go build 直出」，根因正是被摘出工作空间——`wails3 build` 依赖工作空间解析间接依赖，摘出后 `GOWORK=off` 单模块构建可绕过此问题。

## Decision

将 daq-t1603 从 `go.work` 的 use 列表中临时摘除（注释保留，便于回归）。daq-t1603 必须用 `GOWORK=off` 单独构建：

```powershell
cd projects\daq-t1603\apps\desktop-wails
$env:GOWORK="off"
go test ./...
go vet ./...
go build -buildvcs=false ./...
go run github.com/wailsapp/wails/v3/cmd/wails3 build
```

## Consequences

- **正向**：daq-t1603 可独立升级到 v3 alpha，不被其他项目 v2 依赖拖住；其他项目构建不受 daq-t1603 v3 依赖影响
- **负向**：
  - 工作空间 `go build ./...` 不再覆盖 daq-t1603，CI 必须为 daq-t1603 单独跑检查
  - daq-t1603 共享代码改动（如 `shared/device-sdk/go/*`）不会立即在工作空间中验证，需 daq-t1603 项目内 `GOWORK=off` 单独跑测试
  - IDE（VSCode/Trae）对 daq-t1603 的 Go 工具支持可能受限（gopls 工作空间模式不覆盖）
  - pre-commit hook 的 import-direction scan 仍会扫描 daq-t1603，但其他 workspace 级工具不会

## Reversion Criteria

当以下任一条件满足时，可将 daq-t1603 合并回 `go.work`：

1. 工作空间所有 Wails 项目统一升级到 v3 alpha.95 或更高（推荐等待 v3 稳定版）
2. `wails/v3` 发布稳定版，v2/v3 共存冲突消除
3. daq-t1603 单独验证通过后由维护者人工合并并跑全工作空间 `go build ./...` 确认无冲突

回归步骤：
1. 取消 `go.work` 第 9 行 `// projects/daq-t1603/apps/desktop-wails` 注释
2. 跑 `go build ./...` 验证全工作空间构建通过
3. 跑 `go test ./...` 验证全工作空间测试通过
4. 若失败，恢复注释并在 daq-t1603 issue tracker 记录失败原因

## References

- `go.work` 第 7-9 行注释
- `projects/daq-t1603/releases/0.2.0.md` Verification 段落（wails3 不可用根因）
- `docs/decisions/ADR-004-wails-v3-production-build.md`（GOWORK=off 生产构建规则）
- `AGENTS.md` Environment Requirements 段落
