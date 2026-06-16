package traversal

import (
	"fmt"
	"math"
	"sort"
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
		d := distance(from, to)
		if d == 0 {
			continue
		}
		segments := int(math.Ceil(d / step))
		for segment := 1; segment <= segments; segment++ {
			ratio := float64(segment) / float64(segments)
			path = append(path, Point{
				X: from.X + (to.X-from.X)*ratio,
				Y: from.Y + (to.Y-from.Y)*ratio,
				Z: from.Z + (to.Z-from.Z)*ratio,
			})
		}
	}
	return path, nil
}

func distance(a Point, b Point) float64 {
	dx := b.X - a.X
	dy := b.Y - a.Y
	dz := b.Z - a.Z
	return math.Sqrt(dx*dx + dy*dy + dz*dz)
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
	Pattern    string          `json:"pattern"`
	SnakeOrder bool            `json:"snakeOrder,omitempty"`
	Line       *LineLayout     `json:"line,omitempty"`
	Rectangle  *RectangleLayout `json:"rectangle,omitempty"`
	Sector     *SectorLayout   `json:"sector,omitempty"`
	Custom     *CustomLayout   `json:"custom,omitempty"`
}

// LineLayout 线型布局
type LineLayout struct {
	StartX        float64       `json:"startX"`
	StartY        float64       `json:"startY"`
	EndX          float64       `json:"endX"`
	EndY          float64       `json:"endY"`
	XStepSegments []StepSegment `json:"xStepSegments"`
	YStepSegments []StepSegment `json:"yStepSegments"`
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
type CustomLayout struct {
	Points []struct {
		X float64 `json:"x"`
		Y float64 `json:"y"`
	} `json:"points"`
}

// PointsFromLayout 根据布局配置生成遍历点
func PointsFromLayout(cfg LayoutConfig) []Point {
	switch cfg.Pattern {
	case "line":
		if cfg.Line == nil {
			return nil
		}
		xs := StepValues(cfg.Line.StartX, cfg.Line.EndX, cfg.Line.XStepSegments)
		ys := StepValues(cfg.Line.StartY, cfg.Line.EndY, cfg.Line.YStepSegments)
		if len(ys) == 0 {
			ys = []float64{cfg.Line.StartY}
		}
		if cfg.SnakeOrder {
			return GridPointsFromAxesSnake(xs, ys)
		}
		return GridPointsFromAxes(xs, ys)
	case "rectangle":
		if cfg.Rectangle == nil {
			return nil
		}
		xs := StepValues(cfg.Rectangle.XMin, cfg.Rectangle.XMax, cfg.Rectangle.XStepSegments)
		ys := StepValues(cfg.Rectangle.YMin, cfg.Rectangle.YMax, cfg.Rectangle.YStepSegments)
		if cfg.SnakeOrder {
			return GridPointsFromAxesSnake(xs, ys)
		}
		return GridPointsFromAxes(xs, ys)
	case "sector":
		if cfg.Sector == nil {
			return nil
		}
		radii := StepValues(cfg.Sector.RadiusMin, cfg.Sector.RadiusMax, cfg.Sector.RadialStepSegments)
		angles := StepValues(cfg.Sector.AngleStart, cfg.Sector.AngleEnd, cfg.Sector.AngularStepSegments)
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
			points = append(points, Point{X: point.X, Y: point.Y})
		}
		return points
	default:
		return nil
	}
}

// ValidatePressures 验证压力数据是否在有效范围内
// 通道映射约定：按通道索引排序后依次对应 P1,P2,P3,P4,P5,Patm,Tatm
func ValidatePressures(values map[int]float64, config *DataValidationConfig) (bool, []string) {
	if config == nil || !config.Enabled {
		return true, nil
	}

	var warnings []string
	// 通道索引到标签的映射：排序后的通道依次对应
	labels := []string{"P1", "P2", "P3", "P4", "P5", "Patm", "Tatm"}
	orderedKeys := sortedKeys(values)

	for i, label := range labels {
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
