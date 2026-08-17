# Spec: 遍历测试可靠性、数据一致性与断点恢复

> 来源：代码审查 → spec-driven-development Phase 1 → 规格审查修订
> 日期：2026-07-12
> 状态：已批准
> 修订：v2

## Objective

**目标**：修复遍历测试在不同布局、步长、暂停、停止、无效点和进程重启场景下的业务逻辑及数据一致性问题，使一次遍历任务的运动、采样、状态、CSV、伴随结果日志和 checkpoint 始终表达同一进度。

**用户**：执行风洞遍历测试的操作人员、分析 CSV 数据的试验工程师，以及维护遍历流程和设备适配器的开发人员。

**范围**：覆盖代码审查确认的全部 P0 与 P1 问题：

1. 停止后旧任务仍继续运动、采样或保存，并可能污染新任务。
2. 暂停在运动、稳定、采样或保存阶段的语义不一致。
3. 正常前端配置生成的 checkpoint 无法恢复。
4. checkpoint 恢复未初始化 CSV sink，导致恢复数据静默丢失。
5. 恢复不能校验并续写原 CSV。
6. Windows 上 checkpoint 后续覆盖可能失败。
7. 结果缓存仅存在于内存，进程重启后历史结果缺失。
8. `skip` 点只推进进度，不写结果、CSV 或 checkpoint。
9. 非整除步长可能生成越界点或遗漏终点。
10. 最终保存失败时状态、checkpoint 与错误信息不一致。

**不在范围内**：

- 不重做遍历设置页视觉设计。
- 不改变五孔探针插值算法和压力归一化算法。
- 不增加新的数据库、中间件或第三方依赖。
- 不改变已有 line、rectangle、sector、custom 布局的业务含义；只修正点位边界和可靠性。
- 不改变校准工作流，只保证共享资源锁仍与校准互斥。

## Core Invariants

### I1. 任务所有权

每次遍历任务必须拥有不可变的任务标识、取消上下文、运行协程生命周期和输出会话。旧任务不得读取或修改新任务的配置、状态、结果或 sink。

### I2. 同步停止

停止接口只有在以下条件全部成立后才能返回成功：

1. 已发出任务取消信号。
2. 所有参与遍历的运动轴已收到停止指令。
3. 运动等待、稳定等待、采样重试和保存流程已退出。
4. 运行协程已完全结束。
5. CSV 已 Sync 并关闭。
6. 最终 checkpoint 已完成保存（含 `stopped` 状态，保留可恢复信息）。
7. 工作流资源锁已释放。

停止成功返回后，旧任务不得再产生运动、采样、CSV 写入、checkpoint 更新或状态变更。
停止后的任务保留 checkpoint，用户可选择恢复继续或主动放弃。

### I3. 暂停不丢点

暂停可在 moving、stabilizing、acquiring 阶段请求。暂停完成后：

- 未提交点不得计入 `CommittedPoints`。
- 未提交点不得写成已提交结果。
- 恢复时必须从同一 `currentPointIndex` 重新执行完整单点流程。
- 已成功提交的点不得重复写入。
- 暂停不得被后续阶段状态覆盖。

### I4. 点位可追溯

每个已处理点必须存在一条结果记录，结果状态至少区分：

- `completed`：采样与保存成功，参与提交计数。
- `skipped`：数据验证失败并按策略跳过，参与提交计数。
- `failed`：点处理失败并终止任务时的诊断记录，不参与提交计数，不推进 `commitSeq`。

当验证策略为 `skip` 时，主 CSV 保留点位和采样数据，伴随结果日志保存完整 `PointResult`（含 `skipped` 状态与验证告警），计算结果字段留空。

### I5. 提交水位一致性

checkpoint 保存唯一的 `commitSeq`（单调递增提交序号）。在持久化边界上必须满足：

```text
checkpoint.commitSeq == 已提交点数（completed + skipped）
CSV 已提交数据行数 == checkpoint.commitSeq
伴随结果日志已提交记录数 == checkpoint.commitSeq
```

`commitSeq` 从 1 开始。表头不计入 CSV 数据行数。`commitSeq` 推进是单点提交的线性化点。

### I6. 崩溃恢复模型

每个点的提交协议为：

