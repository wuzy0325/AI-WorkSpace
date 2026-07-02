// Command p1604-unit-diag 用于离线诊断 DAQ-P-1604 的工程单位（EU 压力转换系数）写入是否真实生效。
//
// 诊断流程：
//  1. 连接设备 → 启用长度前缀（w1601）
//  2. 读取设备信息（q00 型号 / q01 固件版本）
//  3. 探测多种 u 命令格式，找到设备实际支持的 EU 系数读取入口
//  4. 读取当前 16 通道压力值（rFFFF0，ASCII 格式）
//  5. 写入新 EU 系数（若找到可用入口）
//  6. 验证写入是否持久 + 压力值是否随之变化
//  7. 恢复原始 EU 系数
//
// 用法：
//
//	go run ./programs/p1604-unit-diag -host 192.168.3.101 -port 9000 -unit Pa
package main

import (
	"flag"
	"fmt"
	"net"
	"os"
	"strings"
	"time"

	sharedproto "shared.local/device-sdk/go/protocol"
)

const (
	defaultHost    = "192.168.3.101"
	defaultPort    = 9000
	connectTimeout = 5 * time.Second
	cmdTimeout     = 2 * time.Second
)

func main() {
	host := flag.String("host", defaultHost, "设备 IP")
	port := flag.Int("port", defaultPort, "设备 TCP 端口")
	targetUnit := flag.String("unit", "Pa", "目标单位: psi / Pa / kPa / MPa / kgf/cm²")
	skipWrite := flag.Bool("skip-write", false, "仅探测不写入（诊断模式）")
	probeNewline := flag.Bool("probe-newline", false, "探测换行符对 u01101 的影响")
	flag.Parse()

	coeff, ok := sharedproto.P1604PressureUnitCoefficient[*targetUnit]
	if !ok {
		fmt.Fprintf(os.Stderr, "不支持的单位: %s\n支持: %s\n", *targetUnit, supportedUnits())
		os.Exit(2)
	}

	addr := fmt.Sprintf("%s:%d", *host, *port)
	fmt.Printf("=== DAQ-P-1604 EU 系数写入诊断 ===\n")
	fmt.Printf("目标: %s  目标单位: %s (coeff=%f)\n\n", addr, *targetUnit, coeff)

	conn, err := net.DialTimeout("tcp", addr, connectTimeout)
	if err != nil {
		fail("连接失败: %v", err)
	}
	defer conn.Close()
	fmt.Printf("[OK] TCP 已连接\n")
	if tcp, ok := conn.(*net.TCPConn); ok {
		_ = tcp.SetKeepAlive(true)
		_ = tcp.SetKeepAlivePeriod(10 * time.Second)
	}

	reader := sharedproto.NewFrameReader(conn)

	// 步骤 1: 启用长度前缀
	if err := writeCmd(conn, "w1601"); err != nil {
		fail("w1601 发送失败: %v", err)
	}
	time.Sleep(100 * time.Millisecond)
	drainW1601(conn, reader, 200*time.Millisecond)
	fmt.Printf("[OK] w1601 已启用长度前缀\n\n")

	// 步骤 2: 读设备信息
	fmt.Printf("[步骤2] 读取设备信息...\n")
	model := queryCmd(conn, reader, "q00")
	fmt.Printf("  型号 q00 = %s\n", model)
	firmware := queryCmd(conn, reader, "q01")
	fmt.Printf("  固件 q01 = %s\n", firmware)
	status := queryCmd(conn, reader, "q40")
	fmt.Printf("  状态 q40 = %s\n", status)

	// 新增：探测换行符对 u01101 的影响
	if *probeNewline {
		fmt.Printf("\n[步骤2.5] 探测换行符对 u01101 的影响...\n")
		probeNewlineVariants(conn, reader)
	}

	// 步骤 3: 探测多种 u 命令格式
	fmt.Printf("\n[步骤3] 探测 u 命令格式（查找设备支持的 EU 系数入口）...\n")
	probes := []string{
		"u01101",  // 文档 3.5.3 示例：全局数组 aa=11 cc=01 EU 系数
		"u01100",  // 全局 cc=00 offset
		"u01102",  // 全局 cc=02 EU c0
		"u01105",  // 全局 cc=05 EU c3
		"u0110A",  // 全局 cc=0A 量程代码
		"u0115F",  // 全局 cc=5F 当前压力 EU 值
		"u00101",  // 传感器1 cc=01 slope
		"u00100",  // 传感器1 cc=00 offset
		"u00100-01", // 传感器1 cc=00-01 offset+slope
		"u01101-05", // 全局 cc=01-05
	}
	probeResults := make(map[string]string, len(probes))
	for _, p := range probes {
		resp := queryCmd(conn, reader, p)
		probeResults[p] = resp
		mark := ""
		if strings.HasPrefix(resp, "N") {
			mark = "  ← 失败"
		} else if resp != "" {
			mark = "  ← 成功"
		}
		fmt.Printf("  %-12s => %q%s\n", p, resp, mark)
	}

	if *skipWrite {
		fmt.Printf("\n[skip-write=true] 仅探测模式，跳过写入测试\n")
		fmt.Printf("\n=== 诊断完成 ===\n")
		return
	}

	// 固定使用 u01101 作为 EU 系数读取入口（与协议层 p1604ReadUnitCmd 一致）
	readCmd := "u01101"
	fmt.Printf("\n[OK] 固定使用 %s 作为 EU 系数读取入口\n", readCmd)

	// 步骤 4: 读原始压力值
	fmt.Printf("\n[步骤4] 读取 16 通道压力值（rFFFF0，ASCII）...\n")
	origPressures, err := readPressures(conn, reader)
	if err != nil {
		fail("读取原始压力失败: %v", err)
	}
	printPressures("原始压力", origPressures)

	// 读原始系数
	origResp := queryCmd(conn, reader, readCmd)
	fmt.Printf("\n原始 %s => %q\n", readCmd, origResp)
	origCoeff, parseErr := parseFirstFloat(origResp)
	if parseErr != nil {
		fail("解析原始系数失败: %v (raw=%q)", parseErr, origResp)
	}
	origUnit, _ := sharedproto.P1604MatchUnitByCoefficient(origCoeff)
	fmt.Printf("  解析系数 = %f (匹配单位: %s)\n", origCoeff, origUnit)

	// 步骤 5: 探测多种 v 命令写入格式（找到设备真正接受的写法）
	fmt.Printf("\n[步骤5] 探测 v 命令写入格式（目标系数 %f）...\n", coeff)
	writeVariants := []string{
		"v01101 " + fmt.Sprintf("%.6f", coeff),       // 文档 3.5.3 示例（单 cc）
		"v01101-05 " + fmt.Sprintf("%.6f", coeff),    // 范围语法（匹配读取）
		"v01101-01 " + fmt.Sprintf("%.6f", coeff),    // 单元素范围
	}
	// 先用一个无害的值（原始系数本身）探测格式，避免误写
	probeWriteCmd := "v01101 " + fmt.Sprintf("%.6f", origCoeff)
	probeWriteResp := queryCmd(conn, reader, probeWriteCmd)
	fmt.Printf("  探测1: %-30s => %q\n", probeWriteCmd, probeWriteResp)

	probeWriteCmd2 := "v01101-05 " + fmt.Sprintf("%.6f", origCoeff)
	probeWriteResp2 := queryCmd(conn, reader, probeWriteCmd2)
	fmt.Printf("  探测2: %-30s => %q\n", probeWriteCmd2, probeWriteResp2)

	probeWriteCmd3 := "v01101-01 " + fmt.Sprintf("%.6f", origCoeff)
	probeWriteResp3 := queryCmd(conn, reader, probeWriteCmd3)
	fmt.Printf("  探测3: %-30s => %q\n", probeWriteCmd3, probeWriteResp3)

	// 选第一个返回 A 的写法
	chosenWrite := ""
	for i, resp := range []string{probeWriteResp, probeWriteResp2, probeWriteResp3} {
		if resp == "A" {
			chosenWrite = writeVariants[i]
			break
		}
	}
	if chosenWrite == "" {
		fmt.Printf("\n[警告] 所有 v 命令探测均未返回 A，无法继续写入测试\n")
		fmt.Printf("\n=== 诊断完成 ===\n")
		return
	}
	fmt.Printf("\n[OK] 选用写入格式: %s\n", chosenWrite)

	// 真正写入新系数
	fmt.Printf("\n[步骤5b] 写入新 EU 系数: %s\n", chosenWrite)
	writeResp := queryCmd(conn, reader, chosenWrite)
	fmt.Printf("  写入响应 => %q\n", writeResp)

	// 步骤 6: 验证
	time.Sleep(200 * time.Millisecond)
	newResp := queryCmd(conn, reader, readCmd)
	fmt.Printf("\n[步骤6] 读回 %s => %q (原始: %q)\n", readCmd, newResp, origResp)

	fmt.Printf("\n[步骤6b] 再次读取 16 通道压力值...\n")
	newPressures, err := readPressures(conn, reader)
	if err != nil {
		fail("读取新压力失败: %v", err)
	}
	printPressures("新压力", newPressures)

	// 步骤 7: 恢复（用同一写入格式 + 原始系数）
	if origResp != "" && !strings.HasPrefix(origResp, "N") {
		// 取 chosenWrite 的前缀（v01101 / v01101-05 / v01101-01）+ 原始系数
		parts := strings.SplitN(chosenWrite, " ", 2)
		restoreCmd := parts[0] + " " + fmt.Sprintf("%.6f", origCoeff)
		fmt.Printf("\n[步骤7] 恢复: %s\n", restoreCmd)
		restoreResp := queryCmd(conn, reader, restoreCmd)
		fmt.Printf("  恢复响应 => %q\n", restoreResp)
	}

	fmt.Printf("\n=== 诊断完成 ===\n")
}

