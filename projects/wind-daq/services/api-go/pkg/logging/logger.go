// Package logging 提供 wind-daq 项目统一的结构化日志基础设施。
//
// 设计目标：
//  1. 同时输出到 stderr（开发态）+ 按天轮转文件（生产态留痕）+ 内存 ring buffer（供前端 SSE 拉取）。
//  2. 与标准库 log/slog 完全兼容：业务侧只需 slog.Info(...) / slog.Warn(...) / slog.Error(...)。
//  3. 把旧式 log.Printf / log.Println 通过 log.SetOutput 重定向到 slog 统一管道，
//     避免历史代码改动量爆炸。
//  4. 提供 WithComponent 辅助，规范化 component/session_id/device_id 等关联字段。
//  5. 支持按 category 维度开关 ring buffer 写入（categoryFilter），stderr 和文件日志不受影响。
//
// 不引入第三方依赖（zap/zerolog/lumberjack 等），仅使用标准库；
// 文件按天轮转、按天保留 N 份的逻辑在 dailyRotatingWriter 中实现。
package logging

import (
	"context"
	"fmt"
	"io"
	"log"
	"shared.local/device-sdk/go/pkg/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Options 描述 logger 初始化参数。
type Options struct {
	// LogDir 日志文件目录，例如 "data/logs"。为空则不写文件。
	LogDir string
	// FileBaseName 日志文件基础名，最终文件形如 {FileBaseName}-YYYYMMDD.log；为空时取 "wind-daq"。
	FileBaseName string
	// RetentionDays 历史文件保留天数（按文件 mtime 计算），<=0 表示不清理。
	RetentionDays int
	// Level 默认日志级别。
	Level slog.Level
	// RingCapacity 内存 ring buffer 最大条数（供前端拉取）。<=0 时取 2000。
	RingCapacity int
	// AddSource 是否在日志中附带源码位置（推荐生产关闭）。
	AddSource bool
	// WriteStderr 是否同时输出到 stderr。
	WriteStderr bool
}

// Default 返回带常用默认值的 Options。
func Default(logDir string) Options {
	return Options{
		LogDir:        logDir,
		FileBaseName:  "wind-daq",
		RetentionDays: 7,
		Level:         slog.LevelInfo,
		RingCapacity:  2000,
		AddSource:     false,
		WriteStderr:   true,
	}
}

// categoryFilter 控制哪些日志分类的条目被写入 ring buffer。
// stderr 和文件日志不受影响，始终全量输出。
// 默认所有 category 均启用，可通过 SetCategoryEnabled 按需关闭。
type categoryFilter struct {
	mu      sync.RWMutex
	enabled map[string]bool // category → 是否启用，未在 map 中的默认为 true
}

// isEnabled 检查指定 category 是否启用。nil receiver 表示无过滤器，全部放行。
func (f *categoryFilter) isEnabled(category string) bool {
	if f == nil {
		return true
	}
	f.mu.RLock()
	enabled, ok := f.enabled[category]
	f.mu.RUnlock()
	if !ok {
		return true // 未显式设置，默认启用
	}
	return enabled
}

// setEnabled 设置指定 category 的启用状态。
func (f *categoryFilter) setEnabled(category string, enabled bool) {
	if f == nil {
		return
	}
	f.mu.Lock()
	f.enabled[category] = enabled
	f.mu.Unlock()
}

// snapshot 返回当前所有 category 的启用状态快照。
func (f *categoryFilter) snapshot() map[string]bool {
	if f == nil {
		return nil
	}
	f.mu.RLock()
	defer f.mu.RUnlock()
	result := make(map[string]bool, len(f.enabled))
	for k, v := range f.enabled {
		result[k] = v
	}
	return result
}

// Manager 持有全局 logger 资源，便于 Shutdown 时统一释放。
type Manager struct {
	ring      *RingBuffer
	fileSink  *dailyRotatingWriter
	level     *slog.LevelVar
	catFilter *categoryFilter // 日志分类过滤器，控制 ring buffer 写入
	prevSlog  *slog.Logger
	prevWrite io.Writer // 旧的 log.Default() Output，便于回滚
	prevFlags int
	closed    atomic.Bool
}

// 全局单例：业务侧通常不需要直接拿，slog 默认 logger 已经被 Init 设置成本包 handler。
var (
	globalMu      sync.Mutex
	globalManager *Manager
)

// Get 返回当前的全局 Manager（可能为 nil，调用方需自行判空）。
func Get() *Manager {
	globalMu.Lock()
	defer globalMu.Unlock()
	return globalManager
}

// Init 初始化全局 logger。
//
// 注意事项：
//   - 必须在程序入口尽早调用，重复调用会先关闭上一个 Manager。
//   - 调用后业务代码通过 slog.Info/slog.Warn/slog.Error 即可命中本管道。
//   - log.Printf 等旧式 API 也会被重定向到 slog（component=stdlog，level=info）。
func Init(opts Options) (*Manager, error) {
	if opts.RingCapacity <= 0 {
		opts.RingCapacity = 2000
	}
	if opts.FileBaseName == "" {
		opts.FileBaseName = "wind-daq"
	}

	levelVar := new(slog.LevelVar)
	levelVar.Set(opts.Level)

	ring := NewRingBuffer(opts.RingCapacity)

	// 创建 category 过滤器：默认全启用，共享给 RingHandler 和 Manager
	catFilter := &categoryFilter{enabled: make(map[string]bool)}

	// 构造 sinks：ring（带 category 过滤）/ stderr / file 三路并发
	// RingHandler 在写入前检查 catFilter，stderr 和 file 不受影响
	sinks := []slog.Handler{NewRingHandler(ring, levelVar, catFilter)}
	if opts.WriteStderr {
		sinks = append(sinks, slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
			Level:     levelVar,
			AddSource: opts.AddSource,
		}))
	}

	var fileSink *dailyRotatingWriter
	if strings.TrimSpace(opts.LogDir) != "" {
		var err error
		fileSink, err = newDailyRotatingWriter(opts.LogDir, opts.FileBaseName, opts.RetentionDays)
		if err != nil {
			return nil, fmt.Errorf("logging: open log dir %q failed: %w", opts.LogDir, err)
		}
		sinks = append(sinks, slog.NewTextHandler(fileSink, &slog.HandlerOptions{
			Level:     levelVar,
			AddSource: opts.AddSource,
		}))
	}

	fanout := newFanoutHandler(sinks...)
	logger := slog.New(fanout)

	mgr := &Manager{
		ring:      ring,
		fileSink:  fileSink,
		level:     levelVar,
		catFilter: catFilter,
	}

	// 全局状态切换顺序非常关键：
	//   1. 先把 *当前* slog.Default / log.Output 作为新 manager 的回滚目标快照下来；
	//   2. 然后才 Close 旧 manager（旧 manager.Close 会把全局状态回滚到它启动时的快照）；
	//   3. 最后把新的 default 装上去。
	// 如果颠倒 1 和 2，新 manager 的 prevSlog 会变成旧 manager 已经回滚过的"上上次 default"，
	// 导致连续重复 Init 后回滚链错位。
	globalMu.Lock()
	mgr.prevSlog = slog.Default()
	mgr.prevWrite = log.Writer()
	mgr.prevFlags = log.Flags()
	if globalManager != nil {
		_ = globalManager.Close()
	}
	globalManager = mgr
	globalMu.Unlock()

	slog.SetDefault(logger)

	// 旧式 log.Printf / log.Println / log.Fatal 的输出会被重定向到 slog，
	// 这样既能复用现有调用点，又能让全部日志统一进入 ring/file/stderr。
	log.SetFlags(0)
	log.SetOutput(&stdlogBridge{base: logger})

	return mgr, nil
}

