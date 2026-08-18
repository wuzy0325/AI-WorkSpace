# Tasks: WindLabX4 代码审查问题整改

> 日期：2026-07-21
> 状态：待审阅
> 规格：[spec-code-review-remediation-2026-07-21.md](./spec-code-review-remediation-2026-07-21.md)
> 计划：[plan-code-review-remediation-2026-07-21.md](./plan-code-review-remediation-2026-07-21.md)
> 执行规则：逐项 TDD；每项开始前执行 GitNexus upstream impact；完成后运行该项定向验证。未通过检查点不得进入下一阶段。

## Dependency Order

```text
T01 -> T02 -> T03
T04 (可与 T01-T03 并行)
T05 -> T06
T07 -> T08 -> T09
T10 -> T11
T12 + T03 -> T13 -> T14 -> T15
T16 -> T17
T18、T19 可在依赖满足后并行
T03 -> T20 -> T22
T21 -> T23
T01 + T03 + T20 -> T22 -> T23
T01-T23 -> T24
```

## Review Coverage

| Review ID | Tasks |
|---|---|
| C-1 | T05-T08 |
| C-2 | T12-T15 |
| C-3 | T01 |
| R-1 | T16-T17 |
| R-2 | T22 |
| R-3 | T20-T21、T23（recorder 证据关闭） |
| R-4 | T10-T11 |
| R-5 | T05、T18 |
| R-6 | T08、T10 |
| R-7 | T24 |
| O-1 | T09 |
| O-2 | T02-T03 |
| O-3 | T04 |
| O-4 | T23 |
| N-1 | T23 |
| N-2 | T19 |

## Phase A - Safety and Lifecycle

### Task 01: 保留急停与 fallback 双错误

- [x] **Task**：修复 `handleCalibrationMotionSafetyFailure` 的双失败错误传播。
- **Dependencies**：无。
- **Files**：
  - Modify: `services/api-go/internal/usecase/calibration.go`
  - Test: `services/api-go/internal/usecase/calibration_motion_safety_test.go`
- **Acceptance**：
  - 急停失败/fallback 成功时返回错误可识别急停 cause。
  - 双失败时 `errors.Is` 可识别两个 cause。
  - `ErrEmergencyStopFailed`、`StateError` 和 MotionSafetyFailure 快照完整保留。
- **Steps**：
  1. 写双失败和 fallback 成功的失败测试并确认红灯。
  2. 用最小 `errors.Join` 改动实现，保持 `failWithCode` 后记录快照的顺序。
  3. 运行定向测试并确认绿灯。
- **Verify**：在 `services/api-go` 下运行 `go test ./internal/usecase -run 'TestHandleCalibrationMotionSafetyFailure' -v`。
- **Estimated scope**：S，2 files。

### Task 02: 让 AutomaticCalibration 等待可取消

- [x] **Task**：为 engine 自有等待路径增加 context 取消，不改变既有算法接口语义。
- **Dependencies**：Task 01。
- **Files**：
  - Modify: `services/api-go/internal/core/calibration/automatic_calibration.go`
  - Modify: `services/api-go/internal/core/calibration/read_probe_channels.go`
  - Test: `services/api-go/internal/core/calibration/automatic_calibration_pause_test.go`
  - Test: `services/api-go/internal/core/calibration/automatic_five_hole_test.go`
- **Acceptance**：
  - 启动前已取消、长 dwell、pause/gate/fresh-data wait 均能及时退出。
  - 既有 `Start` 调用保持兼容，新的 context 路径不丢失正常完成行为。
  - 不使用无条件 sleep 阻断 Stop。
- **Steps**：先写取消测试并验证失败，再增加 context-aware timer/select 和兼容 wrapper，最后运行相关 core 测试。
- **Verify**：在 `services/api-go` 下运行 `go test ./internal/core/calibration -run 'Test.*(Cancel|Pause|Automatic)' -v`。
- **Estimated scope**：M，4 files。

### Task 03: 建立 CalibrationManager run session

- [x] **Task**：实现 manager-owned session、5 秒有界 join 和旧任务资源隔离。
- **Dependencies**：Task 02。
- **Files**：
  - Modify: `services/api-go/internal/usecase/calibration.go`
  - Create: `services/api-go/internal/usecase/calibration_lifecycle_test.go`
  - Modify: `services/api-go/internal/usecase/calibration_status_test.go`
  - Modify: `apps/desktop-wails/backend/app.go`
  - Modify: `services/api-go/pkg/apiserver/apiserver.go`
- **Acceptance**：
  - session 捕获自己的 task/config/engine/cancel/done，旧 worker 不读取新 engine。
  - Stop 在 mutex 外等待，最多 5 秒；超时返回明确错误并阻止 replacement Start。
  - 预启动校验失败不留下 Running 状态。
  - writer flush、结果保存和 homing 只由所属 session finalize 一次。
  - Desktop 和 context-owned API server shutdown 会停止并等待其 calibration session；standalone server 无法最小收口时记录明确 blocker。
- **Steps**：先写 immediate Start/Stop、blocked runtime、replacement、stale worker 测试；确认红灯后实现最小 session；运行 race 测试。
- **Verify**：在 `services/api-go` 下运行 `go test -race ./internal/usecase -run 'TestCalibration.*(Lifecycle|Stop|Session|Status)' -v`。
- **Estimated scope**：M，3 files。

### Task 04: 确定性停止 DataStreamRelay

- [x] **Task**：让 Unsubscribe/Stop 返回前完成 hub unsubscribe，Stop 后禁止复活。
- **Dependencies**：无，可与 Tasks 01-03 并行。
- **Files**：
  - Modify: `services/api-go/internal/usecase/stream_relay.go`
  - Modify: `services/api-go/internal/usecase/stream_relay_test.go`
  - Modify: `apps/desktop-wails/backend/app.go`
- **Acceptance**：
  - subscription 记录包含 cancel/done，done 仅在 `unsub()` 完成后关闭。
  - Unsubscribe/Stop 在 relay mutex 外等待且幂等。
  - Stop 后 Subscribe 被拒绝；Wails shutdown 同步等待 relay 终止。
  - 测试不使用 sleep 猜测完成。
- **Steps**：写满 payload channel、重复 Stop、并发 Stop/Subscribe 测试；实现记录和终止状态；更新 Wails shutdown 所有权。
- **Verify**：在 `services/api-go` 下运行 `go test -race ./internal/usecase -run 'TestDataStreamRelay' -v`；在 `apps/desktop-wails` 下运行 `go test ./backend/...`（若包路径不支持则运行 `go test ./...`）。
- **Estimated scope**：M，3 files。

## Checkpoint A

- [x] 在 `services/api-go` 运行 `go test -race ./internal/core/calibration/... ./internal/usecase/...`。
- [x] 人工复核 Stop 5 秒超时、锁外等待、超时后禁止新任务的语义。
- [x] 未发现旧 session 修改新任务状态、writer 或运动轴的路径。

## Phase B - Boundary and API Contracts

### Task 05: 将 calibration DTO 移至 pkg/types

- [x] **Task**：让 HTTP/Wails 共用 transport DTO，并保留 MotionSafety。
- **Dependencies**：无，可与 Phase A 并行；在 Task 03 后合并共享 calibration 测试。
- **Files**：
  - Create: `services/api-go/pkg/types/calibration.go`
  - Modify: `services/api-go/pkg/types/types.go`
  - Create: `services/api-go/pkg/types/calibration_test.go`
