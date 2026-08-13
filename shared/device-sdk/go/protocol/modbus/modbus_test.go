package modbus

import (
	"encoding/binary"
	"errors"
	"io"
	"net"
	"os"
	"slices"
	"sync"
	"testing"
	"time"
)

// serveFakeServer 在 server 端循环：读一个请求 ADU，调 handler 得响应 PDU，
// 写回响应 ADU（回显请求 Transaction ID / Unit ID）。handler 返回 nil 表示
// 不响应（模拟超时）。连接关闭或帧错误时退出。
func serveFakeServer(server net.Conn, handler func(unitID uint8, pdu []byte) []byte) {
	for {
		header := make([]byte, mbapHeaderLen)
		if _, err := io.ReadFull(server, header); err != nil {
			return
		}
		length := int(binary.BigEndian.Uint16(header[4:6]))
		body := make([]byte, length-1)
		if _, err := io.ReadFull(server, body); err != nil {
			return
		}
		resp := handler(header[6], body)
		if resp == nil {
			continue
		}
		out := make([]byte, 0, mbapHeaderLen+len(resp))
		out = append(out, header[0:4]...) // 回显 Transaction ID + Protocol ID
		out = binary.BigEndian.AppendUint16(out, uint16(len(resp)+1))
		out = append(out, header[6])
		out = append(out, resp...)
		if _, err := server.Write(out); err != nil {
			return
		}
	}
}

// newPipePair 建立 net.Pipe 双向连接并启动 fake server goroutine。
func newPipePair(handler func(unitID uint8, pdu []byte) []byte) (client net.Conn, server net.Conn) {
	client, server = net.Pipe()
	go serveFakeServer(server, handler)
	return client, server
}

func TestReadHoldingRegistersRequestAndParse(t *testing.T) {
	var gotUnit uint8
	var gotPDU []byte
	client, server := newPipePair(func(unitID uint8, pdu []byte) []byte {
		gotUnit = unitID
		gotPDU = slices.Clone(pdu)
		resp := []byte{FuncReadHoldingRegisters, 16}
		for i := 0; i < 8; i++ {
			resp = binary.BigEndian.AppendUint16(resp, uint16(100+i))
		}
		return resp
	})
	defer server.Close()

	mb := NewConn(client)
	values, err := mb.ReadHoldingRegisters(1, 200, 8)
	if err != nil {
		t.Fatalf("ReadHoldingRegisters returned error: %v", err)
	}
	if gotUnit != 1 {
		t.Fatalf("unit id = %d, want 1", gotUnit)
	}
	wantPDU := []byte{0x03, 0x00, 0xC8, 0x00, 0x08}
	if !slices.Equal(gotPDU, wantPDU) {
		t.Fatalf("request PDU = % X, want % X", gotPDU, wantPDU)
	}
	for i, v := range values {
		if v != uint16(100+i) {
			t.Fatalf("values[%d] = %d, want %d", i, v, 100+i)
		}
	}
}

func TestReadInputRegistersBigEndianParse(t *testing.T) {
	client, server := newPipePair(func(unitID uint8, pdu []byte) []byte {
		resp := []byte{FuncReadInputRegisters, 16}
		for i := 0; i < 8; i++ {
			resp = binary.BigEndian.AppendUint16(resp, uint16(i*0x1111))
		}
		return resp
	})
	defer server.Close()

	mb := NewConn(client)
	values, err := mb.ReadInputRegisters(2, 0, 8)
	if err != nil {
		t.Fatalf("ReadInputRegisters returned error: %v", err)
	}
	for i, v := range values {
		if v != uint16(i*0x1111) {
			t.Fatalf("values[%d] = %#04x, want %#04x", i, v, i*0x1111)
		}
	}
}

