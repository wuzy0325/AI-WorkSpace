package hardware

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"net"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"shared.local/device-sdk/go/daq/core"
	"shared.local/device-sdk/go/protocol"
)

// testReadTimeout bounds test-side peer reads without controlling production I/O.
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
			_, _ = server.Write([]byte("A"))
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

func TestDAQT1603ReadAllConfigMatchesDelimiterFreeHardwareResponses(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	responses := []struct {
		command  string
		response string
	}{
		{"@e3", "KTTTTTTTTTTTTTTT\n"},
		{"@fd MCH", "FFFF"},
		{"@fd BIN", "1"},
		{"@fd TIME", "0"},
		{"@fd HEAD", "0"},
		{"@fd TYPE", "0"},
		{"@fd TRIG", "0"},
	}

	serverErr := make(chan error, 1)
	go func() {
		for _, item := range responses {
			command, err := readWithTimeout(server, testReadTimeout)
			if err != nil || command != item.command {
				serverErr <- fmt.Errorf("command = %q, want %q, err = %v", command, item.command, err)
				return
			}
			if _, err := server.Write([]byte(item.response)); err != nil {
				serverErr <- err
				return
			}
		}
		serverErr <- nil
	}()

	device := NewDAQT1603(core.Profile{
		ID:   "t1603-read-config",
		Type: core.DeviceDaqT1603,
		DaqT1603Config: core.DaqT1603HardwareConfig{
			SamplingRate: 2,
			AverageCount: 4,
			TriggerCount: 1,
		},
	})
	cfg, err := device.readAllConfig(client, time.Now().Add(configSyncTotalTimeout))
	if err != nil {
		t.Fatalf("readAllConfig returned error: %v", err)
	}
	if err := <-serverErr; err != nil {
		t.Fatal(err)
	}
	if cfg.ThermocoupleTypes != "KTTTTTTTTTTTTTTT" || cfg.ChannelMask != "FFFF" {
		t.Fatalf("unexpected fixed config: %#v", cfg)
	}
	if cfg.SamplingRate != 2 || cfg.AverageCount != 4 || cfg.TriggerCount != 1 {
		t.Fatalf("variable-length saved config was not preserved: %#v", cfg)
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
			if strings.HasPrefix(cmd, "@f0") || cmd == "@f1" {
				go func() { _, _ = server.Write([]byte("A")) }()
			}
		}
	}()

	device := NewDAQT1603(core.Profile{ID: "t1603-1", Type: core.DeviceDaqT1603})
	device.conn = client
	device.frameReader = protocol.NewT1603FrameReader(client)
	device.frameReader.SetBinaryMode(true)
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
	defer device.Disconnect()

	if err := device.StartAcquisition(); err != nil {
		t.Fatalf("StartAcquisition returned error: %v", err)
	}

	if err := device.StopAcquisition(); err != nil {
		t.Fatalf("StopAcquisition returned error: %v", err)
	}
	time.Sleep(20 * time.Millisecond)
	client.Close()

	commands := <-commandsCh
	wantPrefix := []string{"@f0 FFFF 2"}
	if len(commands) < len(wantPrefix) {
		t.Fatalf("commands = %#v, want prefix %#v", commands, wantPrefix)
	}
	for i, want := range wantPrefix {
		if commands[i] != want {
			t.Fatalf("commands[%d] = %q, want %q (all=%#v)", i, commands[i], want, commands)
		}
	}
}

func TestDAQT1603StopWaitsForTailFrameAndACKBeforeReturning(t *testing.T) {
	setT1603StopTimeout(t, 2*time.Second)
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	frame := make([]byte, protocol.TCPFrameSize)
	binary.LittleEndian.PutUint32(frame[14*4:], math.Float32bits(27.5))
	serverErr := make(chan error, 1)
	stopCommandSeen := make(chan struct{})
	releaseACK := make(chan struct{})
	go func() {
		cmd, err := readWithTimeout(server, testReadTimeout)
		if err != nil || cmd != "@f0 FFFF 2" {
			serverErr <- fmt.Errorf("start command = %q, err = %v", cmd, err)
			return
		}
		if _, err := server.Write(append([]byte{'A'}, frame...)); err != nil {
			serverErr <- err
			return
		}
		cmd, err = readWithTimeout(server, testReadTimeout)
		if err != nil || cmd != "@f1" {
			serverErr <- fmt.Errorf("stop command = %q, err = %v", cmd, err)
			return
		}
		close(stopCommandSeen)
		if _, err := server.Write(frame); err != nil {
			serverErr <- err
			return
		}
		<-releaseACK
		_, err = server.Write([]byte{'A'})
		serverErr <- err
	}()

	device := NewDAQT1603(core.Profile{ID: "t1603-1", Type: core.DeviceDaqT1603})
	device.conn = client
	device.frameReader = protocol.NewT1603FrameReader(client)
	device.frameReader.SetBinaryMode(true)
	device.status.Connection = core.ConnectionConnected
	device.config = core.DaqT1603HardwareConfig{
		ChannelMask:  "FFFF",
		BinaryFormat: true,
	}
	payloads := make(chan core.DataPayload, 4)
	device.SetDataSink(func(payload core.DataPayload) { payloads <- payload })
	defer device.Disconnect()

	if err := device.StartAcquisition(); err != nil {
		t.Fatalf("StartAcquisition returned error: %v", err)
	}
	assertT1603CH02(t, payloads, 27.5)

	stopResult := make(chan error, 1)
	go func() { stopResult <- device.StopAcquisition() }()
	select {
	case <-stopCommandSeen:
	case <-time.After(testReadTimeout):
		t.Fatal("device did not send @f1")
	}
	// ACK 未发前，Stop 必须等待（readLoop 在消费尾帧或等 ACK）。
	select {
	case err := <-stopResult:
		t.Fatalf("StopAcquisition returned before Stop ACK: %v", err)
	case <-time.After(10 * time.Millisecond):
	}
	close(releaseACK)
	// ACK 后等待 150 ms 静默窗口，再统一验证 N×frameSize+ACK 的 Stop 响应边界。
	select {
	case err := <-stopResult:
		if err != nil {
			t.Fatalf("StopAcquisition returned error: %v", err)
		}
	case <-time.After(testReadTimeout):
		t.Fatal("StopAcquisition did not return after Stop ACK")
	}
	if err := <-serverErr; err != nil {
		t.Fatal(err)
	}
	select {
	case payload := <-payloads:
		t.Fatalf("StopAcquisition forwarded tail frame: %v", payload.Channels)
	default:
	}
	device.mu.RLock()
	streaming := device.streaming
	status := device.status.Connection
	device.mu.RUnlock()
	if streaming {
		t.Fatal("physical stream still marked active after Stop ACK")
	}
	if status != core.ConnectionConnected {
		t.Fatalf("connection status = %v, want %v", status, core.ConnectionConnected)
	}
}

// TestDAQT1603StopConfirmsQuietBoundaryAndRestartSucceeds 验证协议契约
// （第11章）：Stop 响应 = N 个完整合法帧 + 单字节 'A' ACK，ACK 是事务终止边界。
// 收到 ACK 并完成 150 ms 静默边界确认后 Stop 返回；同连接下次 Start 正常工作。
//
// 替代旧测试 TestDAQT1603StopDrainsBytesArrivingAfterACKBeforeRestart：
// 旧测试假设"ACK 后有迟到字节"，该假设已被第5章修正澄清为缺乏证据
// （TCP 同一连接保证字节有序，ACK 前的字节不会重排到 ACK 后）。
func TestDAQT1603StopConfirmsQuietBoundaryAndRestartSucceeds(t *testing.T) {
	setT1603StopTimeout(t, 500*time.Millisecond)
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	frame := make([]byte, protocol.TCPFrameSizeWithTimestamp)
	binary.LittleEndian.PutUint32(frame[0:4], uint32(time.Now().Unix()))
	binary.LittleEndian.PutUint32(frame[4:8], 123456789)
	for i := 0; i < 16; i++ {
		binary.LittleEndian.PutUint32(frame[8+i*4:], math.Float32bits(float32(i+20)))
	}

	stopACKSeen := make(chan struct{})
	stopReturned := make(chan struct{})
	secondStartSeen := make(chan struct{})
	serverErr := make(chan error, 1)
	go func() {
		cmd, err := readWithTimeout(server, testReadTimeout)
		if err != nil || cmd != "@f0 FFFF 2" {
			serverErr <- fmt.Errorf("first start command = %q, err = %v", cmd, err)
			return
		}
		if _, err := server.Write(append([]byte{'A'}, frame...)); err != nil {
			serverErr <- err
			return
		}
		cmd, err = readWithTimeout(server, testReadTimeout)
		if err != nil || cmd != "@f1" {
			serverErr <- fmt.Errorf("stop command = %q, err = %v", cmd, err)
			return
		}
		if _, err := server.Write([]byte{'A'}); err != nil {
			serverErr <- err
			return
		}
		close(stopACKSeen)
		// 等待 Stop 返回后，再发第二次 Start。
		// 等待 Stop 完成 150 ms 静默边界确认后，再发第二次 Start。
		<-stopReturned
		cmd, err = readWithTimeout(server, testReadTimeout)
		if err != nil || cmd != "@f0 FFFF 2" {
			serverErr <- fmt.Errorf("second start command = %q, err = %v", cmd, err)
			return
		}
		close(secondStartSeen)
		_, err = server.Write(append([]byte{'A'}, frame...))
		serverErr <- err
	}()

	device := NewDAQT1603(core.Profile{ID: "t1603-post-ack-immediate", Type: core.DeviceDaqT1603})
	device.conn = client
	device.frameReader = protocol.NewT1603FrameReader(client)
	device.frameReader.SetBinaryMode(true)
	device.frameReader.SetMetadataMode(true)
	device.status.Connection = core.ConnectionConnected
	device.config = core.DaqT1603HardwareConfig{
		ChannelMask:   "FFFF",
		BinaryFormat:  true,
		ShowTimestamp: true,
	}
	payloads := make(chan core.DataPayload, 2)
	device.SetDataSink(func(payload core.DataPayload) { payloads <- payload })
	defer device.Disconnect()

	if err := device.StartAcquisition(); err != nil {
		t.Fatalf("initial StartAcquisition returned error: %v", err)
	}
	select {
	case <-payloads:
	case <-time.After(testReadTimeout):
		t.Fatal("did not receive initial frame")
	}

	stopResult := make(chan error, 1)
	go func() { stopResult <- device.StopAcquisition() }()
	select {
	case <-stopACKSeen:
	case <-time.After(testReadTimeout):
		t.Fatal("server did not send Stop ACK")
	}
	// 协议契约：ACK 后完成 150 ms 静默边界确认，再返回 Stop 成功。
	select {
	case err := <-stopResult:
		if err != nil {
			t.Fatalf("StopAcquisition returned error after ACK: %v", err)
		}
		close(stopReturned)
	case <-time.After(250 * time.Millisecond):
		t.Fatal("StopAcquisition did not return after the 150 ms quiet-window confirmation")
	}

	if err := device.StartAcquisition(); err != nil {
		t.Fatalf("StartAcquisition after immediate stop returned error: %v", err)
	}
	select {
	case <-secondStartSeen:
	case <-time.After(testReadTimeout):
		t.Fatal("second start command was not sent")
	}
	select {
	case <-payloads:
	case <-time.After(testReadTimeout):
		t.Fatal("did not receive aligned frame after restart")
	}
	if err := <-serverErr; err != nil {
		t.Fatal(err)
	}
}

// setT1603StopTimeout 注入短 Stop 超时，加速 Stop 失败路径测试。
// 正常成功路径只需短静默确认，不依赖总超时；仅 ACK 缺失或边界错乱时触发。
func setT1603StopTimeout(t *testing.T, timeout time.Duration) {
	t.Helper()
	oldTimeout := stopAcquisitionTimeout
	stopAcquisitionTimeout = timeout
	t.Cleanup(func() {
		stopAcquisitionTimeout = oldTimeout
	})
}

func assertT1603CH02(t *testing.T, payloads <-chan core.DataPayload, want float64) {
	t.Helper()
	select {
	case payload := <-payloads:
		if payload.Channels[1] != want {
			t.Fatalf("CH02 = %v, want %v (channels=%v)", payload.Channels[1], want, payload.Channels)
		}
	case <-time.After(testReadTimeout):
		t.Fatal("did not receive initial frame")
	}
}

func TestDAQT1603StopWithoutACKInvalidatesConnection(t *testing.T) {
	client, server := net.Pipe()
	defer server.Close()

	frame := make([]byte, protocol.TCPFrameSize)
	go func() {
		cmd, _ := readWithTimeout(server, testReadTimeout)
		if strings.HasPrefix(cmd, "@f0") {
			_, _ = server.Write(append([]byte{'A'}, frame...))
		}
		_, _ = readWithTimeout(server, testReadTimeout)
	}()

	device := NewDAQT1603(core.Profile{ID: "t1603-stop-no-ack", Type: core.DeviceDaqT1603})
	device.conn = client
	device.frameReader = protocol.NewT1603FrameReader(client)
	device.frameReader.SetBinaryMode(true)
	device.config = core.DaqT1603HardwareConfig{ChannelMask: "FFFF", BinaryFormat: true}
	device.status.Connection = core.ConnectionConnected
	if err := device.StartAcquisition(); err != nil {
		t.Fatalf("StartAcquisition returned error: %v", err)
	}
	// net.Pipe 是同步无缓冲管道：server goroutine 顺序执行（读 @f0 → 写
	// ACK+frame → 读 @f1），若 readLoop 尚未消费 ACK+frame，server 会阻塞在
	// Write 上导致 @f1 写入无接收方而卡满 watchdog。等待 readLoop 进入
	// 稳定循环后再 Stop，避免测试时序依赖 goroutine 调度。
	time.Sleep(100 * time.Millisecond)
	// 优雅停止契约：静默兜底返回 nil（设备已停止，连接由后台 FIN+Close 释放，
	// 下次 Start 自动重连），但连接必须已毒化不可复用。
	err := device.StopAcquisition()
	if err != nil {
		t.Fatalf("StopAcquisition error = %v, want nil (graceful fallback)", err)
	}
	device.mu.RLock()
	conn := device.conn
	status := device.status.Connection
	device.mu.RUnlock()
	if conn != nil {
		t.Fatal("connection remained reusable after missing Stop ACK")
	}
	if status != core.ConnectionError {
		t.Fatalf("connection status = %v, want %v", status, core.ConnectionError)
	}
}

