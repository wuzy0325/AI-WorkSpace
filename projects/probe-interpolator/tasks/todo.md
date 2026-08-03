# Todo: Probe Interpolator v0.1.0

> 详细计划见 [plan.md](./plan.md)，SPEC 见 [../SPEC.md](../SPEC.md)
> 每个任务完成后勾选，并跑该任务的 Verification 步骤

---

## Phase 1: 骨架与启动选择页

### Task 1: 拷贝 5 孔目录骨架并改名

**Description**: 以 `projects/five-hole-interpolator/apps/desktop-wails` 为模板，拷贝到 `projects/probe-interpolator/apps/desktop-wails`，调整 module name、wails.json、main.go、Taskfile.yml 中的命名引用。后端只保留一个空壳 `App` struct（持有三个 service 字段但先不实现），前端 `App.vue` 先放占位内容。

**Acceptance criteria**:
- [ ] `projects/probe-interpolator/` 目录结构存在（SPEC § Architecture 项目布局）
- [ ] `go.mod` module name 改为 `probe-interpolator/apps/desktop-wails`，replace 三条算法包路径（fivehole + threehole + sevenhole）
- [ ] `wails.json` name/outputfilename 改为 `probe-interpolator`，version 改 `0.1.0`
- [ ] `main.go` 改 import 路径、Name/Title 改为"探针插值器"、Description 改为"3/5/7-hole probe interpolation"
- [ ] `Taskfile.yml` 中所有 `five-hole-interpolator` 字串改为 `probe-interpolator`，`check-bindings` 的 `-Projects` 参数改为 `probe-interpolator`
- [ ] `frontend/package.json` name 改为 `probe-interpolator-frontend`
- [ ] `appicon.ico` / `appicon.png` 沿用 5 孔现有图标（v0.1.0 不重新设计）
- [ ] 旧 5 孔 `app.go` 内容**不要**拷过来（v0.1.0 后端从空壳开始按 service 拆分）
- [ ] `frontend/src/App.vue` 先放最小占位（"Probe Interpolator v0.1.0" 文字），后续 Task 2 替换

**Verification**:
- [ ] `cd projects\probe-interpolator\apps\desktop-wails && go build .` 通过
- [ ] `cd projects\probe-interpolator\apps\desktop-wails\frontend && npm install && npm run build` 通过
- [ ] `cd projects\probe-interpolator\apps\desktop-wails\frontend && npm run typecheck` 通过

**Dependencies**: None

**Files likely touched**:
- `projects/probe-interpolator/apps/desktop-wails/go.mod`
- `projects/probe-interpolator/apps/desktop-wails/wails.json`
- `projects/probe-interpolator/apps/desktop-wails/main.go`
- `projects/probe-interpolator/apps/desktop-wails/Taskfile.yml`
- `projects/probe-interpolator/apps/desktop-wails/backend/app.go`（空壳）
- `projects/probe-interpolator/apps/desktop-wails/frontend/package.json`
- `projects/probe-interpolator/apps/desktop-wails/frontend/src/App.vue`（占位）
- `projects/probe-interpolator/VERSION`（内容 `0.1.0`）

**Estimated scope**: M (5-7 files)

---

### Task 2: 实现启动选择页（后端 + 前端路由）

**Description**: 后端新增 `probe_selector.go`，提供 `GetAvailableProbes()` 返回三种探针的元信息（kind/name/description/icon）和 `SetActiveProbe(kind)` 设置当前激活探针（会话内固定，重复调用返回错误）。前端 `App.vue` 改为顶层路由：根据 `activeProbe` 状态显示 `ProbeSelectPage.vue` 或对应工作区组件（工作区组件先放占位）。

**Acceptance criteria**:
- [ ] `backend/probe_selector.go` 存在，定义 `ProbeKind` 常量（`five`/`three`/`seven`）、`ProbeInfo` struct、`GetAvailableProbes() []ProbeInfo`、`SetActiveProbe(kind ProbeKind) error`、`GetActiveProbe() (ProbeKind, error)`
- [ ] `SetActiveProbe` 第二次调用（已设置过）返回错误"会话内探针类型已固定，请重启程序切换"
- [ ] `backend/app.go` 持有 `activeProbe ProbeKind` 字段（带 `sync.RWMutex` 保护）
- [ ] 前端 `ProbeSelectPage.vue` 渲染三个大卡片按钮（3 孔 / 5 孔 / 7 孔），点击调 `SetActiveProbe` 后切到对应工作区
- [ ] `App.vue` 顶层根据 `activeProbe` 状态路由：未设置 → `ProbeSelectPage`；已设置 → `XxxWorkspace`（先放占位 `<div>5 孔工作区（待实现）</div>` 等）
- [ ] 选择页配色继承 5 孔深色顶栏 + 浅色主区风格
- [ ] Wails binding 同步：`wails3 generate bindings -silent` 后 frontend 能正确调用 `GetAvailableProbes` / `SetActiveProbe` / `GetActiveProbe`

