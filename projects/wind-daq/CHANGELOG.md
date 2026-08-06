# Changelog

## [0.13.0] - 2026-08-05

### Fixed

- **七孔插值加载与插值链路修复**（对 `shared/algorithms/go/sevenhole/interpolation`，与 probe-interpolator 0.3.0 同源）：
  - 七孔校准 CSV 系数全精度重算：系数（ka/kb/cpt/cps）由原始压力列（col3=P0、col4=Ps、col5..11=P1..P7）在 float64 全精度下按公式重算，替代历史 3 位小数 K 列（col12..15，误差最大约 5e-4）。
  - 网格节点精确往返修复：内区 21 个边界节点此前被误路由到大角度区，现正确路由到内区；校准行按自身网格节点精确往返，自提取 PRB 反推不再漏判网格边界点。
  - 退化边抖动消除：全精度重算消除了 3dp 截断导致的精确 ka/kb 相等退化边，加载七孔 CSV 不再产生退化边抖动警告（`seven_hole_csv_test.go` 断言同步更新）。
- 新增 NaN/Inf 压力值拒绝（CSV 加载时），避免脏数据静默进入网格。

### Compatibility

- 配置文件格式：兼容。
- 数据文件格式：CSV 校准文件兼容（列位置契约不变）；历史 3 位小数 K 列（col12..15）不再参与网格构建，仅用于表头诊断。
- 历史 PRB 文件：旧 3dp PRB 与新全精度网格存在约 5e-4 差异，建议重新生成后加载。
- API 契约：兼容。
- 设备协议行为：不变。

### Verification

- `go test ./...`（shared/algorithms/go/sevenhole/interpolation）: passed
- `go test ./internal/adapters/interpolation/ -run SevenHole`（wind-daq services/api-go）: passed
- `go vet ./...` + `go test ./internal/... ./api/...`（wind-daq services/api-go）: passed
- `npm run typecheck` / `npm run build`: passed
- `task release`: passed
- `makensis`: passed
- `task archive-release`: passed

### Known Issues

- 安装包未进行 Authenticode 数字签名。
- 内嵌 exe 的 ProductVersion 资源为空（wails3 v3.0.0-alpha2.106 既有行为），installer 的 ProductVersion 为 `0.13.0`。

## [0.12.4] - 2026-08-05

### Added

- **三孔探针插值算法输出气流速度 Velocity**（对应 `shared/algorithms/go/threehole/interpolation`，与 probe-interpolator-miniprogram 同源同步）：`InterpolationResult` 新增 `Velocity` 字段（m/s），由 `MachNumber` 与 `Tatm` 经 `V = Ma · sqrt(γ · R · T_K)` 推导（R=287 J/(kg·K)，γ 复用 `calcGamma` 温度修正）。
  - 业务背景：三孔探针风洞验证需要"速度误差范围"作为验收指标，原先算法只输出 Ma，调用方需各自离线换算 V，跨实现一致性无法保证。
  - 兜底语义：Velocity 严格跟随 MachNumber——Ma 有效或兜底（initMa/currentMa）时给出对应速度，Ma 为 0/NaN（输入非法、calcMach 失败）时返回 0。
  - 5 个返回点全部填充 Velocity，与现有 MachNumber 行为对齐。

### Fixed

- 本版本无 Wind-DAQ 自有 bug 修复，仅因共享算法包新增 Velocity 输出重新打包。

### Compatibility

- 配置文件格式：兼容。
- 数据文件格式：兼容（CSV 暂未新增 Velocity 列，未来 wind-daq 校准 CSV 接入时再扩展）。
- API 契约：兼容（`InterpolationResult` 新增字段为可选，旧调用方无需改动）。
- 设备协议行为：不变。

### Verification

- `go vet ./...`（shared/algorithms/go/threehole）: passed
- `go test ./...`（shared/algorithms/go/threehole，含新增 4 个测试）: passed
- `node verify_three.js`（50 用例 Go↔JS 跨实现数值一致，容差 1e-9）: passed
- `node verify_csv.js / verify_units.js / verify_share_card.js`（103 项断言）: passed
- `task release`: passed
- `makensis '-DARG_WAILS_AMD64_BINARY=..\..\bin\wind-daq.exe' project.nsi`（在 `build/windows/installer/` 目录下执行）: passed
- `task archive-release`: passed

### Known Issues

- 安装包未进行 Authenticode 数字签名，Windows 可能显示"未知发布者"提示。
- wind-daq 前端校准 CSV 暂未接入 Velocity 列输出，仍由前端独立计算；后续接入时统一来源。

## [0.12.3] - 2026-08-04

### Fixed

- **共享驱动 MCH 掩码覆盖修复**（对应 `shared/device-sdk` `daq_t1603.go`，daq-t1603 0.6.8 同源修复）：连接同步阶段 `@fd MCH` 读取的设备端持久化历史通道掩码（如出厂遗留 `0000`）不再写回 `cfg.ChannelMask`。此前若设备端掩码非 `FFFF`，T1603 设备启动采集会发出 `@f0 0000 2`（零通道），导致无数据帧且停止采集报 `invalid Stop response boundary`。现应用配置/profile 才是权威，mask 为空时仍回退 `FFFF`。
- 本版本无 Wind-DAQ 自有代码变更，仅因共享 device-sdk 修复重新打包。

### Compatibility

- 配置文件格式：兼容。
- 数据文件格式：兼容。
- 设备协议行为：`@fd MCH` 仍会被读取以维持协议响应边界，仅不再回写应用配置；协议命令链无变化。
- API 契约：兼容。

### Verification

- `go build ./services/api-go/...`: passed
- `go test ./services/api-go/internal/...`: passed
- `go vet ./services/api-go/...`: passed
- `npm run typecheck`: passed
- `npm run build`: passed
- `task release`: passed
- `makensis '-DARG_WAILS_AMD64_BINARY=..\..\bin\wind-daq.exe' project.nsi`（在 `build/windows/installer/` 目录下执行）: passed
- `task archive-release`: passed

### Known Issues

- 安装包未进行 Authenticode 数字签名，Windows 可能显示"未知发布者"提示。

## [0.12.2] - 2026-08-03

### Changed

- **Breaking**：下线遍历测试"停止后恢复"能力。停止遍历测试现在表示操作员作出最终决定，临时 checkpoint 被清理，不再作为未完成任务提供恢复入口。
  - 后端删除 HTTP 端点：`loadCheckpoint`、`resumeFromCheckpoint`、`clearCheckpoint`（位于 `services/api-go/api/server.go` 与 `server_dual_traversal.go`）。
  - 后端删除 `TraversalManager.LoadCheckpoint` / `ResumeFromCheckpoint` / `resumeInternal`、`ManagerRegistry.LoadCheckpoint` / `ResumeFromCheckpoint` / `ClearCheckpoint` / `authoritativeCheckpoint` / `admitResumeLockedUnderGate` / `loadDualCheckpoint` / `discardOrphanedRecovery` 等恢复相关方法。
  - 前端删除单探针/双探针界面的"未完成任务检测"、"继续测试"、"放弃任务"入口与 `TraversalCheckpointBanner.vue` 组件。
  - 前端 `traversalApi.ts` / `traversalStore.ts` / `dualTraversalStore.ts` / `dualTraversalRuntime.ts` / `i18nStore.ts` / `traversal.ts` 类型同步移除 checkpoint 相关字段与方法。
  - 受影响测试用例同步精简或删除：`DualTraversalCheckpointRecovery.contract.test.ts`、`traversal_v2_integration_test.go` 中 checkpoint 用例、`traversal_managed_checkpoint_test.go` / `traversal_registry_recovery_test.go` 等保留少量非恢复路径用例。

### Internal

- `traversal_checkpoint.go` 从 ~588 行精简至 ~300 行，仅保留 checkpoint 文件写入/清理副作用，不再对外暴露恢复 API。
- `traversal_registry_recovery.go` 从 ~700 行精简至 ~350 行，删除孤儿恢复、权威 checkpoint 解析、双探针 checkpoint 加载等逻辑。
- 共减少 ~2770 行、新增 ~183 行，整体代码维护成本显著降低。

### Compatibility

- 配置文件格式：兼容。
- 数据文件格式：兼容（已落盘的测试结果 CSV 不受影响）。
- API 契约：**不兼容**。依赖上述 checkpoint 端点的调用方必须移除对应调用。
- 升级路径：升级后无法从旧版本遗留的 traversal checkpoint 继续测试；如需保留测试结果，请在升级前完成当前遍历任务。

### Verification

- `go build ./services/api-go/...`: passed
- `go test ./services/api-go/internal/...`: passed
- `go vet ./services/api-go/...`: passed
- `npm run typecheck`: passed
- `npm run build`: passed
- `npm run test`: passed
- `task release`: passed
- `makensis build/windows/installer/project.nsi`: passed
- `task archive-release`: passed

### Known Issues

