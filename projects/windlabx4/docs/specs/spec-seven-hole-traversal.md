# 七孔探针遍历测试插值模块规范（Spec）

> 文档目的：定义七孔探针遍历测试（移位测试）插值模块的数据模型、算法契约、越界行为决策、后端/前端集成设计与验收标准，作为开发与验收的依据。
> 关联计划：[plan-seven-hole-traversal.md](./plan-seven-hole-traversal.md)
> 关联任务：[tasks-seven-hole-traversal.md](./tasks-seven-hole-traversal.md)
> 算法权威：`device-lab/skills/seven-hole-probe/SKILL.md`（算法唯一权威说明）+ `device-lab/skills/seven-hole-probe/seven_hole.py`（Python 参考实现，对拍基准）。
> 关联规范：[spec-seven-hole-calibration.md](./spec-seven-hole-calibration.md)（七孔校准流程规范，压力基准三分离、角度坐标系、孔位布局均以该规范为准）。
> 参考实现：五孔遍历插值链路（`shared/algorithms/go/fivehole/interpolation/`、`services/api-go/internal/usecase/traversal*.go`）。
> 日期：2026-07-17
> 状态：已完成（2026-07 修订：探针切换改为双变体并存语义，见 §2.3/§5.2.1/§6.2）

---

## 1. 文档范围与术语

### 1.1 与五孔遍历模块的关系

七孔遍历测试的状态机、运动安全、布点（line/rectangle/sector/custom）、采集循环、断点恢复、CSV 写入、状态推送、前端主画面（`TraversalMain.vue` 及可视化组件）**全部复用**五孔遍历模块的既有实现；本规范仅定义"插值线"上七孔特有的部分：

- 七孔插值 Go 包（`shared/algorithms/go/sevenhole/interpolation/`，新模块）
- `.prb` 文件集（7.prb + 1.prb~6.prb）的加载与校验
- `probeType` 维度贯通配置、插值器恢复、通道角色映射、API 与前端向导
- usecase 层按探针类型的**策略注册表**（本模块唯一的结构性重构，保持最小）
- 前端保留一个遍历向导壳，但将五孔/七孔 PRB 配置拆为独立子组件，禁止在公共组件内堆叠算法细节
- Go↔Python 对拍验证（算法正确性闸门）

**明确不做**（本期范围外）：多 PRB 按马赫数插值模式、校准 CSV 新算法路径（待七孔**校准**模块 tasks 2-24 落地后再接入，见 §10）、外区越界外推（§4）、前端任何插值算法实现。

### 1.2 术语约定

| 术语 | 含义 |
|---|---|
| 小角度模式 / 内区 | 来流接近正对探针（θ ≲ 30°），使用 7.prb（13×13=169 点网格）的插值路径 |
| 大角度模式 / 外区 | 来流大幅偏离探针轴（30° ≤ θ ≤ 45°），使用最大压力孔 n 对应的 n.prb（每扇区 4×13=52 点网格） |
| 扇区编号 n | 大角度模式最大压力孔编号，n ∈ {1..6}；环形取模：n=1 时左邻为 P6，n=6 时右邻为 P1 |
| (ka, kb) | 由实测压力计算的方向系数对，插值的定位坐标（无量纲） |
| (cpt, cps) | 总压/静压系数，插值出 Pt/Ps 的中间量（无量纲） |
| (a, b) | 小角度模式：插值输出的角度对（°）；大角度模式：校准网格坐标，a=θ、b=φ，需经坐标变换输出（§3.3） |
| θ / φ | 俯仰角（恒正）/ 滚转角（从 +Y 起算、从探针尾部看顺时针递增），定义与校准规范 §3.3 一致 |
| α / β | 七孔输出角度：α=侧滑角、β=迎角（**注意与五孔遍历"Alpha=攻角、Beta=侧滑角"定义不同**，见 §2.4） |
| 对拍 | Go 实现与 Python 参考实现在相同输入下的输出一致性比对 |
| 策略注册表 | usecase 层 `map[probeType]probeStrategy`，把探针相关的标签集/输入构造/插值器访问集中在一处（§5.2） |

### 1.3 与七孔校准模块的依赖关系

七孔**校准**模块（spec-seven-hole-calibration.md）当前仅完成类型骨架（`core/calibration/types.go` 的 `TypeSevenHole`/`SevenHoleRawData`/`SevenHoleCoefficients`/`SevenHoleDataPoint`），tasks 2-24 未实施。本模块**不依赖**校准模块的任何运行时产物：遍历插值只需要用户离线准备好的 7 个 `.prb` 文件（典型来源：历史校准数据或后续校准模块导出）。校准模块落地后，可追加"校准 CSV → 七孔插值"数据源路径（§10 Q2），届时不改动本规范定义的插值包契约。

### 1.4 模块化设计原则

本模块采用“**公共遍历流程 + 探针能力策略 + 独立算法包**”的组合式设计，不建立 `BaseProbe → FiveHoleProbe/SevenHoleProbe` 继承体系。Go/Vue 侧均以小接口、显式组合和判别配置约束变化：

| 层次 | 公共部分 | 探针特有部分 | 禁止事项 |
|---|---|---|---|
| 算法 | `Interpolator` 契约形状、有限数/错误语义 | fivehole 与 sevenhole 各自独立包、公式、网格和越界策略 | 通用 N 孔基类；算法包文件 I/O |
| usecase | 遍历状态机、采集、运动、落盘、恢复 | `probeStrategy` 负责标签、输入装配、加载状态和计算适配 | 在采集/视图/恢复各处重复 `if probeType` |
| API | 统一 traversal 路由和错误形状 | 导入 DTO、实时计算的 `probeType` 判别与字段校验 | 依赖隐式服务器状态猜测请求类型 |
| frontend | 四步向导壳、硬件/布点/摘要、执行主画面 | 探针选择器、五孔 PRB 子组件、七孔 PRB 子组件、展示元数据 | 复制整套七孔页面；在公共组件实现插值算法 |

新增第三种探针时，目标改动面应主要限制在：独立算法包、一个后端策略表项、一个探针配置子组件及一组展示元数据；公共状态机和运动/采集流程不应修改。

---

## 2. 数据模型与文件格式

### 2.1 .prb 文件集

七孔插值的校准数据为 **7 个 `.prb` 文件组成的文件集**：

| 文件 | 模式 | 数据行数 | 网格定义 |
|---|---|---|---|
| `7.prb` | 小角度（内区） | **169**（13×13） | a,b ∈ [-30°, 30°]，步长 5° |
| `1.prb` ~ `6.prb` | 大角度（外区，每孔一份） | **52**（4×13） | a=θ ∈ {30°, 35°, 40°, 45°}；b=φ 覆盖该扇区 13 条网格线（中心 ±30°，步长 5°，归一化到 [0°, 360°)） |

**行格式**：首行为表头（内容不解析，仅跳过，兼容网格尺寸表头或列名表头）；每个数据行 6 列空白分隔：

```
ka  kb  cpt  cps  a  b
```

**扇区 φ 网格线**（与 Python `big_create_square`/`big_create_line` 一致，n 为孔号）：

