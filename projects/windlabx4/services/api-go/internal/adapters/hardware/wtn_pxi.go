package hardware

import (
	"encoding/binary"
	"fmt"
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
	WTN_PXI_DEFAULT_HOST = "127.0.0.1"
	WTN_PXI_DEFAULT_PORT = 9000
	WTN_PXI_TIMEOUT      = 5 * time.Second
)

const (
	WTN_PXI_LENGTH_PREFIX_BYTES = 4
	WTN_PXI_MAX_PAYLOAD_BYTES   = 64 * 1024
	WTN_PXI_REQUIRED_CHANNELS   = 8
	// WTN_PXI_ARRAY_LEN_OFFSET 数据帧 payload 内数组长度字段偏移（2 字节协议前缀后）。
	WTN_PXI_ARRAY_LEN_OFFSET = 2
	// WTN_PXI_ARRAY_LEN_BYTES 数组长度字段字节数（LabVIEW 大端 uint32）。
	WTN_PXI_ARRAY_LEN_BYTES = 4
	// WTN_PXI_HEADER_BYTES 数据帧头部总字节数 = 2 字节前缀 + 4 字节数组长度。
	WTN_PXI_HEADER_BYTES = 6
)

// wtnPXINoDataTimeout 是允许的最长无数据时间，超过则判定为连接异常。
// readLoop 入口启动独立 time.AfterFunc timer，到期触发 invalidateConnectionAfterReadLoopTimeout。
//
// 取值 10s（与 windlabx4 项目内 P1604/DAQP1064Pre 一致）：
// WTN-PXI 通常以高采样率推送 8 通道数据帧，10s 内无任何字节到达一定有问题
// （网络中断/设备断电/网线脱落/服务端崩溃）。
//
// ADR-009 R0-10：原实现 readLoop 通过 SetReadDeadline(100ms) 让 Read 周期返回，
// 依赖循环体执行检测无数据。问题 Windows 电脑 deadline 失效时循环体不可达，
// 半开连接无法自行收敛。独立 timer 不依赖循环体，即使 Read 永久阻塞也能到期触发。
//
// 使用 var 而非 const 是为了允许测试注入短超时（200ms）加速 no-data timer 用例；
// 运行期不应修改，生产代码保持默认 10s。同一包内测试默认串行执行，覆盖安全。
var wtnPXINoDataTimeout = 10 * time.Second

type WTNPXI struct {
	mu         sync.RWMutex
	profile    device.Profile
	status     device.Status
	sink       device.DataSink
	stop       chan struct{}
	acquiring  bool
	conn       net.Conn
	recvBuffer []byte
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
	// 嵌入 StopReasonTracker 提供 SetStopReason/GetStopReason/ClearStopReason，
	// 用于 readLoop 区分调用方主动停止（Stop/Disconnect）与连接意外断开，
	// 避免 Close 触发 Read 错误时误调 onError 误报设备故障。
	sharedproto.StopReasonTracker
}

// 编译期断言：WTNPXI 实现 ports.ErrorNotifiable
var _ ports.ErrorNotifiable = (*WTNPXI)(nil)

func NewWTNPXI(profile device.Profile) *WTNPXI {
	return &WTNPXI{
		profile: profile,
		status: device.Status{
			ID:         profile.ID,
			Name:       profile.Name,
			Type:       profile.Type,
			Connection: device.ConnectionDisconnected,
		},
		recvBuffer: make([]byte, 0, 8192),
	}
}

// SetOnError 实现 ports.ErrorNotifiable：DeviceManager 在 Connect 阶段注入回调。
func (d *WTNPXI) SetOnError(fn func(err error)) {
	d.mu.Lock()
	d.onError = fn
	d.mu.Unlock()
}

func (d *WTNPXI) ID() string { return d.profile.ID }

