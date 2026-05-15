package interpolation

import (
	"fmt"
	"math"
	"sort"
)

// MultiPrbInterpolationMode 多PRB插值模式
type MultiPrbInterpolationMode string

const (
	MultiPrbNearest MultiPrbInterpolationMode = "nearest"
	MultiPrbLinear  MultiPrbInterpolationMode = "linear"
)

// PrbFileInfo PRB 文件信息
type PrbFileInfo struct {
	FilePath string  `json:"filePath"`
	FileName string  `json:"fileName"`
	Mach     float64 `json:"mach"`
	LoadedAt string  `json:"loadedAt"`
}

// MultiPrbInterpolator 多 PRB 文件插值器
type MultiPrbInterpolator struct {
	interpolators []*PrbInterpolator
	machNumbers   []float64
	mode          MultiPrbInterpolationMode
	loaded        bool
}

// NewMultiPrbInterpolator 创建多PRB插值器
func NewMultiPrbInterpolator(mode MultiPrbInterpolationMode) *MultiPrbInterpolator {
	return &MultiPrbInterpolator{
		mode: mode,
	}
}

// AddPrbFile 添加 PRB 文件
func (m *MultiPrbInterpolator) AddPrbFile(content string, filePath string) error {
	interp := NewPrbInterpolator()
	if err := interp.LoadPrbFile(content, filePath); err != nil {
		return fmt.Errorf("load PRB file %s: %w", filePath, err)
	}

	m.interpolators = append(m.interpolators, interp)
	m.machNumbers = append(m.machNumbers, interp.table.Mach)
	m.loaded = true

	// 按马赫数排序
	sort.Slice(m.interpolators, func(i, j int) bool {
		return m.interpolators[i].table.Mach < m.interpolators[j].table.Mach
	})
	sort.Float64s(m.machNumbers)

	return nil
}

// IsLoaded 是否已加载
func (m *MultiPrbInterpolator) IsLoaded() bool {
	return m.loaded
}

// Calculate 执行插值计算
func (m *MultiPrbInterpolator) Calculate(input TraversalInterpolationInput) (InterpolationResult, error) {
	if !m.loaded || len(m.interpolators) == 0 {
		return InterpolationResult{}, fmt.Errorf("no PRB files loaded")
	}

	// 单文件直接计算
	if len(m.interpolators) == 1 {
		return m.interpolators[0].Calculate(input)
	}

	// 初始 Ma 估算: 用中间马赫数的 PRB 做一次插值
	midIdx := len(m.interpolators) / 2
	initialResult, err := m.interpolators[midIdx].Calculate(input)
	if err != nil {
		return InterpolationResult{}, err
	}
	targetMa := initialResult.MachNumber

	switch m.mode {
	case MultiPrbNearest:
		return m.calculateWithNearest(input, targetMa)
	case MultiPrbLinear:
		return m.calculateWithLinear(input, targetMa)
	default:
		return m.calculateWithNearest(input, targetMa)
	}
}

// calculateWithNearest 最近邻模式
func (m *MultiPrbInterpolator) calculateWithNearest(input TraversalInterpolationInput, targetMa float64) (InterpolationResult, error) {
	idx := findNearestMachIndex(m.machNumbers, targetMa)
	result, err := m.interpolators[idx].Calculate(input)
	if err != nil {
		return InterpolationResult{}, err
	}

	// 偏差过大时添加警告
	if math.Abs(m.machNumbers[idx]-targetMa) > 0.01 {
		result.Warning = fmt.Sprintf("Ma deviation %.4f from nearest PRB %.2fMa",
			math.Abs(m.machNumbers[idx]-targetMa), m.machNumbers[idx])
	}

	return result, nil
}

// calculateWithLinear 线性插值模式
func (m *MultiPrbInterpolator) calculateWithLinear(input TraversalInterpolationInput, targetMa float64) (InterpolationResult, error) {
	lower, upper := findMachInterval(m.machNumbers, targetMa)

	resultLower, err := m.interpolators[lower].Calculate(input)
	if err != nil {
		return InterpolationResult{}, err
	}

	resultUpper, err := m.interpolators[upper].Calculate(input)
	if err != nil {
		return InterpolationResult{}, err
	}

	maLower := m.machNumbers[lower]
	maUpper := m.machNumbers[upper]

	var weight float64
	if math.Abs(maUpper-maLower) > 1e-10 {
		weight = (targetMa - maLower) / (maUpper - maLower)
	} else {
		weight = 0.5
	}
	weight = math.Max(0, math.Min(1, weight))

	// 对各参数线性插值
	result := InterpolationResult{
		Alpha:           lerp(resultLower.Alpha, resultUpper.Alpha, weight),
		Beta:            lerp(resultLower.Beta, resultUpper.Beta, weight),
		MachNumber:      lerp(resultLower.MachNumber, resultUpper.MachNumber, weight),
		Velocity:        lerp(resultLower.Velocity, resultUpper.Velocity, weight),
		DynamicPressure: lerp(resultLower.DynamicPressure, resultUpper.DynamicPressure, weight),
		Density:         lerp(resultLower.Density, resultUpper.Density, weight),
		P0:              lerp(resultLower.P0, resultUpper.P0, weight),
		Ps:              lerp(resultLower.Ps, resultUpper.Ps, weight),
		IsValid:         resultLower.IsValid && resultUpper.IsValid,
	}

	return result, nil
}

// GetMachNumbers 获取已加载的马赫数列表
func (m *MultiPrbInterpolator) GetMachNumbers() []float64 {
	return m.machNumbers
}

// findNearestMachIndex 找到最近的马赫数索引
func findNearestMachIndex(machNumbers []float64, target float64) int {
	best := 0
	bestDist := math.Abs(machNumbers[0] - target)
	for i := 1; i < len(machNumbers); i++ {
		dist := math.Abs(machNumbers[i] - target)
		if dist < bestDist {
			bestDist = dist
			best = i
		}
	}
	return best
}

// findMachInterval 二分查找马赫数区间
func findMachInterval(machNumbers []float64, target float64) (lower, upper int) {
	n := len(machNumbers)
	if target <= machNumbers[0] {
		return 0, 0
	}
	if target >= machNumbers[n-1] {
		return n - 1, n - 1
	}

	lo, hi := 0, n-1
	for lo < hi-1 {
		mid := (lo + hi) / 2
		if machNumbers[mid] <= target {
			lo = mid
		} else {
			hi = mid
		}
	}
	return lo, hi
}

// lerp 线性插值
func lerp(a, b, t float64) float64 {
	return a + (b-a)*t
}