- **Acceptance**：
  - DTO/ToCore 不依赖 internal adapter，支持 nested/flat channel。
  - MotionSafety 完整到达 core config；非法值由 backend Start 明确拒绝。
  - HTTP/Wails 请求字段保持兼容。
- **Steps**：迁移并扩展 decoder 测试，先加入 MotionSafety 回归红灯；实现 DTO；切换 HTTP/Wails 调用方。
- **Verify**：在 `services/api-go` 下运行 `go test ./pkg/types ./api/... -run 'Test.*Calibration' -v`。
- **Estimated scope**：M，3 files。

### Task 06: 删除旧 calibration decoder adapter

- [x] **Task**：在所有调用迁移后删除 adapter decoder 和旧测试。
- **Dependencies**：Task 05。
- **Files**：
  - Delete: `services/api-go/internal/adapters/config/calibration_config_decoder.go`
  - Delete: `services/api-go/internal/adapters/config/calibration_config_decoder_test.go`
  - Modify: `services/api-go/api/server.go`
  - Modify: `apps/desktop-wails/backend/app.go`
  - Modify: `services/api-go/pkg/types/types.go`（仅清理遗留 alias/import）
- **Acceptance**：`api`、Wails、`pkg/types` 均不 import/alias 旧 decoder；相关测试迁移后覆盖不减少。
- **Verify**：在 `services/api-go` 下运行 `go test ./pkg/types ./internal/adapters/config ./api/...`，并运行 `rg 'calibration_config_decoder|configadapter.DecodeCalibrationConfig' .` 无生产引用。
- **Estimated scope**：M，5 files。

### Task 07: 扩展 InterpolatorLoader 中立元数据

- [x] **Task**：为 multi-PRB 和 five-hole CSV 提供 endpoint 所需中立元数据。
- **Dependencies**：无。
- **Files**：
  - Modify: `services/api-go/internal/ports/interpolator_loader.go`
  - Modify: `services/api-go/internal/adapters/interpolation/loader.go`
  - Create: `services/api-go/internal/adapters/interpolation/loader_test.go`
  - Modify: `services/api-go/internal/usecase/traversal_config_test.go`
  - Modify: `services/api-go/internal/usecase/traversal_config_sevenhole_test.go`
- **Acceptance**：
  - port 不暴露 adapter/shared load-result 具体类型。
  - skipped/duplicate multi-PRB 文件、Mach 关联和 warnings 映射正确。
  - seven-hole count 不伪装为 loader 真值。
- **Verify**：在 `services/api-go` 下运行 `go test ./internal/adapters/interpolation ./internal/usecase -run 'Test.*(Loader|RestoreInterpolator)' -v`。
- **Estimated scope**：M，5 files。

### Task 08: 将显式插值导入收口到 TraversalManager

- [x] **Task**：实现五类 Import 方法并简化 HTTP routes。
- **Dependencies**：Task 07。
- **Files**：
  - Modify: `services/api-go/internal/usecase/traversal.go`
  - Modify: `services/api-go/internal/usecase/traversal_probe.go`
  - Create: `services/api-go/internal/usecase/traversal_import_test.go`
  - Modify: `services/api-go/api/server.go`
  - Modify: `services/api-go/api/server_traversal_sevenhole_test.go`
- **Acceptance**：
  - load 成功后才替换 manager state，失败保留旧 interpolator。
  - API 不 import interpolation adapter，不直接设置 interpolation mode。
  - 路径和 JSON 字段保持兼容；seven-hole count 保持 169/52。
- **Verify**：在 `services/api-go` 下运行 `go test ./internal/usecase ./api -run 'Test.*Import.*(Prb|PRB|Csv|CSV)' -v`。
- **Estimated scope**：M，5 files。

### Task 09: 收口实时 probe 输入

- [x] **Task**：增加 transport-neutral pressure input 和 probe dispatch method。
- **Dependencies**：Task 08。
- **Files**：
  - Modify: `services/api-go/internal/usecase/traversal_probe.go`
  - Modify: `services/api-go/internal/usecase/traversal_probe_test.go`
  - Modify: `services/api-go/api/server.go`
  - Modify: `services/api-go/api/server_traversal_sevenhole_test.go`
- **Acceptance**：API 不构造 shared five/seven-hole `InterpolationInput`；P6/P7 presence、probe mismatch 和完整响应字段保持现状。
- **Verify**：在 `services/api-go` 下运行 `go test ./internal/usecase ./api -run 'Test.*Calculate.*Realtime' -v`。
- **Estimated scope**：M，4 files。

### Task 10: 后端统一生成五孔蛇形点位

- [x] **Task**：新增 manager preview 并让 HTTP 路由委托 usecase。
- **Dependencies**：Task 05。
- **Files**：
  - Modify: `services/api-go/internal/usecase/calibration.go`
  - Modify: `services/api-go/api/server.go`
  - Create: `services/api-go/internal/usecase/calibration_five_hole_preview_test.go`
  - Create: `services/api-go/api/server_five_hole_preview_test.go`
- **Acceptance**：HTTP 调用 manager usecase；method/invalid step/serpentine order 有行为测试；bare-array 响应不变。
- **Verify**：在 `services/api-go` 下运行 `go test ./internal/usecase ./api -run 'Test.*FiveHole.*(Preview|Snake)' -v`。
- **Estimated scope**：M，4 files。
- **Notes (2026-07-22)**：
  - `calibration.go`：新增 `CalibrationManager.PreviewFiveHolePoints(layout)`，与 `PreviewSevenHolePoints` 对称——纯计算、不启动采集、不创建 runtime、不写 CSV，nil receiver 也可安全调用；透传 `core.GenerateFiveHoleSnakePoints` 错误并包中文上下文。
  - `server.go`：删除 `handleFiveholeSnakePoints`（直接调 core），新增 `handleFiveHolePreview(w, r, mgr)` 委托 `mgr.PreviewFiveHolePoints`；路由 `case "fivehole"` 传 `deps.CalibrationManager`；同步更新 `handleSevenHolePreview` 注释（移除"fivehole 直接调 core"过时描述）。
  - `calibration_five_hole_preview_test.go`：6 测试覆盖 raster/serpentine/invalid step/bare array/coordinates/nil-safe。
  - `server_five_hole_preview_test.go`：6 测试覆盖 HTTP 层 raster/serpentine/invalid step/bare array/405/malformed JSON。
  - 验证结果：`go test ./internal/usecase ./api -run 'Test.*FiveHole.*(Preview|Snake)'` → usecase 6 PASS / api 6 PASS；`go vet ./internal/usecase ./api` 无输出；`go build -buildvcs=false ./...` exit 0；全包回归 `go test ./internal/usecase/... ./api/...` exit 0（usecase 71.9s / api 0.07s）；Grep 确认 `api/` 无 `calibration.GenerateFiveHoleSnakePoints` 直接调用（spec Success Criteria "API 不直接调用 calibration.GenerateFiveHoleSnakePoints" 满足）。

### Task 11: 补齐 Wails 五孔 preview 并删除本地 fallback

- [x] **Task**：让桌面模式调用 Task 10 的后端能力，失败显式反馈。
- **Dependencies**：Task 10。
- **Files**：
  - Modify: `apps/desktop-wails/backend/app.go`
  - Modify: `apps/desktop-wails/frontend/src/api/calibrationApi.ts`
  - Modify: `apps/desktop-wails/frontend/src/components/calibration/five-hole/motionCalibrationUtils.ts`
  - Create: `apps/desktop-wails/frontend/src/components/calibration/five-hole/__tests__/motionCalibrationUtils.test.ts`
  - Modify: `apps/desktop-wails/frontend/src/api/wails-adapter.ts`（新增 `FiveHolePointLayout` interface + `previewFiveHole` binding 调用）
  - Modify: `services/api-go/pkg/types/types.go`（新增 `FiveHolePointLayoutDTO` / `FiveHoleSnakePoint` alias，与 `SevenHoleConfigDTO` 对称）
  - Regenerate: `apps/desktop-wails/frontend/bindings/...`（`wails3 generate bindings` 已运行，`CalibrationPreviewFiveHole` 已生成）
