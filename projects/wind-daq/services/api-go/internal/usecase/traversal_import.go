// Package usecase — 显式插值导入收口（Task 08）。
//
// 本文件托管 TraversalManager 的五个 Import 方法，把原本散落在 api/server.go 的
// adapter 直接调用（interpfiles.LoadXxx + interpolator.SetInterpolationMode 等）
// 收口到 usecase 层。API 层从此只负责 HTTP 解码 + 调用 usecase + 序列化响应，
// 不再 import internal/adapters/interpolation，也不再直接操作插值器 mode。
//
// 关键不变量（spec §Slice B2）：
//  1. load 成功后才替换 manager 状态（SetInterpolator / SetSevenHoleInterpolator），
//     失败保留旧 interpolator——避免短暂窗口内"无插值器"导致实时计算崩。
//  2. 七孔响应 pointCount 从 metadata 读取真实值（InnerPointCount/OuterPointCounts），
//     不再使用 169/52 兼容约定值——外区 theta 维度已动态化，不同校准集点数不同
//     （4×13=52、7×13=91 等），约定值会误导前端。
//  3. 五孔 CSV 的 pointCount 通过类型断言 *coreinterp.FiveHoleNewInterpolator 获取——
//     coreinterp.Interpolator 接口刻意不暴露 GetPointCount（仅具体类型支持），
//     断言失败时降级为 0（与旧 API 行为一致：旧 API 也通过具体类型方法读取）。
//  4. mode 字符串由 usecase 转 coreinterp.MultiPrbInterpolationMode 透传给 loader，
//     API 不再 import coreinterp 来做转换——loader 内部已处理 SetInterpolationMode。
package usecase

import (
	"errors"
	"fmt"
	"path/filepath"
	"time"

	coreinterp "ai-workspace/shared/algorithms/go/fivehole/interpolation"
	seveninterp "ai-workspace/shared/algorithms/go/sevenhole/interpolation"
	"wind-daq/services/api-go/internal/ports"
)

// ==================== 响应 DTO ====================
//
// 这些结构定义 usecase → API 的响应形状，json tag 与既有 API 响应字段逐字对齐，
// 保证前端无感知。API 层只需 writeJSON(w, http.StatusOK, res) 即可。

// PrbImportResult 单 PRB 导入响应（对应 importPrb action）。
type PrbImportResult struct {
	FilePath   string                   `json:"filePath"`
	FileName   string                   `json:"fileName"`
	LoadedAtMs int64                    `json:"loadedAt"`
	ValidRange coreinterp.PrbValidRange `json:"validRange"`
}

// CalibrationCsvImportResult 五孔校准 CSV 导入响应（对应 importCalibrationCsv action）。
// PointCount 来自具体类型 GetPointCount()，类型断言失败时为 0（降级，与旧 API 行为一致）。
type CalibrationCsvImportResult struct {
	FilePath   string                   `json:"filePath"`
	FileName   string                   `json:"fileName"`
	LoadedAtMs int64                    `json:"loadedAt"`
	ValidRange coreinterp.PrbValidRange `json:"validRange"`
	PointCount int                      `json:"pointCount"`
}

// MultiPrbImportResult 多 PRB 导入响应（对应 importMultiPrb action）。
type MultiPrbImportResult struct {
	Files       []MultiPrbFileInfo `json:"files"`
	MachNumbers []float64          `json:"machNumbers"`
	Warnings    []string           `json:"warnings"`
}

// MultiPrbFileInfo 多 PRB 中单个文件的信息（与 ports.PrbFileMetadata 字段对齐）。
type MultiPrbFileInfo struct {
	FilePath   string                   `json:"filePath"`
	FileName   string                   `json:"fileName"`
	LoadedAtMs int64                    `json:"loadedAt"`
	ValidRange coreinterp.PrbValidRange `json:"validRange"`
	MachNumber float64                  `json:"machNumber"`
}

// SevenHoleImportResult 七孔导入响应（PRB / CSV 同形状，对应 importSevenHolePrb / importSevenHoleCalibrationCsv）。
type SevenHoleImportResult struct {
	Files      []SevenHoleFileInfo       `json:"files"`
	ValidRange seveninterp.PrbValidRange `json:"validRange"`
}

// SevenHoleFileInfo 七孔单文件信息（内区 sector=7，扇区 sector=1..6）。
//
// PointCount 来自 metadata 真值——内区固定 169，扇区为 thetaCount×13 动态值
// （4×13=52、7×13=91 等）。Sector 7 表示内区（7.prb），Sector 1..6 表示 6 个
// 外区扇区（按孔号顺序）。
type SevenHoleFileInfo struct {
	FilePath   string `json:"filePath"`
	FileName   string `json:"fileName"`
	Sector     int    `json:"sector"`
	PointCount int    `json:"pointCount"`
	LoadedAtMs int64  `json:"loadedAt"`
}

