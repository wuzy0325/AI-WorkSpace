// Package modbus 实现 Modbus TCP 协议栈最小子集（FC3 读 Holding /
// FC4 读 Input / FC6 写单寄存器），供 DAQ-T1602 温度扫描阀驱动使用。
// 纯协议层：只做帧编解码与请求-响应事务，零域逻辑。
//
// 连接模型（spec-daq-t1602 §Protocol，2026-08-12 真机实测）：
//   - 单 TCP 连接 + Unit ID 复用：一条到 IP:502 的连接上通过 Unit ID 寻址多从站；
//   - 单 in-flight 严格串行：同一连接上所有请求禁止流水线，与固件串行处理行为一致；
//   - Transaction ID 请求侧自增（uint16 回绕），响应必须回显校验，不匹配按超时处理；
//   - 响应超时固定 1s（实测 RTT ~103ms，留足余量）；超时即判定连接损坏并 Close；
//   - ADR-009：deadline 只是软超时，每个请求由独立 watchdog 直接 conn.Close() 兜底；
//     watchdog 关闭的连接不可复用。
package modbus

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"shared.local/device-sdk/go/protocol"
)

const (
	// FuncReadHoldingRegisters FC3：读 Holding 寄存器（T1602 通道类型，地址 200~207）。
	FuncReadHoldingRegisters = 0x03
	// FuncReadInputRegisters FC4：读 Input 寄存器（T1602 采集数据，地址 0~7）。
	FuncReadInputRegisters = 0x04
	// FuncWriteSingleRegister FC6：写单寄存器（T1602 通道类型写入）。
	FuncWriteSingleRegister = 0x06

	// ResponseTimeout 单请求响应超时。实测设备单请求 RTT ~103ms（固件 100ms
	// 响应周期串行处理），1s 留足余量。超时即判定连接损坏，走 ADR-009
	// owner Close() 路径，连接不可复用。
	ResponseTimeout = 1 * time.Second

	// MaxReadCount 单次读寄存器上限。实测读 9+ 返回异常码 2。
	MaxReadCount = 8

	mbapHeaderLen = 7
	// maxMBAPLength MBAP length 字段上限：unit(1) + 最长读响应 PDU（FC + 字节数 + 8×2）。
	maxMBAPLength = 1 + 2 + MaxReadCount*2
)

// ErrConnBroken 表示连接已判定损坏并被 Close：响应超时、Transaction ID 不回显、
// I/O 错误或帧格式非法都会触发。触发后连接不可复用，调用方必须清理状态并重连
// （ADR-009 决策 3/7/9：迟到响应可能污染下一条命令，协议边界不可信）。
var ErrConnBroken = errors.New("modbus: connection broken, closed and not reusable")

// ExceptionError 设备返回的 Modbus 异常响应（FC|0x80 + 异常码）。
// 属业务错误：连接协议边界仍可信，可继续使用（如读 9+ 寄存器返回异常码 2）。
type ExceptionError struct {
	Function uint8
	Code     uint8
}

func (e *ExceptionError) Error() string {
	return fmt.Sprintf("modbus: exception on FC 0x%02X: code %d", e.Function, e.Code)
}

// Conn 是单连接 Modbus TCP 客户端，所有请求严格串行（单 in-flight）。
// 并发安全：roundTrip 全程持锁，多 goroutine 调用自动排队。
type Conn struct {
	mu   sync.Mutex
	conn net.Conn
	// closed 用 atomic 而非 mu 保护：Close 必须能在 roundTrip 永久持锁
	// （deadline 失效 + Read 永久阻塞 + watchdog 的 closesocket 也卡死的
	// ADR-009 极端场景）时不等待事务锁直接返回。
	closed  atomic.Bool
	timeout time.Duration
	txID    uint16
}

// NewConn 在已建立的 TCP 连接上创建 Modbus 客户端。连接生命周期由调用方持有；
// Conn 仅在事务失败（超时/帧错误）时按 ADR-009 主动 Close。
func NewConn(conn net.Conn) *Conn {
	return &Conn{conn: conn, timeout: ResponseTimeout}
}