func (d *WTNPXI) Connect() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.conn != nil {
		return nil
	}

	host := d.profile.Address
	if host == "" {
		host = WTN_PXI_DEFAULT_HOST
	}
	port := d.profile.Port
	if port <= 0 {
		port = WTN_PXI_DEFAULT_PORT
	}
	// ADR-009 R0-7：net.DialTimeout 依赖 Dial 内部 deadline，Windows 故障机器
	// deadline 不可靠时 Dial 可能永远不返回，前端"连接中"状态无法翻转。
	// 改用 sharedproto.DialTCP（goroutine + time.After 软超时 + abandoned 信号），
	// 主线程在 timeout 后立即返回错误，不依赖 Dial 兑现 deadline。
	conn, err := sharedproto.DialTCP(fmt.Sprintf("%s:%d", host, port), "", WTN_PXI_TIMEOUT)
	if err != nil {
		return fmt.Errorf("connect to %s:%d: %w", host, port, err)
	}
	slog.Info("WTN_PXI TCP connected", "category", "hardware-recv", "component", "hardware", "device", d.profile.ID, "address", host, "port", port)

	d.conn = conn
	d.status.Connection = device.ConnectionConnected
	return nil
}

func (d *WTNPXI) Disconnect() error {
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
	// Disconnect 是用户主动断开，超时不再触发 invalidate（不调 onError），
	// 直接 Close conn 兜底解除 readLoop 阻塞——readLoop 检测到 StopReason 后静默退出。
	if done != nil {
		select {
		case <-done:
		case <-time.After(sharedproto.ReadLoopJoinTimeout):
			slog.Warn("WTN_PXI readLoop join timeout on disconnect", "device", d.profile.ID)
		}
	}

	if conn != nil {
		// 后台 detach：join 超时说明 readLoop 可能仍挂在 Read 上，
		// LSP 环境下 Close 可能永久阻塞，不阻塞 Disconnect。
		go sharedproto.AbortConnection(conn)
	}
	slog.Info("WTN_PXI TCP disconnected", "category", "hardware-recv", "component", "hardware", "device", d.profile.ID)
	return nil
}

