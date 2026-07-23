# ADR-008: 独立 Win7 LTS Worktree 产品线

## Date

2026-07-23

## Status

Accepted

## Context

工作空间内 6 个仍在维护的桌面产品需要提供 Windows 7 SP1 x64 版本：`daq-t1603`、`probe-interpolator`、`daq-p1604`、`1604Cal`、`motion-controller` 和 `wind-daq`。

当前主线使用 Go 1.25+、Wails 和 WebView2，并会继续升级。Windows 7 版本必须固定 Go 1.20.14、Electron 22.3.27 和 Chromium 108。若在主线同时维护两套桌面壳和 Go 1.20 兼容依赖闭包，会扩大主线改造范围，并使现有项目长期承受 Win7 的技术限制。

DAQ-T1603 已用平级 Git worktree `AI-Workspace-win7` 的未提交 working tree 完成技术验证，Windows 7 SP1 x64 真机安装和启动通过。当前 HEAD `b041033` 是创建 worktree 时的主线历史点，不包含这些 Win7 改动；必须先鉴别并提交基线，才能形成可审计、可重建的 LTS 起点。因此当前最高优先级是保护主工作空间稳定性，而不是追求最高代码复用率。

## Decision

### 分支与目录

- `AI-Workspace/` 继续检出 `master`，只维护当前 Windows 10/11 产品线。
- `AI-Workspace-win7/` 作为同一 Git 仓库的长期 Win7 worktree，最终分支命名为 `lts/win7`。
- 不把 Win7 Electron 壳、Go 1.20 shim、HTTP bridge 和降级依赖回流 `master`。
- 不将 Win7 目录复制成脱离 Git 历史的独立仓库。
- 当前 `feature/daq-t1603-win7` 在已验证改动提交后迁移或重命名为 `lts/win7`；执行前先确认 worktree 干净并保留基线 tag。

### 产品范围

纳入 Win7 LTS：

1. `daq-t1603`
2. `probe-interpolator`
3. `daq-p1604`
4. `1604Cal`
5. `motion-controller`
6. `wind-daq`

不新增独立 Win7 产品：

- 已由 `probe-interpolator` 取代的 `three-hole-interpolator` 和 `five-hole-interpolator`。
- 作为诊断/应急工具保留的 `daq-t1603-win7-python`。
- 默认不属于 GUI 产品线的 `programs/*` CLI。

### Win7 技术基线

- Windows 7 SP1 x64，不支持 32 位。
- Go 1.20.14，构建时设置 `GOWORK=off` 和 `GOTOOLCHAIN=local`。
- Electron 22.3.27；禁止升级到 Electron 23+。
- Electron 只加载 loopback 本地服务，不加载远程或不受信任网页。
- Electron 强制使用 `contextIsolation: true`、`sandbox: true`、`nodeIntegration: false`、`webSecurity: true`，产品配置不得关闭这些选项。
- Go backend 使用 `net/http`；实时数据按产品选择 SSE、WebSocket 或快照轮询。
- Electron IPC 仅承载文件/目录对话框、外部链接、窗口和退出等桌面能力。
- NSIS 生成 x64 安装包，产物必须记录版本、Git commit、工具链和 SHA256。

### 同步策略

主线是通用 bug 和产品功能的事实来源。Win7 分支由 AI 定期进行选择性同步，不整体 merge `master`，不使用文件覆盖。

同步流程：

1. 读取 `AI-Workspace-win7/WIN7-SYNC-STATE.md` 中的上次审查点。
2. 使用 Git 比较 `<last-reviewed-master>..master` 的提交。
3. 按产品和提交类型分类为直接回移、人工移植、排除、待确认。
4. 独立且兼容的修复使用 `git cherry-pick -x <sha>`。
5. 混有 Wails、Go 新 API、依赖升级或大重构的提交，只移植实际业务修复，并在 Win7 提交信息中记录主线 SHA。
6. 更新同步台账并执行对应项目验证。
7. 硬件协议、DLL、运动、校准和采集改动在发版前做 Win7 真机回归。

提交分类规则：

| 主线提交 | Win7 处理 |
|---|---|
| 算法、协议、数据正确性、业务 bug | 默认同步 |
| UI bug 和仍适用的产品功能 | 按产品状态同步 |
| Wails、WebView2、Go 或现代依赖升级 | 默认排除 |
| 同时包含业务修复和平台重构 | 人工提取业务修复 |
| 已废弃项目变更 | 默认排除 |

### 同步触发时机

- 主线任一目标产品发布新版本时。
- 出现算法、硬件协议、采集、录制或数据正确性修复时。
- Win7 客户需要主线已有功能时。
- 每月一次常规差异巡检。
- 每次 Win7 发布前进行完整差异审查。

常规巡检由维护者发起 AI 同步任务并对结果负责。自动化最多生成只读候选报告，不自动 cherry-pick、提交或发布；如需日历提醒或 CI 定时报告，应单独配置并记录负责人。

## Verification Requirements

每次回移后至少执行目标项目的：

```powershell
# 必须关闭 workspace：根 go.work 需要新 Go，且 daq-t1603 未加入 use 列表。
$env:GOROOT="C:\go-versions\go1.20.14"
$env:PATH="$env:GOROOT\bin;$env:PATH"
$env:GOWORK="off"
$env:GOTOOLCHAIN="local"

go test ./...
go vet ./...
go build -buildvcs=false ./...
```

前端项目还需执行：

```powershell
npm run typecheck
npm run test
npm run build
```

发布前重新生成 NSIS，执行安装后启动、health、页面资源和进程清理 smoke test。涉及实际设备时，自动化测试不能替代 Windows 7 真机验证。

`daq-t1603-win7-python` 作为独立诊断/应急工具保留，不删除、不扩展为统一 Electron 产品，也不计入 6 产品完成标准。

## Alternatives Considered

### 同一主线维护 Wails 与 Win7 Electron 双壳

暂不采用。它能减少同步工作，但需要重构所有项目模块，并把 Go 1.20 兼容约束引入当前工作空间。当前目标更重视主线零侵入。

### 定期整体 merge master

拒绝。两个产品线的 Go 版本、桌面框架、入口和依赖树永久不同，整体 merge 会持续引入无关冲突和不可运行的平台升级。

### 每个产品建立一个 Win7 分支

拒绝。6 条长期分支会放大同步、发布和审计成本。统一 `lts/win7` 分支覆盖所有 Win7 产品。

### 完全复制为另一个仓库

拒绝。失去共同 Git 历史后，AI 无法可靠判断某个修复是否已同步，也无法使用 `cherry-pick -x` 保留来源。

## Consequences

- 主工作空间保持不变，可以继续升级 Go、Wails 和依赖。
- Win7 产品线可独立锁定旧运行时并逐项目迁移。
- 通用修复可能需要主线和 Win7 两次实现/验证。
- 分支差异增大后，直接 cherry-pick 成功率会下降，因此同步台账和主线小提交纪律是强制要求。
- 当长期回移成本明显高于主线双目标 CI 成本时，再重新评估同主线双壳方案。

## References

- `docs/decisions/ADR-007-daq-t1603-win7-lts.md`
- `docs/plans/2026-07-23-workspace-win7-lts-worktree.md`
- `projects/daq-t1603/README.md`
- Electron 官方公告：`https://www.electronjs.org/blog/windows-7-to-8-1-deprecation-notice`
