# Spec: 五孔探针校准 α 轴角度换算开关（angleConvert）

## Objective

五孔探针自动校准模块新增一个配置开关 `angleConvert`。开关打开时，点位网格的 **α 轴运动目标角** 按公式

```
α' = arctan( tan(α) · cos(β) )
```

换算，β 轴目标角不变。预览布点、实际走点、CSV 落盘记录**全部使用换算后的坐标**；开关关闭时行为与现状完全一致（向后兼容）。

**遍历顺序约束（R-Order）**：本模式下网格遍历顺序固定为 **β 慢轴（外层循环）、α 快轴（内层循环）**——β 动一步，α 走一整圈，再步进 β、α 再走一圈。当前 `GenerateFiveHoleSnakePoints` 已是此结构（外层 `for bi`（β）→ 内层 `for ai`（α）），本 spec 将其作为显式约束锁定并加测试，防止后续改动破坏。

**背景**：五孔探针安装在两轴运动台上，当探针绕 β 轴偏航后再绕 α 轴俯仰时，探针自身测量平面内实际得到的迎角会因 cos(β) 因子衰减。打开本开关后，运动台按换算角走点，保证物理布点与配置的逻辑网格一致。

**验收用户故事**：配置向导勾选"α 角度换算"→ 预览点位中 α 列显示换算后角度（如 α=30°, β=30° 显示 26.6°）→ 启动校准后运动台实际走到换算角 → CSV 中 α 列也是换算角。

## Tech Stack

- 后端：Go（hexagonal，`services/api-go`），核心算法在 `internal/core/calibration`
- 前端：Vue 3 + Vite + Pinia + Vitest（Wails v3 桌面壳）
- 共享类型：`apps/desktop-wails/frontend/src/shared/types/calibration.ts`
- Wails binding：`FiveHolePointLayoutDTO` 直接是 `calibration.FiveHolePointLayout` 别名，无需手改

## Commands

在 `projects/WindLabX4` 下执行（后端需 `GOWORK=off`，见 workspace AGENTS.md）：

```bash
# 后端测试（核心/用例/API）
cd services/api-go && go test ./internal/core/calibration/... ./internal/usecase/... ./api/...
# 后端全量 + 构建
cd services/api-go && go test ./internal/... ./api/... && go build -buildvcs=false ./...
# 前端类型检查 / 构建 / 单测
cd apps/desktop-wails/frontend && npm run typecheck
cd apps/desktop-wails/frontend && npm run build
cd apps/desktop-wails/frontend && npm run test
# 结构校验（改动含 Go 文件时）
powershell -File scripts/validate-structure.ps1
# Wails binding 再生成（FiveHolePointLayout 加字段后模型定义变化；DTO 是别名无需手改，
# 但 frontend/bindings 下 models 是生成物，需刷新避免与实际字段漂移）
cd apps/desktop-wails && go run github.com/wailsapp/wails/v3/cmd/wails3 generate bindings -silent
```

## Project Structure

| 路径 | 改动 | 说明 |
|---|---|---|
| `services/api-go/internal/core/calibration/five_hole.go` | 修改 | `FiveHolePointLayout` 加 `AngleConvert bool`；`GenerateFiveHoleSnakePoints` 在 `layout.AngleConvert` 时换算 α；新增换算辅助函数 |
| `services/api-go/internal/core/calibration/five_hole_layout_test.go` | 修改 | 黄金用例单测 |
| `services/api-go/internal/usecase/calibration.go` | 不改 | `PreviewFiveHolePoints` 已透传 layout，自动生效 |
| `services/api-go/api/server.go` | 不改 | `handleFiveHolePreview` 已透传 layout |
| `apps/desktop-wails/backend/app.go` + `types` | 不改 | DTO 是别名，字段自动透传 |
| `apps/desktop-wails/frontend/src/shared/types/calibration.ts` | 修改 | `FiveHolePointLayout` 加 `angleConvert?: boolean` |
| `apps/desktop-wails/frontend/src/api/calibrationApi.ts` | 修改 | `generateFiveHoleSnakePoints` 入参是**内联类型**（非 `FiveHolePointLayout` 引用），需同步补 `angleConvert?: boolean`，保证类型契约完整 |
| `apps/desktop-wails/frontend/src/components/calibration/five-hole/FiveHoleSettings.vue` | 修改 | `pointLayout` ref、`sanitizePointLayout`、`loadSavedConfig` 透传开关；UI 加 checkbox |
| `apps/desktop-wails/frontend/src/components/calibration/five-hole/__tests__/motionCalibrationUtils.test.ts` | 不改 | layout 为透传对象，现有测试不受影响 |

