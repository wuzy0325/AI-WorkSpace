package recording

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"wispa/core"

	"shared.local/device-sdk/go/pkg/slog"
)

const (
	// csvDefaultQueueCapacity 异步队列默认容量。
	// 1kHz × 10 设备 = 1 万 payloads/sec，2s fsync 间隔下偶发 I/O stall 可积压 2000-5000 payloads，
	// 32k 容量可缓冲 6-15 次 fsync 周期的积压。
	csvDefaultQueueCapacity = 32768
	// csvDefaultBufferSize bufio 缓冲区大小（1MB）
	csvDefaultBufferSize = 1 << 20
	// csvDefaultFlushIntervalMs bufio flush 间隔（仅刷到 OS 缓冲，廉价）
	csvDefaultFlushIntervalMs = 100
	// csvDefaultSyncIntervalSec fsync 间隔（真正落盘，昂贵）
	csvDefaultSyncIntervalSec = 2
	// csvDropLogInterval 丢弃日志节流间隔，避免高频丢弃时刷屏
	csvDropLogInterval = 5 * time.Second

	// numChannels 与 P1604 硬件通道数保持一致（16 路压力 + 大气压 + 大温）
	numChannels = 18
	// numPressureChannels 仅压力通道数，用于 CSV 表头中 CH01..CH16
	numPressureChannels = 16
	// defaultPrecision CSV 输出默认小数位数
	defaultPrecision = 4
	// maxPrecision 用户允许配置的最大小数位数
	maxPrecision = 6
	// fileNameSeqRetry 文件名序号冲突时的最大重试次数（同秒内重启且序号撞上的极小概率场景）
	fileNameSeqRetry = 1000
)

// perDeviceWriter 单设备的写入上下文。
// 每个设备独立持有文件、缓冲、统计，设备间互不干扰。
// writer goroutine 单线程访问，无需额外同步（Status 读时通过 statsMu 互斥）。
type perDeviceWriter struct {
	// deviceName 用于日志可读性（实际文件名用 fileSlug 派生）
	deviceName string
	// fileSlug 是文件名中用于标识设备的段（sanitize 后的 deviceName，若与其他设备
	// 冲突则追加 -<deviceID 前 6 位>）。在 getOrCreateWriter 首次为该设备创建
	// 文件时确定，之后本会话内所有滚动文件复用同一 slug。
	fileSlug        string
	file            *os.File
	bw              *bufio.Writer
	fileName        string
	fileSize        int64 // 当前文件累计字节
	fileCount       int64 // 该设备本会话文件数（含当前文件）
	fileStartedAt   time.Time
	fileRecordCount int64 // 该设备当前文件记录数（用于 MaxRecordCount 滚动判断）
}

// CSVRecorder CSV 异步批量录制器（每设备一个 CSV 文件）
//
// 设计要点：
//   - 每设备一个 CSV 文件，按 deviceId 路由（避免多设备数据混杂在同一文件，
//     防止不同设备硬件时间戳交替跳跃）
//   - 文件名格式：<prefix>-<deviceSlug>-YYYYMMDD-HHMMSS-NNN.csv
//     deviceSlug 优先用设备名（sanitize 后），同名冲突时追加 deviceId 前 6 位
//   - Write 通过 select-default 非阻塞投递到 queue channel，绝不阻塞设备 read loop
//   - 单一 writer goroutine 串行消费 channel 写入文件，消除多设备锁争用
//   - flush (100ms) 与 fsync (2s) 分离：flush 廉价（仅刷 OS 缓冲），fsync 昂贵（真正落盘）
//   - strconv.AppendFloat 替代 fmt.Sprintf，避免反射开销，吞吐提升 2-5 倍
//   - 支持 FileRotation（按设备独立评估：达到阈值时切换该设备文件）
//   - 支持 StopConditions（跨设备汇总评估：任一条件满足则整体停止录制）
//   - Start/Stop 用 atomic.Bool CAS 保护，热路径 Write 无锁
type CSVRecorder struct {
	// 配置（Start 时设置，writer goroutine 只读）
	cfg core.RecordingConfig

	// 异步队列
	queue   chan core.PressureSnapshot
	stopCh  chan struct{}
	doneCh  chan struct{}
	started atomic.Bool

	// 运行时状态（writer goroutine 单线程更新，Status 读时加锁）
	statsMu     sync.RWMutex
	session     core.RecordingSession
	writers     map[string]*perDeviceWriter // deviceId -> writer（每设备一个文件）
	totalSize   int64                       // 所有设备所有文件累计大小
	recordCount int64                       // 所有设备累计记录数
	fileCount   int                         // 所有设备累计文件数
	startedAt   time.Time

	// 丢弃计数（atomic 无锁）
	dropped         atomic.Int64
	droppedSinceLog atomic.Int64
	lastDropLogAt   atomic.Int64

	// 错误（writer goroutine 写，Status 读）
	errMu     sync.RWMutex
	lastError string
}

