# WorkflowCoordinator 实施计划

> **基于**: 2026-06-09 架构 Review + 领域建模访谈
> **目标**: 消除 apiServer.sessionToken、handler 直接调用裸状态机、各服务独立持有 sessionMachine

## 当前问题（不改动点）

| 位置 | 代码 | 问题 |
|------|------|------|
| `device_handler.go:48` | `sessionToken session.BindingToken` | 全局字段，最后绑定者覆盖 |
| `session_handler.go:16` | `s.sessionMachine.State()` | handler 直接读裸状态机 |
| `session_handler.go:57` | `s.sessionMachine.ForceStop()` | handler 直接操作状态机 |
| `calibration/service.go:37` | `sessionMachine *workflow.SessionMachine` | 服务持有与 handler 同实例的指针，状态机没有单一所有节点 |
| `measurement/service.go:37` | `sessionMachine *workflow.SessionMachine` | 同上 |

## 新增文件

```
internal/workflow/
├── coordinator.go    # WorkflowCoordinator（新增，~200 行）
├── session_machine.go  # 保持不变
├── calibration_state.go  # 窄接口，供 coordinator 内部使用
├── measurement_state.go  # 窄接口
```

## 执行状态

| 阶段 | 状态 | 说明 |
|------|------|------|
| **阶段 1** — 创建 Coordinator | ✅ 完成 | `coordinator.go` + `coordinator_test.go`（15 用例） |
| **阶段 2** — Handler 接入 | ✅ 完成 | `assembly.go` 创建 coordinator，`router.go` 使用依赖注入，`session_handler.go` 用 `coordinator.State()/End()` 替代裸 `sessionMachine`，`device_session_handler.go` 移除全局 `sessionToken` |
| **阶段 3** — Service 迁移 | ✅ 完成 | calibration/measurement service 改用 coordinator 进行 Begin/End，由 `coordinator.Machine()` 支持细粒度过渡 |

## 已完成改动摘要

| 位置 | 改前 | 改后 |
|------|------|------|
| `device_handler.go:37` | `sessionMachine *workflow.SessionMachine` | `coordinator *workflow.WorkflowCoordinator` |
| `device_handler.go:48` | `sessionToken session.BindingToken` | 移除，改用 `sessionService.Token()` |
| `session_handler.go` | 6 处 `s.sessionMachine.State()` | `s.coordinator.State()` |
| `session_handler.go:57` | `s.sessionMachine.ForceStop()` | `s.coordinator.End()` |
| `device_session_handler.go` | 绑定后存 `s.sessionToken = token` | 移除全局存储，调用时通过 `s.currentToken()` 从 sessionService 获取 |
| `assembly.go` | `NewSessionMachine()` → `Dependencies.SessionMachine` | `NewWorkflowCoordinator()` → `Dependencies.WorkflowCoordinator` |
| `router.go` | `deps.SessionMachine` | `deps.WorkflowCoordinator` |

## 新增与改动的领域模型

| 文件 | 动作 |
|------|------|
| `internal/errors/codes.go` | +4 错误常量 |
| `internal/api/http/error_mapper.go` | +4 HTTP 映射（含 423 Locked） |
| `internal/domain/session_state.go` | +`SessionStateRequiresManualIntervention` |
| `internal/workflow/session_machine.go` | +`error→requires_manual_intervention`、`error→ready` |
| `internal/workflow/coordinator.go` | 新增 WorkflowCoordinator |
| `internal/workflow/coordinator_test.go` | 15 个测试用例 |

### 阶段 1：创建 Coordinator（不改变现有调用链）

新增 `internal/workflow/coordinator.go`：

```go
type WorkflowOwner string

const (
    OwnerCalibration WorkflowOwner = "calibration"
    OwnerMeasurement WorkflowOwner = "measurement"
)

type WorkflowCoordinator struct {
    mu       sync.Mutex
    machine  *SessionMachine
    owner    WorkflowOwner
    ctxID    string
}

func NewWorkflowCoordinator() *WorkflowCoordinator

func (c *WorkflowCoordinator) Owner() WorkflowOwner
func (c *WorkflowCoordinator) State() domain.SessionState
func (c *WorkflowCoordinator) HasActiveWorkflow() bool

func (c *WorkflowCoordinator) Begin(owner WorkflowOwner) error
func (c *WorkflowCoordinator) End()
func (c *WorkflowCoordinator) Fail(cause error) error
func (c *WorkflowCoordinator) ConfirmManualIntervention() error
```

`Begin` 校验冲突逻辑：
- 当前有活跃工作流且 owner != current → `ErrWorkflowConflict`
- 当前有活跃工作流且 owner == current → 幂等成功
- 无活跃工作流 → 设置 owner，进入 ready/collecting

**安全约束**：阶段 1 创建 coordinator，但不替换现有调用链中的 sessionMachine 引用。所有 handler 和服务继续使用原有指针。

**测试**：`coordinator_test.go` 覆盖 Begin/End/Fail/Confirm/冲突拒绝。

### 阶段 2：Coordinator 接管 router 层

- `assembly.go`：创建 coordinator 代替裸 `workflow.NewSessionMachine()`
- `device_handler.go`：`apiServer` 用 coordinator 替换 `sessionMachine` 和 `sessionToken`
- `session_handler.go`：所有 `s.sessionMachine.State()` → `s.coordinator.State()`
- `session_handler.go:57`：`ForceStop` → `coordinator.End()`
- `router.go`：coordinator 传递给 handler 和服务

### 阶段 3：Application Service 迁移

- `calibration.Service` 和 `measurement.Service` 持 `*workflow.WorkflowCoordinator` 而非 `*workflow.SessionMachine`
- 业务动作调用 coordinator 方法，不直接调用 `Transition`
- 采集循环使用 coordinator 允许的状态检查

## 风险

- **阶段 1 到阶段 2 的桥接期**：coordinator 创建了但 handler 仍用 sessionMachine → 短期存在两个入口，但只读不写，不产生冲突
- **阶段 2 到阶段 3 的 API 适配**：`calibration.Service` 和 `measurement.Service` 的方法签名需要调整，涉及测试更新

## 测试策略

- 阶段 1：单元测试覆盖 coordinator 核心路径
- 阶段 2：现有 handler 测试全绿（因只改了 coordinator 接管）
- 阶段 3：service 测试适配 coordinator 接口，原有行为不改变

## 预估工作量

- 阶段 1：1 人日（创建 coordinator + 单元测试）
- 阶段 2：1 人日（改 router/handler + 适配测试）
- 阶段 3：2 人日（迁移两个 service + 测试适配）
- 总计：4 人日
