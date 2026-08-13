package hardware

import (
	"encoding/binary"
	"errors"
	"io"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"shared.local/device-sdk/go/daq/core"
	"shared.local/device-sdk/go/protocol/modbus"
)

// serveT1602FakeDevice 在 server 端模拟 T1602 固件：循环读请求 ADU，
// 调 handler 得响应 PDU，写回响应 ADU（回显 Transaction ID / Unit ID）。
// handler 返回 nil 表示不响应（模拟超时）。连接关闭或帧错误时退出。
func serveT1602FakeDevice(server net.Conn, handler func(unitID uint8, pdu []byte) []byte) {
	for {
		header := make([]byte, 7)
		if _, err := io.ReadFull(server, header); err != nil {
			return
		}
		body := make([]byte, int(binary.BigEndian.Uint16(header[4:6]))-1)
		if _, err := io.ReadFull(server, body); err != nil {
			return
		}
		resp := handler(header[6], body)
		if resp == nil {
			continue
		}
		out := make([]byte, 0, 7+len(resp))
		out = append(out, header[0:4]...)
		out = binary.BigEndian.AppendUint16(out, uint16(len(resp)+1))
		out = append(out, header[6])
		out = append(out, resp...)
		if _, err := server.Write(out); err != nil {
			return
		}
	}
}

// t1602FakeDevice 是 T1602 的内存模型：16 通道类型码 + 16 通道 raw 数据，
// 按 spec 寄存器映射响应 FC3/FC4/FC6。
type t1602FakeDevice struct {
	types [t1602ChannelCount]uint8
	raws  [t1602ChannelCount]uint16
}

func (d *t1602FakeDevice) handle(unitID uint8, pdu []byte) []byte {
	card := int(unitID) - t1602UnitIDCard1
	if card < 0 || card >= t1602CardCount || len(pdu) < 5 {
		return []byte{pdu[0] | 0x80, 0x02}
	}
	addr := binary.BigEndian.Uint16(pdu[1:3])
	switch pdu[0] {
	case 0x03: // FC3 读类型 Holding 200~207
		if addr != t1602TypeRegBase {
			return []byte{0x83, 0x02}
		}
		resp := []byte{0x03, 2 * t1602ChannelsPerCard}
		for i := 0; i < t1602ChannelsPerCard; i++ {
			resp = binary.BigEndian.AppendUint16(resp, uint16(d.types[card*t1602ChannelsPerCard+i]))
		}
		return resp
	case 0x04: // FC4 读数据 Input 0~7
		if addr != t1602DataRegBase {
			return []byte{0x84, 0x02}
		}
		resp := []byte{0x04, 2 * t1602ChannelsPerCard}
		for i := 0; i < t1602ChannelsPerCard; i++ {
			resp = binary.BigEndian.AppendUint16(resp, d.raws[card*t1602ChannelsPerCard+i])
		}
		return resp
	case 0x06: // FC6 写单寄存器（类型 200~207）
		offset := int(addr) - t1602TypeRegBase
		if offset < 0 || offset >= t1602ChannelsPerCard {
			return []byte{0x86, 0x02}
		}
		d.types[card*t1602ChannelsPerCard+offset] = uint8(binary.BigEndian.Uint16(pdu[3:5]))
		return pdu // 原样回显
	default:
		return []byte{pdu[0] | 0x80, 0x01}
	}
}

// newT1602TestDevice 建立 net.Pipe + fake 设备，返回已注入 dialTCP 的驱动。
func newT1602TestDevice(fake *t1602FakeDevice) (*DAQT1602, net.Conn) {
	client, server := net.Pipe()
	go serveT1602FakeDevice(server, fake.handle)
	device := NewDAQT1602(core.Profile{
		ID:      "t1602-test",
		Type:    core.DeviceDaqT1602,
		Address: "192.168.3.201",
		Port:    502,
	})
	device.dialTCP = func(string, string, time.Duration) (net.Conn, error) { return client, nil }
	return device, server
}

