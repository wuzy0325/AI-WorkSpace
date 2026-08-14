package hardware

import (
	"errors"
	"fmt"
	"log/slog"
	"math"
	"net"
	"runtime/debug"
	"sync"
	"time"

	"shared.local/device-sdk/go/daq/core"
	"shared.local/device-sdk/go/protocol"
	"shared.local/device-sdk/go/protocol/modbus"
)

// DAQ-T-1602 温度扫描阀驱动：Modbus TCP 单连接双 Unit ID（卡1=1 卡2=2），
// 每卡 8 通道合并对外 16 通道。协议参数全部来自 spec-daq-t1602 §Protocol
// 真机实测（2026-08-12，192.168.3.201:502）。

const (
	DAQ_T_1602_DEFAULT_HOST = "192.168.3.201"
	DAQ_T_1602_DEFAULT_PORT = 502
	DAQ_T_1602_TIMEOUT      = 5 * time.Second
)

const (
	t1602CardCount       = 2
	t1602ChannelsPerCard = 8
	t1602ChannelCount    = t1602CardCount * t1602ChannelsPerCard
	t1602TypeRegBase     = 200 // 热电偶类型 Holding 寄存器起始地址
	t1602DataRegBase     = 0   // 采集数据 Input 寄存器起始地址
	t1602UnitIDCard1     = 1
	t1602UnitIDCard2     = 2
)

// t1602UnitIDs 卡序号 → Unit ID（卡1=1，卡2=2）。
var t1602UnitIDs = [t1602CardCount]uint8{t1602UnitIDCard1, t1602UnitIDCard2}

// t1602ReadLoopJoinTimeout 限制 StopAcquisition 等待 readLoop 退出的预算。
// 单请求 RTT ~103ms + 响应超时 1s，2s 预算留足余量；超时由 Stop owner
// 直接 Close 连接解除阻塞（ADR-009），连接不可复用。
const t1602ReadLoopJoinTimeout = 2 * time.Second

// t1602ThermocoupleRanges Type Code → 量程 [min,max] ℃（用户提供量程表，
// 2026-08-13）。已真机验证：K 型通道 raw 1513 → 27.7℃ 与室温吻合；
// 无信号通道 raw=0 → 0℃（旧标准表会解出 -270℃，明显错误）。
var t1602ThermocoupleRanges = [8][2]float64{
	{-50, 50},   // 0 = J
	{0, 1200},   // 1 = K
	{0, 1300},   // 2 = T
	{-200, 400}, // 3 = E
	{0, 1000},   // 4 = R
	{0, 1700},   // 5 = S
	{0, 1768},   // 6 = B
	{0, 1800},   // 7 = N
}

type onT1602ConfigSyncedFn func(core.DaqT1602HardwareConfig)

// DAQT1602 DAQ-T-1602 驱动。单 TCP 连接（IP:502）+ Unit ID 1/2 复用寻址双卡；
// 采集为主动轮询（FC4 双卡串行各读 8 寄存器，~206ms/帧 ≈ 4.9Hz），无推送流。
type DAQT1602 struct {
	mu             sync.RWMutex
	logMu          sync.RWMutex
	profile        core.Profile
	status         core.Status
	sink           core.DataSink
	config         core.DaqT1602HardwareConfig
	conn           net.Conn
	mb             *modbus.Conn
	stop           chan struct{}
	readLoopDone   chan struct{}
	acquiring      bool
	onConfigSynced onT1602ConfigSyncedFn
	onReadLoopExit func(error)
	onLog          func(LogEntry)
	dialTCP        func(string, string, time.Duration) (net.Conn, error)

	// 轮询间隔提供者（nil=全速，默认行为）。采集频率跟随全局刷新频率设置，
	// 见 daq_t1602_poll.go 与 spec-daq-t1602 §前端集成约定。
	pollIntervalFn func() time.Duration
	// 帧节拍状态：仅 readLoop goroutine 访问（waitForNextTick），无需加锁。
	lastInterval time.Duration
	nextTick     time.Time
}