```text
1. 将完整 PointResult 作为 prepared 记录追加写入伴随结果日志，执行 Flush + Sync。
2. 将 CSV 行写入并执行 Flush + Sync。
3. 原子替换 checkpoint，将 commitSeq 更新为当前序号。
   步骤 3 成功 → 该点正式提交。
```

恢复时：

- 以 checkpoint.commitSeq 为权威提交水位。
- 伴随结果日志中 commitSeq 大于水位的 prepared 记录 → 安全忽略（回滚）。
- CSV 中超过水位的尾部行 → 校验后截断或忽略。
- CSV 少于 commitSeq → 判定为不可自动修复，拒绝恢复。
- 中间序号缺失或行摘要不一致 → 拒绝恢复。

### I7. 单文件恢复

一个任务对应一份主 CSV 和一份伴随结果日志。断点恢复必须使用 checkpoint 记录的实际路径，校验文件身份、表头和已有行数后续写原文件，不创建分片文件。

### I8. 安全布点

对每个步长分段：

- 生成点不得超出闭区间端点。
- 起点和终点都必须出现一次。
- 相邻分段公共边界不得重复。
- 递增、递减和浮点步长遵循同一规则。
- 非法步长、空区间、NaN、Inf 必须在启动前返回明确错误，不得向运动控制器下发无效目标。

## Functional Requirements

### FR1. 任务生命周期控制

- `TraversalManager` 必须为每个任务建立独立运行会话。
- 运行会话必须包含取消函数和完成信号。
- 所有阻塞等待必须支持取消，禁止使用无法中断的长时间 `time.Sleep`。
- 状态更新、结果提交、CSV 写入和 checkpoint 写入前必须验证当前任务所有权。
- 新任务只能在上一任务运行协程退出、sink 关闭和资源锁释放后启动。
- 重复调用停止必须幂等，不得重复关闭资源或覆盖首个有效错误。

### FR2. 暂停与恢复

- 暂停请求必须使任务进入稳定的 `paused` 状态。
- 如果暂停发生在 `commitSeq` 推进前，该点保持未提交。
- 恢复后从 `currentPointIndex` 对应点重新开始完整单点流程。
- moving 阶段暂停必须停止参与遍历的运动轴。
- stabilizing、acquiring 阶段暂停必须中断等待或采样循环。
- 单点提交的线性化点是 checkpoint 原子替换成功并使 `commitSeq` 前进。此前暂停视为未提交；此后暂停视为已提交。

### FR3. 点位生成与校验

- line、rectangle、sector 使用统一的闭区间步长生成规则。
- custom 点位保留 X/Y/Z/U 四轴输入顺序。
- custom 点位必须拒绝 NaN、Inf 和空点集。
- 点位生成错误必须在 `Start` 前返回，不能创建 CSV、获取工作流锁或启动协程。
- 运动范围是否校验由现有运动控制层能力决定；若已有轴限位信息，启动前必须校验，若没有则保持现有设备侧保护且记录为后续增强。

### FR4. 无效数据策略

- `continue`：保存原始值、验证告警和可用计算结果，点状态为 `completed`。
- `retry`：每次重试必须等待新数据；达到上限仍无效时，保持现有"接受并告警"语义，但结果必须携带验证告警。
- `skip`：主 CSV 保留点位和采样数据；伴随结果日志保存完整 `PointResult`（状态 `skipped`、验证告警、计算结果为空），推进 `commitSeq` 和 checkpoint。
- 最后一个点为 `skipped` 时，任务必须正常进入 `completed`。

### FR5. Checkpoint 格式、发现与恢复

- checkpoint 必须保存可直接恢复的 `TraversalRunSnapshot`（见 FR9），不得把前端 API 配置误当作内部配置。
- 如需保留前端原始配置，必须使用独立字段，不得与内部恢复配置共用同一语义字段。
- checkpoint 字段：
  - `Version`：格式版本（初始为 1）。
  - `TaskID`：任务标识。
  - `Snapshot`：`TraversalRunSnapshot`（含完整运行配置、路径、点数、commitSeq）。
  - `CSVPath`：实际 CSV 绝对路径。
  - `ResultLogPath`：伴随结果日志绝对路径。
  - `CSVHeaderHash`：CSV 表头 SHA-256 或等价摘要。
  - `LastCommitHash`：最后提交行的内容摘要，用于身份校验。
  - `CreatedAt` / `UpdatedAt`：时间戳。
