package hardware

import (
	"fmt"
	"log/slog"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	sharedproto "shared.local/device-sdk/go/protocol"
	"windlabx4/services/api-go/internal/core/device"
	"windlabx4/services/api-go/internal/ports"
)

const (
	DAQ_P_1604_DEFAULT_HOST = "192.168.3.101"
	DAQ_P_1604_DEFAULT_PORT = 9000
	DAQ_P_1604_TIMEOUT      = 5 * time.Second

	// maxConsecutiveFrameErrors 是触发自动重同步的连续帧错误阈值。
	maxConsecutiveFrameErrors = 5
	// readLoop 等待超时使用 sharedproto.ReadLoopJoinTimeout（跨项目统一）
	// 主动停止原因使用 sharedproto.StopReasonUserRequested（跨项目统一）
)

// noDataTimeoutNs 存储允许的最长无数据时间（纳秒），超过则判定为连接异常。
// readLoop 入口启动独立 time.AfterFunc timer，到期触发连接毒化。
//
// 包级共享变量：DAQP1604 / DAQP1064Pre / WTNPXI 三个驱动的 readLoop 共用此超时。
// 各驱动 readLoopWatchdog（兜底 deadline 失效的 Read 阻塞）与本变量独立，
// 避免共享超时导致同时触发后 readLoopWatchdog 覆盖 noDataTimer 设置的 LastError。
//
// ADR-009 R0-10：原实现 readLoop 通过 SetReadDeadline(100~200ms) 让 Read 周期返回，
// 在循环体累计 time.Since(lastDataAt) > noDataTimeout 检测无数据。问题 Windows 电脑
// deadline 失效时循环体不可达，noDataTimeout 永远不会被触发，半开连接无法自行收敛。
// 独立 timer 不依赖循环体，即使 Read 永久阻塞也能到期触发。
//
// 使用 atomic.Int64 而非 const 是为了允许测试注入短超时（200ms）加速 no-data timer 用例；
// 运行期不应修改，生产代码保持默认 10s。
//
// ADR-009 finding 4 修复：原 var noDataTimeout time.Duration 在 readLoop goroutine 跨
// 测试边界读取与下一测试修改全局变量并发触发 data race。改用 atomic.Int64 后，
// 读写均无锁原子操作，race detector 不再报错。helper 函数 getNoDataTimeout / setNoDataTimeout
// 封装访问，避免直接暴露 atomic 类型到外部包。
var noDataTimeoutNs atomic.Int64

func init() {
	noDataTimeoutNs.Store(int64(10 * time.Second))
}

// getNoDataTimeout 返回当前无数据超时。readLoop 与所有生产代码统一通过本函数读取。
func getNoDataTimeout() time.Duration {
	return time.Duration(noDataTimeoutNs.Load())
}

// setNoDataTimeout 修改无数据超时，仅测试使用。
func setNoDataTimeout(d time.Duration) {
	noDataTimeoutNs.Store(int64(d))
}

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
	// cmdTimeout 是 sendCommandACK 的 watchdog 超时；零值时回退到 DAQ_P_1604_TIMEOUT。
	// 暴露为字段供测试覆盖（生产代码保持默认 5s，测试可缩短到 200ms 加速用例），
	// 与 dsa3217.cmdTimeout 标杆模式对齐。
	cmdTimeout time.Duration

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
		// 后台 detach：LSP 环境挂起 Read 时 Close 可能永久阻塞，若同步执行
		// 则 close(timedOut) 永不执行，下方 <-timedOut 永久阻塞（Connect 卡死）。
		go sharedproto.AbortConnection(conn)
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
		if err := d.sendCommand("w1601"); err != nil {
			return fmt.Errorf("enable length prefix: %w", err)
		}
		if err := sharedproto.P1604ReadCommandACK(d.frameReader, conn, 0); err != nil {
			return fmt.Errorf("enable length prefix response: %w", err)
		}
		if err := d.syncUnitFromHardware(0); err != nil {
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
		// 后台 detach：LSP 环境挂起 Read 时 Close 可能永久阻塞，不阻塞 Connect。
		go sharedproto.AbortConnection(conn)
		return err
	}
	return nil
}

