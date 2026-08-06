package interpolation

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
)

const (
	sevenHoleInnerCSVRows = 169
	sevenHoleDitherPasses = 100
)

// 七孔校准 CSV 历史位置契约（spec-seven-hole-calibration §7.2/§7.3）。
//
// 当前数据集的 inner/outer CSV 均为 18 列基础格式：
//
//	inner: 侧滑角α, 迎角β, 马赫数Ma, 来流总压P0, 来流静压Ps,
//	       P1, P2, P3, P4, P5, P6, P7,
//	       α角度系数Kα, β角度系数Kβ, 总压系数K0, 静压系数Ks,
//	       大气压力, 大气温度
//
//	outer: 滚转角φ, 俯仰角θ, 马赫数Ma, 来流总压P0, 来流静压Ps,
//	       P1, P2, P3, P4, P5, P6, P7,
//	       θ角度系数Kθ[n], φ角度系数Kφ[n], 总压系数K0[n], 静压系数Ks[n],
//	       大气压力, 大气温度
//
// 重要：spec §7.3 备注"当前数据集存在历史遗留命名错误"——outer CSV 的表头
// 仍写成"侧滑角α/迎角β/Kα/Kβ"，但数据实际是"φ/θ/Kθ[n]/Kφ[n]"。
// 因此必须按列位置读取，不能按表头名称匹配（表头名称不可信）。
//
// 加载所需的列位置契约（0-indexed）：
//   - col 0:     角度1（inner: α / outer: φ）
//   - col 1:     角度2（inner: β / outer: θ）
//   - col 3..11: P0、Ps、P1..P7，用于全精度重算全部系数
//
// 历史基础格式的 col12..15 是三位小数系数，仅用于表头诊断，不参与加载。
//
// 26 列证书导出格式（spec §7.5）当前数据集未使用；如未来需要支持，应通过
// col 0="点位编号" 检测并切换到 26 列位置映射，不在此处隐式兼容。
const (
	sevenHoleCSVAngle1Column = 0  // inner: α；outer: φ
	sevenHoleCSVAngle2Column = 1  // inner: β；outer: θ
	sevenHoleCSVPtColumn     = 3  // 来流总压 P0
	sevenHoleCSVPsColumn     = 4  // 来流静压 Ps
	sevenHoleCSVP1Column     = 5  // P1..P7 起始列（col5..col11）
	sevenHoleCSVP7Column     = 11 // P7（col11）
	sevenHoleCSVMinColumns   = sevenHoleCSVP7Column + 1
)

// sevenHoleSectorPhiLines 是 6 个外区扇区的 φ 网格线（每扇区 13 点）。
// 由 spec §3.3 几何关系确定，与校准软件导出顺序一致（φ 递减）。
var sevenHoleSectorPhiLines = [6][13]float64{
	{30, 25, 20, 15, 10, 5, 0, 355, 350, 345, 340, 335, 330},
	{90, 85, 80, 75, 70, 65, 60, 55, 50, 45, 40, 35, 30},
	{150, 145, 140, 135, 130, 125, 120, 115, 110, 105, 100, 95, 90},
	{210, 205, 200, 195, 190, 185, 180, 175, 170, 165, 160, 155, 150},
	{270, 265, 260, 255, 250, 245, 240, 235, 230, 225, 220, 215, 210},
	{330, 325, 320, 315, 310, 305, 300, 295, 290, 285, 280, 275, 270},
}

type sevenHoleCSVPoint struct {
	ka, kb, cpt, cps float64
}

type sevenHoleNudgeTarget struct {
	field int
	key   [2]float64
}

// sevenHoleCSVColumnSet 描述两个角度列索引；压力列使用上面的固定位置契约。
// 由 resolveSevenHoleCSVColumns 按历史位置契约填充（不依赖表头名称）。
type sevenHoleCSVColumnSet struct {
	a, b int
}

// SevenHoleCSVSource 表示已解码的校准 CSV 源数据。
//
// 共享算法包不依赖文件系统与编码库——调用方（项目 adapter）负责：
//  1. 读取 CSV 文件字节
//  2. 将 GB18030/GBK 解码为 UTF-8
//  3. 通过本结构传入 Label（用于错误消息标识，通常是文件路径）+ Data（UTF-8 字节）
//
// 这样设计使共享包可被内存数据源、远程拉取、沙箱测试等场景复用，
// 不强制要求调用方物化临时文件。
type SevenHoleCSVSource struct {
	// Label 用于错误消息中标识来源（通常是文件路径或调试标签）。
	Label string
	// Data 是 UTF-8 编码的 CSV 字节内容（调用方已从 GB18030 解码）。
	Data []byte
}

