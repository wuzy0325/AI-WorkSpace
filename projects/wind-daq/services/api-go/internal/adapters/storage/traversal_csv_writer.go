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
	"strings"
	"sync"
	"time"

	"wind-daq/services/api-go/internal/core/traversal"
	"wind-daq/services/api-go/internal/ports"
)

// utf8BOM Excel 等中文环境需要 BOM 才能正确识别 UTF-8 编码
var utf8BOM = []byte{0xEF, 0xBB, 0xBF}

// TraversalCsvWriter 遍历测试 CSV 写入器
// 每完成一个测试点追加写入一行；Stop/Complete 时关闭文件
//
// 编译期接口断言哨兵：
//   - 同时实现 ports.TraversalCSVPort（v2 路径，支持 Open/Append/Sync/Truncate/Close）
//   - 兼容实现 ports.TraversalPointSink（旧路径，InitializeTraversal/WriteTraversalPoint/FinalizeTraversal）
//
// 装配根将同一实例注入两个端口，让 usecase 通过 sinkIsCsvPort 检测同实例并跳过重复调用。
type TraversalCsvWriter struct {
	mu      sync.Mutex
	file    *os.File
	writer  *csv.Writer
	path    string
	options traversal.SaveOptions
	// broken 标记 sink 已进入不可恢复的损坏状态（truncateAfterLocked 在 atomicReplaceFile
	// 或重新 OpenFile 失败后置 true）。此时文件可能已被改写但 sink 内部无句柄，
	// 后续 Append/Sync/Inspect/Close 必须拒绝并返回明确错误，避免静默成功掩盖损坏。
	// Open 在新会话开始时重置为 false。
	broken bool
	// labels  通道索引→标签（如 0→P1, 16→Patm），用于稳定输出列顺序
	labels []labelEntry
	// customFieldNames 已排序的自定义字段名，保证列顺序稳定
	customFieldNames []string
	// motionAxes 逻辑方向→物理轴绑定（来自 config.MotionAxes）。
	// buildRow 按此把 Point.X/Y/Z/U 逻辑坐标值映射到对应物理轴列，
	// 未绑定的物理轴列留空。为空（旧配置兼容）时保持原行为：按 Point 字段顺序直接填值。
	motionAxes       []traversal.MotionAxisBinding
	header           []string
	rows             int
	commitSeq        uint64
	headerHash       string
}

var (
	_ ports.TraversalCSVPort    = (*TraversalCsvWriter)(nil)
	_ ports.TraversalPointSink  = (*TraversalCsvWriter)(nil)
)

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
	// 新会话开始：重置 broken 标志（前一会话的损坏状态不带入新会话）
	w.broken = false
	// 应用列配置：v2 Open 路径必须构建与 InitializeTraversal 一致的 labels /
	// customFieldNames / header，否则 v2 主路径表头会缺少通道列、Resume 时
	// HeaderHash 校验也会失败。applyConfigLocked 与 InitializeTraversal 共享逻辑。
	w.applyConfigLocked(session.SaveOptions, session.Channels, session.ChannelLabels, session.MotionAxes)
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
	if w.broken {
		return ports.TraversalRowSummary{}, errors.New("遍历 CSV 写入器已损坏，不可恢复（truncate 失败后 sink 无句柄）")
	}
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
	if w.broken {
		return errors.New("遍历 CSV 写入器已损坏，不可恢复（truncate 失败后 sink 无句柄）")
	}
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
	if w.broken {
		return ports.TraversalOutputState{}, errors.New("遍历 CSV 写入器已损坏，不可恢复（truncate 失败后 sink 无句柄）")
	}
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
	// 以 O_EXCL 创建目标文件；若已存在（同一天、同名重跑），自动追加 -2/-3
	// 生成唯一文件，避免静默覆盖历史数据，也不再以报错拒绝启动。
	file, finalPath, err := openCreateUnique(path)
	if err != nil {
		return err
	}
	w.file, w.path, w.writer = file, finalPath, csv.NewWriter(file)
	if _, err := file.Write(utf8BOM); err != nil {
		_ = file.Close()
		_ = os.Remove(finalPath) // 失败时清理已创建文件，避免残留空文件污染数据目录
		w.file, w.writer = nil, nil
		return fmt.Errorf("写入 BOM 失败: %w", err)
	}
	if err := w.writer.Write(w.header); err != nil {
		_ = file.Close()
		_ = os.Remove(finalPath)
		w.file, w.writer = nil, nil
		return fmt.Errorf("写入遍历 CSV 表头失败: %w", err)
	}
	if err := w.flushLocked("刷新遍历 CSV 表头失败"); err != nil {
		_ = file.Close()
		_ = os.Remove(finalPath)
		w.file, w.writer = nil, nil
		return err
	}
	// 表头落盘：本次提交核心目标是"崩溃恢复与可靠存储"，
	// 若仅 flush（用户态缓冲）不 fsync（OS page cache），崩溃后表头丢失会让
	// 恢复时 readCSVRecords 因缺 BOM 或表头损坏而失败。
	if err := file.Sync(); err != nil {
		_ = file.Close()
		_ = os.Remove(finalPath)
		w.file, w.writer = nil, nil
		return fmt.Errorf("同步遍历 CSV 表头失败: %w", err)
	}
	return nil
}

