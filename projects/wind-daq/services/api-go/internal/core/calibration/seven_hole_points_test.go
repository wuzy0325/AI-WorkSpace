package calibration

import (
	"math"
	"sort"
	"testing"
)

// ==================== 七孔探针点位生成测试（spec Task 6 / §3.3 / §6.2 / §9.1） ====================
//
// 测试覆盖 spec Task 6 验收标准的全部要求：
//   - 点数：完整模式 715 点（169 内区 + 546 外区）；数据集模式 481 点（169 内区 + 312 外区）
//   - 双坐标：外区 MotionCoordinates 与 Coordinates 通过 §3.3 正向公式换算一致
//   - 黄金用例 G1~G5：覆盖 φ=0°/90°/180°/270°/330° 五个方位，验证 α 公式负号正确性
//   - 蛇形顺序：奇数行 α/θ 反向
//   - 步长校验：≤0 返回错误；min > max 返回错误
//   - ID 唯一性：内/外区 ID 不冲突，从 1 递增到 N
//   - 扇区映射：φ → Sector 1~6 按 60° 等分
//
// 优先级标签（便于时间紧急时优先执行核心测试）：
//   - P0：核心功能（点数、黄金用例、双坐标）——必须全部通过
//   - P1：重要约束（ID 唯一、蛇形、扇区映射）——影响数据正确性
//   - P2：边界与默认值（步长校验、空 Mode）——防御性测试

// sevenHoleBuildFullConfig 构造完整模式默认配置（spec §6.2 推荐值）
//
// 内区 α/β ∈ [-30°, 30°] 步长 5° → 13×13 = 169 点
// 外区 θ ∈ [30°, 60°] 步长 5° × 6 扇区 × 13 φ = 546 点
// 总计 715 点
func sevenHoleBuildFullConfig(serpentine bool) SevenHoleConfig {
	return SevenHoleConfig{
		Mode:           SevenHoleModeFull,
		InnerAlphaMin:  -30.0,
		InnerAlphaMax:  30.0,
		InnerAlphaStep: 5.0,
		InnerBetaMin:   -30.0,
		InnerBetaMax:   30.0,
		InnerBetaStep:  5.0,
		OuterThetaMin:  30.0,
		OuterThetaMax:  60.0,
		OuterThetaStep: 5.0,
		OuterPhiMin:    0.0,
		OuterPhiMax:    355.0,
		OuterPhiStep:   5.0,
		Serpentine:     serpentine,
	}
}

// sevenHoleBuildDatasetConfig 构造数据集模式配置（spec §6.2 验证基准）
//
// 内区 169 点同完整模式
// 外区 θ ∈ {30°, 35°, 40°, 45°}（4 值，忽略 config 的 OuterTheta 范围）
//
//	× 每扇区 φ 跨 60° 步长 5° = 13 点 × 6 扇区 = 312 点
//
// 总计 481 点
//
// 注意：数据集模式下 OuterTheta/OuterPhi 字段被忽略，但为可读性仍填写推荐值
func sevenHoleBuildDatasetConfig(serpentine bool) SevenHoleConfig {
	cfg := sevenHoleBuildFullConfig(serpentine)
	cfg.Mode = SevenHoleModeDataset
	return cfg
}

// ==================== P0 核心测试：点数与黄金用例 ====================

// TestGenerateSevenHolePointsFullMode 【P0】验证完整模式生成 715 点（169 内区 + 546 外区）
//
// 测试前置：构造完整模式默认配置（内区 [-30°,30°] 步长 5°，外区 θ[30°,60°] 步长 5° × φ[0°,355°] 步长 5°）
// 测试步骤：调用 GenerateSevenHolePoints
// 期待结果：
//   - 总点数 = 715
//   - 内区点数 = 169（Region="inner", Sector=7）
//   - 外区点数 = 546（Region="outer", Sector ∈ {1..6}）
func TestGenerateSevenHolePointsFullMode(t *testing.T) {
	points, err := GenerateSevenHolePoints(sevenHoleBuildFullConfig(false))
	if err != nil {
		t.Fatalf("完整模式点位生成失败: %v", err)
	}

	if len(points) != 715 {
		t.Errorf("完整模式总点数应为 715, 实际 %d", len(points))
	}

	// 统计内/外区点数
	innerCount, outerCount := 0, 0
	for _, p := range points {
		switch p.Region {
		case "inner":
			innerCount++
			if p.Sector != 7 {
				t.Errorf("内区点 Sector 应为 7, 点 ID=%d 实际 Sector=%d", p.ID, p.Sector)
			}
		case "outer":
			outerCount++
			if p.Sector < 1 || p.Sector > 6 {
				t.Errorf("外区点 Sector 应为 1..6, 点 ID=%d 实际 Sector=%d", p.ID, p.Sector)
			}
		default:
			t.Errorf("点 ID=%d Region 应为 inner/outer, 实际 %q", p.ID, p.Region)
		}
	}

	if innerCount != 169 {
		t.Errorf("内区点数应为 169, 实际 %d", innerCount)
	}
	if outerCount != 546 {
		t.Errorf("外区点数应为 546, 实际 %d", outerCount)
	}
}

func TestGenerateSevenHolePointsFullModeUsesClosedSectorGrids(t *testing.T) {
	points, err := GenerateSevenHolePoints(sevenHoleBuildFullConfig(false))
	if err != nil {
		t.Fatalf("完整模式点位生成失败: %v", err)
	}

	sectorOnePhi := make([]float64, 0, 13)
	sectorCounts := make(map[int]int, 6)
	for _, point := range points {
		if point.Region != "outer" {
			continue
		}
		sectorCounts[point.Sector]++
		if point.Sector == 1 && point.Coordinates["θ"] == 30 {
			sectorOnePhi = append(sectorOnePhi, point.Coordinates["φ"])
		}
	}

	wantPhi := []float64{330, 335, 340, 345, 350, 355, 0, 5, 10, 15, 20, 25, 30}
	if len(sectorOnePhi) != len(wantPhi) {
		t.Fatalf("完整模式 1 区每个 θ 应有 13 个 φ 点, 实际 %d: %v", len(sectorOnePhi), sectorOnePhi)
	}
	for i, want := range wantPhi {
		if sectorOnePhi[i] != want {
			t.Errorf("完整模式 1 区第 %d 个 φ 应为 %.1f, 实际 %.1f", i, want, sectorOnePhi[i])
		}
	}
	for sector := 1; sector <= 6; sector++ {
		if sectorCounts[sector] != 91 {
			t.Errorf("完整模式 %d 区应有 91 点 (7 θ × 13 φ), 实际 %d", sector, sectorCounts[sector])
		}
	}
}

