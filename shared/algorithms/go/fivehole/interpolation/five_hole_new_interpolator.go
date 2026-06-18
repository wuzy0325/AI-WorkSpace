package interpolation

import (
	"encoding/csv"
	"fmt"
	"math"
	"sort"
	"strings"
)

// ==================== 常量 ====================

const (
	subGridDivisions      = 100 // 子网格细分数
	maxExtrapolationAngle = 70  // 最大外推角度
	pointInTriangleTol    = 1e-9
)

// ==================== 数据结构 ====================

// calibrationRow CSV校准数据行
type calibrationRow struct {
	Alpha float64
	Beta  float64
	P1    float64
	P2    float64
	P3    float64
	P4    float64
	P5    float64
}

// gridPoint 网格点（在Kα-Kβ空间中）
type gridPoint struct {
	X     float64 // Kα
	Y     float64 // Kβ
	Alpha float64
	Beta  float64
}

// gridCell 网格单元
type gridCell struct {
	AlphaLow   float64
	AlphaHigh  float64
	BetaLow    float64
	BetaHigh   float64
	Corners    [4]gridPoint
	IsExtended bool
}

// extendedGrid 扩展网格信息
type extendedGrid struct {
	AllAlphas      []float64
	AllBetas       []float64
	AlphaSpacing   float64
	BetaSpacing    float64
	OriginalAlphas []float64
	OriginalBetas  []float64
}

// aaFormula AA公式类型
type aaFormula string

const (
	aaFormula1 aaFormula = "AA1" // 分母 = (P2 + P第一大 + P第二大) / 3 - P平均
	aaFormula2 aaFormula = "AA2" // 分母 = (P2 + P第一大 + P第二大 + P第三大) / 4 - P平均
	aaFormula3 aaFormula = "AA3" // 分母 = P2 - P平均
)

// region 区域类型
type region string

const (
	regionCorner region = "corner"
	regionEdge   region = "edge"
	regionCenter region = "center"
)

// kaKbResult Kα/Kβ计算结果
type kaKbResult struct {
	Ka    float64
	Kb    float64
	Valid bool
}

// gridInterpolationResult 网格插值结果
type gridInterpolationResult struct {
	Alpha      float64
	Beta       float64
	IsExtended bool
	Found      bool
}

// ==================== FiveHoleNewInterpolator ====================

// FiveHoleNewInterpolator 基于AA公式的五孔探针新插值算法
//
// 核心流程：
// 1. 用AA1公式计算初步角度(α1, β1)
// 2. 根据(α1, β1)判断区域（角区/边缘区/中心区）
// 3. 根据区域选择最终公式（AA1/AA2/AA3）
// 4. 用选定公式重新计算并插值得到最终(α, β)
type FiveHoleNewInterpolator struct {
	loaded          bool
	validRange      PrbValidRange
	calibrationData []calibrationRow
	pointCount      int

	// 预计算网格数据
	aa1Grid       map[float64][]gridPoint
	aa2Grid       map[float64][]gridPoint
	aa3Grid       map[float64][]gridPoint
	aa1BetaGroups map[float64][]gridPoint
	aa2BetaGroups map[float64][]gridPoint
	aa3BetaGroups map[float64][]gridPoint
	aa1ExtGrid *extendedGrid
	aa2ExtGrid *extendedGrid

	aa1SortedAlphas []float64
	aa1SortedBetas  []float64
	aa2SortedAlphas []float64
	aa2SortedBetas  []float64
	aa3SortedAlphas []float64
	aa3SortedBetas  []float64

	aa1ExtCells []gridCell
	aa2ExtCells []gridCell

	// Opt: 预计算每条 alpha 带的 Ka 范围, 避免每次 Calculate 重算
	aa1KaMin []float64
	aa1KaMax []float64
	aa2KaMin []float64
	aa2KaMax []float64
	aa3KaMin []float64
	aa3KaMax []float64

	sortedAlphas []float64
	sortedBetas  []float64
}

// NewFiveHoleNewInterpolator 创建五孔探针新插值器
func NewFiveHoleNewInterpolator() *FiveHoleNewInterpolator {
	return &FiveHoleNewInterpolator{}
}

// LoadPrbFile is kept for source compatibility. File I/O belongs in adapters.
func (f *FiveHoleNewInterpolator) LoadPrbFile(filePath string) error {
	return fmt.Errorf("load calibration file through an adapter and call LoadPrbLines")
}

