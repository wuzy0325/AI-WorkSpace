package interpolation

import (
	"fmt"
	"math"
	"strings"
)

// ==================== 常量定义 ====================

const (
	gridMinAngle      = -30                         // 网格最小角度
	gridMaxAngle      = 30                          // 网格最大角度
	gridStep          = 5                           // 网格步长
	gridAxisSize      = 13                          // 网格轴大小 (-30 到 30, 步长5)
	expectedGridCount = gridAxisSize * gridAxisSize // 169
	numericColCount   = 6                           // 数值列数
	defaultEpsilon    = 1e-3                        // 默认精度容差
	minPressureDelta  = 1e-4                        // 最小压力差
	gasConstantAir    = 287.06                      // 空气气体常数 J/(kg·K)
	gamma             = 1.4                         // 空气比热比
)

// ==================== 核心数据结构 ====================

// probeTableRow PRB文件数据行
type probeTableRow struct {
	Ka    float64 // 攻角系数
	Kb    float64 // 侧滑角系数
	CPT   float64 // 总压恢复系数
	CPS   float64 // 静压恢复系数
	Alpha float64 // 攻角
	Beta  float64 // 侧滑角
}

// point2D 二维点
type point2D struct {
	X float64
	Y float64
}

// lineEquation 直线方程 Ax + By + C = 0
type lineEquation struct {
	A         float64
	B         float64
	C         float64
	NormalLen float64
}

// boundaryPoint 边界点
type boundaryPoint struct {
	Ka    float64
	Kb    float64
	Alpha float64
	Beta  float64
}

// interpolationPoint 插值点
type interpolationPoint struct {
	Ka    float64
	Kb    float64
	Alpha *float64 // 可能为nil（未解析）
	Beta  *float64
}

// interResult 插值中间结果
type interResult struct {
	Alpha float64
	Beta  float64
	Pt    float64
	Ps    float64
	V     float64
	Ma    float64
}

// region9Cell 区域9的网格单元
type region9Cell struct {
	X1       probeTableRow
	X2       probeTableRow
	X3       probeTableRow
	X4       probeTableRow
	Vertices [4]point2D
}

// indexedCalibrationTable 索引化的校准表
type indexedCalibrationTable struct {
	Rows                     []probeTableRow
	GetExactGridPoint        func(alpha, beta float64) *probeTableRow
	GetExactGridPointOrThrow func(alpha, beta float64) (probeTableRow, error)
}

// ==================== PrbInterpolator PRB插值器 ====================

// PrbInterpolator 基于PRB文件的五孔探针插值器
//
// PRB文件格式：
//
//	第一行：网格尺寸 (13 13)
//	后续行：ka kb cpt cps alpha beta
//
// 插值策略：
//  1. 计算Kα/Kβ压力系数
//  2. 判断9个区域（4个角区+4个边区+1个中心区）
//  3. 角区和边区使用边界点插值
//  4. 中心区使用凸四边形网格插值
type PrbInterpolator struct {
	loaded     bool
	validRange PrbValidRange
	context    *prbInterpolationContext
}

type prbInterpolationContext struct {
	ValidRange     PrbValidRange
	GetInterResult func(input runtimeInput) interResult
	AtmCalc        *AtmosphericDataCalculator
}

// runtimeInput 运行时插值输入
type runtimeInput struct {
	AtmP         float64
	AtmT         float64
	FiveHoleData [5]float64
}

// NewPrbInterpolator 创建PRB插值器
func NewPrbInterpolator() *PrbInterpolator {
	return &PrbInterpolator{}
}

// LoadPrbFile is kept for source compatibility. File I/O belongs in adapters.
func (p *PrbInterpolator) LoadPrbFile(filePath string) error {
	return fmt.Errorf("load PRB file through an adapter and call LoadPrbLines")
}

// LoadPrbLines loads PRB calibration data from already-read text lines.
func (p *PrbInterpolator) LoadPrbLines(lines []string, source string) error {
	p.clearState()

	nonEmptyLines := make([]string, 0, len(lines))
	for _, raw := range lines {
		line := strings.TrimSpace(raw)
		if line != "" {
			nonEmptyLines = append(nonEmptyLines, line)
		}
	}

	rows, err := parsePrbFile(nonEmptyLines)
	if err != nil {
		return err
	}

	indexedTable, err := validateAndIndexTable(rows)
	if err != nil {
		return err
	}

	validRange := createValidRange(rows, source)

	atmCalc := NewAtmosphericDataCalculator()
	p.context = &prbInterpolationContext{
		ValidRange:     validRange,
		GetInterResult: createInterResultCalculator(indexedTable, atmCalc),
		AtmCalc:        atmCalc,
	}
	p.validRange = validRange
	p.loaded = true
	return nil
}

