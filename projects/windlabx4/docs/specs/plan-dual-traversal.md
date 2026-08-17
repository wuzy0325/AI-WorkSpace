# Implementation Plan: WindLabX4 双探针并行遍历测试

> 关联规格：[dual-traversal-spec.md](./dual-traversal-spec.md)
> 计划版本：v2（基于 spec v3）
> 日期：2026-07-27

## Overview

在 WindLabX4 现有单探针遍历测试基础上，新增双探针并行模式：两支物理独立的探针在同一次吹风周期内并行运行，分别使用自己的运动控制器、PRB/CSV、通道绑定、点位表、CSV/结果日志/checkpoint。所有有状态资源（manager、运行 session、CSV writer、结果日志、checkpoint、插值器、轮询订阅）按 probe ID 隔离；只读共享 DAQ latest-data bus、MotionAccess、DeviceManager 查询端口。双模式整体仍持有一份全局 `workflow:traversal` lease，由 registry 在第一个 probe 启动时获取、最后一个 probe 清理后释放。

不改变既有 single 模式的 UI、HTTP 契约、配置 key 与 checkpoint v1/v2 行为。

## Architecture Decisions

| # | 决策 | 理由 |
|---|---|---|
| A1 | 引入 `usecase.ManagerRegistry` 作为双模式准入与生命周期编排唯一入口 | spec I2 要求 registry 持有全局 workflow lease、维护 session token 与活动计数；manager 不能各自争抢全局锁 |
| A2 | `TraversalManagerFactory` 接口定义在 usecase，实现在 composition root | spec FR3 明确 registry 不导入 storage/hardware adapter；保留六边形架构边界 |
| A3 | `TraversalManager` 保留 legacy `Start/Resume` 的直接 lease 行为；新增仅供 registry 调用的 managed Start/Resume 路径 | single 兼容不变；managed 路径跳过 manager 的全局 lease Acquire/Release，只携带 token 回报完成，避免双重所有权 |
| A4 | 现有可靠性 checkpoint v2 保持不变；双模式写 v3，在现有 `Checkpoint`/`TraversalRunSnapshot` 完整字段上增加 `ProbeID` 与 `BoundControllerIDs` | 当前代码已使用可靠性 v2；双模式不能复用版本号或退回旧 `Config/Points/LastIndex` 模型 |
| A5 | 新增 `dual-traversal-recovery-index.json`（envelope `version:1`，`probeId→taskId→checkpointPath`）；legacy `traversal-active-index.json` 保持原格式 | spec FR4 要求每 probe 最多一个权威恢复候选，两文件互不读写、互不迁移 |
| A6 | HTTP 路由扩展为两段式 `/api/traversal/{probeId}/{action}`，与既有 `/api/traversal/{action}` 共存 | spec FR4 要求 legacy 路由语义不变，禁止隐式转发到 probe1 |
| A7 | 前端新增 `dualTraversalStore`（keyed `Record<ProbeId, TraversalSessionState>`），保留 `traversalStore` 兼容 | spec FR5 拒绝在单 store 上添加 `dualConfig`/`dualStatus` 平行字段 |
| A8 | shutdown 采用 graceful 5s + hard 10s 双 deadline，到期对活动控制器并发 EmergencyStop，使用从剩余 hard deadline 派生的 context | spec FR9 / I6 要求单 adapter 卡住不延长总 deadline |
| A9 | registry 对外提供 Start/Resume/Stop/Close façade；资源准入事务 = workflow lease + token-checked controller lease + session 登记，任一步失败带回滚 | HTTP 不得通过 `GetOrCreate` 绕过准入；专用 lease ports 提供 Acquire/Renew/Release，最后一路释放使用 transition gate 与新 admission 互斥 |
| A10 | dual task ID 由服务端 `TaskIDGenerator` 生成（probe namespace + UUID/等价随机 ID），客户端值不作为权威 ID | 保证跨 probe、结果 store、索引和 checkpoint 的持久化范围唯一；legacy single 契约不变 |
| A11 | 三个 composition root（appcontext / bootstrap / apiserver）统一使用同一 factory/registry 语义；`cmd/server` 显式拥有 signal shutdown | spec FR3/FR9 要求所有生产装配流和实际 standalone 入口都执行有序 shutdown |

## Task List

### Phase 1: Foundation（类型与契约）

- [ ] Task 1: 新增 registry 契约与 token-checked controller lease adapter
- [ ] Task 2: 新增 dual recovery index adapter（probeId→taskId→checkpointPath）

### Checkpoint: Foundation
- [ ] `go build ./...` 通过，类型与接口可被引用
- [ ] 单元测试覆盖 dual recovery index 的注册/查找/注销/原子替换
- [ ] controller lease adapter 的 Acquire/Renew/token-checked Release race 测试通过

### Phase 2: ManagerRegistry（准入与生命周期）

