package hardware

import (
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"shared.local/device-sdk/go/pkg/slog"
	"math"
	"net"
	"sync"
	"time"

	sharedproto "shared.local/device-sdk/go/protocol"
	"windlabx4/services/api-go/internal/core/device"
	"windlabx4/services/api-go/internal/ports"
)

const (
	// DAQ-P-1604Pre 默认连接参数（与 Cursor DAQ 实测一致）
	// 旧值 192.168.0.7:9001 是参考设备文档写的，实际设备是 192.168.3.232:23
	DAQ_P_1064PRE_DEFAULT_HOST = "192.168.3.232"
	DAQ_P_1064PRE_DEFAULT_PORT = 23
	DAQ_P_1064PRE_TIMEOUT      = 5 * time.Second
	// DAQ_P_1064PRE_CMD_TIMEOUT 是命令收发的总超时（Write + Read 之和）。
	//
	// 历史语义：原代码 SetReadDeadline 单次 timeoutMs（调用方传 2000ms），
	// Write 无独立 deadline 但 TCP 写缓冲通常即时完成。
	// 加 watchdog 后用 2 倍 timeoutMs 保持原 Write + Read 总耗时行为，
	// 避免引入总超时回归。
	// 调用方仍传 timeoutMs 作为单次 Read 的 deadline，watchdog 用 2 倍兜底。
	DAQ_P_1064PRE_CMD_TIMEOUT_FACTOR = 2
	// DAQ_P_1064PRE_READ_LOOP_WATCHDOG_TIMEOUT 是 readLoop 内 Read 阻塞的兜底超时。
	//
	// 设计依据 ADR-009 R0-6：readLoop 持 ioMu 期间阻塞 Read，若 SetReadDeadline(100ms)
	// 在故障 Windows 电脑上失效，Read 会无限阻塞，导致 sendCommand 永久拿不到 ioMu
	// 形成死锁。watchdog 在该超时后强制 Close conn，解除 Read 阻塞并释放 ioMu。
	//
	// 取值 10s：远大于正常 100ms deadline，仅在 deadline 失效的极端情况下触发，
	// 避免误杀正常的慢响应设备。StopAcquisition/Disconnect 不会等 watchdog 自然触发，
	// 而是通过 ReadLoopJoinTimeout（1s）超时主动 Close conn。
	DAQ_P_1064PRE_READ_LOOP_WATCHDOG_TIMEOUT = 10 * time.Second
)

const (
	CMD_READ_STATUS       = 0x00
	CMD_READ_RANGE        = 0x01
	CMD_READ_CALIBRATION  = 0x03
	CMD_READ_EXT_TRIGGER  = 0x13
	CMD_ACQUISITION_CTRL  = 0x14 // 采集控制命令码（参考 Cursor DAQ 实测值，旧值 0x10 设备不识别）
	CMD_WRITE_RANGE       = 0x81
	CMD_WRITE_CALIBRATION = 0x83
	CMD_FACTORY_RESET     = 0x84
	CMD_WRITE_EXT_TRIGGER = 0x93
)

const (
	ACQ_ACTION_STOP       = 0x00 // 停止采集
	ACQ_ACTION_SINGLE     = 0x01 // 单次采集
	ACQ_ACTION_CONTINUOUS = 0xFF // 连续采集（参考 Cursor DAQ 实测值，旧值 2 设备不识别）
	ACQ_DATA_MODE_RAW     = 0x11 // 原始 AD 数据
	ACQ_DATA_MODE_CALIB   = 0x13 // 校准后工程单位数据（参考 Cursor DAQ 实测值，旧值 1 设备不识别）
)

type CalibrationParams struct {
	Channel int
	B       float32
	K1      float32
}

type DeviceStatus1064Pre struct {
	EEPROMStatus uint16
	ADStatus     uint16
}

type DAQP1064Pre struct {
	mu             sync.RWMutex
	profile        device.Profile
	status         device.Status
	sink           device.DataSink
	stop           chan struct{}
	acquiring      bool
	conn           net.Conn
	streamFrameSeq uint32
	recvBuffer     []byte
	// firstDataLogged 首次收到数据时打印 hex dump，用于诊断协议格式
	firstDataLogged bool
	// onError 由 DeviceManager 在 Connect 阶段注入，readLoop 异常退出时回调，
	// 用于将设备异常（断网、读错误等）向上传播，由 DeviceManager 统一更新状态。
	onError func(err error)
	// readLoopDone 由 readLoop 退出时关闭，供 Start/Stop/Disconnect 等待协程结束。
	//
	// 设计依据 ADR-009：close(stop) 无法解除已进入内核 Read 的阻塞 goroutine
	// （Windows 故障电脑 SetReadDeadline 失效时 Read 永久阻塞）。
	// 调用方必须 join readLoopDone，超时则强制 Close conn 兜底，
	// 否则下次 StartAcquisition 会启动第二个 readLoop，TCP 字节随机分配导致数据错位。
	readLoopDone chan struct{}
	// ioMu 串行化 readLoop 的 Read 与 sendCommand 的 Write+Read，确保同一 TCP 字节流
	// 任意时刻只有一个 reader（ADR-009 R0-6 整改）。
	//
	// 历史背景：原实现 readLoop 和 sendCommand 可同时 conn.Read，TCP 字节随机分配到
	// 两个 reader，导致命令响应被 readLoop 抢走（sendCommand 超时关闭健康连接）或
	// 采集帧被 sendCommand 吞掉（数据错位）。
	//
	// 整改后：readLoop 和 sendCommand 在 I/O 阶段都需获取 ioMu。watchdog 必须在
	// ioMu.Lock 之前启动，覆盖"等 ioMu + Write + Read"全流程，避免 readLoop 持 ioMu
	// 阻塞在失效 deadline 的 Read 上时 sendCommand 永久等锁。
	//
	// 与 dsa3217.go ioMu 标杆对齐，便于跨项目 grep 复用同一模式。
	ioMu sync.Mutex
	// readLoopWatchdog 是 readLoop 内 Read 阻塞的 watchdog 超时；零值时回退到
	// DAQ_P_1064PRE_READ_LOOP_WATCHDOG_TIMEOUT 常量。暴露为字段供测试覆盖。
	readLoopWatchdog time.Duration
	// 嵌入 StopReasonTracker 提供 SetStopReason/GetStopReason/ClearStopReason，
	// 用于 readLoop 区分调用方主动停止（Stop/Disconnect）与连接意外断开，
	// 避免 Close 触发 Read 错误时误调 onError 误报设备故障。
	sharedproto.StopReasonTracker
}

// 编译期断言：DAQP1064Pre 实现 ports.ErrorNotifiable
var _ ports.ErrorNotifiable = (*DAQP1064Pre)(nil)

func NewDAQP1064Pre(profile device.Profile) *DAQP1064Pre {
	return &DAQP1064Pre{
		profile: profile,
		status: device.Status{
			ID:         profile.ID,
			Name:       profile.Name,
			Type:       profile.Type,
			Connection: device.ConnectionDisconnected,
		},
		recvBuffer: make([]byte, 0, 4096),
	}
}

