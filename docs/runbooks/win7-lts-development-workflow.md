# Win7 LTS 日常开发与同步流程

> 适用仓库：`https://github.com/wuzy0325/AI-WorkSpace`
>
> 架构决策：`docs/decisions/ADR-008-workspace-win7-lts-worktree.md`
>
> Win7 实施计划：`docs/plans/2026-07-23-workspace-win7-lts-worktree.md`

## 1. 分支与工作目录

同一 Git 仓库长期保留两个 worktree，不在同一目录来回切换产品线：

| 目录 | 分支 | 用途 |
|---|---|---|
| `AI-Workspace/` | `master` | 当前 Windows 10/11 产品线，Go 1.25+、Wails、WebView2 |
| `AI-Workspace-win7/` | `lts/win7` | Windows 7 SP1 x64 产品线，Go 1.20.14、Electron 22.3.27 |

远端固定为：

```text
origin = https://github.com/wuzy0325/AI-WorkSpace.git
```

检查状态：

```powershell
git -C .\AI-Workspace worktree list
git -C .\AI-Workspace status --short --branch
git -C .\AI-Workspace-win7 status --short --branch
```

预期分支分别为 `master` 和 `lts/win7`。如果实际分支不符或有无法识别的修改，停止操作并先确认改动所有权。

## 2. 主线日常开发

所有通用功能、算法、协议和 bug 修复先在 `master` 完成。Win7 分支不是通用修复的首发位置。

### 2.1 开始工作

```powershell
cd .\AI-Workspace
git status --short --branch
git fetch origin --prune
git log --oneline --decorate -5
```

开始修改前确认：

- 当前分支是 `master` 或明确的短期功能分支。
- 现有未提交/暂存改动属于自己当前任务。
- 本地和 `origin/master` 的 ahead/behind 状态已知。

仓库存在其他用户或代理并行工作时，不得还原、覆盖或顺带提交其改动。

### 2.2 提交原则

- 一个提交只做一个可描述的修复或功能。
- 业务修复不要混入格式化、目录重构、Wails 升级或依赖批量升级。
- 协议、算法和数据正确性修复必须附回归测试。
- 使用 Conventional Commits，例如：

```text
fix(device-sdk): correct T1603 partial frame handling
feat(daq-p1604): add recording status display
chore(wails): update desktop runtime
```

提交前检查：

```powershell
git status --short
git diff
git diff --cached
git diff --check
```

只暂存本任务文件，避免无差别使用 `git add .`。

### 2.3 推送主线

```powershell
git fetch origin
git rev-list --left-right --count origin/master...master
git push origin master
```

如果远端包含本地没有的提交，停止推送，先审查并采用团队约定的合并方式。禁止对 `master` 强推。

## 3. Win7 日常开发

Win7 专属平台改造只在 `AI-Workspace-win7` 中进行：

```powershell
cd .\AI-Workspace-win7
git status --short --branch
git fetch origin --prune --tags
git rev-list --left-right --count origin/lts/win7...lts/win7
```

Win7 固定约束：

- Windows 7 SP1 x64。
- Go 1.20.14。
- Electron 22.3.27，禁止升级到 23+。
- `GOWORK=off`、`GOTOOLCHAIN=local`。
- Electron 只加载 loopback 本地服务。
- `contextIsolation: true`、`sandbox: true`、`nodeIntegration: false`、`webSecurity: true`。
- 不把 `node_modules/`、`dist/`、编译后的 backend EXE 或安装包提交到 Git。

Win7 专属提交示例：

```text
feat(win7): add DAQ-P1604 Electron shell
fix(win7): close backend process on window exit
docs(win7): record hardware verification
```

## 4. 主线修复同步到 Win7

### 4.1 何时审查

- 主线目标产品发布后。
- 主线出现算法、协议、采集、录制或数据正确性修复时。
- Win7 客户需要主线已有功能时。
- 每月一次例行审查。
- Win7 发布前。

### 4.2 查看未审查提交

先读取：

```text
AI-Workspace-win7/WIN7-SYNC-STATE.md
```

其中 `Last reviewed master` 是上次已审查的主线提交。然后执行：

```powershell
cd .\AI-Workspace
git fetch origin --prune
git log --oneline --decorate <last-reviewed-master>..origin/master
git diff --stat <last-reviewed-master>..origin/master
```

按以下规则分类：

| 主线提交 | Win7 操作 |
|---|---|
| 算法、协议、业务和数据正确性修复 | 默认同步 |
| 独立且兼容 Go 1.20 的提交 | `cherry-pick -x` |
| UI bug 或仍适用的新功能 | 按产品状态同步 |
| 混有 Wails/新 Go API/重构 | 人工移植实际修复 |
| Wails、WebView2、Go 工具链升级 | 默认排除 |
| 已废弃项目改动 | 默认排除 |

### 4.3 直接回移

确保 Win7 worktree 干净：

```powershell
cd .\AI-Workspace-win7
git status --short
git cherry-pick -x <master-commit-sha>
```

`-x` 会将主线来源写入提交信息，便于审计。

如果发生冲突：

```powershell
git status
git cherry-pick --abort
```

