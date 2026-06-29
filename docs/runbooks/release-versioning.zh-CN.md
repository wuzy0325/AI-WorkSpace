# 发布版本与打包说明规则

本文定义 AI agent 在本工作区执行打包、发布、生成安装包、交付构建产物时必须遵守的版本管理和变更说明规则。

## 适用场景

当用户提出以下任一请求时，必须加载并执行本文规则：

- 打包 Wails 桌面应用
- 构建安装包或可交付 exe
- 发布一个测试包、验收包、现场包或正式包
- 执行 `wails build` 并准备交付产物
- 任何包含“打包”“发布”“版本”“安装包”“交付包”“release”“build installer”的任务

仅做本地开发验证、临时编译排错、CI 失败复现时，不要求更新版本号和变更说明，除非用户明确要求生成可交付包。

## 版本归属

本仓库采用“项目独立版本”规则。每个可交付项目维护自己的版本和发布说明。

推荐结构：

```text
projects/<project>/
  VERSION
  CHANGELOG.md
  releases/
    <version>.md
  apps/desktop-wails/
    wails.json
    frontend/package.json
```

其中：

- `VERSION` 是项目当前可交付版本的单一文本记录，只包含版本号。
- `CHANGELOG.md` 是项目累计变更历史。
- `releases/<version>.md` 是单次打包交付说明。
- `wails.json` 的 `version` 必须与项目版本同步。
- `frontend/package.json` 的 `version` 必须与项目版本同步。
- `build/windows/installer/project.nsi` 中的 `INFO_PRODUCTVERSION` 必须与项目版本同步。
  （此 define 控制 NSIS 安装包文件名中嵌入的版本号。）
- 如果存在 `package-lock.json`，其根包版本也必须同步。

如果目标项目尚未建立上述文件，首次按本文规则打包时应补齐 `VERSION`、`CHANGELOG.md` 和 `releases/`。

## 版本号规则

使用 SemVer：`MAJOR.MINOR.PATCH`。

| 变更类型 | 版本变化 | 示例 |
|---|---:|---|
| 缺陷修复 | PATCH | 修复采集停止后状态错误 |
| 稳定性或容错改进 | PATCH | 串口断开后的错误提示和恢复逻辑 |
| 小型 UI / 文案调整 | PATCH | 调整按钮状态、提示文案、间距 |
| 新增用户可见能力 | MINOR | 新增导出、记录、配置、页面或工作流 |
| 新增设备支持 | MINOR | 新增 DAQ、运动控制或打压设备型号 |
| 兼容性数据扩展 | MINOR | CSV 增加可选列且旧文件仍可用 |
| 不兼容变更 | MAJOR | 配置格式、数据格式、设备协议或用户工作流破坏兼容 |
| 删除已发布功能 | MAJOR | 移除旧采集模式或旧文件格式支持 |

预发布版本可使用：

```text
0.2.0-beta.1
0.2.0-rc.1
```

如果只是重新打包完全相同代码，默认不得修改版本号；如需区分内部构建，可使用构建元数据，例如：

```text
0.2.0+20260608.1
```

正式交付给用户或现场人员的包，优先使用普通 SemVer，不依赖构建元数据表达功能变化。

## AI 打包前强制流程

AI agent 在打包前必须执行以下流程：

1. 确认目标项目和打包类型。
2. 读取目标项目当前 `VERSION`、`CHANGELOG.md`、`apps/desktop-wails/wails.json`、`frontend/package.json`。
3. 检查自上一版本以来的相关变更。
4. 判断版本号应做 PATCH、MINOR、MAJOR 或预发布递增。
5. 如果版本等级存在歧义，先向用户确认，不得擅自选择较大的版本跳跃。
6. 更新项目 `VERSION`。
7. 更新 `apps/desktop-wails/wails.json` 的 `version`。
8. 更新 `apps/desktop-wails/frontend/package.json` 的 `version`。
9. 如果存在 `package-lock.json`，同步其中根包版本。
10. 更新 `apps/desktop-wails/build/windows/installer/project.nsi` 中的 `INFO_PRODUCTVERSION`。
11. 创建或更新 `releases/<version>.md` 单次打包说明，必须包含 `Install / Upgrade` 段。
12. 清理本地遗留的 `apps/desktop-wails/build/bin/` 和上次打包遗留的 `.syso`、`installer/*.exe` 等中间产物。
13. 运行目标项目适用的验证命令。
14. 用「生产构建」方式构建可执行文件（见下文「生产构建必备构建标签」）。
15. 通过本机或现场冒烟测试启动一次新构建产物，确认 GUI 正常启动、没有"Wails applications will not build without the correct build tags"等明显错误。
16. 验证通过后再执行 NSIS 等安装包封装命令。
17. 最终回复必须包含版本号、主要变更、验证结果、产物路径，以及是否要求用户卸载旧版本。

AI agent 不得在版本号不变且无明确说明的情况下生成新的可交付包。

## 生产构建必备构建标签

Wails 桌面项目对外交付时，必须以「生产模式」构建二进制：

- Wails v2 项目：`go build` **必须**传 `-tags production`，否则会落到默认 stub，运行时弹出
  「Wails applications will not build without the correct build tags. Please use "wails build" or press "OK" to open the documentation on how to use "go build"」并立即退出。
- Wails v3 项目：`go build` **应**传 `-tags production`，否则会进入 dev 分支，包含开发期检测和
  开发期 webview 行为，不应作为正式交付。

对 v3 项目推荐的生产构建命令（Windows / amd64）：

```powershell
$env:GOWORK="off"; go build -tags production -trimpath -buildvcs=false -ldflags="-w -s -H windowsgui" -o build/bin/<project>.exe .
```

