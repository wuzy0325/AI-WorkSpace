# 三孔插值算法改善提案

> 本文档记录 `shared/algorithms/go/threehole/interpolation/three_hole.go` 的算法改善提案。
> 背景源于一次现场排查：三孔测试数据因恢复马赫数与标定不符被全部判无效，排查过程中发现算法在正确性、性能与诊断上存在若干可改善点。
> 提案为"记录待办"，尚未落地代码；落地前需按各条目的验收标准补测试。
>
> **v0.2（修订版）**：已按 review findings 修订——A1 明确错误契约、A2 扩展为完整气动校验、新增 A4 加载器一致性校验（B2 前置）、B1 明确对外行为变化、§5 端到端基线改为可复现夹具。

## 1. 背景与排查结论摘要

- 使用 `W532.202608.P.3H.1-01（H1测点）-85米每秒校准文件.prb`（Ma=0.242 单点）对 `三孔测试数据.csv` 计算，63 行全部 `isValid=false`。
- 直接原因：恢复马赫数 ≈0.167，触发 `three_hole.go:149` 的 `mach < minMa-0.01` 判据（校准区间 [0.232, 0.252]）。
- 根因在数据端：测试压力量级与标定工况不符（P2≈+802 Pa vs 标定 0° 时 +7688 Pa），非算法逻辑错误。
- 排查中确认算法本身判断正确，但暴露出以下可改善点。

## 2. 现状算法行为摘要

输入 `P1/P2/P3`（表压）+ `Patm`（绝压）+ `Tatm`（℃）：

1. `deltaP = 2*P2 - P1 - P3`，过小直接返回无效（`three_hole.go:81-89`）
2. `kbTemp = (P3 - P1)/deltaP`，非有限值返回无效（`three_hole.go:91-101`）
3. 按当前 Ma 在 Kb 表上插值得到 `K0/Kv/Alpha`，恢复 `pt/ps`，反算 Ma，迭代至收敛（`three_hole.go:107-127`）
4. 最终校验：恢复 Ma 必须落在 `[minMa-0.01, maxMa+0.01]` 内，否则 `isValid=false`（`three_hole.go:149-152`）

## 3. 改善提案

### A. 正确性 / 健壮性（最高优先）

#### A1. 增加输入有限性校验（含明确错误契约）

- **问题**：`Calculate` 入口无 NaN/Inf 校验。注意问题面没有文档 v0.1 描述的那么宽：
  - `P1/P2/P3` 为 NaN/Inf 时已在 `deltaP` / `kbTemp` 校验处返回无效（`three_hole.go:81-101`），不会静默放行；
  - **真正可静默放行的是 `Patm=NaN`**：`calcMach` 得到 `MachNumber=NaN`，而 NaN 与 `[minMa-0.01, maxMa+0.01]` 比较恒为 false → **`isValid=true, MachNumber=NaN`**；
  - `Tatm=NaN` 当前被 `calcGamma` 兜底成 γ=1.4（`three_hole.go:272-279`），不产生 NaN，但属非法输入，仍应在入口拦截。
- **位置**：`three_hole.go:72`（`Calculate` 入口）。
- **方案**：仿照七孔 `validateFiniteInput`（`prb_interpolator.go:86-101`），对 `P1/P2/P3/PAtm/TAtm` 统一做有限性校验。
- **错误契约（实施前必须选定）**：返回**结构化无效结果**（`isValid=false + Warning`），**不返回 error**。理由：`BatchCalculateThreeHole`（`three_hole_service.go:140-173`）中 error 会使整批 `Success=false`（`three_hole_service.go:169`），与"个别坏行不使批次失败"目标冲突；无效结果则保留正常响应、仅该行 `isValid=false`。此契约需在算法包 `Calculate` 与批量语义间保持一致。
- **影响**：行为变更仅限脏输入路径；正常数据无影响。
- **验收**：
  - `Patm=NaN` → `isValid=false` 且 `MachNumber` 为有限值；
  - `P1=NaN` 保持现有拦截路径（行为不变）；
  - `Tatm=Inf` → `isValid=false`；
  - 批量接口：单行脏输入不使整批 `Success=false`。

#### A2. 完整的气动参数校验（对齐七孔 `calVelocityMach` 全部前置条件）

- **问题**：`calcMach`（`three_hole.go:256-270`）只做了两件事：`absPs < 1e-6 → return 0`、`math.Abs(powered-1)`。缺失校验：
  - `Patm` 非正（NaN/≤0）→ `absPs`/`absPt` 失去物理意义，`ps+Patm<1e-6` 时静默返回 0；
  - `Tatm ≤ -273.15℃` → γ 异常（`calcGamma` 无下限校验）；
  - `pt+Patm ≤ 0`、压力比 `<1`（即 `pt<ps`）→ 被 `math.Abs` 掩盖成"正马赫数"；
  - 最终 Ma 是否有限无校验。
