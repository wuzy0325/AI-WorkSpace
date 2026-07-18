# Tasks: 七孔探针校准模块

> 关联：[spec-seven-hole-calibration.md](./spec-seven-hole-calibration.md)（v1.2） | [plan-seven-hole-calibration.md](./plan-seven-hole-calibration.md)
> 状态：Phase 3 任务清单，待人工批准后进入 BUILD
> 日期：2026-07-16
> 实现范围：**MVP 先行**（内区 3 类图表 + 核心流程，外区 3 类图表第二阶段）
> 采样参数：5 次子采样 + 2000ms 驻留（推荐配置）

## 总体约定

- **TDD 强制**：每个任务先写失败测试再实现，测试覆盖率达 80%+
- **零回归硬门槛**：每个 Task 完成后必须跑 `go test ./internal/core/calibration/...` 确保五孔/三孔/总压/总温既有测试全绿
- **架构硬约束**：前端零校准算法（spec §9.3），点位生成集中在后端 `GenerateSevenHolePoints`
- **中文注释强制**：生产代码必须加清晰中文注释，说明"为什么这么做"而非"做了什么"
- **任务粒度**：每个 Task 触及文件 ≤ 5 个；超出需拆分
- **Config 扩展**：`Config` 结构体新增 `RealtimeCallback func(event RealtimeEvent)` 字段（`json:"-"`，仅运行时注入，不序列化）。七孔算法在多次采样间通过此回调推送实时 P1~P7 值，替代原有的 `AcquireDataWithChannels` 内部 `onRealtime` 参数——因为 `Algorithm.AcquireDataWithConfig` 接口仅暴露 `onSampleProgress`，无 `onRealtime` 参数

---

## Phase 1: Core 层算法基础（独立于 UI）

### Task 1: CalPoint 扩展 + TypeSevenHole 常量 + 七孔类型定义 ✅ 已完成

> **状态**：已实现（2026-07-17）。`TypeSevenHole` 常量、`CalPoint` 三字段（`MotionCoordinates`/`Region`/`Sector`）、`SevenHoleRawData`/`SevenHoleCoefficients`/`SevenHoleDataPoint` 三类型、`RealtimeEvent` 七孔扩展、`DataPoint` 接口适配均已就位。`go test` 全绿（含五孔/三孔/总压/总温零回归）。

**Description:** 在 `types.go` 新增 `TypeSevenHole` 校准类型常量；扩展 `CalPoint` 结构体新增 `MotionCoordinates`、`Region`、`Sector` 三个字段（向后兼容，五孔等已有模块不填新字段时走默认 `Coordinates` 路径）；新增 `SevenHoleRawData`、`SevenHoleCoefficients`、`SevenHoleDataPoint` 三个七孔专属类型；扩展 `RealtimeEvent` 结构体追加 `SevenHoleRaw *SevenHoleRawData` 和 `SevenHoleCoefficients *SevenHoleCoefficients` 字段（omitempty）。

**Acceptance criteria:**
- [ ] `TypeSevenHole CalibrationType = "seven-hole"` 常量定义
- [ ] `CalPoint` 新增三个字段：`MotionCoordinates map[string]float64`（json `motionCoordinates,omitempty`）、`Region string`（json `region,omitempty`）、`Sector int`（json `sector,omitempty`）
- [ ] `SevenHoleRawData` 结构含 P1~P7、PAtm、TAtm、PTotal、PStatic、TTunnel 字段（指针类型，omitempty）
- [ ] `SevenHoleCoefficients` 结构含 Kalpha、Kbeta、K0、Ks（内区）+ Ktheta、Kphi、K0Outer、KsOuter（外区，带扇区编号 n）+ MachNumber、Velocity 字段
- [ ] `SevenHoleDataPoint` 实现 `DataPoint` 接口（`GetPointID() int`、`GetCoordinates() map[string]float64`）
- [ ] `RealtimeEvent` 追加七孔字段，五孔等已有模块不填时序列化无差异
- [ ] 既有五孔/三孔/总压/总温测试全绿（零回归）
- [ ] 所有导出类型有中文注释说明物理意义

**Verification:**
- [ ] 测试通过：`cd projects\wind-daq\services\api-go; go test ./internal/core/calibration/...`
- [ ] Vet 通过：`go vet ./internal/core/calibration/...`
- [ ] Build 成功：`go build ./...`
- [ ] 零回归验证：`go test -count=1 ./internal/core/calibration/... -run 'TestFiveHole|TestThreeHole|TestTotalPressure|TestTotalTemperature'`

**Dependencies:** None

**Files likely touched:**
- `projects/wind-daq/services/api-go/internal/core/calibration/types.go`
- `projects/wind-daq/services/api-go/internal/core/calibration/types_test.go`（新增七孔类型序列化测试）

**Estimated scope:** S（2 文件）

---

### Task 2: 七孔公式实现（内区 4 系数 + 外区 4 系数 + 马赫数）

**Description:** 新建 `seven_hole_formulas.go`，实现 spec §4.1 内区公式（1-8）和 §4.2 外区公式（9-12，含环形取模），以及 §4.4 马赫数与速度公式（复用 `AtmosphericDataCalculator`）。所有压力输入使用表压（基准 A），仅在马赫数计算入口转绝压（基准 C）。α 公式负号必须保留（spec §3.3）。

**Acceptance criteria:**
- [ ] `CalculateSevenHoleInnerCoefficients(raw SevenHoleRawData) (SevenHoleCoefficients, error)` 实现公式 1-8
  - P̄ = (P1+P2+P3+P4+P5+P6)/6
  - Cpa = (P4-P1)/(P7-P̄)，Cpb = (P5-P2)/(P7-P̄)，Cpc = (P6-P3)/(P7-P̄)
  - Kβ = -(2·Cpa + Cpb - Cpc)/3
  - Kα = (Cpb + Cpc)/√3
  - K0 = (P7-p_t)/(p_t-p_s)
  - Ks = (p_s-P̄)/(p_t-p_s)
- [ ] `CalculateSevenHoleOuterCoefficients(raw SevenHoleRawData, n int) (SevenHoleCoefficients, error)` 实现公式 9-12
  - 环形取模：n=1 时 n-1=6，n=6 时 n+1=1
  - Kθ[n] = (Pn-P7)/(Pn-(Pn+1+Pn-1)/2)
  - Kφ[n] = (Pn-1-Pn+1)/(Pn-(Pn+1+Pn-1)/2)
  - K0[n] = (Pn-p_t)/(p_t-p_s)
  - Ks[n] = (p_s-(Pn+1+Pn-1)/2)/(p_t-p_s)
- [ ] `CalculateSevenHoleMachNumber(pTunnel, pStatic, atmPressure float64) (float64, error)` 实现公式 §4.4
  - 仅在此处将 p_t、p_s 转绝压：p_t_abs = p_t + 大气压力，p_s_abs = p_s + 大气压力
  - Ma = √((2/(γ-1)) × ((p_t_abs/p_s_abs)^((γ-1)/γ) - 1))，γ=1.4
- [ ] 数据集中心点验证：Kα=0.043, Kβ=-0.025, K0=0.00056, Ks=-0.110, Ma=0.242（误差 ≤ 0.001）
- [ ] 数据集外区 1 区首点验证：Kθ[1]=0.494, Kφ[1]=1.741, K0[1]=-0.207, Ks[1]=-0.260（误差 ≤ 0.001）
- [ ] 公式代码注释明确标注"A→B 不转换、A→C 仅 Ma 入口转换"
- [ ] Kφ 边界符号反转不取绝对值（spec §4.3 重要约束）

**Verification:**
- [ ] 测试通过：`go test -v -run 'TestCalculateSevenHole' ./internal/core/calibration/...`
- [ ] 数据集中心点单测：`go test -v -run 'TestSevenHoleInnerFormulaCenterPoint'`
- [ ] 数据集外区单测：`go test -v -run 'TestSevenHoleOuterFormulaSector1'`
- [ ] 马赫数单测：`go test -v -run 'TestSevenHoleMachNumber'`
- [ ] Vet 通过：`go vet ./internal/core/calibration/...`

**Dependencies:** Task 1

**Files likely touched:**
- `projects/wind-daq/services/api-go/internal/core/calibration/seven_hole_formulas.go`（新增）
- `projects/wind-daq/services/api-go/internal/core/calibration/seven_hole_formulas_test.go`（新增）

**Estimated scope:** M（2 文件）

---

### Task 3: 流场分区判定 + tie-break 规则

**Description:** 在 `seven_hole.go` 实现 `DetermineRegion` 纯函数（无状态），按 spec §3.2 四条 tie-break 规则（P7 优先 → 编号小优先 → 滞回仅相邻扇区 → 首点无前序跳过滞回）确定性判定来流分区。返回 `(region string, n int, boundaryFlag string)`，boundaryFlag 非空时记录并列孔对（如 "P7-P1"、"P1-P2"）。

**滞回状态所有权**：`DetermineRegion` 本身是无状态纯函数——`prevRegion`/`prevSector` 由 `AutomaticCalibration` 持有（两个字段），在每个点采集前注入 `Config.PrevRegion`/`Config.PrevSector`。首点时 `Config.PrevRegion=""`、`Config.PrevSector=0`，跳过滞回。采集完成后 `AutomaticCalibration` 用返回的 `region`/`sector` 更新自身状态，供下一点使用。`SevenHoleAlgorithm` 保持空结构体（无状态），通过 `Config` 接收上下文。

