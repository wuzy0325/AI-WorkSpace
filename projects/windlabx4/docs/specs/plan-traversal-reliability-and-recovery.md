# Implementation Plan: 遍历测试可靠性、数据一致性与断点恢复

> 关联规格：[spec-traversal-reliability-and-recovery.md](spec-traversal-reliability-and-recovery.md)
> 日期：2026-07-12
> 状态：已批准
> 基线：spec v2

## Overview

本计划将遍历测试从“共享 manager 状态 + 最佳努力写盘”迁移为“任务独立会话 + commitSeq 三阶段耐久提交 + 服务端权威恢复”。实施按依赖自底向上推进：先稳定领域契约和安全布点，再独立交付 CSV、JSONL、checkpoint 和活动索引，随后接入每点提交和进程重启恢复，最后收口暂停/停止、HTTP 信任边界和前端状态。每项任务限定在约 5 个文件内，并在高风险阶段之间设置阻断式检查点。

## Architecture Decisions

- **checkpoint 是提交水位权威源**：结果日志 Sync → CSV Sync → checkpoint 原子替换；只有 checkpoint `commitSeq` 前进才算点正式提交。
- **JSONL 是完整结果权威源**：伴随结果日志保存完整 `PointResult`，CSV 保持用户可配置列和既有表头兼容。
- **任务会话隔离共享状态**：`TraversalRunSession` 持有不可变快照、context/cancel、done、输出会话和首个终止错误；manager 只持有当前会话引用和公开状态快照。
- **恢复只信任服务端磁盘状态**：HTTP 只提交 taskId/确认信息；usecase 通过活动索引加载权威 checkpoint，并校验数据目录、文件摘要和提交水位。
- **状态与索引分离**：`CommittedPoints` 表示已提交数量，`CurrentPointIndex` 表示当前执行位置，避免旧 `CurrentPoint` 双重语义。
- **错误优先而非静默降级**：sink 未初始化、Sync、原子替换、摘要或序号不一致均返回明确错误，不推进水位。
- **测试驱动和影响分析门禁**：每项实现前先补失败测试，并按工作区规则对拟修改符号执行 GitNexus 上游影响分析；HIGH/CRITICAL 风险先报告再修改。

## Dependency Graph

```text
T1 领域契约与安全布点
├── T2 持久化端口契约
│   ├── T4 CSV 耐久会话
│   ├── T5 JSONL 结果日志
│   └── T6 Checkpoint 原子替换与活动索引
├── T3 任务会话与状态机
│   └── T9 暂停/停止生命周期
└── T7 运行快照与权威发现

T4 + T5 + T6 + T7 → T8 三阶段提交与崩溃恢复
T3 + T8 → T9 生命周期收口
T7 + T8 + T9 → T10 HTTP 与装配
T1 + T10 → T11 前端契约
T1...T11 → T12 全量验证与回归审查
```

可并行：T3 可与 T4/T5/T6 并行；T4、T5 可并行。必须串行：T8 → T9 → T10。共享端口或类型变更完成前不得并行修改其调用方。

## Task List

### Phase 1: Domain Foundation

#### Task 1: 领域契约与安全布点

**Description:** 扩展遍历领域类型，建立 `PointStatus`、`commitSeq`、`TraversalRunSnapshot`、版本化 `Checkpoint`、提交计数与执行索引的明确语义；同时修正闭区间步长生成和非法值校验，保留必要兼容入口以降低一次性编译影响。

**Acceptance criteria:**
- [ ] `PointResult` 可表达 taskId、commitSeq、completed/skipped、完整时间戳、告警和 CSV 行摘要；failed 诊断不参与提交计数
- [ ] `TraversalRunSnapshot` 包含 Config、Validation、Stabilization、插值器身份、保存选项、路径和提交水位
- [ ] line/rectangle/sector 的递增、递减、整除、非整除、多分段均包含端点且不越界；NaN、Inf、零/非法步长返回错误