| 孔号 n | 左边缘 | 中心 | 右边缘 | 13 条 b 网格线（降序，归一化） |
|---|---|---|---|---|
| 1 | 330° | 0° | 30° | 30, 25, …, 0, 355, …, 330 |
| 2 | 30° | 60° | 90° | 90, 85, …, 30 |
| 3 | 90° | 120° | 150° | 150, 145, …, 90 |
| 4 | 150° | 180° | 210° | 210, 205, …, 150 |
| 5 | 210° | 240° | 270° | 270, 265, …, 210 |
| 6 | 270° | 300° | 330° | 330, 325, …, 270 |

> **行数决策（52 而非 48）**：SKILL.md 对外区“12 行/12 区间”的文字存在歧义，但 Python `big_create_square` 以 `j ∈ range(12)` 构建 12 个 b 区间，四边形闭合需要第 13 条 b 网格线；`big_create_line` 的两条水平边界亦各取 13 点。因此每份外区文件实际为 4 a × 13 b = **52 个网格点**。佐证：`W532.202608.P.7H.1-01` 数据集每个大角度区 CSV 恰为 13 φ × 4 θ = 52 点（spec-seven-hole-calibration.md §6.2 数据集模式）。加载校验按 52 行执行（§5.3）。最终验证由 Task 6 夹具脚本从数据集 CSV 实际生成 7 份 `.prb` 时完成；任一外区生成结果不等于 52 行时，脚本必须断言失败。

**命名对应**（SKILL.md §1.2）：`.prb` 的 `ka, kb, cpt, cps` 对应校准规范的内区 `Kα, Kβ, K0, Ks` 与外区 `Kθ[n], Kφ[n], K0[n], Ks[n]`，物理含义相同、记号风格不同。

### 2.2 Go 类型（`shared/algorithms/go/sevenhole/interpolation/types.go`）

与五孔包同形（`shared/algorithms/go/fivehole/interpolation/types.go`），保持 per-probe 独立包、不抽象跨探针通用接口（三孔包 `shared/algorithms/go/threehole/interpolation/` 为先例）：

```go
// InterpolationInput 七孔插值输入（压力单位 Pa，基准见 §2.4）
type InterpolationInput struct {
    P1, P2, P3, P4, P5, P6, P7 float64 // 表压 Pa（P7 为中心孔）
    PAtm float64                      // 大气压力，绝压 Pa
    TAtm float64                      // 大气温度，℃
}

// InterpolationResult 七孔插值结果；JSON 字段名与五孔对齐，前端视图零改动
type InterpolationResult struct {
    Alpha           float64 `json:"alpha"`           // 侧滑角 a（°）
    Beta            float64 `json:"beta"`            // 迎角 b（°）
    MachNumber      float64 `json:"machNumber"`      // 马赫数
    Velocity        float64 `json:"velocity"`        // 速度（m/s）
    TotalPressure   float64 `json:"P0"`              // 总压（表压 Pa）
    StaticPressure  float64 `json:"Ps"`              // 静压（表压 Pa）
    DynamicPressure float64 `json:"dynamicPressure"` // 动压 Pt-Ps（Pa）
    IsValid         bool    `json:"isValid"`         // 结果是否有效
    Warning         string  `json:"warning,omitempty"`
}

// PrbValidRange / Interpolator 与五孔包同形：
//   IsLoaded() bool
//   GetValidRange() PrbValidRange
//   Calculate(InterpolationInput) (InterpolationResult, error)
```

`Identity() string` **不加入基础 `Interpolator` 接口**；快照逻辑继续使用现有可选能力接口 `interface{ Identity() string }`。这样既保持与五孔基础接口同形，也不强迫所有插值器暴露与计算无关的方法。

> **字段命名取舍**：`TotalPressure`/`StaticPressure` 的 JSON tag 沿用五孔的 `"P0"`/`"Ps"`（`fivehole/interpolation/types.go` L17-18），使 `internal/core/traversal/types.go` 的 `CalculatedResult{Valid, Alpha, Beta, Pt, Ps, Mach}` 与 CSV 计算结果列（Alpha/Beta/Pt/Ps/Mach）对两种探针保持同一形状——这是"CSV writer 与前端视图零改动"的关键约定（§5.5）。七孔包不提供五孔特有的 `V/Vx/Vy/Vz/CAS/SAT/Density` 字段（本期无消费方）。

> **GetValidRange 语义**：返回内区网格范围（±30°，取自 7.prb 数据行），**仅供 UI 展示**；不用于五孔式的"解析角度超范围 → IsValid=false"后验判定——七孔的越界判定已在多边形/四边形定位阶段完成（§4），外区合法结果经坐标变换后 |α| 可达 45°，套用内区范围检查会误杀。

### 2.3 Config.ProbeType 与通道角色

`internal/core/traversal/types.go` 的 `Config` 新增：

```go
// ProbeType 探针类型："five-hole"（默认，空串等价）/ "seven-hole"。
// 仅标记当前激活的探针，驱动插值器选择、通道标签集与输入装配；
// 不影响状态机/布点/落盘主流程。
ProbeType string `json:"probeType,omitempty"`
```

配置在 JSON/API 边界采用**双变体并存**模型（2026-07 修订，替代原"互斥变体"决策）：五孔扁平字段与 `sevenHolePrb` 允许同时出现在同一份配置 JSON 中，`probeType` 仅标记**当前激活**的探针变体。前端为两种探针各维护一套完整配置（通道绑定 + 插值配置），切换探针时换入换出、互不丢失：

```ts
// 持久化 JSON 允许双变体并存：
{
  "probeType": "seven-hole",                    // 激活方
  "prbFile": { ... },                            // 五孔变体字段（未激活，原样保留）
  "multiPrb": { ... },
  "sevenHolePrb": {                              // 七孔变体字段（激活，必须齐全）
    "kind": "seven-hole-prb-set",
    "innerFile":  { "filePath": "D:/cal/7.prb" },
    "outerFiles": [ { "filePath": "D:/cal/1.prb" }, …共 6 份… ]
  },
  "inactiveProbeChannels": [ ... ]               // 未激活侧通道绑定（后端忽略）
}
```

类型层仍按激活变体建模（`TraversalProbeConfig` 判别联合，见 §6.1），未激活变体字段在持久化往返中原样透传、不进入运行时判别配置。边界规则：

- **未知非空 `probeType` 必须报错**（前后端一致），不得静默降级；
- **激活七孔**时 `sevenHolePrb` 必须 `kind="seven-hole-prb-set"` 且 1+6 文件齐全，否则边界报错；
- 空 `probeType` 仅在读取旧配置时归一化为 `five-hole`；
- 旧扁平五孔 JSON（无 `probeType`、无 `sevenHolePrb`）在读取边界兼容并规范化。

`CalculatedResult{Valid, Alpha, Beta, Pt, Ps, Mach}` **不改动**——七孔输出经 §3.3 坐标变换后与五孔输出形状一致。

**通道角色**（9 个，`roleToLabel` 映射目标与五孔同标签空间）：