**Acceptance criteria:**
- [ ] `DetermineRegion(p1, p2, p3, p4, p5, p6, p7 float64, prevRegion string, prevSector int) (region string, n int, boundaryFlag string)` 函数签名
- [ ] 规则 1：|P7-Pmax| < TIE_BREAK_TOLERANCE（默认 5 Pa，可配置）时返回 ("inner", 7, "P7-Pn")
- [ ] 规则 2：外围孔并列最大时选编号最小（candidates = [|Pi-outerMax| < tol] 的 i 列表，n=min(candidates)）
- [ ] 规则 3：滞回——prevRegion=="outer" 且 prevSector 在 candidates 中且与 candidates 中某元素相邻（环形取模）时保持 prevSector
- [ ] 规则 4：首点（prevRegion=""）跳过滞回，仅按规则 1、2 判定
- [ ] 跨大跨度不滞回：prevSector=1，candidates={3,4} 不触发滞回（3 与 1 不相邻）
- [ ] `boundaryFlag` 仅在 len(candidates)>1 或触发规则 1 时填充，否则空串
- [ ] `TIE_BREAK_TOLERANCE` 通过配置项传入，默认 5.0 Pa，禁止低于 1 Pa 或高于 50 Pa
- [ ] 相同输入永远产生相同输出（确定性可重放）

**Verification:**
- [ ] 测试通过：`go test -v -run 'TestDetermineRegion' ./internal/core/calibration/...`
- [ ] 5 个构造用例测试覆盖：P7=Pmax、P1=P2=Pmax、P1=P2=P3=Pmax、滞回触发、跨大跨度不滞回
- [ ] 数据集 481 点分区判定回归测试：`go test -v -run 'TestDetermineRegionDatasetRegression'`
- [ ] Vet 通过：`go vet ./internal/core/calibration/...`

**Dependencies:** Task 1

**Files likely touched:**
- `projects/wind-daq/services/api-go/internal/core/calibration/seven_hole.go`（新增）
- `projects/wind-daq/services/api-go/internal/core/calibration/seven_hole_test.go`（新增分区判定测试）

**Estimated scope:** M（2 文件）

---

### Task 4: 不确定度计算（u1~u4 + u(A) + 灵敏系数 + 合成 + 扩展）

**Description:** 新建 `seven_hole_uncertainty.go`，实现 spec §5 不确定度评定算法。包含 B 类分量（u1~u4）、A 类分量（u(A,i) = S(P)/√N）、灵敏系数（cᵢ）、合成不确定度（`u_c = √(|Σ cᵢu_B,i|² + Σ cᵢ²u(A,i)²)`，B 类完全正相关取和的绝对值，A 类独立取平方和开方）、扩展不确定度（U = k·u_c，k=2）。

**Acceptance criteria:**
- [ ] `UncertaintyCalculator` 结构体封装 B 类分量配置（u1=20.2, u2=p_t×0.1%/√3, u3=3, u4=5.8 Pa）
- [ ] `CalculateTypeA(samples []float64) (uA float64, stdDev float64)` 实现 A 类：uA = S/√N，S 为样本标准差
- [ ] `SensitivityCoefficientsK0(p7, pTunnel, pStatic float64) (cP7, cPt, cPs float64)` 实现内区 K0 灵敏系数（spec §5.3）
- [ ] `SensitivityCoefficientsKs(p1..p6, pTunnel, pStatic float64)` 实现内区 Ks 灵敏系数
- [ ] `SensitivityCoefficientsKAlpha/Beta(...)` 实现内区 Kα、Kβ 灵敏系数（链式求导）
- [ ] `CombinedUncertainty(cValues, uBValues, uAValues []float64) float64` 实现合成公式
  - B 类：`|Σ cᵢ·u_B,i|`（保留符号求和后取绝对值，**不是绝对值的和**）
  - A 类：`√(Σ cᵢ²·u(A,i)²)`
  - 合成：`√(|Σ cᵢ·u_B,i|² + Σ cᵢ²·u(A,i)²)`
- [ ] `ExpandedUncertainty(uC float64, k float64) float64` 实现 U = k·u_c，默认 k=2
- [ ] 中心点 K0 不确定度算例复算：U(K0) = 8.13e-3（容差 [0.004, 0.0082]，spec §5.5 第 7 步）。自动验收四舍五入到 3 位小数后比较（0.00813 → 0.008，落在 [0.004, 0.0082] 内）。上限 0.0082 而非 0.008 是为避免未舍入原始值 0.00813 被 0.008 上限排斥
- [ ] 注释明确区分"|Σ cᵢuᵢ|"（正确）与"Σ|cᵢ|uᵢ"（错误，高估 46%）

**Verification:**
- [ ] 测试通过：`go test -v -run 'TestUncertainty' ./internal/core/calibration/...`
- [ ] 中心点 7 步算例复算测试：`go test -v -run 'TestUncertaintyCenterPointK0'`，数值精确匹配 spec §5.5
- [ ] 灵敏系数单测：`go test -v -run 'TestSensitivityCoefficients'`
- [ ] 合成公式正确性单测：`go test -v -run 'TestCombinedUncertaintyFormula'`（验证 |Σ cᵢuᵢ| 与 Σ|cᵢ|uᵢ 差异）
- [ ] Vet 通过：`go vet ./internal/core/calibration/...`

**Dependencies:** Task 2

**Files likely touched:**
- `projects/wind-daq/services/api-go/internal/core/calibration/seven_hole_uncertainty.go`（新增）
- `projects/wind-daq/services/api-go/internal/core/calibration/seven_hole_uncertainty_test.go`（新增）

**Estimated scope:** M（2 文件）

---

### Checkpoint: Phase 1 — Core 层算法基础

- [ ] `cd projects\wind-daq\services\api-go; go test ./internal/core/calibration/...` 全绿
- [ ] 数据集中心点 5 项数值精确匹配（Kα/Kβ/K0/Ks/Ma）
- [ ] 数据集外区 1 区首点 4 项数值精确匹配（Kθ[1]/Kφ[1]/K0[1]/Ks[1]）
- [ ] 5 个 tie-break 构造用例全部符合 spec §3.2 规则预期
- [ ] 中心点 K0 不确定度算例复算 U(K0) ∈ [0.004, 0.0082]
- [ ] 五孔/三孔/总压/总温既有测试零回归
- [ ] 人工 review：α 公式负号保留、压力基准三分离注释完整

---

## Phase 2: Core 层算法主体

### Task 5: SevenHoleAlgorithm + AcquireDataWithChannels + ValidateConfig

**Description:** 在 `seven_hole.go` 实现 `SevenHoleAlgorithm` 结构体（空结构体，无状态——滞回状态由 `AutomaticCalibration` 持有，通过 `Config.PrevRegion`/`Config.PrevSector` 注入），实现 `Algorithm` 接口的四个方法（`Type()`、`AcquireData()`、`AcquireDataWithConfig()`、`ValidateConfig()`）；实现 `AcquireDataWithChannels` 方法（参考五孔 `AcquireDataWithChannels` 范式），含多次采样、100ms 节流实时推送（通过 `config.RealtimeCallback`）、采样进度回调、调用 `DetermineRegion` 判定分区、最终均值与系数计算。

**Acceptance criteria:**
- [ ] `SevenHoleAlgorithm` 结构体定义，`NewSevenHoleAlgorithm()` 构造函数
- [ ] `Type() CalibrationType` 返回 `TypeSevenHole`
- [ ] `ValidateConfig(config Config) error` 校验 11 角色（`sevenHole.p1`~`p7`、`sevenHole.pTotal`、`sevenHole.pTunnelStatic`、`sevenHole.pAtm`、`sevenHole.tAtm`）齐全且 `SamplesPerPoint > 0`
- [ ] `AcquireDataWithConfig(point, channelReader, config, checkAbort, onSampleProgress)` 实现 Algorithm 接口
- [ ] `AcquireDataWithChannels(point, channelReader, probeChannels, samplesPerPoint, checkAbort, timestampReader, onSampleProgress)` 内部方法（`onRealtime` 已移除——改用 `config.RealtimeCallback` 注入）
  - 循环采样 `samplesPerPoint` 次，每次读 11 通道
  - 100ms 节流推送：通过 `config.RealtimeCallback` 回调（非 `onRealtime` 参数——因 `AcquireDataWithConfig` 接口签名不含此参数，`RealtimeCallback` 通过 `Config` 注入）
  - 采样进度通过 `onSampleProgress(i+1, samplesPerPoint)` 回调
  - 调用 `DetermineRegion` 判定当前点分区（prevRegion/prevSector 从 `AutomaticCalibration` 注入到 `Config` 的 `PrevRegion`/`PrevSector` 字段，首点传空串/0）
  - 按分区调用 `CalculateSevenHoleInnerCoefficients` 或 `CalculateSevenHoleOuterCoefficients`
  - 最终返回 `*SevenHoleDataPoint`，含 `Region`/`Sector`/`BoundaryFlag`/`StdDev`/`Uncertainty` 字段
- [ ] `AcquireData(point, channelReader, samplesPerPoint)` 转发到 `AcquireDataWithConfig`（兼容旧接口）
- [ ] 11 通道原始数据通过 `readRawDeviceChannelsFromProbe` 读取用于 CSV 落盘（七孔仅需 11 通道，非五孔的 16 通道）
- [ ] timestampReader 非 nil 时等待设备推送新数据帧后才计入有效采样（避免读缓存旧数据）

