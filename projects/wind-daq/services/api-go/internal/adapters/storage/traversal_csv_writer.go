// Package storage 提供 CSV 文件持久化的字节 I/O 实现。
//
// 本文件实现遍历测试 CSV 写入器，对应 ports.TraversalPointSink 接口。
// 与 Cursor DAQ TraversalCsvWriter.ts 对齐：
//   - 首行 UTF-8 BOM，避免 Excel/中文 Win 端打开乱码
//   - 文件已存在时自动追加 -2/-3 后缀，避免覆盖之前的实验数据
//   - SaveOptions.CustomFields 动态生成列
package storage

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"sync"
	"time"

	"wind-daq/services/api-go/internal/core/traversal"
	"wind-daq/services/api-go/internal/ports"
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
	header           []string
	rows             int
	commitSeq        uint64
	headerHash       string
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

func (w *TraversalCsvWriter) Open(ctx context.Context, session ports.TraversalOutputSession) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.file != nil {
		return errors.New("遍历 CSV 会话已打开")
	}
	if session.Path == "" {
		return errors.New("遍历 CSV 路径不能为空")
	}
	w.options = traversal.SaveOptions{SavePointId: true, SaveTimestamp: true, SaveRawPressure: true, SaveCalculatedResult: true}
	w.header = w.buildHeader()
	w.headerHash = hashCSVRecord(w.header)
	if session.HeaderHash != "" && session.HeaderHash != w.headerHash {
		return errors.New("遍历 CSV 表头摘要不匹配")
	}
	if session.Mode == ports.TraversalOutputResume {
		return w.openResumeLocked(session)
	}
	return w.openCreateLocked(session.Path)
}

func (w *TraversalCsvWriter) Append(ctx context.Context, result traversal.PointResult) (ports.TraversalRowSummary, error) {
	if err := ctx.Err(); err != nil {
		return ports.TraversalRowSummary{}, err
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.writer == nil || w.file == nil {
		return ports.TraversalRowSummary{}, errors.New("遍历 CSV 写入器未初始化")
	}
	row := w.buildRow(result)
	if err := w.writer.Write(row); err != nil {
		return ports.TraversalRowSummary{}, fmt.Errorf("写入遍历 CSV 行失败: %w", err)
	}
	w.writer.Flush()
	if err := w.writer.Error(); err != nil {
		return ports.TraversalRowSummary{}, fmt.Errorf("刷新遍历 CSV 行失败: %w", err)
	}
	w.rows++
	w.commitSeq = result.CommitSeq
	return ports.TraversalRowSummary{CommitSeq: result.CommitSeq, RowHash: hashCSVRecord(row)}, nil
}

func (w *TraversalCsvWriter) Sync(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.writer == nil || w.file == nil {
		return errors.New("遍历 CSV 写入器未初始化")
	}
	if err := w.flushLocked("刷新遍历 CSV 文件失败"); err != nil {
		return err
	}
	if err := w.file.Sync(); err != nil {
		return fmt.Errorf("同步遍历 CSV 文件失败: %w", err)
	}
	return nil
}

func (w *TraversalCsvWriter) Inspect(ctx context.Context) (ports.TraversalOutputState, error) {
	if err := ctx.Err(); err != nil {
		return ports.TraversalOutputState{}, err
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.file == nil {
		return ports.TraversalOutputState{}, errors.New("遍历 CSV 写入器未初始化")
	}
	return ports.TraversalOutputState{Path: w.path, HeaderHash: w.headerHash, Rows: w.rows, CommitSeq: w.commitSeq, TailValid: true}, nil
}

func (w *TraversalCsvWriter) TruncateAfter(ctx context.Context, commitSeq uint64) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.truncateAfterLocked(commitSeq)
}

func (w *TraversalCsvWriter) Close(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return w.FinalizeTraversal()
}

func (w *TraversalCsvWriter) openCreateLocked(path string) error {
	if dir := filepath.Dir(path); dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("创建遍历输出目录失败: %w", err)
		}
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0o644)
	if err != nil {
		return fmt.Errorf("创建遍历 CSV 文件失败: %w", err)
	}
	w.file, w.path, w.writer = file, path, csv.NewWriter(file)
	if _, err := file.Write(utf8BOM); err != nil {
		_ = file.Close()
		return fmt.Errorf("写入 BOM 失败: %w", err)
	}
	if err := w.writer.Write(w.header); err != nil {
		_ = file.Close()
		return fmt.Errorf("写入遍历 CSV 表头失败: %w", err)
	}
	if err := w.flushLocked("刷新遍历 CSV 表头失败"); err != nil {
		_ = file.Close()
		return err
	}
	return nil
}

