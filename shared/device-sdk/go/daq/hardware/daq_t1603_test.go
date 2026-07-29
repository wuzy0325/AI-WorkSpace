package hardware

import (
	"errors"
	"fmt"
	"net"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"shared.local/device-sdk/go/daq/core"
	"shared.local/device-sdk/go/protocol"
)

// testReadTimeout 是测试侧读取超时上限，与 drainConnection 的总耗时上限对齐
// （maxIters=10 × timeout=100ms = 1s）。低于该值会在 drainConnection 尚未
// 结束前误判超时；高于该值则拖长测试。三处 readWithTimeout 调用复用此常量。
const testReadTimeout = time.Second

func readWithTimeout(conn net.Conn, timeout time.Duration) (string, error) {
	type result struct {
		data string
		err  error
	}
	ch := make(chan result, 1)
	go func() {
		buf := make([]byte, 4096)
		n, err := conn.Read(buf)
		ch <- result{string(buf[:n]), err}
	}()
	select {
	case r := <-ch:
		return r.data, r.err
	case <-time.After(timeout):
		return "", fmt.Errorf("timeout")
	}
}

func TestDAQT1603ApplyConfigSendsHardwareCommands(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	commandsCh := make(chan []string, 1)
	go func() {
		commands := make([]string, 0)
		for {
			cmd, err := readWithTimeout(server, testReadTimeout)
			if err != nil {
				commandsCh <- commands
				return
			}
			commands = append(commands, cmd)
			_, _ = server.Write([]byte("A\n"))
		}
	}()

	device := NewDAQT1603(core.Profile{ID: "t1603-1", Type: core.DeviceDaqT1603})
	device.conn = client
	cfg := core.DaqT1603HardwareConfig{
		ThermocoupleTypes: "KKKKKKKKKKKKKKKK",
		SamplingRate:      20,
		BinaryFormat:      true,
		AverageCount:      4,
		TriggerMode:       2,
		TriggerEdge:       1,
		TriggerCount:      3,
		ShowTimestamp:     true,
		ShowSequence:      true,
	}

	if err := device.ApplyDaqT1603Config(cfg); err != nil {
		t.Fatalf("ApplyDaqT1603Config returned error: %v", err)
	}
	client.Close()

	commands := <-commandsCh
	want := []string{
		"@f3 0KKKKKKKKKKKKKKKK0",
		"@fe BIN 1",
		"@fe TIME 1",
		"@fe HEAD 1",
		"@fe SPS 20",
		"@fe AVG 4",
		"@fe TYPE 2",
		"@fe TRIG 1",
		"@fe TNUM 3",
	}
	if !reflect.DeepEqual(commands, want) {
		t.Fatalf("commands = %#v, want %#v", commands, want)
	}

	got, err := device.GetDaqT1603Config()
	if err != nil {
		t.Fatalf("GetDaqT1603Config returned error: %v", err)
	}
	if !reflect.DeepEqual(got, cfg) {
		t.Fatalf("config = %#v, want %#v", got, cfg)
	}
}

func TestDAQT1603StartAcquisitionNormalizesHardwareTrigger(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	commandsCh := make(chan []string, 1)
	go func() {
		commands := make([]string, 0)
		for {
			cmd, err := readWithTimeout(server, testReadTimeout)
			if err != nil {
				commandsCh <- commands
				return
			}
			commands = append(commands, cmd)
			_, _ = server.Write([]byte("A\n"))
		}
	}()

	device := NewDAQT1603(core.Profile{ID: "t1603-1", Type: core.DeviceDaqT1603})
	device.conn = client
	device.frameReader = protocol.NewT1603FrameReader(client)
	device.status.Connection = core.ConnectionConnected
	device.config = core.DaqT1603HardwareConfig{
		ChannelMask:   "FFFF",
		TriggerMode:   2,
		TriggerEdge:   1,
		TriggerCount:  3,
		BinaryFormat:  true,
		ShowTimestamp: true,
		ShowSequence:  true,
		SamplingRate:  10,
		AverageCount:  1,
	}

	if err := device.StartAcquisition(); err != nil {
		t.Fatalf("StartAcquisition returned error: %v", err)
	}

	if err := device.StopAcquisition(); err != nil {
		t.Fatalf("StopAcquisition returned error: %v", err)
	}
	time.Sleep(20 * time.Millisecond)
	client.Close()

	commands := <-commandsCh
	wantPrefix := []string{
		"@f1",
		"@f0 FFFF 2",
	}
	if len(commands) < len(wantPrefix) {
		t.Fatalf("commands = %#v, want prefix %#v", commands, wantPrefix)
	}
	for i, want := range wantPrefix {
		if commands[i] != want {
			t.Fatalf("commands[%d] = %q, want %q (all=%#v)", i, commands[i], want, commands)
		}
	}
}

