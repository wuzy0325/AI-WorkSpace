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
// 返回值为当前硬件 EU 压力转换系数；调用方可通过 P1604MatchUnitByCoefficient 反查单位字符串。
func P1604ReadUnitCoefficient(reader *FrameReader, conn net.Conn, timeout time.Duration) (float64, error) {
	if reader == nil {
		return 0, fmt.Errorf("frame reader is nil")
	}
	if conn == nil {
		return 0, fmt.Errorf("conn is nil")
	}
	// 发送 u01101 命令（纯 ASCII，不带换行符——见 p1604ReadUnitCmd 注释）
	if timeout > 0 {
		if err := conn.SetWriteDeadline(time.Now().Add(timeout)); err != nil {
			return 0, fmt.Errorf("set write deadline: %w", err)
		}
		defer func() { _ = conn.SetWriteDeadline(time.Time{}) }()
	}
	if _, err := conn.Write([]byte(p1604ReadUnitCmd)); err != nil {
		return 0, fmt.Errorf("send %s: %w", p1604ReadUnitCmd, err)
	}

	// 读取响应（FrameReader 自动剥离 2 字节长度前缀）
	if timeout > 0 {
		if err := conn.SetReadDeadline(time.Now().Add(timeout)); err != nil {
			return 0, fmt.Errorf("set read deadline: %w", err)
		}
		defer func() { _ = conn.SetReadDeadline(time.Time{}) }()
	}
	payload, err := reader.ReadFrame()
	if err != nil {
		return 0, fmt.Errorf("read %s response: %w", p1604ReadUnitCmd, err)
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

	cmd := fmt.Sprintf(p1604WriteUnitFmt, coeff)
	if err := conn.SetWriteDeadline(time.Now().Add(timeout)); err != nil {
		return fmt.Errorf("set write deadline: %w", err)
	}
	if _, err := conn.Write([]byte(cmd)); err != nil {
		return fmt.Errorf("send v01101: %w", err)
	}

	if err := conn.SetReadDeadline(time.Now().Add(timeout)); err != nil {
		return fmt.Errorf("set read deadline: %w", err)
	}
	payload, err := reader.ReadFrame()
	if err != nil {
		return fmt.Errorf("read v01101 response: %w", err)
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