**Verification**:
- [ ] `cd projects\probe-interpolator\apps\desktop-wails && go test ./backend/...` 通过
- [ ] `cd projects\probe-interpolator\apps\desktop-wails && go build .` 通过
- [ ] `wails3 dev` 启动后看到三按钮选择页
- [ ] 点击任一按钮后切到占位工作区，刷新程序后回到选择页
- [ ] `powershell -ExecutionPolicy Bypass -File scripts\check-wails-bindings.ps1 -Projects probe-interpolator` 通过
- [ ] `npm run typecheck` + `npm run build` 全绿

**Dependencies**: Task 1

**Files likely touched**:
- `projects/probe-interpolator/apps/desktop-wails/backend/app.go`
- `projects/probe-interpolator/apps/desktop-wails/backend/probe_selector.go`（新建）
- `projects/probe-interpolator/apps/desktop-wails/backend/probe_selector_test.go`（新建）
- `projects/probe-interpolator/apps/desktop-wails/frontend/src/App.vue`
- `projects/probe-interpolator/apps/desktop-wails/frontend/src/components/ProbeSelectPage.vue`（新建）
- `projects/probe-interpolator/apps/desktop-wails/frontend/src/wails-adapter.ts`

**Estimated scope**: M (5 files)

---

### Checkpoint: 骨架可启动
- [ ] `wails3 dev` 能起来，看到三按钮选择页
- [ ] `go build` + `npm typecheck` + `npm build` 全绿
- [ ] Wails binding 同步检查通过

---

## Phase 2: 三种探针端到端

### Task 3: 5 孔工作区端到端

**Description**: 后端新增 `five_hole_service.go`，迁移 5 孔现有 `app.go` 的全部逻辑（LoadPrbFiles / IsPrbLoaded / GetPrbFiles / GetMachRange / Calculate / BatchCalculate / ImportCsvData / OpenHelpDoc），但作为 `App` 的方法（不是独立 service struct），与 `probe_selector` 共存。前端把 5 孔现有 `App.vue`（879 行）整段拷到 `FiveHoleWorkspace.vue`，调整 Wails 调用路径。

**Acceptance criteria**:
- [ ] `backend/five_hole_service.go` 存在，包含 5 孔全部 8 个方法，签名与 5 孔现有 `app.go` 一致
- [ ] `InterpolationInput` / `InterpolationResult` 等 struct 从 5 孔现有定义拷贝（保持 JSON 字段一致）
- [ ] `toCoreInput` / `toAppResult` 等转换函数迁移
- [ ] `OpenHelpDoc` 的 `helpDocFileName` 常量与路径搜索逻辑保持不变（5 孔的 `getHelpDocPath` 函数）
- [ ] 前端 `FiveHoleWorkspace.vue` 是 5 孔现有 `App.vue` 的整段拷贝，仅调整：Wails 调用从 `import { ... } from '../wailsjs/go/backend/App'` 调整为新 binding 路径
- [ ] `App.vue` 在 `activeProbe === 'five'` 时渲染 `<FiveHoleWorkspace />`
- [ ] Wails binding 同步

**Verification**:
- [ ] `go test ./backend/...` 通过（包括从 5 孔现有 `app_test.go` 迁移的测试用例）
- [ ] `go build .` 通过
- [ ] `wails3 dev` 启动 → 选 5 孔 → 加载多个 5 孔 .prb → 单点计算 → 结果与旧 5 孔程序一致（用相同 .prb + 相同输入对比）
- [ ] 批量 CSV 导入 + 导出可用，导出 CSV 与旧 5 孔程序列一致
- [ ] `check-wails-bindings.ps1 -Projects probe-interpolator` 通过
- [ ] `npm run typecheck` + `npm run build` 全绿

**Dependencies**: Task 2

**Files likely touched**:
- `projects/probe-interpolator/apps/desktop-wails/backend/five_hole_service.go`（新建）
- `projects/probe-interpolator/apps/desktop-wails/backend/five_hole_service_test.go`（新建，从 5 孔 `app_test.go` 迁移）
- `projects/probe-interpolator/apps/desktop-wails/backend/types.go`（新建，5 孔的 struct 定义）
- `projects/probe-interpolator/apps/desktop-wails/frontend/src/components/FiveHoleWorkspace.vue`（新建）
- `projects/probe-interpolator/apps/desktop-wails/frontend/src/App.vue`（更新路由）
- `projects/probe-interpolator/apps/desktop-wails/docs/用户说明书.html`（先从 5 孔拷过来）