func TestDAQT1603StopCommandCompletesBeforeReturn(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	commandsCh := make(chan string, 4)
	go func() {
		for {
			cmd, err := readWithTimeout(server, testReadTimeout)
			if err != nil {
				return
			}
			commandsCh <- cmd
			_, _ = server.Write([]byte("A\n"))
		}
	}()

	device := NewDAQT1603(core.Profile{ID: "t1603-1", Type: core.DeviceDaqT1603})
	device.conn = client
	device.frameReader = protocol.NewT1603FrameReader(client)
	device.status.Connection = core.ConnectionConnected
	device.config = core.DaqT1603HardwareConfig{
		ChannelMask:  "FFFF",
		BinaryFormat: true,
	}

	if err := device.StartAcquisition(); err != nil {
		t.Fatalf("StartAcquisition returned error: %v", err)
	}
	for _, want := range []string{"@f1", "@f0 FFFF 2"} {
		if got := <-commandsCh; got != want {
			t.Fatalf("command = %q, want %q", got, want)
		}
	}

	if err := device.StopAcquisition(); err != nil {
		t.Fatalf("StopAcquisition returned error: %v", err)
	}
	select {
	case got := <-commandsCh:
		if got != "@f1" {
			t.Fatalf("command = %q, want %q", got, "@f1")
		}
	default:
		t.Fatal("StopAcquisition returned before the stop command reached the connection")
	}
}