- 安装包未进行 Authenticode 数字签名，Windows 可能显示未知发布者提示。
- 旧版本（≤0.12.1）遗留的 traversal checkpoint 文件在升级后会保留在磁盘但不会被读取，可手动清理 `%APPDATA%\wind-daq\` 下的相关 json 文件。

## [0.12.1] - 2026-08-03

### Fixed

- 设备网络操作防本机卡死整改（LSP 环境加固，对应 `b8475f4`）：LSP/安全软件（Astrill ASProxy64.dll、深信服驱动等）hook winsock 导致 SetReadDeadline 失效、conn.Close() 在挂起 I/O 时永久阻塞的故障模式下，退出/扫描/设备操作不再卡死主线程：
  - `WatchdogClose` 语义重构：timer 回调 go AbortConnection（CloseWrite FIN + Close），wdStop 不再等待 Close 完成。
  - `wtnmc4a_motion` FFI 调用全部改走 ffiGate（单 worker + 有界投递等待 + 超时失败），杜绝 DLL syscall 阻塞冻结控制器锁。
  - 驱动层（p1064pre/p1604/dsa3217/wtn_pxi）同步 Close 改后台 detach；扫描链 closesocket 移出 handleMu、scanInFlight 增加 8s deadline + 超时重置。
  - `device_manager` 全局锁内网络 I/O 移出（SetUnit/ApplyConfig 等改锁外执行）。
  - `app.go`：callMgr 与 DeviceScanDevices 增加 10s 有界等待（超时返回"操作超时"提示，前端可重试）；startLocalAPIServer 探测 127.0.0.1:8900 端口占用并弹窗提示；API server Shutdown 改后台 goroutine + 主线程 300ms 有界等待；退出确认后 30s exitWatchdog 强制结束进程；runShutdownStep 超 5s 自动 dump goroutine 栈到 `%APPDATA%\wind-daq\goroutine-dump-*.txt`。
- 修复 `UpsertProfile` 锁外 SetUnit 后旧索引 TOCTOU（重取锁重新定位），补回归测试。
- 修复前端 wails-adapter 静态导入 Events 导致退出确认事件不注册的问题。
- 七孔校准区域归属改用预设配置替代压力判定（`bafa1fd`）：低压力/零压场景下七孔区域不再误判。

### Changed

- 设备扫描、设备/运动管理操作超时后返回"操作超时（底层可能无响应），请重试"，前端不再永久转圈。

### Internal

- ADR-009 补充最终兜底失效章节；归档整改计划文档 `docs/plans/2026-08-01-device-network-hang-lsp-remediation.md`。
- `shared/device-sdk/go/daq/hardware/daq_t1603.go` 增加 deadline 探测 + Stop 静默窗口 350ms goroutine 兜底、stopAbandoned 优雅停止契约。

### Verification

- `go build ./services/api-go/...`: passed
- `go test ./services/api-go/internal/...`: passed（含 UpsertProfile TOCTOU 回归测试）
- `go vet ./services/api-go/...`: passed
- `npm run typecheck`: passed
- `npm run build`: passed
- `task release`: passed
- `makensis build/windows/installer/project.nsi`: passed
- `task archive-release`: passed

### Known Issues

- 安装包未进行 Authenticode 数字签名，Windows 可能显示未知发布者提示。
- LSP 拦截场景下底层操作超时后为受控降级（前端提示重试），卡死 goroutine 随进程退出释放；不影响正常环境。

## [0.12.0] - 2026-07-31

### Fixed

- 修复问题电脑上"退出确认后程序无响应"的卡死问题：`stopAllMotion` 用 `context.Background()` 无超时，B140 通信异常时 `queryStatus` 串行多个 sendCommand 累积 ~70s 阻塞 GUI 主线程。改为 3s 有界超时，最坏 3s+5s=8s 内完成退出。
- 精确化 `stopAllMotion` 注释，区分未启动命令（立即返回 ctx.Err()）与执行中命令（等 watchdog Close conn，最长 5s）的 ctx 取消语义。

### Changed

- 重构 `ServiceShutdown`：抽出 `runShutdownStep` 辅助函数统一执行 + 打点，记录每步 step_ms 与 total_ms，问题电脑再卡时可直接从日志定位卡在哪个子步骤。失败路径也打点，避免日志时间线断裂。主体保持线性流程，达成 AGENTS.md ≤50 行/函数约束。

### Internal

- 新增 `TestStopAllMotion_BoundedByContextTimeout` / `TestStopAllMotion_StopCallRespectsContextTimeout` 防回归测试，改回 `context.Background()` 则 10s 超时失败。实际 3.00s / 3.01s 返回，精确命中 3s 上限。

### Verification

- `go build ./services/api-go/...`: passed
- `go test ./services/api-go/internal/usecase/`: passed (含 2 个新回归测试)
- `go vet ./services/api-go/internal/usecase/...`: passed
- `go build ./apps/desktop-wails/backend/...`: passed
- `go test ./apps/desktop-wails/backend/...`: passed
- `go vet ./apps/desktop-wails/backend/...`: passed
- `task release`: passed
- `makensis build/windows/installer/project.nsi`: passed
- `task archive-release`: passed

### Known Issues

- 安装包未进行 Authenticode 数字签名，Windows 可能显示未知发布者提示。
- 该修复针对 B140 运动控制器通信异常场景；若其他设备（T1603 / P1604）通信异常时退出仍可能等待单命令 watchdog（5s）超时，但不会累积到 ~70s。

## [0.11.4] - 2026-07-31

### Added

- 新增 `scripts/staticcheck-u1000-waivers.txt` 与 `validate-structure.ps1` 的 "2f. Go dead code check (U1000)" 章节：用 staticcheck 自动检测未使用的私有符号（func/type/field/var/const），避免人工 review 漏掉 dead code。豁免清单不含行号，避免代码编辑导致豁免失效；新增 dead code 不得加入豁免清单，必须直接删除源码。
- 新增 `docs/runbooks/go-windows-known-issues.zh-CN.md`（L3 专题规则）：系统化记录 Go 在 Windows 上的内核级已知问题（网络 I/O deadline 失效、runtime crash、文件系统、进程管理），并附上游行 issue 索引（golang/go #5971 / #21133 / #34385 等）。配合 ADR-009 落实"永远不要把 socket deadline 作为有界硬件 I/O 的唯一取消机制"硬约束。
- 新增 `docs/audits/2026-07-29-adr009-remaining-remediation.md`：ADR-009 剩余整改清单复核修订版，撤回旧版"全部 P0/P1/P2/SIM-1 已完成"结论，记录五批整改进度（R0-1 ~ R2-2）与独立审查 finding 1-9 状态。
- 新增 `docs/audits/2026-07-30-daq-t1603-acquisition-control-hardware-report.zh-CN.md`：DAQ-T-1603 采集控制实机验证报告（192.168.1.10:9000），验证 Start ACK / Stop ACK / 旧流排空 / 64 字节数据边界，为"物理停止后允许配置"提供依据。
- 新增 `docs/plans/2026-07-31-daq-p1604-stop-transaction-remediation.zh-CN.md`：DAQ-P-1604 Stop 事务整改方案（待实现），涉及独立应用 / Wind-DAQ / 共享协议三层。
- 新增 `docs/udp-discovery-windows-timeout-root-cause.md`：Windows UDP 设备扫描不返回问题根因与修复方案。
- 新增 `projects/wind-daq/docs/test-cases.html`（181KB）：Wind-DAQ 功能测试用例文档，按用户测试用例格式要求编写（测试前置 / 测试步骤 / 期待结果三段式），用例优先级色板（P0/P1/P2）与结果色（pass/fail/block/skip）解耦，技术术语站在测试人员和用户角度改写。
- 新增 `device-lab/skills/daq-t1603/SKILL.md` "3.2 实机响应证据（2026-07-29，192.168.1.10:9000）" 章节：所有命令裸发、无终止符，记录实机测得的响应字节证据表（@e3 / @fd MCH / @fd SPS / @fd BIN / @fe 等），区分"实机已验证"与"待复核"状态。
- `shared/device-sdk/go/protocol` 新增 `ErrDeviceRejected` sentinel：设备对 `@fe` / `@f3` 等设置命令返回 E 属于合法业务层拒绝，连接协议边界仍可信，不触发 ADR-009 毒化。与 `ErrWatchdogTriggered`（协议不可信）边界区分。
- `shared/device-sdk/go/protocol` 新增 `IsBinaryMode` 只读访问器与 `ExpectControlACK` / `ExpectControlACKAfterFrames` / `HasPendingControlACK` 接口：处理 `@f1` 等命令"先数据帧后 ACK"或"先 ACK 后数据"的可选 ACK 时序。
- `shared/device-sdk/go/daq/hardware/daq_t1603.go` 新增 `resyncHardwareConfigMode`：`applyHardwareConfig` 中途收到 E 错误时逐条查询设备实际 BIN / TIME / HEAD 并同步到本地 cfg 与 FrameReader，避免配置失败后本地 cfg 与设备实际模式不一致。
- `projects/daq-t1603/apps/desktop-wails/cmd/stopstartprobe/`：新增 stop/start probe CLI 调试工具，用于排查快速启停采集时 TCP socket 残留帧问题。

### Changed

- `shared/device-sdk/go/protocol` 的 `DialTCP` 改为无缓冲 channel + abandoned close 信号：原实现 resultCh 为缓冲 1 channel + select-default 检测主线程是否已放弃，但主线程超时返回后 goroutine 仍可能 send 成功到缓冲 channel（default 不触发），导致晚到 conn 滞留缓冲 channel 无人接收、FD 泄漏。改为无缓冲 channel 保证 send 成功 ⇔ 主线程正在接收，主线程超时后 close(abandoned) 让 goroutine 必走 abandoned 分支 Close conn。
- `shared/device-sdk/go/protocol` 的 `P1604ReadCommandACK` 在 soft timeout（net.Error.Timeout 或总预算耗尽）、ReadFrame 任何错误、跳帧上限触发时强制 Close conn 并返回 `ErrWatchdogTriggered` sentinel 让调用方统一毒化驱动状态。迟到响应可能随后进入 TCP 流被下一条命令消费导致协议错位，协议边界已不可信，禁止复用 conn。
- `shared/device-sdk/go/daq/hardware/daq_t1603.go` 的 `sendCommand` 改用 `SendCommandExact(conn, cmd, 1)` 严格单字节响应：'A' 成功；'E' 设备合法拒绝返回 `ErrDeviceRejected` 业务错误（不毒化连接）；其他字节 / 空响应协议错位。删除 `sendCommandIdle`（30ms idle 探测在新模型下无意义）。
- `shared/device-sdk/go/daq/hardware/daq_t1603.go` 的 `queryBinaryMode` 严格校验 `@fd BIN` 响应："1" → binary 模式；"0" → ASCII 模式；其他值 → 协议错误，中止同步并返回错误。修复前 bug：非 "1" 响应被误判为 "0"，导致 BIN=1 命令失败后仍假定 ASCII 模式。
- `shared/device-sdk/go/daq/hardware/daq_t1603.go` 的 `@fe BIN 1` 命令发送后必须重新查询 `@fd BIN` 验证实际模式：设备 temp 固件对 `@fe BIN 1` 会返回 A 但实际不切换二进制模式，仅读回 "1" 时启用二进制模式；"0" 保持/回退 ASCII；其他值视为协议错误中止同步。BIN 验证失败时禁止假定 BIN=1 继续。
- `shared/device-sdk/go/daq/hardware/daq_t1603.go` 的 `noDataTimeout` 改为独立 `time.AfterFunc` timer，不依赖 readLoop 循环体：即使 Read 永久阻塞也能到期触发；var 而非 const 允许测试注入短超时加速用例。新增 `stopAcquisitionTimeout`（3s）限制 StopAcquisition 总预算，超时直接 Close conn。
- `shared/device-sdk/go/motion/adapters/hardware/b140_motion.go` 的 `Disconnect` 修复死锁链：原实现先 `sendCommandLocked("ST")` 再 Close，sendCommand 卡死时 Disconnect 等待 connMu 死锁。改为锁内取 conn 引用 + 置 nil + 置 status.Connected=false，锁外 conn.Close()（TCP FIN 足以让 B140 停止运动，不需要先发 ST 命令）。
- `shared/device-sdk/go/motion/adapters/hardware/b140_motion.go` 新增 `invalidateConnectionLocked` expectedConn 比较：调用方在触发故障前捕获 c.conn 并传入；仅当 c.conn 仍是 expectedConn 时才清空，避免 Disconnect → Connect 替换为新连接后，旧命令的 invalidation 误杀新连接。锁顺序：c.mu（caller）→ c.connMu（本方法），保持正向锁顺序不死锁。
- `shared/device-sdk/go/motion/adapters/hardware/b140_motion.go` 的 `sendCommand` 重构为 I/O goroutine + b140SendResult 结构区分 soft / hard 错误：soft=true（设备返回 "?" 拒绝命令）连接仍可用，不应失效；soft=false（I/O 级硬错误）连接不可靠，应失效。
- `shared/device-sdk/go/ffi/wtnmc4a.go` 的 `getRR1` proc 名从 "WTNMC4A_GetRR1Status" 修正为 "WTNMC4A_GetRR1"（与 DLL 实际导出名一致），移除未使用的 readCV / readCA proc（dead code）。
- `projects/daq-p1604/apps/desktop-wails/adapters/hardware/p1604_adapter.go` 删除本地 `watchdogClose` 函数，改用 `shared/device-sdk/go/protocol.WatchdogClose` 统一 ADR-009 watchdog 实现，避免跨项目代码复制。删除 `p1604ConsecutiveTimeoutThreshold`（25 次 5s 超时）与 `p1604CommandResponseTimeout`（合并入 `p1604HandshakeTimeout`），watchdog + TCP keepalive 已是更可靠的兜底机制。
- `programs/p1604-ts-diag/main.go` 与 `programs/p1604-unit-diag/main.go` 的 `net.DialTimeout` → `sharedproto.DialTCP`（R2-1 整改）：原依赖 Dial 内部 deadline，Windows 故障机器 deadline 不可靠时 Dial 可能永远不返回，工具启动即卡死。新增 5 分钟进程级硬 watchdog，超时后 `conn.Close()` + `os.Exit(2)`，退出码 2 区分 watchdog 硬超时与一般错误。
- `projects/wind-daq/services/api-go/internal/usecase/traversal_registry_recovery.go` 的 `LoadCheckpoint` 入口先检查 `r.sessions[probeID]` 是否仍持有活动 session，若持有则直接返回 (nil, nil)，不扫描 recoveryIndex。即使磁盘上有同 probeID 的 checkpoint 也忽略，因为运行期 session 是真值源。锁实现：r.mu.Lock 取快照后立即 Unlock，避免在持有 registry 锁时进入 recoveryIndex.Find 的 IO 路径。
- `device-lab/skills/daq-t1603/SKILL.md` 状态机图更新：新增 Stopping 中间态（完整尾帧 + A_stop → Stopping → disconnect）；ACK 超时 / 边界异常 → Error（Close 连接，要求重连）。FrameReader 移除 `consumeOptionalACK`（由 `ExpectControlACK`/`HasPendingControlACK` 替代），`reset` 不清零 metadataMode。
- `AGENTS.md` 的 Windows Network I/O Constraint 章节中文化，强化"永远不要把 socket deadline 作为有界硬件 I/O 的唯一取消机制"硬约束表述，引用新增的 `docs/runbooks/go-windows-known-issues.zh-CN.md`。

### Fixed

- 修复 `shared/device-sdk/go/protocol/conn_helpers.go` 的 `DialTCP` 在主线程超时返回后晚到 conn 滞留缓冲 channel 导致 FD 泄漏（R1-4 整改）。
- 修复 `shared/device-sdk/go/protocol` 的 `IsClosedConnError` 未识别 `io.ErrClosedPipe`：net.Pipe 关闭后 Read 返回该错误；单元测试大量使用 net.Pipe 模拟双向连接，noDataTimer/Disconnect Close 后 readLoop Read 会收到 io.ErrClosedPipe，若不识别，readLoop defer 会误走 invalidate 路径覆盖 timer 设置的状态。
- 修复 `shared/device-sdk/go/protocol/daq_t1603_frame.go` 的 `looksLikeReasonableTemperatureFrame` 阈值过严：原 `len(temps)/2`（8 通道合理）导致仅 2 通道接热电偶场景（其余 14 通道为 NaN）触发误判为帧错位，频繁出现 "Frame misalignment; resyncing" 与 "invalid frame at established 64-byte boundary: binary frame values out of expected range" 警告。改为 1（仅需 1 个通道数值合理即接受帧）。
- 修复 `shared/device-sdk/go/daq/hardware/daq_t1603.go` 的 `writeCommandOnly` 中 watchdog 在 `writeMu.Lock` 之后启动的死锁（R0-2）：SetWriteDeadline 失效时 Write 永久阻塞、writeMu 无法释放、所有命令路径死锁。改为 watchdog 在 Lock 之前启动。
- 修复 `shared/device-sdk/go/daq/hardware/daq_t1603.go` 的 `applyHardwareConfig` 中途收到 E 错误后本地 cfg 与设备实际模式不一致导致后续帧解析全错（新增 `resyncHardwareConfigMode` 重新同步实际 BIN / TIME / HEAD）。
- 修复 `shared/device-sdk/go/daq/hardware/daq_t1603.go` 的 `queryBinaryMode` 非 "1" 响应被误判为 "0"：非 "0"/"1" 响应现在返回协议错误，中止同步。
- 修复 `shared/device-sdk/go/daq/hardware/daq_t1603.go` 的 `@fe BIN 1` 命令发送后未重新查询 `@fd BIN` 验证实际模式：设备 temp 固件对 `@fe BIN 1` 会返回 A 但实际不切换二进制模式，必须通过查询 `@fd BIN` 验证实际模式。
- 修复 `shared/device-sdk/go/motion/adapters/hardware/b140_motion.go` 的 `Disconnect` 在 sendCommand 卡死时等待 connMu 死锁：改为锁内取 conn 引用 + 置 nil，锁外 conn.Close()。
- 修复 `shared/device-sdk/go/motion/adapters/hardware/b140_motion.go` 的 `invalidateConnectionLocked` 在 Disconnect → Connect 替换为新连接后误杀新连接（expectedConn 比较）。
- 修复 `shared/device-sdk/go/ffi/wtnmc4a.go` 的 `getRR1` proc 名错误（"WTNMC4A_GetRR1Status" → "WTNMC4A_GetRR1"），原写法在加载阶段会 MustFindProc 失败。
- 修复 `projects/daq-t1603/apps/desktop-wails/adapters/hardware/discovery_socket.go` 的 Send/Receive 在 Windows 故障机器 kernel IOCP deadline 失效时永久阻塞（ADR-009 R0-8 / R0-9 整改）。
- 修复 `projects/daq-p1604/apps/desktop-wails/adapters/hardware/p1604_adapter.go` 的 `p1604WatchdogTimeout` 为 const 时测试无法注入短超时加速 deadline-ignore 回归测试：改为 var 允许测试注入（200ms）。
- 修复 `projects/wind-daq/services/api-go/internal/usecase/traversal_registry_recovery.go` 的 `LoadCheckpoint` 在 registry 仍持有活动 session 时返回磁盘 checkpoint 导致前端同时看到"正在运行"的 session 和"继续/放弃"提示造成状态分裂。
- 移除 `shared/algorithms/go/threehole/interpolation/three_hole.go` 的 `interpolate` 方法（仅是 `interpolateWithWarning` 的薄包装，无调用方）与测试 helper `makePrbLines` / `formatFloat`（历史调试残留）。
- 移除 `projects/three-hole-interpolator/apps/desktop-wails/backend/helpdoc.go`（孤儿文件，原通过 go:embed 嵌入用户说明书 HTML 的调用方已在历史 commit 中移除）。

### Compatibility

- 配置文件格式：兼容。无新增配置字段，无字段含义变更。
- 数据文件格式：兼容。dual traversal checkpoint 仍仅接受 v3 格式，v1/v2 不自动迁移。
- 设备协议行为：
  - `@fe BIN 1` 命令后新增 `@fd BIN` 验证步骤：设备 temp 固件对 `@fe BIN 1` 会返回 A 但实际不切换二进制模式，原代码假定切换成功是 bug，现严格校验。**影响**：若设备固件存在该缺陷，原 0.11.3 行为是"假定切换成功 → 后续二进制帧解析失败 → 触发 resync"，0.11.4 行为是"检测到未切换 → 中止同步并返回错误 → 调用方决定重试或报错"。
  - `sendCommand` 对 'E' 响应从"协议错误 + 毒化连接"改为"业务错误 `ErrDeviceRejected` + 不毒化"：调用方需检查 sentinel 错误决定是否重试或上报，不应假定所有错误都触发重连。
- API 契约：兼容。无 backend 方法签名变更，无 frontend bindings 重生成需求。
- 测试契约：`p1604WatchdogTimeout` 从 const 改为 var 不影响生产代码行为，仅允许测试注入短超时。

### Verification

- `go build ./services/api-go/...` / `go vet ./services/api-go/...`: passed
- `go test -race ./services/api-go/internal/...`: passed
- `shared/device-sdk/go/...`: `go build` / `go vet` / `go test -race -count=1`: passed
- `npm run typecheck`: passed
- `npm run test`: passed
- `npm run build`: passed
- `task release`: passed (production build with -tags production, GOWORK=off)
- `makensis -DARG_WAILS_AMD64_BINARY=build/bin/wind-daq.exe build/windows/installer/project.nsi`: passed
- `task archive-release`: passed (archived to releases/bin/)

### Known Issues

- 安装包未进行 Authenticode 数字签名，Windows 可能显示未知发布者提示。
- dual traversal checkpoint 仅支持 v3 格式，不支持 v1/v2 自动迁移。
- UDP 设备发现 Windows raw winsock 实现依赖 `golang.org/x/sys/windows` 私有常量 `0x1005`（`SO_SNDTIMEO`），后续 Windows SDK 升级需关注该常量稳定性。
- `go.work` 当前包含 `./projects/daq-t1603/apps/desktop-wails`，与 AGENTS.md "daq-t1603 excluded from go.work (ADR-006)" 描述存在不一致。若 ADR-006 仍生效需还原 go.work 改动并同步 AGENTS.md；若已撤销需补充 ADR 记录说明。

## [0.11.3] - 2026-07-31

### Added

- 双探针遍历（dual traversal）后端引入 `ManagerRegistry` 与 `ManagedTraversalManager` 接口，提供 probe-scoped 生命周期管理：每探针一套独立 goroutine + channel（满足"每设备独立线程避免阻塞"硬约束），同 probe 的 Start/Stop/Pause/Resume/RunPoint/CloseProbe 串行化但**不阻塞其他 probe**，全局 `admissionGate` 仅用于 workflow lease 交接等全局事务。
- 实现 dual traversal spec I2 的 lease 端口（`WorkflowLeasePort` / `ControllerLeasePort`）：opaque lease token（`crypto/rand` + hex）保证旧 generation 在 lease 被新 session 接管后无法续约或释放；`(controllerID, axis)` 元组作为资源独占粒度，允许两个 probe 分别 lease 同一控制器的不同物理轴（X 轴 + Y 轴挂同一 B140 是合法配置），`axis=""` 退化为控制器级 lease 兼容遗留调用。
- 新增 `SessionToken`（probeID + 单调 generation）作为完成回调的权威身份：registry 仅在 token 仍是当前 session 且未完成时原子减计数一次，旧 generation 通知只记录诊断不影响当前 session，避免陈旧回调误释放新 session 的 lease。
- dual 模式新增 probe-scoped `LoadCheckpoint` / `ResumeFromCheckpoint` / `ClearCheckpoint` façade 与 `DualTraversalRecoveryIndex`：不变量"映射存在 ⟺ checkpoint 文件存在"，stopped/error 终态保留映射，completed 注销，Stat 失败或文件不存在时注销 stale 映射。
- 新增 `TaskIDGenerator`（进程内单调 taskID 生成）与 `ErrRecoverableTaskExists` / `ErrTaskIDRegisteredToOtherProbe` / `ErrCheckpointVersionMismatch` 哨兵错误；dual 路径仅接受 v3 checkpoint，不自动迁移 v1/v2。
- 新增 `discovery_socket_windows.go`：UDP 设备发现的 Windows raw winsock 分平台实现，使用 `SO_SNDTIMEO`（`0x1005` 常量，因 `golang.org/x/sys/windows` 未导出）+ 独立 watchdog + stop-and-join 语义，落实 ADR-009 "永远不要把 socket deadline 作为有界硬件 I/O 的唯一取消机制"。
- 新增 `discovery_socket.go` 跨平台接口 + `discovery_socket_other.go` 非 Windows fallback；`network_scanner.go` 改用 `openDiscoverySocket` 并新增 `interfaceTimeout=500ms`（`net.Interfaces` 在异常虚拟网卡上无 context 版本会长期阻塞，硬性上限后回退到 `255.255.255.255` 有限广播）。
- 新增 `wtnPXINoDataTimeout=10s` 无数据 timer：WTN-PXI 通常以高采样率推送 8 通道数据帧，10s 内无任何字节到达一定是网络中断/设备断电/网线脱落/服务端崩溃；独立 `time.AfterFunc` timer 不依赖循环体执行，即使 Read 永久阻塞也能到期触发。
- 新增 `DeviceManager.connMuRegistry sync.Map`：序列化同一 device id 上的 Connect/Disconnect/DeleteProfile，防止重连场景下两个 goroutine 同时操作同一物理设备（TCP/串口设备只允许一个会话）。
- 前端新增 `dual-traversal.ts` zh/en 字典与 `i18nInterpolate.ts` 安全占位符替换工具：`String.prototype.replace` 中 replacement 的 `$` 字符会被特殊解析（`$$` → `$`、`$&` → 匹配子串等），后端错误消息中的设备名是不可信输入，函数式 replace 统一兜底避免 `$` 特殊模式注入风险。
- 前端新增 `traversalErrorMapper.ts`：把后端 `device X is not acquiring` / `recoverable_task_exists` 等错误字符串集中映射为本地化用户可读消息，抽取 `RECOVERABLE_TASK_EXISTS_CODE` 常量避免散落字符串匹配。
- 前端新增 `dualTraversalRuntime.ts`：拆分 `pollingUnsubscribers`（终态后停止）与 `snapshotUnsubscribers`（终态后保留让用户在结果界面看压力值），避免无意义轮询浪费 CPU/网络；`realtimeInFlight` 守卫避免高频 snapshot 节流间隙请求堆积导致结果闪烁；`optimisticStatusUntil` 窗口防止 pause/resume 乐观更新期间陈旧轮询回退状态造成 UI 闪烁。
- 前端新增 `traversalModeStore.ts`：single/dual 模式全局 store，带活动检测与 localStorage 持久化。
- 前端 `SevenHoleCharts.vue` 完成 ECharts i18n 完整覆盖：axis / tooltip / legend / 标题全部走 i18nStore，延续 daq-p1604 / daq-t1603 / wind-daq 三项目 i18n 改造工作。
- 新增 `ports/traversal.go` 集中定义 dual traversal 契约端口（`TaskIDGenerator` / `WorkflowLeasePort` / `ControllerLeasePort` / `DualTraversalRecoveryIndex` / `CheckpointStore`），使 usecase 不直接依赖 `resourcelock.Service` 或 storage adapter，符合六边形架构约束。

### Changed

- DAQ-P-1604Pre 默认 host/port 从 `192.168.0.7:9001` 改为 `192.168.3.232:23`（参考 Cursor DAQ 实测值），`CMD_ACQUISITION_CTRL` 命令码从 `0x10` 改为 `0x14`（旧值设备不识别）。旧 profile 已保存的 host/port 不受影响，仅影响新发现设备写入的默认值。
- 五孔探针校准算法 `AcquireData(point, channelReader, samplesPerPoint)` 直接返回 `"五孔探针 AcquireData 缺少探针通道配置，请通过 AcquireDataWithConfig 调用"` 错误，避免悄悄走"零值 fallback"老路径导致 `Kα=0 / Kβ=0 / CPT=0` 无告警。新增 `AcquireDataWithChannels` 通过 `onRealtime` / `onSampleProgress` 回调驱动前端实时监控显示。
- `usecase/traversal_devices.go` 新增采集态与设备可读名检查：`referencedAcquisitionDevices` / `firstNonAcquiringDevice`，dual 启动时若任一引用设备未在采集态则拒绝并返回本地化错误。
- `usecase/device_manager.go` 新增 `connMuRegistry sync.Map` 字段，序列化同一 device id 的硬件 I/O 操作。
- `adapters/runtime/controller_lease.go` 实现 `WorkflowLease` + `ControllerLease` adapter：`WorkflowLease.Renew` 不直接 `Acquire` 兜底（未持有时 `Acquire` 会隐式重建 lease 违背续约语义），先 `IsHeld` 校验当前 holder 匹配再续约。
- `api/server.go` `Deps` 新增 `TraversalRegistry TraversalRegistry` 字段：nil 时 probe-scoped 路由返回 503，legacy 单段 `/api/traversal/{action}` 路径不使用本字段（spec FR4 兼容）。
- `ports/device.go` 新增 `ErrorNotifiable` 接口：设备层错误可主动通知 usecase 触发状态转移。

### Fixed

- 修复双探针遍历实时数据冻结（已在 0.11.2 修复，本版本进一步加固）：撤销 `onSnapshot` 在运行态直接 return 的抑制逻辑，`onSnapshot` 在所有状态下持续更新 `realtimePressures`，`onProgress.latestData` 仅在测点完成时提供权威插值结果覆盖。
- 修复双探针遍历 dashboard 设备流在 dual 页面切换时被错误断开的问题（`deviceStore` 引用计数）。
- 修复双探针遍历实时结果只来自后端 `latestData` 而忽略 DAQ 快照的问题：改为以 DAQ 快照为唯一实时数据源，`latestData` 仅在测点完成时提供权威插值结果。
- 修复七孔校准各区波形图算法错误（图表数据序列错位、tooltip 显示异常）。
- 修复七孔校准顶部状态栏冗余显示马赫数/速度（与侧边栏重复）的问题。
- 修复遍历视图插值状态栏 `interpStatus` 切换时高度跳变的布局抖动问题。
- NSIS 安装脚本添加 UTF-8 BOM（与 1604Cal 项目对齐），避免手动调用 makensis 时解析失败。

### Compatibility

- 配置文件格式：兼容；DAQ-P-1604Pre 默认 host/port 变化仅影响新发现设备，旧 profile 已保存值不受影响。
- 数据文件格式：兼容；dual traversal checkpoint 仅接受 v3，v1/v2 不自动迁移（返回 `ErrCheckpointVersionMismatch`），legacy 单探针 checkpoint 路径不动。
- 设备协议行为：DAQ-P-1604Pre `CMD_ACQUISITION_CTRL` 命令码修正为 `0x14` 是 bug 修复（旧值设备不识别），非破坏性变更。
- API 契约：`Deps` 结构体新增 `TraversalRegistry` 字段，外部装配点需补字段（生产装配由 composition root 注入，影响有限）；`FiveHoleAlgorithm.AcquireData` 直接返回错误而非零值，强制走 `AcquireDataWithConfig`，外部调用方需迁移。

### Verification

- `go build ./services/api-go/...` / `go vet ./services/api-go/...`: passed
- `go test -race ./services/api-go/internal/...`: passed (usecase 108s)
- `shared/device-sdk/go/...`: `go build` / `go vet` / `go test -race -count=1`: passed (daq/hardware 38s, protocol 20s)
- `npm run typecheck`: passed
- `npm run test`: passed (33 files, 348 tests)
- `npm run build`: passed (built in 11.21s)
- `task release`: passed (production build with -tags production, GOWORK=off)
- `makensis -DARG_WAILS_AMD64_BINARY=build/bin/wind-daq.exe build/windows/installer/project.nsi`: passed (6.95 MB installer)
- `task archive-release`: passed (archived to releases/bin/)

### Known Issues

- 安装包未进行 Authenticode 数字签名，Windows 可能显示未知发布者提示。
- dual traversal checkpoint 仅支持 v3 格式，不支持 v1/v2 自动迁移。

## [0.11.2] - 2026-07-29

### Fixed

- 修复双探针遍历测试开始后实时压力值与插值结果冻结的问题。原 C6 修复为避免 DAQ 快照与后端 `latestData` 异步竞态导致抖动，在运行态（running/moving/stabilizing/acquiring）抑制 `onSnapshot` 写入 `realtimePressures`；但 `latestData` 实际来自已保存测点的平均值（历史值，非实时值），导致开始测试后界面冻结在上一个测点或空值，直到下一个测点保存才更新。现撤销该抑制，`onSnapshot` 在所有状态下持续更新 `realtimePressures`，`onProgress.latestData` 仅在测点完成时提供权威插值结果覆盖。新增测试覆盖 moving/acquiring 子状态下 `onSnapshot` 持续更新行为。
- 修复总压校准实时画面概览页"系数卡片"内的马赫数/速度不实时刷新的问题。原实现读 `latestCoefficients.machNumber/velocity`（上一点采集完成时的快照），两点之间不刷新；现改为读 `physics?.machNumber/velocity`（来自后端 `livePhysics` 的 5Hz 推送），与五孔校准和侧边栏"关键数据"卡片的数据源对齐。侧边栏 Ma/V 原本即实时，未改动。数据 Tab 表格中的 Ma/V 列按行渲染每点系数快照，是历史数据，非实时，未改动。
- 修复双探针遍历模式三个问题：1) 运动控制器紧急停止接触后状态未刷新仍显示急停（新增 `refreshStatus()` + `requestFastPolling()` 快速轮询）；2) 不允许配置同一控制器的不同轴（admission 改为 `(controllerID, axis)` 元组粒度，新增 `TestManagerRegistry_Start_SameControllerDifferentAxesAllowed` 等测试覆盖）；3) 误判不同控制器为冲突（前端 `DualTraversalSettings.vue` 改用 ID 比较而非引用比较）。
- 修复双探针遍历实时压力卡片未读取用户在硬件配置步骤逐通道设置的 `precision` 字段的问题。原实现硬编码 `toFixed(3)`，现从 `ch.precision` 提取精度并缺省回退 3，与单探针模式 `useTraversalRealtimeData.ts` 行为对齐。
- 修复遍历视图插值状态栏布局抖动问题（`interpStatus` 切换时高度跳变）。
- 加固 DAQ-T-1603 TCP Dial 路径：原代码缺少 watchdog 兜底，在特定 Windows 环境下 `SetReadDeadline` 失效导致 Dial 永不返回。新增 goroutine + `time.After` 软超时，超时后 `conn.Close()` 解除阻塞。同时修复 T1603 已持 `d.mu` 时再次 `Lock()` 导致自死锁、`drainConnection` watchdog 触发后未清理连接状态等问题。
- 加固 DAQ-P-1064Pre `sendCommand` 在 Write/Read 失败且 watchdog 触发时未调用 `invalidateConnection()` 清空 `d.conn` 的问题，确保上层收到失效通知触发重连。
- 加固 UDP 设备发现：新增 `discovery_socket_windows.go` 分平台实现，Windows 下使用原生 socket + 软超时兜底，防止 `SetReadDeadline` 失效导致扫描永久阻塞；并发扫描保护避免多 goroutine 同时触发发现冲突。
- 加固网络超时取消：所有 sendCommandACK 调用必须循环 ReadFrame 跳过非 ASCII 帧（压力帧），直到读到 ASCII 'A'/'Nxx' 或达到 20 帧上限；StopAcquisition 和 Disconnect 调用 sendCommandACK 前必须 `frameReader.Reset()` 清空残留数据。
- 加固诊断工具（p1604-unit-diag、p1604-ts-diag、freqprobe、frameprobe）：新增 5 分钟进程级硬 watchdog，超时后 `conn.Close()` + `os.Exit(2)`，防止永久卡死。
- 加固 `simulator.go`：`Start(ctx)` 监听 `ctx.Done()` 实现自动 Close；所有 goroutine 纳入 `sync.WaitGroup`，`Close()` 中 `wg.Wait()`；`cmdLoopIdle` 内层循环仅设置一次 idle deadline。

### Changed

- Wails 网络协议层一致性整改：`IsWatchdogTriggered` 改用 sentinel error + `errors.Is` 替代字符串匹配；`WatchdogClose` 入口增加 nil/timeout 防御性检查；`DialTCP` timeout 分支启动 goroutine 关闭 conn 防 FD 泄漏；`dsa3217.readLoop` defer 块检查 conn 失效时跳过 onError 避免双重调用；`daq_t1603.Disconnect` 在 stopAcquisitionLocked 返回 error 时保留 Error 状态不掩盖 invalidate。
- 静态分析集成：`scripts/validate-structure.ps1` pre-submit 流程新增 `staticcheck -checks U1000 ./...`，防止 dead code 漏网；新增 `scripts/staticcheck-u1000-waivers.txt` 豁免清单（仅保留 build tag 分平台存根等误报）；清理 33 处预存 dead code。

### Verification

- `go build ./services/api-go/...` / `go vet ./services/api-go/...`: passed
- `go test -race ./services/api-go/internal/...`: passed
- `shared/device-sdk/go/...`: `go build` / `go vet` / `go test -race -count=1`: passed
- `npm run typecheck`: passed
- `npm run test`: passed (328 用例)
- `npm run build`: passed
- `task release`: passed
- `makensis build/windows/installer/project.nsi`: passed
- `task archive-release`: passed

### Known Issues

- 安装包未进行 Authenticode 数字签名，Windows 可能显示未知发布者提示。

## [0.11.1] - 2026-07-28

### Fixed

- 修复 DAQ-P-1064Pre 启动采集命令 `sendStartAcquisitionLocked` 缺少 watchdog 兜底的问题。原代码仅依赖 `SetWriteDeadline`，在 SetWriteDeadline 失效的 Windows 电脑上 Write 可能永久卡死且无独立 owner 能 Close conn 解除阻塞，违反 ADR-009。现已在 Write 之前启动 `sharedproto.WatchdogClose(conn, DAQ_P_1064PRE_TIMEOUT)`，超时后强制 Close conn 兜底，Write 失败时通过 `WrapWatchdogError` 附加上下文。
- 修复 DAQ-P-1064Pre 响应帧读取 `readResponseFrame` 可能因 `conn.Read` 部分读取导致协议错位的问题。Go 的 `conn.Read` 只保证返回 1-N 字节，单次 Read 可能只读到 3 字节，使 `header[3]<<8 | header[4]` 基于未初始化字节计算 `dataLen`，后续帧对齐错误。现已改用 `io.ReadFull` 保证 6 字节 header 与 dataLen 字节 body 完整读取。

### Verification

- `go build ./services/api-go/...` / `go vet ./services/api-go/...`: passed
- `go test -race ./services/api-go/internal/adapters/hardware/...`: passed (hardware + sim)
- `shared/device-sdk/go/protocol`: `go build` / `go vet` / `go test -race -count=1 ./...`: passed

### Known Issues

- 安装包未进行 Authenticode 数字签名，Windows 可能显示未知发布者提示。

## [0.11.0] - 2026-07-27

### Added

- DAQ-P-1604 设备配置新增可选本地 IP 绑定，多网卡工位可明确选择通信网卡。
- 七孔探针工作流支持动态 PRB 角度数量与多设备采集配置。

### Fixed

- 修复遍历 `PointResult.StartedAt` / `CompletedAt` 并非真实采样起止时间戳的问题。`traversal_acquisition.go` 原先用 `now - DwellTimeMs - SamplesPerPoint×10ms` 合成（文档 0.8.0 已声明记录"真实起止时间戳"，实现未落地），导致单点间隔恒为稳定等待 + 100ms，与设备采样率无关。现改为在 `collectAveragedSamples` 前后用 `time.Now()` 实测（选项 A：采样窗口语义，不含稳定等待 `DwellTimeMs`）；全 1603/采样 10/20Hz 配置下间隔正确反映为 ~0.5s。同时修正 skip 分支漏设 `StartedAt`（原默认 0 → 空串）的隐患。
- 加固 DAQ-P-1604 连接握手：空帧可恢复、握手有总超时、对端关闭时清理死连接。

### Verification

- `go test ./internal/... ./api/...`
- `npm run build`
- `task release`
- `makensis build/windows/installer/project.nsi`

### Known Issues

- 安装包未进行 Authenticode 数字签名，Windows 可能显示未知发布者提示。

## [0.10.0] - 2026-07-24

### Added

- LogViewer 新增硬件收发命令日志（hardware-send / hardware-recv）可见化能力，新增 `showHardware` 开关。

### Changed

- 模拟控制器直线轴默认速度调整为 4；WTNMC4A 轴创建时默认微步设为 4。
- 抽取 `LogEntryRow.vue` 重构日志视图，日志单行显示、按钮风格统一、标题栏新增中英文切换。

### Fixed

- 修复 NSIS 安装包中文文案乱码（mojibake → UTF-8）。
- 修复探针参考 β 角计算错误。
- 修复 1000Hz 采集下高频硬件日志刷屏问题（新增 `CategorySkipHandler`）。

### Compatibility

- 配置文件格式：兼容；新增字段均为可选字段，旧配置继续使用默认值。
- 数据文件格式：兼容；本次变更不涉及数据文件格式变化。
- 设备协议行为：无不兼容变化。

### Verification

- `go build -buildvcs=false ./...` / `go vet ./...` / `go test ./internal/... ./api/...`: passed
- `npm run test`: passed（19 文件，250 测试）
- `task check-bindings`: passed（57/57）
- `task release` + `makensis` + `task archive-release`: passed
- 生产 EXE 冒烟测试: passed

### Known Issues

- 安装包未进行 Authenticode 数字签名；无头环境无法完整渲染 GUI（WebView2 focus/eval 报错）。

## [0.9.0] - 2026-07-22

### Added

- 新增七孔探针校准完整工作流，覆盖分区采样、实时图表、系数计算、不确定度分析及校准数据持久化。
- 遍历配置新增七孔探针与五孔 PRB 能力，支持对应插值器加载、探针参数配置和实时结果展示。
- 自定义布点支持逐点覆盖运动位置与采集参数，可按测点配置差异化测试工况。

### Changed

- 校准与遍历的运动安全设置统一使用产品默认值，减少现场配置负担。
- 探针校准速度显示与保存精度统一为 3 位小数。

### Fixed

- 修复长距离或低速遍历运动超过 120 秒后，机构仍在正常运行却被误判为超时的问题。
- 修复用户临时停止设备采集后，遍历和校准无法在恢复采集后继续完成当前测点的问题。
- 修复校准等待新帧时把停采时间计入超时预算，导致临近截止时间恢复仍被误判超时的问题。
- 修复桌面端与独立 API 装配路径遗漏校准采集状态控制器的问题。
- 恢复 Wails 前后端官方配对版本，避免 runtime 协议不一致引发调用失败。
- 完成 Wind-DAQ 全量代码审查整改，覆盖校准生命周期、并发访问、数据校验与错误传播等问题。

### Compatibility

- 配置文件格式：兼容；新增字段均为可选字段，旧配置继续使用默认值。
- 数据文件格式：兼容；新增七孔校准数据不改变既有五孔、三孔、总压和总温文件格式。
- 设备协议行为：无不兼容变化。

### Verification

- `go test ./internal/... ./api/... ./pkg/...`
- `go test -race ./internal/core/calibration ./internal/usecase`
- `go vet ./internal/... ./api/... ./pkg/...`
- `npm run typecheck`
- `npm run test`
- `npm run build`
- `go build -tags production -trimpath -buildvcs=false -ldflags="-w -s -H windowsgui"`
- `makensis /INPUTCHARSET UTF8` 构建安装包

### Known Issues

- 安装包未进行 Authenticode 数字签名。

## [0.8.0] - 2026-07-18

### Added

- 遍历采集支持多设备并行采集，采集结果按设备/通道归并写入。
- 遍历 CSV 计算结果新增 `StartedAt` / `CompletedAt` 两列：记录单点采集真实起止时间戳（秒级，与 `Timestamp` 列格式一致），用户可直接用 `CompletedAt - StartedAt` 算出单点总耗时，不再依赖"点数×10ms"回填公式。0 值写空字符串，兼容旧数据/异常路径。
- 自定义布点步骤内新增运动轴绑定，布点与运动轴配置合并到同一步骤。

### Changed

- 自定义布点交互改造：布点 UX 重排，运动轴绑定迁入布点步骤。
- 实时刷新率统一到全局 `storageStore.settings.refreshRateHz`（默认 5Hz）：遍历实时插值节流间隔改为派生自全局刷新率，压力卡片与插值卡片同频，消除"刷新节奏不一致"的视觉错位。移除独立的 `useUiRefreshThrottle` composable。

### Fixed

- 无有效样本失败时记录设备应答与通道索引，便于现场定位。

### Internal

- 校准 JSON 序列化测试从 core 迁移到 adapters/storage 层，符合分层边界。
- 七孔校准 spec/plan/tasks 文档与 skill 脚本补充。
- `http-client` 测试冻结 `Date.now`，修复 `deviceApi.pollLatest` 中 `setTimeout` 首次参数从 50 变 49 的 flaky。

### Verification

- `go test ./...`
- `npm run typecheck`
- `npm run build`
- `npm run test`
- `go build -tags production -trimpath -buildvcs=false -ldflags="-w -s -H windowsgui"`
- `makensis` 构建安装包
- 冒烟测试：启动新构建产物确认 GUI 正常

### Known Issues

- 暂无。

### Compatibility

- 数据文件格式：兼容。CSV 仅新增可选列 `StartedAt` / `CompletedAt`，旧文件与旧解析逻辑仍可用。
- 配置文件格式：兼容。
- 设备协议行为：无变化。

## [0.7.0] - 2026-07-15

### Added

- 运动安全机制：遍历与校准运行过程中按轴实时监控目标-实际偏差、临界偏离、限位触发、卡死无进展等异常，触发即停止运动并快照现场。
- 共享组件 `MotionSafetyAlertCard`（故障现场卡片，红色急停 / 橙色停止双色）与 `MotionSafetyPanel`（运动安全配置面板）。
- 遍历设置面板新增运动安全配置区，支持全局与按轴覆盖（到达容差、临界偏离限、无进展超时、进展阈值）。
- 五种校准类型（五孔 / 三孔 / 总压 / 总温 / 总压探针）设置面板与主界面接入运动安全配置与告警展示。
- 运动安全 spec 文档：`spec-traversal-motion-safety.md` 与 `spec-calibration-motion-safety.md` 共同构成需求契约。

### Changed

- b140 / wtnmc4a 控制器停止 / 急停逻辑收敛：`stopAllAxesLocked` 共享、`resolveStartMove` / `resolveStopAxis` 测试 seam 统一。
- `coalesceFloat64Ptr` / `coalesceIntPtr` 统一 `MotionSafetyConfig.Resolve` / `Merge` 的指针字段合并语义。
- 前端 `getMotionSafetyVerdictLabel` 共享函数消除两处独立 switch 实现。

### Fixed

- 校准运动等待竞态：运动安全机制接入后等待逻辑收敛到统一时序。
- WTNMC4A 急停标志位时序：先置位 `EmergencyStopped` 再调用停止，避免停止执行期间其他 goroutine 发起新运动命令的时间窗。

### Internal

- 后端新增 `traversal_motion_watchdog` 看门狗与 `MotionSafetyConfig` / `MotionSafetyFailure` / `MotionSafetyVerdict` 类型体系。
- 新增 `traversal_motion_safety_test` / `calibration_motion_safety_test` 覆盖 verdict 判定、看门狗触发、急停锁存、按轴停止等关键路径（+2380 行测试）。
- Wails binding 同步：重新生成 traversal / calibration / device 的 JS binding，新增 `internal/core/traversal/` 目录。
- `.gitignore` 追加 `.codebuddy/` / `.trae/` / `.opencode/` 三条 AI 工具私有目录规则。

### Verification

- `go build ./...`
- `go test ./internal/core/... ./internal/usecase/... ./internal/ports/...`
- `go test ./...` (device-sdk/go/motion)
- `npm run typecheck`
- `npm run build`
- `go build -tags production -trimpath -buildvcs=false -ldflags="-w -s -H windowsgui"`
- `makensis` 构建安装包

### Known Issues

- DAQ-P-1604 设备固件时间戳 bug 仍存在，CSV 时间戳已统一截断到秒级规避。
- 运动安全机制目前仅覆盖遍历与校准运行场景，手动 Jog / MoveTo 不接入监控。

## [0.6.0] - 2026-07-14

### Added

- 设备类型扩展：`DeviceTypeDAQP1603`、`DeviceTypeDAQT1603` 等新设备类型支持。
- 默认配置迁移：新增默认配置版本迁移逻辑，支持旧配置文件自动升级。
- 默认配置测试：`default_profiles_test.go` 新增 94 行测试覆盖。

### Changed

- DAQ-P-1603 配置面板（DaqP1603Config）UI 改进。
- 设备管理抽屉（DeviceManagementDrawer）UI 优化。
- `shared/device-sdk` 核心类型扩展：新增设备类型常量。
- i18n 补充：覆盖新增设备类型文本。
- 安装程序语言选择后不再卡顿：WebView2 安装移至正式安装阶段，并保留离线包优先、在线下载回退能力。

### Fixed

- 修复 DAQ-P-1603 已关闭“校零应用”的通道仍可发起校零的问题；全部校零会跳过这些通道，并保持用户的使能设置。
- 修复设备所有压力通道均关闭“校零应用”后，前端设备级校零按钮仍可点击的问题。

### Internal

- `shared/device-sdk/go/daq/core/types.go` 新增设备类型枚举值。
- `internal/core/device/types.go` 同步扩展。
- `internal/adapters/config/migration.go` 新增迁移入口。
- `internal/adapters/config/default_profiles.go` 新增默认配置项。

### Verification

- `go test ./...`
- `go build -buildvcs=false ./...`
- `go build -tags production -trimpath -buildvcs=false -ldflags="-w -s -H windowsgui"`
- `npm run typecheck`
- `npm run build`
- `makensis` 构建安装包

### Known Issues

- DAQ-P-1604 设备固件时间戳 bug 仍存在，CSV 时间戳已统一截断到秒级规避。

## [0.5.4] - 2026-07-13

### Changed

- 遍历采集采样循环日志增强：区分"设备无数据"与"通道不匹配"两类失败，便于排查采集异常。

### Verification

- `go test ./...`
- `go build -buildvcs=false ./...`
- `go build -tags production -trimpath -buildvcs=false -ldflags="-w -s -H windowsgui"`
- `npm run typecheck`
- `npm run build`
- `makensis` 构建安装包

### Known Issues

- DAQ-P-1604 设备固件时间戳 bug 仍存在，CSV 时间戳已统一截断到秒级规避。

## [0.5.3] - 2026-07-13

### Changed

- 遍历活动索引（TraversalActiveIndex）重构：接口简化，逻辑清理。
- 遍历采集（traversal_acquisition）微调。

### Internal

- 新增 `traversal_acquisition_test.go` 测试覆盖。
- `traversal_active_index_test.go` 更新适配重构。

### Verification

- `go test ./...`
- `go build -buildvcs=false ./...`
- `go build -tags production -trimpath -buildvcs=false -ldflags="-w -s -H windowsgui"`
- `npm run typecheck`
- `npm run build`
- `makensis` 构建安装包

### Known Issues

- DAQ-P-1604 设备固件时间戳 bug 仍存在，CSV 时间戳已统一截断到秒级规避。

## [0.5.2] - 2026-07-13

### Added

- 校准授权对话框（`CalibrationLicenseDialog`）：首次进入校准功能时展示授权信息，支持确认/关闭交互。
- 前端 i18n 补充：`i18nStore` 新增 36 条翻译项，覆盖新增授权对话框及面板文本。

### Changed

- 遍历步骤面板（TraversalLayoutStep）UI 改进（对齐新配置步骤顺序 Hardware→Probe→Layout→Review）。
- MotionView 控制器配置 UI 优化。
- MainDashboardView 布局调整。

### Fixed

- 安装程序语言选择对话框中文乱码：`MUI_LANGDLL_INFO` 中文字符修复。
- DAQ-P-1604 校零范围限制：大气压/温度辅助通道禁止校零操作。
- 遍历前置检查纳入运动控制器连接态检测：避免"已装配但全部离线"时仍显示绿色就绪状态。
- 五孔探针 PRB 插值算法重构：精简代码约 160 行，提高可维护性。

### Behavior Changes

- 五孔探针 PRB 插值器：超范围输入（压力系数落在 ±30° 校准网格凸包外）不再钳位到角点 (±30, ±30)；
  改为返回 `IsValid=false` 并附 Warning `"压力系数超出PRB校准网格，旧算法不支持外推"`。
  下游消费者需显式处理 `IsValid=false` 路径。

### Changed

- 遍历配置步骤顺序调整：Hardware → Probe → Layout → Review。

### Verification

- `go test ./...`
- `go build -buildvcs=false ./...`
- `go build -tags production -trimpath -buildvcs=false -ldflags="-w -s -H windowsgui"`
- `npm run typecheck`
- `npm run build`
- `makensis` 构建安装包

### Known Issues

- DAQ-P-1604 设备固件时间戳 bug 仍存在，CSV 时间戳已统一截断到秒级规避。

## [0.5.1] - 2026-07-13

### Fixed

- DAQ-P-1604 校零范围限制：大气压/温度辅助通道禁止校零操作，防止误操作导致设备状态异常。
- 遍历前置检查改进：纳入运动控制器连接态检测，避免"已装配但全部离线"时仍显示绿色就绪状态。

### Changed

- 遍历配置步骤顺序调整：由原顺序改为 Hardware → Probe → Layout → Review，优化用户体验。

### Verification

- `go test ./...`
- `go build -buildvcs=false ./...`
- `go build -tags production -trimpath -buildvcs=false -ldflags="-w -s -H windowsgui"`
- `npm run typecheck`
- `npm run build`
- `makensis` 构建安装包

### Known Issues

- DAQ-P-1604 设备固件时间戳 bug 仍存在，CSV 时间戳已统一截断到秒级规避。

## [0.5.0] - 2026-07-13

### Added

- 遍历测点 Point JSON 序列化（`point_json.go`）：NaN↔null 往返契约，支持 line/rectangle/sector 模式未配置轴标记，运动恢复语义完整保留。
- 遍历输出路径集中管理（`output_path.go`）：`ResolveOutputPath` 统一路径派生，消除 CSV/checkpoint/result log 路径碎片。
- 跨平台原子文件操作：`atomic_replace.go` 拆分为 `atomic_replace_common.go` / `_unix.go` / `_windows.go`，支持 Windows 平台安全原子替换。
- 遍历 v2 端口集成测试（`traversal_v2_integration_test.go`，593 行）：覆盖 Resume 截断、ValidateTail 抗损坏、列配置 Open 路径、旧格式兼容性、NaN 往返一致性。
- 遍历 CSV writer 崩溃恢复可靠存储：表头 fsync 落盘、`openCreateUnique` 自动编号防覆盖、双重初始化防御、`applyConfigLocked` 共享列配置逻辑。
- 前端 i18n 国际化补充：FiveHoleMain 模板文案、探针校准入口、设备详情面板。

### Changed

- 遍历采集 `commitPointV2` 三阶段提交增加 NaN 清洗（设备层异常防御），Point 字段 NaN 保持运动恢复语义，Calculated 字段 NaN 清洗为 0。
- 遍历断点重构：`FileCheckpointPort` 增加 `Close` 和 `checkOpen`，`FileCheckpointPortFactory.Create` 接受 `ctx` 参数。
- 遍历活动索引安全加固：`validatePath` 增加 `.` 禁止和 store-based read，防止路径遍历攻击。
- 遍历结果日志：`TraversalResultLog.Open` 支持 `openCreateUnique` 自动编号，与 CSV 行为一致。
- 编译期接口断言哨兵：`TraversalCsvWriter`（2 个：TraversalCSVPort + TraversalPointSink）、`FileCheckpointPort`、`FileCheckpointPortFactory`、`FileCheckpointStore`、`TraversalActiveIndex`、`TraversalResultLog` 共 7 个断言，覆盖 6 个 adapter 文件。
- 前端设备面板优化：DeviceCard、ChartSelector、DeviceDetailPanel UI 调整。

### Fixed

- 修复遍历 Resume 时旧格式 checkpoint（SavePath=目录）被当文件传给 Open 的 bug，通过 `ResolveOutputPath` 重算修复。
- 修复遍历 CSV 表头在 v2 Open 路径缺少通道列的问题（`applyConfigLocked` 共享逻辑）。
- 修复遍历 Resume 时 CSV 文件因已存在直接报错拒绝启动，改为自动编号另存。
- 修复遍历断点文件在 v2 装配下双重初始化导致句柄泄漏 + 文件残留。
- 修复 Point JSON 序列化 NaN 导致 `json.Marshal` 返回 "unsupported value: NaN" 错误。

### Internal

- 遍历核心类型 `types.go` 增加 SaveOptions、CustomFields 等结构字段。
- `ports/traversal.go` 接口扩展（TraversalCSVPort、TraversalCheckpointPort.Close 等）。
- 遍历 usecase 大幅重构：checkpoint 加载/保存、acquisition 状态管理、session 生命周期。
- `traversal_checkpoint_test.go` 适配新接口签名。

### Verification

- `go test ./...`
- `go build -buildvcs=false ./...`
- `go build -tags production -trimpath -buildvcs=false -ldflags="-w -s -H windowsgui"`
- `npm run typecheck`
- `npm run build`
- `makensis` 构建安装包

### Known Issues

- DAQ-P-1604 设备固件时间戳 bug 仍存在，CSV 时间戳已统一截断到秒级规避。

## [0.4.1] - 2026-07-10

### Changed

- 遍历测试 UI 样式优化：PointsPreview 布点画布增强（渐变背景、轨迹线、立体点渲染），WorkspaceArea 顶栏极简化（去掉装饰条、分隔线分组），TopBar 状态指示改用圆点+文字替代徽章、进度条颜色对齐主题色。
- 整体视觉更轻盈现代，降低视觉重量。

### Internal

- 3 个 Vue 组件纯前端样式调整，无逻辑或接口变更。

### Verification

- `npm run typecheck`
- `npm run build`
- `go build -tags production -trimpath -buildvcs=false -ldflags="-w -s -H windowsgui"`
- `makensis` 构建安装包

### Known Issues

- DAQ-P-1604 设备固件时间戳 bug 仍存在，CSV 时间戳已统一截断到秒级规避。

## [0.4.0] - 2026-07-10

### Added

- 全面国际化 i18n 重构：新增 `i18nStore` 全局语言管理，覆盖设备、校准、遍历、存储、布局、设置等所有 UI 模块的中英文切换。
- 遍历测试 UI 大改：左侧栏与布点画布 UI 全面优化，前端补算预估剩余时间。
- 校准采样进度实时反馈：自动校准流程增加进度上报，用户可实时观察采样状态。
- 三孔系数重命名与马赫数指针化：`Kt→K0`、`Sb→Kv`，马赫数改用指针避免零值歧义。
- 总温 CSV 修复：表头与单位修正，适配中文表头。
- DAQ-T1603 Win7 兼容版温度采集程序（`projects/daq-t1603-win7-python/`）：纯 Python 实现，支持 Win7 无 .NET 环境。

### Changed

- 五孔/三孔/总压/总温校准 UI 全面重构：组件拆分、配置面板重排、表单校验增强。
- 日志查看器（LogViewer）布局重构：更清晰的过滤器与日志流展示。
- DeviceManagementDrawer 重构：设备管理抽屉布局优化。
- `shared/device-sdk` DAQ-P-1603 驱动重构：FFI 超时参数传指针修复 readLoop 立即退出，提升长时间采集稳定性。
- 存储设置（StorageSettings）与全局设置（GlobalSettings）重构：对齐新 i18n 体系。

### Fixed

- 修复遍历测试中绝压假表压与混合单位输入的问题：`traversal_view.go` 单位转换逻辑修正 + 单元测试覆盖。
- 修复 DAQ-P-1603 FFI timeout 参数传指针导致 readLoop 立即超时退出。

### Internal

- `i18nStore` 新增 1078 行中英文字典，覆盖全部 UI 模块。
- 校准 CSV writer 新增中文表头支持（BOM 前缀）。
- 前端 `useCalibrationWorkflow` composable 重构，优化校准状态管理。
- 后端 `calibration_total_temperature_csv_test.go`、`total_pressure_test.go` 等新增测试。

### Verification

- `go test ./...`
- `go build -buildvcs=false ./...`
- `go build -tags production -trimpath -buildvcs=false -ldflags="-w -s -H windowsgui"`
- `npm run typecheck`
- `npm run build`
- `makensis` 构建安装包

### Known Issues

- DAQ-P-1604 设备固件时间戳 bug 仍存在，CSV 时间戳已统一截断到秒级规避。

## [0.3.5] - 2026-07-06

### Added

- 校准采集数据新鲜度检查：各算法在多次采样间通过设备时间戳判断是否新帧，避免重复读取缓存旧数据导致标准差为0。

### Changed

- 三孔/五孔/总压/总温校准算法重构：校验方向统一为"来流为正"，消除各算法间符号约定不一致的问题。
- CSV 模式配置重构：区分用户模式（user）和 CSV 模式（csv），校准选项仅在 CSV 模式下可配置。
- 校准暂停/恢复逻辑优化：暂停时自动保存中间状态，恢复时从断点继续。

### Internal

- 校准核心类型定义重构（types.go），预定义通道转换为运行时初始化。
- CSV 格式定义（csv_schema.go）重构，统一各算法表头生成。
- 前端校准工作流 composable（useCalibrationWorkflow）重构，优化状态管理。
- 新增 TimestampReader 类型与 AcquisitionHub.GetLatestTimestamp，校准引擎注入到 Config.TimestampReader。
- total_pressure.go fallback sleep 恢复 BatchPollIntervalMs 配置感知。

### Verification

- `go test ./...`
- `go build -buildvcs=false ./...`
- `go build -tags production -trimpath -buildvcs=false -ldflags="-w -s -H windowsgui"`
- `makensis` 构建安装包

### Known Issues

- DAQ-P-1604 设备固件时间戳 bug 仍存在，CSV 时间戳已统一截断到秒级规避。

## [0.3.4] - 2026-07-06

### Fixed

- 修复 motion 独立窗口可被拉小导致 UI 错乱的问题：禁用缩放，固定最小尺寸为 1280×640，对齐 motion-controller 做法。
- 修复 WTNMC4A 运动控制器负方向点动/定位时位移台不移动的问题：`PLSLogLever` 未与 `Direction` 同步导致 CP 脉冲方向电平不翻转，电机驱动器收不到反向脉冲。

### Verification

- `go test ./internal/... ./api/...`
- `npm run typecheck`
- `npm run build`
- `go build -tags production -trimpath -buildvcs=false -ldflags="-w -s -H windowsgui"`
- `makensis` 构建安装包
- 冒烟测试

### Known Issues

- DAQ-P-1604 设备固件时间戳 bug 仍存在，CSV 时间戳已统一截断到秒级规避。

## [0.3.3] - 2026-07-03

### Fixed

- 修复 DAQ-P-1604 通道选择器文本被截断的问题；排除大气压通道默认不显示在图表中。
- DAQ-P-1604 v0.2.4：应用层增加连续超时断连检测，优化 keepalive 参数，提升长时间采集稳定性。

### Internal

- motion 重构：移除轴启用开关，强制所有轴始终启用（简化状态管理）。

### Verification

- `go test ./internal/... ./api/...`: passed
- `go build -buildvcs=false ./...`: passed
- `npm run typecheck`: passed
- `npm run build`: passed
- `go build -tags production -trimpath -buildvcs=false -ldflags="-w -s -H windowsgui"`: passed
- `makensis` 构建安装包: passed
- 冒烟测试: passed（GUI 启动正常）

### Known Issues

- DAQ-P-1604 设备固件时间戳 bug 仍存在，CSV 时间戳已统一截断到秒级规避。

## [0.3.2] - 2026-07-03

### Fixed

- 修复 T1603 设备停止采集后立即配置参数时命令响应乱码或失败的问题。停止采集后 TCP 缓冲区残留采集数据帧，ApplyDaqT1603Config 的 sendCommand 把残留当作命令响应读出。通过 shared/device-sdk/go/daq/hardware/daq_t1603.go 的修复，在 stopAcquisitionLocked 停止命令后增加 drainConnection 排空残留数据；在 ApplyDaqT1603Config 调用 applyHardwareConfig 前增加 drainConnection。

### Internal

- 修复点位于 shared/device-sdk 共享代码，wind-daq 的 T1603 设备适配器自动受益。
- motion-controller Taskfile.yml 中 rsrc.syso 清理逻辑改用 Test-Path 显式检查，避免 -LiteralPath -ErrorAction SilentlyContinue 在部分环境下失效。
- 同步 wails_windows_amd64.syso 上次 release build 产物。

### Verification

- `go test ./internal/... ./api/...`
- `go build -buildvcs=false ./...`
- `task release`

### Known Issues

- DAQ-P-1604 设备固件时间戳 bug 仍存在，CSV 时间戳已统一截断到秒级规避。

## [0.3.1] - 2026-07-03

### Fixed
- 修复 CSV Timestamp 列时间戳精度问题：采集 CSV 与测点遍历 CSV 统一截断到秒级（`'YYYY-MM-DD HH:MM:SS`），避免展示错误的时间细分。原因详见 daq-p1604 v0.2.2 release note（DAQ-P-1604 设备硬件时间戳固件 bug）。

