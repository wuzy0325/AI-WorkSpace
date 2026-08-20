# 七孔探针角度外推规范（Spec: Seven-Hole Outer-Zone Angle Extrapolation）

> 文档目的：定义七孔探针插值在外区越界场景下的**角度外推**算法契约、行为决策（"能算出数据就显示，有效性由用户判断"）、Go 实现要点与验收标准，作为组入代码的依据。
> 关联算法权威：`device-lab/skills/seven-hole-probe/SKILL.md` §3.8（越界外推原型）+ `seven_hole.py`（`beyond_border`，参考实现）。
> 关联实现包：`shared/algorithms/go/sevenhole/interpolation/`（`SevenHolePrbInterpolator`）。
> 关联桌面应用：`projects/probe-interpolator`（`apps/desktop-wails/backend/seven_hole_service.go`）。
> 日期：2026-08-19
> 状态：草案（待评审）

---

## 1. 背景与动机

### 1.1 现场问题

用户数据（如 `102原始.csv`）中部分测点位于七孔探针大角度扇区**校准网格之外**：外区 PRB 的 θ 仅覆盖 `{30°, 35°, 40°, 45°}`，而实测反推角度可达 46°~70°。现行实现（spec-seven-hole-traversal.md §4 决策）在这些点返回 `IsValid=false`，前端不显示。

### 1.2 需求决策

客户明确要求：**能算出数据就显示，结果是否有效由用户自行判断**。因此：

- 确认属于 θ 外边界且计算成功的越界点走**角度外推**路径，返回可展示的角度与压力数值；无法确认方向或计算失败的点仍按失败处理；
- 结果携带 `Warning` 标注"外推点"，供 UI 提示用户自行判断有效性；
- 成功完成计算的外推点为 `IsValid=true`，但**外推点必须显式标记**（Warning 字段），禁止无标记地混入正常点。

> 本决策**取代** spec-seven-hole-traversal.md §4 "不外推" 与 Q3 "外推留待新 spec 增补" 的旧决策。旧决策中"外推无校准数据支撑、误差不可控"的风险仍成立，故外推点必须标记、不得静默。

---

## 2. 术语与范围

| 术语 | 含义 |
|---|---|
| 外区扇区 n | 最大压力孔对应的扇区，n∈{1..6} |
| (ka,kb) | 大角度模式方向系数对，插值定位坐标（无量纲） |
| θ / φ | 探头坐标系俯仰角（恒正）/ 滚转角（方位角） |
| θMax | 外区网格最大 θ（标准网格 45°；thetaCount=4 时 45°） |
| 外推 | 用扇区最外层网格单元（θ∈[θMax-5, θMax]）的局部映射，把 (ka,kb) 反演到 θ>θMax |
| 仿射反演 | 用最外层网格单元 4 个角点的 (ka,kb)→(θ,φ) 线性映射逆解 |

**范围**：
- 仅覆盖**大角度模式两候选扇区均未命中，且已确认越界方向为 θ > thetaMax**时的外推（`Calculate` 的最终越界分支）。
- 本规范不把 φ 侧边越界伪装成 θ 外推。无法证明为 θ 外边界越界的点保持 `IsValid=false`，并返回非空 Warning。
- 小角度（内区）越界、`(ka,kb)` 为 NaN/Inf、大气参数非法等既有错误路径**不改变**。

---

## 3. 外推算法契约（权威描述）

### 3.1 触发条件

在 `SevenHolePrbInterpolator.Calculate` 中，当：
1. 内区多边形未命中，且
2. 第一候选扇区（最大压力孔）与第二候选扇区（次大压力孔）多边形均未命中，

时，先按 §3.3 筛选候选单元；只有确认存在 `theta_raw > thetaMax` 的 θ 外边界外推解，才进入**外推路径**（替代原 `IsValid=false` 返回）。