func TestWriteSingleRegisterEcho(t *testing.T) {
	var gotPDU []byte
	client, server := newPipePair(func(unitID uint8, pdu []byte) []byte {
		gotPDU = slices.Clone(pdu)
		return pdu // FC6 正常响应原样回显请求 PDU
	})
	defer server.Close()

	mb := NewConn(client)
	if err := mb.WriteSingleRegister(1, 203, 5); err != nil {
		t.Fatalf("WriteSingleRegister returned error: %v", err)
	}
	wantPDU := []byte{0x06, 0x00, 0xCB, 0x00, 0x05}
	if !slices.Equal(gotPDU, wantPDU) {
		t.Fatalf("request PDU = % X, want % X", gotPDU, wantPDU)
	}
}

func TestWriteSingleRegisterEchoMismatchPoisonsConn(t *testing.T) {
	client, server := newPipePair(func(unitID uint8, pdu []byte) []byte {
		resp := slices.Clone(pdu)
		resp[4] ^= 0xFF // 篡改回显值
		return resp
	})
	defer server.Close()

	mb := NewConn(client)
	err := mb.WriteSingleRegister(1, 200, 1)
	if !errors.Is(err, ErrConnBroken) {
		t.Fatalf("WriteSingleRegister error = %v, want ErrConnBroken", err)
	}
	if !mb.Closed() {
		t.Fatal("conn should be poisoned after FC6 echo mismatch")
	}
}

// TestReadCountClientSideValidation 验证单次读上限（实测读 9+ 返回异常码 2）：
// count > MaxReadCount 在客户端被参数校验直接拒绝，作为固件异常码 2 的前置防线；
// 参数校验属客户端错误，不毒化连接。
func TestReadCountClientSideValidation(t *testing.T) {
	client, server := newPipePair(func(unitID uint8, pdu []byte) []byte {
		resp := []byte{pdu[0], 2, 0x00, 0x01}
		return resp
	})
	defer server.Close()

	mb := NewConn(client)
	if _, err := mb.ReadInputRegisters(1, 0, 9); err == nil {
		t.Fatal("read count 9 should be rejected by client-side validation")
	}
	if _, err := mb.ReadHoldingRegisters(1, 200, 0); err == nil {
		t.Fatal("read count 0 should be rejected by client-side validation")
	}
	if mb.Closed() {
		t.Fatal("client-side validation error must not poison conn")
	}
	// 校验失败后连接仍可正常收发。
	if _, err := mb.ReadInputRegisters(1, 0, 1); err != nil {
		t.Fatalf("read after validation error failed: %v", err)
	}
}

func TestExceptionErrorFromDevice(t *testing.T) {
	client, server := newPipePair(func(unitID uint8, pdu []byte) []byte {
		return []byte{pdu[0] | 0x80, 0x02} // 异常码 2：非法数据地址
	})
	defer server.Close()

	mb := NewConn(client)
	_, err := mb.ReadHoldingRegisters(1, 200, 8)
	var excErr *ExceptionError
	if !errors.As(err, &excErr) {
		t.Fatalf("error = %v, want *ExceptionError", err)
	}
	if excErr.Function != FuncReadHoldingRegisters || excErr.Code != 2 {
		t.Fatalf("exception = %+v, want FC 0x03 code 2", excErr)
	}
	// 异常属业务错误：连接必须保持可用。
	if mb.Closed() {
		t.Fatal("exception response must not poison conn")
	}
}

func TestTransactionIDMismatchTreatedAsTimeout(t *testing.T) {
	client, server := net.Pipe()
	defer server.Close()
	go func() {
		header := make([]byte, mbapHeaderLen)
		if _, err := io.ReadFull(server, header); err != nil {
			return
		}
		body := make([]byte, int(binary.BigEndian.Uint16(header[4:6]))-1)
		if _, err := io.ReadFull(server, body); err != nil {
			return
		}
		// 回显错误的 Transaction ID（串帧场景）
		header[0] ^= 0xFF
		resp := []byte{FuncReadInputRegisters, 2, 0x00, 0x2A}
		out := append(header[0:4], 0x00, byte(len(resp)+1), header[6])
		out = append(out, resp...)
		_, _ = server.Write(out)
	}()

	mb := NewConn(client)
	_, err := mb.ReadInputRegisters(1, 0, 1)
	if !errors.Is(err, ErrConnBroken) || !errors.Is(err, os.ErrDeadlineExceeded) {
		t.Fatalf("error = %v, want ErrConnBroken wrapping os.ErrDeadlineExceeded", err)
	}
	if !mb.Closed() {
		t.Fatal("transaction id mismatch must poison conn")
	}
	// 毒化后所有请求直接返回 ErrConnBroken，不再做 I/O。
	if _, err := mb.ReadInputRegisters(1, 0, 1); !errors.Is(err, ErrConnBroken) {
		t.Fatalf("post-poison error = %v, want ErrConnBroken", err)
	}
}