func TestDAQT1602ConnectSyncsChannelTypes(t *testing.T) {
	fake := &t1602FakeDevice{}
	for i := 0; i < t1602ChannelsPerCard; i++ {
		fake.types[i] = uint8(i) // 卡1：0~7 全类型码
		fake.types[t1602ChannelsPerCard+i] = 2
	}
	device, server := newT1602TestDevice(fake)
	defer server.Close()

	synced := make(chan core.DaqT1602HardwareConfig, 1)
	device.OnConfigSynced(func(cfg core.DaqT1602HardwareConfig) { synced <- cfg })

	if err := device.Connect(); err != nil {
		t.Fatalf("Connect returned error: %v", err)
	}
	defer device.Disconnect()

	select {
	case cfg := <-synced:
		if cfg.TypeCodes != fake.types {
			t.Fatalf("synced TypeCodes = %v, want %v", cfg.TypeCodes, fake.types)
		}
	case <-time.After(testReadTimeout):
		t.Fatal("OnConfigSynced was not called on Connect")
	}
	if got := device.Status().Connection; got != core.ConnectionConnected {
		t.Fatalf("connection status = %v, want %v", got, core.ConnectionConnected)
	}
	got, err := device.GetDaqT1602Config()
	if err != nil {
		t.Fatalf("GetDaqT1602Config returned error: %v", err)
	}
	if got.TypeCodes != fake.types {
		t.Fatalf("config TypeCodes = %v, want %v", got.TypeCodes, fake.types)
	}
}

func TestDAQT1602ApplyConfigWritesAndVerifies(t *testing.T) {
	fake := &t1602FakeDevice{}
	device, server := newT1602TestDevice(fake)
	defer server.Close()
	if err := device.Connect(); err != nil {
		t.Fatalf("Connect returned error: %v", err)
	}
	defer device.Disconnect()

	var want [t1602ChannelCount]uint8
	for i := range want {
		want[i] = uint8(i % 8)
	}
	if err := device.ApplyDaqT1602Config(core.DaqT1602HardwareConfig{TypeCodes: want}); err != nil {
		t.Fatalf("ApplyDaqT1602Config returned error: %v", err)
	}
	if fake.types != want {
		t.Fatalf("device types = %v, want %v", fake.types, want)
	}
	got, _ := device.GetDaqT1602Config()
	if got.TypeCodes != want {
		t.Fatalf("saved TypeCodes = %v, want %v", got.TypeCodes, want)
	}
}

func TestDAQT1602ApplyConfigVerifyMismatchFails(t *testing.T) {
	fake := &t1602FakeDevice{}
	device, server := newT1602TestDevice(fake)
	defer server.Close()
	if err := device.Connect(); err != nil {
		t.Fatalf("Connect returned error: %v", err)
	}
	defer device.Disconnect()

	// 损坏 fake 设备的 FC6：写入但不生效（读回旧值），校验必须失败。
	client, brokenServer := net.Pipe()
	defer brokenServer.Close()
	go serveT1602FakeDevice(brokenServer, func(unitID uint8, pdu []byte) []byte {
		if pdu[0] == 0x06 {
			return pdu // 回显成功但不改 types
		}
		return fake.handle(unitID, pdu)
	})
	device.dialTCP = func(string, string, time.Duration) (net.Conn, error) { return client, nil }
	if err := device.Disconnect(); err != nil {
		t.Fatalf("Disconnect returned error: %v", err)
	}
	if err := device.Connect(); err != nil {
		t.Fatalf("reconnect returned error: %v", err)
	}

	var want [t1602ChannelCount]uint8
	want[0] = 5
	err := device.ApplyDaqT1602Config(core.DaqT1602HardwareConfig{TypeCodes: want})
	if err == nil || !strings.Contains(err.Error(), "verify") {
		t.Fatalf("ApplyDaqT1602Config error = %v, want verify mismatch", err)
	}
	if got := device.Status().Connection; got != core.ConnectionConnected {
		// 校验失败属业务错误（设备拒绝了写入生效），连接不应被毒化。
		t.Fatalf("connection status = %v, want %v (verify failure must not poison conn)", got, core.ConnectionConnected)
	}
}