// TestDAQT1603StopWithACKOnDeadlineFailingConn 验证：连接 Read deadline 失效
// （Windows 故障机器 LSP hook 特征：SetReadDeadline 为 no-op）时，设备正常
// 回 Stop ACK 仍能走正常完成路径（非 350ms 兜底）快速结束 Stop。
//
// 背景（2026-07-31 实机复现 + 裸 TCP 探针验证）：@f1 后设备回 1 字节 'A'
// （N=0 尾帧），readLoop 读到 ACK 后进入 collectingStop 静默窗口——旧实现用
// SetReadDeadline(150ms) 唤醒"等 N 帧边界"的阻塞 Read；故障机器 deadline
// 取消失效，该 Read 永久阻塞，done 永不关闭，Stop 掉到 350ms 兜底废弃连接。
// 修复后（goroutine 静默窗口，不依赖 deadline）：Stop 约 180ms（探测缓存后
// 仅 150ms 窗口）正常完成。
//
// 注意：deadline 失效机器的 goroutine 窗口到期（N=0 响应无更多数据）会遗留
// 阻塞读 goroutine（frameReader.IsDirty()），该 goroutine 会抢走连接上后续
// 事务的数据，连接不可安全复用——Stop 正常完成后由上层废弃连接，下次 Start
// 自动重连（与 350ms 兜底的连接处置一致，但完成更快且无兜底告警日志）。
//
// 测试步骤：
//   - net.Pipe 建立双向连接，client 端包 t1603DeadlineIgnoringConn
//   - 模拟已探测：device.deadlineBroken=true + frameReader.SetDeadlineBroken(true)
//   - server 回 Start 响应（'A'+帧）、Stop 响应（'A'）
//   - StartAcquisition → StopAcquisition
//
// 期待结果：
//   - StopAcquisition 在 350ms 兜底前返回 nil（正常完成路径）
//   - 连接已废弃：conn=nil、status=Error（遗留读 goroutine 不可复用）
//   - onReadLoopExit 未被调用（Stop owner 主动废弃，无异常通知）
//   - server 按序收到完整命令序列
func TestDAQT1603StopWithACKOnDeadlineFailingConn(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()

	ignored := newT1603DeadlineIgnoringConn(client)
	frame := make([]byte, protocol.TCPFrameSize)
	stopSeen := make(chan struct{})

	serverErr := make(chan error, 1)
	go func() {
		cmd, err := readWithTimeout(server, testReadTimeout)
		if err != nil || cmd != "@f0 FFFF 2" {
			serverErr <- fmt.Errorf("start command = %q, err = %v", cmd, err)
			return
		}
		if _, err := server.Write(append([]byte{'A'}, frame...)); err != nil {
			serverErr <- err
			return
		}
		cmd, err = readWithTimeout(server, testReadTimeout)
		if err != nil || cmd != "@f1" {
			serverErr <- fmt.Errorf("stop command = %q, err = %v", cmd, err)
			return
		}
		close(stopSeen)
		if _, err := server.Write([]byte{'A'}); err != nil {
			serverErr <- err
			return
		}
		serverErr <- nil
	}()

	device := NewDAQT1603(core.Profile{ID: "t1603-stop-ack-no-deadline", Type: core.DeviceDaqT1603})
	device.conn = ignored
	device.frameReader = protocol.NewT1603FrameReader(ignored)
	device.frameReader.SetBinaryMode(true)
	// 模拟连接建立后已探测：本机器 deadline 失效。
	device.deadlineProbed = true
	device.deadlineBroken = true
	device.frameReader.SetDeadlineBroken(true)
	device.config = core.DaqT1603HardwareConfig{ChannelMask: "FFFF", BinaryFormat: true}
	device.status.Connection = core.ConnectionConnected
	var onReadLoopExitCalled int32
	device.onReadLoopExit = func(err error) {
		atomic.StoreInt32(&onReadLoopExitCalled, 1)
	}

	if err := device.StartAcquisition(); err != nil {
		t.Fatalf("StartAcquisition returned error: %v", err)
	}
	// net.Pipe 是同步无缓冲管道：等待 readLoop 消费 Start 响应进入稳定循环。
	time.Sleep(100 * time.Millisecond)

	stopResult := make(chan error, 1)
	started := time.Now()
	go func() { stopResult <- device.StopAcquisition() }()
	select {
	case <-stopSeen:
	case <-time.After(testReadTimeout):
		t.Fatal("device did not send @f1")
	}
	select {
	case err := <-stopResult:
		if err != nil {
			t.Fatalf("StopAcquisition error = %v, want nil (normal path)", err)
		}
	case <-time.After(300 * time.Millisecond):
		t.Fatal("StopAcquisition did not return within 300ms; fell into 350ms quiet fallback")
	}
	if elapsed := time.Since(started); elapsed >= 300*time.Millisecond {
		t.Fatalf("StopAcquisition took %v, want < 300ms (normal completion, not fallback)", elapsed)
	}

	device.mu.RLock()
	conn := device.conn
	status := device.status.Connection
	device.mu.RUnlock()
	if conn != nil {
		t.Fatal("connection remained reusable after goroutine quiet window left a leftover read")
	}
	if status != core.ConnectionError {
		t.Fatalf("connection status = %v, want %v (recycled)", status, core.ConnectionError)
	}
	if atomic.LoadInt32(&onReadLoopExitCalled) != 0 {
		t.Error("onReadLoopExit should not be called when Stop completes via normal ACK path")
	}
	if err := <-serverErr; err != nil {
		t.Fatal(err)
	}
}


func TestDAQT1603StopAllowsConfigOnSameConnection(t *testing.T) {
	setT1603StopTimeout(t, 200*time.Millisecond)
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	frame := make([]byte, protocol.TCPFrameSize)
	serverErr := make(chan error, 1)
	go func() {
		cmd, err := readWithTimeout(server, testReadTimeout)
		if err != nil || cmd != "@f0 FFFF 2" {
			serverErr <- fmt.Errorf("start command = %q, err = %v", cmd, err)
			return
		}
		if _, err := server.Write(append([]byte{'A'}, frame...)); err != nil {
			serverErr <- err
			return
		}
		cmd, err = readWithTimeout(server, testReadTimeout)
		if err != nil || cmd != "@f1" {
			serverErr <- fmt.Errorf("stop command = %q, err = %v", cmd, err)
			return
		}
		if _, err := server.Write([]byte{'A'}); err != nil {
			serverErr <- err
			return
		}
		for _, want := range []string{"@fe BIN 1", "@fe TIME 0", "@fe HEAD 0", "@fe TYPE 0", "@fe TRIG 0"} {
			cmd, err = readWithTimeout(server, testReadTimeout)
			if err != nil || cmd != want {
				serverErr <- fmt.Errorf("config command = %q, want %q, err = %v", cmd, want, err)
				return
			}
			if _, err := server.Write([]byte{'A'}); err != nil {
				serverErr <- err
				return
			}
		}
		serverErr <- nil
	}()

	device := NewDAQT1603(core.Profile{ID: "t1603-stop-config", Type: core.DeviceDaqT1603})
	device.conn = client
	device.frameReader = protocol.NewT1603FrameReader(client)
	device.frameReader.SetBinaryMode(true)
	device.status.Connection = core.ConnectionConnected
	device.config = core.DaqT1603HardwareConfig{ChannelMask: "FFFF", BinaryFormat: true}

	if err := device.StartAcquisition(); err != nil {
		t.Fatalf("StartAcquisition returned error: %v", err)
	}
	time.Sleep(20 * time.Millisecond)
	if err := device.StopAcquisition(); err != nil {
		t.Fatalf("StopAcquisition returned error: %v", err)
	}
	if err := device.ApplyDaqT1603Config(core.DaqT1603HardwareConfig{BinaryFormat: true}); err != nil {
		t.Fatalf("ApplyDaqT1603Config after physical stop returned error: %v", err)
	}
	if err := <-serverErr; err != nil {
		t.Fatal(err)
	}
}

func TestDAQT1603StopClosesDeadlineIgnoringReadWhenACKMissing(t *testing.T) {
	oldTimeout := stopAcquisitionTimeout
	stopAcquisitionTimeout = 100 * time.Millisecond
	t.Cleanup(func() { stopAcquisitionTimeout = oldTimeout })

	server, client := net.Pipe()
	defer server.Close()
	ignored := newT1603DeadlineIgnoringConn(client)

	device := NewDAQT1603(core.Profile{ID: "t1603-stop-deadline-ignored", Type: core.DeviceDaqT1603})
	device.conn = ignored
	device.frameReader = protocol.NewT1603FrameReader(ignored)
	device.frameReader.SetBinaryMode(true)
	device.acquiring = true
	device.streaming = true
	device.stop = make(chan struct{})
	device.readLoopDone = make(chan struct{})
	device.status.Connection = core.ConnectionAcquiring
	device.status.Acquiring = true
	go device.readLoop(ignored, device.frameReader, device.readLoopDone)

	go func() {
		cmd, _ := readWithTimeout(server, testReadTimeout)
		if cmd == "@f1" {
			// No ACK: the readLoop remains blocked until the Stop owner closes conn.
		}
	}()

	started := time.Now()
	err := device.StopAcquisition()
	if err == nil || !strings.Contains(err.Error(), "reconnect required") {
		t.Fatalf("StopAcquisition error = %v, want reconnect required", err)
	}
	if elapsed := time.Since(started); elapsed >= time.Second {
		t.Fatalf("StopAcquisition took %v despite independent Close fallback", elapsed)
	}
	device.mu.RLock()
	conn := device.conn
	status := device.status.Connection
	device.mu.RUnlock()
	if conn != nil || status != core.ConnectionError {
		t.Fatalf("conn=%v status=%v, want nil/Error", conn, status)
	}
}