func (d *WTNPXI) StartAcquisition() error {
	// 清理上一轮采集的 StopReason，确保本次异常退出时 onError 能正常触发。
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

	// 等待上一次 readLoop 完全退出，避免旧 goroutine 与新采集竞争 conn。
	// ADR-009：close(stop) 不保证 readLoop 立即退出（Read 可能仍在内核阻塞），
	// 若直接启动新 readLoop 会形成双 reader，TCP 字节随机分配导致数据错位/丢失。
	if d.readLoopDone != nil {
		done := d.readLoopDone
		d.mu.Unlock()
		select {
		case <-done:
		case <-time.After(sharedproto.ReadLoopJoinTimeout):
			d.invalidateConnectionAfterReadLoopTimeout("previous read loop did not exit; reconnect required")
			return fmt.Errorf("previous read loop did not exit; reconnect required")
		}
		d.mu.Lock()
		// 等待期间连接可能已被其他路径 invalidate 置 nil，需重新检查
		if d.conn == nil {
			d.mu.Unlock()
			return fmt.Errorf("device not connected")
		}
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

func (d *WTNPXI) StopAcquisition() error {
	// 标记主动停止：readLoop 检测到此原因后静默退出，不触发 onError 误报。
	d.SetStopReason(sharedproto.StopReasonUserRequested)

	d.mu.Lock()
	_ = d.stopAcquisitionLocked()
	done := d.readLoopDone
	d.mu.Unlock()

	// 等待 readLoop 退出，超时强制 invalidate（Close conn + 调 onError）。
	// ADR-009：close(stop) 无法解除内核 Read 阻塞，必须 Close conn 兜底。
	// Stop 是显式停止采集，连接已不可信，需通知 DeviceManager 重连。
	if done != nil {
		select {
		case <-done:
		case <-time.After(sharedproto.ReadLoopJoinTimeout):
			d.invalidateConnectionAfterReadLoopTimeout("read loop did not exit after stop; reconnect required")
			return fmt.Errorf("read loop did not exit after stop; reconnect required")
		}
	}
	return nil
}

func (d *WTNPXI) stopAcquisitionLocked() error {
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

// invalidateConnectionAfterReadLoopTimeout 在 readLoop join 超时后强制作废连接。
//
// 触发场景：
//   - StopAcquisition：close(stop) 后 readLoop 仍在内核 Read 阻塞，1s 内未退出
//   - StartAcquisition：上一次 readLoop 仍未退出，新采集无法启动
//
// 行为：
//   - 锁内取 conn + 置 nil + 标记 status=Error（连接已不可信，禁止复用）
//   - 锁外 Close(conn) 解除 readLoop 阻塞
//   - 调 onError 通知 DeviceManager 移除 driver，下次操作走重连路径
//
// 设计依据 ADR-009 决策 3、7：watchdog/兜底 Close 后 conn 失效，必须置 nil + 通知上层。
func (d *WTNPXI) invalidateConnectionAfterReadLoopTimeout(message string) {
	d.mu.Lock()
	conn := d.conn
	d.conn = nil
	d.acquiring = false
	d.status.Acquiring = false
	d.status.Connection = device.ConnectionError
	d.status.LastError = message
	fn := d.onError
	d.mu.Unlock()

	if conn != nil {
		// 后台 detach：LSP 环境挂起 Read 时 Close 可能永久阻塞（同 t1603 模式）。
		go sharedproto.AbortConnection(conn)
	}
	if fn != nil {
		fn(fmt.Errorf("%s", message))
	}
}

func (d *WTNPXI) SetDataSink(sink device.DataSink) {
	d.mu.Lock()
	d.sink = sink
	d.mu.Unlock()
}

func (d *WTNPXI) Status() device.Status {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.status
}

func (d *WTNPXI) SetUnit(unit string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	for i := range d.profile.Channels {
		d.profile.Channels[i].Unit = unit
	}
	return nil
}

func (d *WTNPXI) SetTare(channelIndex int, offset float64) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if channelIndex < 0 || channelIndex >= len(d.profile.Channels) {
		return fmt.Errorf("invalid channel index: %d", channelIndex)
	}
	d.profile.Channels[channelIndex].TareOffset = offset
	return nil
}

func (d *WTNPXI) GetTare(channelIndex int) (float64, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	if channelIndex < 0 || channelIndex >= len(d.profile.Channels) {
		return 0, fmt.Errorf("invalid channel index: %d", channelIndex)
	}
	return d.profile.Channels[channelIndex].TareOffset, nil
}

func (d *WTNPXI) ClearTare(channelIndex int) error {
	return d.SetTare(channelIndex, 0)
}

func (d *WTNPXI) readLoop(stop <-chan struct{}) {
	// ADR-009 R0-10：no-data owner 必须独立于 read goroutine 与其 mutex。
	//
	// 历史背景：原实现 readLoop 通过 SetReadDeadline(100ms) 让 Read 周期返回，
	// 依赖循环体执行检测无数据。问题 Windows 电脑 deadline 失效时循环体不可可达，
	// 半开连接无法自行收敛（设备断电/网线脱落但 TCP 连接仍存活）。
	//
	// 整改后：readLoop 入口启动独立 time.AfterFunc timer，到期通过 expected conn 比较
	// 安全毒化连接（清空 conn、置 Error 状态、Close conn）。timer 不依赖 readLoop 循环体
	// 执行，即使 Read 永久阻塞也能到期触发。每次收到有效数据（n > 0）调用 Reset 续期；
	// readLoop 退出 defer Stop。
	//
	// 不调 onError：让 readLoop defer 的 unexpectedErr 路径统一调用
	// invalidateConnectionAfterReadLoopTimeout 通知上层，避免同一故障被 onError 回调两次。
	// timer Close 后 readLoop 的 Read 返回错误进入 defer，defer 调用统一毒化路径清理
	// （重置 status=Error、清空 conn、调 onError）。
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

	// 快照 wtnPXINoDataTimeout 到局部变量：测试会覆盖全局 wtnPXINoDataTimeout 加速（200ms），
	// t.Cleanup 在测试结束时恢复原值。若 timer 回调内读取全局变量，可能在与
	// t.Cleanup 写入并发时触发 data race。局部变量在 timer 创建时已固化，回调
	// 读取栈上变量无 race 风险。
	noDataTimeoutSnapshot := wtnPXINoDataTimeout
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
		slog.Warn("WTN_PXI no data timeout, conn closed by watchdog",
			"device", d.profile.ID, "duration", noDataTimeoutSnapshot)
	})
	// readLoop 退出时停止 timer，避免 timer 在 readLoop 已退出后误触发。
	// Stop 不等待已 firing 的回调完成，但回调内 acquiring + expected conn 双重检查
	// 能正确处理 readLoop 退出后 timer 才 fire 的场景。
	defer noDataTimer.Stop()

	var unexpectedErr error

	// defer 顺序（LIFO）：
	//   1. 后注册先执行：close(readLoopDone) 通知等待方 readLoop 已退出
	//   2. 先注册后执行：异常退出且非用户主动停止时调 invalidateConnectionAfterReadLoopTimeout 统一毒化
	// 必须先 close(done) 再 invalidate，确保 Stop/Disconnect 的 join 先收到信号。
	defer func() {
		if unexpectedErr != nil {
			// 主动停止场景（Stop/Disconnect 已设置 StopReason）不视为异常，
			// 避免 Close 触发 Read 错误时误调 onError 误报设备故障。
			if d.GetStopReason() == sharedproto.StopReasonUserRequested {
				return
			}
			// ADR-009 R0-11 扩展（与 P1604/T1603 标杆一致）：terminal read error 必须调用
			// invalidateConnectionAfterReadLoopTimeout 统一毒化连接——清空 d.conn、close conn、
			// 置 Error 状态、保存 LastError、通知 onError。
			//
			// 历史背景：原 defer 手写清理（设 status=Disconnected、Close conn），未清空 conn
			// 也未设 Error。EOF/RST 后连接已死，下次 StartAcquisition 会用旧 conn 发命令爆
			// WSAECONNABORTED。R0-10 整改时附带改造为统一毒化路径，保证 noDataTimer 触发后
			// 状态正确（Error，不是 Disconnected）。
			//
			// invalidateConnectionAfterReadLoopTimeout 不清空 d.stop（它是采集生命周期字段，
			// 非连接状态），此处显式清空对齐原 defer 行为，避免 readLoop 退出后 stop channel 残留。
			slog.Warn("WTN_PXI read loop exited unexpectedly",
				"device", d.profile.ID, "error", unexpectedErr)
			d.mu.Lock()
			d.stop = nil
			d.mu.Unlock()
			d.invalidateConnectionAfterReadLoopTimeout(unexpectedErr.Error())
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

	buf := make([]byte, 8192)
	for {
		select {
		case <-stop:
			// 用户主动停止（StopAcquisition/Disconnect），不触发 onError
			return
		default:
		}
		// SetReadDeadline 仍保留作为单次 Read 的软超时，让循环体能周期性
		// 重新检查 stop channel（ADR-009 R0-10：no-data 检测由独立 timer 负责，
		// deadline 失效场景由 timer 兜底 Close conn）。
		conn.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
		n, err := conn.Read(buf)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				// deadline 软超时：循环体重新检查 stop channel，不视为异常。
				// no-data 检测由独立 noDataTimer 负责，不依赖循环体执行。
				continue
			}
			// 主动停止后连接被关闭属于预期行为，静默退出。
			// Stop/Disconnect 在 close(stop) 之前已 SetStopReason，
			// 且 invalidate/Disconnect 会 Close conn 触发 Read 返回 closed 错误。
			if d.GetStopReason() == sharedproto.StopReasonUserRequested && sharedproto.IsClosedConnError(err) {
				return
			}
			// conn 被外部路径 Close（noDataTimer / Disconnect）
			// 触发的 Read 错误属预期，静默退出。onError 已由 invalidate 路径统一负责，
			// 此处再调 onError 会导致同一故障被 DeviceManager 误判为两次。
			if sharedproto.IsClosedConnError(err) {
				slog.Debug("WTN_PXI read loop exiting silently (conn closed by external path)",
					"device", d.profile.ID, "error", err)
				return
			}
			slog.Debug("WTN_PXI read error", "device", d.profile.ID, "error", err)
			unexpectedErr = err
			return
		}
		if n > 0 {
			// 收到数据，续期 no-data timer。
			// Reset 是原子操作，无需加锁；即使 timer 已 fire 也能安全 Reset（time.AfterFunc 文档保证）。
			// 用 n > 0 而非"有效帧"作为续期信号：表示 TCP 连接仍在传输字节，即使部分帧或非采集帧
			// 也说明对端活跃；只有完全无字节到达才判定为连接异常。
			// 使用入口快照的 noDataTimeoutSnapshot 而非全局 wtnPXINoDataTimeout，避免与
			// 测试 t.Cleanup 写入全局变量并发 race。
			noDataTimer.Reset(noDataTimeoutSnapshot)
			d.processData(buf[:n])
		}
	}
}

