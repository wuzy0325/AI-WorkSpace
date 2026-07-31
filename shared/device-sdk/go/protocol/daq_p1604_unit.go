package protocol

import (
	"fmt"
	"math"
	"net"
	"strconv"
	"strings"
	"time"
)

// P1604PressureUnitCoefficient DAQ-P-1604 EU 压力转换系数表
//
// 系数含义：1 psi 对应的目标单位数值（设备内部 EU 转换系数）。
// 设备通过全局数组 aa=11 / cc=01 的 EU 压力转换系数统一管理压力单位，
// 写入系数即等价于切换硬件压力单位。设备返回的压力数据流已经按该系数完成 EU 转换，
// 应用层无需再做换算。
//
// 参考值：1 psi = 6894.757 Pa
var P1604PressureUnitCoefficient = map[string]float64{
	"psi":     1.0,
	"Pa":      6894.7570,
	"kPa":     6.894757,
	"MPa":     0.006894757,
	"kgf/cm²": 0.070307,
}

// P1604DefaultUnit 默认压力单位
const P1604DefaultUnit = "psi"

// P1604UnitCoeffTolerance 单位系数匹配的相对容差。
//
// 必须用相对容差而非绝对容差：设备内部以 float32 存储 EU 系数，
// 例如 Pa 系数 6894.757 在 float32 中表示为 6894.756836，绝对差 0.000164，
// 但相对误差仅 ~2.4e-8。1e-4 的相对容差可覆盖 float32 精度损失，
// 同时不会误匹配相邻单位（kPa=6.89 vs MPa=0.0069，相对间距远大于 1e-4）。
const P1604UnitCoeffTolerance = 1e-4

// P1604UnitReadTimeout 读取硬件单位响应的默认超时
const P1604UnitReadTimeout = 2 * time.Second

// 命令格式（实测设备型号 9116 / 固件 00F8）：
//   - 读取 EU 系数：u01101（全局数组 aa=11 / cc=01，单 cc 语法）
//   - 写入 EU 系数：v01101 <coeff>（单 cc 语法；范围语法 v01101-05 返回 N05）
//
// 关键约束：命令以纯 ASCII 发送，**不得附加任何换行符（\r\n 或 \n）**。
// 实测 w1601 长度前缀模式下，设备将换行符视为命令字符的一部分，
// 对 u01101 等命令返回 N05（数据字段错误）。仅范围语法 u01101-05 对尾部
// 换行符宽容，曾掩盖此问题。正确做法是所有命令均不带换行符。
const (
	p1604ReadUnitCmd  = "u01101"      // 读取全局 EU 压力转换系数（单 cc 语法）
	p1604WriteUnitFmt = "v01101 %.6f" // 写入全局 EU 压力转换系数（单 cc 语法，无换行符）
)