// NewCSVRecorder 创建 CSV 录制器
func NewCSVRecorder() *CSVRecorder {
	return &CSVRecorder{}
}

// Start 开始录制
func (r *CSVRecorder) Start(config core.RecordingConfig) error {
	if !r.started.CompareAndSwap(false, true) {
		return fmt.Errorf("recording already in progress")
	}

	// 应用默认值
	cfg := applyCSVDefaults(config)

	if err := os.MkdirAll(cfg.OutputDir, 0755); err != nil {
		r.started.Store(false)
		return fmt.Errorf("create output dir: %w", err)
	}

	r.cfg = cfg
	r.queue = make(chan core.PressureSnapshot, cfg.QueueCapacity)
	r.stopCh = make(chan struct{})
	r.doneCh = make(chan struct{})
	r.dropped.Store(0)
	r.droppedSinceLog.Store(0)
	r.lastDropLogAt.Store(0)
	r.totalSize = 0
	r.recordCount = 0
	r.fileCount = 0
	r.writers = make(map[string]*perDeviceWriter)
	// 重置上次会话的 lastError，避免新会话 Status() 返回旧错误
	r.errMu.Lock()
	r.lastError = ""
	r.errMu.Unlock()

	now := time.Now()
	r.statsMu.Lock()
	r.session = core.RecordingSession{
		ID:          fmt.Sprintf("rec_%d", now.UnixMilli()),
		OutputDir:   cfg.OutputDir,
		FilePrefix:  cfg.FilePrefix,
		StartTimeMs: now.UnixMilli(),
		Status:      core.RecordingActive,
	}
	r.startedAt = now
	r.statsMu.Unlock()

	// 不预创建文件：第一个 payload 到达时按 deviceId 懒创建，
	// 避免空文件（多设备场景下未投递数据的设备不应产生空 CSV）

	go r.writerLoop()
	return nil
}

// applyCSVDefaults 填充默认值
func applyCSVDefaults(cfg core.RecordingConfig) core.RecordingConfig {
	if cfg.FlushIntervalMs <= 0 {
		cfg.FlushIntervalMs = csvDefaultFlushIntervalMs
	}
	if cfg.SyncIntervalSec <= 0 {
		cfg.SyncIntervalSec = csvDefaultSyncIntervalSec
	}
	if cfg.QueueCapacity <= 0 {
		cfg.QueueCapacity = csvDefaultQueueCapacity
	}
	return cfg
}

// Write 异步投递一条快照到队列（非阻塞，队列满时丢弃并计数）
func (r *CSVRecorder) Write(snapshot core.PressureSnapshot) error {
	if !r.started.Load() {
		return fmt.Errorf("recording sink is not started")
	}
	select {
	case r.queue <- snapshot:
	default:
		// 队列满：丢弃并计数（快路径，无锁）
		r.dropped.Add(1)
		r.droppedSinceLog.Add(1)
		// 节流检查：上次日志时间距今超过 csvDropLogInterval 才进入慢路径
		last := r.lastDropLogAt.Load()
		now := time.Now().UnixNano()
		if now-last < int64(csvDropLogInterval) {
			return nil
		}
		// CAS 抢占日志权：避免多 goroutine 同时输出
		if !r.lastDropLogAt.CompareAndSwap(last, now) {
			return nil
		}
		sinceLog := r.droppedSinceLog.Swap(0)
		slog.Warn("CSVRecorder 队列已满，丢弃 payload（节流聚合）",
			"droppedSinceLog", sinceLog,
			"totalDropped", r.dropped.Load())
	}
	return nil
}

