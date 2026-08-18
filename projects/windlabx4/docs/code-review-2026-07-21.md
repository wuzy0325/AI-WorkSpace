# WindLabX4 项目代码 Review 报告

> **生成时间**：2026-07-21
> **修订时间**：2026-07-22
> **审查范围**：WindLabX4 后端 Go 六边形架构、Vue 3 前端与 Wails 桌面壳的静态抽查
> **审查依据**：
> - [工作空间 AGENTS.md](../../../AGENTS.md)
> - [工作空间 CLAUDE.md](../../../CLAUDE.md)
> - [项目级 AGENTS.md](../AGENTS.md)
> - [工作空间工程规则](../../../docs/architecture/workspace-engineering-rules.zh-CN.md)
> **审查方式**：分层静态检索、关键调用链抽查和人工复核
> **结论**：**自动验证完成，发布待 HIL**（Tasks 01-24 自动门禁：通过项+waiver 项。Go build/vet/test/race、前端 typecheck/test/build、import-direction、gitnexus detect_changes 均通过；validate-structure / validate-frontend-structure / 全工作区 git diff --check 因预存工作区问题未通过，已登记为 waiver/blocker，不属本轮整改回归；HIL Release Gate 由具备设备与安全条件的维护人员在发布前执行）

本报告是静态审查与自动验证结果，不等同于完整发布验收。Tasks 01-24 整改已完成，规格 `Commands` 全套命令已执行；剩余 HIL 真机验证为人工门禁。本文不使用"整个项目零违规"等绝对结论。

## 整改进度总览（2026-07-22）

| 级别 | 总数 | Fixed | Closed with Evidence | Pending | Blocked |
|---|---:|---:|---:|---:|---:|
| Critical | 3 | 3 | 0 | 0 | 0 |
| Required | 7 | 7 | 0 | 0 | 0 |
| Optional | 4 | 3 | 1（O-4 recorder 子项归入 R-3） | 0 | 0 |
| Nit | 2 | 1 | 1（N-1） | 0 | 0 |

Pending 项：无（所有 Critical/Required/Optional/Nit 整改项均已 Fixed 或 Closed with Evidence；HIL Release Gate 由具备设备与安全条件的维护人员在发布前执行）。

整体状态：**自动验证完成，发布待 HIL**（HIL Release Gate 由具备设备与安全条件的维护人员在发布前执行，自动测试不得代替 HIL 结论）。

预存工作区问题（不属本轮整改回归，已记录）：
- `validate-structure.ps1` 检出 12 个未在 `workspace.structure.json` 白名单的顶层条目（`.trae`/`.codebuddy`/`.workbuddy`/`.worktrees`/`analysis`/`pdf-qa`/`temp`/`WindLabX4-intro-images`/`WindLabX4-trae-work-prototype`/`.tmp-protocol-remaining.html`/`multi-probe-traversal-design.html`/`W505-Protocol-V2.0.html`）。
- `validate-frontend-structure.ps1 -CheckFileSize` 检出多个预存前端规范问题（i18n 硬编码中文、`: any` 类型注解、泛型变量名等）。
- `gofmt -l .` 列出 40+ 文件需格式化（多为 struct 字段对齐问题），包括 Task 23 改动的两个测试文件中**未由 Task 23 触及**的预存段落；Task 23 改动本身符合 gofmt。
- `git diff --check` 在 Wails 生成 bindings 文件中检出 13 处行尾空白（生成器输出特征）；Task 23 改动的两个测试文件 `git diff --check` 通过。

Follow-up 登记（不在本轮整改范围）：
- **FU-1 triggerMode profile validation 一致性**：`internal/usecase/device_manager.go:1091` 的错误文案对桌面调用可接受，但 profile upsert 路径的 triggerMode 校验与 Start 路径存在一致性差异。本轮不扩大范围，登记为独立后续项。
- **FU-2 seven-hole dynamic point count**：当前七孔 PRB/CSV 导入响应 `pointCount` 保持兼容值（内区 169、外区 52），非 loader 真实计数。真实动态计数语义作为独立后续项（spec Open Question 9）。

## Review Findings 二轮整改（2026-07-22）

> 24 Task 整改完成后，reviewer 对自动门禁结果与运行时契约做了二次抽查，发现 7 项遗漏缺陷（4 项 P1 运行时 + 1 项 P1 文档失实 + 2 项 P2）。本节记录二轮整改结果。

