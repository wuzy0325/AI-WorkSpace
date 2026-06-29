# ADR-004: Wails v3 迁移与生产构建标签约束

## Status

Implemented (2026-06-29)

## Context

本工作空间历史上同时存在过 Wails v2 与 Wails v3 alpha 产品桌面应用。
2026 年 6 月，所有桌面端项目（`motion-controller`、`wind-daq`、`five-hole-interpolator`、
`three-hole-interpolator`、`daq-t1603`、`daq-p1604`）的源码已迁移到
`github.com/wailsapp/wails/v3 v3.0.0-alpha.95`。

但工作空间出现了以下真实故障：

- motion-controller v0.2.x 安装包在用户端启动时弹出
  `Wails applications will not build without the correct build tags. Please use "wails build"
  or press "OK" to open the documentation on how to use "go build"`。
- 该错误文本只来自 Wails v2 的 `internal/app/app_default_windows.go`（构建约束
  `//go:build !dev && !production && !bindings && windows`），表明该 exe 是 v2 时代或
  无 `-tags production` 的产物。
- 调查显示：
  - 各项目自定义 `Taskfile.yml` 的 `build-go` 长期没有传 `-tags production`。
  - `docs/runbooks/release-versioning.zh-CN.md` 没有写"生产交付必须 `-tags production`"。
  - 发布说明模板缺少 `Install / Upgrade` 段，导致跨大版本框架升级（v2 → v3）时
    没有告诉用户必须先卸载旧版。
  - 没有"清理上次产物"的强制步骤，导致旧 exe 残留并被错误地继续分发。

文档缺口是这次安装失败的根本原因，而非 Wails v3 本身。

## Decision

工作空间统一采用以下约束。

### 1. Wails 版本

所有桌面端项目均使用 `github.com/wailsapp/wails/v3 v3.0.0-alpha.95`。任何 v2 残留
产物视为废弃，不得再发布。

### 2. 生产构建标签是硬性要求

所有桌面项目对外交付 exe 时必须使用 `-tags production`：

```powershell
go build -tags production -trimpath -buildvcs=false -ldflags="-w -s -H windowsgui" -o build/bin/<project>.exe .
```

理由：

- Wails v2 缺标签会直接弹出 MessageBox 阻塞启动。
- Wails v3 缺标签会落到 dev 分支（`//go:build !production`），开启 devtools 与
  开发期 webview 行为，不应用于生产。

### 3. Taskfile 固化构建参数

所有项目 `apps/desktop-wails/Taskfile.yml` 必须包含 `release` 任务：

```yaml
release:
  cmds:
    - task: clean
    - task: build-frontend
    - task: build-go      # 内部使用 -tags production
```

`build-go` 任务必须包含 `-tags production`，不允许凭记忆补传。

### 4. 发布说明强制段落

`projects/<project>/releases/<version>.md` 必须包含 `Install / Upgrade` 段，
明确：

- 是否需要先卸载旧版本。
- 是否兼容覆盖安装。
- 是否需要清理用户数据/注册表残留。
- 推荐的安装步骤摘要。

### 5. Go Workspace 隔离

工作空间根目录 `go.work` 包含所有项目的 Go 模块。当其中存在混合 v2/v3 Wails 模块的残留时
（即使当前 go.mod 均已升级到 v3），`go build` 在工作空间模式下可能通过模块图解析将
wails/v2 的包间接引入最终二进制，导致 `app_default_windows.go` 中的
"Wails applications will not build" MessageBox stub 被编译进去。

已发生的真实故障（2026-06-29）：motion-controller v0.2.4 安装后启动仍弹此错误。
验证确认根因为 `go.work` 模式导致二进制包含 `github.com/wailsapp/wails/v2` 符号。
`$env:GOWORK="off"; go build ...` 构建后的二进制完全干净。

**强制要求：所有 Taskfile.yml 的 `build-go` 任务必须设置 `env: GOWORK: off`。**

三项目（three-hole-interpolator / daq-t1603 / daq-p1604）虽不使用自定义 Taskfile，
但其 `dev_build.cmd` 或等价脚本也必须设置 `set GOWORK=off`。

### 6. 打包前清理

打包流程必须执行 `task clean` 或等价的清理步骤，移除
`apps/desktop-wails/build/bin/` 旧产物，避免新旧二进制混淆。

## Consequences

### 正面

- 用户端不会再出现 v2 时代的 "Wails applications will not build" MessageBox。
- 任何项目的桌面交付产物风格统一，便于现场支持。
- AI agent 在打包时有明确硬约束，不依赖隐性约定。

### 负面

- 已有的 `build-go` 任务不再适合日常开发期"快速 go build"，dev 流程需走
  `task dev` 或显式不带 `-tags production` 的命令。
- 构建命令变长，未来若 Wails 修改 build flag 语义需要本 ADR 同步更新。

## References

- `docs/runbooks/release-versioning.zh-CN.md`
- `projects/<project>/apps/desktop-wails/Taskfile.yml`
- Wails v2 源码：`internal/app/app_default_windows.go` 与 `app_default_unix.go`
- Wails v3 源码：`pkg/application/application_production.go` 与 `application_dev.go`
