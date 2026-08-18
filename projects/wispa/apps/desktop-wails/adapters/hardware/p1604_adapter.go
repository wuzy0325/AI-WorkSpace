package hardware

import (
	"context"
	"fmt"
	"math"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"shared.local/device-sdk/go/pkg/slog"
	sharedproto "shared.local/device-sdk/go/protocol"

	"wispa/core"
	"wispa/ports"
)

const (
	p1604DefaultHost    = "192.168.3.101"
	p1604DefaultPort    = 9000
	p1604ConnectTimeout = 5 * time.Second
	p1604CommandTimeout = 2 * time.Second
	p1604ReadTimeout    = 200 * time.Millisecond
	p1604NumChannels    = 18
	// readLoop 等待超时使用 sharedproto.ReadLoopJoinTimeout（跨项目统一）
	// 主动停止原因使用 sharedproto.StopReasonUserRequested（跨项目统一）

	// p1604ShardCount 分片锁数量（必须为 2 的幂，用于位与替代取模）。
	// 10 设备场景下平均每锁仅 0.625 设备竞争，显著降低 readLoop 读锁与
	// Connect/Disconnect/StartAcquisition 写锁的争用。
	p1604ShardCount = 16
	// p1604ShardMask 用于位与替代取模（要求 shard 数为 2 的幂）
	p1604ShardMask = p1604ShardCount - 1

	// p1604UnitSyncTimeout 连接后读取硬件单位响应的超时时间。
	// 设备通常在 100ms 内返回 u01101 响应，2s 余量足够应对网络抖动。
	p1604UnitSyncTimeout = 2 * time.Second
	// 覆盖 w1601 应答排空和单位同步；到期后直接关闭 socket，确保 Windows 上
	// 偶发不响应 read deadline 的网络调用也能解除阻塞。
	p1604HandshakeTimeout = 4 * time.Second
	// p1604CalibrationTimeout 零点校准（h 命令）等待设备响应的最长时间。
	// 设备执行零点校准需要数百毫秒（内部 ADC 求平均），2s 余量足够覆盖网络抖动与
	// 采集期间 readLoop 处理积压二进制帧的延迟。超时即视为设备无响应或连接已死。
	p1604CalibrationTimeout = 2 * time.Second

	// p1604KeepAlivePeriod TCP keepalive 探测间隔。
	// 作为应用层连续超时计数器（5s）的兜底：
	// - 采集期间 readLoop 活跃时，连续超时计数器是主要检测手段（5s）
	// - 非采集期间（仅连接无采集）keepalive 是唯一手段（~110s）
	// - 受限环境（低权限/沙箱）中 keepalive 可能不可用，此时回退到系统 TCP 超时
	//   （Windows 默认 2 小时，Linux 默认 2 小时 11 分）
	//
	// Windows 上 SetKeepAlivePeriod 同时设置 KEEPIDLE 与 KEEPINTVL：
	//   - Linux：3s idle + 1s interval × 9 probes ≈ 12s
	//   - Windows：3s idle + 3s interval × 10 retries ≈ 33s
	// 比原来 110s 快 3 倍以上。
	p1604KeepAlivePeriod = 3 * time.Second
)

// p1604WatchdogTimeout 是 Windows 上 SetReadDeadline 可能失效时的硬兜底超时。
//
// 设计依据 ADR-009：现场一台 Windows 电脑（与 192.168.1.7:9000 设备配套使用）
// 可重复出现 SetReadDeadline 到期后阻塞 Read 仍不返回的现象。仅靠 deadline
// 不足以保证有界网络 I/O，必须由独立 timer 调用 conn.Close() 解除阻塞。
//
// 5s 取值理由：
//   - 正常 idle Read 200ms deadline，5s 是 25 倍余量，绝不误杀
//   - 正常命令 ACK 2s deadline，5s 是 2.5 倍余量
//   - StartAcquisition 命令链正常 < 200ms，5s 足以覆盖网络抖动
//   - 用户可接受的最长单步等待时间，超时即报错让用户重连
//
// 使用 var 而非 const 是为了支持测试覆盖：deadline-ignore 回归测试可临时
// 缩短为 200ms 以加速验证（参考 TestStartAcquisition_WatchdogClosesConnOnDeadlineFailure）。
// 串行测试（默认）下无并发风险；并发测试需使用 t.Parallel 前自行 sync 化。
var p1604WatchdogTimeout = 5 * time.Second

// p1604CommandResponseTimeout 是 sendCommandACK 单次 ReadFrame 的 soft deadline。
//
// 取值 2s：设备正常 ACK 在 100ms 内返回，2s 余量覆盖网络抖动。
// 与 p1604WatchdogTimeout（5s）配合形成双层保护：
//   - 正常 Windows 环境：SetReadDeadline 兑现，2s soft deadline 先触发
//   - 故障 Windows 电脑：SetReadDeadline 失效，5s watchdog 兜底 Close conn
//
// ADR-009 R0-12：soft deadline 先触发时也必须 Close conn 阻断迟到响应，
// 不能仅返回 timeout 错误保留 conn。迟到 ACK 会被下一条命令消费导致协议错位。
//
// 使用 var 而非 const 是为了允许测试注入短超时（100ms）加速 soft timeout 用例；
// 运行期不应修改，生产代码保持默认 2s。同一包内测试默认串行执行，覆盖安全。
var p1604CommandResponseTimeout = 2 * time.Second

// p1604NoDataTimeout 是允许的最长无数据时间，超过则判定为连接异常。
// readLoop 入口启动独立 time.AfterFunc timer，到期触发 handleConnectionLost。
//
// 取值 5s（对齐原 consecutiveTimeouts 阈值 25 × 200ms = 5s）：
// 正常采集（1kHz）时设备每秒发约 1000 帧，200ms 内至少可读到 200 帧，
// 因此 5s 无任何数据一定是有问题（网络中断/设备断电/网线脱落）。
//
// ADR-009 R0-10：原实现通过循环体累计连续超时次数检测无数据，依赖 readLoop
// 循环体执行。问题 Windows 电脑 deadline 失效时循环体不可达，5s 检出永远
// 不会触发，半开连接无法自行收敛。独立 timer 不依赖循环体，即使 Read 永久
// 阻塞也能到期触发。
//
// 搭配 TCP keepalive 形成双保险：
//   - 快速通道（应用层）：no-data timer → 5s 检出，不依赖 readLoop 活跃
//   - 慢速兜底（内核层）：TCP keepalive → Windows ~33s / Linux ~12s 检出
//     （p1604KeepAlivePeriod=3s × 系统默认探测次数）
//     readLoop 空闲（非采集）时仍是唯一主动检测手段
//
// 两路径任一触发都会调用 handleConnectionLost。
//
// 使用 var 而非 const 是为了允许测试注入短超时（200ms）加速 no-data timer 用例；
// 运行期不应修改，生产代码保持默认 5s。同一包内测试默认串行执行，覆盖安全。
var p1604NoDataTimeout = 5 * time.Second

// DeviceLogEntry 设备日志条目
type DeviceLogEntry struct {
	Level    string
	Category string
	DeviceID string
	Message  string
	Detail   string
}

// p1604Shard 单个分片：持有该分片下所有设备的状态、驱动、采集槽位
type p1604Shard struct {
	mu         sync.RWMutex
	drivers    map[string]*p1604Driver
	connecting map[string]struct{}
	status     map[string]*core.DeviceState
	sinks      map[string]func(core.PressureSnapshot)
	channels   map[string]chan core.PressureSnapshot
	stopChs    map[string]chan struct{}
}

// P1604Adapter WISPA 硬件适配器（16 分片锁）
//
// 分片策略：按 deviceID FNV-1a 哈希 & 0x0F 选 16 分片之一。
// 10 设备 × 1kHz = 1 万次/sec 读锁访问分散到 16 把锁，平均每锁 625 次/sec，
// 显著降低单 mutex 争用。
type P1604Adapter struct {
	shards    [p1604ShardCount]*p1604Shard
	logSink   func(DeviceLogEntry)
	stateSink func(id string, state core.DeviceState) // 连接状态变更回调，用于通知前端
	// logSink/stateSink 单独用一把锁保护，避免与设备分片锁耦合
	metaMu sync.RWMutex
}

// p1604Driver 单个 P1604 设备的驱动实例
type p1604Driver struct {
	profile     core.PressureProfile
	conn        net.Conn
	frameReader *sharedproto.FrameReader
	acquiring   bool
	// emit 日志回调（由 adapter 注入），用于在 sendCommand 等底层通信路径
	// 打印硬件通信日志（category=hardware-send/hardware-recv），便于前端
	// "通信" 分组展示。driver 不持有 adapter 引用，避免环形依赖。
	emit func(DeviceLogEntry)
	// readLoopDone 由 readLoop 在退出时关闭。
	// Disconnect / StopAcquisition 在 close(stop) 之后等待此 channel，确保
	// readLoop 不再持有 conn 引用，再安全 conn.Close；同时也避免 Disconnect 返回
	// 后老 readLoop 仍在运行、错误清理后续新建立的同 ID 设备状态。
	readLoopDone chan struct{}
	// idleStopCh 由 Connect 创建，close(idleStopCh) 通知 idleReadLoop 退出。
	// idleReadLoop 仅在非采集期间活跃，负责感知 TCP keepalive 失败（CONN-008）。
	// StartAcquisition 启动 readLoop 前必须先 close 它并 join idleLoopDone，
	// 避免 readLoop 和 idleReadLoop 同时操作 frameReader 造成数据竞争。
	idleStopCh chan struct{}
	// idleLoopDone 由 idleReadLoop 在退出时关闭。
	// Disconnect / StopAcquisition 在 close(idleStopCh) 后等待此 channel，
	// 确保 idleReadLoop 不再持有 conn 引用再操作连接。
	idleLoopDone chan struct{}
	// 主动停止原因追踪：嵌入 sharedproto.StopReasonTracker
	// 提供 SetStopReason / GetStopReason / ClearStopReason 方法，跨项目复用
	sharedproto.StopReasonTracker
	// pendingResponseMu 保护 pendingResponse 通道，确保采集期间 ZeroCalibration
	// 与 readLoop/processPayload 之间的请求/响应协调无竞态。
	// 设计：发送命令前注册通道，readLoop 收到 ASCII 响应后投递到通道，
	// ZeroCalibration 通过 select+timeout 等待响应或超时。
	pendingResponseMu sync.Mutex
	pendingResponse   chan []byte
	// operationMu 串行化会改变连接读取者或等待命令响应的操作，防止采集启停
	// 与校零同时读写同一 conn/frameReader。
	operationMu sync.Mutex
}

// NewP1604Adapter 创建 P1604 硬件适配器
func NewP1604Adapter() *P1604Adapter {
	a := &P1604Adapter{}
	for i := range a.shards {
		a.shards[i] = &p1604Shard{
			drivers:    make(map[string]*p1604Driver),
			connecting: make(map[string]struct{}),
			status:     make(map[string]*core.DeviceState),
			sinks:      make(map[string]func(core.PressureSnapshot)),
			channels:   make(map[string]chan core.PressureSnapshot),
			stopChs:    make(map[string]chan struct{}),
		}
	}
	return a
}

var _ ports.DevicePort = (*P1604Adapter)(nil)

// shardIndexForDevice FNV-1a 哈希分片（无外部依赖，对短字符串分布良好）
func shardIndexForDevice(deviceID string) int {
	var h uint32 = 2166136261 // FNV offset basis
	for i := 0; i < len(deviceID); i++ {
		h ^= uint32(deviceID[i])
		h *= 16777619 // FNV prime
	}
	return int(h & p1604ShardMask) // 位与替代取模（要求 shard 数为 2 的幂）
}

// shard 取设备对应的分片
func (a *P1604Adapter) shard(id string) *p1604Shard {
	return a.shards[shardIndexForDevice(id)]
}

// SetLogSink 设置日志回调
func (a *P1604Adapter) SetLogSink(sink func(DeviceLogEntry)) {
	a.metaMu.Lock()
	a.logSink = sink
	a.metaMu.Unlock()
}

// SetStateSink 设置连接状态变更回调
func (a *P1604Adapter) SetStateSink(sink func(id string, state core.DeviceState)) {
	a.metaMu.Lock()
	a.stateSink = sink
	a.metaMu.Unlock()
}

func (a *P1604Adapter) emitLog(entry DeviceLogEntry) {
	a.metaMu.RLock()
	sink := a.logSink
	a.metaMu.RUnlock()
	if sink != nil {
		sink(entry)
	}
}

// emitState 通知前端设备状态变更
func (a *P1604Adapter) emitState(id string) {
	a.metaMu.RLock()
	sink := a.stateSink
	a.metaMu.RUnlock()
	if sink == nil {
		return
	}
	shard := a.shard(id)
	shard.mu.RLock()
	st, exists := shard.status[id]
	shard.mu.RUnlock()
	if exists {
		sink(id, *st)
	}
}