func TestGenerateSevenHolePointsFullModeMatchesFourThetaReferenceLayout(t *testing.T) {
	config := sevenHoleBuildFullConfig(false)
	config.OuterThetaMax = 45

	points, err := GenerateSevenHolePoints(config)
	if err != nil {
		t.Fatalf("完整模式点位生成失败: %v", err)
	}
	if len(points) != 481 {
		t.Fatalf("四层 θ 的标准布局应有 481 点 (169+312), 实际 %d", len(points))
	}

	sectorCounts := make(map[int]int, 6)
	for _, point := range points {
		if point.Region == "outer" {
			sectorCounts[point.Sector]++
		}
	}
	for sector := 1; sector <= 6; sector++ {
		if sectorCounts[sector] != 52 {
			t.Errorf("标准布局 %d 区应有 52 点, 实际 %d", sector, sectorCounts[sector])
		}
	}
}

func TestGenerateSevenHolePointsCompensatesAlphaMotionDirection(t *testing.T) {
	innerConfig := SevenHoleConfig{
		Mode:          SevenHoleModeFull,
		InnerAlphaMin: 30, InnerAlphaMax: 30, InnerAlphaStep: 5,
		InnerBetaMin: 0, InnerBetaMax: 0, InnerBetaStep: 5,
		OuterThetaMin: 30, OuterThetaMax: 30, OuterThetaStep: 5,
		OuterPhiMin: 90, OuterPhiMax: 90, OuterPhiStep: 5,
	}
	points, err := GenerateSevenHolePoints(innerConfig)
	if err != nil {
		t.Fatalf("点位生成失败: %v", err)
	}
	if len(points) != 3 {
		t.Fatalf("φ=90° 边界应生成 1 个内区点和 2 个相邻扇区外区点, 实际 %d", len(points))
	}
	if points[1].Sector != 2 || points[2].Sector != 3 {
		t.Fatalf("φ=90° 边界外区扇区应为 2 和 3, 实际 %d 和 %d", points[1].Sector, points[2].Sector)
	}

	if got := points[0].MotionCoordinates["α"]; got != -30 {
		t.Errorf("内区逻辑 α=30° 时物理 α 应为 -30°, 实际 %.1f", got)
	}
	outer := points[1]
	if got := outer.MotionCoordinates["α"]; got != 30 {
		t.Errorf("外区 θ=30°/φ=90° 时物理 α 应为 30°, 实际 %.1f", got)
	}
}

// TestGenerateSevenHolePointsDatasetMode 【P0】验证数据集模式生成 481 点（169 内区 + 312 外区）
//
// 测试前置：构造数据集模式配置
// 测试步骤：调用 GenerateSevenHolePoints
// 期待结果：
//   - 总点数 = 481
//   - 内区点数 = 169
//   - 外区点数 = 312（4 个 θ × 13 个 φ × 6 个扇区）
//   - 外区 θ 取值仅 {30°, 35°, 40°, 45°}
func TestGenerateSevenHolePointsDatasetMode(t *testing.T) {
	points, err := GenerateSevenHolePoints(sevenHoleBuildDatasetConfig(false))
	if err != nil {
		t.Fatalf("数据集模式点位生成失败: %v", err)
	}

	if len(points) != 481 {
		t.Errorf("数据集模式总点数应为 481, 实际 %d", len(points))
	}

	innerCount, outerCount := 0, 0
	thetaSet := make(map[float64]bool)
	for _, p := range points {
		switch p.Region {
		case "inner":
			innerCount++
		case "outer":
			outerCount++
			thetaSet[p.Coordinates["θ"]] = true
		}
	}

	if innerCount != 169 {
		t.Errorf("内区点数应为 169, 实际 %d", innerCount)
	}
	if outerCount != 312 {
		t.Errorf("外区点数应为 312, 实际 %d", outerCount)
	}

	// 验证外区 θ 取值仅 {30°, 35°, 40°, 45°}
	expectedThetas := map[float64]bool{30.0: true, 35.0: true, 40.0: true, 45.0: true}
	if len(thetaSet) != len(expectedThetas) {
		t.Errorf("外区 θ 取值种类应为 %d, 实际 %d (%v)", len(expectedThetas), len(thetaSet), thetaSet)
	}
	for theta := range thetaSet {
		if !expectedThetas[theta] {
			t.Errorf("外区出现非预期 θ 值 %.1f (应仅 {30,35,40,45})", theta)
		}
	}
}