// effectiveReadLoopWatchdog 返回 readLoop 应使用的 watchdog 超时：
// 字段非零则用字段值，否则回退到 DAQ_P_1064PRE_READ_LOOP_WATCHDOG_TIMEOUT 常量。
//
// 暴露为字段供测试覆盖：生产代码保持默认 10s，测试可缩短到 100ms 加速用例。
func (d *DAQP1064Pre) effectiveReadLoopWatchdog() time.Duration {
	if d.readLoopWatchdog > 0 {
		return d.readLoopWatchdog
	}
	return DAQ_P_1064PRE_READ_LOOP_WATCHDOG_TIMEOUT
}

// SetOnError 实现 ports.ErrorNotifiable：DeviceManager 在 Connect 阶段注入回调。
func (d *DAQP1064Pre) SetOnError(fn func(err error)) {
	d.mu.Lock()
	d.onError = fn
	d.mu.Unlock()
}

func (d *DAQP1064Pre) ID() string { return d.profile.ID }

func (d *DAQP1064Pre) Connect() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.conn != nil {
		return nil
	}

	host := d.profile.Address
	if host == "" {
		host = DAQ_P_1064PRE_DEFAULT_HOST
	}
	port := d.profile.Port
	if port <= 0 {
		port = DAQ_P_1064PRE_DEFAULT_PORT
	}
	// ADR-009 R0-7：net.DialTimeout 依赖 Dial 内部 deadline，Windows 故障机器
	// deadline 不可靠时 Dial 可能永远不返回，前端"连接中"状态无法翻转。
	// 改用 sharedproto.DialTCP（goroutine + time.After 软超时 + abandoned 信号），
	// 主线程在 timeout 后立即返回错误，不依赖 Dial 兑现 deadline。
	conn, err := sharedproto.DialTCP(fmt.Sprintf("%s:%d", host, port), "", DAQ_P_1064PRE_TIMEOUT)
	if err != nil {
		return fmt.Errorf("connect to %s:%d: %w", host, port, err)
	}
	slog.Info("DAQ-P-1064Pre TCP connected", "category", "hardware-recv", "component", "hardware", "device", d.profile.ID, "address", host, "port", port)

	d.conn = conn
	d.status.Connection = device.ConnectionConnected
	return nil
}

func (d *DAQP1064Pre) Disconnect() error {
	// 标记主动停止：readLoop 检测到此原因后静默退出，不触发 onError 误报。
	d.SetStopReason(sharedproto.StopReasonUserRequested)

	d.mu.Lock()
	_ = d.stopAcquisitionLocked()
	done := d.readLoopDone
	conn := d.conn
	d.conn = nil
	d.status.Connection = device.ConnectionDisconnected
	d.mu.Unlock()

	// 等待 readLoop 退出后再关闭连接，避免 Read 与 Close 并发。
	// I-4 修复：join 超时与 StopAcquisition 行为对齐——直接修改 status=Error
	// 并触发 onError 让 DeviceManager 感知连接已异常，避免 readLoop 卡死场景下静默泄漏 goroutine。
	//
	// 关键点：本函数前面已置 d.conn=nil，无法复用 invalidateConnection(expectedConn) ——
	// 该函数在 d.conn != expectedConn 时（此处 d.conn=nil != expectedConn=conn）会跳过状态修改
	// 仅关闭旧 conn（finding 2 修复引入的比较语义）。
	// 但 Disconnect 是连接生命周期的 owner，自己置 nil 后需要主动把 status 改为 Error
	// 让上层感知异常。此处直接持锁修改 status + 调 onError，与 invalidate 行为对齐。
	//
	// 必须先 Close conn 让 readLoop 的 Read 返回 closed 错误后退出，否则 readLoop 会永久
	// 阻塞在内核 Read 上（特别是 deadlineIgnoringConn 场景：deadline 被屏蔽，
	// 唯一解除阻塞的方式就是 Close conn）。
	if done != nil {
		select {
		case <-done:
		case <-time.After(sharedproto.ReadLoopJoinTimeout):
			slog.Warn("DAQ-P-1064Pre readLoop join timeout on disconnect", "device", d.profile.ID)
			if conn != nil {
				// LSP 环境挂起 Read 时 Close 可能永久阻塞（closesocket 等待未完成
				// 重叠读取）：后台执行，调用线程不等，避免 Disconnect 卡死。
				go sharedproto.AbortConnection(conn)
			}
			// ADR-009 finding 2：Disconnect 已自行置 d.conn=nil，invalidateConnection 的
			// expectedConn 比较会因 nil != expectedConn 跳过状态修改。此处由 Disconnect
			// 直接持锁修改 status=Error + 调 onError，复用 invalidate 的清理语义但避开
			// expectedConn 比较的副作用。
			d.mu.Lock()
			d.status.Connection = device.ConnectionError
			d.status.LastError = "read loop did not exit on disconnect; reconnect required"
			fn := d.onError
			d.mu.Unlock()
			if fn != nil {
				fn(fmt.Errorf("read loop did not exit on disconnect; reconnect required"))
			}
			return nil
		}
	}

	if conn != nil {
		// 正常路径 readLoop 已退出，无挂起 Read，Close 应即时返回；
		// 仍放后台兜底 LSP 环境异常（Close 卡死不阻塞 Disconnect）。
		go sharedproto.AbortConnection(conn)
	}
	slog.Info("DAQ-P-1064Pre TCP disconnected", "category", "hardware-recv", "component", "hardware", "device", d.profile.ID)
	return nil
}