func (d *WTNPXI) processData(data []byte) {
	d.mu.Lock()
	d.recvBuffer = append(d.recvBuffer, data...)

	// 在锁内仅做 frame 解析与 payload 切片，避免锁外再访问 recvBuffer；
	// payloads 在锁外处理，避免持锁状态下 spawn goroutine 与父 Lock 形成 RLock 竞争。
	payloads := make([][]byte, 0, 8)
	for len(d.recvBuffer) >= WTN_PXI_LENGTH_PREFIX_BYTES {
		payloadLen := int(d.recvBuffer[0])<<24 | int(d.recvBuffer[1])<<16 |
			int(d.recvBuffer[2])<<8 | int(d.recvBuffer[3])

		if payloadLen == 0 || payloadLen > WTN_PXI_MAX_PAYLOAD_BYTES {
			slog.Debug("WTN_PXI invalid payload length", "device", d.profile.ID, "length", payloadLen)
			d.recvBuffer = d.recvBuffer[1:]
			continue
		}

		frameLen := WTN_PXI_LENGTH_PREFIX_BYTES + payloadLen
		if len(d.recvBuffer) < frameLen {
			break
		}

		payload := make([]byte, payloadLen)
		copy(payload, d.recvBuffer[WTN_PXI_LENGTH_PREFIX_BYTES:frameLen])

		d.recvBuffer = d.recvBuffer[frameLen:]
		payloads = append(payloads, payload)
	}
	d.mu.Unlock()

	// 锁外同步处理 payload：
	// - sink 通常是非阻塞 channel 发送，单帧处理在微秒级
	// - 避免 1000Hz 下无界 spawn goroutine（每秒 1000+ goroutine）
	// - 避免持锁 spawn 的子 goroutine 与父 Lock 形成 RLock 阻塞
	for _, p := range payloads {
		d.handlePayload(p)
	}
}