func NewDAQT1602(profile core.Profile) *DAQT1602 {
	return &DAQT1602{
		profile: profile,
		config:  profile.DaqT1602Config,
		dialTCP: protocol.DialTCP,
		status: core.Status{
			ID:         profile.ID,
			Name:       profile.Name,
			Type:       profile.Type,
			Connection: core.ConnectionDisconnected,
		},
	}
}

func (d *DAQT1602) ID() string { return d.profile.ID }

func (d *DAQT1602) OnConfigSynced(fn onT1602ConfigSyncedFn) {
	d.mu.Lock()
	d.onConfigSynced = fn
	d.mu.Unlock()
}

func (d *DAQT1602) OnReadLoopExit(fn func(error)) {
	d.mu.Lock()
	d.onReadLoopExit = fn
	d.mu.Unlock()
}

func (d *DAQT1602) OnLog(fn func(LogEntry)) {
	d.logMu.Lock()
	d.onLog = fn
	d.logMu.Unlock()
}

func (d *DAQT1602) emitLog(level string, category string, message string, detail string) {
	d.logMu.RLock()
	fn := d.onLog
	d.logMu.RUnlock()
	if fn == nil {
		return
	}
	fn(LogEntry{
		Level:    level,
		Category: category,
		DeviceID: d.profile.ID,
		Message:  message,
		Detail:   detail,
	})
}

func (d *DAQT1602) Connect() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.connectLocked()
}

func (d *DAQT1602) connectLocked() error {
	if d.conn != nil {
		return nil
	}
	conn, err := d.dialT1602Locked()
	if err != nil {
		return err
	}

	mb := modbus.NewConn(conn)
	// Connect 即读回 16 通道类型（双卡 FC3 读 Holding 200~207）。
	types, err := readAllT1602ChannelTypes(mb)
	if err != nil {
		_ = mb.Close()
		if errors.Is(err, modbus.ErrConnBroken) {
			d.status.Connection = core.ConnectionError
			d.status.LastError = fmt.Sprintf("config sync failed: %v", err)
		}
		return fmt.Errorf("sync channel types: %w", err)
	}

	d.conn = conn
	d.mb = mb
	d.config.TypeCodes = types
	d.profile.DaqT1602Config.TypeCodes = types
	d.status.Connection = core.ConnectionConnected
	d.status.LastError = ""

	// 回调在 d.mu 持有期间触发（与包内既有驱动约定一致）：调用方（适配器
	// Connect）必须已释放自身锁，否则回调重入适配器锁会自死锁。
	if fn := d.onConfigSynced; fn != nil {
		fn(d.config)
	}
	slog.Info("DAQ-T-1602 config sync completed", "device", d.profile.ID)
	d.emitLog("info", "system", "Config sync completed", fmt.Sprintf("typeCodes=%v", types))
	return nil
}

// dialT1602Locked 解析地址并拨号。ADR-009：protocol.DialTCP 带 watchdog
// goroutine 兜底，不依赖 Dial 内部 deadline 在故障 Windows 电脑上生效，
// 避免 Connect 永久卡死。调用方必须持有 d.mu。
func (d *DAQT1602) dialT1602Locked() (net.Conn, error) {
	host := d.profile.Address
	if host == "" {
		host = DAQ_T_1602_DEFAULT_HOST
	}
	port := d.profile.Port
	if port <= 0 {
		port = DAQ_T_1602_DEFAULT_PORT
	}
	conn, err := d.dialTCP(fmt.Sprintf("%s:%d", host, port), "", DAQ_T_1602_TIMEOUT)
	if err != nil {
		return nil, fmt.Errorf("connect to %s:%d: %w", host, port, err)
	}
	if tcp, ok := conn.(*net.TCPConn); ok {
		tcp.SetKeepAlive(true)
		tcp.SetKeepAlivePeriod(10 * time.Second)
	}
	return conn, nil
}

