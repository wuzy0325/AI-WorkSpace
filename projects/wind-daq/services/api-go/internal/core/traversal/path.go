package traversal

import (
	"fmt"
	"math"
	"sort"
	"strings"
)

// 走线主轴常量：避免 "x"/"y" 魔法字符串跨层散落，拼写错误难排查。
// PrimaryAxisLegacy 仅用于内部“未设置”语义，外部持久化/协议不应使用此值。
const (
	PrimaryAxisX      = "x" // 先沿 X 方向走完一条线再切换 Y
	PrimaryAxisY      = "y" // 先沿 Y 方向走完一条线再切换 X（原始行为）
	PrimaryAxisLegacy = ""  // 缺省：保旧行为（先走 Y），用于升级前保存的 profile 兼容
)

// InterpolateLinearPath 沿折线插值生成路径点
func InterpolateLinearPath(points []Point, step float64) ([]Point, error) {
	if step <= 0 {
		return nil, fmt.Errorf("step must be greater than zero")
	}
	if len(points) < 2 {
		return nil, fmt.Errorf("at least two points are required")
	}

	path := []Point{points[0]}
	for i := 0; i < len(points)-1; i++ {
		from := points[i]
		to := points[i+1]
		segments := maxSegments(from, to, step)
		if segments == 0 {
			continue
		}
		for segment := 1; segment <= segments; segment++ {
			ratio := float64(segment) / float64(segments)
			// 4 轴线性插值：U 轴与 X/Y/Z 同等对待，避免旋转/第四轴位移被静默丢弃
			path = append(path, Point{
				X: from.X + (to.X-from.X)*ratio,
				Y: from.Y + (to.Y-from.Y)*ratio,
				Z: from.Z + (to.Z-from.Z)*ratio,
				U: from.U + (to.U-from.U)*ratio,
			})
		}
	}
	return path, nil
}

// maxSegments 计算两点间各轴所需的插值段数，取最大值。
//
// 设计动机：原先使用 distance() 计算 4 轴欧氏距离再除以 step 得到段数，
// 但 X/Y/Z（mm）与 U（角度）物理量纲不同，混合求距在数学上无意义，
// 可能导致某些轴插值段数不足（如纯 U 轴旋转 90° + step=1mm → d=90 → 90 段，
// 但若 X 轴位移 100mm + U 轴旋转 0.5° → d≈100 → 100 段，U 轴分辨率过高）。
//
// 按轴分别计算段数取 max，确保每个轴都获得足够的插值点，
// 同时避免混合量纲导致的距离无意义问题。
// step 的单位由调用方根据主运动轴选择（通常为 mm）。
func maxSegments(a, b Point, step float64) int {
	if step <= 0 {
		return 1
	}
	dx := math.Abs(b.X - a.X)
	dy := math.Abs(b.Y - a.Y)
	dz := math.Abs(b.Z - a.Z)
	du := math.Abs(b.U - a.U)
	sx := int(math.Ceil(dx / step))
	sy := int(math.Ceil(dy / step))
	sz := int(math.Ceil(dz / step))
	su := int(math.Ceil(du / step))
	max := sx
	for _, s := range [3]int{sy, sz, su} {
		if s > max {
			max = s
		}
	}
	return max
}

// GenerateGridPath 生成网格路径（旧接口兼容）
func GenerateGridPath(cfg GridConfig) ([]Point, error) {
	if cfg.XStep <= 0 || cfg.YStep <= 0 {
		return nil, fmt.Errorf("step must be greater than zero")
	}
	if cfg.XStart > cfg.XEnd || cfg.YStart > cfg.YEnd {
		return nil, fmt.Errorf("start must be less than or equal to end")
	}
	xSteps := int(math.Round((cfg.XEnd - cfg.XStart) / cfg.XStep))
	ySteps := int(math.Round((cfg.YEnd - cfg.YStart) / cfg.YStep))
	path := make([]Point, 0, (xSteps+1)*(ySteps+1))
	for xi := 0; xi <= xSteps; xi++ {
		x := cfg.XStart + float64(xi)*cfg.XStep
		for yi := 0; yi <= ySteps; yi++ {
			y := cfg.YStart + float64(yi)*cfg.YStep
			path = append(path, Point{X: x, Y: y, Z: cfg.ZStart})
		}
	}
	return path, nil
}

// StepSegment 步进段定义
type StepSegment struct {
	Start float64 `json:"start"`
	End   float64 `json:"end"`
	Step  float64 `json:"step"`
}

