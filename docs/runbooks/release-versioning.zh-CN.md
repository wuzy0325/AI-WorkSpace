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
10. 更新 `apps/desktop-wails/build/windows/installer/project.nsi` 中的 `INFO_PRODUCTVERSION`。**先检测文件编码，再选择修改方式**（实测各项目编码不统一，检测命令见下）：
    ```powershell
    # 检测编码（读前 2 字节）：FF FE = UTF-16LE with BOM；否则为 UTF-8/ASCII
    $b=[System.IO.File]::ReadAllBytes('apps/desktop-wails/build/windows/installer/project.nsi')
    "BOM: $($b[0])-$($b[1])"
    ```
    - **UTF-8/ASCII 编码**（daq-t1603 / motion-controller / five-hole-interpolator / three-hole-interpolator 实测如此）：Edit 工具可直接改，无编码风险。
    - **UTF-16 LE with BOM 编码**（daq-p1604 / wind-daq 实测如此）：Edit 类工具会破坏编码导致 makensis 报 "Bad text encoding"，必须用 PowerShell 保留编码替换：
      ```powershell
      $path = 'apps/desktop-wails/build/windows/installer/project.nsi'
      $content = Get-Content -Path $path -Encoding Unicode -Raw
      $updated = $content -replace '!define INFO_PRODUCTVERSION "0\.12\.1"', '!define INFO_PRODUCTVERSION "0.12.2"'
      Set-Content -Path $path -Value $updated -Encoding Unicode -NoNewline
      ```
      **注意：`-Encoding Unicode` 读 UTF-8 文件会解码成乱码再写回，文件就废了。修改前必须确认 BOM 是 `255-254`，否则禁止用此命令。** 禁止用 `[System.Text.Encoding]::Default` 转编码；禁止删除 project.nsi（如需恢复用 `git checkout`）。`projects/<project>/installer/project.nsi`（备份副本）是普通 UTF-8，Edit 可直接改。
11. 更新 `apps/desktop-wails/build/config.yml` 的 `info.version`（第 6 个版本源；注意 wails3 的 syso 生成并不实际渲染该值进内嵌 exe 版本资源，见下方"已知限制"，同步它仅为保持 6 文件版本一致）。
12. 创建或更新 `releases/<version>.md` 单次打包说明，必须包含 `Install / Upgrade` 段。
13. 清理本地遗留的 `apps/desktop-wails/build/bin/` 和上次打包遗留的 `installer/*.exe` 等中间产物。**注意：`wails_windows_amd64.syso` 是入库的 PE 资源段源文件（由 `generate-icon` 任务管理），不要手动删除；如需刷新图标，执行 `task generate-icon` 重新生成。**

> **注意**：`build/info.json` 和 `build/windows.manifest`（具体值版本）已删除，不再需要手动维护。Taskfile `generate-icon` 任务直接使用 `build/windows/info.json` 和 `build/windows/wails.exe.manifest` 模板文件，版本号源统一收敛到 `VERSION` / `wails.json` / `package.json` / `package-lock.json` / `project.nsi` / `build/config.yml` 共 6 个文件。
>
> **已知限制（wails3 v3.0.0-alpha2.106）**：`wails3 generate syso` 的 `-info` 参数直接 `UnmarshalJSON` info.json，**不渲染 `{{.Info.ProductVersion}}` 模板占位符**（源码见 `internal/commands/syso.go`）。因此生成的内嵌 exe 的 ProductVersion 资源为空字符串，这是 0.6.7 以来的既有状态（已验证 0.6.7 / 0.6.8 行为一致），不是打包回归。**对外交付的版本显示不受影响**：NSIS 安装包壳的版本信息由 `project.nsi` 的 `VIProductVersion` / `VIAddVersionKey` 提供（实测 installer 的 ProductVersion 正确显示 0.6.8），Windows 资源管理器"详细信息"标签页看到的就是 installer 壳版本。不要为内嵌 exe 版本为空而重复打包。
14. 运行目标项目适用的验证命令。
15. 用「生产构建」方式构建可执行文件（见下文「生产构建必备构建标签」）。
16. 通过本机或现场冒烟测试启动一次新构建产物，确认 GUI 正常启动、没有"Wails applications will not build without the correct build tags"等明显错误。
17. 验证通过后再执行 NSIS 等安装包封装命令。
18. **归档 installer 到 `releases/bin/`**：执行 `task archive-release`（有 Taskfile 的项目）或直接调用 `scripts/copy-release-artifacts.ps1 -Project <project>`（无 Taskfile 的项目）。详见下文「打包产物归档」。
19. 最终回复必须包含版本号、主要变更、验证结果、产物路径（含归档路径）、是否要求用户卸载旧版本，以及归档 installer 的 SHA-256。

**版本一致性自检（改完 6 个版本文件后必跑）**——历史上多次出现漏改或改错版本号导致打包失败/交付版本错误，打包前必须验证：

```powershell
cd <workspace>
"VERSION:   $(Get-Content projects/<project>/VERSION)"
Select-String -Path "projects/<project>/apps/desktop-wails/wails.json","projects/<project>/apps/desktop-wails/frontend/package.json","projects/<project>/apps/desktop-wails/frontend/package-lock.json" -Pattern '"version"' | Select-Object -First 3 | ForEach-Object { $_.Line.Trim() }
# project.nsi 的 INFO_PRODUCTVERSION（编码见第 10 步）
# build/config.yml 的 info.version
# 以上 6 处必须全部等于目标版本号
```

打包完成后，对 installer 壳版本信息做一次确认（NSIS 壳版本来自 project.nsi，与内嵌 exe 无关）：

