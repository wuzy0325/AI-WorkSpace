package ports

import (
	coreinterp "ai-workspace/shared/algorithms/go/fivehole/interpolation"
	seveninterp "ai-workspace/shared/algorithms/go/sevenhole/interpolation"
)

// PrbFileMetadata 是 port 层定义的中立"已加载 PRB 文件信息"，
// 供 multi-PRB endpoint 响应字段使用（filePath/fileName/machNumber/validRange）。
//
// 设计要点（Task 07）：
//   - 该结构独立于 shared/algorithms.PrbFileInfo——usecase/api 只看 ports 类型，
//     不依赖 adapter shared load-result 具体类型，遵守六边形分层。
//   - LoadedAtMs 由 adapter 调用方填充（time.Now().UnixMilli()），ports 不引入
//     time 包依赖；0 表示未提供。
//   - MachNumber 仅在 multi-PRB 加载时有意义；单 PRB/CSV 路径不使用该字段。
type PrbFileMetadata struct {
	FilePath   string                   `json:"filePath"`
	FileName   string                   `json:"fileName"`
	LoadedAtMs int64                    `json:"loadedAt"`
	MachNumber float64                  `json:"machNumber"`
	ValidRange coreinterp.PrbValidRange `json:"validRange"`
}

// MultiPrbLoadMetadata 是 multi-PRB 加载返回的中立元数据。
//
// 把 shared.MultiPrbLoadResult 的具体字段映射到 port 层：
//   - Files：仅成功加载的文件（与 MachNumbers 一一对应，按 Mach 升序）。
//   - Warnings：非致命警告，包含 skipped（解析失败/无法解析 Mach）和
//     duplicate（Mach 重复）以及 single-file 退化提示。
//
// 调用方（API / usecase）基于该结构组装响应，不再 import
// shared/algorithms 的 MultiPrbLoadResult 具体类型。
type MultiPrbLoadMetadata struct {
	Files       []PrbFileMetadata
	MachNumbers []float64
	Warnings    []string
}

// SevenHoleLoadMetadata 是七孔 PRB / 校准 CSV 加载返回的中立元数据。
//
// 动态 theta 维度（外区行数不固定 52）：loader 通过 GetInnerPointCount /
// GetOuterPointCount 读取真实点数并填入 InnerPointCount / OuterPointCounts，
// 调用方据此返回前端，不再使用 169/52 兼容约定值。InnerPointCount 固定 169
// （内区物理设计），OuterPointCounts 长度恒为 6、索引 0..5 对应扇区 1..6，
// 每个值为该扇区实际加载的 thetaCount×13 点数（如 4×13=52、7×13=91）。
type SevenHoleLoadMetadata struct {
	LoadedAtMs         int64                    `json:"loadedAt"`
	ValidRange         seveninterp.PrbValidRange `json:"validRange"`
	InnerPointCount    int                      `json:"innerPointCount"`    // 内区实际点数（固定 169）
	OuterPointCounts   [6]int                   `json:"outerPointCounts"`   // 各扇区外区实际点数（动态）
}

// InterpolatorLoader 插值器加载端口。
//
// 目的：让 usecase 层在启动恢复 / 显式加载时，能够通过统一接口从磁盘加载
// PRB / CSV / 多 PRB 插值器，**而不直接依赖 adapters/interpolation 实现包**。
// 这是工作区六边形分层的硬约束：usecase 必须通过 ports 访问外部能力，不允许 import adapters。
//
// 错误处理约定：
//   - 文件不存在、解析失败、行非法等错误均通过返回的 error 暴露，
//     调用方（如 TraversalManager.restoreInterpolatorFromConfig）会把
//     错误消息写入按探针类型分桶的 lastFiveHoleRestoreErr /
//     lastSevenHoleRestoreErr，供前端经 CheckPreconditions 按激活类型读取展示。
//   - 实现可以选择封装底层 IO 错误，但必须保留可读性（建议 fmt.Errorf("...: %w", err)）。
//
// 元数据约定（Task 07）：
//   - multi-PRB 加载返回 *MultiPrbLoadMetadata，包含 skipped/duplicate warnings
//     和 Mach 关联信息；adapter 负责把 shared.MultiPrbLoadResult 映射到该结构。
//   - 七孔加载返回 *SevenHoleLoadMetadata，仅包含 LoadedAtMs 与 ValidRange；
//     不暴露 PointCount（兼容约定值 169/52 不应伪装为 loader 真值）。
//   - 单 PRB / CSV 不返回复杂 metadata，调用方直接通过 Interpolator.GetValidRange()
//     读取有效范围；这与既有 endpoint 响应结构保持兼容。
type InterpolatorLoader interface {
	// LoadPRB 加载单 PRB 文件。
	LoadPRB(filePath string) (coreinterp.Interpolator, error)

	// LoadFiveHoleCSV 加载"新算法"标定 CSV 文件。
	LoadFiveHoleCSV(filePath string) (coreinterp.Interpolator, error)

	// LoadMultiPRB 加载多 PRB 文件并按 Mach 数构建插值器。
	// mode 为 "" 时由实现选择默认模式（通常为 nearest）。
	// 返回的 *MultiPrbLoadMetadata 包含成功加载文件列表、Mach 关联和警告
	// （skipped/duplicate）；调用方据此组装 endpoint 响应。
	LoadMultiPRB(filePaths []string, machNumbers []float64, mode coreinterp.MultiPrbInterpolationMode) (coreinterp.Interpolator, *MultiPrbLoadMetadata, error)

	// LoadSevenHolePRB 加载七孔 .prb 文件集（innerPath=7.prb，outerPaths 按孔号 1..6 顺序 6 份）。
	// 文件缺失、行数/列数非法、网格点缺失或重复均通过 error 暴露
	// （spec-seven-hole-traversal §5.3）。
	// 返回的 *SevenHoleLoadMetadata 含 LoadedAtMs/ValidRange 与真实点数
	// （InnerPointCount 固定 169，OuterPointCounts 为各扇区 thetaCount×13 动态值）。
	LoadSevenHolePRB(innerPath string, outerPaths [6]string) (seveninterp.Interpolator, *SevenHoleLoadMetadata, error)

	// LoadSevenHoleCalibrationCSV 从七孔校准 CSV 文件集构建插值器
	// （GBK 编码、列位置契约；内区 1 份 169 行 + 外区 6 份各 thetaCount×13 行，
	// 校准 CSV → 插值网格的转换在 adapters 层完成，spec §10 Q2 落地）。
	// 返回的 *SevenHoleLoadMetadata 与 LoadSevenHolePRB 同语义。
	LoadSevenHoleCalibrationCSV(innerPath string, outerPaths [6]string) (seveninterp.Interpolator, *SevenHoleLoadMetadata, error)
}