- **位置**：`three_hole.go:256-270`（`calcMach`）及其调用链。
- **方案**：对齐七孔 `calVelocityMach`（`atmosphere.go:25-57`）**全部**前置条件，任一不满足返回 error，由 `Calculate` 转成 `isValid=false + 警告`，不再 `return 0`、不再 `math.Abs`：
  1. `patm` 有限且 `> 0`；
  2. `tatm+273.15` 有限且 `> 0`；
  3. `pt ≥ ps`；
  4. `ps+patm > 0`；
  5. `ratio=(pt+patm)/(ps+patm) ≥ 1`；
  6. `maSq ≥ 0`（由 5 保证，最终 Ma 有限）。
- **影响**：会改变 `TestThreeHole_Boundary_ZeroAtm`（`golden_test.go:485-507`）的现状行为——该用例目前允许 `PAtm=0` 并断言 `MachNumber` 有限；按新契约 `Patm≤0` 应判无效，**需同步更新该用例**为断言 `isValid=false`。
- **验收**：构造 `ps>pt`、`ps+patm≤0`、`Patm=0`/负数、`Tatm=-300℃` 输入，断言 `isValid=false` 且警告非空、`MachNumber` 有限。

#### A3. 增强超范围诊断信息

- **问题**：`three_hole.go:150` 只拼接"计算马赫数超出校准范围"，不携带实际值与阈值，定位需手工复算。
- **方案**：仅增强 `Warning` 字符串，携带 `恢复Ma=%.3f，校准范围[%.3f, %.3f]`。**不新增 `Result.MachRange` 结构字段**——加载响应已提供 `machRange`，新增结果字段涉及核心类型 + 后端映射 + Wails 绑定重新生成，非零成本；Warning 增强已满足诊断需求。
- **影响**：纯增量，向后兼容。
- **验收**：触发超范围用例，断言警告包含实际 Ma 与范围数值。

#### A4. 加载器一致性校验（B2 实施的前置，P0）

- **问题**：`LoadPrbData` 仅校验各档 PRB 的 `Nalpha` 相等（`three_hole.go:295-300`），随后按数组下标混合各 Ma 档数据——`interpolateWithWarning` 以 `t.alphaSeq` 的下标直接取 `calib.Items[i]`（`three_hole.go:205-215`）。未校验：
  - 各档 **Alpha 序列完全一致（含顺序）**：错序/不同序列的 PRB 会被静默混合，产出错误角度；
  - 各档 **Kb 单调且无重复**：重复 Kb 时区间插值 `r := (kbMeasured - Kb[j]) / (Kb[j+1] - Kb[j])` 除零 → `K0/Kv/Alpha` 变 NaN/Inf。
  - 直接实施 B2（预计算 + 二分）时，上述问题会继续静默产生错误结果或直接除零。
- **位置**：`three_hole.go:281-356`（`LoadPrbData`）。
- **方案**（P0，先于 B2）：
  1. 加载时校验每档 Alpha 序列与首档逐元素一致（容差 1e-9）；
  2. 校验每档 Kb 按 Alpha 序严格单调且无重复，不满足则 `LoadPrbData` 返回错误；
  3. 若需容忍重复 Kb，必须先定义处理策略（如报错拒绝），不允许静默插值。
- **影响**：把"错误校准文件静默出错误结果"前置为加载期报错；对合法 PRB 无行为变化。
- **验收**：构造 Alpha 错序 / 不同序列 / Kb 重复的 PRB 组合，断言 `LoadPrbData` 返回明确错误。

### B. 性能（高频热路径）

#### B1. 单 PRB 文件快速路径（对外行为变化，需明确）

- **问题**：`interpolateWithWarning`（`three_hole.go:176-254`）每次调用都拷贝并排序 `calib`、重建 entries；`Calculate` 的迭代循环（`three_hole.go:107-127`）内最多调用 21 次。而 `len(calib)==1` 时 `calib1==calib2`、`ratio=0`，插值结果与 Ma 无关，**整个迭代循环与多次排序均为空转**。
- **方案**：`LoadPrbData`（`three_hole.go:281`）时预排序 `calib`；`len(calib)==1` 时 `Calculate` 直接单次插值，跳过迭代循环与 Ma 钳制。
- **影响（注意：非严格"行为不变"）**：
  - `Alpha` / `MachNumber` / `IsValid` 不变；
  - **`IterationCount` 语义变化**：单文件快速路径固定为 1，替换原"迭代至收敛/上限 20"的计数值；
  - **`Warning` 变化**：C2 删除 `maClamped` 警告后，单文件有效结果不再带"结果精度可能降低"。
  - 两字段均经 `ThreeHoleInterpolationResult`（`three_hole_types.go:43-49`）对外返回。
- **验收**：
  - 基准测试对比单文件场景每次 `Calculate` 的分配数/耗时；
  - 现有 golden 只断言 `Alpha/MachNumber/IsValid`（`golden_test.go:45-48`），**不足以证明对外一致**——必须补充 `Warning`、`Pt`、`Ps`、`IterationCount` 的回归断言；
  - 明确并断言单文件快速路径下 `IterationCount==1` 且无 `maClamped` 警告。

