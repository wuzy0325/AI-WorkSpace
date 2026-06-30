package hardware

import (
	"errors"
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
	// readLoopJoinTimeout 是等待 readLoop 退出的最长时间。
	readLoopJoinTimeout = 1 * time.Second
)

// stopReasonUserRequested 表示调用方主动停止（StopAcquisition / Disconnect）。
// readLoop 识别到该原因后静默退出，避免误判为连接异常。
const stopReasonUserRequested = "user-requested"

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
	onError                 func(err error)

	// readLoopDone 由 readLoop 退出时关闭，供 Start/Stop/Disconnect 等待协程结束。
	readLoopDone chan struct{}
	// stopReason 标记主动停止原因，由停止方在 close(stop) 前设置。
	stopReason   string
	stopReasonMu sync.Mutex
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

	conn, err := net.DialTimeout("tcp", fmt.Sprintf("%s:%d", host, port), DAQ_P_1604_TIMEOUT)
	if err != nil {
		return fmt.Errorf("connect to %s:%d: %w", host, port, err)
	}

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
	return nil
}

func (d *DAQP1604) Disconnect() error {
	d.setStopReason(stopReasonUserRequested)

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
		case <-time.After(readLoopJoinTimeout):
			slog.Warn("DAQ-P-1604 readLoop join timeout on disconnect", "device", d.profile.ID)
		}
	}

	if conn != nil {
		_ = conn.Close()
	}
	return nil
}

func (d *DAQP1604) StartAcquisition() error {
	d.clearStopReason()

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
		case <-time.After(readLoopJoinTimeout):
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
	d.drainConnection(conn, 100*time.Millisecond)

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
	d.setStopReason(stopReasonUserRequested)

	d.mu.Lock()
	err := d.stopAcquisitionLocked()
	done := d.readLoopDone
	connected := d.conn != nil
	d.mu.Unlock()

	// 等待 readLoop 退出后再发送停止命令，避免命令与读取并发。
	if done != nil {
		select {
		case <-done:
		case <-time.After(readLoopJoinTimeout):
			slog.Warn("DAQ-P-1604 readLoop join timeout on stop", "device", d.profile.ID)
		}
	}

	if connected {
		if stopErr := d.sendCommand("c 02 1"); stopErr != nil {
			if isConnectionFault(stopErr) {
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

func (d *DAQP1604) SetUnit(unit string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	for i := range d.profile.Channels {
		d.profile.Channels[i].Unit = unit
	}
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
	if err := d.sendCommand("w1601"); err != nil {
		return fmt.Errorf("enable length prefix: %w", err)
	}
	time.Sleep(50 * time.Millisecond)

	if err := d.sendCommand("c 00 1 FFFF 1 100 7 0"); err != nil {
		return fmt.Errorf("set stream params: %w", err)
	}
	time.Sleep(50 * time.Millisecond)

	// 配置流返回内容：0010=压力，0400=设备时间戳，0800=大气数据
	// 掩码计算：压力(0010) + 可选时间戳(0400) + 大气数据(0800)
	contentMask := 0x0010
	if d.profile.DaqP1604UseDeviceTimestamp {
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
	_ = d.conn.SetWriteDeadline(time.Now().Add(DAQ_P_1604_TIMEOUT))
	_, err := d.conn.Write([]byte(cmd + "\r\n"))
	_ = d.conn.SetWriteDeadline(time.Time{}) // 清除写截止时间，避免影响后续操作
	return err
}

// drainConnection 在指定超时内排空连接中的残留数据。
// 启动新采集前调用，避免旧命令响应或流数据污染帧对齐。
// 限制最大循环次数并在首次超时且无数据时立即退出，避免长时间阻塞
// 导致模拟器/设备端的命令读取 goroutine 因 deadline 超时提前退出。
func (d *DAQP1604) drainConnection(conn net.Conn, timeout time.Duration) {
	if conn == nil {
		return
	}
	buf := make([]byte, 4096)
	totalDrained := 0
	const maxIters = 3 // 安全上限：最多 3 次读取尝试
	for i := 0; i < maxIters; i++ {
		_ = conn.SetReadDeadline(time.Now().Add(timeout))
		n, err := conn.Read(buf)
		if n > 0 {
			totalDrained += n
		}
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				// 当前无残留数据，结束排空
				break
			}
			// 连接已关闭等错误，无需继续
			break
		}
	}
	_ = conn.SetReadDeadline(time.Time{})
	if totalDrained > 0 {
		slog.Debug("DAQ-P-1604 drained residual data", "device", d.profile.ID, "bytes", totalDrained)
	}
}

func (d *DAQP1604) readLoop(stop <-chan struct{}) {
	lastDataAt := time.Now()
	var unexpectedErr error

	defer func() {
		if unexpectedErr != nil {
			// 主动停止场景不视为异常，避免误触发 onError。
			if d.getStopReason() == stopReasonUserRequested {
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
			if d.getStopReason() == stopReasonUserRequested && isClosedConnError(err) {
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
		return
	}

	d.mu.RLock()
	fr := d.frameReader
	useDeviceTs := d.profile.DaqP1604UseDeviceTimestamp
	sink := d.sink
	d.mu.RUnlock()

	if sink == nil {
		return
	}

	// DAQ-P-1604 始终请求大气数据（0800），所以 hasAtmosphericData = true。
	channels, deviceTimestampMs, err := sharedproto.ParseStreamFrameEx(data, useDeviceTs, true)
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
		Timestamp:      device.NowMs(),
		Channels:       values,
		ChannelIndices: indices,
	}
	if useDeviceTs && deviceTimestampMs > 0 {
		payload.DeviceTimestamp = deviceTimestampMs
	}

	sink(payload)
}

func (d *DAQP1604) setStopReason(reason string) {
	d.stopReasonMu.Lock()
	defer d.stopReasonMu.Unlock()
	if d.stopReason == "" {
		d.stopReason = reason
	}
}

func (d *DAQP1604) getStopReason() string {
	d.stopReasonMu.Lock()
	defer d.stopReasonMu.Unlock()
	return d.stopReason
}

func (d *DAQP1604) clearStopReason() {
	d.stopReasonMu.Lock()
	defer d.stopReasonMu.Unlock()
	d.stopReason = ""
}

// isClosedConnError 判断错误是否由连接被主动关闭引起。
func isClosedConnError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, net.ErrClosed) {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "use of closed network connection")
}

// isConnectionFault 启发式判定错误是否由连接故障引起，用于日志分级。
func isConnectionFault(err error) bool {
	if err == nil {
		return false
	}
	if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "i/o timeout") ||
		strings.Contains(msg, "broken pipe") ||
		strings.Contains(msg, "reset by peer") ||
		strings.Contains(msg, "use of closed network connection") ||
		strings.Contains(msg, "connection refused") ||
		strings.Contains(msg, "device disconnected")
}
