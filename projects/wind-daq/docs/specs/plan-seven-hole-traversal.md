# Implementation Plan: 七孔遍历插值（老算法）模块

> 关联规格：[spec-seven-hole-traversal.md](./spec-seven-hole-traversal.md)
> 关联任务：[tasks-seven-hole-traversal.md](./tasks-seven-hole-traversal.md)
> 关联文档（同套流程先例）：[plan-seven-hole-calibration.md](./plan-seven-hole-calibration.md)
> 状态：**待人工批准后进入 BUILD**
> 日期：2026-07-17

## Overview

新增独立 Go 模块 `shared/algorithms/go/sevenhole/interpolation`（镜像五孔包布局与 `Interpolator` 接口），以 7 份 `.prb` 文件为唯一数据源实现七孔老算法（小角度区 + 6 个大角度扇区）；后端通过 `core/traversal.Config.ProbeType` + usecase 策略注册表贯通「原始压力装配 → 逐点计算 → CSV 列序 → 配置持久化/恢复 → HTTP API」；前端遍历向导第 1 步新增探针类型选择并按类型切换 PRB 文件选择与通道预设。算法等价性的唯一判据是与 Python 权威实现（`device-lab/skills/seven-hole-probe/seven_hole.py`）的对拍：481 个标定点 golden 文件 + 8 个构造边界用例，作为合并闸门。全程三不变：五孔插值包零改动、既有五孔遍历行为零回归、`.prb` 文本格式零变化；算法层一律不外推（对齐五孔既定行为）。

## Architecture Decisions