func (d *DAQP1064Pre) StartAcquisition() error {
	// 清理上一轮采集的 StopReason，确保本次异常退出时 onError 能正常触发。
	d.ClearStopReason()
	slog.Info("1064Pre: StartAcquisition called", "device", d.profile.Name, "id", d.profile.ID)

	d.mu.Lock()
	if d.acquiring {
		d.mu.Unlock()
		slog.Info("1064Pre: already acquiring, skip", "device", d.profile.Name)
		return nil
	}
	if d.conn == nil {
		d.mu.Unlock()
		slog.Error("1064Pre: StartAcquisition failed - not connected", "device", d.profile.Name)
		return fmt.Errorf("device not connected")
	}

	// 等待上一次 readLoop 完全退出，避免旧 goroutine 与新采集竞争 conn。
	// ADR-009：close(stop) 不保证 readLoop 立即退出（Read 可能仍在内核阻塞），
	// 若直接启动新 readLoop 会形成双 reader，TCP 字节随机分配导致数据错位/丢失。
	if d.readLoopDone != nil {
		done := d.readLoopDone
		// ADR-009 finding 2：捕获 expectedConn，join 超时后仅关闭此 conn，
		// 避免与并发 Disconnect -> Connect 的新连接误杀。
		expectedConn := d.conn
		d.mu.Unlock()
		select {
		case <-done:
		case <-time.After(sharedproto.ReadLoopJoinTimeout):
			d.invalidateConnectionAfterReadLoopTimeout(expectedConn, "previous read loop did not exit; reconnect required")
			return fmt.Errorf("previous read loop did not exit; reconnect required")
		}
		d.mu.Lock()
		// 等待期间连接可能已被其他路径 invalidate 置 nil，需重新检查
		if d.conn == nil {
			d.mu.Unlock()
			return fmt.Errorf("device not connected")
		}
	}

	// 发送启动采集命令（CMD_ACQUISITION_CTRL = 0x10）
	// 设备收到此命令后开始推送 72B 采集数据帧，无需等待响应帧
	invalidateNeeded, err := d.sendStartAcquisitionLocked()
	if err != nil {
		// ADR-009 finding 2：捕获 expectedConn，watchdog 触发后仅关闭此 conn。
		expectedConn := d.conn
		d.mu.Unlock()
		// P1-3.b 修复：watchdog 触发时 sendStartAcquisitionLocked 返回 invalidateNeeded=true，
		// 此处已释放 d.mu 锁，可安全调用 invalidateConnection 完成 Close conn + onError。
		// 不在 sendStartAcquisitionLocked 内部调用是因为它是 *Locked 方法（持锁），
		// invalidateConnection 内部 d.mu.Lock() 会自死锁（sync.RWMutex 不可重入）。
		if invalidateNeeded {
			d.invalidateConnection(expectedConn, fmt.Sprintf("sendStartAcquisition watchdog triggered: %v", err))
		}
		return fmt.Errorf("start acquisition command failed: %w", err)
	}

	d.acquiring = true
	d.status.Acquiring = true
	d.status.Connection = device.ConnectionAcquiring
	d.stop = make(chan struct{})
	d.readLoopDone = make(chan struct{})
	stop := d.stop
	d.mu.Unlock()

	go d.readLoop(stop)
	return nil
}

func (d *DAQP1064Pre) StopAcquisition() error {
	// 标记主动停止：readLoop 检测到此原因后静默退出，不触发 onError 误报。
	d.SetStopReason(sharedproto.StopReasonUserRequested)

	d.mu.Lock()
	_ = d.stopAcquisitionLocked()
	done := d.readLoopDone
	// ADR-009 finding 2：捕获 expectedConn，join 超时后仅关闭此 conn，
	// 避免与并发 Disconnect -> Connect 的新连接误杀。
	expectedConn := d.conn
	d.mu.Unlock()

	// 等待 readLoop 退出，超时强制 invalidate（Close conn + 调 onError）。
	// ADR-009：close(stop) 无法解除内核 Read 阻塞，必须 Close conn 兜底。
	// Stop 是显式停止采集，连接已不可信，需通知 DeviceManager 重连。
	if done != nil {
		select {
		case <-done:
		case <-time.After(sharedproto.ReadLoopJoinTimeout):
			d.invalidateConnectionAfterReadLoopTimeout(expectedConn, "read loop did not exit after stop; reconnect required")
			return fmt.Errorf("read loop did not exit after stop; reconnect required")
		}
	}
	return nil
}

func (d *DAQP1064Pre) stopAcquisitionLocked() error {
	if d.acquiring && d.stop != nil {
		close(d.stop)
	}
	d.acquiring = false
	d.stop = nil
	d.status.Acquiring = false
	if d.status.Connection == device.ConnectionAcquiring {
		d.status.Connection = device.ConnectionConnected
	}
	return nil
}

// invalidateConnection 置连接为 Error 状态并通知上层（onError）。
//
// 使用场景（ADR-009 决策 3、7：watchdog/兜底 Close 后 conn 失效，必须置 nil + 通知上层）：
//   - sendCommand/sendStartAcquisitionLocked watchdog 触发（Write/Read 永久阻塞后强制 Close conn）
//   - readLoop join 超时（readLoop 卡死，Stop/Start 等不到 done 信号）
//
// 调用方语义：连接已不可用，必须重新 Connect 才能恢复。Close 在锁外执行，
// 避免在 mu.Lock 内做 I/O 与 readLoop 的 Read 竞争。
//
// ADR-009 finding 2 修复：expectedConn 比较避免误杀新连接。
// 调用方必须在触发故障前捕获 d.conn（通常在 RLock 后取引用），并传入此参数。
// 仅当 d.conn 仍是 expectedConn 时才清空 d.conn 并置 Error 状态；
// 若 d.conn 已被 Disconnect -> Connect 替换为新连接，仅关闭旧 expectedConn，
// 不修改状态、不通知 onError，避免旧命令的 invalidation 误杀新连接。
//
// 与 dsa3217.go invalidateConnection 标杆对齐，便于跨项目 grep 复用同一模式。
func (d *DAQP1064Pre) invalidateConnection(expectedConn net.Conn, message string) {
	d.mu.Lock()
	currentConn := d.conn
	if currentConn != expectedConn {
		// d.conn 已被替换为新连接（Disconnect -> Connect）或已置 nil，
		// 旧命令的 invalidation 不应误杀新连接。仅关闭旧 expectedConn，不修改状态。
		d.mu.Unlock()
		if expectedConn != nil {
			// 后台 detach：LSP 环境挂起 Read 时 Close 可能永久阻塞。
			go sharedproto.AbortConnection(expectedConn)
		}
		return
	}
	d.conn = nil
	d.acquiring = false
	d.status.Acquiring = false
	d.status.Connection = device.ConnectionError
	d.status.LastError = message
	fn := d.onError
	d.mu.Unlock()

	if expectedConn != nil {
		// 后台 detach：LSP 环境挂起 Read 时 Close 可能永久阻塞（同 t1603 模式）。
		go sharedproto.AbortConnection(expectedConn)
	}
	if fn != nil {
		fn(fmt.Errorf("%s", message))
	}
}

// invalidateConnectionAfterReadLoopTimeout 在 readLoop join 超时后强制作废连接。
//
// 触发场景：
//   - StopAcquisition：close(stop) 后 readLoop 仍在内核 Read 阻塞，1s 内未退出
//   - StartAcquisition：上一次 readLoop 仍未退出，新采集无法启动
//
// 仅是 invalidateConnection 的语义化别名：readLoop join 超时是 invalidate 的子场景，
// 单独保留命名便于在 Stop/Start 调用点表达"readLoop 卡死"语义，与 dsa3217.go 对齐。
//
// ADR-009 finding 2：同样接收 expectedConn，仅在 d.conn 仍是触发故障时的 conn 时
// 才置 Error 状态；若连接已被替换，仅关闭旧 conn 不修改状态。
func (d *DAQP1064Pre) invalidateConnectionAfterReadLoopTimeout(expectedConn net.Conn, message string) {
	d.invalidateConnection(expectedConn, message)
}

