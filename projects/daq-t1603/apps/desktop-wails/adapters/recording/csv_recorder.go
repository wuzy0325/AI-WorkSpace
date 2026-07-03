package recording

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"daq-t1603/core"
)

const (
	// csvFlushInterval 单个设备 writer 的固定 flush 周期。
	// 10 台 × 1000Hz 场景下，每设备 200ms 累积约 200 行 ≈ 30KB，
	// 远小于 bufio 缓冲（256KB），可平滑落盘且 syscall 频率可控
	// （10 设备 × 5 次/s = 50 次/s，远低于逐帧 10000 次/s）。
	csvFlushInterval = 200 * time.Millisecond

	// csvBufferSize 单设备 bufio 缓冲大小。
	// 256KB ≈ 1700 行（150B/行），覆盖 1.7s 的 1000Hz 流量，
	// 即便 flush ticker 抖动也有充足余量。
	csvBufferSize = 256 * 1024

	// deviceRecChCap 单设备录制队列容量。
	// 8192 帧 = 1000Hz 下 8 秒缓冲，吸收 Windows Defender 实时扫描等
	// 短时磁盘抖动；超过此阈值视为持续背压，丢弃新帧并告警。
	deviceRecChCap = 8192

	// creatorRetryIntervalMs writer 创建失败后的限频重试间隔。
	// 防止磁盘满/权限错误时 1000Hz × 10 设备 = 10kHz 的错误日志刷屏。
	creatorRetryIntervalMs = int64(1000)
)

// ErrRecordingNotActive 表示录制未启动时调用 Write 返回的哨兵错误。
// relay 热路径用 IsActive() 提前判断，正常不会命中此错误。
var ErrRecordingNotActive = errors.New("recording not active")

// BackpressureHandler 是背压回调签名，由上层注入转发到日志/事件。
// handler 内部禁止回调 recorder 任何方法，避免死锁。
type BackpressureHandler = func(core.BackpressureEvent)

// FatalErrorHandler 在 deviceWriter 发生不可恢复 I/O 错误时被调用，
// 上层据此发 fatal 日志并标记设备录制不可用。同 BackpressureHandler
// 约束：handler 内部禁止回调 recorder 任何方法。
type FatalErrorHandler = func(deviceID string, err error)

// CSVRecorder 是多设备 CSV 录制协调器。
//
// 10 台 × 1000Hz 全量保存设计要点：
//   - 每设备独立 deviceWriter（独立 *os.File / bufio.Writer / goroutine），
//     消除单 mutex 锁竞争；
//   - relay 调用 Write 为非阻塞投递：队列满时丢帧 + 计数 + 回调，
//     避免 deviceWriter 磁盘抖动反压到 readLoop 导致设备侧丢帧；
//   - bufio 256KB + 200ms flush ticker，syscall 频率从 10kHz 降到 50Hz；
//   - 行缓冲通过 sync.Pool 复用 []byte，降低 GC 压力；
//   - 全局 active 用 atomic.Bool，热路径无锁；
//   - writer 创建失败时限频重试，避免错误日志刷屏；
//   - bufio.Write 失败（磁盘满/IO 错误）后停 goroutine + 发 fatal，
//     避免静默丢帧。
type CSVRecorder struct {
	// mu 仅保护 writers map 的增删与 session 元信息读写。
	// 不保护 deviceWriter 内部状态（每个 writer 自治理）。
	mu        sync.Mutex
	writers   map[string]*deviceWriter
	session   core.RecordingSession
	outputDir string
	prefix    string

	// creatorFailedUntil 记录每设备 writer 创建失败的限频截止时间。
	// 截止前 Write 直接返回，不重试创建，避免错误日志刷屏。
	creatorFailedUntil map[string]int64

	// active 是热路径无锁判活标志，relay 每帧都查。
	active atomic.Bool

	// totalCount 跨设备聚合快照计数，供 Status() 返回。
	totalCount atomic.Int64

	// droppedTotal 跨设备聚合丢帧计数，用于健康度评估。
	droppedTotal atomic.Int64

	// onBackpressure 是背压回调（通常由 adapter 注入，转发到日志/事件）。
	// 调用方需保证非阻塞；recorder 在丢帧时同步调用。
	onBackpressure atomic.Pointer[BackpressureHandler]

	// onFatal 是 deviceWriter 不可恢复错误回调，由 service 注入。
	onFatal atomic.Pointer[FatalErrorHandler]
}