**Verification:**
- [ ] `go test ./internal/core/traversal -run "Test.*(Step|Path|Snapshot|Checkpoint|PointStatus)"`
- [ ] `go vet ./internal/core/traversal`

**Dependencies:** None

**Files likely touched:**
- `services/api-go/internal/core/traversal/types.go`
- `services/api-go/internal/core/traversal/errors.go`
- `services/api-go/internal/core/traversal/path.go`
- `services/api-go/internal/core/traversal/path_test.go`
- `services/api-go/internal/core/traversal/types_test.go`（新建或复用现有测试文件）

**Estimated scope:** M（4-5 文件）

---

#### Task 2: 固化持久化端口契约

**Description:** 将 CSV、结果日志、checkpoint 和活动索引能力定义为窄端口，表达新建/恢复、耐久写入、提交水位检查、尾部处理和按 taskId 发现；同步更新最小 fake，使后续适配器可独立实现。

**Acceptance criteria:**
- [ ] CSV 端口能区分新建与恢复会话，并表达 Sync、行数/表头校验、尾部截断和行摘要
- [ ] 结果日志端口能追加 prepared 记录、Sync、按水位读取和校验尾部
- [ ] checkpoint/index 端口能保存、按 taskId 发现、加载、注销，并限制在登记数据目录

**Verification:**
- [ ] `go build ./internal/ports ./internal/usecase`
- [ ] `go test ./internal/usecase -run TestTraversal.*Port`

**Dependencies:** Task 1

**Files likely touched:**
- `services/api-go/internal/ports/traversal.go`
- `services/api-go/internal/usecase/traversal_test.go`
- `services/api-go/internal/usecase/traversal_checkpoint_test.go`
- `services/api-go/internal/usecase/traversal_save_test.go`

**Estimated scope:** M（4 文件）

---

#### Task 3: 任务会话与状态机骨架

**Description:** 引入不可变 `TraversalRunSession`、cancel/done 和任务所有权校验；重构状态更新入口，使 `CommittedPoints` 与 `CurrentPointIndex` 分离。本任务只建立会话和状态机骨架，不接入新持久化协议。

**Acceptance criteria:**
- [ ] manager 同一时刻只允许一个活动会话，旧 taskId 无法更新新会话状态
- [ ] 合法状态转换通过，paused/stopped/error 不会被普通阶段更新覆盖
- [ ] done 只关闭一次，重复取消和清理保持幂等

**Verification:**
- [ ] `go test ./internal/usecase -run "TestTraversal.*(Session|Ownership|State)" -count=20`
- [ ] `go test -race ./internal/usecase -run "TestTraversal.*(Session|Ownership|State)"`

**Dependencies:** Task 1

**Files likely touched:**
- `services/api-go/internal/usecase/traversal.go`
- `services/api-go/internal/usecase/traversal_helpers.go`
- `services/api-go/internal/usecase/traversal_view.go`
- `services/api-go/internal/usecase/traversal_lifecycle_test.go`（新建）

**Estimated scope:** M（4 文件）

### Checkpoint A: Domain Foundation

- [ ] `go build -buildvcs=false ./...` 通过
- [ ] `go test ./internal/core/traversal ./internal/usecase` 通过
- [ ] `go vet ./...` 无告警
- [ ] Review：类型和端口不包含文件系统实现；manager 会话所有权有竞态测试证明

---

### Phase 2: Durable Storage

#### Task 4: CSV 新建、恢复与耐久写

**Description:** 将 CSV writer 改为显式新建/恢复模式；恢复时校验 BOM、表头、列顺序、数据行数和路径，支持按 commitSeq 截断未提交尾部；每行提交必须检查 csv/buffer 错误并执行 `File.Sync()`。