// IsLoaded 检查是否已加载
func (p *PrbInterpolator) IsLoaded() bool {
	return p.loaded
}

// GetValidRange 获取有效范围
func (p *PrbInterpolator) GetValidRange() PrbValidRange {
	return p.validRange
}

// Calculate 执行插值计算
func (p *PrbInterpolator) Calculate(input InterpolationInput) (result InterpolationResult, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("插值计算内部panic: %v", r)
		}
	}()

	if !p.loaded || p.context == nil {
		return InterpolationResult{}, fmt.Errorf("PRB文件未加载")
	}

	rtInput := toRuntimeInput(input)
	warnings := collectInputWarnings(rtInput)

	interResult := p.context.GetInterResult(rtInput)
	return toInterpolationResult(interResult, rtInput, p.context.ValidRange, warnings, p.context.AtmCalc), nil
}

func (p *PrbInterpolator) clearState() {
	p.context = nil
	p.validRange = PrbValidRange{
		AlphaMin: gridMinAngle, AlphaMax: gridMaxAngle,
		BetaMin: gridMinAngle, BetaMax: gridMaxAngle,
	}
	p.loaded = false
}

// ==================== PRB文件解析 ====================

func parsePrbFile(lines []string) ([]probeTableRow, error) {
	if len(lines) == 0 {
		return nil, fmt.Errorf("PRB文件为空")
	}

	// 解析表头
	headerParts := strings.Fields(lines[0])
	if len(headerParts) != 2 {
		return nil, fmt.Errorf("PRB文件表头必须包含两个网格维度")
	}

	var width, height int
	if _, err := fmt.Sscanf(headerParts[0], "%d", &width); err != nil {
		return nil, fmt.Errorf("PRB文件表头宽度解析失败")
	}
	if _, err := fmt.Sscanf(headerParts[1], "%d", &height); err != nil {
		return nil, fmt.Errorf("PRB文件表头高度解析失败")
	}

	if width != gridAxisSize || height != gridAxisSize {
		return nil, fmt.Errorf("PRB文件表头必须为 %d %d", gridAxisSize, gridAxisSize)
	}

	rowLines := lines[1:]
	if len(rowLines) != expectedGridCount {
		return nil, fmt.Errorf("PRB文件必须包含 %d 行数据", expectedGridCount)
	}

	rows := make([]probeTableRow, len(rowLines))
	for i, line := range rowLines {
		row, err := parseProbeTableRow(line, i)
		if err != nil {
			return nil, err
		}
		rows[i] = row
	}

	return rows, nil
}

func parseProbeTableRow(line string, index int) (probeTableRow, error) {
	columns := strings.Fields(line)
	if len(columns) != numericColCount {
		return probeTableRow{}, fmt.Errorf("PRB第%d行必须包含%d列数据", index+1, numericColCount)
	}

	values := make([]float64, numericColCount)
	for j, col := range columns {
		val, err := parseFloat(col)
		if err != nil {
			return probeTableRow{}, fmt.Errorf("PRB第%d行列%d不是有效数字", index+1, j+1)
		}
		values[j] = val
	}

	return probeTableRow{
		Ka:    values[0],
		Kb:    values[1],
		CPT:   values[2],
		CPS:   values[3],
		Alpha: values[4],
		Beta:  values[5],
	}, nil
}

func parseFloat(s string) (float64, error) {
	var f float64
	_, err := fmt.Sscanf(s, "%f", &f)
	if err != nil || math.IsNaN(f) || math.IsInf(f, 0) {
		return 0, fmt.Errorf("无效数字: %s", s)
	}
	return f, nil
}

// ==================== 校验与索引 ====================