// probeNewlineVariants 探测不同换行符对 u01101 命令的影响
//
// 用户反馈：第三方工具发 u01101 能成功返回 6894.756836，
// 但本程序发 u01101\r\n 返回 N05。怀疑是 \r\n vs 无换行/单 \n 的差异。
// 此函数分别测试三种发送方式，并打印原始字节响应。
func probeNewlineVariants(conn net.Conn, reader *sharedproto.FrameReader) {
	variants := []struct {
		name string
		data []byte
	}{
		{"u01101 (无换行)", []byte("u01101")},
		{"u01101\\n", []byte("u01101\n")},
		{"u01101\\r\\n", []byte("u01101\r\n")},
	}

	for _, v := range variants {
		// 发送
		if err := conn.SetWriteDeadline(time.Now().Add(cmdTimeout)); err != nil {
			fmt.Printf("  %-22s => write-deadline-error: %v\n", v.name, err)
			continue
		}
		n, err := conn.Write(v.data)
		if err != nil {
			fmt.Printf("  %-22s => write-error: %v\n", v.name, err)
			continue
		}
		// 读响应
		if err := conn.SetReadDeadline(time.Now().Add(cmdTimeout)); err != nil {
			fmt.Printf("  %-22s => read-deadline-error: %v\n", v.name, err)
			continue
		}
		payload, err := reader.ReadFrame()
		if err != nil {
			fmt.Printf("  %-22s => read-error: %v (wrote %d bytes)\n", v.name, err, n)
			continue
		}
		fmt.Printf("  %-22s => payload=%q hex=% X\n", v.name, string(payload), payload)
		// 间隔避免命令粘连
		time.Sleep(100 * time.Millisecond)
	}
}

