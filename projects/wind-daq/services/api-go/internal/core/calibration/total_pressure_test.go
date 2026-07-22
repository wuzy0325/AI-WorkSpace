package calibration

import (
	"strings"
	"testing"
)

// TestTotalPressureAcquireDataWithChannels_PAtmGuard 验证大气压异常守门：
// 当采样平均 PAtm <= 0 时，AcquireDataWithChannels 应明确报错，
// 避免后续用表压当绝对压计算 CPT 导致系数严重偏高且无告警。
//
// 构造：channelReader 对所有通道返回固定值，PAtm=0（异常）。
// 期望：返回错误且错误信息包含 "大气压"。
func TestTotalPressureAcquireDataWithChannels_PAtmGuard(t *testing.T) {
	probeChannels := []ProbeChannel{
		{Role: "totalPressure.pAtm", Name: "大气压", DeviceID: "dev-1", ChannelIndex: 16, Enabled: true},
		{Role: "totalPressure.pTunnelTotal", Name: "风洞总压", DeviceID: "dev-1", ChannelIndex: 1, Enabled: true},
		{Role: "totalPressure.pTunnelStatic", Name: "风洞静压", DeviceID: "dev-1", ChannelIndex: 2, Enabled: true},
		{Role: "totalPressure.pProbeTotal", Name: "探针总压", DeviceID: "dev-1", ChannelIndex: 0, Enabled: true},
	}

	// PAtm 恒为 0（异常），其余通道正常表压
	reader := func(deviceID string, channelIndex int) (float64, bool) {
		switch channelIndex {
		case 16:
			return 0.0, true // pAtm 异常
		case 1:
			return 5000.0, true
		case 2:
			return 3000.0, true
		case 0:
			return 4900.0, true
		}
		return 0, false
	}

	point := CalPoint{ID: 1, Coordinates: map[string]float64{"α": 10}}
	dp, err := (&TotalPressureAlgorithm{}).AcquireDataWithChannels(
		point, reader, probeChannels, 2, nil, nil, nil, nil, nil,
	)
	if err == nil {
		t.Fatalf("PAtm=0 时应返回错误，但得到数据点: %+v", dp)
	}
	if !strings.Contains(err.Error(), "大气压") {
		t.Fatalf("错误信息应包含 '大气压'，实际: %v", err)
	}
}

// TestTotalPressureAcquireDataWithChannels_PAtmNegative 验证 PAtm 为负值同样被守门拦截。
func TestTotalPressureAcquireDataWithChannels_PAtmNegative(t *testing.T) {
	probeChannels := []ProbeChannel{
		{Role: "totalPressure.pAtm", Name: "大气压", DeviceID: "dev-1", ChannelIndex: 16, Enabled: true},
		{Role: "totalPressure.pTunnelTotal", Name: "风总压", DeviceID: "dev-1", ChannelIndex: 1, Enabled: true},
		{Role: "totalPressure.pTunnelStatic", Name: "风洞静压", DeviceID: "dev-1", ChannelIndex: 2, Enabled: true},
		{Role: "totalPressure.pProbeTotal", Name: "探针总压", DeviceID: "dev-1", ChannelIndex: 0, Enabled: true},
	}
	reader := func(deviceID string, channelIndex int) (float64, bool) {
		switch channelIndex {
		case 16:
			return -50.0, true // pAtm 负值（异常）
		default:
			return 1000.0, true
		}
	}
	point := CalPoint{ID: 1, Coordinates: map[string]float64{"α": 0}}
	if _, err := (&TotalPressureAlgorithm{}).AcquireDataWithChannels(
		point, reader, probeChannels, 1, nil, nil, nil, nil, nil,
	); err == nil {
		t.Fatal("PAtm 为负值时应返回错误")
	}
}

// TestTotalPressureAcquireDataWithChannels_NormalPAtm 验证正常 PAtm 不被误拦截。
func TestTotalPressureAcquireDataWithChannels_NormalPAtm(t *testing.T) {
	probeChannels := []ProbeChannel{
		{Role: "totalPressure.pAtm", Name: "大气压", DeviceID: "dev-1", ChannelIndex: 16, Enabled: true},
		{Role: "totalPressure.pTunnelTotal", Name: "风洞总压", DeviceID: "dev-1", ChannelIndex: 1, Enabled: true},
		{Role: "totalPressure.pTunnelStatic", Name: "风洞静压", DeviceID: "dev-1", ChannelIndex: 2, Enabled: true},
		{Role: "totalPressure.pProbeTotal", Name: "探针总压", DeviceID: "dev-1", ChannelIndex: 0, Enabled: true},
	}
	reader := func(deviceID string, channelIndex int) (float64, bool) {
		switch channelIndex {
		case 16:
			return 101325.0, true
		case 1:
			return 5000.0, true
		case 2:
			return 3000.0, true
		case 0:
			return 4900.0, true
		}
		return 0, false
	}
	point := CalPoint{ID: 1, Coordinates: map[string]float64{"α": 10}}
	dp, err := (&TotalPressureAlgorithm{}).AcquireDataWithChannels(
		point, reader, probeChannels, 2, nil, nil, nil, nil, nil,
	)
	if err != nil {
		t.Fatalf("正常 PAtm 不应报错: %v", err)
	}
	if dp == nil {
		t.Fatal("应返回数据点")
	}
	if dp.Coefficients.CPT <= 0 {
		t.Errorf("风洞总压高于阈值时 CPT 应 > 0，实际 %v", dp.Coefficients.CPT)
	}
}