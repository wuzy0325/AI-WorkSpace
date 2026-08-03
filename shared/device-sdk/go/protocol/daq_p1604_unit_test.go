package protocol

import (
	"encoding/binary"
	"errors"
	"math"
	"net"
	"strings"
	"sync"
	"testing"
	"time"
)

// writeFrame 写入一个带 2 字节大端长度前缀的帧（模拟设备响应）
//
// 注意：参数 t 保留是为了向后兼容调用方签名，但函数内部不访问 t。
// 历史上 writeFrame 在子 goroutine 中调用 t.Helper/t.Logf 会与父 goroutine
// 退出产生 race（ADR-009 测试要求 -race 全绿）。Write 失败在 client 关闭后
// 是预期行为，静默忽略即可，调用方通过后续断言验证通信结果。
func writeFrame(_ *testing.T, conn net.Conn, payload string) {
	frameLen := uint16(len(payload) + 2)
	buf := make([]byte, 2, int(frameLen))
	binary.BigEndian.PutUint16(buf, frameLen)
	buf = append(buf, []byte(payload)...)
	_, _ = conn.Write(buf) // 静默：client 关闭后 Write 失败是预期
}

func TestP1604MatchUnitByCoefficient_KnownUnits(t *testing.T) {
	cases := []struct {
		unit string
	}{
		{"psi"}, {"Pa"}, {"kPa"}, {"MPa"}, {"kgf/cm²"},
	}
	for _, c := range cases {
		coeff := P1604PressureUnitCoefficient[c.unit]
		got, ok := P1604MatchUnitByCoefficient(coeff)
		if !ok {
			t.Errorf("unit %s (coeff=%v): expected match, got not found", c.unit, coeff)
			continue
		}
		if got != c.unit {
			t.Errorf("unit %s (coeff=%v): expected %q, got %q", c.unit, coeff, c.unit, got)
		}
	}
}

// TestP1604MatchUnitByCoefficient_Float32Precision 验证设备以 float32 存储系数带来的精度损失
// 仍能正确匹配。例如 Pa 系数 6894.757 在 float32 中为 6894.756836。
func TestP1604MatchUnitByCoefficient_Float32Precision(t *testing.T) {
	cases := []struct {
		name     string
		coeff    float64
		expected string
	}{
		{"Pa float32 loss", 6894.756836, "Pa"},
		{"psi exact", 1.000000, "psi"},
		{"kPa float32 loss", 6.894757, "kPa"},
	}
	for _, c := range cases {
		got, ok := P1604MatchUnitByCoefficient(c.coeff)
		if !ok {
			t.Errorf("%s (coeff=%v): expected match, got not found", c.name, c.coeff)
			continue
		}
		if got != c.expected {
			t.Errorf("%s (coeff=%v): expected %q, got %q", c.name, c.coeff, c.expected, got)
		}
	}
}

func TestP1604MatchUnitByCoefficient_UnknownCoeff(t *testing.T) {
	got, ok := P1604MatchUnitByCoefficient(12345.6789)
	if ok {
		t.Errorf("expected no match for unknown coeff, got %q", got)
	}
}

func TestP1604IsSupportedUnit(t *testing.T) {
	if !P1604IsSupportedUnit("psi") {
		t.Error("psi should be supported")
	}
	if !P1604IsSupportedUnit("kgf/cm²") {
		t.Error("kgf/cm² should be supported")
	}
	if P1604IsSupportedUnit("bar") {
		t.Error("bar should not be supported")
	}
	if P1604IsSupportedUnit("") {
		t.Error("empty string should not be supported")
	}
}

func TestP1604ReadUnitCoefficient_Valid(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	// 模拟设备端：读取 u01101 命令，返回系数 6.894757（kPa）
	go func() {
		// 先读客户端发来的 u01101 命令（纯 ASCII，无换行符）
		buf := make([]byte, 64)
		_, _ = server.Read(buf)
		// 返回 kPa 系数
		writeFrame(t, server, "6.894757")
	}()

	fr := NewFrameReader(client)
	coeff, err := P1604ReadUnitCoefficient(fr, client, time.Second)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if coeff != 6.894757 {
		t.Errorf("expected 6.894757, got %v", coeff)
	}
}