对应 Taskfile：`build-go` 任务必须固化上述 `-tags production` 参数，不得让构建者
凭记忆补传标签。并且 `build-go` 任务必须设置 `env: GOWORK: off` 以隔离工作空间
中其他模块的 wails/v2 污染：

```yaml
build-go:
  env:
    GOWORK: off
  cmds:
    - go build -tags production ...
```

任何项目交付 Wails 桌面安装包前，AI agent 必须检查：

1. 项目 `apps/desktop-wails/Taskfile.yml` 中 `build-go`（或等价任务）已经包含 `-tags production`。
2. `build-go` 任务已经设置 `env: GOWORK: off`（或命令行构建时设置了 `GOWORK=off`）。
3. `wails.json` 中 Wails 版本与 `go.mod` 中实际依赖一致（如 v3 alpha.95 对应 `wails/v3 v3.0.0-alpha.95`）。
4. 没有 v2 时代遗留的安装产物被混入本次发布。

不满足上述任意一项时，AI agent 必须先修复脚本或文档，再继续打包。

## CHANGELOG 格式

每个项目的 `CHANGELOG.md` 使用以下格式：

```md
# Changelog

## [0.2.0] - 2026-06-08

### Added
- 新增采集状态显示。

### Changed
- 调整采集页面布局，提升现场操作可读性。

### Fixed
- 修复停止采集后状态未及时恢复的问题。

### Internal
- 重构采集用例测试夹具，未改变用户行为。

### Verification
- `go test ./...`
- `npm run typecheck`
- `npm run build`
- `wails build`

### Known Issues
- 暂无。
```

分类规则：

- `Added`：新增用户、测试人员或现场人员能感知的能力。
- `Changed`：已有行为、界面、流程、配置的改变。
- `Fixed`：缺陷修复。
- `Internal`：重构、测试、构建脚本、文档等内部改动。
- `Verification`：本次发布实际运行过的验证命令。
- `Known Issues`：本版本已知但未解决的问题。

如果某一类没有内容，可以省略；`Verification` 和 `Known Issues` 不应省略。

不得使用含糊描述，例如：

- “优化代码”
- “修复若干问题”
- “更新内容”
- “misc fixes”

应改成可追溯描述，例如：

- “修复采集停止后按钮仍保持禁用的问题。”
- “将串口连接失败提示改为包含端口号和重试建议。”

## 单次发布说明格式

每次可交付打包必须创建或更新：

```text
projects/<project>/releases/<version>.md
```

推荐模板：

```md
# <Product Name> v<version>

Release date: YYYY-MM-DD
Package: <artifact-name>

## Summary

用 1-3 句话说明本版本解决了什么问题或增加了什么能力。

## User-visible Changes

- 面向用户、测试人员、现场人员的变化。

## Fixes

- 缺陷修复。

## Compatibility

- 配置文件格式：兼容 / 不兼容，说明迁移方式。
- 数据文件格式：兼容 / 不兼容，说明迁移方式。
- 设备协议行为：无变化 / 有变化，说明影响。

## Install / Upgrade

- 是否需要先卸载旧版本（例如跨大版本框架迁移、安装路径或注册表项变化时必须卸载）。
- 是否兼容覆盖安装。
- 是否需要清理用户数据目录、WebView2 缓存目录或注册表残留。
- 推荐的安装步骤摘要：
  1. 关闭正在运行的旧版本。
  2. 如需，先通过"控制面板 → 卸载"或 `Uninstall.exe` 卸载旧版本。
  3. 运行 `<project>-<version>-<arch>-installer.exe`。
  4. 启动新版本并确认 GUI 正常出现。

## Verification

- `<command>`: passed / failed，必要时补充原因。

## Known Issues

- 暂无，或列出已知限制。

## Rollback

- 说明是否可直接回退到上一版本，以及是否需要处理配置或数据文件。
```

## 打包产物命名

建议使用以下命名规则：

可执行文件：
```text
<ProductName>.exe
```

NSIS 安装包（由 `project.nsi` 控制）：
```text
<project>-<version>-<arch>-installer.exe
```

示例：

```text
DAQ-T-1603.exe
daq-t1603-0.1.2-amd64-installer.exe
```

如果 Wails 默认产物名无法直接控制，最终回复和发布说明中必须记录实际产物路径。

## 验证要求

打包前应运行目标项目 `AGENTS.md`、`README.md` 或 `CLAUDE.md` 中列出的适用命令。

Wails 项目的常见最低验证：

```powershell
# Go module root（开发期验证，可不带 production 标签）
go test ./...
go build -buildvcs=false ./...

# frontend root
npm run typecheck
npm run build
npm run test

# Wails app root：交付构建必须带 -tags production
go build -tags production -trimpath -buildvcs=false -ldflags="-w -s -H windowsgui" -o build/bin/<project>.exe .
# 或通过 Taskfile：
task build
```

如果某条命令因环境缺失、硬件依赖或已知项目限制无法运行，AI 必须在发布说明和最终回复中记录：

- 未运行的命令
- 未运行原因
- 风险影响
- 建议补充验证

## 最终回复要求

打包完成后，AI 最终回复必须包含：

- 项目名
- 版本号变化，例如 `0.1.0 -> 0.2.0`
- 版本 bump 理由
- 主要变更摘要
- 更新过的版本和发布说明文件
- 实际运行的验证命令和结果
- 打包产物路径
- 已知风险或未完成验证

## 禁止事项

- 不得跳过版本号和变更说明直接打包可交付产物。
- 不得把多个不相关项目的版本混在一起递增。
- 不得为了省事统一递增所有项目版本。
- 不得把依赖版本升级当作产品版本升级理由，除非它改变了产品行为或安全风险。
- 不得编造验证结果；未运行必须明确写未运行。
- 不得在 changelog 中写无法追溯的泛化描述。
