package hardware

import (
	"fmt"
	"log/slog"
	"net"
	"strings"
	"sync"
	"time"

	sharedproto "shared.local/device-sdk/go/protocol"
	"wind-daq/services/api-go/internal/core/device"
	"wind-daq/services/api-go/internal/ports"
)

const (
	DAQ_P_1604_DEFAULT_HOST = "192.168.3.101"
	DAQ_P_1604_DEFAULT_PORT = 9000
	DAQ_P_1604_TIMEOUT      = 5 * time.Second

	// maxConsecutiveFrameErrors 是触发自动重同步的连续帧错误阈值。
	maxConsecutiveFrameErrors = 5
	// noDataTimeout 是允许的最长无数据时间，超过则判定为连接异常。
	noDataTimeout = 10 * time.Second
	// readLoop 等待超时使用 sharedproto.ReadLoopJoinTimeout（跨项目统一）
	// 主动停止原因使用 sharedproto.StopReasonUserRequested（跨项目统一）
)

type DAQP1604 struct {
	mu          sync.RWMutex
	writeMu     sync.Mutex
	profile     device.Profile
	status      device.Status
	sink        device.DataSink
	stop        chan struct{}
	acquiring   bool
	conn        net.Conn
	frameReader *sharedproto.FrameReader
	readErrors  int
	frameErrors int
	// consecutiveFrameErrors 连续帧解析错误计数，用于触发自动重同步。
	consecutiveFrameErrors int
	onError                func(err error)

	// readLoopDone 由 readLoop 退出时关闭，供 Start/Stop/Disconnect 等待协程结束。
	readLoopDone chan struct{}
	// 主动停止原因追踪：嵌入 sharedproto.StopReasonTracker
	// 提供 SetStopReason / GetStopReason / ClearStopReason 方法，跨项目复用
	sharedproto.StopReasonTracker
}

func NewDAQP1604(profile device.Profile) *DAQP1604 {
	return &DAQP1604{
		profile: profile,
		status: device.Status{
			ID:         profile.ID,
			Name:       profile.Name,
			Type:       profile.Type,
			Connection: device.ConnectionDisconnected,
		},
	}
}

// 编译时接口检查
var _ ports.Device = (*DAQP1604)(nil)
var _ ports.TareConfigurable = (*DAQP1604)(nil)
var _ ports.ErrorNotifiable = (*DAQP1604)(nil)

// SetOnError 设置设备异常退出回调，实现 ports.ErrorNotifiable 接口
func (d *DAQP1604) SetOnError(fn func(err error)) {
	d.mu.Lock()
	d.onError = fn
	d.mu.Unlock()
}

func (d *DAQP1604) ID() string { return d.profile.ID }

func runDAQP1604Handshake(conn net.Conn, timeout time.Duration, handshake func() error) error {
	timedOut := make(chan struct{})
	timer := time.AfterFunc(timeout, func() {
		_ = conn.Close()
		close(timedOut)
	})

	err := handshake()
	if timer.Stop() {
		return err
	}
	<-timedOut
	return fmt.Errorf("connection handshake timed out after %s", timeout)
}