// readAllT1602ChannelTypes 串行读双卡 Holding 200~207，合并为 16 通道类型码。
func readAllT1602ChannelTypes(mb *modbus.Conn) ([t1602ChannelCount]uint8, error) {
	var types [t1602ChannelCount]uint8
	for card, unitID := range t1602UnitIDs {
		regs, err := mb.ReadHoldingRegisters(unitID, t1602TypeRegBase, t1602ChannelsPerCard)
		if err != nil {
			return types, fmt.Errorf("read card %d channel types: %w", card+1, err)
		}
		for i, v := range regs {
			types[card*t1602ChannelsPerCard+i] = uint8(v)
		}
	}
	return types, nil
}

func (d *DAQT1602) Disconnect() error {
	d.mu.Lock()
	if d.acquiring {
		d.mu.Unlock()
		_ = d.StopAcquisition()
		d.mu.Lock()
	}
	mb := d.mb
	d.conn = nil
	d.mb = nil
	d.acquiring = false
	d.status.Acquiring = false
	d.status.Connection = core.ConnectionDisconnected
	d.mu.Unlock()
	if mb != nil {
		_ = mb.Close()
	}
	return nil
}

func (d *DAQT1602) StartAcquisition() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.acquiring {
		return nil
	}
	if d.conn == nil || d.mb == nil {
		return fmt.Errorf("device not connected")
	}
	d.stop = make(chan struct{})
	d.readLoopDone = make(chan struct{})
	d.acquiring = true
	d.status.Acquiring = true
	d.status.Connection = core.ConnectionAcquiring
	go d.readLoop(d.conn, d.mb, d.stop, d.readLoopDone)
	return nil
}

// readLoop 双卡串行轮询：FC4 读卡1 Input 0~7 → 卡2 Input 0~7 → 合并 16 通道
// 换算 ℃ emit。单请求 RTT ~103ms（固件串行节流），双卡一帧 ~206ms（~4.9Hz）。
//
// 异常处理：任何轮询错误（超时/连接断开/协议错位，modbus.Conn 内部已毒化连接）
// 都视为异常退出——触发 OnReadLoopExit 并统一清理状态（invalidateConnection）。
// 单请求有 1s 响应超时 + watchdog 兜底，半开连接会在一次轮询内收敛，
// 无需独立 no-data timer。
func (d *DAQT1602) readLoop(conn net.Conn, mb *modbus.Conn, stop chan struct{}, done chan struct{}) {
	var unexpectedErr error
	defer func() {
		if r := recover(); r != nil {
			d.emitLog("error", "system", "readLoop panic recovered",
				fmt.Sprintf("%v\n%s", r, debug.Stack()))
		}
	}()
	defer func() {
		close(done)
		if unexpectedErr != nil {
			d.emitLog("warn", "system", "Read loop exited unexpectedly", unexpectedErr.Error())
			d.invalidateConnection(conn, unexpectedErr.Error())
			d.mu.RLock()
			fn := d.onReadLoopExit
			d.mu.RUnlock()
			if fn != nil {
				fn(unexpectedErr)
			}
		}
	}()
	for {
		select {
		case <-stop:
			return
		default:
		}
		if !d.waitForNextTick(stop) {
			return
		}
		raw, err := d.pollOnce(mb)
		if err != nil {
			select {
			case <-stop:
				return // 主动停止期间的轮询错误属预期退出
			default:
			}
			unexpectedErr = err
			return
		}
		d.emitTemps(raw)
	}
}

// pollOnce 串行轮询双卡输入寄存器（FC4 读 0~7），合并为 16 通道 raw 值。
func (d *DAQT1602) pollOnce(mb *modbus.Conn) ([]uint16, error) {
	raw := make([]uint16, 0, t1602ChannelCount)
	for card, unitID := range t1602UnitIDs {
		regs, err := mb.ReadInputRegisters(unitID, t1602DataRegBase, t1602ChannelsPerCard)
		if err != nil {
			return nil, fmt.Errorf("poll card %d: %w", card+1, err)
		}
		raw = append(raw, regs...)
	}
	return raw, nil
}