// TestSevenHoleGoldenCases 【P0】验证 spec §3.3 黄金用例 G1~G5
//
// 这组用例覆盖 φ=0°/90°/180°/270° 四个主轴方位 + φ=330° 数据集验证点，
// 用于捕获 α 公式负号被误删的回归——若负号丢失，G2/G4 的 α 符号会反转。
//
// 测试前置：构造单 φ 完整模式配置（边界 φ 会复制到两个相邻扇区）
// 测试步骤：调用 GenerateSevenHolePoints，检查外区点数、扇区与换算结果
// 期待结果：MotionCoordinates 的 (α, β) 与 spec §3.3 表中正向输出一致（误差 < 0.1°）
func TestSevenHoleGoldenCases(t *testing.T) {
	cases := []struct {
		name          string
		theta, phi    float64
		expectAlpha   float64
		expectBeta    float64
		expectSectors []int
	}{
		{"G1_φ0°_P1", 30.0, 0.0, 0.0, 30.0, []int{1}},
		{"G2_φ90°_P2P3", 30.0, 90.0, 30.0, 0.0, []int{2, 3}},
		{"G3_φ180°_P4", 30.0, 180.0, 0.0, -30.0, []int{4}},
		{"G4_φ270°_P5P6", 30.0, 270.0, -30.0, 0.0, []int{5, 6}},
		{"G5_φ330°_P1P6", 30.0, 330.0, -16.1, 26.6, []int{1, 6}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			// 构造仅含一个外区点的配置（θ/φ 范围收紧到目标点）
			cfg := SevenHoleConfig{
				Mode:          SevenHoleModeFull,
				InnerAlphaMin: 0.0, InnerAlphaMax: 0.0, InnerAlphaStep: 1.0,
				InnerBetaMin: 0.0, InnerBetaMax: 0.0, InnerBetaStep: 1.0,
				OuterThetaMin: c.theta, OuterThetaMax: c.theta, OuterThetaStep: 1.0,
				OuterPhiMin: c.phi, OuterPhiMax: c.phi, OuterPhiStep: 1.0,
				Serpentine: false,
			}
			points, err := GenerateSevenHolePoints(cfg)
			if err != nil {
				t.Fatalf("用例 %s 点位生成失败: %v", c.name, err)
			}
			wantCount := 1 + len(c.expectSectors)
			if len(points) != wantCount {
				t.Fatalf("用例 %s 应生成 %d 个点（边界 φ 双扇区复制）, 实际 %d", c.name, wantCount, len(points))
			}
			for i, sector := range c.expectSectors {
				outerPoint := points[i+1]
				if outerPoint.Region != "outer" || outerPoint.Sector != sector {
					t.Fatalf("用例 %s 第 %d 个外区点应属于扇区 %d, 实际 Region=%s Sector=%d",
						c.name, i+1, sector, outerPoint.Region, outerPoint.Sector)
				}
			}
			outer := points[1]

			alpha := outer.MotionCoordinates["α"]
			beta := outer.MotionCoordinates["β"]

			if math.Abs(alpha-c.expectAlpha) > 0.1 {
				t.Errorf("用例 %s α 应为 %.1f°, 实际 %.4f° (差异 %.4f°)",
					c.name, c.expectAlpha, alpha, math.Abs(alpha-c.expectAlpha))
			}
			if math.Abs(beta-c.expectBeta) > 0.1 {
				t.Errorf("用例 %s β 应为 %.1f°, 实际 %.4f° (差异 %.4f°)",
					c.name, c.expectBeta, beta, math.Abs(beta-c.expectBeta))
			}

			// 反向回算前恢复算法坐标系的 α 符号。
			thetaBack, phiBack := ConvertAlphaBetaToThetaPhi(-alpha, beta)
			if math.Abs(thetaBack-c.theta) > 0.1 {
				t.Errorf("用例 %s 反向 θ 应为 %.1f°, 实际 %.4f°", c.name, c.theta, thetaBack)
			}
			if math.Abs(phiBack-c.phi) > 0.1 {
				t.Errorf("用例 %s 反向 φ 应为 %.1f°, 实际 %.4f°", c.name, c.phi, phiBack)
			}
		})
	}
}

// TestSevenHoleAlphaSign 【P0】α 公式负号保护测试（spec Task 15 Verification）
//
// spec §3.3 Critical 1 要求：α = -arctan(tanθ × sinφ) 中的负号必须保留。
// 若负号被误删，会导致外区运动控制器下发的 X 轴方向相反——来流偏 +X（φ=270°）时
// 探针反而向 -X 运动，物理上完全错误。
//
// 此测试聚焦 G2/G4 两个精确符号用例（α=±30°, β=0°，无 round 误差），用 0.01° 容差
// 严格验证负号存在——若 seven_hole_formulas.go L391 的负号丢失，本测试必须失败。
//
// 验证维度：
//  1. 正向换算：φ=90° → α=-30°；φ=270° → α=+30°（直接断言符号）
//  2. 反向回算：α=-30°,β=0° → φ=90°；α=+30°,β=0° → φ=270°（验证正向+反向负号一致）
//  3. 对称性：φ=90° 与 φ=270° 的 α 应符号相反、绝对值相等
//
// 测试前置：无（直接调用 ConvertThetaPhiToAlphaBeta / ConvertAlphaBetaToThetaPhi 纯函数）
// 测试步骤：依次验证 G2/G4 的正向+反向+对称性
// 期待结果：所有断言通过；若负号丢失，正向断言与反向回算 φ 值都会失败
func TestSevenHoleAlphaSign(t *testing.T) {
	// 0.01° 容差：G2/G4 期望值是 ±30°/0° 精确值，无 round 误差
	const alphaSignEpsilon = 1e-2

	// ==================== G2: φ=90° → α=-30°, β=0° ====================
	// 物理含义：φ=90° 对应 -X 方位（spec §3.3 表），来流偏 -X → α=-30°
	// 若误删负号：α=+30°，与 -X 方位矛盾，本断言失败
	alphaG2, betaG2 := ConvertThetaPhiToAlphaBeta(30.0, 90.0)
	if math.Abs(alphaG2-(-30.0)) > alphaSignEpsilon {
		t.Errorf("G2 正向 α 应为 -30° (φ=90° 来流偏 -X), 实际 %.4f° — 检查 seven_hole_formulas.go 中 α 公式负号是否被误删",
			alphaG2)
	}
	if math.Abs(betaG2-0.0) > alphaSignEpsilon {
		t.Errorf("G2 正向 β 应为 0°, 实际 %.4f°", betaG2)
	}

	// 反向回算：α=-30°, β=0° → φ=90°
	// 若正向公式负号丢失但反向公式负号保留，正反换算会不一致——本断言失败
	thetaBackG2, phiBackG2 := ConvertAlphaBetaToThetaPhi(-30.0, 0.0)
	if math.Abs(thetaBackG2-30.0) > alphaSignEpsilon {
		t.Errorf("G2 反向 θ 应为 30°, 实际 %.4f°", thetaBackG2)
	}
	if math.Abs(phiBackG2-90.0) > alphaSignEpsilon {
		t.Errorf("G2 反向 φ 应为 90° (α=-30° 来流偏 -X), 实际 %.4f° — 正向/反向负号可能不一致",
			phiBackG2)
	}

	// ==================== G4: φ=270° → α=+30°, β=0° ====================
	// 物理含义：φ=270° 对应 +X 方位，来流偏 +X → α=+30°
	// 若误删负号：α=-30°，与 +X 方位矛盾，本断言失败
	alphaG4, betaG4 := ConvertThetaPhiToAlphaBeta(30.0, 270.0)
	if math.Abs(alphaG4-30.0) > alphaSignEpsilon {
		t.Errorf("G4 正向 α 应为 +30° (φ=270° 来流偏 +X), 实际 %.4f° — 检查 α 公式负号是否被误删",
			alphaG4)
	}
	if math.Abs(betaG4-0.0) > alphaSignEpsilon {
		t.Errorf("G4 正向 β 应为 0°, 实际 %.4f°", betaG4)
	}

	// 反向回算：α=+30°, β=0° → φ=270°
	thetaBackG4, phiBackG4 := ConvertAlphaBetaToThetaPhi(30.0, 0.0)
	if math.Abs(thetaBackG4-30.0) > alphaSignEpsilon {
		t.Errorf("G4 反向 θ 应为 30°, 实际 %.4f°", thetaBackG4)
	}
	if math.Abs(phiBackG4-270.0) > alphaSignEpsilon {
		t.Errorf("G4 反向 φ 应为 270° (α=+30° 来流偏 +X), 实际 %.4f° — 正向/反向负号可能不一致",
			phiBackG4)
	}

	// ==================== 对称性验证 ====================
	// φ=90° 与 φ=270° 相差 180°，sin(φ) 符号反转，α 应符号相反、绝对值相等
	// 若负号丢失，两者符号会同时反转，对称性仍成立——此断言主要捕获"绝对值不对称"的其他 bug
	if math.Abs(math.Abs(alphaG2)-math.Abs(alphaG4)) > alphaSignEpsilon {
		t.Errorf("G2/G4 α 绝对值应相等 (对称性), 实际 |αG2|=%.4f |αG4|=%.4f",
			math.Abs(alphaG2), math.Abs(alphaG4))
	}
	if (alphaG2 > 0) == (alphaG4 > 0) {
		t.Errorf("G2/G4 α 符号应相反 (φ 相差 180° → sin 符号反转), 实际 αG2=%.4f αG4=%.4f",
			alphaG2, alphaG4)
	}
}