## Code Style

核心换算（新函数，放在 `five_hole.go`，`GenerateFiveHoleSnakePoints` 上方）：

```go
// convertFiveHoleAlpha 按 angleConvert 开关把 α 轴角度换算为运动台实际目标角。
// 公式：α' = arctan(tan(α)·cos(β))（角度制输入输出），β 轴不变。
// 来源：笛卡尔坐标系下两轴旋转（先俯仰 α、再横滚 β）在俯仰平面内的投影角。
func convertFiveHoleAlpha(alphaDeg, betaDeg float64) float64 {
	alphaRad := alphaDeg * math.Pi / 180.0
	betaRad := betaDeg * math.Pi / 180.0
	return math.Atan(math.Tan(alphaRad)*math.Cos(betaRad)) * 180.0 / math.Pi
}
```

在 `GenerateFiveHoleSnakePoints` 内生成 α 后：

```go
alpha := roundTo1Decimal(layout.AlphaMin + float64(alphaIdx)*layout.AlphaStep)
if layout.AngleConvert {
    alpha = roundTo1Decimal(convertFiveHoleAlpha(alpha, beta))
}
```

约定：
- 舍入统一**复用同包 `roundTo1Decimal`**（seven_hole.go），不新写内联 `math.Round(x*10)/10`；顺带把 `GenerateFiveHoleSnakePoints` 现有两处内联 round 统一为该 helper（行为等价，非重构）
- **前置校验（防呆，α/β 双轴同构）**：`AngleConvert=true` 时若 `|AlphaMin| ≥ 90`、`|AlphaMax| ≥ 90`、`|BetaMin| ≥ 90`、`|BetaMax| ≥ 90` 任一成立，返回明确错误——`tan(±90°)` 发散，`|α|>90°` 时 tan 变号导致换算角符号翻转（点位跨象限错乱）；`cosβ` 在 ±90° 退化（β=±90° 使整行 α' 退化为 0，点位重叠落盘），`|β|>90°` 时 α' 符号翻转。开关关闭时同区间仍合法（现状不变）
- 只改 α 值，β 与 `Coordinates` map 键名（`"α"`/`"β"`）不变
- **遍历顺序不动**：`AngleConvert=true` 时仍保持外层 β、内层 α（β 慢轴、α 快轴），不因开关改变循环结构
- 注释用中文，说明公式来源（对应 `docs/specs/spec-five-hole-angle-convert.md`）
- 不做多余重构、不动无关代码（遵循 Karpathy 精准修改准则）

## Testing Strategy

框架：Go 标准库 `testing`（后端）+ Vitest（前端）。

后端（`five_hole_layout_test.go` 追加）：
- `TestConvertFiveHoleAlpha`：辅助函数单元测试，覆盖黄金用例
- `TestGenerateFiveHoleSnakePointsAngleConvert`：`AngleConvert=true` 时点位 α 为换算值、β 不变、ID/顺序不变
- `TestGenerateFiveHoleSnakePointsAngleConvertOff`：默认（false）输出与现状逐点一致（回归）
- `TestGenerateFiveHoleSnakePointsAngleConvert_TraversalOrderBetaSlowAlphaFast`：`AngleConvert=true` 时**遍历顺序锁定为 β 慢轴/α 快轴**——β 递增（外层），每个 β 值内 α 覆盖全区间（raster 升序，serpentine 奇数行反向），β 步进发生在整行 α 扫完之后
- `TestGenerateFiveHoleSnakePointsAngleConvert_AngleRangeGuard`：`AngleConvert=true` 且 α 或 β 范围触及 ±90° 时返回明确错误（错误信息含对应轴名）、不生成点位；开关关闭时同区间合法
- 补 `usecase`/API 层各一条：`PreviewFiveHolePoints`（calibration_five_hole_preview_test.go）+ `handleFiveHolePreview`（server_five_hole_preview_test.go）带 `angleConvert=true` 返回换算坐标