// writeCmd 发送命令（纯 ASCII，不带任何换行符）
//
// 实测发现：设备在 w1601 长度前缀模式下不接受 \r\n 或 \n 作为命令结束符。
// 发送 "u01101\r\n" 或 "u01101\n" 设备返回 N05；
// 发送 "u01101"（无换行）设备正确返回系数。
// 因此所有命令均以纯 ASCII 发送，由设备自行根据命令字符解析边界。
func writeCmd(conn net.Conn, cmd string) error {
	if err := conn.SetWriteDeadline(time.Now().Add(cmdTimeout)); err != nil {
		return err
	}
	_, err := conn.Write([]byte(cmd))
	return err
}

// queryCmd 发送查询命令并返回响应文本（已 trim）
// 失败时返回错误字符串本身（如 "N05" 或 "timeout"）
func queryCmd(conn net.Conn, reader *sharedproto.FrameReader, cmd string) string {
	if err := writeCmd(conn, cmd); err != nil {
		return "send-error:" + err.Error()
	}
	if err := conn.SetReadDeadline(time.Now().Add(cmdTimeout)); err != nil {
		return "deadline-error:" + err.Error()
	}
	payload, err := reader.ReadFrame()
	if err != nil {
		return "read-error:" + err.Error()
	}
	return strings.TrimSpace(string(payload))
}