**Verification:**
- [ ] 测试通过：`go test -v -run 'TestSevenHoleAlgorithm' ./internal/core/calibration/...`
- [ ] ValidateConfig 测试：`go test -v -run 'TestSevenHoleValidateConfig'`（缺角色返回错误、11 角色齐全通过）
- [ ] AcquireDataWithChannels 测试：`go test -v -run 'TestSevenHoleAcquireData'`（mock channelReader，验证多次采样与系数计算）
- [ ] Vet 通过：`go vet ./internal/core/calibration/...`

**Dependencies:** Task 1, Task 2, Task 3, Task 4

**Files likely touched:**
- `projects/wind-daq/services/api-go/internal/core/calibration/seven_hole.go`
- `projects/wind-daq/services/api-go/internal/core/calibration/seven_hole_test.go`

**Estimated scope:** M（2 文件）

---

### Task 6: GenerateSevenHolePoints 双坐标点位生成

**Description:** 在 `seven_hole.go` 实现 `GenerateSevenHolePoints(config SevenHoleConfig) ([]CalPoint, error)`，按 spec §9.1 接口契约生成内+外区点位，每个点同时填充 `Coordinates`（逻辑坐标）和 `MotionCoordinates`（运动坐标），外区点按 §3.3 正向公式（α=-arctan(tanθ×sinφ)）换算为 α-β。支持两种校准模式：完整模式 673 点（内区 169 + 外区 504）、数据集模式 481 点（内区 169 + 外区 312）。

**Acceptance criteria:**
- [ ] `SevenHoleConfig` 结构含 `Mode`（"full"/"dataset"）、`InnerAlphaMin/Max/Step`、`InnerBetaMin/Max/Step`、`OuterThetaMin/Max/Step`、`OuterPhiMin/Max/Step`、`Serpentine` 字段
- [ ] `GenerateSevenHolePoints(config)` 返回 `[]CalPoint`，每个点含 `ID`、`Coordinates`、`MotionCoordinates`、`Region`、`Sector` 字段
- [ ] 内区点：`Coordinates={α,β}`，`MotionCoordinates={α,β}`（相同），`Region="inner"`，`Sector=7`
- [ ] 外区点：`Coordinates={θ,φ}`，`MotionCoordinates={α',β'}`（按 §3.3 正向公式换算），`Region="outer"`，`Sector=n`（1~6）
- [ ] 蛇形顺序：外层 β/φ 循环，奇数行 α/θ 反向（`reverse := Serpentine && bi%2 == 1`）
- [ ] 完整模式：内区 α∈[-30°,30°] 步长 5° × β∈[-30°,30°] 步长 5° = 13×13 = 169 点；外区 θ∈[30°,60°] 步长 5° × φ∈[0°,355°] 步长 5° = 7×72 = 504 点；合计 673 点
- [ ] 数据集模式：内区 169 点同上；外区 θ 固定取 {30°,35°,40°,45°}（4 个值，不可配置），φ 按扇区跨 60° 步长 5° = 13 点/扇区，4×13×6 = 312 点（扇区边界不共享，无需去重）；合计 481 点。数据集模式忽略自定义范围，硬编码在测试代码中（spec §6.2 表）
- [ ] 浮点 round 到 1 位小数：`math.Round((Min+float64(i)*Step)*10) / 10`
- [ ] α 公式负号必须保留：`α = -math.Atan(math.Tan(thetaRad) * math.Sin(phiRad))`
- [ ] 步长校验：步长 ≤ 0 返回错误；范围 min > max 返回错误

**Verification:**
- [ ] 测试通过：`go test -v -run 'TestGenerateSevenHolePoints' ./internal/core/calibration/...`
- [ ] 完整模式点数测试：`go test -v -run 'TestGenerateSevenHolePointsFullMode'`（验证 673 = 169 + 504）
- [ ] 数据集模式点数测试：`go test -v -run 'TestGenerateSevenHolePointsDatasetMode'`（验证 481 = 169 + 312）
- [ ] 双坐标一致性测试：`go test -v -run 'TestSevenHoleDualCoordinates'`（外区点 MotionCoordinates 与 Coordinates 通过 §3.3 公式换算一致）
- [ ] 黄金用例 G1~G5 测试：`go test -v -run 'TestSevenHoleGoldenCases'`（spec §3.3 表）
- [ ] Vet 通过：`go vet ./internal/core/calibration/...`

**Dependencies:** Task 1, Task 5

**Files likely touched:**
- `projects/wind-daq/services/api-go/internal/core/calibration/seven_hole.go`
- `projects/wind-daq/services/api-go/internal/core/calibration/seven_hole_points_test.go`（新增点位生成测试）

**Estimated scope:** M（2 文件）

---

### Checkpoint: Phase 2 — Core 层算法主体

- [ ] `cd projects\wind-daq\services\api-go; go test ./internal/core/calibration/...` 全绿
- [ ] `GenerateSevenHolePoints` 完整模式返回 673 点，数据集模式返回 481 点
- [ ] 5 个黄金用例 G1~G5 全部通过（spec §3.3 表）
- [ ] `SevenHoleAlgorithm` 实现 `Algorithm` 接口四方法
- [ ] `ValidateConfig` 11 角色校验完整
- [ ] 人工 review：α 公式负号保留、双坐标模型字段填充正确

---

## Phase 3: 后端集成

### Task 7: CSV schema 内/外区两套表头（26 列）

**Description:** 扩展 `csv_schema.go` 的 `BuildHeader()` switch，新增 `TypeSevenHole` 分支，按 spec §7.5 完整数据表字段返回内区/外区各 26 列表头（非 §7.2/7.3 的 18 列基础版本——§7.2/7.3 仅列通道+系数，实际 CSV 需包含采样元数据、不确定度、边界标记等全部字段）。扩展 `BuildRecord` 新增 `*SevenHoleDataPoint` 分支。由于七孔要求内/外区分文件落盘，`CsvSchema` 需支持按分区构建（引入 `Region` 字段或为内/外区分别构造 `Config` 副本）。

**内区 26 列表头**：`点位编号,侧滑角α,迎角β,来流总压P0,来流静压Ps,P1,P2,P3,P4,P5,P6,P7,大气压力,大气温度,采样次数,Kα,Kβ,K0,Ks,马赫数,速度,标准差,U(Kα),U(Kβ),U(K0),U(Ks),边界标记`

**外区 26 列表头**：`点位编号,滚转角φ,俯仰角θ,来流总压P0,来流静压Ps,P1,P2,P3,P4,P5,P6,P7,大气压力,大气温度,采样次数,Kθ[n],Kφ[n],K0[n],Ks[n],马赫数,速度,标准差,U(Kθ[n]),U(Kφ[n]),U(K0[n]),U(Ks[n]),边界标记`（扇区编号 n 替换为具体值，如 `Kθ[1]`）

**Acceptance criteria:**
- [ ] `BuildHeader()` 新增 `case TypeSevenHole:` 分支
- [ ] 内区表头 26 列：`点位编号,侧滑角α,迎角β,来流总压P0,来流静压Ps,P1,P2,P3,P4,P5,P6,P7,大气压力,大气温度,采样次数,Kα,Kβ,K0,Ks,马赫数,速度,标准差,U(Kα),U(Kβ),U(K0),U(Ks),边界标记`
- [ ] 外区表头 26 列：`点位编号,滚转角φ,俯仰角θ,来流总压P0,来流静压Ps,P1,P2,P3,P4,P5,P6,P7,大气压力,大气温度,采样次数,Kθ[n],Kφ[n],K0[n],Ks[n],马赫数,速度,标准差,U(Kθ[n]),U(Kφ[n]),U(K0[n]),U(Ks[n]),边界标记`（n 替换为具体扇区编号）
- [ ] `BuildRecord(dataPoint)` 新增 `case *SevenHoleDataPoint:` 分支，按 `dataPoint.Region` 选择内/外区列布局
- [ ] `boundary_flag` 列：非边界点写空字符串，边界点写 "P7-Pn" 或 "Pn-Pm"
- [ ] 七孔不确定度列：nil 指针值写空字符串（参考 project_memory §22 总压探针规范）
- [ ] CsvSchema 支持按分区构建：`NewSevenHoleCsvSchema(config, region string)` 或 `config.Type` 拼接 `"-inner"`/`"-outer"` 子类型
- [ ] 表头后缀规范化：压力类型 `_Pa`、温度类型 `_degC`（参考 project_memory §6）

**Verification:**
- [ ] 测试通过：`go test -v -run 'TestSevenHoleCsvSchema' ./internal/core/calibration/...`
- [ ] 内区表头列数测试：`go test -v -run 'TestSevenHoleInnerHeader'`
- [ ] 外区表头列数测试：`go test -v -run 'TestSevenHoleOuterHeader'`
- [ ] 表头中文 GBK 兼容性测试：`go test -v -run 'TestSevenHoleHeaderGBKCompatible'`
- [ ] Vet 通过：`go vet ./internal/core/calibration/...`

**Dependencies:** Task 1, Task 5

**Files likely touched:**
- `projects/wind-daq/services/api-go/internal/core/calibration/csv_schema.go`
- `projects/wind-daq/services/api-go/internal/core/calibration/csv_schema_test.go`

**Estimated scope:** S（2 文件）

---

### Task 8: CalibrationConfigDTO 七孔角色支持（无需改解码器）

