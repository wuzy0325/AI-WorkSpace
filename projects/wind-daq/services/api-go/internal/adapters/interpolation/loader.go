package interpolation

import (
	"fmt"
	"path/filepath"
	"time"

	coreinterp "ai-workspace/shared/algorithms/go/fivehole/interpolation"
	seveninterp "ai-workspace/shared/algorithms/go/sevenhole/interpolation"
	"wind-daq/services/api-go/internal/ports"
)

// Loader 实现 ports.InterpolatorLoader，负责将 PRB / CSV / 多 PRB 文件
// 通过现有的 LoadXxxFile/LoadMultiPrbFiles 函数加载为统一的 coreinterp.Interpolator。
//
// 该类型刻意做成无字段的零值可用结构，便于在 wiring / bootstrap / appcontext
// 中通过 interpolation.NewLoader() 注入到 TraversalManager。
type Loader struct{}

// NewLoader 返回一个零值 Loader。
func NewLoader() *Loader { return &Loader{} }

// LoadPRB 加载单 PRB 文件，错误透传。
// 注意：底层 LoadPrbFile 返回具体类型 *PrbInterpolator，
// 通过 if err != nil 分支显式返回 nil interface 值，避免 typed-nil 陷阱。
func (Loader) LoadPRB(filePath string) (coreinterp.Interpolator, error) {
	interp, err := LoadPrbFile(filePath)
	if err != nil {
		return nil, err
	}
	return interp, nil
}

// LoadFiveHoleCSV 加载新算法 CSV 标定文件，错误透传。
// 同 LoadPRB：失败路径必须显式返回 nil 接口值，防止外层接口判 nil 失效。
func (Loader) LoadFiveHoleCSV(filePath string) (coreinterp.Interpolator, error) {
	interp, err := LoadFiveHoleNewFile(filePath)
	if err != nil {
		return nil, err
	}
	return interp, nil
}

// LoadMultiPRB 加载多 PRB 文件，按需设置插值模式后返回。
// mode 为空字符串时不调用 SetInterpolationMode，保持实现侧的默认值。
//
// 元数据映射（Task 07）：把 shared.MultiPrbLoadResult 中的 Files/MachNumbers/Warnings
// 映射到 port-owned ports.MultiPrbLoadMetadata，避免 usecase/api 层 import shared
// load-result 具体类型。LoadedAtMs 由本层 time.Now().UnixMilli() 填充。
func (Loader) LoadMultiPRB(filePaths []string, machNumbers []float64, mode coreinterp.MultiPrbInterpolationMode) (coreinterp.Interpolator, *ports.MultiPrbLoadMetadata, error) {
	interpolator, result, err := LoadMultiPrbFiles(filePaths, machNumbers)
	if err != nil {
		return nil, nil, err
	}
	if mode != "" {
		interpolator.SetInterpolationMode(mode)
	}

	// 把 shared.PrbFileInfo 列表映射到 ports.PrbFileMetadata：
	// 按 result.Files 顺序（已经过 LoadPrbData 内部按 Mach 升序排序），
	// 与 result.MachNumbers 一一对应。
	loadedAtMs := time.Now().UnixMilli()
	files := make([]ports.PrbFileMetadata, 0, len(result.Files))
	for i, f := range result.Files {
		var machNumber float64
		if i < len(result.MachNumbers) {
			machNumber = result.MachNumbers[i]
		}
		// FilePrbFileInfo 中 FileName 来自 filepath.Base，已避免空值；
		// 若 LoadPrbData 内部未填充 FileName，这里再兜底（防御性编程，零成本）。
		fileName := f.FileName
		if fileName == "" {
			fileName = filepath.Base(f.FilePath)
		}
		files = append(files, ports.PrbFileMetadata{
			FilePath:   f.FilePath,
			FileName:   fileName,
			LoadedAtMs: loadedAtMs,
			MachNumber: machNumber,
			ValidRange: f.ValidRange,
		})
	}

	metadata := &ports.MultiPrbLoadMetadata{
		Files:       files,
		MachNumbers: append([]float64(nil), result.MachNumbers...),
		Warnings:    append([]string(nil), result.Warnings...),
	}
	return interpolator, metadata, nil
}

// LoadSevenHolePRB 加载七孔 .prb 文件集，错误透传。
// 同 LoadPRB：失败路径必须显式返回 nil 接口值，防止外层接口判 nil 失效。
//
// 元数据：返回 *ports.SevenHoleLoadMetadata，含 LoadedAtMs/ValidRange 与真实点数
// （InnerPointCount/OuterPointCounts）。点数通过类型断言到 *SevenHolePrbInterpolator
// 调用 GetInnerPointCount/GetOuterPointCount 获取——动态 theta 维度下不再硬编码
// 169/52，使前端能展示各扇区实际加载的网格点数（如 4×13=52、7×13=91）。
func (Loader) LoadSevenHolePRB(innerPath string, outerPaths [6]string) (seveninterp.Interpolator, *ports.SevenHoleLoadMetadata, error) {
	interp, err := LoadSevenHolePrbFiles(innerPath, outerPaths)
	if err != nil {
		return nil, nil, err
	}
	metadata, err := buildSevenHoleMetadata(interp)
	if err != nil {
		return nil, nil, err
	}
	return interp, metadata, nil
}

// LoadSevenHoleCalibrationCSV 从七孔校准 CSV 文件集构建插值器，错误透传。
// 同 LoadPRB：失败路径显式返回 nil 接口值（typed-nil 防护）。
//
// 元数据与 LoadSevenHolePRB 同语义。
func (Loader) LoadSevenHoleCalibrationCSV(innerPath string, outerPaths [6]string) (seveninterp.Interpolator, *ports.SevenHoleLoadMetadata, error) {
	interp, err := LoadSevenHoleCalibrationCsvFiles(innerPath, outerPaths)
	if err != nil {
		return nil, nil, err
	}
	metadata, err := buildSevenHoleMetadata(interp)
	if err != nil {
		return nil, nil, err
	}
	return interp, metadata, nil
}

// buildSevenHoleMetadata 从七孔插值器读取真实点数并组装 metadata。
// 类型断言到 *SevenHolePrbInterpolator 调用 GetInnerPointCount /
// GetOuterPointCount；若未来加载入口返回其他具体类型，则显式报错，避免
// pointCount=0 被前端当作无点数而隐藏。
func buildSevenHoleMetadata(interp seveninterp.Interpolator) (*ports.SevenHoleLoadMetadata, error) {
	metadata := &ports.SevenHoleLoadMetadata{
		LoadedAtMs: time.Now().UnixMilli(),
		ValidRange: interp.GetValidRange(),
	}
	p, ok := interp.(*seveninterp.SevenHolePrbInterpolator)
	if !ok {
		return nil, fmt.Errorf("build seven-hole metadata: unsupported interpolator type %T", interp)
	}
	metadata.InnerPointCount = p.GetInnerPointCount()
	for sector := 1; sector <= 6; sector++ {
		metadata.OuterPointCounts[sector-1] = p.GetOuterPointCount(sector)
	}
	return metadata, nil
}