// LoadPrbLines loads CSV calibration data from already-read text lines.
func (f *FiveHoleNewInterpolator) LoadPrbLines(lines []string) error {
	f.clearState()

	nonEmptyLines := make([]string, 0, len(lines))
	for _, raw := range lines {
		line := strings.TrimSpace(raw)
		if line != "" {
			nonEmptyLines = append(nonEmptyLines, line)
		}
	}

	rows, err := f.parseCsvFile(nonEmptyLines)
	if err != nil {
		return err
	}

	if len(rows) == 0 {
		return fmt.Errorf("CSV校准数据文件为空或格式不正确")
	}

	f.calibrationData = rows
	f.pointCount = len(rows)
	f.buildAllGrids()

	// 计算有效范围
	var alphaMin, alphaMax, betaMin, betaMax float64
	alphaMin, alphaMax = rows[0].Alpha, rows[0].Alpha
	betaMin, betaMax = rows[0].Beta, rows[0].Beta
	for _, r := range rows[1:] {
		alphaMin = math.Min(alphaMin, r.Alpha)
		alphaMax = math.Max(alphaMax, r.Alpha)
		betaMin = math.Min(betaMin, r.Beta)
		betaMax = math.Max(betaMax, r.Beta)
	}

	f.validRange = PrbValidRange{
		AlphaMin: alphaMin, AlphaMax: alphaMax,
		BetaMin: betaMin, BetaMax: betaMax,
	}
	f.loaded = true
	return nil
}

// IsLoaded 检查是否已加载
func (f *FiveHoleNewInterpolator) IsLoaded() bool {
	return f.loaded
}

// GetValidRange 获取有效范围
func (f *FiveHoleNewInterpolator) GetValidRange() PrbValidRange {
	return f.validRange
}

// Calculate 执行插值计算
func (f *FiveHoleNewInterpolator) Calculate(input InterpolationInput) (InterpolationResult, error) {
	if !f.loaded {
		return InterpolationResult{}, fmt.Errorf("校准数据未加载")
	}

	p1, p2, p3, p4, p5 := input.P1, input.P2, input.P3, input.P4, input.P5

	// 步骤1：用AA1公式计算初步角度
	aa1Point := f.calculateKaKbAA1(p1, p2, p3, p4, p5)
	aa1Result := f.interpolateOnGrid(aa1Point, f.aa1Grid, f.aa1SortedAlphas, f.aa1SortedBetas, f.aa1ExtGrid, f.aa1ExtCells, true)
	if !aa1Result.Found {
		return InterpolationResult{
			IsValid: false,
			Warning: "AA1初始角度插值失败，目标点不在校准网格或扩展网格内",
		}, nil
	}

	alpha1 := aa1Result.Alpha
	beta1 := aa1Result.Beta

	// 步骤2：区域判断
	r := f.determineRegion(alpha1, beta1)

	// 步骤3：根据区域选择公式
	var finalAlpha, finalBeta float64
	isExtended := aa1Result.IsExtended

	switch r {
	case regionCorner:
		// 角区：直接使用AA1结果
		finalAlpha = alpha1
		finalBeta = beta1

	case regionEdge:
		// 边缘区：用AA2公式重新计算
		aa2Point := f.calculateKaKbAA2(p1, p2, p3, p4, p5)
		aa2Result := f.interpolateOnGrid(aa2Point, f.aa2Grid, f.aa2SortedAlphas, f.aa2SortedBetas, f.aa2ExtGrid, f.aa2ExtCells, true)
		if !aa2Result.Found {
			return InterpolationResult{
				IsValid: false,
				Warning: "AA2边缘区插值失败，目标点不在校准网格或扩展网格内",
			}, nil
		}
		finalAlpha = aa2Result.Alpha
		finalBeta = aa2Result.Beta
		isExtended = aa2Result.IsExtended

	case regionCenter:
		// 中心区：用AA3公式重新计算
		aa3Point := f.calculateKaKbAA3(p1, p2, p3, p4, p5)
		aa3Result := f.interpolateOnGrid(aa3Point, f.aa3Grid, f.aa3SortedAlphas, f.aa3SortedBetas, nil, nil, false)
		if !aa3Result.Found {
			return InterpolationResult{
				IsValid: false,
				Warning: "AA3中心区插值失败，目标点不在校准网格内",
			}, nil
		}
		finalAlpha = aa3Result.Alpha
		finalBeta = aa3Result.Beta
		isExtended = aa3Result.IsExtended
	}

	var warnings []string
	if isExtended {
		warnings = append(warnings, "结果基于扩展网格外推，精度可能下降")
	}
	warnings = append(warnings, "新算法仅计算角度，压力/马赫数等参数未计算")

	isValid := isFinite(finalAlpha) && isFinite(finalBeta)
	var warningStr string
	if len(warnings) > 0 {
		warningStr = strings.Join(warnings, "; ")
	}

	return InterpolationResult{
		Alpha:   finalAlpha,
		Beta:    finalBeta,
		IsValid: isValid,
		Warning: warningStr,
	}, nil
}

// GetPointCount 获取数据点数量
func (f *FiveHoleNewInterpolator) GetPointCount() int {
	return f.pointCount
}

// ==================== 私有方法 ====================