**Acceptance criteria:**
- [ ] 新任务仍唯一创建，恢复只追加原文件且不生成 `-2.csv`
- [ ] 未初始化写入、表头/行数不一致、Flush/Sync 失败均返回错误且不静默成功
- [ ] 可按 checkpoint.commitSeq 安全截断完整尾部行；中间损坏拒绝恢复

**Verification:**
- [ ] `go test ./internal/adapters/storage -run TestTraversalCsvWriter`
- [ ] `go test ./internal/usecase -run TestTraversal.*Sink`

**Dependencies:** Task 2

**Files likely touched:**
- `services/api-go/internal/adapters/storage/traversal_csv_writer.go`
- `services/api-go/internal/adapters/storage/traversal_csv_writer_test.go`
- `services/api-go/internal/usecase/traversal_save_test.go`
- `services/api-go/internal/bootstrap/bootstrap.go`

**Estimated scope:** M（4 文件）

---

#### Task 5: JSONL 完整结果日志

**Description:** 实现伴随 JSONL 结果日志，追加完整 `PointResult` prepared 记录并执行 Flush/Sync；恢复时按 commitSeq 读取，允许忽略水位后的损坏尾部，严格拒绝中间损坏、序号缺口和冲突重复。

**Acceptance criteria:**
- [ ] 每条记录包含版本、taskId、commitSeq、pointIndex、状态、完整结果、告警和 csvRowHash
- [ ] Sync 失败可注入并返回错误；不会把失败记录视为提交
- [ ] 恢复能区分未提交尾部与中间损坏，并按规格处理

**Verification:**
- [ ] `go test ./internal/adapters/storage -run TestTraversalResultLog`
- [ ] `go test -race ./internal/adapters/storage -run TestTraversalResultLog`

**Dependencies:** Task 2

**Files likely touched:**
- `services/api-go/internal/adapters/storage/traversal_result_log.go`（新建）
- `services/api-go/internal/adapters/storage/traversal_result_log_test.go`（新建）
- `services/api-go/internal/bootstrap/bootstrap.go`
- `services/api-go/internal/ports/traversal.go`（仅实现反馈确需收敛时）

**Estimated scope:** M（3-4 文件）

---

#### Task 6: Windows Checkpoint 原子替换与活动索引

**Description:** 升级 checkpoint store 的 Windows 覆盖流程为临时文件写入、Sync、关闭和安全原子替换；新增活动任务索引，实现注册、发现、注销、多任务和损坏诊断。

**Acceptance criteria:**
- [ ] checkpoint 覆盖失败时旧权威文件保持完整，临时文件可诊断清理
- [ ] 活动索引支持 `taskId → checkpointPath` 的注册、更新、发现和注销
- [ ] Windows 循环 1000 次更新并发读取时，不出现空文件、部分 JSON 或版本倒退

**Verification:**
- [ ] `go test ./internal/adapters/storage -run "Test(FileCheckpointStore|TraversalActiveIndex)"`
- [ ] `go test ./internal/adapters/storage -run TestCheckpointAtomicReplace -count=1000`

**Dependencies:** Task 2

**Files likely touched:**
- `services/api-go/internal/adapters/storage/file_checkpoint_store.go`
- `services/api-go/internal/adapters/storage/file_checkpoint_store_test.go`
- `services/api-go/internal/adapters/storage/traversal_active_index.go`（新建）
- `services/api-go/internal/adapters/storage/traversal_active_index_test.go`（新建）
- `services/api-go/internal/bootstrap/bootstrap.go`

**Estimated scope:** M（5 文件）

### Checkpoint B: Durable Storage

- [ ] CSV、JSONL、checkpoint/index 单元测试全部通过
- [ ] `go test -race ./internal/adapters/storage` 通过
- [ ] Windows 原子替换与并发读取验证通过
- [ ] Review：所有“已持久化”路径都检查 Flush/Error/Sync，且失败不提升水位

---

### Phase 3: Commit and Recovery

