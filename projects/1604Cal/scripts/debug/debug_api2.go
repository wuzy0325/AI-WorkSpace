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

	// 先删除旧设备
	req, _ := http.NewRequest("DELETE", base+"/devices/dev-test-01", nil)
	http.DefaultClient.Do(req)

	// 添加并连接
	post(base+"/devices", map[string]any{
		"id": "dev-test-01", "name": "WTN1604-测试", "type": "measure",
		"model": "WTN1604", "host": "192.168.1.7", "port": 9000,
		"unit": "MPa", "status": "disconnected",
	})
	post(base+"/devices/connect", map[string]string{"id": "dev-test-01"})
	post(base+"/calibration/measure-device", map[string]string{"measureDeviceId": "dev-test-01"})

	// 读取阀门
	get(base + "/calibration/valve")

	// 断开
	post(base+"/devices/disconnect", map[string]string{"id": "dev-test-01"})
}

func get(url string) {
	resp, _ := http.Get(url)
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	fmt.Println("GET", url, "->", resp.StatusCode, string(body))
}

func post(url string, payload any) {
	b, _ := json.Marshal(payload)
	resp, _ := http.Post(url, "application/json", bytes.NewReader(b))
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	fmt.Println("POST", url, "->", resp.StatusCode, string(body))
}