// StepValues 从多个步进段生成步进值序列
func StepValues(start, end float64, segments []StepSegment) []float64 {
	var values []float64
	for _, segment := range segments {
		if segment.Step <= 0 {
			continue
		}
		actualStart := math.Max(segment.Start, start)
		actualEnd := math.Min(segment.End, end)
		for value := actualStart; value <= actualEnd+1e-9; value += segment.Step {
			if !ContainsFloat(values, value) {
				values = append(values, value)
			}
		}
	}
	if len(values) == 0 {
		if start == end {
			return []float64{start}
		}
		return []float64{start, end}
	}
	return values
}

// GridPointsFromAxes 从 X/Y 轴值生成网格点
// 注意：此函数保留“外层 X、内层 Y”的原始遍历顺序（即物理上先沿 Y 方向走完一列再切换 X）。
// 调用方需要切换主轴顺序时请使用 GridPointsFromAxesOrdered。
func GridPointsFromAxes(xs, ys []float64) []Point {
	points := make([]Point, 0, len(xs)*len(ys))
	for _, x := range xs {
		for _, y := range ys {
			points = append(points, Point{X: x, Y: y})
		}
	}
	return points
}

// GridPointsFromAxesSnake 从 X/Y 轴值生成蛇形网格点
// 蛇形遍历：偶数行（从0开始）正常顺序，奇数行反转Y轴顺序，减少回程时间
// 注意：此函数保留“外层 X、内层 Y”的原始蛇形顺序。需要切换主轴时使用 GridPointsFromAxesSnakeOrdered。
func GridPointsFromAxesSnake(xs, ys []float64) []Point {
	points := make([]Point, 0, len(xs)*len(ys))
	for i, x := range xs {
		if i%2 == 1 {
			// 奇数行反转Y轴顺序
			for j := len(ys) - 1; j >= 0; j-- {
				points = append(points, Point{X: x, Y: ys[j]})
			}
		} else {
			for _, y := range ys {
				points = append(points, Point{X: x, Y: y})
			}
		}
	}
	return points
}

// GridPointsFromAxesOrdered 按指定主轴顺序生成网格点
// primaryAxis 取值（归一化：大小写不敏感、去空白）：
//   - PrimaryAxisX ("x")：先走 X 方向，外层 Y、内层 X，每条线沿 X 走完再切换 Y
//   - PrimaryAxisY ("y")：先走 Y 方向，外层 X、内层 Y，等价于 GridPointsFromAxes
//   - PrimaryAxisLegacy ("") 或其他未识别值：保旧行为（先走 Y），用于升级前保存的
//     profile 兼容——避免旧配置缺省时静默反转物理走线方向。
//
// 实现上 'x' 分支复用 GridPointsFromAxes(ys, xs) 后转置坐标，避免重复双重循环逻辑。
// 路径生成非热路径，多一次遍历的开销可忽略，换来单一循环实现降低维护成本。
//
// 注意：新 profile 应显式传 "x" 或 "y"；空字符串仅为兼容旧数据保留，不应在新代码中使用。
func GridPointsFromAxesOrdered(xs, ys []float64, primaryAxis string) []Point {
	if normalizePrimaryAxis(primaryAxis) != PrimaryAxisX {
		// 含 "y"、空字符串及任何非 "x" 值都走 legacy（先走 Y）
		return GridPointsFromAxes(xs, ys)
	}
	// 先走 X：交换两轴调用 legacy（外层 Y、内层 X），再转置坐标恢复 (X,Y) 语义
	return swapPoints(GridPointsFromAxes(ys, xs))
}

// GridPointsFromAxesSnakeOrdered 按指定主轴顺序生成蛇形网格点
// primaryAxis 取值（归一化同 GridPointsFromAxesOrdered）：
//   - PrimaryAxisX ("x")：外层 Y、内层 X，奇数行反转 X 顺序
//   - PrimaryAxisY ("y")：外层 X、内层 Y，奇数行反转 Y 顺序，等价于 GridPointsFromAxesSnake
//   - PrimaryAxisLegacy ("") 或其他未识别值：保旧行为
//
// 蛇形反转方向跟随主轴：主轴是“长程走线方向”，反转主轴可避免回程空跑。
// 'x' 分支复用 GridPointsFromAxesSnake(ys, xs) 后转置，逻辑同 GridPointsFromAxesOrdered。
func GridPointsFromAxesSnakeOrdered(xs, ys []float64, primaryAxis string) []Point {
	if normalizePrimaryAxis(primaryAxis) != PrimaryAxisX {
		return GridPointsFromAxesSnake(xs, ys)
	}
	return swapPoints(GridPointsFromAxesSnake(ys, xs))
}

// swapPoints 原地转置点集的 X/Y 坐标。
// 用于主轴='x' 分支：先以 (ys, xs) 调用 legacy 生成外层 Y、内层 X 的点，
// 再通过此函数把 (Y, X) 还原为 (X, Y) 语义。
func swapPoints(points []Point) []Point {
	for i := range points {
		points[i].X, points[i].Y = points[i].Y, points[i].X
	}
	return points
}