// Stop 停止录制：drain 队列后关闭文件
// 重复 Stop 是幂等的：CAS 失败（已停止）静默返回 nil，不影响调用方。
func (r *CSVRecorder) Stop() error {
	if !r.stopWithErrorLocked("") {
		// CAS 失败：录制已停止（可能是 auto-stop 或重复 Stop），记录便于排查
		slog.Debug("CSVRecorder.Stop CAS 失败：录制已停止", "hint", "auto-stop or duplicate stop")
	}
	return nil
}

// StopWithError 先填充错误原因再停止录制，CAS 保护原子性：
//   - 录制活跃：写入 lastError → drain 队列 → 关闭文件 → 同步 session，返回 true
//   - 录制已停止：不修改任何状态，返回 false
//
// 用途：设备断连自动停止录制场景，避免与用户主动 StopRecording 竞争时
// 把"用户主动停止"误覆盖为"设备断连自动停止"。
//
// msg 为空时等价于 Stop（用于用户主动停止路径，不覆盖既有 lastError）。
func (r *CSVRecorder) StopWithError(msg string) bool {
	return r.stopWithErrorLocked(msg)
}

// stopWithErrorLocked 内部共享实现：CAS 成功后写入 msg（非空时），再 drain + 关文件。
// msg=="" 时不修改 lastError，保持 Stop 语义不变。
func (r *CSVRecorder) stopWithErrorLocked(msg string) bool {
	if !r.started.CompareAndSwap(true, false) {
		return false
	}
	// CAS 成功后没有任何其他路径能再翻转 started；lastError 的写入与后续
	// session 同步在 statsMu 内串行，对外可见的最终状态一致。
	if msg != "" {
		r.errMu.Lock()
		r.lastError = msg
		r.errMu.Unlock()
	}
	close(r.stopCh)
	<-r.doneCh

	// 同步最终状态
	r.statsMu.Lock()
	r.session.Status = core.RecordingIdle
	r.session.DroppedCount = r.dropped.Load()
	r.session.FileCount = int64(r.fileCount)
	r.session.SnapshotCount = r.recordCount
	r.errMu.RLock()
	r.session.LastError = r.lastError
	r.errMu.RUnlock()
	r.statsMu.Unlock()
	return true
}

// Status 获取录制状态
func (r *CSVRecorder) Status() core.RecordingSession {
	r.statsMu.RLock()
	defer r.statsMu.RUnlock()
	s := r.session
	s.SnapshotCount = r.recordCount
	s.DroppedCount = r.dropped.Load()
	s.FileCount = int64(r.fileCount)
	// CurrentFile 聚合所有设备当前文件名（逗号分隔），便于 UI 显示
	var currentFiles []string
	for _, w := range r.writers {
		if w.fileName != "" {
			currentFiles = append(currentFiles, w.fileName)
		}
	}
	s.CurrentFile = strings.Join(currentFiles, ", ")
	r.errMu.RLock()
	s.LastError = r.lastError
	r.errMu.RUnlock()
	return s
}

// writerLoop 单 writer goroutine 串行消费队列
func (r *CSVRecorder) writerLoop() {
	// 捕获本次会话的 doneCh 到局部变量：auto-stop 翻转 started 后到此处 defer 之间，
	// 若被 usecase 重入 Start 重新赋值 r.doneCh，旧 goroutine 关闭的仍是自己的 channel，
	// 不会误关新会话的 doneCh 导致新 writerLoop 提前退出。
	doneCh := r.doneCh
	defer close(doneCh)

	flushTicker := time.NewTicker(time.Duration(r.cfg.FlushIntervalMs) * time.Millisecond)
	syncTicker := time.NewTicker(time.Duration(r.cfg.SyncIntervalSec) * time.Second)
	defer flushTicker.Stop()
	defer syncTicker.Stop()

	var buf []byte // 复用 byte buffer 用于格式化

	for {
		select {
		case <-r.stopCh:
			// drain 剩余 payload
			r.drainQueue(&buf)
			if err := r.flushAndSyncAll(true); err != nil {
				r.recordError(err)
			}
			r.closeAllFiles()
			return
		case p := <-r.queue:
			if err := r.writePayload(&buf, p); err != nil {
				// I/O 错误：先 flush+close 所有文件，最后才翻转 started（与 windlabx4 对齐），
				// 避免 started=false 但 writerLoop 仍在做 I/O 的窗口。
				r.recordError(err)
				r.flushAndSyncAll(true)
				r.closeAllFiles()
				r.markStopped()
				return
			}
			// 评估停止条件（跨设备汇总）/ 文件滚动（按设备独立）
			if r.shouldAutoStop() {
				// 自停止：drain 剩余数据 → flush+close → markStopped。
				// 全部 I/O 清理完成后才翻转 started，避免与 Stop/Start 重入竞争。
				r.drainQueue(&buf)
				r.flushAndSyncAll(true)
				r.closeAllFiles()
				r.markStopped()
				return
			} else if r.shouldRotate(p.DeviceID) {
				if err := r.rotateFile(p.DeviceID); err != nil {
					r.recordError(err)
					r.flushAndSyncAll(true)
					r.closeAllFiles()
					r.markStopped()
					return
				}
			}
		case <-flushTicker.C:
			r.flushAndSyncAll(false)
		case <-syncTicker.C:
			r.flushAndSyncAll(true)
		}
	}
}