**Estimated scope**: M (5 files)

---

### Task 4: 3 孔工作区端到端

**Description**: 后端新增 `three_hole_service.go`，迁移 3 孔现有 `app.go` 的全部逻辑。前端新建 `ThreeHoleWorkspace.vue`，从 3 孔现有 `App.vue` 整段拷贝调整。

**Acceptance criteria**:
- [ ] `backend/three_hole_service.go` 存在，包含 3 孔全部方法（LoadPrbFile 单文件版 / Calculate / BatchCalculate / ImportCsvData / OpenHelpDoc）
- [ ] 3 孔的 `InterpolationInput`（P1/P2/P3/Patm/Tatm，无 PressureMode）与 `InterpolationResult`（PtProbe/PsProbe/MachProbe/AlphaProbe/IterationCount）字段定义与 3 孔现有程序一致
- [ ] 3 孔的 .prb 解析逻辑（`CMa` / `Nalpha` / `Kb Kt Sb Alpha` 格式）从 3 孔现有代码迁移
- [ ] 前端 `ThreeHoleWorkspace.vue` 是 3 孔现有 `App.vue` 整段拷贝，调整 Wails binding 路径
- [ ] `App.vue` 在 `activeProbe === 'three'` 时渲染 `<ThreeHoleWorkspace />`
- [ ] 3 孔的 helpDoc 路径搜索与 5 孔共用 `getHelpDocPath`（避免重复代码）

**Verification**:
- [ ] `go test ./backend/...` 通过（含 3 孔迁移的测试用例）
- [ ] `go build .` 通过
- [ ] `wails3 dev` → 选 3 孔 → 加载单 .prb → 单点计算 → 结果与旧 3 孔程序一致
- [ ] 批量 CSV 导入 + 导出可用
- [ ] `check-wails-bindings.ps1` 通过
- [ ] `npm run typecheck` + `npm run build` 全绿

**Dependencies**: Task 2（与 Task 3 可并行，但建议 Task 3 先做作为参照）

**Files likely touched**:
- `projects/probe-interpolator/apps/desktop-wails/backend/three_hole_service.go`（新建）
- `projects/probe-interpolator/apps/desktop-wails/backend/three_hole_service_test.go`（新建）
- `projects/probe-interpolator/apps/desktop-wails/backend/three_hole_types.go`（新建，3 孔 struct 定义）
- `projects/probe-interpolator/apps/desktop-wails/frontend/src/components/ThreeHoleWorkspace.vue`（新建）
- `projects/probe-interpolator/apps/desktop-wails/frontend/src/App.vue`（更新路由）

**Estimated scope**: M (5 files)

---

### Task 5: 7 孔工作区端到端

**Description**: 后端新增 `seven_hole_service.go`，适配 7 孔算法包 API（`NewSevenHolePrbInterpolator` + `LoadInnerPrbLines` + `LoadOuterPrbLines`）。前端新建 `SevenHoleWorkspace.vue`，基于 5 孔外观适配 7 孔输入（7 个压力孔）与结果（Alpha=侧滑、Beta=迎角）。

**Acceptance criteria**:
- [ ] `backend/seven_hole_service.go` 存在，提供 `LoadSevenHolePrbFiles`（多选 7 个文件，按 basename 匹配 `7.prb` → inner、`1.prb`~`6.prb` → outer sector 1-6）
- [ ] 文件名不匹配时报错"未找到 7.prb 内区文件"或"未找到 N.prb 外区扇区 N 文件"
- [ ] `IsSevenHolePrbLoaded` / `GetSevenHoleValidRange` / `CalculateSevenHole` / `BatchCalculateSevenHole` / `ImportSevenHoleCsv` 方法存在
- [ ] 7 孔 `InterpolationInput` 字段：`P1 P2 P3 P4 P5 P6 P7 Patm Tatm`（无 PressureMode，全部表压）
- [ ] 7 孔 `InterpolationResult` 字段：`Alpha Beta MachNumber Velocity DynamicPressure P0 Ps IsValid Warning`
- [ ] 前端 `SevenHoleWorkspace.vue` UI 文案：Alpha 显示为"侧滑角 α"、Beta 显示为"迎角 β"（**不与 5 孔的"迎角 α / 侧滑 β"混淆**）
- [ ] 结果区显示"本次使用：小角度模式"或"本次使用：大角度扇区 N"信息（从算法包返回的 `Warning` 字段或新增 `Mode` 字段提取）
- [ ] `App.vue` 在 `activeProbe === 'seven'` 时渲染 `<SevenHoleWorkspace />`
- [ ] 批量 CSV 导入只要求 `P1`~`P7` + `Patm` + `Tatm` 列，其他列忽略