// ReadHoldingRegisters FC3 读 Holding 寄存器。count ∈ [1, MaxReadCount]。
func (c *Conn) ReadHoldingRegisters(unitID uint8, address uint16, count uint16) ([]uint16, error) {
	return c.readRegisters(unitID, FuncReadHoldingRegisters, address, count)
}

// ReadInputRegisters FC4 读 Input 寄存器。count ∈ [1, MaxReadCount]。
func (c *Conn) ReadInputRegisters(unitID uint8, address uint16, count uint16) ([]uint16, error) {
	return c.readRegisters(unitID, FuncReadInputRegisters, address, count)
}

// WriteSingleRegister FC6 写单寄存器。正常响应必须原样回显请求 PDU，
// 回显不一致判定协议边界不可信，按 ErrConnBroken 处理。
func (c *Conn) WriteSingleRegister(unitID uint8, address uint16, value uint16) error {
	pdu := []byte{FuncWriteSingleRegister, byte(address >> 8), byte(address), byte(value >> 8), byte(value)}
	resp, err := c.roundTrip(unitID, pdu)
	if err != nil {
		return err
	}
	if err := checkException(resp, FuncWriteSingleRegister); err != nil {
		return err
	}
	if !bytesEqual(resp, pdu) {
		c.poison()
		return fmt.Errorf("modbus: FC6 echo mismatch: got % X, want % X: %w", resp, pdu, ErrConnBroken)
	}
	return nil
}

// Close 关闭连接，保证不阻塞调用方：不获取事务锁 mu（roundTrip 在 ADR-009
// 极端场景下可能永久持锁），仅做原子标记并后台 AbortConnection
// （CloseWrite FIN + Close）。幂等，重复调用安全。
// 若有 in-flight 事务，其阻塞 I/O 由 AbortConnection/watchdog 解除后
// 以错误退出，事务方感知 ErrConnBroken。
func (c *Conn) Close() error {
	if c.closed.CompareAndSwap(false, true) {
		go protocol.AbortConnection(c.conn)
	}
	return nil
}

// Closed 报告连接是否已关闭（事务失败毒化或显式 Close）。
func (c *Conn) Closed() bool {
	return c.closed.Load()
}

// readRegisters 是 FC3/FC4 的公共实现：请求 PDU = FC + 起始地址(2B) + 数量(2B)，
// 正常响应 PDU = FC + 字节数(1B) + N×2B 大端寄存器值。
func (c *Conn) readRegisters(unitID uint8, fc uint8, address uint16, count uint16) ([]uint16, error) {
	if count == 0 || count > MaxReadCount {
		return nil, fmt.Errorf("modbus: read count %d out of range [1,%d]", count, MaxReadCount)
	}
	pdu := []byte{fc, byte(address >> 8), byte(address), byte(count >> 8), byte(count)}
	resp, err := c.roundTrip(unitID, pdu)
	if err != nil {
		return nil, err
	}
	if err := checkException(resp, fc); err != nil {
		return nil, err
	}
	byteCount := int(count) * 2
	if len(resp) != 2+byteCount || resp[0] != fc || int(resp[1]) != byteCount {
		c.poison()
		return nil, fmt.Errorf("modbus: FC 0x%02X malformed response % X: %w", fc, resp, ErrConnBroken)
	}
	values := make([]uint16, count)
	for i := range values {
		values[i] = binary.BigEndian.Uint16(resp[2+i*2:])
	}
	return values, nil
}