- **checkpoint 发现**：应用状态目录下维护活动任务索引文件，记录 `taskId → checkpointPath`。恢复时先读索引找到 checkpoint 路径，再加载 checkpoint。索引损坏或 checkpoint 缺失时返回明确错误。
- 恢复前必须校验：格式版本、任务 ID、配置、点数范围、CSV/结果日志路径、表头哈希、CSV 行数、commitSeq 与结果日志记录数。
- checkpoint 与 CSV/结果日志的 commitSeq 不一致时必须拒绝自动恢复并返回明确诊断，不得静默猜测。
- 已完成任务的 checkpoint 不允许恢复，并从活动索引中注销。
- checkpoint 更新必须在 Windows 11 上支持安全覆盖：写临时文件 → Sync → 关闭 → 原子替换。失败必须进入可见错误状态或保留上一个有效 checkpoint，不得只记录不可见 warning。
- **恢复 API 安全**：恢复请求体仅作为任务标识和确认信息；恢复实现必须重新加载服务端权威 checkpoint，不得信任客户端提交的完整 checkpoint 内容。路径必须属于已登记的数据目录。

### FR6. CSV 与结果日志的耐久提交

- 新任务仍使用唯一文件创建，避免覆盖历史任务。
- 恢复任务必须进入显式 append 模式，禁止调用唯一文件创建逻辑。
- 续写前必须检查 BOM、表头、列顺序和已有数据行数。
- 单点提交顺序（I6 协议）：
  1. 构造完整 `PointResult`，分配 `commitSeq`。
  2. 将结果追加写入伴随结果日志，执行 Flush + Sync；任何步骤失败不推进 `commitSeq`。
  3. 将 CSV 行写入并执行 Flush + Sync；失败不推进 `commitSeq`，结果日志中的 prepared 记录在恢复时自动忽略。
  4. 原子替换 checkpoint，将 `commitSeq` 更新为当前序号。
  5. checkpoint 更新成功视为该点正式提交。
- sink 未初始化时写点必须返回错误，不得静默成功。
- Finalize 必须幂等，但不得吞掉首次 flush/close 错误。

### FR7. 结果持久化

- 断点恢复不得依赖进程内 map 获取历史结果。
- 伴随结果日志保存每条已提交点的完整 `PointResult`，使用 JSON Lines 格式。每条记录为独立 JSON 对象，包含：
  - `version`：记录格式版本。
  - `taskId`：任务标识。
  - `commitSeq`：提交序号。
  - `pointIndex`：点位索引。
  - `pointStatus`：`completed` | `skipped`。
  - `timestamp`：完整精度时间戳。
  - `coordinates`：X/Y/Z/U 坐标。
  - `rawValues`：原始采样值。
  - `calculatedValues`：计算结果（skipped 时为空）。
  - `sampleCount`：采样次数。
  - `validationWarnings`：验证告警列表。
  - `csvRowHash`：对应 CSV 行内容摘要，用于身份绑定。
- 恢复时以 checkpoint.commitSeq 为水位，只加载 commitSeq ≤ 水位的记录。
- 结果日志只允许丢弃无法解析的末尾未提交记录；中间记录损坏、序号缺口或同序号内容冲突必须拒绝恢复。
- 最终结果保存失败时任务必须进入 `error`，保留可恢复 checkpoint，并向前端暴露保存错误码。

### FR8. 状态与错误

- 状态机必须明确允许的转换，禁止任意阶段覆盖 `paused`、`stopped` 或 `error`。
- `stopping` 是内部子状态，不对外暴露为公开 API 状态；前端在调用 Stop 后通过轮询等待 `stopped` 或 `error`。
- 状态转换：

```text
idle → running
running → moving → stabilizing → acquiring → running
moving|stabilizing|acquiring → paused → running
running|moving|stabilizing|acquiring|paused → stopped
running → completed
任意活动状态 → error
```

- `CurrentPoint` / `CommittedPoints` 统一表示已提交点数；前端当前执行坐标由独立字段 `CurrentPointIndex` 表示，避免数量与索引混淆。
- 保存、恢复、点位校验和任务取消错误必须使用现有 `traversal.ErrorCode` 或新增明确错误码。

### FR9. 运行快照

