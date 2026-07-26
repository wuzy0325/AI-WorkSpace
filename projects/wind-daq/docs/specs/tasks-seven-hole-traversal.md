# Tasks: 七孔遍历插值（老算法）模块

> 关联规格：[spec-seven-hole-traversal.md](./spec-seven-hole-traversal.md)
> 关联计划：[plan-seven-hole-traversal.md](./plan-seven-hole-traversal.md)
> 关联文档（同套流程先例）：[tasks-seven-hole-calibration.md](./tasks-seven-hole-calibration.md)
> 状态：**已完成**（2026-07 修订：探针切换改为双变体并存语义，见 Task 10/14/15 修订说明）
> 日期：2026-07-17

## 总体约定

- **TDD 强制**：每个 Task 先写失败测试，再实现到通过；测试与实现同 Task 提交。
- **零回归硬门槛**：Phase 2/3 每个 Task 的验证必须包含 `cd projects\wind-daq\services\api-go; go test ./internal/...` 全量通过；任何既有测试失败即阻塞，不得跳过。
- **五孔包冻结**：`shared/algorithms/go/fivehole/` 任何文件零改动；最终验收用 `git status` 佐证。
- **英文注释强制**：本任务遵循工作区 `CLAUDE.md` §Language 的 “Code comments: English only”，该项目级规则覆盖 user profile 中可能存在的中文注释偏好。新增 Go/TS 代码注释一律英文；算法包内注释须标注对应的 Python 权威出处（`seven_hole.py` 函数名 + SKILL.md 章节号双锚点）。文档与 UI 文案仍用中文。
- **任务粒度**：单 Task 触及生产文件 ≤5 个（测试文件与 testdata 不计入）。
- **命令风格**：Windows 路径反斜杠；`daq-t1603` 不涉及，无需 `GOWORK=off`。
- **只读区**：`device-lab/skills/seven-hole-probe/seven_hole.py` 与数据集 `projects/wind-daq/docs/W532.202608.P.7H.1-01/` 原文件只读（Task 6 新增的夹具脚本与转码副本除外）。

---

## Phase 1：七孔插值包（`shared/algorithms/go/sevenhole`）

### Task 1：模块骨架 + types.go + 三处模块注册

**Description**

新建独立 Go 模块 `shared/algorithms/go/sevenhole`（`go.mod` 模块名 `ai-workspace/shared/algorithms/go/sevenhole`，无外部依赖），内含 `interpolation/types.go`：**接口与五孔包同形**（`fivehole/interpolation/types.go` L52-62）——`Interpolator` 接口恰含 `IsLoaded() bool`、`GetValidRange() PrbValidRange`、`Calculate(InterpolationInput) (InterpolationResult, error)` 三个方法；`PrbValidRange` 镜像五孔同名结构体。`InterpolationInput` 含 `P1..P7, PAtm, TAtm`（float64，Pa / ℃），`InterpolationResult` 含 `Alpha, Beta, MachNumber, Velocity, DynamicPressure, TotalPressure (json "P0"), StaticPressure (json "Ps"), IsValid, Warning`（字段与 JSON tag 同五孔，保证 API 响应形状可复用）。**`Identity() string` 不属于接口**：由具体类型 `SevenHolePrbInterpolator` 实现为可选能力方法，消费方经类型断言 `interface{ Identity() string }` 取用（五孔既有模式，`usecase/traversal.go` L903 的 `interpolatorIdentity`）。英文注释注明：七孔语义 Alpha=侧滑角、Beta=迎角，与五孔定义相反，字段名沿用仅为复用管线（spec §2.2、附录 A）。同时在三处注册模块：仓库根 `go.work` use 列表加 `./shared/algorithms/go/sevenhole`；`projects/wind-daq/services/api-go/go.mod` 加 require + replace（照抄 fivehole/threehole 先例）。

**Acceptance criteria**

- [ ] `interpolation/types.go` 编译通过；`Interpolator` 接口方法集与五孔逐一同形（`IsLoaded`/`GetValidRange`/`Calculate`，五孔 `types.go` L52-62）
- [ ] `InterpolationResult` 的 JSON tag 与五孔完全一致（`"P0"`、`"Ps"` 等）
- [ ] `Identity() string` 仅由具体类型实现（编译期断言：接口不含该方法；具体类型满足 `interface{ Identity() string }`）
- [ ] `go.work` 与 `api-go/go.mod` 注册后，`api-go` 可 import `seveninterp "ai-workspace/shared/algorithms/go/sevenhole/interpolation"` 并编译
- [ ] `GetValidRange` 文档注释明确「仅供 UI 参考，不用于事后 invalid」（spec §2.2 GetValidRange 语义注）

**Verification**

- [ ] `cd shared\algorithms\go\sevenhole; go build ./...; go vet ./...`
- [ ] `cd projects\wind-daq\services\api-go; go build ./...`
- [ ] `cd shared\algorithms\go\fivehole; go test ./...` 全绿（冻结证据之一）

**Dependencies**：无

**Files likely touched**

- `shared/algorithms/go/sevenhole/go.mod`（新增）
- `shared/algorithms/go/sevenhole/interpolation/types.go`（新增）
- `shared/algorithms/go/sevenhole/interpolation/types_test.go`（新增）
- `go.work`
- `projects/wind-daq/services/api-go/go.mod`

**Estimated scope**：S

---

### Task 2：prb_loader.go —— 7 文件集解析与强校验

**Description**