// TestSevenHoleDualCoordinates 【P0】验证外区点双坐标一致性
//
// spec §3.4 双坐标模型要求：外区点 MotionCoordinates 必须等于按 §3.3 正向公式
// 从 Coordinates(θ,φ) 换算出的 (α',β')。这是运动控制器下发角度的正确性保证。
//
// spec Task 6 同时要求"浮点 round 到 1 位小数"，因此实现层对 MotionCoordinates
// 做了 roundTo1Decimal 处理。测试侧对独立换算结果同样 round 后再比较，避免
// 把 spec 允许的 round 误差误判为不一致。
//
// 测试前置：构造完整模式默认配置
// 测试步骤：
//   - 调用 GenerateSevenHolePoints
//   - 遍历所有外区点，对每个点用 ConvertThetaPhiToAlphaBeta 独立换算并 round 到 1 位小数
//
// 期待结果：每个外区点的 MotionCoordinates 与"独立换算 + round"结果完全一致
func TestSevenHoleDualCoordinates(t *testing.T) {
	points, err := GenerateSevenHolePoints(sevenHoleBuildFullConfig(false))
	if err != nil {
		t.Fatalf("点位生成失败: %v", err)
	}

	for _, p := range points {
		if p.Region != "outer" {
			continue
		}
		theta := p.Coordinates["θ"]
		phi := p.Coordinates["φ"]
		rawAlpha, rawBeta := ConvertThetaPhiToAlphaBeta(theta, phi)
		// 与实现保持一致：换算后 round 到 1 位小数（spec Task 6）
		expectAlpha := roundTo1Decimal(-rawAlpha)
		expectBeta := roundTo1Decimal(rawBeta)

		actualAlpha := p.MotionCoordinates["α"]
		actualBeta := p.MotionCoordinates["β"]

		if math.Abs(actualAlpha-expectAlpha) > epsilon {
			t.Errorf("外区点 ID=%d (θ=%.1f,φ=%.1f) α 应为 %.4f, 实际 %.4f",
				p.ID, theta, phi, expectAlpha, actualAlpha)
		}
		if math.Abs(actualBeta-expectBeta) > epsilon {
			t.Errorf("外区点 ID=%d (θ=%.1f,φ=%.1f) β 应为 %.4f, 实际 %.4f",
				p.ID, theta, phi, expectBeta, actualBeta)
		}
	}
}

// ==================== P1 重要约束测试 ====================