func validateAndIndexTable(table []probeTableRow) (*indexedCalibrationTable, error) {
	if len(table) != expectedGridCount {
		return nil, fmt.Errorf("校准表必须包含 %d 行", expectedGridCount)
	}

	// 构建网格点索引
	expectedKeys := make(map[string]bool)
	gridAngles := createGridAngles()
	for _, alpha := range gridAngles {
		for _, beta := range gridAngles {
			expectedKeys[gridPointKey(alpha, beta)] = true
		}
	}

	byGridPoint := make(map[string]probeTableRow)
	rows := make([]probeTableRow, 0, len(table))

	for i, row := range table {
		if !isFiniteRow(row) {
			return nil, fmt.Errorf("校准第%d行包含非有限值", i+1)
		}

		key := gridPointKey(row.Alpha, row.Beta)
		if !expectedKeys[key] {
			return nil, fmt.Errorf("校准第%d行网格点(%.0f,%.0f)不在预期范围内", i+1, row.Alpha, row.Beta)
		}
		if _, exists := byGridPoint[key]; exists {
			return nil, fmt.Errorf("校准表存在重复网格点(%.0f,%.0f)", row.Alpha, row.Beta)
		}

		snapshot := row
		byGridPoint[key] = snapshot
		rows = append(rows, snapshot)
	}

	if len(byGridPoint) != len(expectedKeys) {
		return nil, fmt.Errorf("校准表缺少一个或多个网格点")
	}

	return &indexedCalibrationTable{
		Rows: rows,
		GetExactGridPoint: func(alpha, beta float64) *probeTableRow {
			if row, ok := byGridPoint[gridPointKey(alpha, beta)]; ok {
				return &row
			}
			return nil
		},
		GetExactGridPointOrThrow: func(alpha, beta float64) (probeTableRow, error) {
			row, ok := byGridPoint[gridPointKey(alpha, beta)]
			if !ok {
				return probeTableRow{}, fmt.Errorf("校准表不包含网格点(%.0f,%.0f)", alpha, beta)
			}
			return row, nil
		},
	}, nil
}

func createGridAngles() []float64 {
	angles := make([]float64, 0, gridAxisSize)
	for a := float64(gridMinAngle); a <= float64(gridMaxAngle); a += gridStep {
		angles = append(angles, a)
	}
	return angles
}

func gridPointKey(alpha, beta float64) string {
	// 归一化负零：math.Trunc 对 (-5,0) 范围内的负数返回 -0，
	// 乘以 gridStep 后仍为 -0，导致 Sprintf 输出 "-0" 与实际键 "0" 不匹配，
	// 进而 GetExactGridPointOrThrow 失败，interpolateOutputValue 返回 0
	if alpha == 0 {
		alpha = 0
	}
	if beta == 0 {
		beta = 0
	}
	return fmt.Sprintf("%.0f,%.0f", alpha, beta)
}

func isFiniteRow(row probeTableRow) bool {
	return isFinite(row.Ka) && isFinite(row.Kb) && isFinite(row.CPT) &&
		isFinite(row.CPS) && isFinite(row.Alpha) && isFinite(row.Beta)
}

func isFinite(f float64) bool {
	return !math.IsNaN(f) && !math.IsInf(f, 0)
}

// ==================== 有效范围计算 ====================

func createValidRange(rows []probeTableRow, filePath string) PrbValidRange {
	var alphaMin, alphaMax, betaMin, betaMax float64
	alphaMin = rows[0].Alpha
	alphaMax = rows[0].Alpha
	betaMin = rows[0].Beta
	betaMax = rows[0].Beta

	for _, row := range rows[1:] {
		alphaMin = math.Min(alphaMin, row.Alpha)
		alphaMax = math.Max(alphaMax, row.Alpha)
		betaMin = math.Min(betaMin, row.Beta)
		betaMax = math.Max(betaMax, row.Beta)
	}

	machToken := parseMachFromFileName(filePath)

	return PrbValidRange{
		AlphaMin: alphaMin, AlphaMax: alphaMax,
		BetaMin: betaMin, BetaMax: betaMax,
		MachMin: machToken, MachMax: machToken,
	}
}

// parseMachFromFileName 在 multi_prb_interpolator.go 中定义

// ==================== 插值计算核心 ====================