func TestDAQT1603StopAcquisitionWaitsForReadLoopExit(t *testing.T) {
	device := NewDAQT1603(core.Profile{ID: "t1603-1", Type: core.DeviceDaqT1603})
	device.acquiring = true
	device.stop = make(chan struct{})
	device.readLoopDone = make(chan struct{})
	done := device.readLoopDone
	device.status.Connection = core.ConnectionAcquiring
	device.status.Acquiring = true

	returned := make(chan error, 1)
	go func() {
		returned <- device.StopAcquisition()
	}()

	select {
	case err := <-returned:
		t.Fatalf("StopAcquisition returned before readLoop exited: %v", err)
	case <-time.After(50 * time.Millisecond):
	}

	close(done)

	select {
	case err := <-returned:
		if err != nil {
			t.Fatalf("StopAcquisition returned error: %v", err)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("StopAcquisition did not return after readLoop exited")
	}

	if device.status.Acquiring {
		t.Fatal("device still marked acquiring after stop")
	}
	if device.status.Connection != core.ConnectionConnected {
		t.Fatalf("connection status = %v, want %v", device.status.Connection, core.ConnectionConnected)
	}
	if device.readLoopDone != nil {
		t.Fatal("readLoopDone was not cleared after stop")
	}
}

func TestDAQT1603DrainConnectionWaitsForDelayedFrameTail(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	device := NewDAQT1603(core.Profile{ID: "t1603-1", Type: core.DeviceDaqT1603})
	written := make(chan struct{})
	go func() {
		time.Sleep(150 * time.Millisecond)
		_, _ = server.Write([]byte{1, 2, 3, 4})
		close(written)
	}()

	device.drainConnection(client, 100*time.Millisecond)

	select {
	case <-written:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("drainConnection returned before the delayed frame tail arrived")
	}
}

// =================================================================
// ADR-009 watchdog 兜底测试（P0-5）
// -----------------------------------------------------------------
// 设计依据 ADR-009：SetReadDeadline / SetWriteDeadline 在某些 Windows
// 电脑不可靠，Read/Write 在 deadline 到期后仍可能无限阻塞。T1603
// stopAcquisitionLocked / drainConnection / readLoop 必须有独立
// watchdog 计时器，超时强制 Close conn 解除阻塞，且 join 超时后
// 必须废弃连接（避免 readLoop 残留 goroutine 与新命令竞争 conn）。
// =================================================================

// t1603DeadlineIgnoringConn 忽略 SetReadDeadline 与 SetWriteDeadline，
// 模拟 ADR-009 描述的"deadline 在故障 Windows 电脑上失效"场景。
// 此时 Read/Write 不会在 deadline 到期后返回，必须依赖 watchdog Close
// 解除阻塞。仅 Close 后 Read/Write 才返回错误。
//
// 放在 hardware 包内（不复用 protocol 包的同名类型）：
//   - protocol 包的 deadlineIgnoringConn 是未导出类型，跨包无法直接使用
//   - T1603 的 drainConnection / @f1 Write 路径同时涉及 Read 和 Write
//     阻塞，需要同时忽略两个 deadline
type t1603DeadlineIgnoringConn struct {
	net.Conn
	closed chan struct{}
	once   sync.Once
}

func newT1603DeadlineIgnoringConn(inner net.Conn) *t1603DeadlineIgnoringConn {
	return &t1603DeadlineIgnoringConn{
		Conn:   inner,
		closed: make(chan struct{}),
	}
}

// SetReadDeadline 覆盖为 no-op，模拟 Windows 故障环境下 read deadline 失效。
func (c *t1603DeadlineIgnoringConn) SetReadDeadline(t time.Time) error {
	return nil
}

// SetWriteDeadline 覆盖为 no-op，模拟 Windows 故障环境下 write deadline 失效。
// 同时忽略两个 deadline 才能覆盖 @f1 Write 阻塞场景。
func (c *t1603DeadlineIgnoringConn) SetWriteDeadline(t time.Time) error {
	return nil
}

// Close 关闭连接并通知 closed channel，让阻塞中的 Read/Write 立即返回。
func (c *t1603DeadlineIgnoringConn) Close() error {
	c.once.Do(func() { close(c.closed) })
	return c.Conn.Close()
}

// TestDAQT1603StopAcquisition_ClosesConnOnReadLoopJoinTimeout 验证
// stopAcquisitionLocked 在 readLoop join 超时后强制废弃连接。
//
// ADR-009 决策 2：连接生命周期所有者超时或取消时必须能调用 conn.Close()，
// 不能仅打 warn 日志。当前实现（修复前）只打 warn 不 Close，readLoop 残留
// goroutine 会与后续 ApplyDaqT1603Config 竞争 conn。
//
// 测试前置：
//   - net.Pipe 建立双向连接
//   - device.readLoopDone 设置为未关闭的 chan（模拟 readLoop 卡在 conn.Read）
//   - device.acquiring = true, device.stop 已设置
//
// 测试步骤：
//   - 调用 device.StopAcquisition()
//
// 期待结果：
//   - 返回错误，包含 "reconnect required"
//   - device.conn 被置为 nil
//   - device.status.Connection = ConnectionError
//   - conn 已被 Close（server.Write 失败）
func TestDAQT1603StopAcquisition_ClosesConnOnReadLoopJoinTimeout(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	// 注意：client 由 device.StopAcquisition -> invalidateConnection 关闭，
	// 这里不 defer client.Close() 以避免重复 Close 报错。

	device := NewDAQT1603(core.Profile{ID: "t1603-stop-stuck", Type: core.DeviceDaqT1603})
	device.mu.Lock()
	device.conn = client
	device.frameReader = protocol.NewT1603FrameReader(client)
	device.acquiring = true
	device.stop = make(chan struct{})
	// 伪造一个永远不会被关闭的 readLoopDone，模拟 readLoop 卡在 conn.Read 中
	// 无法退出（deadlineIgnoringConn 场景下 frameReader.ReadFrame 阻塞）。
	device.readLoopDone = make(chan struct{})
	device.status.Connection = core.ConnectionAcquiring
	device.status.Acquiring = true
	device.mu.Unlock()

	// 总预算 6s：3s join timeout + 1s 余量 + 2s 缓冲（drainConnection 等附加路径）
	done := make(chan error, 1)
	go func() {
		done <- device.StopAcquisition()
	}()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected 'reconnect required' error, got nil")
		}
		if !strings.Contains(err.Error(), "reconnect required") {
			t.Fatalf("error should mention 'reconnect required', got: %v", err)
		}
	case <-time.After(6 * time.Second):
		t.Fatal("StopAcquisition did not return within 6s budget; join timeout likely did not close conn")
	}

	// 验证 conn 已被废弃：device.conn == nil
	device.mu.RLock()
	connAfter := device.conn
	statusAfter := device.status.Connection
	device.mu.RUnlock()
	if connAfter != nil {
		t.Fatal("device.conn should be nil after readLoop join timeout")
	}
	if statusAfter != core.ConnectionError {
		t.Fatalf("status.Connection = %v, want Error", statusAfter)
	}

	// 验证 conn 已被 Close：server.Write 应失败
	_ = server.SetWriteDeadline(time.Now().Add(200 * time.Millisecond))
	if _, writeErr := server.Write([]byte("probe")); writeErr == nil {
		t.Error("expected server.Write to fail after client was closed by watchdog")
	}
}