// Ring 暴露 ring buffer，供 SSE handler 注册订阅者。
func (m *Manager) Ring() *RingBuffer {
	if m == nil {
		return nil
	}
	return m.ring
}

// SetLevel 在运行时调整日志级别。
func (m *Manager) SetLevel(l slog.Level) {
	if m == nil || m.level == nil {
		return
	}
	m.level.Set(l)
}

// SetCategoryEnabled 控制指定 category 的日志是否写入 ring buffer（不影响 stderr 和文件）。
// 默认所有 category 均为启用状态。
func (m *Manager) SetCategoryEnabled(category string, enabled bool) {
	if m == nil || m.catFilter == nil {
		return
	}
	m.catFilter.setEnabled(category, enabled)
}

// GetCategoryStates 返回当前所有已显式设置的 category 开关状态快照。
// 未在返回结果中的 category 默认启用。
func (m *Manager) GetCategoryStates() map[string]bool {
	if m == nil || m.catFilter == nil {
		return nil
	}
	return m.catFilter.snapshot()
}

// Close 释放文件句柄并回滚 slog/log 全局状态。
func (m *Manager) Close() error {
	if m == nil {
		return nil
	}
	if !m.closed.CompareAndSwap(false, true) {
		return nil
	}
	if m.prevSlog != nil {
		slog.SetDefault(m.prevSlog)
	}
	if m.prevWrite != nil {
		log.SetOutput(m.prevWrite)
		log.SetFlags(m.prevFlags)
	}
	var err error
	if m.fileSink != nil {
		err = m.fileSink.Close()
	}
	return err
}