| 角色 | 映射标签 | 含义 | 参考默认通道 |
|---|---|---|---|
| `sevenHole.p1`~`sevenHole.p6` | `P1`~`P6` | 外围 6 孔（60° 等分环形） | CH1~CH6 |
| `sevenHole.p7` | `P7` | 中心孔 | CH7 |
| `sevenHole.pAtm` | `Patm` | 大气压力 | CH17 |
| `sevenHole.tAtm` | `Tatm` | 大气温度 | CH18 |

> 七孔遍历**不需要**校准用的 `sevenHole.pTotal`/`sevenHole.pTunnelStatic` 角色——插值反算从探针压力直接解 Pt/Ps，不消费风洞参考总/静压。角色全集与 spec-seven-hole-calibration.md §9.4 共用同一命名空间。

### 2.4 压力基准声明

与 spec-seven-hole-calibration.md §2.1 的**压力基准三分离**对齐：

| 基准 | 数据流位置 | 取值 | 说明 |
|---|---|---|---|
| A. 通道原始值 | 设备读取 → `BuildRawPressure` 归一化前 | 表压或绝压（由 `pProbePressureType` 声明） | 与五孔遍历同一归一化管线 |
| B. 插值输入值 | `InterpolationInput.P1..P7` | **表压 Pa（gauge）** | 系数 ka/kb/cpt/cps 均为压差比，表压/绝压同基准等价，与 SKILL.md §1.1 "Pa（表压）"一致 |
| C. 大气计算值 | V/Ma 公式内部 | **绝压** | 仅在 V/Ma 计算边界做一次 `p_abs = p_gauge + PAtm`（SKILL.md §5：压力比 `(Pt+pa)/(Ps+pa)`），禁止提前转绝压 |

- `PAtm` 始终为绝压 Pa，`TAtm` 为 ℃（SKILL.md §1.1/§5）。
- `pProbePressureType`（"gauge"/"absolute"）语义从"P1-P5"扩展为"**当前探针全部压力孔通道 P1..Pn**"，七孔与五孔共用同一开关与归一化实现（`pressure.NormalizePressureToGaugePa`）。
- CSV 落盘：原始压力列记录表压（与五孔一致），计算结果列 Pt/Ps 同为表压。

---

## 3. 插值算法契约

### 3.1 权威声明

七孔插值算法**不在本规范重新定义**。唯一权威：`device-lab/skills/seven-hole-probe/SKILL.md`（算法说明）+ `device-lab/skills/seven-hole-probe/seven_hole.py`（可执行参考）。Go 实现必须逐函数对齐 Python 参考的数值行为，并以对拍测试（§7）证明等价。本节仅给出契约级摘要；实现中遇到任何细节分歧（顶点排列、边界回退、负号、归一化），以 Python 参考为准。

### 3.2 算法管线摘要

**输入**：P1~P7（表压 Pa）、PAtm（绝压 Pa）、TAtm（℃）+ 已加载的 7 份 .prb 网格。

**模式判定（双模式，θ=30° 几何边界）**：先用全部 7 孔计算小角度系数 `(ka, kb)`（对称化 cpa/cpb/cpc → ka/kb，SKILL.md §2.1），再以**射线法**判定 `(ka,kb)` 是否落在 7.prb 边界多边形内（SKILL.md §4，`little_create_line` + `point_in_polygon`）：

- **多边形内（含边界）→ 小角度模式**：144 个四边形定位 + 距离反比插值得 (a,b)（SKILL.md §2.3）；双线性插值得 cpt/cps（§2.4）；解析求解 Pt/Ps（§2.5，`denom = 1+cpt+cps`）。输出角度即 (a,b)。
- **多边形外 → 大角度模式**：取最大压力孔 n（外围 6 孔中最大者）计算扇区系数 (ka,kb)（§3.1），射线法判定是否落入 n 扇区边界多边形（§3.3）；**未命中时回退到第二最大压力孔的扇区再判一次**（Python `cal_ab` 的 first/second 候选逻辑，属算法本体，予以保留）；命中后 36 个四边形定位 + 距离反比插值（§3.4）、反向双线性插值 cpt/cps（§3.5，方向与小角度相反）、解析 Pt/Ps（§3.6）；最后经 §3.3 坐标变换输出 (a,b)。
- **两候选扇区均未命中**：越界，按 §4 决策返回 `IsValid=false`（不外推）。

**V/Ma**（SKILL.md §9）：`R=287.06`、`γ=1.4`；`V = √(2·(Pt-Ps)·R·(TAtm+273.15)/PAtm)`；`Ma = √(5·((Pt+PAtm)/(Ps+PAtm))^(0.4/1.4) - 1)`。

**数值安全**（本规范对 Python 参考的产品化修正，全部为硬性契约）：

| 位置 | 守卫 |
|---|---|
| 小角度 `\|p7 - p_avg\|` | < 1e-12 → 返回错误 |
| 大角度 `\|p_center - p_side\|` | < 1e-12 → 返回错误 |
| 解析求解 `\|1 + cpt + cps\|` | < 1e-12 → 返回错误 |
| 坐标变换 `\|θ\|` | ≥ 89.5° → 跳过 tan 变换（退化分支，标准 .prb 网格 θ≤45° 下不可达，仍需实现并单测） |
| `Pt < Ps` | 见 §4 的产品决策；此处不重复定义错误语义 |
| 压力比 < 1 | 返回错误 |

**错误处理**：所有函数返回 `(result, error)`，错误信息含阶段 + 孔号 + 具体值（如 `大角度模式孔%d: |denom|=%.6e < 1e-12`）。`Calculate` 顶层 recover panic 转为 error（对齐五孔 `PrbInterpolator.Calculate`）。负表压和单个压力值为 0 均是合法输入；只有非有限数、非法大气参数、具体公式分母退化或物理结果非法时才报错。

### 3.3 坐标变换（大角度模式出口）

大角度插值输出的 (a,b) 是网格坐标 (θ,φ)，须变换为 (α,β) 后返回（SKILL.md §3.7）：

```
β =  degrees(atan(tan(radians(θ)) · cos(radians(φ))))
α = -degrees(atan(tan(radians(θ)) · sin(radians(φ))))   // 负号必须保留
```

负号来自"φ 从 +Y 起算、从探针尾部看顺时针递增"的约定，与 spec-seven-hole-calibration.md §3.3 的正向公式完全一致（α+ ↔ φ=270°，α- ↔ φ=90°）。删除负号会使 α 符号反转，对拍与黄金用例均可捕获。

### 3.4 预计算与缓存

校准网格加载后不变：四边形列表、边界多边形在 `LoadXxx` 阶段一次性预计算并缓存（小角度 144 四边形 + 边界；大角度每扇区 36 四边形 + 4 条边界），`Calculate` 不重复构建。插值器实例的并发安全语义与五孔一致（`Calculate` 只读预计算结构，可无锁并发调用）。

---

## 4. 越界行为决策（对齐五孔，不外推）

> **决策**：七孔插值器**不做任何边界外推**。`(ka,kb)` 落在小角度多边形与两个候选扇区多边形之外时，返回 `InterpolationResult{IsValid: false, Warning: "..."}`，不返回角度/压力数值。SKILL.md §3.8 的 `beyond_border` 边界外推**不实现**。

