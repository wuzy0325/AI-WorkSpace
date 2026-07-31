// p1604-ts-diag-v2 连接 DAQ-P-1604 设备，读取 1000Hz 数据帧，
// 打印原始设备时间戳（秒 + 纳秒 fractional），验证每一帧是否有唯一时间戳。
//
// 用法（GOWORK=off）:
//
//	cd programs/p1604-ts-diag && $env:GOWORK="off"; go run . -host 192.168.1.7 -port 9000 -n 200
package main

import (
	"encoding/binary"
	"flag"
	"fmt"
	"math"
	"net"
	"os"
	"strconv"
	"time"

	sharedproto "shared.local/device-sdk/go/protocol"
)

// globalWatchdogTimeout 是进程级硬 watchdog 超时（ADR-009 P2）。
//
// 设计依据：诊断工具的 SetReadDeadline 在故障 Windows 电脑上可能失效，
// ReadFrame 永久阻塞导致 main 挂起。watchdog 在超时后强制 Close conn 解除阻塞，
// 并 os.Exit(2) 退出，避免僵尸进程。
// 取值 5 分钟：默认 200 帧 × 1ms ≈ 0.2s + 配置开销 < 5s，5 分钟足够覆盖最慢场景。
const globalWatchdogTimeout = 5 * time.Minute