// markStopped 在 writerLoop 完成所有 I/O 清理（flush+closeFile）后调用，
// 是翻转 started 标志与 session 状态的唯一出口。保证 writerLoop 退出前
// 文件已关闭，Stop/Start 重入不会撞上正在运行的 I/O。
func (r *CSVRecorder) markStopped() {
	r.statsMu.Lock()
	r.session.Status = core.RecordingIdle
	r.statsMu.Unlock()
	r.started.Store(false)
}

// drainQueue 排空队列中剩余 payload
func (r *CSVRecorder) drainQueue(buf *[]byte) {
	for {
		select {
		case p := <-r.queue:
			if err := r.writePayload(buf, p); err != nil {
				r.recordError(err)
				return
			}
		default:
			return
		}
	}
}

// writePayload 写入一条快照（writer goroutine 独占，无需锁）
// 格式：单快照单行 18 列（Timestamp + 16 压力 + 大气压 + 大温）
// 使用 strconv.AppendFloat 替代 fmt.Sprintf，避免反射开销，吞吐提升 2-5 倍。
//
// 时间来源：硬件时间戳优先（snapshot.HardwareTimestamp > 0 时使用），否则用系统毫秒时间戳。
// 注意：WISPA 设备时间戳存在已知固件 bug（fractional 字段递增不正确），
// 且系统毫秒时间戳在 1000Hz 下精度不足。统一截断到秒级，避免展示错误的时间细分。
// 前缀单引号强制 Excel 按文本显示，避免被默认 "yyyy/m/d h:mm" 格式隐藏秒。
func (r *CSVRecorder) writePayload(buf *[]byte, snapshot core.PressureSnapshot) error {
	w, err := r.getOrCreateWriter(snapshot.DeviceID)
	if err != nil {
		return err
	}

	var t time.Time
	if snapshot.HardwareTimestamp > 0 {
		sec := int64(snapshot.HardwareTimestamp)
		nsec := int64((snapshot.HardwareTimestamp - float64(sec)) * 1e9)
		t = time.Unix(sec, nsec)
	} else {
		t = time.UnixMilli(snapshot.Timestamp)
	}

	// 复用 buf，避免每次分配
	// 可读时间戳：截断到秒级（无小数）。
	// 原因：设备时间戳固件 bug 导致毫秒部分不可信；系统时间毫秒精度在 1000Hz 下不足。
	// 前缀单引号强制 Excel 按文本显示。
	b := (*buf)[:0]
	b = append(b, '\'')
	b = t.AppendFormat(b, "2006-01-02 15:04:05")
	b = append(b, ',')

	for i, v := range snapshot.Values {
		// REC-006 禁用通道占位：用户在配置面板关闭某通道后，CSV 该列应留空（仅保留逗号），
		// 保持 18 列对齐。这样 Excel/脚本读 CSV 时可按列号定位通道，
		// 不会把禁用通道的硬件残留值（0 或最后采样值）误当成有效数据。
		//
		// 多设备隔离：优先用 DeviceChannels[deviceID] 按设备独立判断 Enabled，
		// 避免"设备 A 关闭 CH1 → 设备 B 的 CH1 数据也被置空"的回归。
		// 缺省回退到共享 Channels（向后兼容老调用方）。
		// 边界：配置长度小于 18（旧 profile 或配置切换瞬间）时按"启用"处理，避免误丢有效数据。
		channels := r.cfg.DeviceChannels[snapshot.DeviceID]
		if channels == nil {
			channels = r.cfg.Channels
		}
		if i < len(channels) && !channels[i].Enabled {
			b = append(b, ',')
			continue
		}
		p := defaultPrecision
		if i < len(channels) {
			cp := channels[i].Precision
			if cp >= 0 && cp <= maxPrecision {
				p = cp
			}
		}
		// strconv.AppendFloat 复用底层数组，零分配
		b = strconv.AppendFloat(b, v, 'f', p, 64)
		b = append(b, ',')
	}

	// 去掉末尾逗号，写入换行
	b[len(b)-1] = '\n'
	if _, err := w.bw.Write(b); err != nil {
		return err
	}
	written := int64(len(b))
	*buf = b

	// 更新统计（writer goroutine 单线程，但仍持锁以与 Status() 互斥）
	r.statsMu.Lock()
	w.fileSize += written
	w.fileRecordCount++
	r.recordCount++
	r.totalSize += written
	r.statsMu.Unlock()
	return nil
}