// openCreateUnique 以 O_EXCL 创建目标文件；若目标已存在（如同一天、同名重跑），
// 自动追加 -2/-3 后缀生成唯一文件，保护历史实验数据不被覆盖。
// 返回已打开的文件句柄与最终使用的（可能带后缀的）路径。
func openCreateUnique(path string) (*os.File, string, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0o644)
	if err == nil {
		return f, path, nil
	}
	if !errors.Is(err, os.ErrExist) {
		return nil, "", fmt.Errorf("创建遍历 CSV 文件失败: %w", err)
	}
	// 目标已存在：自动编号另存，与 legacy InitializeTraversal 行为一致
	return createUniqueFile(path)
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
		// 原子替换失败：文件可能已被改写也可能未变，但 sink 无句柄且无法重新打开，
		// 标记 broken 让后续 Append/Sync/Inspect/Close 拒绝静默成功。
		// 调用方应停止使用本 sink 并触发任务 abort。
		w.broken = true
		return fmt.Errorf("原子替换遍历 CSV 失败: %w", err)
	}

	// 重新以追加模式打开文件，定位到末尾
	file, err := os.OpenFile(w.path, os.O_RDWR|os.O_APPEND, 0o644)
	if err != nil {
		// 重新打开失败：文件已替换为新内容但 sink 无法继续写入，标记 broken
		w.broken = true
		return fmt.Errorf("重新打开遍历 CSV 文件失败: %w", err)
	}
	if _, err := file.Seek(0, io.SeekEnd); err != nil {
		_ = file.Close()
		w.broken = true
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

// resolveOutputPath 委托给 core/traversal.ResolveOutputPath，避免路径拼接逻辑重复。
func resolveOutputPath(cfg traversal.Config) string {
	return traversal.ResolveOutputPath(cfg)
}

// applyConfigLocked 应用列配置（SaveOptions + Channels + ChannelLabels + MotionAxes），
// 设置 w.options / w.labels / w.customFieldNames / w.motionAxes。
//
// Open 与 InitializeTraversal 共享此方法，保证 v2 路径与旧 sink 路径
// 表头构建一致：相同输入 → 相同 labels 顺序 → 相同 HeaderHash。
//
// 调用约束：必须持 w.mu。
func (w *TraversalCsvWriter) applyConfigLocked(opts *traversal.SaveOptions, channels []int, labels map[int]string, motionAxes []traversal.MotionAxisBinding) {
	// 默认所有列都开启（兼容前端未传 saveOptions 的情况）
	if opts != nil {
		w.options = *opts
	} else {
		w.options = traversal.SaveOptions{
			SavePointId:          true,
			SaveTimestamp:        true,
			SaveRawPressure:      true,
			SaveCalculatedResult: true,
		}
	}
	// 通道→标签列表（稳定排序）：先按已知标签优先级，再按通道索引兜底
	w.labels = buildLabelEntries(channels, labels)
	// 自定义字段：按字典序固定列顺序（避免每次 map 遍历顺序变动）
	w.customFieldNames = enabledCustomFieldNames(w.options.CustomFields)
	// 物理轴绑定：拷贝一份避免外部修改影响 writer 内部状态
	w.motionAxes = append([]traversal.MotionAxisBinding(nil), motionAxes...)
}

// InitializeTraversal 创建文件并写入表头
//
// 双重初始化防御（v2 装配下关键）：
// 装配根（bootstrap.go / appcontext/context.go）把同一个 TraversalCsvWriter
// 实例同时注入为旧 TraversalPointSink 与新 TraversalCSVPort。Start 路径会先调用
// csvPort.Open 创建文件 A，再调用 sink.InitializeTraversal；若不防御，createUniqueFile
// 会因 A 已存在生成 -2 后缀的文件 B 并直接覆盖 w.file，导致 fileA 句柄泄漏 +
// 残留垃圾 CSV + 状态机不一致。检测到已打开直接返回错误，让调用方感知并修正装配。
func (w *TraversalCsvWriter) InitializeTraversal(cfg traversal.Config) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	// 双重初始化防御：csvPort.Open 已创建文件时拒绝再次初始化
	if w.file != nil {
		return errors.New("遍历 CSV 会话已打开，拒绝重复初始化")
	}

	if cfg.SavePath == "" && cfg.SaveFileName == "" {
		// 用户未指定输出文件 → 不写盘（视为禁用 sink）
		w.file = nil
		w.writer = nil
		w.path = ""
		return nil
	}

	// 共享列配置应用逻辑（与 Open 路径一致）
	w.applyConfigLocked(cfg.SaveOptions, cfg.Channels, cfg.ChannelLabels, cfg.MotionAxes)

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
		_ = file.Close()
		_ = os.Remove(finalPath)
		w.file, w.writer = nil, nil
		return fmt.Errorf("写入 BOM 失败: %w", err)
	}

	header := w.buildHeader()
	if err := w.writer.Write(header); err != nil {
		_ = file.Close()
		_ = os.Remove(finalPath)
		w.file, w.writer = nil, nil
		return fmt.Errorf("写入遍历 CSV 表头失败: %w", err)
	}
	if err := w.flushLocked("刷新遍历 CSV 表头失败"); err != nil {
		_ = file.Close()
		_ = os.Remove(finalPath)
		w.file, w.writer = nil, nil
		return err
	}
	// 表头 fsync：崩溃后表头不丢失，resume 时 readCSVRecords 才能成功
	if err := file.Sync(); err != nil {
		_ = file.Close()
		_ = os.Remove(finalPath)
		w.file, w.writer = nil, nil
		return fmt.Errorf("同步遍历 CSV 表头失败: %w", err)
	}
	return nil
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
		// 计算结果列：插值器输出的关键空气动力量 + 采样元数据 + 单点起止时间
		// StartedAt/CompletedAt：单点采集的真实起止时间戳（秒级字符串，与 Timestamp 列格式一致），
		// 用户可直接用 CompletedAt - StartedAt 算出单点总耗时，不再依赖"点数×10ms"回填公式
		cols = append(cols, "Alpha", "Beta", "Pt", "Ps", "Mach", "SampleCount", "DwellMs", "StartedAt", "CompletedAt")
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
	// 4 轴列：表头固定 X/Y/Z/U（物理轴名），数据按 motionAxes 把逻辑坐标映射到对应物理轴列。
	// 修复 Bug 2: 旧实现硬编码 formatFloat(p.Point.X/Y/Z/U)，导致用户把逻辑 X 绑到物理 Z 轴时
	// 数据仍写入 X 列，与表头不一致。改为按 motionAxes 重映射，未绑定的物理轴列留空。
	row = append(row, w.buildAxisRowValues(p.Point)...)
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
		// StartedAt/CompletedAt：与 Timestamp 同为秒级字符串。
		// 0 值写空字符串（兼容旧数据或异常路径未赋值的场景），避免显示"1970-01-01 08:00:00"误导用户
		row = append(row, formatUnixMilli(p.StartedAt), formatUnixMilli(p.CompletedAt))
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