func (f *FiveHoleNewInterpolator) clearState() {
	f.loaded = false
	f.calibrationData = nil
	f.aa1Grid = nil
	f.aa2Grid = nil
	f.aa3Grid = nil
	f.aa1BetaGroups = nil
	f.aa2BetaGroups = nil
	f.aa3BetaGroups = nil
	f.aa1ExtGrid = nil
	f.aa2ExtGrid = nil
	f.aa1SortedAlphas = nil
	f.aa1SortedBetas = nil
	f.aa2SortedAlphas = nil
	f.aa2SortedBetas = nil
	f.aa3SortedAlphas = nil
	f.aa3SortedBetas = nil
	f.aa1ExtCells = nil
	f.aa2ExtCells = nil
	f.aa1KaMin = nil
	f.aa1KaMax = nil
	f.aa2KaMin = nil
	f.aa2KaMax = nil
	f.aa3KaMin = nil
	f.aa3KaMax = nil
	f.sortedAlphas = nil
	f.sortedBetas = nil
}

// parseCsvFile 解析CSV校准数据文件
func (f *FiveHoleNewInterpolator) parseCsvFile(lines []string) ([]calibrationRow, error) {
	if len(lines) < 2 {
		return nil, fmt.Errorf("CSV文件至少需要包含表头和一行数据")
	}

	reader := csv.NewReader(strings.NewReader(strings.Join(lines, "\n")))
	reader.TrimLeadingSpace = true
	reader.FieldsPerRecord = -1

	records, err := reader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("CSV文件解析失败: %w", err)
	}
	if len(records) < 2 {
		return nil, fmt.Errorf("CSV文件至少需要包含表头和一行数据")
	}

	var rows []calibrationRow
	seen := make(map[string]bool)
	for i := 1; i < len(records); i++ {
		values := records[i]
		if len(values) != 7 {
			return nil, fmt.Errorf("CSV第%d行必须包含7列: alpha,beta,p1,p2,p3,p4,p5", i+1)
		}

		parsed := make([]float64, 7)
		for j, v := range values {
			val, err := parseFloat(strings.TrimSpace(v))
			if err != nil {
				return nil, fmt.Errorf("CSV第%d行第%d列不是有效数字", i+1, j+1)
			}
			parsed[j] = val
		}

		row := calibrationRow{
			Alpha: parsed[0], Beta: parsed[1],
			P1: parsed[2], P2: parsed[3], P3: parsed[4], P4: parsed[5], P5: parsed[6],
		}
		if !isFinite(row.Alpha) || !isFinite(row.Beta) ||
			!isFinite(row.P1) || !isFinite(row.P2) || !isFinite(row.P3) ||
			!isFinite(row.P4) || !isFinite(row.P5) {
			return nil, fmt.Errorf("CSV第%d行包含非有限数值", i+1)
		}
		key := calibrationPointKey(row.Alpha, row.Beta)
		if seen[key] {
			return nil, fmt.Errorf("CSV存在重复角度点(%.6g, %.6g)", row.Alpha, row.Beta)
		}
		seen[key] = true
		rows = append(rows, row)
	}
	if err := validateCalibrationGrid(rows); err != nil {
		return nil, err
	}
	return rows, nil
}

func validateCalibrationGrid(rows []calibrationRow) error {
	if len(rows) == 0 {
		return nil
	}

	alphaSet := make(map[float64]bool)
	betaSet := make(map[float64]bool)
	points := make(map[string]bool)
	for _, row := range rows {
		alphaSet[row.Alpha] = true
		betaSet[row.Beta] = true
		points[calibrationPointKey(row.Alpha, row.Beta)] = true
	}

	alphas := sortedFloat64Keys(alphaSet)
	betas := sortedFloat64Keys(betaSet)
	if len(alphas) < 2 || len(betas) < 2 {
		return fmt.Errorf("CSV校准网格至少需要2个alpha和2个beta角度")
	}

	for _, alpha := range alphas {
		for _, beta := range betas {
			if !points[calibrationPointKey(alpha, beta)] {
				return fmt.Errorf("CSV校准网格缺少角度点(%.6g, %.6g)", alpha, beta)
			}
		}
	}
	return nil
}

func calibrationPointKey(alpha, beta float64) string {
	return fmt.Sprintf("%.12g,%.12g", alpha, beta)
}

// buildAllGrids 构建所有公式的网格数据
func (f *FiveHoleNewInterpolator) buildAllGrids() {
	alphaSet := make(map[float64]bool)
	betaSet := make(map[float64]bool)
	for _, row := range f.calibrationData {
		alphaSet[row.Alpha] = true
		betaSet[row.Beta] = true
	}

	f.sortedAlphas = sortedFloat64Keys(alphaSet)
	f.sortedBetas = sortedFloat64Keys(betaSet)

	f.aa1Grid, _, f.aa1SortedAlphas, f.aa1SortedBetas = f.buildGrid(aaFormula1)
	f.aa2Grid, _, f.aa2SortedAlphas, f.aa2SortedBetas = f.buildGrid(aaFormula2)
	f.aa3Grid, _, f.aa3SortedAlphas, f.aa3SortedBetas = f.buildGrid(aaFormula3)

	// Opt: 预计算每条 alpha 带的 Ka 范围, 避免每次 Calculate 重算
	f.aa1KaMin, f.aa1KaMax = f.computeKaRange(f.aa1Grid, f.aa1SortedAlphas)
	f.aa2KaMin, f.aa2KaMax = f.computeKaRange(f.aa2Grid, f.aa2SortedAlphas)
	f.aa3KaMin, f.aa3KaMax = f.computeKaRange(f.aa3Grid, f.aa3SortedAlphas)

	f.aa1ExtGrid, f.aa1ExtCells = f.generateExtendedGrid(f.aa1Grid, f.aa1SortedAlphas, f.aa1SortedBetas)
	f.aa2ExtGrid, f.aa2ExtCells = f.generateExtendedGrid(f.aa2Grid, f.aa2SortedAlphas, f.aa2SortedBetas)
}

