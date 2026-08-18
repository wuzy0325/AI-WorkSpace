# Win7 Sync State

> 本文件是 `AI-Workspace-win7` 与主线之间的同步事实来源。AI 每次同步审查前必须先读取并在完成后更新。

## Baseline

- Last reviewed master: `795578d`（wispa 文档名称引用同步 + windlabx4 端口占用修复）
- Current Win7 base: `c0d615d`
- Current branch: `lts/win7`
- Last reviewed: 2026-08-18
- Review note: wista、`shared/device-sdk` Go 1.20 兼容层和 Electron 壳已固化为基线提交 `a8de1c2`。

## Ported Commits

| Master SHA | Win7 SHA | Project | Method | Verification |
|---|---|---|---|---|
| `481c053` | `48a3ac5` | workspace docs | `cherry-pick -x` | `git diff --check` |
| `9142150` | `f571832` | WindLabX4 | `cherry-pick -x` | 前端 typecheck 通过、vitest 361 tests 全绿 |
| `2e4429e` | `a3ad1da` | WindLabX4 | `cherry-pick -x`（排除 bindings 产物） | 前端 typecheck 通过、vitest 361 tests 全绿 |
| `795578d` | `4e8f062` | wispa | `cherry-pick -x -X theirs` | docs 名称引用同步 |

## Manually Ported Commits

| Master SHA | Win7 SHA | Project | Excluded platform changes | Verification |
|---|---|---|---|---|
| `9eb77d0` | `c0d615d` | WindLabX4 | Wails `application.Dialog` 弹窗（win7 无 Wails）→ 改在 `main.go` `probeLocalPort` 探测 + 退出码 2 终止，由 Electron 弹窗 | Go 1.20.14 build 通过；冒烟：端口占用时 backend 以退出码 2 提前退出，`/api/health` 200 |

## Excluded Commits

| Master SHA | Project | Reason |
|---|---|---|
| - | - | - |

## Pending Review

| Master SHA | Project | Priority | Note |
|---|---|---|---|
| - | - | - | 下次审查从 `795578d` 之后开始 |

## Verification Records

| Date | Project | Win7 SHA | Automated verification | Win7 hardware/manual result | Artifact SHA256 |
|---|---|---|---|---|---|
| 2026-07-23 | wista | `a8de1c2` | Go 1.20 tests/vet/build, frontend typecheck/build, Electron/NSIS smoke | Windows 7 SP1 x64 原始安装包安装与启动通过；重建包待下次真机复核 | `3C74C055237D3585942D707A14A0CEA549EB22308D14F0356A64E9A28E8BED7E` |
| 2026-07-23 | WindLabX4 | (pending commit) | Go 1.20 tests/vet/build, frontend typecheck/test (45 tests)/build, Electron/NSIS smoke | 本机 smoke 通过：/api/health 200，motion-only 8901 /api/health + /api/motion/status 200；Win7 真机验证待执行 | `64866A1D583B4467AE28C37FBE26CCDF039FEE38CA5A8B24881381EB4D2C94C7` |
| 2026-08-18 | WindLabX4 | `460c6c8` | Go 1.20.14 build 通过；Electron/NSIS 打包通过；双探针路由验证 `/api/traversal/probe1/status` 200（TraversalRegistry 注入修复） | 本机 smoke 通过：/api/health 200；端口占用时 backend 退出码 2；双探针 registry 注入验证通过；Win7 真机验证待执行 | `01BD5AE64AEDF4DE2AB2DE0E2FD7F9BF77CA644EE4B22F278C8466FB92293259` |
| 2026-08-18 | wispa | `559112c` | Go 1.20.14 build 通过；前端 build 通过；Electron/NSIS 打包通过 | 全新安装定位到 `%LOCALAPPDATA%\Programs\WISPA-win7`；backend /api/health 200；Win7 真机验证待执行 | `7093203A15C7D28448881A4141ABB6776884540C4832E80EE0DB3839BAF4B11E` |
| 2026-08-18 | wista | `559112c` | Go 1.20.14 build 通过；前端 build 通过；Electron/NSIS 打包通过 | 全新安装定位到 `%LOCALAPPDATA%\Programs\WISTA-win7`；backend /api/health 200；Win7 真机验证待执行 | `9F6AAE10A520BC4D89C856D8AAF757823790376EA68B6E1D0E7C35690412D1D5` |
| 2026-08-18 | motion-controller | `559112c` | Go 1.20.14 build 通过；前端 build 通过；Electron/NSIS 打包通过 | 全新安装定位到 `%LOCALAPPDATA%\Programs\Motion Controller-win7`；backend /api/health 200；Win7 真机验证待执行 | `8CA3F98948553BDED47238C6D60D20EE24BEDE245F8D5253CEFEA474DF493AB3` |
| 2026-08-18 | probe-interpolator | `559112c` | Go 1.20.14 build 通过；前端 build 通过；Electron/NSIS 打包通过 | 全新安装定位到 `%LOCALAPPDATA%\Programs\Probe Interpolator-win7`；backend /api/health 200；Win7 真机验证待执行 | `AA77D91C722D42AAE5F538E14D3ADCA910314F2629B227525ACEB3245B4479B2` |

## Sync Rules

1. 通用修复先在 `master` 完成。
2. 独立且兼容的提交使用 `git cherry-pick -x <sha>`。
3. 混有 Wails、Go 新 API、依赖升级或重构时，人工移植业务修复并记录来源 SHA。
4. Wails/WebView2/现代 Go 工具链升级默认排除。
5. 不整体 merge `master`，不通过目录复制同步。
6. cherry-pick 无冲突不代表兼容；完成 Go 1.20、前端、打包及必要真机验证后才能标记完成。