func (d *WTNPXI) handlePayload(payload []byte) {
	d.mu.RLock()
	acquiring := d.acquiring
	sink := d.sink
	channels := d.profile.Channels
	deviceID := d.profile.ID
	d.mu.RUnlock()

	if !acquiring || sink == nil {
		return
	}

	// 真实设备数据帧（LabVIEW Unflatten From String + double[]）：
	//   payload = 2 字节协议前缀 + 4 字节大端数组长度 + N × double（大端）
	// 长度校验 len(payload) == 6 + count*8 严格区分旧 float32 小端格式。
	valueCount := decodeWTNPXICount(payload)
	if valueCount < WTN_PXI_REQUIRED_CHANNELS {
		slog.Debug("WTN_PXI insufficient values", "device", d.profile.ID, "count", valueCount)
		return
	}

	values := make([]float64, 0, len(channels))
	indices := make([]int, 0, len(channels))

	for _, ch := range channels {
		if !ch.Enabled {
			continue
		}
		if ch.Index >= 0 && ch.Index < valueCount {
			val := decodeWTNPXIDouble(payload, ch.Index)
			indices = append(indices, ch.Index)
			values = append(values, val)
		}
	}

	sink(device.DataPayload{
		DeviceID:       deviceID,
		DeviceType:     d.profile.Type,
		DeviceName:     d.profile.Name,
		Timestamp:      device.NowMs(),
		Channels:       values,
		ChannelIndices: indices,
	})
}

// decodeWTNPXICount 返回数据帧 payload 内的数组长度（大端 uint32 @[2:6]）。
// 不是数据帧时返回 -1，上层据此跳过（设备信息帧等）。
func decodeWTNPXICount(payload []byte) int {
	if len(payload) < WTN_PXI_HEADER_BYTES {
		return -1
	}
	count := int(binary.BigEndian.Uint32(payload[WTN_PXI_ARRAY_LEN_OFFSET : WTN_PXI_ARRAY_LEN_OFFSET+WTN_PXI_ARRAY_LEN_BYTES]))
	if count <= 0 || count > WTN_PXI_MAX_PAYLOAD_BYTES/8 {
		return -1
	}
	if len(payload) != WTN_PXI_HEADER_BYTES+count*8 {
		return -1
	}
	return count
}

// decodeWTNPXIDouble 读取数据帧第 idx 个 double 值（大端，从偏移 6 起）。
func decodeWTNPXIDouble(payload []byte, idx int) float64 {
	return math.Float64frombits(binary.BigEndian.Uint64(payload[WTN_PXI_HEADER_BYTES+idx*8:]))
}
