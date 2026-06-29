# AI-Workspace 全面测试计划

> **For Claude:** 本计划以 bite-sized 任务粒度组织；执行时按章节顺序推进，每个任务标注文件路径、命令、期望输出与提交点。优先实现 P0 维度，再补齐 P1。

**Goal:** 为工作区中 Go 后端 + Vue3/Wails 桌面应用 + 多种硬件设备 + 6 个子项目建立一套分层、可自动化、可回归的全面测试体系。

**Architecture:** 沿用工作区六边形架构（core → ports → usecase → adapters → backend/api）与前端 stores/bridge/components 分层；按"测试金字塔"组织，从静态验证到硬件在环(HIL)逐层收敛；硬件抽象层用协议模拟器脱离真机覆盖大部分场景，HIL 仅作发版前最后一道关。

**Tech Stack:** Go 1.25 testing、`-race`、`go cover`、Vitest + jsdom + @vue/test-utils、Playwright、PowerShell 校验脚本、golangci-lint、NSIS 安装包、Wails v3 (`-tags production`)。

**适用范围:** `projects/wind-daq`、`projects/daq-t1603`、`projects/daq-p1604`、`projects/motion-controller`、`projects/five-hole-interpolator`、`projects/three-hole-interpolator`、`shared/`、`programs/`、`device-lab/`。

**参考基线:** `projects/daq-t1603/docs/TEST_PLAN.md`（已有分层与用例编号约定）、`projects/daq-t1603/docs/MANUAL_TEST.md`（黑盒用例模板）、`AGENTS.md`（架构零容忍约束）。

---

## 1. 概述

### 1.1 测试目标

| 维度 | 目标 |
|---|---|
| 功能正确性 | 设备连接/断开、采集启停、数据流推送、配置应用、录制保存、运动控制、插值计算 |
| 状态一致性 | 前端状态与后端真实状态同步，避免"假连接""假采集" |
| 架构合规 | core 零硬件、ports 零实现、usecase 零直接硬件、programs 仅依赖 shared |
| 并发安全 | 多设备并发采集、goroutine 泄漏、channel 关闭、race-free |
| 硬件鲁棒性 | 断连、超时、脏帧、多设备并发、协议差异 |
| 可观测性 | 日志输出、轮转、SSE 推送、组件标签、上下文字段 |
| 发布质量 | Win7/Win11 兼容、安装/升级链路、生产构建标签 |

### 1.2 测试原则

1. **分层 + 跨切面**：先按测试金字塔分层（单元/集成/E2E），再用跨切面（并发/性能/兼容性/HIL）补齐。
2. **能自动化的不靠人**：静态校验、单元、集成、E2E smoke 必须可一键执行并纳入 CI。
3. **硬件可脱机**：协议模拟器覆盖 90% 场景，HIL 仅在发版前执行。
4. **架构边界即守卫**：把 AGENTS.md 的零容忍约束做成可执行测试，而非靠 review。
5. **算法回归有基准**：五孔/三孔插值保存"黄金输入→输出"基准数据，防精度漂移。
6. **TDD 优先**：新增功能/修 bug 必须先写失败测试再实现（参考 `tdd-workflow` skill）。
7. **频繁提交**：每个 bite-sized 任务通过即提交，单次提交聚焦一件事。

### 1.3 优先级定义

| 优先级 | 含义 | 执行时机 |
|---|---|---|
| **P0** | 阻塞发布 | 每次提交前必跑（CI 强制） |
| **P1** | 核心功能 | CI 自动跑或每日跑 |
| **P2** | 一般功能 | 发布前手动跑 |
| **P3** | 边缘场景 | 按需执行 |

---

## 2. 测试分层与策略

### 2.1 测试金字塔

```
┌────────────────────────────────────────────────────────────┐
│  HIL / 现场测试（P0，发版前，依赖真实设备）                  │
├────────────────────────────────────────────────────────────┤
│  E2E 桌面测试（P1，Playwright + smoke 脚本）                │
├────────────────────────────────────────────────────────────┤
│  集成测试（P1，usecase+adapter+config+recording 数据流）     │
├────────────────────────────────────────────────────────────┤
│  单元测试（P0，Go 各层 + 前端 stores/bridge/components）     │
├────────────────────────────────────────────────────────────┤
│  静态验证与架构守卫（P0，validate-structure + 边界测试）      │
└────────────────────────────────────────────────────────────┘
```

### 2.2 跨切面关注点

| 切面 | 何时介入 |
|---|---|
| 并发与资源安全 | 单元层 `-race` + 长时间 hook |
| 性能与稳定性 | 发布前压测 + 30 分钟稳定性 |
| 兼容性与安装 | 发版前 NSIS + Win7/Win11 双跑 |
| 可观测性 | 日志系统随单元/集成一起验 |
| 数据完整性 | 录制格式 + 回放对拍 |

### 2.3 测试工具矩阵

| 类型 | 工具 | 命令样例 |
|---|---|---|
| Go 单元 | `go testing` | `go test ./... -race -cover` |
| Go 覆盖率 | `go cover` | `go tool cover -html=coverage.out` |
| Go lint | `golangci-lint` | `golangci-lint run ./...` |
| 前端单元 | Vitest | `npm test -- --coverage` |
| 前端类型 | tsc | `npm run typecheck` |
| 前端构建 | Vite | `npm run build` |
| E2E | Playwright | `npx playwright test` |
| 架构守卫 | PowerShell | `.\scripts\validate-structure.ps1` |
| 前端结构 | PowerShell | `.\scripts\validate-frontend-structure.ps1` |
| naive 导入 | PowerShell | `.\scripts\check-naive-imports.ps1` |
| 发布构建 | Wails v3 | `task release`（`-tags production`、`GOWORK: off`） |

---

## 3. 测试矩阵（项目 × 维度 × 现状）

### 3.1 项目覆盖矩阵

| 项目 | 后端 Go 单元 | 前端单元 | 集成 | E2E | HIL | 现状说明 |
|---|---|---|---|---|---|---|
| `wind-daq` | 待补 | 待补 | 部分 | smoke 脚本已有 | 必须 | 主线集成项目，覆盖最广 |
| `daq-t1603` | 部分实现 | 待创建 | 1/4 | 用例已备 | 必须 | 已有 TEST_PLAN.md |
| `daq-p1604` | 待补 | 待补 | 待补 | 待补 | 必须 | 新项目，从零搭建 |
| `motion-controller` | 待补 | 待补 | 待补 | 用例已备 | 必须 | 涉及位移机构安全 |
| `five-hole-interpolator` | 待补 | N/A | N/A | N/A | 算法基准 | 纯算法，建黄金基准 |
| `three-hole-interpolator` | 待补 | N/A | N/A | N/A | 算法基准 | 纯算法，建黄金基准 |
| `shared/` | 待补 | 视情况 | 待补 | N/A | N/A | 跨项目复用，重点测 |
| `programs/` | 待补 | N/A | N/A | CLI 测试 | N/A | 仅依赖 shared |

