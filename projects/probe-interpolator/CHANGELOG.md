# Changelog

## [0.4.1] - 2026-08-19

### Fixed

- **结果表头希腊字母被 CSS 大写成拉丁字母外观**：`.data-table th` 的 `text-transform: uppercase` 会把 `α` 渲染成与 `A` 几乎相同的 `Α`、`β` 渲染成与 `B` 几乎相同的 `Β`（`θ`→`Θ`），但复制/导出仍取原始字符，造成“看着是 A/B，拷贝出来是 α/β”的观感矛盾。三孔/五孔/七孔结果表统一取消表头大写转换，显示与复制内容一致。

### Compatibility

- 配置文件格式：兼容。
- 数据文件格式：兼容（无字段变更）。
- API 契约：兼容。

### Verification

- `npm run typecheck` / `npm run build`: passed
- `go test ./...`（backend）: passed
- `task release`: passed
- `makensis`: passed
- `task archive-release`: passed

### Known Issues

- 安装包未进行 Authenticode 数字签名。
- 内嵌 exe 的 ProductVersion 资源为空（wails3 v3.0.0-alpha2.106 既有行为），installer 的 ProductVersion 为 `0.4.1`。

## [0.4.0] - 2026-08-19

### Added

- **七孔外区 θ 角度外推**（`shared/algorithms/go/sevenhole/interpolation`，新文件 `outer_extrapolation.go`）：外区两候选扇区均未命中时，用第一候选扇区最外层 θ 单元（θ∈[θMax-5, θMax]）4 角点仿射反演 `(ka,kb)→(θ,φ)`，θ 可超过校准上限 45°；计算用 θ 钳制到 θMax+5°，展示保留 θ_raw。外推结果 `IsValid=true` + 非空 Warning（标注外推点、上限、计算 θ），UI 提示用户自行判断有效性。
- **内区命中但 Pt<Ps 的回退**：内区多边形命中但解析出 `Pt<Ps`（物理异常）时不再直接中止，继续尝试外区两候选扇区与外推路径，全部失败才返回内区原始错误（不静默取绝对值）。
- 实测效果：`102原始.csv`（85 m/s PRB）61 行全部可算，越界行 theta_raw 46°~70° 并带外推 Warning。

### Compatibility

- 配置/数据文件格式：兼容。
- API 契约：兼容（`InterpolationResult` 无字段变更）；越界行为由"无效"变为"可显示 + Warning"，属预期变更。
- 正常网格内路径：数值零回归（golden 435 点对拍不变）。
- golden 测试：越界样本按 spec §3.3 分类更新（θ 外边界越界 → 外推结果断言，其余保持 IsValid=false）。

### Verification

- `go test ./...`（shared/algorithms/go/sevenhole/interpolation，含 golden 对拍与新增外推用例）: passed
- `go test ./backend/ -run SevenHole`（probe-interpolator）: passed
- `npm run typecheck` / `npm run build`: passed
- `task release`: passed
- `makensis`: passed
- `task archive-release`: passed

### Known Issues

- 安装包未进行 Authenticode 数字签名。
- 内嵌 exe 的 ProductVersion 资源为空（wails3 v3.0.0-alpha2.106 既有行为），installer 的 ProductVersion 为 `0.4.0`。
- 外推结果（θ>45°）无校准数据支撑，误差随 θ 增大而增大；以 Warning 标注，有效性由用户自行判断。

## [0.3.0] - 2026-08-05

### Fixed

- **七孔校准 CSV 系数全精度重算**（对 `shared/algorithms/go/sevenhole/interpolation`）：系数（ka/kb/cpt/cps）不再直接采用 CSV 中 col12..15 的 3 位小数 K 列，改为由原始压力列（col3=P0、col4=Ps、col5..11=P1..P7）在 float64 全精度下按公式重算（`recomputeSevenHoleInnerCoeffs` / `recomputeSevenHoleOuterCoeffs`）。历史 3dp K 列误差最大约 5e-4，全精度重算误差约 1e-6。
- **七孔网格节点精确往返修复**（对 `shared/algorithms/go/sevenhole/interpolation`）：
  - 内区 13×13 网格 21 个边界节点此前因 3dp 截断（~5e-4 偏差）被判定在边界多边形外，误路由到大角度区（错误外插结果）；现正确路由到内区并精确命中网格节点。
  - `Calculate` 新增外区节点直命中短路径 `findExactOuterCalibrationNode`，内/外区插值新增网格点直命中兜底（`innerFindGridPointByKaKb` / `outerFindGridPointByKaKb`）。
  - 修复自提取 PRB 反推场景：从校准 CSV 重算生成的 PRB，其压力行能精确反推出原网格角度，不再因浮点噪声导致 `locateInvertAB` 漏判网格边界点。
