package hardware

import (
	"fmt"
	"log/slog"
	"net"
	"strings"
	"sync"
	"time"

	sharedproto "shared.local/device-sdk/go/protocol"

	"daq-p1604/core"
	"daq-p1604/ports"
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
	// p1604W1601DrainTimeout 排空 w1601 启用应答的最长等待时间。
	// w1601 的 A 响应通常即时返回，100ms 内未读到则视为设备未发应答，继续后续流程。
	p1604W1601DrainTimeout = 100 * time.Millisecond

	// p1604KeepAlivePeriod TCP keepalive 探测间隔。
	// 解决物理拔网线场景下 TCP 层无 RST/FIN 包的问题：
	// readLoop 的 SetReadDeadline(200ms) 仅返回 timeout 错误（被 continue 忽略），
	// 导致设备状态长期停留在 Acquiring 而无法触发 handleConnectionLost。
	// 启用 keepalive 后，内核周期性探测对端，连续失败会返回非 timeout 错误，
	// readLoop 据此判定真断连并进入清理流程。
	//
	// 检测时间估算（Go 标准库 SetKeepAlivePeriod 同时设置 KEEPIDLE 与 KEEPINTVL）：
	//   - Linux：10s idle + 10s interval × 9 probes（默认 tcp_keepalive_probes）= ~100s
	//   - Windows：10s idle + 10s interval × 10 retries（默认）= ~110s
	// 远优于 Linux 默认 7200s/9 探测 = 2 小时。
	// 注：Go 未暴露 TCP_KEEPCNT 设置，探测次数依赖系统默认值。
	p1604KeepAlivePeriod = 10 * time.Second
)

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
	mu        sync.RWMutex
	drivers   map[string]*p1604Driver
	status    map[string]*core.DeviceState
	sinks     map[string]func(core.PressureSnapshot)
	channels  map[string]chan core.PressureSnapshot
	stopChs   map[string]chan struct{}
}

// P1604Adapter DAQ-P-1604 硬件适配器（16 分片锁）
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
	// 主动停止原因追踪：嵌入 sharedproto.StopReasonTracker
	// 提供 SetStopReason / GetStopReason / ClearStopReason 方法，跨项目复用
	sharedproto.StopReasonTracker
}