// sendStartAcquisitionLocked 发送启动采集命令（调用方已持有 d.mu 锁）
//
// 命令帧格式：A5 5A 0x10 len(2B BE) data(7B) checksum
// data 载荷按 1064Pre 协议规范构造：
//
//	data[0] = ACQ_ACTION_CONTINUOUS (2)  // 连续采集模式
//	data[1] = ACQ_DATA_MODE_CALIB (1)    // 工程单位（校准后数据）
//	data[2:4] = 采样周期（ms，LE uint16） // 1000 / SamplingRate(Hz)
//	data[4:6] = 通道使能（LE uint16）    // 0xFFFF = 全部通道
//	data[6] = 气象数据开关（1 = 包含）    // 含大气压/温度
//
// 返回值 invalidateNeeded 表示 watchdog 是否触发：true 时调用方必须在释放 d.mu 锁后
// 调用 d.invalidateConnection 完成 Close conn + onError 通知。
//
// 为什么不在本方法内直接调 invalidateConnection（P1-3.b 修复关键点）：
//   - 本方法是 *Locked 方法，调用方 StartAcquisition 持有 d.mu 锁
//   - invalidateConnection 内部 d.mu.Lock() 会与已持有的锁形成自死锁
//     （Go sync.RWMutex 不可重入）
//   - 因此本方法仅返回 invalidateNeeded 标志，由 StartAcquisition 在释放锁后
//     调用 invalidateConnection，与 sendCommand（不持锁）的调用模式保持一致
func (d *DAQP1064Pre) sendStartAcquisitionLocked() (invalidateNeeded bool, err error) {
	samplingRate := d.profile.SamplingRate
	if samplingRate <= 0 {
		samplingRate = 10 // 默认 10Hz
	}
	periodMs := uint16(1000 / samplingRate)

	data := make([]byte, 7)
	data[0] = ACQ_ACTION_CONTINUOUS
	data[1] = ACQ_DATA_MODE_CALIB
	binary.LittleEndian.PutUint16(data[2:4], periodMs)
	binary.LittleEndian.PutUint16(data[4:6], 0xFFFF)
	data[6] = 1

	frame := d.buildFrame(CMD_ACQUISITION_CTRL, data)

	slog.Info("1064Pre: sending acquisition start frame",
		"device", d.profile.Name,
		"hex", hex.EncodeToString(frame),
		"samplingRate", samplingRate, "periodMs", periodMs)

	// watchdog 兜底（ADR-009）：SetWriteDeadline 在某些 Windows 电脑不可靠，
	// Write 也可能因设备 TCP 接收窗口满而卡死。watchdog 在超时后强制 Close conn
	// 解除阻塞，避免 sendStartAcquisitionLocked 永久卡在 Write 上。
	//
	// 启动时机：在 Write 之前启动，覆盖 SetWriteDeadline + Write 全流程。
	// 超时值：用 DAQ_P_1064PRE_TIMEOUT（5s），与原 SetWriteDeadline 一致。
	wdStop := sharedproto.WatchdogClose(d.conn, DAQ_P_1064PRE_TIMEOUT)

	// defer LIFO 顺序（后注册先执行）：
	//   1. 先执行：wdStop() 停止 watchdog 计时器（必须在清 deadline 之前停止，
	//      避免清 deadline 时 watchdog 已触发导致 conn 被错误 Close）
	//   2. 后执行：清除 Write deadline，避免过期的绝对时间影响后续命令
	// 注意：watchdog 触发后 conn 已 Close，SetWriteDeadline 失败被忽略无害。
	// wdStop 通过 sync.Once 幂等，可被 defer 和错误路径多次调用。
	defer d.conn.SetWriteDeadline(time.Time{})
	defer func() { _ = wdStop() }()

	_ = d.conn.SetWriteDeadline(time.Now().Add(DAQ_P_1064PRE_TIMEOUT))
	_, err = d.conn.Write(frame)
	if err != nil {
		// Write 失败时附加 watchdog 上下文，便于排查是否 watchdog 触发导致 conn 已失效。
		wrappedErr := sharedproto.WrapWatchdogError(err, wdStop, "write start acquisition")
		slog.Error("1064Pre: send start acquisition command failed",
			"device", d.profile.Name, "error", wrappedErr)
		// P1-3.b 修复：watchdog 触发（wdStop 返回 false）时 conn 已被强制 Close，
		// 必须置 d.conn=nil + 调 onError 通知上层重连，否则下次 StartAcquisition
		// 通过 d.conn!=nil 检查但实际 conn 已失效，导致后续操作持续报错。
		// WrapWatchdogError 内部已调用 wdStop 一次，此处再次调用幂等（sync.Once 保护）。
		// 不在此调 invalidateConnection（持锁会死锁），返回标志由调用方在锁外执行。
		return !wdStop(), wrappedErr
	}
	slog.Info("1064Pre: acquisition start command sent", "device", d.profile.Name)
	return false, nil
}

func (d *DAQP1064Pre) SetDataSink(sink device.DataSink) {
	d.mu.Lock()
	d.sink = sink
	d.mu.Unlock()
}

func (d *DAQP1064Pre) Status() device.Status {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.status
}

func (d *DAQP1064Pre) SetUnit(unit string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	for i := range d.profile.Channels {
		d.profile.Channels[i].Unit = unit
	}
	return nil
}

func (d *DAQP1064Pre) SetTare(channelIndex int, offset float64) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if channelIndex < 0 || channelIndex >= len(d.profile.Channels) {
		return fmt.Errorf("invalid channel index: %d", channelIndex)
	}
	d.profile.Channels[channelIndex].TareOffset = offset
	return nil
}

func (d *DAQP1064Pre) GetTare(channelIndex int) (float64, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	if channelIndex < 0 || channelIndex >= len(d.profile.Channels) {
		return 0, fmt.Errorf("invalid channel index: %d", channelIndex)
	}
	return d.profile.Channels[channelIndex].TareOffset, nil
}

func (d *DAQP1064Pre) ClearTare(channelIndex int) error {
	return d.SetTare(channelIndex, 0)
}

