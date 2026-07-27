package backend

// seven_hole_csv.go 实现七孔校准 CSV → PRB 行集的转换，使 probe-interpolator
// 能直接导入校准导出的 GBK 编码 CSV（无需先导出为 .prb）。
//
// 参考来源：projects/wind-daq/services/api-go/internal/adapters/interpolation/seven_hole_csv.go
//
// 当前两份代码已分叉，不再保持"同名同实现"：
//   - wind-daq：导出函数 LoadSevenHoleCalibrationCsvFiles(inner, [6]string) (*interp, error)
//   - probe-interpolator：私有 loadSevenHoleCalibrationCsvFiles(inner, []string) (*interp, []string, error)
//     签名差异：入参改切片（调用方已校验长度）、返回值多一个 warnings（CSV 退化边抖动次数提示）
//
// 列位置契约、退化边抖动、网格校验逻辑保持一致；未来提取到
// shared/algorithms/go/sevenhole/interpolation 共享包时需先统一签名（建议采用 probe-interpolator
// 版本的签名：切片入参 + warnings 返回值，调用方自行校验长度）。当前为避免影响
// wind-daq 而在本项目内独立演化。
//
// 列位置契约（与 device-lab/skills/seven-hole-probe/tools/gen_traversal_fixtures.py 一致，
// 表头存在历史命名错误，必须按列位置读取，spec-seven-hole-calibration §12.1）：
//   - 内区（小角度区 CSV）：a=col0, b=col1；ka=col12, kb=col13, cpt=col14, cps=col15
//   - 外区（大角度 n 区 CSV）：theta=col1, phi=col0；ka=col12(Kθ[n]), kb=col13(Kφ[n]),
//     cpt=col14(K0[n]), cps=col15(Ks[n])
//
// 网格规模：内区恰 169 数据行（13×13，a/b ∈ [-30,30] 步长 5）；
// 外区每份 thetaCount×13 数据行（thetaCount 动态，从 CSV 实际数据推断，
// 必须 ≥2 且 theta ∈ {30,35,...,30+5*(thetaCount-1)}）。
//
// GBK 编码：校准导出 CSV 为 GBK（中文表头），本层解码为 UTF-8 后按逗号分隔解析。
//
// 退化边抖动：3 位小数系数在相邻网格点间产生精确 ka/kb 相等值，形成插值
// 定位的退化边（Python 参考实现遇此直接除零崩溃，gen_traversal_fixtures.py
// 头注释）。转换时复刻夹具脚本的确定性抖动：仅对精确相等的退化边按扫描序
// 累加 1e-9 级增量，数值影响远低于对拍容差（角度 1e-4°）。

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"math"
	"os"
	"sort"
	"strconv"

	seveninterp "ai-workspace/shared/algorithms/go/sevenhole/interpolation"
	"golang.org/x/text/encoding/simplifiedchinese"
)

const (
	sevenHoleInnerCsvRows = 169
	sevenHoleCsvMinCols   = 16 // 至少需要覆盖系数列 col12..col15
)

// sevenHoleSectorPhiLines 各扇区 13 条 φ 网格线（索引序=相邻关系，含 0/360 跨界）。
// 与 prb_loader.outerPhiIndex 的物理设计保持一致：center+30 → center-30 步长 -5。
var sevenHoleSectorPhiLines = [6][13]float64{
	{30, 25, 20, 15, 10, 5, 0, 355, 350, 345, 340, 335, 330},
	{90, 85, 80, 75, 70, 65, 60, 55, 50, 45, 40, 35, 30},
	{150, 145, 140, 135, 130, 125, 120, 115, 110, 105, 100, 95, 90},
	{210, 205, 200, 195, 190, 185, 180, 175, 170, 165, 160, 155, 150},
	{270, 265, 260, 255, 250, 245, 240, 235, 230, 225, 220, 215, 210},
	{330, 325, 320, 315, 310, 305, 300, 295, 290, 285, 280, 275, 270},
}

// sevenHoleGridPoint 解析后的单个校准网格点系数。
type sevenHoleGridPoint struct {
	ka, kb, cpt, cps float64
}

