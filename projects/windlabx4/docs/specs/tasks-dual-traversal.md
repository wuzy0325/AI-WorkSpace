# Tasks: WindLabX4 双探针并行遍历测试

> 关联计划：[plan-dual-traversal.md](./plan-dual-traversal.md)
> 关联规格：[dual-traversal-spec.md](./dual-traversal-spec.md)
> 日期：2026-07-27

使用说明：每个 Task 完成后勾选所有 Acceptance Criteria 与 Verification；任何一项未通过均不得勾选 Task。

---

## Phase 1: Foundation

### Task 1: 新增 registry 契约与 controller lease adapter

**Description:** 在 usecase/ports 中新增后续 registry 所需的最小完整契约：`ProbeID`、`SessionToken`、窄 `ManagedTraversalManager`、`TraversalManagerFactory`、服务端 `TaskIDGenerator`、`WorkflowLeasePort` 与 token-checked `ControllerLeasePort`。在 runtime adapter 中基于共享 `resourcelock.Service` 实现两类 lease：workflow 使用固定 resource/holder 并支持续约，controller 使用 controller-scoped resource 和 opaque token holder。Task 4-7 可使用 fake managed manager，Task 8 再让真实 `TraversalManager` 实现该接口，避免依赖环。

**Acceptance criteria:**
- [ ] `internal/usecase/traversal_manager_registry.go` 定义 `ProbeID`、`Probe1`、`Probe2`、`ParseProbeID(s string) (ProbeID, error)`
- [ ] 定义 `SessionToken` 结构（`ProbeID`、`Generation uint64`）与 `String()` 方法
- [ ] 定义窄 `ManagedTraversalManager` 接口，覆盖 registry 所需的 managed Start/Resume、RunPoint、Pause、Resume、Stop、Done/status/result/config/checkpoint 操作
- [ ] 定义 `TraversalManagerFactory` 接口（`NewManager(probeID ProbeID) (ManagedTraversalManager, error)`），registry 不依赖具体 manager 类型
- [ ] 定义 `TaskIDGenerator` 接口；dual task ID 由服务端生成并包含 probe namespace 或等价不可冲突身份
- [ ] 在 `internal/ports/traversal.go` 定义独立 `ControllerLeasePort`：`Acquire(ctx, controllerID, holder, ttl) (leaseToken string, error)`、`Renew(ctx, leaseToken, ttl) error`、`Release(ctx, leaseToken) error`
- [ ] 定义 `WorkflowLeasePort`：`Acquire(ctx, holder, ttl) error`、`Renew(ctx, holder, ttl) error`、`Release(ctx, holder) error`；registry 不导入具体 `resourcelock.Service`
- [ ] lease token 不可由 probe ID/controller ID 单独推导；旧 generation token 不能续约或释放新 session lease
- [ ] runtime lease adapter 基于共享 `resourcelock.Service` 实现 workflow/controller Acquire/Renew/Release；controller resource key 为 controller-scoped，opaque token 为唯一 holder
- [ ] adapter 测试覆盖 workflow 固定 holder 续约、controller 原子争抢、同 token 续约、旧/错误 token 释放拒绝、TTL 到期与不同 controller 并行
- [ ] 不在生产代码中添加 TODO body、空实现或 panic stub
- [ ] 编译通过：`go build ./internal/usecase`

**Verification:**
- [ ] 在 `projects\WindLabX4\services\api-go` 执行 `go build -buildvcs=false ./...`
- [ ] `go vet ./internal/usecase`
- [ ] `go test -race ./internal/adapters/runtime -run TestControllerLease`
- [ ] GitNexus 无 HIGH 风险告警（新增文件无 upstream caller）

**Dependencies:** None

**Files likely touched:**
- `internal/usecase/traversal_manager_registry.go`（新建）
- `internal/ports/traversal.go`（TaskIDGenerator / WorkflowLeasePort / ControllerLeasePort 契约）
- `internal/adapters/runtime/controller_lease.go`（新建）
- `internal/adapters/runtime/controller_lease_test.go`（新建）

**Estimated scope:** M（4 文件，约 280 行）

---

### Task 2: 新增 dual recovery index adapter

**Description:** 在 `internal/adapters/storage/` 新增 `dual_traversal_recovery_index.go`，实现 `ports.DualTraversalRecoveryIndex` 接口（新增于本 Task 的 `internal/ports/traversal.go`）。envelope `version:1`，结构为 `probeId → taskId → checkpointPath` 的两级 map。原子替换流程与现有 `traversal_active_index.go` 一致，但文件独立、互不读写、互不迁移。

**Acceptance criteria:**
- [ ] `internal/ports/traversal.go` 新增 `DualTraversalRecoveryIndex` 接口：`Register(probeId, taskId, checkpointPath)`、`Find(probeId) (TraversalCheckpointRef, bool, error)`、`Unregister(probeId, taskId) error`、`ListProbeTaskIDs(probeId) ([]string, error)`
- [ ] `internal/adapters/storage/dual_traversal_recovery_index.go` 实现该接口，envelope `{"version":1,"probes":{"probe1":{"taskId1":"path1"},...}}`
- [ ] 文件路径与 legacy `traversal-active-index.json` 不同（如 `dual-traversal-recovery-index.json`）
- [ ] 同一 probe 注册第二个 taskId 必须显式拒绝（每 probe 最多一个权威候选），返回 `recoverable_task_exists`
- [ ] 不读取、不迁移、不覆盖 legacy `traversal-active-index.json`

**Verification:**
- [ ] `go test ./internal/adapters/storage -run TestDualTraversalRecoveryIndex`
- [ ] 测试用例：注册/查找/注销/重复注册拒绝/跨 probe 隔离/原子替换/文件不存在时返回空
- [ ] `go test -race ./internal/adapters/storage`

**Dependencies:** None（可与 Task 1 并行）

**Files likely touched:**
- `internal/ports/traversal.go`（追加接口）
- `internal/adapters/storage/dual_traversal_recovery_index.go`（新建）
- `internal/adapters/storage/dual_traversal_recovery_index_test.go`（新建）

**Estimated scope:** S（2-3 文件，约 250 行）

---

### Checkpoint: Foundation

- [ ] 在 `projects\WindLabX4\services\api-go` 执行 `go build -buildvcs=false ./...`
- [ ] `go test ./internal/adapters/storage ./internal/adapters/runtime ./internal/usecase`
- [ ] 类型与接口可被后续 Task 引用

---

## Phase 2: ManagerRegistry

### Task 3: ManagerRegistry 核心结构 + GetOrCreate + ProbeID 校验

**Description:** 定义 `ManagerRegistry` 结构并实现 `GetOrCreate(probeID) (*TraversalManager, error)`：校验 probe ID 和 closing 状态，通过 per-probe creation gate 保证同一 probe 仅有一个 in-flight factory 调用。factory 在 registry mutex 外运行；失败唤醒等待者且不污染 map。