func TestDAQT1602ApplyConfigRejectedWhileAcquiring(t *testing.T) {
	fake := &t1602FakeDevice{}
	device, server := newT1602TestDevice(fake)
	defer server.Close()
	if err := device.Connect(); err != nil {
		t.Fatalf("Connect returned error: %v", err)
	}
	defer device.Disconnect()
	if err := device.StartAcquisition(); err != nil {
		t.Fatalf("StartAcquisition returned error: %v", err)
	}
	defer device.StopAcquisition()

	err := device.ApplyDaqT1602Config(core.DaqT1602HardwareConfig{})
	if err == nil {
		t.Fatal("ApplyDaqT1602Config while acquiring should be rejected")
	}
}

func TestDAQT1602AcquisitionEmitsConvertedTemps(t *testing.T) {
	fake := &t1602FakeDevice{}
	for i := range fake.types {
		fake.types[i] = 2 // 全 T 型
	}
	// T 型量程 (0,1300)：raw 65535 → 1300℃，raw 0 → 0℃（公式端点精确值）。
	fake.raws[0] = 65535
	fake.raws[1] = 0
	// 卡2 CH0（全局 CH8）改 K 型（0,1200）验证跨卡类型映射。
	fake.types[8] = 1
	fake.raws[8] = 65535

	device, server := newT1602TestDevice(fake)
	defer server.Close()
	payloads := make(chan core.DataPayload, 4)
	// sink 必须非阻塞：readLoop 在 sink 内同步发送，缓冲打满会阻塞轮询循环，
	// 导致 Stop join 超时（驱动与既有驱动的 sink 契约一致：sink 不得长时间阻塞）。
	device.SetDataSink(func(p core.DataPayload) {
		select {
		case payloads <- p:
		default:
		}
	})
	if err := device.Connect(); err != nil {
		t.Fatalf("Connect returned error: %v", err)
	}
	defer device.Disconnect()
	if err := device.StartAcquisition(); err != nil {
		t.Fatalf("StartAcquisition returned error: %v", err)
	}

	select {
	case p := <-payloads:
		if len(p.Channels) != t1602ChannelCount {
			t.Fatalf("channel count = %d, want %d", len(p.Channels), t1602ChannelCount)
		}
		if p.Channels[0] != 1300 {
			t.Fatalf("CH0 = %v, want 1300 (T 型 raw 65535)", p.Channels[0])
		}
		if p.Channels[1] != 0 {
			t.Fatalf("CH1 = %v, want 0 (T 型 raw 0)", p.Channels[1])
		}
		if p.Channels[8] != 1200 {
			t.Fatalf("CH8 = %v, want 1200 (K 型 raw 65535)", p.Channels[8])
		}
		if len(p.ChannelIndices) != t1602ChannelCount || p.ChannelIndices[15] != 15 {
			t.Fatalf("ChannelIndices = %v, want 0~15", p.ChannelIndices)
		}
	case <-time.After(testReadTimeout):
		t.Fatal("did not receive payload")
	}

	if err := device.StopAcquisition(); err != nil {
		t.Fatalf("StopAcquisition returned error: %v", err)
	}
	if got := device.Status().Connection; got != core.ConnectionConnected {
		t.Fatalf("connection status = %v, want %v after stop", got, core.ConnectionConnected)
	}
}

func TestDAQT1602StartRequiresConnect(t *testing.T) {
	device := NewDAQT1602(core.Profile{ID: "t1602-not-connected", Type: core.DeviceDaqT1602})
	if err := device.StartAcquisition(); err == nil {
		t.Fatal("StartAcquisition without connect should fail")
	}
	// 未连接时 Apply 仅保存本地，不做硬件 I/O。
	want := core.DaqT1602HardwareConfig{TypeCodes: [16]uint8{1, 1, 1}}
	if err := device.ApplyDaqT1602Config(want); err != nil {
		t.Fatalf("ApplyDaqT1602Config offline returned error: %v", err)
	}
	got, _ := device.GetDaqT1602Config()
	if got != want {
		t.Fatalf("offline config = %v, want %v", got, want)
	}
}