#### Task 7: 运行快照构建与权威 checkpoint 发现

**Description:** 从 API 配置和 manager 运行参数构建完整 `TraversalRunSnapshot`；checkpoint 只保存内部快照。恢复入口通过活动索引按 taskId 加载服务端权威副本，校验版本、路径归属、表头、摘要和点数范围。

**Acceptance criteria:**
- [ ] 真实前端配置可转换为包含 validation、stabilization、插值器身份和 SaveOptions 的完整快照
- [ ] 重建 manager 后无需内存 `lastCheckpointPath` 即可发现 checkpoint
- [ ] 旧格式、索引损坏、路径越界、文件缺失和摘要不匹配返回明确错误且保留原文件

**Verification:**
- [ ] `go test ./internal/usecase -run "TestTraversal.*(Snapshot|CheckpointDiscovery|Authoritative)"`
- [ ] `go test ./internal/core/traversal -run TestTraversalRunSnapshot`

**Dependencies:** Tasks 1, 2, 6

**Files likely touched:**
- `services/api-go/internal/usecase/traversal_config.go`
- `services/api-go/internal/usecase/traversal_checkpoint.go`
- `services/api-go/internal/usecase/traversal_config_test.go`
- `services/api-go/internal/usecase/traversal_checkpoint_test.go`
- `services/api-go/internal/core/traversal/types.go`

**Estimated scope:** M（5 文件）

---

#### Task 8: 三阶段提交与崩溃恢复

**Description:** 将所有 completed/skipped 点保存收敛到唯一 `commitPoint`：结果日志 Sync → CSV Sync → checkpoint 原子替换 → 更新内存公开状态。实现按 checkpoint 水位回滚 prepared 尾部、截断 CSV 尾部并从 JSONL 重建历史结果。

**Acceptance criteria:**
- [ ] checkpoint 替换成功是唯一线性化点；此前任何故障均不推进 commitSeq/CommittedPoints
- [ ] skip 点和正常点使用同一提交路径，最后一点 skip 可完成任务
- [ ] 重启后按活动索引恢复原 CSV/JSONL，正确处理未提交尾部；中间缺口或摘要冲突拒绝恢复

**Verification:**
- [ ] `go test ./internal/usecase -run "TestTraversalCommit.*(ResultLog|CSV|Checkpoint|Skip)"`
- [ ] `go test ./internal/usecase -run "TestTraversalResume.*(Restart|Mismatch|Tail|Completed)"`
- [ ] `go test -race ./internal/usecase ./internal/adapters/storage`

**Dependencies:** Tasks 4, 5, 6, 7

**Files likely touched:**
- `services/api-go/internal/usecase/traversal_acquisition.go`
- `services/api-go/internal/usecase/traversal_checkpoint.go`
- `services/api-go/internal/usecase/traversal_commit_test.go`（新建）
- `services/api-go/internal/usecase/traversal_resume_test.go`（新建）
- `services/api-go/internal/ports/traversal.go`（仅实现反馈确需收敛时）

**Estimated scope:** M（4-5 文件）

### Checkpoint C: Commit and Recovery

- [ ] 每个提交 I/O 边界的故障注入测试通过
- [ ] 模拟进程重启能发现 checkpoint、续写原文件并恢复完整历史结果
- [ ] commitSeq、CSV 行、JSONL 记录和公开状态在稳定边界一致
- [ ] Review：恢复不信任进程内缓存和客户端 checkpoint 内容

---

### Phase 4: Lifecycle and API

#### Task 9: 可取消暂停与同步停止

**Description:** 将 moving、stabilizing、acquiring、retry 和主循环全部接入任务 context；暂停在提交前保持同一点未完成，恢复重新执行该点；Stop 使用固定超时等待 done、停止所有运动轴、持久化 stopped checkpoint、关闭输出并释放资源锁。