// getOrCreateWriter 按 deviceId 找/建 perDeviceWriter。
// 仅在 writerLoop 单线程调用，但 Status() 可能并发读 writers map，故持锁。
func (r *CSVRecorder) getOrCreateWriter(deviceID string) (*perDeviceWriter, error) {
	r.statsMu.Lock()
	defer r.statsMu.Unlock()
	if w, ok := r.writers[deviceID]; ok {
		return w, nil
	}
	// 从 RecordingConfig.DeviceNames 取设备名，找不到则回退到 deviceId
	deviceName := deviceID
	if name, ok := r.cfg.DeviceNames[deviceID]; ok && name != "" {
		deviceName = name
	}
	w := &perDeviceWriter{
		deviceName: deviceName,
		fileSlug:   r.uniqueFileSlugLocked(deviceName, deviceID),
	}
	if err := r.openNewFileForLocked(w); err != nil {
		return nil, err
	}
	r.writers[deviceID] = w
	slog.Info("CSVRecorder 为设备创建文件",
		"component", "CSVRecorder",
		"deviceId", deviceID,
		"deviceName", deviceName,
		"file", w.fileName,
	)
	return w, nil
}

// uniqueFileSlugLocked 生成用于文件名的设备段：
//   - sanitize(deviceName) 为空时回退到 "device"
//   - 若与其他设备已用 slug 冲突，追加 -<deviceID 前 6 位> 保证唯一
//
// 调用方必须持 statsMu 锁（读 writers map）。
func (r *CSVRecorder) uniqueFileSlugLocked(deviceName, deviceID string) string {
	slug := sanitizeFileSegment(deviceName)
	if slug == "" {
		slug = "device"
	}
	// 遍历现有 writer 检查是否有 slug 冲突（同名设备）
	conflict := false
	for _, other := range r.writers {
		if other.fileSlug == slug {
			conflict = true
			break
		}
	}
	if !conflict {
		return slug
	}
	// 冲突时追加 deviceID 前 6 位（UUID 前 6 位重复概率极低），
	// 若 deviceID 也短则整体使用
	suffix := deviceID
	if len(suffix) > 6 {
		suffix = suffix[:6]
	}
	if suffix == "" {
		return slug
	}
	return slug + "-" + suffix
}

// sanitizeFileSegment 把设备名规范化为可用作文件名的段：
//   - 替换 Windows/POSIX 非法字符 \/:*?"<>| 及控制字符为 '_'
//   - 折叠首尾空白和多余的 '_'/'-'
//   - 限制长度到 40，避免拼上时间戳后超出 Windows MAX_PATH
//
// 保留中文、数字、字母、连字符、下划线、点。
func sanitizeFileSegment(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	var b strings.Builder
	b.Grow(len(name))
	for _, r := range name {
		switch {
		case r < 0x20 || r == 0x7f:
			b.WriteByte('_')
		case r == '\\' || r == '/' || r == ':' || r == '*' || r == '?' ||
			r == '"' || r == '<' || r == '>' || r == '|':
			b.WriteByte('_')
		case r == ' ' || r == '\t':
			b.WriteByte('_')
		default:
			b.WriteRune(r)
		}
	}
	out := b.String()
	// 折叠连续的下划线/连字符
	for strings.Contains(out, "__") {
		out = strings.ReplaceAll(out, "__", "_")
	}
	out = strings.Trim(out, "_-.")
	if len(out) > 40 {
		// 按 rune 截断，避免砍出半个中文字符
		runes := []rune(out)
		if len(runes) > 40 {
			runes = runes[:40]
		}
		out = string(runes)
		out = strings.Trim(out, "_-.")
	}
	return out
}

