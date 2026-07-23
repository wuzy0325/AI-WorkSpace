# Win7 Sync State

> 本文件是 `AI-Workspace-win7` 与主线之间的同步事实来源。AI 每次同步审查前必须先读取并在完成后更新。

## Baseline

- Last reviewed master: `49f06a8`（包含 Win7 LTS 治理文档）
- Current Win7 base: `b04103361e2c28360033886d7b6d4a60e24f1e27`
- Current branch: `feature/daq-t1603-win7`（基线提交后计划重命名为 `lts/win7`）
- Last reviewed: 2026-07-23
- Review note: 台账初始化；当前 HEAD 不包含 Win7 实现。已验证内容位于未提交 working tree，主要涉及 DAQ-T1603、`shared/device-sdk` 兼容层和 `projects/daq-t1603/apps/desktop-electron/`。首次同步前必须按计划 Task 0 逐项鉴别并固化基线。

## Ported Commits

| Master SHA | Win7 SHA | Project | Method | Verification |
|---|---|---|---|---|
| - | - | - | - | - |

## Manually Ported Commits

| Master SHA | Win7 SHA | Project | Excluded platform changes | Verification |
|---|---|---|---|---|
| - | - | - | - | - |

## Excluded Commits

| Master SHA | Project | Reason |
|---|---|---|
| - | - | - |

## Pending Review

| Master SHA | Project | Priority | Note |
|---|---|---|---|
| - | - | - | 首次正式审查从本文件的 Last reviewed master 之后开始 |

## Verification Records

| Date | Project | Win7 SHA | Automated verification | Win7 hardware/manual result | Artifact SHA256 |
|---|---|---|---|---|---|
| 2026-07-23 | daq-t1603 | uncommitted baseline | Go 1.20 tests/vet/build, frontend typecheck/build, Electron/NSIS smoke | Windows 7 SP1 x64 原始安装包安装与启动通过；重建包待下次真机复核 | `3C74C055237D3585942D707A14A0CEA549EB22308D14F0356A64E9A28E8BED7E` |

## Sync Rules

1. 通用修复先在 `master` 完成。
2. 独立且兼容的提交使用 `git cherry-pick -x <sha>`。
3. 混有 Wails、Go 新 API、依赖升级或重构时，人工移植业务修复并记录来源 SHA。
4. Wails/WebView2/现代 Go 工具链升级默认排除。
5. 不整体 merge `master`，不通过目录复制同步。
6. cherry-pick 无冲突不代表兼容；完成 Go 1.20、前端、打包及必要真机验证后才能标记完成。