### 3.2 维度优先级与缺口

| # | 测试维度 | 优先级 | 现状 | 缺口/动作 |
|---|---|---|---|---|
| 1 | 静态/结构验证 | P0 | 脚本已具备 | 补架构边界守卫测试 |
| 2 | Go 单元测试 | P0 | 部分实现 | 补齐 TEST_PLAN 待实现项 |
| 3 | 前端单元测试 | P1 | 待创建 | 建 stores/bridge/components 测试 |
| 4 | 算法正确性 | P0 | 缺 | 建黄金基准 + 边界用例 |
| 5 | 集成测试 | P1 | 1/4 | 补 TC-INT-02/03/04 |
| 6 | 硬件抽象层/协议 | P0 | 缺口最大 | 建设备模拟器 + 协议测试 |
| 7 | E2E 桌面测试 | P1 | smoke 起步 | Playwright 自动化 |
| 8 | 并发与资源安全 | P0 | 部分用例 | `-race` 入 CI + goroutine 计数断言 |
| 9 | 性能与稳定性 | P1 | 用例已列 | 30 分钟稳定性 + Web Vitals |
| 10 | 数据完整性与回放 | P1 | 缺 | CSV 校验 + 回放对拍 |
| 11 | 可观测性测试 | P1 | 日志刚完成 | 验日志/SSE 端点 |
| 12 | 兼容性与安装包 | P0 | 流程已定义 | Win7/Win11 + NSIS 升级实测 |
| 13 | 硬件在环/现场 | P0 | 用例已备 | 发版前执行 |

### 3.3 测试用例编号约定

沿用 `daq-t1603/docs/TEST_PLAN.md` 的前缀：
- `TC-CORE-XX`：core 层
- `TC-UC-XX`：usecase 层
- `TC-ADP-XX`：adapters
- `TC-APP-XX`：backend/app 层
- `TC-INT-XX`：集成测试
- `TC-FRONT-STORE/BRIDGE/COMP-XX`：前端各层
- `TC-ALGO-XX`：算法
- `TC-HW-XX`：硬件抽象层
- `TC-E2E-XX`：桌面 E2E
- `TC-CONC-XX`：并发
- `TC-PERF-XX`：性能
- `TC-DATA-XX`：数据完整性
- `TC-OBS-XX`：可观测性
- `TC-REL-XX`：发布/兼容
- `TC-HIL-XX`：硬件在环

---

## 4. 各测试维度详细任务

> 每个维度按 bite-sized 任务粒度给出：文件路径、执行步骤、命令与期望输出、提交点。任务编号沿用 `TC-<维度>-XX`。

### 4.1 维度 1：静态验证与架构守卫（P0）

**目标：** 把 `AGENTS.md` 中的零容忍约束做成可执行测试，并在 CI 中固定运行。

**Files:**
- Create: `scripts/arch-guard.test.ps1`
- Create: `scripts/arch-guard.go`（Go AST 解析器，检测 import 越界）
- Modify: `scripts/validate-structure.ps1`（接入 arch-guard 调用）

#### TC-ARCH-01: core 零硬件/零 I/O/零框架导入检测

**Step 1:** 写失败测试 — 在 `core/` 下故意创建一个 `serial_import_test.go`，`import "go.bug.st/serial"`。
**Step 2:** 运行 `go run ./scripts/arch-guard.go -dir . -rule core-no-hw`，期望：FAIL，列出违规文件与 import。
**Step 3:** 实现 arch-guard：用 `go/parser` + `go/ast` 遍历 `projects/*/core/` 下所有 `.go`，比对 import 前缀黑名单（`serial`、`net`、`os` 文件 I/O、`github.com/wails` 等）。
**Step 4:** 删除测试违规文件，再跑一次，期望：PASS。
**Step 5:** 提交：`feat(arch): add core zero-hardware import guard`。

#### TC-ARCH-02: ports 零实现检测

**Step 1:** 在 `ports/` 下写一个带方法体的 struct，跑 arch-guard `-rule ports-no-impl`。
**Step 2:** 期望 FAIL 并指出文件。
**Step 3:** 实现：检测 `ports/**/*.go` 中是否存在非 interface 的方法体（AST 中 `FuncDecl` 的 `Body != nil` 且接收者类型在 ports 包）。
**Step 4:** 改回 interface，再跑，期望 PASS。
**Step 5:** 提交：`feat(arch): add ports zero-implementation guard`。

#### TC-ARCH-03: usecase 零直接硬件调用

**Files:** 同上扩展规则。
**Step 1-5:** 同模式，黑名单为 `adapters/hardware/*` 的直接 import；usecase 必须经 `ports` 调用。

#### TC-ARCH-04: programs 仅依赖 shared

**Step 1:** 在 `programs/calibrator-cli/` 写一个 import `projects/wind-daq/internal/...` 的文件。
**Step 2:** 跑 `arch-guard -rule programs-shared-only`，期望 FAIL。
**Step 3:** 实现：白名单 `shared/`、标准库、第三方；其余 `projects/*/internal/` 全部禁止。
**Step 4:** 改回合法 import，期望 PASS。
**Step 5:** 提交：`feat(arch): add programs shared-only dependency guard`。

#### TC-ARCH-05: 接入 validate-structure.ps1

**Step 1:** 在 `validate-structure.ps1` 末尾追加调用 `arch-guard` 全部规则。
**Step 2:** 运行 `powershell -File .\scripts\validate-structure.ps1`，期望：原有检查 + 4 条 arch-guard 规则全部 PASS。
**Step 3:** 提交：`chore(scripts): wire arch-guard into validate-structure`。

#### TC-ARCH-06: CI 集成（如有 CI）

**Files:** Modify: `.github/workflows/*.yml` 或 `azure-pipelines.yml`。
**Step 1:** 在 PR 触发 job 中加入 `powershell -File .\scripts\validate-structure.ps1`。
**Step 2:** 提交一个故意违规的 commit 验证 CI 能拦住。
**Step 3:** 回滚违规 commit，CI 绿。
**Step 4:** 提交 CI 配置：`ci: enforce arch-guard on PR`。

---

### 4.2 维度 2：Go 后端单元测试（P0）

**目标：** 补齐 `daq-t1603/docs/TEST_PLAN.md` 中标"待实现"的用例，并向其他项目复制同套分层。

**Files:**（按项目路径前缀）
- `projects/daq-t1603/apps/desktop-wails/usecase/device_usecase_test.go`
- `projects/daq-t1603/apps/desktop-wails/backend/app_test.go`
- `projects/daq-t1603/apps/desktop-wails/adapters/hardware/t1603_adapter_test.go`
- `projects/wind-daq/...`（同构）、`projects/daq-p1604/...`、`projects/motion-controller/...`

