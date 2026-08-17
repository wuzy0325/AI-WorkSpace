// stopstartprobe 精确模拟生产代码 Stop → Start 路径。
//
// 目标：用真实的 FrameReader + ExpectControlACK/ExpectControlACKAfterFrames
// 模拟 readLoop 行为，验证 misalignment 是否可复现。
package main

import (
	"fmt"
	"net"
	"os"
	"strings"
	"time"

	"shared.local/device-sdk/go/protocol"
)

const globalWatchdogTimeout = 5 * time.Minute

func main() {
	conn, err := protocol.DialTCP("192.168.1.10:9000", "", 5*time.Second)
	if err != nil {
		fmt.Printf("连接失败: %v\n", err)
		return
	}
	defer conn.Close()
	wd := time.AfterFunc(globalWatchdogTimeout, func() {
		fmt.Fprintf(os.Stderr, "\n[watchdog] 全局超时触发\n")
		_ = conn.Close()
		os.Exit(2)
	})
	defer wd.Stop()
	fmt.Println("=== 精确模拟生产代码 Stop → Start 流程 ===")

	// 归一化配置
	sendCmd(conn, "@e3")
	sendCmdIdle(conn, "@fd MCH")
	sendCmdExact(conn, "@fd BIN", 1)
	sendCmd(conn, "@fe BIN 1")
	sendCmd(conn, "@fe TIME 0")
	sendCmd(conn, "@fe HEAD 0")
	time.Sleep(100 * time.Millisecond)
	sendCmdNoWait(conn, "@f1")
	time.Sleep(200 * time.Millisecond)
	strictDrain(conn)
	fmt.Println("\n--- 归一化完成 ---")

	for round := 1; round <= 10; round++ {
		fmt.Printf("=== 第 %d 轮 ===\n", round)

		// ==== 模拟 StartAcquisition ====
		fr := protocol.NewT1603FrameReader(conn)
		fr.SetBinaryMode(true)
		fr.Reset()
		fr.ExpectControlACK() // ackAfterData = [false]

		writeCmd(conn, "@f0 FFFF 2")

		// 模拟 readLoop 第一轮：ReadFrame 消费 ACK
		conn.SetReadDeadline(time.Now().Add(5 * time.Second))
		_, err := fr.ReadFrame()
		if err != nil {
			if err == protocol.ErrControlACK {
				fmt.Printf("[Round %d] ACK consumed OK\n", round)
			} else {
				fmt.Printf("[Round %d] ★ 首帧(ACK)失败: %v ★\n", round, err)
				cleanup(conn)
				continue
			}
		} else {
			fmt.Printf("[Round %d] ★ 首帧未返回 ErrControlACK，直接返回了数据帧 ★\n", round)
		}

		// 模拟 readLoop 第二轮：ReadFrame 读第一帧数据
		conn.SetReadDeadline(time.Now().Add(5 * time.Second))
		frame, err := fr.ReadFrame()
		if err != nil {
			fmt.Printf("[Round %d] ★★★ 第一帧失败: %v ★★★\n", round, err)
		} else {
			result, parseErr := protocol.ParseTCPFrameEx(frame)
			if parseErr != nil {
				fmt.Printf("[Round %d] ★★★ 第一帧解析失败: %v ★★★\n", round, parseErr)
			} else {
				nonZero := 0
				for _, t := range result.Temperatures {
					if t != 0 {
						nonZero++
					}
				}
				fmt.Printf("[Round %d] 第一帧 OK: %d 通道有值, CH15=%.2f\n", round, nonZero, result.Temperatures[15])
			}
		}

		// ==== 模拟 StopAcquisition ====
		fr.ExpectControlACKAfterFrames() // ackAfterData = [true]
		writeCmd(conn, "@f1")

		// 读帧直到遇到 ACK
		for {
			conn.SetReadDeadline(time.Now().Add(5 * time.Second))
			_, err := fr.ReadFrame()
			if err == nil {
				continue // 数据帧，继续读
			}
			if err == protocol.ErrControlACK {
				fmt.Printf("[Round %d] Stop ACK consumed OK\n", round)
				break
			}
			fmt.Printf("[Round %d] ★ Stop 读帧异常: %v ★\n", round, err)
			break
		}
		// 严格排空
		residual := strictDrain(conn)
		if residual > 0 {
			fmt.Printf("[Round %d] ★ Stop 后排空 %d 字节残留 ★\n", round, residual)
		}
		fmt.Printf("\n")
	}

	fmt.Println("=== 全部 10 轮完成 ===")
}

func cleanup(conn net.Conn) {
	writeCmd(conn, "@f1")
	time.Sleep(200 * time.Millisecond)
	strictDrain(conn)
}

func strictDrain(conn net.Conn) int {
	buf := make([]byte, 4096)
	total := 0
	emptyCount := 0
	for emptyCount < 2 {
		conn.SetReadDeadline(time.Now().Add(50 * time.Millisecond))
		n, err := conn.Read(buf)
		if n > 0 {
			total += n
			emptyCount = 0
		} else {
			emptyCount++
		}
		if err != nil {
			emptyCount++
		}
	}
	conn.SetReadDeadline(time.Time{})
	return total
}

func writeCmd(conn net.Conn, cmd string) {
	conn.SetWriteDeadline(time.Now().Add(3 * time.Second))
	conn.Write([]byte(cmd + "\n"))
	conn.SetWriteDeadline(time.Time{})
}

func sendCmd(conn net.Conn, cmd string) {
	conn.SetWriteDeadline(time.Now().Add(3 * time.Second))
	conn.Write([]byte(cmd + "\n"))
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	buf := make([]byte, 256)
	n, _ := conn.Read(buf)
	if n > 0 {
		fmt.Printf("  %s -> %q\n", cmd, strings.TrimSpace(string(buf[:n])))
	}
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
	if s := strings.TrimSpace(buf.String()); s != "" {
		fmt.Printf("  %s -> %q\n", cmd, s)
	}
}

func sendCmdExact(conn net.Conn, cmd string, n int) {
	conn.SetWriteDeadline(time.Now().Add(3 * time.Second))
	conn.Write([]byte(cmd + "\n"))
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	buf := make([]byte, n)
	conn.Read(buf)
	fmt.Printf("  %s -> %q\n", cmd, strings.TrimSpace(string(buf)))
}

func sendCmdNoWait(conn net.Conn, cmd string) {
	conn.SetWriteDeadline(time.Now().Add(3 * time.Second))
	conn.Write([]byte(cmd + "\n"))
	conn.SetWriteDeadline(time.Time{})
}