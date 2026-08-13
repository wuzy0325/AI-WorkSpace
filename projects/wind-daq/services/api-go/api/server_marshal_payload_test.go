package api

import (
	"math"
	"strings"
	"testing"

	"wind-daq/services/api-go/internal/core/device"
)

// TestMarshalDataPayloadNaNToNull 验证 channels 中的 NaN/Inf 序列化为 JSON null：
// T1602 用 NaN 表示"通道未接入热电偶"（raw 寄存器值为 0），SSE 流必须输出合法
// JSON（Go encoding/json 默认会把 NaN 写成 "NaN" 字符串，前端 JSON.parse 会整帧失败）。
func TestMarshalDataPayloadNaNToNull(t *testing.T) {
	p := device.DataPayload{
		DeviceID:  "dev-1",
		Timestamp: 123,
		Channels:  []float64{27.7, math.NaN(), math.Inf(1), math.Inf(-1), -50.25},
	}
	out := string(marshalDataPayload(p))
	if !strings.Contains(out, `"channels":[27.7,null,null,null,-50.25]`) {
		t.Fatalf("channels encoding = %s, want NaN/Inf replaced by null", out)
	}
}

// TestMarshalDataPayloadNilChannels 保持既有行为：nil 通道数组输出 null。
func TestMarshalDataPayloadNilChannels(t *testing.T) {
	p := device.DataPayload{DeviceID: "dev-1", Timestamp: 1}
	out := string(marshalDataPayload(p))
	if !strings.Contains(out, `"channels":null`) {
		t.Fatalf("nil channels should encode as null, got %s", out)
	}
}