| 决策 | 选择 | 理由 |
|---|---|---|
| 七孔算法包布局 | 镜像 `shared/algorithms/go/fivehole/interpolation`：`types.go / prb_loader.go / prb_interpolator.go / geometry.go / atmosphere.go / inner_zone.go / outer_zone.go`，独立 `go.mod` 无外部依赖；`Interpolator` 接口与五孔同形（`IsLoaded()`/`GetValidRange()`/`Calculate()`；`Identity()` 为具体类型可选能力，不入接口） | 与既有包同构，评审者零认知成本；`go.work` use + `api-go` require/replace 三处注册照抄三孔先例 |
| 每类探针一套算法包 | 是（fivehole / threehole / sevenhole 各自独立），不做统一 n 孔抽象 | 系数语义与管线结构差异大；per-probe 独立包、不抽象跨探针通用接口（spec §2.2） |
| usecase 结构重构 | 唯一一处：新增 `traversal_probe.go` per-probe 策略注册表（`pressureLabels / isLoaded / calculate`，显式 receiver 签名），`BuildRawPressure` 与 CSV 标签归一化改为查表；probeType 变更时清空非激活探针插值器字段 | 消除 P1..P5 硬编码散点（spec 附录 B「五孔硬编码点」）；五孔行为由既有测试锁定不变；防陈旧校准误过前置检查 |
| 策略实例语义 | 包级策略表保持无状态，所有函数显式接收 `*TraversalManager` | 避免闭包捕获 Manager，保证多实例运行和并行测试之间不共享状态 |
| Manager 插值器持有方式 | 新增独立字段 `sevenHoleInterpolator` + `SetSevenHoleInterpolator` + `ClearProbeInterpolator`，五孔字段与访问器原样 | 类型不同且语义独立；通过显式 clear API 管理生命周期，五孔路径（含 `Interpolator()`、`CheckPreconditions`、`InterpolationCache`）零接触 |
| 插值器生命周期 | 增加按探针类型清除能力和 `clearInterpolator` API；切换探针时后端清除成功后才原子更新前端配置 | 防止只清前端布尔值而继续复用陈旧 PRB；清除失败时完整保留原状态 |
| 越界行为 | 不外推，`IsValid=false` + 明确 Warning；不实现 SKILL.md §3.8 `beyond_border` 外推 | 与五孔包既定行为对齐（spec §4）；外推会静默放大误差 |
| Pt<Ps 处理 | 返回 error——SKILL.md §5 明确指出 Python `math.fabs` 静默取绝对值是缺陷，Go 有意不复刻（与 Python 行为的有意差异，spec §4） | 防止无效 Pt 继续代入开方；有意的跨包差异，usecase 侧按「插值失败、该点不写计算列」处理（spec §4） |
| 外区 .prb 行数 | 每份 52 数据行（4×13 网格；首行表头仅跳过不解析，兼容尺寸/列名表头） | `big_create_square` 12 区间需 13 条网格线；数据集每区 52 个标定点佐证（spec §2.1） |
| 七孔导入 API | 新增专用 action `importSevenHolePrb`（请求体 `innerFilePath` + `outerFilePaths` 路径数组，对齐 `importMultiPrb` 的 `filePaths` 风格），不改 `importPrb` | 避免在现有 action 里按探针类型分支；五孔前端与 API 契约零影响 |
| `calculateRealtime` 请求体 | 增加可选 `probeType` 判别字段并升级为含 P6/P7 的超集 DTO；七孔必须显式传类型 | 旧五孔 body 仍是合法子集；避免依赖 Manager 隐式状态或 P6/P7 数值猜测请求类型 |
| 七孔实时缓存 | 一期不复用 `internal/core/realtime.InterpolationCache`（其类型绑定五孔） | 泛化缓存属独立优化（spec §10 Q1）；七孔为新增路径，无回归风险 |
| `GetValidRange` 语义 | 返回内区网格角域 ±30°（取自 7.prb 数据行），仅供 UI 参考，不用于事后 invalid | 角度有效性已由模式判定的多边形测试内含（spec §2.2 GetValidRange 语义注） |
| 配置模型 | 前端内部使用 `probeType` 判别联合；旧扁平五孔 JSON 仅在读取边界兼容并立即规范化 | 使五孔/七孔混合配置不可表示；未知非空 `probeType` 必须报错 |
| 前端组件边界 | 一个 `TraversalPrbStep` 公共壳 + `FiveHolePrbConfig` / `SevenHolePrbConfig` 两个子组件 | 复用同一遍历 UI，又避免大型向导累积两套条件状态 |
| 角度展示语义 | 用 `TRAVERSAL_PROBE_PRESENTATION` 元数据统一标题及 Alpha/Beta 标签 | 保持公共结果字段兼容，同时防止七孔角度被按五孔语义误读 |
| CSV 原始列序 | `buildLabelEntries` 优先级表追加 `"P6","P7"`（其余零改动；计算列不变） | 全 CSV 写入路径共用此函数，是控制列序的唯一最小改动点 |
| Wails 层 | 零改动 | 遍历功能纯 HTTP 经本地 API 服务器（证据：`app.go` 仅注入 `api.Deps`），新增 action 自动对桌面端可用 |
| 算法等价性证据 | 仅以 Go↔Python 对拍为准（481 点 + 8 边界），golden JSON 入版本库 | 需求硬门槛；Python 实现是需求指定权威；golden 入版本库使 CI 可重复 |

## Assumptions

1. 外区每份 `.prb` 恰 52 行（4a×13b）。若后续真实文件不符，`prb_loader` 的行数校验会在加载期显式报错而非静默错算。
2. 数据集 7 份 CSV（GBK 编码）列位置固定，夹具生成脚本按列位置读取，不依赖表头历史命名错误（对照表见 spec-seven-hole-calibration.md §12.1）。
3. 夹具生成脚本 `device-lab/skills/seven-hole-probe/tools/gen_traversal_fixtures.py` 与其产物 `testdata/` 提交版本库；`seven_hole.py` 本身不修改（脚本自带 cal_ab 驱动）。
4. 配置中 `probeType` 缺省时仅在读取旧配置边界按五孔处理；未知非空值报错，不得静默降级。
5. `traversal.pProbePressureType` 的表压/绝压开关语义对七孔 P1..P7 同样适用（作用于探针孔道，Patm/Tatm 不参与）。
6. 无标签原始数据的 legacy 回退（按 CH 顺序映射 P1..P5）仅服务五孔旧配置；七孔配置必定携带 9 角色标签（spec §5.2）。
7. 新保存的七孔配置不携带五孔专属字段；API/保存边界发现混合字段时返回校验错误。只有读取历史五孔配置时允许兼容适配。
8. 七孔校准模块（types.go 骨架之后的 tasks 2-24）未实施不影响本模块：遍历只依赖 `.prb` 文件集，来源不限。
9. 外区角域上限为 θ=45°（a∈{30,35,40,45}，每份 52 数据行）。若未来产品要求 θ>45°，`.prb` 必须增加 a 网格线（行数不再是 52），加载校验与对拍夹具须同步更新——属 spec §10 Q3 的另立增补，本期不预留兼容。