// TestDAQT1603DrainConnection_WatchdogTriggersOnDeadlineIgnoringConn 验证
// drainConnection 在 deadline 失效场景下由 watchdog 兜底。
//
// ADR-009 决策 1：SetReadDeadline 只作为软超时，不作为有界网络操作的
// 唯一取消机制。当前实现（修复前）drainConnection 仅靠 SetReadDeadline，
// deadline 失效时 conn.Read 无限阻塞，drainConnection 永远不返回。
//
// 测试前置：
//   - net.Pipe 建立双向连接
//   - 包装 client 为 t1603DeadlineIgnoringConn（SetReadDeadline 被 no-op）
//   - server 不写任何数据（确保 client.Read 阻塞）
//
// 测试步骤：
//   - 调用 device.drainConnection(ignored, 100ms)
//
// 期待结果：
//   - drainConnection 在合理预算内返回（不到 5s）
//   - conn 被 Close（server.Write 失败）
func TestDAQT1603DrainConnection_WatchdogTriggersOnDeadlineIgnoringConn(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	// client 由 drainConnection 内的 watchdog Close

	ignored := newT1603DeadlineIgnoringConn(client)
	device := NewDAQT1603(core.Profile{ID: "t1603-drain-stuck", Type: core.DeviceDaqT1603})

	// 总预算 5s：drainConnection 应在 watchdog 超时内返回
	done := make(chan struct{})
	var watchdogTriggered bool
	go func() {
		watchdogTriggered = device.drainConnection(ignored, 100*time.Millisecond)
		close(done)
	}()

	select {
	case <-done:
		// drainConnection 返回，符合预期
	case <-time.After(5 * time.Second):
		t.Fatal("drainConnection did not return within 5s budget; watchdog likely not armed")
	}

	// P2-5 修复后：watchdog 触发时 drainConnection 必须返回 false，
	// 调用方据此清理 d.conn 状态，避免后续命令写到已关闭的 conn 上。
	if watchdogTriggered {
		t.Error("expected drainConnection to return false when watchdog triggered")
	}

	// 验证 conn 已被 watchdog Close：server.Write 应失败
	_ = server.SetWriteDeadline(time.Now().Add(200 * time.Millisecond))
	if _, writeErr := server.Write([]byte("probe")); writeErr == nil {
		t.Error("expected server.Write to fail after client was closed by watchdog")
	}
}

