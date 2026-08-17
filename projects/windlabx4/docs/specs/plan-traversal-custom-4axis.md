# Implementation Plan: 遍历测试·自定义点模式 4 轴扩展 + TXT/CSV 点位导入

> 关联 spec：[spec-traversal-custom-4axis.md](file:///C:/Users/wuzhy/Documents/D/SVN/SoftWare/trunk/AI-Workspace/projects/windlabx4/docs/specs/spec-traversal-custom-4axis.md)
> 日期：2026-07-10
> 状态：待执行（v2 — 已按 code review 修复字面量清单/PointsPreview 改造点/CSV 4 列/i18n/单元测试等 13 项）

## Overview

依据 spec §A1-A5，在遍历测试模块完成：后端 `Point`/`CustomLayout`/`traversalAPIConfig` 加 Z/U 字段、`availableAxisTargets` 加 U 轴映射、CSV writer 输出 4 轴列；前端 `TraversalPoint`/`TraversalLayout` 类型扩展、`TraversalLayoutStep` 4 轴输入行、`PointsPreview` 轴对选择器、TXT/CSV 导入组件、i18n key。改动覆盖 4 个后端文件（含 CSV writer）、3 个前端文件 + 2 个新增模块（导入工具 + 单元测试）+ i18n 文件。

## Architecture Decisions

- **结构体加法不打散**：`Point` 直接加 `U float64` 字段，不引入 `map[AxisName]float64` 动态轴。4 轴是位移机构物理上限，用具名字段比 map 更安全（编译期检查）、零配置解析更快。
- **零值向后兼容**：旧配置文件 JSON 无反序列化的字段填零值 (`float64` → `0`)，`availableAxisTargets` 仅在控制器实际配置了该轴时才写入 map（skip U 轴不存在的控制器）。两个机制共同保障旧配置+旧控制器兼容。
- **前端导入纯前端处理**：TXT/CSV 解析在前端完成，后端不引入文件解析依赖。导入结果直接写入 `customPoints` ref，与手动录入走同一条 save config 通道。
- **PointsPreview 轴对选择器用 props 驱动**：画布 `draw()` 完全基于传入的 `hAxis`/`vAxis` prop，不持有可突变状态。切换轴对后 `watch([...hAxis, vAxis])` → `nextTick(draw)` 重绘。
- **列名归一化算法**：权威实现见 Task 7 的 `normalizeAxisName`（trim → lowercase → 移除括号注释 → 逐轴正则匹配），spec §导入列名归一化的描述以 Task 7 为准。
- **CSV 始终输出 4 轴列**：`traversal_csv_writer` 表头和行写入均改为输出 X/Y/Z/U 4 列。line/rectangle/sector 模式 CSV 多两列 0 不影响读阅，避免按模式分支判断列数的复杂度。
- **Estimated scope 量化标准**：XS = 1 文件 < 10 行；S = 1-3 文件 < 30 行；M = 2-4 文件 30-80 行或含新组件；L = ≥ 5 文件或 > 80 行。

## Task List

### Phase 1: Backend — 核心模型 4 轴扩展

#### Task 1: Point 结构体加 U 字段

**Description:** `core/traversal/types.go` 的 `Point` 加 `U float64 \`json:"u"\`` 字段。

**关键事实**：Go struct literal 加字段对 named field 字面量**透明**（缺字段自动填零值，编译不报错）。唯一**必须改**的是 anonymous struct 字面量（类型不匹配会编译失败）— 见 Task 2。

**已验证的 `traversal.Point{...}` 字面量清单**（按 Grep 实测，行号准确）：

| 文件 | 行号 | 当前字段 | 是否需改 |
|---|---|---|---|
| `core/traversal/path.go` | 70 | `{X, Y, Z}` | 无需改（named field，缺 U 自动填零） |
| `core/traversal/path.go` | 114 | `{X, Y}` | 无需改 |
| `core/traversal/path.go` | 129 | `{X, Y}` | 无需改 |
| `core/traversal/path.go` | 133 | `{X, Y}` | 无需改 |
| `core/traversal/path.go` | 341 | `{X, Y}` | **必须改**（custom 分支漏 Z/U 传递） |
| `core/traversal/path_test.go` | 6 | `{...}` 完整字段 | 无需改 |
| `core/traversal/path_test.go` | 22 | `{X: 0}` | 无需改 |
| `usecase/traversal_checkpoint_test.go` | 138 | `{X, Y, Z}` | 无需改 |
| `usecase/traversal_save_test.go` | 65 | `{X, Y, Z}` | 无需改 |
| `adapters/calstore/store_test.go` | 68 | `{X, Y, Z}` | 无需改 |
| `adapters/storage/traversal_csv_writer_test.go` | 35 | `{X, Y}` | 无需改 |
| `adapters/storage/traversal_csv_writer_test.go` | 73 | `{X, Y}` | 无需改 |

**Acceptance criteria:**
- [ ] `Point` 结构体包含 `X`, `Y`, `Z`, `U` 四个 float64 字段
- [ ] `path.go:341` custom 分支改为 `Point{X: point.X, Y: point.Y, Z: point.Z, U: point.U}`
- [ ] `GridPointsFromAxesOrdered` / `GridPointsFromAxesSnakeOrdered` / `SectorPointsFromRadiiAngles` 等生成函数不做改动（line/rectangle/sector 无 Z/U 参数）
- [ ] 不主动修改其他 named field 字面量（避免无意义 diff）

**Verification:**
- [ ] `go build -buildvcs=false ./...` 通过
- [ ] `go vet ./internal/...` 通过
- [ ] `go test ./internal/...` 通过（含所有已有测试用例）

**Files likely touched:**
- `projects/windlabx4/services/api-go/internal/core/traversal/types.go`
- `projects/windlabx4/services/api-go/internal/core/traversal/path.go`

**Estimated scope:** XS（2 文件，2-4 行改动）

---

#### Task 2: CustomLayout 点结构加 Z/U + traversalAPIConfig 同步

**Description:** `core/traversal/path.go` 的 `CustomLayout` 匿名点结构当前只有 X/Y（path.go:289-295），同时加 `Z float64 \`json:"z"\`` 和 `U float64 \`json:"u"\``。`usecase/traversal_config.go` 的 `traversalAPIConfig.Layout.Custom.Points` 同步加 Z/U 两字段（traversal_config.go:335-340），`ParseAndStartTraversal` 的 Custom 映射（traversal_config.go:462-474）传 `point.Z`, `point.U`。

**注意**：`ParseAndStartTraversal` 的 anonymous struct 字面量 `{X: p.X, Y: p.Y}` 类型必须与扩展后的 struct 一致，否则**编译失败**，必须改。

**Acceptance criteria:**
- [ ] `CustomLayout.Points` 元素包含 `X`, `Y`, `Z`, `U` 四个字段（含 json tag）
- [ ] `traversalAPIConfig.Layout.Custom.Points` 同步扩展
- [ ] `ParseAndStartTraversal` Custom 映射改为 `cl.Points = append(cl.Points, struct{...}{X: p.X, Y: p.Y, Z: p.Z, U: p.U})`

**Verification:**
- [ ] `go build -buildvcs=false ./...` 通过
- [ ] `go vet ./internal/...` 通过
- [ ] `go test ./internal/...` 通过

**Files likely touched:**
- `projects/windlabx4/services/api-go/internal/core/traversal/path.go`
- `projects/windlabx4/services/api-go/internal/usecase/traversal_config.go`

**Estimated scope:** XS（2 文件，6-8 行改动）

---

#### Task 3: availableAxisTargets 加 U 轴映射

**Description:** `usecase/traversal_helpers.go` 的 `availableAxisTargets` switch-case 加 `case motion.AxisU: targets[axis.Name] = point.U`。

**U 轴语义注释**：在 switch 上方加注释说明"U 轴仅在 motion.ControllerStatus.Axes 含 AxisU 时生效（如旋转台 / 第四轴位移机构），无 U 轴的控制器 profile 会自动跳过此 case"。

**Acceptance criteria:**
- [ ] switch-case 包含 X/Y/Z/U 四个 case
- [ ] 上方加 U 轴语义注释（说明何时生效）
- [ ] 未在 `status.Axes` 中注册的轴不被写入 map（由外层 `for _, axis := range status.Axes` 保证）

**Verification:**
- [ ] `go build -buildvcs=false ./...` 通过
- [ ] `go vet ./internal/...` 通过
- [ ] **手动确认**：至少 1 个 motion-controller profile 含 `AxisU` 配置（用 Grep `AxisU` 在 `shared/device-sdk/go/motion/` 和 motion-controller 配置文件中验证），若全部 profile 均无 U 轴则 spec 假设落空，需先在 motion-controller 加测试 profile

**Files likely touched:**
- `projects/windlabx4/services/api-go/internal/usecase/traversal_helpers.go`

**Estimated scope:** XS（1 文件，3-5 行改动含注释）

---

#### Task 4: CSV writer 输出 4 轴列

**Description:** `adapters/storage/traversal_csv_writer.go` 当前 [L234](file:///c:/Users/wuzhy/Documents/D/SVN/SoftWare/trunk/AI-Workspace/projects/windlabx4/services/api-go/internal/adapters/storage/traversal_csv_writer.go#L234) 表头 `cols = append(cols, "X", "Y")` 和 [L260](file:///c:/Users/wuzhy/Documents/D/SVN/SoftWare/trunk/AI-Workspace/projects/windlabx4/services/api-go/internal/adapters/storage/traversal_csv_writer.go#L260) 行写入 `row = append(row, formatFloat(p.Point.X), formatFloat(p.Point.Y))`，均扩展为 4 列。

**决策依据**：始终输出 4 列，避免按 pattern 分支判断列数。line/rectangle/sector 模式 CSV 多两列 0 不影响读阅和后续工具解析；旧 CSV 读入路径不在本任务范围（CSV 是写入路径单向输出）。

**Acceptance criteria:**
- [ ] `buildHeader` L234 改为 `cols = append(cols, "X", "Y", "Z", "U")`
- [ ] `buildRow` L260 改为 `row = append(row, formatFloat(p.Point.X), formatFloat(p.Point.Y), formatFloat(p.Point.Z), formatFloat(p.Point.U))`
- [ ] 现有 `traversal_csv_writer_test.go` 测试用例通过（如断言列数需同步更新）

**Verification:**
- [ ] `go build -buildvcs=false ./...` 通过
- [ ] `go vet ./internal/...` 通过
- [ ] `go test ./internal/adapters/storage/...` 通过
- [ ] **手动验证**：启动一次 line 模式采集，检查 CSV 表头含 `X,Y,Z,U` 四列，Z/U 列值为 `0`

**Files likely touched:**
- `projects/windlabx4/services/api-go/internal/adapters/storage/traversal_csv_writer.go`
- `projects/windlabx4/services/api-go/internal/adapters/storage/traversal_csv_writer_test.go`（若断言列数）

**Estimated scope:** XS（1-2 文件，2-4 行改动）

---

### Phase 2: Frontend — 类型扩展 + 4 轴编辑行

#### Task 5: TraversalPoint / TraversalLayout 类型扩展

**Description:** `shared/types/traversal.ts` 的 `TraversalPoint` 加 `z: number` / `u: number`；`TraversalLayout.custom.points` 元素加 `z` / `u`。

注意：`getTraversalLayoutPoints` 内部的 `gridPointsFromAxesY` / `sectorPointsFromRadiiAngles` / `swapPoints` 等生成函数只构造 X/Y，`TraversalPoint` 加 `z`/`u` 后这些函数隐式填 `undefined`——TypeScript strict 模式下需显式填 `z: 0, u: 0`。

**方案**：字段声明为必填 (`z: number; u: number`)，所有创建 `TraversalPoint` 字面量的位置补 `z: 0, u: 0`。避免可选字段产生 `undefined` 与后端 `float64(0)` 的语义不一致。

**Acceptance criteria:**
- [ ] `TraversalPoint` 有 `x`, `y`, `z`, `u` 四个必填 number 字段
- [ ] `TraversalLayout.custom.points` 元素类型同步扩展
- [ ] `gridPointsFromAxesY` [L198, L202](file:///c:/Users/wuzhy/Documents/D/SVN/SoftWare/trunk/AI-Workspace/projects/windlabx4/apps/desktop-wails/frontend/src/shared/types/traversal.ts#L198) 每个 `push({ x, y })` 补 `z: 0, u: 0`
- [ ] `sectorPointsFromRadiiAngles` [L232-235](file:///c:/Users/wuzhy/Documents/D/SVN/SoftWare/trunk/AI-Workspace/projects/windlabx4/apps/desktop-wails/frontend/src/shared/types/traversal.ts#L232-L235) 每个 `push({ x, y })` 补 `z: 0, u: 0`
- [ ] `getTraversalLayoutPoints` custom 分支 `layout.custom.points.map(...)` 输出携带 z,u

**Verification:**
- [ ] `npm run typecheck` 零错误
- [ ] `npm run build` 通过

**Files likely touched:**
- `projects/windlabx4/apps/desktop-wails/frontend/src/shared/types/traversal.ts`

**Estimated scope:** S（1 文件，约 15-20 处字面量补齐）

---

#### Task 6: TraversalLayoutStep / TraversalSettings 4 轴编辑行

**Description:** `TraversalLayoutStep.vue` 的 `customPoints` / `customPointInput` 类型声明从 `{ x: number; y: number }` 扩展为 `{ x: number; y: number; z: number; u: number }`。

模板改动：
1. `customPointInput` 输入区加 Z/U 两个 `<UiInputNumber>`（[L230-231](file:///c:/Users/wuzhy/Documents/D/SVN/SoftWare/trunk/AI-Workspace/projects/windlabx4/apps/desktop-wails/frontend/src/components/traversal/TraversalLayoutStep.vue#L230-L231) 后追加）
2. 点位列表行 [L236](file:///c:/Users/wuzhy/Documents/D/SVN/SoftWare/trunk/AI-Workspace/projects/windlabx4/apps/desktop-wails/frontend/src/components/traversal/TraversalLayoutStep.vue#L236) 从 `{{ pt.x }}, {{ pt.y }}` 改为 `{{ pt.x }}, {{ pt.y }}, {{ pt.z }}, {{ pt.u }}`
3. `addCustomPoint` 函数 [L103](file:///c:/Users/wuzhy/Documents/D/SVN/SoftWare/trunk/AI-Workspace/projects/windlabx4/apps/desktop-wails/frontend/src/components/traversal/TraversalLayoutStep.vue#L103) push 时带上 `z: customPointInput.value.z, u: customPointInput.value.u`；reset 时 `{ x: 0, y: 0, z: 0, u: 0 }`

`TraversalSettings.vue` 的 `customPoints` / `customPointInput` ref 初始类型同步扩展。

**加载逻辑无需改**：[TraversalSettings.vue:280](file:///c:/Users/wuzhy/Documents/D/SVN/SoftWare/trunk/AI-Workspace/projects/windlabx4/apps/desktop-wails/frontend/src/components/traversal/TraversalSettings.vue#L280) `customPoints.value = layout.custom?.points.map(p => ({ ...p })) ?? []` 用 spread 扩展字段，加 z/u 后**自动覆盖**，无需显式改。

**Acceptance criteria:**
- [ ] 输入区显示 4 个 UiInputNumber（X/Y/Z/U），`class="w-80px"`
- [ ] 点位列表行正确显示 4 轴坐标
- [ ] 添加/删除/清空操作正常
- [ ] 配置保存→重新打开→4 轴数据完整恢复（spread 自动覆盖）

**Verification:**
- [ ] `npm run typecheck` 零错误
- [ ] `npm run build` 通过

**Files likely touched:**
- `projects/windlabx4/apps/desktop-wails/frontend/src/components/traversal/TraversalLayoutStep.vue`
- `projects/windlabx4/apps/desktop-wails/frontend/src/components/traversal/TraversalSettings.vue`

**Estimated scope:** S（2 文件，每文件 ~10 行改动）

---

### Phase 3: PointsPreview 轴对选择器

#### Task 7: PointsPreview 加横/纵轴下拉选择器

**Description:** `PointsPreview.vue` 改为数据轴可配置。当前画布有 7 处硬编码 X/Y 引用，全部需切换为 hAxis/vAxis 驱动：

**改动点清单**（已按 Grep 实测）：

| # | 行号 | 当前实现 | 改造为 |
|---|---|---|---|
| 1 | [L23-45](file:///c:/Users/wuzhy/Documents/D/SVN/SoftWare/trunk/AI-Workspace/projects/windlabx4/apps/desktop-wails/frontend/src/components/traversal/PointsPreview.vue#L23-L45) `bounds` computed | `p.x / p.y` | `p[hAxis] / p[vAxis]`（返回 `{minH, maxH, minV, maxV}` 或保留 minX/maxX 但语义切为 hAxis） |
| 2 | [L56-57](file:///c:/Users/wuzhy/Documents/D/SVN/SoftWare/trunk/AI-Workspace/projects/windlabx4/apps/desktop-wails/frontend/src/components/traversal/PointsPreview.vue#L56-L57) `spanX/spanY` | `bounds.maxX - bounds.minX` | 改为引用 h/v 轴语义字段 |
| 3 | [L82-88](file:///c:/Users/wuzhy/Documents/D/SVN/SoftWare/trunk/AI-Workspace/projects/windlabx4/apps/desktop-wails/frontend/src/components/traversal/PointsPreview.vue#L82-L88) `transformX/transformY` | 函数内引用 `bounds.value.minX/maxX/minY/maxY` 字段名 | 泛化为 `transformAxis(coord, bounds, 'h'/'v')`，或保留双函数但内部读 h/v 字段 |
| 4 | [L148-159](file:///c:/Users/wuzhy/Documents/D/SVN/SoftWare/trunk/AI-Workspace/projects/windlabx4/apps/desktop-wails/frontend/src/components/traversal/PointsPreview.vue#L148-L159) 网格绘制循环 | `bounds.value.minX/maxX/minY/maxY` | 改为引用 h/v 字段 |
| 5 | [L169-172](file:///c:/Users/wuzhy/Documents/D/SVN/SoftWare/trunk/AI-Workspace/projects/windlabx4/apps/desktop-wails/frontend/src/components/traversal/PointsPreview.vue#L169-L172) 十字线 | `transformX(0)` / `transformY(0)` | 不变（0 点对齐），但底层 transformX/Y 已切换 |
| 6 | [L183-184](file:///c:/Users/wuzhy/Documents/D/SVN/SoftWare/trunk/AI-Workspace/projects/windlabx4/apps/desktop-wails/frontend/src/components/traversal/PointsPreview.vue#L183-L184) 点绘制 | `transformX(point.x)` / `transformY(point.y)` | `transformX(point[hAxis])` / `transformY(point[vAxis])` |
| 7 | [L270-271](file:///c:/Users/wuzhy/Documents/D/SVN/SoftWare/trunk/AI-Workspace/projects/windlabx4/apps/desktop-wails/frontend/src/components/traversal/PointsPreview.vue#L270-L271) 底部标签 | `` `X: ${minX} ~ ${maxX}` `` / `` `Y: ${minY} ~ ${maxY}` `` | `` `${hAxis.toUpperCase()}: ${minH} ~ ${maxH}` `` / `` `${vAxis.toUpperCase()}: ${minV} ~ ${maxV}` `` |

**新增内容**：
1. 新增 `hAxis` / `vAxis` prop（默认 `'x'` / `'y'`），父组件 `TraversalSettings` 传入
2. 新增 `AXIS_KEYS = ['x', 'y', 'z', 'u']` 常量
3. 画布左上角渲染两个 `NSelect` 下拉框，v-model 绑定 `hAxis`/`vAxis`
4. 横纵轴不能相同：选项计算时排除对方当前值，或校验后强制重置

**图案模式退化方案**：line/rectangle/sector 模式下 Z/U 数据始终为 0，切换到 X-Z / Y-U 等轴对时画布会塌缩成一条线。处理策略：
- **不禁用选项**（保持探索性，允许用户查看 0 数据分布）
- 在画布顶部叠加半透明提示文本：`该轴对数据全为 0（{pattern} 模式仅生成 X/Y）`
- 检测条件：`bounds.maxH - bounds.minH < 0.01 && bounds.maxV - bounds.minV < 0.01` 且 pattern != 'custom'

`TraversalSettings.vue` 中新增 `previewHAxis` / `previewVAxis` ref（默认 'x'/'y'），传递给 `<PointsPreview :h-axis :v-axis>`。

**Acceptance criteria:**
- [ ] 画布显示两个下拉选择器（左上角），默认 X-Y
- [ ] 切换横轴/纵轴后画布实时重绘，点位置按新轴对数据分布
- [ ] 横纵轴不能选择同一轴（互斥校验）
- [ ] 坐标轴十字线跟随选中轴对的 0 点
- [ ] 底部标签显示选中的轴名和范围
- [ ] 当前点闪烁、已完成/未完成点颜色逻辑不变
- [ ] 图案模式下切换到含 Z/U 的轴对时显示"该轴对数据全为 0"提示
- [ ] bounds/transformX/transformY 内部字段引用全部切换为 h/v 语义（不能只改 bounds 不改 transformX）

**Verification:**
- [ ] `npm run typecheck` 零错误
- [ ] `npm run build` 通过
- [ ] 手动切换轴对多次验证无闪烁/错位
- [ ] 手动切到 X-Z 轴对在 line 模式下显示提示文本

**Files likely touched:**
- `projects/windlabx4/apps/desktop-wails/frontend/src/components/traversal/PointsPreview.vue`
- `projects/windlabx4/apps/desktop-wails/frontend/src/components/traversal/TraversalSettings.vue`

**Estimated scope:** M（2 文件，PointsPreview 重构较多，约 50-80 行改动）

---

### Phase 4: TXT/CSV 点位导入

#### Task 8: 导入工具模块 + 单元测试

**Description:** 新建 `src/shared/utils/pointsFileParser.ts`：

```typescript
// pointsFileParser.ts

/** 列名 → 标准轴名归一化映射
 *  算法：trim → lowercase → 移除括号注释 → 逐轴正则匹配
 *  权威实现，spec §导入列名归一化的描述以此为准
 */
function normalizeAxisName(raw: string): 'x' | 'y' | 'z' | 'u' | null {
  const cleaned = raw.trim().toLowerCase()
  // 移除括号注释（如 "X(mm)" → "x"），含全角括号
  const base = cleaned.replace(/\(.*\)|（.*）/, '').trim()
  // X: "x", "posx", "pos_x"
  if (/^(x|pos[_]?x)$/.test(base)) return 'x'
  if (/^(y|pos[_]?y)$/.test(base)) return 'y'
  if (/^(z|pos[_]?z)$/.test(base)) return 'z'
  // U: "u", "posu", "pos_u", "α", "alpha"
  if (/^(u|pos[_]?u|[α]|alpha)$/.test(base)) return 'u'
  return null
}
```

**解析入口 `parsePointsFile(text: string): Array<{x,y,z,u}>`**：
1. 检测分隔符：统计第一行逗号数 > 0 → CSV（`parseCsv`），否则 → 空格/Tab 分隔（`parseTsv`）
2. 检测首行是否为表头：首行任一字段包含字母 → 视为表头，解析列映射；否则视为数据，列顺序固定 `X,Y,Z,U`
3. 缺列按 0 填充

**新建单元测试** `pointsFileParser.spec.ts`，按三段式格式（测试前置 / 测试步骤 / 期待结果）覆盖：

| 用例 | 测试前置 | 测试步骤 | 期待结果 |
|---|---|---|---|
| 1. CSV 全 4 列 + 表头大小写混合 | 准备 `X,Y,Z,U\n1,2,3,4\n5,6,7,8` | 调用 `parsePointsFile` | 返回 2 点 `[{x:1,y:2,z:3,u:4},{x:5,y:6,z:7,u:8}]` |
| 2. CSV 表头含括号注释 | 准备 `X(mm),Y(mm),Z(mm),U(°)\n1,2,3,4` | 调用 | 正确解析 4 列 |
| 3. TSV 缺 U 列（验证 0 填充） | 准备 `X\tY\tZ\n1\t2\t3` | 调用 | 返回 `[{x:1,y:2,z:3,u:0}]` |
| 4. 无表头纯数据 | 准备 `1,2,3,4\n5,6,7,8` | 调用 | 默认 X,Y,Z,U 顺序解析 |
| 5. 含 `α` / `alpha` 列名 | 准备 `α,alpha\n1,2` | 调用 | 返回 `[{x:0,y:0,z:0,u:0}]`（仅 U 列映射，其余 0） |

**Acceptance criteria:**
- [ ] `normalizeAxisName` 通过 5 个用例
- [ ] `parsePointsFile` CSV/TSV 分隔符自适应
- [ ] 缺列按 0 填充，不抛错
- [ ] 单元测试全绿

**Verification:**
- [ ] `npm run typecheck` 零错误
- [ ] `npm run test` 通过（含新增 spec）

**Files likely touched:**
- `projects/windlabx4/apps/desktop-wails/frontend/src/shared/utils/pointsFileParser.ts`（新建）
- `projects/windlabx4/apps/desktop-wails/frontend/src/shared/utils/pointsFileParser.spec.ts`（新建）

**Estimated scope:** M（2 新建文件，约 80-120 行）

---

#### Task 9: TraversalLayoutStep 集成导入按钮 + i18n

**Description:** `TraversalLayoutStep.vue` 自定义点输入区添加导入按钮：

1. 加 `<UiButton>` "导入点位" 触发 `<input type="file" accept=".txt,.csv">` 隐藏点击
2. `importFile()`：FileReader API → `parsePointsFile(text)` → 二次确认对话框"将替换当前所有自定点，确认导入？" → `customPoints.value = result`
3. 提示用 Naive UI `useMessage` 或 `NModal`，不引入 alert

**i18n key 添加**（按 project_memory §16 国际化经验）：

| key | 中文 | 英文 |
|---|---|---|
| `traversal.importButton` | 导入点位 | Import Points |
| `traversal.importConfirmTitle` | 确认导入 | Confirm Import |
| `traversal.importConfirmBody` | 将替换当前所有自定点，确认导入？ | This will replace all current custom points. Continue? |
| `traversal.hAxis` | 横轴 | Horizontal Axis |
| `traversal.vAxis` | 纵轴 | Vertical Axis |
| `traversal.axisEmptyHint` | 该轴对数据全为 0（{pattern} 模式仅生成 X/Y） | This axis pair has no data ({pattern} pattern only generates X/Y) |

**Acceptance criteria:**
- [ ] 选择逗号分隔的 CSV 文件，按列名匹配成功
- [ ] 选择 Tab/空格分隔的 TXT 文件，按列名匹配成功
- [ ] 缺列（如只有 X,Y）时 Z/U 自动填 0
- [ ] 导入替换现有点位，确认对话框出现
- [ ] 导入后 UI 点位列表更新、画布实时刷新
- [ ] i18n key 在中英文语言下均生效

**Verification:**
- [ ] `npm run typecheck` 零错误
- [ ] `npm run build` 通过
- [ ] 手动准备 3 种测试文件（4 列完整 / 3 列缺 U / 2 列只有 X,Y）逐一导入验证
- [ ] 切换中英文语言，所有新文案正确显示

**Files likely touched:**
- `projects/windlabx4/apps/desktop-wails/frontend/src/components/traversal/TraversalLayoutStep.vue`
- `projects/windlabx4/apps/desktop-wails/frontend/src/locales/zh-CN.ts`（或对应 i18n 文件）
- `projects/windlabx4/apps/desktop-wails/frontend/src/locales/en-US.ts`

**Estimated scope:** S（3 文件，主要逻辑在 TraversalLayoutStep）

---

### Phase 5: 端到端验证

#### Task 10: 全链路验证

**Description:** 运行后端 full test suite + 前端 typecheck/build/test，确保没有 regression。手动验证 4 轴数据端到端流转。

**Verification:**
- [ ] `go build -buildvcs=false ./...` 零错误
- [ ] `go vet ./internal/...` 零告警
- [ ] `go test ./internal/... ./api/...` 全绿
- [ ] `npm run typecheck` 零错误
- [ ] `npm run build` 通过
- [ ] `npm run test` 通过（含新增 pointsFileParser 测试）

**手动端到端验证**（按 user_profile §20 测试用例三段式）：

| # | 测试前置 | 测试步骤 | 期待结果 |
|---|---|---|---|
| 1 | 配置 motion-controller 含 U 轴 profile；启动 WindLabX4 | 进入遍历测试 → 自定义模式 → 手动录入 4 轴点位 (1,2,3,4) 和 (5,6,7,8) → 保存配置 → 关闭配置面板 → 重新打开 | 4 轴数据完整恢复：`{x:1,y:2,z:3,u:4}` / `{x:5,y:6,z:7,u:8}` |
| 2 | 准备 CSV 文件 `X,Y,Z,U\n1,2,3,4\n5,6,7,8` | 进入自定义模式 → 点导入 → 选 CSV → 确认替换 | 点位列表显示 2 行 4 轴数据，画布显示 2 个点 |
| 3 | 已导入 4 轴点位 + motion-controller U 轴 profile 就绪 | 启动遍历采集 → 观察设备移动 | 设备按顺序移动到 (1,2,3,4) → (5,6,7,8) 四轴目标位置，CSV 文件含 X/Y/Z/U 4 列 |
| 4 | 进入 line 模式 + 切换 PointsPreview 轴对为 X-Z | 切换轴对 → 观察画布 | 画布显示一条水平线（所有点 Z=0），顶部提示"该轴对数据全为 0" |

**Estimated scope:** S（验证操作 + 手动 4 项）

---

## Rollback Strategy

- 单 commit 提交整个 plan 改动（含后端 4 文件 + 前端 5 文件 + 2 新建 + i18n），便于 `git revert` 一键回滚
- CSV 4 列扩展若引发现有工具解析问题：单独 revert Task 4 即可（表头和行写入回到 X/Y 两列），不影响 4 轴模型本身
- 前端导入工具为新增文件，回滚只需删除 `pointsFileParser.ts` + 移除 TraversalLayoutStep 的导入按钮

## Summary

| Phase | Tasks | 文件数 | 分类 |
|---|---|---|---|
| 1 | 1-4 | 4-5 | 后端（types, path, config, helpers, csv writer） |
| 2 | 5-6 | 2 | 前端类型 + 编辑行 |
| 3 | 7 | 2 | PointsPreview 改造 |
| 4 | 8-9 | 5（2 新建 + 1 i18n） | 文件导入 + i18n |
| 5 | 10 | - | 验证 |

**总影响范围**：后端 4-5 文件（含 CSV writer）；前端 5 文件 + 2 新建模块 + i18n。

## Review 修复记录（v2）

按 code review 报告修复 13 项：

| ID | 严重度 | 修复内容 |
|---|---|---|
| C1 | Critical | Task 8 明确 `normalizeAxisName` 算法为权威实现，Overview 删除矛盾描述 |
| C2 | Critical | 新增 Task 4 CSV writer 输出 4 轴列 |
| I1 | Important | Task 1 字面量清单按 Grep 实测重写，标注"named field 无需改" |
| I2 | Important | Task 7 补 PointsPreview 7 处改动点清单表格 |
| I3 | Important | Task 7 补图案模式退化方案（不禁用选项 + 提示文本） |
| I4 | Important | Task 6 明确加载逻辑 L280 spread 自动覆盖，无需改 |
| I5 | Important | Task 8 补 5 个单元测试用例（三段式格式） |
| I6 | Important | Task 9 补 i18n key 表（6 个 key，中英双语） |
| I7 | Important | Task 10 补 4 项端到端手动验证（三段式格式） |
| N1 | Note | Task 3 Verification 补 motion-controller U 轴 profile 配置验证 |
| N2 | Note | Overview 补 Estimated scope 量化标准 |
| N3 | Note | 新增 Rollback Strategy 章节 |
| N4 | Note | Task 3 补 U 轴语义注释 |
