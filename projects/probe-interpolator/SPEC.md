# Spec: Probe Interpolator (3/5/7-Hole Unified Desktop App)

## Objective

新建一个独立的 Wails v3 桌面程序 `projects/probe-interpolator`，把现有的 5 孔、3 孔插值程序与即将加入的 7 孔插值功能合并到同一个安装包。用户启动后选择探针类型（3 孔 / 5 孔 / 7 孔），进入对应专属工作区，会话内固定不变。

**User**: 风洞测试工程师，需要在多种探针类型之间切换工作，希望一个安装包解决所有探针插值需求。

**Success**: v0.1.0 一次交付 3/5/7 孔三种模式；启动选择页 → 进入该探针专属工作区；三种工作区共用 5 孔外观框架（顶栏/配色/按钮/结果表格风格），各自维护输入区与结果列；旧的 5 孔 / 3 孔独立程序标记 deprecated、保留代码与 release 历史、README/CHANGELOG 引导迁移到新程序。

## Why Now

- 5 孔、3 孔独立程序已发布到 0.2.x / 0.1.x，UI 框架与算法包都已成熟。
- `shared/algorithms/go/{fivehole,threehole,sevenhole}/interpolation/` 三个算法包均已存在并通过测试，合并成本最低。
- 7 孔算法文档（`docs/内部算法相关说明书/seven_hole_algorithm.html`）与 Go 实现已就绪，无需重新推导。

## Architecture

### 项目布局

```
projects/probe-interpolator/
├── SPEC.md
├── README.md
├── CHANGELOG.md
├── VERSION
├── apps/desktop-wails/
│   ├── main.go                      # Wails 入口
│   ├── go.mod                       # replace shared/{fivehole,threehole,sevenhole}
│   ├── wails.json
│   ├── Taskfile.yml
│   ├── appicon.ico / appicon.png
│   ├── backend/
│   │   ├── app.go                   # 总入口 App struct（持有三种工作区 service）
│   │   ├── probe_selector.go        # 启动选择页后端（GetAvailableProbes / SetActiveProbe）
│   │   ├── five_hole_service.go     # 5 孔工作区后端（从 five-hole-interpolator 迁移）
│   │   ├── three_hole_service.go    # 3 孔工作区后端（从 three-hole-interpolator 迁移）
│   │   ├── seven_hole_service.go    # 7 孔工作区后端（新建，适配 7 孔算法 API）
│   │   └── *_test.go
│   ├── frontend/
│   │   ├── index.html
│   │   ├── package.json
│   │   ├── vite.config.ts
│   │   ├── tsconfig.json
│   │   └── src/
│   │       ├── main.ts
│   │       ├── env.d.ts
│   │       ├── App.vue              # 顶层路由：选择页 ↔ 工作区
│   │       ├── components/
│   │       │   ├── ProbeSelectPage.vue   # 启动选择页（3/5/7 三按钮）
│   │       │   ├── FiveHoleWorkspace.vue # 5 孔工作区（迁移自 five-hole App.vue）
│   │       │   ├── ThreeHoleWorkspace.vue# 3 孔工作区（迁移自 three-hole App.vue）
│   │       │   ├── SevenHoleWorkspace.vue# 7 孔工作区（新建）
│   │       │   └── shared/               # 共用外观组件
│   │       │       ├── AppHeader.vue
│   │       │       ├── ResultTable.vue
│   │       │       └── FilePicker.vue
│   │       └── wails-adapter.ts
│   └── docs/
│       └── 用户说明书.html           # 三种探针合并说明书
└── releases/
    └── 0.1.0.md
```

### 共享算法包复用

| 探针 | 算法包路径 | 入口 API |
|---|---|---|
| 5 孔 | `shared/algorithms/go/fivehole/interpolation` | `NewMultiPrbInterpolator()` + `LoadPrbData([]PrbFileData, nil)` + `Calculate(InterpolationInput)` |
| 3 孔 | `shared/algorithms/go/threehole/interpolation` | 同 5 孔形态 |
| 7 孔 | `shared/algorithms/go/sevenhole/interpolation` | `NewSevenHolePrbInterpolator()` + `LoadInnerPrbLines(lines, source)` + `LoadOuterPrbLines(sector 1-6, lines, source)` + `Calculate(InterpolationInput)` |

**关键差异**：7 孔算法包没有统一 `LoadPrbData` 入口，需要后端先按文件名规则（`7.prb` 为内区、`1.prb`~`6.prb` 为外区扇区 1-6）解析用户选择的 7 个文件，再分别调用 `LoadInnerPrbLines` / `LoadOuterPrbLines`。

### 探针类型切换

启动 → `ProbeSelectPage` 三按钮（3/5/7 孔）→ 用户点击后前端调 `SetActiveProbe(kind)` → 后端返回该探针工作区初始状态 → 前端切换到对应 `XxxWorkspace.vue`。会话内固定，要换探针类型需重启程序。

