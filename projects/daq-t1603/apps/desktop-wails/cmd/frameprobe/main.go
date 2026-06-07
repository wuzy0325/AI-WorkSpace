package main

import (
	"fmt"
	"net"
	"strings"
	"time"

	"shared.local/device-sdk/go/protocol"
)

func main() {
	conn, err := net.DialTimeout("tcp", "192.168.1.10:9000", 5*time.Second)
	if err != nil {
		fmt.Printf("连接失败: %v\n", err)
		return
	}
	defer conn.Close()
	fmt.Println("已连接设备 192.168.1.10:9000")

	// 开启时间戳
	fmt.Println("\n=== 开启时间戳 ===")
	sendCmd(conn, "@fe TIME 1")
	time.Sleep(100 * time.Millisecond)

	// 开始采集
	fmt.Println("\n=== 开始采集 ===")
	sendCmd(conn, "@f0 FFFF 2")
	time.Sleep(500 * time.Millisecond)

	// 用 FrameReader 读取帧
	reader := protocol.NewT1603FrameReader(conn)
	reader.ConsumeOptionalACK(500 * time.Millisecond)

	fmt.Println("\n=== 解析帧 ===")
	for i := 0; i < 5; i++ {
		conn.SetReadDeadline(time.Now().Add(5 * time.Second))
		frame, err := reader.ReadFrame()
		if err != nil {
			fmt.Printf("帧%d 读取失败: %v\n", i+1, err)
			continue
		}

		result, err := protocol.ParseTCPFrameEx(frame)
		if err != nil {
			// 打印更多原始数据帮助调试
			raw := string(frame)
			if len(raw) > 120 {
				raw = raw[:120] + "..."
			}
			fmt.Printf("帧%d 解析失败: %v\n  raw=%q\n", i+1, err, raw)
			continue
		}

		fmt.Printf("帧%d: 时间戳=%.6f 序列号=%d 温度=[%.2f, %.2f, ..., %.2f] (共%d通道)\n",
			i+1, result.HardwareTimestamp, result.SequenceNumber,
			result.Temperatures[0], result.Temperatures[1],
			result.Temperatures[15], len(result.Temperatures))
	}

	// 停止采集
	sendCmd(conn, "@f1")
	time.Sleep(200 * time.Millisecond)

	// 恢复
	sendCmd(conn, "@fe TIME 0")
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