func (d *DAQP1064Pre) readLoop(stop <-chan struct{}) {
	// ADR-009 R0-10：no-data owner 必须独立于 read goroutine 与其 mutex。
	//
	// 历史背景：原实现 readLoop 通过 SetReadDeadline(100ms) 让 Read 周期返回，
	// 依赖循环体执行检测无数据。问题 Windows 电脑 deadline 失效时循环体不可达，
	// 半开连接无法自行收敛（设备断电/网线脱落但 TCP 连接仍存活）。
	//
	// 整改后：readLoop 入口启动独立 time.AfterFunc timer，到期通过 expected conn 比较
	// 安全毒化连接（清空 conn、置 Error 状态、Close conn）。timer 不依赖 readLoop 循环体
	// 执行，即使 Read 永久阻塞也能到期触发。每次收到有效数据（n > 0）调用 Reset 续期；
	// readLoop 退出 defer Stop。
	//
	// 与 readLoop watchdog（effectiveReadLoopWatchdog，覆盖单次 Read 阻塞）的区别：
	//   - readLoop watchdog：每次 Read 必须在超时内完成，否则 Close conn
	//   - noDataTimer：超时内必须收到数据，否则 Close conn
	// 两者使用独立超时值（noDataTimeout 与 effectiveReadLoopWatchdog 默认均为 10s，
	// 但可独立配置），避免共享超时导致同时触发后 readLoopWatchdog 覆盖 noDataTimer
	// 设置的 LastError。readLoop watchdog 在循环内新建 wdStop 每次循环重置；
	// noDataTimer 在入口启动，收到数据时 Reset。
	//
	// 不调 onError：让 readLoop defer 的 unexpectedErr 路径统一调用 invalidateConnection
	// 通知上层，避免同一故障被 onError 回调两次。timer Close 后 readLoop 的 Read 返回
	// 错误进入 defer，defer 调用 invalidateConnection 统一清理（重置 status=Error、
	// 清空 conn、调 onError）。
	//
	// expected conn 比较避免与重连后的新 conn 竞争：Stop/Disconnect 已置 d.conn=nil
	// 或重连后 d.conn 是新连接，timer 触发时 d.conn != expectedConn 直接跳过。
	// acquiring 检查避免 Stop 后 readLoop 尚未退出时 timer 误触发：stopAcquisitionLocked
	// 先置 d.acquiring=false 再等 done，timer 检测到 acquiring=false 直接跳过。
	d.mu.RLock()
	expectedConn := d.conn
	d.mu.RUnlock()
	if expectedConn == nil {
		return
	}

	// 快照 noDataTimeout 到局部变量：测试会通过 setNoDataTimeout 覆盖加速（200ms），
	// t.Cleanup 在测试结束时恢复原值。若 timer 回调内读取全局变量，可能在与
	// t.Cleanup 写入并发时触发 data race。局部变量在 timer 创建时已固化，回调
	// 读取栈上变量无 race 风险。
	// ADR-009 finding 4：通过 getNoDataTimeout 读取 atomic 变量，确保 readLoop 跨测试
	// 边界读取与下一测试修改并发安全。
	noDataTimeoutSnapshot := getNoDataTimeout()
	noDataTimer := time.AfterFunc(noDataTimeoutSnapshot, func() {
		d.mu.Lock()
		if !d.acquiring {
			// 采集已停止（stopAcquisitionLocked 先置 acquiring=false 再等 done），
			// timer 不应毒化保留的连接，让 readLoop 正常退出即可。
			d.mu.Unlock()
			return
		}
		currentConn := d.conn
		if currentConn != expectedConn {
			// 连接已被替换（重连）或置 nil（Stop/Disconnect），跳过。
			d.mu.Unlock()
			return
		}
		// 统一毒化：清空 conn、置 Error 状态、保存 LastError。
		// 不调 onError，让 readLoop defer 统一通知（避免双重回调）。
		d.conn = nil
		d.acquiring = false
		d.status.Acquiring = false
		d.status.Connection = device.ConnectionError
		d.status.LastError = fmt.Sprintf("no data received for %v", noDataTimeoutSnapshot)
		d.mu.Unlock()

		// 锁外 Close expected conn 解除 readLoop 的 Read 阻塞。
		// 后台 detach：LSP 环境挂起 Read 时 Close 可能永久阻塞（卡死 timer 回调
		// 会让后续毒化路径无法继续）。
		if currentConn != nil {
			go sharedproto.AbortConnection(currentConn)
		}
		slog.Warn("DAQ-P-1064Pre no data timeout, conn closed by watchdog",
			"device", d.profile.ID, "duration", noDataTimeoutSnapshot)
	})
	// readLoop 退出时停止 timer，避免 timer 在 readLoop 已退出后误触发。
	// Stop 不等待已 firing 的回调完成，但回调内 acquiring + expected conn 双重检查
	// 能正确处理 readLoop 退出后 timer 才 fire 的场景。
	defer noDataTimer.Stop()

	var unexpectedErr error

	// defer 顺序（LIFO）：
	//   1. 后注册先执行：close(readLoopDone) 通知等待方 readLoop 已退出
	//   2. 先注册后执行：异常退出且非用户主动停止时调 invalidateConnection 统一毒化
	// 必须先 close(done) 再 invalidateConnection，确保 Stop/Disconnect 的 join 先收到信号。
	defer func() {
		if unexpectedErr != nil {
			// 主动停止场景（Stop/Disconnect 已设置 StopReason）不视为异常，
			// 避免 Close 触发 Read 错误时误调 onError 误报设备故障。
			if d.GetStopReason() == sharedproto.StopReasonUserRequested {
				return
			}
			// ADR-009 R0-11 扩展（与 P1604/T1603 标杆一致）：terminal read error 必须调用
			// invalidateConnection 统一毒化连接——清空 d.conn、close conn、置 Error 状态、
			// 保存 LastError、通知 onError。
			//
			// 历史背景：原 defer 手写清理（设 status=Disconnected、Close conn），未清空 conn
			// 也未设 Error。EOF/RST 后连接已死，下次 StartAcquisition 会用旧 conn 发命令爆
			// WSAECONNABORTED。R0-10 整改时附带改造为统一毒化路径，保证 noDataTimer 触发后
			// 状态正确（Error，不是 Disconnected）。
			//
			// invalidateConnection 不清空 d.stop（它是采集生命周期字段，非连接状态），
			// 此处显式清空对齐原 defer 行为，避免 readLoop 退出后 stop channel 残留。
			slog.Warn("DAQ-P-1064Pre read loop exited unexpectedly",
				"device", d.profile.ID, "error", unexpectedErr)
			d.mu.Lock()
			d.stop = nil
			d.mu.Unlock()
			// ADR-009 finding 2：传入 readLoop 启动时捕获的 expectedConn，
			// 避免与并发 Disconnect -> Connect 的新连接误杀。
			d.invalidateConnection(expectedConn, unexpectedErr.Error())
		}
	}()

	defer func() {
		d.mu.Lock()
		done := d.readLoopDone
		d.mu.Unlock()
		if done != nil {
			close(done)
		}
	}()

	// 缓存 conn 引用，避免 readLoop 运行期间 Disconnect 置 d.conn=nil 后下一次迭代 nil panic。
	conn := expectedConn

	buf := make([]byte, 4096)
	for {
		select {
		case <-stop:
			// 用户主动停止（StopAcquisition/Disconnect），不触发 onError
			return
		default:
		}

		// ADR-009 R0-6 整改：watchdog 必须在 ioMu.Lock 之前启动。
		//
		// 原实现：readLoop 仅设 SetReadDeadline(100ms) 后直接 Read，与 sendCommand
		// 可同时 Read 同一 conn（双 reader），TCP 字节随机分配导致数据错位。
		//
		// 整改后：readLoop 持 ioMu 期间 Read，sendCommand 也持 ioMu 期间 Write+Read，
		// 任意时刻只有一个 reader。但若 readLoop 持 ioMu 阻塞在失效 deadline 的 Read 上，
		// sendCommand 会永久等 ioMu。watchdog 在超时后强制 Close conn，解除 Read 阻塞
		// 并释放 ioMu，避免死锁。
		//
		// watchdog 超时取 10s（远大于 100ms deadline），仅在 deadline 失效极端场景触发。
		wdStop := sharedproto.WatchdogClose(conn, d.effectiveReadLoopWatchdog())

		d.ioMu.Lock()
		_ = conn.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
		n, err := conn.Read(buf)
		d.ioMu.Unlock()

		watchdogTriggered := !wdStop()

		// 主动停止后 conn 被 Close 触发的 Read 错误属于预期，静默退出。
		// 此分支覆盖 StopAcquisition/Disconnect close(stop) 后由 invalidateConnection
		// 强制 Close conn 触发的 Read 错误，避免误调 onError。
		select {
		case <-stop:
			return
		default:
		}

		if watchdogTriggered {
			// readLoop watchdog 触发：conn 已被 Close，readLoop 必须退出。
			// 不调用 invalidateConnection（会自死锁，因为 readLoop 持有的资源在 defer 中清理），
			// 而是设置 unexpectedErr 让 defer 路径统一处理（Close conn + onError）。
			unexpectedErr = fmt.Errorf("read loop watchdog triggered: Read blocked and conn closed by watchdog")
			slog.Warn("DAQ-P-1064Pre read loop watchdog triggered", "device", d.profile.ID)
			return
		}

		if err != nil {
			// 读取超时不视为异常，继续等待
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				continue
			}
			// 主动停止后连接被关闭属于预期行为，静默退出。
			// Stop/Disconnect 在 close(stop) 之前已 SetStopReason，
			// 且 invalidate/Disconnect 会 Close conn 触发 Read 返回 closed 错误。
			if d.GetStopReason() == sharedproto.StopReasonUserRequested && sharedproto.IsClosedConnError(err) {
				return
			}
			// conn 被外部路径 Close（sendCommand watchdog / invalidate / Disconnect）
			// 触发的 Read 错误属预期，静默退出。onError 已由 invalidate 路径统一负责，
			// 此处再调 onError 会导致同一故障被 DeviceManager 误判为两次。
			if sharedproto.IsClosedConnError(err) {
				slog.Debug("DAQ-P-1064Pre read loop exiting silently (conn closed by external path)",
					"device", d.profile.ID, "error", err)
				return
			}
			slog.Debug("DAQ-P-1064Pre read error", "device", d.profile.ID, "error", err)
			unexpectedErr = err
			return
		}
		if n > 0 {
			// 收到数据，续期 no-data timer。
			// Reset 是原子操作，无需加锁；即使 timer 已 fire 也能安全 Reset（time.AfterFunc 文档保证）。
			// 用 n > 0 而非"有效帧"作为续期信号：表示 TCP 连接仍在传输字节，即使部分帧或非采集帧
			// 也说明对端活跃；只有完全无字节到达才判定为连接异常。
			// 使用入口快照的 noDataTimeoutSnapshot 而非全局 noDataTimeout，避免与
			// 测试 t.Cleanup 写入全局变量并发 race。
			noDataTimer.Reset(noDataTimeoutSnapshot)
			d.processData(buf[:n])
		}
	}
}