**依据（五孔既有行为，代码证据）**：

| 证据 | 位置 | 行为 |
|---|---|---|
| 插值策略与运行时行为 | `fivehole/interpolation/prb_interpolator.go` 的 `PrbInterpolator.Calculate` 及网格定位函数 | 网格外返回 `IsValid=false`，不做外推 |
| 范围后验 | 同文件的结果有效范围检查 | 解析角度超出 PRB 表范围 → `IsValid=false` + warning |
| Pt<Ps 处理 | 同文件的动压有效性检查 | 动压 ≤ 0 → `IsValid=false` + `Warning="总压低于静压 (pt < ps)"` |

**对齐细则**：

| 场景 | 七孔行为 | 与五孔对应 |
|---|---|---|
| (ka,kb) 在所有候选多边形外 | `IsValid=false`，Warning 指明越界（如"压力系数超出七孔PRB校准网格，不支持外推"） | 五孔同款 warning 语义 |
| θ > 45°（外区网格上限） | 同上（属网格外） | 五孔 \|α\|/\|β\| > 30° 同款 |
| Pt < Ps | **返回 error** | 五孔为 IsValid=false + warning——**有意差异**：Python 的 `fabs` 行为不复刻；usecase 侧对 error 按“插值失败、该点不写计算列”处理（见 `traversal_acquisition.go` 的插值错误分支） |
| 数值守卫触发（1e-12 系列） | 返回 error | 五孔无等价守卫（其 clamp 策略为五孔特有，不移植） |

> 该决策消除两类风险：(1) 外推结果无校准数据支撑，误差不可控；(2) 与五孔遍历"越界即无效"的操作员心智模型一致。后续若需外推，须以新 spec 增补并在 UI 显著标注外推点。

---

## 5. 后端集成设计

### 5.1 新增 Go 包布局

新模块 `shared/algorithms/go/sevenhole/`（独立 go.mod，module 名 `ai-workspace/shared/algorithms/go/sevenhole`，与 fivehole/threehole 模块布局一致）：

| 文件 | 内容 |
|---|---|
| `go.mod` | `module ai-workspace/shared/algorithms/go/sevenhole` |
| `interpolation/types.go` | §2.2 类型 + `Interpolator` 接口 + `PrbValidRange` |
| `interpolation/prb_loader.go` | .prb 行集解析/校验/索引（`LoadInnerPrbLines` / `LoadOuterPrbLines`，纯文本行输入，**文件 I/O 在 adapters**，与五孔 `LoadPrbLines` 同模式） |
| `interpolation/geometry.go` | 射线法多边形判定、四边形边方程、距离反比插值、角度归一化 |
| `interpolation/inner_zone.go` | 小角度模式全流程（§3.2） |
| `interpolation/outer_zone.go` | 大角度模式全流程 + 坐标变换（§3.2/§3.3） |
| `interpolation/atmosphere.go` | V/Ma（SKILL.md §5） |
| `interpolation/prb_interpolator.go` | `SevenHolePrbInterpolator` 编排：模式判定 → 分派 → 结果装配；`IsLoaded`/`GetValidRange`/`Calculate`；`Identity() string`（供快照，与 `TraversalManager.interpolatorIdentity` 契约一致） |

> **`Identity()` 是具体类型的可选能力方法，不是 `Interpolator` 接口成员**：接口仅含 `IsLoaded`/`GetValidRange`/`Calculate` 三方法（§2.2，与五孔 `Interpolator` 同形）；消费方由 `TraversalManager.interpolatorIdentity` 经类型断言 `interface{ Identity() string }` 取用。

**构建注册**：`go.work` 的 `use` 列表加入 `./shared/algorithms/go/sevenhole`；`projects/WindLabX4/services/api-go/go.mod` 加入 require + `replace ai-workspace/shared/algorithms/go/sevenhole => ../../../../shared/algorithms/go/sevenhole`（与 fivehole L6/L13 同款）。

### 5.2 usecase 策略注册表（唯一结构性重构）

现状：五孔硬编码散布在 `traversal_view.go` 的 legacy 标签/输入装配、`traversal_config.go` 的 `roleToLabel`、`traversal.go` 的 `CalculateRealtime`。本期以**一个注册表**收编探针差异（新增文件 `internal/usecase/traversal_probe.go`）：

```go
// probeCalcInput 探针无关插值输入（P 五孔仅用前 5 元素）
type probeCalcInput struct {
    P    [7]float64
    PAtm float64
    TAtm float64
}

// probeCalcResult 探针无关插值结果（落盘/视图共用标量子集）
type probeCalcResult struct {
    Alpha, Beta, Pt, Ps, Mach, Velocity float64
    IsValid  bool
    Warning  string
}

// probeStrategy 按探针类型的策略表项
type probeStrategy struct {
    pressureLabels []string                    // 压力孔标签：五孔 P1..P5 / 七孔 P1..P7
    isLoaded       func(m *TraversalManager) bool
    calculate      func(m *TraversalManager, in probeCalcInput) (probeCalcResult, error)
}

var probeStrategies = map[string]probeStrategy{
    "five-hole":  { ... }, // 内部走既有 CalculateRealtime（保留 InterpolationCache 路径）
    "seven-hole": { ... }, // 直接调 m.sevenHoleInterpolator.Calculate（一期不经缓存，见下）
}
```

策略表必须是**无状态包级定义**，函数显式接收 `*TraversalManager`；禁止用闭包捕获某个 Manager 实例。该约束保证多个 Manager/测试实例并行时不共享运行状态。

要点：

1. `TraversalManager` 新增字段 `sevenHoleInterpolator seveninterp.Interpolator` + `SetSevenHoleInterpolator()`；既有 `interpolator coreinterp.Interpolator` 与 `SetInterpolator()` **不动**（五孔零回归）。
2. `HasLoadedInterpolator()` 改为按 `m.config.ProbeType` 经策略表判定；`CheckPreconditions` 的 PRB 检查自动获得探针感知（其 Patm/Tatm 检查与探针无关，不变）。
3. `BuildRawPressure` 保持导出签名不变（五孔包装，既有测试零回归）；新增内部 `buildRawPressureForProbe(values, labels, deviceID, unitProvider, pressureType, strategy)`，标签集与输入装配由策略表驱动；采集和实时视图路径按 `config.ProbeType` 取策略后调用。legacy 无标签回退路径仅保留五孔行为（七孔配置必经新前端写入 role 映射）。
4. `CalculateRealtime(input coreinterp.InterpolationInput)` 导出方法保留（五孔 + 既有测试/调用方）；新增 `CalculateRealtimeByProbe(in probeCalcInput) (probeCalcResult, error)` 按 `m.config.ProbeType` 分发，供采集落盘与 API 使用。
5. **七孔一期不复用 `realtime.InterpolationCache`**：该缓存类型绑定五孔 `coreinterp.InterpolationInput/Result`（`core/realtime/realtime.go` L26/L62），泛化属于 core 改动，本期规避（七孔单次计算为 O(169+2×52) 网格扫描，耗时微秒级，缓存收益可忽略）。列入 plan 的 Out of Scope。

