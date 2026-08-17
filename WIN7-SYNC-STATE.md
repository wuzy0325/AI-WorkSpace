# Win7 Sync State

> 本文件是 `AI-Workspace-win7` 与主线之间的同步事实来源。AI 每次同步审查前必须先读取并在完成后更新。

## Baseline

- Last reviewed master: `481c053`（包含 Win7 LTS 日常开发 Runbook）
- Current Win7 base: `a8de1c2`
- Current branch: `lts/win7`
- Last reviewed: 2026-07-23
- Review note: DAQ-T1603、`shared/device-sdk` Go 1.20 兼容层和 Electron 壳已固化为基线提交 `a8de1c2`。

## Ported Commits

| Master SHA | Win7 SHA | Project | Method | Verification |
|---|---|---|---|---|
| `481c053` | `48a3ac5` | workspace docs | `cherry-pick -x` | `git diff --check` |
| `9142150` | `f571832` | wind-daq | `cherry-pick -x` | 前端 typecheck 通过、vitest 361 tests 全绿 |
| `2e4429e` | `a3ad1da` | wind-daq | `cherry-pick -x`（排除 bindings 产物） | 前端 typecheck 通过、vitest 361 tests 全绿 |

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
| - | - | - | 下次审查从 `481c053` 之后开始 |

## Verification Records

| Date | Project | Win7 SHA | Automated verification | Win7 hardware/manual result | Artifact SHA256 |
|---|---|---|---|---|---|
| 2026-07-23 | daq-t1603 | `a8de1c2` | Go 1.20 tests/vet/build, frontend typecheck/build, Electron/NSIS smoke | Windows 7 SP1 x64 原始安装包安装与启动通过；重建包待下次真机复核 | `3C74C055237D3585942D707A14A0CEA549EB22308D14F0356A64E9A28E8BED7E` |
| 2026-07-23 | wind-daq | (pending commit) | Go 1.20 tests/vet/build, frontend typecheck/test (45 tests)/build, Electron/NSIS smoke | 本机 smoke 通过：/api/health 200，motion-only 8901 /api/health + /api/motion/status 200；Win7 真机验证待执行 | `64866A1D583B4467AE28C37FBE26CCDF039FEE38CA5A8B24881381EB4D2C94C7` |

## Sync Rules

1. 通用修复先在 `master` 完成。
2. 独立且兼容的提交使用 `git cherry-pick -x <sha>`。
3. 混有 Wails、Go 新 API、依赖升级或重构时，人工移植业务修复并记录来源 SHA。
4. Wails/WebView2/现代 Go 工具链升级默认排除。
5. 不整体 merge `master`，不通过目录复制同步。
6. cherry-pick 无冲突不代表兼容；完成 Go 1.20、前端、打包及必要真机验证后才能标记完成。