## Tech Stack

| Component | Technology | Version |
|---|---|---|
| Backend | Go | 1.25 |
| Frontend | Vue 3 + TypeScript + Vite | ^3.5.14 / ^5.4.20 |
| Desktop Shell | Wails v3 | latest |
| Algorithm | 复用 `shared/algorithms/go/{fivehole,threehole,sevenhole}` | — |
| Test Framework | Go testing + testify | — |

## Input / Output Contracts

### 5 孔（与现有 `five-hole-interpolator` 完全一致）

**输入**：`P1 P2 P3 P4 P5 Patm Tatm PressureMode`（PressureMode ∈ `gauge`/`absolute`）
**.prb**：多文件选择，`NewMultiPrbInterpolator` 按马赫数组织
**输出**：`Alpha Beta MachNumber V Vx Vy Vz Velocity CAS SAT DynamicPressure Density P0 Ps IsValid Warning`（共 15 字段）
**.prb / CSV 格式**：与现有 5 孔程序完全兼容，老用户的 .prb 文件可直接加载

### 3 孔（与现有 `three-hole-interpolator` 完全一致）

**输入**：`P1 P2 P3 Patm Tatm`（P2 为中心孔；无 PressureMode）
**.prb**：单文件，格式 `CMa` / `Nalpha` / `Kb Kt Sb Alpha` 数据行
**输出**：`PtProbe PsProbe MachProbe AlphaProbe IterationCount IsValid Warning`（共 7 字段）
**.prb / CSV 格式**：与现有 3 孔程序完全兼容，老用户的 .prb 文件可直接加载

### 7 孔（新建）

**输入**：`P1 P2 P3 P4 P5 P6 P7 Patm Tatm`（P7 为中心孔；所有压力均为表压，无 PressureMode）
**.prb**：7 个文件，`7.prb`（内区小角度）+ `1.prb`~`6.prb`（外区大角度扇区 1-6）。用户一次性多选 7 个文件，后端按文件名（basename）匹配 sector。
**输出**：`Alpha Beta Theta Phi MachNumber Velocity DynamicPressure P0 Ps IsValid Warning`
**重要语义**：7 孔结果中 **Alpha = 侧滑角、Beta = 迎角**，与 5 孔结果中 Alpha=迎角、Beta=侧滑角的语义**反转**。前端 UI 标签必须按 7 孔语义显示，不能直接复用 5 孔的"Alpha=迎角"文案。
**大小角度模式**：算法包自动判定（基于 P1-P6 最大压力孔位置 + 多边形射线法），前端不需要让用户选择模式，但结果区要显示"本次使用小角度/大角度扇区 N"信息。
**theta/phi 字段**：算法包 `InterpolationResult` 返回 PRB 网格原始角度坐标 `Theta`/`Phi`（deg）——内区（小角度）模式下与 `Alpha`/`Beta` 同值；外区（大角度）模式下是探头坐标系下的俯仰角与滚转角，`Alpha`/`Beta` 是经 `convertThetaPhiToAlphaBeta` 投影到风洞坐标系后的角度。前端结果表格与 CSV 导出均展示 θ、Ψ 列，便于工程师诊断大角度模式下的探头真实气流偏角。

## UI Design

### 启动选择页 `ProbeSelectPage.vue`

- 三个大卡片按钮：3 孔 / 5 孔 / 7 孔，每个卡片含探针示意图 + 名称 + 简短说明
- 配色继承 5 孔现有深色顶栏 + 浅色主区风格
- 选择后不可返回（会话内固定）；要换需重启

### 工作区 `XxxWorkspace.vue`

三套工作区共用以下组件（从 5 孔 App.vue 抽取）：
- `AppHeader`：顶栏（程序名 + 当前探针类型标签 + 帮助按钮）
- `ResultTable`：结果表格（列配置由各工作区自行传入）
- `FilePicker`：文件选择按钮（标签和过滤规则由各工作区自行配置）

各工作区独有：
- **5 孔**：5 个压力输入框 + Patm/Tatm + PressureMode 下拉 + 单点计算 + 批量 CSV 导入/导出
- **3 孔**：3 个压力输入框 + Patm/Tatm + 单点计算 + 批量 CSV 导入/导出
- **7 孔**：7 个压力输入框 + Patm/Tatm + 单点计算 + 批量 CSV 导入/导出 + 当前模式显示（小角度/大角度扇区 N）

### 文案与国际化

- 中文界面（与 5/3 孔现有程序一致）
- 7 孔的 Alpha/Beta 字段在 UI 上显示为"侧滑角 α / 迎角 β"，5 孔沿用"迎角 α / 侧滑 β"（保持各探针原习惯，不强行统一）

## Deprecated 旧程序处理

`projects/five-hole-interpolator/` 和 `projects/three-hole-interpolator/`：