// TestDAQT1602ReadLoopExitOnConnError 验证采集中途连接断开：readLoop 异常退出
// 必须触发 OnReadLoopExit 并清理状态（Error + conn 毒化）。
func TestDAQT1602ReadLoopExitOnConnError(t *testing.T) {
	fake := &t1602FakeDevice{}
	device, server := newT1602TestDevice(fake)
	exited := make(chan error, 1)
	device.OnReadLoopExit(func(err error) { exited <- err })
	if err := device.Connect(); err != nil {
		t.Fatalf("Connect returned error: %v", err)
	}
	if err := device.StartAcquisition(); err != nil {
		t.Fatalf("StartAcquisition returned error: %v", err)
	}
	// 模拟设备掉线：关闭 server 端，readLoop 下一次轮询读 EOF。
	server.Close()

	select {
	case err := <-exited:
		if err == nil {
			t.Fatal("OnReadLoopExit error should be non-nil")
		}
	case <-time.After(testReadTimeout):
		t.Fatal("OnReadLoopExit was not called after conn error")
	}
	status := device.Status()
	if status.Connection != core.ConnectionError {
		t.Fatalf("connection status = %v, want %v", status.Connection, core.ConnectionError)
	}
	device.mu.RLock()
	conn := device.conn
	device.mu.RUnlock()
	if conn != nil {
		t.Fatal("conn should be invalidated after read loop failure")
	}
}

// t1602DeadlineIgnoringConn 忽略所有 SetDeadline（模拟 ADR-009 故障 Windows
// 电脑：deadline 失效，Read 永久阻塞），仅 Close 后 Read 才返回错误。
type t1602DeadlineIgnoringConn struct {
	net.Conn
	closeOnce sync.Once
	closed    chan struct{}
}

func newT1602DeadlineIgnoringConn(inner net.Conn) *t1602DeadlineIgnoringConn {
	return &t1602DeadlineIgnoringConn{Conn: inner, closed: make(chan struct{})}
}

func (c *t1602DeadlineIgnoringConn) SetDeadline(time.Time) error      { return nil }
func (c *t1602DeadlineIgnoringConn) SetReadDeadline(time.Time) error  { return nil }
func (c *t1602DeadlineIgnoringConn) SetWriteDeadline(time.Time) error { return nil }

func (c *t1602DeadlineIgnoringConn) Close() error {
	c.closeOnce.Do(func() { close(c.closed) })
	return c.Conn.Close()
}