## Dependency Graph

```
Phase 1  七孔插值包（shared/algorithms/go/sevenhole）
  Task 1  模块骨架 + types.go + go.work/go.mod 注册
    └─→ Task 2  prb_loader.go（行级解析与校验，算法包零 I/O）
          └─→ Task 3  geometry.go + inner_zone.go（小角度：边界多边形 + 四边形定位反演 (a,b) + cpt/cps 双线性）
                └─→ Task 4  outer_zone.go（大角度：扇区判定 + 四边形定位反演 (θ,φ) + cpt/cps 反向双线性 + 坐标变换）
                      └─→ Task 5  atmosphere.go + prb_interpolator.go（V/Ma + 数值守卫 + 编排）
                            └─→ Task 6  Go↔Python 对拍闸门（夹具生成 + golden 测试 + 边界用例）

Phase 2  后端集成（projects/wind-daq/services/api-go）—— 依赖 Task 6 通过
  Task 7  core/traversal：Config.ProbeType + 常量
    └─→ Task 8  usecase/traversal_probe.go：无状态策略注册表 + Manager 七孔字段/生命周期 + BuildRawPressure 策略化
          ├─→ Task 9  ports/adapters：LoadSevenHolePRB + LoadSevenHolePrbFiles
          ├─→ Task 10 traversal_config.go：probeType 透传 + roleToLabel 9 角色 + 恢复七孔分支
          └─→ Task 11 csv_writer：标签优先级表 +P6/P7
                └─→ Task 12 api/server.go：importSevenHolePrb + clearInterpolator + calculateRealtime 判别分发

Phase 3  前端（apps/desktop-wails/frontend）—— 依赖 Task 12 完成
  Task 13 类型层：calibration.ts 9 角色 + traversal.ts 判别配置/预设/展示元数据
    └─→ Task 14 traversalApi.importSevenHolePrb + traversalStore 动作/状态推导
          └─→ Task 15 向导：TraversalSettings 公共壳 + 五孔/七孔 PRB 子组件 + i18n

Phase 4  端到端验收 —— 依赖 Task 15 完成
  Task 16 全量验证：go test 全绿 + 五孔回归 + npm typecheck/build + validate-structure + 手动 E2E 清单
```

关键路径：Task 2 → 3 → 4 → 5 → 6（算法对拍是全局闸门）；Phase 2 内 Task 9/10/11 在 Task 8 之后可并行。

## Risks & Mitigations

