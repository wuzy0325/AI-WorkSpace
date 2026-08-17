# Spec: 遍历测试·自定义布点 per-point 配置扩展

> 来源：interview-me → 确认意图
> 日期：2026-07-20
> 状态：待评审

## Objective

**目标**：遍历测试·自定义布点（custom layout）的每个点位扩展 3 个 per-point 配置字段：`dwellMs`（稳定时间 ms）、`samples`（采样点数）、`test`（是否测试）。per-point 字段优先，留空字段回退全局默认值（`dwellTimeMs=2000`、`samplesPerPoint=10`）。`test=0` 的点走到位置后直接进入下一步，不采集不保存，但结果 CSV 仍占一行（数据列留空）。

**用户**：WindLabX4 操作员，在自定义布点模式下需要对个别点位做例外配置——例如：
- 某点延长稳定时间（如刚跨过台阶需要更长稳定）
- 某点加密采样（如关键监测点）
- 某点跳过测试（如该点损坏或不可达，但需要保持位置序列以便回溯）

**背景**：
- [traversal/types.go:35-42](file:///c:/Users/wuzhy/Documents/D/SVN/SoftWare/trunk/AI-Workspace/projects/windlabx4/services/api-go/internal/core/traversal/types.go#L35-L42) `Point` 当前只有 `X/Y/Z/U` 4 个坐标字段，无 per-point 采集参数
- [traversal/path.go:333-342](file:///c:/Users/wuzhy/Documents/D/SVN/SoftWare/trunk/AI-Workspace/projects/windlabx4/services/api-go/internal/core/traversal/path.go#L333-L342) `CustomLayout.Points` 用匿名 struct，扩展新字段需要先抽出命名类型
- [usecase/traversal_config.go:439-446](file:///c:/Users/wuzhy/Documents/D/SVN/SoftWare/trunk/AI-Workspace/projects/windlabx4/services/api-go/internal/usecase/traversal_config.go#L439-L446) usecase API DTO 的 Custom 字段也是匿名 struct，与 core 同步
- [pointsFileParser.ts](file:///c:/Users/wuzhy/Documents/D/SVN/SoftWare/trunk/AI-Workspace/projects/windlabx4/apps/desktop-wails/frontend/src/shared/pointsFileParser.ts) 仅解析 `X/Y/Z/U` 4 列，无 per-point 配置列
- [CustomPointsTable.vue](file:///c:/Users/wuzhy/Documents/D/SVN/SoftWare/trunk/AI-Workspace/projects/windlabx4/apps/desktop-wails/frontend/src/components/traversal/CustomPointsTable.vue) 表格列定义只有 selection + 序号 + X/Y/Z/U + 操作
- [TraversalSettings.vue:99-100](file:///c:/Users/wuzhy/Documents/D/SVN/SoftWare/trunk/AI-Workspace/projects/windlabx4/apps/desktop-wails/frontend/src/components/traversal/TraversalSettings.vue#L99-L100) 全局默认值 `dwellTimeMs=2000`、`samplesPerPoint=10`
- [usecase/traversal_acquisition.go:217-219](file:///c:/Users/wuzhy/Documents/D/SVN/SoftWare/trunk/AI-Workspace/projects/windlabx4/services/api-go/internal/usecase/traversal_acquisition.go#L217-L219) 运行时 `samplesPerPoint<=0 → 1` 兜底

**成功画面**：
1. CSV 导入支持扩展表头 `X,Y,Z,U,dwellMs,samples,test`，3 个新列任意可省略（省略则该字段用全局默认）
2. 旧 CSV（仅 `X,Y,Z,U`）继续工作——向后兼容
3. TXT 格式（空白分隔）仅支持纯坐标导入，不引入新字段
4. 表格 UI 每行新增 3 列：稳定时间、采样点数、是否测试（checkbox），留空显示为"用默认"占位
5. 后端 `CustomLayout.Points` 扩展为命名 struct 携带 3 个新字段（指针类型，nil = 用全局默认）
6. 运行时遍历逐点应用：per-point 优先，nil 字段用全局 `dwellTimeMs` / `samplesPerPoint`，`test=false` 跳过采集
7. `test=false` 的点在结果 CSV 中占一行，坐标列填坐标值，数据列（压力、马赫数、系数等）留空
8. 配置保存/加载/恢复全链路携带 per-point 字段，旧配置无新字段时自动 nil → 用全局默认
9. `go build/vet/test` + `npm typecheck/build` 全绿

## Tech Stack

| 层 | 技术 | 版本 |
|---|---|---|
| 后端 | Go | 1.25（go.work 主干） |
| 前端 | Vue 3 + TypeScript + Vite + Naive UI | 与 WindLabX4 现有 |
| 解析器 | 自研 `pointsFileParser.ts` | 已有，扩展列支持 |

## Commands

```powershell
# Backend
cd projects\WindLabX4\services\api-go
go build -buildvcs=false ./...
go vet ./...
go test ./internal/...

# Frontend
cd projects\WindLabX4\apps\desktop-wails\frontend
npm run typecheck
npm run build
npm run test
```

## Project Structure

修改的文件分布（不改目录结构，仅在已有文件内扩展）：

```
projects/windlabx4/
├── services/api-go/internal/
│   ├── core/traversal/
│   │   ├── types.go                        ← Point 不变；CustomLayout 抽出命名 struct
│   │   └── path.go                         ← CustomLayout 定义改写
│   └── usecase/
│       ├── traversal_config.go             ← DTO 的 Custom 字段同步
│       ├── traversal_acquisition.go        ← 逐点应用 per-point 配置
│       └── traversal_helpers.go            ← 如有默认值兜底，扩展到 per-point
└── apps/desktop-wails/frontend/src/
    ├── shared/
    │   ├── types/traversal.ts              ← TraversalLayout.custom 类型扩展
    │   ├── pointsFileParser.ts             ← 解析 3 个新列
    │   └── __tests__/pointsFileParser.test.ts  ← 新增用例
    └── components/traversal/
        └── CustomPointsTable.vue           ← 新增 3 列编辑
```

不动的文件：
- `PointsPreview.vue`（画布仅显示坐标，per-point 配置不影响可视化）
- `TraversalLayoutStep.vue` 的"导入点位"按钮逻辑（保留二次确认对话框）
- `TraversalSettings.vue` 的全局 `dwellTimeMs` / `samplesPerPoint` 输入（仍是 per-config 默认值来源）
- `motion-controller/apps/desktop-wails/frontend/src/shared/types/traversal.ts`（独立维护，不同步扩展）

## Code Style

### 后端：指针表示可空

```go
// CustomPoint 自定义布点的单个点位配置
// 指针字段（*int / *bool）的 nil 语义：未配置 → 用全局 Config.DwellTimeMs / SamplesPerPoint
// 不用 0 作为"未设置"信号：samplesPerPoint 合法最小值是 1，无法用 0 区分"未设置"与"显式 1"
type CustomPoint struct {
    X       float64 `json:"x"`
    Y       float64 `json:"y"`
    Z       float64 `json:"z"`
    U       float64 `json:"u"`
    DwellMs *int    `json:"dwellMs,omitempty"`  // nil = 用全局；非 nil = per-point 覆盖
    Samples *int    `json:"samples,omitempty"`  // nil = 用全局；非 nil = per-point 覆盖
    Test    *bool    `json:"test,omitempty"`     // nil = 用全局默认 true；非 nil = per-point 覆盖
}

type CustomLayout struct {
    Points []CustomPoint `json:"points"`
}
```

### 前端：与后端对齐的可空类型

```typescript
export interface CustomPoint {
  x: number
  y: number
  z: number
  u: number
  /** per-point 稳定时间（ms），undefined 表示用全局 dwellTimeMs */
  dwellMs?: number | null
  /** per-point 采样点数，undefined 表示用全局 samplesPerPoint */
  samples?: number | null
  /** per-point 是否测试，undefined 表示用全局默认 true */
  test?: boolean | null
}
```

### CSV 解析器扩展风格

延续现有 `normalizeAxisName` 模式：小写归一化、支持别名、自动剥离单位注释。

```typescript
export type PointConfigKey = 'dwellMs' | 'samples' | 'test'

export function normalizeConfigKey(raw: string): PointConfigKey | null {
  const cleaned = raw.trim().toLowerCase()
  if (/^(dwell|dwellms|dwell_time_ms|stabilization)$/.test(cleaned)) return 'dwellMs'
  if (/^(samples|samplesperpoint|samples_per_point)$/.test(cleaned)) return 'samples'
  if (/^(test|enable|skip)$/.test(cleaned)) return 'test'  // skip 字段值需取反
  return null
}
```

### 表格 UI 列定义风格

新增 3 列在 X/Y/Z/U 之后、操作列之前：

| 列 | 控件 | 留空显示 | 输入约束 |
|---|---|---|---|
| 稳定时间 | UiInputNumber | "用默认" 灰色占位 | min=100, max=60000 |
| 采样点数 | UiInputNumber | "用默认" 灰色占位 | min=1, max=1000 |
| 是否测试 | NCheckbox | — | 默认勾选 |

留空语义用 `null` 表示（非 `0`），与后端 `*int` nil 语义对齐。

## Testing Strategy

| 测试层 | 框架 | 覆盖目标 |
|---|---|---|
| 前端单元 | Vitest（`npm run test`） | `pointsFileParser.ts` 解析新列、向后兼容、留空字段、`test` 取值约定 |
| 后端单元 | `go test` | `CustomLayout` 反序列化（含 nil 字段、旧配置无新字段）、per-point 应用全局兜底 |
| 后端集成 | `go test ./internal/usecase` | 采集循环遇到 `test=false` 点时跳过采集、结果 CSV 占行空数据 |
| 端到端 | 手动 | 导入新格式 CSV → 表格显示 per-point 配置 → 运行测试 → 验证 CSV 结果行 |

关键测试用例（前端解析器）：
- 旧格式 CSV（仅 `X,Y,Z,U`）→ 三个新字段全为 null
- 新格式 CSV 完整列 → 三个新字段正确解析
- 新格式 CSV 部分列（仅 `X,Y,Z,U,dwellMs`）→ samples / test 为 null
- 单行某列留空（`12.5,-30,0,0,,20,1`）→ dwellMs=null, samples=20, test=true
- `test` 列取值 `0` → false；`1` 或 `true` → true；留空 → null
- TXT 格式（无逗号）→ 仅解析 X/Y/Z/U，忽略任何新字段尝试
- `skip=1` 别名 → `test=false`（取反语义）

关键测试用例（后端）：
- 旧配置 JSON 反序列化（无 dwellMs/samples/test 字段）→ 指针为 nil，无报错
- per-point 配置覆盖全局：dwellMs=5000 的点 → 实际等待 5000ms
- nil 字段回退全局：dwellMs=nil 的点 → 等待 `Config.DwellTimeMs`
- test=false 的点：采集循环跳过，结果 CSV 占一行，数据列空字符串

## Boundaries

- **Always do**:
  - 解析器对未知列保持宽容（忽略而非报错）
  - 任何新字段在 JSON 中带 `omitempty`，确保旧配置序列化不产生冗余字段
  - 后端 `samplesPerPoint<=0 → 1` 兜底逻辑同步应用到 per-point `Samples`
  - 注释用中文说明"为什么"（如"用指针区分未设置与显式 0"）
- **Ask first**:
  - 是否扩展到 line/rectangle/sector 布局（当前 spec 仅限 custom）
  - 是否提供 CSV 导出功能（当前 spec 不含）
  - 是否修改 motion-controller 的同名 traversal 类型（当前 spec 不动）
- **Never do**:
  - 不引入独立的"全局稳定时间"配置项（继续沿用 `dwellTimeMs`）
  - 不修改 4 轴坐标字段语义（X/Y/Z/U 不变）
  - 不破坏旧配置文件兼容性（无新字段时全部走全局默认）
  - 不修改 TXT 格式解析规则（TXT 永远只支持纯坐标）
  - 不在 acquisition 层做"是否测试"的特殊判断（采集循环之外的状态机层处理跳过逻辑）

## Success Criteria

1. 旧 CSV 文件（仅 X,Y,Z,U）导入后所有点 test=true、dwellMs=nil、samples=nil，运行时全部用全局默认值
2. 新 CSV 文件导入后，表格正确显示每点的 per-point 配置值（留空显示"用默认"）
3. 表格内修改任一新字段值，保存配置后重新加载，值正确恢复
4. 运行遍历测试，`test=0` 的点走到位置后直接进入下一步，结果 CSV 中该点占一行（坐标有值，数据列空）
5. 运行遍历测试，`dwellMs=5000` 的点实际等待 5000ms（与全局 2000ms 不同）
6. 运行遍历测试，`samples=50` 的点实际采集 50 次（与全局 10 次不同）
7. `go build -buildvcs=false ./...` 通过
8. `go vet ./...` 通过
9. `go test ./internal/...` 通过
10. `npm run typecheck` 通过
11. `npm run build` 通过
12. `npm run test` 中 `pointsFileParser.test.ts` 全绿

## Open Questions

1. **`test` 列取值约定**：建议 `1/0`（数字，与 X/Y/Z/U 数字风格一致）。备选：`true/false`、`Y/N`、`yes/no`。如无异议采用 `1/0`。
2. **`skip` 别名取反语义**：建议 `skip` 作为 `test` 的别名，值取反（`skip=1 → test=false`）。或直接不接受 `skip` 别名，仅用 `test`/`enable`。如无异议采用前者（接受 `skip` 别名，取反）。
3. **表格 UI per-point 字段留空显示**：建议显示"用默认"灰色占位文本，点击编辑时变为输入框。备选：直接显示全局默认值，编辑时清空。如无异议采用前者。
4. **结果 CSV 的 `test=false` 行**：建议坐标列填坐标值，数据列（压力、马赫数、系数等）全部留空字符串。备选：在该行末尾追加 `tested=false` 标记列。如无异议采用前者（不留标记列）。
5. **是否给 line/rectangle/sector 布局也加 per-point 配置**：当前 spec 仅限 custom。如需扩展另起 spec。
6. **运行时跳过 `test=false` 点时是否仍触发"点位到达"事件推送**：建议触发（UI 状态栏能看到走到该点），仅跳过采集阶段。如无异议采用此方案。
