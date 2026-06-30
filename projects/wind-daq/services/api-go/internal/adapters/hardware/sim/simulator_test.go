package sim

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"

	"shared.local/device-sdk/go/protocol"
	"wind-daq/services/api-go/internal/core/device"
)

// seqLineProducer 生成 "<seq>\n" 文本帧，便于测试断言序号连续性与丢帧。
func seqLineProducer(seq int, channels int) ([]byte, error) {
	return []byte(fmt.Sprintf("%d\n", seq)), nil
}

// startSimWith 启动一个带指定配置的模拟器并返回，测试负责 Close。
func startSimWith(t *testing.T, producer FrameProducer, responder CommandResponder, autoStart bool, channels int) *TCPSimulator {
	t.Helper()
	sim := NewTCPSimulator(producer, responder, autoStart, channels)
	if err := sim.Start(context.Background()); err != nil {
		t.Fatalf("sim start: %v", err)
	}
	t.Cleanup(func() { _ = sim.Close() })
	return sim
}

// dialSim 连接模拟器并返回 bufio.Reader 便于按行读取。
// 调用 WaitClient 等待模拟器 Accept 完成，避免 Dial 后立即 InjectFrame
// 因 currentConn 尚未设置而失败（修复 flaky test）。
func dialSim(t *testing.T, sim *TCPSimulator) (net.Conn, *bufio.Reader) {
	t.Helper()
	conn, err := net.DialTimeout("tcp", sim.Addr(), 2*time.Second)
	if err != nil {
		t.Fatalf("dial sim: %v", err)
	}
	// TCP 三次握手成功不等于模拟器已 Accept：Accept 在独立 goroutine 里异步执行。
	// 这里同步等待 currentConn 被设置，保证后续 InjectFrame/DisconnectClient 能命中连接。
	if err := sim.WaitClient(2 * time.Second); err != nil {
		_ = conn.Close()
		t.Fatalf("sim did not accept connection: %v", err)
	}
	return conn, bufio.NewReader(conn)
}

// readLineInt 从 reader 读一行并解析为整数。
func readLineInt(t *testing.T, r *bufio.Reader) int {
	t.Helper()
	line, err := r.ReadBytes('\n')
	if err != nil {
		t.Fatalf("read line: %v", err)
	}
	n, err := strconv.Atoi(strings.TrimSpace(string(line)))
	if err != nil {
		t.Fatalf("parse int %q: %v", line, err)
	}
	return n
}

// ---------- TCPSimulator 框架测试 ----------

// TestTCPSimulator_StartAcceptClose 验证启动、接受连接、关闭。
func TestTCPSimulator_StartAcceptClose(t *testing.T) {
	sim := startSimWith(t, seqLineProducer, nil, true, 0)
	if sim.Addr() == "" {
		t.Fatal("empty addr after Start")
	}

	conn, r := dialSim(t, sim)
	defer conn.Close()
	// 能读到一帧说明 Accept 与 sendLoop 工作正常
	_ = readLineInt(t, r)

	if err := sim.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	// 关闭后客户端读取应返回 EOF
	conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
	_, err := r.ReadBytes('\n')
	if err == nil {
		t.Fatal("expected read error after close")
	}
}

// TestTCPSimulator_InjectFrame 验证注入一帧，客户端能收到。
func TestTCPSimulator_InjectFrame(t *testing.T) {
	sim := startSimWith(t, seqLineProducer, nil, false, 0) // autoStart=false 不自动发帧
	conn, r := dialSim(t, sim)
	defer conn.Close()

	payload := []byte("injected-frame\n")
	if err := sim.InjectFrame(payload); err != nil {
		t.Fatalf("inject: %v", err)
	}
	line, err := r.ReadBytes('\n')
	if err != nil {
		t.Fatalf("read injected: %v", err)
	}
	if !bytes.Equal(line, payload) {
		t.Fatalf("got %q want %q", line, payload)
	}
}

// TestTCPSimulator_InjectFrame_NoClient 验证无客户端时注入返回 ErrNoClient。
func TestTCPSimulator_InjectFrame_NoClient(t *testing.T) {
	sim := startSimWith(t, seqLineProducer, nil, false, 0)
	if err := sim.InjectFrame([]byte("x\n")); err != ErrNoClient {
		t.Fatalf("got %v want ErrNoClient", err)
	}
}

