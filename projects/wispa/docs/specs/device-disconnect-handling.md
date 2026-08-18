# Spec: wispa 设备断连处理改进

> **状态**：Phase 1 - 待用户 review
> **创建时间**：2026-07-03
> **范围**：仅 wispa 项目（windlabx4 同类问题另立 spec）

## Objective

改进 wispa 在采集过程中遇到网络中断（拔网线）时的处理逻辑，解决三个核心缺陷：

1. **检测延迟可达数小时**：当前 readLoop 仅依赖 `SetReadDeadline(200ms)` 超时，但超时后 `continue`，物理拔线无 RST/FIN 包，需等 TCP keepalive 默认 2 小时才报错
2. **资源不清理**：`handleConnectionLost` 未删除 `shard.drivers[id]`，未 `conn.Close()`，导致 fd 泄漏 + 重连被 `device already connected` 拒绝
3. **录制语义脱节**：单设备录制场景下断连后 `relayStream` 退出但 `CSVRecorder` 仍 active，文件句柄长期占用

**目标用户**：wispa 桌面应用运维人员（实验室环境，单设备或多设备并发采集）

**成功定义**：
- 拔线后 ≤ 30 秒内设备状态变为 Error
- 拔线后 driver 从 `shard.drivers` 清理，`net.Conn` 关闭，无 fd 泄漏
- 单设备录制场景：拔线后录制自动停止，CSV 文件正确关闭
- 多设备录制场景：拔线后其他设备继续录制，仅 emit 警告事件

## Tech Stack

- Go 1.25 标准库（`net` / `log/slog` / `sync` / `atomic` / `testing`）
- `shared.local/device-sdk/go/protocol`（现有 sharedproto helper，本 spec 不修改）
- Wails v3 alpha.95（前端事件 emit）
- 测试：Go 标准 `testing` 包 + `simulated_adapter`，无外部测试框架

## Commands

```powershell
# 构建（wispa 隔离工作空间，必须 GOWORK=off）
cd projects\wispa\apps\desktop-wails
$env:GOWORK="off"
go build ./...

# 单元测试
go test ./...

# Vet
go vet ./...

# 模拟模式运行（手动验证）
$env:WISPA_MODE="simulated"
go run github.com/wailsapp/wails/v3/cmd/wails3 dev

# 前端检查（若涉及前端事件监听改动）
cd frontend
npm run typecheck
npm run build
```

## Project Structure

```
projects/wispa/apps/desktop-wails/
├── adapters/hardware/
│   ├── p1604_adapter.go          # ← 改：TCP keepalive + handleConnectionLost 清理 driver/conn
│   ├── p1604_adapter_test.go     # ← 新增：handleConnectionLost + enableTCPKeepalive 单测
│   ├── simulated_adapter.go      # 不变（无 TCP，不受影响）
│   └── ...
├── backend/
│   └── app.go                    # ← 改：relayStream 退出时按录制设备数决定是否停止录制
├── usecase/
│   └── recording_usecase.go      # 不变（recorder 本身不改）
├── core/
│   └── types.go                  # ← 改：新增 RecordingWarning 事件载荷类型
└── frontend/src/
    └── stores/recordingStore.ts  # ← 改：监听 daq:recording-warning 事件
```

## Code Style