// NewCSVRecorder 创建空的录制协调器。
func NewCSVRecorder() *CSVRecorder {
	return &CSVRecorder{
		writers:            make(map[string]*deviceWriter),
		creatorFailedUntil: make(map[string]int64),
	}
}

// SetBackpressureHandler 注入背压回调。
// 必须在 Start 之前调用；handler 内部不可调用 recorder 的任何方法，避免死锁。
func (r *CSVRecorder) SetBackpressureHandler(handler BackpressureHandler) {
	if handler == nil {
		return
	}
	r.onBackpressure.Store(&handler)
}

// SetFatalErrorHandler 注入 deviceWriter 不可恢复错误回调。
// 必须在 Start 之前调用；handler 内部不可调用 recorder 的任何方法。
func (r *CSVRecorder) SetFatalErrorHandler(handler FatalErrorHandler) {
	if handler == nil {
		return
	}
	r.onFatal.Store(&handler)
}

// Start 启动全局录制会话。
//
// 语义：开启后所有当前/后续设备的 snapshot 都会落盘到
// `<outputDir>/<prefix>_<deviceID>_<ts>.csv`，每设备独立文件。
// 已存在的 deviceWriter 会在锁外被关闭重建（防止 session 边界混淆）。
func (r *CSVRecorder) Start(outputDir string, prefix string) error {
	if r.active.Load() {
		return fmt.Errorf("recording already in progress")
	}

	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return fmt.Errorf("create output dir: %w", err)
	}

	r.mu.Lock()
	// 取出旧 writer 引用，锁外关闭（避免持锁等待 goroutine 退出）
	oldWriters := r.writers
	r.writers = make(map[string]*deviceWriter)
	r.creatorFailedUntil = make(map[string]int64)
	r.outputDir = outputDir
	r.prefix = prefix
	r.session = core.RecordingSession{
		ID:          fmt.Sprintf("rec_%d", time.Now().UnixNano()),
		OutputDir:   outputDir,
		FilePrefix:  prefix,
		StartTimeMs: time.Now().UnixMilli(),
		Status:      core.RecordingActive,
	}
	r.mu.Unlock()

	// 锁外关闭旧 writer（run goroutine 不访问 r.mu，不会死锁）
	for _, w := range oldWriters {
		w.close()
	}

	r.totalCount.Store(0)
	r.droppedTotal.Store(0)
	// 最后才置 active，避免热路径看到 active=true 但 session 未就绪
	r.active.Store(true)
	return nil
}

// IsActive 无锁热路径判活。
func (r *CSVRecorder) IsActive() bool {
	return r.active.Load()
}

// Write 将 snapshot 投递到对应设备的录制队列。
//
// 非阻塞语义：
//   - 队列未满：立即入队返回 nil；
//   - 设备 writer 创建失败限频期内：返回 nil，不重试不刷屏；
//   - 设备 writer 已 dead（不可恢复 I/O 错误）：返回 nil，静默丢帧；
//   - 队列已满（持续磁盘抖动 > 8s）：丢此帧，droppedTotal++，
//     触发 onBackpressure 回调，返回 nil。
//
// 设计上 Write 永不返回 error（除非开发期 bug），
// 避免 relay 因错误日志刷屏。
//
// 设备首次出现时惰性创建 deviceWriter。
func (r *CSVRecorder) Write(snapshot core.TemperatureSnapshot) error {
	if !r.active.Load() {
		return nil
	}
	if snapshot.DeviceID == "" {
		return nil
	}

	w, ok := r.getOrCreateWriter(snapshot.DeviceID)
	if !ok {
		// writer 创建失败限频期内或已 dead，静默丢帧
		return nil
	}

	select {
	case w.recCh <- snapshot:
		w.count.Add(1)
		r.totalCount.Add(1)
		return nil
	default:
	}

	// 队列满：丢帧 + 告警
	dropped := w.dropped.Add(1)
	r.droppedTotal.Add(1)
	if handler := r.onBackpressure.Load(); handler != nil {
		(*handler)(core.BackpressureEvent{
			DeviceID:     snapshot.DeviceID,
			QueueLen:     len(w.recCh),
			QueueCap:     cap(w.recCh),
			DroppedTotal: dropped,
		})
	}
	return nil
}

