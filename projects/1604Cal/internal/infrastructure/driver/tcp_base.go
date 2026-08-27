package driver

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"cal1604/internal/device"
	"cal1604/internal/events"
)

const defaultTCPDialTimeout = 3 * time.Second

// tcpConnectionDriver 负责维护 TCP 级别连接与命令交互。
type tcpConnectionDriver struct {
	model     string
	address   string
	localAddr string
	mu        sync.Mutex
	conn      net.Conn
	breaker   *CircuitBreaker
	retryCfg  RetryConfig
}

func newTCPConnectionDriver(model string, host string, port int) *tcpConnectionDriver {
	return &tcpConnectionDriver{
		model:    model,
		address:  net.JoinHostPort(host, strconv.Itoa(port)),
		breaker:  NewCircuitBreaker(DefaultCircuitBreakerConfig()),
		retryCfg: DefaultRetryConfig(),
	}
}

// newTCPConnectionDriverWithLocalAddr 创建绑定指定本地地址的 TCP 驱动。
// localAddr 为空时等价于 newTCPConnectionDriver，由操作系统自动选择路由。
func newTCPConnectionDriverWithLocalAddr(model string, host string, port int, localAddr string) *tcpConnectionDriver {
	d := newTCPConnectionDriver(model, host, port)
	d.localAddr = localAddr
	return d
}

func (d *tcpConnectionDriver) Connect(ctx context.Context) error {
	if !d.breaker.AllowRequest() {
		return fmt.Errorf("%s: circuit breaker is open", d.model)
	}

	connectCmd := "CONNECT " + d.address
	if d.localAddr != "" {
		connectCmd += " (bind " + d.localAddr + ")"
	}
	var lastErr error
	rs := NewRetryStrategy(d.retryCfg)
	for attempt := 0; rs.ShouldRetry(attempt); attempt++ {
		if attempt > 0 {
			delay := time.Duration(rs.NextDelay(attempt)) * time.Millisecond
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(delay):
			}
		}

		d.mu.Lock()
		if d.conn != nil {
			_ = d.conn.Close()
			d.conn = nil
		}
		d.mu.Unlock()

		d.mu.Lock()
		events.GlobalBus.Publish(events.Event{Type: events.EventHardwareCommand, Data: map[string]any{
			"model": d.model,
			"proto": "TCP",
			"cmd":   connectCmd,
		}})
		d.mu.Unlock()
		log.Printf("[tcp] %s attempt %d/%d %s", d.model, attempt+1, d.retryCfg.MaxAttempts, connectCmd)

		var dialer net.Dialer
		dialer.Timeout = defaultTCPDialTimeout
		if d.localAddr != "" {
			localTCPAddr, addrErr := net.ResolveTCPAddr("tcp", net.JoinHostPort(d.localAddr, "0"))
			if addrErr != nil {
				lastErr = fmt.Errorf("%s resolve local addr %s: %w", d.model, d.localAddr, addrErr)
				log.Printf("[tcp] %v", lastErr)
				events.GlobalBus.Publish(events.Event{Type: events.EventHardwareResponse, Data: map[string]any{
					"model": d.model,
					"proto": "TCP",
					"resp":  "ERROR: " + lastErr.Error(),
					"cmd":   connectCmd,
				}})
				continue
			}
			dialer.LocalAddr = localTCPAddr
		}
		conn, err := d.dialWithTimeout(ctx, dialer, attempt+1)
		if err != nil {
			lastErr = fmt.Errorf("%s dial %s: %w", d.model, d.address, err)
			if errors.Is(err, context.DeadlineExceeded) {
				lastErr = fmt.Errorf("%s dial %s timeout after %s: %w", d.model, d.address, defaultTCPDialTimeout, err)
			}
			log.Printf("[tcp] %v", lastErr)
			events.GlobalBus.Publish(events.Event{Type: events.EventHardwareResponse, Data: map[string]any{
				"model": d.model,
				"proto": "TCP",
				"resp":  "ERROR: " + lastErr.Error(),
				"cmd":   connectCmd,
			}})
			continue
		}
		if tcpConn, ok := conn.(*net.TCPConn); ok {
			_ = tcpConn.SetKeepAlive(true)
			_ = tcpConn.SetKeepAlivePeriod(30 * time.Second)
		}
		d.mu.Lock()
		if d.conn != nil {
			_ = d.conn.Close()
		}
		d.conn = conn
		d.mu.Unlock()

		d.breaker.RecordSuccess()
		log.Printf("[tcp] %s connected %s", d.model, d.address)
		events.GlobalBus.Publish(events.Event{Type: events.EventHardwareResponse, Data: map[string]any{
			"model": d.model,
			"proto": "TCP",
			"resp":  "CONNECTED " + d.address,
			"cmd":   connectCmd,
		}})
		return nil
	}

	d.breaker.RecordFailure()
	log.Printf("[tcp] %s connect failed after %d attempts: %v", d.model, d.retryCfg.MaxAttempts, lastErr)
	return fmt.Errorf("%s connect failed after %d attempts: %w", d.model, d.retryCfg.MaxAttempts, lastErr)
}

