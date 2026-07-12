// Package storage 提供 CSV 文件持久化的字节 I/O 实现。
//
// 本文件实现遍历测试 CSV 写入器，对应 ports.TraversalPointSink 接口。
// 与 Cursor DAQ TraversalCsvWriter.ts 对齐：
//   - 首行 UTF-8 BOM，避免 Excel/中文 Win 端打开乱码
//   - 文件已存在时自动追加 -2/-3 后缀，避免覆盖之前的实验数据
//   - SaveOptions.CustomFields 动态生成列
package storage

import (
	"encoding/csv"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"sync"
	"time"

	"wind-daq/services/api-go/internal/core/traversal"
)

// utf8BOM Excel 等中文环境需要 BOM 才能正确识别 UTF-8 编码
var utf8BOM = []byte{0xEF, 0xBB, 0xBF}

// TraversalCsvWriter 遍历测试 CSV 写入器
// 每完成一个测试点追加写入一行；Stop/Complete 时关闭文件
type TraversalCsvWriter struct {
	mu      sync.Mutex
	file    *os.File
	writer  *csv.Writer
	path    string
	options traversal.SaveOptions
	// labels 通道索引→标签（如 0→P1, 16→Patm），用于稳定输出列顺序
	labels []labelEntry
	// customFieldNames 已排序的自定义字段名，保证列顺序稳定
	customFieldNames []string
}

// labelEntry 单个标签列的元信息
type labelEntry struct {
	Channel int
	Label   string
}

// NewTraversalCsvWriter 创建遍历 CSV 写入器（构造时不打开文件）
func NewTraversalCsvWriter() *TraversalCsvWriter {
	return &TraversalCsvWriter{}
}

// resolveOutputPath 根据 SavePath/SaveFileName/TaskID 计算最终 CSV 文件路径
//   - SavePath 为目录或为空 → 拼接 saveFileName / 默认名 + .csv
//   - SavePath 已经带 .csv → 直接使用
func resolveOutputPath(cfg traversal.Config) string {
	savePath := cfg.SavePath
	saveName := cfg.SaveFileName
	if saveName == "" {
		saveName = fmt.Sprintf("traversal_%s", cfg.TaskID)
	}
	if savePath == "" {
		// 空路径回退到当前工作目录
		if filepath.Ext(saveName) != ".csv" {
			saveName += ".csv"
		}
		return saveName
	}
	if filepath.Ext(savePath) == ".csv" {
		return savePath
	}
	if filepath.Ext(saveName) != ".csv" {
		saveName += ".csv"
	}
	return filepath.Join(savePath, saveName)
}

// InitializeTraversal 创建文件并写入表头
func (w *TraversalCsvWriter) InitializeTraversal(cfg traversal.Config) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if cfg.SavePath == "" && cfg.SaveFileName == "" {
		// 用户未指定输出文件 → 不写盘（视为禁用 sink）
		w.file = nil
		w.writer = nil
		w.path = ""
		return nil
	}

	// 默认所有列都开启（兼容前端未传 saveOptions 的情况）
	if cfg.SaveOptions != nil {
		w.options = *cfg.SaveOptions
	} else {
		w.options = traversal.SaveOptions{
			SavePointId:          true,
			SaveTimestamp:        true,
			SaveRawPressure:      true,
			SaveCalculatedResult: true,
		}
	}

	// 通道→标签列表（稳定排序）：先按已知标签优先级，再按通道索引兜底
	w.labels = buildLabelEntries(cfg.Channels, cfg.ChannelLabels)
	// 自定义字段：按字典序固定列顺序（避免每次 map 遍历顺序变动）
	w.customFieldNames = enabledCustomFieldNames(w.options.CustomFields)

	basePath := resolveOutputPath(cfg)
	if dir := filepath.Dir(basePath); dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("创建遍历输出目录失败: %w", err)
		}
	}

	// 与 Cursor DAQ 行为一致：使用 O_EXCL 原子创建，冲突时追加 -2/-3/...
	file, finalPath, err := createUniqueFile(basePath)
	if err != nil {
		return fmt.Errorf("创建遍历 CSV 文件失败: %w", err)
	}
	w.file = file
	w.path = finalPath
	w.writer = csv.NewWriter(file)

	// 首行写 UTF-8 BOM，Excel 等环境才能识别 UTF-8 中文
	if _, err := file.Write(utf8BOM); err != nil {
		return fmt.Errorf("写入 BOM 失败: %w", err)
	}

	header := w.buildHeader()
	if err := w.writer.Write(header); err != nil {
		return fmt.Errorf("写入遍历 CSV 表头失败: %w", err)
	}
	return w.flushLocked("刷新遍历 CSV 表头失败")
}