// buildAxisRowValues 构建 4 轴列（X/Y/Z/U）的字符串切片。
//
// 输出列固定为物理轴名顺序 ["X","Y","Z","U"]，与 buildHeader 的 cols = "X","Y","Z","U" 对齐。
// 数据填充规则：
//   - motionAxes 为空（旧配置兼容）：保持原行为，按 Point.X/Y/Z/U 字段顺序直接填值，
//     NaN 由 formatFloat 转空字符串。这样不影响未升级 motionAxes 配置的存量任务。
//   - motionAxes 非空：按每个绑定的 Name→Axis 把逻辑坐标值填到对应物理轴列；
//     未出现在任何绑定中的物理轴列输出空字符串（区别于"绑定为 0"）。
//
// 例：motionAxes=[{Name:"X",Axis:"Z"},{Name:"Y",Axis:"U"}]，Point={X:10, Y:5, Z:NaN, U:NaN}
//
//	输出 ["", "", "10.000000", "5.000000"] —— 物理 Z 列承载逻辑 X 方向，
//	物理 U 列承载逻辑 Y 方向，物理 X/Y 列留空。
func (w *TraversalCsvWriter) buildAxisRowValues(p traversal.Point) []string {
	// 旧配置兼容路径：未配置 motionAxes 时按原行为直接输出 Point 字段
	if len(w.motionAxes) == 0 {
		return []string{
			formatFloat(p.X),
			formatFloat(p.Y),
			formatFloat(p.Z),
			formatFloat(p.U),
		}
	}
	// 逻辑方向名 → Point 字段值（用于按 binding.Name 取出逻辑坐标）
	logical := map[string]float64{
		"X": p.X,
		"Y": p.Y,
		"Z": p.Z,
		"U": p.U,
	}
	// 物理轴名 → 已绑定的逻辑坐标值。
	// 用 map 配合 ok 模式区分"未绑定"（输出空字符串）与"绑定为 0"（输出 "0.000000"）。
	bound := make(map[string]float64, len(w.motionAxes))
	for _, b := range w.motionAxes {
		// Axis 字段在 MotionAxisBinding 中是必填（前端约束），
		// Name 字段可能为空（旧数据），跳过无法映射的绑定
		if b.Axis == "" || b.Name == "" {
			continue
		}
		// 大小写防御：用户绕过前端直接构造 JSON 可能传小写 axis/name，
		// map key 大小写敏感，不规范化会导致小写绑定的值静默丢失。
		// 前端始终发大写，此规范化仅作兜底，不影响正常路径。
		axis := strings.ToUpper(b.Axis)
		name := strings.ToUpper(b.Name)
		if v, ok := logical[name]; ok {
			bound[axis] = v
		}
	}
	return []string{
		formatAxisCell(bound, "X"),
		formatAxisCell(bound, "Y"),
		formatAxisCell(bound, "Z"),
		formatAxisCell(bound, "U"),
	}
}

// formatAxisCell 输出物理轴列值：未绑定输出空字符串，已绑定走 formatFloat（NaN 也输出空字符串）。
func formatAxisCell(bound map[string]float64, axis string) string {
	if v, ok := bound[axis]; ok {
		return formatFloat(v)
	}
	return ""
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

// formatUnixMilli 格式化 UnixMilli 时间戳为 CSV 单元格字符串（秒级，与 Timestamp 列一致）。
// 0 值输出空字符串：兼容旧数据或异常路径未赋值场景，避免显示"1970-01-01 08:00:00"误导用户。
func formatUnixMilli(ms int64) string {
	if ms <= 0 {
		return ""
	}
	return time.UnixMilli(ms).Format("2006-01-02 15:04:05")
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
