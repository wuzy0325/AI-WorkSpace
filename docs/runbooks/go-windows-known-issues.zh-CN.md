# Go Windows 已知问题参考

> L3 专题规则，按需加载。涉及 Windows 网络 I/O / runtime crash / 文件系统 / 进程管理时查阅。
> 本文不重复硬约束——硬约束在 AGENTS.md §Windows Network I/O Constraint。

---

## 1. SetReadDeadline 到期不唤醒

### 根因

Windows `CancelIoEx` 在特定条件（单核、高负载、动态优先级提升）下**不发送 IO completion 通知**，goroutine 永久阻塞在 `runtime_pollWaitCanceled`。这是 Windows 内核 bug，非用户代码问题。

### 上游 issue

| Issue | 描述 |
|---|---|
| [#5971](https://github.com/golang/go/issues/5971) | kernel bug：CancelIoEx 调用后完成通知丢失 |
| [#21133](https://github.com/golang/go/issues/21133) | pending read 期间设置 deadline 引发死锁 |
| [#34385](https://github.com/golang/go/issues/34385) | 反复 SetReadDeadline 导致 hang |
| [#70395](https://github.com/golang/go/issues/70395) | 高负载下 deadline read 返回偏移数据（8-12 字节错位） |
| [#4195](https://github.com/golang/go/issues/4195) | CancelIo 不能单独取消 read/write，Vista+ 改用 CancelIoEx 后部分修复 |
| [#24727](https://github.com/golang/go/issues/24727) | Windows 上 Write timeout 总是返回 n=0（即使已发送字节），与 Linux 不一致 |

### 官方文档关键提示

- **"The deadline applies to all future and pending I/O"** — 新 deadline 会影响已在阻塞的 Read。#21133 中确认这是预期行为。
- **"Any blocked Read or Write operations will be unblocked and return errors"** — `conn.Close()` 是 Conn 接口规范保证的唯一可靠取消手段。
- **SetWriteDeadline `n > 0` on timeout** — Write timeout 后可能返回已写入的部分字节。但 Windows 上实际总是返回 n=0（[#24727](https://github.com/golang/go/issues/24727)）。
- **tls.Conn Write timeout 后状态损坏** — "After a Write has timed out, the TLS state is corrupt and all future writes will return the same error"。
- **syscall.RawConn 不传播 deadline 错误** ([#53445](https://github.com/golang/go/issues/53445)) — `syscall.Read/Write` 返回 `0, nil`，错误来自 `RawConn.Read/Write`。写循环时注意不要死循环。

### 项目内缓解方案

- `WatchdogClose` 模式：`shared/device-sdk/go/protocol/conn_helpers.go`
- `deadlineIgnoringConn` 测试替身：`conn_helpers_test.go:481`
- 完整决策：`docs/decisions/ADR-009-windows-network-deadline-fallback.md`

---

## 2. runtime 级问题

### Go 1.26 Windows 栈破坏 crash（Critical，已修复）

| 字段 | 值 |
|---|---|
| Issue | [#77975](https://github.com/golang/go/issues/77975) / [#78041](https://github.com/golang/go/issues/78041) |
| 影响 | Go 1.25.0 - 1.26.1 |
| 根因 | async preemption + stack growth 交互导致 return address 高 32 位被覆盖 |
| 症状 | `access violation 0xc0000005`，PC 高 32 位变成 `0x00000010` |
| 修复 | 升级到 ≥ 1.26.2 |

### 偶发启动 panic — morestack / mcall on g0（未彻底解决）

| 字段 | 值 |
|---|---|
| Issue | [#67108](https://github.com/golang/go/issues/67108) / [#79249](https://github.com/golang/go/issues/79249) |
| 影响 | 特定 CPU（AMD Threadripper）的 Windows 11 偶发 |
| 症状 | `fatal: morestack on g0` 或 `mcall called on m->g0 stack`，init 阶段崩溃 |
| 特点 | 重试即正常，隔几天崩一次 |

### cgo + Windows 系统栈/堆破坏（待调查）

| 字段 | 值 |
|---|---|
| Issue | [#59724](https://github.com/golang/go/issues/59724) |
| 影响 | 仅 Windows |
| 触发模式 | cgo 创建资源 → 起 goroutine → 切 OS 线程 → cgo 销毁 |
| 症状 | `access violation`、堆损坏 `0xc0000374` |
| 备注 | Vulkan / COM / WinRT 回调可能触发 |

---

## 3. 文件系统问题

### os.Rename 非原子、不覆盖

| 字段 | 值 |
|---|---|
| Issue | [#10773](https://github.com/golang/go/issues/10773) |
| 行为 | 使用 `MoveFileW` 而非 `MoveFileExW`，目标存在时报错，跨盘时报错 |
| 对比 Unix | Unix rename 原子覆盖，跨设备返回 EXDEV |

### 长路径（>260 字符）全面问题

| 字段 | 值 |
|---|---|
| Issues | [#3358](https://github.com/golang/go/issues/3358) / [#41734](https://github.com/golang/go/issues/41734) / [#66560](https://github.com/golang/go/issues/66560) / [#78601](https://github.com/golang/go/issues/78601) |
| 影响 API | os.Rename / os.Remove / os/exec.Command / filepath.Walk / os.MkdirAll |
| 当前 hack | Go runtime 通过设置未文档化 PEB bit（`IsLongPathAwareProcess`）绕过 |
| 风险 | Microsoft Go 团队明确提案删除此 hack（#66560），未来行为可能变化 |
| 额外问题 | `CreateProcess` 不受 PEB bit 影响——即使 Go 自身 I/O 走长路径正常，`os/exec` 执行另一个程序仍会失败（#78601） |
| 规避 | 使用 `\\?\` 前缀，或升级到 Win10 1607+ + 注册表 `LongPathsEnabled=1` + 应用清单 |

### 只读文件 rename 失败

| 字段 | 值 |
|---|---|
| Issue | [#35043](https://github.com/golang/go/issues/35043) |
| 根因 | Windows `FILE_ATTRIBUTES_READONLY` 阻止 rename，Unix 无此限制 |

---

## 4. 网络/TCP 问题（非 deadline 相关）

### TCP 数据偶发重复 ~16KB（未修复）

| 字段 | 值 |
|---|---|
| Issue | [#58764](https://github.com/golang/go/issues/58764) |
| 影响 | 仅 Windows |
| 触发 | 特定读写模式（TLS + 非阻塞 wrapper），数据量 > 2.7MB 后 |
| 症状 | 约 16406 字节数据在 TCP 流中重复，导致 `tls: bad record MAC` |
| 状态 | 2023 年提交，至今 open |

---

## 5. 进程管理问题

### os/exec: 修改 Cmd.Path 被忽略（已修复）

| 字段 | 值 |
|---|---|
| Issue | [#68314](https://github.com/golang/go/issues/68314) |
| 影响 | Go 1.22.5 - 1.22.x，release-blocker |
| 根因 | `cachedLookExtensions` 缓存了原始路径 |
| 现象 | `exec.Command(absPath)` 后再改 `cmd.Path`，实际执行原始程序 |
| 修复 | 升级到 ≥ 1.22.6 或 ≥ 1.23 |

---

## 6. 开发与测试准则

### 测试连接替身

项目已有 `deadlineIgnoringConn`（`conn_helpers_test.go:481`），模拟 Windows 上 `SetReadDeadline` 失效的场景：

- `SetReadDeadline` 被覆盖为 no-op
- Read 只在 `conn.Close()` 后返回
- 配合 watchdog 验证双保险机制

### 避免依赖的陷阱

| 问题 | 详情 |
|---|---|
| `net.Pipe` 同步无缓冲 | [#24205](https://github.com/golang/go/issues/24205) 提议改异步但未实现。TLS 测试容易死锁。如果需要缓冲，用 `bufio` 包装或改用真实 TCP |
| `syscall.RawConn` 不传播 deadline 错误 | [#53445](https://github.com/golang/go/issues/53445)。裸 syscall I/O 的 deadline 错误来自 RawConn，不是 syscall.Read |
| HTTP/2 连接 Write timeout 中毒 | [#39337](https://github.com/golang/go/issues/39337)。一次 Write timeout 会导致整个 HTTP/2 连接的所有后续请求立即失败 |
| tls.Conn Write timeout 不可恢复 | Write timeout 后 TLS 状态损坏，必须丢弃整条连接 |