// TestDAQT1602WatchdogClosesUnresponsiveConn 验证 ADR-009 硬性要求：deadline
// 失效的连接上，设备不响应时由 modbus.Conn 的 1s 响应超时 watchdog 直接
// Close 连接解除阻塞 readLoop，触发 OnReadLoopExit 并置 Error 状态。
func TestDAQT1602WatchdogClosesUnresponsiveConn(t *testing.T) {
	server, inner := net.Pipe()
	defer server.Close()
	ignored := newT1602DeadlineIgnoringConn(inner)
	// fake 设备：Connect 阶段正常应答类型查询，采集阶段永不响应数据轮询。
	var respondTypes atomic.Bool
	respondTypes.Store(true)
	go serveT1602FakeDevice(server, func(unitID uint8, pdu []byte) []byte {
		if !respondTypes.Load() && pdu[0] == 0x04 {
			return nil
		}
		return (&t1602FakeDevice{}).handle(unitID, pdu)
	})

	device := NewDAQT1602(core.Profile{ID: "t1602-watchdog", Type: core.DeviceDaqT1602})
	device.dialTCP = func(string, string, time.Duration) (net.Conn, error) { return ignored, nil }
	exited := make(chan error, 1)
	device.OnReadLoopExit(func(err error) { exited <- err })
	if err := device.Connect(); err != nil {
		t.Fatalf("Connect returned error: %v", err)
	}
	respondTypes.Store(false)
	if err := device.StartAcquisition(); err != nil {
		t.Fatalf("StartAcquisition returned error: %v", err)
	}

	started := time.Now()
	select {
	case err := <-exited:
		if !errors.Is(err, modbus.ErrConnBroken) {
			t.Fatalf("OnReadLoopExit error = %v, want modbus.ErrConnBroken", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("watchdog did not release blocked readLoop within 3s")
	}
	if elapsed := time.Since(started); elapsed > 2*time.Second {
		t.Fatalf("watchdog path took %v, want ~1s (modbus.ResponseTimeout)", elapsed)
	}
	select {
	case <-ignored.closed:
	case <-time.After(testReadTimeout):
		t.Fatal("conn should have been closed by watchdog")
	}
	if got := device.Status().Connection; got != core.ConnectionError {
		t.Fatalf("connection status = %v, want %v", got, core.ConnectionError)
	}
}

// TestDAQT1602StopAfterReadLoopFailure 验证 readLoop 被 watchdog 杀掉后的 Stop：
// 停止动作本身成功（返回 nil），但连接已毒化（Error + conn 关闭），不可复用。
func TestDAQT1602StopAfterReadLoopFailure(t *testing.T) {
	server, inner := net.Pipe()
	defer server.Close()
	ignored := newT1602DeadlineIgnoringConn(inner)
	// 吞掉所有请求但永不响应：modbus 1s 响应超时 watchdog 会杀掉 readLoop。
	go serveT1602FakeDevice(server, func(unitID uint8, pdu []byte) []byte { return nil })

	device := NewDAQT1602(core.Profile{ID: "t1602-stop-join", Type: core.DeviceDaqT1602})
	device.conn = ignored
	device.mb = modbus.NewConn(ignored)
	device.status.Connection = core.ConnectionConnected
	exited := make(chan error, 1)
	device.OnReadLoopExit(func(err error) { exited <- err })

	if err := device.StartAcquisition(); err != nil {
		t.Fatalf("StartAcquisition returned error: %v", err)
	}
	// 等 readLoop 被 modbus 响应超时 watchdog 杀掉（~1s），onReadLoopExit 在
	// invalidateConnection 之后触发，观察到回调即保证状态清理已完成。
	select {
	case err := <-exited:
		if !errors.Is(err, modbus.ErrConnBroken) {
			t.Fatalf("OnReadLoopExit error = %v, want modbus.ErrConnBroken", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("readLoop was not killed by response-timeout watchdog within 3s")
	}
	if err := device.StopAcquisition(); err != nil {
		t.Fatalf("StopAcquisition returned error: %v", err)
	}
	if got := device.Status().Connection; got != core.ConnectionError {
		t.Fatalf("connection status = %v, want %v", got, core.ConnectionError)
	}
	select {
	case <-ignored.closed:
	default:
		t.Fatal("conn should have been closed")
	}
}

// TestDAQT1602ConversionMatchesTypeCodeTable 用用户提供的量程表（2026-08-13，
// 已真机验证）逐类型验证换算端点：raw 0 → RangeMin，raw 65535 → RangeMax。
func TestDAQT1602ConversionMatchesTypeCodeTable(t *testing.T) {
	cases := []struct {
		code     uint8
		min, max float64
	}{
		{0, -50, 50},   // J
		{1, 0, 1200},   // K
		{2, 0, 1300},   // T
		{3, -200, 400}, // E
		{4, 0, 1000},   // R
		{5, 0, 1700},   // S
		{6, 0, 1768},   // B
		{7, 0, 1800},   // N
		{255, 0, 1800}, // 未知类型码按 N 型兜底
	}
	for _, tc := range cases {
		if got := t1602RawToTemp(0, tc.code); got != tc.min {
			t.Errorf("code %d raw 0 = %v, want %v", tc.code, got, tc.min)
		}
		if got := t1602RawToTemp(65535, tc.code); got != tc.max {
			t.Errorf("code %d raw 65535 = %v, want %v", tc.code, got, tc.max)
		}
	}
	// 中点线性：T 型 raw 32768 ≈ 32768/65535×1300 ≈ 650.06。
	if got := t1602RawToTemp(32768, 2); got < 650.0 || got > 650.1 {
		t.Errorf("T 型 raw 32768 = %v, want ≈650.06", got)
	}
}
