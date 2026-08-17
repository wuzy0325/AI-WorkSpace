# Implementation Plan: WindLabX4 代码审查问题整改

> 日期：2026-07-21
> 状态：已批准；Phase 3 任务清单待审阅
> 前置规格：[spec-code-review-remediation-2026-07-21.md](./spec-code-review-remediation-2026-07-21.md)
> 审查入口：[code-review-2026-07-21.md](../code-review-2026-07-21.md)

## Overview

本计划落实已批准规格中的全部 16 个审查项。实施以垂直切片组织：先建立安全停止与校准会话生命周期，再收口 API/usecase/adapter 边界和校准 DTO，随后将气动计算迁移到后端状态契约，删除前端 demo/算法 fallback，最后处理日志、清理错误、relay 生命周期、测试占位、parser 和证据关闭项。

不改变 HTTP 路径、既有 JSON 字段、Wails 方法名、持久化格式或 CSV schema；新增字段仅为 `calibration.Status` 中可选的 physics 快照。任何与此计划不符的公共契约、持久化或依赖变更必须回到规格阶段审批。

## Resolved Decisions

- `Pt == Ps` 是有效无流状态，后端返回 `machNumber=0`、`velocity=0`。
- 气动数据通过既有 `calibration.Status` 在 HTTP 与 Wails 间传输；不新增 endpoint 或 event 通道。
- 完全删除当前没有 UI 入口的 traversal simulation 代码和嵌入校准数据；不保留 demo build。
- 校准 DTO/`ToCore` 属于 transport boundary，移至 `pkg/types`；不创建 decoder port，也不让 usecase 接收 raw JSON。
- 插值文件导入复用现有 `ports.InterpolatorLoader`。API 不再 import adapter 或 shared interpolation input type。
- `CalibrationManager` 使用 session 记录捕获自己的 engine、task ID、cancel 和 done；Stop 有界等待，超时后不得允许下一任务复用资源。
- `DataStreamRelay.Stop` 是终止性操作，返回前完成所有 hub unsubscribe；等待发生在 mutex 外。
- `N-1` 与 data sink recorder 错误属于证据关闭项，不引入无收益代码变更。
- HTTP 模式新增 calibration status 轮询是删除前端 physics 公式的必要行为变化；复用现有路径，并以无重叠请求为硬约束。
- 七孔导入响应的 `pointCount` 保持现有兼容值：内区 169、外区 52；不在本轮改为动态文件计数。

## Dependency Graph

```text
Core formula zero-flow rule
  -> calibration.Status optional physics snapshot
     -> CalibrationManager status assembly
        -> HTTP/Wails binding status transport
           -> frontend status types and polling
              -> remove frontend physics formula

pkg/types calibration DTO + ToCore
  -> HTTP calibration start and Wails CalibrationStart share boundary mapping
     -> MotionSafety reaches backend validator

ports.InterpolatorLoader metadata
  -> TraversalManager import methods
     -> API import routes and compatible response mapping

usecase ProbePressureInput
  -> API calculateRealtime route drops shared algorithm types

core engine cancellation
  -> CalibrationManager session ownership and bounded join
     -> desktop/API service shutdown ownership

relay subscription done records
  -> relay terminal Stop/Unsubscribe
     -> Wails drain and service shutdown synchronization

writer/lock error semantics
  -> structured slog migration
     -> final review evidence and full verification

parser and test placeholder cleanup
  -> frontend/backend final verification
```

## Architecture Decisions

### 1. Transport-boundary calibration DTO

`pkg/types.CalibrationConfigDTO` and `ProbeChannelDTO` become concrete named public DTOs with `ToCore`. HTTP decodes that DTO directly and Wails accepts the same DTO. This removes both the API-to-adapter import and the public alias of an internal adapter type. The DTO must preserve `MotionSafety`; current conversion silently drops it.

### 2. Usecase-owned interpolation imports

`TraversalManager` gains explicit import operations. Each snapshots its injected `InterpolatorLoader`, loads through the port, mutates manager state only after success, and returns usecase-owned metadata. The port and adapter expose neutral metadata where current HTTP responses require authoritative loader data, especially multi-PRB files/warnings and five-hole CSV point count. Seven-hole responses retain the existing contract constants (`inner=169`, `outer=52`) rather than introducing dynamic count semantics in this architectural change.

### 3. Backend-owned live physics

