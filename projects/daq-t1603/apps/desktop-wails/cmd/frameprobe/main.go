package main

import (
	"fmt"
	"net"
	"os"
	"strings"
	"time"

	"shared.local/device-sdk/go/protocol"
)

// globalWatchdogTimeout 是进程级硬 watchdog 超时（ADR-009 P2）。
// frameprobe 读取 5 帧 + 配置开销 < 30s，5 分钟足够覆盖最慢场景。
// watchdog 触发后强制 Close conn + os.Exit(2)，避免 sendCmdIdle 的 50ms 短 deadline 循环
// 在故障 Windows 上 hang 住（Go issue #70395/#34385）。
const globalWatchdogTimeout = 5 * time.Minute

func main() {
	conn, err := net.DialTimeout("tcp", "192.168.1.10:9000", 5*time.Second)
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
	fmt.Println("已连接设备 192.168.1.10:9000")

	// 模拟 syncHardwareConfig：查询配置并归一化
	fmt.Println("\n=== 查询并归一化配置 ===")
	sendCmd(conn, "@e3")       // 16 位热电偶类型
	sendCmdIdle(conn, "@fd MCH") // 通道掩码
	sendCmdIdle(conn, "@fd SPS") // 采样间隔
	sendCmdExact(conn, "@fd BIN", 1)  // 当前二进制模式
	sendCmdExact(conn, "@fd TIME", 1) // 时间戳
	sendCmdExact(conn, "@fd HEAD", 1) // 序列号
	sendCmd(conn, "@fe BIN 1")   // ★ 归一化为二进制
	sendCmd(conn, "@fe TIME 0")  // 关闭时间戳
	sendCmd(conn, "@fe HEAD 0")  // 关闭序列号
	time.Sleep(100 * time.Millisecond)

	// 停止可能正在进行的采集，排空缓冲区
	sendCmdNoWait(conn, "@f1")
	time.Sleep(200 * time.Millisecond)
	drainConn(conn, 100*time.Millisecond)
	fmt.Println("\n=== 残留数据排空完成 ===")

	// 开始采集（二进制模式 64 字节/帧）
	fmt.Println("\n=== 开始采集（二进制 BIN=1）===")
	conn.SetWriteDeadline(time.Now().Add(3 * time.Second))
	conn.Write([]byte("@f0 FFFF 2\n"))
	conn.SetWriteDeadline(time.Time{})
	time.Sleep(300 * time.Millisecond)

	reader := protocol.NewT1603FrameReader(conn)
	reader.SetBinaryMode(true)
	reader.ConsumeOptionalACK(500 * time.Millisecond)

	fmt.Println("\n=== 解析二进制帧（64 字节，float32 LE，已反序）===")
	for i := 0; i < 5; i++ {
		conn.SetReadDeadline(time.Now().Add(5 * time.Second))
		frame, err := reader.ReadFrame()
		if err != nil {
			fmt.Printf("帧%d 读取失败: %v\n", i+1, err)
			continue
		}

		result, err := protocol.ParseTCPFrameEx(frame)
		if err != nil {
			fmt.Printf("帧%d 解析失败: %v  raw=% X\n", i+1, err, frame)
			continue
		}

		// 打印所有 16 通道的温度值
		fmt.Printf("帧%d (%d 字节):\n", i+1, len(frame))
		for j := 0; j < 16; j++ {
			fmt.Printf("  CH%02d: %.3f °C", j+1, result.Temperatures[j])
			if (j+1)%4 == 0 {
				fmt.Println()
			}
		}
		fmt.Println()
	}

	// 停止采集
	sendCmdNoWait(conn, "@f1")
	time.Sleep(200 * time.Millisecond)
	fmt.Println("\n完成")
}

func sendCmd(conn net.Conn, cmd string) {
	conn.SetWriteDeadline(time.Now().Add(3 * time.Second))
	conn.Write([]byte(cmd + "\n"))
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	buf := make([]byte, 256)
	n, _ := conn.Read(buf)
	if n > 0 {
		resp := strings.TrimSpace(string(buf[:n]))
		if len(resp) > 40 {
			resp = resp[:40] + "..."
		}
		fmt.Printf("  %s -> %q\n", cmd, resp)
	}
}

func sendCmdExact(conn net.Conn, cmd string, n int) {
	conn.SetWriteDeadline(time.Now().Add(3 * time.Second))
	conn.Write([]byte(cmd + "\n"))
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	buf := make([]byte, n)
	_, err := conn.Read(buf)
	if err != nil {
		fmt.Printf("  %s -> 读取%d字节失败: %v\n", cmd, n, err)
		return
	}
	resp := strings.TrimSpace(string(buf))
	fmt.Printf("  %s -> %q\n", cmd, resp)
}

func sendCmdIdle(conn net.Conn, cmd string) {
	conn.SetWriteDeadline(time.Now().Add(3 * time.Second))
	conn.Write([]byte(cmd + "\n"))
	var buf strings.Builder
	one := make([]byte, 1)
	for {
		conn.SetReadDeadline(time.Now().Add(50 * time.Millisecond))
		n, err := conn.Read(one)
		if err != nil || n == 0 {
			break
		}
		if one[0] == '\n' {
			break
		}
		buf.WriteByte(one[0])
	}
	resp := strings.TrimSpace(buf.String())
	if resp != "" {
		fmt.Printf("  %s -> %q\n", cmd, resp)
	} else {
		fmt.Printf("  %s -> (无响应)\n", cmd)
	}
}

func sendCmdNoWait(conn net.Conn, cmd string) {
	conn.SetWriteDeadline(time.Now().Add(3 * time.Second))
	conn.Write([]byte(cmd + "\n"))
	conn.SetWriteDeadline(time.Time{})
	fmt.Printf("  %s (不等待响应)\n", cmd)
}

func drainConn(conn net.Conn, timeout time.Duration) {
	buf := make([]byte, 4096)
	totalDrained := 0
	for i := 0; i < 20; i++ {
		conn.SetReadDeadline(time.Now().Add(timeout))
		n, err := conn.Read(buf)
		if n > 0 {
			totalDrained += n
		}
		if err != nil {
			break
		}
	}
	conn.SetReadDeadline(time.Time{})
	if totalDrained > 0 {
		fmt.Printf("  排空了 %d 字节残留数据\n", totalDrained)
	}
}