**Description:** 验证 `CalibrationConfigDTO` 通过 `ProbeChannels []ProbeChannelDTO` 字段已支持七孔 11 角色（`sevenHole.p1`~`p7`、`sevenHole.pTotal`、`sevenHole.pTunnelStatic`、`sevenHole.pAtm`、`sevenHole.tAtm`）。由于 `DecodeCalibrationConfig` 是类型无关的通用解码器，七孔无需改 DTO 解码器，只需在 `SevenHoleAlgorithm.ValidateConfig` 中校验角色齐全（Task 5 已覆盖）。本任务主要做集成验证。

**Acceptance criteria:**
- [ ] `CalibrationConfigDTO` 通过 `ProbeChannels` 字段已支持七孔 11 角色（无需扩展 DTO 结构）
- [ ] `ToCore()` 转换 `ProbeChannelDTO` 到 `calibration.ProbeChannel` 时角色字符串原样保留
- [ ] `Points []calibration.CalPoint` 字段因 Task 1 扩展 `MotionCoordinates` 自动支持七孔双坐标点位
- [ ] 集成测试：构造含 11 角色的七孔 DTO JSON，解码后 `ValidateConfig` 通过
- [ ] 集成测试：构造缺 `sevenHole.p7` 的 DTO JSON，解码后 `ValidateConfig` 返回错误
- [ ] 既有五孔/三孔/总压/总温 DTO 解码测试零回归

**Verification:**
- [ ] 测试通过：`go test -v -run 'TestSevenHoleConfigDTO' ./internal/adapters/config/...`
- [ ] 既有 DTO 测试：`go test ./internal/adapters/config/...`
- [ ] Vet 通过：`go vet ./internal/adapters/config/...`

**Dependencies:** Task 1, Task 5

**Files likely touched:**
- `projects/wind-daq/services/api-go/internal/adapters/config/calibration_config_decoder_test.go`（新增七孔 DTO 集成测试）

**Estimated scope:** S（1 文件，仅测试）

---

### Task 9: createAlgorithm 工厂分支 + autoTypes map

**Description:** 在 `usecase/calibration.go` 的 `createAlgorithm` 工厂函数加入 `case calibration.TypeSevenHole: return calibration.NewSevenHoleAlgorithm(), nil` 分支；在 `autoTypes` map 加入 `TypeSevenHole` 让七孔走自动引擎；新增 `PreviewSevenHolePoints` usecase 方法封装点位生成（含配置校验、坐标换算），供 server.go HTTP handler 和 Wails binding 共用。

**Acceptance criteria:**
- [ ] `createAlgorithm` 新增 `case calibration.TypeSevenHole:` 分支
- [ ] `autoTypes` map（约 L205）加入 `calibration.TypeSevenHole: true`
- [ ] 新增 `PreviewSevenHolePoints(config calibration.SevenHoleConfig) (calibration.SevenHolePreviewResult, error)` usecase 方法
  - 调用 `calibration.GenerateSevenHolePoints(config)` 生成点位
  - 返回 `SevenHolePreviewResult{Points []CalPoint, TotalCount int, InnerCount int, OuterCount int}`
  - 不启动采集、不创建 CSV writer、不创建 runtime
- [ ] `Start` 方法对七孔类型走与五孔相同的自动引擎路径
- [ ] **双 CSV writer 方案**：`ports.CalibrationCsvWriter` 接口新增 `NewWriter(path string, schema CsvSchema) (CalibrationCsvWriter, error)` 工厂方法（或新增独立 `CalibrationWriterFactory` 端口）。`CalibrationManager` 在七孔 Start 时通过工厂创建两个 writer 实例（内区/外区各一个），`onDataPoint` 按 `dataPoint.(*SevenHoleDataPoint).Region` 路由到对应 writer。两个 writer 的 `Flush`/`Stop`/导出错误处理统一由 `CalibrationManager` 管理
- [ ] `onDataPoint` 回调按 `dataPoint.(type)` 中 `*SevenHoleDataPoint` 的 `Region` 字段路由到对应 writer
- [ ] 五孔/三孔/总压/总温 usecase 测试零回归

**Verification:**
- [ ] 测试通过：`go test -v -run 'TestCreateAlgorithmSevenHole' ./internal/usecase/...`
- [ ] PreviewSevenHolePoints 测试：`go test -v -run 'TestPreviewSevenHolePoints'`（验证返回 673/481 点）
- [ ] 既有 usecase 测试：`go test ./internal/usecase/...`
- [ ] Vet 通过：`go vet ./internal/usecase/...`

**Dependencies:** Task 5, Task 6

**Files likely touched:**
- `projects/wind-daq/services/api-go/internal/usecase/calibration.go`
- `projects/wind-daq/services/api-go/internal/usecase/calibration_test.go`

**Estimated scope:** M（2 文件）

---

### Task 10: moveToPoint 七孔分支 + RealtimeEvent 扩展集成

**Description:** 在 `automatic_calibration.go` 的 `moveToPoint` 方法（L274-303）加入 `TypeSevenHole` 分支，复用 `MoveToPointWithOrder(point, []string{"α", "β"})` 严格按 α→β 顺序下发；七孔点位的 `MotionCoordinates` 字段在 `MoveToPointWithOrder` 中优先读取（若非 nil），否则回退到 `Coordinates`。本任务还需确认 Task 1 中 `RealtimeEvent` 扩展字段在 `processPoint` 流程中正确填充。

**Acceptance criteria:**
- [ ] `moveToPoint` 新增 `algorithm.Type() == TypeSevenHole` 分支，调用 `MoveToPointWithOrder(point, []string{"α", "β"})`
- [ ] `MoveToPointWithOrder` 优先从 `point.MotionCoordinates[axisName]` 读取目标位置，若 `MotionCoordinates` 为 nil 则回退到 `point.Coordinates[axisName]`
- [ ] 五孔分支保持不变（零回归）
- [ ] 默认分支（非五孔/七孔）保持原 `for axisName, position := range point.Coordinates` 行为
- [ ] `processPoint` 流程在七孔类型时，`Config` 填充 `RealtimeCallback`（注入 `EventPublisher.OnRealtime`），`SevenHoleAlgorithm.AcquireDataWithChannels` 在采样期间通过 `config.RealtimeCallback` 推送实时事件
- [ ] `processPoint` 流程在七孔类型时，将 `AutomaticCalibration` 持有的 `prevRegion`/`prevSector` 注入 `Config`（新增 `PrevRegion string`/`PrevSector int` 字段，`json:"-"`），供 `DetermineRegion` 使用

**Verification:**
- [ ] 测试通过：`go test -v -run 'TestMoveToPointSevenHole' ./internal/core/calibration/...`
- [ ] 双坐标优先级测试：`go test -v -run 'TestMoveToPointDualCoordinates'`（MotionCoordinates 非 nil 时优先用）
- [ ] 五孔 moveToPoint 回归测试：`go test -v -run 'TestMoveToPointFiveHole'`
- [ ] 既有 automatic_calibration 测试零回归：`go test ./internal/core/calibration/...`
- [ ] Vet 通过：`go vet ./internal/core/calibration/...`

**Dependencies:** Task 1, Task 5

**Files likely touched:**
- `projects/wind-daq/services/api-go/internal/core/calibration/automatic_calibration.go`
- `projects/wind-daq/services/api-go/internal/core/calibration/automatic_calibration_test.go`

**Estimated scope:** S（2 文件）

---

### Task 11: EventPublisher OnRegionChanged 必需事件

**Description:** 在 `types.go` 的 `EventPublisher` 接口新增 `OnRegionChanged(event RegionChangedEvent)` 方法（必需事件，非可选）；定义 `RegionChangedEvent` 结构体含 `Region`/`Sector`/`PrevRegion`/`PrevSector`/`BoundaryFlag`/`PointIndex`/`TotalPoints` 字段；在 `automatic_calibration.go` 的 `processPoint` 流程中，首点必推送（prevRegion=null），分区切换时立即推送；所有实现 `EventPublisher` 的适配器（HTTP SSE、Wails EventEmit、轮询状态合并）需补全此方法。

**Acceptance criteria:**
- [ ] `RegionChangedEvent` 结构体定义，含 7 字段：`Region string`、`Sector int`、`PrevRegion *string`、`PrevSector *int`、`BoundaryFlag string`、`PointIndex int`、`TotalPoints int`。`PrevRegion`/`PrevSector` 为指针类型——首点时 `nil`（JSON 序列化为 `null`），后续点指向上一时刻的 region/sector 值。此设计与 spec §9.4 的 JSON null 契约一致，消费者可通过 `== null` 区分"无前序"与"合法零值"
- [ ] `EventPublisher` 接口新增 `OnRegionChanged(event RegionChangedEvent)` 方法
- [ ] `processPoint` 流程在七孔类型时：
  - 首点：必推送一次，`PrevRegion=nil`、`PrevSector=nil`（JSON `null`）
  - 后续点：当 `DetermineRegion` 返回的 region/sector 与上一时刻不同时立即推送，`PrevRegion`/`PrevSector` 指向上一时刻值
  - 不变时不推送（避免噪声）
- [ ] 所有 EventPublisher 实现补全 `OnRegionChanged` 方法（共 4 个实现，需逐一适配）：
  1. `adapters/event/multi_event_publisher.go` — 生产复合发布器，转发到子 publisher
  2. `adapters/event/wails_event_publisher.go` — Wails 模式，通过 `EventEmit("calibration:region-changed", payload)` 推送
  3. `adapters/event/http_event_publisher.go` — HTTP 模式，通过 SSE 或状态轮询暴露 region 字段
  4. `adapters/event/noop_event_publisher.go` — 测试/离线模式，空实现（no-op）