func createInterResultCalculator(table *indexedCalibrationTable, atmCalc *AtmosphericDataCalculator) func(input runtimeInput) interResult {
	// 预计算边界点
	region2Boundary := createBoundaryPoints(table.Rows,
		func(row probeTableRow) bool { return row.Beta == gridMinAngle && row.Kb < 0 },
		func(row probeTableRow) float64 { return row.Alpha },
	)
	region4Boundary := createBoundaryPoints(table.Rows,
		func(row probeTableRow) bool { return row.Alpha == gridMaxAngle && row.Ka > 0 },
		func(row probeTableRow) float64 { return row.Beta },
	)
	region6Boundary := createBoundaryPoints(table.Rows,
		func(row probeTableRow) bool { return row.Beta == gridMaxAngle && row.Kb > 0 },
		func(row probeTableRow) float64 { return row.Alpha },
	)
	region8Boundary := createBoundaryPoints(table.Rows,
		func(row probeTableRow) bool { return row.Alpha == gridMinAngle && row.Ka < 0 },
		func(row probeTableRow) float64 { return row.Beta },
	)

	// 预计算角点
	region1Corner, err := table.GetExactGridPointOrThrow(gridMinAngle, gridMinAngle)
	if err != nil {
		panic(err)
	}
	region3Corner, err := table.GetExactGridPointOrThrow(gridMaxAngle, gridMinAngle)
	if err != nil {
		panic(err)
	}
	region5Corner, err := table.GetExactGridPointOrThrow(gridMaxAngle, gridMaxAngle)
	if err != nil {
		panic(err)
	}
	region7Corner, err := table.GetExactGridPointOrThrow(gridMinAngle, gridMaxAngle)
	if err != nil {
		panic(err)
	}

	// 预计算区域9网格单元
	region9Cells := createRegion9Cells(table.GetExactGridPointOrThrow)

	return func(input runtimeInput) interResult {
		// 步骤1：计算压力系数
		point := calculatePressureCoefficients(input)

		// 步骤2：解析区域1-8
		point = resolveRegions1To8(point, region1Corner, region3Corner, region5Corner, region7Corner,
			region2Boundary, region4Boundary, region6Boundary, region8Boundary)

		// 步骤3：如果角度未解析，尝试区域9
		if point.Alpha == nil || point.Beta == nil {
			point = resolveRegion9(point, region9Cells)
		}

		// 断言角度已解析
		if point.Alpha == nil || point.Beta == nil {
			return interResult{} // 无法解析，返回零值
		}

		alpha := *point.Alpha
		beta := *point.Beta

		// 步骤4：插值计算总压和静压
		cpt := interpolateOutputValue(point, table, func(row probeTableRow) float64 { return row.CPT })
		cps := interpolateOutputValue(point, table, func(row probeTableRow) float64 { return row.CPS })

		_, delta := calculatePressureDelta(input)
		pt := input.FiveHoleData[1] - cpt*delta
		avg := (input.FiveHoleData[0] + input.FiveHoleData[2] + input.FiveHoleData[3] + input.FiveHoleData[4]) * 0.25
		ps := avg - cps*delta

		v := calculateVelocity(atmCalc, input, pt, ps)
		ma := calculateMachFromPressures(atmCalc, input, pt, ps)

		return interResult{
			Alpha: alpha,
			Beta:  beta,
			Pt:    pt,
			Ps:    ps,
			V:     v,
			Ma:    ma,
		}
	}
}

// calculatePressureCoefficients 计算压力系数
func calculatePressureCoefficients(input runtimeInput) interpolationPoint {
	p1, p2, p3, p4, p5 := input.FiveHoleData[0], input.FiveHoleData[1], input.FiveHoleData[2], input.FiveHoleData[3], input.FiveHoleData[4]
	avg := (p1 + p3 + p4 + p5) * 0.25
	delta := clampPressureDelta(p2 - avg)

	return interpolationPoint{
		Ka: (p4 - p5) / delta,
		Kb: (p3 - p1) / delta,
	}
}

// resolveRegions1To8 解析区域1-8的角度
func resolveRegions1To8(
	point interpolationPoint,
	corner1, corner3, corner5, corner7 probeTableRow,
	boundary2, boundary4, boundary6, boundary8 []boundaryPoint,
) interpolationPoint {
	// 区域1：左下角
	if point.Ka < corner1.Ka-defaultEpsilon && point.Kb < corner1.Kb-defaultEpsilon {
		return withResolvedAngles(point, gridMinAngle, gridMinAngle)
	}

	// 区域2：下边
	if bp := resolveBetaBoundary(point, boundary2, gridMinAngle, false); bp != nil {
		return *bp
	}

	// 区域3：右下角
	if point.Ka > corner3.Ka+defaultEpsilon && point.Kb < corner3.Kb-defaultEpsilon {
		return withResolvedAngles(point, gridMaxAngle, gridMinAngle)
	}

	// 区域4：右边
	if bp := resolveAlphaBoundary(point, boundary4, gridMaxAngle, true); bp != nil {
		return *bp
	}

	// 区域5：右上角
	if point.Ka > corner5.Ka+defaultEpsilon && point.Kb > corner5.Kb+defaultEpsilon {
		return withResolvedAngles(point, gridMaxAngle, gridMaxAngle)
	}

	// 区域6：上边
	if bp := resolveBetaBoundary(point, boundary6, gridMaxAngle, true); bp != nil {
		return *bp
	}

	// 区域7：左上角
	if point.Ka < corner7.Ka-defaultEpsilon && point.Kb > corner7.Kb+defaultEpsilon {
		return withResolvedAngles(point, gridMinAngle, gridMaxAngle)
	}

	// 区域8：左边
	if bp := resolveAlphaBoundary(point, boundary8, gridMinAngle, false); bp != nil {
		return *bp
	}

	return point
}