### 5.2.1 活动探针与插值器生命周期（2026-07 修订：双变体语义）

`probeType`、当前探针配置和可用插值器必须保持一致；**后端所有计算与前置检查仅通过当前 `config.ProbeType` 对应的策略访问插值器**（§5.2 策略注册表），这是防止陈旧校准被误用的唯一机制——未激活插值器保持挂载但对计算/前置检查不可达，无需随切换清除：

1. Manager 可分别持有五孔/七孔插值器；两者同时挂载互不影响（`interpolator` 与 `sevenHoleInterpolator` 独立字段）。
2. 保留 `ClearProbeInterpolator(probeType string)` 与 `POST /api/traversal/clearInterpolator` 作为**显式清除**能力（如用户移除校准文件后主动失效），探针切换**不调用**该接口。
3. 前端切换探针类型时**不重置、不弹确认**：向导为两种探针各存一份配置包（通道绑定 + PRB 状态），切换即时换入换出；`activateProbeType` 仅切换激活 `probeType` 并按激活变体重新推导 `hasLoadedInterpolator`，随后经 `checkPreconditions` 复核后端真实加载状态。
4. 导入成功只设置请求中明确指定类型的插值器；不得通过 P6/P7 是否出现猜测探针类型。
5. 启动恢复只恢复当前 `probeType` 对应的数据源；配置 JSON 允许双变体并存，未激活变体字段仅作持久化数据透传，不进入 usecase。
6. 双变体并存下，重启后切到未恢复侧时：`hasLoadedInterpolator` 经 `checkPreconditions` 纠正为未加载并展示根因，用户重新导入即可（与五孔 PRB 文件被删除时的既有体验一致）。
7. `SaveConfigRaw` 将 `probeType` 同步到 `m.config.ProbeType`（导入后、Start 前的实时计算一致性校验依赖）；`SaveConfigRaw` 不承担隐式清理职责。

### 5.3 ports 与 adapters 加载链

`internal/ports/interpolator_loader.go` 新增（接口内追加方法，与既有三方法并列）：

```go
// LoadSevenHolePRB 加载七孔 .prb 文件集（innerPath=7.prb，outerPaths 按孔号 1..6 顺序 6 份）。
// 文件缺失、行数/列数非法、网格点缺失或重复均通过 error 暴露。
LoadSevenHolePRB(innerPath string, outerPaths [6]string) (seveninterp.Interpolator, error)
```

实现落在 `internal/adapters/interpolation/`：`loader.go` 加 `Loader.LoadSevenHolePRB`（透传，typed-nil 防护同既有注释约定）；`files.go` 加 `LoadSevenHolePrbFiles(innerPath, outerPaths)`（`readNonEmptyLines` 读 7 份 → `NewSevenHolePrbInterpolator()` → `LoadInnerPrbLines` + 逐扇区 `LoadOuterPrbLines`）。

**加载校验**（在 sevenhole 包 `prb_loader.go` 内，对齐五孔 `validateAndIndexTable` 风格）：

- 行数：7.prb 恰 169 数据行；n.prb 恰 52 数据行；每行恰 6 列、全部有限数值。
- 网格覆盖：7.prb 覆盖 a,b ∈ {-30..30 步长 5} 全部 169 点；n.prb 覆盖 a ∈ {30,35,40,45} × 该扇区 13 条 b 网格线（§2.1 表）全部 52 点。
- 重复网格点、缺失网格点、越界网格点均报错并指明行号。

### 5.4 配置持久化与启动恢复

`traversalAPIConfig`（`traversal_config.go` L304）新增 `ProbeType string \`json:"probeType,omitempty"\``；`ParseConfig`（L405）写入 `config.ProbeType`，空串兜底 `"five-hole"`。`roleToLabel()`（L381）新增 §2.3 的 9 条七孔映射。

**持久化配置 JSON**（`/api/traversal/config` 保存的前端配置）采用双变体并存（§2.3 修订）：五孔字段、`probeType` 与七孔文件集可同存一份配置，另含未激活侧通道（后端忽略）：

```json
{
  "probeType": "seven-hole",
  "sevenHolePrb": {
    "innerFile":  { "filePath": "D:/cal/7.prb" },
    "outerFiles": [ { "filePath": "D:/cal/1.prb" }, …共 6 份… ]
  },
  "prbFile": { "filePath": "D:/cal/five.prb" },
  "inactiveProbeChannels": [ …未激活侧通道绑定… ]
}
```

`restoreInterpolatorFromConfig()` 分支优先级（仅恢复激活变体，§5.2.1 第 5 条）：

1. `probeType == "seven-hole"` → 读取规范化 `sevenHolePrb` 的 7 份路径调 `loader.LoadSevenHolePRB`（唯一七孔路径；七孔无 CSV/多 PRB 模式；七孔要求 1+6 文件齐全，否则记恢复错误）；
2. 否则保持既有五孔优先级不变：新算法 CSV > 多 PRB > 单 PRB；
3. 未激活变体字段仅作持久化数据透传，不进入 usecase；未知非空 `probeType` 记恢复错误且不启动 loader。

失败语义复用既有机制：`setInterpolatorRestoreErr` 记录、`CheckPreconditions` PRB 项暴露、`runLoaderWithTimeout` 5s 超时。

### 5.5 CSV 落盘

计算结果列（`traversal_csv_writer.go` 的 `TraversalCsvWriter.buildHeader`：`Alpha, Beta, Pt, Ps, Mach, SampleCount, DwellMs`）**零改动**——七孔经 §3.3 变换后的输出与五孔同形（§2.2）。

原始压力列：七孔为 P1..P7 + Patm + Tatm 共 9 列。`traversal_csv_writer.go` 的 `buildLabelEntries` 已知标签优先级表需由 `{"P1".."P5","Patm","Tatm"}` 扩为 `{"P1".."P7","Patm","Tatm"}`——否则 P6/P7 落入“其余按通道索引升序”分支，列序变为 `P1..P5, Patm, Tatm, P6, P7`。此为 1 处小改（含表头 hash 稳定性考量：`HeaderHash` 由 labels 顺序决定，两种探针各自稳定即可）。

### 5.6 API 契约（`api/server.go`）

**新增 action `importSevenHolePrb`**（不复用 `importPrb`：入参为 7 份路径、返回含逐文件信息，保持 `importPrb` 零回归）：

```
POST /api/traversal/importSevenHolePrb
Request:
{ "innerFilePath": "D:/cal/7.prb",
  "outerFilePaths": ["D:/cal/1.prb","D:/cal/2.prb","D:/cal/3.prb","D:/cal/4.prb","D:/cal/5.prb","D:/cal/6.prb"] }

Response 200:
{ "files": [
    { "filePath": "...", "fileName": "7.prb", "sector": 7, "pointCount": 169, "loadedAt": 1752700000000 },
    { "filePath": "...", "fileName": "1.prb", "sector": 1, "pointCount": 52,  "loadedAt": 1752700000000 }, … ],
  "validRange": { "alphaMin": -30, "alphaMax": 30, "betaMin": -30, "betaMax": 30, "machMin": 0, "machMax": 0 } }

Response 400: { "error": "加载七孔PRB文件集失败：3.prb 必须包含 52 行数据，实际 48 行" }
```