The core owns the zero-flow rule and formulas. `CalibrationManager.Status()` snapshots config/status under its lock, releases the lock, reads configured semantic channels, calculates optional physics, and attaches it only to the returned status copy. It must not hold `m.mu` around external data reads or mutate persisted terminal status.

### 4. Session-owned calibration lifecycle

The calibration worker captures an immutable session and finalizes only that session. Stop requests cancellation and motion stop, waits outside `m.mu` for at most five seconds, and reports incomplete shutdown without force-flushing or allowing another session to reuse resources. Context-aware waits are introduced where owned code can safely observe cancellation; legacy runtime capabilities remain optional.

### 5. Deterministic relay shutdown

Relay subscriptions carry `cancel` and `done`. Stop marks the relay terminal while locked, snapshots subscriptions, then cancels and waits outside the lock. A worker closes `done` only after its hub unsubscribe completes. New subscriptions after terminal Stop are rejected.

## Implementation Phases

### Phase A: Safety and Lifecycle Foundation

**Purpose:** Remove safety error loss and eliminate calibration/relay ownership races before changing transport and UI behavior.

**Slice A1 - Emergency fallback error preservation (C-3)**

- In `handleCalibrationMotionSafetyFailure`, retain the emergency error and capture fallback `StopMotion` failure.
- When both fail, return an `errors.Join`-compatible error and include both causes in `failWithCode` message.
- Preserve `ErrEmergencyStopFailed`, `StateError`, and the existing `failWithCode` then `recordMotionSafetyFailure` ordering.
- Add three-path tests: emergency success, emergency fail/fallback success, and double failure.

**Slice A2 - Engine cancellation and manager run session (O-2)**

- Add a private calibration run session with captured engine/config/task ID/cancel/done.
- Make engine execution cancellation-aware for dwell, pause, gate, fresh-data and owned motion wait paths while retaining compatibility for existing runtime interfaces.
- Publish running state only after all pre-launch validation succeeds.
- Move automatic finalization, writer ownership, result persistence and homing to the captured session worker.
- Reject a new Start while a previous session is not done, including after a bounded Stop timeout.
- Add lifecycle tests for immediate Start/Stop, replacement rejection, stale worker isolation, blocked legacy runtime timeout, and concurrent status reads.

**Slice A3 - Explicit service shutdown ownership (O-2)**

- Desktop `ServiceShutdown` stops and joins active calibration before logger closure.
- Context-owned API server construction stops its calibration manager during shutdown.
- Standalone server lifecycle obtains a shutdown context/signal path only if this can be introduced without changing public service behavior; otherwise document the remaining ownership gap as Blocked for human review.

**Slice A4 - Deterministic relay lifecycle (O-3)**

- Replace cancellation-only subscription map entries with records containing cancel/done.
- Make Unsubscribe and Stop wait outside relay mutex; preserve idempotence.
- Make Stop terminal and reject later Subscribe.
- Synchronize Wails drain/service shutdown with relay termination.
- Add no-sleep tests that inspect hub subscriptions after Unsubscribe/Stop, plus full-channel, repeated Stop and concurrent Subscribe/Stop cases.

**Checkpoint A**

在 `projects/windlabx4/services/api-go` 下执行：

- `go test ./internal/core/calibration/... ./internal/usecase/... -run 'Test.*(MotionSafety|Calibration.*Lifecycle|StreamRelay)'`
- `go test -race ./internal/core/calibration/... ./internal/usecase/...`
- No worker finalization, file flush, homing, relay wait or hub unsubscribe runs while its manager/relay mutex is held.
- Human review required before accepting any new shutdown timeout behavior.

### Phase B: Boundary and Contract Vertical Slice

**Purpose:** Move import, realtime interpolation and calibration conversion responsibilities out of API/adapters while preserving existing routes and JSON.

**Slice B1 - Public calibration request DTO (C-1, R-5 prerequisite)**

- Move concrete calibration DTO and channel conversion from `internal/adapters/config` to `pkg/types`.
- Add and map `MotionSafety` so HTTP and Wails do not silently discard it.
- Add migration-facing tests for previously persisted/frontend-produced invalid MotionSafety values: after this fix those values must be rejected explicitly by backend Start rather than silently ignored.
- Update HTTP calibration start and Wails `CalibrationStart` to use the shared DTO.
- Remove obsolete adapter decoder and alias only after all references and tests migrate.
- Regenerate Wails bindings because concrete Go DTO identity may change.

