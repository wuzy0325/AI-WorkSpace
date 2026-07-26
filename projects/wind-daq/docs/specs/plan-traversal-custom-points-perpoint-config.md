# Implementation Plan: 遍历测试·自定义布点 per-point 配置扩展

> 关联 spec：[spec-traversal-custom-points-perpoint-config.md](file:///c:/Users/wuzhy/Documents/D/SVN/SoftWare/trunk/AI-Workspace/projects/wind-daq/docs/specs/spec-traversal-custom-points-perpoint-config.md)
> 日期：2026-07-20
> 状态：待执行

## Overview

依据 spec，在遍历测试模块完成 per-point 配置扩展：每点新增 `dwellMs`（稳定时间 ms）、`samples`（采样点数）、`test`（是否测试）3 个可空字段，per-point 优先，nil 回退全局默认。`test=false` 的点走到位置后跳过采集，结果 CSV 占一行空数据。

改动覆盖：
- 后端 4 个文件：`traversal/types.go` / `traversal/path.go` / `usecase/traversal_config.go` / `usecase/traversal_acquisition.go`
- 前端 3 个文件：`shared/types/traversal.ts` / `shared/pointsFileParser.ts` / `components/traversal/CustomPointsTable.vue`
- 测试 2 个文件：`shared/__tests__/pointsFileParser.test.ts` / 新增后端 `usecase/traversal_perpoint_config_test.go`

## Architecture Decisions

### AD1: 扩展 `traversal.Point` 而非新增 `CustomPoint` 独立类型（简化 spec）

**spec 原意**：定义 `CustomPoint` 独立 struct（X/Y/Z/U + 3 个指针字段），`CustomLayout.Points = []CustomPoint`。

**Plan 决策**：直接扩展 `traversal.Point` 加 3 个指针字段（`DwellMs *int` / `Samples *int` / `Test *bool`），`CustomLayout.Points = []traversal.Point`。

**理由**：
1. **避免重复定义**：CustomPoint 与 Point 字段完全相同，独立类型带来冗余
2. **简化传递路径**：`PointsFromLayout` 不再需要逐字段拷贝，直接 slice copy
3. **采集循环访问简单**：`point := config.Path[pointIndex]` 后直接 `point.DwellMs` 访问，无需并行 slice 查找
4. **零破坏性**：3 个指针字段 `omitempty` + nil 默认，line/rectangle/sector 生成的 Point 留 nil 不影响现有逻辑
5. **JSON 兼容**：旧配置反序列化无新字段时自动 nil → 用全局默认，符合 spec §8

**spec 同步**：spec §Code Style 中 `CustomPoint` struct 定义改为对 `traversal.Point` 的扩展说明（仍保留 per-point 字段语义不变）。

### AD2: `test=false` 跳过采集但保留 CSV 行，新增状态 `PointStatusNotTested`

**spec 要求**：test=false 的点走到位置后直接下一步，结果 CSV 占一行数据列留空。

**Plan 决策**：
1. 在 `RunCurrentPoint` Phase 1（移动完成）后、Phase 2（稳定等待）前插入新分支
2. 检查 `point.Test != nil && !*point.Test` → 跳过 Phase 2/3/4 的采集部分
3. 构造 `PointResult{Point: point, Values: nil, Calculated: nil, SampleCount: 0, DwellTimeElapsed: 0, Status: PointStatusNotTested, StartedAt: motionCompleteMs, CompletedAt: motionCompleteMs}`
4. 调用 `commitPointV2` 写入 CSV（buildRow 已天然支持空 Values/Calculated 写空字符串）
5. 不复用 `PointStatusSkipped`（语义不同：skipped 是数据无效跳过，notTested 是配置主动跳过）

**理由**：
- 区分两种跳过语义便于事后回溯（看状态字段就知道是配置跳过还是异常跳过）
- buildRow 已有空值处理，无需改 sink 层
- 不引入新的 sink 接口

### AD3: 6 个 Open Questions 全部按 spec 建议采纳

| # | 决策 |
|---|---|
| 1 | `test` 列取值：`1` / `0`（数字）。`true`/`false` 也接受。留空 → null |
| 2 | 接受 `skip` 别名，值取反：`skip=1 → test=false`、`skip=0 → test=true` |
| 3 | 表格留空字段显示"用默认"灰色占位 |
| 4 | 结果 CSV `test=false` 行：坐标列填值，数据列空字符串，不加 `tested` 标记列 |
| 5 | 仅限 custom 布局，line/rectangle/sector 不扩展 |
| 6 | 跳过 `test=false` 时不触发"点位到达"事件（遍历测试本无 publisher，无需新建） |

### AD4: 解析器列名归一化扩展

延续现有 `normalizeAxisName` 模式，新增 `normalizeConfigKey`：

```typescript
type PointConfigKey = 'dwellMs' | 'samples' | 'test'

function normalizeConfigKey(raw: string): PointConfigKey | null {
  const cleaned = raw.trim().toLowerCase()
  if (/^(dwell|dwellms|dwell_time_ms|stabilization|stable_ms)$/.test(cleaned)) return 'dwellMs'
  if (/^(samples|samplesperpoint|samples_per_point|sample_count)$/.test(cleaned)) return 'samples'
  if (/^(test|enable|skip)$/.test(cleaned)) return 'test'
  return null
}
```

`skip` 列值需取反：`skip=1 → test=false`。

### AD5: 后端 `traversal.Config` 字段不变

`Config.Path []Point` 已携带 Point（含 3 个新字段），无需新增 `PointConfigs` 并行 slice。采集循环直接读 `point.DwellMs` / `point.Samples` / `point.Test`。

## Task List

### Phase 1: 后端类型扩展

#### Task 1: `traversal.Point` 加 3 个指针字段

**Description**: `core/traversal/types.go` 的 `Point` 加 `DwellMs *int` / `Samples *int` / `Test *bool`，全部 `omitempty`。

**Acceptance**:
- Point struct 包含 7 个字段（X/Y/Z/U + 3 个指针）
- 3 个新字段 JSON tag 分别为 `dwellMs` / `samples` / `test`，均带 `omitempty`
- 字段顺序：X/Y/Z/U 在前，3 个新字段在后（兼容旧 JSON 反序列化）
- 中文注释说明 nil 语义

**Verify**:
- `go build -buildvcs=false ./...` 通过
- `go vet ./...` 通过

**Files**:
- `projects/wind-daq/services/api-go/internal/core/traversal/types.go`

#### Task 2: `CustomLayout.Points` 改为 `[]traversal.Point`

**Description**: `core/traversal/path.go` 中 `CustomLayout` 的匿名 struct 改为 `[]traversal.Point`，`PointsFromLayout` custom 分支简化为直接 slice copy。

**Acceptance**:
- `CustomLayout.Points` 类型为 `[]traversal.Point`（不再是匿名 struct）
- `PointsFromLayout` custom 分支不再逐字段拷贝
- 现有测试（如有 CustomLayout 用例）继续通过

**Verify**:
- `go build -buildvcs=false ./...` 通过
- `go test ./internal/core/traversal/...` 通过

**Files**:
- `projects/wind-daq/services/api-go/internal/core/traversal/path.go`

#### Task 3: usecase DTO 同步

**Description**: `usecase/traversal_config.go:439-446` 的匿名 struct 与 `650-664` 的逐字段拷贝同步更新为 `[]traversal.Point`。

**Acceptance**:
- DTO `Custom.Points` 类型为 `[]traversal.Point`
- DTO → core `CustomLayout` 转换简化为直接 slice 赋值
- 旧配置 JSON 反序列化无新字段时指针为 nil

**Verify**:
- `go build -buildvcs=false ./...` 通过
- `go test ./internal/usecase/...` 通过

**Files**:
- `projects/wind-daq/services/api-go/internal/usecase/traversal_config.go`

### Phase 2: 后端采集逻辑

#### Task 4: 新增 `PointStatusNotTested` 状态

**Description**: `core/traversal/types.go` 中 `PointStatus` 枚举（或常量）加 `PointStatusNotTested`。

**Acceptance**:
- 新增 `PointStatusNotTested` 常量
- 中文注释说明语义区别（vs `PointStatusSkipped`）

**Verify**:
- `go build -buildvcs=false ./...` 通过

**Files**:
- `projects/wind-daq/services/api-go/internal/core/traversal/types.go`

#### Task 5: 采集循环应用 per-point 配置

**Description**: `traversal_acquisition.go` 中 `RunCurrentPoint`：
1. Phase 1（移动）后，检查 `point.Test`，若 `false` → 走新分支（构造空 PointResult + commitPointV2 + 返回）
2. Phase 2（稳定等待）：`dwellMs := point.DwellMs; if dwellMs == nil { dwellMs = config.DwellTimeMs }`
3. Phase 3（采集）：`samples := point.Samples; if samples == nil { samples = config.SamplesPerPoint }; if *samples <= 0 { *samples = 1 }`

**Acceptance**:
- test=false 走新分支，写空数据行，CSV 占一行
- test=true（含 nil）走原流程
- per-point dwellMs 覆盖全局
- per-point samples 覆盖全局，<=0 兜底为 1
- 注释说明 nil 回退全局的逻辑

**Verify**:
- `go build -buildvcs=false ./...` 通过
- 新增单元测试 `traversal_perpoint_config_test.go` 覆盖：
  - test=false 跳过采集
  - per-point dwellMs 覆盖
  - per-point samples 覆盖
  - nil 字段回退全局

**Files**:
- `projects/wind-daq/services/api-go/internal/usecase/traversal_acquisition.go`
- `projects/wind-daq/services/api-go/internal/usecase/traversal_perpoint_config_test.go`（新增）

### Phase 3: 前端类型与解析器

#### Task 6: 前端类型扩展

**Description**: `shared/types/traversal.ts` 中 `TraversalPoint` 加 3 个可选字段。

**Acceptance**:
- `TraversalPoint` 新增 `dwellMs?: number | null` / `samples?: number | null` / `test?: boolean | null`
- `TraversalLayout.custom.points` 类型同步
- 中文 JSDoc 注释

**Verify**:
- `npm run typecheck` 通过

**Files**:
- `projects/wind-daq/apps/desktop-wails/frontend/src/shared/types/traversal.ts`

#### Task 7: 解析器扩展 3 列

**Description**: `shared/pointsFileParser.ts` 新增 `normalizeConfigKey` + 主解析函数支持 3 个新列。

**Acceptance**:
- 新增 `PointConfigKey` 类型 + `normalizeConfigKey` 函数
- 表头识别 3 个新列（含别名）
- 数据行解析 3 个新列值（dwellMs/samples 为整数，test 为 1/0/true/false）
- 留空字段 → `null`（不是 0）
- `skip` 列别名，值取反
- TXT 格式（无逗号）→ 仅解析 X/Y/Z/U
- 旧 CSV（仅 X/Y/Z/U 表头）→ 3 个新字段为 null

**Verify**:
- `npm run typecheck` 通过
- `npm run test -- pointsFileParser` 全绿（含新增用例）

**Files**:
- `projects/wind-daq/apps/desktop-wails/frontend/src/shared/pointsFileParser.ts`
- `projects/wind-daq/apps/desktop-wails/frontend/src/shared/__tests__/pointsFileParser.test.ts`

#### Task 8: 解析器测试用例

**Description**: 新增覆盖 spec §Testing Strategy 中前端解析器全部用例。

**Acceptance**（每个用例一条 test）:
- 旧格式 CSV（仅 X,Y,Z,U）→ 三字段 null
- 新格式 CSV 完整列 → 三字段正确
- 新格式 CSV 部分列（仅 dwellMs）→ samples/test 为 null
- 单行某列留空（`12.5,-30,0,0,,20,1`）→ dwellMs=null, samples=20, test=true
- test 列取值 `0` → false、`1`/`true` → true、留空 → null
- TXT 格式 → 仅解析 X/Y/Z/U
- `skip=1` 别名 → `test=false`
- `skip=0` 别名 → `test=true`

**Verify**:
- `npm run test -- pointsFileParser` 全绿

**Files**:
- `projects/wind-daq/apps/desktop-wails/frontend/src/shared/__tests__/pointsFileParser.test.ts`

### Phase 4: 前端表格 UI

#### Task 9: 表格新增 3 列

**Description**: `CustomPointsTable.vue` 在 X/Y/Z/U 列后、操作列前加 3 列：
- 稳定时间：UiInputNumber，留空显示"用默认"灰色占位，min=100, max=60000
- 采样点数：UiInputNumber，留空显示"用默认"灰色占位，min=1, max=1000
- 是否测试：NCheckbox，默认勾选（null/true 都显示勾选，false 显示未勾选）

**Acceptance**:
- 3 列正确渲染
- 留空显示"用默认"占位
- 编辑后值正确回写 model
- checkbox 状态：null 与 true 都显示勾选，false 显示未勾选
- 表格虚拟滚动性能不受影响

**Verify**:
- `npm run typecheck` 通过
- `npm run build` 通过
- 手动验证：导入新格式 CSV → 表格显示 per-point 配置

**Files**:
- `projects/wind-daq/apps/desktop-wails/frontend/src/components/traversal/CustomPointsTable.vue`

#### Task 10: i18n 文案

**Description**: 更新 `i18nStore.ts` 的 `customPointsFormatHelpBody` 文案，加入新列说明。

**Acceptance**:
- 文案说明 `dwellMs` / `samples` / `test` 三列的取值约定与留空语义
- 包含完整示例

**Verify**:
- `npm run typecheck` 通过
- `npm run build` 通过

**Files**:
- `projects/wind-daq/apps/desktop-wails/frontend/src/stores/i18nStore.ts`

### Phase 5: 全链路验证

#### Task 11: 全量构建与测试

**Verify**:
- `cd projects\wind-daq\services\api-go && go build -buildvcs=false ./...` 通过
- `cd projects\wind-daq\services\api-go && go vet ./...` 通过
- `cd projects\wind-daq\services\api-go && go test ./internal/...` 通过
- `cd projects\wind-daq\apps\desktop-wails\frontend && npm run typecheck` 通过
- `cd projects\wind-daq\apps\desktop-wails\frontend && npm run build` 通过
- `cd projects\wind-daq\apps\desktop-wails\frontend && npm run test` 通过

## Verification Checkpoints

| 检查点 | Phase 完成 | 验证命令 | 期望 |
|---|---|---|---|
| CP1 | Phase 1 后 | `go build && go vet && go test ./internal/core/traversal/... ./internal/usecase/...` | 全绿 |
| CP2 | Phase 2 后 | 同上 + 新增 perpoint config 测试 | 全绿 |
| CP3 | Phase 3 后 | `npm run typecheck && npm run test` | 全绿 |
| CP4 | Phase 4 后 | `npm run typecheck && npm run build` | 全绿 |
| CP5 | Phase 5（最终） | spec §Success Criteria 全部 12 条 | 全部满足 |

## Risks

| 风险 | 缓解 |
|---|---|
| Point struct 加字段影响所有 Point 字面量 | 3 个新字段是指针，零值为 nil，现有字面量 `{X, Y, Z}` 自动 nil 兼容 |
| `test=false` 分支位置错误导致死锁/资源泄漏 | 严格在 Phase 1 后插入，确保 motion complete 信号已消费 |
| 表格虚拟滚动加 3 列后渲染卡顿 | UiInputNumber 用 `lazy` 模式，仅在 blur 时回写 model |
| 旧配置文件加载失败 | `omitempty` + 指针 nil 默认，旧 JSON 无新字段自动 nil |
| 解析器扩展破坏旧 CSV 解析 | 测试用例覆盖"旧格式 CSV"路径，确保零回归 |

## 实施顺序与并行度

```
Phase 1 (后端类型) ─┐
                   ├─ Phase 2 (后端采集) ─┐
                   │                       │
                   └─ Phase 3 (前端类型+解析器) ─┤
                                              ├─ Phase 4 (前端 UI)
                                              │
                                              └─ Phase 5 (全链路验证)
```

- Phase 1 必须先完成（其他 Phase 依赖 Point 类型）
- Phase 2 与 Phase 3 可并行（后端采集 / 前端类型独立）
- Phase 4 依赖 Phase 3（表格用 CustomPoint 类型）
- Phase 5 在所有 Phase 完成后执行
