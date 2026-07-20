package interpolation

import (
	coreinterp "ai-workspace/shared/algorithms/go/fivehole/interpolation"
	seveninterp "ai-workspace/shared/algorithms/go/sevenhole/interpolation"
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
func (Loader) LoadMultiPRB(filePaths []string, machNumbers []float64, mode coreinterp.MultiPrbInterpolationMode) (coreinterp.Interpolator, error) {
	interpolator, _, err := LoadMultiPrbFiles(filePaths, machNumbers)
	if err != nil {
		return nil, err
	}
	if mode != "" {
		interpolator.SetInterpolationMode(mode)
	}
	return interpolator, nil
}

// LoadSevenHolePRB 加载七孔 .prb 文件集，错误透传。
// 同 LoadPRB：失败路径必须显式返回 nil 接口值，防止外层接口判 nil 失效。
func (Loader) LoadSevenHolePRB(innerPath string, outerPaths [6]string) (seveninterp.Interpolator, error) {
	interp, err := LoadSevenHolePrbFiles(innerPath, outerPaths)
	if err != nil {
		return nil, err
	}
	return interp, nil
}

// LoadSevenHoleCalibrationCSV 从七孔校准 CSV 文件集构建插值器，错误透传。
// 同 LoadPRB：失败路径显式返回 nil 接口值（typed-nil 防护）。
func (Loader) LoadSevenHoleCalibrationCSV(innerPath string, outerPaths [6]string) (seveninterp.Interpolator, error) {
	interp, err := LoadSevenHoleCalibrationCsvFiles(innerPath, outerPaths)
	if err != nil {
		return nil, err
	}
	return interp, nil
}