**Acceptance criteria:**
- [ ] 三个运行阶段暂停均不丢点、不重复点、不覆盖 paused 状态
- [ ] Stop 返回成功前协程已退出、文件已 Sync/关闭、checkpoint 已保存、锁已释放
- [ ] Stop 超时后后台继续安全清理且阻止新 Start；重复 Stop 幂等；旧任务不能污染新任务

**Verification:**
- [ ] `go test ./internal/usecase -run "TestTraversal.*Pause.*(Moving|Stabilizing|Acquiring)" -count=20`
- [ ] `go test ./internal/usecase -run "TestTraversal.*(Stop|NoPollution|StoppedCheckpoint)" -count=20`
- [ ] `go test -race ./internal/usecase`

**Dependencies:** Tasks 3, 8

**Files likely touched:**
- `services/api-go/internal/usecase/traversal.go`
- `services/api-go/internal/usecase/traversal_helpers.go`
- `services/api-go/internal/usecase/traversal_acquisition.go`
- `services/api-go/internal/usecase/traversal_lifecycle_test.go`
- `services/api-go/internal/usecase/traversal_pause_test.go`（新建）

**Estimated scope:** M（5 文件）

---

#### Task 10: HTTP 信任边界与统一装配

**Description:** Stop handler 使用后端固定超时 context；resume 请求收窄为 taskId/确认 DTO，由 usecase 重新加载权威 checkpoint。统一三个生产装配根对 CSV、JSONL、checkpoint 和 index 的注入，消除 sink=nil 路径差异。

**Acceptance criteria:**
- [ ] resume 不再接受或信任客户端完整 checkpoint，篡改路径/水位无法影响服务端恢复
- [ ] Stop HTTP 契约保持无请求体，超时返回明确错误
- [ ] bootstrap、apiserver、appcontext 三条路径注入相同持久化实现且现有启动测试通过

**Verification:**
- [ ] `go test ./api/... -run TestTraversal`
- [ ] `go test ./tests/integration -run "TestTraversal.*(Resume|Stop|Tamper)"`
- [ ] `go build -buildvcs=false ./...`

**Dependencies:** Tasks 7, 8, 9

**Files likely touched:**
- `services/api-go/api/server.go`
- `services/api-go/tests/integration/server_test.go`
- `services/api-go/internal/bootstrap/bootstrap.go`
- `services/api-go/pkg/apiserver/apiserver.go`
- `services/api-go/pkg/appcontext/context.go`

**Estimated scope:** M（5 文件）

### Checkpoint D: Backend End-to-End

- [ ] HTTP start/pause/resume/stop/result/checkpoint 流程通过
- [ ] 三个装配根 build 和相关测试通过
- [ ] `go test -race ./internal/usecase ./internal/adapters/storage` 通过
- [ ] Review：停止后无副作用、恢复只读服务端权威状态、工作流锁行为未回归

---

### Phase 5: Frontend and Release Verification

#### Task 11: 前端状态与恢复契约适配

**Description:** 同步 `committedPoints/currentPointIndex`、completed/skipped 点状态和 v2 checkpoint 摘要；resume API 只提交 taskId；恢复失败保留 checkpoint 提示和后端诊断，Stop 不构造公开 stopping 状态。

**Acceptance criteria:**
- [ ] 前端类型与后端公开状态字段一致，不暴露可编辑的内部 snapshot
- [ ] resume 请求仅包含 taskId/确认信息；失败时不清空有效 checkpoint
- [ ] skipped 点可识别，完成数量和当前执行点展示不混淆

**Verification:**
- [ ] `npm run typecheck`
- [ ] `npm run test -- traversal`
- [ ] `npm run build`

**Dependencies:** Tasks 1, 10

**Files likely touched:**
- `apps/desktop-wails/frontend/src/shared/types/traversal.ts`
- `apps/desktop-wails/frontend/src/api/traversalApi.ts`
- `apps/desktop-wails/frontend/src/stores/traversalStore.ts`
- `apps/desktop-wails/frontend/src/shared/__tests__/traversal.test.ts`
- `apps/desktop-wails/frontend/src/stores/__tests__/traversalStore.test.ts`（若不存在则新建）