// resolveBetaBoundary 解析β方向的边界
func resolveBetaBoundary(point interpolationPoint, boundary []boundaryPoint, targetBeta float64, isAbove bool) *interpolationPoint {
	for i := 0; i < len(boundary)-1; i++ {
		bp1 := boundary[i]
		bp2 := boundary[i+1]

		// 检查点是否在边界线段的Kα范围内
		minKa := math.Min(bp1.Ka, bp2.Ka)
		maxKa := math.Max(bp1.Ka, bp2.Ka)
		if point.Ka < minKa-defaultEpsilon || point.Ka > maxKa+defaultEpsilon {
			continue
		}

		// 检查点是否在边界的外侧
		if isAbove && point.Kb < math.Max(bp1.Kb, bp2.Kb)+defaultEpsilon {
			continue
		}
		if !isAbove && point.Kb > math.Min(bp1.Kb, bp2.Kb)-defaultEpsilon {
			continue
		}

		// 线性插值计算α
		t := 0.0
		if math.Abs(bp2.Ka-bp1.Ka) > defaultEpsilon {
			t = (point.Ka - bp1.Ka) / (bp2.Ka - bp1.Ka)
		}
		alpha := bp1.Alpha + t*(bp2.Alpha-bp1.Alpha)

		return &interpolationPoint{
			Ka:    point.Ka,
			Kb:    point.Kb,
			Alpha: &alpha,
			Beta:  &targetBeta,
		}
	}
	return nil
}

// resolveAlphaBoundary 解析α方向的边界
func resolveAlphaBoundary(point interpolationPoint, boundary []boundaryPoint, targetAlpha float64, isRight bool) *interpolationPoint {
	for i := 0; i < len(boundary)-1; i++ {
		bp1 := boundary[i]
		bp2 := boundary[i+1]

		minKb := math.Min(bp1.Kb, bp2.Kb)
		maxKb := math.Max(bp1.Kb, bp2.Kb)
		if point.Kb < minKb-defaultEpsilon || point.Kb > maxKb+defaultEpsilon {
			continue
		}

		if isRight && point.Ka < math.Max(bp1.Ka, bp2.Ka)+defaultEpsilon {
			continue
		}
		if !isRight && point.Ka > math.Min(bp1.Ka, bp2.Ka)-defaultEpsilon {
			continue
		}

		t := 0.0
		if math.Abs(bp2.Kb-bp1.Kb) > defaultEpsilon {
			t = (point.Kb - bp1.Kb) / (bp2.Kb - bp1.Kb)
		}
		beta := bp1.Beta + t*(bp2.Beta-bp1.Beta)

		return &interpolationPoint{
			Ka:    point.Ka,
			Kb:    point.Kb,
			Alpha: &targetAlpha,
			Beta:  &beta,
		}
	}
	return nil
}

// resolveRegion9 解析区域9（中心区）
func resolveRegion9(point interpolationPoint, cells []region9Cell) interpolationPoint {
	testPoint := point2D{X: point.Ka, Y: point.Kb}

	for _, cell := range cells {
		if !isPointInsideConvexQuad(testPoint, cell.Vertices) {
			continue
		}

		line12 := createLineThroughPoints(cell.Vertices[0], cell.Vertices[1])
		line23 := createLineThroughPoints(cell.Vertices[1], cell.Vertices[2])
		line34 := createLineThroughPoints(cell.Vertices[2], cell.Vertices[3])
		line41 := createLineThroughPoints(cell.Vertices[3], cell.Vertices[0])

		if line12 == nil || line23 == nil || line34 == nil || line41 == nil {
			continue
		}

		beta := interpolateBetweenParallelEdges(
			cell.X1.Beta, cell.X4.Beta,
			distanceToLine(testPoint, *line12),
			distanceToLine(testPoint, *line34),
		)
		alpha := interpolateBetweenParallelEdges(
			cell.X1.Alpha, cell.X2.Alpha,
			distanceToLine(testPoint, *line41),
			distanceToLine(testPoint, *line23),
		)

		return withResolvedAngles(point, alpha, beta)
	}

	return point
}