// enableTCPKeepalive 在 TCP 拨号成功后启用 keepalive 探测。
//
// 设计取舍：
//   - 仅用 SetKeepAlive + SetKeepAlivePeriod 两个标准库 API，跨平台兼容
//   - 不引入 syscall.SetSockoptInt 平台相关代码（保持标准库纯净）
//   - Windows 上 SetKeepAlivePeriod 同时设置 KEEPIDLE 与 KEEPINTVL（间隔合并），
//     无法精细控制 idle/count，但 3s 间隔 × 系统默认探测次数已满足
//     ~33s（Windows）/ ~12s（Linux）检测目标
//
// 返回值语义：失败时返回错误，调用方决定是否中止。keepalive 是拔线检测的优化项
// 而非连接正确性前提，受限环境（沙箱/低权限）下 SetKeepAlive 可能失败，
// 调用方可选择仅记 warn 不中止，退化为 readLoop 的连续超时计数器（采集期）检测。
//
// 非 TCP 连接（如测试 mock）返回 nil 保持兼容。
func enableTCPKeepalive(conn net.Conn) error {
	tcpConn, ok := conn.(*net.TCPConn)
	if !ok {
		return nil
	}
	if err := tcpConn.SetKeepAlive(true); err != nil {
		return fmt.Errorf("enable keepalive: %w", err)
	}
	if err := tcpConn.SetKeepAlivePeriod(p1604KeepAlivePeriod); err != nil {
		return fmt.Errorf("set keepalive period: %w", err)
	}
	return nil
}