// TestTCPSimulator_DropNext 验证 DropNext 丢弃帧，客户端收到的序号出现跳跃。
func TestTCPSimulator_DropNext(t *testing.T) {
	sim := startSimWith(t, seqLineProducer, nil, true, 0)
	sim.SetLatency(2 * time.Millisecond)
	conn, r := dialSim(t, sim)
	defer conn.Close()

	// 预热读若干帧，建立连续基线
	for i := 0; i < 5; i++ {
		_ = readLineInt(t, r)
	}
	lastSeq := readLineInt(t, r)

	// 丢弃接下来若干帧
	const dropN = 10
	sim.DropNext(dropN)

	// 读下一帧：DropNext 会让序号跳跃（TCP buffer 可能有 1~2 帧已缓冲，
	// 所以只需断言发生了显著跳跃，而非精确等于 dropN）
	nextSeq := readLineInt(t, r)
	gap := nextSeq - lastSeq
	if gap < 2 {
		t.Fatalf("DropNext did not drop frames: last=%d next=%d gap=%d", lastSeq, nextSeq, gap)
	}
}

// TestTCPSimulator_SetLatency 验证延迟生效：大延迟下帧间隔明显大于小延迟。
func TestTCPSimulator_SetLatency(t *testing.T) {
	sim := startSimWith(t, seqLineProducer, nil, true, 0)
	conn, r := dialSim(t, sim)
	defer conn.Close()

	// 大延迟
	sim.SetLatency(80 * time.Millisecond)
	_ = readLineInt(t, r) // 预热
	start := time.Now()
	_ = readLineInt(t, r)
	_ = readLineInt(t, r)
	slow := time.Since(start)

	// 小延迟
	sim.SetLatency(2 * time.Millisecond)
	_ = readLineInt(t, r) // 预热
	start = time.Now()
	for i := 0; i < 5; i++ {
		_ = readLineInt(t, r)
	}
	fast := time.Since(start)

	// 慢速 2 帧应明显大于快速 5 帧的一半
	if slow < fast/2 {
		t.Fatalf("latency not effective: slow(2 frames)=%v fast(5 frames)=%v", slow, fast)
	}
}

// TestTCPSimulator_SetFailOnConnect 验证拒绝连接：客户端 Dial 后读取立即失败。
func TestTCPSimulator_SetFailOnConnect(t *testing.T) {
	sim := startSimWith(t, seqLineProducer, nil, true, 0)
	sim.SetFailOnConnect(true)

	conn, err := net.DialTimeout("tcp", sim.Addr(), 2*time.Second)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	// 连接被模拟器立即关闭，读取应失败
	conn.SetReadDeadline(time.Now().Add(1 * time.Second))
	_, err = conn.Read(make([]byte, 64))
	if err == nil {
		t.Fatal("expected read error (connection rejected)")
	}
}

// TestTCPSimulator_DisconnectClient 验证主动断开客户端：客户端读取返回 EOF。
func TestTCPSimulator_DisconnectClient(t *testing.T) {
	sim := startSimWith(t, seqLineProducer, nil, true, 0)
	conn, r := dialSim(t, sim)
	defer conn.Close()

	// 确认连接已建立并收到数据
	_ = readLineInt(t, r)

	// 模拟掉线
	sim.DisconnectClient()

	// 客户端读取应返回错误（EOF 或 reset）
	conn.SetReadDeadline(time.Now().Add(1 * time.Second))
	_, err := r.ReadBytes('\n')
	if err == nil {
		t.Fatal("expected read error after DisconnectClient")
	}
}

// TestTCPSimulator_MultiInstanceConcurrency 验证多个 Simulator 实例并发运行于不同端口。
// 对应 DeviceManager 多设备并发模型：不同设备不同端口，互不干扰。
func TestTCPSimulator_MultiInstanceConcurrency(t *testing.T) {
	const n = 4
	sims := make([]*TCPSimulator, 0, n)
	for i := 0; i < n; i++ {
		s := startSimWith(t, seqLineProducer, nil, true, 0)
		sims = append(sims, s)
	}

	// 每个模拟器应监听不同端口
	addrs := map[string]bool{}
	for _, s := range sims {
		if addrs[s.Addr()] {
			t.Fatalf("duplicate addr %s", s.Addr())
		}
		addrs[s.Addr()] = true
	}

	// 并发连接所有模拟器并各自读帧
	done := make(chan int, n)
	for _, s := range sims {
		s := s
		go func() {
			conn, err := net.DialTimeout("tcp", s.Addr(), 2*time.Second)
			if err != nil {
				done <- -1
				return
			}
			defer conn.Close()
			r := bufio.NewReader(conn)
			seq := readLineInt(t, r)
			done <- seq
		}()
	}
	for i := 0; i < n; i++ {
		if v := <-done; v < 0 {
			t.Fatalf("concurrent dial/read failed")
		}
	}
}