// createUniqueFile 在 basePath 已存在时，在文件名 stem 后追加 -2/-3/... 自增直到找到空位
// 例如 traversal_001.csv 已存在 → traversal_001-2.csv → -3 ...
func createUniqueFile(basePath string) (*os.File, string, error) {
	const maxAttempts = 1000
	ext := filepath.Ext(basePath)
	stem := basePath[:len(basePath)-len(ext)]
	candidate := basePath
	// 第 1 次尝试原始名；2..N 次追加 -2/-3/...
	for suffix := 1; suffix <= maxAttempts; suffix++ {
		f, err := os.OpenFile(candidate, os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0o644)
		if err == nil {
			return f, candidate, nil
		}
		if !errors.Is(err, os.ErrExist) {
			return nil, "", err
		}
		// 文件已存在 → 下一个候选名
		candidate = fmt.Sprintf("%s-%d%s", stem, suffix+1, ext)
	}
	return nil, "", fmt.Errorf("超过 %d 次尝试仍无法生成唯一文件名", maxAttempts)
}

// enabledCustomFieldNames 提取启用为 true 的自定义字段名并按字典序排序
func enabledCustomFieldNames(custom map[string]bool) []string {
	if len(custom) == 0 {
		return nil
	}
	names := make([]string, 0, len(custom))
	for name, enabled := range custom {
		if enabled {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names
}

// WriteTraversalPoint 追加一个数据点到 CSV
func (w *TraversalCsvWriter) WriteTraversalPoint(p traversal.PointResult) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.writer == nil {
		// sink 在 InitializeTraversal 中被禁用（未配置输出路径），静默丢弃
		return nil
	}
	row := w.buildRow(p)
	if err := w.writer.Write(row); err != nil {
		return fmt.Errorf("写入遍历 CSV 行失败: %w", err)
	}
	return w.flushLocked("刷新遍历 CSV 行失败")
}

// FinalizeTraversal 关闭文件
func (w *TraversalCsvWriter) FinalizeTraversal() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	var flushErr error
	if w.writer != nil {
		flushErr = w.flushLocked("刷新遍历 CSV 文件失败")
		w.writer = nil
	}
	if w.file != nil {
		err := w.file.Close()
		w.file = nil
		if flushErr != nil {
			if err != nil {
				return fmt.Errorf("%v; 关闭遍历 CSV 文件失败: %w", flushErr, err)
			}
			return flushErr
		}
		return err
	}
	return flushErr
}

// OutputPath 返回 InitializeTraversal 实际创建的 CSV 文件路径。
func (w *TraversalCsvWriter) OutputPath() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.path
}

func (w *TraversalCsvWriter) flushLocked(message string) error {
	w.writer.Flush()
	if err := w.writer.Error(); err != nil {
		return fmt.Errorf("%s: %w", message, err)
	}
	return nil
}

// buildHeader 构建表头（按 SaveOptions 控制列）
func (w *TraversalCsvWriter) buildHeader() []string {
	cols := []string{}
	if w.options.SavePointId {
		cols = append(cols, "PointId")
	}
	if w.options.SaveTimestamp {
		cols = append(cols, "Timestamp")
	}
	cols = append(cols, "X", "Y", "Z", "U")
	if w.options.SaveRawPressure {
		for _, e := range w.labels {
			cols = append(cols, e.Label)
		}
	}
	if w.options.SaveCalculatedResult {
		// 计算结果列：插值器输出的关键空气动力量 + 采样元数据
		cols = append(cols, "Alpha", "Beta", "Pt", "Ps", "Mach", "SampleCount", "DwellMs")
	}
	// 自定义字段列（按字典序）
	cols = append(cols, w.customFieldNames...)
	return cols
}