- 新增 NaN/Inf 压力值拒绝（CSV 加载时），避免脏数据静默进入网格。

### Compatibility

- 配置文件格式：兼容。
- 数据文件格式：CSV 校准文件兼容（列位置契约不变）；历史 3 位小数 K 列（col12..15）不再参与网格构建，仅用于表头诊断。
- 历史 PRB 文件：旧 3dp PRB 与新全精度网格存在约 5e-4 差异，建议重新生成后加载。
- API 契约：兼容（`InterpolationResult` 无字段变更）。

### Verification

- `go test ./...`（shared/algorithms/go/sevenhole/interpolation）: passed
- `go test ./backend/ -run SevenHole`（probe-interpolator）: passed
- `go test ./internal/adapters/interpolation/ -run SevenHole`（wind-daq adapter）: passed
- `npm run typecheck` / `npm run build`: passed
- `task release`: passed
- `makensis`: passed
- `task archive-release`: passed

### Known Issues

- 安装包未进行 Authenticode 数字签名。
- 内嵌 exe 的 ProductVersion 资源为空（wails3 v3.0.0-alpha2.106 既有行为），installer 的 ProductVersion 为 `0.3.0`。

## [0.2.2] - 2026-08-05

### Added

- **三孔插值结果新增气流速度 Velocity 输出**（对应 `shared/algorithms/go/threehole/interpolation`，与 probe-interpolator-miniprogram 同源同步）：`InterpolationResult` 新增 `Velocity` 字段（m/s），由 `MachNumber` 与 `Tatm` 经 `V = Ma · sqrt(γ · R · T_K)` 推导（R=287 J/(kg·K)，γ 复用 `calcGamma` 温度修正）。
  - 业务背景：三孔探针风洞验证需要"速度误差范围"作为验收指标，原先算法只输出 Ma，调用方需各自离线换算 V，跨实现一致性无法保证。
  - 兜底语义：Velocity 严格跟随 MachNumber——Ma 有效或兜底（initMa/currentMa）时给出对应速度，Ma 为 0/NaN（输入非法、calcMach 失败）时返回 0。
  - 5 个返回点全部填充 Velocity，与现有 MachNumber 行为对齐。
  - 数值示例：Ma=0.8 / Tatm=20℃ → V≈274.55 m/s；Tatm=-40℃ → V≈245.90 m/s。

### Compatibility

- 配置文件格式：兼容。
- 数据文件格式：兼容（PRB / CSV 输入输出无字段变更；批量结果 JSON 新增 `velocity` 字段为可选，旧调用方无需改动）。
- API 契约：兼容（`InterpolationResult` 新增字段为可选）。
- 校准文件加载行为：不变。

### Verification

- `go vet ./...`（shared/algorithms/go/threehole）: passed
- `go test ./...`（shared/algorithms/go/threehole，含新增 4 个测试）: passed
- `go vet ./...` + `go test ./backend/`（probe-interpolator）: passed
- `node verify_three.js`（50 用例 Go↔JS 跨实现数值一致，容差 1e-9）: passed
- `npm run typecheck`: passed
- `npm run build`: passed
- `task release`: passed
- `makensis -DARG_WAILS_AMD64_BINARY=..\..\bin\probe-interpolator.exe project.nsi`（在 `windows/nsis/` 目录下执行）: passed
- `task archive-release`: passed

### Known Issues

- 安装包未进行 Authenticode 数字签名，Windows 可能显示"未知发布者"提示。
- 前端结果表暂未展示 Velocity 列（后续按需追加；当前可通过 Go bindings 调用拿到 V 值）。
- 内嵌 exe 的 ProductVersion 资源为空是 wails3 v3.0.0-alpha2.106 的既有行为（不渲染 `{{.Info.ProductVersion}}` 模板），installer 壳 ProductVersion 为 `0.2.2`，对外交付不受影响。

## [0.2.1] - 2026-08-02

### Fixed