黄金用例（单位：度，换算值 round 到 1 位小数）：

| α | β | α' = atan(tanα·cosβ) | α' (1 位小数) |
|---|---|---|---|
| 30 | 30 | 26.5651 | 26.6 |
| 30 | 25 | 27.6211 | 27.6 |
| 25 | 30 | 21.9905 | 22.0 |
| 30 | 10 | 29.6217 | 29.6 |
| 30 | 0  | 30.0000 | 30.0 |
| 0  | 30 | 0.0000 | 0.0 |
| -30 | 30 | -26.5651 | -26.6 |

边界：`β=0` 换算不改变 α；`α=0` 换算恒为 0；负数对称。

断言精度约定：黄金表 **"α' (1 位小数)" 列为断言基准**；高精度列仅供推导参照（第 4 位小数存在 ±0.001 量级噪声，禁止做 exact 断言）。测试侧独立计算期望值后同样 `roundTo1Decimal` 再比较——对齐 `seven_hole_points_test.go` 既有模式。

前端：
- `FiveHoleSettings.vue` 开关勾选后 `sanitizePointLayout` 保留 `angleConvert`，保存/加载往返一致（现有组件测试模式沿用）

## Boundaries

- **Always**：
  - 改完跑 `go test ./internal/core/calibration/... ./internal/usecase/... ./api/...` 与 `npm run typecheck`
  - 换算值保留 1 位小数；`angleConvert` 默认 false
  - 新字段加 `omitempty`，旧配置文件/旧请求体（无该字段）按 false 处理
- **Ask first**：
  - 修改 Wails binding 签名（本任务不需要——DTO 是别名）
  - 改动 CSV schema 表头（本任务不改，只换 α 列数值）
  - 调整舍入精度或改为双坐标模型（`MotionCoordinates`）
- **Never**：
  - 改动 `core/` 之外的领域逻辑；不引入硬件/文件 IO 到 `core/`
  - 改动七孔/三孔/总压/总温模块代码
  - 提交前不跑 `gitnexus_detect_changes()` 核对影响面

## Success Criteria

- [ ] `FiveHolePointLayout` 增加 `angleConvert`（默认 false），前后端字段名与 JSON tag 一致
- [ ] `AngleConvert=true` 时，`GenerateFiveHoleSnakePoints` 返回的每个点 `Coordinates["α"] = round1(atan(tanα·cosβ))`，`β` 不变；黄金用例全通过
- [ ] `AngleConvert=true` 时遍历顺序为 β 慢轴/α 快轴（外层 β 递增、每行内 α 全区间），有专项测试锁定
- [ ] `AngleConvert=true` 且 α/β 范围触及 ±90° 时返回明确错误，不生成点位
- [ ] `AngleConvert=false` 或字段缺失时，输出与改动前逐点一致（回归测试通过）
- [ ] 前端配置向导出现开关（默认关闭），勾选后保存/加载往返不丢，预览返回换算坐标
- [ ] 走点链路自动生效：`moveToPoint` 读 `point.Coordinates`，无需后端其他改动
- [ ] `go test ./internal/... ./api/...`、`npm run typecheck`、`npm run build` 全绿
- [ ] `gitnexus_detect_changes()` 确认受影响符号仅限五孔布局与设置页

## Open Questions

- 换算值的显示精度已定为 1 位小数（与现有点位一致）；如需 4 位精度（与表格原始值一致）需另行确认。
- 前端图表（Kα/Kβ 散点等）坐标随 CSV 用换算值——符合"全用换算值"选择；如需逻辑坐标对照需改双坐标模型。