func TestP1604ReadUnitCoefficient_ZeroTimeoutDoesNotSetDeadline(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	tracked := &deadlineTrackingConn{Conn: client}
	go func() {
		buf := make([]byte, 64)
		_, _ = server.Read(buf)
		writeFrame(t, server, "1.000000")
	}()

	coeff, err := P1604ReadUnitCoefficient(NewFrameReader(tracked), tracked, 0)
	if err != nil {
		t.Fatalf("read coefficient without deadline: %v", err)
	}
	if coeff != 1 {
		t.Fatalf("coefficient = %v, want 1", coeff)
	}
	if tracked.readDeadlineCalls != 0 || tracked.writeDeadlineCalls != 0 {
		t.Fatalf("deadline calls: read=%d write=%d, want 0", tracked.readDeadlineCalls, tracked.writeDeadlineCalls)
	}
}

func TestP1604ReadUnitCoefficient_RejectsMultiValueResponse(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	go func() {
		buf := make([]byte, 64)
		_, _ = server.Read(buf)
		writeFrame(t, server, "6894.756836 0.000000 0.000000 0.000000 0.000000")
	}()

	fr := NewFrameReader(client)
	if _, err := P1604ReadUnitCoefficient(fr, client, time.Second); err == nil {
		t.Fatal("expected error for multi-value response")
	}
}

func TestP1604ReadUnitCoefficient_DeviceError(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	go func() {
		buf := make([]byte, 64)
		_, _ = server.Read(buf)
		// 设备返回 N05 错误
		writeFrame(t, server, "N05")
	}()

	fr := NewFrameReader(client)
	_, err := P1604ReadUnitCoefficient(fr, client, time.Second)
	if err == nil {
		t.Fatal("expected error for device N05 response")
	}
	if !strings.Contains(err.Error(), "N05") {
		t.Errorf("error should mention N05, got: %v", err)
	}
}

func TestP1604ReadUnitCoefficient_InvalidNumber(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	go func() {
		buf := make([]byte, 64)
		_, _ = server.Read(buf)
		writeFrame(t, server, "abc-not-a-number")
	}()

	fr := NewFrameReader(client)
	_, err := P1604ReadUnitCoefficient(fr, client, time.Second)
	if err == nil {
		t.Fatal("expected error for non-numeric response")
	}
}

func TestP1604ReadUnitCoefficient_NilArgs(t *testing.T) {
	_, err := P1604ReadUnitCoefficient(nil, nil, time.Second)
	if err == nil {
		t.Fatal("expected error for nil reader/conn")
	}
}