#### TC-UC-06: 重复启动采集应报错

**Step 1:** 写失败测试。
```go
func TestDeviceUsecase_DoubleAcquisition(t *testing.T) {
    uc, fake := newUsecaseWithFakeDevice("dev1", true)
    require.NoError(t, uc.StartAcquisition("dev1"))
    err := uc.StartAcquisition("dev1")
    require.Error(t, err)
    require.Contains(t, err.Error(), "already acquiring")
}
```
**Step 2:** `cd projects/daq-t1603/apps/desktop-wails && go test ./usecase/ -run TestDeviceUsecase_DoubleAcquisition -v`，期望 FAIL。
**Step 3:** 在 usecase 中实现状态守卫：`if s.acquiring { return ErrAlreadyAcquiring }`。
**Step 4:** 再跑，期望 PASS。
**Step 5:** 提交：`test(usecase): add double-acquisition guard test`。

#### TC-UC-07: 未连接设备启动采集应报错

**Step 1-5:** 同模式，断言 `ErrNotConnected`。

#### TC-UC-09: 应用配置到未连接设备应报错

**Step 1-5:** 同模式。

#### TC-UC-13/14: 录制多条数据 / 未启动写入忽略

**Step 1-5:** 同模式，断言 `snapshotCount`。

#### TC-APP-13/14/15: relayStream 事件推送

**Files:** `backend/app_test.go`
**Step 1:** 写失败测试，用 mock runtime（`runtime.EventsEmit = func(...) { captured <- payload }`）。
**Step 2:** 跑，期望 FAIL（无事件或格式不符）。
**Step 3:** 实现/修正 `relayStream`，确保 `daq:payload` 与 `daq:device-status` 事件格式正确。
**Step 4:** 再跑 PASS。
**Step 5:** 提交：`test(backend): cover relayStream event emission`。

#### TC-ADP-01 ~ 05: T1603Adapter 行为

**Files:** `adapters/hardware/t1603_adapter_test.go`
**Step 1-5:** 每个用例独立 bite-sized；其中 TC-ADP-03 采集数据流需用"协议模拟器"（见维度 6）作为 fake 连接层。

#### TC-APP-16/17: GetStatusString 与 SyncDeviceStatus

**Step 1-5:** 同模式。

#### TC-UC-WIND-XX: wind-daq 专属用例

**Files:** `projects/wind-daq/...`
**Step 1:** 复制 daq-t1603 的测试模式与 fake 辅助，按 wind-daq usecase 写对应用例。
**Step 2-5:** 同模式。

---

### 4.3 维度 3：前端单元测试（P1）

**目标：** 建立 `src/stores`、`src/bridge`、`src/components`、`src/composables` 的 Vitest 覆盖。

**Files:**（以 daq-t1603 为例，其他项目同构）
- Create: `projects/daq-t1603/apps/desktop-wails/frontend/vitest.setup.ts`
- Create: `src/stores/deviceStore.test.ts`、`recordingStore.test.ts`
- Create: `src/bridge/deviceBridge.test.ts`
- Create: `src/components/device/ChannelCard.test.ts`、`RealtimeChart.test.ts`
- Create: `src/views/MonitorView.test.ts`

#### TC-FRONT-SETUP-01: Vitest 配置与 Wails runtime mock

**Step 1:** 在 `package.json` 加 `vitest`、`jsdom`、`@vue/test-utils` 依赖。
**Step 2:** 写 `vitest.setup.ts`：
```ts
import { vi } from 'vitest'
vi.mock('../../wailsjs/runtime/runtime', () => ({ EventsOn: vi.fn(), EventsOff: vi.fn() }))
vi.mock('../../wailsjs/go/backend/App', () => ({
  GetProfiles: vi.fn(() => Promise.resolve([])),
  Connect: vi.fn(() => Promise.resolve()),
  StartAcquisition: vi.fn(() => Promise.resolve()),
}))
```
**Step 3:** `npm test`，期望：无测试时退出码 0。
**Step 4:** 提交：`chore(frontend): setup vitest with wails runtime mocks`。

#### TC-FRONT-STORE-01 ~ 08: deviceStore/recordingStore 行为

**Step 1:** 每个用例按 TEST_PLAN.md 中已有代码片段写。
**Step 2:** `npm test -- src/stores/deviceStore.test.ts`，期望 FAIL（store 未实现或行为不符）。
**Step 3:** 修正 store 实现（如 `pushSnapshot` 历史限制、`initializeDefaultChartSelection`）。
**Step 4:** 再跑 PASS。
**Step 5:** 提交：`test(stores): cover deviceStore lifecycle and limits`。

#### TC-FRONT-BRIDGE-01/02: bridge 类型与常量

**Step 1-5:** 同模式。

#### TC-FRONT-COMP-01 ~ 04: ChannelCard / RealtimeChart / MonitorView

**Step 1:** `mount(ChannelCard, { props: {...} })`。
**Step 2:** 期望 FAIL。
**Step 3:** 修正组件 props/渲染逻辑。
**Step 4:** PASS。
**Step 5:** 提交：`test(components): cover ChannelCard render and toggle`。

#### TC-FRONT-WIND-XX: wind-daq 专属

**Step 1-5:** 复制同模式，覆盖 wind-daq 的 stores/bridge/components（注意 naive-ui 导入禁令，需通过 `check-naive-imports.ps1`）。

---

### 4.4 维度 4：算法正确性测试（P0）

**目标：** 为五孔/三孔探针插值建立黄金基准与边界用例，防止精度漂移。

**Files:**
- Create: `projects/five-hole-interpolator/testdata/golden/*.json`（输入 + 期望输出）
- Create: `projects/five-hole-interpolator/golden_test.go`
- Create: `projects/three-hole-interpolator/testdata/golden/*.json`
- Create: `projects/three-hole-interpolator/golden_test.go`
- Reference: `docs/内部算法相关说明书/seven_hole_algorithm.html`、`五孔插值新算法.html`

#### TC-ALGO-01: 五孔插值黄金基准

**Step 1:** 选 5~10 组典型工况（不同迎角、侧滑角、马赫数），用现有实现或参考算法生成"已知正确输出"，存为 `testdata/golden/case_01.json`：
```json
{ "input": {"pitch":5,"yaw":0,"mach":0.3,"pressures":[...]},
  "expected": {"alpha":5.0,"beta":0.0,"cp":[...],"precision":1e-6} }
```
**Step 2:** 写失败测试：
```go
func TestFiveHole_Golden(t *testing.T) {
    cases := loadGolden(t, "testdata/golden")
    for _, c := range cases {
        got := fivehole.Interpolate(c.Input)
        require.InDeltaSlice(t, c.Expected.Alpha, got.Alpha, 1e-6)
    }
}
```
**Step 3:** `go test ./... -run TestFiveHole_Golden -v`，期望 PASS（首次跑就是基准）；故意改一个常数验证能 FAIL。
**Step 4:** 还原常数，PASS。
**Step 5:** 提交：`test(algo): add five-hole golden baseline`。

