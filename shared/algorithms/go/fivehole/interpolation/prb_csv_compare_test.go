package interpolation

import (
	"encoding/csv"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// resolveRepoRoot 从测试源文件位置推算仓库根目录，避免硬编码开发者本地绝对路径。
// 本文件位于 <repo-root>/shared/algorithms/go/fivehole/interpolation/，
// 向上回溯 5 层即为 repo-root。
func resolveRepoRoot() string {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return ""
	}
	dir := filepath.Dir(file)
	for i := 0; i < 5; i++ {
		dir = filepath.Dir(dir)
	}
	return dir
}

// resolveCsvPath 解析 CSV 数据路径，支持 repo-root/temp 与开发者原绝对路径回退。
// CSV 数据默认不被纳入版本控制（受 .gitignore 覆盖），缺失时由调用方 Skip。
func resolveCsvPath(filename string) string {
	root := resolveRepoRoot()
	if root != "" {
		if _, err := os.Stat(filepath.Join(root, "temp", filename)); err == nil {
			return filepath.Join(root, "temp", filename)
		}
	}
	return filename
}

// ==================== CSV 校准数据行 ====================

// csvCalRow 从 CSV 文件中解析出的单行校准数据
type csvCalRow struct {
	RowIndex int     // 行号（0-based，不含表头）
	Hole1    float64 // 五孔压力值
	Hole2    float64
	Hole3    float64
	Hole4    float64
	Hole5    float64
	AtmTemp  float64 // 大气温度 (°C)
	AtmPress float64 // 大气压力 (Pa)
	KaCSV    float64 // CSV 中记录的 Ka
	KbCSV    float64 // CSV 中记录的 Kb
	CptCSV   float64 // CSV 中记录的 Cpt
	CpsCSV   float64 // CSV 中记录的 Cps
	AlphaCSV float64 // CSV 中记录的 Alpha (度)
	BetaCSV  float64 // CSV 中记录的 Beta (度)
	MachCSV  float64 // CSV 中记录的马赫数
}

// ==================== 对比结果 ====================

// compareStats 单参数对比统计
type compareStats struct {
	Name      string  // 参数名
	Count     int     // 有效样本数
	MeanErr   float64 // 平均误差
	MaxAbsErr float64 // 最大绝对误差
	RMSE      float64 // 均方根误差
	MeanRel   float64 // 平均相对误差 (%)
	MaxRel    float64 // 最大相对误差 (%)
}

// ==================== CSV 解析 ====================

// csvColumnIndex 定义 CSV 中各列的索引
// CSV 列顺序: Hole1(0),Hole2(1),Hole3(2),Hole4(3),Hole5(4),
//
//	大气温度(5),大气压力(6),当前总压差(7),当前稳定时间(8),风洞总压(9),
//	风洞静压(10),总压(11),静压(12),ka(13),kb(14),Cpt(15),Cps(16),
//	Alpha(17),Beta(18),马赫数(19),CH1-16(20-35)
const (
	csvColHole1   = 0
	csvColHole2   = 1
	csvColHole3   = 2
	csvColHole4   = 3
	csvColHole5   = 4
	csvColAtmTemp = 5  // 大气温度 (°C)
	csvColAtmPres = 6  // 大气压力 (Pa)
	csvColKa      = 13 // ka 系数
	csvColKb      = 14 // kb 系数
	csvColCpt     = 15 // Cpt 系数
	csvColCps     = 16 // Cps 系数
	csvColAlpha   = 17 // Alpha (度)
	csvColBeta    = 18 // Beta (度)
	csvColMach    = 19 // 马赫数
)

