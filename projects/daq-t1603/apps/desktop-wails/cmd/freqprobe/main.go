package main

import (
	"fmt"
	"math"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	"shared.local/device-sdk/go/protocol"
)

// 频率测试工具：验证 DAQ-T-1603 设备频率设置是否生效，以及采集数据是否按设定频率到达
// SPS 参数含义：采集间隔时间（毫秒），实际频率 = 1000 / SPS
// 用法: go run ./cmd/freqprobe

const (
	deviceHost     = "192.168.1.10"
	devicePort     = 9000
	collectSeconds = 5
	// globalWatchdogTimeout 是进程级硬 watchdog 超时（ADR-009 P2）。
	// 8 个 SPS 值 × 5s 采集 + 配置开销 ≈ 50s，5 分钟足够覆盖最慢场景。
	// watchdog 触发后强制 Close conn + os.Exit(2)，避免 SetReadDeadline 失效导致 main 挂起。
	globalWatchdogTimeout = 5 * time.Minute
)

// 测试的 SPS 间隔列表（毫秒），对应频率 = 1000/SPS
var testSPSValues = []int{1, 2, 5, 10, 50, 100, 500, 1000}

func main() {
	// 用 net.JoinHostPort 拼接地址，正确处理 IPv6 主机（自动加方括号）
	addr := net.JoinHostPort(deviceHost, strconv.Itoa(devicePort))
	fmt.Printf("=== DAQ-T-1603 频率测试工具 ===\n")
	fmt.Printf("目标设备: %s\n", addr)
	fmt.Printf("SPS 含义: 采集间隔(毫秒), 实际频率 = 1000/SPS\n\n")

	conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
	if err != nil {
		fmt.Printf("连接失败: %v\n", err)
		return
	}
	defer conn.Close()
	// 进程级硬 watchdog（ADR-009 P2）：5 分钟超时后强制 Close conn + os.Exit(2)。
	// 退出码 2 = watchdog 硬超时，区别于连接失败的 return，便于脚本区分。
	// 保存 timer 句柄并在 main 正常返回前 Stop，避免长生命周期场景下 watchdog 误触发。
	wd := time.AfterFunc(globalWatchdogTimeout, func() {
		fmt.Fprintf(os.Stderr, "\n[watchdog] 全局超时 %v 触发，强制关闭连接并退出\n", globalWatchdogTimeout)
		_ = conn.Close()
		os.Exit(2)
	})
	defer wd.Stop()
	fmt.Println("已连接设备")

	fmt.Println("\n--- 查询当前设备配置 ---")
	queryConfig(conn)

	// 逐个 SPS 值测试
	for _, sps := range testSPSValues {
		expectedHz := 1000.0 / float64(sps)
		fmt.Printf("\n========== 测试 SPS=%d (理论频率 %.1f Hz) ==========\n", sps, expectedHz)
		testFrequency(sps, conn)
	}

	// 恢复默认 SPS=10（100Hz）
	fmt.Println("\n--- 恢复默认 SPS=10 (100Hz) ---")
	sendCmd(conn, "@fe SPS 10")
	time.Sleep(100 * time.Millisecond)
	queryConfig(conn)

	fmt.Println("\n=== 测试完成 ===")
}

