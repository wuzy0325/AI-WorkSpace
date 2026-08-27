//go:build ignore
// +build ignore

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

func main() {
	base := "http://localhost:8080/api/v1"

	// 1. 添加设备
	dev := map[string]any{
		"id":     "dev-test-01",
		"name":   "WTN1604-测试",
		"type":   "measure",
		"model":  "WTN1604",
		"host":   "192.168.1.7",
		"port":   9000,
		"unit":   "MPa",
		"status": "disconnected",
	}
	post(base+"/devices", dev)

	// 2. 连接设备
	post(base+"/devices/connect", map[string]string{"id": "dev-test-01"})

	// 3. 设置校准计量设备
	post(base+"/calibration/measure-device", map[string]string{"measureDeviceId": "dev-test-01"})

	// 4. 读取阀门状态
	get(base + "/calibration/valve")

	// 5. 读取单位
	get(base + "/calibration/measure-unit")

	// 6. 设置阀门状态
	post(base+"/calibration/valve", map[string]string{"status": "calibration"})

	// 7. 读取阀门状态
	get(base + "/calibration/valve")

	// 8. 断开设备
	post(base+"/devices/disconnect", map[string]string{"id": "dev-test-01"})
}

func get(url string) {
	resp, err := http.Get(url)
	if err != nil {
		fmt.Println("GET", url, "error:", err)
		return
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	fmt.Println("GET", url, "->", resp.StatusCode, string(body))
}

func post(url string, payload any) {
	b, _ := json.Marshal(payload)
	resp, err := http.Post(url, "application/json", bytes.NewReader(b))
	if err != nil {
		fmt.Println("POST", url, "error:", err)
		return
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	fmt.Println("POST", url, "->", resp.StatusCode, string(body))
}