#### TC-ALGO-02: 五孔插值边界用例

**Step 1:** 写边界表：零迎角、最大迎角、负迎角、对称工况、马赫数极值、压力为零/饱和。
**Step 2:** `go test ./... -run TestFiveHole_Boundary -v`，期望 FAIL 或 panic（如有未保护除零）。
**Step 3:** 在算法中加边界保护。
**Step 4:** PASS。
**Step 5:** 提交：`fix(algo): guard five-hole boundary division by zero`。

#### TC-ALGO-03: 五孔插值数值稳定性

**Step 1:** 用 `testing/quick` 或随机扰动 1000 次，断言输出在合理范围且连续。
**Step 2:** 期望 FAIL（如发现 NaN/Inf）。
**Step 3:** 修正。
**Step 4:** PASS。
**Step 5:** 提交：`test(algo): add five-hole randomized stability`。

#### TC-ALGO-04 ~ 06: 三孔插值同套

**Step 1-5:** 同 TC-ALGO-01 ~ 03 模式，文件路径换 `three-hole-interpolator`。

#### TC-ALGO-07: 算法回归基准纳入 CI

**Step 1:** 在 CI job 中加 `go test ./projects/five-hole-interpolator/... ./projects/three-hole-interpolator/...`。
**Step 2:** 提交：`ci: run algorithm golden tests on PR`。

---

### 4.5 维度 5：集成测试（P1）

**目标：** 串通 usecase + adapter + config + recording + relayStream 数据流，验证从前端 EventsOn 到后端 channel 关闭的全链路。

**Files:**
- Create: `projects/daq-t1603/apps/desktop-wails/tests/integration/full_flow_test.go`
- Reference: `projects/daq-t1603/docs/TEST_PLAN.md` §2.4 TC-INT-01~04

#### TC-INT-02: Connect → StartAcquisition → 录制 → Stop 全链路

**Step 1:** 写失败测试，用 fake 硬件端口 + 真 usecase + 真 recording + 真 CSV 临时目录：
```go
func TestIntegration_FullAcquisitionAndRecording(t *testing.T) {
    tmp := t.TempDir()
    uc := newIntegrationStack(t, tmp, fakeDevice{interval: 10*time.Millisecond})
    require.NoError(t, uc.Connect("dev1"))
    require.NoError(t, uc.StartAcquisition("dev1"))
    require.NoError(t, uc.Recording.Start(tmp, "case01"))
    time.Sleep(200 * time.Millisecond)
    require.NoError(t, uc.StopAcquisition("dev1"))
    require.NoError(t, uc.Recording.Stop())
    files, _ := os.ReadDir(tmp)
    require.NotEmpty(t, files)
    // 校验 CSV 行数 > 0
}
```
**Step 2:** `go test ./tests/integration/ -run TestIntegration_FullAcquisitionAndRecording -v`，期望 FAIL。
**Step 3:** 修正集成装配（如 recording 未注入到 usecase、CSV 未刷盘）。
**Step 4:** PASS。
**Step 5:** 提交：`test(integration): full acquisition + recording flow`。

#### TC-INT-03: 多设备并发采集与录制

**Step 1:** 写测试：2 台设备同时采集，各自录制到不同目录，断言互不串数据。
**Step 2-5:** 同模式，提交：`test(integration): concurrent multi-device recording isolation`。

#### TC-INT-04: 设备断连后 relayStream 推送状态事件

**Step 1:** 写测试：采集过程中关闭 fake device channel，断言收到 `daq:device-status` 事件、status="Connected"（非 Acquiring）。
**Step 2-5:** 同模式。

#### TC-INT-WIND-XX: wind-daq 集成

**Files:** `projects/wind-daq/tests/integration/`
**Step 1-5:** 按 wind-daq usecase 编写同套集成用例。

---

### 4.6 维度 6：硬件抽象层 / 协议测试（P0，最大缺口）

**目标：** 为每类设备做协议模拟器，让 SCPI 帧解析、断连重连、超时、错误帧、多设备并发都能脱离真机跑。

**Files:**
- Create: `shared/device-sdk/testing/sim/` 目录（统一模拟器框架）
- Create: `shared/device-sdk/testing/sim/t1603_sim.go`、`p1604_sim.go`、`motion_b140_sim.go`、`press_spc4000_sim.go`、`mps03_sim.go`、`dsa3217_sim.go`
- Create: `projects/daq-t1603/.../adapters/hardware/t1603_protocol_test.go`
- Reference: `device-lab/drivers/*.pdf|*.docx`（协议手册）、`device-lab/skills/*/SKILL.md`

#### TC-HW-SIM-01: 统一模拟器框架

**Step 1:** 在 `shared/device-sdk/testing/sim/` 写 `Simulator` 接口：
```go
type Simulator interface {
    Serve(ctx context.Context, addr string) error      // TCP/串口监听
    InjectFrame(frame []byte)                          // 注入脏帧
    DropNext(n int)                                     // 模拟丢包
    SetLatency(d time.Duration)                         // 模拟延迟
    Close() error
}
```
**Step 2:** 写一个最小 TCP echo 模拟器作为骨架测试。
**Step 3:** `go test ./shared/device-sdk/testing/sim/ -v`，PASS。
**Step 4:** 提交：`feat(sim): add unified device simulator framework`。

#### TC-HW-T1603-01: T1603 SCPI 帧解析正确性

**Step 1:** 写表驱动测试：合法帧（含完整校验和、不同通道数 1/8/16）、非法帧（校验和错、截断、超长）。
**Step 2:** 期望 FAIL（解析器未处理边界）。
**Step 3:** 修正解析器（参考 `DAQ-T-1603指令说明书.docx`）。
**Step 4:** PASS。
**Step 5:** 提交：`test(hw/t1603): scsi frame parsing with golden vectors`。

#### TC-HW-T1603-02: T1603 断连与超时

**Step 1:** 用模拟器 `Close()` 模拟设备掉线，断言 adapter 状态转为 Disconnected 或 Error，且采集自动停止。
**Step 2:** 用 `SetLatency(5s)` 模拟超时，断言返回 `ErrTimeout` 而非阻塞。
**Step 3-5:** 修正 + PASS + 提交：`test(hw/t1603): disconnect and timeout behavior`。

#### TC-HW-T1603-03: T1603 脏帧与丢包

**Step 1:** `InjectFrame([]byte{0x00, 0xFF})` + `DropNext(3)`，断言解析器跳过脏帧、不 panic、最终能恢复。
**Step 2-5:** 同模式。