如果候选单元只能得到 φ 侧边越界、`theta_raw <= thetaMax`、非有限值或无法通过残差校验，
不得进入本路径，保持 `IsValid=false` 并返回说明具体原因的 Warning。

**内区命中但物理结果非法的回退**（2026-08 修订）：内区（小角度）多边形命中、但解析出的
`Pt < Ps`（物理异常）时，`Calculate` **不直接中止**，而是继续尝试外区两候选扇区与外推路径
——实测数据中这类行的内区系数虽落在小角度多边形内，压力分布却已超出小角度量程，外区/外推
反而能给出可展示的结果（如 `102原始.csv` 第 60/61 行：内区 `Pt=-779 < Ps=-747`，外推路径
给出 `Pt=-706 > Ps=-735`）。若外区与外推均无法给出结果，再回退返回内区的原始 `Pt<Ps` 错误。
此回退不改变"`Pt<Ps` 报错"的既有守卫语义——仅改变路径优先级，不静默取绝对值。

### 3.2 候选扇区选择

使用**第一候选扇区**（最大压力孔 `first`）。依据：SKILL.md §3.8 的 `beyond_border` 对越界点同样使用 `max_keys['first']`；次大孔在最大孔扇区外时通常更远，无额外信息。

### 3.3 仿射反演 (ka,kb) → (θ,φ)

对扇区 n（`thetaCount` 动态，标准网格 =4），取最外层 θ 单元 `[θLo, θHi] = [θMax-5, θMax]`（标准网格 `[40°,45°]`）。

对每个 φ 网格区间 `ip ∈ [0, 11]`（`φLo = center+30-5·ip`，`φHi = center+30-5·(ip+1)`），取 4 个角点：

```
A = 网格点(θLo, φLo)   B = 网格点(θLo, φHi)
C = 网格点(θHi, φLo)   D = 网格点(θHi, φHi)
```

建立局部仿射映射：

```
a1 = (C.ka - A.ka) / (θHi - θLo)          // dka/dθ @ φLo
b1 = (B.ka - A.ka) / (φHi - φLo)          // dka/dφ @ θLo
a2 = (C.kb - A.kb) / (θHi - θLo)          // dkb/dθ @ φLo
b2 = (B.kb - A.kb) / (φHi - φLo)          // dkb/dφ @ θLo
den = a1·b2 - a2·b1
```

逆解：

```
dθ = ((ka - A.ka)·b2 - (kb - A.kb)·b1) / den
dφ = (a1·(kb - A.kb) - a2·(ka - A.ka)) / den
θ  = θLo + dθ
φ  = φLo + dφ
```

候选单元筛选与选择：

1. 对反演得到的 `φ` 做周期展开，使其落在当前扇区中心附近的连续区间；跨 0°/360° 时允许等价的 `φ±360°`。
2. 仅保留满足 `theta > thetaHi + eps` 的候选；本规范只允许从最外层 θ 边界向外推，不允许把 θ 内侧或 φ 侧边结果钳制成 θ 外推。
3. 用实际校准单元的双线性映射（不是同一仿射逆解对应的线性映射）重建 `(ka,kb)`，计算重建残差 `r`。残差只用于候选排序，不作为固定绝对阈值的硬拒绝条件；否则外推距离增大时会把正确的 θ 外推点误判为无效。若实现没有双线性正向映射，则使用有限性检查并跳过残差排序。
4. 在剩余候选中选择残差最小者；残差相同再选择 `|φ-(φLo+φHi)/2|` 最小者。

如果没有候选通过筛选，返回“无法确认 θ 外边界外推方向”的不可用结果，不得任意选择一个 φ 区间。

**退化守卫**：`|den| < 1e-12` 时跳过该区间；全部区间退化则返回 error。单点调用返回失败响应；批量调用由既有 `BatchCalculateSevenHole` 将 error 转为该行的非空 Warning。