func (d *DAQP1064Pre) processData(data []byte) {
	d.mu.Lock()
	d.recvBuffer = append(d.recvBuffer, data...)

	if !d.firstDataLogged && len(d.recvBuffer) > 0 {
		d.firstDataLogged = true
		dumpLen := len(d.recvBuffer)
		if dumpLen > 128 {
			dumpLen = 128
		}
		slog.Info("1064Pre: first data received after start",
			"device", d.profile.Name,
			"totalBytes", len(d.recvBuffer),
			"hex", hex.EncodeToString(d.recvBuffer[:dumpLen]))
	}

	for len(d.recvBuffer) >= 6 {
		if d.recvBuffer[0] != 0xA5 || d.recvBuffer[1] != 0x5A {
			if len(d.recvBuffer) >= 2 {
				slog.Debug("1064Pre: bad frame header, resyncing",
					"device", d.profile.Name, "buf0", d.recvBuffer[0], "buf1", d.recvBuffer[1],
					"buf[0:2] hex", hex.EncodeToString(d.recvBuffer[:2]))
			}
			d.recvBuffer = d.recvBuffer[1:]
			continue
		}

		cmd := d.recvBuffer[2]
		dataLen := int(d.recvBuffer[3])<<8 | int(d.recvBuffer[4])
		frameLen := 2 + 1 + 2 + dataLen + 1

		if len(d.recvBuffer) < frameLen {
			break
		}

		payload := make([]byte, dataLen)
		copy(payload, d.recvBuffer[5:5+dataLen])

		expectedChecksum := d.recvBuffer[5+dataLen]
		checksum := d.calculateChecksum(d.recvBuffer[:5+dataLen])
		if checksum != expectedChecksum {
			slog.Debug("DAQ-P-1064Pre checksum mismatch", "device", d.profile.ID)
		}

		if cmd == CMD_ACQUISITION_CTRL && d.acquiring && dataLen == 72 {
			slog.Debug("1064Pre: received valid data frame",
				"device", d.profile.Name, "dataLen", dataLen)
			d.handleAcquisitionDataLocked(payload)
		} else {
			if d.acquiring {
				slog.Debug("1064Pre: non-data frame ignored",
					"device", d.profile.Name, "cmd", cmd, "dataLen", dataLen, "expectedCmd", CMD_ACQUISITION_CTRL, "expectedLen", 72)
			}
		}

		d.recvBuffer = d.recvBuffer[frameLen:]
	}
	d.mu.Unlock()
}

func (d *DAQP1064Pre) calculateChecksum(data []byte) byte {
	var sum byte
	for _, b := range data {
		sum += b
	}
	return sum
}