// buildGrid 构建指定公式的网格
func (f *FiveHoleNewInterpolator) buildGrid(formula aaFormula) (map[float64][]gridPoint, map[float64][]gridPoint, []float64, []float64) {
	alphaGroups := make(map[float64][]gridPoint)
	betaGroups := make(map[float64][]gridPoint)

	for _, row := range f.calibrationData {
		// AA3仅用于中心区域
		if formula == aaFormula3 && (math.Abs(row.Alpha) > 20 || math.Abs(row.Beta) > 20) {
			continue
		}

		result := f.calculateKaKb(row.P1, row.P2, row.P3, row.P4, row.P5, formula)
		if !result.Valid {
			continue
		}
		gp := gridPoint{
			X: result.Ka, Y: result.Kb,
			Alpha: row.Alpha, Beta: row.Beta,
		}

		alphaGroups[row.Alpha] = append(alphaGroups[row.Alpha], gp)
		betaGroups[row.Beta] = append(betaGroups[row.Beta], gp)
	}

	sortedAlphas := sortedFloat64Keys(alphaGroups)
	sortedBetas := sortedFloat64Keys(betaGroups)
	return alphaGroups, betaGroups, sortedAlphas, sortedBetas
}

// computeKaRange 预计算每条 alpha 带的 [Kamin, Kamax]，缓存到 struct 字段，
// 避免在 findGridCell 中每次 Calculate 都重新扫描 alphaGroups。
func (f *FiveHoleNewInterpolator) computeKaRange(alphaGroups map[float64][]gridPoint, sortedAlphas []float64) ([]float64, []float64) {
	if len(sortedAlphas) == 0 {
		return nil, nil
	}
	mn := make([]float64, len(sortedAlphas))
	mx := make([]float64, len(sortedAlphas))
	for i, a := range sortedAlphas {
		group := alphaGroups[a]
		if len(group) == 0 {
			mn[i], mx[i] = 0, 0
			continue
		}
		lo, hi := group[0].X, group[0].X
		for _, gp := range group[1:] {
			if gp.X < lo {
				lo = gp.X
			}
			if gp.X > hi {
				hi = gp.X
			}
		}
		mn[i] = lo
		mx[i] = hi
	}
	return mn, mx
}

// getKaRange 根据传入的 alphaGroups 反查对应公式的 KaMin/KaMax 缓存。
// 通过指针地址判断是哪一套数据 (aa1/aa2/aa3)。
func (f *FiveHoleNewInterpolator) getKaRange(alphaGroups map[float64][]gridPoint) ([]float64, []float64) {
	switch {
	case isSameMap(alphaGroups, f.aa1Grid):
		return f.aa1KaMin, f.aa1KaMax
	case isSameMap(alphaGroups, f.aa2Grid):
		return f.aa2KaMin, f.aa2KaMax
	case isSameMap(alphaGroups, f.aa3Grid):
		return f.aa3KaMin, f.aa3KaMax
	}
	return nil, nil
}

// isSameMap 判断两个 map 是否指向同一底层数据（用于 getKaRange 路由）。
func isSameMap(a, b map[float64][]gridPoint) bool {
	return fmt.Sprintf("%p", a) == fmt.Sprintf("%p", b)
}

// ==================== AA公式计算 ====================

func (f *FiveHoleNewInterpolator) calculateKaKbAA1(p1, p2, p3, p4, p5 float64) kaKbResult {
	return f.calculateKaKb(p1, p2, p3, p4, p5, aaFormula1)
}

func (f *FiveHoleNewInterpolator) calculateKaKbAA2(p1, p2, p3, p4, p5 float64) kaKbResult {
	return f.calculateKaKb(p1, p2, p3, p4, p5, aaFormula2)
}

func (f *FiveHoleNewInterpolator) calculateKaKbAA3(p1, p2, p3, p4, p5 float64) kaKbResult {
	return f.calculateKaKb(p1, p2, p3, p4, p5, aaFormula3)
}