实现 .prb 文本行解析与强校验，**算法包零 I/O**——以五孔 `PrbInterpolator.LoadPrbLines(lines, filePath)` 为惯例（`adapters/interpolation/files.go` L18），实现为插值器方法：`NewSevenHolePrbInterpolator()` 构造 + `LoadInnerPrbLines(lines []string, source string) error` + `LoadOuterPrbLines(sector int, lines []string, source string) error`（`sector` ∈ 1..6；`source` 仅作错误消息的来源标注，通常为文件路径的字符串透传）。**包内不得出现路径参数或 `os.*` 调用**，文件读取在 Task 9 的 adapters 层。表头契约（spec §2.1）：首行必须存在且仅跳过不解析（兼容尺寸表头 `13 13`/`4 13` 与列名表头 `ka kb cpt cps a b`，对齐 Python `next(file)` 行为）；维度不由表头判定，而由数据行数（内区 169 / 外区 52）+ 网格覆盖完整性校验确定。数据行：每行恰 6 列空白分隔、全部有限数值；内区覆盖 a,b ∈ {-30..30 步长 5} 全部 169 点；外区（行序 = b 外层 a 内层）覆盖 a=θ ∈ {30,35,40,45} × 该扇区 13 条 b 网格线（spec §2.1 扇区表）全部 52 点。强校验：行数恰为 169 / 52；网格点缺失/重复/越界均报错；六份外区扇区中心角互异且并集覆盖 360°（容差 1e-9）；任何失败返回指明 `source` 与行号的 error。数据结构按 spec §2.1：`InnerGrid`（13×13）+ `OuterSectors[6]`（各 4×13，含扇区中心 φ 与边界多边形顶点）。

**Acceptance criteria**

- [ ] 正确行集加载成功，网格维度、扇区数、角度覆盖（内区 ±30°、外区 θ 30–45°）与输入内容一致
- [ ] 行数错误（168/170、51/53）、网格点缺失/重复/越界均报错且消息含 source 与行号
- [ ] 列名表头（`ka kb cpt cps a b`）与尺寸表头（`13 13`/`4 13`）的行集均可正常加载（首行仅跳过不解析）
- [ ] 包内无 `os.*`/路径操作（grep 佐证；`source` 仅作字符串透传）
- [ ] 测试用最小合成 .prb 文本行（测试内联构造），不依赖真实数据文件

**Verification**

- [ ] `cd shared\algorithms\go\sevenhole; go test ./interpolation/ -run 'TestLoadInnerPrbLines|TestLoadOuterPrbLines' -v`
- [ ] `cd shared\algorithms\go\sevenhole; go vet ./...`

**Dependencies**：Task 1

**Files likely touched**

- `shared/algorithms/go/sevenhole/interpolation/prb_loader.go`（新增）
- `shared/algorithms/go/sevenhole/interpolation/prb_loader_test.go`（新增）

**Estimated scope**：M

---

### Task 3：geometry.go + inner_zone.go —— 小角度区插值

**Description**

`geometry.go`：公共几何工具 —— `pointInPolygon`（射线法，对应 `point_in_polygon`（SKILL §4））、边界多边形构造（对应 `little_create_line`（SKILL §4），由内区网格最外圈标定点生成）、四边形边方程定位（对应 `little_cal_ab` 的定位段，SKILL §2.3）、距离反比加权（SKILL §2.3）、规则网格双线性插值（SKILL §2.4）。`inner_zone.go`：小角度区管线**两阶段**（spec §3.2，以它为准）——阶段 A：由 (ka,kb) 经边界多边形判定属小角度后，在**畸变 (ka,kb) 网格**上做 144 个四边形定位（边方程）+ 距离反比插值，**反演得到角度坐标 (a,b)**（对应 `little_cal_ab`（SKILL §2.3））；阶段 B：在**规则角度网格**上对 `cpt`/`cps` 两个系数场做双线性插值——内区方向为先沿 a 求斜率再沿 b 插值（对应 `little_cptcps_square`（SKILL §2.4））。**角度 (a,b) 不是被插值的『场』，是阶段 A 的反演输出**。纯数据驱动，不写死角度阈值（spec §3、plan Risks）。英文注释逐段标注对应 `seven_hole.py` 函数名 + SKILL 锚点。

**Acceptance criteria**

- [ ] 阶段 A：手算四边形用例（构造已知畸变四边形与内部点，权重心算可验）反演 (a,b) 与手算一致；网格原点/边缘单元格定位正确
- [ ] 阶段 B：线性系数场精确还原（构造 cpt/cps 为 a,b 线性函数的网格，插值误差为 0 量级）
- [ ] 边界多边形外（但 ka/kb 数值合法）返回「不属小角度」信号而非错误（供编排层落大角度）
- [ ] 边界上的点（a/b=±30° 网格线）判定为内区（与 Python 分支顺序一致：内区优先，`point_in_polygon` 边界按内部处理）

**Verification**

- [ ] `cd shared\algorithms\go\sevenhole; go test ./interpolation/ -run 'TestInnerZone|TestCalAB|TestPointInPolygon|TestCptCpsSquare' -v`

**Dependencies**：Task 2

**Files likely touched**

- `shared/algorithms/go/sevenhole/interpolation/geometry.go`（新增）
- `shared/algorithms/go/sevenhole/interpolation/geometry_test.go`（新增）
- `shared/algorithms/go/sevenhole/interpolation/inner_zone.go`（新增）
- `shared/algorithms/go/sevenhole/interpolation/inner_zone_test.go`（新增）

**Estimated scope**：M

---

### Task 4：outer_zone.go —— 大角度区插值 + 坐标变换

**Description**

大角度管线**两阶段**（对应 Python 大角度分支，spec §3.2）：以七孔压力确定最大孔与次大孔 → 先试最大孔扇区的 (ka,kb) 边界多边形（`big_create_line`（SKILL §3.3）+ `point_in_polygon`（SKILL §4）），miss 再试次大孔扇区（`cal_ab` 的 first/second 候选逻辑）；两试均 miss 返回 invalid（**不实现** `beyond_border` 外推，spec §4）。命中后——阶段 A：在**畸变 (ka,kb) 网格**做 36 个四边形定位（边方程）+ 距离反比插值，**反演得网格坐标 (a,b)**（对应 `big_cal_ab`（SKILL §3.4）；外区 a=θ、b=φ）；阶段 B：在**规则角度网格**对 `cpt`/`cps` 双线性插值，方向与小角度**相反**（对应 `big_cptcps_square`（SKILL §3.5））。出口坐标变换 (θ,φ)→(α,β)：`α=-atan(tanθ·sinφ)`、`β=atan(tanθ·cosφ)`（`cal_ab` 大角度出口，SKILL §3.7）；`|θ|≥89.5°` 退化分支 `a=-θ, b=φ`（标准 .prb 网格 θ≤45° 下不可达，仍须实现并做函数级单测）。