// ==================== Import 方法 ====================

// ImportPRB 加载单 PRB 文件并注入五孔插值器。
//
// 失败时保留旧插值器（不调用 SetInterpolator），仅返回错误；
// 成功时调用 SetInterpolator（内部清空缓存 + 清除恢复错误）。
func (m *TraversalManager) ImportPRB(filePath string) (*PrbImportResult, error) {
	loader := m.snapshotLoader()
	if loader == nil {
		return nil, errors.New("插值器加载端口未注入（loader is nil），无法导入 PRB")
	}

	interpolator, err := loader.LoadPRB(filePath)
	if err != nil {
		return nil, fmt.Errorf("加载 PRB 文件失败：%w", err)
	}
	// 成功后才替换状态：SetInterpolator 内部清空缓存 + 清除恢复错误
	m.SetInterpolator(interpolator)

	return &PrbImportResult{
		FilePath:   filePath,
		FileName:   filepath.Base(filePath),
		LoadedAtMs: time.Now().UnixMilli(),
		ValidRange: interpolator.GetValidRange(),
	}, nil
}

// ImportCalibrationCSV 加载五孔校准 CSV 文件并注入插值器。
//
// PointCount 通过类型断言 *coreinterp.FiveHoleNewInterpolator 获取——
// coreinterp.Interpolator 接口刻意不暴露 GetPointCount（仅具体类型支持），
// 断言失败时降级为 0（与旧 API 通过具体类型方法读取的行为一致）。
func (m *TraversalManager) ImportCalibrationCSV(filePath string) (*CalibrationCsvImportResult, error) {
	loader := m.snapshotLoader()
	if loader == nil {
		return nil, errors.New("插值器加载端口未注入（loader is nil），无法导入校准 CSV")
	}

	interpolator, err := loader.LoadFiveHoleCSV(filePath)
	if err != nil {
		return nil, fmt.Errorf("加载校准 CSV 文件失败：%w", err)
	}
	m.SetInterpolator(interpolator)

	// 类型断言取 pointCount：接口不含 GetPointCount，仅 *FiveHoleNewInterpolator 支持。
	// 若未来新增其他五孔插值器实现，应在此扩展断言或推动接口补齐方法。
	pointCount := 0
	if fh, ok := interpolator.(*coreinterp.FiveHoleNewInterpolator); ok {
		pointCount = fh.GetPointCount()
	}

	return &CalibrationCsvImportResult{
		FilePath:   filePath,
		FileName:   filepath.Base(filePath),
		LoadedAtMs: time.Now().UnixMilli(),
		ValidRange: interpolator.GetValidRange(),
		PointCount: pointCount,
	}, nil
}

// ImportMultiPRB 加载多 PRB 文件集，按 Mach 数构建插值器。
//
// mode 为空字符串时不调用 SetInterpolationMode（由 loader 内部判断）；
// 非空时透传给 loader，loader 负责调用 SetInterpolationMode。
// 响应 Files / MachNumbers / Warnings 来自 loader 返回的 metadata。
func (m *TraversalManager) ImportMultiPRB(filePaths []string, machNumbers []float64, mode string) (*MultiPrbImportResult, error) {
	loader := m.snapshotLoader()
	if loader == nil {
		return nil, errors.New("插值器加载端口未注入（loader is nil），无法导入多 PRB")
	}

	// mode 字符串 → coreinterp.MultiPrbInterpolationMode 透传给 loader。
	// usecase 允许 import coreinterp（共享算法包，非 adapter）。
	interpolator, metadata, err := loader.LoadMultiPRB(filePaths, machNumbers, coreinterp.MultiPrbInterpolationMode(mode))
	if err != nil {
		return nil, fmt.Errorf("加载多 PRB 文件失败：%w", err)
	}
	m.SetInterpolator(interpolator)

	// metadata 由 loader 填充（LoadedAtMs 已在 loader 内 time.Now().UnixMilli()）；
	// 此处仅做 ports.PrbFileMetadata → MultiPrbFileInfo 的字段映射。
	files := make([]MultiPrbFileInfo, 0, len(metadata.Files))
	for _, f := range metadata.Files {
		files = append(files, MultiPrbFileInfo{
			FilePath:   f.FilePath,
			FileName:   f.FileName,
			LoadedAtMs: f.LoadedAtMs,
			ValidRange: f.ValidRange,
			MachNumber: f.MachNumber,
		})
	}

	return &MultiPrbImportResult{
		Files:       files,
		MachNumbers: metadata.MachNumbers,
		Warnings:    metadata.Warnings,
	}, nil
}