func runConnectionHandshake(conn net.Conn, timeout time.Duration, handshake func() error) error {
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

// describeConnectHandshakeFailure 为握手阶段"设备拒绝新连接"补充可执行的运维提示。
//
// 现场现象（192.168.1.7:9000 实机复现）：设备长时间拔网线后重连，TCP 拨号成功、
// w1601 发送成功，但设备随即 FIN 关闭新连接，读 ACK 得到 io.EOF
// （P1604ReadCommandACK 对任意读错误统一附加 ErrWatchdogTriggered 哨兵，会误报 watchdog）。
//
// 根因是设备固件缺陷：链路长时间中断后设备仍保留旧连接槽位（单客户端固件），
// 对新连接在第一条命令时直接关闭，且该状态不会自行恢复——实测重试 20 秒后仍 EOF，
// 只有重启设备电源才能复位其 TCP 服务。应用无法通过重试或命令绕开，因此必须把
// "重启设备电源"作为可执行建议返回给操作员，而不是暴露难以理解的 EOF 原始错误。
//
// 判定依据：IsConnResetByPeer（对端 FIN/RST 硬证据）覆盖 w1601 与 u01101 两处
// 设备主动关闭连接的握手路径；纯超时（连接存活但无响应）属另一类故障，走原错误。
//
// 提示文案取舍：IsConnResetByPeer 也覆盖瞬时故障（设备瞬间重启、网络抖动），
// 这类场景简单重试即可恢复，直接建议重启电源会误导排查。因此文案把"重试"放
// 在前面，"重启电源"作为持续失败后的兜底建议。
func describeConnectHandshakeFailure(err error) error {
	if err == nil {
		return nil
	}
	if sharedproto.IsConnResetByPeer(err) {
		return fmt.Errorf("%w（设备拒绝了新连接，可能是长时间断网后旧连接未释放。请先重试，仍失败再重启设备电源）", err)
	}
	return err
}

// Connect 连接设备
// 锁策略：仅在读写共享状态时持锁，TCP 拨号和 w1601 命令在锁外执行
//
// 连接后单位同步流程：
//  1. 发送 w1601 启用长度前缀并校验 A 应答
//  2. 发送 u01101 读取硬件 EU 压力转换系数，识别硬件当前压力单位
//  3. 若硬件单位与 profile 配置不一致，以硬件为准更新 profile
//     （硬件是数据源，配置侧标签必须与硬件一致，否则数值与单位不匹配）
func (a *P1604Adapter) Connect(profile core.PressureProfile) error {
	shard := a.shard(profile.ID)
	shard.mu.Lock()
	if _, exists := shard.drivers[profile.ID]; exists {
		shard.mu.Unlock()
		return fmt.Errorf("device %s already connected", profile.ID)
	}
	if _, exists := shard.connecting[profile.ID]; exists {
		shard.mu.Unlock()
		return fmt.Errorf("device %s connection already in progress", profile.ID)
	}
	shard.connecting[profile.ID] = struct{}{}
	shard.mu.Unlock()
	defer func() {
		shard.mu.Lock()
		delete(shard.connecting, profile.ID)
		shard.mu.Unlock()
	}()

	host := profile.Address
	if host == "" {
		host = p1604DefaultHost
	}
	port := profile.Port
	if port <= 0 {
		port = p1604DefaultPort
	}

	// TCP 拨号在锁外执行，避免阻塞其他设备操作
	conn, err := sharedproto.DialTCP(fmt.Sprintf("%s:%d", host, port), profile.LocalAddress, p1604ConnectTimeout)
	if err != nil {
		return fmt.Errorf("connect to %s:%d: %w", host, port, err)
	}

	// TCP 连接成功，打印硬件通信日志（前端 "通信" 分组可见）
	a.emitLog(DeviceLogEntry{
		Level: "info", Category: "hardware-recv", DeviceID: profile.ID,
		Message: "TCP connected", Detail: fmt.Sprintf("%s:%d", host, port),
	})

	// 启用 TCP keepalive：物理拔网线场景下 TCP 层无 RST/FIN，
	// 采集期 readLoop 的连续超时计数器（5s）是主检测路径，
	// 非采集期 readLoop 空闲时 keepalive 是唯一主动检测手段（~33s/12s）。
	// keepalive 探测失败会触发非 timeout 错误，readLoop 据此进入 handleConnectionLost。
	// 启用失败不中止连接——keepalive 是优化项，受限环境下退化为连续超时计数器检测。
	if err := enableTCPKeepalive(conn); err != nil {
		a.emitLog(DeviceLogEntry{
			Level: "warn", Category: "hardware", DeviceID: profile.ID,
			Message: "TCP keepalive enable failed, fallback to read-deadline detection",
			Detail:  err.Error(),
		})
	}

	// idleStopCh/idleLoopDone 在 driver 创建时即初始化，避免与 Disconnect 并发执行时
	// 出现 nil channel close panic：Connect 在锁外启动 idleReadLoop 之前，driver
	// 已存入 shard.drivers，并发的 Disconnect 可在锁内读到 driver 并尝试 close(idleStopCh)。
	// 提前初始化保证 Disconnect 看到的 idleStopCh 永远非 nil。
	driver := &p1604Driver{
		profile:      profile,
		conn:         conn,
		frameReader:  sharedproto.NewFrameReader(conn),
		emit:         a.emitLog,
		idleStopCh:   make(chan struct{}),
		idleLoopDone: make(chan struct{}),
	}

	var hwUnit, unitNote string
	err = runConnectionHandshake(conn, p1604HandshakeTimeout, func() error {
		// 连接后必须先发 w1601 启用长度前缀模式
		if err := driver.sendCommand("w1601"); err != nil {
			return fmt.Errorf("enable length prefix: %w", err)
		}
		if err := sharedproto.P1604ReadCommandACK(driver.frameReader, conn, 0); err != nil {
			return fmt.Errorf("enable length prefix response: %w", err)
		}
		a.emitLog(DeviceLogEntry{
			Level: "info", Category: "hardware-recv", DeviceID: profile.ID,
			Message: "Command response", Detail: "w1601 -> A",
		})

		var unitErr error
		hwUnit, unitNote, unitErr = a.syncUnitFromHardware(driver, profile, 0)
		if unitErr != nil {
			return fmt.Errorf("sync unit from hardware: %w", unitErr)
		}
		return nil
	})
	if err != nil {
		_ = conn.Close()
		// 设备主动关闭连接（长时间断网后旧连接未释放）：补充"重启设备电源"运维提示，
		// 避免操作员面对 EOF/watchdog 原始错误无从下手。
		return describeConnectHandshakeFailure(err)
	}

	shard.mu.Lock()
	// 二次检查：拨号期间可能已被其他 goroutine 连接
	if _, exists := shard.drivers[profile.ID]; exists {
		shard.mu.Unlock()
		conn.Close()
		return fmt.Errorf("device %s already connected", profile.ID)
	}
	// 若硬件单位与 profile 不一致，以硬件为准更新 driver.profile。
	// 同步级联更新 channels[i].Unit，否则前端通道卡片会显示陈旧单位
	// （顶部状态条读 p1604Config.unit，卡片读 channels[i].unit，两者需同源）。
	if hwUnit != "" && hwUnit != profile.P1604Cfg.Unit {
		profile.P1604Cfg.Unit = hwUnit
		syncChannelsUnit(profile.Channels, hwUnit)
		driver.profile = profile
	}
	shard.drivers[profile.ID] = driver
	shard.status[profile.ID] = &core.DeviceState{
		Profile:     profile,
		Status:      core.StatusConnected,
		StatusText:  core.StatusConnected.String(),
		ConnectedAt: core.TimestampMs(),
	}
	// 启动 idleReadLoop：非采集期间感知 TCP keepalive 失败（CONN-008）。
	// 仅连接不采集时拔网线，内核 keepalive 探测失败会让 socket 进入 abort 状态，
	// idleReadLoop 周期性短超时 Read 即可立刻感知该错误并触发 handleConnectionLost。
	// StartAcquisition 启动 readLoop 前会先停止它，避免与 readLoop 竞争 frameReader。
	//
	// 在锁内启动 goroutine 是安全的：go 语句本身立即返回，不阻塞锁持有者；
	// 且锁内启动可保证 Disconnect 在锁内看到 driver 时，idleReadLoop 已确定启动
	// （否则锁外启动期间 Disconnect 并发执行会 close 一个还没被启动的 channel，
	// 导致 idleReadLoop goroutine 永远不运行、idleLoopDone 永不 close）。
	go a.idleReadLoop(profile.ID, driver, driver.idleStopCh)
	shard.mu.Unlock()

	a.emitLog(DeviceLogEntry{
		Level: "info", Category: "hardware", DeviceID: profile.ID,
		Message: "Device connected", Detail: fmt.Sprintf("%s:%d | %s", host, port, unitNote),
	})
	// 通知前端：profile 可能因硬件单位同步而变化，前端需要刷新配置面板
	a.emitState(profile.ID)
	return nil
}

// syncUnitFromHardware 读取硬件 EU 压力转换系数并匹配为标准单位字符串
//
// 通信日志：u01101 命令通过 sharedproto.P1604ReadUnitCoefficient 发送，
// 不走 driver.sendCommand，故在此补充 hardware-send/hardware-recv 日志，
// 让前端 "通信" 分组能看到完整的连接阶段命令交互。
func (a *P1604Adapter) syncUnitFromHardware(driver *p1604Driver, profile core.PressureProfile, timeout time.Duration) (string, string, error) {
	// 打印 u01101 命令发送日志
	a.emitLog(DeviceLogEntry{
		Level: "info", Category: "hardware-send", DeviceID: profile.ID,
		Message: "Command sent", Detail: "u01101 (read unit coefficient)",
	})
	coeff, err := sharedproto.P1604ReadUnitCoefficient(driver.frameReader, driver.conn, timeout)
	if err != nil {
		// 打印 u01101 响应失败日志（通信层）
		a.emitLog(DeviceLogEntry{
			Level: "warn", Category: "hardware-recv", DeviceID: profile.ID,
			Message: "Command response error", Detail: fmt.Sprintf("u01101: %v", err),
		})
		return "", "", fmt.Errorf("read u01101: %w", err)
	}
	// 打印 u01101 响应日志（通信层，记录解析出的系数）
	a.emitLog(DeviceLogEntry{
		Level: "info", Category: "hardware-recv", DeviceID: profile.ID,
		Message: "Command response", Detail: fmt.Sprintf("u01101 -> coeff=%f", coeff),
	})
	hwUnit, matched := sharedproto.P1604MatchUnitByCoefficient(coeff)
	if !matched {
		a.emitLog(DeviceLogEntry{
			Level: "error", Category: "hardware", DeviceID: profile.ID,
			Message: "Hardware unit coefficient unknown",
			Detail:  fmt.Sprintf("coeff=%f", coeff),
		})
		return "", "", fmt.Errorf("unknown hardware unit coefficient: %f", coeff)
	}
	if hwUnit != profile.P1604Cfg.Unit {
		// 硬件与 profile 不一致：以硬件为准
		a.emitLog(DeviceLogEntry{
			Level: "info", Category: "hardware", DeviceID: profile.ID,
			Message: "Unit synced from hardware",
			Detail:  fmt.Sprintf("profile=%s -> hardware=%s (coeff=%f)", profile.P1604Cfg.Unit, hwUnit, coeff),
		})
		return hwUnit, fmt.Sprintf("unit=%s (synced from hardware, coeff=%f)", hwUnit, coeff), nil
	}
	return hwUnit, fmt.Sprintf("unit=%s (coeff=%f)", hwUnit, coeff), nil
}

// syncChannelsUnit 硬件单位同步后级联更新通道单位。
//
// 物理量约束（与前端 DaqP1604Config.vue getChannelUnit 规则保持一致）：
//   - CH1-CH16（index 0-15）压力通道：跟随全局压力单位
//   - CH17（index 16）大气压力：锁定 Pa（独立物理量，不归压力 EU 系数管理）
//   - CH18（index 17）大气温度：锁定 °C
//
// 必须在硬件单位同步时同步调用，否则前端通道卡片会显示陈旧单位
// （顶部状态条读 p1604Config.unit 已是 Pa，卡片读 channels[i].unit 仍是 psi）。
func syncChannelsUnit(channels []core.ChannelConfig, globalUnit string) {
	for i := range channels {
		switch i {
		case 16:
			channels[i].Unit = "Pa"
		case 17:
			channels[i].Unit = "°C"
		default:
			if globalUnit != "" {
				channels[i].Unit = globalUnit
			}
		}
	}
}

// Disconnect 断开设备连接
//
// 关闭顺序对竞态修复至关重要：
//  1. 锁内：标记主动停止原因 + close(stop) 通知 readLoop 退出 + close(idleStopCh)
//     通知 idleReadLoop 退出 + 清理共享状态。
//  2. 锁外：等待 readLoop 和 idleReadLoop 退出（join），确保它们不再持有 driver.conn 引用。
//  3. 锁外：发送停止命令、conn.Close。
//
// ADR-009 finding 4 整改：watchdog 必须在 operationMu.Lock 之前启动，覆盖锁等待阶段。
// 历史背景：原实现直接 driver.operationMu.Lock()，若前序操作（StartAcquisition/
// StopAcquisition/ApplyConfig/ZeroCalibration）持 operationMu 阻塞在 sendCommandACK
// 的 Write/Read 上（SetWriteDeadline/SetReadDeadline 失效），Disconnect 会永久卡在
// operationMu.Lock()，前端"断开"按钮永远转圈。watchdog 触发后 close conn 解除前序
// I/O 阻塞，operationMu 得以释放。
// 锁等待期间 watchdog 触发时直接返回错误，handleConnectionLost 已由 watchdog close
// 触发的前序操作调用，无需重复调用。
func (a *P1604Adapter) Disconnect(id string) error {
	shard := a.shard(id)
	shard.mu.RLock()
	driver := shard.drivers[id]
	shard.mu.RUnlock()
	if driver != nil {
		// finding 4：watchdog 在 operationMu.Lock 之前启动，覆盖锁等待阶段。
		// driver.conn 在 Connect 时创建后不可变（重连会创建新 driver），可安全快照。
		var wdStop func() bool
		if driver.conn != nil {
			wdStop = sharedproto.WatchdogClose(driver.conn, p1604WatchdogTimeout)
		}
		driver.operationMu.Lock()
		defer driver.operationMu.Unlock()
		// 锁等待期间 watchdog 可能已触发（conn 已 Close）。检查并走毒化路径，
		// 避免后续操作在已死 conn 上做无效 I/O。
		if wdStop != nil && !wdStop() {
			// 锁等待期间 watchdog 触发：连接已死。handleConnectionLost 应由前序
			// 阻塞操作在 conn Close 后自行调用（sendCommandACK 的 IsConnectionFault
			// 分支会触发）。此处仅做防御性二次清理：若 driver 仍在 map 中则清理。
			shard.mu.Lock()
			if cur, ok := shard.drivers[id]; ok && cur == driver {
				if st, exists := shard.status[id]; exists {
					st.SetStatus(core.StatusError)
					st.Error = "disconnect: operationMu lock wait watchdog triggered; reconnect required"
					st.AcquiringAt = 0
				}
				if driver.conn != nil {
					_ = driver.conn.Close()
				}
				delete(shard.drivers, id)
				if done, exists := shard.stopChs[id]; exists {
					close(done)
					delete(shard.stopChs, id)
				}
				if ch, exists := shard.channels[id]; exists {
					close(ch)
					delete(shard.channels, id)
				}
				delete(shard.sinks, id)
			}
			shard.mu.Unlock()
			a.emitState(id)
			return fmt.Errorf("disconnect: operationMu lock wait watchdog triggered; reconnect required")
		}
	}

	shard.mu.Lock()
	current, ok := shard.drivers[id]
	if current != driver {
		ok = false
	}
	wasAcquiring := ok && driver != nil && driver.acquiring
	// 在 close(stop) 之前标记主动停止原因，readLoop 看到该原因会静默退出
	if wasAcquiring && driver != nil {
		driver.SetStopReason(sharedproto.StopReasonUserRequested)
	}
	if done, exists := shard.stopChs[id]; exists {
		close(done)
		delete(shard.stopChs, id)
	}
	if ch, exists := shard.channels[id]; exists {
		close(ch)
		delete(shard.channels, id)
	}
	delete(shard.sinks, id)
	// 通知 idleReadLoop 退出：close 前保存引用避免 race，nil 时跳过（StartAcquisition 已停）
	idleStop := func() chan struct{} {
		if driver == nil || driver.idleStopCh == nil {
			return nil
		}
		ch := driver.idleStopCh
		select {
		case <-ch: // 已关闭，避免重复 close panic
		default:
			close(ch)
		}
		driver.idleStopCh = nil // 标记已停止，避免后续误重启
		return ch
	}()
	if !ok {
		// driver 不存在（可能已被 handleConnectionLost 清理）：
		// 仍需把 status 改为 Disconnected，让前端感知用户主动断开意图，
		// 覆盖之前由 handleConnectionLost 设置的 Error 状态。
		if st, exists := shard.status[id]; exists {
			st.SetStatus(core.StatusDisconnected)
			st.Error = ""
			st.AcquiringAt = 0
		}
		shard.mu.Unlock()
		a.emitState(id)
		return nil
	}
	delete(shard.drivers, id)
	if driver != nil {
		driver.acquiring = false
	}
	if st, exists := shard.status[id]; exists {
		st.SetStatus(core.StatusDisconnected)
		st.Error = ""
		st.AcquiringAt = 0
	}
	connected := driver != nil && driver.conn != nil
	shard.mu.Unlock()

	// 等待 readLoop 退出后再操作连接，避免 ReadFrame 与 Close 竞争。
	// join 超时仅是兜底，正常情况下 readLoop 在 200ms 读超时内就会观察到 stop。
	if driver != nil && wasAcquiring && !driver.joinReadLoop(id, sharedproto.ReadLoopJoinTimeout) {
		connected = false
		if driver.conn != nil {
			_ = driver.conn.Close()
		}
	}
	// 等待 idleReadLoop 退出，避免它与 conn.Close 竞争（CONN-008）。
	// idleStop 非 nil 表示本次 Disconnect 触发了 close，必须 join；
	// idleStop 为 nil 表示 idleReadLoop 已被 StartAcquisition 停止或本次未启动，跳过。
	if driver != nil && idleStop != nil {
		driver.joinIdleLoop(id, sharedproto.ReadLoopJoinTimeout)
	}
	// 等待 idleReadLoop 退出，避免它与 conn.Close 竞争（CONN-008）。
	// idleStop 非 nil 表示本次 Disconnect 触发了 close，必须 join；
	// idleStop 为 nil 表示 idleReadLoop 已被 StartAcquisition 停止或本次未启动，跳过。
	if driver != nil && idleStop != nil {
		driver.joinIdleLoop(id, sharedproto.ReadLoopJoinTimeout)
	}

	var stopErr error
	if wasAcquiring && connected && driver != nil {
		// readLoop 退出前可能已读到压力数据帧并放入 frameReader 缓冲区。
		// 必须清空缓冲区，否则 sendCommandACK 的 ReadFrame 会读到残留数据帧
		// 而非 ASCII A，导致 c 02 停止命令响应解析失败（详见 StopAcquisition 同款修复）。
		driver.frameReader.Reset()
		stopErr = driver.sendCommandACK("c 02 1")
	}
	if driver != nil && driver.conn != nil {
		_ = driver.conn.Close()
	}

	// 通知前端状态变更
	a.emitState(id)
	// TCP 断开日志（通信层），归类到 hardware-recv 便于前端 "通信" 分组展示
	a.emitLog(DeviceLogEntry{
		Level: "info", Category: "hardware-recv", DeviceID: id,
		Message: "TCP disconnected",
	})
	a.emitLog(DeviceLogEntry{
		Level: "info", Category: "hardware", DeviceID: id,
		Message: "Device disconnected",
	})
	if stopErr != nil {
		return fmt.Errorf("stop stream before disconnect: %w", stopErr)
	}
	return nil
}

// isDeadConnectionError 判定错误是否表明"连接已不可用，必须清理 driver 走重连"。
//
// 组合 IsConnectionFault（超时/重置/关闭）与 IsConnResetByPeer（对端 FIN/RST/abort）
// 的原因：IsConnectionFault 的匹配串漏掉了 WSAECONNABORTED（"connection was aborted"），
// 该错误只在 IsConnResetByPeer 的 wsasend/wsarecv 匹配中。现场 Windows 上
// TCP keepalive 检测到半死连接后会 abort socket，后续 Write 立即返回该错误；
// 若仅用 IsConnectionFault 判定，会走"软错误"分支只重启 idleReadLoop、不清理
// driver，应用停留在"已连接但不可用"的假状态，每次操作都失败。
//
// 用于 handleConnectionLost 等状态迁移决策，以及 rollbackAcquisition 等
// "连接已死"场景的日志分级（Debug/Warn），保证日志分级与状态迁移判定口径一致。
func isDeadConnectionError(err error) bool {
	return sharedproto.IsConnectionFault(err) || sharedproto.IsConnResetByPeer(err)
}

// StartAcquisition 启动数据采集
// 锁策略：仅在状态检查和状态更新时持锁，所有 sendCommand 和 Sleep 在锁外执行
//
// ADR-009 finding 4 整改：watchdog 必须在 operationMu.Lock 之前启动，覆盖锁等待阶段。
// 与 Disconnect 同模式：watchdog 触发后 close conn 解除前序 I/O 阻塞，operationMu 释放。
// 锁等待期间 watchdog 触发时调用 handleConnectionLost 清理 driver + conn 并返回错误。
func (a *P1604Adapter) StartAcquisition(id string) (<-chan core.PressureSnapshot, error) {
	shard := a.shard(id)
	shard.mu.RLock()
	driver, ok := shard.drivers[id]
	shard.mu.RUnlock()
	if !ok || driver == nil {
		return nil, fmt.Errorf("device %s not connected", id)
	}
	// finding 4：watchdog 在 operationMu.Lock 之前启动，覆盖锁等待阶段。
	var wdStop func() bool
	if driver.conn != nil {
		wdStop = sharedproto.WatchdogClose(driver.conn, p1604WatchdogTimeout)
	}
	driver.operationMu.Lock()
	defer driver.operationMu.Unlock()
	if wdStop != nil && !wdStop() {
		// 锁等待期间 watchdog 触发：连接已死，毒化连接返回错误。
		a.handleConnectionLost(id, driver, fmt.Errorf("start acquisition: operationMu lock wait watchdog triggered; reconnect required"))
		return nil, fmt.Errorf("start acquisition: operationMu lock wait watchdog triggered; reconnect required")
	}

	shard.mu.Lock()
	current, ok := shard.drivers[id]
	if !ok || current != driver {
		shard.mu.Unlock()
		return nil, fmt.Errorf("device %s not connected", id)
	}
	if _, exists := shard.channels[id]; exists {
		shard.mu.Unlock()
		return nil, fmt.Errorf("device %s already acquiring", id)
	}
	// 预占采集槽位，防止并发重复启动
	// 队列容量 8192：1kHz 下可缓冲 8 秒积压；recorder 异步投递后极少积压
	ch := make(chan core.PressureSnapshot, 8192)
	done := make(chan struct{})
	shard.channels[id] = ch
	shard.stopChs[id] = done
	shard.mu.Unlock()

	// 命令应答必须由当前操作同步读取，先停止 idleReadLoop，避免抢读 ACK。
	// idleReadLoop 改造后只等 stop channel，close 即退出，不会阻塞调用方。
	stoppedIdle := a.stopIdleLoop(id, driver)
	driver.frameReader.Reset()
	// 不再调用 DrainConnection：依赖 SetReadDeadline 的阻塞 Read 在现场 Windows 环境
	// 不可靠（参考 docs/acquisition-start-no-response.md §4.2/§5.1）。首次采集无旧流帧，
	// 正常停止会消费 c 02 应答；异常残留由 frameReader.Reset 处理已足够。

	// 配置数据流参数：c 00 <st> <mask> <sync> <per> <fmt> <mode>
	periodMs := driver.profile.P1604Cfg.SamplingRate
	if err := driver.sendCommandACK(fmt.Sprintf("c 00 1 FFFF 1 %d 7 0", periodMs)); err != nil {
		if isDeadConnectionError(err) {
			// sendCommandACK 内部 watchdog 触发 / 对端 FIN/RST / 本地 keepalive
			// abort（WSAECONNABORTED）：连接已死，必须删除 driver + close conn，
			// 让用户重连而非继续撞同一死连接。
			a.handleConnectionLost(id, driver, fmt.Errorf("set stream params: %w", err))
		} else {
			// 软错误（如设备返回 Nxx）：连接仍可用，重启 idleReadLoop 保持检测。
			a.restartIdleLoop(id, driver, stoppedIdle)
		}
		a.rollbackAcquisition(id, ch, done)
		return nil, fmt.Errorf("set stream params: %w", err)
	}

	// 配置流返回内容：0010=压力，0400=设备时间戳，0800=大气压力/温度
	// 掩码按 profile.UseDeviceTimestamp 动态构建：
	//   - 开启硬件时间戳：0C10 = 压力(0010) + 时间戳(0400) + 大气(0800)
	//   - 关闭硬件时间戳：0810 = 压力(0010) + 大气(0800)，帧内不含时间戳字段
	// 历史行为：默认开启（UseDeviceTimestampEnabled()==true 时走 0C10 路径）
	contentMask := 0x0010 // 压力
	if driver.profile.P1604Cfg.UseDeviceTimestampEnabled() {
		contentMask |= 0x0400 // 设备时间戳
	}
	contentMask |= 0x0800 // 始终包含大气数据
	contentMaskHex := fmt.Sprintf("%04X", contentMask)
	if err := driver.sendCommandACK(fmt.Sprintf("c 05 1 %s", contentMaskHex)); err != nil {
		if isDeadConnectionError(err) {
			a.handleConnectionLost(id, driver, fmt.Errorf("set stream content: %w", err))
		} else {
			a.restartIdleLoop(id, driver, stoppedIdle)
		}
		a.rollbackAcquisition(id, ch, done)
		return nil, fmt.Errorf("set stream content: %w", err)
	}

	// 启动数据流
	if err := driver.sendCommandACK("c 01 1"); err != nil {
		if isDeadConnectionError(err) {
			a.handleConnectionLost(id, driver, fmt.Errorf("start stream: %w", err))
		} else {
			a.restartIdleLoop(id, driver, stoppedIdle)
		}
		a.rollbackAcquisition(id, ch, done)
		return nil, fmt.Errorf("start stream: %w", err)
	}

	directSink := func(snapshot core.PressureSnapshot) {
		select {
		case ch <- snapshot:
		case <-done:
		}
	}

	shard.mu.Lock()
	shard.sinks[id] = directSink
	driver.acquiring = true
	// 重置 stop 状态，准备启动新的 readLoop
	driver.readLoopDone = make(chan struct{})
	driver.ClearStopReason()
	// 关闭 idleReadLoop：采集期间由 readLoop 接管 frameReader，idleReadLoop 必须停止
	// 避免 readLoop 和 idleReadLoop 同时操作 driver.conn 造成数据竞争。
	// 锁内仅 close channel（无阻塞），锁外再 join，避免持锁等待 goroutine。
	// close 前保存引用避免 race，nil 时跳过（Connect 必然已初始化，此处 nil 是冗余防御）。
	idleStop := func() chan struct{} {
		if driver.idleStopCh == nil {
			return nil
		}
		ch := driver.idleStopCh
		select {
		case <-ch: // 已关闭，避免重复 close panic
		default:
			close(ch)
		}
		driver.idleStopCh = nil // 标记已停止，避免 Disconnect/StopAcquisition 重复 close
		return ch
	}()
	if st, exists := shard.status[id]; exists {
		st.SetStatus(core.StatusAcquiring)
		st.AcquiringAt = core.TimestampMs()
	}
	shard.mu.Unlock()

	// 锁外等待 idleReadLoop 退出：必须先 join 再启动 readLoop，
	// 否则 readLoop 与残留的 idleReadLoop 会同时 Read driver.conn 造成数据竞争。
	// join 超时仅是兜底（正常 idleReadLoop 在 p1604IdleCheckInterval=1s 内观察到 stop）。
	if idleStop != nil {
		driver.joinIdleLoop(id, sharedproto.ReadLoopJoinTimeout)
	}

	// 启动读取循环
	go a.readLoop(id, driver, done)

	return ch, nil
}

// rollbackAcquisition 启动失败时回滚采集状态
// 在锁外执行停止命令，并在锁内清理采集槽位
//
// channel 关闭单一 owner 原则（ADR-009 复核修订 finding 1）：
//   - 连接故障路径下 handleConnectionLost 已先行 close(done)/close(ch) 并从 map 删除；
//     若此处无条件再次 close 必然 panic（close of closed channel）。
//   - 通过 owner 检查（map 中仍是本调用关联的 ch/done）确保 close 只执行一次。
//   - handleConnectionLost 已删除 driver 时跳过 sendCommandACK，
//     避免在已死连接上发送 c 02 1。
func (a *P1604Adapter) rollbackAcquisition(id string, ch chan core.PressureSnapshot, done chan struct{}) {
	shard := a.shard(id)
	// 先在锁内检查 driver 是否仍存在：handleConnectionLost 已删除 driver 时，
	// 连接已死，c 02 1 既无法成功也会误导日志级别，直接跳过停止命令。
	shard.mu.RLock()
	driver, driverExists := shard.drivers[id]
	shard.mu.RUnlock()
	if driverExists && driver != nil {
		if err := driver.sendCommandACK("c 02 1"); err != nil {
			// 与状态迁移判定同口径：死连接（含 WSAECONNABORTED）属预期清理竞态，记 Debug；
			// 仅真正意外的软错误记 Warn，避免日志分级与连接状态判定不一致误导排查。
			if isDeadConnectionError(err) {
				slog.Debug("WISPA rollback stop stream: connection already gone", "device", id, "error", err)
			} else {
				slog.Warn("WISPA rollback stop stream failed", "device", id, "error", err)
			}
		}
	}

	// owner 检查：仅在 map 中仍是本调用关联的 ch/done 时才 close + delete。
	// handleConnectionLost 在连接故障路径下已 close 并删除，此时跳过避免 double close panic。
	shard.mu.Lock()
	if cur, ok := shard.stopChs[id]; ok && cur == done {
		close(done)
		delete(shard.stopChs, id)
	}
	if cur, ok := shard.channels[id]; ok && cur == ch {
		close(ch)
		delete(shard.channels, id)
	}
	delete(shard.sinks, id)
	shard.mu.Unlock()
}

// StopAcquisition 停止数据采集
//
// 与 Disconnect 相同的关闭顺序：
//  1. 锁内：标记主动停止 + close(stop) + 清理共享状态。
//  2. 锁外：等待 readLoop 退出。
//  3. 锁外：发送停止命令（不关 conn，连接保留以便后续重新 StartAcquisition）。
//
// ADR-009 finding 4 整改：watchdog 必须在 operationMu.Lock 之前启动，覆盖锁等待阶段。
// 与 Disconnect/StartAcquisition 同模式：watchdog 触发后 close conn 解除前序 I/O 阻塞。
// 锁等待期间 watchdog 触发时调用 handleConnectionLost 清理 driver + conn 并返回错误。
func (a *P1604Adapter) StopAcquisition(id string) error {
	shard := a.shard(id)
	shard.mu.RLock()
	driver := shard.drivers[id]
	shard.mu.RUnlock()
	if driver != nil {
		// finding 4：watchdog 在 operationMu.Lock 之前启动，覆盖锁等待阶段。
		var wdStop func() bool
		if driver.conn != nil {
			wdStop = sharedproto.WatchdogClose(driver.conn, p1604WatchdogTimeout)
		}
		driver.operationMu.Lock()
		defer driver.operationMu.Unlock()
		if wdStop != nil && !wdStop() {
			// 锁等待期间 watchdog 触发：连接已死，毒化连接返回错误。
			a.handleConnectionLost(id, driver, fmt.Errorf("stop acquisition: operationMu lock wait watchdog triggered; reconnect required"))
			return fmt.Errorf("stop acquisition: operationMu lock wait watchdog triggered; reconnect required")
		}
	}

	shard.mu.Lock()
	current, ok := shard.drivers[id]
	if current != driver {
		ok = false
	}
	wasAcquiring := ok && driver != nil && driver.acquiring
	if wasAcquiring && driver != nil {
		driver.SetStopReason(sharedproto.StopReasonUserRequested)
	}
	if done, exists := shard.stopChs[id]; exists {
		close(done)
		delete(shard.stopChs, id)
	}
	if ch, exists := shard.channels[id]; exists {
		close(ch)
		delete(shard.channels, id)
	}
	delete(shard.sinks, id)
	connected := ok && driver != nil && driver.conn != nil
	if driver != nil {
		driver.acquiring = false
	}
	if st, exists := shard.status[id]; exists {
		if connected {
			st.SetStatus(core.StatusConnected)
		} else {
			st.SetStatus(core.StatusDisconnected)
		}
		st.AcquiringAt = 0
	}
	shard.mu.Unlock()

	// 等待 readLoop 退出，避免它和后续命令并发使用同一 conn
	if driver != nil && wasAcquiring && !driver.joinReadLoop(id, sharedproto.ReadLoopJoinTimeout) {
		a.handleConnectionLost(id, driver, fmt.Errorf("read loop did not exit after stop; reconnect required"))
		return fmt.Errorf("read loop did not exit after stop; reconnect required")
	}

	// 仅在确实在采集且连接有效时，才在锁外发送停止命令
	if wasAcquiring && connected && driver != nil {
		// readLoop 退出前可能已读到压力数据帧并放入 frameReader 缓冲区。
		// 必须清空缓冲区，否则 sendCommandACK 的 ReadFrame 会读到残留数据帧
		// （现场复现：StartAcquisition 后 60ms 内 StopAcquisition，c 02 响应被
		// 解析为 "\x01\x00\x00..." 二进制帧而非 ASCII A，连接被误判为故障）。
		driver.frameReader.Reset()
		if err := driver.sendCommandACK("c 02 1"); err != nil {
			a.handleConnectionLost(id, driver, fmt.Errorf("stop stream: %w", err))
			return fmt.Errorf("stop stream: %w", err)
		}
	}

	// 采集停止后连接仍保留：重启 idleReadLoop 继续感知 keepalive 失败（CONN-008）。
	// 必须在 readLoop 完全退出后启动，避免两者同时操作 driver.conn。
	// 加锁是为了与 Disconnect 同步：避免 Disconnect 在我们启动 idleReadLoop 时
	// 同时操作 driver.idleStopCh/idleLoopDone 字段。
	// 二次检查 driver 仍存在：sendCommand 失败可能触发 handleConnectionLost 删除 driver。
	if connected && driver != nil && driver.conn != nil {
		shard.mu.Lock()
		if _, stillExists := shard.drivers[id]; stillExists && driver.conn != nil && driver.idleStopCh == nil {
			driver.idleStopCh = make(chan struct{})
			driver.idleLoopDone = make(chan struct{})
			go a.idleReadLoop(id, driver, driver.idleStopCh)
		}
		shard.mu.Unlock()
	}
	return nil
}

// ZeroCalibration 对全部压力通道执行设备原生零点校准。
//
// 实现要点：把"只发命令不验响应"升级为请求/响应模式，避免设备返回 Nxx 或
// 连接已死时前端仍提示"校准已启动"。两条路径分别由当前连接读取者协调：
//   - 采集期间：readLoop 持有 frameReader，通过 driver.pendingResponse 通道
//     把 ASCII 响应投递回本方法。本方法仅发送命令并 select 等待响应/超时。
//   - 空闲期间：idleReadLoop 不使用 frameReader（仅裸读 conn），先停止它
//     再用 frameReader 直接读写，结束后重启 idleReadLoop（与 ApplyConfig 同模式）。
//
// 错误处理与采集路径对齐：使用 isDeadConnectionError（IsConnectionFault ∪
// IsConnResetByPeer）判定连接故障并触发 handleConnectionLost。原因：
//   - 校零常在采集运行中触发，若设备连接半开（TCP keepalive 超时但未收到 RST），
//     IsConnResetByPeer 不匹配超时，会遗漏清理，导致后续采集调用持续撞同一死连接；
//   - IsConnectionFault 覆盖超时/broken pipe/closed conn/RST 等"连接不可用"证据，
//     但漏匹配 WSAECONNABORTED（keepalive abort 半死连接后的 Write 立即返回该错误），
//     该错误只在 IsConnResetByPeer 的匹配范围内，两者组合才是完整的死连接判定；
//   - 保守触发清理比静默失败更安全；
//   - 注：IsConnectionFault 文档标注"不可作为状态机输入"，但 StartAcquisition/StopAcquisition
//     的 rollback 路径已事实上用它做连接状态判定，此处与已有实践保持一致。
//
// ADR-009 R1-2：watchdog 必须在获取 operationMu 之前启动，覆盖锁等待阶段。
// 原实现直接 operationMu.Lock()，锁等待阶段无 watchdog 兜底：若前序操作持锁卡死
// （sendCommand Write 阻塞，SetWriteDeadline 失效），ZeroCalibration 会永久阻塞在
// operationMu.Lock() 上，前端"校零"按钮永远转圈。watchdog 触发后 close conn，
// 前序操作的 Write 会因 conn Close 返回错误，operationMu 得以释放。
// 获取锁后立即停止外层 watchdog，由 zeroCalibrationViaReadLoop/zeroCalibrationDirect
// 启动内层 watchdog 覆盖 Write + 等待响应（避免双重 watchdog 浪费 timer）。
func (a *P1604Adapter) ZeroCalibration(id string) error {
	shard := a.shard(id)
	shard.mu.RLock()
	driver, ok := shard.drivers[id]
	shard.mu.RUnlock()
	if !ok || driver == nil || driver.conn == nil {
		return fmt.Errorf("device %s not connected", id)
	}

	// R1-2：外层 watchdog 覆盖锁等待阶段。获取锁后立即停止，内层方法各自启动自己的 watchdog。
	wdStop := sharedproto.WatchdogClose(driver.conn, p1604WatchdogTimeout)

	driver.operationMu.Lock()
	defer driver.operationMu.Unlock()

	// 获取锁后检查 watchdog 是否已触发（锁等待期间 watchdog 可能已 close conn）。
	// wdStop 返回 false 表示 watchdog 已触发，conn 已 Close，必须毒化连接让 DeviceManager 重连。
	if !wdStop() {
		calibErr := fmt.Errorf("zero calibration: lock wait watchdog triggered; reconnect required")
		a.handleConnectionLost(id, driver, calibErr)
		return calibErr
	}

	shard.mu.RLock()
	current, stillConnected := shard.drivers[id]
	acquiring := stillConnected && current == driver && driver.acquiring
	shard.mu.RUnlock()
	if !stillConnected || current != driver || driver.conn == nil {
		return fmt.Errorf("device %s not connected", id)
	}

	if acquiring {
		return a.zeroCalibrationViaReadLoop(id, driver)
	}
	return a.zeroCalibrationDirect(id, driver)
}

// zeroCalibrationViaReadLoop 采集期间路径：通过 pendingResponse 通道协调响应。
//
// 约束：readLoop 是 frameReader 的唯一读取者，本方法绝不能直接调用 ReadFrame，
// 否则与 readLoop 竞争同一 conn 的 Read，造成帧字节流错位。
//
// 并发：pendingResponseMu 保证同一 driver 同时只有一个等待者；若并发调用，
// 第二个调用方立即返回错误，不阻塞。
//
// ADR-009 R1-2 + R0-12：watchdog 必须在 sendCommand 之前启动，覆盖 Write + 等待响应总预算。
// 原实现 sendCommand 内部仅 SetWriteDeadline 无 watchdog 兜底，SetWriteDeadline 失效时
// Write 永久阻塞，operationMu 永远无法释放，后续 Disconnect/StopAcquisition 永久等待。
// watchdog 触发后 close conn 解除阻塞，operationMu 得以释放。
// 响应超时也意味着协议边界已不可信（迟到响应可能随后进入 TCP 流被下一条命令消费），
// 必须毒化连接阻断迟到响应，不能仅返回 timeout 错误保留 conn。
func (a *P1604Adapter) zeroCalibrationViaReadLoop(id string, driver *p1604Driver) error {
	driver.pendingResponseMu.Lock()
	if driver.pendingResponse != nil {
		driver.pendingResponseMu.Unlock()
		return fmt.Errorf("zero calibration: 另一个命令响应正在等待")
	}
	ch := make(chan []byte, 1)
	driver.pendingResponse = ch
	driver.pendingResponseMu.Unlock()

	defer func() {
		driver.pendingResponseMu.Lock()
		driver.pendingResponse = nil
		driver.pendingResponseMu.Unlock()
	}()

	// R1-2：watchdog 覆盖 Write + 等待响应总预算。sendCommand 内部 SetWriteDeadline
	// 失效时 watchdog 兜底 close conn；等待响应期间设备无响应时 watchdog 也兜底 close conn。
	wdStop := sharedproto.WatchdogClose(driver.conn, p1604WatchdogTimeout)
	defer func() { _ = wdStop() }()

	if err := driver.sendCommand("h"); err != nil {
		// watchdog 触发：conn 已 Close，必须毒化连接让 DeviceManager 重连。
		if !wdStop() {
			calibErr := fmt.Errorf("zero calibration: write watchdog triggered; reconnect required")
			a.handleConnectionLost(id, driver, calibErr)
			return calibErr
		}
		// 普通写错误：连接级故障（含 WSAECONNABORTED）才清理，软错误保留连接。
		if isDeadConnectionError(err) {
			a.handleConnectionLost(id, driver, fmt.Errorf("zero calibration: %w", err))
		}
		return fmt.Errorf("zero calibration: %w", err)
	}

	select {
	case resp, ok := <-ch:
		if !ok {
			// 通道被 readLoop 关闭（handleConnectionLost 触发）：连接已断
			return fmt.Errorf("zero calibration: 连接在等待响应期间断开")
		}
		// 等待响应期间 watchdog 可能已 close conn：毒化连接让 DeviceManager 重连。
		if !wdStop() {
			calibErr := fmt.Errorf("zero calibration: read watchdog triggered; reconnect required")
			a.handleConnectionLost(id, driver, calibErr)
			return calibErr
		}
		if err := verifyZeroCalibrationResponse(resp); err != nil {
			return err
		}
		// 成功路径补充通信日志：readLoop 已将响应路由到本方法，不再走 processPayload 的日志分支
		a.emitLog(DeviceLogEntry{
			Level: "info", Category: "hardware-recv", DeviceID: id,
			Message: "Command response", Detail: "h -> " + strings.TrimSpace(string(resp)),
		})
		return nil
	case <-time.After(p1604CalibrationTimeout):
		// R0-12：响应超时意味着设备无响应或连接已半开，迟到响应可能随后进入 TCP 流
		// 被下一条命令消费导致协议错位。必须毒化连接，不能仅返回 timeout 错误保留 conn。
		calibErr := fmt.Errorf("zero calibration: 等待设备响应超时（%v）", p1604CalibrationTimeout)
		a.handleConnectionLost(id, driver, calibErr)
		return calibErr
	}
}

// zeroCalibrationDirect 空闲期间路径：停止 idleReadLoop 后直接读写 frameReader。
//
// 顺序与 ApplyConfig 一致：joinReadLoop（防御性 no-op）→ stopIdleLoop →
// Reset → sendCommand → ReadFrame → 验证响应；defer 重启 idleReadLoop。
//
// 不再调用 DrainConnection（参考 docs/acquisition-start-no-response.md §5.1）：
// 依赖 SetReadDeadline 的阻塞 Read 在现场 Windows 环境不可靠，
// frameReader.Reset 已足以清理应用层缓冲区残留。
//
// 【双重超时保护覆盖 Write + Read】（ADR-009 + P1-2.c 修复）
//   - sendCommand 内部 SetWriteDeadline（2s）+ 本方法 SetReadDeadline（2s）软超时
//   - 5s watchdog 硬兜底，必须在 sendCommand 之前启动，覆盖 Write + Read 全流程
//   - 原代码 wdStopRead 在 sendCommand 之后启动，Write 阶段裸奔；P1-2.c 修复后
//     wdStop 提到 sendCommand 之前，SetWriteDeadline 失效时 watchdog 兜底 Close
//   - watchdog 触发即连接已废，必须 handleConnectionLost 清理 driver + status
//   - 正常电脑上 watchdog 永不触发，连接仍可用
func (a *P1604Adapter) zeroCalibrationDirect(id string, driver *p1604Driver) error {
	driver.joinReadLoop(id, sharedproto.ReadLoopJoinTimeout)
	stoppedIdle := a.stopIdleLoop(id, driver)
	defer func() {
		shard := a.shard(id)
		shard.mu.Lock()
		if shard.drivers[id] == driver && driver.conn != nil && !driver.acquiring && driver.idleStopCh == stoppedIdle {
			driver.idleStopCh = make(chan struct{})
			driver.idleLoopDone = make(chan struct{})
			go a.idleReadLoop(id, driver, driver.idleStopCh)
		}
		shard.mu.Unlock()
	}()

	// 清理上次采集停止后尚未读完的二进制流数据帧。
	driver.frameReader.Reset()

	// wdStop 必须在 sendCommand 之前启动，覆盖 Write + Read 全流程（P1-2.c 修复）。
	// 原代码在 sendCommand 之后启动 wdStopRead，Write 阶段裸奔：SetWriteDeadline
	// 失效时 sendCommand 永久阻塞，wdStopRead 永远不会被创建。
	// 使用 sharedproto.WatchdogClose（sync.Once 幂等）替代本地 watchdogClose，
	// 与 windlabx4 项目对齐，避免本地版本缺乏幂等保护的已知历史问题。
	wdStop := sharedproto.WatchdogClose(driver.conn, p1604WatchdogTimeout)
	defer func() { _ = wdStop() }()

	// Write 阶段：sendCommand 内部走 SendCommandNoNewline，仅 SetWriteDeadline。
	// 若 SetWriteDeadline 失效导致 Write 阻塞，watchdog 会在 p1604WatchdogTimeout
	// 后强制 Close conn 解除阻塞。
	if err := driver.sendCommand("h"); err != nil {
		// wdStop 返回 false 表示 watchdog 已触发（conn 已被 Close）。
		// 此时 Write 错误由 watchdog close 引起，必须触发断线清理并返回统一错误，
		// 让 handleConnectionLost 通知前端与函数返回值一致（与 Read 阶段同模式）。
		if !wdStop() {
			calibErr := fmt.Errorf("zero calibration: write watchdog triggered; reconnect required")
			a.handleConnectionLost(id, driver, calibErr)
			return calibErr
		}
		// 普通 Write 错误：连接级故障（含 WSAECONNABORTED）才清理，软错误保留连接。
		// 与 zeroCalibrationViaReadLoop 的 Write 路径对齐：IsConnectionFault 漏匹配
		// WSAECONNABORTED（keepalive abort 半死连接），必须用 isDeadConnectionError，
		// 否则 driver 停留在"已连接但不可用"假状态。
		if isDeadConnectionError(err) {
			a.handleConnectionLost(id, driver, fmt.Errorf("zero calibration: %w", err))
		}
		return fmt.Errorf("zero calibration: %w", err)
	}

	// 读响应：长度前缀帧。采集未运行时无二进制流干扰，响应帧是 ReadFrame 的首个返回。
	// 双重超时保护（ADR-009）：
	//   - SetReadDeadline 设 2s 软超时，覆盖任何残留 deadline
	//   - 5s watchdog 硬兜底（已在 sendCommand 之前启动），应对 SetReadDeadline 失效场景
	// watchdog 触发时构造统一的 read watchdog 错误信息：
	//   - handleConnectionLost 用该信息通知前端
	//   - 函数返回用相同信息，保证日志与前端展示一致（Note #5 修复）
	_ = driver.conn.SetReadDeadline(time.Now().Add(p1604CalibrationTimeout))
	resp, err := driver.frameReader.ReadFrame()
	watchdogTriggered := !wdStop()
	_ = driver.conn.SetReadDeadline(time.Time{})
	if err != nil {
		// watchdog 触发：构造统一的错误信息，让 handleConnectionLost 和返回值一致
		var calibErr error
		if watchdogTriggered {
			calibErr = fmt.Errorf("zero calibration: read watchdog triggered; reconnect required")
		} else {
			calibErr = fmt.Errorf("zero calibration: %w", err)
		}
		// watchdog 触发或死连接（FIN/RST/超时/abort）：均视为连接已死，触发清理
		if watchdogTriggered || isDeadConnectionError(err) {
			a.handleConnectionLost(id, driver, calibErr)
		}
		return calibErr
	}

	if err := verifyZeroCalibrationResponse(resp); err != nil {
		return err
	}
	a.emitLog(DeviceLogEntry{
		Level: "info", Category: "hardware-recv", DeviceID: id,
		Message: "Command response", Detail: "h -> " + strings.TrimSpace(string(resp)),
	})
	return nil
}

// verifyZeroCalibrationResponse 校验设备对 h 命令返回的 16 路新零位系数。
func verifyZeroCalibrationResponse(resp []byte) error {
	s := strings.TrimSpace(string(resp))
	if strings.HasPrefix(s, "N") {
		return fmt.Errorf("零点校准被设备拒绝: %s", s)
	}
	fields := strings.Fields(s)
	if len(fields) == 16 {
		valid := true
		for _, field := range fields {
			value, err := strconv.ParseFloat(field, 64)
			if err != nil || math.IsNaN(value) || math.IsInf(value, 0) {
				valid = false
				break
			}
		}
		if valid {
			return nil
		}
	}
	return fmt.Errorf("零点校准响应异常: %q", s)
}

// joinReadLoop 等待 readLoop 关闭其 done channel，并返回是否已退出。
// 调用方通常先 close(stop) + 标记 stopReason，然后调用本方法等待 readLoop 退出，
// 再安全 conn.Close。driver 为 nil 或 readLoop 未启动时直接返回。
func (d *p1604Driver) joinReadLoop(id string, timeout time.Duration) bool {
	if d == nil || d.readLoopDone == nil {
		return true
	}
	select {
	case <-d.readLoopDone:
		return true
	case <-time.After(timeout):
		slog.Warn("WISPA readLoop join timeout", "device", id, "timeout", timeout)
		return false
	}
}

// joinIdleLoop 等待 idleReadLoop 关闭其 done channel。
//
// idleReadLoop 改造为只等 stop channel 后（参考 docs/acquisition-start-no-response.md §5.2），
// close(stop) 必然立即唤醒 idleReadLoop 退出，本方法不会超时。保留 timeout 参数
// 仅为防御性兜底（如未来 idleReadLoop 又引入阻塞操作）。
//
// 调用方通常先 close(idleStopCh)，然后调用本方法等待 idleReadLoop 退出，
// 再启动 readLoop 或 conn.Close。driver 为 nil 或 idleReadLoop 未启动时返回 true。
func (d *p1604Driver) joinIdleLoop(id string, timeout time.Duration) bool {
	if d == nil || d.idleLoopDone == nil {
		return true
	}
	select {
	case <-d.idleLoopDone:
		return true
	case <-time.After(timeout):
		slog.Warn("WISPA idleLoop join timeout", "device", id, "timeout", timeout)
		return false
	}
}

// stopIdleLoop 通知 idleReadLoop 退出并等待其结束。
// 必须在 shard.mu 锁外调用（避免持锁等待 goroutine），
// 方法内部在锁内取得 idleStopCh，close 和 join 均在锁外执行。
//
// 返回 stoppedIdleCh 用于 restartIdleLoop 比对（nil 表示无需重启）。
//
// idleReadLoop 改造后不再调用 conn.Read，close(stop) 立即唤醒退出，
// join 必然在 ReadLoopJoinTimeout 内成功，不再有 forceClosed 场景。
func (a *P1604Adapter) stopIdleLoop(id string, driver *p1604Driver) (stoppedIdleCh chan struct{}) {
	if driver == nil {
		return nil
	}
	shard := a.shard(id)
	shard.mu.Lock()
	if shard.drivers[id] != driver || driver.idleStopCh == nil {
		shard.mu.Unlock()
		return nil
	}
	stopCh := driver.idleStopCh
	shard.mu.Unlock()

	select {
	case <-stopCh: // 已关闭，避免重复 close panic
	default:
		close(stopCh)
	}
	driver.joinIdleLoop(id, sharedproto.ReadLoopJoinTimeout)
	return stopCh
}

func (a *P1604Adapter) restartIdleLoop(id string, driver *p1604Driver, stopped chan struct{}) {
	if driver == nil || stopped == nil {
		return
	}
	shard := a.shard(id)
	shard.mu.Lock()
	defer shard.mu.Unlock()
	if shard.drivers[id] != driver || driver.conn == nil || driver.acquiring || driver.idleStopCh != stopped {
		return
	}
	driver.idleStopCh = make(chan struct{})
	driver.idleLoopDone = make(chan struct{})
	go a.idleReadLoop(id, driver, driver.idleStopCh)
}

// idleReadLoop 非采集期间的占位协程，仅等待 stop channel 关闭后退出。
//
// 设计取舍（参考 docs/acquisition-start-no-response.md §5.2）：
//   - 原实现每秒对连接执行 200ms deadline 的 Read 来感知 TCP keepalive 失败。
//     但现场一台 Windows 电脑上 SetReadDeadline 到期后阻塞 Read 仍不返回，
//     导致 StartAcquisition 调 stopIdleLoop 时 join 超时，整体永久卡死
//     （现场日志：Start acquisition requested 后 30s 无任何 hardware-send 日志）。
//   - 本协程不再调用 conn.Read，彻底回避 SetReadDeadline 不可靠问题。
//     StartAcquisition / Disconnect 调 stopIdleLoop 时本协程必然在 select 上
//     等待，close(stop) 立即唤醒退出，不会阻塞调用方。
//
// 主动断线检测的取舍：
//   - 仅连接不采集时不再有 30~40s 内主动变错误的检测能力；
//   - TCP keepalive（3s 间隔，Windows ~33s / Linux ~12s 检出）仍是兜底；
//   - 下一次 StartAcquisition / ApplyConfig / ZeroCalibration 会主动发命令
//     确认连接可用性，连接已死时立即在命令路径上失败并 handleConnectionLost。
//   - 用户操作频率远高于 30s，实际感知延迟可接受；优先保证采集可启动。
//
// 退出前必须 close(idleLoopDone)，让 StartAcquisition / Disconnect 能 join 到本协程。
func (a *P1604Adapter) idleReadLoop(id string, driver *p1604Driver, stop <-chan struct{}) {
	defer close(driver.idleLoopDone)
	<-stop
}

// Status 获取设备状态
func (a *P1604Adapter) Status(id string) (core.DeviceState, bool) {
	shard := a.shard(id)
	shard.mu.RLock()
	defer shard.mu.RUnlock()

	st, ok := shard.status[id]
	if !ok {
		return core.DeviceState{}, false
	}
	driver, hasDriver := shard.drivers[id]
	if hasDriver && driver.acquiring {
		st.SetStatus(core.StatusAcquiring)
	} else if hasDriver {
		st.SetStatus(core.StatusConnected)
	}
	return *st, true
}

// ApplyConfig 应用设备配置
//
// 单位变更会立即写入硬件（v01101 修改 EU 压力转换系数），
// 采样周期等其它配置在下次 StartAcquisition 时生效。
//
// 调用约束：
//   - 设备未连接：仅更新 profile，无硬件可写
//   - 设备已连接但未采集：若单位发生变化，发送 v01101 写入硬件；写入失败则不更新 profile
//   - 设备正在采集：拒绝（由 usecase 层保证，adapter 自身也做防御性检查）
//
// ADR-009 finding 4 整改：watchdog 必须在 operationMu.Lock 之前启动，覆盖锁等待阶段。
// 与 Disconnect/StartAcquisition/StopAcquisition 同模式。锁等待期间 watchdog 触发时
// 调用 handleConnectionLost 清理 driver + conn 并返回错误。
func (a *P1604Adapter) ApplyConfig(id string, cfg core.P1604Config) error {
	shard := a.shard(id)
	shard.mu.RLock()
	driver := shard.drivers[id]
	shard.mu.RUnlock()
	if driver != nil {
		// finding 4：watchdog 在 operationMu.Lock 之前启动，覆盖锁等待阶段。
		var wdStop func() bool
		if driver.conn != nil {
			wdStop = sharedproto.WatchdogClose(driver.conn, p1604WatchdogTimeout)
		}
		driver.operationMu.Lock()
		defer driver.operationMu.Unlock()
		if wdStop != nil && !wdStop() {
			// 锁等待期间 watchdog 触发：连接已死，毒化连接返回错误。
			a.handleConnectionLost(id, driver, fmt.Errorf("apply config: operationMu lock wait watchdog triggered; reconnect required"))
			return fmt.Errorf("apply config: operationMu lock wait watchdog triggered; reconnect required")
		}
	}

	shard.mu.Lock()
	st, exists := shard.status[id]
	current, hasDriver := shard.drivers[id]
	if current != driver {
		hasDriver = false
	}
	if exists && hasDriver && driver != nil && driver.acquiring {
		shard.mu.Unlock()
		return fmt.Errorf("cannot apply config while acquiring")
	}
	prevUnit := ""
	if exists {
		prevUnit = st.Profile.P1604Cfg.Unit
	}
	shard.mu.Unlock()

	if !exists {
		return fmt.Errorf("device %s not found", id)
	}

	// 单位规范化：空字符串或不支持时回退到默认单位 psi
	if cfg.Unit == "" || !sharedproto.P1604IsSupportedUnit(cfg.Unit) {
		cfg.Unit = sharedproto.P1604DefaultUnit
	}

	// 已连接且未采集：若单位变化，写硬件 EU 系数
	// 未连接：跳过硬件写入，仅更新 profile（下次连接时由 Connect 阶段同步）
	if hasDriver && driver != nil && driver.conn != nil && cfg.Unit != prevUnit {
		// 未采集（acquiring==false）但已连接时，conn 的实际并发读取者是 idleReadLoop，
		// 而非采集 readLoop——readLoop 仅在 acquiring==true 时运行，且本函数在 L927 已
		// 拒绝 acquiring 状态，故此处 joinReadLoop 实为防御性 no-op（readLoopDone 已 close）。
		// 因此必须显式停止 idleReadLoop（stopIdleLoop 内部 close(idleStopCh)+joinIdleLoop），
		// 确保 idleReadLoop 完全退出后再操作 conn（Reset/DrainConnection/
		// P1604WriteUnitCoefficient），避免与残留 idleReadLoop 竞争同一 conn 的 Read，
		// 消除帧字节流错位导致后续 v01101 响应被污染为乱码的窗口。
		// idleLoopDone 已 close 时立即返回，正常路径无额外开销；末尾 defer 会重启 idleReadLoop。
		driver.joinReadLoop(id, sharedproto.ReadLoopJoinTimeout)
		stoppedIdle := a.stopIdleLoop(id, driver)
		defer func() {
			shard.mu.Lock()
			if shard.drivers[id] == driver && driver.conn != nil && !driver.acquiring && driver.idleStopCh == stoppedIdle {
				driver.idleStopCh = make(chan struct{})
				driver.idleLoopDone = make(chan struct{})
				go a.idleReadLoop(id, driver, driver.idleStopCh)
			}
			shard.mu.Unlock()
		}()

		coeff, ok := sharedproto.P1604PressureUnitCoefficient[cfg.Unit]
		if !ok {
			return fmt.Errorf("unsupported unit: %s", cfg.Unit)
		}
		// 清理应用层缓冲区残留。不再调用 DrainConnection：依赖 SetReadDeadline 的阻塞
		// Read 在现场 Windows 环境不可靠（参考 docs/acquisition-start-no-response.md §5.1）。
		driver.frameReader.Reset()
		// 打印 v01101 命令发送日志（v01101 通过 sharedproto.P1604WriteUnitCoefficient 发送，不走 sendCommand）
		a.emitLog(DeviceLogEntry{
			Level: "info", Category: "hardware-send", DeviceID: id,
			Message: "Command sent", Detail: fmt.Sprintf("v01101 %.6f (write unit coefficient, unit=%s)", coeff, cfg.Unit),
		})
		if err := sharedproto.P1604WriteUnitCoefficient(driver.frameReader, driver.conn, coeff, p1604UnitSyncTimeout); err != nil {
			// 打印 v01101 响应失败日志（通信层）
			a.emitLog(DeviceLogEntry{
				Level: "warn", Category: "hardware-recv", DeviceID: id,
				Message: "Command response error", Detail: fmt.Sprintf("v01101: %v", err),
			})
			// 写硬件失败：不更新 profile，让前端感知到失败状态
			a.emitLog(DeviceLogEntry{
				Level: "error", Category: "hardware", DeviceID: id,
				Message: "Write hardware unit failed",
				Detail:  fmt.Sprintf("unit=%s coeff=%f | %v", cfg.Unit, coeff, err),
			})
			// ADR-009 R1-1 + R0-12：对端 FIN/RST 或 soft timeout / watchdog 触发都意味着
			// 连接已不可信，必须复用 handleConnectionLost 清理 driver + conn，避免后续
			// StartAcquisition 的 c 00 命令爆 WSAECONNABORTED。ApplyConfig 要求未采集状态
			// （开头已校验 driver.acquiring==false），readLoop 此时已退出，调用
			// handleConnectionLost 无并发风险。
			if sharedproto.IsConnResetByPeer(err) || sharedproto.IsWatchdogTriggered(err) {
				a.handleConnectionLost(id, driver, fmt.Errorf("v01101: %w", err))
			}
			return fmt.Errorf("write hardware unit: %w", err)
		}
		// 打印 v01101 响应日志（通信层，写入成功）
		a.emitLog(DeviceLogEntry{
			Level: "info", Category: "hardware-recv", DeviceID: id,
			Message: "Command response", Detail: "v01101 -> A (ack, unit written)",
		})
		a.emitLog(DeviceLogEntry{
			Level: "info", Category: "hardware", DeviceID: id,
			Message: "Hardware unit updated",
			Detail:  fmt.Sprintf("%s -> %s (coeff=%f)", prevUnit, cfg.Unit, coeff),
		})
	}

	shard.mu.Lock()
	if st, exists := shard.status[id]; exists {
		st.Profile.P1604Cfg = cfg
	}
	if d, exists := shard.drivers[id]; exists && d != nil {
		d.profile.P1604Cfg = cfg
	}
	shard.mu.Unlock()
	// 通知前端：profile 已变更
	a.emitState(id)
	return nil
}

// SetDataSink 设置数据回调
func (a *P1604Adapter) SetDataSink(id string, sink func(core.PressureSnapshot)) {
	shard := a.shard(id)
	shard.mu.Lock()
	defer shard.mu.Unlock()
	shard.sinks[id] = sink
}

// readLoop 读取设备数据帧
//
// 退出路径：
//  1. stop channel 关闭（调用方主动停止）→ 静默退出。
//  2. no-data timer 到期（p1604NoDataTimeout = 5s 内无任何数据帧）：
//     - ADR-009 R0-10：独立 time.AfterFunc timer，不依赖循环体执行，
//     即使 SetReadDeadline 失效导致 Read 永久阻塞也能到期触发。
//     - timer 回调检查 driver.acquiring，Stop 后不毒化保留的连接。
//     - timer 调用 handleConnectionLost 清理 driver + close conn + 通知前端。
//  3. ReadFrame 返回非 timeout 错误：
//     - 若 driver.stopReason 已置位（调用方刚刚 close(stop) 但 select 尚未轮到）→
//     静默退出，不修改任何共享状态（避免与 StopAcquisition/Disconnect 的清理冲突）。
//     - 否则视为连接意外断开，标记设备为 Error 状态并通知前端。
//
// 退出前必须 close(readLoopDone)，让 Disconnect/StopAcquisition 能 join 到本协程。
//
// 退出时必须清除 SetReadDeadline：readLoop 每次循环都设 200ms deadline，退出时
// 残留的 deadline 会让后续 sendCommandACK 的 ReadFrame 立即返回 i/o timeout
// （现场复现：StopAcquisition 后 174ms 内再次 StartAcquisition，c 00 响应 23ms 内
// 报 i/o timeout，正是 readLoop 残留 deadline 在生效）。
func (a *P1604Adapter) readLoop(id string, driver *p1604Driver, stop <-chan struct{}) {
	defer close(driver.readLoopDone)
	defer func() { _ = driver.conn.SetReadDeadline(time.Time{}) }()

	// ADR-009 R0-10：no-data owner 必须独立于 read goroutine 与其 mutex。
	//
	// 历史背景：原实现通过循环体累计 consecutiveTimeouts 检测无数据，依赖 readLoop
	// 循环体执行。问题 Windows 电脑 deadline 失效时循环体不可达，5s 检出永远不会触发，
	// 半开连接无法自行收敛。
	//
	// 整改后：readLoop 入口启动独立 time.AfterFunc timer，到期调用 handleConnectionLost
	// 清理 driver + close conn。timer 不依赖循环体执行，即使 Read 永久阻塞也能到期触发。
	// 每次收到有效帧调用 Reset 续期；readLoop 退出 defer Stop。
	//
	// 独立 P1604 的 driver.conn 在 Connect 时创建后不可变（重连会创建新 driver），
	// 因此无需 expected conn 比较——handleConnectionLost 内部的 shard.drivers[id]==driver
	// 检查已覆盖"driver 被替换"场景。timer 仅需检查 driver.acquiring 防止 Stop 后误触发：
	// StopAcquisition 先置 driver.acquiring=false 再 join readLoop，timer 检测到
	// acquiring=false 直接跳过，避免毒化用户保留的连接。
	// 捕获 noDataTimeout 到局部变量：测试会覆盖 p1604NoDataTimeout 全局变量加速，
	// timer 回调必须使用创建时的快照值，避免回调 fire 时全局变量已被测试恢复
	// 导致 data race（ADR-009 R0-10 测试要求 -race 全绿）。
	noDataTimeout := p1604NoDataTimeout
	noDataTimer := time.AfterFunc(noDataTimeout, func() {
		shard := a.shard(id)
		shard.mu.RLock()
		acquiring := driver.acquiring
		shard.mu.RUnlock()
		if !acquiring {
			// 采集已停止（StopAcquisition 先置 acquiring=false 再 join readLoop），
			// timer 不应毒化保留的连接，让 readLoop 正常退出即可。
			return
		}
		a.handleConnectionLost(id, driver,
			fmt.Errorf("no data received for %v", noDataTimeout))
		slog.Warn("WISPA no data timeout, conn closed by watchdog",
			"device", id, "duration", noDataTimeout)
	})
	// readLoop 退出时停止 timer，避免 timer 在 readLoop 已退出后误触发。
	// Stop 不等待已 firing 的回调完成，但回调内 acquiring 检查 + handleConnectionLost
	// 的 driver 身份检查能正确处理 readLoop 退出后 timer 才 fire 的场景。
	defer noDataTimer.Stop()

	for {
		select {
		case <-stop:
			return
		default:
			// SetReadDeadline 仍保留作为单次 Read 的软超时，让循环体能周期性
			// 重新检查 stop channel（ADR-009 R0-10：no-data 检测由独立 timer 负责，
			// deadline 失效场景由 timer 兜底 Close conn）。
			driver.conn.SetReadDeadline(time.Now().Add(p1604ReadTimeout))
			payload, err := driver.frameReader.ReadFrame()
			if err != nil {
				if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
					// deadline 软超时：循环体重新检查 stop channel，不视为异常。
					// no-data 检测由独立 noDataTimer 负责，不依赖循环体执行。
					continue
				}
				// 调用方主动停止时 ReadFrame 会因 conn 关闭/超时返回错误，
				// 此时不应再修改共享状态——清理工作已由 Stop/Disconnect 完成。
				if driver.GetStopReason() != "" {
					return
				}
				a.handleConnectionLost(id, driver, err)
				return
			}
			if len(payload) > 0 {
				// 收到有效帧，续期 no-data timer。
				// Reset 是原子操作，无需加锁；即使 timer 已 fire 也能安全 Reset（time.AfterFunc 文档保证）。
				noDataTimer.Reset(p1604NoDataTimeout)
				a.processPayload(id, payload, driver)
			}
		}
	}
}