// WithComponent 返回一个带 component 字段的 slog.Logger。
// 业务代码统一通过 component 字段标识来源（traversal/storage/motion/...）。
func WithComponent(component string) *slog.Logger {
	return slog.Default().With(slog.String("component", component))
}

// WithContext 在已有 logger 上追加 context 字段（device_id / session_id / task_id 等）。
func WithContext(base *slog.Logger, kv ...any) *slog.Logger {
	if base == nil {
		base = slog.Default()
	}
	return base.With(kv...)
}

// stdlogBridge 把 log.Printf 等 std log 调用转成 slog。
// 由于 std log 总是按行写入，这里默认按 INFO 级别提交，并附带 component=stdlog。
// 为了不让历史代码里手写的 "ERROR: ..." / "WARN: ..." 等错误信息一律被降为 INFO，
// 这里启发式识别消息前缀，命中后提升到对应级别。
type stdlogBridge struct {
	base *slog.Logger
}

// 级别前缀启发式表：大小写不敏感匹配，命中后剥掉前缀作为最终 message。
var stdlogLevelPrefixes = []struct {
	prefix string
	level  slog.Level
}{
	{"ERROR:", slog.LevelError},
	{"ERR:", slog.LevelError},
	{"[ERROR]", slog.LevelError},
	{"FATAL:", slog.LevelError},
	{"[FATAL]", slog.LevelError},
	{"WARN:", slog.LevelWarn},
	{"WARNING:", slog.LevelWarn},
	{"[WARN]", slog.LevelWarn},
	{"[WARNING]", slog.LevelWarn},
	{"DEBUG:", slog.LevelDebug},
	{"[DEBUG]", slog.LevelDebug},
}

func classifyStdlogLevel(msg string) (slog.Level, string) {
	trimmed := strings.TrimLeft(msg, " \t")
	upper := strings.ToUpper(trimmed)
	for _, p := range stdlogLevelPrefixes {
		if strings.HasPrefix(upper, p.prefix) {
			rest := strings.TrimSpace(trimmed[len(p.prefix):])
			return p.level, rest
		}
	}
	return slog.LevelInfo, msg
}

func (b *stdlogBridge) Write(p []byte) (int, error) {
	// std log 默认在末尾追加换行，去掉它避免日志条目出现空行
	msg := strings.TrimRight(string(p), "\r\n")
	if msg == "" {
		return len(p), nil
	}
	level, payload := classifyStdlogLevel(msg)
	b.base.LogAttrs(context.Background(), level, payload,
		slog.String("component", "stdlog"),
	)
	return len(p), nil
}

// ---- fanout handler ----

type fanoutHandler struct {
	sinks []slog.Handler
}

func newFanoutHandler(sinks ...slog.Handler) *fanoutHandler {
	return &fanoutHandler{sinks: sinks}
}

func (h *fanoutHandler) Enabled(ctx context.Context, level slog.Level) bool {
	for _, s := range h.sinks {
		if s.Enabled(ctx, level) {
			return true
		}
	}
	return false
}