// emitTemps 按通道类型码量程线性换算 16 通道 raw 值为 ℃ 并推送。
// 驱动层只输出工程值 ℃，不做其他单位换算。
func (d *DAQT1602) emitTemps(raw []uint16) {
	d.mu.RLock()
	sink := d.sink
	cfg := d.config
	d.mu.RUnlock()
	if sink == nil || len(raw) < t1602ChannelCount {
		return
	}
	temps := make([]float64, t1602ChannelCount)
	indices := make([]int, t1602ChannelCount)
	for i := 0; i < t1602ChannelCount; i++ {
		temps[i] = t1602RawToTemp(raw[i], cfg.TypeCodes[i])
		indices[i] = i
	}
	sink(core.DataPayload{
		DeviceID:       d.profile.ID,
		Timestamp:      core.NowMs(),
		Channels:       temps,
		ChannelIndices: indices,
	})
}

// t1602RawToTemp 线性换算：temp = raw/65535×(max-min)+min。
// 公式经用户确认（LabVIEW 逆向 + 真机验证 2026-08-13：K 型 raw 1513 → 27.7℃
// 与室温吻合；T 型配置 raw 1369 → 27.2℃ 交叉一致）。
//
// raw == 0 表示该通道未接入热电偶（设备开路输出 0，2026-08-13 用户确认），
// 不是"量程下限温度"——返回 NaN 表示无测量值，由上层序列化为 null、
// UI 显示 "--"、波形图留空。注意 T 型量程下限恰为 0℃ 时真实 0℃ 测量与
// 未接入不可区分，这是该约定的已知边界（用户确认可接受）。
func t1602RawToTemp(raw uint16, typeCode uint8) float64 {
	if raw == 0 {
		return math.NaN()
	}
	r := t1602RangeOf(typeCode)
	return float64(raw)/65535*(r[1]-r[0]) + r[0]
}

// t1602RangeOf 返回类型码对应量程；未知类型码按 N 型量程兜底。
func t1602RangeOf(typeCode uint8) [2]float64 {
	if int(typeCode) < len(t1602ThermocoupleRanges) {
		return t1602ThermocoupleRanges[typeCode]
	}
	return t1602ThermocoupleRanges[7]
}

func (d *DAQT1602) StopAcquisition() error {
	d.mu.Lock()
	if !d.acquiring || d.stop == nil {
		d.finishStopLocked()
		d.mu.Unlock()
		return nil
	}
	close(d.stop)
	d.stop = nil
	done := d.readLoopDone
	conn := d.conn
	d.mu.Unlock()

	select {
	case <-done:
		d.mu.Lock()
		d.finishStopLocked()
		d.mu.Unlock()
		return nil
	case <-time.After(t1602ReadLoopJoinTimeout):
		// ADR-009：join 超时由 Stop owner 直接 Close 连接解除阻塞 readLoop，
		// 连接不可复用（modbus.Conn 单请求 1s 超时正常不会走到这里，
		// 此分支兜底"watchdog 也失效"的极端场景）。
		d.invalidateConnection(conn, "readLoop did not exit before StopAcquisition timeout; reconnect required")
		return fmt.Errorf("readLoop did not exit before StopAcquisition timeout; reconnect required")
	}
}

// finishStopLocked 结束采集状态。调用方必须持有 d.mu。
// 若连接已被 readLoop 异常毒化（Error），保持 Error 不回退 Connected。
func (d *DAQT1602) finishStopLocked() {
	d.acquiring = false
	d.status.Acquiring = false
	d.readLoopDone = nil
	if d.status.Connection == core.ConnectionAcquiring {
		d.status.Connection = core.ConnectionConnected
	}
}