- [ ] Task 3: ManagerRegistry 核心结构 + GetOrCreate + 探针 ID 校验
- [ ] Task 4: registry Start façade + 原子准入事务（全局 lease + controller lease + task ID + token + 回滚）
- [ ] Task 5: session token / generation 的 exactly-once 完成与 lease 续约/释放路径
- [ ] Task 6: Stop / CloseProbe 生命周期（cancel → wait Done → completion linearization → delete；超时保留）
- [ ] Task 7: Shutdown 双 deadline + 并发 EmergencyStop + 聚合错误

### Checkpoint: Registry
- [ ] `go test -race ./internal/usecase` 通过
- [ ] 并发启动两 probe 不出现双重占用或 lease 提前释放
- [ ] shutdown 在 hard deadline 内返回，未退出 probe/task 可诊断

### Phase 3: TraversalManager 重构

- [ ] Task 8: TraversalManager 增加 managed Start/Resume 路径与 probe-scoped config key，保留 legacy lease 行为
- [ ] Task 9: Stop / EmergencyStop 限制在启动快照中的控制器，不跨 probe 联停

### Checkpoint: Manager Refactor
- [ ] 既有 single 路径全部测试通过（无回归）
- [ ] managed manager 完成只通过 registry 回报 token，不直接释放 workflow/controller lease；legacy single 行为不变

### Phase 4: Checkpoint v3 与恢复

- [ ] Task 10: checkpoint v3 metadata（含 ProbeID/taskID/controller 绑定）+ legacy v1/v2 与 dual v3 路由分离
- [ ] Task 11: probe-scoped resume / clear / loadCheckpoint（校验 taskID 一致 + 控制器冲突拒绝）

### Checkpoint: Recovery
- [ ] legacy v1/v2 checkpoint 经 legacy 路由保持原行为，dual v3 不被 legacy 误加载
- [ ] resume 在打开 append 文件或运动前重新走准入事务；冲突时 checkpoint 与输出文件保持不变

### Phase 5: HTTP API

- [ ] Task 12: probe-scoped 路由解析 `{probeId}/{action}` 与 registry façade dispatcher（含终态 close）
- [ ] Task 13: legacy single 路由回归测试 + 错误码区分（unknown probe / resource_conflict / recoverable_task_exists / registry_closing）

### Checkpoint: HTTP API
- [ ] `go test ./api/... ./tests/integration` 通过
- [ ] legacy 路由请求/响应字节级回归
- [ ] 两路并发 start/status/pause/resume/stop/result/checkpoint 不串状态

### Phase 6: Composition Root 对齐

- [ ] Task 14: 三个装配根统一 factory + registry，并补齐 `cmd/server` signal shutdown 与 Wails fatal 路径

### Checkpoint: Wiring
- [ ] Wails 桌面模式与 standalone server 均能启动双模式
- [ ] shutdown 在两条退出路径都被调用

### Phase 7: 前端基础（types / api / store）

- [ ] Task 15: shared/types/traversal.ts 新增 ProbeId 与 keyed session 类型
- [ ] Task 16: traversalApi 增加 probe-aware 客户端 + keyed polling（每 probe single-flight）
- [ ] Task 17: dualTraversalStore 实现 keyed session + 隔离的 timer / 订阅 / 实时计算

### Checkpoint: Frontend Store
- [ ] `npm run typecheck` / `npm run test` 通过
- [ ] 一路 reset/unmount/失败不取消另一路订阅或清空状态

### Phase 8: 前端 UI

- [ ] Task 18: TraversalView 模式开关（single 渲染 TraversalMain，dual 渲染 DualTraversalMain）+ 活动态禁用
- [ ] Task 19: DualTraversalMain + DualStatusBar（双列摘要）
- [ ] Task 20: DualProbeRow + DualProbeCompactMonitor（紧凑监测 + Tab 详情）
- [ ] Task 21: DualTraversalSettings（每 probe 独立配置入口）

### Checkpoint: Frontend UI
- [ ] 1440x900 / 1600x900、light/dark、五孔/七孔组合、双 Warning 下关键控制不被遮挡
- [ ] single 模式既有 TraversalMain 行为不变

### Phase 9: 集成与验收

- [ ] Task 22: 后端并发与隔离全量测试（race + 资源独占 + writer 隔离 + lease 引用计数）
- [ ] Task 23: HTTP 集成测试（probe-scoped action 全集 + 两路并发 + 错误码）
- [ ] Task 24: 前端隔离测试（keyed polling / 多设备订阅 / 模式开关门禁）
- [ ] Task 25: 视觉与人工验收（分辨率/主题/探针类型组合）
- [ ] Task 26: 双控制器 HIL 验证（独立完成 / 跨 probe 不联停 / 应用退出分别 EmergencyStop）