func TestResponseTimeoutPoisonsConn(t *testing.T) {
	// handler 返回 nil：fake server 收到请求但不响应。
	client, server := newPipePair(func(unitID uint8, pdu []byte) []byte { return nil })
	defer server.Close()

	mb := NewConn(client)
	mb.timeout = 100 * time.Millisecond
	started := time.Now()
	_, err := mb.ReadInputRegisters(1, 0, 8)
	if !errors.Is(err, ErrConnBroken) {
		t.Fatalf("error = %v, want ErrConnBroken", err)
	}
	if elapsed := time.Since(started); elapsed > 2*time.Second {
		t.Fatalf("timeout path took %v, want bounded by response timeout", elapsed)
	}
	if !mb.Closed() {
		t.Fatal("timeout must poison conn")
	}
}

func TestFragmentedResponseParsed(t *testing.T) {
	client, server := net.Pipe()
	defer server.Close()
	go func() {
		header := make([]byte, mbapHeaderLen)
		if _, err := io.ReadFull(server, header); err != nil {
			return
		}
		body := make([]byte, int(binary.BigEndian.Uint16(header[4:6]))-1)
		if _, err := io.ReadFull(server, body); err != nil {
			return
		}
		resp := []byte{FuncReadInputRegisters, 4, 0x00, 0x01, 0x00, 0x02}
		out := append(header[0:4], 0x00, byte(len(resp)+1), header[6])
		out = append(out, resp...)
		// 分两片写：先 3 字节（MBAP 不完整），再剩余部分，验证帧边界按 length 切割。
		if _, err := server.Write(out[:3]); err != nil {
			return
		}
		time.Sleep(10 * time.Millisecond)
		_, _ = server.Write(out[3:])
	}()

	mb := NewConn(client)
	values, err := mb.ReadInputRegisters(1, 0, 2)
	if err != nil {
		t.Fatalf("ReadInputRegisters returned error: %v", err)
	}
	if !slices.Equal(values, []uint16{1, 2}) {
		t.Fatalf("values = %v, want [1 2]", values)
	}
}

func TestTransactionIDIncrements(t *testing.T) {
	var txIDs []uint16
	client, server := net.Pipe()
	defer server.Close()
	go func() {
		for {
			header := make([]byte, mbapHeaderLen)
			if _, err := io.ReadFull(server, header); err != nil {
				return
			}
			body := make([]byte, int(binary.BigEndian.Uint16(header[4:6]))-1)
			if _, err := io.ReadFull(server, body); err != nil {
				return
			}
			txIDs = append(txIDs, binary.BigEndian.Uint16(header[0:2]))
			resp := []byte{body[0], 2, 0x00, 0x01}
			out := append(header[0:4], 0x00, byte(len(resp)+1), header[6])
			out = append(out, resp...)
			if _, err := server.Write(out); err != nil {
				return
			}
		}
	}()

	mb := NewConn(client)
	for i := 0; i < 3; i++ {
		if _, err := mb.ReadHoldingRegisters(1, 200, 1); err != nil {
			t.Fatalf("request %d returned error: %v", i, err)
		}
	}
	if !slices.Equal(txIDs, []uint16{1, 2, 3}) {
		t.Fatalf("transaction ids = %v, want [1 2 3]", txIDs)
	}
}

// stuckConn 模拟 ADR-009 最极端场景：deadline 失效、Read 永久阻塞，
// 且 Close 自身也卡死（安全软件 hook winsock，closesocket 等待未完成的
// 重叠读取），因此 Close 也无法解除 Read 阻塞。
type stuckConn struct{}

