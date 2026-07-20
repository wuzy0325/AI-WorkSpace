package ports

import (
	coreinterp "ai-workspace/shared/algorithms/go/fivehole/interpolation"
	seveninterp "ai-workspace/shared/algorithms/go/sevenhole/interpolation"
)

// InterpolatorLoader 插值器加载端口。
//
// 目的：让 usecase 层在启动恢复 / 显式加载时，能够通过统一接口从磁盘加载
// PRB / CSV / 多 PRB 插值器，**而不直接依赖 adapters/interpolation 实现包**。
// 这是工作区六边形分层的硬约束：usecase 必须通过 ports 访问外部能力，不允许 import adapters。
//
// 错误处理约定：
//   - 文件不存在、解析失败、行非法等错误均通过返回的 error 暴露，
//     调用方（如 TraversalManager.restoreInterpolatorFromConfig）会把
//     错误消息写入 lastInterpolatorRestoreErr 供前端展示。
//   - 实现可以选择封装底层 IO 错误，但必须保留可读性（建议 fmt.Errorf("...: %w", err)）。
type InterpolatorLoader interface {
	// LoadPRB 加载单 PRB 文件。
	LoadPRB(filePath string) (coreinterp.Interpolator, error)

	// LoadFiveHoleCSV 加载"新算法"标定 CSV 文件。
	LoadFiveHoleCSV(filePath string) (coreinterp.Interpolator, error)

	// LoadMultiPRB 加载多 PRB 文件并按 Mach 数构建插值器。
	// mode 为 "" 时由实现选择默认模式（通常为 nearest）。
	LoadMultiPRB(filePaths []string, machNumbers []float64, mode coreinterp.MultiPrbInterpolationMode) (coreinterp.Interpolator, error)

	// LoadSevenHolePRB 加载七孔 .prb 文件集（innerPath=7.prb，outerPaths 按孔号 1..6 顺序 6 份）。
	// 文件缺失、行数/列数非法、网格点缺失或重复均通过 error 暴露
	// （spec-seven-hole-traversal §5.3）。
	LoadSevenHolePRB(innerPath string, outerPaths [6]string) (seveninterp.Interpolator, error)

	// LoadSevenHoleCalibrationCSV 从七孔校准 CSV 文件集构建插值器
	// （GBK 编码、列位置契约；内区 1 份 169 行 + 外区 6 份各 52 行，
	// 校准 CSV → 插值网格的转换在 adapters 层完成，spec §10 Q2 落地）。
	LoadSevenHoleCalibrationCSV(innerPath string, outerPaths [6]string) (seveninterp.Interpolator, error)
}