// ==================== 输出值插值 ====================

func interpolateOutputValue(point interpolationPoint, table *indexedCalibrationTable, selector func(probeTableRow) float64) float64 {
	if point.Alpha == nil || point.Beta == nil {
		return 0
	}

	alpha1, alpha2 := resolveOutputCellAxis(*point.Alpha)
	beta1, beta2 := resolveOutputCellAxis(*point.Beta)

	x1, err := table.GetExactGridPointOrThrow(alpha1, beta1)
	if err != nil {
		return 0
	}
	x2, err := table.GetExactGridPointOrThrow(alpha2, beta1)
	if err != nil {
		return 0
	}
	x3, err := table.GetExactGridPointOrThrow(alpha2, beta2)
	if err != nil {
		return 0
	}
	x4, err := table.GetExactGridPointOrThrow(alpha1, beta2)
	if err != nil {
		return 0
	}

	lowerEdge := interpolateValue(selector(x1), selector(x2), interpolateFactor(*point.Alpha, x1.Alpha, x2.Alpha))
	upperEdge := interpolateValue(selector(x4), selector(x3), interpolateFactor(*point.Alpha, x4.Alpha, x3.Alpha))

	return interpolateValue(lowerEdge, upperEdge, interpolateFactor(*point.Beta, x1.Beta, x4.Beta))
}

func resolveOutputCellAxis(angle float64) (float64, float64) {
	truncatedIndex := math.Trunc(angle / gridStep)

	if (angle >= 0 && angle < gridMaxAngle) || angle <= gridMinAngle {
		return gridStep * truncatedIndex, gridStep * (truncatedIndex + 1)
	}
	return gridStep * (truncatedIndex - 1), gridStep * truncatedIndex
}

// ==================== 几何计算工具 ====================

func createBoundaryPoints(
	table []probeTableRow,
	predicate func(probeTableRow) bool,
	angleSelector func(probeTableRow) float64,
) []boundaryPoint {
	// 使用整数作为 map key 避免浮点精度问题
	boundaryByAngle := make(map[int]boundaryPoint)

	for _, row := range table {
		if !predicate(row) {
			continue
		}
		angle := angleSelector(row)
		angleKey := int(math.Round(angle * 1000))
		boundaryByAngle[angleKey] = boundaryPoint{
			Ka:    row.Ka,
			Kb:    row.Kb,
			Alpha: row.Alpha,
			Beta:  row.Beta,
		}
	}

	var boundary []boundaryPoint
	for a := float64(gridMinAngle); a <= gridMaxAngle; a += gridStep {
		angleKey := int(math.Round(a * 1000))
		if bp, ok := boundaryByAngle[angleKey]; ok {
			boundary = append(boundary, bp)
		}
	}
	return boundary
}

func createRegion9Cells(getExact func(float64, float64) (probeTableRow, error)) []region9Cell {
	var cells []region9Cell
	angles := createGridAngles()

	for i := 0; i < len(angles)-1; i++ {
		for j := 0; j < len(angles)-1; j++ {
			x1, err := getExact(angles[i], angles[j])
			if err != nil {
				continue
			}
			x2, err := getExact(angles[i+1], angles[j])
			if err != nil {
				continue
			}
			x3, err := getExact(angles[i+1], angles[j+1])
			if err != nil {
				continue
			}
			x4, err := getExact(angles[i], angles[j+1])
			if err != nil {
				continue
			}

			cells = append(cells, region9Cell{
				X1: x1, X2: x2, X3: x3, X4: x4,
				Vertices: [4]point2D{
					{X: x1.Ka, Y: x1.Kb},
					{X: x2.Ka, Y: x2.Kb},
					{X: x3.Ka, Y: x3.Kb},
					{X: x4.Ka, Y: x4.Kb},
				},
			})
		}
	}
	return cells
}

func isPointInsideConvexQuad(point point2D, vertices [4]point2D) bool {
	for i := 0; i < len(vertices); i++ {
		start := vertices[i]
		end := vertices[(i+1)%len(vertices)]
		line := createLineThroughPoints(start, end)
		if line == nil {
			return false
		}

		refDist := resolveReferenceDistance(*line, vertices, i)
		if refDist == nil {
			return false
		}

		pointDist := signedDistanceToLine(point, *line)
		if *refDist > defaultEpsilon && pointDist < -defaultEpsilon {
			return false
		}
		if *refDist < -defaultEpsilon && pointDist > defaultEpsilon {
			return false
		}
	}
	return true
}