func (d *tcpConnectionDriver) dialWithTimeout(ctx context.Context, dialer net.Dialer, attempt int) (net.Conn, error) {
	type dialResult struct {
		conn net.Conn
		err  error
	}
	resultCh := make(chan dialResult, 1)
	dialCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	go func() {
		conn, err := dialer.DialContext(dialCtx, "tcp", d.address)
		resultCh <- dialResult{conn: conn, err: err}
	}()

	timer := time.NewTimer(defaultTCPDialTimeout)
	defer timer.Stop()

	select {
	case result := <-resultCh:
		return result.conn, result.err
	case <-timer.C:
		message := fmt.Sprintf("%s %s 第 %d 次 TCP 连接超过 %s 未返回", d.model, d.address, attempt, defaultTCPDialTimeout)
		log.Printf("[tcp] %s", message)
		events.GlobalBus.Publish(events.Event{Type: events.EventSystemError, Data: map[string]any{
			"code":    "TCP_DIAL_TIMEOUT",
			"status":  0,
			"message": message,
		}})
		return nil, context.DeadlineExceeded
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (d *tcpConnectionDriver) Disconnect(_ context.Context) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.conn == nil {
		return nil
	}
	if err := d.conn.Close(); err != nil {
		return fmt.Errorf("%s close connection: %w", d.model, err)
	}
	d.conn = nil
	return nil
}

// closeConn 关闭并清理已损坏的连接（调用者必须持有 d.mu）。
func (d *tcpConnectionDriver) closeConn() {
	if d.conn != nil {
		_ = d.conn.Close()
		d.conn = nil
	}
}

// sendSCPICommand 发送 SCPI 命令并读取响应（带超时）。
// 对于设置类命令（不含 ?），设备通常不回复，直接返回空响应以免阻塞 3 秒。

// WithPollContext 返回一个标记了"轮询"的新 context。
// 实现已上移到 device 包（ports 层），此处保留转发以维持 driver 包内部调用不变。
// application 层应直接调用 device.WithPollContext，避免依赖 adapters 层。
func WithPollContext(ctx context.Context) context.Context {
	return device.WithPollContext(ctx)
}

// IsPollContext 检查 context 中是否标记了轮询操作。
// 实现已上移到 device 包（ports 层），此处保留转发以维持 driver 包内部调用不变。
func IsPollContext(ctx context.Context) bool {
	return device.IsPollContext(ctx)
}

func (d *tcpConnectionDriver) sendSCPICommand(ctx context.Context, cmd string, readTimeout time.Duration) (string, error) {
	return d.sendSCPICommandWithTerminator(ctx, cmd, "\r\n", readTimeout)
}

// sendSCPICommandWithoutTerminator 用于要求按原始字符串长度写入的设备命令。
// 查询仍应使用 sendSCPICommand，以便设备收到明确的命令结束符。
func (d *tcpConnectionDriver) sendSCPICommandWithoutTerminator(ctx context.Context, cmd string) (string, error) {
	return d.sendSCPICommandWithTerminator(ctx, cmd, "", 0)
}

func (d *tcpConnectionDriver) sendSCPICommandWithTerminator(ctx context.Context, cmd, terminator string, readTimeout time.Duration) (string, error) {
	poll := IsPollContext(ctx)
	commandData := map[string]any{
		"model": d.model,
		"proto": "SCPI",
		"cmd":   cmd,
	}
	if poll {
		commandData["poll"] = true
	}
	events.GlobalBus.Publish(events.Event{Type: events.EventHardwareCommand, Data: map[string]any{
		"model": d.model,
		"proto": "SCPI",
		"cmd":   cmd,
		"poll":  poll,
	}})
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.conn == nil {
		err := fmt.Errorf("%s: not connected", d.model)
		publishHardwareError(commandData, err)
		return "", err
	}
	// 检查 context 是否已过期，避免将过期期限设置到 socket 导致 Windows WSAECONNABORTED
	if err := ctx.Err(); err != nil {
		wrapped := fmt.Errorf("%s write SCPI command %q: context error: %w", d.model, cmd, err)
		publishHardwareError(commandData, wrapped)
		return "", wrapped
	}
	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = time.Now().Add(5 * time.Second)
	}
	if err := d.conn.SetWriteDeadline(deadline); err != nil {
		wrapped := fmt.Errorf("%s set write deadline: %w", d.model, err)
		publishHardwareError(commandData, wrapped)
		return "", wrapped
	}
	if _, err := fmt.Fprintf(d.conn, "%s%s", cmd, terminator); err != nil {
		d.closeConn()
		wrapped := fmt.Errorf("%s write SCPI command %q: %w", d.model, cmd, err)
		publishHardwareError(commandData, wrapped)
		return "", wrapped
	}
	// 设置类命令（不含 ?）通常无响应，跳过读取避免 3 秒超时阻塞
	if !strings.Contains(cmd, "?") {
		return "", nil
	}
	if err := d.conn.SetReadDeadline(time.Now().Add(readTimeout)); err != nil {
		wrapped := fmt.Errorf("%s set read deadline: %w", d.model, err)
		publishHardwareError(commandData, wrapped)
		return "", wrapped
	}
	var resp strings.Builder
	buf := make([]byte, 4096)
	for {
		n, err := d.conn.Read(buf)
		if err != nil {
			break
		}
		resp.Write(buf[:n])
		if strings.Contains(resp.String(), "\n") {
			break
		}
	}
	response := strings.TrimSpace(resp.String())
	events.GlobalBus.Publish(events.Event{Type: events.EventHardwareResponse, Data: map[string]any{
		"model": d.model,
		"proto": "SCPI",
		"resp":  response,
		"cmd":   cmd,
		"poll":  poll,
	}})
	return response, nil
}