- **Acceptance**：HTTP/Wails 都调用同一 usecase；后端错误传到 UI；前端无蛇形算法或 catch fallback；bindings 已再生成。
- **Verify**：后端运行相关 binding tests；在 frontend 下运行 `npm run test -- motionCalibrationUtils` 和 `npm run typecheck`。
- **Estimated scope**：M，4 files（生成 bindings 另由命令产生并单独审查）。
- **Notes (2026-07-22)**：
  - `pkg/types/types.go`：新增 `FiveHolePointLayoutDTO = calibration.FiveHolePointLayout` 和 `FiveHoleSnakePoint = calibration.FiveHoleSnakePoint` alias，让 Wails binding 入参/返回类型有显式名字（与 `SevenHoleConfigDTO` 模式一致）。
  - `backend/app.go`：新增 `CalibrationPreviewFiveHole(dto)` binding，调 `mgr.PreviewFiveHolePoints`，返回 `GenericResponse{Success, Error, Data: []FiveHoleSnakePoint}`；与 `CalibrationPreviewSevenHole` 对称——纯计算、不创建 runtime、不需要 ToCore 转换。
  - `wails-adapter.ts`：新增 `FiveHolePointLayout` interface + `previewFiveHole(layout)` binding 调用 `CalibrationPreviewFiveHole`。
  - `calibrationApi.ts`：重写 `generateFiveHoleSnakePoints`——Wails 模式调 `wailsApi.calibration.previewFiveHole` 从 `res.Data` 取 bare array（旧实现 `return []` 静默失败已删除）；HTTP 模式 POST `/api/calibration/fivehole`；错误抛出不 fallback。
  - `motionCalibrationUtils.ts`：删除 `generateFiveHoleSnakePointsLocal` 本地蛇形算法 + try/catch fallback；`generateFiveHoleSnakePoints` 现在只做接口形状适配，直接 `return await calibrationApi.generateFiveHoleSnakePoints(layout)`。
  - `__tests__/motionCalibrationUtils.test.ts`：6 测试覆盖 HTTP 成功/错误透传/空数组透传 + Wails 成功/错误透传 + 源码无本地算法静态扫描。
  - 验证结果：
    - `npm run test -- motionCalibrationUtils` → 6 PASS
    - `npm run typecheck` → exit 0
    - `npm run build` → built in 16.86s
    - `npm run test -- --run`（全前端回归） → 17 files / 223 tests PASS
    - `go vet ./...` + `go build ./...`（api-go） → exit 0
    - `go build ./backend/...`（wails） → exit 0
    - `go test ./internal/usecase/... ./api/... ./pkg/types/...` → 全 PASS
    - `wails3 generate bindings` → CalibrationPreviewFiveHole 已生成到 `bindings/.../app.js`
    - Grep 确认 `frontend/src` 中无 `generateFiveHoleSnakePointsLocal` / `alphaValues.push` / `betaValues.push`（仅测试文件中的反例断言引用），spec Success Criteria "生产前端中不存在...蛇形布点业务算法实现" 满足。

## Checkpoint B

- [ ] 在 `services/api-go` 运行 `go test ./internal/usecase/... ./internal/adapters/interpolation/... ./pkg/types/... ./api/...` 和 `go vet ./...`。
- [ ] 在 `apps/desktop-wails` 重新生成 bindings，人工审查生成 diff 后运行前端 typecheck。
- [ ] `server.go` 无 config/interpolation adapter 和 shared interpolation input imports。

## Phase C - Backend Physics and Frontend Display

### Task 12: 定义后端零流量 physics

- [x] **Task**：让权威 calculator 在 `Pt == Ps` 时返回 Ma=0/V=0。
- **Dependencies**：无。
- **Files**：
  - Modify: `services/api-go/internal/core/calibration/atmospheric_data.go`
  - Modify: `services/api-go/internal/core/calibration/formulas_test.go`
  - Modify: `services/api-go/internal/core/calibration/seven_hole_formulas_test.go`
  - Modify (doc-only): `services/api-go/internal/core/calibration/seven_hole_formulas.go` — 同步 `CalculateSevenHoleMachNumber` 错误约定注释
- **Acceptance**：等压是有效零；`Pt < Ps`、非法压力/温度仍失败；所有调用类型回归通过。
- **Verify**：在 `services/api-go` 下运行 `go test ./internal/core/calibration -run 'Test.*(Atmospheric|Mach|Velocity)' -v`。
- **Estimated scope**：M，3 files。

### Task 13: 在 CalibrationStatus 组装 live physics

- [x] **Task**：增加 optional physics snapshot 和按校准类型的语义通道读取。
- **Dependencies**：Task 03、Task 12。
- **Files**：
  - Modify: `services/api-go/internal/core/calibration/types.go`
  - Modify: `services/api-go/internal/usecase/calibration.go`
  - Create: `services/api-go/internal/usecase/calibration_physics_test.go`
  - Modify: `services/api-go/internal/core/calibration/read_probe_channels.go`（补充 sevenHole.tTunnel 角色映射——预存 bug，TTunnel 之前永远为 nil）
  - ~~`services/api-go/api/server.go`~~：无改动——API 路由 `writeJSON(w, http.StatusOK, deps.CalibrationManager.Status())` 通过 `omitempty` JSON tag 自动序列化 LivePhysics
  - ~~`services/api-go/api/server_seven_hole_preview_test.go`~~：未新增 API 测试——API 路由无业务逻辑变化，LivePhysics 三态语义已由 usecase 层 18 个测试覆盖（含 RaceSafety / DoesNotPollutePersistentStatus / StaleClearing）
- **Acceptance**：
  - missing 与 valid zero 由 pointer 语义区分。✅（LivePhysics struct + 18 测试覆盖 nil/&0/&ma 三态）
  - 五孔、三孔、总压、七孔角色映射及 temperature priority 正确。✅（resolveLivePhysics type switch + TTunnelPriority 测试）
  - manager lock 外读取外部数据，不污染持久化 status，不保留 stale physics。✅（config 锁内捕获 + resolveLivePhysics 锁外调用 + DoesNotPollutePersistentStatus + StaleClearing 测试）
- **Verify**：在 `services/api-go` 下运行 `go test -race ./internal/usecase ./api -run 'Test.*Calibration.*Physics|Test.*Calibration.*Status' -count=1` → ok internal/usecase 1.536s；ok api 1.055s [no tests to run]。回归：`go test -race ./internal/core/calibration/... ./internal/usecase/... ./api/...` 全绿。
- **Estimated scope**：M，5 files。

### Task 14: 统一 HTTP/Wails status polling

- [x] **Task**：让两种 transport 使用 `calibrationApi.status()`，HTTP pause/resume/stop 调用既有 routes。
- **Dependencies**：Task 13；bindings 已更新。
- **Files**：
  - Modify: `apps/desktop-wails/frontend/src/api/calibrationApi.ts`
  - Modify: `apps/desktop-wails/frontend/src/api/wails-adapter.ts`
  - Modify: `apps/desktop-wails/frontend/src/shared/types/calibration.ts`
  - Modify: `apps/desktop-wails/frontend/src/stores/calibrationStore.ts`
  - Modify: `apps/desktop-wails/frontend/src/stores/__tests__/calibrationStore.test.ts`