// ---------- Producer 测试（用真实 protocol 解析器验证帧有效性）----------

// TestP1604BinaryFrameProducer_ValidFrame 验证生成的帧能被 protocol.FrameReader
// 读取并由 ParseStreamFrame 解析为 18 个非零通道值。
func TestP1604BinaryFrameProducer_ValidFrame(t *testing.T) {
	frame, err := P1604BinaryFrameProducer(0, 18)
	if err != nil {
		t.Fatal(err)
	}
	// 长度前缀 = payload 长度 + 2
	if prefix := binary.BigEndian.Uint16(frame[:2]); int(prefix) != len(frame) {
		t.Fatalf("length prefix %d != frame len %d", prefix, len(frame))
	}

	client, server := net.Pipe()
	go func() { _, _ = client.Write(frame); _ = client.Close() }()
	reader := protocol.NewFrameReader(server)
	payload, err := reader.ReadFrame()
	_ = server.Close()
	if err != nil {
		t.Fatalf("FrameReader.ReadFrame: %v", err)
	}
	channels, err := protocol.ParseStreamFrame(payload)
	if err != nil {
		t.Fatalf("ParseStreamFrame: %v", err)
	}
	if len(channels) != 18 {
		t.Fatalf("channels=%d want 18", len(channels))
	}
	for i, v := range channels {
		if v == 0 {
			t.Fatalf("channel %d is zero", i)
		}
	}
}

func TestP1604BinaryFrameProducerWithDeviceTimestamp_ValidFrame(t *testing.T) {
	frame, err := P1604BinaryFrameProducerWithDeviceTimestamp(0, 18)
	if err != nil {
		t.Fatal(err)
	}
	client, server := net.Pipe()
	go func() { _, _ = client.Write(frame); _ = client.Close() }()
	reader := protocol.NewFrameReader(server)
	payload, err := reader.ReadFrame()
	_ = server.Close()
	if err != nil {
		t.Fatalf("FrameReader.ReadFrame: %v", err)
	}
	channels, deviceTimestampMs, err := protocol.ParseStreamFrameEx(payload, true, true)
	if err != nil {
		t.Fatalf("ParseStreamFrameEx: %v", err)
	}
	if len(channels) != 18 {
		t.Fatalf("channels=%d want 18", len(channels))
	}
	if deviceTimestampMs == 0 {
		t.Fatal("device timestamp is zero")
	}
}

// TestP1604ASCIIFrameProducer_ValidFrame 验证 ASCII 帧能被解析。
func TestP1604ASCIIFrameProducer_ValidFrame(t *testing.T) {
	frame, err := P1604ASCIIFrameProducer(0, 18)
	if err != nil {
		t.Fatal(err)
	}
	client, server := net.Pipe()
	go func() { _, _ = client.Write(frame); _ = client.Close() }()
	reader := protocol.NewFrameReader(server)
	payload, err := reader.ReadFrame()
	_ = server.Close()
	if err != nil {
		t.Fatalf("ReadFrame: %v", err)
	}
	// ASCII 帧为逗号分隔文本
	if !bytes.Contains(payload, []byte(",")) {
		t.Fatalf("not ascii frame: %q", payload)
	}
}

// TestT1603BinaryFrameProducer_ValidFrame 验证生成的 64 字节帧能被
// protocol.T1603FrameReader 读取并解析为 16 个合理温度值。
func TestT1603BinaryFrameProducer_ValidFrame(t *testing.T) {
	frame, err := T1603BinaryFrameProducer(0, 16)
	if err != nil {
		t.Fatal(err)
	}
	if len(frame) != 64 {
		t.Fatalf("frame len=%d want 64", len(frame))
	}

	client, server := net.Pipe()
	go func() { _, _ = client.Write(frame); _ = client.Close() }()
	reader := protocol.NewT1603FrameReader(server)
	// 切到 64 字节纯二进制模式（默认 192 字节 ASCII）
	reader.SetBinaryMode(true)
	read, err := reader.ReadFrame()
	_ = server.Close()
	if err != nil {
		t.Fatalf("T1603FrameReader.ReadFrame: %v", err)
	}
	channels, err := protocol.ParseTCPFrame(read)
	if err != nil {
		t.Fatalf("ParseTCPFrame: %v", err)
	}
	if len(channels) != 16 {
		t.Fatalf("channels=%d want 16", len(channels))
	}
	for i, v := range channels {
		if v < -200 || v > 1350 {
			t.Fatalf("channel %d temp %f out of range", i, v)
		}
	}
}