**Acceptance criteria**

- [ ] 扇区选择：构造最大孔/次大孔压力分布，命中正确扇区（六个扇区各至少一例）
- [ ] 最大孔扇区 miss 时正确回退次大孔扇区（`cal_ab` first/second 候选逻辑）
- [ ] 两试均 miss 返回 `IsValid=false` + Warning，且不外推（对照点位于所有扇区多边形外）
- [ ] 阶段 A 反演：手算四边形用例（θ,φ）与手算一致；阶段 B：线性系数场精确还原（方向与内区相反，用反对称构造场验证方向未写反）
- [ ] 坐标变换与 SKILL §3.7 公式手算值一致（θ=30°、45° 两档 + 若干 φ）；89.5°/90° 退化分支走 `a=-θ, b=φ`（函数级单测）
- [ ] 扇区系数 (ka,kb) 按 SKILL §3.1 定义由最大孔扇区压力计算，与 Python 中间量一致（对拍前的单元级核对）

**Verification**

- [ ] `cd shared\algorithms\go\sevenhole; go test ./interpolation/ -run 'TestOuter|TestSector|TestCoordTransform' -v`

**Dependencies**：Task 3

**Files likely touched**

- `shared/algorithms/go/sevenhole/interpolation/outer_zone.go`（新增）
- `shared/algorithms/go/sevenhole/interpolation/outer_zone_test.go`（新增）

**Estimated scope**：M

---

### Task 5：atmosphere.go + prb_interpolator.go —— V/Ma + 数值守卫 + 编排

**Description**

`atmosphere.go`：流速与马赫 —— `V=√(2ΔP·R·T/pa)`、`Ma=√(5·((Pt+pa)/(Ps+pa))^0.4/1.4−1)`，`R=287.06`、`γ=1.4`（`cal_velocity_mach`（SKILL §5），与 Python 常量逐位一致）。`prb_interpolator.go`：`SevenHolePrbInterpolator` 实现 Task 1 的三方法接口 —— 编排（`Calculate`）：ka/kb 计算 → 内区判定 → 内区两阶段 或 大角度两扇区尝试 → cpt/cps → Pt/Ps 解析闭式（SKILL §2.5/§3.6 闭式，替代 sympy 消元，数学等价）→ V/Ma → 装配 `InterpolationResult`。数值守卫（全部返回 error 或 invalid+warning，**不 panic**）：分母 `<1e-12`；`Pt<Ps` 返回 error（SKILL §5 指明 Python `math.fabs` 为缺陷，Go 不复刻——与 Python 行为的有意差异，spec §4）；负 radicand；输入 NaN/Inf。具体类型另实现可选能力 `Identity() string`（不入接口，Task 1），返回含内区文件名+六扇区文件名的稳定标识；`GetValidRange()` 返回内区 ±30°（仅 UI，spec §2.2）。

**Acceptance criteria**

- [ ] 接口三方法完整；具体类型的可选 `Identity()` 稳定且含 7 文件名信息（经类型断言取用）
- [ ] `Pt<Ps` 返回 error（非 invalid+warning），测试显式锁定该与 Python 的有意差异（spec §4）
- [ ] 除零 / 负 radicand / NaN / Inf 各守卫用例均不 panic，且错误消息可读
- [ ] Pt/Ps 闭式解与 Python sympy 结果在单元级一致（用 Task 6 前的 3 个手算点核对，容差 rel 1e-9）
- [ ] 越界（小角度多边形外 + 两扇区均 miss）返回 `IsValid=false` + Warning，Warning 文案指明「不支持外推」

**Verification**

- [ ] `cd shared\algorithms\go\sevenhole; go test ./interpolation/ -v`
- [ ] `cd shared\algorithms\go\sevenhole; go vet ./...`

**Dependencies**：Task 4

**Files likely touched**

- `shared/algorithms/go/sevenhole/interpolation/atmosphere.go`（新增）
- `shared/algorithms/go/sevenhole/interpolation/atmosphere_test.go`（新增）
- `shared/algorithms/go/sevenhole/interpolation/prb_interpolator.go`（新增）
- `shared/algorithms/go/sevenhole/interpolation/prb_interpolator_test.go`（新增）

**Estimated scope**：M

---

### Task 6：Go↔Python 对拍闸门（481 点 golden + 8 边界用例）

**Description**

算法等价性的唯一判据（plan Architecture Decisions）。

**Python 环境边界**：Python 仅用于开发期一次性生成/更新 `.prb` 与 golden 夹具；提交后的 Go `golden_test.go` 只读取 `testdata`，CI 运行 Go 测试不依赖 Python、sympy 或 GBK 原始 CSV。夹具脚本文件头必须注明：当 `.prb` 格式、七孔算法、`seven_hole.py` API 或源数据集发生变化时，必须重新生成夹具并审查产物 diff。

1. 新增夹具生成脚本 `device-lab/skills/seven-hole-probe/tools/gen_traversal_fixtures.py`：
   - 将数据集 7 份 GBK CSV 转码为 UTF-8 副本（写入 `tools/` 下临时目录，**不改原文件**）；
   - 按**列位置**读数值（绕过表头历史命名错误）；命名映射：内区 `ka=Kα, kb=Kβ, cpt=K0, cps=Ks`；外区 `ka=Kθ[n], kb=Kφ[n], cpt=K0[n], cps=Ks[n], a=θ, b=φ`；
   - 由 CSV 系数生成 7 份 `.prb` 文本（内区 13×13=169 行；外区每份 4×13=52 行），落 `shared/algorithms/go/sevenhole/interpolation/testdata/prb/`；
   - 对 481 个标定点（169 小角度 + 6×52 大角度）调用 `seven_hole.cal_ab`（自写驱动，库无 `__main__`），输出 golden JSON（每点：输入 P1..P7/t/pa + 输出 alpha/beta/Pt/Ps/Ma/V），落 `testdata/golden/`；
   - 完整性断言：481 行齐全、无 NaN。