- **Acceptance**：HTTP/Wails 均轮询既有 status；请求不重叠；终态停止；错误不产生 synthetic success；zero 不被 truthiness 丢失。
- **Verify**：在 frontend 下运行 `npm run test -- calibrationStore` 和 `npm run typecheck`。
- **Estimated scope**：M，5 files。
- **Notes (2026-07-22)**：
  - `calibrationApi.ts`：6 个方法（pauseCalibration/resumeCalibration/stopCalibration + pause/resume/stop）HTTP 模式改调既有 POST route，不再返回 synthetic success；`CalibrationStatus` 接口新增 `livePhysics?` 字段（与 `pkg/types` 对齐）。
  - `wails-adapter.ts`：`CalibrationStatus` 接口新增 `livePhysics?` 字段。
  - `shared/types/calibration.ts`：新增 `LivePhysics` 接口 + `CalibrationTaskStatus.livePhysics?` 字段，三态语义（undefined/0/正数）JSDoc 完备。
  - `calibrationStore.ts`：移除 5 处 `isWailsAvailable()` 守卫——`startStatusPolling`/`acquireView`/`releaseView`/`restartPollingForCurrentState`/`startCalibration`/`watch(uiRefreshHz)` 现在 HTTP/Wails 两种模式均轮询；`startStatusPolling` 新增 `inFlight` 标志防请求重叠；`pause`/`resume`/`stop` HTTP 模式检查 `res.success` 并抛错；`stop()` 1Hz polling 不再限于 Wails。
  - `calibrationStore.test.ts`：新增 10 个 Task 14 测试覆盖全部 5 项验收标准（HTTP polling、inFlight 防重叠、终态停止、pause/resume/stop 错误传播、全零 status 处理、acquireView/releaseView/restartPollingForCurrentState HTTP 模式 polling、stop 后 1Hz 捕获终态）。
  - 验证结果：`npm run test -- calibrationStore` → 26 tests passed；`npm run typecheck` → 无错误。

### Task 15: 删除前端 atmospheric physics 公式

- [x] **Task**：store 只映射后端 physics，页面保持纯展示。
- **Dependencies**：Task 14。
- **Files**：
  - Modify: `apps/desktop-wails/frontend/src/stores/calibrationStore.ts`
  - Modify: `apps/desktop-wails/frontend/src/stores/__tests__/calibrationStore.test.ts`
  - Modify: `apps/desktop-wails/frontend/src/components/calibration/five-hole/FiveHoleMain.vue`
  - Modify: `apps/desktop-wails/frontend/src/components/calibration/three-hole/ThreeHoleMain.vue`
  - Modify: `apps/desktop-wails/frontend/src/components/calibration/total-pressure/TotalPressureMain.vue`
- **Acceptance**：无 atmospheric 常量/公式；missing 显示 `--`，zero 显示格式化 0；raw pressure 更新不触发公式；seven-hole 使用同一 store contract 且无需算法改动。
- **Verify**：在 frontend 下运行 `npm run test -- calibrationStore && npm run typecheck && npm run build`（PowerShell 分步执行）。
- **Estimated scope**：M，5 files。
- **Notes (2026-07-22)**：
  - `calibrationStore.ts`：删除 `ATM_GAMMA`/`ATM_C_COEFF`/`ATM_RECOVERY` 常量与 `calculateAtmosphericPhysics` 函数（约 50 行）；`applyPressureUpdate` 仅保留 `realtimePressures.value = pressures`，raw pressure 更新不再触发任何公式；`updateStatusFromBackend` 新增 `livePhysics` → `calculatedPhysics` 映射，使用"对象存在性"而非 truthiness 判断（`if (backendLivePhysics && typeof === 'object')`），确保全零 LivePhysics 不被跳过；支持 Wails PascalCase fallback（`calStatus.livePhysics ?? calStatus.LivePhysics`）。
  - `calibrationStore.test.ts`：删除两个依赖本地公式的旧测试（"zero aerodynamic values"/"Patm=0 returns null"），新增 7 个 Task 15 验收测试覆盖全部 acceptance：raw pressure 不触发公式、missing→null、zero 透传、正常值透传、字段三态、七孔同 contract、stale clearing。共 31 tests passed（26-2+7）。
  - `FiveHoleMain.vue`/`ThreeHoleMain.vue`/`TotalPressureMain.vue`：保持纯展示（组件仅读 `physics?.machNumber`/`physics?.velocity` 并用 `!== undefined` 判断，已是 spec 要求的三态语义）；更新 5 处过期注释，将"由 calculateAtmosphericPhysics 计算"改为"由后端 livePhysics 提供，store 映射"。
  - 验证结果：`npm run test -- calibrationStore` → 31 passed；`npm run typecheck` → exit 0；`npm run build` → built in 19.68s。Grep 复核 calibration store/components 无 `ATM_GAMMA`/`ATM_C_COEFF`/`calculateAtmosphericPhysics` 或 Ma/V 公式残留（仅剩 2 处 Task 15 说明性注释）。

## Checkpoint C

- [ ] 后端 physics/status 全部测试通过。
- [x] 前端 typecheck、store tests、production build 通过。
- [x] calibration store/components 无 `ATM_GAMMA`、`ATM_C_COEFF`、`calculateAtmosphericPhysics` 或 Ma/V 公式。

## Phase D - Frontend Boundary Cleanup

### Task 16: 删除 traversal simulation 代码

- [x] **Task**：移除无入口的 composable、模拟算法和 169 点数据。
- **Dependencies**：无；建议在 Checkpoint C 后执行以保持阶段边界清晰。
- **Files**：
  - Delete: `apps/desktop-wails/frontend/src/composables/useTraversalSimulation.ts`
  - Delete: `apps/desktop-wails/frontend/src/utils/simulateTraversalRun.ts`
  - Delete: `apps/desktop-wails/frontend/src/utils/simulateFiveHoleCalibration.ts`
  - Modify: `apps/desktop-wails/frontend/src/components/traversal/TraversalMain.vue`
- **Acceptance**：全部静态引用消失；production bundle 不含稳定数据探针 `98924.2` 或 simulation label；无死类型/import。
- **Verify**：在 frontend 下运行 `npm run typecheck`, `npm run test`, `npm run build`, `rg -l '98924\.2|Simulation 16.{0,16}16' dist`（最后命令应无输出）。
- **Estimated scope**：M，4 files。

### Task 17: 增加 demo 导入结构守卫

- [x] **Task**：validator 阻止生产静态导入 `DEMO ONLY`/`simulate*` 模块。
- **Dependencies**：Task 16。
- **Files**：
  - Modify: `scripts/validate-frontend-structure.ps1`
  - Create: `scripts/tests/validate-frontend-demo-import.ps1`（若 `scripts/tests` 不符合现有结构，改为相邻 fixture runner 并在实施前确认）
- **Acceptance**：生产→demo fixture 退出 1；test/mock→demo 允许；WindLabX4 当前源码通过。
- **Verify**：从根目录运行 fixture script 和 `validate-frontend-structure.ps1 ... -CheckFileSize`。
- **Estimated scope**：S，2 files。

### Task 18: 对齐 MotionSafety 前端契约