// TestP1064PreFrameProducer_ValidFrame 验证 0xA5 0x5A 头与校验和格式。
func TestP1064PreFrameProducer_ValidFrame(t *testing.T) {
	frame, err := P1064PreFrameProducer(0, 16)
	if err != nil {
		t.Fatal(err)
	}
	if frame[0] != 0xA5 || frame[1] != 0x5A {
		t.Fatalf("header %x %x want A5 5A", frame[0], frame[1])
	}
	if frame[2] != p1064preCmdAcquisition {
		t.Fatalf("cmd %x want %x", frame[2], p1064preCmdAcquisition)
	}
	// 校验和验证：累加和（头到 data 末尾）低 8 位
	dataLen := int(binary.BigEndian.Uint16(frame[3:5]))
	if dataLen != p1064preAcqDataLen {
		t.Fatalf("dataLen=%d want %d", dataLen, p1064preAcqDataLen)
	}
	var sum byte
	for i := 0; i < len(frame)-1; i++ {
		sum += frame[i]
	}
	if got := frame[len(frame)-1]; got != sum {
		t.Fatalf("checksum %x want %x", got, sum)
	}
}

// TestWTNPXIFrameProducer_ValidFrame 验证 4 字节大端长度前缀。
func TestWTNPXIFrameProducer_ValidFrame(t *testing.T) {
	frame, err := WTNPXIFrameProducer(0, 8)
	if err != nil {
		t.Fatal(err)
	}
	prefix := binary.BigEndian.Uint32(frame[:4])
	payloadLen := len(frame) - 4
	if int(prefix) != payloadLen {
		t.Fatalf("prefix %d != payload %d", prefix, payloadLen)
	}
	if payloadLen != 8*4 {
		t.Fatalf("payload len %d want 32", payloadLen)
	}
}

// TestDSA3217FrameProducer_ValidFrame 验证 ASCII 数据行格式。
func TestDSA3217FrameProducer_ValidFrame(t *testing.T) {
	frame, err := DSA3217FrameProducer(0, 16)
	if err != nil {
		t.Fatal(err)
	}
	if frame[len(frame)-1] != '\n' {
		t.Fatalf("frame not newline-terminated: %q", frame)
	}
	fields := strings.Fields(strings.TrimRight(string(frame), "\n"))
	if len(fields) != 16 {
		t.Fatalf("fields=%d want 16", len(fields))
	}
}

// ---------- Responder 测试 ----------

// TestP1604Responder_Commands 验证 P1604 命令识别。
func TestP1604Responder_Commands(t *testing.T) {
	r := NewP1604Responder()

	cases := []struct {
		cmd         string
		startStream bool
		stopStream  bool
	}{
		{"c 01 1\r\n", true, false},
		{"c 02 1\r\n", false, true},
		{"w1601\r\n", false, false},
		{"c 00 1 FFFF 1 100 7 0\r\n", false, false},
		{"c 05 1 0810\r\n", false, false},
	}
	for _, c := range cases {
		resp, err := r.HandleCommand([]byte(c.cmd))
		if err != nil {
			t.Fatalf("HandleCommand %q: %v", c.cmd, err)
		}
		if resp.StartStream != c.startStream {
			t.Fatalf("cmd %q startStream=%v want %v", c.cmd, resp.StartStream, c.startStream)
		}
		if resp.StopStream != c.stopStream {
			t.Fatalf("cmd %q stopStream=%v want %v", c.cmd, resp.StopStream, c.stopStream)
		}
	}
	if r.ReadMode() != ReadModeLine {
		t.Fatal("P1604 ReadMode want line")
	}
}