- [ ] 各实现的构造函数注入 `OnRegionChanged` 回调或通过接口方法实现
- [ ] 所有 EventPublisher 实现的单元测试补全 `OnRegionChanged` 测试用例
- [ ] HTTP 实现通过 SSE 或状态轮询暴露 region 字段（前端 5Hz 轮询 `status` 时能拿到当前 region/sector）
- [ ] Wails 实现通过 `EventEmit("calibration:region-changed", payload)` 推送
- [ ] `Status` 结构体新增 `CurrentRegion string` 和 `CurrentSector int` 字段（json omitempty），供前端轮询读取
- [ ] 五孔/三孔/总压/总温类型不触发 `OnRegionChanged`（保持默认空实现或 no-op）

**Verification:**
- [ ] 测试通过：`go test -v -run 'TestOnRegionChanged' ./internal/core/calibration/...`
- [ ] 首点推送测试：`go test -v -run 'TestRegionChangedFirstPoint'`（prevRegion="" 必推送）
- [ ] 分区切换测试：`go test -v -run 'TestRegionChangedSwitch'`（inner→outer、outer/1→outer/2 等场景）
- [ ] 不变不推送测试：`go test -v -run 'TestRegionChangedNoSwitch'`
- [ ] 既有 EventPublisher 实现测试零回归
- [ ] Vet 通过：`go vet ./internal/core/calibration/...`

**Dependencies:** Task 1, Task 5, Task 10

**Files likely touched:**
- `projects/wind-daq/services/api-go/internal/core/calibration/types.go`
- `projects/wind-daq/services/api-go/internal/core/calibration/automatic_calibration.go`
- `projects/wind-daq/services/api-go/internal/adapters/event/calibration_event_publisher.go`（或类似文件，补全 OnRegionChanged 实现）

**Estimated scope:** M（3 文件）

---

### Task 12: server.go 路由 /sevenhole/preview + /sevenhole/start

**Description:** 在 `api/server.go` 的 `/api/calibration/` 路由块（L207-331）新增 `sevenhole-preview` 和 `sevenhole-start` 两个路由（参考五孔 `fivehole` 路由 L296 范式）；新增 `handleSevenHolePreview` handler 调用 `CalibrationManager.PreviewSevenHolePoints(config)` 返回 `{points, totalCount, innerCount, outerCount}` 包装；新增 `handleSevenHoleStart` handler 复用现有 `start` 路由逻辑（或直接走 `start` 路由，type 字段为 `seven-hole`）。

**Acceptance criteria:**
- [ ] 路由 `sevenhole-preview`（POST）→ `handleSevenHolePreview`
- [ ] 路由 `sevenhole-start`（POST）→ 复用到现有 `handleStart` handler（`type` 字段为 `"seven-hole"`）。`handleStart` 中 `DecodeCalibrationConfig` 解码后 `startCalibration` 走 `createAlgorithm` 工厂，无需新增独立 handler。若现有 `start` 路由的参数校验阻挡七孔类型，改为按 `type` 字段分发（`case "seven-hole":` 跳过五孔特有校验）
- [ ] `handleSevenHolePreview` 接收 `calibration.SevenHoleConfig` JSON，调用 `CalibrationManager.PreviewSevenHolePoints(config)`，返回 `{points, totalCount, innerCount, outerCount}` 包装
- [ ] 错误处理：配置非法（步长 ≤ 0、范围 min > max）返回 400
- [ ] 路由命名兼容：当前路由器是单段 `strings.TrimPrefix`，使用 `sevenhole-preview` 单段命名（避免二级路径不兼容问题）
- [ ] 五孔 `fivehole` 路由保持不变（零回归）
- [ ] NaN 哨兵清洗：返回前调用 `sanitizePointForJSON` 清洗未配置轴的 NaN 值（参考 project_memory §38）

**Verification:**
- [ ] 测试通过：`go test -v -run 'TestHandleSevenHolePreview' ./api/...`
- [ ] 完整模式预览测试：`curl -X POST http://localhost:port/api/calibration/sevenhole-preview -d '{"mode":"full",...}'` 返回 673 点
- [ ] 数据集模式预览测试：`curl -X POST http://localhost:port/api/calibration/sevenhole-preview -d '{"mode":"dataset",...}'` 返回 481 点
- [ ] 错误处理测试：`go test -v -run 'TestHandleSevenHolePreviewError'`
- [ ] NaN 清洗测试：`go test -v -run 'TestSevenHolePreviewSanitizeNaN'`
- [ ] Vet 通过：`go vet ./api/...`

**Dependencies:** Task 9

**Files likely touched:**
- `projects/wind-daq/services/api-go/api/server.go`
- `projects/wind-daq/services/api-go/api/server_test.go`（或新增 handler 测试文件）

**Estimated scope:** S（2 文件）

---

### Task 13: Wails backend CalibrationPreviewSevenHole binding

**Description:** 在 `apps/desktop-wails/backend/app.go` 新增 `CalibrationPreviewSevenHole(config SevenHoleConfigDTO) GenericResponse` binding，调用 `CalibrationManager.PreviewSevenHolePoints(config)` 返回点位列表（参考 `CalibrationStart` L976-990 范式）。同步在 `pkg/types` 暴露 `SevenHoleConfigDTO` 公共别名（若需要）。

**Acceptance criteria:**
- [ ] `CalibrationPreviewSevenHole(config SevenHoleConfigDTO) GenericResponse` binding 定义
- [ ] 调用 `appContext.CalibrationMgr.PreviewSevenHolePoints(config.ToCore())`
- [ ] 返回 `GenericResponse{Success: true, Data: SevenHolePreviewResult}`
- [ ] 错误时返回 `GenericResponse{Success: false, Error: err.Error()}`
- [ ] `SevenHoleConfigDTO` 在 `pkg/types` 暴露公共别名（指向 `calibration.SevenHoleConfig`）
- [ ] Wails binding 生成：执行 `wails3 generate bindings -silent` 同步 TS binding
- [ ] 五孔既有 binding 零回归

**Verification:**
- [ ] Build 成功：`cd projects\wind-daq\apps\desktop-wails\backend; go build ./...`
- [ ] Wails binding 生成：`wails3 generate bindings -silent`
- [ ] 前端 TS 类型检查：`cd projects\wind-daq\apps\desktop-wails\frontend; npm run typecheck`
- [ ] 既有 binding 测试：`go test ./...`

**Dependencies:** Task 9, Task 12

**Files likely touched:**
- `projects/wind-daq/apps/desktop-wails/backend/app.go`
- `projects/wind-daq/pkg/types/calibration.go`（或类似公共类型文件，新增 SevenHoleConfigDTO 别名）

**Estimated scope:** S（2 文件）

---

### Checkpoint: Phase 3 — 后端集成

- [ ] `cd projects\wind-daq\services\api-go; go test ./internal/... ./api/...` 全绿
- [ ] `cd projects\wind-daq\apps\desktop-wails\backend; go build ./...` 成功
- [ ] `curl POST /api/calibration/sevenhole-preview` 返回 673 点（完整模式）/ 481 点（数据集模式）
- [ ] Wails binding `CalibrationPreviewSevenHole` 可从前端调用
- [ ] `moveToPoint` 七孔分支按 α→β 顺序下发
- [ ] `OnRegionChanged` 首点必推送、切换时推送、不变不推送
- [ ] CSV 内/外区两套表头正确，分文件落盘
- [ ] 五孔/三孔/总压/总温既有功能零回归
- [ ] 人工 review：路由命名兼容、NaN 清洗完整、压力基准三分离注释完整

---

## Phase 4: 数据集回归测试（算法正确性闸门）

### Task 14: 481 点数据集模式回归测试

**Description:** 编写端到端回归测试，加载数据集 `W532.202608.P.7H.1-01/` 的 481 个点位配置，调用 `GenerateSevenHolePoints` 生成点位，对每个点用数据集 CSV 中的原始通道值计算系数，验证计算值与 CSV 实测值误差 ≤ 0.001（系数）/ ≤ 0.005（Ma）。

**Acceptance criteria:**
- [ ] 测试 fixture 加载数据集 CSV 文件（UTF-8 转码副本或 `_headers_utf8.txt` 对照表头）
- [ ] 481 点全部走 `GenerateSevenHolePoints(mode=dataset)` 生成
- [ ] 每点用 CSV 原始通道值（P1~P7、p_t、p_s、大气压力）调用 `CalculateSevenHoleInnerCoefficients` 或 `CalculateSevenHoleOuterCoefficients`
- [ ] 系数误差验证：Kα/Kβ/K0/Ks/Kθ[n]/Kφ[n]/K0[n]/Ks[n] 计算值与 CSV 实测值误差 ≤ 0.001
- [ ] 马赫数误差验证：Ma 计算值与 CSV 实测值误差 ≤ 0.005
- [ ] 速度误差验证：V 计算值与标称 85 m/s 误差 ≤ 5%
- [ ] 分区判定验证：每点的 `DetermineRegion` 结果与数据集扇区编号一致
- [ ] 测试报告：失败点列表 + 误差分布直方图

**Verification:**
- [ ] 测试通过：`go test -v -run 'TestSevenHoleDatasetRegression481' ./internal/core/calibration/...`
- [ ] 测试报告生成：`go test -v -run 'TestSevenHoleDatasetRegression481' -report=dataset_regression.html`
- [ ] 481 点全部通过（误差在容差范围内）

