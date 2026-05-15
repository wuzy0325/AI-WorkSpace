package interpolation

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
)

// PRB 网格常量
const (
	GridSize    = 13                  // 每轴格点数
	AngleMin    = -30                 // 最小角度
	AngleMax    = 30                  // 最大角度
	AngleStep   = 5                   // 角度步长
	TotalPoints = GridSize * GridSize // 169
)

// ProbeTableRow PRB 表中的一行数据
type ProbeTableRow struct {
	Ka    float64 `json:"ka"`
	Kb    float64 `json:"kb"`
	Cpt   float64 `json:"cpt"`
	Cps   float64 `json:"cps"`
	Alpha float64 `json:"alpha"`
	Beta  float64 `json:"beta"`
}

// PrbTable PRB 校准表
type PrbTable struct {
	Rows     []ProbeTableRow          `json:"rows"`
	Index    map[string]ProbeTableRow `json:"-"` // (alpha,beta) -> row
	Mach     float64                  `json:"mach"`
	FilePath string                   `json:"filePath"`
}

// InterpolationResult 插值结果
type InterpolationResult struct {
	Alpha           float64 `json:"alpha"`
	Beta            float64 `json:"beta"`
	MachNumber      float64 `json:"machNumber"`
	Velocity        float64 `json:"velocity"`
	DynamicPressure float64 `json:"dynamicPressure"`
	Density         float64 `json:"density"`
	P0              float64 `json:"p0,omitempty"`
	Ps              float64 `json:"ps,omitempty"`
	IsValid         bool    `json:"isValid"`
	Warning         string  `json:"warning,omitempty"`
}

// TraversalInterpolationInput 插值输入
type TraversalInterpolationInput struct {
	P1   float64 `json:"P1"`
	P2   float64 `json:"P2"`
	P3   float64 `json:"P3"`
	P4   float64 `json:"P4"`
	P5   float64 `json:"P5"`
	Patm float64 `json:"Patm"`
	Tatm float64 `json:"Tatm"`
}

// PrbInterpolator 单 PRB 文件插值器
type PrbInterpolator struct {
	table  *PrbTable
	loaded bool
}

// NewPrbInterpolator 创建插值器
func NewPrbInterpolator() *PrbInterpolator {
	return &PrbInterpolator{}
}

// LoadPrbFile 加载 PRB 文件
func (p *PrbInterpolator) LoadPrbFile(content string, filePath string) error {
	table, err := parsePrbFile(content, filePath)
	if err != nil {
		return err
	}
	p.table = table
	p.loaded = true
	return nil
}

// IsLoaded 是否已加载
func (p *PrbInterpolator) IsLoaded() bool {
	return p.loaded
}

// Calculate 执行插值计算
func (p *PrbInterpolator) Calculate(input TraversalInterpolationInput) (InterpolationResult, error) {
	if !p.loaded {
		return InterpolationResult{}, fmt.Errorf("PRB file not loaded")
	}

	// Step 1: 计算压力系数
	ka, kb, delta := calculatePressureCoefficients(input)

	// Step 2-3: 区域判定与角度求解
	alpha, beta, err := resolveAngle(p.table, ka, kb)
	if err != nil {
		return InterpolationResult{IsValid: false, Warning: err.Error()}, nil
	}

	// Step 4: 双线性插值输出值
	cpt, cps := interpolateOutputValues(p.table, alpha, beta)

	// Step 5: 物理量计算
	result := calculatePhysics(alpha, beta, cpt, cps, delta, input)

	return result, nil
}

// GetValidRange 获取有效范围
func (p *PrbInterpolator) GetValidRange() (alphaMin, alphaMax, betaMin, betaMax float64) {
	return float64(AngleMin), float64(AngleMax), float64(AngleMin), float64(AngleMax)
}