// handleConnectionLost 处理连接意外断开：清理共享状态并通知前端。
// 仅由 readLoop 在非主动停止场景调用。
//
// 与原实现差异（2026-07-03 改进）：
//   - 删除 shard.drivers[id]：让设备回到"未连接"状态，重连时不再被
//     "device already connected" 拒绝，用户无需先 Disconnect 即可直接 Connect
//   - 锁外关闭 driver.conn：释放底层 fd，避免长期累积导致 fd 泄漏
//
// 安全性论证：
//   - readLoop 是本函数的调用方，本函数 return 后 readLoop 也立即 return，
//     不存在访问已删除 driver 的窗口
//   - conn.Close 在锁外执行，避免持锁 I/O 阻塞同分片其他设备；
//     readLoop 已退出，无并发 ReadFrame 与 Close 竞争
func (a *P1604Adapter) handleConnectionLost(id string, driver *p1604Driver, cause error) {
	shard := a.shard(id)
	shard.mu.Lock()
	st, exists := shard.status[id]
	if !exists || st.Status == core.StatusDisconnected {
		shard.mu.Unlock()
		return
	}
	// 校验当前 drivers[id] 仍是本 readLoop 关联的 driver。
	// 若 Disconnect 已删除并启动新 driver，则放弃清理避免误伤新设备。
	if cur, ok := shard.drivers[id]; !ok || cur != driver {
		shard.mu.Unlock()
		return
	}
	delete(shard.sinks, id)
	if done, ok := shard.stopChs[id]; ok {
		close(done)
		delete(shard.stopChs, id)
	}
	if ch, ok := shard.channels[id]; ok {
		close(ch)
		delete(shard.channels, id)
	}
	// 删除 driver：让设备回到"未连接"状态，重连时不再被 already connected 拒绝。
	// status[id] 保留（设为 Error），前端设备列表仍能看到该设备及其错误信息，
	// 用户重新 Connect 时 Connect 函数会重新创建 driver 并覆盖 status。
	delete(shard.drivers, id)
	st.SetStatus(core.StatusError)
	st.Error = fmt.Sprintf("连接断开: %v", cause)
	st.AcquiringAt = 0
	// 关闭 pendingResponse 通道：让 ZeroCalibration 等待方立即感知断连，
	// 不必等到 p1604CalibrationTimeout 超时才返回。锁顺序：shard.mu → pendingResponseMu，
	// 与 processPayload（先释放 shard.mu 再取 pendingResponseMu）和 zeroCalibrationViaReadLoop
	// （仅持 pendingResponseMu）不冲突，无死锁风险。
	driver.pendingResponseMu.Lock()
	pendingCh := driver.pendingResponse
	driver.pendingResponse = nil
	if pendingCh != nil {
		close(pendingCh)
	}
	driver.pendingResponseMu.Unlock()
	shard.mu.Unlock()

	// 锁外关闭 conn：释放底层 fd 资源。
	// readLoop 已退出（本函数 return 后 readLoop 也 return），无并发竞争。
	if driver.conn != nil {
		_ = driver.conn.Close()
	}

	a.emitState(id)
	level := "error"
	if !sharedproto.IsConnectionFault(cause) {
		// 非典型连接故障（解析错误等）降级为 warn
		level = "warn"
	}
	a.emitLog(DeviceLogEntry{
		Level: level, Category: "hardware", DeviceID: id,
		Message: "Connection lost", Detail: cause.Error(),
	})
}