2. Go 侧 `golden_test.go`：加载 testdata .prb → 对 golden 每点调用 `Calculate` → 分级容差比较：角度 abs ≤1e-4°；Pt/Ps 对每个值断言 `abs(got-want) <= max(abs(want)*1e-6, 1e-4 Pa)`；Ma/V rel ≤1e-6。
3. 8 个构造边界用例（golden 同法生成或手算，**全部位于合法网格内**、与 Python 对拍）：内区 a/b=±30° 网格线、θ=30° 内区/外区交界网格点、θ=45° 外边界（最外圈网格线，边界按内部处理）、ka=kb=0 原点、纯侧滑（β≈0）、纯迎角（α≈0）、次大孔扇区回退例、扇区内一般点（φ 非特殊角）。
4. Go 产品策略守卫用例（**单独断言，依据 spec §4/§7.2，禁止声称与 Python 行为一致**；仅当 Python 同样报错的情形——如 1e-12 分母——才允许双向对照）：
   - 越界不外推：两候选扇区均 miss、θ>45°（外区网格上限之外）→ `IsValid=false` + Warning（Python 的 `beyond_border` 外推不复刻）；
   - `Pt<Ps` → 返回 error（Python `math.fabs` 缺陷不复刻）；
   - 1e-12 系列守卫、负 radicand、压力比 <1 → 返回 error；
   - 非法输入仅指 NaN/Inf/非有限数 → 返回 error；
   - **负表压是合法输入**（数据集大角度 1 区 P3≈-2771 Pa、Ps≈-30 Pa，已在 481 点 golden 内覆盖）：「单孔压力为负」「P7=0」不得作为拒绝条件（P7=0 仅在触发分母守卫时才报错）。

**Acceptance criteria**

- [ ] (a) 网格内标定点 golden（实测 435/481：240 小角度 + 195 大角度）+ 8 边界用例，与 Python 对拍在容差内 100% 通过；任一点失败时测试输出该点的中间量（ka/kb/系数/Pt/Ps）供二分定位。其余 46 点落在所有多边形外（Python 走 `beyond_border` 外推），Go 按 §4 不外推，断言 `IsValid=false`+Warning 而非数值对拍（2026-07 实施记录）
- [ ] (a2) 夹具抖动说明：数据集 CSV 系数仅 3 位小数，相邻网格点 ka/kb 产生精确相等值，Python 扫描到垂直边即 `ZeroDivisionError` 崩溃（195/481 点原本不可对拍）；夹具生成脚本对精确相等的退化边加确定性 ≤1e-7 抖动（双方读同一 `.prb`，等价性不受影响），已在脚本头注释记录（2026-07 实施记录）
- [ ] (b) Go 产品策略守卫用例按 spec §4/§7.2 断言通过（越界 invalid、Pt<Ps error、1e-12 系列、负 radicand、压力比 <1、NaN/Inf）；断言不引用 Python 行为作依据（仅 1e-12 分母等 Python 同报错情形允许双向对照）
- [ ] 负表压合法输入用例通过（数据集大角度 1 区 P3≈-2771 Pa 点包含在 golden 内）；「单孔压力为负」「P7=0」不被拒绝
- [ ] 负号保护：删除坐标变换负号后重跑，对拍/单测必须失败（spec §8.1）
- [ ] 夹具脚本与 testdata 提交版本库；`seven_hole.py` 与数据集原文件用其所属版本控制工具确认零改动
- [ ] Go 测试运行不依赖 Python 环境（golden 已落盘），CI 可重复；Python 仅在显式重新生成夹具时需要

**Verification**

- [ ] `python device-lab\skills\seven-hole-probe\tools\gen_traversal_fixtures.py` 成功且打印 481/481
- [ ] `cd shared\algorithms\go\sevenhole; go test ./interpolation/ -run 'TestGolden|TestBoundary|TestGuard' -v`
- [ ] 按文件归属使用 Git 或 SVN 检查状态，确认只修改夹具脚本与生成产物；若本机缺少对应 CLI，在完成说明中记录并使用可用的仓库状态工具/IDE 证据替代

**Dependencies**：Task 5

**Files likely touched**

- `device-lab/skills/seven-hole-probe/tools/gen_traversal_fixtures.py`（新增）
- `shared/algorithms/go/sevenhole/interpolation/testdata/prb/`（新增，7 份）
- `shared/algorithms/go/sevenhole/interpolation/testdata/golden/`（新增）
- `shared/algorithms/go/sevenhole/interpolation/golden_test.go`（新增）

**Estimated scope**：L（夹具目录按 1 个逻辑单元计）

**Checkpoint CP1/CP2（Phase 1 出口）**：
- [ ] `cd shared\algorithms\go\sevenhole; go test ./...` 全绿、`go vet ./...` 干净
- [ ] 对拍闸门 481+8 全过（合法网格内与 Python 一致）；Go 产品策略守卫用例按 spec §4/§7.2 断言通过
- [ ] `cd shared\algorithms\go\fivehole; go test ./...` 全绿（冻结证据）

---

## Phase 2：后端集成（`projects/wind-daq/services/api-go`）

### Task 7：core/traversal —— Config.ProbeType + 常量

**Description**