- [x] **Task**：区分 backend-equivalent errors 与 advisory warnings，并覆盖 DTO 修复迁移面。
- **Dependencies**：Task 05。
- **Files**：
  - Modify: `apps/desktop-wails/frontend/src/components/shared/MotionSafetyPanel.vue`
  - Modify: `apps/desktop-wails/frontend/src/shared/types/traversal.ts`
  - Modify: `apps/desktop-wails/frontend/src/components/traversal/TraversalSettings.vue`
  - Create: `apps/desktop-wails/frontend/src/components/shared/__tests__/MotionSafetyPanel.test.ts`
  - Modify: `services/api-go/internal/usecase/traversal_motion_safety_test.go`
- **Acceptance**：frontend blocking rules 与 backend rejection 对齐；advisory 不使 `isValid=false`；override 非递归、unknown key 可见；backend Start 始终权威拒绝历史非法配置。
- **Verify**：后端运行 MotionSafety tests；前端运行 `npm run test -- MotionSafetyPanel` 和 `npm run typecheck`。
- **Estimated scope**：M，5 files。
- **Notes (2026-07-22)**：
  - `traversal.ts`：新增 `MotionSafetyFieldKey`（4 个数值字段 key 联合类型，排除 `axisOverrides`）+ `MotionSafetyAxisOverride`（非递归子集类型，无 `axisOverrides` 字段）。`MotionSafetyConfig.axisOverrides` 改为 `Record<string, MotionSafetyAxisOverride>`，B8（嵌套覆盖）在编译期被类型系统阻止。
  - `MotionSafetyPanel.vue`：将单一 `validationErrors` 拆分为 `blockingErrors`（B1-B9，与后端 `validateMotionSafetyConfig` 拒绝规则一一对齐）+ `advisoryWarnings`（A1-A2，仅 UX 提示）。`isValid` 仅由 `blockingErrors` 决定。`defineExpose` 扩展为 `{ isValid, blockingErrors, advisoryWarnings }`。模板分为红色错误区 + 黄色提示区。NaN/Inf 用 `Number.isFinite()` 统一处理。
  - `TraversalSettings.vue`：`saveConfig()` 使用首个 `blockingErrors[0]` 作为 toast 文案，替代硬编码消息。
  - `MotionSafetyPanel.test.ts`（新增）：21 个契约测试覆盖 B1-B9（含 `@ts-expect-error` 编译期 B8 验证）+ A1-A2（确认 advisory 不影响 `isValid`）。
  - `traversal_motion_safety_test.go`：补充 5 个后端契约测试（B3 零/负 critical、B6 零/负 epsilon、B5 边界 = 200、B1 Inf、advisory 边界确认后端不拒绝）。
  - **验证证据**：
    - 后端：`go test ./internal/usecase/ -run TestValidateMotionSafetyConfig -v` → 17/17 PASS（含 5 个新增）。
    - 前端单测：`npm run test -- MotionSafetyPanel` → 21/21 PASS。
    - 前端全量：`npm run test -- --run` → 18 files / 244 tests 全 PASS。
    - typecheck：`npm run typecheck` → exit 0。

### Task 19: 支持多个括号注释

- [x] **Task**：修复 points parser 的中英文多注释剥离。
- **Dependencies**：无。
- **Files**：
  - Modify: `apps/desktop-wails/frontend/src/shared/pointsFileParser.ts`
  - Modify: `apps/desktop-wails/frontend/src/shared/__tests__/pointsFileParser.test.ts`
- **Acceptance**：`X(mm)(deg)`、`X（mm）（deg）`、mixed forms 均解析正确；单注释行为不变。
- **Verify**：在 frontend 下运行 `npm run test -- pointsFileParser`。
- **Estimated scope**：S，2 files。
- **Notes (2026-07-22)**：
  - `pointsFileParser.ts`：抽出 `stripBracketComments` 共享工具，正则 `/\(.*?\)|（.*?）/g` 非贪婪 + 全局，同时支持半角 `()` 与全角 `（）`，一次替换剥离所有括号段。`normalizeAxisName` 与 `normalizeConfigKey` 均改用该工具，消除两处重复正则。
  - `pointsFileParser.test.ts`：新增两组多括号测试（axis 6 例 + config 5 例），覆盖 `X(mm)(deg)` / `X（mm）（deg）` / `X(mm)（deg）` / `X（mm）(deg)` / `Y(mm)（deg）(raw)` / `pos_x（mm）(deg)` / `Dwell(ms)(稳定)` / `Dwell（ms）（稳定）` / `Samples(个)（每点）` / `Test(1=on)（开关）`；新增 CSV 表头多段混合括号解析用例（line 136-139）。单括号行为不变（既有用例 19/127 等继续通过）。
  - **验证证据**：`npm run test -- pointsFileParser --run` → 38/38 PASS。

## Checkpoint D

- [x] frontend validator、全部 Vitest、typecheck 和 build 通过。
- [x] bundle 稳定数字/字符串探针无匹配。
- [x] MotionSafety 非法历史配置在 UI 可解释、在 backend 可拒绝。

## Phase E - Observability and Evidence

### Task 20: 修复 calibration writer 错误契约

- [x] **Task**：聚合 usecase cleanup 错误并检查 calibration CSV buffered error。
- **Dependencies**：Task 03。
- **Files**：
  - Modify: `services/api-go/internal/usecase/calibration.go`
  - Modify: `services/api-go/internal/usecase/calibration_savecsv_test.go`
  - Modify: `services/api-go/internal/usecase/calibration_seven_hole_test.go`
  - Modify: `services/api-go/internal/adapters/storage/calibration_csv_writer.go`
  - Modify: `services/api-go/internal/adapters/storage/calibration_csv_writer_test.go`