// parseCSVFile 解析 CSV 校准文件，返回所有数据行
func parseCSVFile(filePath string) ([]csvCalRow, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("打开文件失败: %w", err)
	}
	defer f.Close()

	reader := csv.NewReader(f)
	reader.LazyQuotes = true
	reader.FieldsPerRecord = -1 // 允许变长行

	allRows, err := reader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("读取 CSV 失败: %w", err)
	}
	if len(allRows) < 2 {
		return nil, fmt.Errorf("CSV 文件为空")
	}

	// 跳过表头行
	var rows []csvCalRow
	for i := 1; i < len(allRows); i++ {
		cols := allRows[i]
		if len(cols) <= csvColMach {
			continue // 跳过不完整的行
		}

		row := csvCalRow{RowIndex: i - 1} // 0-based 数据行索引

		// 解析各列（跳过解析失败的行）
		var ok bool
		row.Hole1, ok = parseFloatSafe(cols[csvColHole1])
		if !ok {
			continue
		}
		row.Hole2, ok = parseFloatSafe(cols[csvColHole2])
		if !ok {
			continue
		}
		row.Hole3, ok = parseFloatSafe(cols[csvColHole3])
		if !ok {
			continue
		}
		row.Hole4, ok = parseFloatSafe(cols[csvColHole4])
		if !ok {
			continue
		}
		row.Hole5, ok = parseFloatSafe(cols[csvColHole5])
		if !ok {
			continue
		}
		row.AtmTemp, ok = parseFloatSafe(cols[csvColAtmTemp])
		if !ok {
			continue
		}
		row.AtmPress, ok = parseFloatSafe(cols[csvColAtmPres])
		if !ok {
			continue
		}
		row.KaCSV, _ = parseFloatSafe(cols[csvColKa])
		row.KbCSV, _ = parseFloatSafe(cols[csvColKb])
		row.CptCSV, _ = parseFloatSafe(cols[csvColCpt])
		row.CpsCSV, _ = parseFloatSafe(cols[csvColCps])
		row.AlphaCSV, ok = parseFloatSafe(cols[csvColAlpha])
		if !ok {
			continue
		}
		row.BetaCSV, ok = parseFloatSafe(cols[csvColBeta])
		if !ok {
			continue
		}
		row.MachCSV, ok = parseFloatSafe(cols[csvColMach])
		if !ok {
			continue
		}

		rows = append(rows, row)
	}

	return rows, nil
}

func parseFloatSafe(s string) (float64, bool) {
	val, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil || math.IsNaN(val) || math.IsInf(val, 0) {
		return 0, false
	}
	return val, true
}

// ==================== PRB 格式构建 ====================

// buildPRBLinesFromGrid 从 CSV 数据中提取网格点，构建 PRB 格式行
// 网格点：Alpha 和 Beta 均在 [-30, 30] 范围内，且为 5° 的整数倍
func buildPRBLinesFromGrid(rows []csvCalRow) ([]string, error) {
	// 筛选出网格点
	gridPoints := make(map[string]csvCalRow)
	inRangeCount := 0
	for _, row := range rows {
		alpha := row.AlphaCSV
		beta := row.BetaCSV
		// 检查是否在 ±30° 范围内
		if alpha < gridMinAngle-0.01 || alpha > gridMaxAngle+0.01 ||
			beta < gridMinAngle-0.01 || beta > gridMaxAngle+0.01 {
			continue
		}
		inRangeCount++
		// 检查是否为 5° 的整数倍：使用 math.Remainder 替代 math.Mod 避免负零问题
		alphaRem := math.Remainder(alpha, gridStep)
		betaRem := math.Remainder(beta, gridStep)
		if math.Abs(alphaRem) > 0.01 || math.Abs(betaRem) > 0.01 {
			continue
		}

		// 归一化到精确的 5° 步长值
		alphaGrid := math.Round(alpha/gridStep) * gridStep
		betaGrid := math.Round(beta/gridStep) * gridStep
		key := fmt.Sprintf("%.0f,%.0f", alphaGrid, betaGrid)
		if _, exists := gridPoints[key]; !exists {
			gridPoints[key] = row
		}
	}

	if len(gridPoints) != expectedGridCount {
		return nil, fmt.Errorf("网格点数量不足: 期望 %d, 实际 %d (范围内行数: %d)", expectedGridCount, len(gridPoints), inRangeCount)
	}

	// 按 Alpha 升序、Beta 升序排列（PRB 格式要求）
	type gridEntry struct {
		alpha, beta float64
		row         csvCalRow
	}
	var entries []gridEntry
	for _, row := range gridPoints {
		alphaGrid := math.Round(row.AlphaCSV/gridStep) * gridStep
		betaGrid := math.Round(row.BetaCSV/gridStep) * gridStep
		entries = append(entries, gridEntry{alpha: alphaGrid, beta: betaGrid, row: row})
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].alpha != entries[j].alpha {
			return entries[i].alpha < entries[j].alpha
		}
		return entries[i].beta < entries[j].beta
	})

	// 构建 PRB 格式行
	lines := make([]string, 0, expectedGridCount+1)
	lines = append(lines, fmt.Sprintf("%d %d", gridAxisSize, gridAxisSize))
	for _, e := range entries {
		line := fmt.Sprintf("%.6f %.6f %.6f %.6f %.1f %.1f",
			e.row.KaCSV, e.row.KbCSV, e.row.CptCSV, e.row.CpsCSV,
			e.alpha, e.beta)
		lines = append(lines, line)
	}
	return lines, nil
}