成功后调用 `TraversalManager.SetSevenHoleInterpolator(...)`（写配置由前端 `saveConfig` 一并持久化 §5.4 结构）。

> **形状约定**：导入 API 请求体收**路径数组**（`innerFilePath` + `outerFilePaths`，对齐 `importMultiPrb` 的 `filePaths []string` 风格）；持久化配置（§5.4）与响应层则组装为逐文件信息对象。三层形状各自稳定，由 handler 与前端 store 转换。

**保留 action `clearInterpolator`**（五孔/七孔通用，作为显式清除能力——如用户移除校准文件后主动失效；双变体语义下探针切换**不调用**本接口，§5.2.1）：

```
POST /api/traversal/clearInterpolator
Request:  { "probeType": "seven-hole" }   // probeType 必填，不做隐式猜测
Response 200: { "cleared": true }
```

清空对应类型的插值器字段（七孔清 `sevenHoleInterpolator`，五孔清 `interpolator`）。`importPrb`/`importCalibrationCsv`/`importMultiPrb` 等既有 action 不变。

**`calculateRealtime` 兼容升级**：请求体增加可选判别字段 `probeType`，并把 `pressures` 升级为含可选 P6/P7 的超集：

```json
{ "probeType": "seven-hole", "pressures": { "P1": 0, "P2": 0, "P3": 0, "P4": 0, "P5": 0, "P6": 0, "P7": 0, "Patm": 101325, "Tatm": 20 } }
```

既有五孔请求可省略 `probeType`，省略时按 `five-hole` 处理以保持兼容；七孔请求必须显式传 `seven-hole`。handler 校验请求类型与 Manager 当前配置一致，并按类型校验必填压力字段，不得通过字段是否为零或是否出现猜测类型。五孔走既有 `CalculateRealtime`（响应原样），七孔走 `CalculateRealtimeByProbe`（响应 §2.2 的 JSON 子集）。

API 解码 DTO 的 `P6`/`P7` 必须使用 `*float64`：`nil` 表示字段缺失，非 nil（包括值为 0）表示调用方显式提供。handler 校验完成后再解引用并装配内部 `probeCalcInput`。不得使用普通 `float64` 的零值判断字段缺失，也不增加前端 `hasP6/hasP7` 冗余字段。

**Wails 侧无改动**：遍历 API 在 Wails 模式经本地 HTTP server 暴露（`apps/desktop-wails/backend/app.go` L159-194，`127.0.0.1:8900`，`TraversalManager` 注入 `api.Deps` L178），不存在 traversal 的 Wails binding；前端 `traversalApi.ts` 纯 HTTP（`apiBase()` 在 Wails 下指向 8900）。新增 action 自动两端可用，无需 `wails3 generate bindings`。

---

## 6. 前端集成设计

### 6.1 类型扩展

| 文件 | 改动 |
|---|---|
| `shared/types/calibration.ts` | `ProbeChannelRole` 增加 `'sevenHole.p1'…'sevenHole.p7' \| 'sevenHole.pAtm' \| 'sevenHole.tAtm'` 9 个成员 |
| `shared/types/traversal.ts` | 新增 `TraversalProbeType`、§2.3 判别联合配置、五孔/七孔插值配置类型、七孔 9 通道预设；`TraversalRawPressure` 增加可选 `P6?/P7?`；旧配置仅在读取适配器中兼容，不扩散到组件内部 |

### 6.2 向导探针类型选择

`TraversalSettings.vue` 第 1 步（硬件配置，`currentStep === 0`）顶部、压力类型开关之侧，提供**探针类型选择**（UiSelect：五孔探针/七孔探针）。切换采用**双变体换入换出**（2026-07 修订，替代原"确认后重置"流程）：

1. **不重置、不弹确认**：向导为两种探针各维护一份配置包（五孔：通道 + PRB/多 PRB/校准 CSV/算法选择；七孔：通道 + 7 文件槽位），切换时暂存当前侧、载入目标侧（首次使用对应预设），两套配置互不丢失；
2. 通道绑定按探针类型各自保留（通道语义不同，不互相映射）；未激活侧通道随配置持久化到 `inactiveProbeChannels`；
3. store 侧仅 `activateProbeType` 切换激活 `probeType` 并复核后端加载状态，**不清理任何插值器**；后端按激活 `probeType` 经策略表取用对应插值器（§5.2.1）；
4. `interpolationAlgorithm`/`prbMode` 仅对五孔有意义，七孔时在 PRB 步骤隐藏（见 §6.3）。

`saveConfig()` 组装配置时**两侧变体都写入**：五孔字段、`sevenHolePrb`、`probeType`（激活方）与 `inactiveProbeChannels`；`loadSavedConfig()` 恢复时两侧分别装入激活 refs 与未激活暂存。第 4 步摘要区显示探针类型行。`TraversalHardwareStep.vue` 按 `probeChannels` 泛型渲染（现为 `v-for` 行），无需结构改动。

### 6.3 PRB 步骤组件边界

`TraversalPrbStep.vue` 保持公共步骤壳，只负责标题、恢复错误 Banner、步骤有效性和按 `probeType` 选择子组件：

- `FiveHolePrbConfig.vue`：承载现有算法选择、single/multi PRB、校准 CSV 控件；从现有 `TraversalPrbStep.vue` 原样抽取，不改变行为。
- `SevenHolePrbConfig.vue`：承载 1 个 7.prb 槽位 + 6 个 n.prb 槽位；7 份齐备后调用 store 导入动作，展示逐文件 pointCount/validRange；任一份缺失时无效。
- 两个子组件只通过类型化 `modelValue`/事件交换各自的判别配置，不读取或清理对方字段。

`TraversalSettings.vue` 为两种探针各维护一份配置包（五孔 refs 组 / 七孔 Draft + 通道），探针切换、保存和恢复通过各自的 bundle 与纯函数 `normalizeTraversalProbeConfig`（仅抽取激活变体）完成，避免大型组件内出现两套配置状态机。

### 6.4 store 与 API

| 文件 | 改动 |
|---|---|
| `api/traversalApi.ts` | 新增 `importSevenHolePrb(innerFilePath, outerFilePaths)` → POST `/api/traversal/importSevenHolePrb`；新增 `clearInterpolator(probeType)` → POST `/api/traversal/clearInterpolator`（显式清除能力，探针切换不调用） |
| `stores/traversalStore.ts` | 新增 `importSevenHolePrbFiles()`、`clearProbeInterpolator()`（显式清除用）与 `activateProbeType()`（双变体激活切换：仅改 `config.probeType` 并复核后端加载状态，不清理插值器）；`hasLoadedInterpolator` 从激活变体配置与后端校验结果派生 |

### 6.5 展示语义注册表

公共主画面和摘要不得硬编码 Alpha/Beta 的物理名称。前端定义无业务计算的展示元数据：

```ts
const TRAVERSAL_PROBE_PRESENTATION = {
  'five-hole':  { titleKey: 'fiveHoleTraversalTest',  alphaLabelKey: 'angleOfAttack', betaLabelKey: 'sideslipAngle' },
  'seven-hole': { titleKey: 'sevenHoleTraversalTest', alphaLabelKey: 'sideslipAngle', betaLabelKey: 'angleOfAttack' }
} as const
```