```powershell
(Get-Item projects/<project>/releases/bin/<project>-<version>-amd64-installer.exe).VersionInfo.ProductVersion
# 应显示目标版本号；内嵌 exe 的 ProductVersion 为空是已知限制（见上文「已知限制」），属正常
```

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
3. 前后端 Wails 版本锁定一致（见 ADR-004 第 7 节）：Go `wails/v3 v3.0.0-alpha2.106` 必须配 npm `@wailsio/runtime 3.0.0-alpha.94`；不要自行替换为其他 `alpha.*` / `alpha2.*` 组合。
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

## 打包产物归档

每次完成 NSIS 打包后，必须将 installer 归档到项目 `releases/bin/` 目录，便于现场人员自取和历史版本回溯。

### 归档规则

| 项 | 规则 |
|---|---|
| 归档对象 | 仅 NSIS installer（`<project>-<version>-amd64-installer.exe`）；裸 exe 不归档，可随时从源码重建 |
| 归档路径 | `projects/<project>/releases/bin/`（平铺，文件名带版本号天然区分） |
| 命名规则 | 沿用 NSIS 输出文件名，不重命名 |
| SVN 入库 | **不入库**——`releases/bin/` 已加 `svn:ignore`，归档仅本地保留，避免仓库膨胀 |
| 覆盖策略 | 同版本号重复打包时直接覆盖（同名文件强制替换） |

### 归档命令

**有 Taskfile 的项目**（daq-p1604 / daq-t1603 / wind-daq / motion-controller / five-hole-interpolator）：

```powershell
cd projects/<project>/apps/desktop-wails
# 1. 构建 Go 生产二进制 + 前端（含 -tags production、GOWORK=off，均由 Taskfile 固化）
task release
# 2. NSIS 打包（必须传 -DARG_WAILS_AMD64_BINARY 指向 Go 二进制，否则报 "Undefined ARCH"）
cd build/windows/installer
makensis '-DARG_WAILS_AMD64_BINARY=..\..\bin\<project>.exe' project.nsi
cd ..\..\..
# 3. 归档到 releases/bin/
task archive-release
```

> **makensis 调用硬规则**：
> - 必须从 `build/windows/installer/` 目录调用，二进制路径用反斜杠相对路径 `..\..\bin\<project>.exe`。
> - 整个 `-D` 参数用**单引号**包裹，避免 PowerShell 把 `=` 后的内容拆分；不引号包裹时 makensis 会把 `-D...` 中的 `.exe` 当作脚本文件报 `Can't open script ".exe"`（实测踩坑）。
> - 从父目录调用且用正斜杠路径会报 "no files found"。绝对路径可用但必须用引号包裹整个 `-D` 参数（实测可行）。
> - windlabx4 可用一键脚本替代上述 1+2 步：`cd projects/windlabx4; powershell -ExecutionPolicy Bypass -File scripts/build-release.ps1 -WithInstaller`（自动处理 wails build + DLL 复制 + NSIS 打包 + 编码转换）。

**无 Taskfile 的项目**（three-hole-interpolator）：

```powershell
cd projects/<project>/apps/desktop-wails
$env:GOWORK="off"
# 1. 构建 Go 生产二进制
go build -tags production -trimpath -buildvcs=false -ldflags="-w -s -H windowsgui" -o build/bin/<project>.exe .
# 2. NSIS 打包（同样需要 -DARG_WAILS_AMD64_BINARY 参数，见上文硬规则）
cd build/windows/installer
makensis '-DARG_WAILS_AMD64_BINARY=..\..\bin\<project>.exe' project.nsi
cd ../../..
# 3. 归档到 releases/bin/（直接调用统一脚本）
powershell -ExecutionPolicy Bypass -File ../../../../scripts/copy-release-artifacts.ps1 -Project <project>
```

### 统一脚本说明

`scripts/copy-release-artifacts.ps1` 是所有项目共用的归档脚本，支持以下参数：

| 参数 | 说明 |
|---|---|
| `-Project <name[]>` | 必填，项目名（可传多个，空格分隔） |
| `-Version <ver>` | 可选，不传则读取 `projects/<project>/VERSION` |
| `-DryRun` | 预演模式，只打印动作不实际拷贝 |
| `-Quiet` | 静默模式，成功不输出 |

脚本逻辑：
1. 读取 `projects/<project>/VERSION` 解析版本号
2. 查找 `apps/desktop-wails/build/bin/<project>-<version>-amd64-installer.exe`
3. 若不存在则报错（提示先执行 `makensis`）
4. 创建 `releases/bin/` 目录（不存在时）
5. 拷贝并覆盖同名文件

### 首次为项目配置 svn:ignore

`releases/bin/` 目录首次创建后，必须设置 SVN 忽略属性，避免归档文件被误入库：

```powershell
svn propset svn:ignore bin projects/<project>/releases
svn commit projects/<project>/releases -m "chore: ignore releases/bin/ for local artifact archive"
```

若项目使用 Git，将 `releases/bin/` 加入项目根 `.gitignore`：
```text
projects/<project>/releases/bin/
```

### 验证归档结果

归档完成后，最终回复必须包含：

- 归档文件路径：`projects/<project>/releases/bin/<project>-<version>-amd64-installer.exe`
- 文件大小（MB）
- SHA-256（必填，用于现场交付校验；与强制流程第 19 步一致）

```powershell
Get-FileHash projects/<project>/releases/bin/<project>-<version>-amd64-installer.exe -Algorithm SHA256
```

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