func (d *DAQP1604) Connect() error {
	d.mu.Lock()
	if d.conn != nil {
		d.mu.Unlock()
		return nil
	}
	d.mu.Unlock()

	host := d.profile.Address
	if host == "" {
		host = DAQ_P_1604_DEFAULT_HOST
	}
	port := d.profile.Port
	if port <= 0 {
		port = DAQ_P_1604_DEFAULT_PORT
	}

	conn, err := sharedproto.DialTCP(fmt.Sprintf("%s:%d", host, port), d.profile.LocalAddress, DAQ_P_1604_TIMEOUT)
	if err != nil {
		return fmt.Errorf("connect to %s:%d: %w", host, port, err)
	}
	slog.Info("DAQ-P-1604 TCP connected", "category", "hardware-recv", "component", "hardware", "device", d.profile.ID, "address", host, "port", port)

	// 启用 TCP keepalive，尽早发现连接中断。
	if tcp, ok := conn.(*net.TCPConn); ok {
		_ = tcp.SetKeepAlive(true)
		_ = tcp.SetKeepAlivePeriod(10 * time.Second)
	}

	d.mu.Lock()
	d.conn = conn
	d.frameReader = sharedproto.NewFrameReader(conn)
	d.status.Connection = device.ConnectionConnected
	d.mu.Unlock()

	// 强制覆盖整个握手，防止 Windows 极端情况下 read deadline 未解除阻塞。
	err = runDAQP1604Handshake(conn, DAQ_P_1604_TIMEOUT+time.Second, func() error {
		// 启用长度前缀模式（供后续 FrameReader 读取单位系数 + 数据流帧解析）
		if err := d.sendCommand("w1601"); err != nil {
			return fmt.Errorf("enable length prefix: %w", err)
		}
		sharedproto.DrainW1601Response(d.frameReader, conn, 100*time.Millisecond)
		if err := d.syncUnitFromHardware(); err != nil {
			return fmt.Errorf("sync unit from hardware: %w", err)
		}
		return nil
	})
	if err != nil {
		d.mu.Lock()
		d.conn = nil
		d.frameReader = nil
		d.status.Connection = device.ConnectionError
		d.status.LastError = err.Error()
		d.mu.Unlock()
		_ = conn.Close()
		return err
	}
	return nil
}

func (d *DAQP1604) Disconnect() error {
	d.SetStopReason(sharedproto.StopReasonUserRequested)

	d.mu.Lock()
	_ = d.stopAcquisitionLocked()
	done := d.readLoopDone
	conn := d.conn
	d.conn = nil
	d.frameReader = nil
	d.status.Connection = device.ConnectionDisconnected
	d.mu.Unlock()

	// 等待 readLoop 退出后再关闭连接，避免 ReadFrame 与 Close 并发。
	if done != nil {
		select {
		case <-done:
		case <-time.After(sharedproto.ReadLoopJoinTimeout):
			slog.Warn("DAQ-P-1604 readLoop join timeout on disconnect", "device", d.profile.ID)
		}
	}

	if conn != nil {
		_ = conn.Close()
	}
	slog.Info("DAQ-P-1604 TCP disconnected", "category", "hardware-recv", "component", "hardware", "device", d.profile.ID)
	return nil
}