### Internal
- Taskfile `generate-icon` 任务改用 `build/windows/info.json` 和 `build/windows/wails.exe.manifest` 模板文件，`wails3 generate syso` 内部用 `wails.json` 的 info 字段渲染模板。删除冗余的具体值版本 `build/info.json` 和 `build/windows.manifest`，版本号源从 7 个收敛到 6 个。
- 重新生成 `wails_windows_amd64.syso` 资源段。
- 同步更新 `build/config.yml` 的 `info.version` 字段，对齐项目版本号。

### Verification
- `go build ./...`: passed
- `go vet ./internal/adapters/storage/...`: passed
- `go test ./internal/adapters/storage/...`: passed

### Known Issues
- 暂无。

## [0.3.0] - 2026-07-02

### Added
- CSV sink 按设备切文件：DAQ-P-1604 采用 18 通道宽格式，其余设备长格式；文件名 `prefix-deviceId-YYYYMMDD-HHMMSS-NNN.csv`。
- DAQ-P-1604 硬件时间戳开关（`DaqP1604UseDeviceTimestamp` 改为三态 *bool，nil 视为开启）。
- B140 编码器补偿编辑器 UI（预设/自定义参数、实时 warning 校验、光栅尺精度约束提示）。
- DataPayload 新增 DeviceType 字段，填充 6 个 hardware adapter。