**Slice B2 - Loader metadata and usecase import operations (C-1, R-6)**

- Extend `ports.InterpolatorLoader` with port-owned metadata sufficient for the existing multi-PRB and CSV endpoint response fields.
- Adapt `adapters/interpolation.Loader` to map concrete loader results into port metadata.
- Add `TraversalManager.ImportPRB`, `ImportCalibrationCSV`, `ImportMultiPRB`, `ImportSevenHolePRB`, and `ImportSevenHoleCalibrationCSV`.
- Preserve cache clearing, restore-error reset, seven-file validation, timestamps, warnings and failure-without-state-replacement behavior.
- Preserve seven-hole response `pointCount` values exactly as today (`169` for inner, `52` for each outer sector). Do not claim they are loader-derived real counts; record dynamic count semantics as a follow-up in E4.

**Slice B3 - Transport-neutral realtime probe input (O-1)**

- Export a named pressure input from usecase and one probe-dispatch calculation method.
- Preserve blank probe type as five-hole, P6/P7 presence validation, seven-hole configuration mismatch checks, and concrete five-hole/seven-hole JSON output values.
- Change the route to decode only transport DTOs and call usecase; remove shared interpolation input imports.

**Slice B4 - Five-hole preview usecase and Wails parity (R-4, R-6)**

- Add `CalibrationManager.PreviewFiveHolePoints`, mirroring existing seven-hole preview ownership.
- Route `POST /api/calibration/fivehole` through the manager without changing its bare-array response.
- Add the minimal Wails binding/API facade support required for desktop mode to call the same backend operation.
- Delete local snake fallback only after both transports return backend errors consistently.

**Checkpoint B**

在 `projects/windlabx4/services/api-go` 下执行：

- `go test ./internal/usecase/... ./internal/adapters/interpolation/... ./pkg/types/... ./api/...`
- `go vet ./...`
- Wails binding generation succeeds and generated diff is reviewed; no generated file is edited manually.
- `server.go` has no imports from `internal/adapters/config`, `internal/adapters/interpolation`, five-hole interpolation input package or seven-hole interpolation input package.
- API contract tests confirm unchanged paths, request fields, success fields and operator-facing error behavior.

### Phase C: Backend Physics and Frontend Display Slice

**Purpose:** Make backend physics authoritative in both HTTP and Wails while removing frontend formulas.

**Slice C1 - Core zero-flow physics contract (C-2)**

- Change the authoritative calculator so equal absolute total/static pressure yields valid zero Mach and velocity.
- Keep invalid ordering (`Pt < Ps`), invalid pressure/temperature and non-finite inputs unavailable/errors as currently defined by core behavior.
- Add table-driven core tests for zero, valid non-zero, invalid pressure relation, atmospheric conversion and temperature priority.

**Slice C2 - Status physics snapshot and backend tests (C-2)**

- Add optional physics to `calibration.Status` with pointer semantics that distinguish missing from valid zero.
- Add a manager helper to resolve per-type semantic channels from `currentConfig` and calculate physics after unlocking.
- Cover five-hole, three-hole, total-pressure and seven-hole mappings, missing channels, valid zero, stale clearing, multi-device channel selection and race safety.
- Verify existing HTTP status response omits unavailable values and preserves zero values; retain existing route/method.

**Slice C3 - Wails/HTTP transport-neutral status consumption (C-2)**

- Regenerate bindings for the additive status model.
- Consolidate frontend transport status interfaces around the explicit optional physics contract.
- Change status polling to call `calibrationApi.status()` in both Wails and HTTP modes with an in-flight guard or chained timeout.
- Treat HTTP polling as an explicitly approved behavior change required to preserve live Ma/V after removing frontend formulas; reuse the current status endpoint and do not add a second polling contract.
- Wire existing HTTP pause/resume/stop routes instead of synthetic success behavior.

**Slice C4 - Remove frontend atmospheric formulas (C-2)**

- Delete atmospheric constants and calculation function from `calibrationStore`.
- Map backend status physics into the existing display ref and clear it when status omits physics.
- Keep raw pressure mapping for cards; it no longer derives Ma/V.
- Add tests for missing, zero, non-zero, stale clearing, delayed HTTP polls and all four page display contracts.