// shouldRotate 评估指定设备是否需要滚动到新文件（按设备独立评估）。
// 调用方必须不持 statsMu 锁，内部自行加锁。
func (r *CSVRecorder) shouldRotate(deviceID string) bool {
	r.statsMu.RLock()
	defer r.statsMu.RUnlock()
	w, ok := r.writers[deviceID]
	if !ok {
		return false
	}
	rot := r.cfg.Rotation
	if rot.MaxSizeBytes > 0 && w.fileSize >= rot.MaxSizeBytes {
		return true
	}
	if rot.MaxRecordCount > 0 && w.fileRecordCount >= rot.MaxRecordCount {
		return true
	}
	if rot.MaxDurationMs > 0 && !w.fileStartedAt.IsZero() {
		if time.Since(w.fileStartedAt).Milliseconds() >= rot.MaxDurationMs {
			return true
		}
	}
	return false
}

// shouldAutoStop 评估是否触发自动停止（跨设备汇总）
func (r *CSVRecorder) shouldAutoStop() bool {
	sc := r.cfg.StopConditions
	r.statsMu.RLock()
	totalSize := r.totalSize
	recordCount := r.recordCount
	startedAt := r.startedAt
	r.statsMu.RUnlock()

	if sc.MaxFileSizeBytes > 0 && totalSize >= sc.MaxFileSizeBytes {
		return true
	}
	if sc.MaxRecordCount > 0 && recordCount >= sc.MaxRecordCount {
		return true
	}
	if sc.MaxDurationMs > 0 && !startedAt.IsZero() {
		if time.Since(startedAt).Milliseconds() >= sc.MaxDurationMs {
			return true
		}
	}
	return false
}

