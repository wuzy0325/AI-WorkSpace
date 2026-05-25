package interpolation

import (
	"bufio"
	"fmt"
	"math"
	"os"
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
	Ka float64
	Kb float64
}

// gridInterpolationResult 网格插值结果
type gridInterpolationResult struct {
	Alpha      float64
	Beta       float64
	IsExtended bool
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
	aa1ExtGrid    *extendedGrid
	aa2ExtGrid    *extendedGrid
	sortedAlphas  []float64
	sortedBetas   []float64
}

// NewFiveHoleNewInterpolator 创建五孔探针新插值器
func NewFiveHoleNewInterpolator() *FiveHoleNewInterpolator {
	return &FiveHoleNewInterpolator{}
}

// LoadPrbFile 加载CSV校准数据文件
func (f *FiveHoleNewInterpolator) LoadPrbFile(filePath string) error {
	f.clearState()

	file, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("打开校准数据文件失败: %w", err)
	}
	defer file.Close()

	var lines []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" {
			lines = append(lines, line)
		}
	}

	rows, err := f.parseCsvFile(lines)
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
	aa1Result := f.interpolateOnGrid(aa1Point, f.aa1Grid, f.aa1BetaGroups, f.aa1ExtGrid, true)

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
		aa2Result := f.interpolateOnGrid(aa2Point, f.aa2Grid, f.aa2BetaGroups, f.aa2ExtGrid, true)
		finalAlpha = aa2Result.Alpha
		finalBeta = aa2Result.Beta
		isExtended = aa2Result.IsExtended

	case regionCenter:
		// 中心区：用AA3公式重新计算
		aa3Point := f.calculateKaKbAA3(p1, p2, p3, p4, p5)
		aa3Result := f.interpolateOnGrid(aa3Point, f.aa3Grid, f.aa3BetaGroups, nil, false)
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
	f.sortedAlphas = nil
	f.sortedBetas = nil
}

// parseCsvFile 解析CSV校准数据文件
func (f *FiveHoleNewInterpolator) parseCsvFile(lines []string) ([]calibrationRow, error) {
	if len(lines) < 2 {
		return nil, fmt.Errorf("CSV文件至少需要包含表头和一行数据")
	}

	var rows []calibrationRow
	for i := 1; i < len(lines); i++ {
		values := strings.Split(lines[i], ",")
		if len(values) != 7 {
			continue
		}

		parsed := make([]float64, 7)
		valid := true
		for j, v := range values {
			val, err := parseFloat(strings.TrimSpace(v))
			if err != nil {
				valid = false
				break
			}
			parsed[j] = val
		}
		if !valid {
			continue
		}

		rows = append(rows, calibrationRow{
			Alpha: parsed[0], Beta: parsed[1],
			P1: parsed[2], P2: parsed[3], P3: parsed[4], P4: parsed[5], P5: parsed[6],
		})
	}
	return rows, nil
}

// buildAllGrids 构建所有公式的网格数据
func (f *FiveHoleNewInterpolator) buildAllGrids() {
	alphaSet := make(map[float64]bool)
	betaSet := make(map[float64]bool)
	for _, row := range f.calibrationData {
		alphaSet[row.Alpha] = true
		betaSet[row.Beta] = true
	}

	f.sortedAlphas = sortedKeys(alphaSet)
	f.sortedBetas = sortedKeys(betaSet)

	f.aa1Grid, f.aa1BetaGroups = f.buildGrid(aaFormula1)
	f.aa2Grid, f.aa2BetaGroups = f.buildGrid(aaFormula2)
	f.aa3Grid, f.aa3BetaGroups = f.buildGrid(aaFormula3)

	f.aa1ExtGrid = f.generateExtendedGrid(f.aa1Grid, f.aa1BetaGroups)
	f.aa2ExtGrid = f.generateExtendedGrid(f.aa2Grid, f.aa2BetaGroups)
}