// testFrequency 测试指定 SPS 间隔的设置和数据采集
func testFrequency(sps int, conn net.Conn) {
	expectedHz := 1000.0 / float64(sps)

	// 步骤1: 停止可能正在进行的采集
	sendCmdNoWait(conn, "@f1")
	time.Sleep(200 * time.Millisecond)
	drainConn(conn, 100*time.Millisecond)

	// 步骤2: 设置 SPS（采集间隔毫秒）
	fmt.Printf("  设置采集间隔: @fe SPS %d (理论频率 %.1f Hz)\n", sps, expectedHz)
	resp, err := sendCmd(conn, fmt.Sprintf("@fe SPS %d", sps))
	if err != nil {
		fmt.Printf("  设置失败: %v\n", err)
		return
	}
	fmt.Printf("  设备响应: %q\n", resp)
	time.Sleep(100 * time.Millisecond)

	// 步骤3: 读回 SPS 确认
	readbackResp, err := sendCmdIdle(conn, "@fd SPS")
	if err != nil {
		fmt.Printf("  读回失败: %v\n", err)
	} else {
		readbackResp = strings.TrimSpace(readbackResp)
		fmt.Printf("  读回 SPS: %s ms\n", readbackResp)
		if readbackResp != fmt.Sprintf("%d", sps) {
			fmt.Printf("  [警告] 读回值(%s)与设置值(%d)不匹配!\n", readbackResp, sps)
		} else {
			fmt.Printf("  [OK] SPS 设置成功确认\n")
		}
	}

	// 步骤4: ASCII 模式，无时间戳/序列号，帧大小固定192字节
	sendCmd(conn, "@fe TIME 0")
	sendCmd(conn, "@fe HEAD 0")
	sendCmd(conn, "@fe BIN 0")
	time.Sleep(100 * time.Millisecond)

	// 步骤5: 开始采集
	fmt.Printf("  开始采集 (%d秒, ASCII模式)...\n", collectSeconds)
	sendCmdNoWait(conn, "@f0 FFFF 2")
	time.Sleep(300 * time.Millisecond)

	reader := protocol.NewT1603FrameReader(conn)
	reader.SetBinaryMode(false)
	reader.ConsumeOptionalACK(500 * time.Millisecond)

	// 步骤6: 采集数据并记录时间戳
	type frameRecord struct {
		timestamp time.Time
		rawSize   int
		data      []float64
	}
	var frames []frameRecord
	rawSizeDist := make(map[int]int)
	deadline := time.Now().Add(time.Duration(collectSeconds) * time.Second)
	timeoutCount := 0
	parseErrors := 0

	for time.Now().Before(deadline) {
		conn.SetReadDeadline(time.Now().Add(2 * time.Second))
		frame, err := reader.ReadFrame()
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				timeoutCount++
				if timeoutCount > 3 {
					break
				}
				continue
			}
			if err == protocol.ErrIncompleteFrame {
				continue
			}
			fmt.Printf("  读取帧错误: %v\n", err)
			break
		}

		temps, err := protocol.ParseTCPFrame(frame)
		if err != nil {
			parseErrors++
			if parseErrors <= 3 {
				fmt.Printf("  解析帧错误: %v (原始长度=%d)\n", err, len(frame))
			}
			continue
		}

		rawSizeDist[len(frame)]++
		frames = append(frames, frameRecord{
			timestamp: time.Now(),
			rawSize:   len(frame),
			data:      temps,
		})
	}

	// 步骤7: 停止采集
	sendCmdNoWait(conn, "@f1")
	time.Sleep(200 * time.Millisecond)
	drainConn(conn, 100*time.Millisecond)

	// 步骤8: 分析结果
	fmt.Printf("\n  --- 采集结果分析 ---\n")
	fmt.Printf("  采集帧数: %d (解析错误: %d)\n", len(frames), parseErrors)

	for size, count := range rawSizeDist {
		fmt.Printf("  帧大小: %d 字节 × %d 帧\n", size, count)
	}

	if len(frames) < 2 {
		fmt.Printf("  [失败] 帧数不足\n")
		return
	}

	// 计算帧间隔统计
	intervals := make([]float64, 0, len(frames)-1)
	for i := 1; i < len(frames); i++ {
		intervals = append(intervals, frames[i].timestamp.Sub(frames[i-1].timestamp).Seconds())
	}

	var sumInterval float64
	minInterval := math.MaxFloat64
	maxInterval := 0.0
	for _, iv := range intervals {
		sumInterval += iv
		if iv < minInterval {
			minInterval = iv
		}
		if iv > maxInterval {
			maxInterval = iv
		}
	}
	avgInterval := sumInterval / float64(len(intervals))
	actualHz := 1.0 / avgInterval

	// 跳过前3帧（启动延迟），计算稳定区间
	skipFrames := 3
	var stableHz float64
	var stableStdDevMs float64
	if len(intervals) > skipFrames*2 {
		stableIntervals := intervals[skipFrames:]
		var stableSum float64
		for _, iv := range stableIntervals {
			stableSum += iv
		}
		stableAvg := stableSum / float64(len(stableIntervals))
		stableHz = 1.0 / stableAvg

		var stableSumSqDiff float64
		for _, iv := range stableIntervals {
			diff := iv - stableAvg
			stableSumSqDiff += diff * diff
		}
		stableStdDevMs = math.Sqrt(stableSumSqDiff/float64(len(stableIntervals))) * 1000

		fmt.Printf("  稳定区间 (跳过前%d帧):\n", skipFrames)
		fmt.Printf("    稳定帧数: %d\n", len(stableIntervals))
		fmt.Printf("    平均间隔: %.3f ms\n", stableAvg*1000)
		fmt.Printf("    实际频率: %.2f Hz\n", stableHz)
		fmt.Printf("    间隔标准差: %.3f ms\n", stableStdDevMs)
	}

	fmt.Printf("\n  全部帧统计:\n")
	fmt.Printf("    总帧数: %d\n", len(frames))
	fmt.Printf("    平均间隔: %.3f ms\n", avgInterval*1000)
	fmt.Printf("    最小间隔: %.3f ms\n", minInterval*1000)
	fmt.Printf("    最大间隔: %.3f ms\n", maxInterval*1000)
	fmt.Printf("    实际频率: %.2f Hz\n", actualHz)

	// 与理论频率比较
	compareHz := actualHz
	if stableHz > 0 {
		compareHz = stableHz
	}
	rateError := math.Abs(compareHz-expectedHz) / expectedHz * 100
	fmt.Printf("    SPS设置: %d ms (理论频率 %.1f Hz)\n", sps, expectedHz)
	fmt.Printf("    频率偏差: %.1f%%\n", rateError)

	if rateError < 10 {
		fmt.Printf("  [OK] 实际频率与理论频率匹配 (偏差 < 10%%)\n")
	} else if rateError < 20 {
		fmt.Printf("  [警告] 实际频率与理论频率偏差较大 (10%% ~ 20%%)\n")
	} else {
		fmt.Printf("  [失败] 实际频率与理论频率严重不匹配 (偏差 > 20%%)\n")
	}

	// 打印前3帧数据样例
	fmt.Printf("\n  数据样例 (前3帧):\n")
	for i := 0; i < len(frames) && i < 3; i++ {
		temps := frames[i].data
		fmt.Printf("    帧%d: %s\n", i+1, formatTemps(temps))
	}
}