**Estimated scope:** M（4-5 文件）

---

#### Task 12: 全量验证与回归审查

**Description:** 执行结构、格式、build、vet、单元、集成、race、前端类型和构建验证；对最终变更运行 GitNexus changes 检测和代码审查，确认只影响预期 traversal 流程，并逐项映射 spec 成功标准。

**Acceptance criteria:**
- [ ] spec 15 项 Success Criteria 均有测试或可重复验证证据
- [ ] 所有验证命令通过，无竞态、结构违规或前后端类型漂移
- [ ] 最终影响范围仅包含预期 traversal、存储装配和前端状态流程

**Verification:**
- [ ] 执行下方 Final Verification 全部命令
- [ ] GitNexus detect changes 只报告预期符号和流程
- [ ] 最终 code review 无未解决 P0/P1 问题

**Dependencies:** Tasks 1-11

**Files likely touched:**
- 原则上不修改生产文件；仅修复验证中暴露且属于本规格范围的问题

**Estimated scope:** S（验证任务）

## Final Verification

```powershell
cd c:\Users\wuzhy\Documents\D\SVN\SoftWare\trunk\AI-Workspace\projects\WindLabX4\services\api-go
gofmt -l .
go build -buildvcs=false ./...
go vet ./...
go test ./internal/... ./api/... ./tests/integration/...
go test -race ./internal/usecase ./internal/adapters/storage

cd c:\Users\wuzhy\Documents\D\SVN\SoftWare\trunk\AI-Workspace\projects\WindLabX4\apps\desktop-wails\frontend
npm run typecheck
npm run test
npm run build

cd c:\Users\wuzhy\Documents\D\SVN\SoftWare\trunk\AI-Workspace
powershell -ExecutionPolicy Bypass -File scripts\validate-structure.ps1
powershell -ExecutionPolicy Bypass -File scripts\validate-frontend-structure.ps1 -CheckFileSize
```

## Risks and Mitigations

| Risk | Impact | Mitigation |
|---|---|---|
| `TraversalManager` 并发模型重构影响大量测试与装配 | 高 | T3 先建立会话骨架；T9 再接入取消与停止；每阶段运行 race |
| 三文件无法形成文件系统事务 | 高 | 使用 checkpoint.commitSeq 作为线性化点；对水位后的尾部做确定性回滚 |
| Windows 覆盖语义和共享读句柄导致替换失败 | 高 | 旧文件保留优先；1000 次替换 + 并发读测试；失败显式返回 |
| CSV 可选列无法重建完整结果 | 高 | JSONL 保存完整 PointResult，CSV 仅用于用户输出和摘要校验 |
| 端口变更引发 fake 和构造函数大面积修改 | 中 | T2 一次固化契约；适配器独立交付；装配集中在 T10 收口 |
| Stop 超时后新任务抢占未释放资源 | 高 | session 未 done 前 Start 必须拒绝；锁在后台清理完成后释放 |
| 旧 checkpoint 与 v2 混用 | 中 | 严格版本检查；不迁移、不删除，返回可诊断错误 |
| 前后端布点算法继续漂移 | 中 | 后端启动校验为权威；前端补同一测试向量，后续另行评估预览 API |

## Open Questions

无。实施前唯一门禁是：spec v2 与本计划均需用户明确批准。

## Approval Gate

进入 IMPLEMENT 前必须满足：

- [ ] spec v2 状态更新为“已批准”
- [ ] 本计划经用户审阅并批准
- [ ] 12 个任务均有验收条件、验证命令和依赖关系
- [ ] 每个任务预计修改不超过 5 个文件
- [ ] Checkpoint A-D 均作为阻断式验证门禁