**Dependencies:** Task 5, Task 6

**Files likely touched:**
- `projects/wind-daq/services/api-go/internal/core/calibration/seven_hole_dataset_regression_test.go`（新增）
- `projects/wind-daq/services/api-go/internal/core/calibration/testdata/seven_hole/`（新增数据集 fixture 目录）

**Estimated scope:** M（2 文件，含 fixture）

---

### Task 15: 5 个黄金用例角度换算测试

**Description:** 编写单元测试覆盖 spec §3.3 黄金用例表 G1~G5，验证正向（θ,φ → α,β）和反向（α,β → θ,φ）角度换算公式正确性。重点验证 α 公式负号未被误删。

**Acceptance criteria:**
- [ ] G1：θ=30°, φ=0° → α=0°, β=+30°；反向回算 (30°, 0°) ✓
- [ ] G2：θ=30°, φ=90° → α=-30°, β=0°；反向回算 (30°, 90°) ✓（验证负号）
- [ ] G3：θ=30°, φ=180° → α=0°, β=-30°；反向回算 (30°, 180°) ✓
- [ ] G4：θ=30°, φ=270° → α=+30°, β=0°；反向回算 (30°, 270°) ✓（验证负号）
- [ ] G5：θ=30°, φ=330° → α=+16.1°, β=+26.6°；反向回算 (30°, 330°) ✓
- [ ] 误差容差：角度误差 ≤ 0.01°
- [ ] G2/G4 负号验证：若误删负号，G2 会给出 α=+30°（来流偏 +X），与 φ=90°（-X 方位）矛盾，测试必须失败

**Verification:**
- [ ] 测试通过：`go test -v -run 'TestSevenHoleGoldenCases' ./internal/core/calibration/...`
- [ ] 负号保护测试：`go test -v -run 'TestSevenHoleAlphaSign'`（删除负号代码后测试必须失败）

**Dependencies:** Task 6

**Files likely touched:**
- `projects/wind-daq/services/api-go/internal/core/calibration/seven_hole_formulas_test.go`（追加黄金用例测试）

**Estimated scope:** S（1 文件）

---

### Task 16: 5 个 tie-break 构造用例测试

**Description:** 编写单元测试覆盖 spec §3.2 tie-break 规则的 5 个构造用例：P7=Pmax、P1=P2=Pmax、P1=P2=P3=Pmax、滞回触发、跨大跨度不滞回。

**Acceptance criteria:**
- [ ] 用例 1（P7 优先）：P1=100, P2=100, P3=100, P4=100, P5=100, P6=100, P7=102 → ("inner", 7, "")
- [ ] 用例 2（P7 与 Pn 并列）：P1=100, P7=103, P2=102, P3=50, P4=50, P5=50, P6=50, |P7-P1|=3 < 5 → 实际 |P7-Pmax|=0 < 5 → ("inner", 7, "P7-Pmax")
- [ ] 用例 3（P1=P2 并列，prevRegion="" 首点）：P1=100, P2=100, P3=50, P4=50, P5=50, P6=50, P7=80 → ("outer", 1, "P1-P2")
- [ ] 用例 4（滞回触发）：prevRegion="outer", prevSector=1, P1=100, P2=100（并列），P3=50... → ("outer", 1, "P1-P2")（保持 prevSector）
- [ ] 用例 5（跨大跨度不滞回）：prevRegion="outer", prevSector=1, P3=100, P4=100（candidates={3,4}，与 1 不相邻）→ ("outer", 3, "P3-P4")（不触发滞回，按编号小优先）
- [ ] 确定性验证：相同输入永远产生相同输出

**Verification:**
- [ ] 测试通过：`go test -v -run 'TestSevenHoleTieBreak' ./internal/core/calibration/...`
- [ ] 5 个用例全部符合 spec §3.2 规则预期

**Dependencies:** Task 3

**Files likely touched:**
- `projects/wind-daq/services/api-go/internal/core/calibration/seven_hole_test.go`（追加 tie-break 测试）

**Estimated scope:** S（1 文件）

---

### Task 17: 中心点不确定度 7 步算例测试

**Description:** 编写单元测试覆盖 spec §5.5 中心点 K0 不确定度 7 步算例，逐行复算验证 `UncertaintyCalculator` 实现正确性。数值精确匹配（容差 1%）。

**Acceptance criteria:**
- [ ] 第 1 步 B 类分量：u_B(P7)=20.42, u_B(p_t)=3.81, u_B(p_s)=6.53 Pa（容差 ±0.1）
- [ ] 第 2 步 A 类分量：u(A,P7)=0.894, u(A,p_t)=1.342, u(A,p_s)=0.671 Pa（容差 ±0.01）
- [ ] 第 3 步 K0 灵敏系数：c(P7)=2.436e-4, c(p_t)=-2.437e-4, c(p_s)=1.352e-7（容差 ±1e-6）
- [ ] 第 4 步 B 类合成：|Σ cᵢ·u_B,i| = 4.046e-3（容差 ±1e-4）
- [ ] 第 5 步 A 类合成：√(Σ cᵢ²·u(A,i)²) = 3.928e-4（容差 ±1e-5）
- [ ] 第 6 步合成标准不确定度：u_c(K0) = 4.065e-3（容差 ±1e-4）
- [ ] 第 7 步扩展不确定度：U(K0) = 8.13e-3（容差 ±1e-3）
- [ ] 验收：U(K0) ∈ [0.004, 0.0082]（自动验收四舍五入到 3 位小数后比较）
- [ ] 对比验证：误用"Σ|cᵢ|uᵢ"公式会得到 5.903e-3（高估 46%），测试必须区分两种公式

**Verification:**
- [ ] 测试通过：`go test -v -run 'TestUncertaintyCenterPointK0' ./internal/core/calibration/...`
- [ ] 7 步逐步验证：`go test -v -run 'TestUncertaintyCenterPointK0Step'`
- [ ] 公式对比测试：`go test -v -run 'TestUncertaintyFormulaComparison'`

**Dependencies:** Task 4

**Files likely touched:**
- `projects/wind-daq/services/api-go/internal/core/calibration/seven_hole_uncertainty_test.go`（追加 7 步算例测试）

**Estimated scope:** S（1 文件）

---

### Checkpoint: Phase 4 — 算法正确性闸门

- [ ] 481 点数据集回归测试全绿（系数误差 ≤ 0.001，Ma 误差 ≤ 0.005）
- [ ] 5 个黄金用例 G1~G5 全部通过（角度误差 ≤ 0.01°）
- [ ] 5 个 tie-break 构造用例全部符合 spec §3.2 规则
- [ ] 中心点 K0 不确定度 7 步算例复算 U(K0) = 8.13e-3（∈ [0.004, 0.0082]）
- [ ] α 公式负号保护测试通过（删除负号必失败）
- [ ] 合成公式正确性验证（|Σ cᵢuᵢ| 与 Σ|cᵢ|uᵢ 差异 46%）
- [ ] 人工 review：数据集 fixture 不修改原始文件，使用 UTF-8 转码副本

---

## Phase 5: 前端 MVP

### Task 18: calibrationApi.ts 新增 previewSevenHole

**Description:** 在 `calibrationApi.ts` 新增 `previewSevenHolePoints(config)` 方法，Wails 模式调 `wailsApi.calibration.previewSevenHole(config)`，HTTP 模式调 `POST /api/calibration/sevenhole-preview`。**禁止像五孔那样在 Wails 模式返回空数组**——必须真实调用后端 API。

**Acceptance criteria:**
- [ ] `previewSevenHolePoints(config: SevenHoleConfig): Promise<SevenHolePreviewResult>` 方法定义
- [ ] Wails 模式：`return await wailsApi.calibration.previewSevenHole(config)`（**禁止 `return []`**）
- [ ] HTTP 模式：`return await request('/api/calibration/sevenhole-preview', { method: 'POST', body: config })`
- [ ] 返回类型 `SevenHolePreviewResult`：`{ points: CalPoint[], totalCount: number, innerCount: number, outerCount: number }`
- [ ] `SevenHoleConfig` TS 类型定义（与后端 Go 结构对齐）
- [ ] 离线场景（Wails 不可用 + HTTP 失败）抛错，不 fallback 到本地点位生成
- [ ] 五孔 `generateFiveHoleSnakePoints` 方法保持不变（零回归）

**Verification:**
- [ ] TypeScript 类型检查：`cd projects\wind-daq\apps\desktop-wails\frontend; npm run typecheck`
- [ ] 单元测试：`npm run test -- --grep previewSevenHole`
- [ ] Build 成功：`npm run build`
- [ ] Code review：禁止出现 `motionCalibrationUtils.ts` 风格的本地 fallback

**Dependencies:** Task 12, Task 13

**Files likely touched:**
- `projects/wind-daq/apps/desktop-wails/frontend/src/api/calibrationApi.ts`
- `projects/wind-daq/apps/desktop-wails/frontend/src/shared/types/calibration.ts`（新增七孔 TS 类型）

**Estimated scope:** S（2 文件）

---

### Task 19: calibrationStore.ts 七孔状态字段 + RealtimePressures 扩展