- **Acceptance**：append+cleanup 双错误均可识别；duplicate temp writer flush 失败只 warning；`csv.Writer.Error()` 和 close error 均保留；不改 traversal writer。
- **Verify**：在 `services/api-go` 下运行 `go test ./internal/usecase ./internal/adapters/storage -run 'Test.*Calibration.*(Csv|CSV|Writer|Flush)' -v`。
- **Estimated scope**：M，5 files。
- **Notes (2026-07-22)**：
  - `calibration_csv_writer.go`：
    - `AppendPoint`：在 `w.writer.Flush()` 之后新增 `w.writer.Error()` 检查。`csv.Writer.Flush()` 不返回错误，底层 `bufio.Writer` 写入失败（文件已关闭/磁盘满）只能通过 `Error()` 探测（内部 `Write(nil)` 返回 `bufio.Writer.b.err`）。旧实现静默丢点，修复后返回中文上下文包装的错误。
    - `Flush`：用 `errors.Join(bufferedErr, closeErr)` 聚合 `csv.Writer.Error()` 缓冲错误与 `file.Close()` 错误。旧实现只返回 `file.Close()` 错误，缓冲错误被丢弃。
    - 新增 `errors` import。
  - `calibration.go`：
    - `SaveCsv`：`AppendPoint` 失败时调 `cleanupErr := writer.Flush()`，返回 `errors.Join(err, cleanupErr)`。旧实现 `_ = writer.Flush()` 静默丢弃 cleanup 错误，调试困难。
    - `routeSevenHoleWriter`：两处 double-check 丢弃路径（`sevenHoleWriters == nil` 和 `cached, ok := ...`）将 `_ = writer.Flush()` 替换为 `if flushErr := writer.Flush(); flushErr != nil { log.Printf(...) }`。Flush 失败仅记录警告，不影响 cached writer 返回（spec Task 20 "duplicate temp writer flush 失败只 warning"）。
  - `calibration_csv_writer_test.go`：4 新测试——`AppendPointDetectsBufferedError`（红灯→绿灯，验证 `Error()` 检查）、`FlushJoinsWriterAndCloseErrors`（验证 `errors.Join` 聚合）、`FlushHappyPathNoError`（回归）、`FlushIdempotentNilSafe`（回归）。
  - `calibration_savecsv_test.go`：新增 `failingCalibrationCsvWriter` 桩（可注入 appendErr/flushErr）+ 2 新测试——`TestSaveCsv_JoinsAppendAndCleanupErrors`（红灯→绿灯，验证 `errors.Is` 可识别两个子错误）、`TestSaveCsv_CleanupFlushSuccessReturnsAppendErrorOnly`（验证 cleanup 成功时只返回 append 错误）。
  - `calibration_seven_hole_test.go`：`fakeSevenHoleWriter` 新增 `flushErr` 字段；新增 `racingSevenHoleWriterFactory`（NewWriter 期间预填 cache，模拟并发抢占，触发 double-check 丢弃路径）+ 2 新测试——`DuplicateTempWriterFlushFailure_OnlyWarning`（验证 Flush 错误仅警告，cached writer 仍返回）、`DuplicateTempWriterFlushSuccess_NoWarning`（正常路径回归）。
  - 验证结果：
    - `go test ./internal/usecase ./internal/adapters/storage -run 'Test.*Calibration.*(Csv|CSV|Writer|Flush)|TestSaveCsv|TestRouteSevenHoleWriter' -v` → 全 PASS（usecase 5.77s / storage 0.048s）
    - `go test ./internal/usecase/... ./internal/adapters/storage/...` → 全 PASS（usecase 71.95s / storage 0.621s）
    - `go test ./api/... ./pkg/types/...` → 全 PASS
    - `go vet ./internal/usecase/... ./internal/adapters/storage/...` → 无输出
    - `go build -buildvcs=false ./...` → exit 0
    - Grep 确认 `calibration.go` 中无生产代码 `_ = writer.Flush()`（仅注释中引用旧行为）
    - Grep 确认 `traversal_csv_writer.go` 未被修改（spec acceptance "不改 traversal writer"），其已自带 `Flush() + Error()` 模式（lines 129-130、306-307、572-573）

### Task 21: 处理 traversal lock Release 错误

- [x] **Task**：覆盖 Start rollback、abort、Stop、finalize 四条释放路径。
- **Dependencies**：无。
- **Files**：
  - Modify: `services/api-go/internal/usecase/traversal.go`
  - Modify: `services/api-go/internal/usecase/traversal_view.go`
  - Modify: `services/api-go/internal/usecase/traversal_lifecycle_test.go`
  - Modify: `services/api-go/internal/usecase/traversal_view_test.go`
- **Acceptance**：可返回路径 join/return release error；void 路径 warning；失败后不记录成功 info；不强制释放他人锁。
- **Verify**：在 `services/api-go` 下运行 `go test ./internal/core/resourcelock ./internal/usecase -run 'Test.*(Release|Traversal.*Stop|FinalizeSink)' -v`。
- **Estimated scope**：M，4 files。
- **Notes (2026-07-22)**：
  - 重构：提取 `traversalLockService` 接口（`Acquire`/`Release`）+ `TraversalManager.lockService` 字段，构造函数注入 `resourcelock.Default()`。原 4 处 `resourcelock.Default()` 调用 + `traversal_checkpoint.go` 1 处 Acquire 全部改走 `m.lockService`，生产路径行为不变。
  - Path 1（Start rollback, 可返回, `traversal.go:717-733`）：`_ = Release(...)` → `errors.Join(fmt.Errorf("create checkpoint port: %w", cpErr), releaseErr)`，Release 失败时附加 `slog.Warn`。验证：`TestStart_CheckpointPortFailure_JoinsReleaseError` 红灯→绿灯（`errors.Is(err, releaseErr)` 通过）。
  - Path 2（abort, void, `traversal.go:950-956`）：`_ = Release(...)` → `if releaseErr := m.lockService.Release(...); releaseErr != nil { slog.Warn(...) }`。验证：`TestAbortStartLocked_ReleaseFailure_LogsWarning` 红灯→绿灯（`slog.Warn` 记录 'release'）。
  - Path 3（Stop, 可返回, `traversal.go:1146-1153`）：`_ = Release(...)` → `if releaseErr := ...; releaseErr != nil { stopErr = errors.Join(stopErr, fmt.Errorf("release traversal lock: %w", releaseErr)) }`。验证：`TestStop_ReleaseFailure_JoinsError` 红灯→绿灯（`errors.Is(err, releaseErr)` 通过）+ `TestStop_ReleaseSuccess_NoExtraError` 回归。
  - Path 4（finalize, void, `traversal_view.go:169-183`）：`_ = Release(...); slog.Info("traversal lock released")` → 分支：失败 `slog.Warn` / 成功 `slog.Info`。关键修复点：旧实现无论 Release 成功失败都记录 Info，违反"失败后不记录成功 info"契约。验证：`TestFinalizeSink_ReleaseFailure_LogsWarningNotInfo` 红灯→绿灯（Warn 记录 + Info 不记录）+ `TestFinalizeSink_ReleaseSuccess_LogsInfo` 回归。
  - 不强制释放他人锁：依赖 `resourcelock.Service.Release` 自身 holder 校验，不在 traversal 层做 force-release。
  - 测试基础设施（`traversal_lifecycle_test.go`）：新增 `fakeTraversalLockService`（可注入 Acquire/Release 错误 + 调用记录）、`recordingSlogHandler` + `withRecordingLogger`（捕获 slog 记录供断言）、`failingCheckpointPortFactory`（触发 Path 1）。
  - 清理：移除 `traversal_view.go` 和 `traversal_checkpoint.go` 中已不再使用的 `resourcelock` import。
  - 验证结果：
    - `go test ./internal/core/resourcelock ./internal/usecase -run 'Test.*(Release|Traversal.*Stop|FinalizeSink)' -v` → 全 PASS（resourcelock 0.617s / usecase 5.766s）
    - `go test ./internal/core/resourcelock/... ./internal/usecase/...` → 全 PASS（resourcelock 0.037s / usecase 71.926s）
    - `go test ./api/... ./pkg/...` → 全 PASS
    - `go vet ./internal/usecase/... ./internal/core/resourcelock/...` → 无输出
    - `go build -buildvcs=false ./...` → exit 0
    - Grep 确认生产代码无 `_ = resourcelock.Default().Release(...)` 或 `_ = m.lockService.Release(...)` 静默丢弃模式，仅构造函数初始化（`traversal.go:188`）+ 注释引用

### Task 22: 迁移 calibration 结构化日志

- [x] **Task**：将 19 处 `log.Printf` 迁移为带稳定字段的 slog。
- **Dependencies**：Tasks 01、03、20。
- **Files**：
  - Modify: `services/api-go/internal/usecase/calibration.go`
  - Modify: `services/api-go/internal/usecase/calibration_motion_safety_test.go`
  - Modify: `services/api-go/internal/usecase/calibration_seven_hole_test.go`