- `TraversalRunSnapshot` 是 checkpoint 中保存的完整恢复配置，必须包含：
  - `Config`：`traversal.Config`（点位路径）。
  - `Validation`：`DataValidationConfig`（策略与阈值）。
  - `Stabilization`：`StabilizationConfig`（稳定等待参数）。
  - `InterpolatorIdentity`：插值器文件路径或身份信息。
  - `SaveOptions`：`SaveOptions`（列开关）。
  - `TotalPoints`：总点数。
  - `CommittedPoints`：已提交点数。
  - `CommitSeq`：当前提交序号。
  - `CSVPath` / `ResultLogPath`：输出文件路径。
  - `CSVHeaderHash` / `LastCommitHash`：身份校验信息。
- 恢复时从 `Snapshot` 重建 manager 执行状态，不得依赖原始前端配置或内存残留。

## Tech Stack

| 层 | 技术 | 约束 |
|---|---|---|
| 后端 | Go，现有模块版本 | 使用 `context`、同步原语和现有端口；不新增第三方依赖 |
| 前端 | Vue 3 + TypeScript + Pinia | 仅适配状态和结果展示，不承载遍历算法 |
| 桌面 | Wails v3 | 后端绑定保持薄层 |
| 存储 | 标准库 CSV/JSON/文件系统 | 兼容 Windows 11 |
| 测试 | Go `testing`、Vitest | 增加单元、集成、竞态和 Windows 文件语义测试 |

## Commands

```powershell
# Backend build, formatting, vet and tests
cd projects\WindLabX4\services\api-go
gofmt -l .
go build -buildvcs=false ./...
go vet ./...
go test ./internal/... ./api/...
go test -race ./internal/usecase ./internal/adapters/storage

# Frontend typecheck, tests and build
cd projects\WindLabX4\apps\desktop-wails\frontend
npm run typecheck
npm run test
npm run build

# Workspace structural validation
cd c:\Users\wuzhy\Documents\D\SVN\SoftWare\trunk\AI-Workspace
powershell -ExecutionPolicy Bypass -File scripts\validate-structure.ps1
powershell -ExecutionPolicy Bypass -File scripts\validate-frontend-structure.ps1 -CheckFileSize
```

## Project Structure

```text
projects/windlabx4/services/api-go/internal/
├── core/traversal/
│   ├── types.go                         # PointResult 状态、commitSeq、Checkpoint/Snapshot 类型、错误码
│   ├── path.go                          # 安全闭区间布点规则
│   └── path_test.go                     # 布点组合与非法输入测试
├── ports/traversal.go                   # sink、结果日志和 checkpoint 端口能力
├── usecase/
│   ├── traversal.go                     # 任务生命周期、同步停止、状态机、commitSeq 管理
│   ├── traversal_acquisition.go         # 可取消单点流程和 I6 提交协议
│   ├── traversal_checkpoint.go          # checkpoint 创建、校验、恢复编排和索引管理
│   ├── traversal_config.go              # API 配置到 TraversalRunSnapshot 转换
│   └── traversal_*_test.go              # 生命周期、恢复、提交协议和一致性测试
└── adapters/storage/
    ├── traversal_csv_writer.go           # 新建与恢复续写模式（含 Sync）
    ├── traversal_result_log.go           # 伴随结果日志（JSONL 追加、校验、Sync）
    ├── file_checkpoint_store.go          # Windows 安全原子替换 + 活动索引
    ├── traversal_active_index.go         # 活动任务索引文件管理
    └── *_test.go                         # CSV/日志/checkpoint/索引文件语义测试

projects/windlabx4/apps/desktop-wails/frontend/src/
├── shared/types/traversal.ts             # 点结果状态与 stopped 状态类型
├── stores/traversalStore.ts              # 状态恢复与跳过点展示适配
└── components/traversal/                 # 仅在需要展示 skipped 时小范围修改
```

实际文件清单在 PLAN 阶段根据影响分析收敛；单个实施任务原则上不修改超过 5 个文件。

## Code Style

遵循现有 Go 六边形架构、明确类型和锁外 I/O 约定。业务规则进入 `core`，流程编排进入 `usecase`，文件 I/O 进入 `adapters/storage`，API/Wails 保持薄层。

示例接口形态：

```go
type TraversalRunSession struct {
    TaskID string
    Done   <-chan struct{}
}

func (manager *TraversalManager) Stop(ctx context.Context) error {
    session, err := manager.requestStop()
    if err != nil {
        return err
    }
    return waitForTraversalExit(ctx, session.Done)
}
```