### Checkpoint: Complete
- [ ] spec Success Criteria 1-17 全部满足
- [ ] `gofmt` / `go build` / `go vet` / `go test` / `go test -race` / `npm typecheck` / `npm test` / `npm build` / `validate-structure.ps1` / `validate-frontend-structure.ps1 -CheckFileSize` 全绿
- [ ] GitNexus `gitnexus_detect_changes()` 确认变更范围与预期一致

## Risks and Mitigations

| Risk | Impact | Mitigation |
|---|---|---|
| TraversalManager 重构破坏既有 single 路径 | High | Task 8 保持 legacy 构造与直接 Start/Resume lease 路径；managed 入口单独实现；既有 traversal_*_test.go 不修改期望 |
| registry shutdown 卡死导致进程无法退出 | High | Task 7 使用 hard deadline + per-controller EmergencyStop 各自独立 context；测试覆盖单 adapter 卡住场景 |
| 全局 workflow lease 引用计数错乱（提前释放或阻止第二路） | High | Task 4 准入事务原子提交；Task 5 token/generation 保证 exactly-once；race 测试覆盖第一路/最后一路边界 |
| HTTP 两段式路由与 legacy `/api/traversal/{action}` 冲突 | Med | Task 12 严格区分单段 vs 两段；Task 13 字节级回归测试 |
| dual recovery index 与 legacy active index 互相污染 | Med | Task 2 完全独立文件、独立原子替换；Task 11 dual 路径不读取 legacy index |
| 前端 keyed polling 退化为 N 倍请求风暴 | Med | Task 16 每 probe single-flight + 共享 channel；测试断言 in-flight 上限 |
| DualProbeRow 在 1440x900 长错误文本下遮挡控制 | Med | Task 20 控制栏与 Warning 不随详情区滚动；Task 25 多分辨率截图验收 |
| 七孔插值器跨 probe 共享实例导致状态污染 | High | Task 8 强制每 manager 新建 five/seven hole interpolator；Task 22 race 测试覆盖并发计算 |
| Wails binding 方法签名变更影响发布 | Med | 若 manager 公共方法签名变化，Task 14 后立即 `wails3 generate bindings` 并验证 |
| 将现有 checkpoint v2 误当 legacy v1 导致恢复数据丢失 | High | Task 10 以当前 `Checkpoint` + `TraversalRunSnapshot` 为基线新增 v3 元数据；禁止删除 commitSeq/hash/真实输出路径字段 |
| HTTP 直接获取 manager 绕过准入事务 | High | Task 12 的所有生命周期 action 只调用 registry façade；API 测试断言文件/运动 I/O 前已完成 admission |
| controller lease TTL 到期后被其它任务接管 | High | Task 1 定义 token-checked Acquire/Renew/Release；Task 5 负责续约生命周期与续约失败处置 |

## Open Questions

无。spec v3 已明确所有 Core Invariants、Functional Requirements 与 Confirmed Decisions。若实施过程中发现 spec 描述与现有代码契约冲突，必须先回到 spec 阶段更新 dual-traversal-spec.md，再调整本计划。

## Parallelization Opportunities

| 可并行 | 必须串行 | 需协调 |
|---|---|---|
| Task 2（dual index adapter）与 Task 1（契约） | Task 3-7（registry 内部强依赖） | Task 12（HTTP 路由）与 Task 14（装配根）共享 registry façade 契约 |
| Task 15-17（前端基础）与 Task 8-11（manager 重构） | Task 8 → Task 9（manager 内部） | Task 18-21（UI）依赖 Task 17 store 契约稳定 |
| Task 22-24（测试）在对应实现 Task 完成后可并行 | Task 26（HIL）必须在所有功能 Task 完成后 | Task 25（视觉）需 Task 18-21 全部就绪 |

## Verification Strategy

每个 Task 必须满足：

1. **Acceptance Criteria** 全部勾选
2. **Verification** 命令实际执行通过（不能只写"应该通过"）
3. 修改的生产 symbol 在编辑前已执行 `gitnexus_impact({target, direction:"upstream"})`，HIGH/CRITICAL 风险已告警
4. 提交前执行 `gitnexus_detect_changes()` 确认范围
5. 后端文件 ≤ 500 行、函数 ≤ 50 行（`validate-structure.ps1`）
6. 前端文件大小符合 `validate-frontend-structure.ps1 -CheckFileSize`

## See Also

- [spec-traversal-reliability-and-recovery.md](./spec-traversal-reliability-and-recovery.md) — 停止/提交/恢复不变量
- [spec-traversal-motion-safety.md](./spec-traversal-motion-safety.md) — 运动安全策略
- [AGENTS.md](../../../../AGENTS.md) — workspace 启动规则与硬约束
- [CLAUDE.md](../../../../CLAUDE.md) — workspace 架构与硬约束
- [projects/windlabx4/CLAUDE.md](../../CLAUDE.md) — WindLabX4 项目级边界