// ==================== 对比计算 ====================

// compareRow 对单行 CSV 数据执行旧插值算法并对比
// 返回: (算法输出的 Alpha, Beta, Mach, Velocity, 是否有效)
func compareRow(interp *PrbInterpolator, row csvCalRow) (InterpolationResult, error) {
	input := InterpolationInput{
		P1:   row.Hole1,
		P2:   row.Hole2,
		P3:   row.Hole3,
		P4:   row.Hole4,
		P5:   row.Hole5,
		PAtm: row.AtmPress,
		TAtm: row.AtmTemp,
	}
	return interp.Calculate(input)
}

// runCompare 对一个 CSV 文件执行完整对比
func runCompare(t *testing.T, csvPath string) {
	t.Helper()

	// CSV 数据不在版本控制内，缺失时跳过测试而非失败
	if _, err := os.Stat(csvPath); err != nil {
		t.Skipf("CSV 校准数据不可用，跳过: %s (%v)", csvPath, err)
	}

	// 1. 解析 CSV
	rows, err := parseCSVFile(csvPath)
	if err != nil {
		t.Fatalf("解析 CSV 失败: %v", err)
	}
	t.Logf("CSV 总行数: %d", len(rows))

	// 2. 构建 PRB 网格
	prbLines, err := buildPRBLinesFromGrid(rows)
	if err != nil {
		t.Fatalf("构建 PRB 网格失败: %v", err)
	}
	t.Logf("PRB 网格行数: %d (含表头)", len(prbLines))

	// 3. 创建插值器并加载
	interp := NewPrbInterpolator()
	if err := interp.LoadPrbLines(prbLines, csvPath); err != nil {
		t.Fatalf("加载 PRB 数据失败: %v", err)
	}

	// 4. 筛选 ±30° 范围内的数据行
	var inRange []csvCalRow
	for _, row := range rows {
		if row.AlphaCSV >= gridMinAngle && row.AlphaCSV <= gridMaxAngle &&
			row.BetaCSV >= gridMinAngle && row.BetaCSV <= gridMaxAngle {
			inRange = append(inRange, row)
		}
	}
	t.Logf("±30° 范围内数据行数: %d", len(inRange))

	// 5. 逐行对比
	var alphaErrs, betaErrs, machErrs, velErrs []float64
	var alphaRels, betaRels, machRels, velRels []float64
	validCount := 0
	invalidCount := 0

	for _, row := range inRange {
		result, err := compareRow(interp, row)
		if err != nil {
			t.Logf("  行 %d: 计算失败: %v", row.RowIndex, err)
			invalidCount++
			continue
		}
		if !result.IsValid {
			invalidCount++
			continue
		}

		validCount++

		// 计算各参数误差
		alphaErr := result.Alpha - row.AlphaCSV
		betaErr := result.Beta - row.BetaCSV
		machErr := result.MachNumber - row.MachCSV
		// CSV 中没有直接记录速度，跳过速度对比
		velErr := 0.0

		alphaErrs = append(alphaErrs, alphaErr)
		betaErrs = append(betaErrs, betaErr)
		machErrs = append(machErrs, machErr)
		velErrs = append(velErrs, velErr)

		// 计算相对误差（百分比）
		if math.Abs(row.AlphaCSV) > 0.01 {
			alphaRels = append(alphaRels, math.Abs(alphaErr/row.AlphaCSV)*100)
		}
		if math.Abs(row.BetaCSV) > 0.01 {
			betaRels = append(betaRels, math.Abs(betaErr/row.BetaCSV)*100)
		}
		if row.MachCSV > 0.001 {
			machRels = append(machRels, math.Abs(machErr/row.MachCSV)*100)
		}
	}

	// 6. 计算统计量
	alphaStats := computeStats("Alpha(°)", alphaErrs, alphaRels)
	betaStats := computeStats("Beta(°)", betaErrs, betaRels)
	machStats := computeStats("马赫数", machErrs, machRels)
	velStats := computeStats("速度(m/s)", velErrs, velRels)

	// 7. 打印结果
	fileName := filepath.Base(csvPath)
	t.Logf("\n========== %s 旧插值算法对比结果 ==========", fileName)
	t.Logf("  ±30°范围内总行数: %d, 有效: %d, 无效: %d", len(inRange), validCount, invalidCount)
	t.Logf("")

	printStats(t, alphaStats)
	printStats(t, betaStats)
	printStats(t, machStats)
	printStats(t, velStats)

	// 打印误差最大的前 5 个点
	t.Logf("\n  --- Alpha 误差最大的 5 个点 ---")
	printTopErrors(t, inRange, func(r InterpolationResult, row csvCalRow) float64 {
		return math.Abs(r.Alpha - row.AlphaCSV)
	}, interp)
	t.Logf("\n  --- Beta 误差最大的 5 个点 ---")
	printTopErrors(t, inRange, func(r InterpolationResult, row csvCalRow) float64 {
		return math.Abs(r.Beta - row.BetaCSV)
	}, interp)
	t.Logf("\n  --- 马赫数误差最大的 5 个点 ---")
	printTopErrors(t, inRange, func(r InterpolationResult, row csvCalRow) float64 {
		return math.Abs(r.MachNumber - row.MachCSV)
	}, interp)
}