**Acceptance criteria:**
- [ ] `GetOrCreate` 在 `closing == true` 时返回 `registry_closing` 错误
- [ ] 未知 probeID 返回 `invalid_probe_id` 错误
- [ ] factory 创建失败时返回原始错误且不污染 map
- [ ] 并发调用同一 probeID 的 `GetOrCreate` 只调用 factory 一次，不创建或丢弃第二个已装配 manager
- [ ] factory 调用不在持锁状态下进行（避免 factory 内 I/O 阻塞）
- [ ] factory 失败时所有等待者收到同一失败结果；后续新调用可重试创建

**Verification:**
- [ ] `go test -race ./internal/usecase -run TestManagerRegistry_GetOrCreate`
- [ ] 测试用例：未知 ID 拒绝 / closing 拒绝 / 并发同 ID factory 仅调用一次 / factory 失败唤醒等待者且允许重试
- [ ] GitNexus `gitnexus_impact({target:"NewTraversalManager", direction:"upstream"})` 确认 factory 调用链

**Dependencies:** Task 1

**Files likely touched:**
- `internal/usecase/traversal_manager_registry.go`（实现 GetOrCreate）
- `internal/usecase/traversal_manager_registry_test.go`（新建）

**Estimated scope:** S（2 文件，约 200 行）

---

### Task 4: registry Start façade + 原子准入事务

**Description:** 实现 registry 对外 `Start(ctx, probeID, rawConfig) (taskID, error)` façade。registry 解析并校验配置，使用 `TaskIDGenerator` 生成服务端权威 dual task ID，再执行 admission：1) 若 activeCount==0 获取全局 `workflow:traversal` lease；2) 通过 `ControllerLeasePort.Acquire` 原子预占启动快照中的唯一 controller；3) 生成 SessionToken 并登记 session/lease token；4) 调用 manager 的 managed Start。任一步失败按相反顺序回滚；HTTP 不得直接调用 manager Start。

**Acceptance criteria:**
- [ ] 双模式启动前校验两个 controller ID 非空且不同；相同或空绑定返回 `resource_conflict`
- [ ] admission 从 probe-scoped 持久化配置读取 probe1/probe2 controller binding 并在同一 registry 临界区比较；任一路未配置时拒绝启动
- [ ] dual task ID 由服务端生成；客户端 task ID 被忽略或拒绝，但不得成为结果/index/checkpoint 的权威键
- [ ] 第一路启动获取全局 lease；第二路启动不重复 Acquire（activeCount > 0 时跳过）
- [ ] 准入失败时回滚：lease 释放（若本次获取）+ 控制器预占撤销 + session 不登记 + activeCount 不递增
- [ ] 控制器预占仅通过 Task 1 的 `ControllerLeasePort.Acquire` 实现并保存 opaque lease token
- [ ] managed Start 失败时不触发 manager completion callback，由 admission rollback 释放临时资源
- [ ] 并发 `admit(probe1)` + `admit(probe2)` 在 race 测试下不出现双重占用同一控制器
- [ ] Start façade 拒绝已有 recoverable checkpoint 的 probe，且拒绝发生在文件/运动 I/O 前

**Verification:**
- [ ] `go test -race ./internal/usecase -run TestManagerRegistry_Admit`
- [ ] 测试用例：服务端 task ID / 空或相同 controller 拒绝 / 第一路获取 lease / 第二路复用 lease / 每个失败点完整回滚 / TOCTOU 竞态 / recoverable task 拒绝
- [ ] channel/barrier 同步测试，不依赖 `time.Sleep`

**Dependencies:** Task 1, Task 2, Task 3

**Files likely touched:**
- `internal/usecase/traversal_manager_registry.go`（admit / rollback）
- `internal/usecase/traversal_manager_registry_test.go`
- `internal/ports/traversal.go`（ControllerLeasePort）

**Estimated scope:** M（2-3 文件，约 350 行）

---

### Task 5: session token / generation 的 exactly-once 完成与 lease 生命周期

**Description:** 实现 registry 唯一 completion linearization point、lease renewer 与 `workflowTransition` gate。managed manager 在 goroutine 退出且输出端口 finalize 完成后回调 `NotifyCompletion(token)`；registry 校验 generation 并标记 completion in-flight。最后一路清理时先在 mutex 内设置 `workflowTransition=true` 阻止新 admission，再在锁外释放 controller/global leases，最后在锁内原子提交 activeCount=0 并清除 gate。每个 session 暴露 completion done/error，供 Stop/CloseProbe 有界等待。重复或旧 token 只记录诊断。

**Acceptance criteria:**
- [ ] 正常完成：activeCount 递减一次；归零时释放全局 lease
- [ ] admission 在 `workflowTransition=true` 时等待或返回稳定 `registry_transitioning`，不得按旧 activeCount 跳过 workflow Acquire
- [ ] 每个 active session 定期 Renew controller lease；续约器由 session context 管理且 completion 后退出
- [ ] Renew 失败进入可诊断错误并请求该 probe 停止，不允许静默继续到 lease 过期
- [ ] 重复完成（同一 token 二次通知）：仅记录诊断，不再递减
- [ ] 旧 generation 完成：不释放当前 controller lease、不停止当前 renewer、不影响当前 session
- [ ] 并发 NotifyCompletion 与新 Start：新 Start 必须等待旧 session 完成才可复用 probe ID
- [ ] normal/error/Stop/EmergencyStop 最终都收敛到 goroutine exit + finalize 后的同一回调；Stop/CloseProbe 不重复通知
- [ ] controller Release 失败时 session 进入 `completion_failed`，activeCount 不递减、全局 lease 保留、同 probe Start 禁止
- [ ] 最后一路 global Release 失败时 activeCount 保持 1、session 保留为 `completion_failed`；不得报告完成或允许新 Start
- [ ] global Release 成功与 activeCount=0 的提交受 transition gate 保护，不存在新 Start 在两者之间复用旧 lease 状态的窗口
- [ ] `CloseProbe`/`Shutdown` 可用同 token 幂等重试失败的 Release；成功后才提交计数与 completion done
- [ ] registry mutex 内不执行 Renew/Release 或其它外部 I/O
- [ ] 准入事务回滚路径不走 NotifyCompletion（直接撤销临时资源）

**Verification:**
- [ ] `go test -race ./internal/usecase -run TestManagerRegistry_Completion`
- [ ] 测试用例：单路完成释放 / 双路顺序完成 / 并发完成 / 旧 generation 通知 / renew 成功与失败 / renewer 退出 / 回滚不递减
- [ ] fake manager 注入完成回调，断言调用次数

**Dependencies:** Task 4

**Files likely touched:**
- `internal/usecase/traversal_manager_registry.go`（NotifyCompletion）
- `internal/usecase/traversal_manager_registry_test.go`

**Estimated scope:** M（2 文件，约 300 行）

---

### Task 6: Stop / CloseProbe 生命周期

**Description:** 实现 registry `Stop(ctx, probeID)` 与 `CloseProbe(ctx, probeID)` façade。二者请求 manager cancel/stop 并等待 Done；manager goroutine defer 负责 finalize 并回调 Task 5 的唯一 completion point。`CloseProbe` 仅在 completion 已提交后删除终态 manager；不得自行重复 finalize、Release 或 NotifyCompletion。超时保留 closing 条目与 leases。