func (w *TraversalCsvWriter) openResumeLocked(session ports.TraversalOutputSession) error {
	records, err := readCSVRecords(session.Path)
	if err != nil {
		return err
	}
	if len(records) == 0 || !equalCSVRecord(records[0], w.header) {
		return errors.New("遍历 CSV 表头不匹配")
	}
	if uint64(len(records)-1) < session.CommittedSeq {
		return fmt.Errorf("遍历 CSV 行数少于提交水位: rows=%d commitSeq=%d", len(records)-1, session.CommittedSeq)
	}
	file, err := os.OpenFile(session.Path, os.O_RDWR, 0o644)
	if err != nil {
		return fmt.Errorf("打开遍历 CSV 恢复文件失败: %w", err)
	}
	w.file, w.path, w.writer = file, session.Path, csv.NewWriter(file)
	w.rows, w.commitSeq = len(records)-1, uint64(len(records)-1)
	if err := w.truncateAfterLocked(session.CommittedSeq); err != nil {
		_ = file.Close()
		w.file, w.writer = nil, nil
		return err
	}
	return nil
}

// truncateAfterLocked 原子化截断 CSV 文件至 commitSeq 对应的提交水位。
//
// 实现策略（与 file_checkpoint_store.go 的原子替换模式对齐）：
//  1. 先关闭当前打开的文件句柄（Windows 上替换文件需要独占）
//  2. 将保留的 BOM + records[:commitSeq+1] 写入同目录临时文件
//  3. Sync + Close 临时文件，确保数据落盘
//  4. 用 MoveFileEx (Windows) / os.Rename (Unix) 原子替换原文件
//  5. 重新以追加模式打开文件，定位到末尾，重建 csv.Writer
//
// 这样即使在第 3 步前崩溃，原文件仍完整；在第 4 步崩溃则原文件被临时文件覆盖
// （临时文件已 Sync，数据完整）。避免了原实现 Truncate+Seek+Write 在原文件上
// 操作时崩溃导致半写入状态的问题。
func (w *TraversalCsvWriter) truncateAfterLocked(commitSeq uint64) error {
	if w.file == nil {
		return errors.New("遍历 CSV 写入器未初始化")
	}
	records, err := readCSVRecords(w.path)
	if err != nil {
		return err
	}
	if commitSeq > uint64(len(records)-1) {
		return fmt.Errorf("提交水位超出遍历 CSV 行数: rows=%d commitSeq=%d", len(records)-1, commitSeq)
	}

	// 构造保留行：BOM + 表头 + commitSeq 行数据
	var buffer bytes.Buffer
	buffer.Write(utf8BOM)
	writer := csv.NewWriter(&buffer)
	for _, record := range records[:commitSeq+1] {
		if err := writer.Write(record); err != nil {
			return err
		}
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		return err
	}

	// 关闭当前句柄：Windows 上替换文件需要独占；Unix 上保留句柄替换文件不会报错但语义混乱
	if err := w.flushLocked("刷新遍历 CSV 失败"); err != nil {
		return err
	}
	if err := w.file.Close(); err != nil {
		return fmt.Errorf("关闭遍历 CSV 文件失败: %w", err)
	}
	w.file, w.writer = nil, nil

	// 原子替换原文件
	if err := atomicReplaceFile(w.path, buffer.Bytes(), 0o644); err != nil {
		// 原子替换失败后，writer 已无效，调用方应停止使用本 sink
		return fmt.Errorf("原子替换遍历 CSV 失败: %w", err)
	}

	// 重新以追加模式打开文件，定位到末尾
	file, err := os.OpenFile(w.path, os.O_RDWR|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("重新打开遍历 CSV 文件失败: %w", err)
	}
	if _, err := file.Seek(0, io.SeekEnd); err != nil {
		_ = file.Close()
		return fmt.Errorf("定位遍历 CSV 末尾失败: %w", err)
	}
	w.file = file
	w.writer = csv.NewWriter(file)
	w.rows, w.commitSeq = int(commitSeq), commitSeq
	return nil
}

func readCSVRecords(path string) ([][]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取遍历 CSV 失败: %w", err)
	}
	if !bytes.HasPrefix(data, utf8BOM) {
		return nil, errors.New("遍历 CSV 缺少 UTF-8 BOM")
	}
	records, err := csv.NewReader(bytes.NewReader(data[len(utf8BOM):])).ReadAll()
	if err != nil {
		return nil, fmt.Errorf("解析遍历 CSV 失败: %w", err)
	}
	return records, nil
}

func equalCSVRecord(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func hashCSVRecord(record []string) string {
	var buffer bytes.Buffer
	writer := csv.NewWriter(&buffer)
	_ = writer.Write(record)
	writer.Flush()
	sum := sha256.Sum256(buffer.Bytes())
	return hex.EncodeToString(sum[:])
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