// calculateKaKb 通用Kα/Kβ计算
func (f *FiveHoleNewInterpolator) calculateKaKb(p1, p2, p3, p4, p5 float64, formula aaFormula) kaKbResult {
	// Opt: 4 个浮点数手写降序排序，避免 slice 堆分配 + sort.Float64s 接口开销
	a, b, c, d := p1, p3, p4, p5
	if a < b {
		a, b = b, a
	}
	if c < d {
		c, d = d, c
	}
	if a < c {
		a, c = c, a
	}
	if b < d {
		b, d = d, b
	}
	if b < c {
		b, c = c, b
	}
	// 此时 a >= b >= c >= d，即降序
	sorted0, sorted1, sorted2 := a, b, c
	_ = d

	pAvg := (p1 + p3 + p4 + p5) / 4

	var denominator float64
	switch formula {
	case aaFormula1:
		denominator = (p2+sorted0+sorted1)/3 - pAvg
	case aaFormula2:
		denominator = (p2+sorted0+sorted1+sorted2)/4 - pAvg
	case aaFormula3:
		denominator = p2 - pAvg
	}

	if math.Abs(denominator) < 1e-12 || !isFinite(denominator) {
		return kaKbResult{}
	}

	return kaKbResult{
		Ka:    (p4 - p5) / denominator,
		Kb:    (p1 - p3) / denominator,
		Valid: true,
	}
}

// ==================== 区域判断 ====================

func (f *FiveHoleNewInterpolator) determineRegion(alpha1, beta1 float64) region {
	absAlpha := math.Abs(alpha1)
	absBeta := math.Abs(beta1)

	// 角区：α1和β1都在±15°之外
	if absAlpha > 15 && absBeta > 15 {
		return regionCorner
	}

	// 边缘区
	if (absAlpha <= 15 && absBeta > 20) || (absBeta <= 15 && absAlpha > 20) {
		return regionEdge
	}

	// 中心区
	return regionCenter
}

// ==================== 网格插值 ====================

func (f *FiveHoleNewInterpolator) interpolateOnGrid(
	point kaKbResult,
	alphaGroups map[float64][]gridPoint,
	sortedAlphas []float64,
	sortedBetas []float64,
	extGrid *extendedGrid,
	extCells []gridCell,
	useExtended bool,
) gridInterpolationResult {
	if !point.Valid || !isFinite(point.Ka) || !isFinite(point.Kb) {
		return gridInterpolationResult{}
	}

	px, py := point.Ka, point.Kb

	// 先在原始网格中查找
	cell := f.findGridCell(px, py, alphaGroups, sortedAlphas, sortedBetas)
	isExtended := false

	// 如果在原始网格中找不到，尝试扩展网格
	if cell == nil && useExtended && extGrid != nil && len(extCells) > 0 {
		cell = f.findCellInList(px, py, extCells)
		if cell != nil {
			isExtended = cell.IsExtended
		}
	}

	if cell == nil {
		return gridInterpolationResult{}
	}

	// Opt1: 双线性反演解析求解 O(1), 失败回退到子网格暴力扫描
	tAlpha, tBeta, ok := f.solveBilinearInverse(px, py, cell.Corners)
	if !ok {
		nearest := f.findNearestSubGridPoint(px, py, cell)
		if nearest == nil {
			return gridInterpolationResult{IsExtended: isExtended}
		}
		tAlpha = float64(nearest.GridI) / subGridDivisions
		tBeta = float64(nearest.GridJ) / subGridDivisions
	}

	alpha := cell.AlphaLow + tAlpha*(cell.AlphaHigh-cell.AlphaLow)
	beta := cell.BetaLow + tBeta*(cell.BetaHigh-cell.BetaLow)

	return gridInterpolationResult{Alpha: alpha, Beta: beta, IsExtended: isExtended, Found: true}
}

// subGridPoint 子网格点
type subGridPoint struct {
	X     float64
	Y     float64
	GridI int
	GridJ int
}

// findGridCell 在原始网格中查找包含目标点的网格单元
//
// 优化策略：预计算每个 alpha 带的 Ka 范围 (minKa, maxKa)，
// 定位 px 可能落入的 alpha 带，再在邻域内遍历 beta 交叉单元。
// 平均 O(log N + K*M) 替代原 O(N*M)。
func (f *FiveHoleNewInterpolator) findGridCell(px, py float64, alphaGroups map[float64][]gridPoint, sortedAlphas, sortedBetas []float64) *gridCell {
	na, nb := len(sortedAlphas), len(sortedBetas)
	if na < 2 || nb < 2 {
		return nil
	}

	findPoint := func(alpha, beta float64) *gridPoint {
		group, ok := alphaGroups[alpha]
		if !ok {
			return nil
		}
		for i := range group {
			if group[i].Beta == beta {
				return &group[i]
			}
		}
		return nil
	}

	// Ka 范围来自 struct 预计算缓存（buildAllGrids 阶段一次性构建）
	alphaKamin, alphaKamax := f.getKaRange(alphaGroups)
	if alphaKamin == nil || len(alphaKamin) != na {
		// 兜底：缓存缺失时即时计算（不应发生）
		alphaKamin, alphaKamax = f.computeKaRange(alphaGroups, sortedAlphas)
	}

	const neighbor = 2
	startI, endI := -1, -1
	for i := 0; i < na; i++ {
		if px >= alphaKamin[i]-1e-9 && px <= alphaKamax[i]+1e-9 {
			if startI < 0 { startI = i }
			endI = i
		}
	}
	if startI < 0 {
		for i := 0; i < na-1; i++ {
			for j := 0; j < nb-1; j++ {
				if cell := f.tryCell(px, py, sortedAlphas, sortedBetas, i, j, findPoint); cell != nil {
					return cell
				}
			}
		}
		return nil
	}
	if startI >= neighbor { startI -= neighbor } else { startI = 0 }
	endI += neighbor
	if endI >= na-1 { endI = na - 2 }

	for i := startI; i <= endI; i++ {
		for j := 0; j < nb-1; j++ {
			if cell := f.tryCell(px, py, sortedAlphas, sortedBetas, i, j, findPoint); cell != nil {
				return cell
			}
		}
	}
	return nil
}