// getOrCreateWriter 查找或创建设备 writer。
//
// 返回 (writer, true) 表示拿到可用 writer；
// 返回 (nil, false) 表示创建失败限频期内或已 dead，调用方应静默丢帧。
//
// 创建路径下的 I/O 操作（建文件/写表头）不在 r.mu 内长持锁，
// 避免 Start/Stop 路径被阻塞。失败状态缓存在 creatorFailedUntil，
// 限频期内不重试，避免错误日志刷屏。
func (r *CSVRecorder) getOrCreateWriter(deviceID string) (*deviceWriter, bool) {
	r.mu.Lock()
	if w, ok := r.writers[deviceID]; ok {
		// writer 已存在：dead 状态返回 false 让上层静默丢帧
		dead := w.isDead()
		r.mu.Unlock()
		if dead {
			return nil, false
		}
		return w, true
	}
	// 检查创建失败限频
	if until, ok := r.creatorFailedUntil[deviceID]; ok {
		if core.TimestampMs() < until {
			r.mu.Unlock()
			return nil, false
		}
		delete(r.creatorFailedUntil, deviceID)
	}
	// 在锁内取出 session 元信息快照，锁外创建 writer
	outputDir := r.outputDir
	prefix := r.prefix
	r.mu.Unlock()

	if outputDir == "" || prefix == "" {
		return nil, false
	}

	w, err := newDeviceWriter(deviceID, outputDir, prefix)
	if err != nil {
		// 创建失败：缓存失败状态限频，避免每帧重试刷屏
		r.mu.Lock()
		r.creatorFailedUntil[deviceID] = core.TimestampMs() + creatorRetryIntervalMs
		r.mu.Unlock()
		// 同步通知上层发 fatal 日志（每秒最多 1 条，由 service 限频）
		if handler := r.onFatal.Load(); handler != nil {
			(*handler)(deviceID, err)
		}
		return nil, false
	}

	r.mu.Lock()
	if existing, ok := r.writers[deviceID]; ok {
		// 并发创建竞态：复用已存在的，关闭新建的。
		// 注意：新建的 w 从未 start()，不可调 w.close()，
		// 否则 <-w.closed 永久阻塞（run goroutine 未启动）。
		// 直接关文件即可。
		r.mu.Unlock()
		_ = w.file.Close()
		if existing.isDead() {
			return nil, false
		}
		return existing, true
	}
	r.writers[deviceID] = w
	r.mu.Unlock()
	w.start()
	return w, true
}

// Stop 停止所有设备 writer，最终 flush 并关闭文件。
// 幂等：重复调用安全。
func (r *CSVRecorder) Stop() error {
	// 先翻 active，让热路径立即停止入队
	r.active.Store(false)

	r.mu.Lock()
	writers := r.writers
	r.writers = make(map[string]*deviceWriter)
	r.creatorFailedUntil = make(map[string]int64)
	r.session.Status = core.RecordingIdle
	r.session.SnapshotCount = int(r.totalCount.Load())
	r.session.DroppedCount = int(r.droppedTotal.Load())
	r.mu.Unlock()

	for _, w := range writers {
		w.close()
	}
	return nil
}

// Status 返回聚合录制状态。
//
// 注意：SnapshotCount 是跨设备聚合值；如需 per-device 明细，
// 可扩展接口，但当前前端只需要聚合值。
func (r *CSVRecorder) Status() core.RecordingSession {
	r.mu.Lock()
	s := r.session
	r.mu.Unlock()
	s.SnapshotCount = int(r.totalCount.Load())
	s.DroppedCount = int(r.droppedTotal.Load())
	return s
}