func (d *DAQP1604) StartAcquisition() error {
	d.ClearStopReason()

	d.mu.Lock()
	if d.acquiring {
		d.mu.Unlock()
		return nil
	}
	if d.conn == nil {
		d.mu.Unlock()
		return fmt.Errorf("device not connected")
	}

	// 等待上一次 readLoop 完全退出，避免旧 goroutine 与新采集竞争 conn 或状态。
	if d.readLoopDone != nil {
		done := d.readLoopDone
		d.mu.Unlock()
		select {
		case <-done:
		case <-time.After(sharedproto.ReadLoopJoinTimeout):
			slog.Warn("DAQ-P-1604 previous readLoop join timeout on start", "device", d.profile.ID)
		}
		d.mu.Lock()
	}

	// 重置帧读取器并排空连接中的残留数据，防止旧命令响应或流字节污染新采集。
	if d.frameReader != nil {
		d.frameReader.Reset()
	}
	conn := d.conn
	d.mu.Unlock()
	// 排空连接残留数据并记录字节数，便于诊断帧错位问题
	if drained := sharedproto.DrainConnection(conn, 100*time.Millisecond); drained > 0 {
		slog.Debug("DAQ-P-1604 drained residual data",
			"category", "hardware-drain", "component", "hardware",
			"device", d.profile.ID, "bytes", drained)
	}

	if err := d.initStream(); err != nil {
		return fmt.Errorf("init stream: %w", err)
	}
	if err := d.sendCommand("c 01 1"); err != nil {
		return fmt.Errorf("start stream: %w", err)
	}

	d.mu.Lock()
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

func (d *DAQP1604) StopAcquisition() error {
	d.SetStopReason(sharedproto.StopReasonUserRequested)

	d.mu.Lock()
	err := d.stopAcquisitionLocked()
	done := d.readLoopDone
	connected := d.conn != nil
	d.mu.Unlock()

	// 等待 readLoop 退出后再发送停止命令，避免命令与读取并发。
	if done != nil {
		select {
		case <-done:
		case <-time.After(sharedproto.ReadLoopJoinTimeout):
			slog.Warn("DAQ-P-1604 readLoop join timeout on stop", "device", d.profile.ID)
		}
	}

	if connected {
		if stopErr := d.sendCommand("c 02 1"); stopErr != nil {
			if sharedproto.IsConnectionFault(stopErr) {
				slog.Debug("DAQ-P-1604 stop stream: connection already gone", "device", d.profile.ID, "error", stopErr)
			} else {
				slog.Warn("DAQ-P-1604 stop stream command failed", "device", d.profile.ID, "error", stopErr)
			}
		}
	}
	return err
}

func (d *DAQP1604) stopAcquisitionLocked() error {
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

func (d *DAQP1604) SetDataSink(sink device.DataSink) {
	d.mu.Lock()
	d.sink = sink
	d.mu.Unlock()
}

func (d *DAQP1604) Status() device.Status {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.status
}

// SetUnit 切换压力单位。
//
// 行为：
//   - 已连接：写硬件 EU 系数（v01101），硬件实际完成单位转换；成功后才更新 profile
//   - 未连接：只更新 profile.Unit，下次 Connect 时由 syncUnitFromHardware 以硬件为准同步
//
// 设计原则：硬件是单位转换的唯一执行者。软件层不做 PSI↔Pa↔kPa 换算，
// 避免软件换算与硬件 EU 系数双重转换导致不一致。
func (d *DAQP1604) SetUnit(unit string) error {
	unit = strings.TrimSpace(unit)
	if !sharedproto.P1604IsSupportedUnit(unit) {
		return fmt.Errorf("unsupported unit: %s", unit)
	}

	d.mu.Lock()
	// adapter 层防御性检查：采集期间禁止写硬件 EU 系数。
	// 虽然 usecase 层 UpsertProfile 已检查 Acquiring，但 HTTP API 路径（SetUnit）
	// 未检查，此处兜底避免 v01101 与 readLoop 的 ReadFrame 竞争 frameReader。
	if d.acquiring {
		d.mu.Unlock()
		return fmt.Errorf("cannot change unit while acquiring")
	}
	conn := d.conn
	fr := d.frameReader
	d.mu.Unlock()

	// 已连接则写硬件 EU 系数
	if conn != nil && fr != nil {
		coeff := sharedproto.P1604PressureUnitCoefficient[unit]
		d.writeMu.Lock()
		// 清理 FrameReader 内部 buf 与 TCP 接收缓冲区的残留数据。
		// 残留来源：上次采集停止后 readLoop 退出时未读空的二进制流数据帧，
		// 或上一条命令延迟到达的应答。若不清理，P1604WriteUnitCoefficient 的
		// ReadFrame 会把残留当作 v01101 响应读出，触发
		// "unexpected v01101 response: <二进制乱码>"。
		// 必须先 fr.Reset()（清 buf）再 DrainConnection（清 TCP 缓冲区），
		// 顺序不能反：DrainConnection 读裸字节，不会清 FrameReader.buf。
		fr.Reset()
		if drained := sharedproto.DrainConnection(conn, 100*time.Millisecond); drained > 0 {
			slog.Debug("DAQ-P-1604 drained residual data before SetUnit",
				"category", "hardware-drain", "component", "hardware",
				"device", d.profile.ID, "bytes", drained)
		}
		err := sharedproto.P1604WriteUnitCoefficient(fr, conn, coeff, DAQ_P_1604_TIMEOUT)
		d.writeMu.Unlock()
		if err != nil {
			// 记录 warn 日志便于诊断：此前错误只返回前端，后端 log 画面不可见。
			slog.Warn("DAQ-P-1604 write hardware unit coefficient failed",
				"category", "hardware-send", "component", "hardware",
				"device", d.profile.ID, "unit", unit, "coeff", coeff, "error", err)
			// 对端已 FIN/RST → 连接已死，清理 driver + 通知 DeviceManager 删除。
			// SetUnit 要求非采集状态（开头已校验 d.acquiring==false），readLoop
			// 此时已退出，重置 conn/frameReader 无并发风险。
			// 与 readLoop defer 块的清理逻辑一致，确保 DeviceManager 从 map 中
			// 删除 driver，避免后续 StartAcquisition 爆 WSAECONNABORTED。
			if sharedproto.IsConnResetByPeer(err) {
				d.mu.Lock()
				d.conn = nil
				d.frameReader = nil
				d.status.Connection = device.ConnectionError
				d.status.LastError = err.Error()
				fn := d.onError
				d.mu.Unlock()
				_ = conn.Close()
				if fn != nil {
					fn(fmt.Errorf("write hardware unit coefficient: %w", err))
				}
			}
			return fmt.Errorf("write hardware unit coefficient: %w", err)
		}
		slog.Info("DAQ-P-1604 hardware unit updated",
			"category", "hardware-send", "component", "hardware",
			"device", d.profile.ID, "unit", unit, "coeff", coeff)
	}

	// 更新 profile（无论是否写硬件，都要保持 profile.Unit 与请求一致）
	d.mu.Lock()
	for i := range d.profile.Channels {
		d.profile.Channels[i].Unit = unit
	}
	d.mu.Unlock()
	return nil
}

// syncUnitFromHardware 读取硬件实际 EU 系数并同步 profile.Unit。
//
// 在 Connect 阶段调用：以硬件实际单位为准覆盖 profile，避免 profile 与硬件脱节
// 导致"单位标签变了但数据值没变"的现象（硬件 EU 系数没改，数据仍是旧单位量级）。
//
// 返回值：
//   - err: 仅在"连接已死"（对端 FIN/RST）时非空。其他软错误（超时、解析失败、
//     系数未知）不返回 err，保留 profile 单位并继续连接流程——兼容旧固件/模拟器
//
// 连接已死的判定依据 sharedproto.IsConnResetByPeer：包含 io.EOF、connection reset、
// broken pipe、WSAECONNABORTED 等。此时若继续保留 conn，后续 StartAcquisition
// 的 c 00 命令会爆 WSAECONNABORTED，且本地 TCP 已不可用，
// 必须让 Connect 失败并关闭 conn，强制用户重连。
func (d *DAQP1604) syncUnitFromHardware() error {
	d.mu.RLock()
	conn := d.conn
	fr := d.frameReader
	id := d.profile.ID
	currentUnit := ""
	if len(d.profile.Channels) > 0 {
		currentUnit = d.profile.Channels[0].Unit
	}
	d.mu.RUnlock()

	if conn == nil || fr == nil {
		return nil
	}

	// 读取硬件 EU 系数（内部发 u01101 并读响应）
	d.writeMu.Lock()
	coeff, err := sharedproto.P1604ReadUnitCoefficient(fr, conn, DAQ_P_1604_TIMEOUT)
	d.writeMu.Unlock()
	if err != nil {
		// 关键分支：对端已 FIN/RST → 连接已死，返回 error 让 Connect 失败
		if sharedproto.IsConnResetByPeer(err) {
			slog.Error("DAQ-P-1604 connection reset by peer during unit sync",
				"category", "hardware-recv", "component", "hardware",
				"device", id, "error", err)
			return fmt.Errorf("read unit coefficient: %w", err)
		}
		// 软错误（超时/解析失败等）：保留 profile 单位，记录 warn（不阻断连接）
		slog.Warn("read hardware unit coefficient failed",
			"category", "hardware-recv", "component", "hardware",
			"device", id, "error", err)
		return nil
	}

	hwUnit, ok := sharedproto.P1604MatchUnitByCoefficient(coeff)
	if !ok {
		slog.Warn("hardware unit coefficient unmatched to known units",
			"category", "hardware-recv", "component", "hardware",
			"device", id, "coeff", coeff)
		return nil
	}

	if currentUnit == hwUnit {
		return nil // profile 与硬件一致，无需同步
	}

	d.mu.Lock()
	for i := range d.profile.Channels {
		d.profile.Channels[i].Unit = hwUnit
	}
	d.mu.Unlock()
	slog.Info("DAQ-P-1604 unit synced from hardware",
		"category", "hardware-recv", "component", "hardware",
		"device", id, "profile_unit", currentUnit, "hardware_unit", hwUnit, "coeff", coeff)
	return nil
}

func (d *DAQP1604) SetTare(channelIndex int, offset float64) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if channelIndex < 0 || channelIndex >= len(d.profile.Channels) {
		return fmt.Errorf("invalid channel index: %d", channelIndex)
	}
	d.profile.Channels[channelIndex].TareOffset = offset
	return nil
}

func (d *DAQP1604) GetTare(channelIndex int) (float64, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	if channelIndex < 0 || channelIndex >= len(d.profile.Channels) {
		return 0, fmt.Errorf("invalid channel index: %d", channelIndex)
	}
	return d.profile.Channels[channelIndex].TareOffset, nil
}

func (d *DAQP1604) ClearTare(channelIndex int) error {
	return d.SetTare(channelIndex, 0)
}

func (d *DAQP1604) initStream() error {
	// w1601 已在 Connect 阶段发送并保持开启，此处不重复发送。
	// profile.SamplingRate 单位为 Hz，c 00 命令的第 5 个参数 <per> 单位为毫秒周期，需换算。
	// 修复历史 bug：原实现硬编码 100ms (10Hz)，导致 UI 设置的采样率被完全忽略。
	samplingRateHz := d.profile.SamplingRate
	if samplingRateHz <= 0 {
		// 兜底默认 20Hz，与项目历史默认值一致（防止 profile 字段缺失或异常导致除零）
		samplingRateHz = 20
	}
	periodMs := 1000 / samplingRateHz
	if periodMs < 1 {
		// 最小 1ms = 1000Hz，超过设备物理极限时钳制
		periodMs = 1
	}
	if err := d.sendCommand(fmt.Sprintf("c 00 1 FFFF 1 %d 7 0", periodMs)); err != nil {
		return fmt.Errorf("set stream params: %w", err)
	}
	time.Sleep(50 * time.Millisecond)

	// 配置流返回内容：0010=压力，0400=设备时间戳，0800=大气数据
	// 掩码计算：压力(0010) + 可选时间戳(0400) + 大气数据(0800)
	contentMask := 0x0010
	if d.profile.UseDeviceTimestampEnabled() {
		contentMask |= 0x0400
	}
	contentMask |= 0x0800 // 始终包含大气数据
	contentMaskHex := fmt.Sprintf("%04X", contentMask)
	if err := d.sendCommand(fmt.Sprintf("c 05 1 %s", contentMaskHex)); err != nil {
		return fmt.Errorf("set stream content: %w", err)
	}
	time.Sleep(50 * time.Millisecond)

	return nil
}

func (d *DAQP1604) sendCommand(cmd string) error {
	if d.conn == nil {
		return fmt.Errorf("not connected")
	}
	d.writeMu.Lock()
	defer d.writeMu.Unlock()
	// 命令收发用 Info 级别：ring buffer 透传，stderr / file 由 CategorySkipHandler 跳过。
	slog.Info("DAQ-P-1604 command send", "category", "hardware-send", "component", "hardware", "device", d.profile.ID, "command", cmd)
	// 命令发送委托给 sharedproto.SendCommandNoNewline：
	//   - 纯 ASCII，不带换行符（实测设备 w1601 模式下 \r\n 会导致 N05）
	//   - 内部处理 write deadline 设置与清除
	return sharedproto.SendCommandNoNewline(d.conn, cmd, DAQ_P_1604_TIMEOUT)
}

func (d *DAQP1604) readLoop(stop <-chan struct{}) {
	lastDataAt := time.Now()
	var unexpectedErr error

	defer func() {
		if unexpectedErr != nil {
			// 主动停止场景不视为异常，避免误触发 onError。
			if d.GetStopReason() == sharedproto.StopReasonUserRequested {
				return
			}

			d.mu.Lock()
			d.acquiring = false
			d.stop = nil
			d.status.Acquiring = false
			d.status.LastError = unexpectedErr.Error()
			d.status.Connection = device.ConnectionError
			fn := d.onError
			d.mu.Unlock()

			slog.Warn("DAQ-P-1604 read loop exited unexpectedly", "device", d.profile.ID, "error", unexpectedErr)
			if fn != nil {
				fn(unexpectedErr)
			}
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

	for {
		select {
		case <-stop:
			return
		default:
		}

		d.mu.RLock()
		fr := d.frameReader
		d.mu.RUnlock()
		if fr == nil {
			return
		}

		d.mu.RLock()
		conn := d.conn
		d.mu.RUnlock()
		if conn == nil {
			return
		}
		_ = conn.SetReadDeadline(time.Now().Add(200 * time.Millisecond))

		payload, err := fr.ReadFrame()
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				if time.Since(lastDataAt) > noDataTimeout {
					unexpectedErr = fmt.Errorf("no data received for %v", noDataTimeout)
					return
				}
				continue
			}
			// 主动停止后连接被关闭属于预期行为，静默退出。
			if d.GetStopReason() == sharedproto.StopReasonUserRequested && sharedproto.IsClosedConnError(err) {
				return
			}
			d.mu.Lock()
			d.readErrors++
			d.mu.Unlock()
			slog.Debug("DAQ-P-1604 read error", "device", d.profile.ID, "error", err)
			unexpectedErr = err
			return
		}
		if len(payload) > 0 {
			lastDataAt = time.Now()
			d.processPayload(payload)
		}
	}
}

func (d *DAQP1604) processPayload(data []byte) {
	// ASCII 帧属于命令响应，不应作为采集数据下发。
	if sharedproto.IsASCIIFrame(data) {
		slog.Info("DAQ-P-1604 command response", "category", "hardware-recv", "component", "hardware", "device", d.profile.ID, "response", strings.TrimSpace(string(data)))
		return
	}

	d.mu.RLock()
	fr := d.frameReader
	useDeviceTs := d.profile.UseDeviceTimestampEnabled()
	sink := d.sink
	d.mu.RUnlock()

	if sink == nil {
		return
	}

	// DAQ-P-1604 始终请求大气数据（0800），所以 hasAtmosphericData = true。
	channels, deviceTimestampMs, _, err := sharedproto.ParseStreamFrameEx(data, useDeviceTs, true)
	if err != nil {
		d.mu.Lock()
		d.frameErrors++
		d.consecutiveFrameErrors++
		consecutive := d.consecutiveFrameErrors
		d.mu.Unlock()

		slog.Debug("DAQ-P-1604 frame parse error", "device", d.profile.ID, "n", len(data), "error", err)

		// 连续帧错误达到阈值时尝试丢弃缓冲区首字节以重新对齐。
		if consecutive >= maxConsecutiveFrameErrors && fr != nil {
			slog.Warn("DAQ-P-1604 auto-resync triggered", "device", d.profile.ID, "consecutiveErrors", consecutive)
			fr.Resync()
			d.mu.Lock()
			d.consecutiveFrameErrors = 0
			d.mu.Unlock()
		}
		return
	}

	d.mu.Lock()
	d.consecutiveFrameErrors = 0
	d.mu.Unlock()

	if len(channels) == 0 {
		return
	}

	indices := make([]int, len(channels))
	values := make([]float64, len(channels))
	for i := range channels {
		indices[i] = i
		values[i] = channels[i]
	}

	payload := device.DataPayload{
		DeviceID:       d.profile.ID,
		DeviceType:     d.profile.Type,
		DeviceName:     d.profile.Name,
		Timestamp:      device.NowMs(),
		Channels:       values,
		ChannelIndices: indices,
	}
	if useDeviceTs && deviceTimestampMs > 0 {
		payload.DeviceTimestamp = deviceTimestampMs
	}

	sink(payload)
}

// 注：stopReason 三件套（SetStopReason/GetStopReason/ClearStopReason）、
// isConnectionFault、isClosedConnError、drainConnection 已下沉到
// shared.local/device-sdk/go/protocol（conn_helpers.go），
// 通过嵌入 sharedproto.StopReasonTracker 和直接调用 sharedproto.* 函数复用。