func (f *FiveHoleNewInterpolator) tryCell(px, py float64, alphas, betas []float64, i, j int, findPoint func(float64, float64) *gridPoint) *gridCell {
	c1 := findPoint(alphas[i], betas[j])
	c2 := findPoint(alphas[i+1], betas[j])
	c3 := findPoint(alphas[i], betas[j+1])
	c4 := findPoint(alphas[i+1], betas[j+1])
	if c1 == nil || c2 == nil || c3 == nil || c4 == nil {
		return nil
	}
	if pointInQuad(px, py, [4]*gridPoint{c1, c2, c4, c3}) {
		return &gridCell{
			AlphaLow: alphas[i], AlphaHigh: alphas[i+1],
			BetaLow: betas[j], BetaHigh: betas[j+1],
			Corners: [4]gridPoint{*c1, *c2, *c3, *c4},
		}
	}
	return nil
}

// findCellInList 在预计算的 cell 列表中查找（Opt4：加载阶段预建四角）
func (f *FiveHoleNewInterpolator) findCellInList(px, py float64, cells []gridCell) *gridCell {
	for i := range cells {
		c := &cells[i]
		if pointInQuad(px, py, [4]*gridPoint{&c.Corners[0], &c.Corners[1], &c.Corners[3], &c.Corners[2]}) {
			return c
		}
	}
	return nil
}

func estimateGridPoint(alpha, beta float64, alphaGroups map[float64][]gridPoint, extGrid *extendedGrid) (*gridPoint, bool) {
	alpha0, alpha1, ok := interpolationBounds(alpha, extGrid.OriginalAlphas)
	if !ok {
		return nil, false
	}
	beta0, beta1, ok := interpolationBounds(beta, extGrid.OriginalBetas)
	if !ok {
		return nil, false
	}

	p00, ok00 := findGridPoint(alphaGroups, alpha0, beta0)
	p10, ok10 := findGridPoint(alphaGroups, alpha1, beta0)
	p01, ok01 := findGridPoint(alphaGroups, alpha0, beta1)
	p11, ok11 := findGridPoint(alphaGroups, alpha1, beta1)
	if !ok00 || !ok10 || !ok01 || !ok11 {
		return nil, false
	}

	tAlpha := interpolateFactor(alpha, alpha0, alpha1)
	tBeta := interpolateFactor(beta, beta0, beta1)
	point := gridPoint{
		X:     bilinearInterpolate(p00.X, p10.X, p01.X, p11.X, tAlpha, tBeta),
		Y:     bilinearInterpolate(p00.Y, p10.Y, p01.Y, p11.Y, tAlpha, tBeta),
		Alpha: alpha,
		Beta:  beta,
	}
	if !isFinite(point.X) || !isFinite(point.Y) {
		return nil, false
	}
	return &point, true
}

func interpolationBounds(value float64, sortedValues []float64) (float64, float64, bool) {
	n := len(sortedValues)
	if n < 2 {
		return 0, 0, false
	}

	// Opt: 二分搜索代替线性扫描，平均 O(log n)
	if value <= sortedValues[0] {
		return sortedValues[0], sortedValues[1], true
	}
	if value >= sortedValues[n-1] {
		return sortedValues[n-2], sortedValues[n-1], true
	}
	lo, hi := 0, n-1
	for hi-lo > 1 {
		mid := (lo + hi) / 2
		if sortedValues[mid] <= value {
			lo = mid
		} else {
			hi = mid
		}
	}
	return sortedValues[lo], sortedValues[hi], true
}

func findGridPoint(alphaGroups map[float64][]gridPoint, alpha, beta float64) (gridPoint, bool) {
	group, ok := alphaGroups[alpha]
	if !ok {
		return gridPoint{}, false
	}
	for _, point := range group {
		if point.Beta == beta {
			return point, true
		}
	}
	return gridPoint{}, false
}