func computeStats(name string, absErrs, relErrs []float64) compareStats {
	stats := compareStats{Name: name, Count: len(absErrs)}
	if len(absErrs) == 0 {
		return stats
	}

	var sumErr, sumSq, maxAbs float64
	for _, e := range absErrs {
		sumErr += e
		sumSq += e * e
		if math.Abs(e) > maxAbs {
			maxAbs = math.Abs(e)
		}
	}
	stats.MeanErr = sumErr / float64(len(absErrs))
	stats.RMSE = math.Sqrt(sumSq / float64(len(absErrs)))
	stats.MaxAbsErr = maxAbs

	if len(relErrs) > 0 {
		var sumRel, maxRel float64
		for _, r := range relErrs {
			sumRel += r
			if r > maxRel {
				maxRel = r
			}
		}
		stats.MeanRel = sumRel / float64(len(relErrs))
		stats.MaxRel = maxRel
	}

	return stats
}

func printStats(t *testing.T, s compareStats) {
	t.Helper()
	if s.Count == 0 {
		t.Logf("  %-12s: 无有效数据", s.Name)
		return
	}
	t.Logf("  %-12s: 样本=%d | 平均误差=%.6f | 最大绝对误差=%.6f | RMSE=%.6f | 平均相对=%.4f%% | 最大相对=%.4f%%",
		s.Name, s.Count, s.MeanErr, s.MaxAbsErr, s.RMSE, s.MeanRel, s.MaxRel)
}

type errorEntry struct {
	rowIndex int
	alphaCSV float64
	betaCSV  float64
	err      float64
}

func printTopErrors(t *testing.T, rows []csvCalRow, errFn func(InterpolationResult, csvCalRow) float64, interp *PrbInterpolator) {
	t.Helper()
	var entries []errorEntry
	for _, row := range rows {
		input := InterpolationInput{
			P1: row.Hole1, P2: row.Hole2, P3: row.Hole3, P4: row.Hole4, P5: row.Hole5,
			PAtm: row.AtmPress, TAtm: row.AtmTemp,
		}
		result, err := interp.Calculate(input)
		if err != nil || !result.IsValid {
			continue
		}
		entries = append(entries, errorEntry{
			rowIndex: row.RowIndex,
			alphaCSV: row.AlphaCSV,
			betaCSV:  row.BetaCSV,
			err:      errFn(result, row),
		})
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].err > entries[j].err
	})
	n := 5
	if len(entries) < n {
		n = len(entries)
	}
	for i := 0; i < n; i++ {
		e := entries[i]
		t.Logf("    行%3d (α=%.1f°, β=%.1f°): 误差=%.6f", e.rowIndex, e.alphaCSV, e.betaCSV, e.err)
	}
}

// ==================== 测试入口 ====================

// TestPrbCsvCompare_03Ma 对比 0.3Ma CSV 数据
func TestPrbCsvCompare_03Ma(t *testing.T) {
	runCompare(t, resolveCsvPath("WTN.202502.P.5H.1-08-0.3Ma.csv"))
}

// TestPrbCsvCompare_02Ma 对比 0.2Ma CSV 数据
func TestPrbCsvCompare_02Ma(t *testing.T) {
	runCompare(t, resolveCsvPath("WTN.202502.P.5H.1-08-0.2Ma.csv"))
}

// TestPrbCsvCompare_01Ma 对比 0.1Ma CSV 数据
func TestPrbCsvCompare_01Ma(t *testing.T) {
	runCompare(t, resolveCsvPath("WTN.202502.P.5H.1-08-0.1Ma.csv"))
}