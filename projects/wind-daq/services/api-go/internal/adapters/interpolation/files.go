package interpolation

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	coreinterp "ai-workspace/shared/algorithms/go/fivehole/interpolation"
	seveninterp "ai-workspace/shared/algorithms/go/sevenhole/interpolation"
)

func LoadPrbFile(filePath string) (*coreinterp.PrbInterpolator, error) {
	lines, err := readNonEmptyLines(filePath)
	if err != nil {
		return nil, err
	}
	interpolator := coreinterp.NewPrbInterpolator()
	if err := interpolator.LoadPrbLines(lines, filePath); err != nil {
		return nil, err
	}
	return interpolator, nil
}

func LoadFiveHoleNewFile(filePath string) (*coreinterp.FiveHoleNewInterpolator, error) {
	lines, err := readNonEmptyLines(filePath)
	if err != nil {
		return nil, err
	}
	interpolator := coreinterp.NewFiveHoleNewInterpolator()
	if err := interpolator.LoadPrbLines(lines); err != nil {
		return nil, err
	}
	return interpolator, nil
}

func LoadMultiPrbFiles(filePaths []string, machNumbers []float64) (*coreinterp.MultiPrbInterpolator, *coreinterp.MultiPrbLoadResult, error) {
	data := make([]coreinterp.PrbFileData, 0, len(filePaths))
	for _, filePath := range filePaths {
		lines, err := readNonEmptyLines(filePath)
		if err != nil {
			return nil, nil, err
		}
		data = append(data, coreinterp.PrbFileData{
			FilePath: filePath,
			Lines:    lines,
		})
	}
	interpolator := coreinterp.NewMultiPrbInterpolator()
	result, err := interpolator.LoadPrbData(data, machNumbers)
	if err != nil {
		return nil, nil, err
	}
	return interpolator, result, nil
}

// LoadSevenHolePrbFiles 加载七孔 .prb 文件集（7.prb 内区 + 1.prb~6.prb 外区六个扇区）。
//
// 设计要点：
//   - 七孔 .prb 文件集由 7 个独立文件组成：1 个内区文件（13×13=169 点）+
//     6 个外区扇区文件（每个 4×13=52 点）。文件之间无依赖，但缺一不可，
//     否则后续插值会在缺失扇区返回错误。
//   - 这里只负责"读取文本行 + 调用 LoadXxxPrbLines 解析"，所有网格校验
//     （行数、列数、坐标在网格点上、重复点、缺失点）由 seveninterp 包内部
//     严格校验，本函数不做任何业务判定。
//   - sector 编号 1..6 对应 outerPaths[0..5]——这是七孔探针的物理孔位编号
//     约定（spec §3.2：外围 6 孔按 60° 等分），与文件名 1.prb..6.prb 一一对应。
//   - 失败时返回的 *SevenHolePrbInterpolator 仍可能部分填充（已加载的扇区
//     不会回滚），调用方应直接丢弃错误返回值并使用 err。
func LoadSevenHolePrbFiles(innerPath string, outerPaths [6]string) (*seveninterp.SevenHolePrbInterpolator, error) {
	interpolator := seveninterp.NewSevenHolePrbInterpolator()

	// 内区文件必须最先加载：外区插值在某些边界场景会回查内区网格，
	// 若内区未初始化会导致 panic。错误信息带上文件路径便于定位。
	innerLines, err := readNonEmptyLines(innerPath)
	if err != nil {
		return nil, fmt.Errorf("read inner prb %s: %w", innerPath, err)
	}
	if err := interpolator.LoadInnerPrbLines(innerLines, innerPath); err != nil {
		return nil, fmt.Errorf("parse inner prb %s: %w", innerPath, err)
	}

	// 6 个外区文件按 sector=1..6 顺序加载。任一文件缺失/格式错误立即返回，
	// 不尝试继续加载后续扇区——避免错误信息被淹没。
	for i, outerPath := range outerPaths {
		sector := i + 1
		outerLines, err := readNonEmptyLines(outerPath)
		if err != nil {
			return nil, fmt.Errorf("read outer prb sector %d %s: %w", sector, outerPath, err)
		}
		if err := interpolator.LoadOuterPrbLines(sector, outerLines, outerPath); err != nil {
			return nil, fmt.Errorf("parse outer prb sector %d %s: %w", sector, outerPath, err)
		}
	}

	return interpolator, nil
}

func readNonEmptyLines(filePath string) ([]string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("open calibration file: %w", err)
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
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read calibration file: %w", err)
	}
	return lines, nil
}