// TestGenerateSevenHolePoints_IDUniqueness 【P1】验证 ID 全局唯一且从 1 递增
//
// 防止 Task 6.1 实现中"子生成器各自从 1 编号、合并未重编号"的回归——
// 该 bug 会导致内区 ID=1..169 与外区 ID=1..546 冲突。
//
// 测试前置：构造完整模式默认配置
// 测试步骤：调用 GenerateSevenHolePoints，收集所有 ID 并排序
// 期待结果：
//   - ID 数量 = 总点数（无重复）
//   - 排序后第 1 个 ID = 1，最后一个 ID = 总点数
//   - 内区最后一个点 ID=169，外区第一个点 ID=170（连续衔接）
func TestGenerateSevenHolePoints_IDUniqueness(t *testing.T) {
	points, err := GenerateSevenHolePoints(sevenHoleBuildFullConfig(false))
	if err != nil {
		t.Fatalf("点位生成失败: %v", err)
	}

	ids := make(map[int]bool)
	for _, p := range points {
		if ids[p.ID] {
			t.Errorf("ID %d 重复出现（内/外区 ID 冲突）", p.ID)
		}
		ids[p.ID] = true
	}
	if len(ids) != len(points) {
		t.Errorf("ID 唯一性失败: 总点数 %d, 唯一 ID 数 %d", len(points), len(ids))
	}

	// 验证 ID 范围 [1, 715]
	sortedIDs := make([]int, 0, len(points))
	for _, p := range points {
		sortedIDs = append(sortedIDs, p.ID)
	}
	sort.Ints(sortedIDs)
	if sortedIDs[0] != 1 {
		t.Errorf("最小 ID 应为 1, 实际 %d", sortedIDs[0])
	}
	if sortedIDs[len(sortedIDs)-1] != 715 {
		t.Errorf("最大 ID 应为 715, 实际 %d", sortedIDs[len(sortedIDs)-1])
	}

	// 验证内/外区 ID 衔接：内区最后一个点 ID=169，外区第一个点 ID=170
	// 注：points 顺序为内区在前、外区在后（spec §6.2）
	if points[168].ID != 169 {
		t.Errorf("内区最后一个点 ID 应为 169, 实际 %d", points[168].ID)
	}
	if points[169].Region != "outer" || points[169].ID != 170 {
		t.Errorf("外区第一个点应为 outer/ID=170, 实际 Region=%s ID=%d",
			points[169].Region, points[169].ID)
	}
}

// TestGenerateSevenHolePoints_InnerCoordinates 【P1】验证内区点逻辑坐标与运动台方向补偿
//
// 内区逻辑坐标保留标准数据的 α/β 定义；运动坐标仅将 α 反号，以补偿当前运动台安装方向。
//
// 测试前置：构造完整模式默认配置
// 测试步骤：调用 GenerateSevenHolePoints，遍历内区点
// 期待结果：MotionCoordinates.α = -Coordinates.α，β 保持相同
func TestGenerateSevenHolePoints_InnerCoordinates(t *testing.T) {
	points, err := GenerateSevenHolePoints(sevenHoleBuildFullConfig(false))
	if err != nil {
		t.Fatalf("点位生成失败: %v", err)
	}

	for _, p := range points {
		if p.Region != "inner" {
			continue
		}
		alphaC := p.Coordinates["α"]
		betaC := p.Coordinates["β"]
		alphaM := p.MotionCoordinates["α"]
		betaM := p.MotionCoordinates["β"]

		if math.Abs(alphaC+alphaM) > epsilon {
			t.Errorf("内区点 ID=%d α 运动坐标应为逻辑坐标反号: %.4f/%.4f", p.ID, alphaC, alphaM)
		}
		if math.Abs(betaC-betaM) > epsilon {
			t.Errorf("内区点 ID=%d β 逻辑坐标 %.4f ≠ 运动坐标 %.4f", p.ID, betaC, betaM)
		}

		// 内区点不应包含 θ/φ 键
		if _, ok := p.Coordinates["θ"]; ok {
			t.Errorf("内区点 ID=%d Coordinates 不应包含 θ 键", p.ID)
		}
		if _, ok := p.Coordinates["φ"]; ok {
			t.Errorf("内区点 ID=%d Coordinates 不应包含 φ 键", p.ID)
		}
	}
}

// TestGenerateSevenHolePoints_OuterCoordinatesKeys 【P1】验证外区点 Coordinates 仅含 θ/φ 键
//
// 外区点逻辑坐标用 (θ, φ)，不应混入 α/β 键——避免 CSV 落盘时列名混乱
func TestGenerateSevenHolePoints_OuterCoordinatesKeys(t *testing.T) {
	points, err := GenerateSevenHolePoints(sevenHoleBuildFullConfig(false))
	if err != nil {
		t.Fatalf("点位生成失败: %v", err)
	}

	for _, p := range points {
		if p.Region != "outer" {
			continue
		}
		if _, ok := p.Coordinates["α"]; ok {
			t.Errorf("外区点 ID=%d Coordinates 不应包含 α 键", p.ID)
		}
		if _, ok := p.Coordinates["β"]; ok {
			t.Errorf("外区点 ID=%d Coordinates 不应包含 β 键", p.ID)
		}
		if _, ok := p.Coordinates["θ"]; !ok {
			t.Errorf("外区点 ID=%d Coordinates 应包含 θ 键", p.ID)
		}
		if _, ok := p.Coordinates["φ"]; !ok {
			t.Errorf("外区点 ID=%d Coordinates 应包含 φ 键", p.ID)
		}

		// MotionCoordinates 应仅含 α/β 键
		if _, ok := p.MotionCoordinates["α"]; !ok {
			t.Errorf("外区点 ID=%d MotionCoordinates 应包含 α 键", p.ID)
		}
		if _, ok := p.MotionCoordinates["β"]; !ok {
			t.Errorf("外区点 ID=%d MotionCoordinates 应包含 β 键", p.ID)
		}
		if _, ok := p.MotionCoordinates["θ"]; ok {
			t.Errorf("外区点 ID=%d MotionCoordinates 不应包含 θ 键", p.ID)
		}
	}
}