func (h *fanoutHandler) Handle(ctx context.Context, r slog.Record) error {
	var firstErr error
	for _, s := range h.sinks {
		if !s.Enabled(ctx, r.Level) {
			continue
		}
		// 每个 sink 单独处理一份拷贝，避免某个 sink 修改 attrs 影响后续
		if err := s.Handle(ctx, r.Clone()); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func (h *fanoutHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	clones := make([]slog.Handler, len(h.sinks))
	for i, s := range h.sinks {
		clones[i] = s.WithAttrs(attrs)
	}
	return &fanoutHandler{sinks: clones}
}

func (h *fanoutHandler) WithGroup(name string) slog.Handler {
	clones := make([]slog.Handler, len(h.sinks))
	for i, s := range h.sinks {
		clones[i] = s.WithGroup(name)
	}
	return &fanoutHandler{sinks: clones}
}

// ---- daily rotating writer ----

// dailyRotatingWriter 按天切换文件，文件命名 wind-daq-YYYYMMDD.log。
// 启动时根据 RetentionDays 清理过期文件；写入时如果跨天则自动 reopen 新文件。
type dailyRotatingWriter struct {
	mu            sync.Mutex
	dir           string
	baseName      string
	retentionDays int
	currentDate   string // YYYYMMDD
	file          *os.File
}

func newDailyRotatingWriter(dir, baseName string, retentionDays int) (*dailyRotatingWriter, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	w := &dailyRotatingWriter{
		dir:           dir,
		baseName:      baseName,
		retentionDays: retentionDays,
	}
	if err := w.rotateLocked(time.Now()); err != nil {
		return nil, err
	}
	w.cleanup()
	return w, nil
}

func (w *dailyRotatingWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := w.rotateLocked(time.Now()); err != nil {
		return 0, err
	}
	if w.file == nil {
		return len(p), nil
	}
	return w.file.Write(p)
}

func (w *dailyRotatingWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.file == nil {
		return nil
	}
	err := w.file.Close()
	w.file = nil
	return err
}

func (w *dailyRotatingWriter) rotateLocked(now time.Time) error {
	date := now.Format("20060102")
	if date == w.currentDate && w.file != nil {
		return nil
	}
	if w.file != nil {
		_ = w.file.Close()
		w.file = nil
	}
	path := filepath.Join(w.dir, fmt.Sprintf("%s-%s.log", w.baseName, date))
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	w.file = f
	w.currentDate = date
	return nil
}

// cleanup 异步清理过期日志，避免阻塞启动。
//
// 判定方式：以文件名中的 YYYYMMDD 日期为准（而非 mtime），
// 避免跨午夜重命名 / 文件系统时间漂移等情况下误删刚创建的当日日志。
// 文件名不符合 {baseName}-YYYYMMDD.log 格式时跳过，不做任何处理。
// 清理失败仅静默吞掉，不影响主流程。
func (w *dailyRotatingWriter) cleanup() {
	if w.retentionDays <= 0 {
		return
	}
	go func(dir, base string, retain int) {
		entries, err := os.ReadDir(dir)
		if err != nil {
			return
		}
		cutoff := time.Now().AddDate(0, 0, -retain)
		// 把 cutoff 截断到当天 0 点，避免"刚跨过 retain 天分界几秒"就被误删
		cutoffDay := time.Date(cutoff.Year(), cutoff.Month(), cutoff.Day(), 0, 0, 0, 0, cutoff.Location())
		prefix := base + "-"
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			name := e.Name()
			if !strings.HasPrefix(name, prefix) || !strings.HasSuffix(name, ".log") {
				continue
			}
			// 提取文件名中的 YYYYMMDD 段：{base}-YYYYMMDD.log
			datePart := strings.TrimSuffix(strings.TrimPrefix(name, prefix), ".log")
			fileDay, parseErr := time.ParseInLocation("20060102", datePart, time.Local)
			if parseErr != nil {
				// 非标准命名（例如外部脚本拷贝的备份）保守起见不动它
				continue
			}
			if fileDay.Before(cutoffDay) {
				_ = os.Remove(filepath.Join(dir, name))
			}
		}
	}(w.dir, w.baseName, w.retentionDays)
}