参考现有 [p1604_adapter.go](file:///c:/Users/wuzhy/Documents/D/SVN/SoftWare/trunk/AI-Workspace/projects/wispa/apps/desktop-wails/adapters/hardware/p1604_adapter.go) 风格。

**TCP keepalive 启用示例**：

```go
const (
    // p1604KeepAlivePeriod TCP keepalive 探测间隔。
    // 解决拔网线后物理层无 RST/FIN，readLoop 仅靠 SetReadDeadline 无法判定真断连的问题。
    // Linux 默认 7200s/9 探测 = 2 小时，完全不可接受；10s × 3 探测 = 30s 内可判定断连。
    p1604KeepAlivePeriod = 10 * time.Second
    // p1604KeepAliveIdle 连接空闲多久后开始发送 keepalive 探测
    p1604KeepAliveIdle  = 10 * time.Second
    // p1604KeepAliveCount 连续多少次探测失败后判定连接已断
    p1604KeepAliveCount = 3
)

// enableTCPKeepalive 在 TCP 拨号成功后启用 keepalive。
// 非 TCP 连接（如模拟器 pipe）返回 nil 不报错，保持兼容。
func enableTCPKeepalive(conn net.Conn) error {
    tcpConn, ok := conn.(*net.TCPConn)
    if !ok {
        return nil
    }
    if err := tcpConn.SetKeepAlive(true); err != nil {
        return fmt.Errorf("enable keepalive: %w", err)
    }
    if err := tcpConn.SetKeepAlivePeriod(p1604KeepAlivePeriod); err != nil {
        return fmt.Errorf("set keepalive period: %w", err)
    }
    return nil
}
```

**handleConnectionLost 清理示例**：

```go
// handleConnectionLost 处理连接意外断开：清理共享状态并通知前端。
// 与原实现差异：删除 driver + close conn，避免 fd 泄漏和重连被拒。
func (a *P1604Adapter) handleConnectionLost(id string, driver *p1604Driver, cause error) {
    shard := a.shard(id)
    shard.mu.Lock()
    st, exists := shard.status[id]
    if !exists || st.Status == core.StatusDisconnected {
        shard.mu.Unlock()
        return
    }
    // driver 一致性校验：若 Disconnect 已删除并启动新 driver，放弃清理
    if cur, ok := shard.drivers[id]; !ok || cur != driver {
        shard.mu.Unlock()
        return
    }
    delete(shard.sinks, id)
    if done, ok := shard.stopChs[id]; ok {
        close(done)
        delete(shard.stopChs, id)
    }
    if ch, ok := shard.channels[id]; ok {
        close(ch)
        delete(shard.channels, id)
    }
    // 新增：删除 driver，让设备回到"未连接"状态，重连时不再被 already connected 拒绝
    delete(shard.drivers, id)
    driver.acquiring = false
    st.SetStatus(core.StatusError)
    st.Error = fmt.Sprintf("连接断开: %v", cause)
    st.AcquiringAt = 0
    shard.mu.Unlock()

    // 锁外关闭 conn：避免持锁 I/O，且 readLoop 已退出无竞争
    if driver.conn != nil {
        _ = driver.conn.Close()
    }

    a.emitState(id)
    // ... 日志保持不变
}
```

**关键约定**：
- 错误处理用 `fmt.Errorf("xxx: %w", err)` 包装
- 中文注释说明"为什么"而非"做了什么"
- 常量集中在文件顶部，带说明
- 并发修改持锁，I/O 锁外执行
- 状态变更通过 `emitState` 通知前端，不直接调 Wails API

## Testing Strategy

**单元测试位置**：`adapters/hardware/p1604_adapter_test.go`（项目当前无测试文件，本次新增首个）

**测试用例**：

| 测试函数 | 验证点 | 验收条件 |
|---|---|---|
| `TestEnableTCPKeepalive_TCPConn` | TCP keepalive 启用 | 对 `net.TCPConn` 调用后无错误 |
| `TestEnableTCPKeepalive_NonTCPConn` | 非 TCP 连接兼容 | 对 mock conn 调用返回 nil 不报错 |
| `TestHandleConnectionLost_CleansDriverAndConn` | driver + conn 清理 | 执行后 `shard.drivers[id]` 不存在，conn.Close 被调用 |
| `TestHandleConnectionLost_EmitsErrorState` | 状态推送 | `emitState` 收到 `StatusError` + 错误信息 |
| `TestHandleConnectionLost_GuardsAgainstNewDriver` | driver 一致性校验 | 旧 driver 触发时不影响新 driver |
| `TestHandleConnectionLost_SkipsAlreadyDisconnected` | 幂等性 | 设备已 Disconnected 时不重复清理 |

**测试策略**：
- 用 mock `net.Conn`（实现 `Read/Write/Close/SetReadDeadline` 等）注入错误，模拟拔线
- 不依赖真实硬件或真实 TCP server
- `simulated_adapter` 不受影响（无 TCP），不需要新增测试

**覆盖率目标**：新增代码 ≥ 80%

## Boundaries

**Always**：
- 改完跑 `go build ./...` + `go vet ./...` + `go test ./...` 全绿
- 错误处理用 `fmt.Errorf + %w` 包装
- 中文注释说明"为什么"
- 并发修改持锁，I/O 锁外执行
- 状态变更通过 `emitState`/`emitLog` 通知前端

**Ask first**：
- 修改 `sharedproto` 共享代码（影响 windlabx4）
- 改变 CSV 文件格式
- 新增外部依赖
- 修改 Wails binding 方法签名（需重新生成 binding）

**Never**：
- 在 `handleConnectionLost` 持有 `shard.mu` 时执行 `conn.Close()` 等 I/O
- 关闭 driver 时不通过 `readLoopDone` join readLoop
- 引入自动重连逻辑（P2，本 spec 不覆盖）
- 修改 `simulated_adapter`（无 TCP，不受影响）

## Success Criteria

1. **检测延迟**：拔网线后 ≤ 30 秒内设备状态变为 Error（TCP keepalive 10s idle + 3 × 10s 探测）
2. **资源清理**：`handleConnectionLost` 执行后 `shard.drivers[id]` 不存在，`driver.conn` 已 Close
3. **重连无阻塞**：拔线后用户直接 `Connect` 同 ID 设备成功，无需先 `Disconnect`
4. **单设备录制**：仅一台设备录制时断连，CSV 文件在 5 秒内关闭，`session.Status` 变为 `Idle`，`LastError` 填充断连原因
5. **多设备录制**：多台设备录制时一台断连，其他设备继续写入 CSV，前端收到 `daq:recording-warning` 事件
6. **回归无影响**：现有主动停止路径（`Disconnect` / `StopAcquisition`）行为不变
7. **测试通过**：新增单元测试全绿，新增代码覆盖率 ≥ 80%
8. **构建通过**：`go build` + `go vet` + `go test` + `npm typecheck` + `npm build` 全绿

## Open Questions

1. **多设备警告事件 UI 表现**：是否需要前端弹 toast？还是仅写入日志 + 状态栏提示？
   - **默认**：仅日志 + 状态栏黄色提示，不弹 toast（避免干扰用户）

2. **单设备录制自动停止的事件**：是否需要让前端区分"用户主动停止"和"因断连自动停止"？
   - **默认**：是，复用现有 `daq:recording-status` 事件，`session.LastError` 填充"因设备 P1604-001 断连自动停止录制"

3. **TCP keepalive 参数可配置性**：是否需要做成 profile 配置项暴露给用户？
   - **默认**：硬编码常量，不暴露 UI（参数经行业验证，10s/3 次是合理默认）

4. **单设备 vs 多设备判定**：如何判定"单设备录制场景"？
   - **默认**：录制开始时记录 `acquiringDeviceIDs []string`，长度为 1 视为单设备；断连任一设备即触发自动停止。多设备场景仅 emit 警告。

## 风险与缓解

| 风险 | 缓解策略 |
|---|---|
| TCP keepalive 在 Windows 行为差异 | Windows 支持 `SetKeepAlivePeriod` 但 idle/count 参数需通过 `SetSocketOption` 单独设置；本 spec 仅用 `SetKeepAlivePeriod`，Windows 上等同于设置整个 keepalive 间隔，行为可接受 |
| `handleConnectionLost` 删除 driver 后 readLoop 仍在运行 | readLoop 是调用方，删除 driver 不影响 readLoop 退出（它已 return） |
| 多设备场景下 recorder 仍可能因其他设备 I/O 异常停止 | 不在本 spec 范围，保持现有 recorder auto-stop 逻辑 |
| 单设备录制自动停止与用户主动停止竞争 | 复用 recorder 的 `started.CompareAndSwap` 保护，竞争时 CAS 失败方静默返回 |