// processPayload 处理接收到的数据帧
//
// 日志策略：
//   - ASCII 响应帧（命令确认）：若 driver.pendingResponse 已注册（如零点校准等待响应），
//     优先路由到通道由等待方处理；否则打印 hardware-recv 日志，属于硬件通信信息，
//     频率低（仅连接/配置/启停时出现），可安全输出。
//   - 二进制数据帧（采集压力数据）：不打印每帧内容，因为采集期间帧率高达
//     1kHz × 多设备，逐帧打印会刷爆日志文件与前端面板。
//     仅在解析错误或通道数异常时打印 warn/debug 日志。
func (a *P1604Adapter) processPayload(id string, data []byte, driver *p1604Driver) {
	// 区分 ASCII 响应和二进制帧
	if sharedproto.IsASCIIFrame(data) {
		// 优先路由到等待中的命令响应通道（如 ZeroCalibration 采集期间路径）
		if driver != nil {
			driver.pendingResponseMu.Lock()
			ch := driver.pendingResponse
			if ch != nil {
				select {
				case ch <- data:
				default:
					// 通道满（多帧响应或等待方已超时退出但通道尚未清理）：
					// 按原逻辑记录日志，避免静默丢失通信证据
					a.emitLog(DeviceLogEntry{
						Level: "warn", Category: "hardware-recv", DeviceID: id,
						Message: "Command response dropped (pending channel full)",
						Detail:  strings.TrimSpace(string(data)),
					})
				}
				driver.pendingResponseMu.Unlock()
				return
			}
			driver.pendingResponseMu.Unlock()
		}
		// 无等待方：打印通信日志后忽略，不作为采集数据下发
		a.emitLog(DeviceLogEntry{
			Level: "info", Category: "hardware-recv", DeviceID: id,
			Message: "Command response", Detail: strings.TrimSpace(string(data)),
		})
		return
	}

	shard := a.shard(id)
	shard.mu.RLock()
	sink := shard.sinks[id]
	unit := "psi"
	useDeviceTs := true // 默认开启，与历史行为一致
	if st, ok := shard.status[id]; ok {
		if st.Profile.P1604Cfg.Unit != "" {
			unit = st.Profile.P1604Cfg.Unit
		}
		useDeviceTs = st.Profile.P1604Cfg.UseDeviceTimestampEnabled()
	}
	shard.mu.RUnlock()

	// 解析二进制数据帧
	// withDeviceTimestamp 与 StartAcquisition 下发的 content mask 一致：
	//   开启硬件时间戳 → 帧内含时间戳字段，解析并填充 deviceTimestampMs
	//   关闭硬件时间戳 → 帧内无时间戳字段，deviceTimestampMs 恒为 0，后续用系统时间
	channels, deviceTimestampMs, deviceTimestampSec, err := sharedproto.ParseStreamFrameEx(data, useDeviceTs, true)
	if err != nil {
		a.emitLog(DeviceLogEntry{
			Level: "debug", Category: "hardware", DeviceID: id,
			Message: "Frame parse error", Detail: err.Error(),
		})
		return
	}

	if len(channels) != p1604NumChannels {
		a.emitLog(DeviceLogEntry{
			Level: "warn", Category: "hardware", DeviceID: id,
			Message: "Unexpected channel count", Detail: fmt.Sprintf("expected %d, got %d", p1604NumChannels, len(channels)),
		})
		return
	}

	if sink != nil {
		snapshot := core.PressureSnapshot{
			DeviceID:  id,
			Timestamp: core.TimestampMs(),
			Values:    channels,
			Unit:      unit,
		}
		// 设备时间戳转换为秒（float64），供 CSV 录制器的时间格式化使用。
		// 使用纳秒精度的 deviceTimestampSec（直接由协议帧的 uint32 秒 + uint32 fractional 计算），
		// 消除 ParseStreamFrameEx 返回 deviceTimestampMs 的 ms 精度截断——
		// 1000Hz 采样下每毫秒仅一帧，原 float64(deviceTimestampMs)/1000.0 精度损失
		// 会导致同毫秒内多帧时间戳相同，表现为 CSV 中每毫秒出现多行、部分毫秒被跳过。
		// useDeviceTs==false 时 deviceTimestampMs 恒为 0，deviceTimestampSec 也恒为 0，
		// HardwareTimestamp 留空，csv_recorder 会自动回退到系统时间（Timestamp 字段）
		if deviceTimestampMs > 0 {
			snapshot.HardwareTimestamp = deviceTimestampSec
		}
		sink(snapshot)
	}
}