// TestGenerateSevenHolePoints_Serpentine 【P1】验证蛇形顺序：奇数行 α/θ 反向
//
// spec Task 6 验收标准：蛇形走位时，外层 β/φ 循环，奇数行（bi%2==1）的 α/θ 反向遍历。
// 验证策略：对比 Serpentine=true/false 两种配置——偶数行顺序相同，奇数行顺序相反。
//
// 测试前置：构造 Serpentine=true 和 Serpentine=false 两个完整模式配置
// 测试步骤：分别调用 GenerateSevenHolePoints，取第 0/1/2/3 行（每行 13 个点）
// 期待结果：
//   - 第 0 行（偶数行）：两种配置 α 顺序相同
//   - 第 1 行（奇数行）：Serpentine=true 时 α 顺序与 Serpentine=false 相反
func TestGenerateSevenHolePoints_Serpentine(t *testing.T) {
	pointsNormal, err := GenerateSevenHolePoints(sevenHoleBuildFullConfig(false))
	if err != nil {
		t.Fatalf("非蛇形模式生成失败: %v", err)
	}
	pointsSerp, err := GenerateSevenHolePoints(sevenHoleBuildFullConfig(true))
	if err != nil {
		t.Fatalf("蛇形模式生成失败: %v", err)
	}

	if len(pointsNormal) != len(pointsSerp) {
		t.Fatalf("两种模式点数应相同, 实际 normal=%d serp=%d", len(pointsNormal), len(pointsSerp))
	}

	// 内区每行 13 个点（α 从 -30° 到 +30° 步长 5° = 13 个值）
	const rowSize = 13

	// 第 0 行（bi=0，偶数行）：α 顺序相同
	for i := 0; i < rowSize; i++ {
		alphaNormal := pointsNormal[i].Coordinates["α"]
		alphaSerp := pointsSerp[i].Coordinates["α"]
		if math.Abs(alphaNormal-alphaSerp) > epsilon {
			t.Errorf("第 0 行位置 %d 偶数行 α 应相同: normal=%.1f serp=%.1f", i, alphaNormal, alphaSerp)
		}
	}

	// 第 1 行（bi=1，奇数行）：蛇形模式 α 反向
	// normal 第 1 行：α = -30, -25, ..., +30（升序）
	// serpentine 第 1 行：α = +30, +25, ..., -30（降序）
	for i := 0; i < rowSize; i++ {
		alphaNormal := pointsNormal[rowSize+i].Coordinates["α"]
		alphaSerp := pointsSerp[rowSize+i].Coordinates["α"]
		alphaSerpReversed := pointsSerp[rowSize+(rowSize-1-i)].Coordinates["α"]
		if math.Abs(alphaNormal-alphaSerpReversed) > epsilon {
			t.Errorf("第 1 行位置 %d 蛇形 α 应为 normal 的反向: normal[%d]=%.1f serp[%d]=%.1f",
				i, i, alphaNormal, rowSize-1-i, alphaSerp)
		}
		// 同时验证 β 相同（蛇形只反转 α，不反转 β）
		betaNormal := pointsNormal[rowSize+i].Coordinates["β"]
		betaSerp := pointsSerp[rowSize+i].Coordinates["β"]
		if math.Abs(betaNormal-betaSerp) > epsilon {
			t.Errorf("第 1 行位置 %d β 应相同（蛇形不反转 β）: normal=%.1f serp=%.1f",
				i, betaNormal, betaSerp)
		}
	}
}

func TestGenerateSevenHolePoints_FullSerpentineOuterReversesSectorAndPhiOrder(t *testing.T) {
	points, err := GenerateSevenHolePoints(sevenHoleBuildFullConfig(true))
	if err != nil {
		t.Fatalf("完整模式蛇形点位生成失败: %v", err)
	}

	const (
		innerCount   = 169
		outerRowSize = 78
	)
	oddRow := points[innerCount+outerRowSize : innerCount+2*outerRowSize]
	if oddRow[0].Sector != 6 || oddRow[len(oddRow)-1].Sector != 1 {
		t.Fatalf("奇数 θ 行扇区应从 6 反向到 1, 实际首尾扇区 %d→%d", oddRow[0].Sector, oddRow[len(oddRow)-1].Sector)
	}
	wantFirstPhi := []float64{330, 325, 320, 315}
	for i, want := range wantFirstPhi {
		if got := oddRow[i].Coordinates["φ"]; got != want {
			t.Errorf("奇数 θ 行 Sector 6 第 %d 个 φ 应为 %.1f, 实际 %.1f", i, want, got)
		}
	}
	if got := oddRow[0].Coordinates["θ"]; got != 35 {
		t.Errorf("奇数外区行 θ 应为 35, 实际 %.1f", got)
	}
}

// TestComputeSectorFromPhi 【P1】验证 φ → Sector 映射（spec §3.1 / §3.3 扇区居中约定）
//
// 扇区划分：每 60° 一个扇区，以 Pn 孔位方位角 (n-1)×60° 为中心、左右各跨 30°
//   - φ ∈ [330°, 360°) ∪ [0°, 30°) → Sector 1 (P1 孔位，中心 0°)
//   - φ ∈ [30°, 90°)  → Sector 2 (P2 孔位，中心 60°)
//   - ... 以此类推
//   - 边界 φ=30°/90°/.../330° 归高编号扇区（如 φ=30° → Sector 2）
//   - φ = 360° 等价于 φ = 0°，归入 Sector 1
//
// 测试前置：无
// 测试步骤：调用 computeSectorFromPhi 传入不同 φ 值
// 期待结果：返回值与 spec §3.1 扇区居中划分一致（Pn 为扇区内压力最大孔）
func TestComputeSectorFromPhi(t *testing.T) {
	cases := []struct {
		phi       float64
		expectSec int
	}{
		{0.0, 1}, {5.0, 1}, {29.0, 1}, {330.0, 1}, {355.0, 1}, {359.0, 1},
		{30.0, 2}, {60.0, 2}, {89.0, 2},
		{90.0, 3}, {120.0, 3}, {149.0, 3},
		{150.0, 4}, {180.0, 4}, {209.0, 4},
		{210.0, 5}, {240.0, 5}, {269.0, 5},
		{270.0, 6}, {300.0, 6}, {329.0, 6},
		{360.0, 1}, // 360° 等价于 0°，归 Sector 1
		{-5.0, 1},  // 负角度归一化到 [0,360)：-5°→355° → Sector 1
		{-31.0, 6}, // -31°→329° → Sector 6
		{720.0, 1}, // 多圈归一化
	}

	for _, c := range cases {
		sector := computeSectorFromPhi(c.phi)
		if sector != c.expectSec {
			t.Errorf("φ=%.1f° 应归 Sector %d, 实际 %d", c.phi, c.expectSec, sector)
		}
	}
}