// openNewFileForLocked 为指定设备创建新文件并写入表头（调用方持 statsMu 锁）。
// 文件名格式：<prefix>-<deviceSlug>-YYYYMMDD-HHMMSS-NNN.csv
// NNN 从该设备当前 fileCount+1 开始递增。若目标文件已存在（同秒内重启且序号撞上），序号 +1 重试。
func (r *CSVRecorder) openNewFileForLocked(w *perDeviceWriter) error {
	w.fileCount++
	fileCount := w.fileCount

	base := fmt.Sprintf("%s-%s-%s", r.cfg.FilePrefix, w.fileSlug, time.Now().Format("20060102-150405"))
	var name string
	var file *os.File
	var err error
	for seq := fileCount; seq < fileCount+fileNameSeqRetry; seq++ {
		name = fmt.Sprintf("%s-%03d.csv", base, seq)
		full := filepath.Join(r.cfg.OutputDir, name)
		// O_CREATE|O_EXCL 保证不覆盖已存在文件
		file, err = os.OpenFile(full, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
		if err == nil {
			break
		}
		if os.IsExist(err) {
			continue
		}
		return err
	}
	if err != nil {
		return fmt.Errorf("create file: %w", err)
	}

	bw := bufio.NewWriterSize(file, csvDefaultBufferSize)

	// 19 列 CSV 表头：Timestamp（秒级精度）+ 16 压力 + 大气压 + 大温
	// 每个文件（含滚动文件）都要写入表头
	header := make([]string, 0, numChannels+1)
	header = append(header, "Timestamp")
	for i := 0; i < numPressureChannels; i++ {
		header = append(header, fmt.Sprintf("CH%02d", i+1))
	}
	header = append(header, "CH17_AtmPressure")
	header = append(header, "CH18_AtmTemp")
	if _, err := bw.Write([]byte(joinCSVHeader(header))); err != nil {
		file.Close()
		return fmt.Errorf("write header: %w", err)
	}
	if err := bw.Flush(); err != nil {
		file.Close()
		return fmt.Errorf("flush header: %w", err)
	}

	w.file = file
	w.bw = bw
	w.fileName = name
	w.fileSize = 0
	w.fileRecordCount = 0
	w.fileStartedAt = time.Now()
	r.fileCount++

	r.session.FileCount = int64(r.fileCount)
	r.session.CurrentFile = name
	return nil
}

// joinCSVHeader 简单的 CSV 表头拼接（避免引入 encoding/csv 的反射开销）
func joinCSVHeader(fields []string) string {
	var b []byte
	for i, f := range fields {
		if i > 0 {
			b = append(b, ',')
		}
		b = append(b, f...)
	}
	b = append(b, '\n')
	return string(b)
}

// rotateFile 滚动指定设备到新文件
// 仅在 writerLoop 单线程调用，持 statsMu 锁以与 Status() 互斥（防止读 fileName 时被改）。
func (r *CSVRecorder) rotateFile(deviceID string) error {
	r.statsMu.Lock()
	defer r.statsMu.Unlock()
	w, ok := r.writers[deviceID]
	if !ok {
		return nil
	}
	if err := flushWriter(w, true); err != nil {
		return err
	}
	if err := w.file.Close(); err != nil {
		return fmt.Errorf("close csv file %s: %w", w.fileName, err)
	}
	return r.openNewFileForLocked(w)
}

// closeAllFiles 关闭所有设备文件（正常退出路径）
func (r *CSVRecorder) closeAllFiles() {
	r.statsMu.Lock()
	defer r.statsMu.Unlock()
	for _, w := range r.writers {
		if w.bw != nil {
			_ = w.bw.Flush()
		}
		if w.file != nil {
			_ = w.file.Close()
		}
	}
}

// flushAndSyncAll 遍历所有设备文件执行 flush（+sync）
// 仅在 writerLoop 单线程调用，与 rotateFile/closeAllFiles 不会并发。
// 先 RLock 拷贝 writers 列表后释放，避免长时间持锁阻塞 Status()。
func (r *CSVRecorder) flushAndSyncAll(sync bool) error {
	r.statsMu.RLock()
	writers := make([]*perDeviceWriter, 0, len(r.writers))
	for _, w := range r.writers {
		writers = append(writers, w)
	}
	r.statsMu.RUnlock()

	var firstErr error
	for _, w := range writers {
		if err := flushWriter(w, sync); err != nil {
			if firstErr == nil {
				firstErr = err
			}
			slog.Error("CSVRecorder flush/sync 失败",
				"component", "CSVRecorder",
				"file", w.fileName,
				"error", err)
		}
	}
	return firstErr
}

// flushWriter 刷新单个 writer 的 bufio 缓冲，sync=true 时额外 fsync。
// 仅在 writerLoop 单线程调用，无需持锁（writer goroutine 独占 w.bw/w.file）。
func flushWriter(w *perDeviceWriter, sync bool) error {
	if w.bw == nil {
		return nil
	}
	if err := w.bw.Flush(); err != nil {
		return fmt.Errorf("flush csv file %s: %w", w.fileName, err)
	}
	if sync && w.file != nil {
		if err := w.file.Sync(); err != nil {
			return fmt.Errorf("sync csv file %s: %w", w.fileName, err)
		}
	}
	return nil
}

// recordError 记录最近一次 I/O 错误（lastError + 日志）。
// 仅记录错误信息，不翻转 started/session 状态——状态翻转由调用方在
// 完成 flush+closeFile 后通过 markStopped 统一执行，避免出现
// "started=false 但 writerLoop 仍在做 I/O" 的窗口。
func (r *CSVRecorder) recordError(err error) {
	r.errMu.Lock()
	r.lastError = err.Error()
	r.errMu.Unlock()
	r.statsMu.Lock()
	r.session.LastError = err.Error()
	r.statsMu.Unlock()
	slog.Error("CSVRecorder 写入失败", "error", err)
}

// IsActive 无锁热路径判活。relayStream 每帧调用，避免 Status() 的双 RLock
// 与 writer goroutine 的 statsMu 写锁在 1kHz×N 设备下争用。
func (r *CSVRecorder) IsActive() bool {
	return r.started.Load()
}