`internal/core/traversal/types.go`：`Config` 新增 `ProbeType string`（json `"probeType,omitempty"`），新增常量 `ProbeTypeFiveHole = "five-hole"`、`ProbeTypeSevenHole = "seven-hole"` 与 `IsSevenHole() bool` 辅助方法；空值仅作为旧配置兼容值，由边界规范化为五孔，未知非空值由 Task 10 拒绝。`CalculatedResult` 不变。core 层零 I/O 约束不破。

**Acceptance criteria**

- [ ] 既有 Config JSON（无 probeType 字段）反序列化后 `IsSevenHole()==false`
- [ ] 新常量与 JSON tag 与 spec §2.3 一致
- [ ] core 包测试全绿

**Verification**

- [ ] `cd projects\wind-daq\services\api-go; go test ./internal/core/... -run TestTraversal -v`
- [ ] `cd projects\wind-daq\services\api-go; go test ./internal/...` 全量零回归

**Dependencies**：Task 6（Phase 2 统一以对拍通过为前提）

**Files likely touched**

- `internal/core/traversal/types.go`
- `internal/core/traversal/types_test.go`

**Estimated scope**：S

---

### Task 8：usecase/traversal_probe.go —— 策略注册表 + Manager 七孔通路

**Description**

新增 `internal/usecase/traversal_probe.go`：`probeStrategy` 采用显式 receiver 签名（与 spec §5.2 一致，不用包级无参闭包）——`pressureLabels []string`、`isLoaded func(m *TraversalManager) bool`、`calculate func(m *TraversalManager, in probeCalcInput) (probeCalcResult, error)`；`probeStrategies map[string]probeStrategy` 是无状态包级表（`"five-hole"` / `"seven-hole"` 两项）。Manager 新增字段 `sevenHoleInterpolator seveninterp.Interpolator` 与方法 `SetSevenHoleInterpolator`、`ClearProbeInterpolator(probeType string) error`；五孔字段、`SetInterpolator`、`Interpolator()` **原样不动**。`HasLoadedInterpolator()` 改为按 `m.config.ProbeType` 经策略表判定，`CheckPreconditions` 自动获得探针感知。`BuildRawPressure` 签名不变，内部改走 `buildRawPressureForProbe(...)` 查表取标签；`normalizeRawPressure` 的硬编码 P1..P5 同样改为查表。新增 `CalculateRealtimeByProbe(probeType string, in probeCalcInput)`：先校验显式请求类型与当前 `m.config.ProbeType` 一致，再由对应策略分发；七孔直接调 `m.sevenHoleInterpolator.Calculate(...)`，五孔委托既有 `CalculateRealtime`。插值器清理由显式 `ClearProbeInterpolator` 执行（显式清除能力，探针切换不调用——双变体语义，见 Task 14 修订说明），不在 `SaveConfigRaw` 中隐式发生。`interpolatorIdentity` 经可选能力类型断言取用，基础接口不含 `Identity`。

**Acceptance criteria**

- [ ] 既有 `TestBuildRawPressure*` 全部原样通过（五孔行为逐字节锁定）
- [ ] 新增用例：七孔原始压力按 9 标签（P1..P7/Patm/Tatm）装配正确；乱序通道经标签归一化正确
- [ ] `CalculateRealtimeByProbe` 七孔走新字段、五孔走旧缓存路径（测试断言两条路径各自命中）
- [ ] 包级策略表不捕获 Manager；两个 Manager 并行加载不同插值器时计算结果互不串扰
- [ ] `CheckPreconditions` 经策略表探针感知：七孔配置未加载 → PRB 项报错；已加载 → 通过；五孔行为不变（既有测试）
- [ ] `ClearProbeInterpolator` 五孔/七孔分别只清指定类型；未知类型返回 error；清除后对应类型前置检查失败
- [ ] 显式请求 probeType 与当前配置不一致时拒绝计算，不读取另一类型插值器
- [ ] `interpolatorIdentity()` 对七孔具体插值器的类型断言路径测试通过，返回稳定标识且包含 7 个文件名信息；不得静默退化为空串

**Verification**

- [ ] `cd projects\wind-daq\services\api-go; go test ./internal/usecase/... -run 'TestBuildRawPressure|TestCalculateRealtime|TestCheckPreconditions|TestInterpolatorIdentity' -v`
- [ ] `cd projects\wind-daq\services\api-go; go test ./internal/...` 全量零回归

**Dependencies**：Task 7

**Files likely touched**

- `internal/usecase/traversal_probe.go`（新增）
- `internal/usecase/traversal_probe_test.go`（新增）
- `internal/usecase/traversal.go`
- `internal/usecase/traversal_view.go`

**Estimated scope**：M

---

### Task 9：ports + adapters —— LoadSevenHolePRB 贯通

**Description**

`internal/ports/interpolator_loader.go`：接口新增第 4 个方法 `LoadSevenHolePRB(innerPath string, outerPaths [6]string) (seveninterp.Interpolator, error)`（返回七孔包接口类型；ports 只允许接口与类型引用，不破零实现约束）。`internal/adapters/interpolation/loader.go`：`Loader` 实现该方法，委托 `files.go` 新增 `LoadSevenHolePrbFiles(innerPath string, outerPaths [6]string)`——用 `readNonEmptyLines` 读 7 份文件后，调算法包行接口逐份装载：`NewSevenHolePrbInterpolator()` → `LoadInnerPrbLines(innerLines, innerPath)` → 按孔号 1..6 逐份 `LoadOuterPrbLines(sector, lines, path)`（文件 I/O 只在 adapters 层，算法包零 I/O，Task 2；沿用 typed-nil 防护注释风格）。mock 实现同步补方法。

**Acceptance criteria**

- [ ] ports 接口编译通过，mock/fake 全部补齐
- [ ] adapters 用 Task 6 testdata 的 7 份 .prb 加载成功；缺文件/坏行数时 error 透传且含路径
- [ ] 五孔三个既有 Load 方法行为不变（既有测试）

**Verification**

- [ ] `cd projects\wind-daq\services\api-go; go test ./internal/adapters/interpolation/... -v`
- [ ] `cd projects\wind-daq\services\api-go; go test ./internal/...` 全量零回归