// buildRow 构建一行数据
func (w *TraversalCsvWriter) buildRow(p traversal.PointResult) []string {
	row := []string{}
	if w.options.SavePointId {
		row = append(row, strconv.Itoa(p.PointIndex+1))
	}
	if w.options.SaveTimestamp {
		// 截断到秒级：与采集 CSV 时间戳格式对齐，避免展示错误的时间细分。
		ts := time.UnixMilli(p.Timestamp).Format("2006-01-02 15:04:05")
		row = append(row, ts)
	}
	row = append(row, formatFloat(p.Point.X), formatFloat(p.Point.Y), formatFloat(p.Point.Z), formatFloat(p.Point.U))
	if w.options.SaveRawPressure {
		for _, e := range w.labels {
			if v, ok := p.Values[e.Channel]; ok {
				row = append(row, formatFloat(v))
			} else {
				row = append(row, "")
			}
		}
	}
	if w.options.SaveCalculatedResult {
		// 从 PointResult.Calculated 读取插值结果；若上游未填充则写空
		calc := p.Calculated
		if calc != nil && calc.Valid {
			row = append(row,
				formatFloat(calc.Alpha),
				formatFloat(calc.Beta),
				formatFloat(calc.Pt),
				formatFloat(calc.Ps),
				formatFloat(calc.Mach),
			)
		} else {
			row = append(row, "", "", "", "", "")
		}
		row = append(row, strconv.Itoa(p.SampleCount), strconv.Itoa(p.DwellTimeElapsed))
	}
	// 自定义字段：以 PointResult.CustomValues 为准；缺失写空
	for _, name := range w.customFieldNames {
		if p.CustomValues != nil {
			if v, ok := p.CustomValues[name]; ok {
				row = append(row, v)
				continue
			}
		}
		row = append(row, "")
	}
	return row
}

// formatFloat 格式化浮点数为 CSV 单元格字符串。
// NaN 输出空字符串：line/rectangle/sector 模式通过 markAxesNaN 将未配置的轴标记为 NaN，
// CSV 中对应列留空比输出 "NaN" 更易读，且不会干扰 Excel 数值解析。
func formatFloat(v float64) string {
	if math.IsNaN(v) {
		return ""
	}
	return strconv.FormatFloat(v, 'f', 6, 64)
}

// buildLabelEntries 构建通道→标签的稳定排序列表
//   - 已知标签优先级：P1,P2,P3,P4,P5,Patm,Tatm
//   - 其余按通道索引升序追加
func buildLabelEntries(channels []int, labelMap map[int]string) []labelEntry {
	priority := []string{"P1", "P2", "P3", "P4", "P5", "Patm", "Tatm"}
	priorityIdx := make(map[string]int, len(priority))
	for i, l := range priority {
		priorityIdx[l] = i
	}

	entries := make([]labelEntry, 0, len(channels))
	for _, ch := range channels {
		label := ""
		if labelMap != nil {
			label = labelMap[ch]
		}
		if label == "" {
			label = fmt.Sprintf("CH%d", ch)
		}
		entries = append(entries, labelEntry{Channel: ch, Label: label})
	}

	// 按优先级 + 通道索引排序（稳定）
	for i := 0; i < len(entries)-1; i++ {
		for j := i + 1; j < len(entries); j++ {
			ai, aOk := priorityIdx[entries[i].Label]
			bi, bOk := priorityIdx[entries[j].Label]
			swap := false
			switch {
			case aOk && bOk:
				swap = ai > bi
			case aOk && !bOk:
				swap = false
			case !aOk && bOk:
				swap = true
			default:
				swap = entries[i].Channel > entries[j].Channel
			}
			if swap {
				entries[i], entries[j] = entries[j], entries[i]
			}
		}
	}
	return entries
}