// sendCommand 发送命令到设备
//
// 命令发送委托给 sharedproto.SendCommandNoNewline：
//   - 纯 ASCII，不带换行符（实测设备 w1601 模式下 \r\n 会导致 N05）
//   - 内部处理 write deadline 设置与清除
//
// 通信日志：通过 emit 回调打印 category=hardware-send 的日志，便于前端
// "通信" 分组展示。命令发送频率低（连接/配置/启停采集），不会刷屏。
func (d *p1604Driver) sendCommand(cmd string) error {
	if d.conn == nil {
		return fmt.Errorf("not connected")
	}
	if d.emit != nil {
		d.emit(DeviceLogEntry{
			Level: "info", Category: "hardware-send", DeviceID: d.profile.ID,
			Message: "Command sent", Detail: cmd,
		})
	}
	return sharedproto.SendCommandNoNewline(d.conn, cmd, p1604CommandTimeout)
}

// p1604MaxResidualFrameSkips 是 sendCommandACK 在等待 ACK 期间最多可跳过的
// 非 ASCII 帧数量上限。
//
// 设计依据：
//   - 正常情况下 StopAcquisition/Disconnect 后 socket 中残留压力帧不会超过 5 个
//     （采集 100ms 周期 × StopAcquisition 命令链 < 50ms 完成即停止数据流）
//   - 留 4 倍余量设为 20，覆盖极端积压场景
//   - 超过 20 帧仍非 ACK 时认定连接已混乱（可能帧对齐错位或设备异常），
//     让 5s watchdog 兜底触发 handleConnectionLost
const p1604MaxResidualFrameSkips = 20