**Checkpoint C**

后端命令在 `projects/windlabx4/services/api-go` 下执行，前端命令在 `projects/windlabx4/apps/desktop-wails/frontend` 下执行：

- `go test ./internal/core/calibration/... ./internal/usecase/... ./api/...`
- `npm run typecheck`
- `npm run test -- calibrationStore`
- `npm run build`
- Production calibration store and calibration components contain no `ATM_GAMMA`, `ATM_C_COEFF`, `calculateAtmosphericPhysics` or Ma/V formula implementation. Demo-only identifiers are checked after D1 in Checkpoint D.

### Phase D: Production Demo Removal and MotionSafety Contract

**Purpose:** Remove unused production simulation code and make frontend safety validation an accurate non-authoritative mirror.

**Slice D1 - Delete traversal simulation implementation (R-1)**

- Remove `useTraversalSimulation`, `simulateTraversalRun`, `simulateFiveHoleCalibration`, their 169-point dataset and all dead imports from traversal UI.
- Remove any now-unused simulation-only types or code only when source references prove they are exclusive to this feature.
- Update the frontend structure validator to reject production static imports/re-exports/literal dynamic imports targeting `DEMO ONLY` or `simulate*` modules.
- Add a validator fixture or focused PowerShell verification proving a production-to-demo import fails.
- Add a production bundle scan proving the deleted identifiers/dataset are absent.

**Slice D2 - MotionSafety frontend contract alignment (R-5)**

- Retain backend `validateMotionSafetyConfig` as the only authority.
- Split frontend blocking errors from advisory warnings; only backend-equivalent violations make `isValid` false.
- Make override TypeScript type non-recursive and surface stale/unknown override keys where the UI has bound axes.
- Make calibration and traversal settings use consistent local validation behavior while retaining backend Start validation.
- Correct stale default documentation comments and add parity fixtures for backend-invalid and advisory-only configurations.

**Slice D2 状态（2026-07-22）**：**Done**。`MotionSafetyPanel.vue` 拆分 `blockingErrors`（B1-B9 与后端 `validateMotionSafetyConfig` 对齐）+ `advisoryWarnings`（A1-A2 仅 UX 提示），`isValid` 仅由 `blockingErrors` 决定；`traversal.ts` 引入 `MotionSafetyAxisOverride` 非递归子集类型 + `MotionSafetyFieldKey` 联合类型，B8 嵌套覆盖在编译期被类型系统阻止；`TraversalSettings.vue` `saveConfig` 改用 `blockingErrors[0]` 作为 toast 文案；新增 21 个前端契约测试 + 5 个后端契约测试。验证：后端 `go test ./internal/usecase/ -run TestValidateMotionSafetyConfig` → 17/17 PASS；前端 `npm run test` → 244/244 PASS；`npm run typecheck` + `npm run build` → exit 0。

**Slice D3 - Parser multiple-annotation handling (N-2)**

- Replace duplicate greedy single-match regexes with one shared non-greedy global annotation stripper.
- Add cases for `X(mm)(deg)`, `X（mm）（deg）`, and mixed annotation forms.

**Slice D3 状态（2026-07-22）**：**Done**。`pointsFileParser.ts` 抽出 `stripBracketComments` 共享工具（`/\(.*?\)|（.*?）/g` 非贪婪 + 全局，半角/全角混排一次剥离），`normalizeAxisName` 与 `normalizeConfigKey` 共用该工具；`pointsFileParser.test.ts` 新增 axis 6 例 + config 5 例多括号测试 + CSV 表头多段混合用例。验证：`npm run test -- pointsFileParser --run` → 38/38 PASS；单括号行为不变。

**Checkpoint D**

在工作空间根目录执行结构校验，在 `projects/windlabx4/apps/desktop-wails/frontend` 下执行 npm 命令与 bundle 扫描：

- `powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\validate-frontend-structure.ps1 -ProjectDir .\projects\WindLabX4\apps\desktop-wails\frontend\src -CheckFileSize`
- `npm run test`
- `npm run typecheck`
- `npm run build`
- `rg -l "98924\.2|Simulation 16.{0,16}16|169-point calibration" dist` returns no files. Stable numeric/string literals are the primary bundle probes; minified identifier names are not treated as proof of exclusion.

### Phase E: Observability, Cleanup and Evidence Closure