**Acceptance criteria:**
- [ ] running / paused / terminal / error 状态下均满足生命周期契约
- [ ] ctx 超时返回 `close_probe_timeout` 错误，map 条目保留且 manager 状态标记为 `closing`
- [ ] 保留期间 `GetOrCreate` 同 probeID 返回 `probe_closing` 错误，不创建新 manager
- [ ] Stop 成功仅在 goroutine 退出、输出 finalize、checkpoint 保留和 completion lease 释放完成后返回
- [ ] CloseProbe 不直接调用 finalize、controller Release 或 NotifyCompletion；唯一 completion point 在 Task 5
- [ ] 请求停止与等待错误使用 `errors.Join` 聚合，不因第一步失败跳过有界等待
- [ ] 并发 `CloseProbe(probe1)` 与 `CloseProbe(probe2)` 互不阻塞
- [ ] 不持 registry mutex 时执行 manager shutdown I/O

**Verification:**
- [ ] `go test -race ./internal/usecase -run TestManagerRegistry_CloseProbe`
- [ ] 测试用例：四态生命周期 / 超时保留 / 并发关闭双 probe / 错误聚合
- [ ] fake manager 控制完成时机，验证超时路径

**Dependencies:** Task 5

**Files likely touched:**
- `internal/usecase/traversal_manager_registry.go`（CloseProbe）
- `internal/usecase/traversal_manager_registry_test.go`

**Estimated scope:** M（2 文件，约 280 行）

---

### Task 7: Shutdown 双 deadline + 并发 EmergencyStop + 聚合错误

**Description:** 实现 `ManagerRegistry.Shutdown(ctx) error`：原子设置 `closing=true`（拒绝新任务）；并行关闭所有 manager（每个 manager 用 graceful deadline 5s 等待）；graceful 到期后对仍活动 controller 并发 EmergencyStop（每个用从剩余 hard deadline 派生的 context）；hard deadline 10s 到期返回包含 probe/task ID 的 shutdown error。单个 adapter 卡住不延长总 deadline、不阻止尝试其它控制器。

**Acceptance criteria:**
- [ ] graceful 默认 5s、hard 默认 10s；可通过 composition root 配置覆盖（必须为有限正值且 hard > graceful）
- [ ] EmergencyStop context deadline = hard_deadline - elapsed，禁止 `context.Background()` 或无界等待
- [ ] 并发尝试所有活动 controller 的 EmergencyStop，单 adapter 卡住不阻止其它
- [ ] 返回的错误包含未退出 probe/task ID，便于诊断
- [ ] shutdown 失败时调用方禁止继续关闭共享 motion/acquisition/storage 服务
- [ ] 超时任务保留最后一个有效 checkpoint，不删除 registry 条目

**Verification:**
- [ ] `go test -race ./internal/usecase -run TestManagerRegistry_Shutdown`
- [ ] 测试用例：双 probe graceful 退出 / 单 adapter 卡住至 EmergencyStop / hard deadline 强制返回 / 配置覆盖 / 保留 checkpoint
- [ ] fake controller 注入延迟，验证 EmergencyStop 并发性

**Dependencies:** Task 6

**Files likely touched:**
- `internal/usecase/traversal_manager_registry.go`（Shutdown）
- `internal/usecase/traversal_manager_registry_test.go`

**Estimated scope:** M（2 文件，约 320 行）

---

### Checkpoint: Registry

- [ ] 在 `projects\WindLabX4\services\api-go` 执行 `go test -race ./internal/usecase`
- [ ] 并发启动两 probe 不出现双重占用或 lease 提前释放
- [ ] shutdown 在 hard deadline 内返回，未退出 probe/task 可诊断
- [ ] GitNexus `gitnexus_detect_changes()` 仅包含 registry/usecase、对应 ports 契约与 controller lease adapter，无无关流程

---

## Phase 3: TraversalManager 重构

### Task 8: TraversalManager managed Start/Resume 与 probe-scoped config key

**Description:** 修改 `TraversalManager` 实现 Task 1 的 `ManagedTraversalManager`，显式区分 legacy 与 managed lease ownership。既有 `Start`/`ResumeFromCheckpoint` 继续用于 single 路径并自行 Acquire/Release workflow lease；新增 registry-only managed Start/Resume 入口，接收不可变 `ManagedSessionOptions{ProbeID, ConfigKey, Token, CompletionCallback}`，不 Acquire/Release 全局或 controller lease。两条路径共享任务执行逻辑，但同一 session 只能选择一种 ownership。

**Acceptance criteria:**
- [ ] managed session options 在启动时一次性注入并冻结，禁止通过多个 setter 形成半配置状态
- [ ] managed Start/Resume 不调用 workflow lease Acquire/Release；完成时 finalize 后调用 `completionCallback(token)`
- [ ] legacy Start/Resume/Stop/abort/finalize 的 Acquire/Release 行为与错误传播保持不变
- [ ] ownership mode 是 session snapshot 的一部分；legacy 与 managed 路径不能在同一 session 混用
- [ ] managed admission rollback 发生在 manager session 提交前，不触发 completion callback
- [ ] probe-scoped config key 持久化与加载（`traversal.probe1`、`traversal.probe2`）

**Verification:**
- [ ] `go test ./internal/usecase -run TestTraversal_`
- [ ] 既有 traversal_*_test.go 全部通过（无修改期望）
- [ ] `go test -race ./internal/usecase`
- [ ] GitNexus `gitnexus_impact({target:"TraversalManager.Start"})` 确认 callers 安全

**Dependencies:** Task 5

**Files likely touched:**
- `internal/usecase/traversal.go`（字段 + setter + finalizeSink 改造）
- `internal/usecase/traversal_config.go`（probe-scoped key 加载）
- `internal/usecase/traversal_lifecycle_test.go`（新增 probe-scoped 测试）

**Estimated scope:** M（3 文件，约 350 行）

---

### Task 9: Stop / EmergencyStop 限制在启动快照中的控制器

**Description:** 修改 manager 的 Stop / EmergencyStop 路径：启动时在 `TraversalRunSnapshot` 中冻结该 probe 绑定的 controller ID 列表；运行期 Stop / EmergencyStop / 位置超差 / 限位 / 掉线 / 运动安全故障只对快照中的 controller 发送停止命令，绝不向另一 probe 的 controller 发送。普通任务错误只停止故障 probe，不跨 probe 联停。

**Acceptance criteria:**
- [ ] `TraversalRunSnapshot` 新增 `BoundControllerIDs []string` 字段（启动快照时冻结）
- [ ] `Stop` 只对 `BoundControllerIDs` 中的 controller 调用 `MotionAccess.Stop`
- [ ] `EmergencyStop` 只对 `BoundControllerIDs` 中的 controller 调用 `MotionAccess.EmergencyStop`
- [ ] 位置超差 / 限位 / 掉线触发路径均使用 `BoundControllerIDs`，不扫描全部已连接 controller
- [ ] 双模式禁止"空绑定时操作所有已连接控制器"的兼容回退（spec I1）
- [ ] 测试覆盖：probe1 故障时 probe2 controller 不收到 Stop/EmergencyStop