// TestDSA3217Responder_Commands 验证 DSA3217 SCPI 命令响应与配置读写。
func TestDSA3217Responder_Commands(t *testing.T) {
	r := NewDSA3217Responder()

	// SCAN 开始
	resp, _ := r.HandleCommand([]byte("SCAN\r\n"))
	if !resp.StartStream || string(resp.Data) != "OK\r\n" {
		t.Fatalf("SCAN resp %+v", resp)
	}
	// STOP 停止
	resp, _ = r.HandleCommand([]byte("STOP\r\n"))
	if !resp.StopStream {
		t.Fatalf("STOP resp %+v", resp)
	}
	// SET AVG 5
	resp, _ = r.HandleCommand([]byte("SET AVG 5\r\n"))
	if string(resp.Data) != "OK\r\n" {
		t.Fatalf("SET AVG resp %q", resp.Data)
	}
	// LIST S 应反映 AVG=5
	resp, _ = r.HandleCommand([]byte("LIST S\r\n"))
	if !bytes.Contains(resp.Data, []byte("SET AVG 5")) {
		t.Fatalf("LIST S missing AVG=5: %q", resp.Data)
	}
}

// TestT1603Responder_Commands 验证 T1603 SCPI 命令响应。
func TestT1603Responder_Commands(t *testing.T) {
	r := NewT1603Responder()
	if r.ReadMode() != ReadModeIdle {
		t.Fatal("T1603 ReadMode want idle")
	}

	// @e3 返回 16 字节热电偶类型
	resp, _ := r.HandleCommand([]byte("@e3"))
	if len(resp.Data) != 16 {
		t.Fatalf("@e3 len=%d want 16", len(resp.Data))
	}
	// @fd MCH 返回 FFFF
	resp, _ = r.HandleCommand([]byte("@fd MCH"))
	if !bytes.Contains(resp.Data, []byte("FFFF")) {
		t.Fatalf("@fd MCH resp %q", resp.Data)
	}
	// @fd BIN 返回 1 字节 "1"
	resp, _ = r.HandleCommand([]byte("@fd BIN"))
	if len(resp.Data) != 1 || resp.Data[0] != '1' {
		t.Fatalf("@fd BIN resp %q", resp.Data)
	}
	// @f0 开始采集
	resp, _ = r.HandleCommand([]byte("@f0 FFFF 2"))
	if !resp.StartStream || !bytes.HasPrefix(resp.Data, []byte("A")) {
		t.Fatalf("@f0 resp %+v", resp)
	}
	// @f1 停止采集
	resp, _ = r.HandleCommand([]byte("@f1"))
	if !resp.StopStream {
		t.Fatalf("@f1 resp %+v", resp)
	}
}

// ---------- Wiring 测试 ----------

// TestStartSimulatorForDeviceType 验证各设备类型能启动模拟器并接受连接。
func TestStartSimulatorForDeviceType(t *testing.T) {
	types := []device.Type{
		device.DeviceDAQP1604,
		device.DeviceDaqT1603,
		device.DeviceDSA3217,
		device.DeviceDAQP1064Pre,
		device.DeviceWTNPXI,
	}
	for _, dt := range types {
		dt := dt
		t.Run(string(dt), func(t *testing.T) {
			sim, err := StartSimulatorForDeviceType(dt, 0)
			if err != nil {
				t.Fatalf("start %s: %v", dt, err)
			}
			defer sim.Close()
			if sim.Addr() == "" {
				t.Fatal("empty addr")
			}
			// 能连接即说明监听正常
			conn, err := net.DialTimeout("tcp", sim.Addr(), 2*time.Second)
			if err != nil {
				t.Fatalf("dial %s: %v", dt, err)
			}
			conn.Close()
		})
	}
}

// TestWithSimulatedDevice 验证 wiring helper 构造的 profile 指向模拟器地址。
func TestWithSimulatedDevice(t *testing.T) {
	WithSimulatedDevice(t, device.DeviceDAQP1604, func(sim Simulator, profile device.Profile) {
		if profile.Address == "" || profile.Port == 0 {
			t.Fatalf("profile not populated: %+v", profile)
		}
		if profile.Type != device.DeviceDAQP1604 {
			t.Fatalf("type=%s", profile.Type)
		}
		// profile 地址应等于模拟器地址
		host, port := SplitAddr(sim.Addr())
		if profile.Address != host || profile.Port != port {
			t.Fatalf("profile %s:%d != sim %s:%d", profile.Address, profile.Port, host, port)
		}
	})
}

// TestStartSimulatorForDeviceType_Unsupported 验证不支持的类型返回错误。
func TestStartSimulatorForDeviceType_Unsupported(t *testing.T) {
	_, err := StartSimulatorForDeviceType(device.DeviceSimulated, 0)
	if err == nil {
		t.Fatal("expected error for SIMULATED type")
	}
}

// 确保 io 包被引用（部分测试路径可能用到）
var _ = io.EOF