### Changed
- rotation 路径持 statsMu 写锁，修复与 Status() 的 data race。
- T1603 驱动同步化 Connect 时序：OnConfigSynced 回调在 Connect 内同步触发。

### Fixed
- 修复 wind-daq T1603Adapter.Connect() 自死锁：适配器持 a.mu 调 dev.Connect()，而同步 OnConfigSynced 回调重入 a.mu 导致永久死锁。
- 修复 T1603 硬件时间戳开关不生效：驱动层原本无条件下发 `@fe TIME 0` 并将 ShowTimestamp 置 false，导致 UI 开关一直无效；改为按 `d.config.ShowTimestamp` 下发。

### Internal
- shared/device-sdk T1603 驱动重构：Connect/OnConfigSynced 时序重排，支持同步配置同步。
- 编码器补偿校验链路：ValidateCompensationConfig / 三层精度约束链（脉冲当量 ≥ encoderScale ≥ tolerance > minStep）。

### Verification
- `go test ./internal/... ./api/...`: passed
- `go vet ./internal/... ./api/...`: passed
- `npm run typecheck`: passed
- `npm run build`: passed
- `go build -tags production -trimpath -buildvcs=false -ldflags="-w -s -H windowsgui"`: passed
- 冒烟测试: passed（GUI 启动正常，无"correct build tags"错误）