**Verification:**
- [ ] `go test -race ./internal/usecase -run TestTraversal_MotionSafety`
- [ ] `go test ./internal/usecase -run TestTraversal_BoundControllers`
- [ ] 既有 traversal_motion_safety_test.go 全部通过

**Dependencies:** Task 8

**Files likely touched:**
- `internal/usecase/traversal.go`（snapshot 字段）
- `internal/usecase/traversal_acquisition.go`（Stop/ES 路径）
- `internal/usecase/traversal_motion_watchdog.go`（限位/超差路径）
- `internal/usecase/traversal_motion_safety_test.go`（新增隔离断言）

**Estimated scope:** M（4 文件，约 320 行）

---

### Checkpoint: Manager Refactor

- [ ] 既有 single 路径全部测试通过（traversal_*_test.go 无修改期望）
- [ ] `go test -race ./internal/usecase`
- [ ] managed manager 完成只通过 registry 回报 token，不直接释放 lease；legacy single 原测试保持通过
- [ ] GitNexus `gitnexus_detect_changes()` 仅包含 manager managed-mode 重构及对应测试，无无关流程

---

## Phase 4: Checkpoint v3 与恢复

### Task 10: checkpoint v3 metadata + legacy v1/v2 与 dual v3 路由分离

**Description:** 以当前可靠性 `Checkpoint` v2 和 `TraversalRunSnapshot` 为兼容基线，定义 dual checkpoint v3。v3 保留 v2 的完整 snapshot、commitSeq、真实 CSV/结果日志路径、header/commit hash 与时间字段，并增加 `ProbeID`、`BoundControllerIDs`。legacy single 继续按现有行为读取 v1/v2、写当前 single 版本；dual 只写/读 v3，禁止版本号复用和静默迁移。

**Acceptance criteria:**
- [ ] 定义 `DualCheckpointVersion = 3`；不修改或复用现有 `CheckpointVersion = 2` 的语义
- [ ] v3 包含 `ProbeID`、`BoundControllerIDs`，并完整保留当前 `Checkpoint`/`TraversalRunSnapshot` 的所有可靠性字段
- [ ] v3 不引入旧模型的 `Points`/`LastIndex`，恢复点位继续来自 `Snapshot.Config.Path` 与 `CommitSeq`
- [ ] `FileCheckpointPort` 通过显式 checkpoint mode/codec 写入和校验版本，不按文件名猜测版本
- [ ] legacy 路径保持当前 v1/v2 解码与写入行为，不读取 v3
- [ ] dual 路径遇到 v1/v2 返回 `checkpoint_version_mismatch`，不自动迁移
- [ ] 输出文件名以 probe ID 为前缀（如 `probe1-traversal-...csv`），保留 `-2/-3` 防覆盖机制
- [ ] 实际最终路径以 `OutputPath()` 返回值为准，前端不推断

**Verification:**
- [ ] `go test ./internal/adapters/storage -run TestFileCheckpointPort_V2`
- [ ] `go test ./internal/adapters/storage -run TestTraversalCsvWriter_ProbePrefix`
- [ ] 测试用例：v3 round-trip 保留全部 reliability 字段 / legacy v1/v2 回归 / legacy 不读 v3 / dual 不读 v1/v2 / probe 前缀 / 撞名 -2/-3

**Dependencies:** Task 2, Task 8

**Files likely touched:**
- `internal/core/traversal/types.go`（v3 metadata/codec contract）
- `internal/adapters/storage/file_checkpoint_port.go`（legacy v1/v2 与 dual v3 codec 分支）
- `internal/adapters/storage/traversal_csv_writer.go`（probe 前缀）
- `internal/adapters/storage/file_checkpoint_port_test.go`
- `internal/adapters/storage/traversal_csv_writer_test.go`

**Estimated scope:** M（4-5 文件，约 350 行）

---

### Task 11: probe-scoped resume / clear / loadCheckpoint

**Description:** 实现 registry probe-scoped 恢复 façade：`LoadCheckpoint(probeID)` 只返回 dual index 唯一候选；`Resume(probeID, taskID)` 与 `ClearCheckpoint(probeID, taskID)` 校验权威 index。Resume 加载并校验 v3 后，通过与 Start 相同的 admission helper 取得 workflow/controller leases 和 token，再调用 manager managed Resume；任何 append 文件或运动 I/O 均在 admission commit 之后。

**Acceptance criteria:**
- [ ] `LoadCheckpoint(probeID)` 只查 dual recovery index，不扫描目录
- [ ] `ResumeFromCheckpoint` 校验 taskID == index[probeID].taskID，否则返回 `task_id_mismatch`
- [ ] `ClearCheckpoint` 同样校验 taskID
- [ ] resume 路径通过 registry façade 和共享 admission helper，再调用 managed Resume；HTTP/manager 不可绕过
- [ ] checkpoint 中 `ProbeID` 与请求 probeID 不一致返回 `probe_id_mismatch`
- [ ] checkpoint 中 `BoundControllerIDs` 被其它 session 占用返回 `resource_conflict`，checkpoint 与输出文件保持不变
- [ ] 正常完成或显式放弃后原子注销 dual recovery mapping；dual 路径不读写 legacy `traversal-active-index.json`
- [ ] 相同 taskID 在不同 probe 注册被拒绝

**Verification:**
- [ ] `go test ./internal/usecase -run TestTraversal_Resume`
- [ ] `go test -race ./internal/usecase -run TestTraversal_Resume_Concurrency`
- [ ] 测试用例：唯一候选 / taskID 不匹配 / probeID 不匹配 / controller 冲突 / 注销路径 / 跨 probe taskID 拒绝

**Dependencies:** Task 4, Task 10

**Files likely touched:**
- `internal/usecase/traversal_checkpoint.go`（probe-scoped resume/clear）
- `internal/usecase/traversal_manager_registry.go`（resume 走 admit）
- `internal/usecase/traversal_checkpoint_test.go`

**Estimated scope:** M（3 文件，约 320 行）

---

### Checkpoint: Recovery

- [ ] legacy v1/v2 checkpoint 经 legacy 路由保持现有恢复行为
- [ ] dual v3 checkpoint 不被 legacy 路径误加载；dual 路径不加载 v1/v2
- [ ] resume 在打开 append 文件或运动 I/O 前重新走准入事务
- [ ] 资源冲突时 checkpoint 与输出文件保持不变
- [ ] `go test -race ./internal/usecase ./internal/adapters/storage`

---

## Phase 5: HTTP API

### Task 12: probe-scoped 路由解析 `{probeId}/{action}` 与 registry façade dispatcher

**Description:** 修改 `api/server.go`：严格解析单段 legacy 与两段 probe-scoped 路径。只读 config/status/result 与导入/实时计算等非生命周期操作可由 registry 选择 manager 后委托共享 action handler；`start`、`runPoint`、`pause`、暂停后的 `resume`、`stop`、`resumeFromCheckpoint`、`clearCheckpoint`、终态 `close` 必须调用 registry façade，不能直接调用 manager 生命周期方法。`api.Deps` 增加窄的 `TraversalRegistry` 接口而非依赖具体 registry 结构。