func publishHardwareError(commandData map[string]any, err error) {
	events.GlobalBus.Publish(events.Event{Type: events.EventHardwareResponse, Data: map[string]any{
		"model": commandData["model"],
		"proto": commandData["proto"],
		"resp":  "ERROR: " + err.Error(),
		"cmd":   commandData["cmd"],
		"poll":  commandData["poll"],
	}})
}

// sendWTN1604Command 发送 WTN1604 命令并读取长度前缀响应。
func (d *tcpConnectionDriver) sendWTN1604Command(ctx context.Context, cmd string, readTimeout time.Duration) (string, error) {
	poll := IsPollContext(ctx)
	events.GlobalBus.Publish(events.Event{Type: events.EventHardwareCommand, Data: map[string]any{
		"model": d.model,
		"proto": "WTN1604",
		"cmd":   cmd,
		"poll":  poll,
	}})
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.conn == nil {
		return "", fmt.Errorf("%s: not connected", d.model)
	}
	// 检查 context 是否已过期，避免将过期期限设置到 socket 导致 Windows WSAECONNABORTED
	if err := ctx.Err(); err != nil {
		return "", fmt.Errorf("%s write command %q: context error: %w", d.model, cmd, err)
	}
	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = time.Now().Add(5 * time.Second)
	}
	if err := d.conn.SetWriteDeadline(deadline); err != nil {
		return "", fmt.Errorf("%s set write deadline: %w", d.model, err)
	}
	if _, err := d.conn.Write([]byte(cmd)); err != nil {
		d.closeConn()
		return "", fmt.Errorf("%s write command %q: %w", d.model, cmd, err)
	}
	if err := d.conn.SetReadDeadline(time.Now().Add(readTimeout)); err != nil {
		d.closeConn()
		return "", fmt.Errorf("%s set read deadline: %w", d.model, err)
	}
	lenBuf := make([]byte, 2)
	if _, err := io.ReadFull(d.conn, lenBuf); err != nil {
		d.closeConn()
		return "", fmt.Errorf("%s read length prefix: %w", d.model, err)
	}
	totalLen := int(binary.BigEndian.Uint16(lenBuf))
	if totalLen == 0 {
		// 空响应（00-00）：设备已接受命令但无数据返回。不关闭连接，避免后续命令失败。
		return "", nil
	}
	if totalLen < 2 {
		d.closeConn()
		return "", fmt.Errorf("%s invalid response length: %d", d.model, totalLen)
	}
	dataLen := totalLen - 2
	data := make([]byte, dataLen)
	if dataLen > 0 {
		if _, err := io.ReadFull(d.conn, data); err != nil {
			d.closeConn()
			return "", fmt.Errorf("%s read response data: %w", d.model, err)
		}
	}
	response := strings.TrimSpace(strings.ReplaceAll(string(data), "\x00", ""))
	events.GlobalBus.Publish(events.Event{Type: events.EventHardwareResponse, Data: map[string]any{
		"model": d.model,
		"proto": "WTN1604",
		"resp":  response,
		"cmd":   cmd,
		"poll":  poll,
	}})
	return response, nil
}