#### B2. 预计算 Kb 插值表（依赖 A4）

- **问题**：`entries` 切片与 `alphaSeq` 循环每帧重复构造。
- **方案**：加载时预计算按 Kb 排序的插值表，运行时二分定位区间。**前置条件：A4 已落地**（Alpha 网格一致 + Kb 单调无重复校验），否则预计算会继承错误混合与除零风险。
- **影响**：与 B1 可合并实施；多文件时按当前 Ma 取两档表插值。
- **验收**：与 B1 共用基准与 golden 校验；补充 A4 的加载失败用例证明风险已被拦截。

### C. 功能 / 灵活性

#### C1. 恢复 Ma 失配策略可配置

- **现状**：恢复 Ma 偏离标定区间 ±0.01 即硬失败（`three_hole.go:149`）。
- **选项**：
  1. 保持硬失败（现状，最安全）；
  2. 多加载若干 Ma 点 PRB 撑开范围（算法已支持，属数据使用方式而非代码改动）；
  3. 降级为 warning + 置信度，由上层决定（**需权衡**：本次 0.167 的脏数据也会被放行）。
- **建议**：先做 A/B，C1 选项 3 需产品决策后再实施，避免放开有效性护栏。
- **验收**：若实施选项 3，需配套在结果中显式标注"马赫数超出校准范围（仅供参考）"。

#### C2. 消除单文件场景的 Ma 钳制噪音

- **问题**：单文件时迭代 `currentMa` 恒被钳到边界，`maClamped=true` 触发"结果精度可能降低"警告，但该场景下钳制无实际意义。
- **方案**：随 B1 一并处理（单文件不再迭代，自然不再钳制）。**注意该行为变化已计入 B1 的对外影响（`Warning` 字段）**。
- **验收**：并入 B1 验收——单文件 golden 用例断言无 `maClamped` 相关警告，并记录 `Warning` 精确值。

## 4. 优先级与建议实施顺序

| 批次 | 条目 | 理由 |
|---|---|---|
| P0 | A1 输入有限性校验 + 错误契约 | 防 `Patm=NaN` 静默产出 NaN 结果；契约先定 |
| P0 | A2 完整气动参数校验 | 对齐七孔全部前置条件，消除非物理静默 |
| P0 | A4 加载器一致性校验 | B2 的前置，防错序/重复 Kb 静默出错 |
| P1 | A3 诊断增强（仅 Warning） | 纯增量，改善可定位性 |
| P1 | B1 单文件快速路径 + C2 | 性能收益集中点；需同步补对外字段回归断言 |
| P2 | B2 预计算表 | 依赖 A4，与 B1 合并实施更优 |
| P3 | C1 失配策略 | 涉及产品决策，先留现状 |

实施顺序建议：**A1 → A2 → A4 →（更新受影响测试）→ B1+C2 → B2 → A3**。其中 A1/A2/A4 完成后先跑全量测试确认基线，再动性能优化。

## 5. 回归验证基线

- `go test ./shared/algorithms/go/threehole/interpolation/`（含 golden、customer、unit 全部用例）
- 预提交：`go test ./...` 于 `projects/probe-interpolator/apps/desktop-wails`（算法包为 replace 依赖）

**端到端夹具（可复现，落地时固化为测试夹具文件）**：

1. **合法输入夹具**（来自标定母本，独立可复现）：取 `W532.202608.P.3H.1-01-校准数据.xlsx` H1 sheet（或 `0.284.csv`）0° 行的探针测1/2/3 绝压，减固定 `Patm=95343` 转表压：
   - `P1=+3791.1, P2=+7687.6, P3=+4031.4, Patm=95343, Tatm=26.574`，`pressureMode=gauge`
   - 期望：`isValid=true`，`Alpha≈0°`（容差 ±0.5°），`MachNumber∈[0.232, 0.252]`（校准容差 ±0.01）
   - 夹具落地时记录完整输入、期望 `Pt/Ps`、`IterationCount`、`Warning` 及各自容差。

2. **非法数据回归**：`三孔测试数据.csv` 的 63 行固定断言 **全部 `isValid=false`**，并断言首行恢复 Ma≈0.167（容差 ±0.01），保持现有拒绝行为不被优化项破坏。

## 6. 关联资料

- 算法实现：`shared/algorithms/go/threehole/interpolation/three_hole.go`
- 七孔对标：`shared/algorithms/go/sevenhole/interpolation/`（`validateFiniteInput`、`atmosphere.go`）
- 后端批量语义：`projects/probe-interpolator/apps/desktop-wails/backend/three_hole_service.go`
- 结果字段：`projects/probe-interpolator/apps/desktop-wails/backend/three_hole_types.go`
- 排查数据：`projects/probe-interpolator/.temp/20260716-W532/`
- 标定母本：`W532.202608.P.3H.1-01-校准数据.xlsx`（H1 sheet → `.prb`）