// P1604ReadUnitCoefficient 从设备读取 EU 压力转换系数
//
// 命令：u01101（全局数组 aa=11 / cc=01，单 cc 语法）
//   - f=0  十进制 ASCII
//   - aa=11 全局数组
//   - cc=01 EU 压力转换系数
//
// 调用前提：
//   - 已通过 w1601 启用 2 字节长度前缀
//   - 调用方独占 conn 与 frameReader（通常在 Connect 阶段或未采集时调用）
//
// ADR-009 watchdog 兜底：SetWriteDeadline / SetReadDeadline 在故障 Windows 电脑
// 不可靠，Write / Read 在 deadline 到期后仍可能无限阻塞。watchdog 在独立 timer
// goroutine 上跑，超时后强制 conn.Close() 解除阻塞，覆盖 Write + Read 全流程。
// timeout == 0 时不启动 watchdog，由调用方负责通过 Close conn 解除阻塞。
//
// ADR-009 R0-12：soft deadline 触发时（net.Error.Timeout()），即使 watchdog 未触发也必须
// 毒化连接——迟到响应可能随后进入 TCP 流被下一条命令消费，导致协议错位。
// 整改后：soft timeout 时强制 Close conn 并返回 ErrWatchdogTriggered，让调用方
// 统一毒化驱动状态。
//
// 返回值为当前硬件 EU 压力转换系数；调用方可通过 P1604MatchUnitByCoefficient 反查单位字符串。
func P1604ReadUnitCoefficient(reader *FrameReader, conn net.Conn, timeout time.Duration) (float64, error) {
	if reader == nil {
		return 0, fmt.Errorf("frame reader is nil")
	}
	if conn == nil {
		return 0, fmt.Errorf("conn is nil")
	}

	// watchdog 仅在 timeout > 0 时启动；timeout == 0 时由调用方负责 Close。
	// 不启动 watchdog 时用 NoopWatchdogStop 占位，保持后续 wrapP1604IOError + defer 判断统一。
	var wdStop func() bool = NoopWatchdogStop
	if timeout > 0 {
		wdStop = WatchdogClose(conn, timeout)
	}

	// watchdog 未触发时清 deadline 让 conn 可复用；触发时 conn 已 Close，不清。
	// wdStop 幂等，defer 与 wrapP1604IOError 内部的多次调用安全（修复前 bug：
	// 第二次调用 stop 永久阻塞，已通过 sync.Once 修复）。
	defer func() {
		if wdStop() {
			if timeout > 0 {
				_ = conn.SetReadDeadline(time.Time{})
				_ = conn.SetWriteDeadline(time.Time{})
			}
		}
	}()

	// 发送 u01101 命令（纯 ASCII，不带换行符——见 p1604ReadUnitCmd 注释）
	if timeout > 0 {
		_ = conn.SetWriteDeadline(time.Now().Add(timeout))
	}
	if _, err := conn.Write([]byte(p1604ReadUnitCmd)); err != nil {
		return 0, wrapP1604IOError(err, wdStop, conn, "send "+p1604ReadUnitCmd)
	}

	// 读取响应（FrameReader 自动剥离 2 字节长度前缀）
	if timeout > 0 {
		_ = conn.SetReadDeadline(time.Now().Add(timeout))
	}
	payload, err := reader.ReadFrame()
	if err != nil {
		return 0, wrapP1604IOError(err, wdStop, conn, "read "+p1604ReadUnitCmd+" response")
	}

	text := strings.TrimSpace(string(payload))
	// 设备错误响应形如 N01/N05 等
	if strings.HasPrefix(text, "N") && len(text) >= 2 {
		return 0, fmt.Errorf("device returned error: %s", text)
	}
	fields := strings.Fields(text)
	if len(fields) == 0 {
		return 0, fmt.Errorf("empty response: %q", text)
	}
	if len(fields) != 1 {
		return 0, fmt.Errorf("unexpected %s response: %q", p1604ReadUnitCmd, text)
	}
	coeff, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return 0, fmt.Errorf("parse coefficient %q: %w", fields[0], err)
	}
	if math.IsNaN(coeff) || math.IsInf(coeff, 0) || coeff <= 0 {
		return 0, fmt.Errorf("invalid coefficient: %v", coeff)
	}
	return coeff, nil
}

// P1604WriteUnitCoefficient 写入 EU 压力转换系数到设备
//
// 命令：v01101 <coeff>（单 cc 语法，实测设备型号 9116 / 固件 00F8 要求）
// 设备返回 A 表示成功，Nxx 表示失败。
//
// 调用前提同 P1604ReadUnitCoefficient；此外建议仅在未采集时调用，
// 避免与数据流读取竞争 frameReader。
//
// ADR-009 watchdog 兜底：同 P1604ReadUnitCoefficient，覆盖 Write + Read 全流程。
// 修复前 bug：设 deadline 后无 defer 清除，失败路径 deadline 残留影响后续命令。
//
// ADR-009 R0-12：soft deadline 触发时同样强制 Close conn 并返回 ErrWatchdogTriggered，
// 防止迟到响应污染下一条命令（详见 P1604ReadUnitCoefficient 注释）。
func P1604WriteUnitCoefficient(reader *FrameReader, conn net.Conn, coeff float64, timeout time.Duration) error {
	if reader == nil {
		return fmt.Errorf("frame reader is nil")
	}
	if conn == nil {
		return fmt.Errorf("conn is nil")
	}
	if timeout <= 0 {
		timeout = P1604UnitReadTimeout
	}
	if math.IsNaN(coeff) || math.IsInf(coeff, 0) || coeff <= 0 {
		return fmt.Errorf("invalid coefficient: %v", coeff)
	}

	// watchdog 兜底：覆盖 Write + Read 全流程。timeout 已确保 > 0。
	wdStop := WatchdogClose(conn, timeout)
	defer func() {
		if wdStop() {
			_ = conn.SetReadDeadline(time.Time{})
			_ = conn.SetWriteDeadline(time.Time{})
		}
	}()

	cmd := fmt.Sprintf(p1604WriteUnitFmt, coeff)
	_ = conn.SetWriteDeadline(time.Now().Add(timeout))
	if _, err := conn.Write([]byte(cmd)); err != nil {
		return wrapP1604IOError(err, wdStop, conn, "send v01101")
	}

	_ = conn.SetReadDeadline(time.Now().Add(timeout))
	payload, err := reader.ReadFrame()
	if err != nil {
		return wrapP1604IOError(err, wdStop, conn, "read v01101 response")
	}
	response := string(payload)
	if response == "A" {
		return nil
	}
	if strings.HasPrefix(response, "N") {
		return fmt.Errorf("device rejected unit change: %s", response)
	}
	return fmt.Errorf("unexpected v01101 response: %q", response)
}