**Acceptance criteria:**
- [ ] `/api/traversal/{probeId}/{action}` 路由命中 probe-scoped dispatcher
- [ ] `/api/traversal/{action}` 单段路径继续走 legacy 路径（不隐式转发到 probe1）
- [ ] start/runPoint/pause/resume/stop/resumeFromCheckpoint/clearCheckpoint/close 仅调用 registry façade；handler 不直接调用 manager 生命周期方法
- [ ] `POST /api/traversal/{probeId}/close` 接受已完成清理的 terminal manager；`completion_failed` 时幂等重试 Task 5 的 lease cleanup，活动状态返回稳定冲突错误
- [ ] `generateGrid` 纯操作保留既有无 probe 路由
- [ ] 未知 probeID 返回 `400 invalid_probe_id`
- [ ] manager 创建失败返回 `503 manager_creation_failed`
- [ ] 资源冲突返回 `409 resource_conflict`
- [ ] 同 probe 已运行返回 `409 already_running`
- [ ] registry closing 返回 `503 registry_closing`
- [ ] taskID 不匹配返回 `400 task_id_mismatch`
- [ ] 缺失 action / 多余路径段 / 错误 method 返回 `404` / `405`

**Verification:**
- [ ] `go test ./api/... -run TestServer_DualTraversal`
- [ ] `go test ./tests/integration`
- [ ] 测试用例：probe-scoped action 全集 / 未知 probe / 资源冲突 / 单段 legacy 兼容 / generateGrid 兼容

**Dependencies:** Task 6, Task 8, Task 11

**Files likely touched:**
- `api/server.go`（路由解析 + dispatcher）
- `api/server_dual_traversal_test.go`（新建）
- `tests/integration/server_test.go`（追加 dual 集成测试）

**Estimated scope:** M（3 文件，约 380 行）

---

### Task 13: legacy single 路由回归测试 + 错误码区分

**Description:** 在 `api/server_traversal_*_test.go` 既有测试基础上，补充字节级回归测试：legacy `/api/traversal/{action}` 的请求/响应/错误码完全不变。新增错误码白盒测试覆盖 spec FR4 列出的所有错误情况。

**Acceptance criteria:**
- [ ] legacy `/api/traversal/config` GET/POST 字节级回归
- [ ] legacy `/api/traversal/start` / `status` / `pause` / `resume` / `stop` / `result` 回归
- [ ] legacy `/api/traversal/loadCheckpoint` / `resumeFromCheckpoint` / `clearCheckpoint` 回归
- [ ] legacy `/api/traversal/importPrb` / `importCalibrationCsv` / `importMultiPrb` / `importSevenHolePrb` / `importSevenHoleCalibrationCsv` / `clearInterpolator` / `calculateRealtime` / `checkPreconditions` 回归
- [ ] 错误码白盒测试：unknown probe / manager_creation_failed / resource_conflict / already_running / registry_closing / task_id_mismatch / probe_id_mismatch / recoverable_task_exists / checkpoint_version_mismatch

**Verification:**
- [ ] `go test ./api/... -run TestServer_LegacyTraversal`
- [ ] `go test ./api/... -run TestServer_DualTraversal_ErrorCodes`
- [ ] 既有 server_traversal_sevenhole_test.go / server_five_hole_preview_test.go / server_seven_hole_preview_test.go 全部通过

**Dependencies:** Task 12

**Files likely touched:**
- `api/server_traversal_legacy_regression_test.go`（新建）
- `api/server_dual_traversal_error_codes_test.go`（新建）

**Estimated scope:** M（2 文件，约 350 行）

---

### Checkpoint: HTTP API

- [ ] `go test ./api/... ./tests/integration`
- [ ] legacy 路由请求/响应字节级回归
- [ ] 两路并发 start/status/pause/resume/stop/result/checkpoint 不串状态
- [ ] 错误码全覆盖

---

## Phase 6: Composition Root 对齐

### Task 14: 装配根、standalone signal shutdown 与 Wails fatal 路径

**Description:** 修改三个生产装配流，构造统一的 factory/registry、TaskIDGenerator、WorkflowLeasePort 和 ControllerLeasePort。实际 standalone executable 改为 `signal.NotifyContext` + owned `http.Server`，收到信号后先 registry shutdown，再关闭 HTTP/共享服务；Wails `ServiceShutdown` 同样先执行 registry shutdown，并通过可测试的 host exit seam 实现 fatal 非零退出语义。registry shutdown 失败时不得关闭仍可能被 traversal goroutine 使用的 motion/acquisition/storage/log 服务。

**Acceptance criteria:**
- [ ] 三个装配根均构造 `ManagerRegistry` 实例并注入 `api.Deps`
- [ ] 三个装配根注入同语义的服务端 TaskIDGenerator、WorkflowLeasePort 与 token-checked ControllerLeasePort
- [ ] factory 在创建 manager 时为每 probe 新建 `TraversalCsvWriter`、`TraversalResultLog`、checkpoint port factory 实例
- [ ] 共享依赖（AcquisitionHub / MotionAccess / DeviceManager 查询端口 / appConfigStore）通过闭包传入
- [ ] `AppContext.TraversalMgr` 保留作为 legacy single 路径入口（与 registry 并存）
- [ ] Wails `app.go` 的 `ServiceShutdown` 在 relay/motion window/calibration/shared service cleanup 前调用 `registry.Shutdown(ctx)`
- [ ] `cmd/server/main.go` 使用 `signal.NotifyContext` 和显式 `http.Server.Shutdown`，不再裸 `http.ListenAndServe` 至进程被杀
- [ ] `pkg/apiserver.Start` 的 context-owned shutdown 路径使用相同顺序
- [ ] registry shutdown 失败时 standalone 与 Wails 都记录 fatal、走可测试的非零 exit seam，并跳过共享服务 Close
- [ ] 所有有界 EmergencyStop 尝试结束或 hard deadline 到达后才触发 fatal exit
- [ ] graceful/hard deadline 可通过配置文件覆盖（必须为有限正值且 hard > graceful）

**Verification:**
- [ ] `go build -buildvcs=false ./...`
- [ ] `go test ./pkg/appcontext ./internal/bootstrap ./pkg/apiserver`
- [ ] `go test ./cmd/server -run TestShutdown`
- [ ] 手动：Wails 桌面模式 `task run-dev` 启动成功
- [ ] 手动：standalone `go run ./cmd/server` 启动成功
- [ ] Wails binding 方法签名变化时执行 `wails3 generate bindings`

**Dependencies:** Task 7, Task 12

**Files likely touched:**
- `pkg/appcontext/context.go`
- `internal/bootstrap/bootstrap.go`
- `pkg/apiserver/apiserver.go`
- `apps/desktop-wails/backend/app.go`（ServiceShutdown）
- `cmd/server/main.go`（standalone shutdown）

**Estimated scope:** L（6 文件，约 500 行）— 拆分时优先把 factory 实现提取到 `pkg/appcontext/traversal_factory.go` 单独文件以控制单文件行数

---

### Checkpoint: Wiring

- [ ] Wails 桌面模式与 standalone server 均能启动双模式
- [ ] shutdown 在两条退出路径都被调用
- [ ] registry shutdown 失败时共享服务 Close 未被调用且进程结果为非零
- [ ] `go test -race ./...`
- [ ] `validate-structure.ps1` 通过