func TestP1604WriteUnitCoefficient_Success(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	go func() {
		buf := make([]byte, 64)
		_, _ = server.Read(buf)
		// 设备返回 A 表示成功
		writeFrame(t, server, "A")
	}()

	fr := NewFrameReader(client)
	if err := P1604WriteUnitCoefficient(fr, client, 6894.757, time.Second); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestP1604WriteUnitCoefficient_DeviceReject(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	go func() {
		buf := make([]byte, 64)
		_, _ = server.Read(buf)
		writeFrame(t, server, "N07")
	}()

	fr := NewFrameReader(client)
	err := P1604WriteUnitCoefficient(fr, client, 6894.757, time.Second)
	if err == nil {
		t.Fatal("expected error for device N07 rejection")
	}
	if !strings.Contains(err.Error(), "N07") {
		t.Errorf("error should mention N07, got: %v", err)
	}
}

func TestP1604WriteUnitCoefficient_RejectsUnexpectedResponse(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	go func() {
		buf := make([]byte, 64)
		_, _ = server.Read(buf)
		writeFrame(t, server, "A ")
	}()

	err := P1604WriteUnitCoefficient(NewFrameReader(client), client, 6894.757, time.Second)
	if err == nil || !strings.Contains(err.Error(), "unexpected v01101 response") {
		t.Fatalf("expected strict response error, got %v", err)
	}
}

func TestP1604WriteUnitCoefficient_InvalidCoeff(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	fr := NewFrameReader(client)
	// 负数、零、NaN、+Inf 均应被拒绝
	cases := []float64{-1.0, 0.0, math.NaN(), math.Inf(1)}
	for _, c := range cases {
		err := P1604WriteUnitCoefficient(fr, client, c, time.Second)
		if err == nil {
			t.Errorf("expected error for invalid coeff %v", c)
		}
	}
}

// TestP1604ReadUnitCoefficient_WatchdogTriggersOnDeadlineIgnoringConn 验证 ADR-009
// watchdog 兜底：当 SetReadDeadline 失效（Read 无限阻塞）时，watchdog 在 timeout 后
// 强制 Close conn，解除阻塞并返回 "watchdog triggered" 错误。
//
// 修复前 bug：P1604ReadUnitCoefficient 仅依赖 SetReadDeadline 做 Read 超时控制，
// 在故障 Windows 电脑上 deadline 失效时 Read 永久阻塞，函数永不返回。运行期切换
// 硬件压力单位（SetUnit）时调用本函数，会卡死整个驱动 → 升级为 P0。
//
// 测试前置：
//   - net.Pipe 建立双向连接
//   - 包装 client 为 deadlineIgnoringConn（SetReadDeadline 被 no-op）
//   - server 读取 u01101 命令后不写任何响应（确保 client.Read 阻塞）
//
// 测试步骤：
//   - 调用 P1604ReadUnitCoefficient，timeout=100ms（足够短快速触发 watchdog）
//
// 期待结果：
//   - 函数在 5s 预算内返回（watchdog 兜底解除阻塞）
//   - 错误信息包含 "watchdog triggered"
//   - conn 已被 watchdog Close（server.Write 失败）
func TestP1604ReadUnitCoefficient_WatchdogTriggersOnDeadlineIgnoringConn(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	// client 由 watchdog Close，不在 defer 中重复 Close

	ignored := newDeadlineIgnoringConn(client)

	// server 读取 u01101 命令后退出，不写响应让 client.Read 阻塞
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		buf := make([]byte, 64)
		_, _ = server.Read(buf)
		// 故意不写响应，触发 client.Read 永久阻塞
	}()

	done := make(chan error, 1)
	go func() {
		_, err := P1604ReadUnitCoefficient(NewFrameReader(ignored), ignored, 100*time.Millisecond)
		done <- err
	}()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected watchdog-triggered error, got nil")
		}
		if !strings.Contains(err.Error(), "watchdog triggered") {
			t.Errorf("error should mention 'watchdog triggered', got: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("P1604ReadUnitCoefficient did not return within 5s budget; watchdog likely not armed")
	}

	// 验证 conn 已被 watchdog Close：server.Write 应失败
	_ = server.SetWriteDeadline(time.Now().Add(200 * time.Millisecond))
	if _, writeErr := server.Write([]byte("probe")); writeErr == nil {
		t.Error("expected server.Write to fail after client was closed by watchdog")
	}
	wg.Wait()
}

// TestP1604ReadUnitCoefficient_ClearsDeadlineOnSuccess 验证成功路径清 deadline：
// watchdog 未触发时，函数退出前清掉 SetReadDeadline / SetWriteDeadline 残留，
// 保持 conn 可复用语义（ADR-009 决策 3）。
//
// 测试前置：
//   - 包装 client 为 deadlineTrackingConn（传播 deadline + 跟踪最后一次调用值）
//   - server 读取 u01101 后返回 "1.000000"
//
// 期待结果：
//   - 函数返回 coeff=1, err=nil
//   - 最后一次 SetReadDeadline 调用值为 time.Time{}（已清除）
//   - 最后一次 SetWriteDeadline 调用值为 time.Time{}（已清除）
func TestP1604ReadUnitCoefficient_ClearsDeadlineOnSuccess(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	tracked := &deadlineTrackingConn{Conn: client}

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		buf := make([]byte, 64)
		_, _ = server.Read(buf)
		writeFrame(t, server, "1.000000")
	}()

	coeff, err := P1604ReadUnitCoefficient(NewFrameReader(tracked), tracked, time.Second)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if coeff != 1 {
		t.Fatalf("coefficient = %v, want 1", coeff)
	}

	if last := tracked.lastReadDeadlineValue(); !last.IsZero() {
		t.Errorf("expected last SetReadDeadline to be zero (cleared) on success, got %v", last)
	}
	if last := tracked.lastWriteDeadlineValue(); !last.IsZero() {
		t.Errorf("expected last SetWriteDeadline to be zero (cleared) on success, got %v", last)
	}
	wg.Wait()
}