### Known Issues
- 暂无。

## [0.2.0] - 2026-06-30

### Added
- 运行时遥测（runtime telemetry）系统：实时采集性能监控与状态上报。
- 设备协议模拟器框架（Simulator）：支持故障注入、DAQ-P-1604 协议处理器，用于硬件不可用时的开发与测试。
- 新增二进制存储引擎（binary_sink）、存储工厂（sink_factory）与存储装配（storage_assembly），支持多格式存储扩展。
- 数据采集用例层（acquisition usecase）新增完整的数据分发与存储链路编排。
- 前端遥测 UI：DeviceDetailPanel 增加实时数据流可视化，DeviceManagementDrawer 增加遥测面板。
- 全局设置弹窗（GlobalSettingsModal）、Log SSE 客户端（logSseClient）增强。
- spacing.css 设计令牌扩充。

### Changed
- DAQ-P-1604 适配器大幅重构，提升协议解析效率与稳定性。
- CSV 存储引擎重构，支持新存储装配体系。
- 前端设备卡（DeviceCard）与管理抽屉布局优化。
- HTTP 客户端与设备 API 层增强以支持遥测数据拉取。
- Bootstrap 服务初始化流程集成新模块。
- appcontext 重构以支持更干净的依赖注入。

### Fixed
- DataStreamRelay 测试稳定性提升：增加 defer Stop 和 sleep margin。
- 模拟器中 acceptLoop 区分正常退出与异常退出。

### Internal
- 五孔探针与三孔探针算法新增 golden baseline、边界值测试。
- 设备存储层新增 CSV sink 基准测试与 binary sink 单元测试。
- acquisition 新增基准测试。
- 工作区全面测试计划文档。

### Verification
- `go test ./internal/... ./api/...`: passed
- `go vet ./internal/... ./api/...`: passed
- `npm run typecheck`: passed
- `npm run build`: passed
- `wails3 build` (含前端 + Go 生产二进制): passed
- `makensis` 构建安装包: passed

### Known Issues
- 暂无。

### Internal
- 规范化生产构建标签：Taskfile.yml `build-go` 增加 `-tags production -trimpath` 与 `-w -s`。
- 新增 `clean` 和 `release` 任务。
- AGENTS.md 和 README.md 区分开发与交付命令。
- 创建 VERSION、CHANGELOG.md 和发布基础设施。

### Verification
- `go test ./...`: passed
- `go vet ./...`: passed
- `npm run typecheck`: passed
- `npm run build`: passed
- `go build -tags production -trimpath -buildvcs=false -ldflags="-w -s -H windowsgui"`: passed
- `makensis` 构建安装包: passed

### Known Issues
- 暂无。