// invalidateConnection 统一毒化连接：expectedConn 比较避免误杀重连后的新连接，
// 清空 conn/mb、置 Error 状态、Close conn。调用方不得持有 d.mu。
func (d *DAQT1602) invalidateConnection(expectedConn net.Conn, reason string) {
	d.mu.Lock()
	if d.conn != expectedConn {
		d.mu.Unlock()
		return
	}
	mb := d.mb
	d.conn = nil
	d.mb = nil
	d.acquiring = false
	d.status.Acquiring = false
	d.status.Connection = core.ConnectionError
	d.status.LastError = reason
	d.mu.Unlock()
	if mb != nil {
		_ = mb.Close()
	}
	d.emitLog("error", "system", "Connection invalidated", reason)
}

func (d *DAQT1602) SetDataSink(sink core.DataSink) {
	d.mu.Lock()
	d.sink = sink
	d.mu.Unlock()
}

func (d *DAQT1602) Status() core.Status {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.status
}

func (d *DAQT1602) GetDaqT1602Config() (core.DaqT1602HardwareConfig, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.config, nil
}

// ApplyDaqT1602Config 应用 16 通道热电偶类型配置。已连接时逐通道 FC6 写
// 类型寄存器（200~207，双卡 Unit ID 1/2），写完后 FC3 读回校验；未连接时
// 仅保存到本地（下次 Connect 读回设备实际值覆盖）。
func (d *DAQT1602) ApplyDaqT1602Config(cfg core.DaqT1602HardwareConfig) error {
	d.mu.RLock()
	acquiring := d.acquiring
	conn := d.conn
	mb := d.mb
	d.mu.RUnlock()
	if acquiring {
		return fmt.Errorf("cannot apply DAQ-T-1602 config while acquiring")
	}
	if mb != nil {
		if err := d.writeAllChannelTypes(mb, cfg.TypeCodes); err != nil {
			// ErrConnBroken：modbus.Conn 已毒化并 Close 连接，驱动同步清理状态；
			// ExceptionError（设备拒绝）属业务错误，连接仍可复用，不毒化。
			if errors.Is(err, modbus.ErrConnBroken) {
				d.invalidateConnection(conn, fmt.Sprintf("apply config: %v; reconnect required", err))
			}
			return err
		}
	}
	d.mu.Lock()
	d.config = cfg
	d.profile.DaqT1602Config = cfg
	d.mu.Unlock()
	return nil
}

// writeAllChannelTypes 逐通道 FC6 写类型寄存器，写完后 FC3 读回 16 通道校验
// （spec §Success Criteria 3：ApplyConfig → 逐通道写 → 读回校验）。
func (d *DAQT1602) writeAllChannelTypes(mb *modbus.Conn, types [t1602ChannelCount]uint8) error {
	for ch := 0; ch < t1602ChannelCount; ch++ {
		unitID, addr := t1602ChannelRegister(ch)
		if err := mb.WriteSingleRegister(unitID, addr, uint16(types[ch])); err != nil {
			return fmt.Errorf("write channel %d type: %w", ch, err)
		}
	}
	actual, err := readAllT1602ChannelTypes(mb)
	if err != nil {
		return fmt.Errorf("read back channel types: %w", err)
	}
	for ch := 0; ch < t1602ChannelCount; ch++ {
		if actual[ch] != types[ch] {
			return fmt.Errorf("verify channel %d type: wrote %d, read back %d", ch, types[ch], actual[ch])
		}
	}
	return nil
}

// t1602ChannelRegister 通道索引 → （Unit ID, 类型寄存器地址）。
// 索引 0~7 → 卡1（Unit ID 1）200~207；索引 8~15 → 卡2（Unit ID 2）200~207。
func t1602ChannelRegister(ch int) (uint8, uint16) {
	return t1602UnitIDs[ch/t1602ChannelsPerCard], t1602TypeRegBase + uint16(ch%t1602ChannelsPerCard)
}