// ImportSevenHolePRB 加载七孔 .prb 文件集（1 内区 + 6 扇区）并注入七孔插值器。
//
// 校验规则（spec §5.6）：innerPath 非空、outerPaths 恰为 6 份；
// 校验失败返回错误且不调用 loader。pointCount 使用兼容约定值 169/52。
func (m *TraversalManager) ImportSevenHolePRB(innerPath string, outerPaths []string) (*SevenHoleImportResult, error) {
	if err := validateSevenHolePaths(innerPath, outerPaths, "PRB"); err != nil {
		return nil, err
	}

	loader := m.snapshotLoader()
	if loader == nil {
		return nil, errors.New("插值器加载端口未注入（loader is nil），无法导入七孔 PRB")
	}

	var outer [6]string
	copy(outer[:], outerPaths)
	interpolator, metadata, err := loader.LoadSevenHolePRB(innerPath, outer)
	if err != nil {
		return nil, fmt.Errorf("加载七孔PRB文件集失败：%w", err)
	}
	m.SetSevenHoleInterpolator(interpolator)

	return m.buildSevenHoleImportResult(innerPath, outerPaths, metadata), nil
}

// ImportSevenHoleCalibrationCSV 加载七孔校准 CSV 文件集并注入七孔插值器。
// 校验与响应形状与 ImportSevenHolePRB 一致；区别仅在底层 loader 调用。
func (m *TraversalManager) ImportSevenHoleCalibrationCSV(innerPath string, outerPaths []string) (*SevenHoleImportResult, error) {
	if err := validateSevenHolePaths(innerPath, outerPaths, "校准 CSV"); err != nil {
		return nil, err
	}

	loader := m.snapshotLoader()
	if loader == nil {
		return nil, errors.New("插值器加载端口未注入（loader is nil），无法导入七孔校准 CSV")
	}

	var outer [6]string
	copy(outer[:], outerPaths)
	interpolator, metadata, err := loader.LoadSevenHoleCalibrationCSV(innerPath, outer)
	if err != nil {
		return nil, fmt.Errorf("加载七孔校准CSV文件集失败：%w", err)
	}
	m.SetSevenHoleInterpolator(interpolator)

	return m.buildSevenHoleImportResult(innerPath, outerPaths, metadata), nil
}

// ==================== 内部工具 ====================

// snapshotLoader 在读锁下取 interpLoader 快照，避免 Import 方法内长时间持锁。
// 返回 nil 表示未注入 loader，调用方需返回明确错误而非 panic。
func (m *TraversalManager) snapshotLoader() ports.InterpolatorLoader {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.interpLoader
}

// validateSevenHolePaths 七孔导入入参校验：innerPath 非空、outerPaths 恰为 6 份。
// kind 仅用于错误消息区分（"PRB" / "校准 CSV"）。
func validateSevenHolePaths(innerPath string, outerPaths []string, kind string) error {
	if innerPath == "" {
		return errors.New("innerFilePath 不能为空（七孔内区" + kind + "）")
	}
	if len(outerPaths) != 6 {
		return fmt.Errorf("outerFilePaths 必须恰为 6 份扇区%s（按孔号 1..6 顺序），实际 %d 份", kind, len(outerPaths))
	}
	return nil
}

// buildSevenHoleImportResult 构造七孔导入响应（PRB / CSV 共用）。
//
// 内区 sector=7 / pointCount=metadata.InnerPointCount（loader 真值，固定 169），
// 扇区 sector=1..6 / pointCount=metadata.OuterPointCounts[i]（loader 真值，动态
// thetaCount×13，如 4×13=52、7×13=91）。LoadedAtMs 与 ValidRange 同样来自 metadata。
func (m *TraversalManager) buildSevenHoleImportResult(
	innerPath string,
	outerPaths []string,
	metadata *ports.SevenHoleLoadMetadata,
) *SevenHoleImportResult {
	files := make([]SevenHoleFileInfo, 0, 7)
	files = append(files, SevenHoleFileInfo{
		FilePath:   innerPath,
		FileName:   filepath.Base(innerPath),
		Sector:     7,
		PointCount: metadata.InnerPointCount,
		LoadedAtMs: metadata.LoadedAtMs,
	})
	for i, p := range outerPaths {
		files = append(files, SevenHoleFileInfo{
			FilePath:   p,
			FileName:   filepath.Base(p),
			Sector:     i + 1,
			PointCount: metadata.OuterPointCounts[i],
			LoadedAtMs: metadata.LoadedAtMs,
		})
	}
	return &SevenHoleImportResult{
		Files:      files,
		ValidRange: metadata.ValidRange,
	}
}