func main() {
	host := flag.String("host", "192.168.1.7", "设备 IP")
	port := flag.Int("port", 9000, "设备 TCP 端口")
	numFrames := flag.Int("n", 200, "读取帧数")
	periodMs := flag.Int("p", 1, "采样周期 ms (1=1000Hz)")
	flag.Parse()

	addr := net.JoinHostPort(*host, strconv.Itoa(*port))
	fmt.Printf("连接 %s (采样周期=%dms, 读取%d帧)...\n", addr, *periodMs, *numFrames)
	// ADR-009 R2-1：改用 sharedproto.DialTCP 替代 net.DialTimeout。
	// net.DialTimeout 依赖 Dial 内部 deadline，Windows 故障机器 deadline 不可靠时
	// Dial 可能永远不返回，工具启动即卡死。sharedproto.DialTCP 内部用 goroutine +
	// time.After 软超时 + abandoned 信号，主线程在 timeout 后立即返回，晚到 conn
	// 被 Close 不泄漏（R1-4 整改保证）。
	conn, err := sharedproto.DialTCP(addr, "", 5*time.Second)
	if err != nil {
		fmt.Fprintf(os.Stderr, "连接失败: %v\n", err)
		os.Exit(1)
	}
	defer conn.Close()
	// 进程级硬 watchdog（ADR-009 P2）：超时后强制 Close conn + os.Exit(2)。
	// 退出码 2 = watchdog 硬超时，区别于 os.Exit(1)（连接失败等一般错误），便于脚本区分。
	// 保存 timer 句柄并在 main 正常返回前 Stop，避免长生命周期场景下 watchdog 误触发。
	wd := time.AfterFunc(globalWatchdogTimeout, func() {
		fmt.Fprintf(os.Stderr, "\n[watchdog] 全局超时 %v 触发，强制关闭连接并退出\n", globalWatchdogTimeout)
		_ = conn.Close()
		os.Exit(2)
	})
	defer wd.Stop()
	fmt.Println("✅ TCP 已连接")

	fr := sharedproto.NewFrameReader(conn)

	// 第1步: w1601 启用长度前缀模式
	fmt.Println("[1/4] w1601 ...")
	if err := sendCmd(conn, "w1601"); err != nil {
		fmt.Fprintf(os.Stderr, "w1601 失败: %v\n", err)
		os.Exit(1)
	}
	if err := sharedproto.P1604ReadCommandACK(fr, conn, 2*time.Second); err != nil {
		fmt.Fprintf(os.Stderr, "w1601 应答失败: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("  ✅ 长度前缀已启用")

	coeff, err := sharedproto.P1604ReadUnitCoefficient(fr, conn, 2*time.Second)
	if err != nil {
		fmt.Fprintf(os.Stderr, "u01101 失败: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("  ✅ u01101 系数: %.6f\n", coeff)
	if err := sharedproto.P1604WriteUnitCoefficient(fr, conn, coeff, 2*time.Second); err != nil {
		fmt.Fprintf(os.Stderr, "v01101 原值写回失败: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("  ✅ v01101 原值写回 ACK")

	// 第2步: 配置采集参数
	fmt.Println("[2/4] c 00 (配置采集参数) ...")
	cmd := fmt.Sprintf("c 00 1 FFFF 1 %d 7 0", *periodMs)
	if err := sendCmd(conn, cmd); err != nil {
		fmt.Fprintf(os.Stderr, "c 00 失败: %v\n", err)
		os.Exit(1)
	}
	if err := sharedproto.P1604ReadCommandACK(fr, conn, 2*time.Second); err != nil {
		fmt.Fprintf(os.Stderr, "c 00 应答失败: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("  ✅")

	// 第3步: 配置流内容 (压力+时间戳+大气)
	fmt.Println("[3/4] c 05 (时间戳+大气) ...")
	if err := sendCmd(conn, "c 05 1 0C10"); err != nil {
		fmt.Fprintf(os.Stderr, "c 05 失败: %v\n", err)
		os.Exit(1)
	}
	if err := sharedproto.P1604ReadCommandACK(fr, conn, 2*time.Second); err != nil {
		fmt.Fprintf(os.Stderr, "c 05 应答失败: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("  ✅")

	// 第4步: 启动采集
	fmt.Println("[4/4] c 01 (启动) ...")
	if err := sendCmd(conn, "c 01 1"); err != nil {
		fmt.Fprintf(os.Stderr, "c 01 失败: %v\n", err)
		os.Exit(1)
	}
	if err := sharedproto.P1604ReadCommandACK(fr, conn, 2*time.Second); err != nil {
		fmt.Fprintf(os.Stderr, "c 01 应答失败: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("  ✅ 采集已启动")
	fmt.Println()

	// wall clock 计时：用于对比"设备时间戳跨度"与"系统时间跨度"
	// 如果两者一致 → 设备时间戳真实（设备确实以该频率采集）
	// 如果系统时间远大于设备时间戳 → 时间戳伪造（设备实际频率低于时间戳显示的频率）
	wallStart := time.Now()
	firstFrameSec := uint32(0)
	firstFrameFrac := uint32(0)
	lastFrameSec := uint32(0)
	lastFrameFrac := uint32(0)
	// 记录首帧的 wall clock，用于逐帧对比"设备时间戳"与"系统时间"
	var firstFrameWall time.Time

	// 表头加入 wall clock 列与偏差列
	fmt.Printf("%-6s %-12s %-12s %-10s %-10s %-12s %-12s\n",
		"帧号", "秒(BE)", "fractional(BE)", "设备us", "与上帧差us", "wall ms", "wall-设备 ms")
	fmt.Println("------ ------------ ------------ ---------- ---------- ------------ ------------")

	prevSec := uint32(0)
	prevFrac := uint32(0)
	totalDup := 0
	readCount := 0

	for readCount < *numFrames {
		conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
		data, err := fr.ReadFrame()
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				fmt.Fprintf(os.Stderr, "\n⏱ 读取超时 (已读 %d 帧)\n", readCount)
				break
			}
			fmt.Fprintf(os.Stderr, "\nReadFrame 错误（已读 %d 帧）: %v\n", readCount, err)
			break
		}
		if sharedproto.IsASCIIFrame(data) {
			continue
		}

		const headerSize = 5
		const numPressure = 16
		tsOffset := headerSize + numPressure*4
		expectedLen := tsOffset + 8 + 2*4
		if len(data) != expectedLen {
			continue
		}

		seconds := binary.BigEndian.Uint32(data[tsOffset:])
		fractional := binary.BigEndian.Uint32(data[tsOffset+4:])

		// 同时用 LittleEndian 解析，验证字节序假设
		secondsLE := binary.LittleEndian.Uint32(data[tsOffset:])
		fractionalLE := binary.LittleEndian.Uint32(data[tsOffset+4:])

		nsFloat := float64(fractional) / float64(0x100000000) * 1e9
		nsUint := uint32(math.Round(nsFloat))
		us := nsUint / 1000

		// 当前帧的 wall clock（接收时刻）
		wallNow := time.Now()
		if readCount == 0 {
			firstFrameWall = wallNow
		}

		readCount++

		// 记录首帧与末帧的设备时间戳，用于计算设备时间戳总跨度
		if readCount == 1 {
			firstFrameSec = seconds
			firstFrameFrac = fractional
		}
		lastFrameSec = seconds
		lastFrameFrac = fractional

		// 计算与上一帧的微秒差
		diffUs := int64(0)
		if readCount > 1 {
			prevTotalNs := uint64(prevSec)*1e9 + uint64(math.Round(float64(prevFrac)/float64(0x100000000)*1e9))
			currTotalNs := uint64(seconds)*1e9 + uint64(nsFloat)
			diffUs = int64(currTotalNs-prevTotalNs) / 1000
		}

		// wall clock 相对首帧的偏移（ms）
		wallElapsedMs := float64(wallNow.Sub(firstFrameWall).Microseconds()) / 1000.0
		// 设备时间戳相对首帧的偏移（ms）
		firstDeviceNs := float64(firstFrameSec)*1e9 + float64(firstFrameFrac)/float64(0x100000000)*1e9
		currDeviceNs := float64(seconds)*1e9 + float64(fractional)/float64(0x100000000)*1e9
		deviceElapsedMs := (currDeviceNs - firstDeviceNs) / 1e6
		// 偏差：wall - device。若设备时间戳真实，此值应稳定在 0 附近
		// 若偏差持续增长（wall > device），说明设备时间戳走得慢于真实时间（伪造）
		deviationMs := wallElapsedMs - deviceElapsedMs

		// 首帧额外打印原始字节与 LE 解析值，便于人工核对字节序
		if readCount == 1 {
			fmt.Printf("[首帧原始字节 tsOffset..tsOffset+8] %02X %02X %02X %02X | %02X %02X %02X %02X\n",
				data[tsOffset], data[tsOffset+1], data[tsOffset+2], data[tsOffset+3],
				data[tsOffset+4], data[tsOffset+5], data[tsOffset+6], data[tsOffset+7])
			fmt.Printf("[BE] sec=%d frac=0x%08X -> us=%d\n", seconds, fractional, us)
			fmt.Printf("[LE] sec=%d frac=0x%08X -> us=%d\n",
				secondsLE, fractionalLE,
				uint32(math.Round(float64(fractionalLE)/float64(0x100000000)*1e9))/1000)
			fmt.Println()
		}

		if readCount <= 30 || readCount > *numFrames-5 || readCount%100 == 0 {
			fmt.Printf("%-6d %-12d 0x%08X %-10d %-10d %-12.3f %-12.3f\n",
				readCount, seconds, fractional, us, diffUs, wallElapsedMs, deviationMs)
		}

		// 检测重复：连续两帧的秒+fractional完全相同
		if readCount > 1 && prevSec == seconds && prevFrac == fractional {
			totalDup++
		}

		prevSec = seconds
		prevFrac = fractional
	}

	if err := sendCmd(conn, "c 02 1"); err != nil {
		fmt.Fprintf(os.Stderr, "c 02 发送失败: %v\n", err)
		os.Exit(1)
	}
	if err := sharedproto.P1604ReadCommandACK(fr, conn, 2*time.Second); err != nil {
		fmt.Fprintf(os.Stderr, "c 02 应答失败: %v\n", err)
		os.Exit(1)
	}
	wallEnd := time.Now()
	fmt.Println()

	fmt.Printf("=== 统计结果 ===\n")
	fmt.Printf("成功读取帧数:    %d\n", readCount)
	fmt.Printf("连续重复时间戳:  %d\n", totalDup)
	fmt.Printf("唯一时间戳比例:  %.1f%%\n", float64(readCount-totalDup)/float64(readCount)*100)

	// 关键诊断：对比"设备时间戳跨度"与"系统时间跨度"
	// 两者一致 → 设备时间戳是真实的，设备确实以该频率采集
	// 系统时间 > 设备时间戳 → 时间戳被伪造（设备实际频率低于时间戳显示值）
	if readCount > 1 {
		firstNs := float64(firstFrameSec)*1e9 + float64(firstFrameFrac)/float64(0x100000000)*1e9
		lastNs := float64(lastFrameSec)*1e9 + float64(lastFrameFrac)/float64(0x100000000)*1e9
		deviceElapsedMs := (lastNs - firstNs) / 1e6
		wallElapsedMs := float64(wallEnd.Sub(wallStart).Microseconds()) / 1000.0
		deviceRate := float64(readCount-1) / (deviceElapsedMs / 1000.0)
		wallRate := float64(readCount-1) / (wallElapsedMs / 1000.0)
		ratio := wallElapsedMs / deviceElapsedMs

		fmt.Println()
		fmt.Println("=== 时间戳真实性验证 ===")
		fmt.Printf("设备时间戳跨度:  %.3f ms\n", deviceElapsedMs)
		fmt.Printf("系统时间跨度:    %.3f ms\n", wallElapsedMs)
		fmt.Printf("系统/设备比率:    %.2f x\n", ratio)
		fmt.Printf("设备时间戳频率:  %.1f Hz\n", deviceRate)
		fmt.Printf("系统实测频率:    %.1f Hz\n", wallRate)
		fmt.Println()
		if ratio > 2.0 {
			fmt.Println("❌ 时间戳被伪造！设备实际频率远低于时间戳显示值")
			fmt.Printf("   设备时间戳声称 %.1f Hz，但系统实测只有 %.1f Hz\n", deviceRate, wallRate)
			fmt.Printf("   设备每帧时间戳增量 %.0fμs，但实际间隔 %.0fμs\n",
				deviceElapsedMs*1000/float64(readCount-1), wallElapsedMs*1000/float64(readCount-1))
		} else if ratio < 0.5 {
			fmt.Println("⚠️  系统时间小于设备时间戳跨度（可能丢帧或读取过慢）")
		} else {
			fmt.Println("✅ 设备时间戳与系统时间一致，时间戳是真实的")
			fmt.Printf("   设备确实以 %.1f Hz 采集（per=%d）\n", wallRate, *periodMs)
		}
	}
}

func sendCmd(conn net.Conn, cmd string) error {
	return sharedproto.SendCommandNoNewline(conn, cmd, 2*time.Second)
}