// parsePrbFile 解析 PRB 文件
func parsePrbFile(content string, filePath string) (*PrbTable, error) {
	lines := strings.Split(strings.TrimSpace(content), "\n")
	if len(lines) < 2 {
		return nil, fmt.Errorf("PRB file too short")
	}

	// 解析首行: "13 13"
	parts := strings.Fields(lines[0])
	if len(parts) < 2 {
		return nil, fmt.Errorf("invalid PRB header")
	}
	n1, _ := strconv.Atoi(parts[0])
	n2, _ := strconv.Atoi(parts[1])
	if n1 != GridSize || n2 != GridSize {
		return nil, fmt.Errorf("expected grid %dx%d, got %dx%d", GridSize, GridSize, n1, n2)
	}

	// 解析数据行
	rows := make([]ProbeTableRow, 0, TotalPoints)
	for i := 1; i < len(lines) && len(rows) < TotalPoints; i++ {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 6 {
			continue
		}
		vals := make([]float64, 6)
		for j := 0; j < 6; j++ {
			v, err := strconv.ParseFloat(fields[j], 64)
			if err != nil {
				return nil, fmt.Errorf("line %d: parse error: %w", i+1, err)
			}
			vals[j] = v
		}
		rows = append(rows, ProbeTableRow{
			Ka:    vals[0],
			Kb:    vals[1],
			Cpt:   vals[2],
			Cps:   vals[3],
			Alpha: vals[4],
			Beta:  vals[5],
		})
	}

	if len(rows) != TotalPoints {
		return nil, fmt.Errorf("expected %d rows, got %d", TotalPoints, len(rows))
	}

	// 构建索引
	index := make(map[string]ProbeTableRow, TotalPoints)
	for _, row := range rows {
		key := fmt.Sprintf("%.0f,%.0f", row.Alpha, row.Beta)
		index[key] = row
	}

	mach := parseMachFromFileName(filePath)

	return &PrbTable{
		Rows:     rows,
		Index:    index,
		Mach:     mach,
		FilePath: filePath,
	}, nil
}

// parseMachFromFileName 从文件名解析马赫数
var machRegex = regexp.MustCompile(`(\d+\.?\d*)\s*Ma`)

func parseMachFromFileName(filePath string) float64 {
	match := machRegex.FindStringSubmatch(filePath)
	if len(match) >= 2 {
		if v, err := strconv.ParseFloat(match[1], 64); err == nil {
			return v
		}
	}
	return 0
}

// calculatePressureCoefficients 计算压力系数
func calculatePressureCoefficients(input TraversalInterpolationInput) (ka, kb, delta float64) {
	// delta = P2 - (P1+P3+P4+P5)/4, clamp to avoid division by zero
	delta = input.P2 - (input.P1+input.P3+input.P4+input.P5)/4.0
	if math.Abs(delta) < 1e-10 {
		delta = 1e-10
	}
	ka = (input.P4 - input.P5) / delta
	kb = (input.P3 - input.P1) / delta
	return
}

// resolveAngle 区域判定与角度求解 (9区域法)
func resolveAngle(table *PrbTable, ka, kb float64) (alpha, beta float64, err error) {
	// 尝试区域 1-8 (边界区域)
	if a, b, ok := resolveRegions1To8(table, ka, kb); ok {
		return a, b, nil
	}

	// 区域 9: 内部区域
	a, b, ok := resolveRegion9(table, ka, kb)
	if !ok {
		return 0, 0, fmt.Errorf("point (%.4f, %.4f) outside PRB table range", ka, kb)
	}
	return a, b, nil
}

// resolveRegions1To8 处理边界区域 1-8
func resolveRegions1To8(table *PrbTable, ka, kb float64) (alpha, beta float64, ok bool) {
	// 获取边界网格点
	getRow := func(a, b float64) (ProbeTableRow, bool) {
		key := fmt.Sprintf("%.0f,%.0f", a, b)
		r, exists := table.Index[key]
		return r, exists
	}

	// 检查四个角点 (区域 1,3,5,7)
	corners := []struct{ a, b float64 }{
		{-30, -30}, {-30, 30}, {30, -30}, {30, 30},
	}
	for _, c := range corners {
		if r, exists := getRow(c.a, c.b); exists {
			if math.Abs(ka-r.Ka) < 0.01 && math.Abs(kb-r.Kb) < 0.01 {
				return r.Alpha, r.Beta, true
			}
		}
	}

	// 检查边界 (区域 2,4,6,8) - 沿边界线性插值
	// 上边界 beta=30
	if a, b, found := interpolateAlongBoundary(table, ka, kb, 30, true); found {
		return a, b, true
	}
	// 下边界 beta=-30
	if a, b, found := interpolateAlongBoundary(table, ka, kb, -30, true); found {
		return a, b, true
	}
	// 左边界 alpha=-30
	if a, b, found := interpolateAlongBoundary(table, ka, kb, -30, false); found {
		return a, b, true
	}
	// 右边界 alpha=30
	if a, b, found := interpolateAlongBoundary(table, ka, kb, 30, false); found {
		return a, b, true
	}

	return 0, 0, false
}