#### TC-HW-P1604-01 ~ 03: P1604 同套

**Files:** `projects/daq-p1604/.../adapters/hardware/p1604_*_test.go`
**Step 1-5:** 按 `DAQ-P-1604 通讯协议说明书.html` 写同套用例。

#### TC-HW-MOTION-01 ~ 03: 位移机构（DMC-B140 / WTNMC4A）

**Files:** `projects/motion-controller/.../adapters/hardware/b140_*_test.go`
**Step 1:** 模拟器响应 `MOV`、`POS?`、限位触发。
**Step 2:** 测试运动完成回调、限位异常、急停。
**Step 3-5:** 同模式；**特别注意运动安全**：模拟器必须返回"运动到位"事件后才允许下一条指令。

#### TC-HW-PRESS-01 ~ 03: 打压设备（SPC4000）

**Files:** `projects/daq-p1604/.../adapters/hardware/press_*_test.go`（或 `projects/wind-daq/...`）
**Step 1:** 模拟打压曲线（保压、泄压、异常超压）。
**Step 2-5:** 同模式。

#### TC-HW-MPS03-01 / DSA3217-01: 其他设备同套

**Step 1-5:** 同模式，参考各自 `device-lab/skills/*/SKILL.md`。

#### TC-HW-CONC-01: 多设备并发独立线程

**Step 1:** 启动 3 个不同类型模拟器，每个 adapter 用独立 goroutine 访问，断言互不阻塞（响应时延 < 2× 单设备基线）。
**Step 2:** 用 `runtime.NumGoroutine()` 前后对比断言无泄漏（对应 user rule 9）。
**Step 3-5:** 提交：`test(hw): multi-device concurrent access isolation`。

---

### 4.7 维度 7：E2E 桌面测试（P1）

**目标：** 用 Playwright 跑通完整黑盒流程，替代/增强已有 `smoke-ui.py`、`smoke_test_echarts.py`。

**Files:**
- Create: `projects/wind-daq/e2e/playwright.config.ts`
- Create: `projects/wind-daq/e2e/specs/full_flow.spec.ts`
- Reference: `projects/wind-daq/docs/e2e-click-test-2026-06-25.md`、`e2e-issues-2026-06-26.md`、`projects/daq-t1603/docs/MANUAL_TEST.md`（用例模板）

#### TC-E2E-SETUP-01: Playwright 接入 Wails dev server

**Step 1:** `npm i -D @playwright/test`，写 `playwright.config.ts`，`webServer` 指向 `wails dev` 暴露的端口。
**Step 2:** `npx playwright test --list`，期望列出 0 个 spec 退出码 0。
**Step 3:** 提交：`chore(e2e): add playwright config`。

#### TC-E2E-01: 添加设备 → 连接（模拟器）→ 采集 → 录制 → 停止 → 断开

**Step 1:** 写 spec，用前端 mock 替换 Wails binding 指向本地模拟器（或后端启动时注入 fake adapter）。
**Step 2:** `npx playwright test full_flow.spec.ts`，期望 FAIL（某步未通过）。
**Step 3:** 修正对应 bug（参考 `e2e-issues-2026-06-26.md` 已知问题）。
**Step 4:** PASS + 生成截图到 `projects/wind-daq/build/e2e/`。
**Step 5:** 提交：`test(e2e): full device lifecycle flow`。

#### TC-E2E-02: 空状态与错误提示

**Step 1:** 写 spec：无设备时显示空状态；连接错误 IP 时显示错误徽章。
**Step 2-5:** 同模式。

#### TC-E2E-03: 多设备切换

**Step 1:** 写 spec：添加 3 台模拟设备，切换选择，断言波形/数据不串。
**Step 2-5:** 同模式。

#### TC-E2E-04: 主题切换与窗口缩放

**Step 1:** 写 spec：深/浅色切换、最大化/还原、拖拽缩放，截图对比。
**Step 2-5:** 同模式；截图基线存入 `e2e/snapshots/`。

#### TC-E2E-05: smoke 脚本归并

**Step 1:** 把 `scripts/smoke-ui.py`、`smoke_test_echarts.py` 中仍有效的断言迁入 Playwright spec，删除冗余 Python 脚本。
**Step 2:** 确认 `projects/wind-daq/scripts/` 下仅保留构建/度量脚本。
**Step 3:** 提交：`refactor(e2e): consolidate smoke scripts into playwright`。

---

### 4.8 维度 8：并发与资源安全（P0）

**目标：** 把 `-race` 固化进 CI，建立 goroutine 计数断言，杜绝泄漏与死锁。

**Files:**
- Create: `scripts/go-test-race.ps1`（包装 `go test -race -count=1`）
- Create: `projects/*/apps/desktop-wails/tests/leak/goroutine_leak_test.go`
- Reference: `projects/daq-t1603/docs/TEST_PLAN.md` §1.2 并发安全维度

#### TC-CONC-01: relayStream 无泄漏

**Step 1:** 写测试：StartAcquisition → StopAcquisition 循环 100 次，每次记录 `runtime.NumGoroutine()`，断言末值不超过初值 +N。
**Step 2:** 期望 FAIL（如有泄漏）。
**Step 3:** 用 `context.Cancel` + `channel close` 修正 goroutine 退出路径。
**Step 4:** PASS。
**Step 5:** 提交：`test(conc): relayStream goroutine leak assertion`。

#### TC-CONC-02: 多设备并发采集 race 检测

**Step 1:** 写测试：3 台设备同时采集 5 秒，跑 `go test -race`。
**Step 2:** 期望 FAIL（如有 data race）。
**Step 3:** 加锁或改用 channel 通信。
**Step 4:** PASS。
**Step 5:** 提交：`fix(conc): eliminate race in shared state`。

#### TC-CONC-03: channel 关闭安全

**Step 1:** 写测试：在 receiver 还在读时关闭 channel、重复 close、向已关闭 channel 发送，断言不 panic。
**Step 2-5:** 同模式。

#### TC-CONC-04: 每设备独立线程（user rule 9）

**Step 1:** 写测试：启动 N 台设备，每台注入不同延迟，断言总耗时 ≈ max(单台) 而非 sum（证明并行）。
**Step 2-5:** 同模式。

#### TC-CONC-05: -race 进 CI

**Step 1:** `scripts/go-test-race.ps1` 遍历 `projects/*/apps/desktop-wails`、`shared/`、`programs/` 跑 `go test -race -count=1 ./...`。
**Step 2:** 接入 CI。
**Step 3:** 提交：`ci: run go test -race on all modules`。

---

### 4.9 维度 9：性能与稳定性（P1）

**目标：** 验证长时间运行内存稳定、高采样率不丢帧、UI 不卡顿。

**Files:**
- Create: `projects/wind-daq/tests/perf/long_run_test.go`
- Reference: `projects/wind-daq/scripts/measure_web_vitals.py`、`docs/MANUAL_TEST.md` §十 TC-LONG-01/02