| 风险 | 影响 | 缓解 |
|---|---|---|
| Go 实现对拍不通过（公式/坐标变换/扇区判定与 Python 有出入） | 合并闸门失败，模块不可用 | 先逐段单测（模式判定→系数→Pt/Ps→V/Ma）再整体对拍；角度容差 1e-4°，压力按 `abs(got-want) <= max(abs(want)*1e-6, 1e-4 Pa)`；负号保护注入 `cal_ab` 失败用例；对拍失败时按分段中间量二分定位 |
| 五孔遍历行为回归 | 破坏既有生产路径（零回归硬门槛） | 五孔插值包零改动；usecase 改动由既有测试（`TestBuildRawPressure*`、`TestParseConfig*`、恢复/CSV 测试）锁定；每个 Task 的验证步骤强制跑 `go test ./internal/...` 全量；spec §7.3 列明不可破坏的既有行为 |
| .prb 文件集缺失或格式错误（行数/网格覆盖不对） | 运行时错算或 panic | 加载期强校验：内区恰 169 数据行、外区每份恰 52 数据行、网格覆盖内区 ±30°/外区 θ 30–45°（缺失/重复/越界网格点均报错）；错误信息指明来源与行号；数值守卫（除零、Pt<Ps、负 radicand）返回 error 而非 panic |
| θ=30° 边界处大小角度判定抖动 | 边界点无效或角度跳变 | 判定纯数据驱动（`little_create_line` 边界点射线法），不写死角度阈值；边界用例（±30°、交界网格点）进对拍集合；交界网格点被两边同时覆盖时按内区优先（与 Python 分支顺序一致） |
| 前端向导复杂度上升（七孔 7 文件选择易选错） | 配置错误、用户困惑 | 探针选择放第 1 步（硬件），切换时重置通道预设并清空 PRB 状态（带确认提示）；七孔模式隐藏无关控件（老/新算法、单/多 PRB）；内区文件单独置顶 + 六扇区固定顺序标签；复用泛型组件（`TraversalHardwareStep` 通道表零改动） |
| 公共向导累积大量五孔/七孔条件状态 | 修改一类探针误伤另一类，恢复逻辑难验证 | `TraversalPrbStep` 只作公共壳；两类 PRB 配置各自独立组件；`TraversalSettings` 只持有一个判别配置对象 |
| 切换探针后端仍保留旧插值器 | 前端显示未加载但计算仍使用旧 PRB | 切换确认后先调用 `clearInterpolator`；失败则不改变选择；成功后原子重置配置和实时结果 |
| 数据集 GBK 编码读取乱码 | 夹具生成失败或数值错 | 夹具脚本先转码副本（不改原文件），按列位置读数值；生成后对 481 点做完整性断言（行数、NaN 检查） |
| 夹具脚本与 `seven_hole.py` API 漂移 | 夹具无法重建或静默生成错误 golden | 脚本文件头记录依赖的 `cal_ab` 契约；生成时断言 481 点和每份 PRB 行数；Python API/格式/数据集变化时强制重生成并审查产物 diff |
| 「不外推」与需求方预期不符 | 验收争议 | spec §4 已记录决策与五孔证据；评审时显式确认；如改为允许外推，仅影响 `outer_zone` 一个函数 + 对拍加用例，改动面可控 |
| 七孔不过 `InterpolationCache` 被质疑一致性 | 评审返工 | spec §10 Q1 已记录为有意延后；缓存语义（Patm/Tatm 变化失效）与探针类型无关，后续泛化是独立小任务 |

## Verification Checkpoints

| 检查点 | 时机 | 验证内容 | 通过标准 |
|---|---|---|---|
| CP1 算法包独立全绿 | Phase 1 Task 1-5 每个任务 | `cd shared\algorithms\go\sevenhole; go test ./...`；`go vet ./...` | 全部通过，无外部依赖 |
| CP2 对拍闸门 | Task 6 | 481 标定点 + 8 边界用例 golden 对比（全部位于合法网格内，与 Python 对拍一致）；Go 产品策略守卫用例（越界不外推、Pt<Ps、1e-12 守卫、NaN/Inf——与 Python 的差异已记录于 spec §4/§7.2） | 对拍容差内 100% 通过；守卫用例按 spec §4/§7.2 断言通过 |
| CP3 后端集成 | Phase 2 每个任务 | `cd projects\wind-daq\services\api-go; go test ./internal/...`（含既有五孔用例）；`importSevenHolePrb` / `calculateRealtime`（P7 体）curl 冒烟 | 新增用例通过；既有用例零失败；curl 返回结构符合 spec §5.6 |
| CP4 前端集成 | Phase 3 每个任务 | `npm run typecheck`；`npm run build`；向导手动走查（选择七孔→导入 7 文件→核对预览） | 类型/构建零错误；向导行为符合 spec §6 |
| CP5 最终验收 | Task 16 | api-go 全量 `go test ./...`；五孔包 `go test ./...`；`scripts\validate-structure.ps1`；手动 E2E 清单（spec §8） | 全部通过；按文件归属使用 Git/SVN 状态与 diff 确认冻结区零改动 |

## Out of Scope

- 七孔多 PRB（马赫模式）、新算法校准 CSV 数据源（待七孔校准模块 tasks 2-24 实施后另行接入）
- SKILL.md §3.8 `beyond_border` 外推（任何方向）
- `InterpolationCache` 泛化与七孔接入
- 前端任何插值/后处理算法（仅类型、向导与 API 调用）
- Wails binding 重生成（无新增绑定）
- 七孔校准模块本身的实施