> 说明：本反演是线性化（忽略了网格单元的二次交叉项），在 θ 略超 45° 时误差小；θ 外推越远，精度越低——这正是"有效性由用户判断"的物理依据。

### 3.4 θ 钳制

```
thetaMax = outerThetaMin + gridStep·(thetaCount-1)
thetaExtrapMax = thetaMax + 5
θ_clamp = clamp(θ, thetaMax, thetaExtrapMax)
```

- 下界为当前扇区的 `thetaMax`：外推只沿 θ 外边界进行，禁止把 θ 内侧结果压到 30°后继续计算。
- 上界为当前扇区 `thetaMax+5°`。超过该上界的原始外推（标准数据集最高约 70°）**系数外推误差过大，不用于 Pt/Ps/V/Ma 计算**，但原始 θ 仍可展示（见 §3.7）。
- `thetaExtrapMax` 必须由当前扇区最后一个 θ 网格点推导，禁止使用固定的全局 50° 常量。

### 3.5 cpt/cps 外推插值

在 θ 方向对最外层单元**线性外推**（仅当 `theta_raw > thetaHi` 时沿用最后一段斜率）：

- 复用 §3.3 选出的 φ 网格区间 `ip`；不得重新选择另一个区间。跨界时使用连续展开的 φ 坐标，`ip2=ip+1` 并按周期回绕。
- 取 4 角点 cpt/cps 值，先沿 φ 求 θ=θLo 与 θ=θHi 两列的斜率，再沿 θ 混合：

```
对 cpt / cps 分别计算:
k_lo = (v_B - v_A) / (φHi - φLo)          // θ=θLo 列斜率
cp_lo = v_A + k_lo·(φ - φLo)
k_hi = (v_D - v_C) / (φHi - φLo)          // θ=θHi 列斜率（角点 C/D）
cp_hi = v_C + k_hi·(φ - φLo)
cp = cp_lo + (cp_hi - cp_lo)·(θ_clamp - θLo) / (θHi - θLo)
```

> 与既有 `outerBilinearCptCps`（outer_zone.go）方向一致，仅把 θ 方向混合从"插值"放宽为"插值/外推"。

### 3.6 Pt/Ps 求解、坐标变换、V/Ma

与既有大角度路径**完全相同**：

- `solveOuterPtPs(in, n, cpt, cps)`：`Pt = (pc·(1+cps) + cpt·pside)/d`，`Ps = (pc·cps + pside·(1+cpt))/d`，`d=1+cpt+cps`；`|d|<1e-12` 返回 error。
- 坐标变换 `convertThetaPhiToAlphaBeta(θ_clamp, φ)`：`β = atan(tanθ·cosφ)`，`α = -atan(tanθ·sinφ)`（负号保留）。
- V/Ma：`calVelocityMach(pt, ps, PAtm, TAtm)`，`Pt<Ps`、压力比<1 等守卫照旧。

### 3.7 结果语义（IsValid / Warning / 展示）

| 字段 | 值 | 说明 |
|---|---|---|
| `IsValid` | 完整计算成功时 `true`；失败时不返回成功结果 | 客户要求只约束可计算的外推点 |
| `Theta` / `Phi` | **原始外推** (θ_raw, φ) | 展示探头感受到的真实偏角，供用户判断；φ 需归一化到 `[0,360)` |
| `Alpha` / `Beta` | 由 **θ_clamp** 变换 | 与 Pt/Ps/V/Ma 一致（同一 θ 体系） |
| `Warning` | `"外推点: theta=%.1f° 超出当前扇区校准上限 %.1f°，计算 theta 使用 %.1f°；结果超出校准范围，有效性请用户自行判断"` | 必须非空，UI 据此显著标注外推点；文案中的上限和计算值必须来自当前扇区 |

只有在完整计算成功（角度、cpt/cps、Pt/Ps、V/Ma 均完成且通过既有物理守卫）时，越界结果才是
`IsValid=true`。仿射矩阵退化、无法确认外推方向、Pt/Ps 方程退化或速度/马赫数守卫失败时，返回 error，
由调用方按失败行处理，不承诺 `IsValid=true`。