- 三孔插值输入有限性校验（A1）：`Patm=NaN` 等脏输入不再静默产出 `MachNumber=NaN` 且 `isValid=true` 的结果，统一返回 `isValid=false` + 警告（结构化无效结果，不使批量接口整体失败）。
- 三孔完整气动参数校验（A2）：`calcMach` 对齐七孔 `calVelocityMach` 全部前置条件（`Patm>0`、`Tatm>-273.15℃`、`pt≥ps`、`ps+Patm>0`、压力比≥1、最终 Ma 有限），不再用 `Abs` 掩盖非物理状态、不再对异常静默返回 0。
- 三孔 PRB 加载器一致性校验（A4）：多文件加载时校验各档 Alpha 网格完全一致、Kb 严格单调无重复，违规文件拒绝加载，避免错序 PRB 静默错配角度、重复 Kb 区间插值除零。
- 三孔超范围警告携带诊断数值（A3）：警告包含"恢复Ma=X，校准范围[Y,Z]"。

### Changed

- 三孔结果状态统一显示"参考"（不再区分"有效/无效"）：已计算行一律标注"参考"并展示计算数值，超出校准范围的行以琥珀色 + 悬停 Warning 提示原因；导出 CSV 同步输出数值与"参考: 原因"。
- 三孔单 PRB 文件快速路径（B1/C2）：跳过空转迭代与每帧排序，`IterationCount=1`、不再出现"马赫数被限制到标定边界"噪音警告。
- 三孔预计算 Kb 排序表（B2）：单文件热路径免去每帧排序/分配。

### Internal

- `three_hole_improvements_test.go` 新增 A1/A2/A3/A4/B1/C2 回归测试；更新 `TestCalcMach` 新签名与 `TestThreeHole_Boundary_ZeroAtm`（`PAtm≤0` 现判无效）。

### Verification

- `$env:GOWORK="off"; go test ./interpolation/ -count=1`（shared/algorithms/go/threehole）: passed
- `$env:GOWORK="off"; go test ./backend/ -count=1`（probe-interpolator）: passed
- `npm run typecheck`: passed
- `npm run build`: passed
- 端到端回归：W532 三孔 PRB + 三孔测试数据.csv，63 行仍全部 `isValid=false`，首行恢复 Ma=0.167。

### Known Issues

- 暂无。

## [0.2.0] - 2026-07-31

### Added

- **七孔探针校准模块**：
  - 新增 `LoadSevenHoleCalibrationCsvFiles` API，支持 7 个 GBK 编码 CSV 文件
    （内区 1 + 外区 6）导入，退化边缘自动 dither，复用 PRB 折线渲染器展示校准曲线，
    并返回 warnings 用于退化边缘可观测性。
  - 新增 `PickSevenHoleFiles` / `GetSevenHoleDataSource` API，
    支持前端槽位 UI 文件选择与 PRB/CSV 校准数据源切换。
  - 前端新增 `SevenHoleSlotRow.vue` 组件，与 wind-daq traversal-test 槽位 UI 模式一致，
    每个槽位独立选择 PRB 或 CSV 校准文件。
  - 前端新增 `useSevenHoleCalibration.ts` composable，
    封装校准流程状态管理与 CSV 导入交互逻辑。
  - 新增 `seven-hole-helpers.ts` 适配层辅助函数与 `seven-hole-slot-row.css` /
    `seven-hole-workspace.css` 样式文件。

### Changed

- **七孔 PRB 加载流程重构**：`LoadSevenHolePrbFiles` 接受前端预分配的
  inner + outer[6] 路径数组，取代后端多选对话框 + 后端 basename 路由的旧实现，
  与 wind-daq traversal-test slot-UI 模式对齐。
- **CSV 导入重构**：`ImportSevenHoleCsvData` 改为 `csvFieldSetter` 数据驱动模式，
  取代 9 个独立解析调用 + 9 路错误联合，降低维护成本。
- **七孔工作区重写**：`SevenHoleWorkspace.vue` 从 947 行精简至约 440 行，
  校准逻辑迁移至 `useSevenHoleCalibration.ts` composable。
- **`loadSevenHoleCalibrationCsvFiles` 签名**：从 `[6]string` 改为 `[]string`，
  移除调用方冗余的数组转换。
- **build/config.yml `info.version`**：从 wails 模板默认值 `0.0.1` 修正为 `0.2.0`，
  解决历史版本不同步问题（后续 release 将随 VERSION 一并更新）。

### Fixed

- **文件对话框过滤器不再污染 CSV 导入**（80ed7c8）：
  `openFileDialog` 预设 PRB 过滤器，CSV 导入链式 `AddFilter` 只是追加而非替换，
  导致 CSV 对话框同时显示 PRB 文件类型。改为 `openFileDialog` 仅 `SetTitle`，
  各调用方按场景显式 `AddFilter`（PRB 加载加 `.prb`，CSV 导入加 `.csv/.txt/.dat`）。