#### TC-PERF-01: 30 分钟连续采集内存稳定

**Step 1:** 写测试：启动采集 + 录制，30 分钟后断言 `runtime.ReadMemStats().Alloc` 增量 < 50MB、CSV 行数符合采样率 × 时长。
**Step 2:** 期望 FAIL（如内存持续增长）。
**Step 3:** 排查 history ring buffer、未释放的 snapshot 引用。
**Step 4:** PASS。
**Step 5:** 提交：`test(perf): 30min memory stability`。

#### TC-PERF-02: 历史数据上限 200 条

**Step 1:** 写测试：采集 3000 条（10Hz × 5min），断言 history 长度恒为 200。
**Step 2-5:** 同模式。

#### TC-PERF-03: 高采样率压测

**Step 1:** 模拟器以 100Hz 推送，断言前端 ECharts 不丢帧、CPU < 60%。
**Step 2-5:** 同模式；用 `measure_web_vitals.py` 采集指标。

#### TC-PERF-04: UI 流畅度（Web Vitals）

**Step 1:** 用 `measure_web_vitals.py` 采集 LCP/CLS/INP，断言 LCP < 2.5s、CLS < 0.1、INP < 200ms。
**Step 2-5:** 同模式。

---

### 4.10 维度 10：数据完整性与回放（P1）

**目标：** 验证 CSV 录制格式、追加、断电恢复，以及 `programs/data-replay` 回放与原数据一致。

**Files:**
- Create: `projects/daq-t1603/.../adapters/recording/csv_format_test.go`
- Create: `programs/data-replay/replay_test.go`
- Reference: `projects/daq-t1603/docs/TEST_PLAN.md` TC-ADP-09/10

#### TC-DATA-01: CSV 表头与编码

**Step 1:** 写测试：录制 1 条 snapshot，断言文件首行为表头（`timestamp,CH01,...,CH16`）、UTF-8 BOM 可选、列数匹配通道数。
**Step 2-5:** 同模式。

#### TC-DATA-02: CSV 多段追加

**Step 1:** 写测试：Start→5 条→Stop→Start（同文件）→3 条→Stop，断言共 8 条数据且表头只出现一次。
**Step 2-5:** 同模式，提交：`test(data): csv append without duplicate header`。

#### TC-DATA-03: 断电/异常关闭恢复

**Step 1:** 写测试：录制中 `kill -9` 模拟（`os.Exit` 或 ctx cancel 不走 Stop），重新打开文件断言已写入的行完整、无半行截断。
**Step 2:** 期望 FAIL（如未 flush）。
**Step 3:** 在 recorder 加周期 flush 或写前 buffer。
**Step 4:** PASS。
**Step 5:** 提交：`fix(data): flush csv on abnormal shutdown`。

#### TC-DATA-04: 回放对拍

**Step 1:** 用 `programs/data-replay` 读取录制 CSV，重放到一个 fake sink，断言每个 snapshot 与原 recording 时序、数值一致。
**Step 2-5:** 同模式，提交：`test(data): replay parity with recorded csv`。

#### TC-DATA-05: 大文件回放性能

**Step 1:** 生成 1GB CSV，回放，断言内存占用 < 200MB（流式读取而非全载入）。
**Step 2-5:** 同模式。

---

### 4.11 维度 11：可观测性测试（P1）

**目标：** 验证新日志系统（stderr + 每日文件 + SSE 推送）符合 project_memory 中的约束。

**Files:**
- Create: `projects/wind-daq/.../pkg/logging/logging_test.go`
- Create: `projects/wind-daq/.../pkg/logging/sse_test.go`
- Reference: project_memory（Go 1.25 标准库、WithComponent、WithContext、`data/logs/`、7 天轮转、SSE 端点）

#### TC-OBS-01: 日志组件标签与上下文字段

**Step 1:** 写测试：`logger.WithComponent("traversal").WithContext(sessionID, deviceID).Info("msg")`，断言输出 JSON 含 `component=traversal`、`session_id`、`device_id`。
**Step 2:** 期望 FAIL（字段缺失或拼写错）。
**Step 3:** 修正 logger。
**Step 4:** PASS。
**Step 5:** 提交：`test(logging): component label and context fields`。

#### TC-OBS-02: 每日轮转与 7 天保留

**Step 1:** 写测试：注入 fake clock，跨日写日志，断言生成 `wind-daq-YYYYMMDD.log`；写入 8 天前的日志，断言被清理。
**Step 2-5:** 同模式，提交：`test(logging): daily rotation and 7-day retention`。

#### TC-OBS-03: 仅用 Go 1.25 标准库

**Step 1:** 写测试：扫描 `pkg/logging/` 下所有 `.go` 的 import，断言无第三方日志库（如 `zap`、`zerolog`、`logrus`）。
**Step 2-5:** 同模式，提交：`test(logging): stdlib-only dependency guard`。

#### TC-OBS-04: SSE 端点推送实时日志

**Step 1:** 写测试：GET `/api/log/stream`，断言 `Content-Type: text/event-stream`、能收到后续 `Info` 写入的日志行。
**Step 2-5:** 同模式。

#### TC-OBS-05: /api/log/recent 返回 RingBuffer 历史

**Step 1:** 写测试：写入 N 条日志（N > RingBuffer 容量），GET `/api/log/recent`，断言返回最后 capacity 条、按时间正序。
**Step 2-5:** 同模式。

#### TC-OBS-06: stdlogBridge 重定向旧 log.Printf

**Step 1:** 写测试：调用 `log.Printf("legacy")`，断言被重定向到 slog 且带 `component=legacy` 标签。
**Step 2-5:** 同模式。

---

### 4.12 维度 12：兼容性与安装包（P0）

**目标：** 保证 Win7/Win11 双兼容、NSIS 安装/升级链路、生产构建标签正确。

**Files:**
- Reference: `docs/runbooks/release-versioning.zh-CN.md`、`docs/decisions/ADR-004-wails-v3-production-build.md`
- Create: `projects/wind-daq/scripts/verify-installer.ps1`
- Create: `projects/wind-daq/scripts/verify-upgrade.ps1`

#### TC-REL-01: 生产构建标签正确

**Step 1:** 写脚本检测 `wails build` 命令含 `-tags production`、env 含 `GOWORK: off`。
**Step 2:** 跑 `task release`，断言产物在 `build/bin/`。
**Step 3:** 提交：`test(release): production build tag verification`。

#### TC-REL-02: Win11 全新安装

**Step 1:** 在 Win11 干净虚拟机运行 NSIS 安装包。
**Step 2:** 断言：安装路径正确、桌面快捷方式生成、首次启动无崩溃、日志写入 `data/logs/`。
**Step 3:** 提交测试报告到 `releases/<version>.md`。