// normalizePrimaryAxis 归一化主轴字段：去前后空白 + 转小写。
// 仅用于入口判别，不改变持久化值。返回值与 PrimaryAxisX/Y/Legacy 常量比较即可。
func normalizePrimaryAxis(primaryAxis string) string {
	return strings.ToLower(strings.TrimSpace(primaryAxis))
}

// SectorPointsFromRadiiAngles 从半径和角度生成扇形点
func SectorPointsFromRadiiAngles(centerX, centerY float64, radii, angles []float64) []Point {
	var points []Point
	for _, radius := range radii {
		for _, angle := range angles {
			radian := angle * math.Pi / 180
			points = append(points, Point{
				X: centerX + radius*math.Cos(radian),
				Y: centerY + radius*math.Sin(radian),
			})
		}
	}
	return points
}

// SectorPointsFromRadiiAnglesSnake 从半径和角度生成蛇形扇形点
func SectorPointsFromRadiiAnglesSnake(centerX, centerY float64, radii, angles []float64) []Point {
	var points []Point
	for i, radius := range radii {
		if i%2 == 1 {
			for j := len(angles) - 1; j >= 0; j-- {
				radian := angles[j] * math.Pi / 180
				points = append(points, Point{
					X: centerX + radius*math.Cos(radian),
					Y: centerY + radius*math.Sin(radian),
				})
			}
		} else {
			for _, angle := range angles {
				radian := angle * math.Pi / 180
				points = append(points, Point{
					X: centerX + radius*math.Cos(radian),
					Y: centerY + radius*math.Sin(radian),
				})
			}
		}
	}
	return points
}

// ContainsFloat 检查浮点数切片是否包含指定值（容差 1e-9）
func ContainsFloat(values []float64, needle float64) bool {
	for _, value := range values {
		if math.Abs(value-needle) < 1e-9 {
			return true
		}
	}
	return false
}

// LayoutConfig 遍历布局配置
type LayoutConfig struct {
	Pattern    string           `json:"pattern"`
	SnakeOrder bool             `json:"snakeOrder,omitempty"`
	// PrimaryAxis 控制矩形/线型布局的走线主轴（仅 line / rectangle 消费，扇形不消费）：
	//   - PrimaryAxisX ("x")：先沿 X 走完一条线再切换 Y
	//   - PrimaryAxisY ("y")：先沿 Y 走完一条线再切换 X（原始行为）
	//   - PrimaryAxisLegacy ("")：缺省，保旧行为（先走 Y），用于升级前保存的 profile 兼容
	// 大小写不敏感、去空白。新代码应显式传 "x" 或 "y"。
	PrimaryAxis string           `json:"primaryAxis,omitempty"`
	Line        *LineLayout      `json:"line,omitempty"`
	Rectangle   *RectangleLayout `json:"rectangle,omitempty"`
	Sector      *SectorLayout    `json:"sector,omitempty"`
	Custom      *CustomLayout    `json:"custom,omitempty"`
}

// LineLayout 线型布局：仅沿 X 轴布点，Y 恒为 0。
// 设计取舍：line 模式语义本质是"沿一条直线布点"，不需要 Y 步进。
// 若需要 2D 平面布点，应使用 rectangle 或 sector 模式。
// 旧配置中残留的 startY/endY/yStepSegments 字段会被 JSON 反序列化时静默忽略，无需 migration。
type LineLayout struct {
	StartX        float64       `json:"startX"`
	EndX          float64       `json:"endX"`
	XStepSegments []StepSegment `json:"xStepSegments"`
}

// RectangleLayout 矩形布局
type RectangleLayout struct {
	XMin          float64       `json:"xMin"`
	XMax          float64       `json:"xMax"`
	XStepSegments []StepSegment `json:"xStepSegments"`
	YMin          float64       `json:"yMin"`
	YMax          float64       `json:"yMax"`
	YStepSegments []StepSegment `json:"yStepSegments"`
}

// SectorLayout 扇形布局
type SectorLayout struct {
	CenterX             float64       `json:"centerX"`
	CenterY             float64       `json:"centerY"`
	RadiusMin           float64       `json:"radiusMin"`
	RadiusMax           float64       `json:"radiusMax"`
	RadialStepSegments  []StepSegment `json:"radialStepSegments"`
	AngleStart          float64       `json:"angleStart"`
	AngleEnd            float64       `json:"angleEnd"`
	AngularStepSegments []StepSegment `json:"angularStepSegments"`
}