// findNearestSubGridPoint 子网格细化查找最近点
func (f *FiveHoleNewInterpolator) findNearestSubGridPoint(px, py float64, cell *gridCell) *subGridPoint {
	var nearest *subGridPoint
	minDist := math.MaxFloat64

	for i := 0; i <= subGridDivisions; i++ {
		for j := 0; j <= subGridDivisions; j++ {
			tAlpha := float64(i) / subGridDivisions
			tBeta := float64(j) / subGridDivisions

			// 双线性插值计算子网格点的Kα/Kβ
			x := bilinearInterpolate(
				cell.Corners[0].X, cell.Corners[1].X,
				cell.Corners[2].X, cell.Corners[3].X,
				tAlpha, tBeta,
			)
			y := bilinearInterpolate(
				cell.Corners[0].Y, cell.Corners[1].Y,
				cell.Corners[2].Y, cell.Corners[3].Y,
				tAlpha, tBeta,
			)

			dist := math.Hypot(px-x, py-y)
			if dist < minDist {
				minDist = dist
				nearest = &subGridPoint{X: x, Y: y, GridI: i, GridJ: j}
			}
		}
	}
	return nearest
}

// ==================== 扩展网格 ====================

func (f *FiveHoleNewInterpolator) generateExtendedGrid(alphaGroups map[float64][]gridPoint, sortedAlphas, sortedBetas []float64) (*extendedGrid, []gridCell) {
	// Opt: 直接复用 buildGrid 已经排序好的 alpha/beta，避免重复排序
	alphas := sortedAlphas
	betas := sortedBetas

	if len(alphas) < 2 || len(betas) < 2 {
		return nil, nil
	}

	alphaSpacing := averageSpacing(alphas)
	betaSpacing := averageSpacing(betas)

	minAlpha, maxAlpha := alphas[0], alphas[len(alphas)-1]
	minBeta, maxBeta := betas[0], betas[len(betas)-1]

	// 生成扩展角度
	allAlphas := make(map[float64]bool)
	allBetas := make(map[float64]bool)

	for _, a := range alphas {
		allAlphas[a] = true
	}
	for _, b := range betas {
		allBetas[b] = true
	}

	for a := minAlpha - alphaSpacing; a >= -maxExtrapolationAngle; a -= alphaSpacing {
		allAlphas[a] = true
	}
	for a := maxAlpha + alphaSpacing; a <= maxExtrapolationAngle; a += alphaSpacing {
		allAlphas[a] = true
	}
	for b := minBeta - betaSpacing; b >= -maxExtrapolationAngle; b -= betaSpacing {
		allBetas[b] = true
	}
	for b := maxBeta + betaSpacing; b <= maxExtrapolationAngle; b += betaSpacing {
		allBetas[b] = true
	}

	allAlphasSorted := sortedFloat64Keys(allAlphas)
	allBetasSorted := sortedFloat64Keys(allBetas)

	extGrid := &extendedGrid{
		AllAlphas:      allAlphasSorted,
		AllBetas:       allBetasSorted,
		AlphaSpacing:   alphaSpacing,
		BetaSpacing:    betaSpacing,
		OriginalAlphas: alphas,
		OriginalBetas:  betas,
	}

	cells := preBuildExtendedCells(allAlphasSorted, allBetasSorted, alphaGroups, extGrid)

	return extGrid, cells
}

// preBuildExtendedCells 预计算扩展网格中所有 cell 的四角点
//
// 将原本在每次 Calculate 中重复执行的 estimateGridPoint 合并到加载阶段一次性完成。
// 返回值可被多次只读遍历（findCellInList）。
func preBuildExtendedCells(allAlphas, allBetas []float64, alphaGroups map[float64][]gridPoint, extGrid *extendedGrid) []gridCell {
	if len(allAlphas) < 2 || len(allBetas) < 2 {
		return nil
	}

	estimate := func(alpha, beta float64) (*gridPoint, bool) {
		return estimateGridPoint(alpha, beta, alphaGroups, extGrid)
	}

	cells := make([]gridCell, 0, (len(allAlphas)-1)*(len(allBetas)-1))
	for i := 0; i < len(allAlphas)-1; i++ {
		for j := 0; j < len(allBetas)-1; j++ {
			alphaLow, alphaHigh := allAlphas[i], allAlphas[i+1]
			betaLow, betaHigh := allBetas[j], allBetas[j+1]

			c1, ok1 := estimate(alphaLow, betaLow)
			c2, ok2 := estimate(alphaHigh, betaLow)
			c3, ok3 := estimate(alphaLow, betaHigh)
			c4, ok4 := estimate(alphaHigh, betaHigh)
			if !ok1 || !ok2 || !ok3 || !ok4 {
				continue
			}

			isExtended := alphaLow < extGrid.OriginalAlphas[0] ||
				alphaHigh > extGrid.OriginalAlphas[len(extGrid.OriginalAlphas)-1] ||
				betaLow < extGrid.OriginalBetas[0] ||
				betaHigh > extGrid.OriginalBetas[len(extGrid.OriginalBetas)-1]

			cells = append(cells, gridCell{
				AlphaLow: alphaLow, AlphaHigh: alphaHigh,
				BetaLow: betaLow, BetaHigh: betaHigh,
				Corners:    [4]gridPoint{*c1, *c2, *c3, *c4},
				IsExtended: isExtended,
			})
		}
	}
	return cells
}
// ==================== 几何工具 ====================