// roundTrip 发送一个请求 PDU 并等待响应 PDU。mu 覆盖整个请求-响应周期，
// 保证单 in-flight 严格串行（与固件串行排队行为一致，也简化帧边界切割）。
//
// ADR-009：SetDeadline 仅作软超时；独立 watchdog（WatchdogClose）在超时后
// 直接 conn.Close() 解除阻塞 I/O，不依赖 deadline 在故障 Windows 电脑上生效。
// 任何 I/O 错误、超时、Transaction ID 不回显都会毒化连接（fail），之后所有
// 请求直接返回 ErrConnBroken。
func (c *Conn) roundTrip(unitID uint8, pdu []byte) ([]byte, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed.Load() {
		return nil, ErrConnBroken
	}

	c.txID++
	txID := c.txID
	adu := make([]byte, 0, mbapHeaderLen+len(pdu))
	adu = binary.BigEndian.AppendUint16(adu, txID)
	adu = binary.BigEndian.AppendUint16(adu, 0) // Protocol ID 固定 0
	adu = binary.BigEndian.AppendUint16(adu, uint16(len(pdu)+1))
	adu = append(adu, unitID)
	adu = append(adu, pdu...)

	wdStop := protocol.WatchdogClose(c.conn, c.timeout)
	defer wdStop()
	_ = c.conn.SetDeadline(time.Now().Add(c.timeout))
	if _, err := c.conn.Write(adu); err != nil {
		return nil, c.fail(fmt.Errorf("modbus: write request: %w", err))
	}
	resp, err := c.readResponse(txID, unitID)
	if err != nil {
		return nil, c.fail(err)
	}
	return resp, nil
}

// readResponse 读取一个响应 ADU 并校验 MBAP 头：Transaction ID 回显、
// Protocol ID=0、length 字段限定帧边界、Unit ID 回显。
// Transaction ID 不回显的响应按超时处理（spec §连接模型：防止串帧），
// 调用方 fail 毒化连接。
func (c *Conn) readResponse(wantTxID uint16, wantUnit uint8) ([]byte, error) {
	header := make([]byte, mbapHeaderLen)
	if _, err := io.ReadFull(c.conn, header); err != nil {
		return nil, fmt.Errorf("modbus: read MBAP header: %w", err)
	}
	txID := binary.BigEndian.Uint16(header[0:2])
	protoID := binary.BigEndian.Uint16(header[2:4])
	length := int(binary.BigEndian.Uint16(header[4:6]))
	if protoID != 0 {
		return nil, fmt.Errorf("modbus: unexpected protocol id %d", protoID)
	}
	if length < 2 || length > maxMBAPLength {
		return nil, fmt.Errorf("modbus: invalid MBAP length %d", length)
	}
	if txID != wantTxID {
		return nil, fmt.Errorf("modbus: transaction id mismatch (got %d, want %d), treated as timeout: %w",
			txID, wantTxID, os.ErrDeadlineExceeded)
	}
	pdu := make([]byte, length-1)
	if _, err := io.ReadFull(c.conn, pdu); err != nil {
		return nil, fmt.Errorf("modbus: read PDU: %w", err)
	}
	if header[6] != wantUnit {
		return nil, fmt.Errorf("modbus: unit id mismatch (got %d, want %d)", header[6], wantUnit)
	}
	return pdu, nil
}

// fail 在 roundTrip 持锁期间毒化连接：标记 closed 并后台 Close，返回包装
// ErrConnBroken 的错误。ADR-009 决策 7：watchdog 超时错误必须明确表示连接已失效。
func (c *Conn) fail(err error) error {
	if c.closed.CompareAndSwap(false, true) {
		go protocol.AbortConnection(c.conn)
	}
	return fmt.Errorf("%w; %w", err, ErrConnBroken)
}

// poison 在 roundTrip 之外（响应内容校验失败）毒化连接，语义同 fail。
func (c *Conn) poison() {
	if c.closed.CompareAndSwap(false, true) {
		go protocol.AbortConnection(c.conn)
	}
}

// checkException 解析异常响应（FC|0x80 + 异常码）。异常属业务错误，不毒化连接。
func checkException(resp []byte, fc uint8) error {
	if len(resp) == 2 && resp[0] == fc|0x80 {
		return &ExceptionError{Function: fc, Code: resp[1]}
	}
	return nil
}

// bytesEqual 比较两个 []byte 是否相等（Go 1.20 替代 slices.Equal）。
func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