// resolveSevenHoleCSVColumns 按历史位置契约（spec §7.2/§7.3）填充角度列索引。
//
// inner 与 outer 共用 18 列基础格式，列位置稳定；差异仅在于 a/b 字段的语义解释：
//   - inner: a=侧滑角α(col0), b=迎角β(col1) —— 网格坐标 (a,b)=(alpha,beta)
//   - outer: a=俯仰角θ(col1), b=滚转角φ(col0) —— 网格坐标 (a,b)=(theta,phi)
//
// 注意：外区 CSV 表头存在历史遗留命名错误（spec §7.3），表头写"侧滑角α/迎角β"
// 但数据实际是"φ/θ"。按位置读取避免被错误表头误导。
//
// 若表头列数不足，返回错误并列出实际列数与最小必需列数；若表头名称与历史格式
// 不符（可能是 26 列证书导出格式或未来 spec-compliant 命名），返回警告但不报错——
// 位置契约对各种表头命名都成立，只要列顺序不变。
func resolveSevenHoleCSVColumns(path string, header []string, inner bool) (sevenHoleCSVColumnSet, []string, error) {
	if len(header) < sevenHoleCSVMinColumns {
		return sevenHoleCSVColumnSet{}, nil, fmt.Errorf("csv %s: 表头列数 %d < %d（最小必需列数），实际表头: [%s]",
			path, len(header), sevenHoleCSVMinColumns, strings.Join(header, ", "))
	}
	cols := sevenHoleCSVColumnSet{}
	if inner {
		cols.a = sevenHoleCSVAngle1Column // α
		cols.b = sevenHoleCSVAngle2Column // β
	} else {
		cols.a = sevenHoleCSVAngle2Column // θ
		cols.b = sevenHoleCSVAngle1Column // φ
	}

	// 表头诊断：检查是否匹配历史格式。不匹配时返回警告（不报错），
	// 提示用户可能使用了未支持的 26 列证书导出格式或未来新格式。
	var warnings []string
	missing := make([]string, 0, 6)
	for _, name := range []string{
		"侧滑角α", "迎角β", "α角度系数Kα", "β角度系数Kβ", "总压系数K0", "静压系数Ks",
	} {
		if !containsHeader(header, name) {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		warnings = append(warnings, fmt.Sprintf(
			"csv %s: 表头缺少历史别名 [%s]（实际表头: [%s]）；按位置契约继续解析，若数据为 26 列证书导出格式请勿使用本加载入口",
			path, strings.Join(missing, ", "), strings.Join(header, ", ")))
	}
	return cols, warnings, nil
}

// containsHeader 检查表头切片中是否包含指定名称（trim 后比较）。
func containsHeader(header []string, name string) bool {
	for _, h := range header {
		if strings.TrimSpace(h) == name {
			return true
		}
	}
	return false
}

// LoadCalibrationCSVFromUTF8 从 1 份内区 + 6 份外区 UTF-8 编码的 CSV 字节构建七孔插值器。
//
// 共享算法包不执行文件 I/O 与字符编码解码——调用方需通过 SevenHoleCSVSource
// 传入已解码的 UTF-8 字节。返回插值器、抖动警告与错误。
//
// 角度按 spec §7.2/§7.3 的历史位置契约从 col0/1 读取；系数由
// col3..11 的 P0/Ps/P1..P7 全精度重算。col12..15 的历史三位小数系数
// 仅用于表头诊断，不参与网格构建。
func LoadCalibrationCSVFromUTF8(inner SevenHoleCSVSource, outer []SevenHoleCSVSource) (*SevenHolePrbInterpolator, []string, error) {
	if len(outer) != 6 {
		return nil, nil, fmt.Errorf("seven-hole outer calibration csv count must be 6, got %d", len(outer))
	}
	gridLines := make([]float64, 13)
	for i := range gridLines {
		gridLines[i] = -30 + 5*float64(i)
	}
	interpolator, warnings, err := loadInnerCalibrationCSV(inner, gridLines)
	if err != nil {
		return nil, nil, err
	}
	for i, src := range outer {
		sectorWarnings, err := loadOuterCalibrationCSV(interpolator, i+1, src, sevenHoleSectorPhiLines[i][:])
		if err != nil {
			return nil, nil, err
		}
		warnings = append(warnings, sectorWarnings...)
	}
	return interpolator, warnings, nil
}

func loadInnerCalibrationCSV(src SevenHoleCSVSource, gridLines []float64) (*SevenHolePrbInterpolator, []string, error) {
	points, headerWarnings, err := parseSevenHoleCalibrationCSV(src, true, 0)
	if err != nil {
		return nil, nil, err
	}
	nudges, err := ditherSevenHoleCSVGrid(points, gridLines, gridLines)
	if err != nil {
		return nil, nil, fmt.Errorf("内区 csv %s: %w", src.Label, err)
	}
	lines := buildSevenHoleCSVPrbLines(points, gridLines, gridLines, sevenHoleInnerCSVRows)
	interpolator := NewSevenHolePrbInterpolator()
	if err := interpolator.LoadInnerPrbLines(lines, src.Label); err != nil {
		return nil, nil, err
	}
	warnings := headerWarnings
	if nudges > 0 {
		warnings = append(warnings, fmt.Sprintf("内区 CSV 已对 %d 处退化边施加确定性抖动（1e-9 量级）", nudges))
	}
	return interpolator, warnings, nil
}

func loadOuterCalibrationCSV(interpolator *SevenHolePrbInterpolator, sector int, src SevenHoleCSVSource, phi []float64) ([]string, error) {
	points, headerWarnings, err := parseSevenHoleCalibrationCSV(src, false, sector)
	if err != nil {
		return nil, err
	}
	theta, err := deriveSevenHoleOuterThetaGrid(points, src.Label)
	if err != nil {
		return nil, err
	}
	nudges, err := ditherSevenHoleCSVGrid(points, theta, phi)
	if err != nil {
		return nil, fmt.Errorf("扇区 %d csv %s: %w", sector, src.Label, err)
	}
	lines := buildSevenHoleCSVPrbLines(points, theta, phi, len(theta)*len(phi))
	if err := interpolator.LoadOuterPrbLines(sector, lines, src.Label); err != nil {
		return nil, err
	}
	warnings := headerWarnings
	if nudges > 0 {
		warnings = append(warnings, fmt.Sprintf("扇区 %d CSV 已对 %d 处退化边施加确定性抖动（1e-9 量级）", sector, nudges))
	}
	return warnings, nil
}

// parseSevenHoleCalibrationCSV 解析已解码的 UTF-8 CSV 字节，返回按 (a,b) 索引的网格点。
// sector=0 表示内区（用 inner 公式）；sector=1..6 表示外区扇区（用对应扇区公式）。
//
// 系数（ka/kb/cpt/cps）由原始压力列（col3=P0, col4=Ps, col5..11=P1..P7）在
// float64 全精度下按公式重算，而非直接采用 CSV 存储的 3 位小数 K 列（col12..15）。
// 这样校准 CSV 与按压力重算生成的 PRB（gen_traversal_fixtures.py
// recompute_inner/outer_coeffs）行为一致，避免 3 位截断导致的边界网格节点
// 无法定位问题（自提取 PRB 反推场景）。
//
// 共享算法包不读文件、不解码 GB18030——调用方通过 SevenHoleCSVSource.Data 传入
// UTF-8 字节，src.Label 仅用于错误消息标识。返回网格点 map、表头诊断警告与错误。
func parseSevenHoleCalibrationCSV(src SevenHoleCSVSource, inner bool, sector int) (map[[2]float64]*sevenHoleCSVPoint, []string, error) {
	reader := csv.NewReader(bytes.NewReader(src.Data))
	reader.FieldsPerRecord = -1
	records, err := reader.ReadAll()
	if err != nil {
		return nil, nil, fmt.Errorf("parse csv %s: %w", src.Label, err)
	}
	if len(records) < 2 {
		return nil, nil, fmt.Errorf("csv %s: 至少需要表头和一行数据", src.Label)
	}
	// 按位置契约解析列索引（不依赖表头名称，避免被历史命名错误误导）。
	cols, headerWarnings, err := resolveSevenHoleCSVColumns(src.Label, records[0], inner)
	if err != nil {
		return nil, nil, err
	}
	points, err := parseSevenHoleCalibrationRecords(src.Label, records[1:], cols, inner, sector)
	if err != nil {
		return nil, nil, err
	}
	return points, headerWarnings, nil
}

func parseSevenHoleCalibrationRecords(path string, records [][]string, cols sevenHoleCSVColumnSet, inner bool, sector int) (map[[2]float64]*sevenHoleCSVPoint, error) {
	points := make(map[[2]float64]*sevenHoleCSVPoint, len(records))
	for i, record := range records {
		point, key, err := parseSevenHoleCalibrationRecord(path, i+2, record, cols, inner, sector)
		if err != nil {
			return nil, err
		}
		if _, duplicate := points[key]; duplicate {
			return nil, fmt.Errorf("csv %s 第%d行: 重复网格点 (a=%v, b=%v)", path, i+2, key[0], key[1])
		}
		points[key] = point
	}
	return points, nil
}

func parseSevenHoleCalibrationRecord(path string, line int, record []string, cols sevenHoleCSVColumnSet, inner bool, sector int) (*sevenHoleCSVPoint, [2]float64, error) {
	// 列数校验：取所有必需列索引的最大值+1作为最小列数阈值。
	// 系数由压力列全精度重算，需要 P1..P7 + P0 + Ps（col3..11），
	// 加上角度列（col0/1），故最小必需列 = sevenHoleCSVP7Column+1。
	required := sevenHoleCSVP7Column + 1
	if len(record) < required {
		return nil, [2]float64{}, fmt.Errorf("csv %s 第%d行: 至少 %d 列（压力+角度必需列），实际 %d 列", path, line, required, len(record))
	}
	// 角度列按位置契约读取（a/b 语义由调用方 inner 决定）。
	aValue, err := strconv.ParseFloat(strings.TrimSpace(record[cols.a]), 64)
	if err != nil {
		return nil, [2]float64{}, fmt.Errorf("csv %s 第%d行第%d列: 不是有效数字 %q", path, line, cols.a+1, record[cols.a])
	}
	bValue, err := strconv.ParseFloat(strings.TrimSpace(record[cols.b]), 64)
	if err != nil {
		return nil, [2]float64{}, fmt.Errorf("csv %s 第%d行第%d列: 不是有效数字 %q", path, line, cols.b+1, record[cols.b])
	}
	key := [2]float64{aValue, bValue}

	var ka, kb, cpt, cps float64
	if inner {
		ka, kb, cpt, cps, err = recomputeSevenHoleInnerCoeffs(record)
	} else {
		ka, kb, cpt, cps, err = recomputeSevenHoleOuterCoeffs(record, sector)
	}
	if err != nil {
		return nil, [2]float64{}, fmt.Errorf("csv %s 第%d行: 系数重算失败: %v", path, line, err)
	}
	return &sevenHoleCSVPoint{ka: ka, kb: kb, cpt: cpt, cps: cps}, key, nil
}

func deriveSevenHoleOuterThetaGrid(points map[[2]float64]*sevenHoleCSVPoint, path string) ([]float64, error) {
	thetaSet := make(map[float64]struct{})
	for key := range points {
		thetaSet[key[0]] = struct{}{}
	}
	if len(thetaSet) < 2 {
		return nil, fmt.Errorf("csv %s: 外区 theta 网格点数 %d < 2（至少需要 2 点形成插值单元格）", path, len(thetaSet))
	}
	theta := make([]float64, 0, len(thetaSet))
	for value := range thetaSet {
		theta = append(theta, value)
	}
	sort.Float64s(theta)
	if math.Abs(theta[0]-OuterThetaMin) > GridEps {
		return nil, fmt.Errorf("csv %s: 外区 theta 起点 %.6g 必须 = %.0f", path, theta[0], OuterThetaMin)
	}
	for i := 1; i < len(theta); i++ {
		if step := theta[i] - theta[i-1]; math.Abs(step-GridStep) > GridEps {
			return nil, fmt.Errorf("csv %s: 外区 theta 步长 %.6g 必须 = %.0f（theta=%.6g→%.6g）", path, step, GridStep, theta[i-1], theta[i])
		}
	}
	return theta, nil
}

func findDegenerateSevenHoleCSVEdges(points map[[2]float64]*sevenHoleCSVPoint, aValues, bValues []float64) []sevenHoleNudgeTarget {
	var bad []sevenHoleNudgeTarget
	for bi, b := range bValues {
		for ai, a := range aValues {
			point := points[[2]float64{a, b}]
			if point == nil {
				continue
			}
			bad = appendSevenHoleHorizontalNudge(bad, points, point, aValues, ai, b)
			if bi+1 < len(bValues) {
				bad = appendSevenHoleVerticalNudges(bad, points, point, a, bValues[bi+1])
			}
		}
	}
	return bad
}

func appendSevenHoleHorizontalNudge(bad []sevenHoleNudgeTarget, points map[[2]float64]*sevenHoleCSVPoint, point *sevenHoleCSVPoint, aValues []float64, ai int, b float64) []sevenHoleNudgeTarget {
	if ai+1 >= len(aValues) {
		return bad
	}
	key := [2]float64{aValues[ai+1], b}
	if next := points[key]; next != nil && next.ka == point.ka {
		return append(bad, sevenHoleNudgeTarget{field: 0, key: key})
	}
	return bad
}

func appendSevenHoleVerticalNudges(bad []sevenHoleNudgeTarget, points map[[2]float64]*sevenHoleCSVPoint, point *sevenHoleCSVPoint, a, nextB float64) []sevenHoleNudgeTarget {
	key := [2]float64{a, nextB}
	next := points[key]
	if next == nil {
		return bad
	}
	if next.ka == point.ka {
		bad = append(bad, sevenHoleNudgeTarget{field: 0, key: key})
	}
	if next.kb == point.kb {
		bad = append(bad, sevenHoleNudgeTarget{field: 1, key: key})
	}
	return bad
}

// ditherSevenHoleCSVGrid 对退化边施加确定性抖动，使 bilinear 插值单元格可逆。
//
// 返回值：
//   - nudges: 实际施加的抖动次数（0 表示原始数据无退化边）
//   - error: 若 sevenHoleDitherPasses 轮后仍存在退化边，返回错误。这通常意味着
//     校准数据存在系统性异常（如大面积同 ka/kb 值的网格），后续 bilinear 计算
//     会除零或产生 NaN，让加载流程整体失败比静默失真更安全。
func ditherSevenHoleCSVGrid(points map[[2]float64]*sevenHoleCSVPoint, aValues, bValues []float64) (int, error) {
	nudges := 0
	for range sevenHoleDitherPasses {
		bad := findDegenerateSevenHoleCSVEdges(points, aValues, bValues)
		if len(bad) == 0 {
			return nudges, nil
		}
		for _, target := range bad {
			nudges++
			point := points[target.key]
			if target.field == 0 {
				point.ka += 1e-9 * float64(nudges)
			} else {
				point.kb += 1e-9 * float64(nudges)
			}
		}
	}
	// 100 轮抖动后仍存在退化边：校准数据存在系统性异常，
	// 返回错误而非 warning，避免后续 bilinear 计算静默产生 NaN/Inf。
	finalBad := findDegenerateSevenHoleCSVEdges(points, aValues, bValues)
	return nudges, fmt.Errorf("退化边抖动 %d 轮后仍存在 %d 处退化边，校准数据可能存在系统性异常", sevenHoleDitherPasses, len(finalBad))
}

func buildSevenHoleCSVPrbLines(points map[[2]float64]*sevenHoleCSVPoint, aValues, bValues []float64, capacity int) []string {
	lines := make([]string, 0, capacity+1)
	lines = append(lines, "ka kb cpt cps a b")
	for _, b := range bValues {
		for _, a := range aValues {
			point := points[[2]float64{a, b}]
			if point == nil {
				continue
			}
			lines = append(lines, fmt.Sprintf("%s %s %s %s %s %s", formatSevenHoleCSVFloat(point.ka), formatSevenHoleCSVFloat(point.kb), formatSevenHoleCSVFloat(point.cpt), formatSevenHoleCSVFloat(point.cps), formatSevenHoleCSVFloat(a), formatSevenHoleCSVFloat(b)))
		}
	}
	return lines
}

func formatSevenHoleCSVFloat(value float64) string {
	return strconv.FormatFloat(value, 'g', -1, 64)
}