// sendCommandACK 发送命令并读取设备 ACK 响应（自动跳过残留压力帧）。
//
// 【为什么需要跳帧】
//   - StopAcquisition/Disconnect 路径：readLoop 退出前已从 socket 读到压力数据帧
//     放入 frameReader 应用层缓冲；同时内核 socket 接收缓冲也可能堆积了
//     设备已发但应用没 Read 的压力帧。这些残留帧会让 sendCommandACK 内的
//     ReadFrame 读到二进制流帧（"\x01\x00\x00..."）而非 ASCII 'A' 应答，
//     导致 unexpected command response 错误并触发误判的 handleConnectionLost
//     （现场复现：快速启停 5 次后第 6 次 StopAcquisition 报
//     "stop stream: c 02 1 response: unexpected command response: \x01\x00\x00..."）
//   - StartAcquisition / ApplyConfig / ZeroCalibration 路径同样可能命中残留
//
// 【跳帧策略】
//   - 设备应答帧只有两种 ASCII 形式：
//     · 'A'（0x41）—— 命令成功
//     · 'N' 开头（0x4E）—— 命令拒绝（如 N05 等错误码）
//   - 压力数据帧第一字节固定为 0x01（二进制流帧头同步标记，见 ParseStreamFrameEx）
//   - 利用 IsASCIIFrame 区分：非 ASCII 帧 = 残留压力帧，丢弃后继续读下一帧
//
// 【双重超时保护】（ADR-009）
//  1. SetWriteDeadline / SetReadDeadline 设 2s 软超时——sendCommand 内部设
//     WriteDeadline，本方法设 ReadDeadline；都是正常路径的超时控制
//  2. watchdog 5s 硬兜底——应对现场 Windows 环境下 SetWriteDeadline/SetReadDeadline
//     到期后阻塞 Write/Read 仍不返回的故障。watchdog 必须在 sendCommand 之前启动，
//     覆盖 Write + Read 全流程。期间不持有任何锁，watchdog Close 后其他 goroutine
//     能正常清理
//
// 【P1-2.b 修复】（ADR-009 审计）
//   - 原代码 wdStop 在 sendCommand 之后启动，Write 阶段完全无 watchdog 覆盖。
//     当 SetWriteDeadline 失效（Write 无限阻塞）时，sendCommandACK 永远卡在
//     sendCommand，无法返回，最终只能靠进程级超时兜底。
//   - 修复后：wdStop 提到 sendCommand 之前，Write 阻塞 → watchdog 触发 Close →
//     Write 返回错误 → 返回 "watchdog triggered" 让调用方走断线清理。
//
// 【 watchdog 触发时】（wdStop 返回 false）
//   - conn 已被 close，Write/ReadFrame 必然已返回错误（close 解除阻塞）
//   - 返回连接错误让调用方通过 IsConnectionFault 识别并触发 handleConnectionLost
//   - 极端情况下 ackErr 仍为 nil（watchdog 在 ReadFrame 返回后立即触发），
//     构造 net.ErrClosed 保证调用方走断线清理路径
//
// 【Connect 阶段不走本方法】
//   - Connect 用 sendCommand + P1604ReadCommandACK 分开调用，
//     由 runConnectionHandshake 提供 watchdog，避免双重 watchdog
func (d *p1604Driver) sendCommandACK(cmd string) error {
	// wdStop 必须在 sendCommand 之前启动，覆盖 Write + Read 全流程（P1-2.b 修复）。
	// 原代码在 sendCommand 之后启动 wdStop，Write 阶段裸奔：SetWriteDeadline 失效时
	// sendCommand 永久阻塞，wdStop 永远不会被创建。
	// 使用 sharedproto.WatchdogClose（sync.Once 幂等）替代本地 watchdogClose，
	// 与 windlabx4 项目对齐，避免本地版本缺乏幂等保护的已知历史问题。
	wdStop := sharedproto.WatchdogClose(d.conn, p1604WatchdogTimeout)
	defer func() {
		// watchdog 未触发时主动停止；已触发时 Stop 是幂等空操作（sync.Once 保护）
		_ = wdStop()
	}()

	// Write 阶段：sendCommand 内部走 SendCommandNoNewline，仅 SetWriteDeadline。
	// 若 SetWriteDeadline 失效导致 Write 阻塞，watchdog 会在 p1604WatchdogTimeout
	// 后强制 Close conn 解除阻塞。
	if err := d.sendCommand(cmd); err != nil {
		// wdStop 返回 false 表示 watchdog 已触发（conn 已被 Close）。
		// 此时 Write 错误由 watchdog close 引起，附加 "watchdog triggered" 上下文
		// 让调用方走断线清理路径。
		if !wdStop() {
			return fmt.Errorf("%s write: %w (watchdog triggered, conn closed)", cmd, err)
		}
		return err
	}

	// 循环 ReadFrame 跳过残留压力帧，直到读到 ASCII ACK/Nxx 或超时
	// ADR-009 R0-12：记录循环开始时间，用于总预算耗尽判定。p1604WatchdogTimeout（5s）
	// 是硬兜底，p1604CommandResponseTimeout（2s）是单次 Read soft deadline。
	// 若 SetReadDeadline 正常兑现，soft deadline 先于 watchdog 触发，此时必须
	// 强制 Close conn 阻断迟到响应——否则迟到 ACK 会被下一条命令消费导致协议错位。
	// 捕获 softDeadline 到局部变量：测试会覆盖 p1604CommandResponseTimeout 全局变量
	// 加速，循环内必须使用快照值避免 data race。
	softDeadline := p1604CommandResponseTimeout
	overallStart := time.Now()
	var softTimeoutTriggered bool
	var ackErr error
	skipped := 0
	for ; skipped < p1604MaxResidualFrameSkips; skipped++ {
		// 每次迭代用剩余总预算（不超过 softDeadline）设置 deadline。
		// 剩余不足时立即触发 soft timeout，避免单次 Read 超过总预算后 watchdog
		// 才触发产生的窗口（迟到响应可能在该窗口内到达）。
		remaining := p1604WatchdogTimeout - time.Since(overallStart)
		if remaining <= 0 {
			// 总预算耗尽等同于 soft timeout：协议边界已不可信。
			softTimeoutTriggered = true
			ackErr = fmt.Errorf("read command response: %w", context.DeadlineExceeded)
			break
		}
		perReadDeadline := softDeadline
		if remaining < perReadDeadline {
			perReadDeadline = remaining
		}
		// 注意：SetReadDeadline 在故障电脑上可能失效，但 watchdog 会兜底
		_ = d.conn.SetReadDeadline(time.Now().Add(perReadDeadline))
		payload, readErr := d.frameReader.ReadFrame()
		if readErr != nil {
			// R0-12：soft deadline 兑现（net.Error.Timeout()==true）时协议边界已不可信，
			// 迟到响应可能随后进入 TCP 流被下一条命令消费。标记后强制 Close conn。
			if netErr, ok := readErr.(net.Error); ok && netErr.Timeout() {
				softTimeoutTriggered = true
			}
			ackErr = readErr
			break
		}
		// 命中 ASCII 应答帧（'A' 或 'N' 开头）：退出循环处理结果
		if len(payload) > 0 && sharedproto.IsASCIIFrame(payload) {
			ackErr = interpretACKPayload(payload, cmd)
			break
		}
		// 否则是残留压力帧（二进制，第一字节通常 0x01）：丢弃后继续读下一帧
	}
	// 循环正常结束（skipped == 上限）且未读到 ACK/错误时，认定连接已混乱：
	// 设备可能持续发压力流（StopAcquisition 未生效）或帧对齐错位，必须返回错误
	// 避免误报成功导致调用方继续后续操作（Critical bug 修复）
	if ackErr == nil && skipped == p1604MaxResidualFrameSkips {
		ackErr = fmt.Errorf("too many residual frames (>%d) while waiting for ACK", p1604MaxResidualFrameSkips)
	}
	// 跳过残留帧时记 debug 日志，便于现场排查（正常情况 skipped=0 不记）
	if skipped > 0 && ackErr == nil {
		slog.Debug("WISPA sendCommandACK skipped residual frames",
			"device", d.profile.ID, "cmd", cmd, "skipped", skipped)
	}

	// ADR-009 R0-12：soft timeout 触发时强制 Close conn 阻断迟到响应，
	// 统一返回 ErrWatchdogTriggered 让调用方（StartAcquisition / StopAcquisition /
	// ApplyConfig / ZeroCalibration）通过 IsWatchdogTriggered 毒化驱动状态。
	// 与 watchdog 触发路径共享同一返回格式，调用方只需 IsWatchdogTriggered 判定。
	if softTimeoutTriggered {
		_ = d.conn.Close()
		if ackErr == nil {
			ackErr = net.ErrClosed
		}
		return fmt.Errorf("%s response: %w; %w", cmd, ackErr, sharedproto.ErrWatchdogTriggered)
	}

	// watchdog 检查：返回 false 表示已触发 close，conn 已废
	if !wdStop() {
		// watchdog 触发：conn 已被 close。ReadFrame 此时必然已返回错误
		// （close 解除阻塞）。极端情况下 ackErr 仍为 nil（watchdog 在
		// ReadFrame 返回后立即触发），构造 net.ErrClosed 让调用方走断线清理
		if ackErr == nil {
			ackErr = net.ErrClosed
		}
		return fmt.Errorf("%s response: %w; %w", cmd, ackErr, sharedproto.ErrWatchdogTriggered)
	}

	// 清除 deadline，避免影响后续 readLoop 或命令读取
	// 仅在 conn 仍存活时调用（soft timeout / watchdog 未触发）
	_ = d.conn.SetReadDeadline(time.Time{})

	if ackErr != nil {
		return fmt.Errorf("%s response: %w", cmd, ackErr)
	}

	// 成功路径：设备回 A
	if d.emit != nil {
		d.emit(DeviceLogEntry{
			Level: "info", Category: "hardware-recv", DeviceID: d.profile.ID,
			Message: "Command response", Detail: cmd + " -> A",
		})
	}
	return nil
}

// interpretACKPayload 解析 ASCII 应答 payload，返回 nil 表示成功（'A'），
// 返回 error 表示设备拒绝（'N' 开头）或非预期应答。
// 仅处理 ASCII 帧（调用方已用 IsASCIIFrame 校验），二进制压力帧不应进入本函数。
func interpretACKPayload(payload []byte, cmd string) error {
	response := string(payload)
	if response == "A" {
		return nil
	}
	if strings.HasPrefix(response, "N") {
		return fmt.Errorf("device returned error: %s", response)
	}
	return fmt.Errorf("unexpected command response: %q", response)
}

// 注：isConnectionFault 已下沉到 shared.local/device-sdk/go/protocol（conn_helpers.go），
// 调用处直接使用 sharedproto.IsConnectionFault。