**Verification**:
- [ ] `go test ./backend/...` 通过（含 7 孔 service 测试，覆盖文件名匹配、inner/outer 加载、计算结果）
- [ ] `go build .` 通过
- [ ] 用 wind-daq 项目的 7 孔 .prb 测试数据 + 相同输入，对比新程序与 `shared/algorithms/go/sevenhole/interpolation` 的 golden test 结果一致
- [ ] `wails3 dev` → 选 7 孔 → 加载 7 个 .prb → 单点计算 → 大小角度模式自动判定并显示
- [ ] 批量 CSV 导入 + 导出可用
- [ ] UI 文案 code review：确认 Alpha 显示"侧滑角 α"、Beta 显示"迎角 β"
- [ ] `check-wails-bindings.ps1` 通过
- [ ] `npm run typecheck` + `npm run build` 全绿

**Dependencies**: Task 2（与 Task 3/4 可并行）

**Files likely touched**:
- `projects/probe-interpolator/apps/desktop-wails/backend/seven_hole_service.go`（新建）
- `projects/probe-interpolator/apps/desktop-wails/backend/seven_hole_service_test.go`（新建）
- `projects/probe-interpolator/apps/desktop-wails/backend/seven_hole_types.go`（新建）
- `projects/probe-interpolator/apps/desktop-wails/frontend/src/components/SevenHoleWorkspace.vue`（新建）
- `projects/probe-interpolator/apps/desktop-wails/frontend/src/App.vue`（更新路由）

**Estimated scope**: L (5 files，但 7 孔 service 逻辑较复杂，工作量大于 Task 3/4)

---

### Checkpoint: 三种探针端到端可用
- [ ] 5 孔加载多 .prb + 单点 + 批量 CSV，结果与旧 5 孔程序一致
- [ ] 3 孔加载单 .prb + 单点 + 批量 CSV，结果与旧 3 孔程序一致
- [ ] 7 孔加载 7 个 .prb + 单点 + 批量 CSV，大小角度模式自动判定显示
- [ ] 7 孔 UI 文案 Alpha=侧滑、Beta=迎角
- [ ] 全部 `go test` + `npm typecheck` + `npm build` 全绿

---

## Phase 3: 收尾

### Task 6: 抽取共用组件（可选）

**Description**: 评估三个工作区组件（`FiveHoleWorkspace.vue` / `ThreeHoleWorkspace.vue` / `SevenHoleWorkspace.vue`）的实际重复度，若有明显重复（顶栏、结果表格、文件选择按钮），抽取到 `components/shared/`。若重复度低则跳过此任务。

**Acceptance criteria**:
- [ ] 评估三个工作区的重复代码块（顶栏、结果表格、文件选择按钮等）
- [ ] 若重复度 >30%：抽取 `shared/AppHeader.vue` / `shared/ResultTable.vue` / `shared/FilePicker.vue`，三个工作区改用共用组件
- [ ] 若重复度 ≤30%：在 `tasks/plan.md` 记录"评估结论：重复度低，不抽取"，跳过此任务
- [ ] 抽取后三个工作区行为不变（回归测试三个工作区仍可用）

**Verification**:
- [ ] `npm run typecheck` + `npm run build` 全绿
- [ ] 三个工作区手动回归：加载 .prb + 单点计算 + 批量 CSV 仍可用

**Dependencies**: Task 3, Task 4, Task 5

**Files likely touched**:
- `projects/probe-interpolator/apps/desktop-wails/frontend/src/components/shared/AppHeader.vue`（新建，若抽取）
- `projects/probe-interpolator/apps/desktop-wails/frontend/src/components/shared/ResultTable.vue`（新建，若抽取）
- `projects/probe-interpolator/apps/desktop-wails/frontend/src/components/shared/FilePicker.vue`（新建，若抽取）
- `projects/probe-interpolator/apps/desktop-wails/frontend/src/components/FiveHoleWorkspace.vue`（重构，若抽取）
- `projects/probe-interpolator/apps/desktop-wails/frontend/src/components/ThreeHoleWorkspace.vue`（重构，若抽取）
- `projects/probe-interpolator/apps/desktop-wails/frontend/src/components/SevenHoleWorkspace.vue`（重构，若抽取）