// TestDAQT1603SyncHardwareConfig_FailsFastOnWatchdogTriggered 验证
// syncHardwareConfigLocked 中 readAllConfig 在第 1 条命令 watchdog 触发后立即失败，
// 不再吞掉错误继续走完剩余 9 条命令 + 3 条 @fe 强制命令。
//
// 修复前 bug：readAllConfig 的 query/readExact 函数对任何错误都 return nil（吞错误），
// 即使 watchdog 已触发 conn Close，后续命令仍会循环发送（虽然每条立即失败 ~0ms）。
// 副作用：syncHardwareConfigLocked 最终返回的错误是 @fe BIN 的
// "force BIN mode: use of closed network connection"，掩盖了真正的 watchdog 根因，
// 操作员误判为"配置命令失败"而非"连接被 watchdog 强制关闭"。
//
// 测试前置：
//   - net.Pipe 建立双向连接
//   - 包装 client 为 t1603DeadlineIgnoringConn（SetReadDeadline/SetWriteDeadline 被 no-op）
//   - device.conn = ignored, device.frameReader 已初始化
//   - server 不读不写（让 client.Read 阻塞，触发 watchdog）
//
// 测试步骤：
//   - 调用 device.syncHardwareConfigLocked(ignored)
//
// 期待结果：
//   - syncHardwareConfigLocked 在 watchdog 超时内返回（不到 5s）
//   - 返回错误包含 ErrWatchdogTriggered（暴露真正的 watchdog 根因）
//   - conn 被 Close（server.Write 失败）
func TestDAQT1603SyncHardwareConfig_FailsFastOnWatchdogTriggered(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	// client 由 SendCommandExact 内的 watchdog Close

	ignored := newT1603DeadlineIgnoringConn(client)
	device := NewDAQT1603(core.Profile{ID: "t1603-sync-stuck", Type: core.DeviceDaqT1603})
	device.mu.Lock()
	device.conn = ignored
	device.frameReader = protocol.NewT1603FrameReader(ignored)
	device.mu.Unlock()

	// 总预算 5s：第 1 条 @e3 watchdog 应在 2s 内触发，syncHardwareConfigLocked 立即返回
	done := make(chan error, 1)
	go func() {
		done <- device.syncHardwareConfigLocked(ignored)
	}()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected syncHardwareConfigLocked to fail when watchdog triggered, got nil")
		}
		// 修复后：readAllConfig 检测到 ErrWatchdogTriggered 立即返回，
		// 错误链包含 protocol.ErrWatchdogTriggered sentinel，便于上层识别。
		if !errors.Is(err, protocol.ErrWatchdogTriggered) {
			t.Fatalf("expected error to wrap ErrWatchdogTriggered, got: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("syncHardwareConfigLocked did not return within 5s budget; watchdog likely not armed or not propagated")
	}

	// 验证 conn 已被 watchdog Close：server.Write 应失败
	_ = server.SetWriteDeadline(time.Now().Add(200 * time.Millisecond))
	if _, writeErr := server.Write([]byte("probe")); writeErr == nil {
		t.Error("expected server.Write to fail after client was closed by watchdog")
	}
}

// TestDAQT1603StopAcquisition_StopCommandWriteHasWatchdog 验证
// stopAcquisitionLocked 中 @f1 Write 路径有 watchdog 兜底。
//
// ADR-009 决策 1：SetWriteDeadline 同样不可靠，Write 阻塞时必须有
// 独立 Close 兜底。当前实现（修复前）@f1 Write 仅有 SetWriteDeadline，
// deadline 失效时 Write 永久阻塞，stopAcquisitionLocked 永远不返回。
//
// 测试前置：
//   - net.Pipe 建立双向连接
//   - 包装 client 为 t1603DeadlineIgnoringConn（SetWriteDeadline 被 no-op）
//   - device.readLoopDone 已关闭（跳过 join 阶段，直接进入 @f1 Write 路径）
//   - device.acquiring = true，触发 @f1 Write 分支
//   - server 不读不写（让 client Write 阻塞，net.Pipe 无缓冲）
//
// 测试步骤：
//   - 调用 device.StopAcquisition()
//
// 期待结果：
//   - StopAcquisition 在 watchdog 超时内返回（不到 5s）
//   - conn 被 Close（server.Write 失败）
func TestDAQT1603StopAcquisition_StopCommandWriteHasWatchdog(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()

	ignored := newT1603DeadlineIgnoringConn(client)
	device := NewDAQT1603(core.Profile{ID: "t1603-write-stuck", Type: core.DeviceDaqT1603})
	device.mu.Lock()
	device.conn = ignored
	device.frameReader = protocol.NewT1603FrameReader(ignored)
	device.acquiring = true
	device.stop = make(chan struct{})
	// readLoopDone 已关闭，跳过 join 阶段直接进入 @f1 Write 路径
	closedDone := make(chan struct{})
	close(closedDone)
	device.readLoopDone = closedDone
	device.status.Connection = core.ConnectionAcquiring
	device.status.Acquiring = true
	device.mu.Unlock()

	// 总预算 5s：watchdog 应在 1s 内触发 Close 解除 Write 阻塞
	done := make(chan struct{})
	go func() {
		_ = device.StopAcquisition()
		close(done)
	}()

	select {
	case <-done:
		// StopAcquisition 返回，符合预期
	case <-time.After(5 * time.Second):
		t.Fatal("StopAcquisition did not return within 5s budget; @f1 Write watchdog likely not armed")
	}

	// 验证 conn 已被 watchdog Close：server.Write 应失败
	_ = server.SetWriteDeadline(time.Now().Add(200 * time.Millisecond))
	if _, writeErr := server.Write([]byte("probe")); writeErr == nil {
		t.Error("expected server.Write to fail after client was closed by watchdog")
	}
}