---

## Phase 7: 前端基础

### Task 15: shared/types/traversal.ts 新增 ProbeId 与 keyed session 类型

**Description:** 在 `apps/desktop-wails/frontend/src/shared/types/traversal.ts` 新增 `ProbeId = 'probe1' | 'probe2'` 类型、`TraversalSessionState` 接口（聚合 config/status/isStarting/error/completeEvent/checkpoint/realtimePressures/realtimeResult/hasLoadedInterpolator/interpolatorRestoreMessage）、`TraversalMode = 'single' | 'dual'` 类型。

**Acceptance criteria:**
- [ ] `ProbeId` 类型导出且严格为 `'probe1' | 'probe2'`
- [ ] `TraversalSessionState` 接口字段与 spec FR5 完全一致
- [ ] `TraversalMode` 类型导出
- [ ] 不修改既有 single 模式使用的类型（TraversalTestConfig / TraversalTestStatus 等）

**Verification:**
- [ ] `npm run typecheck`
- [ ] `npm run test -- --run`（既有 shared/types 测试通过）

**Dependencies:** None（可与后端 Task 并行）

**Files likely touched:**
- `apps/desktop-wails/frontend/src/shared/types/traversal.ts`

**Estimated scope:** S（1 文件，约 80 行）

---

### Task 16: traversalApi 增加 probe-aware 客户端 + keyed polling

**Description:** 在 `apps/desktop-wails/frontend/src/api/traversalApi.ts` 新增 probe-aware 函数：`getConfig(probeId)`、`saveConfig(probeId, config)`、`start(probeId, ...)`、`getStatus(probeId)`、`pause/resume/stop(probeId)`、`result(probeId, taskId)`、`importPrb(probeId, ...)` 等全套。keyed polling：每 probe 一个共享 channel，同时最多一个 in-flight status 请求；首个 subscriber 立即请求一次， thereafter 500ms；最后一个 subscriber 注销停止 timer；使用 AbortController + generation 丢弃旧响应。

**Acceptance criteria:**
- [ ] probe-aware API 函数全套覆盖 spec FR4 路由表（含终态 `close(probeId)`）
- [ ] legacy single API 函数（无 probeId 参数）保留不变
- [ ] 每 probe 同时最多一个 in-flight status 请求
- [ ] 首个 subscriber 立即请求一次，之后 500ms 调度
- [ ] 最后一个 subscriber 注销停止该 probe timer
- [ ] 模式切换 / 新任务启动 / 取消订阅后旧响应被丢弃（generation 校验）
- [ ] 停止一路 polling 不停止另一路

**Verification:**
- [ ] `npm run typecheck`
- [ ] `npm run test -- --run traversalApi`
- [ ] 测试用例：keyed polling in-flight 上限 / 首次立即请求 / 最后订阅注销停止 / generation 丢弃旧响应 / 双 probe 独立 timer

**Dependencies:** Task 15

**Files likely touched:**
- `apps/desktop-wails/frontend/src/api/traversalApi.ts`
- `apps/desktop-wails/frontend/src/api/__tests__/traversalApi.probe.test.ts`（新建）

**Estimated scope:** M（2 文件，约 380 行）

---

### Task 17: dualTraversalStore 实现 keyed session + 隔离的 timer / 订阅 / 实时计算

**Description:** 在 `apps/desktop-wails/frontend/src/stores/dualTraversalStore.ts` 实现 keyed session store：`Record<ProbeId, TraversalSessionState>` + 每 probe 的 requestId / startWindow / subscriptionHandle / realtimeCalcTimer / pendingInput。每个 action 显式接收 `ProbeId`。一路 reset/失败/卸载不修改另一条 session。实时 DAQ 订阅覆盖该 probe 配置中所有唯一 `deviceId`，使用 device-level 引用计数；两路共享设备时一路卸载不取消另一路订阅。

**Acceptance criteria:**
- [ ] `useDualTraversalStore` Pinia store 导出 `sessions: Record<ProbeId, TraversalSessionState>`
- [ ] 每个 action（loadConfig / saveConfig / start / pause / resume / stop / close / loadCheckpoint / etc.）接收 `ProbeId` 首参
- [ ] 一路 reset / 失败 / unmount 不修改另一路 session 状态
- [ ] 实时计算 timer 按 probe ID 独立调度
- [ ] DAQ 订阅按 deviceId 引用计数：两路共享设备时一路卸载不取消另一路订阅
- [ ] 500ms 双路轮询（最多 4 req/s）作为本功能基线
- [ ] 不修改既有 `traversalStore.ts`（single 模式保持兼容）

**Verification:**
- [ ] `npm run typecheck`
- [ ] `npm run test -- --run dualTraversalStore`
- [ ] 测试用例：双路隔离 / timer 独立 / 共享设备引用计数 / 一路失败不影响另一路 / 模式切换清理

**Dependencies:** Task 16

**Files likely touched:**
- `apps/desktop-wails/frontend/src/stores/dualTraversalStore.ts`（新建）
- `apps/desktop-wails/frontend/src/stores/__tests__/dualTraversalStore.test.ts`（新建）

**Estimated scope:** M（2 文件，约 420 行）

---

### Checkpoint: Frontend Store

- [ ] `npm run typecheck` / `npm run test`
- [ ] 一路 reset/unmount/失败不取消另一路订阅或清空状态
- [ ] keyed polling in-flight 上限验证

---

## Phase 8: 前端 UI

### Task 18: TraversalView 模式开关（single 渲染 TraversalMain，dual 渲染 DualTraversalMain）+ 活动态禁用

**Description:** 修改 `apps/desktop-wails/frontend/src/views/TraversalView.vue`：增加 `mode: TraversalMode` 状态与切换 UI；single 渲染既有 `TraversalMain`，dual 渲染 `DualTraversalMain`。任一 session 为 running/moving/stabilizing/acquiring/saving/paused 时模式开关禁用并显示原因；全部 session 为 idle/completed/error/stopped 且清理完成后才允许切换。模式切换不直接删除活动 manager，终态由 registry 清理。

**Acceptance criteria:**
- [ ] `mode` 状态持久化到 localStorage（`WindLabX4.traversal.mode`）
- [ ] single 模式 DOM 结构、布局、交互与现有完全一致（保留 TraversalMain 原行为）
- [ ] dual 模式渲染 `DualTraversalMain`
- [ ] 任一 session 活动时模式开关 disabled + tooltip 显示原因
- [ ] 全部终态且 registry `close` 成功后才切换；不遗留 goroutine / 文件句柄 / 锁 / 轮询 timer
- [ ] 切换模式时取消旧模式的所有订阅与 timer

**Verification:**
- [ ] `npm run typecheck`
- [ ] `npm run build`
- [ ] 手动：single ↔ dual 切换在 idle/running/paused/terminal 各状态下行为正确

**Dependencies:** Task 17

**Files likely touched:**
- `apps/desktop-wails/frontend/src/views/TraversalView.vue`
- `apps/desktop-wails/frontend/src/components/traversal/dual/DualTraversalMain.vue`（占位骨架，Task 19 填充）