**Purpose:** Complete remaining error handling, make logs queryable, remove no-value test code and close non-code review items with evidence.

**Slice E1 - Writer and lock cleanup errors (R-3)**

- Join append and cleanup Flush errors where the caller receives an error.
- Record temporary duplicate writer Flush failures without failing a valid cached writer path.
- Make `calibration_csv_writer.go` inspect `csv.Writer.Error()` and join writer/close errors. Do not modify already compliant traversal CSV writer paths unless a targeted test finds a separate defect.
- Handle all four traversal lock Release paths according to their propagation ability: join on returned errors, warn on void paths, and never claim successful release after a failed release.
- Do not add a second recorder error log in `data_sink.go`.

**Slice E2 - Structured calibration logs (R-2)**

- Replace all 19 `log.Printf` calls with `slog` using stable event names and fields.
- Split seven-hole writer factory missing from optional empty save path; only the missing injected factory is an error.
- Preserve task-level and controller-level event distinction for motion safety.
- Add targeted slog record assertions, avoid brittle full-message snapshots, and remove unused `log` import.

**Slice E3 - Remove compile-only test placeholders (O-4)**

- Delete two `var _` blocks and their unused imports after confirming existing HTTP/core behavior tests cover the symbols.

Status (2026-07-22, Task 23): Done. Both `var _` blocks and their now-unused `traversal` imports deleted from `services/api-go/api/server_traversal_sevenhole_test.go` and `services/api-go/internal/core/calibration/automatic_calibration_seven_hole_test.go`. Existing HTTP/core behavior tests continue to cover `ProbeTypeSevenHole` and `MotionInterruptNone`. Evidence: `rg -n 'var _ =.*(ProbeTypeSevenHole|MotionInterruptNone)' services/api-go` returns no matches; `go test ./api/... ./internal/core/calibration/... -v` all PASS; `go vet ./...` + `go build -buildvcs=false ./...` exit 0.

**Slice E4 - Evidence closures and review report update (N-1 and recorder sub-item)**

- Mark `N-1` Closed with Evidence: triggerMode numeric values are HTTP, Wails, persistence and driver contract values.
- Mark recorder error sub-item Closed with Evidence: `StorageRecorder.HandlePayload` already logs at source and data sink duplication would be hot-path noise.
- Record the profile-upsert triggerMode validation inconsistency as a separately scoped follow-up, not an opportunistic fix in this plan.
- Record seven-hole dynamic/actual point-count semantics as a separately scoped follow-up; this plan intentionally preserves response values 169/52.
- Update every review item to Fixed, Closed with Evidence, Blocked or Deferred with command/test evidence.

Status (2026-07-22, Task 23): Done for in-scope items. N-1 marked Closed with Evidence (triggerMode 0/2 is HTTP/Wails/persistence/driver contract value; no external-only enum to decouple). Recorder sub-item marked Closed with Evidence in code-review-2026-07-21.md R-3. FU-1 (profile-upsert triggerMode validation inconsistency) and FU-2 (seven-hole dynamic point count) registered as separately scoped follow-ups. 14 of 16 review items now have status+evidence; N-2 remains Pending (Task 19 not yet done). Remaining item-level evidence closure for R-7 will be filled by Task 24's full verification.

**Checkpoint E**

在 `projects/windlabx4/services/api-go` 下执行：

- `go test ./internal/usecase/... ./internal/adapters/storage/... ./internal/core/resourcelock/... ./api/...`
- `go vet ./...`
- `rg -n "log\.Printf" .\projects\WindLabX4\services\api-go\internal\usecase\calibration.go` returns no matches.
- All four resource-lock paths are covered by behavior or source-level verification, with no `_ = resourcelock.Default().Release(...)` remaining in production traversal usecase paths.

### Phase F: Final Verification and HIL Gate (R-7)

**Purpose:** Prove source, test and build acceptance before requesting merge/release decisions.

1. Run workspace structure/import/frontend validators.
2. Run Go formatting check, build, vet, targeted tests, full backend tests and race tests.
3. Run frontend typecheck, unit tests and production build.
4. Regenerate Wails bindings if models/signatures changed, then rerun frontend checks.
5. Run `git diff --check`, inspect status and use GitNexus `detect_changes`.
6. Update the review report with actual command results, skipped checks and remaining HIL risks.
7. Execute/document the manual HIL checklist before release: emergency stop failure, fallback ordinary stop, controller reset and post-reset motion state.