// TestP1604WriteUnitCoefficient_WatchdogTriggersOnDeadlineIgnoringConn 验证 ADR-009
// watchdog 兜底：当 SetReadDeadline 失效（Read 无限阻塞）时，watchdog 在 timeout 后
// 强制 Close conn，解除阻塞并返回 "watchdog triggered" 错误。
//
// 修复前 bug：P1604WriteUnitCoefficient 设 SetReadDeadline 后无 defer 清除，
// 失败路径 deadline 残留；且无 watchdog，deadline 失效时永久阻塞。
//
// 测试前置：
//   - 包装 client 为 deadlineIgnoringConn（SetReadDeadline 被 no-op）
//   - server 读取 v01101 命令后不写任何响应（确保 client.Read 阻塞）
//
// 测试步骤：
//   - 调用 P1604WriteUnitCoefficient，timeout=100ms
//
// 期待结果：
//   - 函数在 5s 预算内返回
//   - 错误信息包含 "watchdog triggered"
//   - conn 已被 watchdog Close（server.Write 失败）
func TestP1604WriteUnitCoefficient_WatchdogTriggersOnDeadlineIgnoringConn(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	// client 由 watchdog Close

	ignored := newDeadlineIgnoringConn(client)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		buf := make([]byte, 64)
		_, _ = server.Read(buf) // 读取 v01101 命令
		// 故意不写响应
	}()

	done := make(chan error, 1)
	go func() {
		err := P1604WriteUnitCoefficient(NewFrameReader(ignored), ignored, 6894.757, 100*time.Millisecond)
		done <- err
	}()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected watchdog-triggered error, got nil")
		}
		if !strings.Contains(err.Error(), "watchdog triggered") {
			t.Errorf("error should mention 'watchdog triggered', got: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("P1604WriteUnitCoefficient did not return within 5s budget; watchdog likely not armed")
	}

	_ = server.SetWriteDeadline(time.Now().Add(200 * time.Millisecond))
	if _, writeErr := server.Write([]byte("probe")); writeErr == nil {
		t.Error("expected server.Write to fail after client was closed by watchdog")
	}
	wg.Wait()
}

// TestP1604WriteUnitCoefficient_ClearsDeadlineOnSuccess 验证成功路径清 deadline：
// watchdog 未触发时，函数退出前清掉 SetReadDeadline / SetWriteDeadline 残留。
//
// 修复前 bug：P1604WriteUnitCoefficient 设 deadline 后无 defer 清除，
// 即使成功返回也会残留 deadline 影响后续命令。
//
// 期待结果：
//   - 函数返回 nil
//   - 最后一次 SetReadDeadline / SetWriteDeadline 调用值为 time.Time{}（已清除）
func TestP1604WriteUnitCoefficient_ClearsDeadlineOnSuccess(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	tracked := &deadlineTrackingConn{Conn: client}

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		buf := make([]byte, 64)
		_, _ = server.Read(buf)
		writeFrame(t, server, "A")
	}()

	if err := P1604WriteUnitCoefficient(NewFrameReader(tracked), tracked, 6894.757, time.Second); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if last := tracked.lastReadDeadlineValue(); !last.IsZero() {
		t.Errorf("expected last SetReadDeadline to be zero (cleared) on success, got %v", last)
	}
	if last := tracked.lastWriteDeadlineValue(); !last.IsZero() {
		t.Errorf("expected last SetWriteDeadline to be zero (cleared) on success, got %v", last)
	}
	wg.Wait()
}

// =================================================================
// ADR-009 R0-12 + R1-1：soft timeout 毒化路径测试
// =================================================================
//
// 测试目标：当 SetReadDeadline 兑现（soft timeout 先于 watchdog 返回）时，
// helper 必须强制 Close conn 防止迟到响应被下一条命令消费，并返回
// ErrWatchdogTriggered sentinel 让调用方毒化驱动状态。
//
// 与 watchdog 触发测试的区别：
//   - watchdog 触发测试使用 deadlineIgnoringConn（SetReadDeadline no-op），
//     模拟 Windows 故障环境下 deadline 失效，依赖 watchdog 兜底 Close。
//   - soft timeout 测试使用真实 net.Pipe（deadline 兑现），server 故意延迟
//     响应到 timeout 之后，验证 helper 检测到 net.Error.Timeout() 后主动 Close。
// =================================================================