关键约定：

- 导出类型和函数使用 PascalCase，内部变量使用 camelCase。
- 所有外部 I/O 返回并传播错误，不静默吞错。
- 不在持有 `TraversalManager.mu` 时执行设备、文件或结果存储 I/O。
- 跨 goroutine 共享状态必须通过锁、channel 或不可变快照保护。
- 新增或修改生产代码时，注释使用项目现有语言约定；注释解释业务原因和并发不变量，不复述代码。
- 不使用无类型 map 表达新的领域状态。

## Testing Strategy

### Unit Tests

1. `StepValues`：递增、递减、整除、非整除、浮点误差、多分段、公共边界、零步长、负步长、NaN、Inf。
2. 状态机：合法转换、非法覆盖、暂停/停止优先级、重复停止。
3. CSV：新建、续写、表头不匹配、行数不匹配、未初始化写入、flush/Sync 错误。
4. 伴随结果日志：JSONL 追加、Flush/Sync 失败、尾部截断、中间序号缺失、重复序号、损坏记录恢复。
5. Checkpoint：`TraversalRunSnapshot` 往返、格式版本、Windows 重复覆盖、损坏文件、commitSeq 不一致拒绝。
6. 活动索引：创建、更新、删除、损坏恢复、多任务共存。
7. `skip`：普通点、checkpoint 每点、最后一点。

### Usecase Integration Tests

1. moving 阶段暂停并恢复同一点。
2. stabilizing 阶段暂停，不采样、不写盘。
3. acquiring/retry 阶段暂停，不推进 commitSeq。
4. 任意阶段停止，接口等待协程退出，checkpoint 处于 stopped 状态。
5. 停止后立即启动新任务，旧任务不能污染新任务。
6. 进程重启模拟：重新构造 manager → 读活动索引 → 加载 checkpoint → 校验并续写原 CSV 和结果日志 → 恢复历史结果并完成。
7. CSV、结果日志、checkpoint、status 在每个提交边界满足 I5。
8. 最终结果存储失败进入 error 且保留 checkpoint。
9. 提交协议各步骤注入故障：结果日志 Sync 失败、CSV Sync 失败、checkpoint 替换失败，验证 commitSeq 不推进且恢复时尾部记录正确回滚。
10. 客户端提交篡改 checkpoint 恢复请求，验证服务端拒绝或忽略并改用权威副本。

### Frontend Tests

- `stopped`、`skipped` 状态映射正确。
- 恢复失败时显示后端诊断，不清空有效 checkpoint 提示。
- 完成点数量（`CommittedPoints`）与当前执行点（`CurrentPointIndex`）展示不混淆。
- Stop 按钮在 `stopped` 和 `error` 状态下正确禁用。

### Concurrency Verification

- `go test -race ./internal/usecase ./internal/adapters/storage` 必须通过。
- 测试必须使用可控 fake reader、fake motion、阻塞 sink 和同步屏障，避免依赖不稳定的固定 sleep。
- checkpoint 原子替换需并发读取验证：读协程持续打开并解析目标文件，允许读到旧版本或新版本，但不得出现部分 JSON、空文件或版本倒退。

### Coverage Expectation

不以单一百分比替代场景覆盖。上述 P0/P1 每个问题必须至少有一个修复前失败、修复后通过的回归测试。

## Boundaries

### Always

- 修改任何函数、方法或类型前执行 GitNexus 上游影响分析，并报告风险。
- 先写失败测试，再修改实现。
- 保持 core、ports、usecase、adapters 的依赖方向。
- 所有设备等待和文件 I/O 都支持错误传播；长等待支持取消。
- "已持久化"必须同时满足：写入成功、Flush 成功、错误检查通过、`File.Sync()` 成功。
- 在 Windows 11 上验证：checkpoint 原子替换、结果日志追加 Sync、CSV 续写 Sync、活动索引并发安全。
- 完成后运行结构校验、Go build/vet/test/race、前端 typecheck/test/build。

### Ask First

- 增加第三方依赖。
- 修改公开 HTTP API 请求或响应的不兼容字段。
- 改变 CSV 既有列名、列顺序或时间格式。
- 引入数据库或新的持久化文件类型（伴随结果日志和活动索引属新增文件类型，已在本规格中批准）。
- 改变 `retry` 达上限后的现有"接受并告警"语义。
- 修改校准工作流或共享资源锁协议。