## Plan-Level Risks and Mitigations

| Risk | Impact | Mitigation |
|---|---|---|
| Core zero-flow change affects all callers | High | Table-driven core tests plus five-hole/three-hole/total-pressure/seven-hole regression tests before frontend migration |
| Calibration worker touches hardware/storage outside cancellation control | High | Five-second bounded join, reject replacement session after timeout, do not force-flush/homing on timeout, HIL gate |
| DTO relocation changes generated Wails TS identity | Medium | Regenerate bindings, inspect generated diff, typecheck before frontend work continues |
| MotionSafety begins reaching backend validation after DTO repair | High | Treat newly rejected invalid historical/UI configs as intentional error explicitization; align D2 parity fixtures and provide operator-visible validation feedback before Start |
| Multi-PRB loader metadata can lose association after skipped files | Medium | Map metadata inside adapter where concrete result association is still authoritative; test skipped/duplicate warnings |
| Seven-hole pointCount is currently a fixed response contract, not loader truth | Medium | Preserve 169/52 in this plan and record dynamic real-count behavior as a separate follow-up |
| Status polling creates overlapping requests or lock contention | Medium | In-flight guard/chained scheduling, external reads after manager unlock, race tests |
| HTTP status polling is a new runtime behavior | Medium | Explicit approval gate, reuse existing endpoint, stop on terminal state, test no overlap and error handling |
| Demo deletion leaves hidden product dependency | Medium | Run static searches, typecheck and production bundle scan; delete only after all imports removed |
| Frontend/backend MotionSafety rules drift | Medium | Classify only backend-equivalent conditions as errors, test shared invalid fixtures, retain backend validation at Start |
| Global slog and resource lock tests interfere | Low | Avoid `t.Parallel`, restore globals in `t.Cleanup`, release singleton locks with actual holder |
| TriggerMode review exposes unrelated validation inconsistency | Low | Record separate follow-up; do not enlarge this plan without approval |
| GitNexus index is stale | Low | Use direct source evidence for planning; re-index before relying on graph for implementation impact analysis |

## Parallelization

Sequential dependencies:

- A1 through A3 must be ordered because error handling and session ownership share `calibration.go`.
- B1 must precede Wails regeneration and any `MotionSafety` transport verification.
- B2/B3 must precede route simplification in `server.go`.
- C1 and C2 must precede all frontend physics removal.
- E2 follows A1 and E1 because it logs the final error semantics.

Potentially parallel after their dependencies are stable:

- B1 DTO relocation can proceed in parallel with Phase A because it primarily touches `pkg/types`, `internal/adapters/config`, HTTP/Wails boundary files and DTO tests; coordinate only when both branches reach shared calibration tests/binding generation.
- A4 relay lifecycle can proceed independently of calibration session work.
- B2 interpolation imports and B3 realtime input can proceed independently once their shared route ownership convention is agreed.
- D1 demo deletion, D2 MotionSafety UI alignment and D3 parser work are independent frontend slices.
- E3/O-4 cleanup and E4 evidence closure can occur independently after affected tests are green.

## Open Items for Phase 3 Tasks

The following are resolved implementation constraints, not new product decisions:

- Exact private type/function names are selected during tasks to match local code style.
- The standalone server shutdown change is implemented only if a minimal signal/context ownership path can be proven without expanding runtime behavior; otherwise it is marked Blocked with evidence.
- HIL executor, device environment and evidence storage remain manual release responsibilities.

## Plan Approval Gate

Do not create Phase 3 task checklist or modify production code until:

- [x] User approves this plan and dependency order.
- [x] User accepts the bounded five-second calibration Stop join behavior and its timeout semantics.
- [x] User accepts adding non-overlapping HTTP calibration status polling as the required replacement transport for live Ma/V.
- [x] User accepts that repairing the DTO causes invalid MotionSafety configurations, previously ignored, to be rejected by backend Start with explicit feedback.
- [x] User accepts preserving seven-hole import `pointCount` response values at 169/52 and recording real dynamic counts as a separate follow-up.
- [x] User accepts the scope boundary that profile-upsert triggerMode validation is recorded as a separate follow-up.
- [x] User confirms HIL remains a release gate owned by a qualified operator.
- [ ] User approves the Phase 3 task checklist before production-code implementation.