// readPressures 发送 rFFFF0 并解析 16 通道 ASCII 压力值
func readPressures(conn net.Conn, reader *sharedproto.FrameReader) ([]float64, error) {
	if err := writeCmd(conn, "rFFFF0"); err != nil {
		return nil, fmt.Errorf("send rFFFF0: %w", err)
	}
	if err := conn.SetReadDeadline(time.Now().Add(cmdTimeout)); err != nil {
		return nil, err
	}
	payload, err := reader.ReadFrame()
	if err != nil {
		return nil, fmt.Errorf("read rFFFF0 response: %w", err)
	}
	text := strings.TrimSpace(string(payload))
	if strings.HasPrefix(text, "N") {
		return nil, fmt.Errorf("device error: %s", text)
	}
	parts := strings.Fields(text)
	values := make([]float64, 0, len(parts))
	for _, p := range parts {
		var v float64
		if _, err := fmt.Sscanf(p, "%f", &v); err != nil {
			continue
		}
		values = append(values, v)
	}
	return values, nil
}

// drainW1601 在超时窗口内尽量读出残留帧
func drainW1601(conn net.Conn, reader *sharedproto.FrameReader, window time.Duration) {
	deadline := time.Now().Add(window)
	for time.Now().Before(deadline) {
		_ = conn.SetReadDeadline(time.Now().Add(50 * time.Millisecond))
		_, err := reader.ReadFrame()
		if err != nil {
			return
		}
	}
}

func printPressures(label string, values []float64) {
	fmt.Printf("[%s] 共 %d 个值:\n", label, len(values))
	for i, v := range values {
		chIdx := i + 1
		if chIdx <= 16 {
			fmt.Printf("  CH%02d = %10.4f\n", chIdx, v)
		}
	}
}

func supportedUnits() string {
	keys := make([]string, 0, len(sharedproto.P1604PressureUnitCoefficient))
	for k := range sharedproto.P1604PressureUnitCoefficient {
		keys = append(keys, k)
	}
	return strings.Join(keys, " / ")
}

// parseFirstFloat 从可能包含多个空格分隔值的响应中解析第一个浮点数
func parseFirstFloat(resp string) (float64, error) {
	parts := strings.Fields(strings.TrimSpace(resp))
	if len(parts) == 0 {
		return 0, fmt.Errorf("empty response")
	}
	var f float64
	if _, err := fmt.Sscanf(parts[0], "%f", &f); err != nil {
		return 0, fmt.Errorf("parse %q: %w", parts[0], err)
	}
	return f, nil
}

func fail(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "[FAIL] "+format+"\n", args...)
	os.Exit(1)
}