`TraversalMain.vue`、实时监视和摘要统一查该表。该表仅描述标签，不包含角度转换或插值算法。CSV 数值列名继续保持兼容，但任务元数据/配置必须保存 `probeType`，导入或审阅 CSV 时据此解释 Alpha/Beta。

上述 `titleKey`、`alphaLabelKey`、`betaLabelKey` 的中英文键定义见 §6.6。

### 6.6 i18n（`stores/i18nStore.ts`，zh + en 双份）

新增键（命名沿用 `ch_fiveHole*`/`fiveHoleTraversalTest` 风格）：`travProbeType`、`travProbeTypeFiveHole`、`travProbeTypeSevenHole`、`travProbeTypeHint`（双变体语义提示）、`sevenHolePrbTitle`、`sevenHolePrbInnerFile`、`sevenHolePrbOuterFile`（带 {n} 占位）、`sevenHolePrbIncomplete`、`travErrImportSevenHolePrb`、`travErrClearInterpolator`、`sevenHoleTraversalTest`（主画面标题按探针类型切换）、`angleOfAttack`、`sideslipAngle`（展示语义注册表用）。所有新增 UI 文案禁止硬编码。

---

## 7. 验证策略（算法正确性闸门）

### 7.1 Go↔Python 对拍

**基准**：`device-lab/skills/seven-hole-probe/seven_hole.py` 的 `cal_ab`（输入 JSON：`{p1..p7, t, pa}` + 含 7 份 .prb 的目录）。对拍范围仅限合法校准网格内的数值路径；本规范有意修改的“不外推、Pt<Ps 报错、有限数守卫”按 Go 产品契约独立测试，不要求复刻 Python 的 `beyond_border`/`fabs` 行为。

**夹具生成**（一次性，脚本 `device-lab/skills/seven-hole-probe/tools/gen_traversal_fixtures.py`）：

1. 数据集 `projects/WindLabX4/docs/W532.202608.P.7H.1-01/` 的 7 个 CSV 为 GBK 编码且表头存在历史命名错误（spec-seven-hole-calibration.md §12.1），**按列位置读取**并转码 UTF-8 副本（不改原始文件）。
2. 小角度区 CSV → `7.prb`：每行 `ka kb cpt cps a b` ← `Kα Kβ K0 Ks α β`；大角度 n 区 CSV → `n.prb` ← `Kθ[n] Kφ[n] K0[n] Ks[n] θ φ`（命名对应 §2.1）。
3. 对数据集 481 点逐点取原始压力（P1..P7、大气压力 pa、大气温度 t）运行 `cal_ab`，输出落为 golden JSON（含模式、扇区 n、a/b 或 θ/φ、pt/ps/v/ma）。
4. 夹具落盘：`shared/algorithms/go/sevenhole/interpolation/testdata/prb/`（7 份 .prb）+ `testdata/golden/`（golden JSON）。

**精度声明**：CSV 系数仅 4 位小数、压力 3 位小数（spec-seven-hole-calibration.md §7.4 数值精度），重建 .prb 的绝对精度受 CSV 舍入限制；但对拍双方使用**同一份 .prb 与同一输入**，输出差仅来自实现差异（Go 解析闭式 vs Python sympy 消元，数学等价），容差不受 CSV 舍入影响。

**容差**：角度 α/β 绝对误差 ≤ 1e-4°；Pt/Ps 对每个值按显式公式验收：`|got - want| <= max(|want| * 1e-6, 1e-4 Pa)`；Ma/V 相对误差 ≤ 1e-6。

> **实施注记（2026-07）**：
> 1. 数据集 CSV 系数仅 3 位小数，相邻网格点 ka/kb 存在精确相等值；Python 参考实现的四边形边斜率计算遇垂直边（Δka=0）即 `ZeroDivisionError` 崩溃，481 点中 195 点原本不可对拍。夹具生成脚本（`gen_traversal_fixtures.py`）对精确相等的退化边加确定性 ≤1e-7 抖动以恢复可比性（对拍双方读同一 `.prb`，等价性不受影响）。
> 2. 481 点中 46 点落在所有候选多边形外（Python 走 `beyond_border` 外推路径）。Go 按 §4 不外推：这些点在 golden 测试中断言 `IsValid=false` + 越界 Warning（属 Go 产品契约断言，非数值对拍）；其余 435 点（240 小角度 + 195 大角度）在容差内 100% 数值对拍通过。

### 7.2 边界用例

| 用例 | 构造 | 期望 |
|---|---|---|
| θ=30° 模式边界 | 数据集内区边缘点 + 外区 θ=30° 首行点 | 两侧结果连续（角度/压力差在对拍容差量级）、无 panic |
| 模式切换 | 构造 (ka,kb) 恰好落在内区多边形边界的输入 | `point_in_polygon` 返回 0（边界）按"内部"处理（与 Python `sign==0 or sign==1` 一致） |
| 第二候选扇区 | 第一最大孔扇区多边形 miss、第二最大孔命中的输入 | 走第二扇区出正确结果 |
| 越界 | (ka,kb) 远离全部多边形 | `IsValid=false` + 越界 Warning，无角度/压力输出（§4） |
| θ 退化分支 | 直接单测坐标变换函数，输入 θ=89.5°/90° | 走 `a=-θ, b=φ` 退化路径（标准网格不可达，函数级覆盖） |
| Pt<Ps | 构造 cpt/cps 使解出 Pt<Ps | 返回 error |
| 1e-12 守卫 | p7 == p_avg；p_center == p_side；cpt+cps 使 denom≈0 | 返回含上下文的 error |
| 加载校验 | 行数不足/列数错误/重复网格点/缺失网格点 | 加载 error 指明文件与行号 |

### 7.3 零回归与端到端

- 五孔算法包测试全绿：`cd shared\algorithms\go\fivehole; go test ./...`
- api-go 全量：`cd projects\WindLabX4\services\api-go; go test ./...`（含 usecase 既有 `TestBuildRawPressure_*`、`TestParseConfig`、遍历 v2 集成测试）
- 前端：`cd projects\WindLabX4\apps\desktop-wails\frontend; npm run typecheck; npm run build`
- 结构校验：工作区根 `scripts/validate-structure.ps1`
- e2e 手动：七孔配置 → 导入 7 文件 → 前置检查通过 → 跑点 → CSV 9 原始列 + 计算列正确 → 切回五孔配置行为不变。

---

## 8. 验收标准

### 8.1 算法正确性验收（硬闸门）

| 验收项 | 验收方式 | 通过标准 |
|---|---|---|
| Go↔Python 对拍 | 481 点 golden 对拍测试 | §7.1 容差全部满足，零例外 |
| 边界用例 | §7.2 表 8 类用例单测 | 全部符合期望 |
| 负号保护 | 删除 §3.3 负号后重跑坐标变换测试 | 测试必须失败（防误删） |
| 加载校验 | 构造非法 .prb（48 行/缺列/重复点） | 加载报错且指明文件行号 |