// TestSevenHoleSectorBoundaryNeighbors 【P1】验证扇区边界 φ 的几何邻接判定
//
// 边界 φ ∈ {30°,90°,150°,210°,270°,330°} 同时属于两个相邻扇区；
// 非边界 φ（含扇区中心 0°/60°/...）返回 ok=false
//
// 测试步骤：调用 SevenHoleSectorBoundaryNeighbors 传入边界/非边界 φ
// 期待结果：返回边界两侧扇区对（φ=330° 跨 0°，邻接对为 {6,1}）
func TestSevenHoleSectorBoundaryNeighbors(t *testing.T) {
	cases := []struct {
		phi          float64
		lower, upper int
		ok           bool
	}{
		{30.0, 1, 2, true},
		{90.0, 2, 3, true},
		{150.0, 3, 4, true},
		{210.0, 4, 5, true},
		{270.0, 5, 6, true},
		{330.0, 6, 1, true},  // 跨 0° 边界
		{-30.0, 6, 1, true},  // 负角度归一化为 330°
		{0.0, 0, 0, false},   // 扇区 1 中心，非边界
		{60.0, 0, 0, false},  // 扇区 2 中心，非边界
		{355.0, 0, 0, false}, // 扇区 1 内部，非边界
	}

	for _, c := range cases {
		lower, upper, ok := SevenHoleSectorBoundaryNeighbors(c.phi)
		if ok != c.ok || lower != c.lower || upper != c.upper {
			t.Errorf("φ=%.1f° 期望 (%d,%d,%v), 实际 (%d,%d,%v)",
				c.phi, c.lower, c.upper, c.ok, lower, upper, ok)
		}
	}
}

// ==================== P2 边界与默认值测试 ====================

// TestGenerateSevenHolePoints_StepValidation 【P2】验证步长与范围校验
//
// spec Task 6 验收标准：步长 ≤ 0 返回错误；min > max 返回错误
//
// 测试前置：构造 4 种非法配置（内区步长 0、内区 min>max、外区步长负数、外区 min>max）
// 测试步骤：分别调用 GenerateSevenHolePoints
// 期待结果：4 种情况均返回错误
func TestGenerateSevenHolePoints_StepValidation(t *testing.T) {
	// 1. 内区 α 步长 = 0
	cfg := sevenHoleBuildFullConfig(false)
	cfg.InnerAlphaStep = 0
	if _, err := GenerateSevenHolePoints(cfg); err == nil {
		t.Error("内区 α 步长 = 0 应返回错误, 实际 nil")
	}

	// 2. 内区 β min > max
	cfg = sevenHoleBuildFullConfig(false)
	cfg.InnerBetaMin = 30.0
	cfg.InnerBetaMax = -30.0
	if _, err := GenerateSevenHolePoints(cfg); err == nil {
		t.Error("内区 β min > max 应返回错误, 实际 nil")
	}

	// 3. 外区 θ 步长 < 0
	cfg = sevenHoleBuildFullConfig(false)
	cfg.OuterThetaStep = -5.0
	if _, err := GenerateSevenHolePoints(cfg); err == nil {
		t.Error("外区 θ 步长 < 0 应返回错误, 实际 nil")
	}

	// 4. 外区 φ min > max
	cfg = sevenHoleBuildFullConfig(false)
	cfg.OuterPhiMin = 355.0
	cfg.OuterPhiMax = 0.0
	if _, err := GenerateSevenHolePoints(cfg); err == nil {
		t.Error("外区 φ min > max 应返回错误, 实际 nil")
	}
}

// TestGenerateSevenHolePoints_DefaultMode 【P2】验证空 Mode 默认走完整模式
//
// spec §6.2：空 Mode 视为完整模式（产品默认）
//
// 测试前置：构造 Mode="" 的配置（其他字段同完整模式）
// 测试步骤：调用 GenerateSevenHolePoints
// 期待结果：返回 715 点（与 SevenHoleModeFull 一致）
func TestGenerateSevenHolePoints_DefaultMode(t *testing.T) {
	cfg := sevenHoleBuildFullConfig(false)
	cfg.Mode = "" // 空模式

	points, err := GenerateSevenHolePoints(cfg)
	if err != nil {
		t.Fatalf("空 Mode 点位生成失败: %v", err)
	}
	if len(points) != 715 {
		t.Errorf("空 Mode 应默认走完整模式 (715 点), 实际 %d 点", len(points))
	}
}

// TestGenerateSevenHolePoints_RoundTo1Decimal 【P2】验证角度坐标 round 到 1 位小数
//
// spec Task 6 验收标准：浮点 round 到 1 位小数
// 防止浮点累积误差导致 CSV 出现 30.0000000001 等长尾小数
//
// 测试前置：构造完整模式默认配置
// 测试步骤：遍历所有点，检查坐标值的小数位数
// 期待结果：所有坐标值 × 10 后为整数（误差 < epsilon）
func TestGenerateSevenHolePoints_RoundTo1Decimal(t *testing.T) {
	points, err := GenerateSevenHolePoints(sevenHoleBuildFullConfig(false))
	if err != nil {
		t.Fatalf("点位生成失败: %v", err)
	}

	for _, p := range points {
		for key, val := range p.Coordinates {
			scaled := val * 10.0
			if math.Abs(scaled-math.Round(scaled)) > epsilon {
				t.Errorf("点 ID=%d 坐标 %s=%.6f 未 round 到 1 位小数", p.ID, key, val)
			}
		}
		for key, val := range p.MotionCoordinates {
			scaled := val * 10.0
			if math.Abs(scaled-math.Round(scaled)) > epsilon {
				t.Errorf("点 ID=%d 运动坐标 %s=%.6f 未 round 到 1 位小数", p.ID, key, val)
			}
		}
	}
}