#### TC-REL-03: Win7 兼容

**Step 1:** 在 Win7 SP1 虚拟机运行 Win7 版安装包（如 `three-hole-interpolator/交付包_Win7版/`）。
**Step 2:** 断言：能启动、核心功能可用（注意 Win7 缺少某些 Win32 API）。
**Step 3:** 提交报告。

#### TC-REL-04: 从旧版本升级

**Step 1:** 安装上一版（如 0.1.1），配置好设备和数据。
**Step 2:** 不卸载直接运行新版安装包。
**Step 3:** 断言：配置/数据迁移正确、旧日志按规则保留、应用版本号更新。
**Step 4:** 提交报告：`test(release): upgrade from <old> to <new>`。

#### TC-REL-05: 卸载干净

**Step 1:** 安装后用"控制面板"卸载。
**Step 2:** 断言：程序文件删除；用户数据目录按提示保留或删除。
**Step 3:** 提交报告。

#### TC-REL-06: 版本号与 changelog 一致

**Step 1:** 写脚本读取 `VERSION` 文件、`CHANGELOG.md` 首条、`releases/<version>.md` 文件名，断言三者一致。
**Step 2:** 接入 `task release` 前置检查。
**Step 3:** 提交：`test(release): version consistency guard`。

---

### 4.13 维度 13：硬件在环 / 现场测试（P0，发版前）

**目标：** 在真实设备上做最终验证，覆盖模拟器无法还原的物理现象。

**Files:**
- Reference: `projects/daq-t1603/docs/MANUAL_TEST.md`（完整黑盒用例模板）、`device-lab/rigs/`（现场配置）、各 `device-lab/skills/*/SKILL.md`
- Execute: 人工 + 现场

#### TC-HIL-T1603-01: 真机连接与采集

**Step 1:** 按 MANUAL_TEST §二 执行：连接真实 T1603（192.168.3.x:9000），采集 16 通道热电偶。
**Step 2:** 用热水/冰块验证温度合理性（室温~100°C 范围）。
**Step 3:** 填写 MANUAL_TEST 用例表，标记通过/不通过。
**Step 4:** 不通过项回写到 `docs/` issue。

#### TC-HIL-T1603-02: 真机断连恢复

**Step 1:** 采集过程中拔网线 10 秒再插回。
**Step 2:** 断言状态转 Error→Disconnected→可重新连接。
**Step 3:** 填表。

#### TC-HIL-P1604-01: 压力采集真机

**Step 1:** 连接 DAQ-P-1604，按协议说明书配置通道与采样率。
**Step 2:** 用已知压力源（如气压泵）验证读数。
**Step 3:** 填表。

#### TC-HIL-MOTION-01: 位移机构运动安全

**Step 1:** 连接 DMC-B140/WTNMC4A，执行小行程往复运动。
**Step 2:** **安全前置**：先低速、限位内、人在场，验证急停按钮有效。
**Step 3:** 填表；任何异常立即停机并记录。

#### TC-HIL-PRESS-01: 打压设备保压测试

**Step 1:** 连接 SPC4000，按 `device-lab/skills/press-devices/REFERENCE.md` 配置打压曲线。
**Step 2:** 执行保压 5 分钟，断压降在允许范围。
**Step 3:** 填表。

#### TC-HIL-MULTI-01: 多设备联合运行

**Step 1:** 同时连接 T1603 + P1604 + 位移机构，各自采集/运动 10 分钟。
**Step 2:** 断言互不干扰、无资源竞争、日志中各组件标签清晰。
**Step 3:** 填表。

#### TC-HIL-LONG-01: 现场长时间运行

**Step 1:** 现场连续运行 8 小时（一个班次）。
**Step 2:** 记录内存/CPU/磁盘增长、异常日志、是否有崩溃。
**Step 3:** 填表，作为发版签字依据。

---

## 5. 执行计划与里程碑

### 5.1 阶段划分

| 阶段 | 目标 | 主要维度 | 里程碑 |
|---|---|---|---|
| **阶段 0：基础设施** | 搭测试骨架 | 维度 1、6（sim 框架）、3（Vitest 配置） | arch-guard 与 sim 框架可跑 |
| **阶段 1：P0 单元与算法** | 拦住回归 | 维度 2、4、8 | daq-t1603 单元补齐、五孔/三孔黄金基准入库 |
| **阶段 2：硬件抽象层** | 脱机覆盖 90% | 维度 6 全部 | 6 类设备 sim + 协议测试全绿 |
| **阶段 3：集成与 E2E** | 数据流闭环 | 维度 5、7 | 集成全链路 + Playwright full_flow 通过 |
| **阶段 4：跨切面** | 性能/数据/可观测 | 维度 9、10、11 | 30 分钟稳定性 + 回放对拍 + 日志断言 |
| **阶段 5：发布闸门** | 兼容与安装 | 维度 12 | Win7/Win11 + 升级链路通过 |
| **阶段 6：现场验证** | 真机签字 | 维度 13 | HIL 全套用例填写完毕 |

### 5.2 执行节奏

| 节奏 | 任务 | 触发 |
|---|---|---|
| **每次提交** | `validate-structure.ps1` + `go test -race ./...` + `npm run typecheck` + `npm test` + `npm run build` | git pre-commit / CI |
| **每日** | 阶段 2-3 全量集成测试 + Playwright smoke | 定时 CI |
| **每周** | 性能基线（30 分钟短跑版）+ 覆盖率趋势 | 周末 job |
| **发版前** | 维度 9 全套 + 维度 12 全套 + 维度 13 HIL | release 流程触发 |
| **每次 release** | 版本号/changelog 一致性 + NSIS 升级实测 | `task release` 前置 |

### 5.3 CI 流水线（建议）

```
PR 触发:
  job-static:     validate-structure + validate-frontend-structure + check-naive-imports + golangci-lint
  job-go-unit:    go test -race -count=1 -cover ./...（按模块矩阵）
  job-fe-unit:    npm ci && npm test && npm run typecheck && npm run build
  job-algo:       go test ./projects/{five,three}-hole-interpolator/...
  job-integration:go test ./tests/integration/...
  job-e2e:        npx playwright test（仅 full_flow + smoke 子集）
  job-arch-guard: go run ./scripts/arch-guard.go -dir . -all

Nightly:
  job-perf:       30 分钟稳定性 + Web Vitals
  job-long-e2e:   全量 Playwright + 多设备并发

Release:
  job-build:      task release (-tags production, GOWORK: off)
  job-install:    Win11/Win7 双虚拟机安装 + 升级
  job-hil:        手动触发，现场填写 MANUAL_TEST 用例表
```

### 5.4 提交规范（与 `code-standards.zh-CN.md` 对齐）

