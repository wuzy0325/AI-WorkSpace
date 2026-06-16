package main

import (
	"fmt"
	"net"
	"time"

	sharedproto "shared.local/device-sdk/go/protocol"
)

func main() {
	// 测试 TCP 连接到 P1604 设备
	addr := "192.168.1.7:9000"
	fmt.Printf("正在连接 %s ...\n", addr)

	conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
	if err != nil {
		fmt.Printf("连接失败: %v\n", err)
		return
	}
	defer conn.Close()
	fmt.Println("TCP 连接成功!")

	// 发送 w1601 启用长度前缀模式
	fmt.Println("发送 w1601 启用长度前缀模式...")
	conn.SetWriteDeadline(time.Now().Add(2 * time.Second))
	_, err = conn.Write([]byte("w1601\r\n"))
	if err != nil {
		fmt.Printf("发送 w1601 失败: %v\n", err)
		return
	}
	fmt.Println("w1601 已发送")

	// 等待响应
	time.Sleep(200 * time.Millisecond)
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	frameReader := sharedproto.NewFrameReader(conn)
	payload, err := frameReader.ReadFrame()
	if err != nil {
		fmt.Printf("读取响应失败: %v\n", err)
	} else {
		fmt.Printf("w1601 响应: %s\n", string(payload))
	}

	// 配置数据流参数
	fmt.Println("发送 c 00 1 FFFF 1 100 7 0 (100ms 采样周期)...")
	conn.SetWriteDeadline(time.Now().Add(2 * time.Second))
	_, err = conn.Write([]byte("c 00 1 FFFF 1 100 7 0\r\n"))
	if err != nil {
		fmt.Printf("发送配置失败: %v\n", err)
		return
	}
	time.Sleep(100 * time.Millisecond)
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	payload, err = frameReader.ReadFrame()
	if err != nil {
		fmt.Printf("读取配置响应失败: %v\n", err)
	} else {
		fmt.Printf("配置响应: %s\n", string(payload))
	}

	// 配置流返回内容
	fmt.Println("发送 c 05 1 0810 (压力+大气压力+大气温度)...")
	conn.SetWriteDeadline(time.Now().Add(2 * time.Second))
	_, err = conn.Write([]byte("c 05 1 0810\r\n"))
	if err != nil {
		fmt.Printf("发送流内容配置失败: %v\n", err)
		return
	}
	time.Sleep(100 * time.Millisecond)
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	payload, err = frameReader.ReadFrame()
	if err != nil {
		fmt.Printf("读取流内容配置响应失败: %v\n", err)
	} else {
		fmt.Printf("流内容配置响应: %s\n", string(payload))
	}

	// 启动数据流
	fmt.Println("发送 c 01 1 (启动数据流)...")
	conn.SetWriteDeadline(time.Now().Add(2 * time.Second))
	_, err = conn.Write([]byte("c 01 1\r\n"))
	if err != nil {
		fmt.Printf("发送启动命令失败: %v\n", err)
		return
	}
	time.Sleep(100 * time.Millisecond)

	// 读取几帧数据
	fmt.Println("\n开始读取数据帧...")
	for i := 0; i < 5; i++ {
		conn.SetReadDeadline(time.Now().Add(2 * time.Second))
		payload, err = frameReader.ReadFrame()
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				fmt.Printf("帧 %d: 超时\n", i+1)
				continue
			}
			fmt.Printf("帧 %d: 读取错误: %v\n", i+1, err)
			break
		}

		if sharedproto.IsASCIIFrame(payload) {
			fmt.Printf("帧 %d: ASCII 响应: %s\n", i+1, string(payload))
			continue
		}

		channels, err := sharedproto.ParseStreamFrame(payload)
		if err != nil {
			fmt.Printf("帧 %d: 解析错误: %v\n", i+1, err)
			continue
		}

		fmt.Printf("帧 %d: %d 通道\n", i+1, len(channels))
		for j, v := range channels {
			if j < 16 {
				fmt.Printf("  CH%02d: %.4f psi\n", j+1, v)
			} else if j == 16 {
				fmt.Printf("  CH17 (大气压力): %.2f Pa\n", v)
			} else if j == 17 {
				fmt.Printf("  CH18 (大气温度): %.2f °C\n", v)
			}
		}
	}

	// 停止数据流
	fmt.Println("\n发送 c 02 1 (停止数据流)...")
	conn.SetWriteDeadline(time.Now().Add(2 * time.Second))
	conn.Write([]byte("c 02 1\r\n"))
	time.Sleep(100 * time.Millisecond)

	fmt.Println("测试完成!")
}