func resolveReferenceDistance(line lineEquation, vertices [4]point2D, edgeIndex int) *float64 {
	primaryRef := signedDistanceToLine(vertices[(edgeIndex+2)%4], line)
	if math.Abs(primaryRef) > defaultEpsilon {
		return &primaryRef
	}
	secondaryRef := signedDistanceToLine(vertices[(edgeIndex+3)%4], line)
	if math.Abs(secondaryRef) > defaultEpsilon {
		return &secondaryRef
	}
	return nil
}

func createLineThroughPoints(start, end point2D) *lineEquation {
	A := start.Y - end.Y
	B := end.X - start.X
	C := start.X*end.Y - end.X*start.Y
	normalLen := math.Hypot(A, B)

	if normalLen == 0 {
		return nil
	}
	return &lineEquation{A: A, B: B, C: C, NormalLen: normalLen}
}

func distanceToLine(point point2D, line lineEquation) float64 {
	return math.Abs(signedDistanceToLine(point, line))
}

func signedDistanceToLine(point point2D, line lineEquation) float64 {
	return (line.A*point.X + line.B*point.Y + line.C) / line.NormalLen
}

// ==================== 辅助函数 ====================

func withResolvedAngles(point interpolationPoint, alpha, beta float64) interpolationPoint {
	return interpolationPoint{
		Ka:    point.Ka,
		Kb:    point.Kb,
		Alpha: &alpha,
		Beta:  &beta,
	}
}

func clampPressureDelta(delta float64) float64 {
	if math.Abs(delta) >= minPressureDelta {
		return delta
	}
	if delta == 0 {
		return minPressureDelta
	}
	return math.Copysign(minPressureDelta, delta)
}

func calculatePressureDelta(input runtimeInput) (avg, delta float64) {
	p1, p2, p3, p4, p5 := input.FiveHoleData[0], input.FiveHoleData[1], input.FiveHoleData[2], input.FiveHoleData[3], input.FiveHoleData[4]
	avg = (p1 + p3 + p4 + p5) * 0.25
	delta = clampPressureDelta(p2 - avg)
	return
}

// calculateVelocity 计算真空速（使用 AtmosphericDataCalculator 气压静温密度法）
// 需要绝对总压、绝对静压和总温（开氏温度）
func calculateVelocity(calc *AtmosphericDataCalculator, input runtimeInput, pt, ps float64) float64 {
	absPt := pt + input.AtmP
	absPs := ps + input.AtmP
	tempK := input.AtmT + 273.15

	if absPs <= 0 || tempK <= 0 || absPt <= absPs {
		return 0
	}

	ma, err := calc.CalculateMach(absPt, absPs)
	if err != nil {
		return 0
	}

	sat := calc.CalculateSAT(tempK, ma)
	qc := calc.CalculateQc(absPt, absPs)
	return calc.CalculateTASByDensity(absPs, qc, sat)
}

// calculateMachFromPressures 计算马赫数（使用 AtmosphericDataCalculator）
func calculateMachFromPressures(calc *AtmosphericDataCalculator, input runtimeInput, pt, ps float64) float64 {
	absPt := pt + input.AtmP
	absPs := ps + input.AtmP
	if absPs <= 0 || absPt <= absPs {
		return 0
	}

	ma, err := calc.CalculateMach(absPt, absPs)
	if err != nil {
		return 0
	}
	return ma
}

func interpolateFactor(value, start, end float64) float64 {
	if start == end {
		return 0
	}
	return (value - start) / (end - start)
}

func interpolateValue(start, end, factor float64) float64 {
	return start + (end-start)*factor
}

func interpolateBetweenParallelEdges(startVal, endVal, distToStart, distToEnd float64) float64 {
	totalDist := distToStart + distToEnd
	if totalDist == 0 {
		return startVal
	}
	return startVal + ((endVal-startVal)*distToStart)/totalDist
}

// ==================== 输入输出转换 ====================

func toRuntimeInput(input InterpolationInput) runtimeInput {
	return runtimeInput{
		AtmP:         input.PAtm,
		AtmT:         input.TAtm,
		FiveHoleData: [5]float64{input.P1, input.P2, input.P3, input.P4, input.P5},
	}
}