- 测试新增：`test(<scope>): <what>`
- 测试修正 bug：`fix(<scope>): <what>`，配套测试在同 PR
- 架构守卫：`feat(arch): <rule>`
- CI 接入：`ci: <what>`
- 不通过项：禁止 `--no-verify` 绕过 CI；如确需绕过，PR 描述必须说明并开 follow-up issue。

---

## 6. 验收标准

### 6.1 覆盖率门槛

| 模块 | 单元覆盖率门槛 | 关键路径 |
|---|---|---|
| `core/` | ≥ 90% | 全部 |
| `usecase/` | ≥ 85% | Connect/Start/Stop/ApplyConfig/Recording |
| `adapters/hardware/` | ≥ 75% | 协议解析 + 断连 + 超时 |
| `adapters/recording/` | ≥ 85% | CSV 写入 + flush |
| `backend/` | ≥ 80% | relayStream + status sync |
| `pkg/logging/` | ≥ 85% | WithComponent/WithContext/Rotate/SSE |
| 前端 `stores` | ≥ 80% | pushSnapshot/limit/select |
| 前端 `bridge` | ≥ 70% | 类型导出 + 常量 |
| 算法 | 100% | 黄金基准 + 边界 |

### 6.2 发版门槛（必须全部通过）

- [ ] 所有 P0 维度全绿
- [ ] `go test -race ./...` 无 race
- [ ] goroutine 泄漏断言通过
- [ ] 30 分钟稳定性内存增量 < 50MB
- [ ] Web Vitals: LCP < 2.5s、CLS < 0.1、INP < 200ms
- [ ] Win11 全新安装 + Win7 兼容 + 旧版升级三件套通过
- [ ] HIL 现场用例全部填写且无 P0 不通过项
- [ ] `VERSION` / `CHANGELOG.md` / `releases/<version>.md` 一致
- [ ] 生产构建含 `-tags production`、`GOWORK: off`

### 6.3 不通过项处理

| 严重度 | 处理 |
|---|---|
| P0 不通过 | 阻塞发版；立即修或开 issue + 临时绕过需评审 |
| P1 不通过 | 可带 issue 发版，但下个版本必修 |
| P2 不通过 | 记录入 backlog，按计划修复 |

### 6.4 测试用例维护

- 用例编号一旦分配不可复用；废弃用例标记 `DEPRECATED` 保留历史。
- 黄金基准数据修改需 PR 评审 + 说明数据来源。
- MANUAL_TEST 用例表填写后归档到 `releases/<version>-hil-report.md`。

---

## 7. 附录

### 7.1 命令清单（速查）

```powershell
# === 静态验证 ===
powershell -File .\scripts\validate-structure.ps1
powershell -File .\scripts\validate-frontend-structure.ps1 -ProjectDir "projects/<name>/apps/desktop-wails/frontend/src"
powershell -File .\scripts\check-naive-imports.ps1 -ProjectDir "projects/wind-daq/apps/desktop-wails/frontend/src"

# === Go 测试 ===
go test ./... -race -count=1                       # 单模块
go test ./... -race -cover -coverprofile=c.out    # 带覆盖
go tool cover -html=c.out                           # 覆盖报告
golangci-lint run ./...

# === 前端测试 ===
npm test                                            # vitest run
npm test -- --coverage                             # 带覆盖
npm run typecheck
npm run build

# === E2E ===
npx playwright test
npx playwright test --grep "full flow"

# === 发布 ===
task release                                        # 含 -tags production, GOWORK: off
```

### 7.2 缺口清单（首批需补）

| 优先级 | 缺口 | 对应任务 |
|---|---|---|
| P0 | 架构边界守卫未自动化 | TC-ARCH-01 ~ 06 |
| P0 | 五孔/三孔黄金基准缺失 | TC-ALGO-01 ~ 07 |
| P0 | 设备协议模拟器缺失 | TC-HW-SIM-01 + 各设备 TC-HW-*-01~03 |
| P0 | `-race` 未入 CI | TC-CONC-05 |
| P0 | Win7/Win11 升级链路未实测 | TC-REL-02 ~ 04 |
| P1 | 前端 Vitest 未配置 | TC-FRONT-SETUP-01 |
| P1 | 集成测试 TC-INT-02~04 未实现 | TC-INT-02 ~ 04 |
| P1 | Playwright E2E 未建 | TC-E2E-SETUP-01 ~ 05 |
| P1 | 30 分钟稳定性未跑 | TC-PERF-01 |
| P1 | 回放对拍缺失 | TC-DATA-04 |

### 7.3 风险与应对

| 风险 | 应对 |
|---|---|
| 模拟器与真机行为不一致 | HIL 阶段发现差异时，把差异场景补回模拟器用例，形成闭环 |
| 黄金基准数据本身有错 | 首次入库需 2 人交叉验证 + 标注数据来源 |
| 真机资源紧张排期难 | 把 HIL 用例分优先级，P0 必跑、P1 按设备可用时机跑 |
| Win7 环境稀缺 | 用虚拟机快照固定一份 Win7 SP1 测试镜像 |
| 长时间稳定性占用 CI 资源 | 每晚一次，失败发告警但不阻塞 PR |
| naive-ui 误导入 | `check-naive-imports.ps1` 入 pre-commit |

### 7.4 参考文档

| 文档 | 用途 |
|---|---|
| `AGENTS.md` | 架构零容忍约束 |
| `CLAUDE.md` | 工作区架构与硬约束 |
| `docs/architecture/workspace-engineering-rules.zh-CN.md` | 工程规则 |
| `docs/runbooks/release-versioning.zh-CN.md` | 发布与版本规则 |
| `docs/decisions/ADR-004-wails-v3-production-build.md` | 生产构建标签 |
| `docs/runbooks/frontend-ai-rules.zh-CN.md` | 前端规则 |
| `docs/runbooks/code-standards.zh-CN.md` | 代码与提交规范 |
| `projects/daq-t1603/docs/TEST_PLAN.md` | 分层与用例编号基线 |
| `projects/daq-t1603/docs/MANUAL_TEST.md` | 黑盒用例模板 |
| `device-lab/skills/*/SKILL.md` | 各设备协议与操作 |

### 7.5 执行交接

**本计划已保存至 `docs/plans/2026-06-29-workspace-comprehensive-testing-plan.md`。两种执行方式：**

1. **Subagent-Driven（本会话）** — 我按维度逐个分派子代理实现，每完成一个维度做代码审查后再继续，快速迭代。
2. **Parallel Session（新会话）** — 打开新会话用 `executing-plans` 技能批量执行，带检查点回看。

**建议从阶段 0 开始：先建 arch-guard（TC-ARCH-01~06）和设备模拟器框架（TC-HW-SIM-01），这两件是后续 90% 测试能脱离真机跑的前提。**

告诉我选哪种方式，或先挑某个维度（如算法黄金基准、硬件抽象层模拟器）我直接动手实现。