**Description:** 扩展 `calibrationStore.ts` 支持七孔类型：`RealtimePressures` 接口扩展 P6/P7 字段（或七孔独立定义 `SevenHoleRealtimePressures`）；`CalibrationAnyDataPoint` 联合类型追加 `SevenHoleDataPoint`；store 状态新增 `currentRegion`/`currentSector`/`boundaryFlag` 字段，由 `OnRegionChanged` 事件更新；5Hz 轮询和状态恢复协议复用既有机制。

**Acceptance criteria:**
- [ ] `RealtimePressures` 接口新增 P6/P7 字段（可选，五孔数据不填时为 undefined）
- [ ] `SevenHoleDataPoint` TS 类型定义，追加到 `CalibrationAnyDataPoint` 联合类型
- [ ] store 状态新增 `currentRegion: Ref<string>`、`currentSector: Ref<number>`、`boundaryFlag: Ref<string>`
- [ ] `OnRegionChanged` 事件订阅：更新 `currentRegion`/`currentSector`/`boundaryFlag`
- [ ] `recoveryFromBackend()` 同步后端 `Status.CurrentRegion`/`CurrentSector` 字段
- [ ] 5Hz 轮询机制复用既有 `startStatusPolling` / `acquireView` / `releaseView`
- [ ] 状态恢复协议复用既有 `recoveryFromBackend`（参考 project_memory §53）
- [ ] 五孔/三孔/总压/总温 store 行为零回归

**Verification:**
- [ ] TypeScript 类型检查：`npm run typecheck`
- [ ] 单元测试：`npm run test -- --grep calibrationStore`
- [ ] Build 成功：`npm run build`
- [ ] 状态恢复测试：`npm run test -- --grep recoveryFromBackend`

**Dependencies:** Task 18

**Files likely touched:**
- `projects/wind-daq/apps/desktop-wails/frontend/src/stores/calibrationStore.ts`
- `projects/wind-daq/apps/desktop-wails/frontend/src/shared/types/calibration.ts`

**Estimated scope:** M（2 文件）

---

### Task 20: SevenHoleSettings.vue 3 步配置向导

**Description:** 新建 `SevenHoleSettings.vue`，参考 `FiveHoleSettings.vue` 的 3 步向导范式（`UiSteps`/`UiStep` 自定义组件）。3 步内容：基本设置（配置名、刷新率、CSV 保存路径）+ 内区角度范围（α/β min/max/step）+ 外区角度范围（θ/φ min/max/step）+ 不确定度参数 + 11 通道映射 + 运动轴配置 + 球罐门控。**每次配置变更后调用 `previewSevenHolePoints` API 获取真实点数与预计耗时**，禁止本地实现点位生成。

**Acceptance criteria:**
- [ ] 3 步向导：基本设置 → 硬件配置（11 通道 + 运动轴 + 球罐门控）→ 确认保存
- [ ] 步骤 0：配置名、界面刷新率、CSV 保存目录+文件名、校准模式（完整/数据集）
- [ ] 步骤 0：内区 α/β min/max/step、外区 θ/φ min/max/step 输入框
- [ ] 步骤 0：不确定度参数（k=2、TIE_BREAK_TOLERANCE、采样次数、驻留时间）
- [ ] 步骤 1：11 通道映射表（`sevenHole.p1`~`p7`、`sevenHole.pTotal`、`sevenHole.pTunnelStatic`、`sevenHole.pAtm`、`sevenHole.tAtm`），含必填校验
- [ ] 步骤 1：运动轴配置 + `MotionSafetyPanel` + 球罐判定门控
- [ ] 步骤 2：配置摘要双列网格
- [ ] 右侧 sidebar 跨步骤常驻：总点数/驻留/采样统计 + SVG 点阵预览
- [ ] **点阵预览调 `calibrationApi.previewSevenHolePoints(layout)` 获取真实点位**，禁止本地生成
- [ ] 离线场景（API 失败）：显示"请先连接后端"提示，不 fallback
- [ ] 配置变更触发预览防抖（500ms）
- [ ] i18n 完整：所有 label 用 i18nStore 翻译
- [ ] 颜色规范：所有颜色用设计 token（var(--xx)），禁止硬编码
- [ ] 文件命名清洗：CSV 保存路径清洗非法字符 `/ \ : * ? " < > |` 为 `-`（参考 project_memory §6）

**Verification:**
- [ ] TypeScript 类型检查：`npm run typecheck`
- [ ] Build 成功：`npm run build`
- [ ] Code review：禁止新增 `motionCalibrationUtils.ts` 或任何点位生成相关文件
- [ ] 手动验证：配置向导调整角度范围后，右侧 sidebar 点阵预览实时更新
- [ ] 手动验证：离线场景显示"请先连接后端"提示

**Dependencies:** Task 18, Task 19

**Files likely touched:**
- `projects/wind-daq/apps/desktop-wails/frontend/src/components/calibration/seven-hole/SevenHoleSettings.vue`（新增）
- `projects/wind-daq/apps/desktop-wails/frontend/src/components/calibration/seven-hole/SevenHoleSettings.spec.ts`（新增单元测试，可选）

**Estimated scope:** L（1-2 文件，但同模块内聚）

---

### Task 21: SevenHoleMain.vue 主画面 + 顶部状态栏

**Description:** 新建 `SevenHoleMain.vue`，参考 `FiveHoleMain.vue` 的顶部状态栏范式（L932-1008）。顶部状态栏需含状态徽章、进度条、已用/剩余时间、目标 α/β、实际 α/β、**当前区域（内区/外区 n 区）**、马赫数、速度、运动状态、采样子进度、错误详情、配置摘要折叠按钮。

**Acceptance criteria:**
- [ ] 顶部状态栏（`sticky top-0 z-10`，跨全宽 flex 横排）：
  - 状态徽章（`statusText` + `statusColorToken`）
  - 进度条（`progressPercent` + `formattedProgress`）
  - 时间信息（`travElapsed`/`travRemaining`）
  - 目标 α/β + 实际 α/β（`isMoving` 脉冲指示）
  - **当前区域徽章**（`currentRegion`/`currentSector`，如"内区"/"外区 1 区"）
  - 马赫数 + 速度（`calculatedPhysics.machNumber`/`velocity`）
  - 当前点采样子进度（`sampleProgress.current/total`）
  - 错误详情（`lastError` 截断 30 字符）
  - 配置摘要折叠按钮
- [ ] 左侧栏（固定 384px 宽，参考 project_memory §23）：
  - 顶部固定控制按钮区（启动/暂停/恢复/停止/校零）
  - 中间可滚动数据区：P1~P7 核心通道大字号（20px+）+ Ma/V 紧随 + PAtm/TAtm/PTotal/PStatic 次要通道可折叠
  - 底部固定球罐门控状态条
- [ ] 主区域：内区 3 类图表（Task 22 实现）+ 配置信息顶部可折叠小字
- [ ] `MotionSafetyAlertCard` 独立卡片
- [ ] `cleanupSubscriptions()` 在 `onBeforeUnmount` 调用（**禁止 `calibrationStore.reset()`**，参考 project_memory §53）
- [ ] 颜色规范：所有 UI 元素颜色用设计 token
- [ ] i18n 完整：所有 label 用 i18nStore 翻译
- [ ] 角度定义明确标注：UI 区分"内区 α/β"和"外区 θ/φ"（spec §12 第 2 条）

**Verification:**
- [ ] TypeScript 类型检查：`npm run typecheck`
- [ ] Build 成功：`npm run build`
- [ ] 手动验证：顶部状态栏正确显示当前区域（内区/外区 n 区）
- [ ] 手动验证：切走切回状态恢复正常（不调用 reset）

**Dependencies:** Task 19, Task 20

**Files likely touched:**
- `projects/wind-daq/apps/desktop-wails/frontend/src/components/calibration/seven-hole/SevenHoleMain.vue`（新增）

**Estimated scope:** L（1 文件，但内容多）

---

### Task 22: SevenHoleCharts.vue 内区 3 类图表

**Description:** 新建 `SevenHoleCharts.vue`，实现 spec §11.2 内区 3 类特性曲线图（MVP 阶段）：Kα-Kβ 特性曲线（散点图）、α-K0 总压系数曲线、α-Ks 静压系数曲线。外区 3 类图表（Kθ-Kφ、φ-K0[n]、φ-Ks[n]）留待 Phase 6 第二阶段。

**Acceptance criteria:**
- [ ] 内区图表 1：Kα-Kβ 散点图（X 轴 Kα，Y 轴 Kβ，颜色按 α 角度渐变）
- [ ] 内区图表 2：α-K0 曲线（X 轴 α，Y 轴 K0，多条曲线按 β 分组）
- [ ] 内区图表 3：α-Ks 曲线（X 轴 α，Y 轴 Ks，多条曲线按 β 分组）
- [ ] 图表组件使用 ECharts，配置 `large: true` 优化渲染性能（参考 project_memory §27）
- [ ] 颜色规范：所有图表颜色从 `ChartTheme` 读取（参考 project_memory §54）
- [ ] i18n 完整：axis 名称、tooltip、legend 用 i18nStore 翻译
- [ ] 数据更新节流：通过 `useThrottledChartUpdate` composable 实现 rAF 节流（参考 project_memory §54）
- [ ] unmount 时取消挂起的 rAF 任务
- [ ] 图表高度 160-180px，宽高比约 3:2（参考 project_memory §23）
- [ ] 坐标轴含线条、刻度标签和网格线
- [ ] 外区图表占位：显示"外区图表第二阶段实现"提示，不报错