**Estimated scope:** S（2 文件，约 180 行）

---

### Task 19: DualTraversalMain + DualStatusBar（双列摘要）

**Description:** 实现 `DualTraversalMain.vue`（容器：模式开关已由 TraversalView 持有，本组件负责 dual 模式的整体布局）与 `DualStatusBar.vue`（顶部双列摘要：两 probe 的状态/进度/当前点位/Alpha·Beta/总压/静压/速度/Warning 摘要）。布局遵循 `DESIGN.md` 固定桌面布局，无移动端 breakpoint。

**Acceptance criteria:**
- [ ] `DualTraversalMain` 上下排列两个 `DualProbeRow`
- [ ] 顶部 `DualStatusBar` 双列摘要，每列对应一个 probe
- [ ] 摘要包含：状态、进度、当前点位、Alpha/Beta、总压、静压、速度、Warning/Error 摘要
- [ ] 不使用既有 `TraversalLiveMonitor` 简单缩到 240px
- [ ] 1440x900 与 1600x900 下双列不重叠、不溢出
- [ ] light/dark 主题切换正常

**Verification:**
- [ ] `npm run typecheck`
- [ ] `npm run build`
- [ ] 手动：1440x900 / 1600x900 / light / dark 截图验收

**Dependencies:** Task 18

**Files likely touched:**
- `apps/desktop-wails/frontend/src/components/traversal/dual/DualTraversalMain.vue`
- `apps/desktop-wails/frontend/src/components/traversal/dual/DualStatusBar.vue`

**Estimated scope:** M（2 文件，约 320 行）

---

### Task 20: DualProbeRow + DualProbeCompactMonitor（紧凑监测 + Tab 详情）

**Description:** 实现 `DualProbeRow.vue`（单 probe 完整 row 容器：紧凑监测 + Tab 详情区）与 `DualProbeCompactMonitor.vue`（仅保留状态/进度/当前点位/Alpha·Beta/总压/静压/速度/Warning/Error 摘要/独立启动·暂停·恢复·停止·设置入口）。完整通道值、运动状态、点位预览、诊断信息放在该 row 的 Tab 详情区域。每个 row 详情区可独立滚动；控制栏与 Warning 不得被滚动隐藏。

**Acceptance criteria:**
- [ ] `DualProbeRow` 接收 `probeId` props，从 `dualTraversalStore` 读取该 probe 的 session
- [ ] `DualProbeCompactMonitor` 仅展示 spec FR7 列出的紧凑字段
- [ ] 独立启动 / 暂停 / 恢复 / 停止 / 设置入口按钮，按该 probe 状态启用/禁用
- [ ] Tab 详情区可独立滚动，控制栏与 Warning 固定不滚动
- [ ] 长错误文本 / 双 Warning 下关键控制不被遮挡
- [ ] 五孔 / 七孔探针类型组合下展示字段正确（Alpha/Beta 标签按 `TRAVERSAL_PROBE_PRESENTATION` 切换）

**Verification:**
- [ ] `npm run typecheck`
- [ ] `npm run build`
- [ ] 手动：五孔+五孔 / 五孔+七孔 / 七孔+七孔组合下布局正确
- [ ] 手动：长错误文本 + 双 Warning 截图验收

**Dependencies:** Task 19

**Files likely touched:**
- `apps/desktop-wails/frontend/src/components/traversal/dual/DualProbeRow.vue`
- `apps/desktop-wails/frontend/src/components/traversal/dual/DualProbeCompactMonitor.vue`

**Estimated scope:** M（2 文件，约 380 行）

---

### Task 21: DualTraversalSettings（每 probe 独立配置入口）

**Description:** 实现 `DualTraversalSettings.vue`：双探针模式下的配置对话框，每 probe 独立配置型号、PRB/校准 CSV、通道、点位表、大气压、温度、运动控制器、输出路径。复用既有 `TraversalSettings` 的步骤结构（通道/PRB/布点/摘要），但通过 probeId 切换数据源。启动前原子校验两个 controller ID 非空且不同。

**Acceptance criteria:**
- [ ] 对话框顶部 tab 切换 probe1 / probe2
- [ ] 每 probe 独立保存到 `traversal.probe1` / `traversal.probe2` 配置 key
- [ ] 保存前原子校验 controller ID 非空且 probe1 ≠ probe2，冲突时禁止保存并显示原因
- [ ] PRB / CSV / 七孔 PRB / 七孔 CSV 导入按 probe 路由
- [ ] 复用既有 `FiveHolePrbConfig` / `SevenHolePrbConfig` / `CustomPointsTable` / `PointsPreview` / `TraversalHardwareStep` / `TraversalLayoutStep` / `TraversalPrbStep` 组件
- [ ] 不修改既有 `TraversalSettings.vue`（single 模式保持兼容）

**Verification:**
- [ ] `npm run typecheck`
- [ ] `npm run build`
- [ ] 手动：双 probe 独立配置 + 相同 controller 拒绝保存

**Dependencies:** Task 17, Task 20

**Files likely touched:**
- `apps/desktop-wails/frontend/src/components/traversal/dual/DualTraversalSettings.vue`

**Estimated scope:** M（1 文件，约 380 行）

---

### Checkpoint: Frontend UI

- [ ] 1440x900 / 1600x900、light/dark、五孔/七孔组合、双 Warning 下关键控制不被遮挡
- [ ] single 模式既有 TraversalMain 行为不变
- [ ] `npm run typecheck` / `npm run build`
- [ ] `validate-frontend-structure.ps1 -CheckFileSize` 通过

---

## Phase 9: 集成与验收

### Task 22: 后端并发与隔离全量测试

**Description:** 补全 spec Testing Strategy 后端单元与并发 1-16 项的全部测试用例：双 manager 并行启动 / 资源原子预占 / 重复启动拒绝 / task ID 不冲突 / writer 实例独立 / 一路故障不影响另一路 / CloseProbe 四态 / shutdown 聚合错误 / probe-scoped 配置独立恢复 / checkpoint 隔离 / 全局 lease 引用计数 / 每 probe 唯一恢复候选 / shutdown deadline / 重复完成幂等 / resume 资源准入 / dual index 独立更新。

**Acceptance criteria:**
- [ ] spec Testing Strategy 后端 1-16 项全部覆盖
- [ ] 并发测试使用 channel/barrier + 可控 fake，不依赖 `time.Sleep`
- [ ] `go test -race ./internal/usecase ./internal/adapters/storage` 通过
- [ ] 测试用例命名规范：`TestRegistry_<Scenario>_<Expected>` / `TestManager_<Scenario>_<Expected>`

**Verification:**
- [ ] `go test -race ./internal/usecase ./internal/adapters/storage`
- [ ] `go test -count=10 -race ./internal/usecase -run TestRegistry`（race 稳定性）

**Dependencies:** Task 7, Task 9, Task 11

**Files likely touched:**
- `internal/usecase/traversal_manager_registry_test.go`
- `internal/usecase/traversal_lifecycle_test.go`
- `internal/usecase/traversal_v2_integration_test.go`
- `internal/adapters/storage/dual_traversal_recovery_index_test.go`