func (stuckConn) Read([]byte) (int, error)         { select {} }
func (stuckConn) Write(b []byte) (int, error)      { return len(b), nil }
func (stuckConn) Close() error                     { select {} }
func (stuckConn) LocalAddr() net.Addr              { return nil }
func (stuckConn) RemoteAddr() net.Addr             { return nil }
func (stuckConn) SetDeadline(time.Time) error      { return nil }
func (stuckConn) SetReadDeadline(time.Time) error  { return nil }
func (stuckConn) SetWriteDeadline(time.Time) error { return nil }

// TestCloseDoesNotBlockOnStuckTransaction 验证 ADR-009 极端场景回归：
// roundTrip 永久阻塞在 Read 上并持有事务锁 mu（deadline 失效 + watchdog 的
// closesocket 卡死），Close 必须不等待事务锁直接返回（closed 为 atomic 标记），
// 否则 StopAcquisition → invalidateConnection → mb.Close() 链路会永久挂死。
func TestCloseDoesNotBlockOnStuckTransaction(t *testing.T) {
	mb := NewConn(stuckConn{})
	mb.timeout = 50 * time.Millisecond

	// 启动一个事务：写成功后永久阻塞在 Read 上，全程持有事务锁。
	go func() { _, _ = mb.ReadInputRegisters(1, 0, 8) }()
	// 等待事务进入阻塞 Read，且 watchdog 已触发（closesocket 卡死无法解除阻塞）。
	time.Sleep(200 * time.Millisecond)

	done := make(chan struct{})
	go func() { _ = mb.Close(); close(done) }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Close blocked on transaction lock held by permanently stuck roundTrip")
	}
	if !mb.Closed() {
		t.Fatal("Close should mark conn closed")
	}
}

// deadlineIgnoringConn 忽略所有 SetDeadline（模拟 ADR-009 描述的故障 Windows
// 电脑：deadline 失效，Read 永久阻塞），仅 Close 后 Read 才返回错误。
// 用于验证 WatchdogClose 是唯一的硬取消机制。
type deadlineIgnoringConn struct {
	net.Conn
	closeOnce sync.Once
	closed    chan struct{}
}

func newDeadlineIgnoringConn(inner net.Conn) *deadlineIgnoringConn {
	return &deadlineIgnoringConn{Conn: inner, closed: make(chan struct{})}
}

func (c *deadlineIgnoringConn) SetDeadline(time.Time) error      { return nil }
func (c *deadlineIgnoringConn) SetReadDeadline(time.Time) error  { return nil }
func (c *deadlineIgnoringConn) SetWriteDeadline(time.Time) error { return nil }

func (c *deadlineIgnoringConn) Close() error {
	c.closeOnce.Do(func() { close(c.closed) })
	return c.Conn.Close()
}

// TestWatchdogCloseReleasesBlockedRead 验证 ADR-009 硬性要求：deadline 失效的
// 连接上，响应超时由独立 watchdog 直接 conn.Close() 解除阻塞 Read，
// 不依赖 SetDeadline 生效。
func TestWatchdogCloseReleasesBlockedRead(t *testing.T) {
	server, inner := net.Pipe()
	defer server.Close()
	ignored := newDeadlineIgnoringConn(inner)
	// fake server 收到请求但永不响应。
	go serveFakeServer(server, func(unitID uint8, pdu []byte) []byte { return nil })

	mb := NewConn(ignored)
	mb.timeout = 100 * time.Millisecond
	started := time.Now()
	_, err := mb.ReadInputRegisters(1, 0, 8)
	if !errors.Is(err, ErrConnBroken) {
		t.Fatalf("error = %v, want ErrConnBroken", err)
	}
	// deadline 失效时阻塞只能由 watchdog Close 解除：耗时应接近 timeout 而非卡死。
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("watchdog path took %v, want ~100ms", elapsed)
	}
	select {
	case <-ignored.closed:
	default:
		t.Fatal("watchdog should have closed the conn")
	}
}