**Verification:**
- [ ] TypeScript 类型检查：`npm run typecheck`
- [ ] Build 成功：`npm run build`
- [ ] 手动验证：内区 169 点走完后 3 类图表正确绘制
- [ ] 手动验证：unmount 时无 rAF 泄漏（DevTools 检查）

**Dependencies:** Task 19, Task 21

**Files likely touched:**
- `projects/wind-daq/apps/desktop-wails/frontend/src/components/calibration/seven-hole/SevenHoleCharts.vue`（新增）
- `projects/wind-daq/apps/desktop-wails/frontend/src/composables/useThrottledChartUpdate.ts`（若不存在则新增，参考 project_memory §54）

**Estimated scope:** M（1-2 文件）

---

### Task 23: OnRegionChanged 前端订阅 + 区域显示

**Description:** 在前端订阅后端 `OnRegionChanged` 事件（Wails 模式通过 `EventEmit`，HTTP 模式通过 5Hz 轮询 `status` 中的 `currentRegion`/`currentSector` 字段），更新 `calibrationStore` 的 `currentRegion`/`currentSector`/`boundaryFlag` 字段；在 `SevenHoleMain.vue` 顶部状态栏显示当前区域。

**Acceptance criteria:**
- [ ] Wails 模式：订阅 `wailsApi.events.on('calibration:region-changed', (payload) => updateRegion(payload))`
- [ ] HTTP 模式：5Hz 轮询 `status` 时，从 `Status.CurrentRegion`/`CurrentSector` 字段同步到 store
- [ ] `updateRegion(payload)` 更新 store 的 `currentRegion`/`currentSector`/`boundaryFlag`
- [ ] 首点事件（`prevRegion=null`，即 `PrevRegion *string` 为 `nil`）：正确初始化 store 的 `currentRegion`
- [ ] 分区切换：store 的 `currentRegion` 立即更新，UI 顶部状态栏同步刷新
- [ ] 边界点标记：`boundaryFlag` 非空时在 UI 显示"边界点"标记
- [ ] `cleanupSubscriptions()` 在 `onBeforeUnmount` 取消 Wails 事件订阅
- [ ] 不引入新的轮询机制（复用 5Hz status 轮询）

**Verification:**
- [ ] TypeScript 类型检查：`npm run typecheck`
- [ ] Build 成功：`npm run build`
- [ ] 手动验证：内区 → 外区切换时，顶部状态栏立即显示"外区 n 区"
- [ ] 手动验证：边界点显示"边界点"标记

**Dependencies:** Task 19, Task 21

**Files likely touched:**
- `projects/wind-daq/apps/desktop-wails/frontend/src/components/calibration/seven-hole/SevenHoleMain.vue`
- `projects/wind-daq/apps/desktop-wails/frontend/src/stores/calibrationStore.ts`（追加 OnRegionChanged 订阅逻辑）
- `projects/wind-daq/apps/desktop-wails/frontend/src/api/calibrationApi.ts`（追加事件订阅辅助函数，可选）

**Estimated scope:** S（2-3 文件）

---

### Task 24: 状态恢复协议 recoveryFromBackend 七孔扩展

**Description:** 验证 `calibrationStore.ts` 的 `recoveryFromBackend()` 方法对七孔类型正确工作：同步后端 `Status.CurrentRegion`/`CurrentSector` 字段；落点决策由后端 status 决定（running/paused/completed/error/stopped 进入 SevenHoleMain，idle 进入 Home 卡片选择页）；跨类型切换保护（CalibrationWindow.vue 切换探针类型前检查 store.status?.type）。

**Acceptance criteria:**
- [ ] `recoveryFromBackend()` 同步七孔 `Status.CurrentRegion`/`CurrentSector` 到 store
- [ ] `recoveryFromBackend()` 同步七孔 `DataPoints` 到 store（含 `SevenHoleDataPoint` 类型）
- [ ] `recoveryFromBackend()` 失败时保留旧 store 状态（不 reset），UI 显示错误条 + 可重试（参考 project_memory §53）
- [ ] `CalibrationWindow.vue` 落点决策：
  - 后端 status.type="seven-hole" 且 state=running/paused/completed/error/stopped → 进入 SevenHoleMain
  - 后端 status.type="seven-hole" 且 state=idle → 进入 Home 卡片选择页
  - 后端 status.type 与用户选择类型不匹配 → 弹确认框（参考 project_memory §53）
- [ ] `CalibrationWindow.vue` 切换探针类型前检查 `store.status?.type`，若后端任务运行中且类型不匹配，弹确认框
- [ ] 停止失败时提示错误并立即 return，禁止继续切换
- [ ] 落点由后端 status 决定，禁止落点闪烁、禁止空闲态记忆类型（参考 project_memory §53）
- [ ] `CalibrationHome.vue` 新增七孔卡片入口：卡片标题"七孔探针校准"，描述"7 孔自动校准，支持内区/外区双坐标系"，图标色 `#7c3aed`，点击跳转 `type='seven-hole'`。位置在五孔卡片右侧（参考五孔卡片 `type='five-hole'` 范式）
- [ ] 五孔/三孔/总压/总温状态恢复协议零回归

**Verification:**
- [ ] TypeScript 类型检查：`npm run typecheck`
- [ ] Build 成功：`npm run build`
- [ ] 手动验证：七孔任务运行中切走再切回，正确进入 SevenHoleMain 且状态恢复
- [ ] 手动验证：七孔任务 idle 时切回，进入 Home 卡片选择页
- [ ] 手动验证：七孔任务运行中切换到五孔，弹确认框

**Dependencies:** Task 19, Task 21

**Files likely touched:**
- `projects/wind-daq/apps/desktop-wails/frontend/src/stores/calibrationStore.ts`（追加七孔状态恢复逻辑）
- `projects/wind-daq/apps/desktop-wails/frontend/src/components/calibration/CalibrationWindow.vue`（追加七孔落点决策与跨类型切换保护）
- `projects/wind-daq/apps/desktop-wails/frontend/src/components/calibration/CalibrationHome.vue`（追加七孔卡片入口）

**Estimated scope:** M（3 文件）

---

### Checkpoint: Phase 5 — MVP 端到端可用

- [ ] `cd projects\wind-daq\apps\desktop-wails\frontend; npm run typecheck; npm run test; npm run build` 全绿
- [ ] 配置向导 → 启动校准 → 内区 169 点走完 → CSV 正确 → 状态恢复正常
- [ ] 顶部状态栏正确显示当前区域（内区/外区 n 区）
- [ ] 内区 3 类图表正确绘制
- [ ] 切走切回状态恢复（不调用 reset）
- [ ] 跨类型切换保护生效
- [ ] Code review：前端无 `motionCalibrationUtils.ts` 风格的本地 fallback
- [ ] Code review：所有颜色用设计 token，i18n 完整
- [ ] 人工 review：UI 区分"内区 α/β"和"外区 θ/φ"

---

## 最终验收（CP5 — MVP 子集）

> **范围说明**：CP5 验收 MVP 子集，完整 spec §11.1~§11.3 验收由 Phase 6 承担。MVP 不包含：673 点端到端流程、外区 3 类图表（Kθ-Kφ/φ-K0[n]/φ-Ks[n]）、准备阶段预热倒计时与安装检查清单。

- [ ] spec §11.1 算法正确性验收全部通过（内区/外区公式、分区判定、tie-break、黄金用例、马赫数、速度、不确定度）——数据集模式 481 点回归覆盖
- [ ] MVP 流程完整性验收通过：配置向导 → 启动校准 → 内区 169 点走完 → 内区 CSV 正确 → 状态恢复正常
- [ ] spec §11.3 性能验收（MVP 子集）：单点本体 ≤ 10s、典型 ≤ 30s、481 点 ≤ 4h
- [ ] `OnRegionChanged` 必需事件验收（首点必推送、切换时推送、payload 完整）
- [ ] 点位生成（后端唯一）验收（前端无本地点位生成算法）
- [ ] 双坐标模型验收（每个点位同时填充 Coordinates 和 MotionCoordinates）
- [ ] 运动顺序验收（moveToPoint 按 ["α","β"] 顺序下发）
- [ ] 全工作区 `validate-structure.ps1` 通过
- [ ] wind-daq 全量构建与测试通过：`go build` + `go test` + `go vet` + `npm typecheck` + `npm build`
- [ ] 人工最终 review：可发布

---

## Out of Scope（本期不实现，留待 Phase 6 第二阶段）

- **外区 3 类特性曲线图**（Kθ-Kφ、φ-K0[n]、φ-Ks[n]）—— Task 22 仅实现内区 3 类
- **完整模式 673 点端到端流程验收** —— Task 14 仅验证 481 点数据集模式
- **五孔历史遗留 `motionCalibrationUtils.ts` 清理** —— 单独议题，七孔模块严格禁止新增类似文件
- **校准证书 PDF/Excel 导出** —— Q10 选 CSV，其他格式后续按需
- **七孔实测反算应用阶段**（spec §4.3 "边界不确定区双解输出"）—— 属于应用层，非校准模块范围

---

## See Also

- [spec-seven-hole-calibration.md](./spec-seven-hole-calibration.md) — 完整规格（v1.2）
- [plan-seven-hole-calibration.md](./plan-seven-hole-calibration.md) — 实现计划（6 Phase / 24 Task）
- 既有 tasks 参考：[tasks-motion-status-monitor.md](./tasks-motion-status-monitor.md)
- 数据集验证：`projects/wind-daq/docs/W532.202608.P.7H.1-01/`（85 m/s，Ma≈0.242，481 个点位）