| ID | 级别 | 问题 | 状态 | 证据 |
|---|---|---|---|---|
| P1-1 | P1 95% | DataStreamRelay.Stop() 可能在订阅尚未注销时提前返回 | **Fixed** | `stream_relay.go` 新增 retiring 集合，Unsubscribe 锁外等 sub.done 期间移入 retiring，Stop 遍历 subs+retiring 合并等待；新增 2 个并发契约测试 |
| P1-2 | P1 95% | 轮询重启不能隔离旧请求，旧状态可覆盖新任务 | **Fixed** | `calibrationStore.ts` 新增 pollingGeneration token，start/stop 自增，响应返回时比对 generation 丢弃过期响应 |
| P1-3 | P1 92% | 终态仍计算并返回 livePhysics，前端永久保留最后一帧 | **Fixed** | `calibration.go` Status() 只在 running/paused 调 resolveLivePhysics；新增 TestCalibrationStatusPhysics_TerminalStatesSkipLivePhysics + RunningAndPausedIncludeLivePhysics |
| P1-4 | P1 90% | 五孔 tTunnel 配置被前端强制要求，但后端权威 physics 完全忽略 | **Fixed** | types.go/read_probe_channels.go/formulas.go/calibration.go 四处补齐 TTunnel 字段+roleMap+平均值+传参 |
| P1-5 | P1 98% | 文档把未通过的自动门禁标为完成 | **Fixed** | spec.md Verification Gate 两项 [x]→[ ]+【Blocked / Waiver】；tasks.md Task 24 acceptance 加 waiver 标注；review.md 结论改为"通过项+waiver 项" |
| P2-6 | P2 92% | 带括号注释的 skip 列不会执行反转 | **Fixed** | `pointsFileParser.ts` invert 判定改用 stripBracketComments 后的 normalized base；新增 4 个中英文/多括号 skip 列测试（17a-17d） |
| P2-7 | P2 90% | HTTP pause/resume 失败仍没有操作员可见反馈 | **Fixed** | `useCalibrationWorkflow.ts` pause/resume 失败路径新增 toast（与 stop 路径一致）；i18nStore 新增 wf_pauseCalibrationFailed/wf_resumeCalibrationFailed 中英文 key |

二轮整改验证（2026-07-22）：
- `go build -buildvcs=false ./...` → exit 0
- `go vet ./...` → exit 0
- `go test ./internal/... ./api/...` → 全 PASS（usecase 71.7s）
- `go test -race ./internal/usecase/... ./api/...` → 全 PASS（usecase 74.0s + api 1.1s）
- `npm run typecheck` → exit 0
- `npm run test -- --run` → 18 files / 248 tests 全 PASS（含 P2-6 新增 4 个测试）
- `npm run build` → exit 0
- `validate-import-direction.ps1` → exit 0（699 Go files checked）
- `npx gitnexus detect-changes --repo AI-WorkSpace` → exit 0（改动范围与 7 项 review findings 整改一致）
- 本轮整改改动的源文件 `git diff --check` → exit 0（干净，无行尾空白）
- 预存问题仍为 waiver/blocker：`validate-structure.ps1` exit 1（12 个未知顶层条目）、`validate-frontend-structure.ps1` exit 1（i18n/any 预存问题）、全工作区 `git diff --check` exit 2（Wails 生成 bindings 13 处行尾空白）

---

## 目录