1. **保留代码与 release 历史**：不删除任何文件、不删 `releases/` 目录
2. **README 顶部加废弃声明**：标注"本程序已 deprecated，建议迁移到 `probe-interpolator`"，给出新程序下载链接占位
3. **CHANGELOG 最新版本追加 deprecation 说明**：标注"自 vX.Y.Z 起，本程序进入维护模式，仅修关键 bug，不再新增功能"
4. **不再发新版本**（除非有 Critical bug 修复）
5. **VERSION 不动**（不人为 bump）

## Commands

```powershell
# 进入项目
cd projects\probe-interpolator\apps\desktop-wails

# 开发模式
go run github.com/wailsapp/wails/v3/cmd/wails3 dev

# 后端测试
go test ./...

# 算法包测试（共享）
cd shared\algorithms\go\fivehole\interpolation   ; go test ./...
cd shared\algorithms\go\threehole\interpolation   ; go test ./...
cd shared\algorithms\go\sevenhole\interpolation   ; go test ./...

# 前端构建
cd projects\probe-interpolator\apps\desktop-wails\frontend
npm run build
```

## Boundaries

**Always**:
- 复用 `shared/algorithms/go/{fivehole,threehole,sevenhole}/interpolation/` 算法包，不重写算法
- 后端每种探针一个 service 文件，互不耦合
- 前端三种工作区组件独立，共用外观组件抽到 `components/shared/`
- 7 孔 Alpha/Beta 语义按算法包定义（Alpha=侧滑、Beta=迎角），UI 文案与之对齐
- 输入校验放在 backend 边界（与 5 孔现有风格一致）

**Architecture note (项目级覆盖)**:
- 本项目采用扁平 `apps/desktop-wails/{frontend,backend}` 结构，业务逻辑直接放在 `backend/*_service.go`，**不**套六边形 `{core,ports,usecase,adapters}` 分层。
- 这与工作区根 [AGENTS.md](../../../AGENTS.md) §Hard Constraints 中 `apps/desktop-wails/backend/ | zero business logic` 的硬约束存在表面冲突，但 `scripts/validate-structure.ps1` 的 Go 架构校验仅针对 `projects/wind-daq/services/api-go/internal/{core,usecase,ports,adapters}`，**不覆盖 `projects/probe-interpolator/`**。
- 决策理由：单项目交付的桌面插值工具，三种探针 service 互相独立、无硬件依赖、无外部 I/O 复杂度，引入 usecase/ports 层会徒增样板代码而无收益。此决策为 SPEC 显式选择，未来若新增项目级硬件交互或多服务编排需重新评估分层。

**Ask first**:
- 修改任何共享算法包的公开 API（影响 wind-daq 和旧独立程序）
- 改动 .prb / CSV 文件格式（影响老用户数据兼容）
- 引入新的第三方依赖

**Never**:
- 从 `projects/wind-daq` 内部包 import 任何代码（保持独立交付）
- 删除旧的 5/3 孔独立程序代码与 release 历史
- 在旧独立程序里加新功能
- 强制三个探针的输入区/结果列完全对齐（7 孔的大小角度模式是其独有特性）
- 在会话内允许切换探针类型（要换重开程序）
- 把 7 孔校准模块（wind-daq 里的 `calibration/seven_hole*.go`）混进来——本程序只做"插值"，不做"校准"

## Success Criteria

- [ ] `projects/probe-interpolator/` 可独立 `wails3 dev` 启动
- [ ] 启动后看到选择页，3 个探针按钮可点
- [ ] 5 孔工作区：加载多 .prb、单点计算、批量 CSV 导入/导出，结果与旧 5 孔程序一致（用相同 .prb + 输入对比）
- [ ] 3 孔工作区：加载单 .prb、单点计算、批量 CSV 导入/导出，结果与旧 3 孔程序一致
- [ ] 7 孔工作区：加载 7 个 .prb、单点计算、批量 CSV 导入/导出，大小角度模式自动判定并显示
- [ ] 7 孔 UI 文案 Alpha=侧滑角、Beta=迎角（不与 5 孔混淆）
- [ ] `go test ./...` + `npm run typecheck` + `npm run build` 全绿
- [ ] 旧 5 孔 / 3 孔独立程序 README 顶部有 deprecation 声明，代码可继续编译
- [ ] v0.1.0 release note 完成

## Open Questions

1. 7 孔的 .prb 文件名规则是否固定为 `7.prb` / `1.prb`~`6.prb`？还是支持用户任意命名（后端按内容识别）？默认按文件名 basename 识别，若用户文件名不规范则报错提示。
2. 7 孔批量 CSV 导入的输入列是否需要包含 `hole_index` 或时间戳列？默认只要求 `P1`~`P7` + `Patm` + `Tatm`，其他列忽略。
3. 启动选择页是否需要"上次使用过的探针类型"记忆？默认不记忆（每次启动都让用户主动选，避免误操作）。
4. 帮助文档是合并成一份"三种探针说明书"还是各探针独立一份？默认合并成一份，按探针类型分章节。