// deviceWriter 是单设备的异步 CSV 写入器。
//
// 串行模型：单个 goroutine 独占 bw，无需 mu 保护 bufio 状态。
// relay 通过 recCh 非阻塞投递，bw 内部 256KB 缓冲 + 200ms ticker flush。
//
// 状态机：
//   - 正常运行：recCh 接收帧 → writeOne → bufio
//   - I/O 错误：writeOne 失败 → markDead → emit fatal → 继续读 recCh 但跳过写入
//     （避免 goroutine 退出后 recCh 堆积导致 relay 阻塞）
//   - Stop 信号：drain 队列内剩余帧 → 最终 flush → 关闭文件
type deviceWriter struct {
	deviceID string
	file     *os.File
	bw       *bufio.Writer
	recCh    chan core.TemperatureSnapshot
	done     chan struct{}
	closed   chan struct{}
	count    atomic.Int64
	dropped  atomic.Int64
	// dead 标记不可恢复 I/O 错误，true 后 writeOne 跳过写入。
	dead atomic.Bool
	// writeErr 记录首个 I/O 错误，供 markDead 时回调使用。
	writeErr atomic.Pointer[error]
	bufPool  sync.Pool
	stopOnce sync.Once
}

// newDeviceWriter 创建设备 writer 并写入 CSV 表头。
// 不启动 goroutine，由调用方在注册到 map 后调用 start()。
//
// 表头列：DeviceID, Timestamp, Millisecond, Unit, CH01..CH16
//   - Timestamp：人类可读秒精度（2006-01-02 15:04:05），Excel 友好，默认识别为时间类型
//   - Millisecond：毫秒部分（0-999 整数），1000Hz 采样下相邻样本靠此列区分
//   - Unit：快照单位（°C/°F/mV 等），10 台设备可能不同，必须落盘
//
// 时间来源：硬件时间戳优先（snap.HardwareTimestamp > 0 时使用），否则用系统毫秒时间戳。
func newDeviceWriter(deviceID, outputDir, prefix string) (*deviceWriter, error) {
	// 文件名加毫秒后缀，避免同设备同秒重建覆盖
	filename := fmt.Sprintf("%s_%s_%s.csv",
		prefix,
		sanitizeDeviceID(deviceID),
		time.Now().Format("20060102-150405.000"))
	filePath := filepath.Join(outputDir, filename)
	f, err := os.Create(filePath)
	if err != nil {
		return nil, fmt.Errorf("create file for %s: %w", deviceID, err)
	}

	w := &deviceWriter{
		deviceID: deviceID,
		file:     f,
		bw:       bufio.NewWriterSize(f, csvBufferSize),
		recCh:    make(chan core.TemperatureSnapshot, deviceRecChCap),
		done:     make(chan struct{}),
		closed:   make(chan struct{}),
		bufPool: sync.Pool{
			New: func() interface{} {
				return make([]byte, 0, 256)
			},
		},
	}

	// 写表头：DeviceID + 单列 Timestamp（带毫秒，前缀单引号强制 Excel 文本模式）+ Unit + CH01..CH16
	buf := w.bufPool.Get().([]byte)[:0]
	buf = append(buf, "DeviceID,Timestamp,Unit"...)
	for i := 0; i < 16; i++ {
		buf = append(buf, ',')
		buf = append(buf, fmt.Sprintf("CH%02d", i+1)...)
	}
	buf = append(buf, '\n')
	if _, err := w.bw.Write(buf); err != nil {
		f.Close()
		return nil, fmt.Errorf("write header for %s: %w", deviceID, err)
	}
	w.bufPool.Put(buf)
	return w, nil
}

// start 启动写入 goroutine。
func (w *deviceWriter) start() {
	go w.run()
}

// run 写入主循环：从 recCh 读 snapshot，格式化后写 bufio，200ms flush。
//
// 不再处理 recCh 被 close 的分支（代码中无任何位置 close(recCh)，
// Stop 用 close(done) 通知停止），保持逻辑精简。
func (w *deviceWriter) run() {
	ticker := time.NewTicker(csvFlushInterval)
	defer ticker.Stop()
	defer close(w.closed)

	for {
		select {
		case snap := <-w.recCh:
			w.writeOne(snap)
		case <-ticker.C:
			_ = w.bw.Flush()
		case <-w.done:
			// 收到停止信号：drain 队列内剩余帧后最终 flush
			for {
				select {
				case snap := <-w.recCh:
					w.writeOne(snap)
				default:
					_ = w.bw.Flush()
					_ = w.file.Close()
					return
				}
			}
		}
	}
}

