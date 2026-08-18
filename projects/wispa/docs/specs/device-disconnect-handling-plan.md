# Implementation Plan: wispa 设备断连处理改进

> **关联 spec**：[device-disconnect-handling.md](file:///c:/Users/wuzhy/Documents/D/SVN/SoftWare/trunk/AI-Workspace/projects/wispa/docs/specs/device-disconnect-handling.md)
> **状态**：Phase 2 - 待用户 review
> **创建时间**：2026-07-03

## Overview

基于 spec 的三个 P0 改进项，垂直切片为 4 个独立可验证的 Task + 1 个集成验证 Task。每个 Task 横跨"后端改动 + 单测 + 验证"完整路径，不按层水平切片。

## Architecture Decisions

### 决策 1: TCP keepalive 用 `SetKeepAlivePeriod` 单一 API
** rationale **：`SetKeepAlivePeriod` 在 Linux/Windows 均可用，Windows 上等同于设置整个 keepalive 间隔（idle + interval 合并）。不引入 `syscall.SetSockopt` 平台相关代码，保持标准库纯净。代价是无法精细控制 idle/count，但 10s 间隔 × 系统默认探测次数已满足 30s 检测目标。

### 决策 2: handleConnectionLost 内 driver + conn 清理放在锁外
** rationale **：`conn.Close()` 是 I/O 操作，持锁执行会阻塞其他设备访问同分片。readLoop 此时已 return（handleConnectionLost 是 readLoop 退出前最后一步），无并发竞争，锁外关闭安全。

### 决策 3: 单/多设备判定在 app.go 而非 recorder
** rationale **：recorder 是通用录制器，不应感知"设备"概念（未来可能扩展到非设备数据源）。判定逻辑放在 `backend/app.go` 的 `relayStream`，通过 `relays map` 的 key 集合动态判定——任何时候 `len(relays) == 1` 视为单设备场景，断连即停止录制。这避免了在 recorder 引入设备耦合。

### 决策 4: 多设备警告事件用独立 `daq:recording-warning` 而非复用 `daq:recording-status`
** rationale **：`daq:recording-status` 携带 `RecordingSession` 结构，语义是"录制会话状态变更"。断连警告是"数据源健康度告警"，语义不同。混用会让前端难以区分"录制真的停了"和"只是有设备掉线"。

### 决策 5: 不引入 RecordingWarning core 类型，直接用 backend 结构
** rationale **：警告事件载荷简单（deviceID + message），且仅 backend → frontend 单向传递，无业务逻辑。在 `backend/app.go` 内定义 `RecordingWarningEvent` struct 即可，避免污染 `core/`。

## Task List

### Phase 1: 硬件层基础（独立可并行，但同改一文件，建议顺序）

- [ ] **Task 1: 启用 TCP keepalive 加速断连检测**
  - **Description**：在 `Connect` 阶段 TCP 拨号成功后启用 keepalive，解决物理拔线无 RST/FIN 导致的数小时检测延迟。
  - **Acceptance criteria**:
    - [ ] 新增 `enableTCPKeepalive(conn net.Conn) error` 函数
    - [ ] `Connect` 在 `net.DialTimeout` 成功后立即调用，失败时关闭 conn 并返回错误
    - [ ] 非 TCP 连接（mock）返回 nil 不报错
    - [ ] 单元测试 `TestEnableTCPKeepalive_TCPConn` 和 `TestEnableTCPKeepalive_NonTCPConn` 通过
  - **Verification**:
    - [ ] `cd projects\wispa\apps\desktop-wails; $env:GOWORK="off"; go test ./adapters/hardware/ -run TestEnableTCPKeepalive -v`
    - [ ] `go vet ./...`
    - [ ] `go build ./...`
  - **Dependencies**: None
  - **Files likely touched**:
    - `adapters/hardware/p1604_adapter.go`（新增常量 + enableTCPKeepalive 函数 + Connect 调用点）
    - `adapters/hardware/p1604_adapter_test.go`（新增测试文件）
  - **Estimated scope**: S（2 文件）

- [ ] **Task 2: handleConnectionLost 清理 driver + conn**
  - **Description**：在 `handleConnectionLost` 中删除 `shard.drivers[id]` 并在锁外关闭 `driver.conn`，解决 fd 泄漏和重连被拒问题。
  - **Acceptance criteria**:
    - [ ] `handleConnectionLost` 在锁内 `delete(shard.drivers, id)`
    - [ ] `driver.conn.Close()` 在锁外执行（解锁后）
    - [ ] driver 一致性校验保留（避免误伤新 driver）
    - [ ] 幂等性保留（设备已 Disconnected 时不重复清理）
    - [ ] 单元测试覆盖：driver 清理、conn 关闭、driver 一致性、幂等性
  - **Verification**:
    - [ ] `go test ./adapters/hardware/ -run TestHandleConnectionLost -v`
    - [ ] `go vet ./...`
    - [ ] 手动验证：模拟模式下断连后重新 Connect 同 ID 设备成功（无需先 Disconnect）
  - **Dependencies**: 建议在 Task 1 之后（同文件改动，避免合并冲突）
  - **Files likely touched**:
    - `adapters/hardware/p1604_adapter.go`（修改 handleConnectionLost）
    - `adapters/hardware/p1604_adapter_test.go`（新增测试用例）
  - **Estimated scope**: S（2 文件）

### Checkpoint: 硬件层基础完成
- [ ] `go test ./adapters/hardware/ -v` 全绿
- [ ] `go build ./...` + `go vet ./...` 全绿
- [ ] 新增代码覆盖率 ≥ 80%
- [ ] Review with human before proceeding to Phase 2

### Phase 2: 后端录制联动

- [ ] **Task 3: 单设备录制断连自动停止**
  - **Description**：当 `relayStream` 因 channel 关闭退出时，若当前仅有一个活跃 relay（单设备录制场景），自动调用 `recordUC.Stop()` 并在 `LastError` 填充断连原因。
  - **Acceptance criteria**:
    - [ ] `relayStream` 退出时检查 `len(a.relays)`（持 a.mu）
    - [ ] 长度为 0（自己已 clearRelay）且 `recordUC.IsActive()` 为 true 时，调用 `recordUC.Stop()`
    - [ ] 停止前通过 recorder 的 `SetLastError` 机制填充"因设备 {id} 断连自动停止录制"（需在 recorder 暴露 SetLastError 方法，或在 Stop 前通过 errMu 设置）
    - [ ] 多设备场景（len > 0）不停止录制，转而 emit `daq:recording-warning` 事件
    - [ ] 竞态保护：用户主动 Stop 与自动 Stop 竞争时，CAS 失败方静默返回
  - **Verification**:
    - [ ] `go test ./backend/ -run TestRelayStreamDisconnect -v`（新增）
    - [ ] `go build ./...`
    - [ ] 手动验证：单设备录制中拔线 → CSV 文件关闭、`session.Status=Idle`、`LastError` 含断连原因
  - **Dependencies**: Task 2（handleConnectionLost 触发 relay 退出）
  - **Files likely touched**:
    - `backend/app.go`（修改 relayStream + 新增 emitRecordingWarning helper）
    - `adapters/recording/csv_recorder.go`（新增 SetLastError 方法，供自动停止前填充错误信息）
    - `backend/app_test.go`（新增测试文件）
  - **Estimated scope**: M（3 文件）

### Checkpoint: 后端录制联动完成
- [ ] `go test ./...` 全绿
- [ ] `go build ./...` + `go vet ./...` 全绿
- [ ] 单设备录制断连自动停止 → CSV 关闭、状态正确
- [ ] 多设备录制断连 → 其他设备继续录制
- [ ] Review with human before proceeding to Phase 3

### Phase 3: 前端告警感知

- [ ] **Task 4: 多设备录制警告事件前端监听 + UI 提示**
  - **Description**：前端监听 `daq:recording-warning` 事件，在状态栏显示黄色警告提示，不弹 toast。同时区分"用户主动停止"和"因断连自动停止"的录制状态显示。
  - **Acceptance criteria**:
    - [ ] `recordingBridge.ts` 新增 `onRecordingWarning` / `offRecordingWarning` 监听函数
    - [ ] `recordingStore.ts` 新增 `warningMessage` ref，监听事件并填充
    - [ ] `MainBottomBar.vue` 在 `warningMessage` 非空时显示黄色提示条（带关闭按钮）
    - [ ] `lastError` 非空时显示红色错误条（区分于警告）
    - [ ] 关闭按钮点击后清空 `warningMessage`
    - [ ] 类型检查 + 构建通过
  - **Verification**:
    - [ ] `cd frontend; npm run typecheck`
    - [ ] `npm run build`
    - [ ] 手动验证：多设备录制中拔一台 → 状态栏出现黄色警告，其他设备继续录制
    - [ ] 手动验证：单设备录制中拔线 → 状态栏显示红色错误条（`LastError` 内容）
  - **Dependencies**: Task 3（后端 emit 事件）
  - **Files likely touched**:
    - `frontend/src/bridge/recordingBridge.ts`（新增 onRecordingWarning）
    - `frontend/src/stores/recordingStore.ts`（新增 warningMessage + 监听）
    - `frontend/src/components/layout/MainBottomBar.vue`（UI 显示警告/错误条）
  - **Estimated scope**: M（3 文件）

### Checkpoint: 前端告警感知完成
- [ ] `npm run typecheck` + `npm run build` 全绿
- [ ] 端到端验证：单设备/多设备断连场景均符合预期
- [ ] Review with human before proceeding to Phase 4

### Phase 4: 集成验证

- [ ] **Task 5: 端到端集成验证 + 回归测试**
  - **Description**：完整跑一遍 spec 的 8 条 Success Criteria，确认无回归。
  - **Acceptance criteria**:
    - [ ] Spec Success Criteria 1-8 全部通过
    - [ ] 现有主动停止路径（Disconnect / StopAcquisition）行为不变
    - [ ] 模拟模式（simulated_adapter）功能不受影响
    - [ ] Wails binding 同步检查（若 backend 方法签名未改，则无需重新生成）
  - **Verification**:
    - [ ] `go build ./...` + `go vet ./...` + `go test ./...` 全绿
    - [ ] `npm run typecheck` + `npm run build` 全绿
    - [ ] `powershell -File .\scripts\check-wails-bindings.ps1 -Projects wispa`
    - [ ] 模拟模式手动验证：Connect → StartAcquisition → StartRecording → 模拟断连 → 验证状态
  - **Dependencies**: Task 1, 2, 3, 4
  - **Files likely touched**: None（仅验证）
  - **Estimated scope**: XS

## Risks and Mitigations

| Risk | Impact | Mitigation |
|---|---|---|
| TCP keepalive 在 Windows 上行为与 Linux 不一致 | Med | 仅用 `SetKeepAlivePeriod` 标准库 API，Windows 上等同设置整个 keepalive 间隔。Task 1 单测用 mock conn 验证 API 调用，实机验证留到 Task 5 |
| `handleConnectionLost` 删除 driver 后 readLoop 仍在运行 | Low | readLoop 是 handleConnectionLost 的调用方，删除 driver 后 readLoop 立即 return，不存在访问已删除 driver 的窗口 |
| 单设备录制自动停止与用户主动 Stop 竞争 | Med | 复用 recorder 的 `started.CompareAndSwap` 保护，CAS 失败方静默返回。Task 3 单测覆盖此竞争场景 |
| `relayStream` 退出时 `len(a.relays)` 判定不准（clearRelay 时机） | Med | `clearRelay` 在 relay goroutine 内 defer 调用，relayStream 退出时已 delete。判定时当前 relay 已从 map 移除，剩余 relays 即"其他活跃设备"。Task 3 单测验证多设备场景 |
| 前端 warningMessage 内存泄漏（未清空） | Low | 关闭按钮点击清空 + 下次 StartRecording 时重置。Task 4 验证步骤覆盖 |
| Wails binding 漂移 | Low | 本 spec 不改 backend 方法签名，仅改内部逻辑 + 新增 Event.Emit 事件名。前端通过 `Events.On` 监听，无需 binding 文件。Task 5 用 check-wails-bindings.ps1 兜底确认 |

## Open Questions

1. **Task 3 的 SetLastError 机制**：CSVRecorder 当前 `lastError` 仅在 writerLoop 内 I/O 错误时设置。自动停止前需要从外部设置 `LastError`。
   - **方案 A**：在 `CSVRecorder` 暴露 `SetLastError(msg string)` 方法（持 errMu 写入）
   - **方案 B**：在 `RecordingUsecase` 层包装，Stop 后通过 `app.go` 直接 emit 携带 LastError 的 status 事件
   - **默认**：方案 A（recorder 内部状态由 recorder 自己管理，更内聚）

2. **Task 4 的警告条 UI 位置**：`MainBottomBar.vue` 是否有合适的展示位？
   - 需在 Task 4 实施时读 `MainBottomBar.vue` 确认布局
   - 若空间不足，备选方案是 `MainTopBar.vue` 顶部条

3. **多设备场景下"全部设备都断连"的边界**：若多设备场景下设备逐个断连直到全部断连，是否应在最后一个 relay 退出时自动停止录制？
   - **默认**：是，`len(a.relays) == 0` 即触发自动停止（与单设备场景一致）。Task 3 单测覆盖此边界。

## Parallelization Opportunities

- **Task 1 + Task 2**：理论可并行（改不同函数），但同改 `p1604_adapter.go` 建议顺序避免合并冲突
- **Task 3 + Task 4**：必须顺序，Task 4 依赖 Task 3 的 emit 事件
- **Task 5**：必须在所有 Task 完成后

## Verification Checkpoints Summary

| Checkpoint | 位置 | 验证内容 |
|---|---|---|
| CP1 | Task 1-2 后 | 硬件层单测全绿 + go build/vet 通过 |
| CP2 | Task 3 后 | 后端录制联动单测 + 手动模拟模式验证 |
| CP3 | Task 4 后 | 前端 typecheck/build + 端到端验证 |
| CP4 | Task 5 后 | Spec 8 条 Success Criteria 全部通过 |