- **GetHelpDocPath 在 Windows dev 模式下找不到 docs/**（80ed7c8）：
  补充 cwd 兜底及上 4 级目录查找，并校验命中是文件而非目录。
- **应用图标嵌入错误**（c162dcc）：
  `main.go` 的 `//go:embed` 由 `appicon.png` 改为 `app_icon.png`，
  对应图标文件重命名，避免编译时找不到 embed 文件。
- **NSIS installer 文件名缺前缀**（bdbed99）：
  本地 makensis 不经过 wails build，`wails_tools.nsh` 不会从 `wails.json` 回填
  `INFO_PROJECTNAME` 等变量，默认空字符串导致 installer 文件名变成
  `-<version>-amd64-installer.exe`。显式 `!define` 所有 INFO_* 变量修复。

### Internal

- `Taskfile.yml` 更新 build/release 流程，移除旧 `config.yml`（79 行 wails 模板冗余配置）。
- `app_icon.png` 替换为高分辨率版本（135KB → 1.19MB），同步 `windows/icon.ico`（21KB → 107KB）。
- `seven_hole_service_test.go` 扩展测试覆盖（新增校准 CSV 导入与槽位 UI 相关测试）。
- 三份用户说明书 HTML 同步更新（5/3/7 孔）。

### Verification

- `$env:GOWORK="off"; go test ./... -count=1 -timeout 120s`: passed
- `$env:GOWORK="off"; go vet ./...`: passed
- `npm install --no-audit --no-fund`: passed
- `npm run typecheck`: passed
- `npm run build`: passed
- `task release`: passed
- `makensis -DARG_WAILS_AMD64_BINARY=<exe> project.nsi`: passed（UTF-8 BOM 已恢复）
- `task archive-release`: passed

### Known Issues

- 暂无。

## [0.1.0] - 2026-07-20

### Added

- 首次发布：将 5 孔 / 3 孔 / 7 孔探针插值整合为单一桌面程序。
- 启动选择页：3 个卡片按钮，session-locked（重启后切换探针类型）。
- 三个独立工作区组件：`FiveHoleWorkspace.vue` / `ThreeHoleWorkspace.vue` / `SevenHoleWorkspace.vue`，
  通过 `App.vue` 动态 `import()` 懒加载，避免启动时一次性加载全部。
- 共享算法包引用：
  - `shared/algorithms/go/fivehole/interpolation`
  - `shared/algorithms/go/threehole/interpolation`
  - `shared/algorithms/go/sevenhole/interpolation`
- 三套用户说明书（HTML）：`five-hole-用户说明书.html` / `three-hole-用户说明书.html` / `seven-hole-用户说明书.html`。
- 探针选择器（`probe_selector.go`）：基于 `sync.RWMutex` 的并发安全实现，
  每个 probe 类型独立 state（含自己的 `sync.RWMutex`），避免锁混用。
- 7 孔专属 API：`LoadSevenHolePrbFiles` / `CalculateSevenHole` / `BatchCalculateSevenHole` 等 8 个方法，
  所有类型加 `SevenHole` 前缀避免 Wails binding 生成冲突。
- 7 孔后端测试套件：11 个测试函数覆盖内区/外区插值、批量计算、并发安全、PRB 加载等场景，
  使用算法包 golden test data（`boundary.json`）作为权威输入输出对。

### Behavior

- 5 孔 / 3 孔工作区与旧独立程序功能等价：PRB 文件格式、CSV 输入/输出格式、压力模式开关完全兼容。
- 7 孔工作区遵循 spec §1.1：强制表压输入，不提供 PressureMode 切换开关；
  CSV 必含 P1-P7 + Patm + Tatm 共 9 列，全部必需。
- 7 孔结果 α/β 语义与 5 孔反转（spec §2.2）：α=侧滑角、β=迎角，CSV 导出表头明确标注物理含义。
- 7 孔 PRB 加载要求文件名 basename 为 1~7 的纯数字（7.prb=内区，1~6.prb=外区扇区 n）。

### Deprecation

- 旧独立程序 `projects/five-hole-interpolator` 与 `projects/three-hole-interpolator` 标记为 deprecated。
- 旧项目仓库代码与历史 release 制品保留，不再发布新版本或修复缺陷。
- 用户应迁移至本项目（`probe-interpolator`）进行后续开发与使用。

### Verification

- `GOWORK=off; go build ./...`: passed
- `GOWORK=off; go vet ./...`: passed
- `GOWORK=off; go test -race ./backend/...`: passed（含 7 孔 11 个测试 + 5 孔 / 3 孔既有测试）
- `npm --prefix frontend run typecheck`: passed
- `npm --prefix frontend run build`: passed

### Known Issues

- 暂无。
