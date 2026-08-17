# ADR-004: Wails v3 迁移与生产构建标签约束

## Status

Implemented (2026-06-29，2026-06-30 修订前后端版本锁定)

## Context

本工作空间历史上同时存在过 Wails v2 与 Wails v3 alpha 产品桌面应用。
2026 年 6 月，所有桌面端项目（`motion-controller`、`windlabx4`、`five-hole-interpolator`、
`three-hole-interpolator`、`daq-t1603`、`wispa`）的源码已迁移到
`github.com/wailsapp/wails/v3 v3.0.0-alpha2.106`。

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

2026-06-30 又出现一次回归：windlabx4 启动运动控制器独立窗口报
`Invalid runtime call:missing method value`。根因是 npm `@wailsio/runtime`
与 Go `wails/v3` 出现两条平行版本序列（`alpha.*` 与 `alpha2.*`），项目
go.mod 写 `alpha.95`，但 npm 包最新只到 `alpha.94`，两者协议不兼容导致
HTTP transport 把请求体 `method` 字段判为 nil。详见本文
"前后端版本锁定" 一节。

## Decision

工作空间统一采用以下约束。

### 1. Wails 版本

所有桌面端项目均使用 `github.com/wailsapp/wails/v3 v3.0.0-alpha2.106`，
前端 npm 包 `@wailsio/runtime` 锁定 `3.0.0-alpha.94`。任何 v2 残留
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

三项目（three-hole-interpolator / daq-t1603 / wispa）虽不使用自定义 Taskfile，
但其 `dev_build.cmd` 或等价脚本也必须设置 `set GOWORK=off`。

### 6. 打包前清理

打包流程必须执行 `task clean` 或等价的清理步骤，移除
`apps/desktop-wails/build/bin/` 旧产物，避免新旧二进制混淆。

### 7. 前后端 Wails 版本锁定（硬性约束）

Wails v3 处于 alpha 阶段，npm 与 Go module 出现过两条平行版本序列：
`3.0.0-alpha.*`（旧序列，最高 alpha.102）与 `3.0.0-alpha2.*`（新序列，
alpha2.96 起步）。不能仅按版本号序列自行配对；错误组合会让前端 `$Call.ByID`
请求被后端判为 `missing method value` / `missing object value`。

**强制锁定版本：**

| 端 | 包 | 锁定版本 |
|---|---|---|
| Go 后端 | `github.com/wailsapp/wails/v3` | `v3.0.0-alpha2.106` |
| 前端 | `@wailsio/runtime` (npm) | `3.0.0-alpha.94` |

> npm 上 `@wailsio/runtime` 只发到 `alpha.*` 序列；它的 `runtimeURL` 实现为
> `function`，与 Go `alpha2.*` 捆绑的 `runtime.ts` 一致，所以 npm alpha.94
> 必须配 Go alpha2.*，不能配 Go alpha.95/alpha.102。

**版本对应关系判断方法（升级时必读）：**

1. 打开 Go module 缓存中实际使用的 wails 版本目录，例如
   `C:\Users\<user>\go\pkg\mod\github.com\wailsapp\wails\v3@<版本号>\`。
2. 读取 `internal/runtime/desktop/@wailsio/runtime/package.json` 中的 `version` 字段，
   这个字段就是该 Go 版本"官方捆绑"的 npm runtime 版本。
3. 前端 `package.json` 中 `@wailsio/runtime` 的版本**必须**与之一致，或在该
   Go 版本捆绑的 npm 版本序列内。
4. 升级 Go wails 版本后，必须重新执行 `wails3 generate bindings`，让
   `frontend/bindings/` 与后端方法 ID 同步。

**禁止行为：**

- 禁止只升 Go 端不升 npm 端，或反之。
- 禁止凭印象写版本号；ADR 与 `package.json` / `go.mod` 中出现的版本号必须
  能在上述对应关系步骤中复现。
- 禁止使用 `@wailsio/runtime@latest` 或 `wails/v3@latest` 这类浮动引用。

**AI agent 升级 Wails 时的强制检查清单：**

1. 在 [npm registry](https://registry.npmjs.org/@wailsio/runtime) 查 npm 最新版本。
2. 在 [GitHub releases](https://github.com/wailsapp/wails/releases) 查 Go 最新版本。
3. 用上面"版本对应关系判断方法"核对一遍。
4. 同时更新所有 6 个桌面项目的 `go.mod` 和（如需）`package.json`。
5. 重新生成所有项目的 `frontend/bindings/`。
6. 同步更新本 ADR 第 7 节的锁定版本表。
7. 在 `docs/runbooks/release-versioning.zh-CN.md` 同步版本号。

## Consequences

### 正面

- 用户端不会再出现 v2 时代的 "Wails applications will not build" MessageBox。
- 任何项目的桌面交付产物风格统一，便于现场支持。
- AI agent 在打包时有明确硬约束，不依赖隐性约定。
- 前后端版本锁定后，不会再出现 `missing method value` 这类协议错配。

### 负面

- 已有的 `build-go` 任务不再适合日常开发期"快速 go build"，dev 流程需走
  `task dev` 或显式不带 `-tags production` 的命令。
- 构建命令变长，未来若 Wails 修改 build flag 语义需要本 ADR 同步更新。
- Wails 处于 alpha，版本号会持续滚动；每次升级都必须走第 7 节的检查清单，
  不得跳步。

## References

- `docs/runbooks/release-versioning.zh-CN.md`
- `projects/<project>/apps/desktop-wails/Taskfile.yml`
- Wails v2 源码：`internal/app/app_default_windows.go` 与 `app_default_unix.go`
- Wails v3 源码：`pkg/application/application_production.go` 与 `application_dev.go`
- Wails v3 HTTP transport：`pkg/application/transport_http.go`
- npm 包：`@wailsio/runtime` https://www.npmjs.com/package/@wailsio/runtime
- Go module：`github.com/wailsapp/wails/v3` https://github.com/wailsapp/wails/releases