// interpolateAlongBoundary 沿边界插值
func interpolateAlongBoundary(table *PrbTable, ka, kb, fixedVal float64, isBeta bool) (alpha, beta float64, found bool) {
	var prevRow *ProbeTableRow
	for i := 0; i < GridSize; i++ {
		var a, b float64
		if isBeta {
			a = float64(AngleMin + i*AngleStep)
			b = fixedVal
		} else {
			a = fixedVal
			b = float64(AngleMin + i*AngleStep)
		}

		key := fmt.Sprintf("%.0f,%.0f", a, b)
		row, exists := table.Index[key]
		if !exists {
			prevRow = nil
			continue
		}

		if prevRow != nil {
			// 检查 (ka,kb) 是否在 prevRow 和 row 之间的线段上
			kaMin := math.Min(prevRow.Ka, row.Ka)
			kaMax := math.Max(prevRow.Ka, row.Ka)
			kbMin := math.Min(prevRow.Kb, row.Kb)
			kbMax := math.Max(prevRow.Kb, row.Kb)

			if ka >= kaMin && ka <= kaMax && kb >= kbMin && kb <= kbMax {
				// 线性插值
				dKa := row.Ka - prevRow.Ka
				dKb := row.Kb - prevRow.Kb
				var t float64
				if math.Abs(dKa) > math.Abs(dKb) {
					t = (ka - prevRow.Ka) / dKa
				} else if math.Abs(dKb) > 1e-10 {
					t = (kb - prevRow.Kb) / dKb
				} else {
					t = 0.5
				}
				t = math.Max(0, math.Min(1, t))

				alpha = prevRow.Alpha + t*(row.Alpha-prevRow.Alpha)
				beta = prevRow.Beta + t*(row.Beta-prevRow.Beta)
				return alpha, beta, true
			}
		}
		r := row
		prevRow = &r
	}
	return 0, 0, false
}

// resolveRegion9 区域9内部插值
func resolveRegion9(table *PrbTable, ka, kb float64) (alpha, beta float64, ok bool) {
	// 遍历 12x12 个四边形单元格
	for i := 0; i < GridSize-1; i++ {
		for j := 0; j < GridSize-1; j++ {
			a1 := float64(AngleMin + i*AngleStep)
			a2 := float64(AngleMin + (i+1)*AngleStep)
			b1 := float64(AngleMin + j*AngleStep)
			b2 := float64(AngleMin + (j+1)*AngleStep)

			// 四个角点
			x1, ok1 := table.Index[fmt.Sprintf("%.0f,%.0f", a1, b1)]
			x2, ok2 := table.Index[fmt.Sprintf("%.0f,%.0f", a2, b1)]
			x3, ok3 := table.Index[fmt.Sprintf("%.0f,%.0f", a2, b2)]
			x4, ok4 := table.Index[fmt.Sprintf("%.0f,%.0f", a1, b2)]

			if !ok1 || !ok2 || !ok3 || !ok4 {
				continue
			}

			// 判断点是否在凸四边形内
			if !isPointInsideConvexQuad(ka, kb, x1.Ka, x1.Kb, x2.Ka, x2.Kb, x3.Ka, x3.Kb, x4.Ka, x4.Kb) {
				continue
			}

			// 通过到对边的距离比进行插值
			distToLine12 := pointToLineDistance(ka, kb, x1.Ka, x1.Kb, x2.Ka, x2.Kb)
			distToLine34 := pointToLineDistance(ka, kb, x3.Ka, x3.Kb, x4.Ka, x4.Kb)
			distToLine41 := pointToLineDistance(ka, kb, x4.Ka, x4.Kb, x1.Ka, x1.Kb)
			distToLine23 := pointToLineDistance(ka, kb, x2.Ka, x2.Kb, x3.Ka, x3.Kb)

			totalBeta := distToLine12 + distToLine34
			totalAlpha := distToLine41 + distToLine23

			if totalBeta > 1e-10 {
				beta = x1.Beta + (distToLine12/totalBeta)*(x4.Beta-x1.Beta)
			} else {
				beta = (x1.Beta + x4.Beta) / 2
			}

			if totalAlpha > 1e-10 {
				alpha = x1.Alpha + (distToLine41/totalAlpha)*(x2.Alpha-x1.Alpha)
			} else {
				alpha = (x1.Alpha + x2.Alpha) / 2
			}

			return alpha, beta, true
		}
	}
	return 0, 0, false
}

// isPointInsideConvexQuad 判断点是否在凸四边形内
func isPointInsideConvexQuad(px, py, x1, y1, x2, y2, x3, y3, x4, y4 float64) bool {
	d1 := signDistance(px, py, x1, y1, x2, y2)
	d2 := signDistance(px, py, x2, y2, x3, y3)
	d3 := signDistance(px, py, x3, y3, x4, y4)
	d4 := signDistance(px, py, x4, y4, x1, y1)

	hasPos := d1 > 0 || d2 > 0 || d3 > 0 || d4 > 0
	hasNeg := d1 < 0 || d2 < 0 || d3 < 0 || d4 < 0

	return !(hasPos && hasNeg)
}

// signDistance 点到直线的符号距离
func signDistance(px, py, x1, y1, x2, y2 float64) float64 {
	return (px-x1)*(y2-y1) - (py-y1)*(x2-x1)
}