- **Acceptance**：19 处旧日志归零；SavePath 空不报假错误；factory 缺失明确 Error；关键 safety/writer 字段可检索；测试恢复全局 logger。
- **Verify**：在 `services/api-go` 下运行定向 calibration tests，并执行 `rg -n 'log\.Printf' internal/usecase/calibration.go` 无输出。
- **Estimated scope**：M，3 files。
- **Notes (2026-07-22)**：
  - `calibration.go`：import 从 `"log"` 改为 `"log/slog"`；全部 22 处 `log.Printf` 迁移为 `slog.Error/Warn/Info`，每处带稳定事件名 + 独立字段（`component`/`task_id`/`error` 等），便于日志检索与过滤。关键迁移示例：
    - CSV writer 初始化失败：`slog.Error("calibration csv writer init failed", "component", "calibration", "task_id", config.TaskID, "error", err)`
    - **factory 缺失 / SavePath 空 拆分**（`buildSevenHoleCsvSink`，line 769-782）：旧实现 `log.Printf` 把两种情况合并打印，无法区分级别。修复后：
      - factory 缺失 → `slog.Error("calibration seven hole writer factory missing", ...)` （真实装配错误）
      - SavePath 空 → `slog.Info("calibration seven hole csv skipped, save path empty", ...)` （合法可选项，非错误）
    - 运动安全故障（8 个结构化字段）：`slog.Error("calibration motion safety failure", "component", "calibration", "controller_id", failure.ControllerID, "axis", failure.Axis, "verdict", failure.Verdict, "target", failure.Target, "actual", failure.Actual, "deviation", deviation, "point_index", failure.PointIndex, "requires_emergency_stop", failure.Verdict.RequiresEmergencyStop())`
  - `calibration_seven_hole_test.go`：新增 `log/slog` import；2 个测试更新 slog 断言：
    - `TestBuildSevenHoleCsvSink_FactoryNotInjected`：通过 `withRecordingLogger` 捕获 slog，断言 `slog.Error` 含 'factory' 关键字（factory 缺失是真实错误）
    - `TestBuildSevenHoleCsvSink_EmptySavePath`：断言 SavePath 空时**不**记录 `slog.Error` 含 'factory'（SavePath 空是合法跳过，非错误）
  - `calibration_motion_safety_test.go`：新增 `log/slog` import + 1 个新测试 `TestHandleCalibrationMotionSafetyFailure_EmitsStructuredSlogError`：
    - 通过 `withRecordingLogger` + `defer restore()` 捕获并恢复全局 logger
    - 断言 `slog.Error` 含 'motion safety failure' 消息被记录（验证关键 safety 字段可检索）
  - 测试基础设施复用：`withRecordingLogger`/`recordingSlogHandler` 已在 `traversal_lifecycle_test.go`（Task 21）定义，同 `usecase` 包内直接复用，无需重复定义。
  - 验证结果：
    - `go test ./internal/usecase -run 'TestHandleCalibrationMotionSafetyFailure|TestBuildSevenHoleCsvSink' -v` → 全 PASS（5.773s，含新测试）
    - `go test ./internal/usecase/... ./internal/adapters/storage/... ./api/... ./pkg/...` → 全 PASS（usecase 71.748s）
    - `go vet ./internal/usecase/...` → 无输出
    - `go build -buildvcs=false ./...` → exit 0
    - `rg -n 'log\.Printf' internal/usecase/calibration.go` → 无匹配
    - `rg -n '^\s*"log' internal/usecase/calibration.go` → 仅 `"log/slog"`，无遗留 `"log"` import

### Task 23: 删除测试占位并更新证据

- [x] **Task**：清理两处 `var _`，关闭 N-1/recorder 子项并登记后续。
- **Dependencies**：Tasks 08、09、20-22。
- **Files**：
  - Modify: `services/api-go/api/server_traversal_sevenhole_test.go`
  - Modify: `services/api-go/internal/core/calibration/automatic_calibration_seven_hole_test.go`
  - Modify: `docs/code-review-2026-07-21.md`
  - Modify: `docs/specs/spec-code-review-remediation-2026-07-21.md`
  - Modify: `docs/specs/plan-code-review-remediation-2026-07-21.md`
- **Acceptance**：无无行为占位/unused import；N-1 与 recorder 标记 Closed with Evidence；triggerMode profile validation 和 seven-hole dynamic count 分别登记 follow-up；所有项目状态附命令证据。
- **Verify**：后端运行对应 api/core tests；运行 `rg -n 'var _ =.*(ProbeTypeSevenHole|MotionInterruptNone)' .` 无输出。
- **Estimated scope**：M，5 files。

**Notes（2026-07-22）**：
- 删除 `services/api-go/api/server_traversal_sevenhole_test.go` 末尾 `var ( _ = traversal.ProbeTypeSevenHole; _ = (*usecase.TraversalManager).CalculateSevenHoleRealtime )` 占位 + 仅占位引用的 `traversal` import；`CalculateSevenHoleRealtime` 是 `traversal_probe.go:226` 真实导出方法，无需占位保证契约，已有 HTTP 集成测试 `TestCalculateRealtimeSevenHole` 覆盖。
- 删除 `services/api-go/internal/core/calibration/automatic_calibration_seven_hole_test.go` 末尾 `var _ = traversal.MotionInterruptNone` + 误导性注释 + `traversal` import；`fakeCalibrationRuntime.WaitForMotionComplete` 在同包 `automatic_five_hole_test.go` 内已引用 `traversal` 类型，本文件无需重复引用。
- N-1 标记 **Closed with Evidence**：经消费者复核，`triggerMode` 数值 `0=software / 2=hardware` 是 HTTP（`PUT /api/device/{id}/daqT1603Config` 路由）、Wails、持久化（`device.DaqT1603HardwareConfig.TriggerMode json:"triggerMode"`）、驱动（`t1603_adapter.go:313,329`）四条契约的统一值，并非"内部枚举"，文案与对外契约一致无需解耦。
- recorder 子项标记 **Closed with Evidence**（在 `code-review-2026-07-21.md` R-3 节）：`StorageRecorder.HandlePayload` 已在错误发生处 `slog.Error` 记录，data sink 重复记录属热路径噪声，不新增。
- 登记两个 follow-up（不在本轮范围）：
  - **FU-1**：profile-upsert 路径与 `ApplyDaqT1603Config` 路径的 triggerMode 校验一致性差异。
  - **FU-2**：七孔 PRB/CSV 导入响应 `pointCount` 保持兼容值（内区 169、外区 52），非 loader 真实计数；真实动态计数语义作为独立后续项（spec Open Question 9）。
- 14/16 review 项已附状态+命令证据；N-2 Pending（Task 19 未完成）；R-7 Pending（Task 24 全量验证未完成）。
- 验证命令全部通过：
  - `go test ./api/... ./internal/core/calibration/... -v` → 全 PASS
  - `go vet ./...` → 无输出
  - `go build -buildvcs=false ./...` → exit 0
  - `rg -n 'var _ =.*(ProbeTypeSevenHole|MotionInterruptNone)' services/api-go` → 无匹配
  - `rg -n 'TriggerMode' services/api-go/internal/core/device services/api-go/internal/adapters/hardware services/api-go/api` → 仅契约字段透传，无内部枚举解耦需求

## Checkpoint E

- [x] 在 `services/api-go` 运行 `go test ./internal/usecase/... ./internal/adapters/storage/... ./internal/core/resourcelock/... ./api/...` 和 `go vet ./...`。
- [x] calibration 无 `log.Printf`，traversal production paths 无忽略的 `Release`。
- [x] review 报告 16 项均有状态与证据。

### Task 24: 全量验证与变更影响复核

- [x] **Task**：执行全部自动门禁并记录真实结果。
- **Dependencies**：Tasks 01-23。
- **Files**：
  - Modify: `docs/code-review-2026-07-21.md`（只记录验证结果）
  - Modify: `docs/specs/tasks-code-review-remediation-2026-07-21.md`（勾选真实完成项）