func (d *DAQP1604) Disconnect() error {
	d.SetStopReason(sharedproto.StopReasonUserRequested)
	d.mu.RLock()
	wasAcquiring := d.acquiring
	d.mu.RUnlock()
	var stopErr error
	if wasAcquiring {
		stopErr = d.StopAcquisition()
	}

	d.mu.Lock()
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
		// 后台 detach：join 超时说明 readLoop 可能仍挂在 Read 上，
		// LSP 环境下 Close 可能永久阻塞，不阻塞 Disconnect。
		go sharedproto.AbortConnection(conn)
	}
	slog.Info("DAQ-P-1604 TCP disconnected", "category", "hardware-recv", "component", "hardware", "device", d.profile.ID)
	return stopErr
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
	}

	// ADR-009 R0-5 整改：移除 StartAcquisition 前的 DrainConnection 调用。
	//
	// 历史背景：原实现通过 sharedproto.DrainConnection（阻塞 Read + watchdog Close）
	// 排空 TCP 接收缓冲区中的残留数据。但空缓冲是正常状态，watchdog 到期只能证明
	// 探测无法完成，不能证明物理连接故障——问题 Windows 电脑 deadline 失效时
	// 健康连接会被误杀（违反 ADR-009 决策 8）。
	//
	// 整改后：仅依赖 frameReader.Reset() 清空应用层缓冲区；残留字节（如上次 c 02
	// ACK 或 readLoop 退出时设备已排队的压力帧）由后续 sendCommandACK 调用的
	// P1604ReadCommandACK 跳帧逻辑安全跳过（maxResidualFrameSkips=20）。
	// 对齐独立 P1604（p1604_adapter.go StartAcquisition）的整改模式。
	if d.frameReader != nil {
		d.frameReader.Reset()
	}
	d.mu.Unlock()

	if err := d.initStream(); err != nil {
		return fmt.Errorf("init stream: %w", err)
	}
	if err := d.sendCommandACK("c 01 1"); err != nil {
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
	// ADR-009 finding 2：捕获 expectedConn，join 超时后仅关闭此 conn，
	// 避免与并发 Disconnect -> Connect 的新连接误杀。
	expectedConn := d.conn
	d.mu.Unlock()

	// 等待 readLoop 退出后再发送停止命令，避免命令与读取并发。
	if done != nil {
		select {
		case <-done:
		case <-time.After(sharedproto.ReadLoopJoinTimeout):
			d.invalidateConnectionAfterReadLoopTimeout(expectedConn, "read loop did not exit after stop; reconnect required")
			return fmt.Errorf("read loop did not exit after stop; reconnect required")
		}
	}

	if expectedConn != nil {
		if stopErr := d.sendCommandACK("c 02 1"); stopErr != nil {
			return fmt.Errorf("stop stream: %w", stopErr)
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

// invalidateConnection 置连接为 Error 状态并通知上层（onError）。
//
// 使用场景：
//   - readLoop watchdog 触发（ReadFrame 永久阻塞）
//   - StopAcquisition/Disconnect 等待 readLoopDone 超时（readLoop 卡死）
//   - sendCommandACK Write/Read watchdog 触发（P1-2.a / P1-3.d）
//
// 调用方语义：连接已不可用，必须重新 Connect 才能恢复。Close 在锁外执行，
// 避免在 mu.Lock 内做 I/O 与 readLoop 的 Read 竞争。
//
// ADR-009 finding 2 修复：expectedConn 比较避免误杀新连接。
// 调用方必须在触发故障前捕获 d.conn（通常在 RLock 后取引用），并传入此参数。
// 仅当 d.conn 仍是 expectedConn 时才清空 d.conn/frameReader 并置 Error 状态；
// 若 d.conn 已被 Disconnect -> Connect 替换为新连接，仅关闭旧 expectedConn，
// 不修改状态、不通知 onError，避免旧命令的 invalidation 误杀新连接。
func (d *DAQP1604) invalidateConnection(expectedConn net.Conn, message string) {
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
	d.frameReader = nil
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

// invalidateConnectionAfterReadLoopTimeout 在 StopAcquisition/Disconnect 等待
// readLoop 退出超时时调用：强制 Close conn 解除 readLoop 阻塞，并标记连接为 Error。
//
// 命名与 dsa3217.go 标杆对齐，便于跨项目 grep 复用同一模式。
// 实际逻辑委托给 invalidateConnection，避免清理逻辑分叉。
//
// ADR-009 finding 2：同样接收 expectedConn，仅在 d.conn 仍是触发故障时的 conn 时
// 才置 Error 状态；若连接已被替换，仅关闭旧 conn 不修改状态。
func (d *DAQP1604) invalidateConnectionAfterReadLoopTimeout(expectedConn net.Conn, message string) {
	d.invalidateConnection(expectedConn, message)
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
		// ADR-009 R0-5 整改：移除 SetUnit 前的 DrainConnection 调用。
		//
		// 历史背景：原实现通过 sharedproto.DrainConnection（阻塞 Read + watchdog Close）
		// 排空 TCP 接收缓冲区中的残留数据。但空缓冲是正常状态，watchdog 到期只能证明
		// 探测无法完成，不能证明物理连接故障——问题 Windows 电脑 deadline 失效时
		// 健康连接会被误杀（违反 ADR-009 决策 8）。
		//
		// 整改后：仅依赖 frameReader.Reset() 清空应用层缓冲区；残留字节由后续
		// P1604WriteUnitCoefficient 内部的 P1604ReadCommandACK 跳帧逻辑安全跳过。
		// 对齐独立 P1604（p1604_adapter.go ApplyConfig）的整改模式。
		fr.Reset()
		err := sharedproto.P1604WriteUnitCoefficient(fr, conn, coeff, DAQ_P_1604_TIMEOUT)
		d.writeMu.Unlock()
		if err != nil {
			// 记录 warn 日志便于诊断：此前错误只返回前端，后端 log 画面不可见。
			slog.Warn("DAQ-P-1604 write hardware unit coefficient failed",
				"category", "hardware-send", "component", "hardware",
				"device", d.profile.ID, "unit", unit, "coeff", coeff, "error", err)
			// ADR-009 R1-1 + R0-12 + 复核修订 finding 2：对端 FIN/RST 或 soft timeout / watchdog
			// 触发都意味着连接已不可信，必须统一毒化驱动状态——清空 conn/frameReader、置 Error、
			// Close conn 并通知 onError。SetUnit 要求非采集状态（开头已校验
			// d.acquiring==false），readLoop 此时已退出，重置 conn/frameReader 无并发风险。
			//
			// 复核修订 finding 2 修复：必须调用 invalidateConnection(conn, message) 而非
			// 内联清状态。invalidateConnection 内部比较 d.conn == expectedConn，若旧 SetUnit
			// 与 Disconnect -> Connect 并发，新连接不会被旧操作误杀。内联清状态会无条件
			// 清掉当前 d.conn，可能误杀新连接。
			if sharedproto.IsConnResetByPeer(err) || sharedproto.IsWatchdogTriggered(err) {
				d.invalidateConnection(conn, fmt.Sprintf("write hardware unit coefficient: %v", err))
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
// 读取超时、设备错误、格式错误和未知系数均返回错误并中止连接。
//
// ADR-009 R1-1 + R0-12：soft timeout / watchdog 触发时 helper 已 Close conn，
// 本函数必须毒化驱动状态——清空 conn/frameReader、置 Error、Close 旧 conn、通知 onError，
// 避免已死的 conn 被后续 StartAcquisition 复用爆 WSAECONNABORTED。
func (d *DAQP1604) syncUnitFromHardware(timeout time.Duration) error {
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
	coeff, err := sharedproto.P1604ReadUnitCoefficient(fr, conn, timeout)
	d.writeMu.Unlock()
	if err != nil {
		slog.Error("read hardware unit coefficient failed",
			"category", "hardware-recv", "component", "hardware",
			"device", id, "error", err)
		// ADR-009 R1-1 + R0-12 + 复核修订 finding 2：soft timeout / watchdog 触发时 helper 已 Close conn，
		// 必须毒化驱动状态避免后续命令复用已死连接。
		//
		// 复核修订 finding 2 修复：必须调用 invalidateConnection(conn, message) 而非
		// 内联清状态。invalidateConnection 内部比较 d.conn == expectedConn，若旧 syncUnitFromHardware
		// 与 Disconnect -> Connect 并发，新连接不会被旧操作误杀。内联清状态会无条件
		// 清掉当前 d.conn，可能误杀新连接。
		if sharedproto.IsWatchdogTriggered(err) {
			d.invalidateConnection(conn, fmt.Sprintf("read unit coefficient: %v", err))
		}
		return fmt.Errorf("read unit coefficient: %w", err)
	}

	hwUnit, ok := sharedproto.P1604MatchUnitByCoefficient(coeff)
	if !ok {
		err := fmt.Errorf("unknown hardware unit coefficient: %v", coeff)
		slog.Error("hardware unit coefficient unmatched to known units",
			"category", "hardware-recv", "component", "hardware",
			"device", id, "coeff", coeff)
		return err
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
	if err := d.sendCommandACK(fmt.Sprintf("c 00 1 FFFF 1 %d 7 0", periodMs)); err != nil {
		return fmt.Errorf("set stream params: %w", err)
	}

	// 配置流返回内容：0010=压力，0400=设备时间戳，0800=大气数据
	// 掩码计算：压力(0010) + 可选时间戳(0400) + 大气数据(0800)
	contentMask := 0x0010
	if d.profile.UseDeviceTimestampEnabled() {
		contentMask |= 0x0400
	}
	contentMask |= 0x0800 // 始终包含大气数据
	contentMaskHex := fmt.Sprintf("%04X", contentMask)
	if err := d.sendCommandACK(fmt.Sprintf("c 05 1 %s", contentMaskHex)); err != nil {
		return fmt.Errorf("set stream content: %w", err)
	}

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

// effectiveCmdTimeout 返回 sendCommandACK 应使用的 watchdog 超时：
// 字段非零则用字段值，否则回退到 DAQ_P_1604_TIMEOUT 常量。
// 与 dsa3217.effectiveCmdTimeout 标杆模式对齐，供测试覆盖加速。
func (d *DAQP1604) effectiveCmdTimeout() time.Duration {
	if d.cmdTimeout > 0 {
		return d.cmdTimeout
	}
	return DAQ_P_1604_TIMEOUT
}

func (d *DAQP1604) sendCommandACK(cmd string) error {
	// 取 conn / reader 引用并启动外部 watchdog 覆盖 Write 阶段（ADR-009 / P1-2.a）。
	//
	// 此前 sendCommand 仅设 SetWriteDeadline，在 Windows 故障环境下 deadline 不可靠，
	// Write 可能永久阻塞。外部 watchdog 在超时后强制 Close conn 解除阻塞。
	//
	// sendCommandACK 仅在 Connect 完成后的命令路径调用（StartAcquisition /
	// StopAcquisition / initStream），无外层握手 watchdog，可安全启动外部 watchdog。
	// Connect 路径调用 sendCommand + P1604ReadCommandACK(timeout=0)，由
	// runDAQP1604Handshake 外层 watchdog 兜底，不会双重 watchdog。
	d.mu.RLock()
	conn := d.conn
	reader := d.frameReader
	d.mu.RUnlock()
	if conn == nil {
		return fmt.Errorf("not connected")
	}

	timeout := d.effectiveCmdTimeout()
	wdStop := sharedproto.WatchdogClose(conn, timeout)

	// Write 阶段：sendCommand 内部用 writeMu 串行化 + SetWriteDeadline 兜底，
	// 外部 watchdog 作为 ADR-009 兜底机制覆盖 writeMu.Lock 等待 + Write 全流程。
	if err := d.sendCommand(cmd); err != nil {
		// ADR-009 finding 3：Write 任何错误（timeout、broken pipe、RST 等）都意味着
		// 协议边界不可信。watchdog 可能已 Close conn（wdStop 返回 false），也可能未触发
		// 但 Write 返回非 timeout 错误（如 RST）。统一毒化连接，避免下次复用已死 conn。
		if !wdStop() {
			d.invalidateConnection(conn, fmt.Sprintf("sendCommandACK %s write watchdog: %v", cmd, err))
			return fmt.Errorf("%s write: %w; %w", cmd, err, sharedproto.ErrWatchdogTriggered)
		}
		// watchdog 未触发但 Write 失败：conn 可能已死（RST/FIN）或协议不可信。
		// 强制 Close conn 并毒化，返回 ErrWatchdogTriggered 让调用方统一识别。
		d.invalidateConnection(conn, fmt.Sprintf("sendCommandACK %s write error: %v", cmd, err))
		return fmt.Errorf("%s write: %w; %w", cmd, err, sharedproto.ErrWatchdogTriggered)
	}

	// Write 完成，停止外部 watchdog，避免与 P1604ReadCommandACK 内嵌 watchdog 双重计时。
	// 检查 wdStop 返回值：若 watchdog 在 Write 临界点触发（conn 已 Close），
	// 必须立即 invalidate 连接，否则后续 P1604ReadCommandACK 在已死 conn 上读到
	// "use of closed network connection" 错误（不含 "watchdog triggered"），
	// 导致 invalidateConnection 不被调用，连接状态不一致。
	if !wdStop() {
		d.invalidateConnection(conn, fmt.Sprintf("sendCommandACK %s write watchdog triggered after write: conn closed", cmd))
		return fmt.Errorf("%s write: watchdog triggered after write completed", cmd)
	}

	// Read 阶段：P1604ReadCommandACK 内嵌 watchdog（timeout>0）保护 Read 阻塞，
	// 并处理跳帧循环。外部 watchdog 已停止，不会干扰。
	if err := sharedproto.P1604ReadCommandACK(reader, conn, timeout); err != nil {
		// P1604ReadCommandACK 内嵌 watchdog 触发时，错误含 "watchdog triggered"，
		// 检测到后同样清理连接状态（P1-3.d），避免下次复用已 Close 的 conn。
		// 使用 sharedproto.IsWatchdogTriggered 替代字符串匹配，避免依赖错误消息字面量。
		if sharedproto.IsWatchdogTriggered(err) {
			d.invalidateConnection(conn, fmt.Sprintf("sendCommandACK %s read watchdog: %v", cmd, err))
		}
		return fmt.Errorf("%s response: %w", cmd, err)
	}
	return nil
}

func (d *DAQP1604) readLoop(stop <-chan struct{}) {
	// ADR-009 R0-10：no-data owner 必须独立于 read goroutine 与其 mutex。
	//
	// 历史背景：原实现 readLoop 通过 SetReadDeadline(200ms) 让 ReadFrame 周期返回，
	// 在循环体累计 `time.Since(lastDataAt) > noDataTimeout` 检测无数据。问题 Windows 电脑
	// deadline 失效时循环体不可达，noDataTimeout 永远不会被触发，半开连接无法自行收敛。
	//
	// 整改后：readLoop 入口启动独立 time.AfterFunc timer，到期通过 expected conn 比较
	// 安全毒化连接（清空 conn/frameReader、置 Error 状态、Close conn）。timer 不依赖
	// readLoop 循环体执行，即使 Read 永久阻塞也能到期触发。每次收到有效帧调用 Reset 续期；
	// readLoop 退出 defer Stop。
	//
	// 不调 onError：让 readLoop defer 的 unexpectedErr 路径统一调用 invalidateConnection
	// 通知上层，避免同一故障被 onError 回调两次。timer Close 后 readLoop 的 Read 返回
	// 错误进入 defer，defer 调用 invalidateConnection 统一清理（重置 status=Error、
	// 清空 conn/frameReader、调 onError）。
	//
	// expected conn 比较避免与重连后的新 conn 竞争：Stop/Disconnect 已置 d.conn=nil
	// 或重连后 d.conn 是新连接，timer 触发时 d.conn != expectedConn 直接跳过。
	// acquiring 检查避免 Stop 后 readLoop 尚未退出时 timer 误触发：stopAcquisitionLocked
	// 先置 d.acquiring=false 再等 done，timer 检测到 acquiring=false 直接跳过。
	d.mu.RLock()
	expectedConn := d.conn
	d.mu.RUnlock()

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
		// 统一毒化：清空 conn/frameReader、置 Error 状态、保存 LastError。
		// 不调 onError，让 readLoop defer 统一通知（避免双重回调）。
		d.conn = nil
		d.frameReader = nil
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
		slog.Warn("DAQ-P-1604 no data timeout, conn closed by watchdog",
			"device", d.profile.ID, "duration", noDataTimeoutSnapshot)
	})
	// readLoop 退出时停止 timer，避免 timer 在 readLoop 已退出后误触发。
	// Stop 不等待已 firing 的回调完成，但回调内 acquiring + expected conn 双重检查
	// 能正确处理 readLoop 退出后 timer 才 fire 的场景。
	defer noDataTimer.Stop()

	var unexpectedErr error

	defer func() {
		if unexpectedErr != nil {
			// 主动停止场景不视为异常，避免误触发 onError。
			if d.GetStopReason() == sharedproto.StopReasonUserRequested {
				return
			}

			// ADR-009 R0-11：terminal read error 必须调用 invalidateConnection 统一毒化连接——
			// 清空 d.conn/d.frameReader、close conn、置 Error 状态、保存 LastError、通知 onError。
			// 历史背景：原 defer 仅设置 status=Error 但未清空 conn/frameReader 也未 close conn，
			// EOF/RST 后连接已死，下次 StartAcquisition 会用旧 conn 发命令爆 WSAECONNABORTED。
			//
			// invalidateConnection 不清空 d.stop（它是采集生命周期字段，非连接状态），
			// 此处显式清空对齐原 defer 行为，避免 readLoop 退出后 stop channel 残留。
			slog.Warn("DAQ-P-1604 read loop exited unexpectedly", "device", d.profile.ID, "error", unexpectedErr)
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
		// SetReadDeadline 仍保留作为单次 Read 的软超时，让循环体能周期性
		// 重新检查 stop channel（ADR-009 R0-10：no-data 检测由独立 timer 负责，
		// deadline 失效场景由 timer 兜底 Close conn）。
		_ = conn.SetReadDeadline(time.Now().Add(200 * time.Millisecond))

		payload, err := fr.ReadFrame()
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				// deadline 软超时：循环体重新检查 stop channel，不视为异常。
				// no-data 检测由独立 noDataTimer 负责，不依赖循环体执行。
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
			// 收到有效帧，续期 no-data timer。
			// Reset 是原子操作，无需加锁；即使 timer 已 fire 也能安全 Reset（time.AfterFunc 文档保证）。
			// 使用入口快照的 noDataTimeoutSnapshot 而非全局 noDataTimeout，避免与
			// 测试 t.Cleanup 写入全局变量并发 race。
			noDataTimer.Reset(noDataTimeoutSnapshot)
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
	// 部分旧固件会 ACK 0x0400 时间戳配置，但仍返回不含时间戳的 77 字节旧帧。
	// 这类帧已由长度前缀正确分帧，可安全回退到主机接收时间解析。
	if err != nil && useDeviceTs && len(data) == sharedproto.StreamFrameHeaderSize+18*4 {
		channels, deviceTimestampMs, _, err = sharedproto.ParseStreamFrameEx(data, false, true)
	}
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