func collectInputWarnings(input runtimeInput) []string {
	p1, p2, p3, p4, p5 := input.FiveHoleData[0], input.FiveHoleData[1], input.FiveHoleData[2], input.FiveHoleData[3], input.FiveHoleData[4]
	avg := (p1 + p3 + p4 + p5) * 0.25
	delta := p2 - avg

	var warnings []string
	if math.Abs(delta) < minPressureDelta {
		warnings = append(warnings, "参考压力差接近零，插值使用了最小压力差钳位")
	}
	return warnings
}

func toInterpolationResult(result interResult, input runtimeInput, validRange PrbValidRange, warnings []string, atmCalc *AtmosphericDataCalculator) InterpolationResult {
	dynamicPressure := result.Pt - result.Ps
	tempK := input.AtmT + 273.15
	absPt := input.AtmP + result.Pt
	absPs := input.AtmP + result.Ps
	vx, vy, vz := calculateVelocityComponents(result.V, result.Alpha, result.Beta)

	var density float64
	if isFinite(absPs) && absPs > 0 && tempK > 0 {
		density = absPs / (gasConstantAir * tempK)
	}

	// 使用复用的 AtmosphericDataCalculator 计算 CAS 和 SAT
	var cas float64
	var sat float64
	if absPs > 0 && absPt > absPs && tempK > 0 {
		if ma, err := atmCalc.CalculateMach(absPt, absPs); err == nil {
			sat = atmCalc.CalculateSAT(tempK, ma)
			qc := atmCalc.CalculateQc(absPt, absPs)
			cas = atmCalc.CalculateCAS(qc)
		}
	}

	isValid := true

	if !isFinite(result.Alpha) || !isFinite(result.Beta) || !isFinite(result.V) || !isFinite(result.Ma) {
		warnings = appendUnique(warnings, "插值返回非有限输出")
		isValid = false
	}
	if !isFinite(dynamicPressure) {
		warnings = appendUnique(warnings, "解析动压不是有限值")
		isValid = false
	}
	if isFinite(dynamicPressure) && dynamicPressure <= 0 {
		warnings = appendUnique(warnings, "总压低于静压 (pt < ps)")
		isValid = false
	}
	if !isWithinRange(result.Alpha, validRange.AlphaMin, validRange.AlphaMax) {
		warnings = appendUnique(warnings, "解析攻角超出PRB表范围")
		isValid = false
	}
	if !isWithinRange(result.Beta, validRange.BetaMin, validRange.BetaMax) {
		warnings = appendUnique(warnings, "解析侧滑角超出PRB表范围")
		isValid = false
	}

	var warningStr string
	if len(warnings) > 0 {
		warningStr = strings.Join(warnings, "; ")
	}

	return InterpolationResult{
		Alpha:           result.Alpha,
		Beta:            result.Beta,
		MachNumber:      result.Ma,
		V:               result.V,
		Vx:              vx,
		Vy:              vy,
		Vz:              vz,
		Velocity:        result.V,
		CAS:             ternary(isFinite(cas), cas, 0),
		SAT:             ternary(isFinite(sat), sat, 0),
		DynamicPressure: ternary(isFinite(dynamicPressure), dynamicPressure, 0),
		Density:         ternary(isFinite(density) && density > 0, density, 0),
		TotalPressure:   ternary(isFinite(result.Pt), result.Pt, 0),
		StaticPressure:  ternary(isFinite(result.Ps), result.Ps, 0),
		IsValid:         isValid,
		Warning:         warningStr,
	}
}

func calculateVelocityComponents(v, alphaDeg, betaDeg float64) (vx, vy, vz float64) {
	if !isFinite(v) || !isFinite(alphaDeg) || !isFinite(betaDeg) {
		return 0, 0, 0
	}

	alpha := alphaDeg * math.Pi / 180
	beta := betaDeg * math.Pi / 180

	vx = v * math.Cos(beta) * math.Sin(alpha)
	vy = v * math.Sin(beta)
	vz = v * math.Cos(beta) * math.Cos(alpha)
	return vx, vy, vz
}

func appendUnique(warnings []string, msg string) []string {
	for _, w := range warnings {
		if w == msg {
			return warnings
		}
	}
	return append(warnings, msg)
}

func isWithinRange(value, min, max float64) bool {
	return value >= min-defaultEpsilon && value <= max+defaultEpsilon
}

func ternary(cond bool, ifTrue, ifFalse float64) float64 {
	if cond {
		return ifTrue
	}
	return ifFalse
}