**Dependencies**：Task 8

**Files likely touched**

- `internal/ports/interpolator_loader.go`
- `internal/adapters/interpolation/loader.go`
- `internal/adapters/interpolation/files.go`
- `internal/adapters/interpolation/loader_test.go`（或既有测试文件补充）

**Estimated scope**：S

---

### Task 10：traversal_config.go —— probeType 透传 + 9 角色映射 + 恢复七孔分支

**Description**

`traversalAPIConfig` 在兼容旧扁平五孔 JSON 的读取 DTO 之外，新增 `probeType` 与 `sevenHolePrb` DTO。`ParseConfig` 在边界规范化（2026-07 修订为双变体语义）：缺省类型归一化为五孔；未知非空类型报错；激活七孔必须使用 `kind="seven-hole-prb-set"` 且 1+6 文件齐全；五孔字段与 `sevenHolePrb` 并存合法（持久化双变体），不再按"混合配置"拒绝。规范化后再透传 `core.Config.ProbeType`。`roleToLabel` 新增七孔 9 角色映射。恢复分支只恢复当前激活变体：七孔调 `LoadSevenHolePRB` + `SetSevenHoleInterpolator`，五孔保持原优先级链；未激活变体字段仅作持久化数据透传，不进入 usecase。

**Acceptance criteria**

- [ ] 旧配置（无 probeType）恢复行为与现行完全一致（既有 `TestParseConfig*` / 恢复测试原样通过）
- [ ] 七孔配置持久化→重启恢复→七孔 probeType 下 `HasLoadedInterpolator()==true`
- [ ] `roleToLabel` 七孔 9 角色映射逐项正确；五孔映射不变
- [ ] 七孔配置缺 outerFiles 或数量≠6 时恢复报错且消息可读
- [ ] 未知非空 probeType 返回校验 error 且不启动任何 loader；五孔/七孔字段并存的双变体配置按激活方正常恢复（2026-07 修订）
- [ ] 旧扁平五孔配置仍可读取，并被规范化为 five-hole 变体后恢复

**Verification**

- [ ] `cd projects\wind-daq\services\api-go; go test ./internal/usecase/... -run 'TestParseConfig|TestRestore|TestRoleToLabel' -v`
- [ ] `cd projects\wind-daq\services\api-go; go test ./internal/...` 全量零回归

**Dependencies**：Task 8、Task 9

**Files likely touched**

- `internal/usecase/traversal_config.go`
- `internal/usecase/traversal_config_test.go`

**Estimated scope**：M

---

### Task 11：csv_writer —— 标签优先级表追加 P6/P7

**Description**

`internal/adapters/storage/traversal_csv_writer.go` 的 `buildLabelEntries` 优先级表追加 `"P6","P7"`（位于 `"P5"` 之后、`"Patm"` 之前，保持 P1..P7,Patm,Tatm 顺序）。**唯一 CSV 侧改动**：计算列（`Alpha,Beta,Pt,Ps,Mach,SampleCount,DwellMs`）与 `buildHeader` 其余逻辑零改动。

**Acceptance criteria**

- [ ] 七孔 9 标签数据写 CSV，列序为 P1..P7,Patm,Tatm，其后其余通道按索引升序
- [ ] 五孔 CSV 输出逐字节不变（既有测试原样通过）
- [ ] 计算列头与内容不变

**Verification**

- [ ] `cd projects\wind-daq\services\api-go; go test ./internal/adapters/storage/... -run 'TestTraversalCSV|TestBuildHeader|TestBuildLabelEntries' -v`
- [ ] `cd projects\wind-daq\services\api-go; go test ./internal/...` 全量零回归

**Dependencies**：Task 8

**Files likely touched**

- `internal/adapters/storage/traversal_csv_writer.go`
- `internal/adapters/storage/traversal_csv_writer_test.go`

**Estimated scope**：S

---

### Task 12：api/server.go —— 导入、清理与 calculateRealtime 判别分发

**Description**

`/api/traversal/` 路由表新增 `importSevenHolePrb`：请求路径 DTO `innerFilePath + outerFilePaths[6]`，加载成功后只设置七孔插值器并返回逐文件信息对象。新增 `clearInterpolator`：请求必须显式携带合法 `probeType`，调用 `ClearProbeInterpolator`；不允许缺省猜测当前类型（显式清除能力，2026-07 修订：双变体语义下探针切换**不调用**本接口，保留给"移除校准文件后主动失效"类场景）。`calculateRealtime` 请求体增加可选 `probeType` + `pressures` 超集：旧五孔 body 可省略类型并按 five-hole 兼容；七孔必须显式传 seven-hole。API 解码 DTO 的 `P6`/`P7` 使用 `*float64`，`nil` 表示缺失，非 nil（包括 0）表示已提供；handler 校验后再解引用装配内部输入。禁止以普通 float64 零值或 P6/P7 是否出现猜测类型。既有五孔 action 和响应不变。

**Acceptance criteria**

- [ ] curl 冒烟：`importSevenHolePrb` 成功返回 7 文件信息；随后 `calculateRealtime`（含 P6/P7 body）返回七孔结果字段齐全
- [ ] `clearInterpolator`：七孔/五孔各一例，清空后对应前置检查报未加载；缺失/未知 probeType 返回 400
- [ ] 七孔 calculateRealtime 缺 probeType、缺 P6/P7、类型与当前配置不一致分别返回 400；P6/P7=0 不被当成缺失
- [ ] API DTO 测试覆盖 `P6/P7=nil` 与 `P6/P7=&0` 两种情况，证明 absent 与 present-zero 可区分
- [ ] 旧五孔 `calculateRealtime` body 响应与现行一致（既有 API 测试原样通过）
- [ ] `importPrb`、`importCalibrationCsv`、`importMultiPrb` 三个 action 零改动（diff 佐证）
- [ ] 各 4xx 错误路径消息明确（指明探针类型/缺文件）

