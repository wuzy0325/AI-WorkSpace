# Implementation Plan: 位移机构统一状态监视与订阅

> 关联规格：[spec-motion-status-monitor.md](./spec-motion-status-monitor.md)
> 状态：Phase 2 计划，待人工批准后进入 BUILD
> 日期：2026-07-16

## Overview

在 `shared/motion-control/go/` 新建 `monitor/` 包，提供 `MotionStatusMonitor`：每台已连接控制器唯一轮询、不可变快照、序号/Generation/ValidUntil 元数据、FreshnessPolicy 调用瞬间计算、WaitNext 含 ErrGenerationChanged 语义、NotifyCommandExecuted 区分 Move/Stop/Config 三类触发。B140 与 WTNMC4A 驱动各自新增 Connection Priority Coordinator（六条不变量），MotionManager 持有 monitor 并负责生命周期、原子 ownership 切换、急停不可信协议。WindLabX4 通过 `MotionStatusReader` 端口投影 shared 快照（保留 Generation 与 Freshness），校准/遍历迁移到 WaitNext + JudgeArrival 四层门禁，前端删除 3 类校准 Main 的 300ms timer 并实现独立窗口失联降级。

## Architecture Decisions

| 决策 | 理由 |
|---|---|
| monitor 位于 shared 层，MotionManager 持有 | 前端关闭后校准/遍历仍需可靠状态；前端不能成为唯一采集源（Decision 1） |
| FreshnessPolicy 接口 + ValidUntil 字段，不固化静态 IsStale | Age 是时间敏感值，发布时固化会立即失真（评审 P1-3） |
| Generation 透传到项目端口，重连后 Sequence 重置为 0 | 避免重连后等待语义不完整（评审遗漏项） |
| Connection Priority Coordinator 只规定行为不变量，不固定接口形态 | Phase 1 不固化 PriorityLatch Go 接口；Phase 2 候选实现对比后定（评审 P0-3 修正） |
| 急停不可信协议由 MotionManager 实现，UI 通过事件 + HTTP 端点订阅 | 命令冻结需要后端强制；告警锁存需要前端状态机 |
| 独立窗口失联检测在前端实现，HTTP 503 作为主进程故障信号 | 失联是网络层问题，前端最易感知 |
| motion-controller 项目几乎无独立代码（仅类型别名），验证而非迁移 | 勘察确认复用 shared 包（[usecase/motion.go](file:///c:/Users/wuzhy/Documents/D/SVN/SoftWare/trunk/AI-Workspace/projects/motion-controller/services/api-go/internal/usecase/motion.go)） |
| 沿用项目约定 `plan-*.md` / `tasks-*.md` 与 spec 同目录 | 匹配既有 8 份 plan / 3 份 tasks 文件命名 |

## Assumptions (待 Phase 3 开始前确认)

1. **急停不可信状态 API 暴露方式**：通过新增 `GET /api/motion/emergency-status` 端点 + Wails 事件 `motion:emergency-untrusted`（push）。独立窗口通过 HTTP 轮询该端点。
2. **`emergency_sop` 配置 schema**：`motion_profiles` 增加 `emergency_sop.physical_stop_location` 与 `emergency_sop.power_off_procedure` 两个必填字符串字段；未配置时拒绝启动运动任务。
3. **独立窗口失联重试参数**：HTTP 超时 1s，重试间隔 500ms（固定，无指数退避），连续失败 3 次锁定 UI 为只读。
4. **B140 真实地址**：本期不强制确认；B140 实机测试由 fake TCP server 覆盖命令数与并发，待真实地址确认后补只读测试。

## Dependency Graph

```
Phase 1: Driver Hardening (独立于 monitor)
  Task 1 (B140 single-flight) ──┐
  Task 2 (B140 Priority Coor.) ──┤
  Task 3 (WTNMC4A Priority)    ──┤
                                 │
Phase 2: Shared Monitor          ▼
  Task 4 (data model) ────────┬──> Task 5 (monitor core) ──┬──> Task 6 (FreshnessPolicy + Generation)
                               │                            │
                               │                            ▼
                               │              Phase 3: Manager + Safety
                               │              Task 7 (manager owns monitor) ──┬──> Task 8 (JudgeArrival)
                               │                                            │
                               │                                            └──> Task 9 (E-Stop protocol)
                               │                                            │
                               │              Phase 4: Project Port + API    │
                               │              Task 10 (MotionStatusReader) ─┤
                               │              Task 11 (API cache)            │
                               │                                            │
                               │              Phase 5: Use Case Migration    │
                               │              Task 12 (calibration) ────────┤
                               │              Task 13 (traversal) ──────────┤
                               │                                            │
                               │              Phase 6: Frontend              │
                               │              Task 14 (motionApi/Store)     │
                               │              Task 15 (delete Main timers) ─┤
                               │              Task 16 (standalone window) ───┤
                               │              Task 17 (E-Stop UI) ───────────┘
                               │                                            │
                               │              Phase 7: Real HW + Cross-Proj │
                               │              Task 18 (motion-controller)   │
                               │              Task 19 (WTNMC4A readonly)     │
                               │              Task 20 (DLL fault inject)    │
                               │                                            │
                               │              Phase 8: Cleanup               │
                               │              Task 21 (delete StartPoller)   │
                               │              Task 22 (ADR)                  │
                               │                                            │
                               ▼                                            ▼
                          (TDD: 测试在每个任务内编写，含 race 测试)
```

## Task List

### Phase 1: Driver Hardening

- [ ] Task 1: B140 Status() single-flight
- [ ] Task 2: B140 Connection Priority Coordinator
- [ ] Task 3: WTNMC4A Connection Priority Coordinator

### Checkpoint: Phase 1 — Driver Hardening
- [ ] `cd shared\device-sdk\go; $env:GOWORK="off"; go test -race ./motion/adapters/hardware -count=10` 全绿
- [ ] B140 fake TCP server 验证：3 并发消费者不放大 TD/TS/MG/TP 命令数
- [ ] WTNMC4A 既有 single-flight 测试无回归
- [ ] 人工 review：六条不变量测试覆盖完整

### Phase 2: Shared Monitor

- [ ] Task 4: Monitor 数据模型 + FreshnessPolicy 接口
- [ ] Task 5: MotionStatusMonitor 核心（Latest/LatestController/Subscribe/RequestRefresh/NotifyCommandExecuted）
- [ ] Task 6: WaitNext + FreshnessPolicy 默认实现 + Generation 重连语义

### Checkpoint: Phase 2 — Shared Monitor
- [ ] `cd shared\motion-control\go; $env:GOWORK="off"; go test -race ./monitor/... -count=10` 全绿
- [ ] monitor 单元测试覆盖：TOCTOU、慢订阅者、Generation 切换、ErrGenerationChanged、Freshness 调用瞬间计算
- [ ] 人工 review：FreshnessPolicy 不固化静态 IsStale；MotionControllerSnapshot 不丢失 Generation

### Phase 3: Manager + Safety

- [ ] Task 7: MotionManager 持有 monitor（生命周期、原子 ownership 切换、NotifyCommandExecuted 调用点）
- [ ] Task 8: JudgeArrival 四层门禁
- [ ] Task 9: 急停不可信协议（后端：命令冻结、告警锁存、SOP 来源、恢复条件、CRITICAL 日志）

### Checkpoint: Phase 3 — Manager + Safety
- [ ] `cd shared\motion-control\go; $env:GOWORK="off"; go test -race ./manager/... ./monitor/... -count=10` 全绿
- [ ] 集成测试：MotionManager + Simulated + Monitor 连接/运动/停止/断开产生预期 sequence + generation
- [ ] 急停不可信协议测试：超阈值触发命令冻结、告警锁存、恢复条件三重门禁
- [ ] 人工 review：ownership 切换原子性、JudgeArrival 四层顺序

### Phase 4: Project Port + API

- [ ] Task 10: WindLabX4 MotionStatusReader 端口 + adapter
- [ ] Task 11: /api/motion/status + Wails MotionGetStatus 读取 monitor 缓存

### Checkpoint: Phase 4 — Project Port + API
- [ ] `cd projects\WindLabX4\services\api-go; go test ./internal/... ./api/...` 全绿
- [ ] `/api/motion/status` 高频并发请求不触发 fake controller `Status()` 调用
- [ ] HTTP 503 在 monitor 故障时正确返回
- [ ] 人工 review：MotionControllerSnapshot 保留 Generation 与 Freshness

### Phase 5: Use Case Migration

- [ ] Task 12: 校准 usecase 迁移到 WaitNext + JudgeArrival
- [ ] Task 13: 遍历 usecase 迁移到 WaitNext + JudgeArrival

### Checkpoint: Phase 5 — Use Case Migration
- [ ] 校准/遍历到位/NoProgress/StatusUnavailable 语义不变（既有回归测试全绿）
- [ ] ErrGenerationChanged 在重连后正确 abort 当前点
- [ ] 既有 NoProgress 到位容差修复和 RR0 Moving 修复全部回归通过
- [ ] 人工 review：无残留 `StatusAll` 直读 ticker

### Phase 6: Frontend

- [ ] Task 14: motionApi/motionStore 单一状态源
- [ ] Task 15: 删除 3 类校准 Main 的 300ms motionStatusPollTimer
- [ ] Task 16: 独立运动窗口失联降级协议
- [ ] Task 17: 急停不可信 UI（红色横幅、SOP 显示、确认按钮、复位流程）

### Checkpoint: Phase 6 — Frontend
- [ ] `cd projects\WindLabX4\apps\desktop-wails\frontend; npm run typecheck; npm run test; npm run build` 全绿
- [ ] 四类校准 Main 不再创建 motionStatusPollTimer（TotalTemperatureMain 本无）
- [ ] 独立窗口失联测试：HTTP 超时/连续失败/重连/HTTP 503 全覆盖
- [ ] 急停不可信 UI 测试：告警锁存、确认按钮不恢复命令、复位三重门禁

### Phase 7: Real Hardware + Cross-Project

- [ ] Task 18: motion-controller 项目验证（build/test/vet 全绿，无代码变更或最小迁移）
- [ ] Task 19: WTNMC4A 192.168.3.141 只读测试（单调用 p50/p95、3 并发批次 p50/p95、排队增量比）
- [ ] Task 20: WTNMC4A DLL 200ms timeout 故障注入测试（验证是否触发 CRITICAL 日志 + 急停不可信协议）

### Checkpoint: Phase 7 — Real Hardware + Cross-Project
- [ ] motion-controller 项目 `go build` + `go test` + `go vet` + 前端 typecheck/build 全绿
- [ ] WTNMC4A 实机只读测试报告：单调用 p95 < 500ms，3 并发批次 p95 排队增量 ≤ 单调用 p95 × 25%
- [ ] WTNMC4A DLL 故障注入测试：超阈值触发 Decision 15 急停不可信协议
- [ ] 人工 review：区分底层真实耗时和排队耗时；决定 WTNMC4A 是否需要专属 staleThreshold（若需要，单独 ADR）

### Phase 8: Cleanup

- [ ] Task 21: 删除/弃用 events.StartStatusPoller
- [ ] Task 22: ADR（急停不可信协议 + Generation 重连语义 + FreshnessPolicy 设计）

### Checkpoint: Phase 8 — Complete
- [ ] 全工作区 `validate-structure.ps1` 通过
- [ ] WindLabX4 + motion-controller 全量构建与测试通过
- [ ] ADR 已提交，含设计决策、风险、验证证据
- [ ] 人工最终 review：可发布

## Risks and Mitigations

| 风险 | 影响 | 缓解 |
|---|---|---|
| WTNMC4A DLL 200ms timeout 实际不可中断 | 高 | Phase 7 Task 20 故障注入测试先于实机部署；超阈值按 Decision 15 进入急停不可信协议，不宣称硬保证 |
| B140 fake TCP server 与真实协议行为偏差 | 中 | fake server 覆盖命令数与并发；真实地址确认后补只读测试；不强制本期确认真实地址 |
| Generation 切换在途 WaitNext 行为复杂 | 高 | Phase 2 Task 6 TDD 优先；race 测试 -count=10；Phase 5 校准/遍历迁移时验证 abort 行为 |
| 急停不可信协议在 motion-controller 项目无对应 UI | 中 | Phase 7 Task 18 验证 motion-controller 是否需要独立 UI；如需要，单独迁移任务 |
| 独立窗口失联检测误报（网络抖动） | 低 | 默认 3 次连续失败才锁定；重连按钮立即恢复；CRITICAL 日志区分超时与 503 |
| 优先级协调器实现复杂度超预期 | 中 | Phase 1 Task 2/3 先实现行为不变量；Phase 2 候选实现对比后再定接口形态 |
| 校准/遍历既有 100ms ticker 删除后行为回归 | 高 | Phase 5 既有回归测试必须全绿；NoProgress 到位容差修复和 RR0 Moving 修复作为金标准 |
| monitor goroutine 泄漏 | 高 | Phase 2 Task 5/6 必须包含 goroutine 泄漏测试；Phase 3 manager 关闭路径验证 |
| 前端删除 300ms timer 后运动面板感知延迟 | 低 | NotifyCommandExecuted 触发 2s 快速窗口；motionApi 已有 fast poll 机制（FAST_POLL_MS=100） |

## Open Questions (Phase 3 开始前需确认)

1. **急停不可信状态 API 暴露方式**：是否新增 `GET /api/motion/emergency-status` + Wails 事件 `motion:emergency-untrusted`？还是扩展 `/api/motion/status` 响应？（推荐前者，避免破坏兼容性）
2. **`emergency_sop` 配置字段必填性**：未配置的 profile 是拒绝启动运动任务（推荐）还是降级为默认 SOP 文案？
3. **B140 真实地址确认**：本期是否需要联系硬件团队确认？还是延后到 Phase 7 再决定？
4. **WTNMC4A 故障注入测试环境**：是否需要在实机环境注入 DLL 超时？还是用 mock DLL 模拟？

## Verification Strategy

- **TDD 强制**：每个任务先写失败测试再实现；测试覆盖率达 80%+
- **Race 测试强制**：shared 层所有并发代码必须通过 `go test -race -count=10`
- **回归测试金标准**：NoProgress 到位容差修复 + RR0 Moving 修复全绿
- **实机只读测试**：WTNMC4A 192.168.3.141，禁止发送任何运动命令
- **前端 typecheck + build 全绿**：禁止 `any` / `@ts-ignore`
- **结构校验**：`validate-structure.ps1` 通过

## See Also

- [spec-motion-status-monitor.md](./spec-motion-status-monitor.md) — 完整规格
- [spec-calibration-motion-safety.md](./spec-calibration-motion-safety.md) — 校准运动安全消费者
- [spec-traversal-motion-safety.md](./spec-traversal-motion-safety.md) — 遍历运动安全消费者
- [spec-calibration-view-state-recovery.md](./spec-calibration-view-state-recovery.md) — 前端校准状态恢复与轮询治理
- 既有 plan 参考：[plan-traversal-motion-safety.md](./plan-traversal-motion-safety.md)
