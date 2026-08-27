//go:build ignore

package main

import (
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"time"
)

func main() {
	addr := "192.168.3.7:9000"
	fmt.Printf("连接 %s ...\n", addr)

	conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
	if err != nil {
		fmt.Printf("连接失败: %v\n", err)
		return
	}
	defer conn.Close()
	fmt.Println("已连接")

	// ===== 读单位 =====
	fmt.Println("\n=== 读单位 (u01101) ===")
	resp, err := sendWTN1604Command(conn, "u01101", 3*time.Second)
	if err != nil {
		fmt.Printf("发送失败: %v\n", err)
		return
	}
	fmt.Printf("原始响应: %q\n", resp)
	unit := parseWTN1604Unit(resp)
	fmt.Printf("解析单位: %q\n", unit)

	// ===== 读阀门状态 =====
	fmt.Println("\n=== 读阀门状态 (@01  0) ===")
	resp, err = sendWTN1604Command(conn, "@01  0", 3*time.Second)
	if err != nil {
		fmt.Printf("发送失败: %v\n", err)
		return
	}
	fmt.Printf("原始响应: %q\n", resp)
	valve := parseValveStatus(resp)
	fmt.Printf("阀门状态: %q\n", valve)

	// ===== 读设备信息 =====
	fmt.Println("\n=== 读设备型号 (q00) ===")
	resp, err = sendWTN1604Command(conn, "q00", 3*time.Second)
	if err != nil {
		fmt.Printf("发送失败: %v\n", err)
	} else {
		fmt.Printf("型号: %q\n", resp)
	}

	fmt.Println("\n=== 读设备版本 (q01) ===")
	resp, err = sendWTN1604Command(conn, "q01", 3*time.Second)
	if err != nil {
		fmt.Printf("发送失败: %v\n", err)
	} else {
		fmt.Printf("版本: %q\n", resp)
	}

	// ===== 通信测试 =====
	fmt.Println("\n=== 通信测试 (A) ===")
	resp, err = sendWTN1604Command(conn, "A", 3*time.Second)
	if err != nil {
		fmt.Printf("发送失败: %v\n", err)
	} else {
		fmt.Printf("响应: %q\n", resp)
	}
}

// sendWTN1604Command 复现驱动中的命令发送和长度前缀响应解析逻辑
func sendWTN1604Command(conn net.Conn, cmd string, readTimeout time.Duration) (string, error) {
	if err := conn.SetWriteDeadline(time.Now().Add(5 * time.Second)); err != nil {
		return "", fmt.Errorf("set write deadline: %w", err)
	}
	if _, err := conn.Write([]byte(cmd)); err != nil {
		return "", fmt.Errorf("write command %q: %w", cmd, err)
	}
	if err := conn.SetReadDeadline(time.Now().Add(readTimeout)); err != nil {
		return "", fmt.Errorf("set read deadline: %w", err)
	}
	lenBuf := make([]byte, 2)
	if _, err := io.ReadFull(conn, lenBuf); err != nil {
		return "", fmt.Errorf("read length prefix: %w", err)
	}
	totalLen := int(binary.BigEndian.Uint16(lenBuf))
	fmt.Printf("  长度前缀: %d (0x%04x)\n", totalLen, totalLen)
	if totalLen < 2 {
		return "", fmt.Errorf("invalid response length: %d", totalLen)
	}
	dataLen := totalLen - 2
	data := make([]byte, dataLen)
	if dataLen > 0 {
		if _, err := io.ReadFull(conn, data); err != nil {
			return "", fmt.Errorf("read response data: %w", err)
		}
	}
	response := strings.TrimSpace(strings.ReplaceAll(string(data), "\x00", ""))
	return response, nil
}

// parseWTN1604Unit 复现驱动中的单位解析逻辑
func parseWTN1604Unit(response string) string {
	val := strings.TrimSpace(response)
	if strings.HasPrefix(val, "A") {
		val = strings.TrimSpace(strings.TrimPrefix(val, "A"))
	}
	v, err := strconv.ParseFloat(val, 64)
	if err != nil {
		return coefficientToUnit(strings.TrimSpace(response))
	}
	// 先尝试系数匹配
	if unit, ok := matchCoefficientToUnit(v); ok {
		return unit
	}
	// 回退到整数码匹配
	unitInt := int(v + 0.5)
	switch unitInt {
	case 0:
		return "kgf/cm2"
	case 1:
		return "psi"
	case 6:
		return "kPa"
	case 6894:
		return "Pa"
	default:
		return response
	}
}

func matchCoefficientToUnit(v float64) (string, bool) {
	switch {
	case v == 1.0:
		return "psi", true
	case approxEqual(v, 0.07031):
		return "kgf/cm2", true
	case approxEqual(v, 0.0689476):
		return "bar", true
	case approxEqual(v, 68.9476):
		return "mbar", true
	case approxEqual(v, 6.89476):
		return "kPa", true
	case approxEqual(v, 0.00689476):
		return "MPa", true
	case approxEqual(v, 51.7149):
		return "mmHg", true
	case approxEqual(v, 0.068046):
		return "atm", true
	case approxEqual(v, 6894.76):
		return "Pa", true
	default:
		return "", false
	}
}

func approxEqual(a, b float64) bool {
	if a == 0 && b == 0 {
		return true
	}
	diff := a - b
	avg := (a + b) / 2
	if avg == 0 {
		return diff == 0
	}
	return (diff/avg) < 0.01 && (diff/avg) > -0.01
}

func coefficientToUnit(coef string) string {
	v, err := strconv.ParseFloat(coef, 64)
	if err != nil {
		return coef
	}
	switch {
	case v == 1.0:
		return "psi"
	case approxEqual(v, 0.07031):
		return "kgf/cm2"
	case approxEqual(v, 0.0689476):
		return "bar"
	case approxEqual(v, 68.9476):
		return "mbar"
	case approxEqual(v, 6.89476):
		return "kPa"
	case approxEqual(v, 0.00689476):
		return "MPa"
	case approxEqual(v, 51.7149):
		return "mmHg"
	case approxEqual(v, 0.068046):
		return "atm"
	case approxEqual(v, 6894.76):
		return "Pa"
	default:
		return coef
	}
}

// parseValveStatus 复现驱动中的阀门状态解析逻辑
func parseValveStatus(resp string) string {
	raw := strings.TrimSpace(resp)
	val := strings.TrimSpace(strings.TrimPrefix(raw, "A"))
	if val == "" {
		val = raw
	}
	if num, err := strconv.Atoi(strings.TrimSpace(val)); err == nil {
		switch num {
		case 1:
			return "calibration"
		case 0, 2, 3:
			return "measurement"
		}
	}
	switch strings.ToLower(strings.TrimSpace(val)) {
	case "calibration", "calibrate", "open", "opened", "on":
		return "calibration"
	case "measurement", "measure", "close", "closed", "off":
		return "measurement"
	default:
		return raw
	}
}