> **θ_raw 与 θ_clamp 的取舍**：展示用原始外推 θ（诚实反映偏角已达 46°~70°），计算用当前扇区 `thetaMax+5°` 钳制值（系数外推有界）。

---

## 4. 与既有 spec 决策的关系

| 旧决策（spec-seven-hole-traversal.md） | 本规范 |
|---|---|
| §4：越界 → `IsValid=false`，不外推 | 确认是 θ 外边界越界且完整计算成功 → `IsValid=true` + Warning 标记；其他越界仍不可用 |
| Q3：θ>45° 外推"本期不实现" | **实现**（本规范） |

兼容性：正常网格内路径零改动；既有 golden 测试中的越界样本需按 §3.3 分类：确认属于 θ 外边界的样本更新为外推结果断言，其余样本保留 `IsValid=false` 断言。

---

## 5. Go 实现要点（组入代码）

### 5.1 新增文件与函数

**新文件** `shared/algorithms/go/sevenhole/interpolation/outer_extrapolation.go`：

```go
// outerZoneExtrapolate 在外区两候选扇区均未命中且确认 θ 外边界方向时，
// 对第一候选扇区 n 做角度外推：(ka,kb) → (θ,φ)，返回 θ_raw、θ_clamp、φ
// 和外推 cpt/cps；无法确认方向或发生退化时返回 error。
func (p *SevenHolePrbInterpolator) outerZoneExtrapolate(in InterpolationInput, sector int, ka, kb float64) (zoneCoefficients, float64, float64, error)
```

内部子函数（均为包内私有）：

| 函数 | 职责 |
|---|---|
| `outerExtrapInvertThetaPhi(sec *outerSector, ka, kb float64) (theta, phi float64, ok bool)` | §3.3 仿射反演，逐 φ 区间扫描选主区间 |
| `outerExtrapCptCps(sec *outerSector, theta, phi float64) (cpt, cps float64, err error)` | §3.5 外推双线性（复用 `outerThetaCellLo` 取最外层单元、`outerPhiCell` 取 φ 单元） |

### 5.2 `Calculate` 编排改动

`prb_interpolator.go` 的最终越界分支（现 `return InterpolationResult{IsValid:false, Warning:...}`）改为：

```go
// 越界：两候选扇区均未命中，且已确认 theta 外边界方向 → 外推第一候选扇区。
if sector, gp, ok := p.outerZoneExtrapolatePath(input, first); ok {
    // gp.a/gp.b 已含钳制后 θ、φ；组装结果，Warning 置外推标记
}
```

建议以独立方法 `outerZoneExtrapolatePath` 封装"取 (ka,kb) → 外推 → 组装"，保持 `Calculate` 主流程 <= 50 行（代码规范）。

### 5.3 边界与异常

| 场景 | 行为 |
|---|---|
| 全部 φ 区间 `|den|<1e-12` | 返回 error，由调用方按失败行处理 |
| `|d|=|1+cpt+cps|<1e-12` | 返回 error |
| `Pt < Ps` | 返回 error（沿用既有守卫，不因外推放宽） |
| `θ_raw` 远大于当前 `thetaMax+5` | 展示 θ_raw，计算用当前扇区 `thetaMax+5`；Warning 注明 θ_raw、扇区上限和计算值 |
| φ 越出扇区范围 | 不进入本 θ 外推路径，返回 `IsValid=false` + 非空 Warning；不得由坐标变换掩盖侧边越界 |

### 5.4 前端展示（probe-interpolator）

- 结果表中外推行按 `Warning` 非空渲染"外推"徽标/底色，提示用户自行判断。
- 不改变结果表列结构（Alpha/Beta/Pt/Ps/Ma/V 原样显示）。