### 8.2 集成与零回归验收

| 验收项 | 通过标准 |
|---|---|
| 五孔零回归 | §7.3 全部测试绿；五孔遍历 e2e 手动冒烟通过 |
| probeType 贯通 | 七孔配置保存→重启→启动恢复成功加载 7 文件；`CheckPreconditions` PRB 项通过 |
| CSV 落盘 | 七孔 CSV 原始压力列序为 P1..P7,Patm,Tatm；计算列 Alpha/Beta/Pt/Ps/Mach 与五孔同位置 |
| API | `importSevenHolePrb` 200/400 契约符合 §5.6；`calculateRealtime` 五孔响应字段不变 |
| 越界行为 | §4 表全部场景符合决策（不外推） |

### 8.3 端到端验收

七孔向导（探针选择→9 通道→7 文件→布点→启动）→ 实时插值显示 α/β/Pt/Ps/Ma → CSV 落盘正确 → 状态恢复（切走切回）正常；前端 `npm run typecheck` / `npm run build` 全绿。

---

## 9. 关键约束与注意事项

1. **算法权威**：实现细节一律以 SKILL.md + seven_hole.py 为准；本规范不重述公式细节，审查时对拍测试是唯一等价性证据。
2. **架构硬约束**（AGENTS.md）：`core/` 零 I/O——七孔包只接受文本行，文件读取在 `adapters/interpolation/files.go`；`ports/` 仅接口；前端零插值算法（7 文件选择器只传路径，不解析内容）。
3. **角度语义陷阱**：七孔输出 Alpha=侧滑角、Beta=迎角，与五孔遍历"Alpha=攻角、Beta=侧滑角"**定义相反**。字段名保持一致是为复用 CSV/视图管线；UI 文案与文档必须按探针类型标注（沿用 spec-seven-hole-calibration.md §12.2 的警示）。
4. **七孔无多 PRB/CSV 模式**：`interpolationAlgorithm`、`prbMode`、`multiPrb`、`calibrationCsvFile` 均为五孔专属配置，七孔配置中不得携带（`saveConfig` 组装时按 probeType 裁剪）。
5. **负号保留**：坐标变换 `α = -atan(tanθ·sinφ)` 的负号是 φ 方向约定的数学结果，禁止删除（对拍 + 单测双重防护）。
6. **Pt<Ps 返回 error 而非静默**：与五孔 IsValid=false+warning 的有意差异，依据 SKILL.md §5 对 Python `math.fabs` 缺陷的明确修正指示。

---

## 10. 待评审确认事项

| 编号 | 待确认事项 | 现状与选项 |
|---|---|---|
| Q1 | 七孔一期是否接入 `InterpolationCache` | 建议**不接入**（§5.2 第 5 条，避免泛化 `core/realtime`）；如需接入另立议题 |
| Q2 | 校准 CSV → 七孔插值数据源 | **已落地（2026-07 遍历侧）**：新增 `importSevenHoleCalibrationCsv` API 与 `LoadSevenHoleCalibrationCSV` 加载链（adapters 层 GBK/列位置解析 + 退化边抖动 → .prb 行集 → 既有 loader 强校验），配置 `kind="seven-hole-calibration-csv"` 与恢复分支贯通；向导七孔 PRB 步提供「PRB 文件集 / 校准 CSV」数据源切换与批量导入。校准模块自身的 CSV 导出对接沿用同一格式契约（列位置：内区 a=col0,b=col1、外区 θ=col1,φ=col0、系数 col12..15） |
| Q3 | 外区越界外推（SKILL.md §3.8） | 本期**不实现**（§4）；如现场确有 θ>45° 需求，另立 spec 增补并加 UI 标注 |
| Q4 | 对拍 golden 是否纳入版本库 | **已决策并采纳**：纳入 `testdata/`（481 点 JSON + 7 份 .prb，体积约数百 KB），保证 CI 可复现；见 plan Architecture Decisions |

---

## 附录 A：与五孔遍历插值的差异对照表

| 维度 | 五孔遍历 | 七孔遍历（本模块） |
|---|---|---|
| 插值包 | `shared/algorithms/go/fivehole/interpolation` | `shared/algorithms/go/sevenhole/interpolation`（新模块） |
| 输入 | P1..P5 + PAtm + TAtm（7 通道） | P1..P7 + PAtm + TAtm（9 通道） |
| 校准数据 | 单 .prb（169 点）/ 多 PRB / 校准 CSV | 7 份 .prb 文件集（169 + 6×52） |
| 模式 | 单区（region9） | 小角度/大角度双模式（多边形判定切换） |
| 角度语义 | Alpha=攻角, Beta=侧滑角 | Alpha=侧滑角, Beta=迎角（外区经 θ/φ→α/β 变换） |
| 越界 | IsValid=false + warning（不外推） | 同款对齐（§4） |
| Pt<Ps | IsValid=false + warning | 返回 error（§4 有意差异） |
| 实时缓存 | `realtime.InterpolationCache` | 一期不经缓存（§5.2） |
| Manager 插值器字段 | `interpolator` | `sevenHoleInterpolator`；通过显式 `ClearProbeInterpolator` 管理生命周期 |
| 角度有效范围后验 | 有（超 PRB 表 → invalid） | 无（多边形判定前置，§2.2） |

## 附录 B：关键代码证据索引

| 主张 | 证据位置 |
|---|---|
| 五孔不外推 | `shared/algorithms/go/fivehole/interpolation/prb_interpolator.go` 的 `PrbInterpolator.Calculate` 及网格定位函数 |
| 五孔接口形状 | `shared/algorithms/go/fivehole/interpolation/types.go` 的 `Interpolator`、`InterpolationInput`、`InterpolationResult` |
| 三孔 per-probe 先例 | `shared/algorithms/go/threehole/interpolation/types.go` |
| 模块注册方式 | `go.work` 的 `use` 列表；`projects/WindLabX4/services/api-go/go.mod` 的 require/replace |
| Manager 五孔绑定 | `internal/usecase/traversal.go` 的 `TraversalManager`、`CalculateRealtime`、`HasLoadedInterpolator`、`interpolatorIdentity` |
| 五孔硬编码点 | `internal/usecase/traversal_view.go` 的 legacy 标签/输入装配；`traversal_config.go` 的 `roleToLabel` |
| 恢复优先级 | `internal/usecase/traversal_config.go` 的 `restoreInterpolatorFromConfig` |
| CSV 标签优先级 | `internal/adapters/storage/traversal_csv_writer.go` 的 `buildLabelEntries` |
| Wails 无 traversal binding | `apps/desktop-wails/backend/app.go` 的本地 HTTP server 与 `api.Deps` 装配 |
| 通道预设/配置类型 | `frontend/src/shared/types/traversal.ts` 的遍历配置/预设；`calibration.ts` 的 `ProbeChannelRole` |
| 向导步骤结构 | `components/traversal/TraversalSettings.vue` 的 `steps`/步骤模板；`TraversalPrbStep.vue` |
| 数据集规模 | `docs/W532.202608.P.7H.1-01/`（481 点，GBK CSV）；spec-seven-hole-calibration.md §6.2 |