**Verification**

- [ ] `cd projects\wind-daq\services\api-go; go test ./api/... -run 'TestTraversal|TestImportSevenHole|TestCalculateRealtime' -v`
- [ ] `cd projects\wind-daq\services\api-go; go test ./internal/...; go test ./api/...` 全量零回归
- [ ] 手动 curl 冒烟两条路径（命令与预期 JSON 记入任务完成说明）

**Dependencies**：Task 9、Task 10、Task 11

**Files likely touched**

- `api/server.go`
- `api/server_test.go`（或既有 API 测试文件补充）

**Estimated scope**：M

**Checkpoint CP3（Phase 2 出口）**：
- [ ] `cd projects\wind-daq\services\api-go; go test ./...` 全量全绿（含全部既有五孔用例）
- [ ] `git diff --stat` 确认五孔包零改动、usecase 改动仅限本 Phase 清单
- [ ] curl 冒烟：`importSevenHolePrb` + 七孔 `calculateRealtime` 通过

---

## Phase 3：前端（`apps/desktop-wails/frontend`）

### Task 13：类型层 —— 角色、判别配置、预设与展示元数据

**Description**

`shared/types/calibration.ts` 新增七孔 9 个角色。`shared/types/traversal.ts` 新增 `TraversalProbeType`、五孔/七孔插值配置和 `TraversalProbeConfig` 判别联合（spec §2.3）；七孔 `outerFiles` 在完整持久化类型中为固定长度 6 元组，编辑态可用独立 Draft 类型表达空槽位。新增七孔 9 通道预设、`SevenHolePrbFileInfo`、P6/P7 可选原始压力字段，以及 `normalizeTraversalProbeConfig(raw)` 纯函数：旧扁平五孔 JSON 读入后立即转成 five-hole 变体，未知类型或混合新配置返回错误。新增 `TRAVERSAL_PROBE_PRESENTATION`，只承载标题及 Alpha/Beta i18n key，不含计算逻辑。

**Acceptance criteria**

- [ ] 新枚举字符串值与后端 `roleToLabel` 键逐字一致（前后端契约测试或对照注释）
- [ ] 既有类型引用处零破坏性变更（`npm run typecheck` 通过）
- [ ] 预设通道数：five-hole 7 通道不变；seven-hole 9 通道且 CH 映射正确
- [ ] TypeScript 类型测试证明 seven-hole 变体不能赋值五孔 interpolation；完整七孔 outerFiles 必须恰为 6 项
- [ ] 旧五孔配置规范化成功；未知类型和混合新配置返回错误
- [ ] 展示元数据锁定五孔 Alpha=攻角/Beta=侧滑角，七孔 Alpha=侧滑角/Beta=迎角

**Verification**

- [ ] `cd projects\wind-daq\apps\desktop-wails\frontend; npm run typecheck`

**Dependencies**：Task 12（Phase 3 以后端契约定稿为前提）

**Files likely touched**

- `shared/types/calibration.ts`
- `shared/types/traversal.ts`

**Estimated scope**：S

---

### Task 14：traversalApi + traversalStore —— 七孔导入动作与状态推导

**Description**

`api/traversalApi.ts` 新增七孔导入和显式 `clearInterpolator(probeType)`，`calculateRealtime` 对七孔传 `probeType`。`traversalStore.ts` 新增七孔导入、显式清除用 `clearProbeInterpolator(probeType)` 与 `activateProbeType(probeType)`。探针切换采用**双变体激活切换**（2026-07 修订，替代原"事务式清理"）：仅更新 `config.probeType` 并按激活变体重新推导 `hasLoadedInterpolator`，随后经 `checkPreconditions` 复核后端真实加载状态；**不清理任何插值器、不重置任何变体字段**——五孔字段与 `sevenHolePrb` 在配置中并存，未激活插值器保持挂载但对计算/前置检查不可达。`hasLoadedInterpolator` 从激活变体配置和后端校验结果派生，不允许单独的 no-op 五孔动作掩盖非法调用；五孔专属动作接收 five-hole 变体并由类型系统限制调用。

**Acceptance criteria**

- [ ] 导入成功后 store 状态：插值器已加载、文件信息 7 项齐全
- [ ] `clearProbeInterpolator` 先清后端再原子更新本地；后端失败时 probeType、通道、PRB 和 loaded 状态逐项保持原值
- [ ] 五孔既有动作（importPrbFile / importCalibrationCsvFile / importMultiPrbFiles）行为不变
- [ ] `inferInterpolatorState` 对两种探针类型分别推导正确
- [ ] 七孔实时计算请求显式携带 `probeType: 'seven-hole'`；旧五孔请求仍可省略

**Verification**

- [ ] `cd projects\wind-daq\apps\desktop-wails\frontend; npm run typecheck`
- [ ] store 相关既有单测（如有）全绿：`npm run test -- --run traversalStore`（无则跳过并在完成说明注明）

**Dependencies**：Task 13

**Files likely touched**

- `api/traversalApi.ts`
- `stores/traversalStore.ts`

**Estimated scope**：S

---

### Task 15：向导 UI —— 公共壳 + 两类 PRB 子组件 + 动态展示语义

**Description**

`TraversalSettings.vue` 保持四步公共向导壳，为两种探针各维护一份配置包（五孔 refs 组 / 七孔 Draft + 通道），不新增两套平行 PRB refs。第 1 步新增探针选择；切换采用双变体换入换出（暂存当前侧、载入目标侧），**不重置、不弹确认**（2026-07 修订）。`TraversalPrbStep.vue` 仅作公共壳，根据判别类型渲染 `FiveHolePrbConfig.vue` 或 `SevenHolePrbConfig.vue`：前者从现有 PRB UI 原样抽取，后者管理 1+6 文件槽位；两个组件只接收各自类型的 model，不读取对方字段。`TraversalMain.vue`、实时监视和摘要从 `TRAVERSAL_PROBE_PRESENTATION` 读取标题和 Alpha/Beta 标签。补齐 zh/en i18n；前端不得包含插值公式。