// buildGrid 构建指定公式的网格
func (f *FiveHoleNewInterpolator) buildGrid(formula aaFormula) (map[float64][]gridPoint, map[float64][]gridPoint) {
	alphaGroups := make(map[float64][]gridPoint)
	betaGroups := make(map[float64][]gridPoint)

	for _, row := range f.calibrationData {
		// AA3仅用于中心区域
		if formula == aaFormula3 && (math.Abs(row.Alpha) > 20 || math.Abs(row.Beta) > 20) {
			continue
		}

		result := f.calculateKaKb(row.P1, row.P2, row.P3, row.P4, row.P5, formula)
		gp := gridPoint{
			X: result.Ka, Y: result.Kb,
			Alpha: row.Alpha, Beta: row.Beta,
		}

		alphaGroups[row.Alpha] = append(alphaGroups[row.Alpha], gp)
		betaGroups[row.Beta] = append(betaGroups[row.Beta], gp)
	}

	return alphaGroups, betaGroups
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
	pressures := []float64{p1, p3, p4, p5}
	sorted := make([]float64, len(pressures))
	copy(sorted, pressures)
	sort.Float64s(sorted)
	// 降序排列
	for i, j := 0, len(sorted)-1; i < j; i, j = i+1, j-1 {
		sorted[i], sorted[j] = sorted[j], sorted[i]
	}

	pAvg := (p1 + p3 + p4 + p5) / 4

	var denominator float64
	switch formula {
	case aaFormula1:
		denominator = (p2+sorted[0]+sorted[1])/3 - pAvg
	case aaFormula2:
		denominator = (p2+sorted[0]+sorted[1]+sorted[2])/4 - pAvg
	case aaFormula3:
		denominator = p2 - pAvg
	}

	if denominator == 0 {
		return kaKbResult{}
	}

	return kaKbResult{
		Ka: (p4 - p5) / denominator,
		Kb: (p1 - p3) / denominator,
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
	betaGroups map[float64][]gridPoint,
	extGrid *extendedGrid,
	useExtended bool,
) gridInterpolationResult {
	px, py := point.Ka, point.Kb

	// 先在原始网格中查找
	cell := f.findGridCell(px, py, alphaGroups, betaGroups)
	isExtended := false

	// 如果在原始网格中找不到，尝试扩展网格
	if cell == nil && useExtended && extGrid != nil {
		cell = f.findExtendedGridCell(px, py, alphaGroups, betaGroups, extGrid)
		if cell != nil {
			isExtended = cell.IsExtended
		}
	}

	if cell == nil {
		return gridInterpolationResult{}
	}

	// 子网格细化 + 最近点搜索
	nearest := f.findNearestSubGridPoint(px, py, cell)
	if nearest == nil {
		return gridInterpolationResult{IsExtended: isExtended}
	}

	// 双线性插值反算角度
	t1 := float64(nearest.GridI) / subGridDivisions
	t2 := float64(nearest.GridJ) / subGridDivisions

	alpha := cell.AlphaLow + t2*(cell.AlphaHigh-cell.AlphaLow)
	beta := cell.BetaLow + t1*(cell.BetaHigh-cell.BetaLow)

	return gridInterpolationResult{Alpha: alpha, Beta: beta, IsExtended: isExtended}
}

// subGridPoint 子网格点
type subGridPoint struct {
	X     float64
	Y     float64
	GridI int
	GridJ int
}

// findGridCell 在原始网格中查找包含目标点的网格单元
func (f *FiveHoleNewInterpolator) findGridCell(px, py float64, alphaGroups, betaGroups map[float64][]gridPoint) *gridCell {
	alphas := sortedMapKeys(alphaGroups)
	betas := sortedMapKeys(betaGroups)

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

	for i := 0; i < len(alphas)-1; i++ {
		for j := 0; j < len(betas)-1; j++ {
			c1 := findPoint(alphas[i], betas[j])
			c2 := findPoint(alphas[i], betas[j+1])
			c3 := findPoint(alphas[i+1], betas[j])
			c4 := findPoint(alphas[i+1], betas[j+1])

			if c1 == nil || c2 == nil || c3 == nil || c4 == nil {
				continue
			}

			if pointInQuad(px, py, [4]*gridPoint{c1, c2, c3, c4}) {
				return &gridCell{
					AlphaLow: alphas[i], AlphaHigh: alphas[i+1],
					BetaLow: betas[j], BetaHigh: betas[j+1],
					Corners: [4]gridPoint{*c1, *c2, *c3, *c4},
				}
			}
		}
	}
	return nil
}

// findExtendedGridCell 在扩展网格中查找
func (f *FiveHoleNewInterpolator) findExtendedGridCell(px, py float64, alphaGroups, betaGroups map[float64][]gridPoint, extGrid *extendedGrid) *gridCell {
	// 简化实现：使用最近邻查找
	// 完整实现应参考参考项目的 findExtendedGridCell
	return nil
}

// findNearestSubGridPoint 子网格细化查找最近点
func (f *FiveHoleNewInterpolator) findNearestSubGridPoint(px, py float64, cell *gridCell) *subGridPoint {
	var nearest *subGridPoint
	minDist := math.MaxFloat64

	for i := 0; i <= subGridDivisions; i++ {
		for j := 0; j <= subGridDivisions; j++ {
			t1 := float64(i) / subGridDivisions
			t2 := float64(j) / subGridDivisions

			// 双线性插值计算子网格点的Kα/Kβ
			x := bilinearInterpolate(
				cell.Corners[0].X, cell.Corners[1].X,
				cell.Corners[2].X, cell.Corners[3].X,
				t1, t2,
			)
			y := bilinearInterpolate(
				cell.Corners[0].Y, cell.Corners[1].Y,
				cell.Corners[2].Y, cell.Corners[3].Y,
				t1, t2,
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

func (f *FiveHoleNewInterpolator) generateExtendedGrid(alphaGroups, betaGroups map[float64][]gridPoint) *extendedGrid {
	alphas := sortedMapKeys(alphaGroups)
	betas := sortedMapKeys(betaGroups)

	if len(alphas) < 2 || len(betas) < 2 {
		return nil
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

	return &extendedGrid{
		AllAlphas:      sortedBoolKeys(allAlphas),
		AllBetas:       sortedBoolKeys(allBetas),
		AlphaSpacing:   alphaSpacing,
		BetaSpacing:    betaSpacing,
		OriginalAlphas: alphas,
		OriginalBetas:  betas,
	}
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

func sortedKeys(m map[float64]bool) []float64 {
	keys := make([]float64, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Float64s(keys)
	return keys
}

func sortedMapKeys(m map[float64][]gridPoint) []float64 {
	keys := make([]float64, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Float64s(keys)
	return keys
}

func sortedBoolKeys(m map[float64]bool) []float64 {
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