// writeOne 格式化单帧并写入 bufio。
//
// 使用 strconv.AppendFloat 直接追加到复用 []byte，
// 避免 fmt.Sprintf / strconv.FormatFloat 的字符串分配。
//
// I/O 错误处理：
//   - 首次错误：markDead + emit fatal（通过 fatalCh）
//   - 后续：dead=true 时直接返回，跳过写入
//   - 不退出 goroutine：避免 recCh 堆塞导致 relay 阻塞
func (w *deviceWriter) writeOne(snap core.TemperatureSnapshot) {
	if w.dead.Load() {
		return
	}

	buf := w.bufPool.Get().([]byte)[:0]

	// DeviceID
	buf = append(buf, snap.DeviceID...)
	buf = append(buf, ',')

	// 时间来源：硬件时间戳优先（更精确），否则用系统毫秒时间戳
	var t time.Time
	if snap.HardwareTimestamp > 0 {
		sec := int64(snap.HardwareTimestamp)
		nsec := int64((snap.HardwareTimestamp - float64(sec)) * 1e9)
		t = time.Unix(sec, nsec)
	} else {
		t = time.UnixMilli(snap.Timestamp)
	}
	// 单列 Timestamp：截断到秒级。
	// 原因：DAQ-P-1604 等设备时间戳存在固件 bug（fractional 字段递增不正确），
	// 且系统毫秒时间戳在 1000Hz 下精度不足。统一秒级避免展示错误的时间细分。
	// 前缀单引号强制 Excel 按文本显示，避免被默认 "yyyy/m/d h:mm" 格式隐藏秒。
	buf = append(buf, '\'')
	buf = t.AppendFormat(buf, "2006-01-02 15:04:05")
	buf = append(buf, ',')

	// Unit（单位）：10 台设备可能不同，必须落盘
	buf = append(buf, snap.Unit...)

	// 16 通道值
	values := snap.Values
	for i := 0; i < 16; i++ {
		buf = append(buf, ',')
		if i < len(values) {
			buf = strconv.AppendFloat(buf, values[i], 'f', 3, 64)
		}
	}
	buf = append(buf, '\n')

	if _, err := w.bw.Write(buf); err != nil {
		w.markDead(err)
	}
	w.bufPool.Put(buf)
}

// markDead 标记 writer 不可恢复，记录首个错误。
// 后续 writeOne 检查 dead 标志跳过写入，避免 bufio 错误状态后的无效调用。
//
// 注意：不退出 run goroutine，避免 recCh 堆塞导致 relay 阻塞。
// 真正的资源回收由 close() 在 Stop 时完成。
func (w *deviceWriter) markDead(err error) {
	if !w.dead.CompareAndSwap(false, true) {
		return
	}
	w.writeErr.Store(&err)
}

// isDead 返回 writer 是否已不可恢复。
func (w *deviceWriter) isDead() bool {
	return w.dead.Load()
}

// close 关闭 writer：发送 done 信号并等待 goroutine 退出。
// 幂等：多次调用安全。仅在 writer 已 start 后调用。
func (w *deviceWriter) close() {
	w.stopOnce.Do(func() {
		close(w.done)
	})
	<-w.closed
}

// sanitizeDeviceID 将 deviceID 中可能影响文件名的字符替换为下划线。
// deviceID 通常已是合法标识符，此处作防御性处理。
func sanitizeDeviceID(id string) string {
	out := make([]byte, len(id))
	for i := 0; i < len(id); i++ {
		c := id[i]
		if c == '/' || c == '\\' || c == ':' || c == '*' || c == '?' || c == '"' || c == '<' || c == '>' || c == '|' || c == ' ' {
			out[i] = '_'
		} else {
			out[i] = c
		}
	}
	return string(out)
}