### Never

- 不通过等待旧协程退出就允许新任务复用 manager/sink。
- 不以静默丢弃方式处理未初始化 sink、checkpoint 写失败或结果保存失败。
- 不通过删除失败测试来获得绿色结果。
- 不在 frontend、API 或 Wails 绑定层实现遍历业务规则。
- 不覆盖已有实验 CSV。
- 不提交密钥、设备凭据或真实试验数据。

## Success Criteria

1. line、rectangle、sector 在递增、递减、整除和非整除步长下均包含起止端点，不越界、不重复公共边界。
2. custom 空点集、NaN、Inf 在启动前被拒绝，且不会运动或创建 CSV。
3. moving、stabilizing、acquiring 任一阶段暂停后均不丢点、不重复点；恢复从正确 `currentPointIndex` 继续。
4. 停止接口仅在运行协程退出、运动停止、CSV 与结果日志 Sync 关闭、checkpoint/状态保存和资源锁释放后返回。
5. 停止返回后启动新任务，旧任务不能产生任何设备、文件或状态副作用。
6. 正常前端配置产生的 `TraversalRunSnapshot` 可以在重新构造进程内对象后恢复。
7. 恢复使用 checkpoint 的实际路径，校验并续写原 CSV 和结果日志，不生成分片文件。
8. Windows 11 上 checkpoint 原子替换成功；循环更新至少 1000 次，并发读协程持续解析目标文件，允许读到旧/新版本，但不得出现部分 JSON、空文件或版本倒退。
9. `skip` 点在 CSV 和结果日志中各有一条可追溯记录，最后一点 skip 时任务仍进入 completed。
10. 每个提交边界都满足 I5（commitSeq 一致性）；不一致时拒绝恢复并给出明确错误码。
11. sink 未初始化、CSV/结果日志 Sync、checkpoint 原子替换、最终结果保存失败都进入可见错误态，不推进 commitSeq。
12. 伴随结果日志的 prepared 尾部记录在恢复时正确忽略；CSV 尾部行按 commitSeq 校验后截断。
13. 活动索引损坏或 checkpoint 缺失时返回明确诊断，不静默降级。
14. 恢复 API 不信任客户端提交的 checkpoint 内容；始终加载服务端权威副本。
15. 后端结构校验、build、vet、test、race 和前端 typecheck、test、build 全部通过。

## Confirmed Decisions

1. 主 CSV 不新增 `PointStatus` 和 `ValidationWarnings` 列，保持现有表头兼容。
2. 新增伴随结果日志（JSON Lines 格式）保存完整 `PointResult`（含状态、告警、csvRowHash），CSV 保持现有可选列。恢复时以伴随结果日志为权威数据源，CSV 为辅助校验。
3. 引入单调递增 `commitSeq` 作为提交线性化点。提交协议为：结果日志 Sync → CSV Sync → checkpoint 原子替换。checkpoint 替换成功视为点正式提交。
4. Stop HTTP API 保持现有请求契约，后端使用固定超时；超时返回明确错误，后台继续安全清理且不得允许新任务抢占资源。停止后的任务保留可恢复 checkpoint。
5. 旧版本 checkpoint 不自动迁移、不删除；检测后明确提示格式不兼容并保留原文件。
6. `stopping` 为内部子状态，不暴露为公开 API 状态。前端通过轮询 `stopped` 或 `error` 判断停止完成。
7. 新增活动任务索引文件，维护 `taskId → checkpointPath` 映射，支持进程重启后 checkpoint 发现。
8. `failed` 诊断记录不参与 `commitSeq` 计数，不计入 I5 持久化结果记录数，存储在独立诊断集合中。
9. 恢复 API 不信任客户端提交的 checkpoint 内容；恢复实现必须重新加载服务端权威副本，路径必须属于已登记的数据目录。

## Open Questions

无。

## Approval Gate

只有在以下条件满足后才能进入 PLAN：

- 用户确认 v2 修订后的 Objective、Core Invariants、Functional Requirements 和 Success Criteria。
- 所有 Confirmed Decisions 已获批准。
- 本规格状态由"待审批"更新为"已批准"。