func pointInQuad(px, py float64, corners [4]*gridPoint) bool {
	// 使用叉积法判断点是否在凸四边形内
	points := [4]point2D{
		{X: corners[0].X, Y: corners[0].Y},
		{X: corners[1].X, Y: corners[1].Y},
		{X: corners[2].X, Y: corners[2].Y},
		{X: corners[3].X, Y: corners[3].Y},
	}
	testPoint := point2D{X: px, Y: py}
	return isPointInsideConvexQuad(testPoint, points)
}

func bilinearInterpolate(v00, v01, v10, v11, t1, t2 float64) float64 {
	return v00*(1-t1)*(1-t2) + v01*t1*(1-t2) + v10*(1-t1)*t2 + v11*t1*t2
}

// ==================== 通用工具 ====================

// sortedFloat64Keys 从 map[float64]V 中提取 key 并排序
func sortedFloat64Keys[V any](m map[float64]V) []float64 {
	keys := make([]float64, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Float64s(keys)
	return keys
}

func averageSpacing(values []float64) float64 {
	if len(values) < 2 {
		return 5
	}
	total := 0.0
	for i := 1; i < len(values); i++ {
		total += values[i] - values[i-1]
	}
	return total / float64(len(values)-1)
}


// solveBilinearInverse 求解双线性映射的反函数
//
// 双线性映射 (t1,t2) → (X,Y)：
//   X = c1.X*(1-t1)*(1-t2) + c2.X*t1*(1-t2) + c3.X*(1-t1)*t2 + c4.X*t1*t2
//   Y = c1.Y*(1-t1)*(1-t2) + c2.Y*t1*(1-t2) + c3.Y*(1-t1)*t2 + c4.Y*t1*t2
//
// 消去 t1 得到关于 t2 的二次方程 A·t2² + B·t2 + C = 0。
// 解方程后选重投影误差最小的根，退化时返回 false 让上层回退到子网格暴力扫描。
//
// 参考：Heckbert 1989, "Fundamentals of Texture Mapping and Image Warping".
func (f *FiveHoleNewInterpolator) solveBilinearInverse(px, py float64, corners [4]gridPoint) (float64, float64, bool) {
	const cornerEps = 1e-4
	if math.Abs(corners[0].X-px) < cornerEps && math.Abs(corners[0].Y-py) < cornerEps { return 0, 0, true }
	if math.Abs(corners[1].X-px) < cornerEps && math.Abs(corners[1].Y-py) < cornerEps { return 1, 0, true }
	if math.Abs(corners[2].X-px) < cornerEps && math.Abs(corners[2].Y-py) < cornerEps { return 0, 1, true }
	if math.Abs(corners[3].X-px) < cornerEps && math.Abs(corners[3].Y-py) < cornerEps { return 1, 1, true }

	x1, y1 := corners[0].X, corners[0].Y
	x2, y2 := corners[1].X, corners[1].Y
	x3, y3 := corners[2].X, corners[2].Y
	x4, y4 := corners[3].X, corners[3].Y

	a1 := x2 - x1; b1 := x3 - x1; c1c := x1 - x2 - x3 + x4
	a2 := y2 - y1; b2 := y3 - y1; c2c := y1 - y2 - y3 + y4

	dx := px - x1; dy := py - y1

	A := b1*c2c - b2*c1c
	B := dx*c2c - dy*c1c + a1*b2 - a2*b1
	C := dx*a2 - dy*a1

	const quadEps = 1e-12
	var t2Candidates []float64
	if math.Abs(A) < quadEps {
		if math.Abs(B) < quadEps { return 0, 0, false }
		t2Candidates = []float64{-C / B}
	} else {
		disc := B*B - 4*A*C
		if disc < 0 {
			if disc > -quadEps { disc = 0 } else { return 0, 0, false }
		}
		sq := math.Sqrt(disc)
		t2Candidates = []float64{(-B + sq) / (2 * A), (-B - sq) / (2 * A)}
	}

	const inRangeEps = 1e-6
	bestT1, bestT2 := 0.0, 0.0
	bestErr := math.MaxFloat64
	found := false
	for _, t2 := range t2Candidates {
		if !isFinite(t2) || t2 < -inRangeEps || t2 > 1+inRangeEps { continue }
		denom := a1 + c1c*t2
		var t1 float64
		if math.Abs(denom) > quadEps {
			t1 = (dx - b1*t2) / denom
		} else {
			denom2 := a2 + c2c*t2
			if math.Abs(denom2) < quadEps { continue }
			t1 = (dy - b2*t2) / denom2
		}
		if !isFinite(t1) || t1 < -inRangeEps || t1 > 1+inRangeEps { continue }
		t1c := math.Max(0, math.Min(1, t1))
		t2c := math.Max(0, math.Min(1, t2))
		gx := bilinearInterpolate(x1, x2, x3, x4, t1c, t2c)
		gy := bilinearInterpolate(y1, y2, y3, y4, t1c, t2c)
		errSq := (gx-px)*(gx-px) + (gy-py)*(gy-py)
		if errSq < bestErr {
			bestErr = errSq
			bestT1, bestT2 = t1c, t2c
			found = true
		}
	}
	return bestT1, bestT2, found
}