// handleAcquisitionDataLocked 在调用方已持有 d.mu.Lock 的前提下运行。
// 不再内部 RLock，避免与父 processData 的 Lock 形成自死锁。
//
// 1604Pre 数据帧 payload 布局（72 字节）：
//
//	[0..3]  大气压（float32 LE，单位 Pa）
//	[4..7]  大气温度（float32 LE，单位 °C）
//	[8..71] 16 路压力（16×float32 LE，单位 Pa）
//
// 通道映射（与 defaultDAQP1604PreChannels 一致）：
//
//	Index 0..15 → payload[8+i*4]  压力通道
//	Index 16    → payload[0..3]   大气压
//	Index 17    → payload[4..7]   大气温度
//
// 兼容性：若 profile.Channels 仅 16 通道（历史 profile），不读气象数据，不影响功能。
func (d *DAQP1064Pre) handleAcquisitionDataLocked(payload []byte) {
	if len(payload) < 72 {
		return
	}

	// 调用方持锁，直接读取 sink/channels
	sink := d.sink
	channels := d.profile.Channels
	deviceID := d.profile.ID

	if sink == nil {
		return
	}

	values := make([]float64, 0, len(channels))
	indices := make([]int, 0, len(channels))

	for i := 0; i < len(channels); i++ {
		if !channels[i].Enabled {
			continue
		}
		var val float64
		switch channels[i].Index {
		case device.P1604PreAtmChannelIndex: // 大气压
			val = float64(readFloat32LE(payload, 0))
		case device.P1604PreAtmTempChannelIndex: // 大气温度
			val = float64(readFloat32LE(payload, 4))
		default: // 16 路压力通道（Index 0..15）
			if channels[i].Index >= 0 && channels[i].Index < device.P1604PrePressureChannelCount {
				val = float64(readFloat32LE(payload, 8+channels[i].Index*4))
			}
		}
		indices = append(indices, channels[i].Index)
		values = append(values, val)
	}

	d.streamFrameSeq++
	// sink 是 device.DataSink 函数，通常是非阻塞发送到 channel；
	// 但仍建议避免在持锁状态下长时间执行——目前实现仅做指针拷贝，可接受。
	sink(device.DataPayload{
		DeviceID:       deviceID,
		DeviceType:     d.profile.Type,
		DeviceName:     d.profile.Name,
		Timestamp:      device.NowMs(),
		Channels:       values,
		ChannelIndices: indices,
	})
}

func readFloat32LE(data []byte, offset int) float32 {
	if offset+4 > len(data) {
		return 0
	}
	return math.Float32frombits(binary.LittleEndian.Uint32(data[offset : offset+4]))
}

func (d *DAQP1064Pre) ReadCalibration(channel int) (*CalibrationParams, error) {
	if channel < 0 || channel > 15 {
		return nil, fmt.Errorf("invalid channel: %d", channel)
	}

	resp, err := d.sendCommand(CMD_READ_CALIBRATION, []byte{byte(channel)}, 2000)
	if err != nil {
		return nil, err
	}
	if len(resp) < 10 || resp[0] == 0xFF {
		return nil, fmt.Errorf("read calibration failed")
	}

	return &CalibrationParams{
		Channel: int(resp[1]),
		B:       readFloat32LE(resp, 2),
		K1:      readFloat32LE(resp, 6),
	}, nil
}

func (d *DAQP1064Pre) WriteCalibration(channel int, b, k1 float32) error {
	if channel < 0 || channel > 15 {
		return fmt.Errorf("invalid channel: %d", channel)
	}

	data := make([]byte, 29)
	data[0] = byte(channel)
	writeFloat32LE(data[1:5], b)
	writeFloat32LE(data[5:9], k1)

	resp, err := d.sendCommand(CMD_WRITE_CALIBRATION, data, 2000)
	if err != nil {
		return err
	}
	if len(resp) < 2 || resp[0] != 0x00 {
		return fmt.Errorf("write calibration failed")
	}
	return nil
}

func (d *DAQP1064Pre) TareChannel(channel int, currentValue float64) error {
	return d.WriteCalibration(channel, float32(currentValue), 1.0)
}

func (d *DAQP1064Pre) sendCommand(cmd byte, data []byte, timeoutMs int) ([]byte, error) {
	// 先用 RLock 取 conn 引用，再启 watchdog，最后 ioMu.Lock。
	//
	// 顺序不能反（ADR-009 R0-6）：若先 ioMu.Lock 再启 watchdog，readLoop 持 ioMu 阻塞
	// Read（deadline 失效）时 sendCommand 会卡在 ioMu.Lock 上，watchdog 永远不启动，
	// 形成死锁。watchdog 在 ioMu.Lock 之前启动，覆盖"等 ioMu + Write + Read"全流程，
	// 触发后 Close conn 解除 readLoop 阻塞，readLoop 释放 ioMu 后本函数才能继续。
	d.mu.RLock()
	conn := d.conn
	d.mu.RUnlock()

	if conn == nil {
		return nil, fmt.Errorf("not connected")
	}
	// 防御 timeoutMs<=0：WatchdogClose(0) 会触发 time.AfterFunc(0) 立即执行 Close，
	// 导致后续 Read 立即失败。当前调用方都传 2000（2s），此处防御防止未来误用。
	if timeoutMs <= 0 {
		return nil, fmt.Errorf("invalid timeoutMs: %d", timeoutMs)
	}

	// watchdog 兜底（ADR-009 R0-6）：覆盖"等 ioMu + Write + Read"全流程。
	//
	// 启动时机：在 ioMu.Lock 之前启动。若 readLoop 持 ioMu 阻塞在失效 deadline 的 Read 上，
	// sendCommand 会卡在 ioMu.Lock 上。watchdog 超时后强制 Close conn，解除 readLoop
	// 阻塞并释放 ioMu，sendCommand 才能拿到 ioMu 并发现 conn 已失效。
	//
	// 超时值：用 2 倍 timeoutMs 保持原代码 Write + Read 总耗时行为，
	// 避免引入总超时回归。timeoutMs 仍作为单次 Read 的 deadline。
	timeout := time.Duration(timeoutMs) * time.Millisecond
	wdStop := sharedproto.WatchdogClose(conn, DAQ_P_1064PRE_CMD_TIMEOUT_FACTOR*timeout)

	d.ioMu.Lock()
	defer d.ioMu.Unlock()

	// defer LIFO 顺序（后注册先执行）：
	//   1. 先执行 wdStop() 停止 watchdog 计时器
	//   2. 后执行：清除 Read/Write deadline，避免过期的绝对时间影响后续命令
	// 注意：watchdog 触发后 conn 已 Close，SetReadDeadline/SetWriteDeadline 失败被忽略无害。
	defer func() {
		_ = conn.SetReadDeadline(time.Time{})
		_ = conn.SetWriteDeadline(time.Time{})
	}()
	defer func() { _ = wdStop() }()

	frame := d.buildFrame(cmd, data)
	// 命令收发用 Info 级别：ring buffer 透传，stderr / file 由 CategorySkipHandler 跳过。
	slog.Info("DAQ-P-1064Pre command send", "category", "hardware-send", "component", "hardware", "device", d.profile.ID, "command", fmt.Sprintf("0x%02X", cmd), "bytes", len(frame))
	if _, err := conn.Write(frame); err != nil {
		// ADR-009 finding 3：Write 任何错误（timeout、broken pipe、RST 等）都意味着
		// 协议边界不可信。强制 Close conn 并毒化连接，避免下次 sendCommand 复用已死 conn。
		// 后台 detach：LSP 环境挂起 Write 时 Close 可能永久阻塞。
		go sharedproto.AbortConnection(conn)
		d.invalidateConnection(conn, fmt.Sprintf("sendCommand write error: %v", err))
		return nil, fmt.Errorf("write command: %w; %w", err, sharedproto.ErrWatchdogTriggered)
	}

	conn.SetReadDeadline(time.Now().Add(timeout))
	respData, err := d.readResponseFrame(conn, wdStop, cmd)
	if err != nil {
		// ADR-009 finding 3：readResponseFrame 在任何协议边界错误时已 Close conn 并
		// 包装 ErrWatchdogTriggered sentinel。检测到 sentinel 即毒化连接
		// （清空 d.conn、置 Error 状态、调 onError），让上层重连。
		if sharedproto.IsWatchdogTriggered(err) {
			d.invalidateConnection(conn, fmt.Sprintf("sendCommand read error: %v", err))
		}
		return nil, err
	}

	slog.Info("DAQ-P-1064Pre command response", "category", "hardware-recv", "component", "hardware", "device", d.profile.ID, "command", fmt.Sprintf("0x%02X", cmd), "bytes", len(respData))
	return respData, nil
}