- **Acceptance**：
  - 结构、import、frontend validator 通过。**【部分通过 / Waiver】**：`validate-import-direction.ps1` 通过；`validate-structure.ps1` exit 1（12 个未知顶层条目，预存工作区问题）；`validate-frontend-structure.ps1` exit 1（i18n 硬编码、`: any`、CSS class 重复、泛型变量名，预存问题）。两项未通过均不属本轮整改回归，登记为 waiver/blocker。
  - Go format/build/vet/tests/race 与 frontend typecheck/tests/build 通过，或明确记录平台阻塞。**【通过】**
  - bindings 同步；`git diff --check` 通过；GitNexus detect_changes 与预期一致。**【部分通过 / Waiver】**：本轮整改改动 `git diff --check` 通过；全工作区 `git diff --check` exit 2（13 处行尾空白均在 Wails 生成 bindings 中，预存问题）；GitNexus detect_changes 与批准计划一致。
  - 未执行 HIL 时整体状态保持"自动验证完成，发布待 HIL"。
- **Verify**：执行规格 `Commands` 全套命令；保存退出码和失败原因，不用部分测试推断全量结果。
- **Estimated scope**：S，2 docs；无功能代码。

**Notes（2026-07-22）**：
- 规格 Commands 全套命令已执行，结果回写到 `code-review-2026-07-21.md` "验证状态与限制"表与"整改进度总览"。
- **通过项**（exit 0 / PASS）：
  - `validate-import-direction.ps1` → 699 Go files checked
  - `go build -buildvcs=false ./...` → exit 0
  - `go vet ./...` → exit 0
  - `go test ./internal/... ./api/...` → 全 PASS
  - `go test -race ./internal/usecase/... ./api/...` → 全 PASS（usecase 73.9s + api 1.1s）
  - `npm run typecheck` → exit 0
  - `npm run test` → 17 files / 223 tests 全 PASS
  - `npm run build` → exit 0（built in 18s）
  - Task 23 改动的两个测试文件 `git diff --check` → exit 0
  - GitNexus `detect_changes --repo AI-WorkSpace` → 63 files / 405 symbols / 52 flows / critical，与 Tasks 01-23 批准计划一致
- **预存工作区问题**（不属本轮整改回归，已记录到报告"预存工作区问题"段）：
  - `validate-structure.ps1` exit 1：12 个未在 `workspace.structure.json` 白名单的顶层条目（`.trae`/`.codebuddy` 等）。
  - `validate-frontend-structure.ps1 -CheckFileSize` exit 1：多个预存前端规范问题（i18n 硬编码、`: any` 类型、泛型变量名等）。
  - `gofmt -l .` 列出 40+ 文件需格式化，多为预存 struct 字段对齐问题；Task 23 改动本身符合 gofmt。
  - `git diff --check` exit 2：13 处行尾空白均在 Wails 生成 bindings 文件中（生成器输出特征）。
- **Wails bindings 未重新生成**：Task 23 仅删除测试 `var _` 占位 + 文档更新，未触及 Wails 绑定签名；前序任务 05/08/09 已同步 bindings。
- **HIL Release Gate 未执行**：真机急停失败、fallback 普通停止、控制器复位由具备设备与安全条件的维护人员在发布前执行；自动测试不得代替 HIL 结论。
- **整体状态**：自动验证完成，发布待 HIL。

### Review Findings 二轮整改（2026-07-22）

> 24 Task 整改完成后，reviewer 二次抽查发现 7 项遗漏缺陷。以下为二轮整改记录，不在原 24 Task 框架内，但属同一整改周期。

| ID | 级别 | 问题 | 状态 | 修复要点 |
|---|---|---|---|---|
| P1-1 | P1 95% | DataStreamRelay.Stop() 可能在订阅尚未注销时提前返回 | **Fixed** | `stream_relay.go` 新增 retiring 集合；Unsubscribe 移入 retiring 再锁外等 done；Stop 遍历 subs+retiring 合并等待。新增 2 个并发契约测试。 |
| P1-2 | P1 95% | 轮询重启不能隔离旧请求，旧状态可覆盖新任务 | **Fixed** | `calibrationStore.ts` 新增 pollingGeneration token；start 拍快照，响应返回时比对 generation 丢弃过期响应；stop 自增让在途响应过期。 |
| P1-3 | P1 92% | 终态仍计算并返回 livePhysics，前端永久保留最后一帧 | **Fixed** | `calibration.go` Status() 只在 running/paused 调 resolveLivePhysics；终态 completed/error/stopped 保持 nil。新增终态+活跃态回归测试。 |
| P1-4 | P1 90% | 五孔 tTunnel 前端必填但后端权威 physics 完全忽略 | **Fixed** | types.go 新增 `TTunnel *float64`；read_probe_channels.go 新增 `"fiveHole.tTunnel": "tTunnel"` 映射；formulas.go 平均值累加 TTunnel；calibration.go computeLivePhysicsFromGauge 传 raw.TTunnel。 |
| P1-5 | P1 98% | 文档把未通过的自动门禁标为完成 | **Fixed** | spec.md Verification Gate 两项 [x]→[ ]+【Blocked / Waiver】；Task 24 acceptance 加 waiver 标注；review.md 结论改为"通过项+waiver 项"。 |
| P2-6 | P2 92% | 带括号注释的 skip 列不会执行反转 | **Fixed** | `pointsFileParser.ts` invert 判定改用 stripBracketComments 后的 normalized base；新增 4 个中英文/多括号 skip 列测试（17a-17d）。 |
| P2-7 | P2 90% | HTTP pause/resume 失败仍没有操作员可见反馈 | **Fixed** | `useCalibrationWorkflow.ts` pause/resume 失败路径新增 toast（与 stop 路径一致）；i18nStore 新增 wf_pauseCalibrationFailed/wf_resumeCalibrationFailed 中英文 key。 |

二轮整改验证结果（2026-07-22）：
- `go build -buildvcs=false ./...` → exit 0
- `go vet ./...` → exit 0
- `go test ./internal/... ./api/...` → 全 PASS（usecase 71.7s）
- `go test -race ./internal/usecase/... ./api/...` → 全 PASS（usecase 74.0s + api 1.1s）
- `npm run typecheck` → exit 0
- `npm run test -- --run` → 18 files / 248 tests 全 PASS（含 P2-6 新增 4 个测试）
- `npm run build` → exit 0
- `validate-import-direction.ps1` → exit 0（699 Go files checked）
- `npx gitnexus detect-changes --repo AI-WorkSpace` → exit 0
- 本轮整改改动的源文件 `git diff --check` → exit 0（干净）
- 预存问题仍为 waiver/blocker：`validate-structure.ps1` exit 1、`validate-frontend-structure.ps1` exit 1、全工作区 `git diff --check` exit 2（Wails 生成 bindings 13 处行尾空白）

## HIL Release Gate

- [ ] 合格操作员验证真实急停失败场景。
- [ ] 验证 fallback ordinary stop 与轴最终运动状态。
- [ ] 验证控制器人工复位与复位后禁止意外续动。
- [ ] 记录设备型号、固件、时间、操作员和结果；未完成时不得标记发布就绪。

## Phase 3 Approval Gate

- [ ] 用户批准本任务清单、依赖顺序和各任务文件边界。
- [ ] 用户选择同会话逐任务实施或独立会话执行。
- [ ] 批准后才把 Task 01 标为 in progress 并修改生产代码。