- [问题汇总](#问题汇总)
- [Critical 问题（3 项）](#critical-问题3-项)
- [Required 问题（7 项）](#required-问题7-项)
- [Optional 问题（4 项）](#optional-问题4-项)
- [Nit 问题（2 项）](#nit-问题2-项)
- [静态抽查未发现的问题](#静态抽查未发现的问题)
- [工程亮点](#工程亮点)
- [修复优先级路线图](#修复优先级路线图)
- [验证状态与限制](#验证状态与限制)

---

## 问题汇总

| 级别 | 数量 | 主要问题 |
|---|---:|---|
| Critical | 3 | API 绕过 usecase 调 adapter、生产前端计算气动参数、急停 fallback 错误丢失 |
| Required | 7 | demo 算法被生产入口引用、结构化日志不足、部分错误缺少诊断、前端算法 fallback、API 直调 core |
| Optional | 4 | API 类型耦合、goroutine 生命周期、测试占位 |
| Nit | 2 | 错误文案、括号剥离正则 |

---

## Critical 问题（3 项）

### C-1 api/ 层系统性绕过 usecase 直接调用 adapter

**位置**：`services/api-go/api/server.go`

| 行号 | 直接依赖或调用 |
|---:|---|
| 17 | `internal/adapters/config` |
| 18 | `internal/adapters/interpolation` |
| 235 | `configadapter.DecodeCalibrationConfig(data)` |
| 392 | `interpfiles.LoadPrbFile(body.FilePath)` |
| 411 | `interpfiles.LoadFiveHoleNewFile(body.FilePath)` |
| 439 | `interpfiles.LoadMultiPrbFiles(...)` |
| 488 | `interpfiles.LoadSevenHolePrbFiles(...)` |
| 536 | `interpfiles.LoadSevenHoleCalibrationCsvFiles(...)` |

**问题本质**：路由层承担了文件加载、插值器构造和 usecase 状态注入，形成 `api -> adapters` 编译期依赖，并把“加载 + 设置”这一业务操作拆散在路由中。

**合规修复方案**：

1. 在 `internal/ports` 定义校准配置解码和插值文件加载所需接口。
2. 由现有 `internal/adapters/config`、`internal/adapters/interpolation` 实现这些 port。
3. 在 composition root 将实现注入 `CalibrationManager` / `TraversalManager`。
4. 在 usecase 新增 `StartRawCalibration`、`ImportPrbFile`、`ImportMultiPrb` 等操作，usecase 只依赖 port，不得 import adapter。
5. API 只解析 HTTP 基础参数、调用 usecase 并映射响应，删除两个 adapter import。

> 禁止把 `interpfiles.LoadXxx` 或 `configadapter.DecodeCalibrationConfig` 直接搬进 usecase；这会形成同样属于零容忍违规的 `usecase -> adapters` 依赖。

**状态（2026-07-22）**：**Fixed**（Task 08 + Task 09）。API 不再 import `internal/adapters/config` 或 `internal/adapters/interpolation`；五类显式插值导入（单 PRB、五孔 CSV、多 PRB、七孔 PRB、七孔 CSV）下沉到 `TraversalManager.ImportXxx`；实时计算走 `CalculateRealtimeForAPI` + `ProbePressureInput` transport-neutral DTO。
**证据**：`go test ./internal/usecase ./api -run 'Test.*Import.*(Prb|PRB|Csv|CSV)|Test.*Calculate.*Realtime' -v` 全 PASS；`rg 'calibration_config_decoder|configadapter.DecodeCalibrationConfig' services/api-go` 无生产引用。

---

### C-2 生产前端实现马赫数、静温和流速公式

**位置**：`apps/desktop-wails/frontend/src/stores/calibrationStore.ts:40`

`calculateAtmosphericPhysics` 在生产 store 中实现等熵马赫数、静温恢复和流速公式，并由多个校准页面消费。项目规则要求 Vue 前端不得包含校准或遍历业务算法；“仅用于 UI 显示”不改变该逻辑的业务属性，也造成前后端公式双实现漂移风险。

**修复建议**：后端通过校准状态、采集快照或专用 usecase 返回 `machNumber` / `velocity`。前端仅显示后端值；缺失时显示占位符，不保留公式 fallback。

**状态（2026-07-22）**：**Fixed**（Task 12 + Task 13 + Task 14 + Task 15）。后端 `calibration.Status` 新增可选 physics 快照（`MachNumber`/`Velocity` 等指针字段，区分缺失与有效零）；HTTP/Wails 共用同一载体；前端 `calibrationStore` 删除 `calculateAtmosphericPhysics` 与 `ATM_GAMMA`/`ATM_C_COEFF` 常量，改为消费后端 status physics；HTTP 模式新增非重叠 calibration status 轮询。
**证据**：`rg 'ATM_GAMMA|ATM_C_COEFF|calculateAtmosphericPhysics' apps/desktop-wails/frontend/src` 无生产引用；`npm run typecheck` + `npm run test` + `npm run build` 全绿。

---

### C-3 急停 fallback 失败被吞

**位置**：`services/api-go/internal/usecase/calibration.go:1101`

急停失败后执行 `_ = runtime.StopMotion()`。若普通停止也失败，调用方、状态快照和日志均无法看到第二个错误。

**修复要求**：

- 捕获 `fallbackErr`，使用 `errors.Join` 或清晰的包装错误同时保留急停和普通停止错误。
- 保留现有 `failWithCode` 和 `recordMotionSafetyFailure` 调用，确保前端仍能获得结构化错误码和现场快照。
- 增加测试，覆盖“急停失败但 fallback 成功”和“两次停止均失败”两条路径。

示意代码仅表达错误聚合，不应替换现有状态更新逻辑：

```go
fallbackErr := runtime.StopMotion()
if fallbackErr != nil {
    stopErr = errors.Join(stopErr, fmt.Errorf("fallback stop failed: %w", fallbackErr))
}
```

**状态（2026-07-22）**：**Fixed**（Task 01）。`handleCalibrationMotionSafetyFailure` 使用 `errors.Join` 聚合急停与 fallback stop 错误；`failWithCode` 消息含两个根因；`recordMotionSafetyFailure` 快照完整保留；新增 3 个测试覆盖急停成功/急停失败 fallback 成功/双失败路径。
**证据**：`go test ./internal/usecase -run 'TestHandleCalibrationMotionSafetyFailure' -v` 全 PASS；`errors.Is(err, errSentinelEmergencyStop) && errors.Is(err, errSentinelFallbackStop)` 双 cause 链断言通过。

---

## Required 问题（7 项）

### R-1 demo 算法文件被生产组件导入

**位置**：

- `apps/desktop-wails/frontend/src/utils/simulateFiveHoleCalibration.ts`
- `apps/desktop-wails/frontend/src/utils/simulateTraversalRun.ts`
- `apps/desktop-wails/frontend/src/composables/useTraversalSimulation.ts:20`
- `apps/desktop-wails/frontend/src/components/traversal/TraversalMain.vue:43`

两个 `simulate*` 文件已位于规则允许的路径并带 `DEMO ONLY` 文件头，因此“demo 文件含算法”本身不是违规。实际问题是生产组件通过 composable 导入了这些文件，不满足“无生产 feature 导入 demo 算法”的例外条件。

**修复建议**：切断 `TraversalMain.vue` 的生产导入链，将模拟入口置于独立 demo 构建入口或构建时明确排除的模块中。仅移动到 `__demo__/` 或在运行时增加 `if (import.meta.env.DEV)` 不足以证明生产 bundle 不包含该代码；应由构建配置和结构校验共同保证。

同时补强 `scripts/validate-frontend-structure.ps1`：识别 `DEMO ONLY` / `simulate*` 文件，并阻止生产目录对其进行静态导入。

**状态（2026-07-22）**：**Fixed**（Task 16 + Task 17）。删除 `useTraversalSimulation.ts` / `simulateTraversalRun.ts` / `simulateFiveHoleCalibration.ts` 及 169 点数据集；`TraversalMain.vue` 移除生产导入；`scripts/validate-frontend-structure.ps1` 新增 demo 导入守卫，阻止生产静态导入 `DEMO ONLY`/`simulate*` 模块。
**证据**：`npm run typecheck` + `npm run test` + `npm run build` 全绿；`rg -l '98924\.2|Simulation 16.{0,16}16' apps/desktop-wails/frontend/dist` 无输出；fixture script 验证生产→demo 导入报错。

---

### R-2 calibration.go 19 处 log.Printf 未迁移 slog

**位置**：`services/api-go/internal/usecase/calibration.go`

**行号清单**：209 / 214 / 315 / 323 / 424 / 433 / 482 / 655 / 667 / 672 / 676 / 790 / 1007 / 1051 / 1088 / 1100 / 1111 / 1530 / 1572

当前日志可通过 stdlog bridge 进入统一日志系统，但 task ID、设备、轴、错误码等信息被插入 message，无法按结构化字段检索。应按事件严重度改用 `slog.Info/Warn/Error`，并加入稳定的 `component`、`task_id`、`controller_id`、`axis`、`error` 等字段。

**状态（2026-07-22）**：**Fixed**（Task 22）。`calibration.go` 全部 22 处 `log.Printf` 迁移为 `slog.Error/Warn/Info`，每处带稳定事件名 + 独立字段（`component`/`task_id`/`controller_id`/`axis`/`verdict`/`error` 等）；factory 缺失与 SavePath 空拆分为 Error/Info；新增 3 个 slog 断言测试（factory missing、empty SavePath、motion safety failure）。
**证据**：`rg -n 'log\.Printf' services/api-go/internal/usecase/calibration.go` 无匹配；`go test ./internal/usecase -run 'TestHandleCalibrationMotionSafetyFailure|TestBuildSevenHoleCsvSink' -v` 全 PASS。

---

### R-3 部分清理错误缺少诊断信息

| 位置 | 当前行为 | 建议 |
|---|---|---|
| `internal/usecase/calibration.go:590` | Append 失败后的清理 `Flush` 错误被忽略 | 将 Flush 错误与原错误聚合或记录 warning |
| `internal/usecase/calibration.go:727,733` | 丢弃临时 writer 时忽略 `Flush` 错误 | 记录路径/key 和错误，避免文件句柄问题不可诊断 |
| `internal/usecase/traversal.go:707,927,1119` | 忽略资源锁 Release 错误 | 记录 task ID；该错误通常表示 holder 不匹配，不应表述为必然“锁泄漏” |
| `internal/usecase/traversal_view.go:171` | 忽略资源锁 Release 错误 | 同上 |

`data_sink.go` 对 `StorageRecorder.HandlePayload` 返回值的忽略不属于静默吞错：`HandlePayload` 已在错误发生处使用 `slog.Error` 记录。除非后续证明现有错误通道或状态暴露不足，不应在热路径重复记录同一错误。

**状态（2026-07-22）**：writer/lock 清理错误项 **Fixed**（Task 20 + Task 21）；recorder 子项 **Closed with Evidence**。
- Task 20：`calibration_csv_writer.go` `AppendPoint` 新增 `csv.Writer.Error()` 检查；`Flush` 用 `errors.Join` 聚合 buffered 与 close 错误；`SaveCsv` 用 `errors.Join` 聚合 append 与 cleanup flush 错误；`routeSevenHoleWriter` double-check 丢弃路径记录 `slog.Warn`。
- Task 21：提取 `traversalLockService` 接口；4 条 Release 路径全部处理（Start rollback/Stop 可返回路径用 `errors.Join`；abort/finalize void 路径用 `slog.Warn`；finalize 失败不再记录成功 Info）。
- recorder 子项：`StorageRecorder.HandlePayload` 已在错误发生处 `slog.Error` 记录，data sink 重复记录属热路径噪声，不新增。
**证据**：`go test ./internal/usecase ./internal/adapters/storage -run 'Test.*Calibration.*(Csv|CSV|Writer|Flush)|TestSaveCsv|TestRouteSevenHoleWriter|Test.*(Release|Traversal.*Stop|FinalizeSink)' -v` 全 PASS；`rg -n '_ = resourcelock\.Default\(\)\.Release|_ = m\.lockService\.Release' services/api-go/internal/usecase` 无生产引用。

---

### R-4 前端五孔蛇形布点算法本地 fallback

**位置**：`apps/desktop-wails/frontend/src/components/calibration/five-hole/motionCalibrationUtils.ts:6`

`generateFiveHoleSnakePoints` 在后端调用失败或返回空数组时静默切换到本地蛇形布点算法。该 fallback 既复制后端业务逻辑，也会掩盖后端不可用或响应异常。

**修复建议**：删除本地算法和空结果 fallback；后端失败时向 UI 返回可操作错误。若确有离线 demo 需求，应走 R-1 所述的独立 demo 构建入口。

**状态（2026-07-22）**：**Fixed**（Task 10 + Task 11）。新增 `CalibrationManager.PreviewFiveHolePoints`，HTTP `/api/calibration/fivehole` 委托 usecase；前端 `motionCalibrationUtils.ts` 删除 `generateFiveHoleSnakePoints` 本地 fallback，后端失败显式反馈错误。
**证据**：`go test ./internal/usecase ./api -run 'Test.*FiveHole.*Preview|Test.*FiveHole.*Snake' -v` 全 PASS；`rg 'generateFiveHoleSnakePoints' apps/desktop-wails/frontend/src` 无生产引用。

---

### R-5 前端复刻 MotionSafety 跨字段校验

**位置**：

- `apps/desktop-wails/frontend/src/components/shared/MotionSafetyPanel.vue:166`
- `apps/desktop-wails/frontend/src/shared/types/traversal.ts:1342`

前端校验可作为即时 UX 提示，但后端必须保持唯一权威。应明确标注非权威语义，确保提交时始终由后端重新校验，并为前后端规则漂移增加契约测试。`isMotionSafetyEmergency` / `isMotionSafetyFailure` 属于展示分类，可保留，但不应参与控制决策。

**状态（2026-07-22）**：**Fixed**（Task 05 + Task 18）。Task 05 修复 DTO 携带 MotionSafety 到达后端，backend Start 权威拒绝已生效；Task 18 完成前端 blocking vs advisory 拆分、override 非递归类型、unknown key 可见、新增前后端契约测试。
**证据**：
- 后端：`go test ./internal/usecase/ -run TestValidateMotionSafetyConfig -v` → 17/17 PASS（含 5 个 Task 18 新增：B3 零/负 critical、B6 零/负 epsilon、B5 边界 = 200、B1 Inf、advisory 边界确认后端不拒绝）。
- 前端单测：`npm run test -- MotionSafetyPanel` → 21/21 PASS（B1-B9 blocking + A1-A2 advisory + B8 编译期 `@ts-expect-error`）。
- 前端全量：`npm run test -- --run` → 18 files / 244 tests 全 PASS。
- `npm run typecheck` → exit 0；`npm run build` → exit 0。
- `MotionSafetyPanel.vue`：`blockingErrors` 与后端 `validateMotionSafetyConfig` 拒绝规则一一对齐；`advisoryWarnings` 仅 UX 提示；`isValid` 仅由 `blockingErrors` 决定；`defineExpose({ isValid, blockingErrors, advisoryWarnings })`。
- `traversal.ts`：`MotionSafetyAxisOverride` 非递归子集类型（无 `axisOverrides` 字段）+ `MotionSafetyFieldKey` 联合类型；B8 嵌套覆盖在编译期被类型系统阻止。
- `TraversalSettings.vue`：`saveConfig()` 使用 `blockingErrors[0]` 作为 toast 文案，与面板语义一致。

---

### R-6 api/ 直接调用 core 算法函数

**位置**：`services/api-go/api/server.go`

| 行号 | 当前调用 |
|---:|---|
| 445 | `interpolator.SetInterpolationMode(...)` |
| 1280 | `calibration.GenerateFiveHoleSnakePoints(body)` |

API 应只做传输层参数转换并委托 usecase。插值模式设置应并入 C-1 的 `ImportMultiPrb` usecase 操作；五孔点位生成应由 `CalibrationManager` 暴露方法，与已有七孔预览路径保持一致。

**状态（2026-07-22）**：**Fixed**（Task 08 + Task 10）。`api/server.go` 不再调用 `interpolator.SetInterpolationMode`（并入 `ImportMultiPrb` usecase 操作）；`calibration.GenerateFiveHoleSnakePoints` 调用替换为 `CalibrationManager.PreviewFiveHolePoints`。
**证据**：`rg 'SetInterpolationMode|GenerateFiveHoleSnakePoints' services/api-go/api` 无生产引用；`go test ./api/... -v` 全 PASS。

---

### R-7 结构校验未实际跑通

报告生成时 `validate-structure.ps1` 未完成，Grep 只能辅助定位，不能替代脚本中的全部规则。修复完成后必须执行工作空间结构校验和项目级前后端验证；在此之前，不应将静态抽查结果用于发布放行。

**状态（2026-07-22）**：**Fixed**（Task 24）。规格 `Commands` 全套命令已执行，结果记录在本报告"验证状态与限制"表与下方证据。后端 Go build/vet/test/race 与前端 typecheck/test/build 全绿；import-direction 通过；GitNexus detect_changes 与批准计划一致。3 项预存工作区问题（结构校验未知顶层条目、前端 i18n/any 检查、Wails 生成 bindings 行尾空白）不属本轮整改回归，已分别记录。
**证据**：
- `validate-import-direction.ps1` → exit 0（699 Go files checked）
- `go build -buildvcs=false ./...` → exit 0
- `go vet ./...` → exit 0
- `go test ./internal/... ./api/...` → 全 PASS
- `go test -race ./internal/usecase/... ./api/...` → 全 PASS（usecase 73.9s + api 1.1s）
- `npm run typecheck` → exit 0（vue-tsc --noEmit）
- `npm run test` → 17 files / 223 tests 全 PASS
- `npm run build` → exit 0（built in 18s）
- `npx gitnexus detect-changes --repo AI-WorkSpace` → 63 files / 405 symbols / 52 flows / critical（与 Tasks 01-23 批准计划一致；critical 风险源于本轮大面积整改，已逐项人工复核）
- Task 23 改动的两个测试文件 `git diff --check` → exit 0（无行尾空白）；Wails 生成 bindings 中的行尾空白是生成器输出特征，非本轮回归
- Wails bindings 未重新生成：Task 23 仅删除测试 `var _` 占位 + 文档更新，未触及 Wails 绑定签名

---

## Optional 问题（4 项）

### O-1 api/ 直接依赖算法库输入类型

**位置**：`services/api-go/api/server.go:612`、`services/api-go/api/server.go:630`

API 直接构造五孔和七孔算法库的 `InterpolationInput`。建议由 usecase 暴露传输无关的基础输入结构或基础参数方法，再在 usecase 内转换为算法类型，减少路由对算法库签名的耦合。

**状态（2026-07-22）**：**Fixed**（Task 09）。新增 `usecase.ProbePressureInput` transport-neutral DTO + `CalculateRealtimeForAPI` dispatch 方法；API 不再 import `coreinterp`/`seveninterp` 来构造 `InterpolationInput`。
**证据**：`rg 'coreinterp|seveninterp' services/api-go/api` 无生产引用；`go test ./internal/usecase ./api -run 'Test.*Calculate.*Realtime' -v` 全 PASS。

---

### O-2 calibration autoEngine goroutine 缺少可等待的生命周期

**位置**：`services/api-go/internal/usecase/calibration.go:285`

goroutine 目前依赖 `autoEngine.Stop()` 间接退出，manager 没有等待机制。建议先确认 `autoEngine.Start` 的取消契约和 manager 的实际 shutdown 调用链，再决定是否引入 context 与 WaitGroup；不能只加 WaitGroup 后无超时等待，否则可能把现有泄漏风险转换为 shutdown 永久阻塞。

**状态（2026-07-22）**：**Fixed**（Task 02 + Task 03）。Task 02 为 engine 自有等待路径增加 context 取消（dwell/pause/gate/fresh-data wait 均可及时退出）；Task 03 引入 `calibrationRunSession`（捕获 engine/config/taskID/cancel/done）+ 5 秒有界 join + 旧任务资源隔离 + replacement 拒绝；Desktop/API server shutdown 停止并等待其 calibration session。
**证据**：`go test -race ./internal/usecase -run 'TestCalibration.*(Lifecycle|Stop|Session|Status)' -v` 全 PASS；`go test ./internal/core/calibration -run 'Test.*(Cancel|Pause|Automatic)' -v` 全 PASS。

---

### O-3 DataStreamRelay Stop 不等待订阅 goroutine 完成

**位置**：`services/api-go/internal/usecase/stream_relay.go:80`

`Stop` 只触发 cancel，不确认每个 goroutine 已执行 `unsub()`。当前发送 `r.payloads <- payload` 与 `<-ctx.Done()` 位于同一个 select，不会因为 payload channel 无接收者而永久阻塞；因此这不是已证实的 goroutine 卡死。

若 shutdown 后续步骤要求所有 hub 订阅已经注销，可增加 WaitGroup 或 completion channel，并避免在持有 `r.mu` 时等待 goroutine。

**状态（2026-07-22）**：**Fixed**（Task 04）。subscription 记录含 cancel/done，done 仅在 `unsub()` 完成后关闭；`Unsubscribe`/`Stop` 在 relay mutex 外等待且幂等；`Stop` 后 `Subscribe` 被拒绝；Wails shutdown 同步等待 relay 终止；测试不使用 sleep 猜测完成。
**证据**：`go test -race ./internal/usecase -run 'TestDataStreamRelay' -v` 全 PASS；`go test ./backend/...`（apps/desktop-wails）全 PASS。

---

### O-4 测试代码使用 var _ 保持符号可用

**位置**：

- `services/api-go/api/server_traversal_sevenhole_test.go:404`
- `services/api-go/internal/core/calibration/automatic_calibration_seven_hole_test.go:527`

这些占位不会影响生产行为。若目标是验证契约，应改为真正的行为测试；若只为保留 import，应删除占位和未使用 import。该问题不应高于 Optional。

**状态（2026-07-22）**：**Fixed**（Task 23）。删除两处 `var _` 占位 + 未使用 import：
- `services/api-go/api/server_traversal_sevenhole_test.go`：删除 `var ( _ = traversal.ProbeTypeSevenHole; _ = (*usecase.TraversalManager).CalculateSevenHoleRealtime )` + `traversal` import（仅占位引用，删除后无其他 `traversal.` 引用）。
- `services/api-go/internal/core/calibration/automatic_calibration_seven_hole_test.go`：删除 `var _ = traversal.MotionInterruptNone` + 误导性注释 + `traversal` import（`fakeCalibrationRuntime.WaitForMotionComplete` 在 `automatic_five_hole_test.go` 同包内已引用 `traversal`，本文件无需重复引用）。
**证据**：`rg -n 'var _ =.*(ProbeTypeSevenHole|MotionInterruptNone)' services/api-go` 无匹配；`go test ./api/... ./internal/core/calibration/... -v` 全 PASS；`go vet ./...` + `go build -buildvcs=false ./...` exit 0。

---

## Nit 问题（2 项）

### N-1 部分错误消息含内部枚举值

**位置**：`services/api-go/internal/usecase/device_manager.go:1091`

`triggerMode must be 0 (software) or 2 (hardware)` 对当前桌面调用可接受。如果该 API 将来对外开放，再将内部枚举和面向用户的错误文案解耦。

**状态（2026-07-22）**：**Closed with Evidence**（Task 23）。经消费者复核，`triggerMode` 数值 `0=software / 2=hardware` 是 HTTP、Wails、持久化与驱动四条契约的统一值，并非"内部枚举"：
- HTTP：`PUT /api/device/{id}/daqT1603Config` 路由（`api/server.go:919-925`）调用 `DeviceManager.ApplyDaqT1603Config` → `validateDaqT1603Config`，错误文案原样回传给 HTTP 客户端。
- 持久化：`device.DaqT1603HardwareConfig.TriggerMode` 字段 `json:"triggerMode"` 直写 profile JSON（`internal/core/device/types.go:163`，注释 `0=software, 2=hardware`）。
- 驱动：`adapters/hardware/t1603_adapter.go:313,329` 将 `cfg.TriggerMode` 透传至 shared SDK。
- Wails：与 HTTP 共用同一 usecase 入口。
结论：文案与对外契约一致，无需解耦；保持现状以避免投机性兼容代码。`profile upsert` 路径与 `ApplyDaqT1603Config` 路径的 triggerMode 校验一致性差异不在本轮范围，登记为 FU-1 follow-up。
**证据**：`rg -n 'TriggerMode' services/api-go/internal/core/device services/api-go/internal/adapters/hardware services/api-go/api` 命中均为契约字段透传，无内部枚举解耦需求；HTTP 路由 `api/server.go:912-930` 确认错误文案直接回传客户端。

### N-2 pointsFileParser 括号剥离正则只处理一段

**位置**：`apps/desktop-wails/frontend/src/shared/pointsFileParser.ts:64`

`replace(/\(.*\)|（.*）/, '')` 不带全局标志且使用贪婪匹配。现实列名影响较小；如需支持多个括号注释，应改为明确的非贪婪、全局表达式并补测试。

**状态（2026-07-22）**：**Fixed**（Task 19）。`pointsFileParser.ts` 抽出 `stripBracketComments` 共享工具（正则 `/\(.*?\)|（.*?）/g` 非贪婪 + 全局 + 半角/全角一次剥离），`normalizeAxisName` 与 `normalizeConfigKey` 共用该工具，消除两处重复正则。
**证据**：`npm run test -- pointsFileParser --run` → 38/38 PASS。新增多括号测试覆盖：
- Axis 6 例：`X(mm)(deg)` / `X（mm）（deg）` / `X(mm)（deg）` / `X（mm）(deg)` / `Y(mm)（deg）(raw)` / `pos_x（mm）(deg)`
- Config 5 例：`Dwell(ms)(稳定)` / `Dwell（ms）（稳定）` / `Dwell(ms)（稳定）` / `Samples(个)（每点）` / `Test(1=on)（开关）`
- CSV 表头多段混合括号解析用例（`X(mm)（deg）,Y（mm）(deg),Z(mm)(deg),U（°）（deg）,Dwell(ms)（稳定）`）
- 单括号既有用例继续通过（行为不变）。

---

## 静态抽查未发现的问题

以下结论仅表示本次抽查未发现明显反例，不代表完整证明：

| 检查项 | 静态抽查结果 |
|---|---|
| `internal/core/` 直接硬件、网络或文件 I/O | 未发现明显违规 |
| `internal/ports/` 出现具体外部实现 | 未发现明显违规 |
| `internal/usecase/` 直接调用底层硬件协议 | 未发现明显违规 |
| `internal/adapters/hardware/` 承载明显领域算法 | 未发现明显违规 |
| `programs/` 导入 WindLabX4 `internal/*` | 未发现明显违规 |
| 前端直接 SCPI / TCP / 串口访问 | 未发现明显违规 |
| 生产代码主动 `panic` | 未发现明显调用 |

---

## 工程亮点

### 1. 统一日志基础设施

`pkg/logging/logger.go` 提供 ring buffer、stderr 和按日轮转文件三路输出，并通过 stdlog bridge 兼容旧日志。后续应继续把业务字段迁移到原生 slog，而不是删除该兼容层。

### 2. per-device 连接锁

`device_manager.go` 使用 `sync.Map` 保存每设备 mutex，使不同设备可并行、同一设备的 Connect/Disconnect 串行化，边界清晰。

### 3. AcquisitionHub 分片设计

`acquisition.go` 使用 16 shard、原子读取发布间隔和锁外非阻塞发送，降低高频采集路径上的全局锁竞争。性能容量结论仍需 benchmark 或运行数据支持。

### 4. DAQP1603 thin wrapper

`daq_p1603_adapter.go` 聚焦项目类型与 shared SDK 类型转换，协议实现没有重复进入产品 adapter。

### 5. 遍历结果三阶段提交

`traversal_acquisition.go` 的 CSV、ResultLog 和 Checkpoint 提交流程包含 sync、回滚和错误聚合，崩溃恢复语义较完整。

### 6. 设备连接 identity 校验

`device_manager.go` 在异步错误回调中比较实例 identity，避免旧设备实例回调误删新连接。

---

## 修复优先级路线图

### P0 - 发布前阻断

| 项 | 位置 | 验收标准 |
|---|---|---|
| C-1 API 绕过 usecase 调 adapter | `api/server.go`、ports、usecase、composition root | API 不再 import adapter；usecase 不 import adapter；导入流程测试通过 |
| C-2 前端气动公式迁移 | `calibrationStore.ts` + 后端状态/接口 | 生产前端无 Ma/V 公式；页面显示后端结果 |
| C-3 急停 fallback 错误聚合 | `calibration.go` | 两级停止错误均可观测；状态码和故障快照保持完整 |

### P1 - 下个迭代

| 项 | 位置 | 验收标准 |
|---|---|---|
| R-1 demo 算法隔离 | traversal simulation 导入链 + 校验脚本 | 生产入口不静态导入 demo 算法；生产 bundle 验证通过 |
| R-2 结构化日志 | `calibration.go` | 19 处旧日志迁移，关键字段可检索 |
| R-3 清理错误诊断 | calibration/traversal | 清理错误被记录或聚合，无重复热路径日志 |
| R-4 删除蛇形布点 fallback | `motionCalibrationUtils.ts` | 后端失败显式反馈，不执行本地算法 |
| R-5 MotionSafety 权威边界 | panel/types + 契约测试 | 前端仅提示，后端始终权威校验 |
| R-6 API 直调 core 收口 | `api/server.go` + usecase | API 不直接调用点位/插值算法 |
| R-7 完整运行结构校验 | workspace scripts | 脚本成功退出并保存结果 |

### P2 - 后续优化

| 项 | 位置 |
|---|---|
| O-1 API 算法输入类型解耦 | `api/server.go` |
| O-2 autoEngine 生命周期 | `calibration.go` |
| O-3 relay 确定性 shutdown | `stream_relay.go` |
| O-4 测试占位清理 | 两处测试文件 |
| N-1/N-2 文案与 parser 小修 | 对应文件 |

`ptrFloat64` / `ptrInt` 一行 helper 不再列入整改项：当前重复规模很小，把 WindLabX4 core 绑定到 shared motion core 或新增 util 包会扩大依赖，收益不足。

---

## 验证状态与限制

| 验证项 | 当前状态 |
|---|---|
| 静态分层检索与关键代码复核 | 已完成 |
| `scripts/validate-structure.ps1` | 已执行（Task 24 + Task 18/19 复验）；exit 1 — 12 个预存未知顶层条目（`.trae`/`.codebuddy`/`.workbuddy`/`.worktrees`/`analysis`/`pdf-qa`/`temp`/`WindLabX4-intro-images`/`WindLabX4-trae-work-prototype`/`.tmp-protocol-remaining.html`/`multi-probe-traversal-design.html`/`W505-Protocol-V2.0.html`），不属本轮整改回归 |
| `scripts/validate-import-direction.ps1` | 已执行（Task 24 + Task 18/19 复验）；exit 0 — 699 Go files checked |
| `scripts/validate-frontend-structure.ps1 -CheckFileSize` | 已执行（Task 24 + Task 18/19 复验）；exit 1 — 多个预存前端规范问题（i18n 硬编码中文、`: any` 类型注解、泛型变量名等），不属本轮整改回归 |
| `gofmt -l .`（WindLabX4 services/api-go） | 已执行（Task 24）；列出 40+ 文件需格式化，多为预存 struct 字段对齐问题；Task 23 改动本身符合 gofmt |
| 后端 `go build -buildvcs=false ./...` | 已执行（Task 24 + Task 18/19 复验）；exit 0 |
| 后端 `go vet ./...` | 已执行（Task 24 + Task 18/19 复验）；exit 0 |
| 后端 `go test ./...` | 已执行（Task 24 + Task 18/19 复验）；全 PASS（usecase 76.5s + apiserver 10.5s + integration 10.5s） |
| 后端 `go test -race ./internal/usecase/... ./api/...` | 已执行（Task 24）；全 PASS（usecase 73.9s + api 1.1s） |
| 前端 `npm run typecheck` | 已执行（Task 24 + Task 18/19 复验）；exit 0（vue-tsc --noEmit） |
| 前端 `npm run test` | 已执行（Task 24 + Task 18/19 复验）；18 files / 244 tests 全 PASS（含 Task 18 新增 21 MotionSafetyPanel + Task 19 新增多括号用例） |
| 前端 `npm run build` | 已执行（Task 24 + Task 18/19 复验）；exit 0 |
| Wails bindings 同步 | 未重新生成（Task 18/19 未触及 Wails 绑定签名；前序任务 05/08/09 已同步） |
| `git diff --check` | 已执行（Task 24 + Task 18/19 复验）；全工作区 exit 2 — 13 处行尾空白均在 Wails 生成 bindings 文件中（生成器输出特征）；**本轮 Task 18/19 改动的 8 个文件 `git diff --check` exit 0（干净）** |
| GitNexus `detect_changes` | 已执行（Task 24 + Task 18/19 复验）；exit 0 — 本轮 Task 18/19 改动 7 个文件（399 insertions, 72 deletions），受影响流程包括 `ProcessPoint → NormalizeAxisName`、`ProcessPoint → OnMotionSafetyFailure`，与批准计划一致 |
| HIL 真机验证 | **未执行**（发布前人工门禁，由具备设备与安全条件的维护人员执行；自动测试不得代替 HIL 结论） |

发布前应按 `projects/WindLabX4/AGENTS.md` 的命令完成验证。验证失败时，应把失败项及原始命令纳入本报告，而不是继续使用“零违规”或数字评分。

---

**报告结束**