// wrapP1604IOError 统一包装 P1604 unit helper 的 I/O 错误，覆盖三种场景：
//
//  1. soft timeout（net.Error.Timeout()==true）：deadline 先于 watchdog 触发。
//     ADR-009 R0-12：协议边界已不可信，迟到响应可能随后进入 TCP 流被下一条命令消费。
//     强制 Close conn 阻断迟到响应，返回包含 ErrWatchdogTriggered sentinel 的错误，
//     让调用方（DAQP1604.SetUnit / syncUnitFromHardware / P1604Adapter.ApplyConfig 等）
//     通过 errors.Is 检测并毒化驱动状态。
//
//  2. watchdog 已触发（wdStop()==false）：conn 已 Close，WrapWatchdogError 已包装 sentinel。
//
//  3. 其他错误（连接重置 / EOF / 协议解析错误）：仅附加 op 上下文，由调用方按错误类型决策。
//     调用方通常已通过 IsConnResetByPeer 处理"对端已 FIN/RST"场景，此处不重复 Close。
//
// 调用约束：必须在错误返回路径调用；conn 在 soft timeout 分支会被 Close，调用方
// 不可再使用该 conn 进行 I/O。
func wrapP1604IOError(err error, wdStop func() bool, conn net.Conn, op string) error {
	if err == nil {
		return nil
	}
	// soft timeout 检测：deadline 兑现但 watchdog 未触发。
	// 强制 Close conn 阻断迟到响应，统一返回 ErrWatchdogTriggered 让调用方毒化连接。
	if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
		_ = conn.Close()
		return fmt.Errorf("%s: %w; %w", op, err, ErrWatchdogTriggered)
	}
	// 其他错误（含 watchdog 已触发场景）委托给 WrapWatchdogError 统一处理。
	return WrapWatchdogError(err, wdStop, op)
}

// P1604MatchUnitByCoefficient 根据系数反查最接近的单位字符串
//
// 使用相对容差 P1604UnitCoeffTolerance（1e-4），覆盖设备 float32 存储精度损失。
// 匹配失败返回 ("", false)。用于读取硬件系数后识别当前单位。
func P1604MatchUnitByCoefficient(coeff float64) (string, bool) {
	for unit, c := range P1604PressureUnitCoefficient {
		// 相对容差：|coeff - c| / max(|c|, eps) < tolerance
		// 避免除以零，对接近 0 的系数（如 MPa=0.0069）退化为绝对比较
		base := math.Abs(c)
		if base < 1e-9 {
			if math.Abs(coeff-c) < P1604UnitCoeffTolerance {
				return unit, true
			}
			continue
		}
		if math.Abs(coeff-c)/base < P1604UnitCoeffTolerance {
			return unit, true
		}
	}
	return "", false
}

// P1604IsSupportedUnit 判断单位字符串是否在硬件支持的列表内
func P1604IsSupportedUnit(unit string) bool {
	_, ok := P1604PressureUnitCoefficient[unit]
	return ok
}