// loadSevenHoleCalibrationCsvFiles 从七孔校准 CSV 文件集构建七孔插值器：
// 1 份内区 CSV（169 行）+ 6 份外区 CSV（按孔号 1..6 顺序，每份 thetaCount×13 行，
// thetaCount 由 CSV 实际数据动态推断，不再硬编码 52）。
// 文件缺失、GBK/CSV 解析失败、列数/数值非法、网格覆盖不符均通过 error 暴露并含路径。
//
// 转换流程：CSV → 网格映射 → 退化边抖动 → 渲染为 .prb 行集 → 复用
// SevenHolePrbInterpolator.LoadInnerPrbLines / LoadOuterPrbLines 的强校验
// （行数/网格覆盖/重复点/越界点）。两种数据源在算法包层完全统一。
//
// 返回值 warnings 收集退化边抖动次数提示，供前端 UI 展示"已对 N 处退化边施加抖动"
// （让用户可观测到 CSV 解析的隐式修正，避免静默修改系数）。
//
// 入参 outerPaths 用 []string 而非 [6]string：调用方 LoadSevenHoleCalibrationCsvFiles
// 已校验长度为 6，固定长度数组会强制外层做无意义的 copy 转换，切片更符合 Go 习惯。
func loadSevenHoleCalibrationCsvFiles(innerPath string, outerPaths []string) (*seveninterp.SevenHolePrbInterpolator, []string, error) {
	// 内区 a/b 网格线：13 个点，[-30, 30] 步长 5（物理设计固定）。
	gridLines := make([]float64, 13)
	for i := range gridLines {
		gridLines[i] = -30 + 5*float64(i)
	}

	innerPoints, err := parseSevenHoleCsv(innerPath, true)
	if err != nil {
		return nil, nil, err
	}
	innerNudges := ditherSevenHoleGrid(innerPoints, gridLines, gridLines)
	innerLines := buildSevenHolePrbLines(innerPoints, gridLines, gridLines, sevenHoleInnerCsvRows)

	interpolator := seveninterp.NewSevenHolePrbInterpolator()
	if err := interpolator.LoadInnerPrbLines(innerLines, innerPath); err != nil {
		return nil, nil, err
	}

	var warnings []string
	if innerNudges > 0 {
		warnings = append(warnings, fmt.Sprintf("内区 CSV 已对 %d 处退化边施加确定性抖动（1e-9 量级）", innerNudges))
	}

	for i, path := range outerPaths {
		sector := i + 1
		points, err := parseSevenHoleCsv(path, false)
		if err != nil {
			return nil, nil, err
		}
		// 动态推断 theta 网格点：从解析数据中收集所有 theta 值（去重、升序），
		// 校验起点为 30、步长为 5（与 prb_loader 物理设计一致）。
		aVals, err := deriveOuterThetaGrid(points, path)
		if err != nil {
			return nil, nil, err
		}
		bVals := sevenHoleSectorPhiLines[i][:]
		outerNudges := ditherSevenHoleGrid(points, aVals, bVals)
		if outerNudges > 0 {
			warnings = append(warnings, fmt.Sprintf("扇区 %d CSV 已对 %d 处退化边施加确定性抖动（1e-9 量级）", sector, outerNudges))
		}
		lines := buildSevenHolePrbLines(points, aVals, bVals, len(aVals)*len(bVals))
		if err := interpolator.LoadOuterPrbLines(sector, lines, path); err != nil {
			return nil, nil, err
		}
	}
	return interpolator, warnings, nil
}

// deriveOuterThetaGrid 从外区 CSV 解析结果中动态推断 theta 网格点序列。
// 校验：theta 必须从 seveninterp.OuterThetaMin（30）起、步长 seveninterp.GridStep（5）、
// 至少 2 个点（与 prb_loader 的动态约束一致）。错误含路径便于定位。
// 物理常量直接引用 seveninterp 包，避免本地重复定义导致双源不一致。
func deriveOuterThetaGrid(points map[[2]float64]*sevenHoleGridPoint, path string) ([]float64, error) {
	thetaSet := make(map[float64]struct{})
	for k := range points {
		thetaSet[k[0]] = struct{}{} // key[0]=theta（外区列契约 col1）
	}
	if len(thetaSet) < 2 {
		return nil, fmt.Errorf("csv %s: 外区 theta 网格点数 %d < 2（至少需要 2 点形成插值单元格）", path, len(thetaSet))
	}
	thetas := make([]float64, 0, len(thetaSet))
	for t := range thetaSet {
		thetas = append(thetas, t)
	}
	sort.Float64s(thetas)
	// 校验起点与步长（与 prb_loader 物理设计一致）
	if math.Abs(thetas[0]-seveninterp.OuterThetaMin) > seveninterp.GridEps {
		return nil, fmt.Errorf("csv %s: 外区 theta 起点 %.6g 必须 = %.0f", path, thetas[0], seveninterp.OuterThetaMin)
	}
	for i := 1; i < len(thetas); i++ {
		diff := thetas[i] - thetas[i-1]
		if math.Abs(diff-seveninterp.GridStep) > seveninterp.GridEps {
			return nil, fmt.Errorf("csv %s: 外区 theta 步长 %.6g 必须 = %.0f（theta=%.6g→%.6g）",
				path, diff, seveninterp.GridStep, thetas[i-1], thetas[i])
		}
	}
	return thetas, nil
}