// TestP1604ReadUnitCoefficient_SoftTimeoutClosesConnAndReturnsSentinel 验证
// soft deadline 兑现时 helper 强制 Close conn 并返回 ErrWatchdogTriggered。
//
// 测试前置：
//   - net.Pipe 建立双向连接（SetReadDeadline 真实兑现）
//   - server 读取 u01101 命令后延迟 300ms 写响应（超过 client 的 100ms timeout）
//
// 测试步骤：
//   - 调用 P1604ReadUnitCoefficient，timeout=100ms
//
// 期待结果：
//   - 函数在 5s 预算内返回（soft timeout 触发，无需等 watchdog）
//   - 错误包含 ErrWatchdogTriggered（errors.Is 命中 sentinel）
//   - conn 已被 helper Close：server.Write 迟到响应失败
func TestP1604ReadUnitCoefficient_SoftTimeoutClosesConnAndReturnsSentinel(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	// client 由 helper Close，不在 defer 中重复 Close

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		// 读取 u01101 命令
		buf := make([]byte, 64)
		_, _ = server.Read(buf)
		// 延迟 300ms 写响应（超过 client 的 100ms timeout，模拟迟到响应）
		time.Sleep(300 * time.Millisecond)
		// 迟到响应：此时 client 应已被 helper Close，Write 失败
		_, _ = server.Write([]byte{0x00, 0x05, '1', '.', '0'})
	}()

	done := make(chan error, 1)
	go func() {
		_, err := P1604ReadUnitCoefficient(NewFrameReader(client), client, 100*time.Millisecond)
		done <- err
	}()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected soft timeout error, got nil")
		}
		if !errors.Is(err, ErrWatchdogTriggered) {
			t.Errorf("expected error to wrap ErrWatchdogTriggered, got: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("P1604ReadUnitCoefficient did not return within 5s budget; soft timeout likely not detected")
	}

	// 验证 conn 已被 helper Close：server.Write 迟到响应应失败
	_ = server.SetWriteDeadline(time.Now().Add(200 * time.Millisecond))
	if _, writeErr := server.Write([]byte("probe")); writeErr == nil {
		t.Error("expected server.Write to fail after client was closed by soft timeout handler")
	}
	wg.Wait()
}

// TestP1604WriteUnitCoefficient_SoftTimeoutClosesConnAndReturnsSentinel 验证
// soft deadline 兑现时 helper 强制 Close conn 并返回 ErrWatchdogTriggered。
//
// 测试前置：
//   - net.Pipe 建立双向连接（SetReadDeadline 真实兑现）
//   - server 读取 v01101 命令后延迟 300ms 写响应（超过 client 的 100ms timeout）
//
// 测试步骤：
//   - 调用 P1604WriteUnitCoefficient，timeout=100ms
//
// 期待结果：
//   - 函数在 5s 预算内返回
//   - 错误包含 ErrWatchdogTriggered
//   - conn 已被 helper Close：server.Write 迟到响应失败
func TestP1604WriteUnitCoefficient_SoftTimeoutClosesConnAndReturnsSentinel(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	// client 由 helper Close

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		buf := make([]byte, 64)
		_, _ = server.Read(buf)
		// 延迟 300ms 写响应
		time.Sleep(300 * time.Millisecond)
		_, _ = server.Write([]byte{0x00, 0x03, 'A'})
	}()

	done := make(chan error, 1)
	go func() {
		err := P1604WriteUnitCoefficient(NewFrameReader(client), client, 6894.757, 100*time.Millisecond)
		done <- err
	}()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected soft timeout error, got nil")
		}
		if !errors.Is(err, ErrWatchdogTriggered) {
			t.Errorf("expected error to wrap ErrWatchdogTriggered, got: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("P1604WriteUnitCoefficient did not return within 5s budget; soft timeout likely not detected")
	}

	_ = server.SetWriteDeadline(time.Now().Add(200 * time.Millisecond))
	if _, writeErr := server.Write([]byte("probe")); writeErr == nil {
		t.Error("expected server.Write to fail after client was closed by soft timeout handler")
	}
	wg.Wait()
}