---

## 6. 验证与验收

### 6.1 参考数据（实测，W532 85 m/s PRB + 102原始.csv 越界 12 行）

使用 §3 算法（标准网格 `thetaMax=45`，`theta_clamp=min(theta_raw,50)`）的计算结果：

| 行 | X | θ_raw | θ_clamp | φ | α | β | Pt | Ps | V | Ma |
|---|---|---|---|---|---|---|---|---|---|---|
| 50 | -12.25 | 47.4 | 47.4 | 120.5 | -43.2 | -28.9 | -627 | -683 | 10.1 | 0.029 |
| 51 | -12.50 | 51.8 | 50.0 | 117.5 | -46.6 | -28.8 | -637 | -690 | 9.8 | 0.028 |
| 52 | -12.75 | 45.9 | 45.9 | 128.6 | -38.8 | -32.7 | -666 | -725 | 10.3 | 0.030 |
| 53 | -13.00 | 56.4 | 50.0 | 114.7 | -47.3 | -26.4 | -642 | -693 | 9.6 | 0.028 |
| 54 | -13.25 | 61.8 | 50.0 | 110.5 | -48.2 | -22.6 | -656 | -702 | 9.1 | 0.026 |
| 55 | -13.50 | 57.0 | 50.0 | 113.5 | -47.5 | -25.4 | -682 | -725 | 8.7 | 0.025 |
| 56 | -13.75 | 64.6 | 50.0 | 109.9 | -48.2 | -22.1 | -692 | -730 | 8.3 | 0.024 |
| 57 | -14.00 | 67.9 | 50.0 | 107.5 | -48.7 | -19.7 | -693 | -726 | 7.8 | 0.023 |
| 58 | -14.25 | 65.7 | 50.0 | 109.5 | -48.3 | -21.7 | -706 | -741 | 8.0 | 0.023 |
| 59 | -14.50 | 66.6 | 50.0 | 109.1 | -48.4 | -21.3 | -717 | -745 | 7.2 | 0.021 |
| 60 | -14.75 | 67.4 | 50.0 | 105.7 | -48.9 | -17.8 | -706 | -735 | 7.1 | 0.021 |
| 61 | -15.00 | 69.6 | 50.0 | 104.9 | -49.0 | -17.0 | -700 | -727 | 7.0 | 0.020 |

> 观察：这些点 Pt/Ps 均为强负表压且非常接近（动压 27~59 Pa），V 仅 7~10 m/s。这是外推在远离校准区后的典型表现——**展示给用户，由用户判断数据合理性**，符合 §1.2 决策。

### 6.2 单元测试

| 用例 | 构造 | 期望 |
|---|---|---|
| 越界→外推 | 经 §3.3 确认为 θ>45 的 golden 越界点 | 返回 `IsValid=true` + 非空 Warning，θ_raw>45 |
| 外推精度 | 构造 θ=46°~50° 的合成 (ka,kb)，反演回 θ | `|θ_反演-θ_真值| < 0.5°`（线性化精度声明） |
| θ 钳制 | 标准网格 thetaMax=45，θ_raw=70 | θ_clamp=50，Warning 含 θ_raw、45 和 50 |
| 动态 θ 上限 | thetaCount=7，thetaMax=60，θ_raw=70 | θ_clamp=65，不得使用 50 |
| φ 侧边越界 | 构造 φ 超出扇区左右边界且 theta_raw<=thetaMax | 不进入 θ 外推，`IsValid=false`，Warning 不得声称 theta 超界 |
| 逆解候选 | 构造多个仿射候选，其中仅一个满足 θ 外边界和 φ 区间条件 | 选择通过方向/区间筛选且残差最小的候选 |
| 跨 0°/360° | sector 1，φ 接近 0°/360° | 使用连续展开坐标，结果 φ 归一化到 `[0,360)`，cpt/cps 不跳变 |
| 守卫 | 全区间 den≈0；d≈0 | 返回 error |
| 原正常路径 | golden 435 正常点 | 数值零回归（§6.3） |
| Warning 标记 | 任一外推点 | Warning 非空、含 θ 值，供 UI 渲染 |
| 外推计算失败 | θ 外推方向确认后 Pt/Ps 或 V/Ma 守卫失败 | 返回 error，不承诺 `IsValid=true` |