// parseSevenHoleCsv 读取并解析一份 GBK 校准 CSV：返回 (a,b)→系数 的网格映射。
// inner=true 时按内区列契约（a=col0, b=col1），否则按外区（theta=col1→a, phi=col0→b）。
func parseSevenHoleCsv(path string, inner bool) (map[[2]float64]*sevenHoleGridPoint, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read seven-hole calibration csv %s: %w", path, err)
	}
	utf8, err := simplifiedchinese.GBK.NewDecoder().Bytes(raw)
	if err != nil {
		return nil, fmt.Errorf("decode GBK csv %s: %w", path, err)
	}
	reader := csv.NewReader(bytes.NewReader(utf8))
	reader.FieldsPerRecord = -1 // 行宽自适配，列数按行校验（错误消息更明确）
	records, err := reader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("parse csv %s: %w", path, err)
	}
	if len(records) < 2 {
		return nil, fmt.Errorf("csv %s: 至少需要表头和一行数据", path)
	}
	points := make(map[[2]float64]*sevenHoleGridPoint, len(records)-1)
	for i, rec := range records[1:] {
		lineNo := i + 2 // 表头占第 1 行
		if len(rec) < sevenHoleCsvMinCols {
			return nil, fmt.Errorf("csv %s 第%d行: 至少 %d 列（含系数列 12..15），实际 %d 列", path, lineNo, sevenHoleCsvMinCols, len(rec))
		}
		vals := make([]float64, 6)
		var cols [6]int
		if inner {
			cols = [6]int{12, 13, 14, 15, 0, 1} // ka, kb, cpt, cps, a, b
		} else {
			cols = [6]int{12, 13, 14, 15, 1, 0} // ka, kb, cpt, cps, theta, phi
		}
		for j, c := range cols {
			v, err := strconv.ParseFloat(rec[c], 64)
			if err != nil {
				return nil, fmt.Errorf("csv %s 第%d行第%d列: 不是有效数字 %q", path, lineNo, c+1, rec[c])
			}
			vals[j] = v
		}
		key := [2]float64{vals[4], vals[5]}
		if _, dup := points[key]; dup {
			return nil, fmt.Errorf("csv %s 第%d行: 重复网格点 (a=%v, b=%v)", path, lineNo, vals[4], vals[5])
		}
		points[key] = &sevenHoleGridPoint{ka: vals[0], kb: vals[1], cpt: vals[2], cps: vals[3]}
	}
	return points, nil
}

// ditherSevenHoleGrid 对精确相等的退化边加确定性 1e-9 级抖动
// （复刻 gen_traversal_fixtures.py 的 nudge_degenerate 扫描序与增量）：
// a 方向相邻边要求 Δka≠0；b 方向相邻边要求 Δka≠0 且 Δkb≠0。
// 返回抖动次数（0 表示无退化边）。
func ditherSevenHoleGrid(points map[[2]float64]*sevenHoleGridPoint, aVals, bVals []float64) int {
	nudges := 0
	type nudgeTarget struct {
		field int // 0=ka, 1=kb
		key   [2]float64
	}
	badEdges := func() []nudgeTarget {
		var bad []nudgeTarget
		for bi := 0; bi < len(bVals); bi++ {
			for ai := 0; ai < len(aVals); ai++ {
				p := points[[2]float64{aVals[ai], bVals[bi]}]
				if p == nil {
					continue
				}
				if ai+1 < len(aVals) {
					q := points[[2]float64{aVals[ai+1], bVals[bi]}]
					if q != nil && q.ka == p.ka {
						bad = append(bad, nudgeTarget{0, [2]float64{aVals[ai+1], bVals[bi]}})
					}
				}
				if bi+1 < len(bVals) {
					q := points[[2]float64{aVals[ai], bVals[bi+1]}]
					if q != nil {
						if q.ka == p.ka {
							bad = append(bad, nudgeTarget{0, [2]float64{aVals[ai], bVals[bi+1]}})
						}
						if q.kb == p.kb {
							bad = append(bad, nudgeTarget{1, [2]float64{aVals[ai], bVals[bi+1]}})
						}
					}
				}
			}
		}
		return bad
	}
	for range 100 {
		bad := badEdges()
		if len(bad) == 0 {
			return nudges
		}
		for _, t := range bad {
			nudges++
			p := points[t.key]
			if t.field == 0 {
				p.ka += 1e-9 * float64(nudges)
			} else {
				p.kb += 1e-9 * float64(nudges)
			}
		}
	}
	return nudges
}

// buildSevenHolePrbLines 把网格映射渲染为 .prb 行集（复用既有 loader 的
// 强校验：行数/网格覆盖/重复点/越界点），行序为 b 外层、a 内层。
// wantRows 仅用于容量预分配；行数不符由 loader 行数校验统一报错（含来源路径）。
func buildSevenHolePrbLines(points map[[2]float64]*sevenHoleGridPoint, aVals, bVals []float64, wantRows int) []string {
	lines := make([]string, 0, wantRows+1)
	lines = append(lines, "ka kb cpt cps a b")
	for _, b := range bVals {
		for _, a := range aVals {
			p := points[[2]float64{a, b}]
			if p == nil {
				continue
			}
			lines = append(lines, fmt.Sprintf("%s %s %s %s %s %s",
				strconv.FormatFloat(p.ka, 'g', -1, 64),
				strconv.FormatFloat(p.kb, 'g', -1, 64),
				strconv.FormatFloat(p.cpt, 'g', -1, 64),
				strconv.FormatFloat(p.cps, 'g', -1, 64),
				strconv.FormatFloat(a, 'g', -1, 64),
				strconv.FormatFloat(b, 'g', -1, 64)))
		}
	}
	return lines
}