先退出冲突状态，再决定是否人工移植。不要为了完成 cherry-pick 而接受主线的 Wails、Go 新 API 或依赖升级。

### 4.4 人工兼容移植

适用于业务修复与平台升级混在同一提交的情况。只实现 Win7 需要的业务变化，提交信息必须记录来源：

```text
fix(<scope>): port <fix> to Win7

Ported from master commit <sha>.
Excluded platform-only changes: <reason>.
```

### 4.5 回滚错误同步

未完成的 cherry-pick：

```powershell
git cherry-pick --abort
```

已形成提交：

```powershell
git revert <win7-commit-sha>
```

禁止使用 `git reset --hard`、强推、覆盖目录或重写远端 `lts/win7` 历史。

## 5. 更新同步台账

每次审查都更新 `WIN7-SYNC-STATE.md`，包括没有需要同步的情况。

必须维护：

- `Last reviewed master`
- `Current Win7 base`
- `Last reviewed`
- 直接 cherry-pick 的主线 SHA 和 Win7 SHA
- 人工移植的来源和排除内容
- 排除提交及原因
- 待处理提交和优先级
- 自动化、安装包和真机验证结果

只有完成审查后才能推进 `Last reviewed master`。不能因为 cherry-pick 无冲突就标记验证完成。

## 6. Win7 验证

### 6.1 Go 1.20

在目标 Go module 目录执行：

```powershell
$env:GOROOT="C:\go-versions\go1.20.14"
$env:PATH="$env:GOROOT\bin;$env:PATH"
$env:GOWORK="off"
$env:GOTOOLCHAIN="local"

go version
go test ./...
go vet ./...
go build -buildvcs=false ./...
```

必须看到 `go version go1.20.14 windows/amd64`。根 `go.work` 需要新 Go，且部分项目不在其 use 列表，所以 Win7 构建必须关闭 workspace。

### 6.2 前端

```powershell
npm run typecheck
npm run test
npm run build
```

如果项目没有测试文件，必须记录“测试集为空”，不能声称测试通过。

### 6.3 Electron 与安装包

以 DAQ-T1603 为例：

```powershell
cd projects\daq-t1603\apps\desktop-electron
node --check main.cjs
node --check preload.cjs
npm run dist:win7
```

打包后验证：

1. unpacked EXE 能启动 backend。
2. `/api/health` 返回成功。
3. 首页、CSS 和 JavaScript 资源返回 HTTP 200。
4. 退出 Electron 后 backend 进程被清理。
5. NSIS 能安装、覆盖安装和卸载。
6. 记录安装包大小和 SHA256。

### 6.4 真机

涉及设备、DLL、运动、校准或采集时，Windows 7 真机验收是发布门禁。记录：

- Windows 版本、SP1、x64 和补丁状态。
- 产品版本和 Win7 commit。
- 设备型号、固件、DLL 和驱动版本。
- 核心操作、异常恢复和长时间运行结果。
- 安装包 SHA256。

## 7. Win7 发布

发布前确认：

```powershell
git status --short
git log --oneline --decorate -5
git rev-list --left-right --count origin/lts/win7...lts/win7
```

提交并推送：

```powershell
git push origin lts/win7
```

创建带注释 tag：

```powershell
git tag -a <product>-win7-v<version> -m "<product> Windows 7 release v<version>"
git push origin <product>-win7-v<version>
```

产物名必须包含 `Win7`，例如：

```text
DAQ-T-1603-Win7-Setup-0.3.3-x64.exe
```

安装包不提交 Git；归档到约定的构建目录或 GitHub Release，并记录 SHA256。

## 8. GitHub 基础引用

当前长期引用：

| Ref | Purpose |
|---|---|
| `master` | 当前产品线 |
| `lts/win7` | Win7 LTS 产品线 |
| `win7-baseline-daq-t1603-0.3.3` | DAQ-T1603 首个已验证源码基线 |

核对远端：

```powershell
git ls-remote --heads --tags origin master lts/win7 "win7-*"
```

## 9. 禁止操作

- 不整体 merge `master` 到 `lts/win7`。
- 不通过复制整个项目目录同步。
- 不在 Win7 分支升级到 Go 1.21+ 或 Electron 23+。
- 不对 `master` 或 `lts/win7` 强推。
- 不使用 `git reset --hard` 清理共享 worktree。
- 不提交 `node_modules/`、`dist/`、安装包、临时数据或凭据。
- 不将其他用户/代理的改动混入当前提交。
- 不因构建成功而跳过 Windows 7 真机设备验证。

## 10. AI 执行检查清单

AI 接到“同步 Win7”任务时依次执行：

1. 读取本 Runbook、ADR-008 和 `WIN7-SYNC-STATE.md`。
2. 检查两个 worktree 状态，不修改未知改动。
3. fetch 远端并列出上次审查点后的主线提交。
4. 给出直接回移、人工移植、排除和待确认分类。
5. 经确认后逐提交同步，不批量整体 merge。
6. 对每个同步提交执行目标项目验证。
7. 更新同步台账和验证记录。
8. 只提交本次相关文件。
9. 推送后核对远端 SHA。