**Estimated scope:** M（3-4 文件，约 450 行）

---

### Task 23: HTTP 集成测试（probe-scoped action 全集 + 两路并发 + 错误码）

**Description:** 在 `api/` 与 `tests/integration/` 补全 spec HTTP 集成测试 1-5 项：probe-scoped action 全集选择正确 manager / 未知 probe·缺失 action·多余路径段·错误 method 返回稳定错误 / legacy single 路由回归 / 两路并发 start·status·pause·resume·stop·result·checkpoint 不串状态 / `go test ./...` 包含 `tests/integration`。

**Acceptance criteria:**
- [ ] spec HTTP 集成 1-5 项全部覆盖
- [ ] probe-scoped action 全集（config/importPrb/importCalibrationCsv/importMultiPrb/importSevenHolePrb/importSevenHoleCalibrationCsv/clearInterpolator/calculateRealtime/checkPreconditions/start/status/runPoint/pause/resume/stop/close/result/loadCheckpoint/resumeFromCheckpoint/clearCheckpoint）逐一测试
- [ ] 两路并发场景使用 fake controller + fake DAQ，断言状态隔离
- [ ] `go test ./...` 包含 `tests/integration`

**Verification:**
- [ ] `go test ./api/... ./tests/integration`
- [ ] `go test -race ./api/... ./tests/integration`

**Dependencies:** Task 13, Task 14

**Files likely touched:**
- `api/server_dual_traversal_test.go`
- `tests/integration/server_test.go`

**Estimated scope:** M（2 文件，约 380 行）

---

### Task 24: 前端隔离测试（keyed polling / 多设备订阅 / 模式开关门禁）

**Description:** 补全 spec 前端测试 1-7 项：两 session 完全隔离 / 一路 reset·unmount·失败不取消另一条订阅 / keyed polling in-flight 上限 + 丢弃旧响应 / 多设备订阅与共享设备引用计数 / active/paused 模式开关禁用 + 终态清理后可切换 / single 模式继续渲染 TraversalMain（既有测试不修改期望）/ 1440x900·1600x900·light·dark 截图验收。

**Acceptance criteria:**
- [ ] spec 前端 1-7 项全部覆盖
- [ ] Vitest 测试使用 fake fetch / fake wails adapter，不依赖真实后端
- [ ] 截图验收由人工执行并记录结果（Markdown 报告附 PNG）
- [ ] 既有 `traversalStore` / `TraversalMain` 测试不修改期望

**Verification:**
- [ ] `npm run typecheck`
- [ ] `npm run test -- --run`
- [ ] 截图报告：1440x900 / 1600x900 / light / dark / 五孔+五孔 / 五孔+七孔 / 七孔+七孔 / 长错误 / 双 Warning

**Dependencies:** Task 21

**Files likely touched:**
- `apps/desktop-wails/frontend/src/stores/__tests__/dualTraversalStore.test.ts`
- `apps/desktop-wails/frontend/src/api/__tests__/traversalApi.probe.test.ts`
- `apps/desktop-wails/frontend/src/components/traversal/dual/__tests__/DualProbeRow.test.ts`

**Estimated scope:** M（3 文件，约 380 行）

---

### Task 25: 视觉与人工验收

**Description:** 在 1440x900 与 1600x900 分辨率、light/dark 主题、五孔/七孔探针组合、长错误文本、双 Warning 场景下执行人工验收，记录截图与问题清单。

**Acceptance criteria:**
- [ ] 1440x900 light/dark 双探针布局截图
- [ ] 1600x900 light/dark 双探针布局截图
- [ ] 五孔+五孔 / 五孔+七孔 / 七孔+七孔 组合截图
- [ ] 长错误文本下控制栏不被遮挡截图
- [ ] 双 Warning 下控制栏不被遮挡截图
- [ ] 模式开关在 running/paused/terminal 各状态下的禁用/启用行为截图

**Verification:**
- [ ] 人工验收报告（Markdown + PNG 附件）
- [ ] 所有 P0 视觉问题已修复

**Dependencies:** Task 21, Task 24

**Files likely touched:**
- `docs/acceptance/dual-traversal-visual-acceptance.md`（新建，仅人工验收报告）

**Estimated scope:** S（1 文件，验收报告）

---

### Task 26: 双控制器 HIL 验证

**Description:** 使用两套真实运动控制器执行 spec HIL 验证：同时运动+同时采样+独立完成 / 停止 probe1 时 probe2 控制器不接收 Stop / probe1 普通故障时 probe2 继续 / probe1 位置超差·限位·掉线·EmergencyStop 时仅 probe1 控制器收到对应停机命令 / 应用退出 hard deadline 路径分别尝试 EmergencyStop 两个仍活动控制器 / 输出文件·checkpoint·日志可按 probe/task 追溯。

**Acceptance criteria:**
- [ ] spec HIL 全部场景执行并记录
- [ ] probe1 Stop 期间 probe2 控制器日志无 Stop 命令记录
- [ ] probe1 故障时 probe2 继续运行至完成
- [ ] 应用退出时两控制器均收到 EmergencyStop（按 hard deadline）
- [ ] 输出文件按 probe 前缀可追溯
- [ ] checkpoint 与日志按 probe/task 可追溯

**Verification:**
- [ ] HIL 验收报告（Markdown + 日志附件）
- [ ] 所有 P0 安全问题已修复

**Dependencies:** Task 22, Task 23, Task 24, Task 25

**Files likely touched:**
- `docs/acceptance/dual-traversal-hil-report.md`（新建，仅验收报告）

**Estimated scope:** S（1 文件，验收报告）

---

### Checkpoint: Complete

- [ ] spec Success Criteria 1-17 全部满足
- [ ] `gofmt -l .` 无输出
- [ ] `go build -buildvcs=false ./...`
- [ ] `go vet ./...`
- [ ] `go test ./...`
- [ ] `go test -race ./internal/usecase ./internal/adapters/storage`
- [ ] `npm run typecheck` / `npm run test` / `npm run build`
- [ ] `powershell -ExecutionPolicy Bypass -File scripts\validate-structure.ps1`
- [ ] `powershell -ExecutionPolicy Bypass -File scripts\validate-frontend-structure.ps1 -CheckFileSize`
- [ ] GitNexus `gitnexus_detect_changes()` 确认变更范围与预期一致
- [ ] Wails binding 方法签名变化时已执行 `wails3 generate bindings`

---

## 总览统计

| Phase | Task 数 | 总预估 scope |
|---|---|---|
| 1. Foundation | 2 | 1S + 1M |
| 2. Registry | 5 | 1S + 4M |
| 3. Manager Refactor | 2 | 2M |
| 4. Checkpoint v3 | 2 | 2M |
| 5. HTTP API | 2 | 2M |
| 6. Composition Root | 1 | 1L |
| 7. Frontend Foundation | 3 | 1S + 2M |
| 8. Frontend UI | 4 | 1S + 3M |
| 9. Integration & Acceptance | 5 | 3M + 2S |
| **Total** | **26** | 6S + 19M + 1L |

> L 任务（Task 14）需在实施时拆分为子任务以控制单文件 ≤ 500 行约束。