// CustomLayout 自定义布局
// Z 和 U 字段为零值填充语义：无此字段的旧配置自动填 0
type CustomLayout struct {
	Points []struct {
		X float64 `json:"x"`
		Y float64 `json:"y"`
		Z float64 `json:"z"`
		U float64 `json:"u"`
	} `json:"points"`
}

// PointsFromLayout 根据布局配置生成遍历点
func PointsFromLayout(cfg LayoutConfig) []Point {
	switch cfg.Pattern {
	case "line":
		if cfg.Line == nil {
			return nil
		}
		// line 模式仅沿 X 轴布点，Y 固定为 0（详见 LineLayout 注释）。
		// 不再消费 PrimaryAxis / SnakeOrder：单行点位无走线方向概念。
		// 旧配置若携带 SnakeOrder=true 也会被忽略，避免单行点位产生无意义的蛇形走线。
		// primaryAxis 固定传 "x"：ys 只有一个值时 GridPointsFromAxesOrdered 的主轴参数不影响结果。
		xs := StepValues(cfg.Line.StartX, cfg.Line.EndX, cfg.Line.XStepSegments)
		ys := []float64{0}
		return GridPointsFromAxesOrdered(xs, ys, "x")
	case "rectangle":
		if cfg.Rectangle == nil {
			return nil
		}
		xs := StepValues(cfg.Rectangle.XMin, cfg.Rectangle.XMax, cfg.Rectangle.XStepSegments)
		ys := StepValues(cfg.Rectangle.YMin, cfg.Rectangle.YMax, cfg.Rectangle.YStepSegments)
		if cfg.SnakeOrder {
			return GridPointsFromAxesSnakeOrdered(xs, ys, cfg.PrimaryAxis)
		}
		return GridPointsFromAxesOrdered(xs, ys, cfg.PrimaryAxis)
	case "sector":
		if cfg.Sector == nil {
			return nil
		}
		radii := StepValues(cfg.Sector.RadiusMin, cfg.Sector.RadiusMax, cfg.Sector.RadialStepSegments)
		angles := StepValues(cfg.Sector.AngleStart, cfg.Sector.AngleEnd, cfg.Sector.AngularStepSegments)
		// 扇形布局不消费 PrimaryAxis，保持原“外层半径、内层角度”语义
		if cfg.SnakeOrder {
			return SectorPointsFromRadiiAnglesSnake(cfg.Sector.CenterX, cfg.Sector.CenterY, radii, angles)
		}
		return SectorPointsFromRadiiAngles(cfg.Sector.CenterX, cfg.Sector.CenterY, radii, angles)
	case "custom":
		if cfg.Custom == nil {
			return nil
		}
		points := make([]Point, 0, len(cfg.Custom.Points))
		for _, point := range cfg.Custom.Points {
			points = append(points, Point{X: point.X, Y: point.Y, Z: point.Z, U: point.U})
		}
		return points
	default:
		return nil
	}
}

// ValidatePressures 验证压力数据是否在有效范围内
// 通道映射策略：
//  1. 若 labels 非空，则按 channelIndex→label 显式映射，避免依赖通道索引顺序；
//  2. 否则回退为"通道索引升序对应 P1..P5,Patm,Tatm"（旧行为，向后兼容）。
func ValidatePressures(values map[int]float64, config *DataValidationConfig, labels map[int]string) (bool, []string) {
	if config == nil || !config.Enabled {
		return true, nil
	}

	var warnings []string

	if len(labels) > 0 {
		// 显式映射模式
		for chIdx, value := range values {
			label, ok := labels[chIdx]
			if !ok {
				continue
			}
			if r, ok := config.PressureRange[label]; ok {
				if value < r.Min || value > r.Max {
					warnings = append(warnings, fmt.Sprintf("%s 超出范围: %.2f (%.2f-%.2f)", label, value, r.Min, r.Max))
				}
			}
		}
	} else {
		// 兼容旧行为：通道索引升序映射
		legacyLabels := []string{"P1", "P2", "P3", "P4", "P5", "Patm", "Tatm"}
		orderedKeys := sortedKeys(values)

		for i, label := range legacyLabels {
			if i >= len(orderedKeys) {
				break
			}
			value := values[orderedKeys[i]]
			if r, ok := config.PressureRange[label]; ok {
				if value < r.Min || value > r.Max {
					warnings = append(warnings, fmt.Sprintf("%s 超出范围: %.2f (%.2f-%.2f)", label, value, r.Min, r.Max))
				}
			}
		}
	}

	valid := len(warnings) == 0 || config.OnInvalid == "continue"
	return valid, warnings
}

// sortedKeys 返回排序后的 map 键
func sortedKeys(m map[int]float64) []int {
	keys := make([]int, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Ints(keys)
	return keys
}