// queryConfig 查询设备当前配置
func queryConfig(conn net.Conn) {
	if resp, err := sendCmdIdle(conn, "@fd SPS"); err == nil {
		spsVal := strings.TrimSpace(resp)
		fmt.Printf("  采集间隔(SPS): %s ms\n", spsVal)
	}
	if resp, err := sendCmdIdle(conn, "@fd MCH"); err == nil {
		fmt.Printf("  通道掩码(MCH): %s\n", strings.TrimSpace(resp))
	}
	if resp, err := sendCmdIdle(conn, "@fd AVG"); err == nil {
		fmt.Printf("  平均次数(AVG): %s\n", strings.TrimSpace(resp))
	}
	if resp, err := sendCmdExact(conn, "@e3", 16); err == nil {
		fmt.Printf("  热电偶类型: %s\n", resp)
	}
}

func sendCmd(conn net.Conn, cmd string) (string, error) {
	return protocol.SendCommand(conn, cmd)
}

func sendCmdIdle(conn net.Conn, cmd string) (string, error) {
	return protocol.SendCommandIdle(conn, cmd, 30*time.Millisecond)
}

func sendCmdExact(conn net.Conn, cmd string, n int) (string, error) {
	return protocol.SendCommandExact(conn, cmd, n)
}

func sendCmdNoWait(conn net.Conn, cmd string) {
	conn.SetWriteDeadline(time.Now().Add(3 * time.Second))
	conn.Write([]byte(cmd + "\n"))
	conn.SetWriteDeadline(time.Time{})
}

func drainConn(conn net.Conn, timeout time.Duration) {
	buf := make([]byte, 4096)
	for i := 0; i < 20; i++ {
		conn.SetReadDeadline(time.Now().Add(timeout))
		n, err := conn.Read(buf)
		if n == 0 || err != nil {
			break
		}
	}
	conn.SetReadDeadline(time.Time{})
}

func formatTemps(temps []float64) string {
	if len(temps) == 0 {
		return "[]"
	}
	if len(temps) <= 4 {
		parts := make([]string, len(temps))
		for i, t := range temps {
			parts[i] = fmt.Sprintf("%.2f", t)
		}
		return fmt.Sprintf("[%s]", strings.Join(parts, ", "))
	}
	return fmt.Sprintf("[%.2f, %.2f, ..., %.2f] (共%d通道)", temps[0], temps[1], temps[len(temps)-1], len(temps))
}