// NewP1604Adapter 创建 P1604 硬件适配器
func NewP1604Adapter() *P1604Adapter {
	a := &P1604Adapter{}
	for i := range a.shards {
		a.shards[i] = &p1604Shard{
			drivers:  make(map[string]*p1604Driver),
			status:   make(map[string]*core.DeviceState),
			sinks:    make(map[string]func(core.PressureSnapshot)),
			channels: make(map[string]chan core.PressureSnapshot),
			stopChs:  make(map[string]chan struct{}),
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
//   - Windows 上 SetKeepAlivePeriod 等同于设置整个 keepalive 间隔（idle+interval 合并），
//     无法精细控制 idle/count，但 10s 间隔 × 系统默认探测次数已满足 ~100s 检测目标
//
// 返回值语义：失败时返回错误，调用方决定是否中止。keepalive 是拔线检测的优化项
// 而非连接正确性前提，受限环境（沙箱/低权限）下 SetKeepAlive 可能失败，
// 调用方可选择仅记 warn 不中止，退化为 readLoop 的 SetReadDeadline 超时检测。
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

// Connect 连接设备
// 锁策略：仅在读写共享状态时持锁，TCP 拨号和 w1601 命令在锁外执行
//
// 连接后单位同步流程：
//  1. 发送 w1601 启用长度前缀，排空 w1601 的 A 应答
//  2. 发送 u01101 读取硬件 EU 压力转换系数，识别硬件当前压力单位
//  3. 若硬件单位与 profile 配置不一致，以硬件为准更新 profile
//     （硬件是数据源，配置侧标签必须与硬件一致，否则数值与单位不匹配）
//  4. 若读不到硬件单位（如设备不支持 u01101），保留 profile 单位并记录 warn
func (a *P1604Adapter) Connect(profile core.PressureProfile) error {
	shard := a.shard(profile.ID)
	shard.mu.Lock()
	if _, exists := shard.drivers[profile.ID]; exists {
		shard.mu.Unlock()
		return fmt.Errorf("device %s already connected", profile.ID)
	}
	shard.mu.Unlock()

	host := profile.Address
	if host == "" {
		host = p1604DefaultHost
	}
	port := profile.Port
	if port <= 0 {
		port = p1604DefaultPort
	}

	// TCP 拨号在锁外执行，避免阻塞其他设备操作
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("%s:%d", host, port), p1604ConnectTimeout)
	if err != nil {
		return fmt.Errorf("connect to %s:%d: %w", host, port, err)
	}

	// TCP 连接成功，打印硬件通信日志（前端 "通信" 分组可见）
	a.emitLog(DeviceLogEntry{
		Level: "info", Category: "hardware-recv", DeviceID: profile.ID,
		Message: "TCP connected", Detail: fmt.Sprintf("%s:%d", host, port),
	})

	// 启用 TCP keepalive：物理拔网线场景下 TCP 层无 RST/FIN，
	// 仅靠 readLoop 的 SetReadDeadline 无法判定真断连（超时被 continue 忽略）。
	// keepalive 探测失败会触发非 timeout 错误，readLoop 据此进入 handleConnectionLost。
	// 启用失败不中止连接——keepalive 是优化项，受限环境下退化为 readDeadline 检测。
	if err := enableTCPKeepalive(conn); err != nil {
		a.emitLog(DeviceLogEntry{
			Level: "warn", Category: "hardware", DeviceID: profile.ID,
			Message: "TCP keepalive enable failed, fallback to read-deadline detection",
			Detail:  err.Error(),
		})
	}

	driver := &p1604Driver{
		profile:     profile,
		conn:        conn,
		frameReader: sharedproto.NewFrameReader(conn),
		emit:        a.emitLog,
	}

	// 连接后必须先发 w1601 启用长度前缀模式
	if err := driver.sendCommand("w1601"); err != nil {
		conn.Close()
		return fmt.Errorf("enable length prefix: %w", err)
	}
	time.Sleep(50 * time.Millisecond)

	// 排空 w1601 的 A 应答，避免污染后续 u01101 响应
	sharedproto.DrainW1601Response(driver.frameReader, conn, p1604W1601DrainTimeout)
	// 打印 w1601 应答接收日志（A 应答已被排空丢弃，此处仅记录通信事件）
	a.emitLog(DeviceLogEntry{
		Level: "info", Category: "hardware-recv", DeviceID: profile.ID,
		Message: "Command response", Detail: "w1601 -> A (ack, drained)",
	})

	// 读取硬件当前 EU 压力转换系数，识别硬件单位
	// 读硬件失败不阻断连接（兼容旧固件或模拟器），仅记录 warn
	hwUnit, unitNote := a.syncUnitFromHardware(driver, profile)

	shard.mu.Lock()
	// 二次检查：拨号期间可能已被其他 goroutine 连接
	if _, exists := shard.drivers[profile.ID]; exists {
		shard.mu.Unlock()
		conn.Close()
		return fmt.Errorf("device %s already connected", profile.ID)
	}
	// 若硬件单位与 profile 不一致，以硬件为准更新 driver.profile
	if hwUnit != "" && hwUnit != profile.P1604Cfg.Unit {
		profile.P1604Cfg.Unit = hwUnit
		driver.profile = profile
	}
	shard.drivers[profile.ID] = driver
	shard.status[profile.ID] = &core.DeviceState{
		Profile:     profile,
		Status:      core.StatusConnected,
		StatusText:  core.StatusConnected.String(),
		ConnectedAt: core.TimestampMs(),
	}
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
// 返回值：
//   - unit: 识别到的硬件单位（如 "psi"、"kPa"）；读取失败返回 ""
//   - note: 给日志使用的简短描述（如 "unit=psi (coeff=1.000000)"）
//
// 通信日志：u01101 命令通过 sharedproto.P1604ReadUnitCoefficient 发送，
// 不走 driver.sendCommand，故在此补充 hardware-send/hardware-recv 日志，
// 让前端 "通信" 分组能看到完整的连接阶段命令交互。
func (a *P1604Adapter) syncUnitFromHardware(driver *p1604Driver, profile core.PressureProfile) (string, string) {
	// 打印 u01101 命令发送日志
	a.emitLog(DeviceLogEntry{
		Level: "info", Category: "hardware-send", DeviceID: profile.ID,
		Message: "Command sent", Detail: "u01101 (read unit coefficient)",
	})
	coeff, err := sharedproto.P1604ReadUnitCoefficient(driver.frameReader, driver.conn, p1604UnitSyncTimeout)
	if err != nil {
		// 打印 u01101 响应失败日志（通信层）
		a.emitLog(DeviceLogEntry{
			Level: "warn", Category: "hardware-recv", DeviceID: profile.ID,
			Message: "Command response error", Detail: fmt.Sprintf("u01101: %v", err),
		})
		// 读硬件失败：保留 profile 单位，记录 warn（不阻断连接）
		a.emitLog(DeviceLogEntry{
			Level: "warn", Category: "hardware", DeviceID: profile.ID,
			Message: "Read hardware unit failed, keep profile unit",
			Detail:  fmt.Sprintf("%v | profile=%s", err, profile.P1604Cfg.Unit),
		})
		return "", fmt.Sprintf("unit=%s (hardware read failed)", profile.P1604Cfg.Unit)
	}
	// 打印 u01101 响应日志（通信层，记录解析出的系数）
	a.emitLog(DeviceLogEntry{
		Level: "info", Category: "hardware-recv", DeviceID: profile.ID,
		Message: "Command response", Detail: fmt.Sprintf("u01101 -> coeff=%f", coeff),
	})
	hwUnit, matched := sharedproto.P1604MatchUnitByCoefficient(coeff)
	if !matched {
		// 系数不在标准表内：保留 profile 单位，记录 warn 并暴露实际系数便于排查
		a.emitLog(DeviceLogEntry{
			Level: "warn", Category: "hardware", DeviceID: profile.ID,
			Message: "Hardware unit coefficient unknown",
			Detail:  fmt.Sprintf("coeff=%f | profile=%s", coeff, profile.P1604Cfg.Unit),
		})
		return "", fmt.Sprintf("unit=%s (hardware coeff=%f unknown)", profile.P1604Cfg.Unit, coeff)
	}
	if hwUnit != profile.P1604Cfg.Unit {
		// 硬件与 profile 不一致：以硬件为准
		a.emitLog(DeviceLogEntry{
			Level: "info", Category: "hardware", DeviceID: profile.ID,
			Message: "Unit synced from hardware",
			Detail:  fmt.Sprintf("profile=%s -> hardware=%s (coeff=%f)", profile.P1604Cfg.Unit, hwUnit, coeff),
		})
		return hwUnit, fmt.Sprintf("unit=%s (synced from hardware, coeff=%f)", hwUnit, coeff)
	}
	return hwUnit, fmt.Sprintf("unit=%s (coeff=%f)", hwUnit, coeff)
}

// 注：drainW1601Response 已下沉到 shared.local/device-sdk/go/protocol（conn_helpers.go），
// 调用处直接使用 sharedproto.DrainW1601Response。

// Disconnect 断开设备连接
//
// 关闭顺序对竞态修复至关重要：
//  1. 锁内：标记主动停止原因 + close(stop) 通知 readLoop 退出 + 清理共享状态。
//  2. 锁外：等待 readLoop 退出（join），确保它不再持有 driver.conn 引用。
//  3. 锁外：发送停止命令、conn.Close。
func (a *P1604Adapter) Disconnect(id string) error {
	shard := a.shard(id)
	shard.mu.Lock()
	driver, ok := shard.drivers[id]
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
	if driver != nil && wasAcquiring {
		driver.joinReadLoop(id, sharedproto.ReadLoopJoinTimeout)
	}

	// 在锁外执行 I/O：发送停止命令和关闭连接
	if wasAcquiring && connected && driver != nil {
		if err := driver.sendCommand("c 02 1"); err != nil {
			if sharedproto.IsConnectionFault(err) {
				slog.Debug("DAQ-P-1604 stop stream on disconnect: connection already gone", "device", id, "error", err)
			} else {
				slog.Warn("DAQ-P-1604 stop stream on disconnect failed", "device", id, "error", err)
			}
		}
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
	return nil
}

// StartAcquisition 启动数据采集
// 锁策略：仅在状态检查和状态更新时持锁，所有 sendCommand 和 Sleep 在锁外执行
func (a *P1604Adapter) StartAcquisition(id string) (<-chan core.PressureSnapshot, error) {
	shard := a.shard(id)
	shard.mu.Lock()
	driver, ok := shard.drivers[id]
	if !ok {
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

	// 以下命令在锁外执行，避免阻塞其他设备操作
	// 配置数据流参数：c 00 <st> <mask> <sync> <per> <fmt> <mode>
	periodMs := driver.profile.P1604Cfg.SamplingRate
	if err := driver.sendCommand(fmt.Sprintf("c 00 1 FFFF 1 %d 7 0", periodMs)); err != nil {
		a.rollbackAcquisition(id, ch, done)
		return nil, fmt.Errorf("set stream params: %w", err)
	}
	time.Sleep(50 * time.Millisecond)

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
	if err := driver.sendCommand(fmt.Sprintf("c 05 1 %s", contentMaskHex)); err != nil {
		a.rollbackAcquisition(id, ch, done)
		return nil, fmt.Errorf("set stream content: %w", err)
	}
	time.Sleep(50 * time.Millisecond)

	// 启动数据流
	if err := driver.sendCommand("c 01 1"); err != nil {
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
	if st, exists := shard.status[id]; exists {
		st.SetStatus(core.StatusAcquiring)
		st.AcquiringAt = core.TimestampMs()
	}
	shard.mu.Unlock()

	// 启动读取循环
	go a.readLoop(id, driver, done)

	return ch, nil
}

// rollbackAcquisition 启动失败时回滚采集状态
// 在锁外执行停止命令，并在锁内清理采集槽位
func (a *P1604Adapter) rollbackAcquisition(id string, ch chan core.PressureSnapshot, done chan struct{}) {
	// 尝试停止可能已部分配置的流（在锁外执行 I/O）。
	// 启动失败常见原因之一就是连接异常，用 isConnectionFault 把这种场景从 warn 降到 debug。
	if err := a.driverSendCommandSafe(id, "c 02 1"); err != nil {
		if sharedproto.IsConnectionFault(err) {
			slog.Debug("DAQ-P-1604 rollback stop stream: connection already gone", "device", id, "error", err)
		} else {
			slog.Warn("DAQ-P-1604 rollback stop stream failed", "device", id, "error", err)
		}
	}
	close(done)

	shard := a.shard(id)
	shard.mu.Lock()
	delete(shard.channels, id)
	delete(shard.stopChs, id)
	delete(shard.sinks, id)
	shard.mu.Unlock()
	close(ch)
}

// driverSendCommandSafe 在锁内安全获取 driver 后发送命令
func (a *P1604Adapter) driverSendCommandSafe(id, cmd string) error {
	shard := a.shard(id)
	shard.mu.RLock()
	driver, ok := shard.drivers[id]
	shard.mu.RUnlock()
	if !ok || driver == nil {
		return fmt.Errorf("device %s not connected", id)
	}
	return driver.sendCommand(cmd)
}

// StopAcquisition 停止数据采集
//
// 与 Disconnect 相同的关闭顺序：
//  1. 锁内：标记主动停止 + close(stop) + 清理共享状态。
//  2. 锁外：等待 readLoop 退出。
//  3. 锁外：发送停止命令（不关 conn，连接保留以便后续重新 StartAcquisition）。
func (a *P1604Adapter) StopAcquisition(id string) error {
	shard := a.shard(id)
	shard.mu.Lock()
	driver, ok := shard.drivers[id]
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
	if driver != nil && wasAcquiring {
		driver.joinReadLoop(id, sharedproto.ReadLoopJoinTimeout)
	}

	// 仅在确实在采集且连接有效时，才在锁外发送停止命令
	if wasAcquiring && connected && driver != nil {
		if err := driver.sendCommand("c 02 1"); err != nil {
			if sharedproto.IsConnectionFault(err) {
				slog.Debug("DAQ-P-1604 stop stream: connection already gone", "device", id, "error", err)
			} else {
				slog.Warn("DAQ-P-1604 stop stream command failed", "device", id, "error", err)
			}
		}
	}
	return nil
}

// joinReadLoop 等待 readLoop 关闭其 done channel；超时仅日志，不阻塞调用方。
// 调用方通常先 close(stop) + 标记 stopReason，然后调用本方法等待 readLoop 退出，
// 再安全 conn.Close。driver 为 nil 或 readLoop 未启动时直接返回。
func (d *p1604Driver) joinReadLoop(id string, timeout time.Duration) {
	if d == nil || d.readLoopDone == nil {
		return
	}
	select {
	case <-d.readLoopDone:
	case <-time.After(timeout):
		slog.Warn("DAQ-P-1604 readLoop join timeout", "device", id, "timeout", timeout)
	}
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
func (a *P1604Adapter) ApplyConfig(id string, cfg core.P1604Config) error {
	shard := a.shard(id)
	shard.mu.Lock()
	st, exists := shard.status[id]
	driver, hasDriver := shard.drivers[id]
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
		coeff, ok := sharedproto.P1604PressureUnitCoefficient[cfg.Unit]
		if !ok {
			return fmt.Errorf("unsupported unit: %s", cfg.Unit)
		}
		// 清理 FrameReader 内部 buf 与 TCP 接收缓冲区的残留数据。
		// 残留来源：上次采集停止后 readLoop 退出时未读空的二进制流数据帧，
		// 或上一条命令延迟到达的应答。若不清理，P1604WriteUnitCoefficient 的
		// ReadFrame 会把残留当作 v01101 响应读出，触发
		// "unexpected v01101 response: <二进制乱码>"。
		// 必须先 fr.Reset()（清 buf）再 DrainConnection（清 TCP 缓冲区），
		// 顺序不能反：DrainConnection 读裸字节，不会清 FrameReader.buf。
		driver.frameReader.Reset()
		if drained := sharedproto.DrainConnection(driver.conn, p1604W1601DrainTimeout); drained > 0 {
			slog.Debug("DAQ-P-1604 drained residual data before ApplyConfig",
				"device", id, "bytes", drained)
		}
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
//  2. ReadFrame 返回非 timeout 错误：
//     - 若 driver.stopReason 已置位（调用方刚刚 close(stop) 但 select 尚未轮到）→
//       静默退出，不修改任何共享状态（避免与 StopAcquisition/Disconnect 的清理冲突）。
//     - 否则视为连接意外断开，标记设备为 Error 状态并通知前端。
//
// 退出前必须 close(readLoopDone)，让 Disconnect/StopAcquisition 能 join 到本协程。
func (a *P1604Adapter) readLoop(id string, driver *p1604Driver, stop <-chan struct{}) {
	defer close(driver.readLoopDone)

	for {
		select {
		case <-stop:
			return
		default:
			driver.conn.SetReadDeadline(time.Now().Add(p1604ReadTimeout))
			payload, err := driver.frameReader.ReadFrame()
			if err != nil {
				if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
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
				a.processPayload(id, payload)
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
//   - ASCII 响应帧（命令确认）：打印 hardware-recv 日志，属于硬件通信信息，
//     频率低（仅连接/配置/启停时出现），可安全输出。
//   - 二进制数据帧（采集压力数据）：不打印每帧内容，因为采集期间帧率高达
//     1kHz × 多设备，逐帧打印会刷爆日志文件与前端面板。
//     仅在解析错误或通道数异常时打印 warn/debug 日志。
func (a *P1604Adapter) processPayload(id string, data []byte) {
	// 区分 ASCII 响应和二进制帧
	if sharedproto.IsASCIIFrame(data) {
		// ASCII 响应（命令确认等）：打印通信日志后忽略，不作为采集数据下发
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

// 注：isConnectionFault 已下沉到 shared.local/device-sdk/go/protocol（conn_helpers.go），
// 调用处直接使用 sharedproto.IsConnectionFault。