**Estimated scope**: M（若抽取）/ S（若跳过）

---

### Task 7: 旧 5/3 孔程序 deprecation 标记

**Description**: 在 `projects/five-hole-interpolator/README.md` 和 `projects/three-hole-interpolator/README.md` 顶部加 deprecation 声明，CHANGELOG 最新版本追加 deprecation 说明。不删除任何代码、不改 VERSION。

**Acceptance criteria**:
- [ ] `projects/five-hole-interpolator/README.md` 顶部加 `> ⚠ 本程序已 deprecated，建议迁移到 [probe-interpolator](../probe-interpolator/)` 声明
- [ ] `projects/three-hole-interpolator/README.md` 顶部加同样声明
- [ ] `projects/five-hole-interpolator/CHANGELOG.md` 最新版本（0.2.2）下追加"自 0.2.2 起，本程序进入维护模式，仅修关键 bug，不再新增功能。建议迁移到 probe-interpolator"
- [ ] `projects/three-hole-interpolator/CHANGELOG.md` 最新版本（0.1.1）下追加同样说明
- [ ] 两个旧程序的 VERSION 文件不动
- [ ] 两个旧程序的代码、releases/ 目录、SPEC.md 不动
- [ ] 旧 5 孔程序仍可 `go build` + `npm build` 编译通过（确认未被破坏）

**Verification**:
- [ ] `cd projects\five-hole-interpolator\apps\desktop-wails && go build .` 通过
- [ ] `cd projects\three-hole-interpolator\apps\desktop-wails && go build .` 通过
- [ ] README 顶部 deprecation 声明可见
- [ ] CHANGELOG 最新版本有 deprecation 说明

**Dependencies**: None（可与 Task 6 并行）

**Files likely touched**:
- `projects/five-hole-interpolator/README.md`
- `projects/five-hole-interpolator/CHANGELOG.md`
- `projects/three-hole-interpolator/README.md`
- `projects/three-hole-interpolator/CHANGELOG.md`

**Estimated scope**: S (4 files)

---

### Task 8: v0.1.0 release note + 最终验证

**Description**: 写 `projects/probe-interpolator/releases/0.1.0.md` release note，跑全套验证（go test + go vet + npm typecheck + npm build + 手动三种探针端到端），完成 SPEC Success Criteria 全部勾选。

**Acceptance criteria**:
- [ ] `projects/probe-interpolator/releases/0.1.0.md` 存在，包含：新功能清单（3/5/7 孔统一）、使用说明、已知限制、下载链接占位
- [ ] `projects/probe-interpolator/CHANGELOG.md` 存在，0.1.0 条目完整
- [ ] `projects/probe-interpolator/README.md` 存在，含简短产品说明 + 启动选择页截图占位 + 三种探针使用链接
- [ ] SPEC § Success Criteria 全部勾选
- [ ] `cd projects\probe-interpolator\apps\desktop-wails && go test ./...` 通过
- [ ] `cd projects\probe-interpolator\apps\desktop-wails && go vet ./...` 通过
- [ ] `cd projects\probe-interpolator\apps\desktop-wails\frontend && npm run typecheck` 通过
- [ ] `cd projects\probe-interpolator\apps\desktop-wails\frontend && npm run build` 通过
- [ ] `powershell -ExecutionPolicy Bypass -File scripts\validate-structure.ps1` 通过
- [ ] 手动验证：5 孔 / 3 孔 / 7 孔三种工作区端到端可用，结果与旧程序/算法包 golden test 一致
- [ ] 7 孔 UI 文案最终确认：Alpha=侧滑角、Beta=迎角

**Verification**:
- [ ] 上述全部 acceptance criteria 勾选完成
- [ ] 与用户确认 v0.1.0 可发布

**Dependencies**: Task 6, Task 7

**Files likely touched**:
- `projects/probe-interpolator/releases/0.1.0.md`（新建）
- `projects/probe-interpolator/CHANGELOG.md`（新建）
- `projects/probe-interpolator/README.md`（新建）
- `projects/probe-interpolator/SPEC.md`（更新 Success Criteria 勾选状态）

**Estimated scope**: S (3-4 files)

---

### Checkpoint: Complete
- [ ] SPEC Success Criteria 全部勾选
- [ ] `go test ./...` + `go vet ./...` + `npm typecheck` + `npm build` 全绿
- [ ] 旧 5/3 孔程序 README 顶部有 deprecation 声明
- [ ] v0.1.0 release note 完成
- [ ] 与用户确认可发布