**Acceptance criteria**

- [ ] 选择七孔 → 硬件步通道表变为 9 行预设；PRB 步显示 7 文件选择器且无算法/模式控件
- [ ] 切换探针类型即时生效（不重置、不弹确认）：两套配置包换入换出互不丢失，回切后通道与 PRB 配置原样恢复（2026-07 修订，替代原"确认+重置+回滚"语义）
- [ ] `TraversalPrbStep` 公共壳不持有五孔/七孔算法状态；两个子组件不存在读取对方配置字段的 prop/event
- [ ] 五孔向导全流程 UI 与行为零变化（手动对照）
- [ ] zh/en 双语无缺失键（界面无 raw key 显示）
- [ ] 保存 JSON 双变体并存（五孔字段 + `sevenHolePrb` + `inactiveProbeChannels`），`probeType` 标记激活方；重进向导两侧恢复正确；七孔主画面显示 Alpha=侧滑角、Beta=迎角

**Verification**

- [ ] `cd projects\wind-daq\apps\desktop-wails\frontend; npm run typecheck; npm run build`
- [ ] 按 spec §6 执行前端向导功能走查：七孔选型→导入 7 文件→核对预览→保存→重进恢复；结果记入完成说明。本 Task 不执行采集/CSV/重启后端等全链路 E2E，后者统一由 Task 16 按 spec §8.3 验收

**Dependencies**：Task 14

**Files likely touched**

- `components/traversal/TraversalSettings.vue`
- `components/traversal/TraversalPrbStep.vue`
- `components/traversal/FiveHolePrbConfig.vue`（新增）
- `components/traversal/SevenHolePrbConfig.vue`（新增）
- `stores/i18nStore.ts`
- `components/traversal/TraversalMain.vue`（仅展示元数据接入）

**Estimated scope**：L

**Checkpoint CP4（Phase 3 出口）**：
- [ ] `npm run typecheck` 与 `npm run build` 零错误
- [ ] 向导两种探针类型手动走查通过
- [ ] `git diff --stat` 确认 `TraversalHardwareStep.vue` 零改动（预期）

---

## Phase 4：端到端验收

### Task 16：全量验证 + 手动 E2E + 冻结证据

**Description**

汇总验收（spec §8）：自动测试全量 + 五孔回归 + 结构校验 + 手动 E2E 清单执行并记录。

**Acceptance criteria**

- [ ] `cd shared\algorithms\go\sevenhole; go test ./...` 全绿（含对拍 481+8）
- [ ] `cd shared\algorithms\go\fivehole; go test ./...` 全绿
- [ ] `cd projects\wind-daq\services\api-go; go test ./...` 全绿
- [ ] `cd projects\wind-daq\apps\desktop-wails\frontend; npm run typecheck; npm run build` 零错误
- [ ] `powershell -File scripts\validate-structure.ps1` 通过
- [ ] 用文件所属的版本控制工具确认冻结区零改动：Git 管理文件使用 `git status --short`/`git diff -- <path>`；SVN 管理文件使用 `svn status`/`svn diff --summarize`。覆盖 `fivehole/`、`threehole/`、`seven_hole.py` 和原始数据集；若当前环境缺少某一 CLI，必须记录未执行项并由具备该工具的验收环境补证
- [ ] 手动 E2E 清单全部勾选：七孔配置→导入 7 份 .prb→执行遍历→CSV 列 P1..P7,Patm,Tatm 且计算列填充→重启恢复→五孔遍历冒烟不回归

**Verification**

- [ ] 上述命令逐条执行并留存输出摘要
- [ ] E2E 清单结果写入任务完成说明（含 curl 响应样例与 CSV 片段）

**Dependencies**：Task 15

**Files likely touched**：无（仅验证；发现问题回到对应 Task 修复）

**Estimated scope**：M

**Checkpoint CP5（最终验收）**：spec §8 验收标准逐条过。

---

## 最终验收（Definition of Done）

1. 三份文档（spec/plan/tasks）与代码状态一致；任何实现期对 spec 决策的偏离已回写文档。
2. 对拍闸门 481 标定点 + 8 边界用例全过（合法网格内与 Python 一致）；Go 产品策略守卫用例按 spec §4/§7.2 断言通过。
3. `go test ./...`（sevenhole、fivehole、api-go）全绿；`npm run typecheck && npm run build` 零错误；`validate-structure.ps1` 通过。
4. 五孔包、三孔包、`seven_hole.py`、数据集原文件零改动（按文件归属使用 Git/SVN 状态与 diff 佐证）。
5. 手动 E2E 清单全勾，CSV 列序与计算列符合 spec §5.5。
6. spec §10 待确认项 Q1–Q4 的处理结果已在代码/文档中体现。

## Out of Scope

- 七孔多 PRB（马赫模式）与新算法校准 CSV 数据源
- SKILL.md §3.8 `beyond_border` 外推
- `InterpolationCache` 泛化与七孔接入
- 前端任何插值算法；Wails binding 重生成
- 七孔校准模块实施（types.go 骨架之后的 tasks 2-24）

## See Also

- [spec-seven-hole-traversal.md](./spec-seven-hole-traversal.md) / [plan-seven-hole-traversal.md](./plan-seven-hole-traversal.md)
- 同套流程先例：[spec-seven-hole-calibration.md](./spec-seven-hole-calibration.md) / [plan-seven-hole-calibration.md](./plan-seven-hole-calibration.md) / [tasks-seven-hole-calibration.md](./tasks-seven-hole-calibration.md)
- 算法权威：`device-lab/skills/seven-hole-probe/SKILL.md` 与 `seven_hole.py`（只读）
- 数据集：`projects/wind-daq/docs/W532.202608.P.7H.1-01/`（GBK，只读）