// TestGenerateSevenHolePoints_DatasetSectorDistribution 【P2】验证数据集模式扇区分布
//
// spec §6.2：数据集模式按 6 个扇区分组遍历，每扇区 4 θ × 13 φ = 52 点
// 6 扇区共 312 点，且扇区边界点（如 φ=60°）不共享——每个扇区独立采集
//
// 测试前置：构造数据集模式配置
// 测试步骤：调用 GenerateSevenHolePoints，按 Sector 分组统计点数
// 期待结果：每个扇区（1..6）各 52 点
func TestGenerateSevenHolePoints_DatasetSectorDistribution(t *testing.T) {
	points, err := GenerateSevenHolePoints(sevenHoleBuildDatasetConfig(false))
	if err != nil {
		t.Fatalf("数据集模式点位生成失败: %v", err)
	}

	sectorCounts := make(map[int]int)
	for _, p := range points {
		if p.Region == "outer" {
			sectorCounts[p.Sector]++
		}
	}

	if len(sectorCounts) != 6 {
		t.Errorf("数据集模式应覆盖 6 个扇区, 实际 %d 个", len(sectorCounts))
	}
	for sector := 1; sector <= 6; sector++ {
		if sectorCounts[sector] != 52 {
			t.Errorf("扇区 %d 应有 52 点 (4 θ × 13 φ), 实际 %d 点", sector, sectorCounts[sector])
		}
	}
}

// TestGenerateSevenHolePoints_DatasetSerpentineOuter 【P2】验证数据集模式外区蛇形顺序
//
// 数据集模式外区遍历顺序：外层 θ → 中层扇区 → 内层 φ
// 蛇形走位时，奇数 θ 行的扇区顺序和 φ 方向都反向
//
// 测试前置：构造数据集模式 Serpentine=true 配置
// 测试步骤：取 Sector 1 在 θ=30°（ti=0, 正向）和 θ=35°（ti=1, 蛇形反向）的前 4 个点
// 期待结果：
//   - θ=30°（ti=0, 正向）Sector 1 是首个扇区，位于外区起始：φ = 330, 335, 340, 345
//   - θ=35°（ti=1, 蛇形反向）Sector 1 是末个扇区（反向后），位于外区位置 143..155：
//     φ = 30, 25, 20, 15（反向，从扇区末端起）
func TestGenerateSevenHolePoints_DatasetSerpentineOuter(t *testing.T) {
	points, err := GenerateSevenHolePoints(sevenHoleBuildDatasetConfig(true))
	if err != nil {
		t.Fatalf("数据集蛇形模式点位生成失败: %v", err)
	}

	// 跳过内区 169 点，外区从 index 169 开始
	// 主循环顺序：外层 θ (4 值) → 中层扇区 (6 个) → 内层 φ (13 值)
	//   每个θ行跨6扇区×13φ=78点
	//   ti=0 (θ=30°, 正向): Sector 1→2→3→4→5→6, Sector 1 占主循环 0..12 (加偏移 169 = 169..181)
	//   ti=1 (θ=35°, 反向): Sector 6→5→4→3→2→1, Sector 1 占主循环 143..155 (加偏移 169 = 312..324)
	outerStart := 169

	// θ=30°（ti=0, 正向）Sector 1 的前 4 个点
	// Sector 1 起始 φ=330°（归一化后），正向: 330, 335, 340, 345
	expectedTheta0 := 30.0
	expectedRow0Phi := []float64{330.0, 335.0, 340.0, 345.0}
	for i, wantPhi := range expectedRow0Phi {
		theta := points[outerStart+i].Coordinates["θ"]
		phi := points[outerStart+i].Coordinates["φ"]
		if math.Abs(theta-expectedTheta0) > epsilon {
			t.Errorf("θ=30° 正向 Sector 1 位置 %d θ 应为 %.1f, 实际 %.1f", i, expectedTheta0, theta)
		}
		if math.Abs(phi-wantPhi) > epsilon {
			t.Errorf("θ=30° 正向 Sector 1 位置 %d φ 应为 %.1f, 实际 %.1f", i, wantPhi, phi)
		}
	}

	// θ=35°（ti=1, 蛇形反向）Sector 1 的前 4 个点
	// Sector 1 反向后是最后一个扇区，φ 反向: 30, 25, 20, 15
	// 主循环位置：ti=1 占 78..155，Sector 1 反向后是最后 13 点（143..155），加偏移 169 = 312..324
	row1Start := outerStart + 143
	expectedTheta1 := 35.0
	expectedRow1Phi := []float64{30.0, 25.0, 20.0, 15.0}
	for i, wantPhi := range expectedRow1Phi {
		theta := points[row1Start+i].Coordinates["θ"]
		phi := points[row1Start+i].Coordinates["φ"]
		if math.Abs(theta-expectedTheta1) > epsilon {
			t.Errorf("θ=35° 蛇形反向 Sector 1 位置 %d θ 应为 %.1f, 实际 %.1f", i, expectedTheta1, theta)
		}
		if math.Abs(phi-wantPhi) > epsilon {
			t.Errorf("θ=35° 蛇形反向 Sector 1 位置 %d φ 应为 %.1f (反向), 实际 %.1f", i, wantPhi, phi)
		}
	}

	// 额外验证：θ 轴切换时 φ 轴不动（停在 330°）
	// θ=30° 行末尾（Sector 6 末尾，主循环 77）应为 φ=330°
	lastIdx30 := outerStart + 77
	lastPhi30 := points[lastIdx30].Coordinates["φ"]
	if math.Abs(lastPhi30-330.0) > epsilon {
		t.Errorf("θ=30° Sector 6 末尾 φ 应为 330.0 (停在起点), 实际 %.1f", lastPhi30)
	}
	// θ=35° 行起点（Sector 6 反向起点，主循环 78）也应为 φ=330°
	firstIdx35 := outerStart + 78
	firstPhi35 := points[firstIdx35].Coordinates["φ"]
	if math.Abs(firstPhi35-330.0) > epsilon {
		t.Errorf("θ=35° Sector 6 反向起点 φ 应为 330.0 (与上行末尾一致), 实际 %.1f", firstPhi35)
	}
}