func TestDAQT1603StartRejectedUntilConcurrentStopCompletes(t *testing.T) {
	setT1603StopTimeout(t, 200*time.Millisecond)
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	frame := make([]byte, protocol.TCPFrameSize)
	stopSeen := make(chan struct{})
	releaseStopACK := make(chan struct{})
	secondStartSeen := make(chan struct{})
	serverErr := make(chan error, 1)
	go func() {
		cmd, err := readWithTimeout(server, testReadTimeout)
		if err != nil || cmd != "@f0 FFFF 2" {
			serverErr <- fmt.Errorf("first start command = %q, err = %v", cmd, err)
			return
		}
		if _, err := server.Write(append([]byte{'A'}, frame...)); err != nil {
			serverErr <- err
			return
		}
		cmd, err = readWithTimeout(server, testReadTimeout)
		if err != nil || cmd != "@f1" {
			serverErr <- fmt.Errorf("stop command = %q, err = %v", cmd, err)
			return
		}
		close(stopSeen)
		<-releaseStopACK
		if _, err := server.Write([]byte{'A'}); err != nil {
			serverErr <- err
			return
		}
		cmd, err = readWithTimeout(server, testReadTimeout)
		if err != nil || cmd != "@f0 FFFF 2" {
			serverErr <- fmt.Errorf("second start command = %q, err = %v", cmd, err)
			return
		}
		close(secondStartSeen)
		serverErr <- nil
	}()

	device := NewDAQT1603(core.Profile{ID: "t1603-stop-start-race", Type: core.DeviceDaqT1603})
	device.conn = client
	device.frameReader = protocol.NewT1603FrameReader(client)
	device.frameReader.SetBinaryMode(true)
	device.status.Connection = core.ConnectionConnected
	device.config = core.DaqT1603HardwareConfig{ChannelMask: "FFFF", BinaryFormat: true}
	if err := device.StartAcquisition(); err != nil {
		t.Fatalf("initial StartAcquisition returned error: %v", err)
	}
	time.Sleep(20 * time.Millisecond)

	stopResult := make(chan error, 1)
	go func() { stopResult <- device.StopAcquisition() }()
	select {
	case <-stopSeen:
	case <-time.After(testReadTimeout):
		t.Fatal("device did not send @f1")
	}

	startResult := make(chan error, 1)
	go func() { startResult <- device.StartAcquisition() }()
	select {
	case err := <-startResult:
		if err == nil || !strings.Contains(err.Error(), "stop in progress") {
			t.Fatalf("concurrent StartAcquisition error = %v, want stop in progress", err)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("concurrent StartAcquisition blocked instead of rejecting stop in progress")
	}
	close(releaseStopACK)
	if err := <-stopResult; err != nil {
		t.Fatalf("StopAcquisition returned error: %v", err)
	}
	if err := device.StartAcquisition(); err != nil {
		t.Fatalf("StartAcquisition after completed stop returned error: %v", err)
	}
	select {
	case <-secondStartSeen:
	case <-time.After(testReadTimeout):
		t.Fatal("second @f0 was not sent after stop completed")
	}
	if err := <-serverErr; err != nil {
		t.Fatal(err)
	}
}

// =================================================================
// ADR-009 watchdog 兜底测试（P0-5）
// -----------------------------------------------------------------
// 设计依据 ADR-009：SetReadDeadline / SetWriteDeadline 在某些 Windows
// 电脑不可靠，Read/Write 在 deadline 到期后仍可能无限阻塞。T1603
// stopAcquisitionLocked / readLoop 必须有独立 watchdog 计时器，超时
// 强制 Close conn 解除阻塞，且 join 超时后必须废弃连接（避免 readLoop
// 残留 goroutine 与新命令竞争 conn）。
// =================================================================

// t1603DeadlineIgnoringConn 忽略 SetReadDeadline 与 SetWriteDeadline，
// 模拟 ADR-009 描述的"deadline 在故障 Windows 电脑上失效"场景。
// 此时 Read/Write 不会在 deadline 到期后返回，必须依赖 watchdog Close
// 解除阻塞。仅 Close 后 Read/Write 才返回错误。
//
// 放在 hardware 包内（不复用 protocol 包的同名类型）：
//   - protocol 包的 deadlineIgnoringConn 是未导出类型，跨包无法直接使用
//   - T1603 的 @f1 Write 路径同时涉及 Read 和 Write 阻塞，需要同时忽略两个 deadline
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

// t1603RecordingDeadlineIgnoringConn 在 t1603DeadlineIgnoringConn 基础上
// 增加 Write 调用计数，用于验证 join 超时后是否继续发送命令。
//
// ADR-009 R0-1 测试专用：修复后 StartAcquisition join 超时应直接废弃连接，
// 不调用任何 Write；修复前会继续 @f1 Write（即使最终失败）。
type t1603RecordingDeadlineIgnoringConn struct {
	*t1603DeadlineIgnoringConn
	writeCount int32
}

type t1603BlockingCloseConn struct {
	*t1603DeadlineIgnoringConn
	closeStarted chan struct{}
	releaseClose chan struct{}
}

func newT1603BlockingCloseConn(inner net.Conn) *t1603BlockingCloseConn {
	return &t1603BlockingCloseConn{
		t1603DeadlineIgnoringConn: newT1603DeadlineIgnoringConn(inner),
		closeStarted:              make(chan struct{}),
		releaseClose:              make(chan struct{}),
	}
}

func (c *t1603BlockingCloseConn) Close() error {
	c.once.Do(func() {
		close(c.closeStarted)
		<-c.releaseClose
		close(c.closed)
	})
	return c.Conn.Close()
}

func newT1603RecordingDeadlineIgnoringConn(inner net.Conn) *t1603RecordingDeadlineIgnoringConn {
	return &t1603RecordingDeadlineIgnoringConn{
		t1603DeadlineIgnoringConn: newT1603DeadlineIgnoringConn(inner),
	}
}

// Write 覆盖以记录调用次数，底层委托给 t1603DeadlineIgnoringConn（阻塞语义不变）。
func (c *t1603RecordingDeadlineIgnoringConn) Write(b []byte) (int, error) {
	atomic.AddInt32(&c.writeCount, 1)
	return c.t1603DeadlineIgnoringConn.Write(b)
}

// WriteCount 返回累计 Write 调用次数（线程安全）。
func (c *t1603RecordingDeadlineIgnoringConn) WriteCount() int {
	return int(atomic.LoadInt32(&c.writeCount))
}

// TestDAQT1603StopReturnsWhileFallbackClosePending 验证 Stop 的 350ms 兜底
// 不被阻塞的 conn.Close 拖住（故障电脑实机验证：conn.Close() 在挂起 Read 时
// 可能永久阻塞，旧实现先 Close 再 close(quietFallback)，Stop 卡满
// stopAcquisitionTimeout）。
//
// 优雅停止契约（2026-07-31 实机验证）：
//   - 兜底 timer 到期（readLoop 未完成边界校验）即返回 nil，不等待 Close
//     完成（Stop 本身成功，连接废弃由后台 FIN+Close 处理，下次 Start 自动重连）；
//   - 连接毒化：conn=nil、status=Error；
//   - 后台 goroutine 发起 Close（阻塞中可观察），release 后连接真正关闭；
//   - readLoop 随后退出（EOF）属预期，onReadLoopExit 回调不被调用。
func TestDAQT1603StopReturnsWhileFallbackClosePending(t *testing.T) {
	setT1603StopTimeout(t, 5*time.Second)
	server, client := net.Pipe()
	defer server.Close()
	blocked := newT1603BlockingCloseConn(client)

	device := NewDAQT1603(core.Profile{ID: "t1603-stop-close-order", Type: core.DeviceDaqT1603})
	device.conn = blocked
	device.frameReader = protocol.NewT1603FrameReader(blocked)
	device.frameReader.SetBinaryMode(true)
	device.acquiring = true
	device.streaming = true
	device.stop = make(chan struct{})
	device.readLoopDone = make(chan struct{})
	device.status.Connection = core.ConnectionAcquiring
	device.status.Acquiring = true
	var onExitCalled int32
	device.OnReadLoopExit(func(err error) { atomic.StoreInt32(&onExitCalled, 1) })
	// 缓存 readLoopDone：finalizeStopQuietFallbackLocked 会置 d.readLoopDone=nil，
	// 若 Stop 后仍读 device 字段会阻塞在 nil channel 上（noDataTimer 测试同款坑）。
	readLoopDone := device.readLoopDone
	go device.readLoop(blocked, device.frameReader, readLoopDone)

	go func() {
		_, _ = readWithTimeout(server, testReadTimeout)
	}()

	stopResult := make(chan error, 1)
	go func() { stopResult <- device.StopAcquisition() }()

	// 兜底 timer（350ms）到期即返回，不被阻塞的 Close 拖住（2s 断言窗口能
	// 区分修复后 ~350ms 与修复前 5s 超时）。
	select {
	case err := <-stopResult:
		if err != nil {
			t.Fatalf("StopAcquisition error = %v, want nil (graceful fallback)", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("StopAcquisition did not return within 2s; fallback Close is blocking Stop")
	}

	// Stop 返回后，后台 goroutine 正在 Close（阻塞中），连接尚未真正关闭。
	select {
	case <-blocked.closeStarted:
	case <-time.After(time.Second):
		t.Fatal("background close was not attempted")
	}

	device.mu.RLock()
	conn := device.conn
	status := device.status.Connection
	device.mu.RUnlock()
	if conn != nil || status != core.ConnectionError {
		t.Fatalf("conn=%v status=%v, want nil/Error", conn, status)
	}

	// 释放 Close 后，连接被真正关闭（server.Write 失败），readLoop 退出，
	// 但 EOF 属预期（stopAbandoned），onReadLoopExit 不应被调用。
	close(blocked.releaseClose)
	select {
	case <-blocked.closed:
	case <-time.After(time.Second):
		t.Fatal("conn.Close did not complete after release")
	}
	_ = server.SetWriteDeadline(time.Now().Add(500 * time.Millisecond))
	if _, writeErr := server.Write([]byte("probe")); writeErr == nil {
		t.Error("expected server.Write to fail after conn was closed")
	}
	select {
	case <-readLoopDone:
	case <-time.After(time.Second):
		t.Fatal("readLoop did not exit after conn close")
	}
	if atomic.LoadInt32(&onExitCalled) != 0 {
		t.Error("onReadLoopExit should not be called on graceful stop fallback (EOF is expected)")
	}
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
func TestDAQT1603Disconnect_ClosesConnOnReadLoopJoinTimeout(t *testing.T) {
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
	go func() {
		buf := make([]byte, 16)
		_, _ = server.Read(buf)
	}()

	// 总预算 6s：3s join timeout + 1s 余量 + 2s 缓冲
	done := make(chan error, 1)
	go func() {
		done <- device.Disconnect()
	}()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected 'reconnect required' error, got nil")
		}
		if !strings.Contains(err.Error(), "read loop did not exit") {
			t.Fatalf("error should mention read loop exit, got: %v", err)
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

// TestDAQT1603StopAcquisition_DoesNotCloseHealthyConnWhenNoData 验证
// stopAcquisitionLocked 在正常无数据场景下不会误杀健康连接。
//
// ADR-009 决策 8：可选 ACK、空缓冲探测、quiet-window、drain 等"无数据也正常"
// 的操作，不得把 watchdog 到期等同于物理连接故障。原 drainConnection 通过
// 阻塞 Read + watchdog Close 排空 TCP 缓冲区，空缓冲是正常状态但 watchdog 会
// 关闭健康连接。整改后移除 drain，依赖 frameReader.Reset() 清空应用层缓冲区。
//
// 测试前置：
//   - net.Pipe 建立双向连接
//   - server 不写任何数据（模拟空缓冲，正常状态）
//   - device.conn / frameReader 已设置，acquiring=false（无 readLoop）
//
// 测试步骤：
//   - 调用 device.StopAcquisition()
//
// 期待结果：
//   - 返回 nil（正常停止）
//   - device.conn 仍为非 nil（未被关闭）
//   - device.status.Connection = ConnectionConnected（未变为 Error）
//   - 连接仍可用：server.Write 成功，client.Read 能读到数据
func TestDAQT1603StopAcquisition_DoesNotCloseHealthyConnWhenNoData(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	device := NewDAQT1603(core.Profile{ID: "t1603-healthy", Type: core.DeviceDaqT1603})
	device.mu.Lock()
	device.conn = client
	device.frameReader = protocol.NewT1603FrameReader(client)
	device.acquiring = false // 无 readLoop 运行，仅测试 stop 路径不误杀
	device.status.Connection = core.ConnectionConnected
	device.mu.Unlock()

	// StopAcquisition 应正常返回，不触发任何 watchdog
	if err := device.StopAcquisition(); err != nil {
		t.Fatalf("StopAcquisition returned error on healthy conn: %v", err)
	}

	// 验证 conn 仍可用
	device.mu.RLock()
	connAfter := device.conn
	statusAfter := device.status.Connection
	device.mu.RUnlock()
	if connAfter == nil {
		t.Fatal("device.conn should not be nil after stop on healthy conn")
	}
	if statusAfter == core.ConnectionError {
		t.Fatal("status should not be Error after stop on healthy conn")
	}

	// 验证连接仍可读写。
	// net.Pipe 的 Write 阻塞等待 Read，需用 goroutine 并发读写避免死锁。
	// 启动 client.Read goroutine 等待 server.Write 的数据
	readCh := make(chan struct {
		data string
		err  error
	}, 1)
	go func() {
		_ = client.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
		buf := make([]byte, 16)
		n, err := client.Read(buf)
		readCh <- struct {
			data string
			err  error
		}{string(buf[:n]), err}
	}()

	// 短暂等待确保 client.Read goroutine 已启动并阻塞在 Read 上
	time.Sleep(20 * time.Millisecond)

	// server.Write 应能成功（client.Read 在等待）
	_ = server.SetWriteDeadline(time.Now().Add(500 * time.Millisecond))
	if _, err := server.Write([]byte("alive")); err != nil {
		t.Fatalf("server.Write failed after stop, conn was killed: %v", err)
	}

	// 等待 client.Read 完成
	select {
	case r := <-readCh:
		if r.err != nil {
			t.Fatalf("client.Read failed after stop, conn was killed: %v", r.err)
		}
		if r.data != "alive" {
			t.Fatalf("client.Read got %q, want %q", r.data, "alive")
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("client.Read did not complete within 500ms")
	}
}

// TestDAQT1603ApplyDaqT1603Config_DoesNotCloseHealthyConnWhenNoData 验证
// ApplyDaqT1603Config 在正常无数据场景下不会误杀健康连接。
//
// ADR-009 决策 8：原 drainConnection 通过阻塞 Read + watchdog Close 排空 TCP
// 缓冲区，空缓冲是正常状态但 watchdog 会关闭健康连接。整改后移除 drain，
// 依赖 frameReader.Reset() 和 sendCommand 的跳帧逻辑处理残留字节。
//
// 测试前置：
//   - net.Pipe 建立双向连接
//   - server 端读取命令并返回单字节 ACK（与现有 ApplyConfigSendsHardwareCommands 测试一致）
//   - server 不主动发送额外数据（确保空缓冲是正常状态）
//
// 测试步骤：
//   - 调用 device.ApplyDaqT1603Config(cfg)
//
// 期待结果：
//   - 返回 nil（配置成功）
//   - device.conn 仍为非 nil（未被关闭）
//   - device.status.Connection 未变为 Error
//   - 连接仍可用：后续命令能成功收发
func TestDAQT1603ApplyDaqT1603Config_DoesNotCloseHealthyConnWhenNoData(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	// server 端读取命令并返回单字节 ACK "A"。
	// 使用 readWithTimeout 模式与现有 TestDAQT1603ApplyConfigSendsHardwareCommands 一致，
	// 避免 net.Pipe 在边界条件下死锁。
	serverDone := make(chan struct{})
	go func() {
		defer close(serverDone)
		for {
			_, err := readWithTimeout(server, testReadTimeout)
			if err != nil {
				return
			}
			// 所有命令统一回单字节 "A"，由 SendCommand 的 cmdTailTimeout 终止读取
			_, _ = server.Write([]byte("A"))
		}
	}()

	device := NewDAQT1603(core.Profile{ID: "t1603-apply-healthy", Type: core.DeviceDaqT1603})
	device.mu.Lock()
	device.conn = client
	device.frameReader = protocol.NewT1603FrameReader(client)
	device.status.Connection = core.ConnectionConnected
	device.mu.Unlock()

	cfg := core.DaqT1603HardwareConfig{
		ThermocoupleTypes: "KKKKKKKKKKKKKKKK",
		BinaryFormat:      true,
		SamplingRate:      10,
		AverageCount:      1,
		TriggerMode:       0,
		TriggerEdge:       0,
	}

	if err := device.ApplyDaqT1603Config(cfg); err != nil {
		t.Fatalf("ApplyDaqT1603Config returned error on healthy conn: %v", err)
	}

	// 验证 conn 仍可用
	device.mu.RLock()
	connAfter := device.conn
	statusAfter := device.status.Connection
	device.mu.RUnlock()
	if connAfter == nil {
		t.Fatal("device.conn should not be nil after ApplyDaqT1603Config on healthy conn")
	}
	if statusAfter == core.ConnectionError {
		t.Fatal("status should not be Error after ApplyDaqT1603Config on healthy conn")
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

// TestDAQT1603StartAcquisition_InvalidatesConnOnReadLoopJoinTimeout 验证
// ADR-009 R0-1：StartAcquisition 入口在 readLoop join 超时后必须废弃连接，
// 禁止清空 readLoopDone 后在原连接上启动第二个 reader 或继续发送 @f1/@f0 命令。
//
// 历史背景：原实现（修复前）join 超时只打 warn 日志，随后清空 readLoopDone
// 并继续在原连接上执行 @f1/@f0 Write + 启动新 readLoop。问题 Windows 电脑
// deadline 失效导致旧 reader 仍阻塞在 conn.Read 时，新 reader 会与旧 reader
// 竞争同一 TCP 字节流，产生错帧、命令响应被抢读和不可恢复的协议错位。
//
// 测试前置：
//   - net.Pipe 建立双向连接
//   - 包装 client 为 t1603DeadlineIgnoringConn（SetWriteDeadline no-op，
//     模拟 Windows 故障环境 deadline 失效）
//   - device.readLoopDone 设置为未关闭的 chan（模拟 readLoop 卡在 conn.Read）
//   - device.acquiring = false, device.conn 已设置
//   - server goroutine 持续 Read 并记录收到的命令
//
// 测试步骤：
//   - 调用 device.StartAcquisition()
//
// 期待结果：
//   - 返回错误，包含 "reconnect required"
//   - device.conn 被置为 nil
//   - device.status.Connection = ConnectionError
//   - device.readLoopDone 被置为 nil
//   - device.acquiring == false（未启动新 readLoop）
//   - server 未收到任何 @f1/@f0 命令（join 超时后不应继续发送命令）
//   - conn 已被 Close（server.Write 失败）
func TestDAQT1603StartAcquisition_InvalidatesConnOnReadLoopJoinTimeout(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	// 注意：client 由 StartAcquisition -> invalidateConnectionAfterReadLoopTimeout 关闭，
	// 这里不 defer client.Close() 以避免重复 Close 报错。

	// 包装 client：忽略 deadline + 记录 Write 调用次数
	recordingClient := newT1603RecordingDeadlineIgnoringConn(client)

	// 不启动 server Read goroutine：让 @f1 Write 阻塞（deadlineIgnoringConn 忽略
	// SetWriteDeadline），watchdog 1s 后 Close conn 解除阻塞。
	// 修复前：join 超时后继续 @f1 Write → 阻塞 → watchdog Close → 返回 watchdog 错误
	// 修复后：join 超时后直接废弃连接，不调用任何 Write

	device := NewDAQT1603(core.Profile{ID: "t1603-start-join-stuck", Type: core.DeviceDaqT1603})
	device.mu.Lock()
	device.conn = recordingClient
	device.frameReader = protocol.NewT1603FrameReader(recordingClient)
	// 伪造一个永远不会被关闭的 readLoopDone，模拟旧 readLoop 卡在 conn.Read
	// 无法退出（deadlineIgnoringConn 场景下 frameReader.ReadFrame 阻塞）。
	device.readLoopDone = make(chan struct{})
	device.acquiring = false
	device.status.Connection = core.ConnectionConnected
	device.mu.Unlock()

	// 总预算 6s：3s join timeout + 1s 余量 + 2s 缓冲
	done := make(chan error, 1)
	go func() {
		done <- device.StartAcquisition()
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
		t.Fatal("StartAcquisition did not return within 6s budget; join timeout likely did not invalidate conn")
	}

	// 验证 conn 已被废弃：device.conn == nil
	device.mu.RLock()
	connAfter := device.conn
	statusAfter := device.status.Connection
	acquiringAfter := device.acquiring
	readLoopDoneAfter := device.readLoopDone
	device.mu.RUnlock()
	if connAfter != nil {
		t.Fatal("device.conn should be nil after readLoop join timeout (must not start second reader on same conn)")
	}
	if statusAfter != core.ConnectionError {
		t.Fatalf("status.Connection = %v, want Error", statusAfter)
	}
	if acquiringAfter {
		t.Fatal("device.acquiring should be false; must not start new readLoop after join timeout")
	}
	if readLoopDoneAfter != nil {
		t.Fatal("device.readLoopDone should be nil after invalidate; must not leave stale done channel")
	}

	// 验证 join 超时后未继续发送 @f1/@f0 命令
	// 修复前：join 超时后继续 @f1 Write（recordingClient 会记录 Write 调用）
	// 修复后：join 超时直接废弃连接，不调用任何 Write
	if writeCount := recordingClient.WriteCount(); writeCount > 0 {
		t.Fatalf("StartAcquisition should not send any command after join timeout, but Write was called %d times", writeCount)
	}

	// 验证 conn 已被 Close：server.Write 应失败
	_ = server.SetWriteDeadline(time.Now().Add(200 * time.Millisecond))
	if _, writeErr := server.Write([]byte("probe")); writeErr == nil {
		t.Error("expected server.Write to fail after client was closed by invalidate")
	}
}

// TestDAQT1603ReadLoop_InvalidatesConnOnTerminalReadError 验证 ADR-009 R0-11：
// readLoop 在收到 terminal read error（EOF/RST/协议错误）后必须统一毒化连接——
// 清空 d.conn/d.frameReader、置 Error 状态、保存 LastError、close conn、
// 通知 onReadLoopExit 回调。
//
// 历史背景：原实现 readLoop defer 在 unexpectedErr != nil 时仅把 status 从
// Acquiring 改回 Connected，未清空 conn/frameReader，也未 close conn。EOF 后
// 连接已死，下次 StartAcquisition 会用旧 conn 发命令爆 WSAECONNABORTED。
//
// 测试前置：
//   - net.Pipe 建立双向连接
//   - device.conn / frameReader 已设置，acquiring=true
//   - 启动 readLoop goroutine
//
// 测试步骤：
//   - 关闭 server 端模拟对端 EOF（client.Read 返回 io.EOF）
//   - 等待 readLoopDone 关闭
//
// 期待结果：
//   - d.conn 被置为 nil
//   - d.frameReader 被置为 nil
//   - d.status.Connection = ConnectionError
//   - d.status.LastError 非空
//   - conn 已被 Close（server.Write 失败）
//   - onReadLoopExit 回调被调用并收到非 nil error
func TestDAQT1603ReadLoop_InvalidatesConnOnTerminalReadError(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	// 注意：client 由 readLoop defer -> invalidateConnectionAfterReadLoopTimeout 关闭，
	// 这里不 defer client.Close() 以避免重复 Close 报错。

	device := NewDAQT1603(core.Profile{ID: "t1603-eof", Type: core.DeviceDaqT1603})
	device.mu.Lock()
	device.conn = client
	device.frameReader = protocol.NewT1603FrameReader(client)
	device.acquiring = true
	device.stop = make(chan struct{})
	device.readLoopDone = make(chan struct{})
	device.status.Connection = core.ConnectionAcquiring
	device.status.Acquiring = true
	device.mu.Unlock()

	var onReadLoopExitCalled int32
	var capturedErr error
	var errMu sync.Mutex
	device.OnReadLoopExit(func(err error) {
		atomic.StoreInt32(&onReadLoopExitCalled, 1)
		errMu.Lock()
		capturedErr = err
		errMu.Unlock()
	})

	// 启动 readLoop（直接调用未导出方法，仅在包内测试可用）
	go device.readLoop(device.conn, device.frameReader, device.readLoopDone)

	// 等待 readLoop 进入 ReadFrame 阻塞
	time.Sleep(50 * time.Millisecond)

	// 关闭 server 模拟对端 EOF：client.Read 返回 io.EOF
	server.Close()

	// 等待 readLoop 退出
	device.mu.RLock()
	done := device.readLoopDone
	device.mu.RUnlock()
	// readLoopDone 可能被 invalidate 置为 nil，需缓存当前值
	if done == nil {
		// readLoop 已退出且 invalidate 已清空 readLoopDone
	} else {
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			t.Fatal("readLoop did not exit within 3s after server EOF")
		}
	}

	// 短暂等待 defer 完成状态清理
	time.Sleep(100 * time.Millisecond)

	// 验证状态已被统一毒化
	device.mu.RLock()
	connAfter := device.conn
	frameReaderAfter := device.frameReader
	statusAfter := device.status.Connection
	lastError := device.status.LastError
	device.mu.RUnlock()
	if connAfter != nil {
		t.Error("d.conn should be nil after terminal read error")
	}
	if frameReaderAfter != nil {
		t.Error("d.frameReader should be nil after terminal read error")
	}
	if statusAfter != core.ConnectionError {
		t.Errorf("status.Connection = %v, want Error", statusAfter)
	}
	if lastError == "" {
		t.Error("status.LastError should be non-empty after terminal read error")
	}
	if atomic.LoadInt32(&onReadLoopExitCalled) != 1 {
		t.Error("onReadLoopExit callback should be invoked")
	}
	errMu.Lock()
	if capturedErr == nil {
		t.Error("onReadLoopExit callback should receive non-nil error")
	}
	errMu.Unlock()
}

// TestDAQT1603ReadLoop_InvalidatesConnOnNoDataTimeout 验证 ADR-009 R0-10：
// readLoop 入口启动的独立 no-data timer 在 deadline 失效（Read 永久阻塞）且
// 无任何数据到达时，必须在 noDataTimeout 到期后独立触发连接毒化——
// 清空 d.conn/d.frameReader、置 Error 状态、close conn、通知 onReadLoopExit。
//
// 关键差异（与 terminal read error 测试的对比）：
//   - terminal read error：对端 EOF → Read 返回错误 → defer invalidate
//   - no-data timer：无数据 → timer 到期 → Close conn → Read 返回 closed 错误
//     → readLoop 走 unexpectedErr 路径调用 invalidate + onReadLoopExit
//
// timer 必须独立于 readLoop 循环体执行。本测试用 t1603DeadlineIgnoringConn 让
// Read 永久阻塞（模拟 Windows 故障环境下 deadline 失效），循环体不可达，
// 仅靠 timer 到期能触发毒化。若 timer 未启动或依赖循环体，测试会超时。
//
// 测试前置：
//   - net.Pipe 建立双向连接，client 端包 t1603DeadlineIgnoringConn（Read 永久阻塞）
//   - device.conn / frameReader / readLoopDone 已设置，acquiring=true
//   - noDataTimeout 临时覆盖为 200ms（生产默认 10s）
//   - server 端不写任何数据，让 client Read 永久阻塞
//
// 测试步骤：
//   - 启动 readLoop goroutine
//   - 等待 noDataTimeout(200ms) + 余量(800ms) = 1s 预算
//
// 期待结果：
//   - d.conn 被置为 nil（timer 回调 + invalidate 双重设置）
//   - d.frameReader 被置为 nil
//   - d.status.Connection = ConnectionError
//   - d.status.LastError 非空（timer 设 "no data" 或 invalidate 覆盖为 closed err）
//   - onReadLoopExit 回调被调用并收到非 nil error
//   - readLoopDone channel 已关闭（readLoop 已退出）
func TestDAQT1603ReadLoop_InvalidatesConnOnNoDataTimeout(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()

	// 临时覆盖 noDataTimeout 为 200ms 加速测试。
	// 同一包内测试默认串行执行，覆盖安全；t.Cleanup 确保恢复。
	origTimeout := noDataTimeout
	noDataTimeout = 200 * time.Millisecond
	t.Cleanup(func() { noDataTimeout = origTimeout })

	device := NewDAQT1603(core.Profile{ID: "t1603-nodata", Type: core.DeviceDaqT1603})
	var onReadLoopExitCalled int32
	var capturedErr error
	var errMu sync.Mutex
	device.OnReadLoopExit(func(err error) {
		atomic.StoreInt32(&onReadLoopExitCalled, 1)
		errMu.Lock()
		capturedErr = err
		errMu.Unlock()
	})

	device.mu.Lock()
	device.conn = newT1603DeadlineIgnoringConn(client)
	device.frameReader = protocol.NewT1603FrameReader(device.conn)
	device.acquiring = true
	device.stop = make(chan struct{})
	device.readLoopDone = make(chan struct{})
	device.status.Connection = core.ConnectionAcquiring
	device.status.Acquiring = true
	device.mu.Unlock()

	// 缓存 readLoopDone：noDataTimer 回调会设置 d.readLoopDone=nil，
	// 测试必须先缓存局部变量才能等待 readLoop 退出。
	device.mu.RLock()
	done := device.readLoopDone
	device.mu.RUnlock()

	// 启动 readLoop（直接调用未导出方法，仅在包内测试可用）
	go device.readLoop(device.conn, device.frameReader, device.readLoopDone)

	// 预算 1s：覆盖 200ms noDataTimer + Read 在 conn Close 后返回的延迟 + defer 清理。
	// 若 timer 未触发，readLoop 永久阻塞，select 会超时 fatal。
	select {
	case <-done:
	case <-time.After(1 * time.Second):
		t.Fatal("readLoop did not exit within 1s; no-data timer may not have fired")
	}

	// 短暂等待 defer 完成状态清理
	time.Sleep(100 * time.Millisecond)

	device.mu.RLock()
	connAfter := device.conn
	frameReaderAfter := device.frameReader
	statusAfter := device.status.Connection
	lastError := device.status.LastError
	device.mu.RUnlock()
	if connAfter != nil {
		t.Error("d.conn should be nil after no-data timer fired")
	}
	if frameReaderAfter != nil {
		t.Error("d.frameReader should be nil after no-data timer fired")
	}
	if statusAfter != core.ConnectionError {
		t.Errorf("status.Connection = %v, want Error", statusAfter)
	}
	if lastError == "" {
		t.Error("status.LastError should be non-empty after no-data timer fired")
	}
	if atomic.LoadInt32(&onReadLoopExitCalled) != 1 {
		t.Error("onReadLoopExit callback should be invoked after no-data timer fired")
	}
	errMu.Lock()
	if capturedErr == nil {
		t.Error("onReadLoopExit callback should receive non-nil error")
	}
	errMu.Unlock()
}

// =================================================================
// ADR-009 第三批整改测试（R0-2 / R0-4 / R0-12）
// -----------------------------------------------------------------
// R0-2：writeCommandOnly 必须在 writeMu.Lock 之前启动 watchdog，
//       防止上一任锁持有者在 Write 中永久阻塞导致死锁。
// R0-4：命令路径返回 ErrWatchdogTriggered 时调用 invalidateConnection
//       统一毒化连接，expectedConn 比较避免与并发操作竞争。
// R0-12：SendCommand/SendCommandIdle/SendCommandExact soft timeout 时
//        强制 Close conn 阻断迟到响应，防止协议错位。
// =================================================================

// TestDAQT1603InvalidateConnection_PoisonsMatchingConn 验证 R0-4：
// invalidateConnection 在 expectedConn 匹配 d.conn 时统一毒化连接——
// 清空 conn/frameReader、置 Error 状态、保存 LastError、Close conn。
//
// 测试前置：
//   - net.Pipe 建立双向连接
//   - device.conn / frameReader 已设置，status=Connected
//
// 测试步骤：
//   - 调用 invalidateConnection(expectedConn=client, reason="test")
//
// 期待结果：
//   - 返回 true（conn 被毒化）
//   - device.conn 被置为 nil
//   - device.frameReader 被置为 nil
//   - device.status.Connection = ConnectionError
//   - device.status.LastError = "test"
//   - conn 已被 Close（server.Write 失败）
func TestDAQT1603InvalidateConnection_PoisonsMatchingConn(t *testing.T) {
	client, server := net.Pipe()
	defer server.Close()
	// 注意：client 由 invalidateConnection 关闭，不 defer client.Close()

	device := NewDAQT1603(core.Profile{ID: "t1603-inval-match", Type: core.DeviceDaqT1603})
	device.mu.Lock()
	device.conn = client
	device.frameReader = protocol.NewT1603FrameReader(client)
	device.status.Connection = core.ConnectionConnected
	device.mu.Unlock()

	ok := device.invalidateConnection(client, "test reason")
	if !ok {
		t.Fatal("invalidateConnection should return true when expectedConn matches")
	}

	device.mu.RLock()
	connAfter := device.conn
	frameReaderAfter := device.frameReader
	statusAfter := device.status.Connection
	lastError := device.status.LastError
	device.mu.RUnlock()

	if connAfter != nil {
		t.Error("device.conn should be nil after invalidate")
	}
	if frameReaderAfter != nil {
		t.Error("device.frameReader should be nil after invalidate")
	}
	if statusAfter != core.ConnectionError {
		t.Errorf("status.Connection = %v, want Error", statusAfter)
	}
	if lastError != "test reason" {
		t.Errorf("status.LastError = %q, want %q", lastError, "test reason")
	}

	// 验证 conn 已被 Close：server.Write 应失败
	_ = server.SetWriteDeadline(time.Now().Add(200 * time.Millisecond))
	if _, writeErr := server.Write([]byte("probe")); writeErr == nil {
		t.Error("expected server.Write to fail after client was closed by invalidate")
	}
}

// TestDAQT1603InvalidateConnection_NoopOnMismatch 验证 R0-4：
// invalidateConnection 在 expectedConn 不匹配 d.conn 时是 no-op，
// 避免与并发 readLoop 错误或重连后的新 conn 竞争。
//
// 测试前置：
//   - net.Pipe 建立双向连接（oldConn）
//   - device.conn 已被并发操作替换为 newConn（模拟重连场景）
//
// 测试步骤：
//   - 调用 invalidateConnection(expectedConn=oldConn, reason="stale")
//
// 期待结果：
//   - 返回 false（no-op）
//   - device.conn 仍为 newConn（未被替换或清空）
//   - device.status.Connection 未变为 Error
//   - oldConn 未被 Close（仍可写入；但 oldConn 是测试私有，验证 newConn 仍活即可）
//   - newConn 仍可用（server2.Write 成功）
func TestDAQT1603InvalidateConnection_NoopOnMismatch(t *testing.T) {
	oldClient, oldServer := net.Pipe()
	defer oldServer.Close()
	defer oldClient.Close() // oldClient 不会被 invalidate 关闭

	newClient, newServer := net.Pipe()
	defer newServer.Close()
	// newClient 由 device 持有，测试结束由 device 释放（这里 defer newServer 即可）

	device := NewDAQT1603(core.Profile{ID: "t1603-inval-mismatch", Type: core.DeviceDaqT1603})
	device.mu.Lock()
	// 模拟并发场景：device.conn 已被重连替换为 newClient
	device.conn = newClient
	device.frameReader = protocol.NewT1603FrameReader(newClient)
	device.status.Connection = core.ConnectionConnected
	device.mu.Unlock()

	// 用 oldConn 调用 invalidate，应 no-op（d.conn=newClient != oldClient）
	ok := device.invalidateConnection(oldClient, "stale reason")
	if ok {
		t.Fatal("invalidateConnection should return false when expectedConn does not match")
	}

	device.mu.RLock()
	connAfter := device.conn
	statusAfter := device.status.Connection
	lastError := device.status.LastError
	device.mu.RUnlock()

	if connAfter != newClient {
		t.Error("device.conn should still be newClient (no-op on mismatch)")
	}
	if statusAfter == core.ConnectionError {
		t.Errorf("status should not be Error on mismatch; got %v", statusAfter)
	}
	if lastError != "" {
		t.Errorf("status.LastError should be empty on mismatch; got %q", lastError)
	}

	// 验证 newConn 仍可用：newServer.Write 成功 + newClient.Read 能读到
	readCh := make(chan string, 1)
	go func() {
		_ = newClient.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
		buf := make([]byte, 16)
		n, _ := newClient.Read(buf)
		readCh <- string(buf[:n])
	}()
	time.Sleep(20 * time.Millisecond)
	_ = newServer.SetWriteDeadline(time.Now().Add(500 * time.Millisecond))
	if _, err := newServer.Write([]byte("alive")); err != nil {
		t.Fatalf("newServer.Write failed; newConn was killed by mistaken invalidate: %v", err)
	}
	select {
	case got := <-readCh:
		if got != "alive" {
			t.Fatalf("newClient.Read got %q, want %q", got, "alive")
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("newClient.Read did not complete within 500ms")
	}

	// 验证 oldConn 也未被 Close（oldServer.Write 仍能成功）
	// net.Pipe Write 阻塞等待对端 Read，必须先启动 oldClient.Read goroutine。
	oldReadCh := make(chan struct {
		data string
		err  error
	}, 1)
	go func() {
		_ = oldClient.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
		buf := make([]byte, 16)
		n, err := oldClient.Read(buf)
		oldReadCh <- struct {
			data string
			err  error
		}{string(buf[:n]), err}
	}()
	time.Sleep(20 * time.Millisecond)
	_ = oldServer.SetWriteDeadline(time.Now().Add(500 * time.Millisecond))
	if _, err := oldServer.Write([]byte("x")); err != nil {
		t.Errorf("oldServer.Write failed; oldConn was mistakenly closed: %v", err)
	}
	select {
	case <-oldReadCh:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("oldClient.Read did not complete within 500ms")
	}
}

// TestDAQT1603InvalidateConnection_NoopOnNilConn 验证 R0-4：
// invalidateConnection 在 d.conn 已为 nil（已被并发毒化）时是 no-op。
//
// 测试前置：
//   - device.conn = nil（模拟 readLoop 已先一步毒化连接）
//   - expectedConn 仍指向旧 client
//
// 测试步骤：
//   - 调用 invalidateConnection(expectedConn=oldClient, reason="race")
//
// 期待结果：
//   - 返回 false（no-op）
//   - device.conn 仍为 nil
//   - device.status 未被覆盖（保留 readLoop 设置的 Error 状态和 LastError）
func TestDAQT1603InvalidateConnection_NoopOnNilConn(t *testing.T) {
	client, server := net.Pipe()
	defer server.Close()
	defer client.Close() // client 不会被 invalidate 关闭

	device := NewDAQT1603(core.Profile{ID: "t1603-inval-nil", Type: core.DeviceDaqT1603})
	device.mu.Lock()
	// 模拟 readLoop no-data timer 已先一步毒化连接
	device.conn = nil
	device.frameReader = nil
	device.status.Connection = core.ConnectionError
	device.status.LastError = "no data received for 10s"
	device.mu.Unlock()

	ok := device.invalidateConnection(client, "race reason")
	if ok {
		t.Fatal("invalidateConnection should return false when d.conn is nil")
	}

	device.mu.RLock()
	connAfter := device.conn
	statusAfter := device.status.Connection
	lastError := device.status.LastError
	device.mu.RUnlock()

	if connAfter != nil {
		t.Error("device.conn should still be nil")
	}
	if statusAfter != core.ConnectionError {
		t.Errorf("status should remain Error; got %v", statusAfter)
	}
	if lastError != "no data received for 10s" {
		t.Errorf("status.LastError should not be overwritten; got %q", lastError)
	}
}

// TestDAQT1603WriteCommandOnly_HasWatchdogBeforeLock 验证 R0-2：
// writeCommandOnly 必须在 writeMu.Lock 之前启动 watchdog。
// 模拟场景：上一任 writeMu 持有者在 deadlineIgnoringConn 上 Write 永久阻塞，
// writeCommandOnly 的 watchdog 必须能 Close conn 解除阻塞，让本调用最终拿到锁。
//
// 测试前置：
//   - net.Pipe 建立双向连接，client 端包 t1603DeadlineIgnoringConn（Write 永久阻塞）
//   - device.conn = ignored, acquiring=false（不触发 @f1 路径）
//   - device.writeMu 已被另一 goroutine 持有（模拟上一任锁持有者阻塞在 Write 中）
//
// 测试步骤：
//   - 启动 goroutine 持有 writeMu 并阻塞在 conn.Write（永不返回）
//   - 调用 device.writeCommandOnly(ignored, "@fe BIN 1")
//
// 期待结果：
//   - writeCommandOnly 在 DAQ_T_1603_TIMEOUT(5s) + 余量 内返回错误
//   - 返回错误包含 ErrWatchdogTriggered
//   - conn 已被 Close（server.Write 失败）
func TestDAQT1603WriteCommandOnly_HasWatchdogBeforeLock(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	// client 由 watchdog 关闭，不 defer client.Close()

	ignored := newT1603DeadlineIgnoringConn(client)
	device := NewDAQT1603(core.Profile{ID: "t1603-write-lock-stuck", Type: core.DeviceDaqT1603})
	device.mu.Lock()
	device.conn = ignored
	device.frameReader = protocol.NewT1603FrameReader(ignored)
	device.status.Connection = core.ConnectionConnected
	device.mu.Unlock()

	// 模拟上一任 writeMu 持有者阻塞在 Write：先抢锁，再 goroutine 中 Write 永久阻塞。
	// 由于 deadlineIgnoringConn 忽略 SetWriteDeadline，Write 在 net.Pipe 无对端 Read 时永久阻塞。
	writeMuHeld := make(chan struct{})
	writeMuReleased := make(chan struct{})
	go func() {
		device.writeMu.Lock()
		close(writeMuHeld)
		// Write 永久阻塞（server 端不 Read），watchdog Close 后才能解除
		_, _ = ignored.Write([]byte("stuck"))
		device.writeMu.Unlock()
		close(writeMuReleased)
	}()

	// 等待 goroutine 拿到 writeMu
	select {
	case <-writeMuHeld:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("goroutine did not acquire writeMu within 500ms")
	}

	// server 端不读不写，让 ignored.Write 永久阻塞。

	// 调用 writeCommandOnly：watchdog 必须在 writeMu.Lock 之前启动，
	// DAQ_T_1603_TIMEOUT(5s) 后 Close conn 解除 ignored.Write 阻塞，
	// goroutine 释放 writeMu，writeCommandOnly 拿到锁后返回 ErrWatchdogTriggered。
	// 总预算 7s：5s watchdog + 1s 锁传递 + 1s 余量。
	done := make(chan error, 1)
	go func() {
		done <- device.writeCommandOnly(ignored, "@fe BIN 1")
	}()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected error from writeCommandOnly, got nil")
		}
		if !errors.Is(err, protocol.ErrWatchdogTriggered) {
			t.Fatalf("expected error to wrap ErrWatchdogTriggered, got: %v", err)
		}
	case <-time.After(7 * time.Second):
		t.Fatal("writeCommandOnly did not return within 7s budget; watchdog likely not armed before writeMu.Lock")
	}

	// 验证 conn 已被 Close：server.Write 应失败
	_ = server.SetWriteDeadline(time.Now().Add(200 * time.Millisecond))
	if _, writeErr := server.Write([]byte("probe")); writeErr == nil {
		t.Error("expected server.Write to fail after client was closed by watchdog")
	}

	// 等待后台 goroutine 退出，避免 race
	select {
	case <-writeMuReleased:
	case <-time.After(2 * time.Second):
		t.Fatal("stuck Write goroutine did not release writeMu within 2s after watchdog Close")
	}
}

// TestDAQT1603ApplyDaqT1603Config_InvalidatesConnOnWatchdogTrigger 验证 R0-4 集成路径：
// ApplyDaqT1603Config 在 applyHardwareConfig 返回 ErrWatchdogTriggered 时
// 调用 invalidateConnection 统一毒化连接。
//
// 测试前置：
//   - net.Pipe 建立双向连接，client 端包 t1603DeadlineIgnoringConn（Read 永久阻塞）
//   - device.conn = ignored, frameReader 已设置
//   - server 不读不写（让 client.Read 阻塞，触发 SendCommand 内部 watchdog）
//
// 测试步骤：
//   - 调用 device.ApplyDaqT1603Config(cfg)
//
// 期待结果：
//   - 返回错误包含 ErrWatchdogTriggered
//   - device.conn 被置为 nil
//   - device.status.Connection = ConnectionError
//   - device.status.LastError 包含 "config apply watchdog triggered"
//   - conn 已被 Close（server.Write 失败）
func TestDAQT1603ApplyDaqT1603Config_InvalidatesConnOnWatchdogTrigger(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	// client 由 watchdog Close 后 invalidateConnection 再次 Close（幂等）

	ignored := newT1603DeadlineIgnoringConn(client)
	device := NewDAQT1603(core.Profile{ID: "t1603-apply-stuck", Type: core.DeviceDaqT1603})
	device.mu.Lock()
	device.conn = ignored
	device.frameReader = protocol.NewT1603FrameReader(ignored)
	device.status.Connection = core.ConnectionConnected
	device.mu.Unlock()

	cfg := core.DaqT1603HardwareConfig{
		ThermocoupleTypes: "KKKKKKKKKKKKKKKK",
		BinaryFormat:      true,
	}

	// 总预算 5s：第 1 条 @f3 命令的 SendCommand 内部 watchdog 2s 触发，
	// applyHardwareConfig 立即返回，ApplyDaqT1603Config 调用 invalidateConnection。
	done := make(chan error, 1)
	go func() {
		done <- device.ApplyDaqT1603Config(cfg)
	}()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected ApplyDaqT1603Config to fail when watchdog triggered, got nil")
		}
		if !errors.Is(err, protocol.ErrWatchdogTriggered) {
			t.Fatalf("expected error to wrap ErrWatchdogTriggered, got: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("ApplyDaqT1603Config did not return within 5s budget; watchdog likely not propagated")
	}

	// 验证连接已被统一毒化
	device.mu.RLock()
	connAfter := device.conn
	frameReaderAfter := device.frameReader
	statusAfter := device.status.Connection
	lastError := device.status.LastError
	device.mu.RUnlock()

	if connAfter != nil {
		t.Error("device.conn should be nil after watchdog-triggered invalidate")
	}
	if frameReaderAfter != nil {
		t.Error("device.frameReader should be nil after watchdog-triggered invalidate")
	}
	if statusAfter != core.ConnectionError {
		t.Errorf("status.Connection = %v, want Error", statusAfter)
	}
	if !strings.Contains(lastError, "watchdog triggered") {
		t.Errorf("status.LastError = %q, want contains 'watchdog triggered'", lastError)
	}

	// 验证 conn 已被 Close：server.Write 应失败
	_ = server.SetWriteDeadline(time.Now().Add(200 * time.Millisecond))
	if _, writeErr := server.Write([]byte("probe")); writeErr == nil {
		t.Error("expected server.Write to fail after client was closed by invalidate")
	}
}

// TestDAQT1603StartAcquisition_InvalidatesConnOnStartCmdWatchdog 验证 R0-4 集成路径：
// StartAcquisition 在 writeCommandOnly(@f0) 返回 ErrWatchdogTriggered 时
// 统一毒化连接（直接改字段，因为已持有 d.mu）。
//
// 测试前置：
//   - net.Pipe 建立双向连接，client 端包 t1603DeadlineIgnoringConn（Write 永久阻塞）
//   - device.conn = ignored, acquiring=false, readLoopDone=nil（跳过 join 阶段）
//   - server 不读不写（让 @f1 和 @f0 Write 都阻塞，但 @f1 路径有独立 watchdog）
//
// 测试步骤：
//   - 调用 device.StartAcquisition()
//
// 期待结果：
//   - 返回错误包含 ErrWatchdogTriggered
//   - device.conn 被置为 nil
//   - device.status.Connection = ConnectionError
//   - device.acquiring == false（未启动 readLoop）
func TestDAQT1603StartAcquisition_InvalidatesConnOnStartCmdWatchdog(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	// client 由 watchdog 关闭

	ignored := newT1603DeadlineIgnoringConn(client)
	device := NewDAQT1603(core.Profile{ID: "t1603-start-cmd-stuck", Type: core.DeviceDaqT1603})
	device.mu.Lock()
	device.conn = ignored
	device.frameReader = protocol.NewT1603FrameReader(ignored)
	device.acquiring = false
	device.readLoopDone = nil // 跳过 join 阶段
	device.status.Connection = core.ConnectionConnected
	device.config = core.DaqT1603HardwareConfig{
		ChannelMask:  "FFFF",
		BinaryFormat: true,
	}
	device.mu.Unlock()

	// 总预算 8s：
	//   - @f1 Write watchdog 1s 触发 → 不 invalidate（依赖后续 join 阶段，但 join 跳过）
	//   - 50ms sleep
	//   - @f0 writeCommandOnly watchdog 5s 触发 → 返回 ErrWatchdogTriggered → invalidate
	// 实际路径：@f1 watchdog 1s Close conn，@f0 Write 立即失败（conn 已死），
	// 但 writeCommandOnly 仍启动 watchdog，watchdog 5s 后再次 Close（幂等）。
	// 总耗时约 1s + 50ms + 5s = 6.05s，预算 8s 足够。
	done := make(chan error, 1)
	go func() {
		done <- device.StartAcquisition()
	}()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected StartAcquisition to fail when @f0 watchdog triggered, got nil")
		}
		if !errors.Is(err, protocol.ErrWatchdogTriggered) {
			t.Fatalf("expected error to wrap ErrWatchdogTriggered, got: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("StartAcquisition did not return within 10s budget; @f0 watchdog likely not propagated")
	}

	// 验证连接已被毒化
	device.mu.RLock()
	connAfter := device.conn
	statusAfter := device.status.Connection
	acquiringAfter := device.acquiring
	lastError := device.status.LastError
	device.mu.RUnlock()

	if connAfter != nil {
		t.Error("device.conn should be nil after @f0 watchdog-triggered invalidate")
	}
	if statusAfter != core.ConnectionError {
		t.Errorf("status.Connection = %v, want Error", statusAfter)
	}
	if acquiringAfter {
		t.Error("device.acquiring should be false; must not start readLoop after watchdog trigger")
	}
	if !strings.Contains(lastError, "watchdog triggered") {
		t.Errorf("status.LastError = %q, want contains 'watchdog triggered'", lastError)
	}

	// 验证 conn 已被 Close：server.Write 应失败
	_ = server.SetWriteDeadline(time.Now().Add(200 * time.Millisecond))
	if _, writeErr := server.Write([]byte("probe")); writeErr == nil {
		t.Error("expected server.Write to fail after client was closed by invalidate")
	}
}

// TestDAQT1603SendCommand_AcceptsAAsSuccess 验证 sendCommand 收到单字节 'A'
// 时返回成功，与 SKILL.md 中 @fe/@f3 ACK 契约一致。
//
// 测试前置：
//   - net.Pipe 建立双向连接
//   - server goroutine 读取命令后回写单字节 'A'
//
// 测试步骤：
//   - 调用 device.sendCommand(client, "@fe BIN 1")
//
// 期待结果：
//   - 返回 ("A", nil)
//   - 连接未被关闭（server.Write 仍可成功）
func TestDAQT1603SendCommand_AcceptsAAsSuccess(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	serverDone := make(chan struct{})
	go func() {
		defer close(serverDone)
		for {
			_, err := readWithTimeout(server, testReadTimeout)
			if err != nil {
				return
			}
			_, _ = server.Write([]byte("A"))
		}
	}()

	device := NewDAQT1603(core.Profile{ID: "t1603-ack-a", Type: core.DeviceDaqT1603})
	resp, err := device.sendCommand(client, "@fe BIN 1")
	if err != nil {
		t.Fatalf("sendCommand returned error on 'A' response: %v", err)
	}
	if resp != "A" {
		t.Fatalf("resp = %q, want %q", resp, "A")
	}

	// 验证 conn 未被毒化：再发一条命令仍能成功
	if _, err := device.sendCommand(client, "@fe HEAD 0"); err != nil {
		t.Fatalf("second sendCommand failed: %v", err)
	}
}

// TestDAQT1603SendCommand_RejectsEResponseAsBusinessError 验证 sendCommand
// 收到单字节 'E' 时返回 ErrDeviceRejected 业务错误，**不**关闭连接。
//
// 业务背景（device-lab/skills/daq-t1603/SKILL.md:683、702）：
//   - 'E' 是设备发出的合法、完整的错误响应；
//   - 调用方应终止当前操作但连接边界仍可信，不应触发 ADR-009 毒化。
//
// 测试前置：
//   - net.Pipe 建立双向连接
//   - server goroutine 第 1 条命令回 'E'，第 2 条命令回 'A'
//
// 测试步骤：
//   - 调用 device.sendCommand(client, "@fe BIN 1")（收到 'E'）
//   - 再次调用 device.sendCommand(client, "@fe HEAD 0")（应能成功，证明连接未被关闭）
//
// 期待结果：
//   - 第 1 次返回错误且 errors.Is(err, protocol.ErrDeviceRejected) 为 true
//   - errors.Is(err, protocol.ErrWatchdogTriggered) 为 false（不应毒化）
//   - 第 2 次调用成功返回 "A"（连接仍可用）
func TestDAQT1603SendCommand_RejectsEResponseAsBusinessError(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	// 第 1 条回 'E'，第 2 条回 'A'，验证连接未被错误关闭
	responses := []byte{'E', 'A'}
	serverErr := make(chan error, 1)
	go func() {
		for _, r := range responses {
			if _, err := readWithTimeout(server, testReadTimeout); err != nil {
				serverErr <- fmt.Errorf("server read failed: %v", err)
				return
			}
			if _, err := server.Write([]byte{r}); err != nil {
				serverErr <- fmt.Errorf("server write failed: %v", err)
				return
			}
		}
		serverErr <- nil
	}()

	device := NewDAQT1603(core.Profile{ID: "t1603-ack-e", Type: core.DeviceDaqT1603})
	_, err := device.sendCommand(client, "@fe BIN 1")
	if err == nil {
		t.Fatal("sendCommand should return error on 'E' response")
	}
	if !errors.Is(err, protocol.ErrDeviceRejected) {
		t.Fatalf("error should wrap ErrDeviceRejected, got: %v", err)
	}
	if errors.Is(err, protocol.ErrWatchdogTriggered) {
		t.Fatalf("ErrDeviceRejected must NOT wrap ErrWatchdogTriggered (connection still valid), got: %v", err)
	}

	// 关键断言：连接仍可用，第 2 条命令应成功收到 'A'
	resp, err := device.sendCommand(client, "@fe HEAD 0")
	if err != nil {
		t.Fatalf("second sendCommand should succeed (connection must remain open after 'E'), got: %v", err)
	}
	if resp != "A" {
		t.Errorf("second sendCommand resp = %q, want %q", resp, "A")
	}

	select {
	case serr := <-serverErr:
		if serr != nil {
			t.Fatalf("server goroutine error: %v", serr)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("server goroutine did not complete within 2s")
	}
}

// TestDAQT1603SendCommand_RejectsInvalidACKAsProtocolError 验证 sendCommand
// 收到非 A/E 字节时返回协议错误。
//
// 业务背景：A/E 之外的字节意味着协议错位（如设备发回了数据帧前导字节），
// 调用方需根据上下文决定是否毒化连接。本测试只验证协议错误被正确识别。
//
// 测试前置：
//   - net.Pipe 建立双向连接
//   - server goroutine 读取命令后回写单字节 'X'（非法 ACK）
//
// 测试步骤：
//   - 调用 device.sendCommand(client, "@fe BIN 1")
//
// 期待结果：
//   - 返回错误，包含 "invalid ACK"
//   - 不属于 ErrDeviceRejected 也不属于 ErrWatchdogTriggered
func TestDAQT1603SendCommand_RejectsInvalidACKAsProtocolError(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	serverDone := make(chan struct{})
	go func() {
		defer close(serverDone)
		for {
			_, err := readWithTimeout(server, testReadTimeout)
			if err != nil {
				return
			}
			_, _ = server.Write([]byte("X"))
		}
	}()

	device := NewDAQT1603(core.Profile{ID: "t1603-ack-invalid", Type: core.DeviceDaqT1603})
	_, err := device.sendCommand(client, "@fe BIN 1")
	if err == nil {
		t.Fatal("sendCommand should return error on invalid ACK 'X'")
	}
	if !strings.Contains(err.Error(), "invalid ACK") {
		t.Fatalf("error should mention 'invalid ACK', got: %v", err)
	}
	if errors.Is(err, protocol.ErrDeviceRejected) {
		t.Errorf("invalid ACK should not be ErrDeviceRejected, got: %v", err)
	}
	if errors.Is(err, protocol.ErrWatchdogTriggered) {
		t.Errorf("invalid ACK should not be ErrWatchdogTriggered at sendCommand layer, got: %v", err)
	}
}

// TestDAQT1603SyncHardwareConfig_TempFirmwareFallsBackToASCII 验证
// temp 型号固件场景下 syncHardwareConfigLocked 的回退逻辑：
// 发送 @fe BIN 1 收到 'A' 后，重新查询 @fd BIN 读回 '0'，应回退 ASCII
// 而不是错误地把 FrameReader 切到二进制模式。
//
// 业务背景（device-lab/skills/daq-t1603/SKILL.md:494、992）：
//   - temp 固件对 @fe BIN 1 仍回 ACK 'A' 但实际不切换；
//   - 若仅依赖 ACK 判定成功，FrameReader 会按 64 字节二进制解析 ASCII 帧 → 帧错位。
//
// 测试前置：
//   - net.Pipe 建立双向连接
//   - server goroutine 按命令序列返回响应：
//     @e3 → 16 字符 + LF
//     @fd MCH → "FFFF"
//     @fd BIN (第 1 次，readAllConfig 阶段) → "1"
//     @fd TIME → "0"
//     @fd HEAD → "0"
//     @fd TYPE → "0"
//     @fd TRIG → "0"
//     @fe BIN 1 → "A"
//     @fd BIN (第 2 次，queryBinaryMode 验证) → "0"（**关键：temp 固件读回 0**）
//     @fe TIME 0 → "A"
//     @fe HEAD 0 → "A"
//
// 测试步骤：
//   - 调用 device.syncHardwareConfigLocked(client)
//
// 期待结果：
//   - 返回 nil（temp 固件回退属预期行为，不报错）
//   - device.config.BinaryFormat == false（回退 ASCII）
func TestDAQT1603SyncHardwareConfig_TempFirmwareFallsBackToASCII(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	// 命令 → 响应映射（按发送顺序）
	responses := []struct {
		command  string
		response string
	}{
		{"@e3", "KKKKKKKKKKKKKKKK\n"},
		{"@fd MCH", "FFFF"},
		{"@fd BIN", "1"}, // readAllConfig 第 1 次查询（temp 固件实际也可能返回 0，这里测典型路径）
		{"@fd TIME", "0"},
		{"@fd HEAD", "0"},
		{"@fd TYPE", "0"},
		{"@fd TRIG", "0"},
		{"@fe BIN 1", "A"},
		{"@fd BIN", "0"}, // queryBinaryMode 第 2 次查询，temp 固件读回 0 → 触发回退
		{"@fe TIME 0", "A"},
		{"@fe HEAD 0", "A"},
	}

	serverErr := make(chan error, 1)
	go func() {
		for i, item := range responses {
			cmd, err := readWithTimeout(server, testReadTimeout)
			if err != nil {
				serverErr <- fmt.Errorf("read %d-th command failed: %v (want %q)", i, err, item.command)
				return
			}
			if cmd != item.command {
				serverErr <- fmt.Errorf("command %d = %q, want %q", i, cmd, item.command)
				return
			}
			if _, err := server.Write([]byte(item.response)); err != nil {
				serverErr <- fmt.Errorf("write response %q failed: %v", item.response, err)
				return
			}
		}
		serverErr <- nil
	}()

	device := NewDAQT1603(core.Profile{ID: "t1603-temp", Type: core.DeviceDaqT1603})
	device.mu.Lock()
	device.conn = client
	device.frameReader = protocol.NewT1603FrameReader(client)
	device.mu.Unlock()

	if err := device.syncHardwareConfigLocked(client); err != nil {
		t.Fatalf("syncHardwareConfigLocked should succeed with ASCII fallback, got: %v", err)
	}

	device.mu.RLock()
	binaryFormat := device.config.BinaryFormat
	fr := device.frameReader
	device.mu.RUnlock()

	if binaryFormat {
		t.Errorf("temp firmware should fall back to ASCII (BinaryFormat=false), got BinaryFormat=true")
	}
	if fr != nil && fr.IsBinaryMode() {
		t.Errorf("FrameReader should be in ASCII mode after temp firmware fallback, still binary")
	}

	// 等待 server goroutine 完成
	select {
	case err := <-serverErr:
		if err != nil {
			t.Fatalf("server goroutine error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("server goroutine did not complete within 2s")
	}
}

// TestDAQT1603SyncHardwareConfig_BinVerifiedStaysBinary 验证正常固件路径：
// @fe BIN 1 后读回 @fd BIN = "1"，应保持二进制模式。
//
// 测试前置：与 TempFirmwareFallsBackToASCII 相同，但 queryBinaryMode 读回 "1"。
//
// 期待结果：
//   - 返回 nil
//   - device.config.BinaryFormat == true
func TestDAQT1603SyncHardwareConfig_BinVerifiedStaysBinary(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	responses := []struct {
		command  string
		response string
	}{
		{"@e3", "KKKKKKKKKKKKKKKK\n"},
		{"@fd MCH", "FFFF"},
		{"@fd BIN", "1"}, // readAllConfig 第 1 次
		{"@fd TIME", "0"},
		{"@fd HEAD", "0"},
		{"@fd TYPE", "0"},
		{"@fd TRIG", "0"},
		{"@fe BIN 1", "A"},
		{"@fd BIN", "1"}, // queryBinaryMode 第 2 次，读回 1 → 保持二进制
		{"@fe TIME 0", "A"},
		{"@fe HEAD 0", "A"},
	}

	serverErr := make(chan error, 1)
	go func() {
		for i, item := range responses {
			cmd, err := readWithTimeout(server, testReadTimeout)
			if err != nil {
				serverErr <- fmt.Errorf("read %d-th command failed: %v", i, err)
				return
			}
			if cmd != item.command {
				serverErr <- fmt.Errorf("command %d = %q, want %q", i, cmd, item.command)
				return
			}
			if _, err := server.Write([]byte(item.response)); err != nil {
				serverErr <- fmt.Errorf("write response %q failed: %v", item.response, err)
				return
			}
		}
		serverErr <- nil
	}()

	device := NewDAQT1603(core.Profile{ID: "t1603-normal", Type: core.DeviceDaqT1603})
	device.mu.Lock()
	device.conn = client
	device.frameReader = protocol.NewT1603FrameReader(client)
	device.mu.Unlock()

	if err := device.syncHardwareConfigLocked(client); err != nil {
		t.Fatalf("syncHardwareConfigLocked failed on normal firmware: %v", err)
	}

	device.mu.RLock()
	binaryFormat := device.config.BinaryFormat
	fr := device.frameReader
	device.mu.RUnlock()

	if !binaryFormat {
		t.Errorf("normal firmware should stay in binary mode (BinaryFormat=true), got false")
	}
	if fr != nil && !fr.IsBinaryMode() {
		t.Errorf("FrameReader should be in binary mode after BIN verified, still ASCII")
	}

	select {
	case err := <-serverErr:
		if err != nil {
			t.Fatalf("server goroutine error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("server goroutine did not complete within 2s")
	}
}

func TestDAQT1603SyncHardwareConfig_ReturnsWhenCallerHoldsDeviceLock(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	responses := []struct {
		command  string
		response string
	}{
		{"@e3", "KKKKKKKKKKKKKKKK\n"},
		{"@fd MCH", "FFFF"},
		{"@fd BIN", "1"},
		{"@fd TIME", "0"},
		{"@fd HEAD", "0"},
		{"@fd TYPE", "0"},
		{"@fd TRIG", "0"},
		{"@fe BIN 1", "A"},
		{"@fd BIN", "1"},
		{"@fe TIME 0", "A"},
		{"@fe HEAD 0", "A"},
	}

	serverErr := make(chan error, 1)
	go func() {
		for i, item := range responses {
			cmd, err := readWithTimeout(server, testReadTimeout)
			if err != nil {
				serverErr <- fmt.Errorf("read %d-th command failed: %v", i, err)
				return
			}
			if cmd != item.command {
				serverErr <- fmt.Errorf("command %d = %q, want %q", i, cmd, item.command)
				return
			}
			if _, err := server.Write([]byte(item.response)); err != nil {
				serverErr <- fmt.Errorf("write response %q failed: %v", item.response, err)
				return
			}
		}
		serverErr <- nil
	}()

	device := NewDAQT1603(core.Profile{ID: "t1603-connect-lock", Type: core.DeviceDaqT1603})
	device.conn = client
	device.frameReader = protocol.NewT1603FrameReader(client)

	done := make(chan error, 1)
	go func() {
		device.mu.Lock()
		defer device.mu.Unlock()
		done <- device.syncHardwareConfigLocked(client)
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("syncHardwareConfigLocked failed: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("syncHardwareConfigLocked deadlocked while caller held device mutex")
	}

	if err := <-serverErr; err != nil {
		t.Fatal(err)
	}
}

// TestDAQT1603QueryBinaryMode_RejectsInvalidResponse 验证 queryBinaryMode
// 对非 "0"/"1" 响应（如 E、X、空、多字节）返回协议错误，不再误判为 "0" 回退 ASCII。
//
// 业务背景（finding 9 复核修订 High 2）：
//   - 旧实现 strings.TrimSpace(resp) == "1" 把所有非 "1" 响应都当成合法 "0"，
//     E、X 等非法字节被掩盖为 temp 固件回退；
//   - 修订后严格校验只接受 "0" 或 "1"，其他响应返回协议错误中止同步。
//
// 测试前置：
//   - net.Pipe 建立双向连接
//   - server goroutine 对每条 @fd BIN 查询依次回 'X' / 'Y' / 'Z' / 'W'
//
// 注意：响应必须为单字节，因为 sendCommandExact 读取 1 字节；
// 多字节响应会残留字节污染下次读取。'E' 也不使用，因为虽然 queryBinaryMode
// 走 sendCommandExact 不做 A/E 校验，但为避免与 sendCommand 语义混淆统一用其他字节。
//
// 测试步骤：
//   - 依次调用 device.queryBinaryMode(client, deadline) 4 次
//
// 期待结果：
//   - 每次都返回错误，错误消息包含 "invalid response"
//   - 不返回 (false, nil)（即不被误判为 temp 回退）
func TestDAQT1603QueryBinaryMode_RejectsInvalidResponse(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	invalidResponses := []string{"X", "Y", "Z", "W"}
	serverErr := make(chan error, 1)
	go func() {
		for i, r := range invalidResponses {
			cmd, err := readWithTimeout(server, testReadTimeout)
			if err != nil {
				serverErr <- fmt.Errorf("read %d-th cmd failed: %v", i, err)
				return
			}
			if cmd != "@fd BIN" {
				serverErr <- fmt.Errorf("cmd %d = %q, want @fd BIN", i, cmd)
				return
			}
			if _, err := server.Write([]byte(r)); err != nil {
				serverErr <- fmt.Errorf("write %q failed: %v", r, err)
				return
			}
		}
		serverErr <- nil
	}()

	device := NewDAQT1603(core.Profile{ID: "t1603-qbm-invalid", Type: core.DeviceDaqT1603})
	deadline := time.Now().Add(30 * time.Second)

	for i, r := range invalidResponses {
		got, err := device.queryBinaryMode(client, deadline)
		if err == nil {
			t.Errorf("[%d] response %q: expected error, got (bool=%v, nil)", i, r, got)
			continue
		}
		if !strings.Contains(err.Error(), "invalid response") {
			t.Errorf("[%d] response %q: error = %v, want contains 'invalid response'", i, r, err)
		}
		// 关键断言：不能被误判为 (false, nil) 触发 temp 回退
		if got {
			t.Errorf("[%d] response %q: should not be interpreted as binary=true", i, r)
		}
	}

	select {
	case err := <-serverErr:
		if err != nil {
			t.Fatalf("server goroutine error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("server goroutine did not complete within 2s")
	}
}

// TestDAQT1603SyncHardwareConfig_BinVerifyFailureAbortsSync 验证 BIN 验证
// 失败（非 watchdog，如读回非法字节）时中止同步，不再假定 BIN=1 继续下发命令。
//
// 业务背景（finding 9 复核修订 Medium 3）：
//   - 旧实现对非 watchdog 错误记录 warn 后假定 BIN=1 继续同步，重新引入
//     "ACK 不代表生效"的同类风险；
//   - 修订后任何验证失败都中止同步，让调用方感知错误。
//
// 测试前置：
//   - net.Pipe 建立双向连接
//   - server goroutine 响应序列：
//     @e3 → 16 字符 + LF
//     @fd MCH/BIN/TIME/HEAD/TYPE/TRIG → 正常值
//     @fe BIN 1 → "A"
//     @fd BIN（验证查询）→ "X"（非法响应）
//
// 测试步骤：
//   - 调用 device.syncHardwareConfigLocked(client)
//
// 期待结果：
//   - 返回错误，包含 "verify BIN mode"
//   - 不应继续发送 @fe TIME / @fe HEAD（server goroutine 不会读到这两条命令）
func TestDAQT1603SyncHardwareConfig_BinVerifyFailureAbortsSync(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	// 同步前的查询 + 强制 BIN + 验证查询（返回非法 'X'）
	responses := []struct {
		command  string
		response string
	}{
		{"@e3", "KKKKKKKKKKKKKKKK\n"},
		{"@fd MCH", "FFFF"},
		{"@fd BIN", "1"},
		{"@fd TIME", "0"},
		{"@fd HEAD", "0"},
		{"@fd TYPE", "0"},
		{"@fd TRIG", "0"},
		{"@fe BIN 1", "A"},
		{"@fd BIN", "X"}, // 非法响应 → 中止同步
	}

	// serverErr 通道带缓冲：sync 中止后 client.Close 触发 server.Read 失败，
	// goroutine 写入错误但不阻塞主流程。
	serverErr := make(chan error, 1)
	go func() {
		for i, item := range responses {
			cmd, err := readWithTimeout(server, testReadTimeout)
			if err != nil {
				serverErr <- fmt.Errorf("read %d-th cmd failed: %v (want %q)", i, err, item.command)
				return
			}
			if cmd != item.command {
				serverErr <- fmt.Errorf("cmd %d = %q, want %q", i, cmd, item.command)
				return
			}
			if _, err := server.Write([]byte(item.response)); err != nil {
				serverErr <- fmt.Errorf("write %q failed: %v", item.response, err)
				return
			}
		}
		// 不应再读到 @fe TIME / @fe HEAD，若读到说明同步未中止
		// 等待主线程 Close client 触发 server.Read 失败后退出
		_, err := readWithTimeout(server, testReadTimeout)
		serverErr <- fmt.Errorf("expected no more commands after BIN verify failure, but got: %v", err)
	}()

	device := NewDAQT1603(core.Profile{ID: "t1603-binverify-abort", Type: core.DeviceDaqT1603})
	device.mu.Lock()
	device.conn = client
	device.frameReader = protocol.NewT1603FrameReader(client)
	device.mu.Unlock()

	err := device.syncHardwareConfigLocked(client)
	if err == nil {
		t.Fatal("syncHardwareConfigLocked should fail when BIN verify returns invalid response")
	}
	if !strings.Contains(err.Error(), "verify BIN mode") {
		t.Errorf("error = %v, want contains 'verify BIN mode'", err)
	}

	// 等待 server goroutine
	select {
	case serr := <-serverErr:
		// 预期 server 报"不应再读到命令"或 Read 失败
		if serr == nil {
			t.Error("server should report extra commands or read failure, got nil")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("server goroutine did not complete within 2s")
	}
}

// TestDAQT1603ApplyHardwareConfig_PartialFailureResyncsMode 验证 applyHardwareConfig
// 中途收到 'E' 拒绝后，resyncHardwareConfigMode 重新读取设备实际 BIN/TIME/HEAD
// 并同步到本地 cfg 与 FrameReader。
//
// 业务背景（finding 9 复核修订 High 1）：
//   - applyHardwareConfig 按顺序下发 @fe BIN/TIME/HEAD；
//   - 若 @fe BIN 0 成功（设备已切 ASCII）、@fe TIME 1 返回 'E'，
//     设备实际是 ASCII 但本地仍按旧 binary 解析；
//   - resync 后本地 cfg.BinaryFormat=false 与设备对齐。
//
// 测试前置：
//   - net.Pipe 建立双向连接
//   - device 初始 BinaryFormat=true（模拟旧 binary 模式）
//   - server goroutine 响应序列：
//     @fe BIN 0 → "A"（成功，设备已切 ASCII）
//     @fe TIME 1 → "E"（拒绝，applyHardwareConfig 返回错误）
//     @fd BIN → "0"（resync 读回 ASCII）
//     @fd TIME → "0"
//     @fd HEAD → "0"
//
// 测试步骤：
//   - 调用 device.ApplyDaqT1603Config(cfg) with BinaryFormat=false, ShowTimestamp=true
//
// 期待结果：
//   - 返回错误（包含原始 'E' 拒绝信息）
//   - device.config.BinaryFormat == false（resync 后与设备对齐）
//   - FrameReader.IsBinaryMode() == false（已同步）
func TestDAQT1603ApplyHardwareConfig_PartialFailureResyncsMode(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	responses := []struct {
		command  string
		response string
	}{
		{"@fe BIN 0", "A"},  // 第 1 条成功，设备切 ASCII
		{"@fe TIME 1", "E"}, // 第 2 条被拒绝
		{"@fd BIN", "0"},    // resync: BIN 实际为 0（ASCII）
		{"@fd TIME", "0"},   // resync: TIME 实际为 0
		{"@fd HEAD", "0"},   // resync: HEAD 实际为 0
	}

	serverErr := make(chan error, 1)
	go func() {
		for i, item := range responses {
			cmd, err := readWithTimeout(server, testReadTimeout)
			if err != nil {
				serverErr <- fmt.Errorf("read %d-th cmd failed: %v (want %q)", i, err, item.command)
				return
			}
			if cmd != item.command {
				serverErr <- fmt.Errorf("cmd %d = %q, want %q", i, cmd, item.command)
				return
			}
			if _, err := server.Write([]byte(item.response)); err != nil {
				serverErr <- fmt.Errorf("write %q failed: %v", item.response, err)
				return
			}
		}
		serverErr <- nil
	}()

	device := NewDAQT1603(core.Profile{ID: "t1603-resync", Type: core.DeviceDaqT1603})
	// 初始状态：本地按 binary 模式解析（与 resync 后的 ASCII 形成对比）
	device.mu.Lock()
	device.conn = client
	device.frameReader = protocol.NewT1603FrameReader(client)
	device.frameReader.SetBinaryMode(true)
	device.config.BinaryFormat = true
	device.status.Connection = core.ConnectionConnected
	device.mu.Unlock()

	// 目标配置：切 ASCII + 启用 TIME
	cfg := core.DaqT1603HardwareConfig{
		BinaryFormat:  false,
		ShowTimestamp: true,
		ShowSequence:  false,
	}
	err := device.ApplyDaqT1603Config(cfg)
	if err == nil {
		t.Fatal("ApplyDaqT1603Config should return error after @fe TIME 1 rejected with 'E'")
	}
	// 应包含原始 E 拒绝信息（resync 成功时返回原始错误）
	if !strings.Contains(err.Error(), "ErrDeviceRejected") && !strings.Contains(err.Error(), "rejected") {
		t.Errorf("error should mention device rejection, got: %v", err)
	}

	// 关键断言：resync 后本地与设备实际状态对齐
	device.mu.RLock()
	binaryFormat := device.config.BinaryFormat
	fr := device.frameReader
	device.mu.RUnlock()

	if binaryFormat {
		t.Errorf("after resync, BinaryFormat should be false (device actual=0), got true")
	}
	if fr != nil && fr.IsBinaryMode() {
		t.Errorf("after resync, FrameReader should be ASCII mode, still binary")
	}

	select {
	case serr := <-serverErr:
		if serr != nil {
			t.Fatalf("server goroutine error: %v", serr)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("server goroutine did not complete within 3s")
	}
}

// TestDAQT1603ApplyHardwareConfig_AllSuccessNoResync 验证 applyHardwareConfig
// 全部成功时不调用 resyncHardwareConfigMode，本地 cfg 直接生效。
//
// 测试前置：
//   - net.Pipe 建立双向连接
//   - server goroutine 响应 3 条 @fe 命令都回 "A"
//
// 期待结果：
//   - 返回 nil
//   - device.config 与传入 cfg 一致
//   - server 只读到 3 条命令（不应读到 @fd BIN 等 resync 查询）
func TestDAQT1603ApplyHardwareConfig_AllSuccessNoResync(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	responses := []struct {
		command  string
		response string
	}{
		{"@fe BIN 0", "A"},
		{"@fe TIME 0", "A"},
		{"@fe HEAD 0", "A"},
		{"@fe TYPE 0", "A"},
		{"@fe TRIG 0", "A"},
	}

	serverErr := make(chan error, 1)
	go func() {
		for i, item := range responses {
			cmd, err := readWithTimeout(server, testReadTimeout)
			if err != nil {
				serverErr <- fmt.Errorf("read %d-th cmd failed: %v", i, err)
				return
			}
			if cmd != item.command {
				serverErr <- fmt.Errorf("cmd %d = %q, want %q", i, cmd, item.command)
				return
			}
			if _, err := server.Write([]byte(item.response)); err != nil {
				serverErr <- fmt.Errorf("write %q failed: %v", item.response, err)
				return
			}
		}
		// 等待主线程 Close 后 Read 失败退出
		_, err := readWithTimeout(server, testReadTimeout)
		serverErr <- fmt.Errorf("expected no resync commands, but got: %v", err)
	}()

	device := NewDAQT1603(core.Profile{ID: "t1603-nosync", Type: core.DeviceDaqT1603})
	device.mu.Lock()
	device.conn = client
	device.frameReader = protocol.NewT1603FrameReader(client)
	device.status.Connection = core.ConnectionConnected
	device.mu.Unlock()

	// 默认配置：无热电偶类型、无采样率/平均/触发数，命令序列为 BIN/TIME/HEAD/TYPE/TRIG
	cfg := core.DaqT1603HardwareConfig{
		BinaryFormat:  false,
		ShowTimestamp: false,
		ShowSequence:  false,
	}
	if err := device.ApplyDaqT1603Config(cfg); err != nil {
		t.Fatalf("ApplyDaqT1603Config should succeed on all-A path, got: %v", err)
	}

	device.mu.RLock()
	binaryFormat := device.config.BinaryFormat
	showTimestamp := device.config.ShowTimestamp
	device.mu.RUnlock()

	if binaryFormat {
		t.Errorf("BinaryFormat should be false, got true")
	}
	if showTimestamp {
		t.Errorf("ShowTimestamp should be false, got true")
	}

	select {
	case serr := <-serverErr:
		// 预期 server 报"不应再读到命令"或 Read 失败
		if serr == nil {
			t.Error("server should report no-more-commands, got nil")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("server goroutine did not complete within 2s")
	}
}

// =================================================================
// Stop 边界严格化测试（2026-07-31 协议契约落地）
// -----------------------------------------------------------------
// 协议契约（docs/audits/2026-07-30-daq-t1603-acquisition-control-
// hardware-report.zh-CN.md 第11章）：
//   - Stop 响应 = N 个完整合法帧 + 单字节 'A' ACK
//   - ACK 是 Stop 事务终止边界，ACK 后无数据
//   - Stop 期间边界错乱（isResyncableReadError）必须立即终止并毒化连接，
//     不走 resync，避免掩盖问题导致下次 Start 才失败
//   - 正常采集期间边界错乱可 resync 重同步，保持采集连续性
// =================================================================

// makeValidT1603BinaryFrame 构造 64 字节合法 BIN 模式温度帧。
// 16 个 float32 little-endian，全部填合理温度（25°C），通过 ParseTCPFrameEx 校验。
func makeValidT1603BinaryFrame() []byte {
	frame := make([]byte, protocol.TCPFrameSize)
	for i := 0; i < 16; i++ {
		binary.LittleEndian.PutUint32(frame[i*4:], math.Float32bits(25.0))
	}
	return frame
}

// makeInvalidT1603BinaryFrame 构造 64 字节错帧。
// 所有通道均超出物理可能范围，使 ParseTCPFrameEx 校验失败，
// extractFixedFrameLocked 返回 "invalid frame at established 64-byte boundary"，
// 触发 isResyncableReadError 匹配。
func makeInvalidT1603BinaryFrame() []byte {
	frame := make([]byte, protocol.TCPFrameSize)
	for i := 0; i < 16; i++ {
		binary.LittleEndian.PutUint32(frame[i*4:], math.Float32bits(99999))
	}
	return frame
}

// TestDAQT1603StopInvalidatesConnOnResyncableFrameError 验证协议契约：
// Stop 事务期间遇到错帧（isResyncableReadError）必须立即终止并毒化连接，
// 不走 resync 重同步。
//
// 设计动机：Stop 期间 acquiring=false，readLoop 只消费尾帧维持边界。
// 若此时边界错乱仍走 resync，会掩盖协议错位问题，导致下次 Start 才失败，
// 增加诊断难度。符合 SKILL.md "固定边界解析失败时废弃连接"原则。
//
// 测试前置：
//   - net.Pipe 建立双向连接
//   - device.StartAcquisition() 已启动，readLoop 进入稳定采集状态
//   - server 在收到 @f1 后发送一个 64 字节错帧（不发送 ACK）
//
// 测试步骤：
//   - 调用 device.StopAcquisition()
//   - server 收到 @f1 后发送错帧
//
// 期待结果：
//   - StopAcquisition 返回错误（"reconnect required" 或 "lost connection"）
//   - device.conn 被置为 nil（毒化）
//   - device.status.Connection = ConnectionError
//   - 不应继续发送 @f1 重试或新 @f0 命令
func TestDAQT1603StopInvalidatesConnOnResyncableFrameError(t *testing.T) {
	setT1603StopTimeout(t, 500*time.Millisecond)
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	validFrame := makeValidT1603BinaryFrame()
	invalidFrame := makeInvalidT1603BinaryFrame()
	stopSeen := make(chan struct{})
	serverErr := make(chan error, 1)

	go func() {
		// 1. 读 @f0 启动命令，回 ACK + 合法帧
		cmd, err := readWithTimeout(server, testReadTimeout)
		if err != nil || cmd != "@f0 FFFF 2" {
			serverErr <- fmt.Errorf("start command = %q, err = %v", cmd, err)
			return
		}
		if _, err := server.Write(append([]byte{'A'}, validFrame...)); err != nil {
			serverErr <- err
			return
		}
		// 2. 读 @f1 停止命令
		cmd, err = readWithTimeout(server, testReadTimeout)
		if err != nil || cmd != "@f1" {
			serverErr <- fmt.Errorf("stop command = %q, err = %v", cmd, err)
			return
		}
		close(stopSeen)
		// 3. 故意发送错帧（不发送 ACK），触发 isResyncableReadError
		//    Stop 上下文应立即终止并毒化连接，不走 resync
		if _, err := server.Write(invalidFrame); err != nil {
			serverErr <- err
			return
		}
		// 等待 device 关闭连接后 Read 失败退出
		_, _ = readWithTimeout(server, testReadTimeout)
		serverErr <- nil
	}()

	device := NewDAQT1603(core.Profile{ID: "t1603-stop-resync-fail", Type: core.DeviceDaqT1603})
	device.conn = client
	device.frameReader = protocol.NewT1603FrameReader(client)
	device.frameReader.SetBinaryMode(true)
	device.status.Connection = core.ConnectionConnected
	device.config = core.DaqT1603HardwareConfig{ChannelMask: "FFFF", BinaryFormat: true}
	payloads := make(chan core.DataPayload, 4)
	device.SetDataSink(func(payload core.DataPayload) { payloads <- payload })
	defer device.Disconnect()

	if err := device.StartAcquisition(); err != nil {
		t.Fatalf("StartAcquisition returned error: %v", err)
	}
	// 等待 readLoop 收到首帧进入稳定状态
	select {
	case <-payloads:
	case <-time.After(testReadTimeout):
		t.Fatal("did not receive initial frame")
	}

	// 调用 Stop，server 收到 @f1 后发错帧，应触发毒化
	stopResult := make(chan error, 1)
	go func() { stopResult <- device.StopAcquisition() }()
	select {
	case <-stopSeen:
	case <-time.After(testReadTimeout):
		t.Fatal("device did not send @f1")
	}

	select {
	case err := <-stopResult:
		if err == nil {
			t.Fatal("StopAcquisition should return error on resyncable frame error during Stop")
		}
		if !strings.Contains(err.Error(), "reconnect required") && !strings.Contains(err.Error(), "lost connection") {
			t.Fatalf("StopAcquisition error = %v, want reconnect required or lost connection", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("StopAcquisition did not return within 2s; resync likely masked the error")
	}

	// 验证连接已被毒化
	device.mu.RLock()
	connAfter := device.conn
	statusAfter := device.status.Connection
	lastError := device.status.LastError
	device.mu.RUnlock()
	if connAfter != nil {
		t.Error("device.conn should be nil after resyncable frame error during Stop")
	}
	if statusAfter != core.ConnectionError {
		t.Errorf("status.Connection = %v, want Error", statusAfter)
	}
	if !strings.Contains(lastError, "invalid frame") && !strings.Contains(lastError, "Stop ACK") &&
		!strings.Contains(lastError, "invalid Stop response boundary") {
		t.Errorf("status.LastError = %q, want mention of invalid frame or Stop response boundary", lastError)
	}

	if err := <-serverErr; err != nil {
		t.Fatal(err)
	}
}

// TestDAQT1603ReadLoop_ResyncsOnFrameMisalignmentDuringAcquisition 验证协议契约：
// 正常采集期间（非 Stop 事务）遇到错帧（isResyncableReadError）应 Reset 缓冲并继续读取，
// 保持采集连续性，不毒化连接。
//
// 设计动机：采集期间偶发错帧（如 TCP 重传乱序、设备瞬时故障）可通过 resync 恢复，
// 避免一次错帧就强制重连，影响数据采集连续性。这是 resync 机制的合理使用场景。
//
// 测试前置：
//   - net.Pipe 建立双向连接
//   - device.StartAcquisition() 已启动，readLoop 进入稳定采集状态
//   - server 发送：合法帧 → 错帧 → 合法帧 → 合法帧
//
// 测试步骤：
//   - 启动采集，等待 readLoop 进入稳定状态
//   - server 注入错帧
//   - server 继续发送合法帧
//
// 期待结果：
//   - readLoop 收到错帧后 Reset 缓冲并继续读取（不毒化连接）
//   - 至少收到一帧合法帧（错帧后的合法帧），证明 resync 成功
//   - device.conn 仍为非 nil（未毒化）
//   - device.status.Connection 未变为 Error
func TestDAQT1603ReadLoop_ResyncsOnFrameMisalignmentDuringAcquisition(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	validFrame := makeValidT1603BinaryFrame()
	invalidFrame := makeInvalidT1603BinaryFrame()
	serverErr := make(chan error, 1)

	go func() {
		// 1. 读 @f0 启动命令，回 ACK
		cmd, err := readWithTimeout(server, testReadTimeout)
		if err != nil || cmd != "@f0 FFFF 2" {
			serverErr <- fmt.Errorf("start command = %q, err = %v", cmd, err)
			return
		}
		if _, err := server.Write([]byte{'A'}); err != nil {
			serverErr <- err
			return
		}
		// 2. 发送：合法帧 → 错帧 → 合法帧 → 合法帧
		//    错帧触发 isResyncableReadError，readLoop 应 Reset 并继续读
		frames := [][]byte{validFrame, invalidFrame, validFrame, validFrame}
		for i, f := range frames {
			if _, err := server.Write(f); err != nil {
				serverErr <- fmt.Errorf("write frame %d failed: %v", i, err)
				return
			}
			// 短暂间隔确保 readLoop 有时间处理每帧
			time.Sleep(20 * time.Millisecond)
		}
		// 等待 device 关闭连接后 Read 失败退出
		_, _ = readWithTimeout(server, testReadTimeout)
		serverErr <- nil
	}()

	device := NewDAQT1603(core.Profile{ID: "t1603-resync-acq", Type: core.DeviceDaqT1603})
	device.conn = client
	device.frameReader = protocol.NewT1603FrameReader(client)
	device.frameReader.SetBinaryMode(true)
	device.status.Connection = core.ConnectionConnected
	device.config = core.DaqT1603HardwareConfig{ChannelMask: "FFFF", BinaryFormat: true}
	payloads := make(chan core.DataPayload, 16)
	device.SetDataSink(func(payload core.DataPayload) { payloads <- payload })
	defer device.Disconnect()

	if err := device.StartAcquisition(); err != nil {
		t.Fatalf("StartAcquisition returned error: %v", err)
	}

	// 收集 3 帧合法 payload（首帧 + 错帧后 2 帧）
	// 错帧会被丢弃（resync），不会送 sink
	received := 0
	timeout := time.After(2 * time.Second)
	for received < 3 {
		select {
		case <-payloads:
			received++
		case <-timeout:
			t.Fatalf("only received %d valid payloads, expected at least 3 (resync may have failed)", received)
		}
	}

	// 验证连接未被毒化：resync 应保持连接可用
	device.mu.RLock()
	connAfter := device.conn
	statusAfter := device.status.Connection
	device.mu.RUnlock()
	if connAfter == nil {
		t.Error("device.conn should not be nil after resyncable frame error during acquisition")
	}
	if statusAfter == core.ConnectionError {
		t.Error("status.Connection should not be Error after successful resync")
	}

	if err := <-serverErr; err != nil {
		t.Fatal(err)
	}
}