// readResponseFrame 读取设备返回的响应帧（5 字节 header + payload + 1 字节 checksum）并校验。
//
// 协议帧布局（与 buildFrame 一致）：
//
//	[0xA5, 0x5A, cmd, dataLenHi, dataLenLo, payload[dataLen]..., checksum]
//
// 抽取为独立方法以满足函数行数硬约束（CLAUDE.md ≤50 行/函数）。
// wdStop 用于在 Read 失败时附加 watchdog 上下文，便于排查是否 watchdog 触发。
// expectedCmd 是请求帧的 cmd 字节，用于校验响应帧是否匹配本次请求。
//
// ADR-009 finding 3 整改：任何协议边界错误（io.ReadFull EOF/短读、invalid header
// magic、cmd mismatch、response too long、checksum mismatch 等）都强制 Close conn
// 并包装 ErrWatchdogTriggered sentinel。迟到响应可能随后进入 TCP 流被下一条命令
// 消费导致协议错位，必须阻断。调用方通过 IsWatchdogTriggered 统一触发 invalidateConnection。
//
// ADR-009 复核修订 finding 1 修复：原实现错误地读取 6 字节作为 header，多读的 1 字节
// 实际是 payload 首字节（当 dataLen > 0 时），导致后续 io.ReadFull(respData) 读到的
// 是 payload[1:] + checksum，payload 首字节丢失、末尾混入 checksum。整改为：
// 5 字节 header → dataLen 字节 payload → 1 字节 checksum → 校验 checksum。
//
// 使用 io.ReadFull 保证完整读取：
// Go 的 conn.Read 协议只保证返回 1-N 字节（N = len(buf)），可能只读到部分数据。
// io.ReadFull 内部循环 Read 直到填满 buffer 或遇到错误。
func (d *DAQP1064Pre) readResponseFrame(conn net.Conn, wdStop func() bool, expectedCmd byte) ([]byte, error) {
	// 5 字节 header：magic(2) + cmd(1) + dataLen(2)
	header := make([]byte, 5)
	if _, err := io.ReadFull(conn, header); err != nil {
		// ADR-009 finding 3：header 读取失败（EOF、短读、timeout 等）意味着
		// 协议边界不可信。强制 Close conn 阻断迟到响应，包装 sentinel 让调用方毒化。
		go sharedproto.AbortConnection(conn)
		return nil, fmt.Errorf("read response header: %w; %w", err, sharedproto.ErrWatchdogTriggered)
	}

	if header[0] != 0xA5 || header[1] != 0x5A {
		// ADR-009 finding 3：header magic 不匹配说明协议已错位（上一命令迟到响应、
		// 采集帧串入、或帧错位）。强制 Close conn 阻断。
		go sharedproto.AbortConnection(conn)
		return nil, fmt.Errorf("invalid response header; %w", sharedproto.ErrWatchdogTriggered)
	}

	// ADR-009 finding 3：响应 cmd 必须与请求 cmd 一致。
	// 不一致说明协议边界已不可信，必须 Close conn 阻断。
	if header[2] != expectedCmd {
		go sharedproto.AbortConnection(conn)
		return nil, fmt.Errorf("%w: expected 0x%02X, got 0x%02X; %w", ErrResponseCmdMismatch, expectedCmd, header[2], sharedproto.ErrWatchdogTriggered)
	}

	dataLen := int(header[3])<<8 | int(header[4])
	if dataLen > 4096-6 {
		// ADR-009 finding 3：响应长度异常，协议已错位。强制 Close conn 阻断。
		go sharedproto.AbortConnection(conn)
		return nil, fmt.Errorf("response too long; %w", sharedproto.ErrWatchdogTriggered)
	}

	// dataLen 字节 payload
	respData := make([]byte, dataLen)
	if _, err := io.ReadFull(conn, respData); err != nil {
		// ADR-009 finding 3：body 读取失败（EOF、短读、timeout 等）意味着
		// 协议边界不可信。强制 Close conn 阻断迟到响应。
		go sharedproto.AbortConnection(conn)
		return nil, fmt.Errorf("read response body: %w; %w", err, sharedproto.ErrWatchdogTriggered)
	}

	// 1 字节 checksum（复核修订 finding 1：原实现漏读 checksum，导致下一帧错位）
	var checksumByte [1]byte
	if _, err := io.ReadFull(conn, checksumByte[:]); err != nil {
		go sharedproto.AbortConnection(conn)
		return nil, fmt.Errorf("read response checksum: %w; %w", err, sharedproto.ErrWatchdogTriggered)
	}
	// 校验 checksum：与 buildFrame 一致，对 header + payload 求和
	expectedChecksum := d.calculateChecksum(append(header, respData...))
	if checksumByte[0] != expectedChecksum {
		// checksum 不匹配说明协议边界已不可信（数据损坏、帧错位等），必须 Close conn 阻断。
		go sharedproto.AbortConnection(conn)
		return nil, fmt.Errorf("checksum mismatch: expected 0x%02X, got 0x%02X; %w", expectedChecksum, checksumByte[0], sharedproto.ErrWatchdogTriggered)
	}
	return respData, nil
}

// ErrResponseCmdMismatch 是响应帧 cmd 与请求 cmd 不匹配时的 sentinel 错误。
// readResponseFrame 在 cmd 不匹配时包装此 sentinel + ErrWatchdogTriggered，
// 调用方通过 IsWatchdogTriggered 统一触发 invalidateConnection（ADR-009 finding 3）。
var ErrResponseCmdMismatch = errors.New("response cmd mismatch")

func (d *DAQP1064Pre) buildFrame(cmd byte, data []byte) []byte {
	frame := make([]byte, 0, 6+len(data))
	frame = append(frame, 0xA5, 0x5A, cmd)
	frame = append(frame, byte(len(data)>>8), byte(len(data)&0xFF))
	frame = append(frame, data...)
	frame = append(frame, d.calculateChecksum(frame))
	return frame
}

func writeFloat32LE(data []byte, v float32) {
	binary.LittleEndian.PutUint32(data, math.Float32bits(v))
}