### 6.3 回归与更新

- **golden 测试更新**：当前仓库的 `golden.json` 仅包含 169 个 little 和 312 个 big 条目，`boundary.json` 仅包含合法网格边界条目，没有现成的 out 条目。新增真实越界夹具后，需按 §3.3 分类：确认 θ 外边界的点使用外推值断言，其他点保留 `IsValid=false` 断言。外推断言不要求与 Python `beyond_border` 数值一致（本算法已替换其硬编码 45° 行为）。
- **后端服务**：`seven_hole_service.go` 的 `CalculateSevenHole` / `BatchCalculateSevenHole` 无需改动（透传 result + Warning）；`BatchCalculate` 的失败行逻辑不变。
- **miniprogram 验证工具**（`probe-interpolator-miniprogram/verify/genref_seven.go`）需确认其断言不因 `Calculate` 行为变化而误报。
- 构建/校验：`go test ./...`（sevenhole 包 + 后端）+ `scripts/validate-structure.ps1` + 前端 `npm run typecheck`。

---

## 7. 风险与注意事项

1. **外推误差随 θ 增大而失控**：θ=70° 时 cpt/cps 由 45° 网格外推 25°，系数误差大 → Pt/Ps/V/Ma 可信度低。已通过当前扇区 `thetaMax+5` 限制计算端外推，但展示端 θ_raw 仍可达 70°，UI 必须显著标注。
2. **客户认知**：Warning 文案须明确"超出校准范围"，避免用户把外推值当实测值。
3. **不静默**：外推点禁止 `Warning=""`；`IsValid` 恒 true 仅针对能算出数值的越界点。
4. **φ 侧边越界不属于本方案**：坐标变换虽然数学上仍可计算，但会把侧向越界误报为 θ 外推，因此本方案返回不可用结果并要求用户修正数据或校准范围。
5. **动态上限**：`thetaExtrapMax` 必须按扇区网格推导为 `thetaMax+5`，不得硬编码 50；不同扇区允许使用不同 thetaCount。

---

## 8. 待评审确认事项

| 编号 | 事项 | 现状与选项 |
|---|---|---|
| Q1 | θ_raw 展示 vs 钳制 | 已确定：展示 θ_raw，计算使用当前扇区 `thetaMax+5` |
| Q2 | `thetaExtrapMax` 来源 | 已确定：由当前扇区最后一个 θ 网格点推导，非硬编码 |
| Q3 | Warning 是否含 φ | 已确定：θ 外推 Warning 含 θ_raw/上限/计算值；φ 侧边越界返回不可用 Warning，不得声称 θ 超界 |
| Q4 | 是否写 CSV 外推标记列 | 默认不新增列（Warning 已足够）；如需按行标记另议 |
| Q5 | 外推误差接受标准 | 已确定：θ=46°~50° 反演误差 <0.5°；超过上限只展示原始 θ，计算使用钳制值 |

---

## 附录 A：与 Python `beyond_border` 的差异

| 项 | Python `beyond_border`（SKILL.md §3.8） | 本规范 |
|---|---|---|
| θ | **硬编码 45°**（不真外推） | 仿射反演 + 钳制，θ 可达 45°~50°（计算）/ 70°（展示） |
| φ 越界 | 落回扇区中心角 | 仿射反演解析 φ，不落回中心 |
| cpt/cps | 45° 单元双线性（含退化风险） | 最外层单元 θ 方向线性外推 |
| 错误处理 | try-except 吞错返回 None | 显式 error |