// pointToLineDistance 点到直线的距离
func pointToLineDistance(px, py, x1, y1, x2, y2 float64) float64 {
	dx := x2 - x1
	dy := y2 - y1
	length := math.Sqrt(dx*dx + dy*dy)
	if length < 1e-10 {
		return math.Sqrt((px-x1)*(px-x1) + (py-y1)*(py-y1))
	}
	return math.Abs((py-y1)*dx-(px-x1)*dy) / length
}

// interpolateOutputValues 双线性插值输出值
func interpolateOutputValues(table *PrbTable, alpha, beta float64) (cpt, cps float64) {
	// 确定网格位置
	i := int(math.Round((alpha - float64(AngleMin)) / float64(AngleStep)))
	j := int(math.Round((beta - float64(AngleMin)) / float64(AngleStep)))

	i = clampInt(i, 0, GridSize-1)
	j = clampInt(j, 0, GridSize-1)

	// 获取四个相邻网格点
	i0, i1 := clampInt(i-1, 0, GridSize-1), clampInt(i+1, 0, GridSize-1)
	j0, j1 := clampInt(j-1, 0, GridSize-1), clampInt(j+1, 0, GridSize-1)

	a0 := float64(AngleMin + i0*AngleStep)
	a1 := float64(AngleMin + i1*AngleStep)
	b0 := float64(AngleMin + j0*AngleStep)
	b1 := float64(AngleMin + j1*AngleStep)

	r00 := table.Index[fmt.Sprintf("%.0f,%.0f", a0, b0)]
	r10 := table.Index[fmt.Sprintf("%.0f,%.0f", a1, b0)]
	r01 := table.Index[fmt.Sprintf("%.0f,%.0f", a0, b1)]
	r11 := table.Index[fmt.Sprintf("%.0f,%.0f", a1, b1)]

	// 双线性插值
	da := a1 - a0
	db := b1 - b0
	ta := 0.5
	tb := 0.5
	if da > 1e-10 {
		ta = (alpha - a0) / da
	}
	if db > 1e-10 {
		tb = (beta - b0) / db
	}
	ta = math.Max(0, math.Min(1, ta))
	tb = math.Max(0, math.Min(1, tb))

	cpt = bilinear(r00.Cpt, r10.Cpt, r01.Cpt, r11.Cpt, ta, tb)
	cps = bilinear(r00.Cps, r10.Cps, r01.Cps, r11.Cps, ta, tb)
	return
}

// bilinear 双线性插值
func bilinear(v00, v10, v01, v11, t, s float64) float64 {
	return v00*(1-t)*(1-s) + v10*t*(1-s) + v01*(1-t)*s + v11*t*s
}

// calculatePhysics 计算物理量
func calculatePhysics(alpha, beta, cpt, cps, delta float64, input TraversalInterpolationInput) InterpolationResult {
	const R = 287.06 // 空气气体常数

	// 总压和静压
	Pt := input.P2 - cpt*delta
	Ps := input.Patm + (input.P2 - (input.P1+input.P3+input.P4+input.P5)/4.0) - cps*delta

	// 动压
	dynamicPressure := Pt - Ps

	// 速度
	var velocity float64
	if dynamicPressure > 0 {
		T_K := input.Tatm + 273.15
		velocity = math.Sqrt(2 * math.Abs(dynamicPressure) * R * T_K / (input.Patm + Ps))
	}

	// 马赫数
	var machNumber float64
	if Ps+input.Patm > 0 && Pt+input.Patm > 0 {
		ratio := (Pt + input.Patm) / (Ps + input.Patm)
		if ratio > 0 {
			machNumber = math.Sqrt(5 * math.Abs(math.Pow(ratio, 2.0/7.0)-1))
		}
	}

	// 密度
	T_K := input.Tatm + 273.15
	var density float64
	if T_K > 0 {
		density = (input.Patm + Ps) / (R * T_K)
	}

	// 验证
	isValid := true
	var warning string
	if math.Abs(alpha) > float64(AngleMax) || math.Abs(beta) > float64(AngleMax) {
		isValid = false
		warning = "angle out of PRB table range"
	}
	if dynamicPressure <= 0 {
		isValid = false
		if warning != "" {
			warning += "; "
		}
		warning += "non-positive dynamic pressure"
	}

	return InterpolationResult{
		Alpha:           alpha,
		Beta:            beta,
		MachNumber:      machNumber,
		Velocity:        velocity,
		DynamicPressure: dynamicPressure,
		Density:         density,
		P0:              Pt,
		Ps:              Ps,
		IsValid:         isValid,
		Warning:         warning,
	}
}

func clampInt(v, min, max int) int {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}
