package device

import "testing"

// TestIsAtmosphericChannel 验证大气压/大气温度辅助通道的识别逻辑。
//
// 测试前置：DAQ-P-1604 / DAQ-P-1604Pre 协议规定 Index 16/17 为大气辅助通道，
// 其他设备类型无此类通道。
// 测试步骤：对每种设备类型的代表性通道索引调用 IsAtmosphericChannel。
// 期待结果：仅 1604/1604Pre 的 Index 16/17 返回 true，其余返回 false。
func TestIsAtmosphericChannel(t *testing.T) {
	tests := []struct {
		name         string
		profileType  Type
		channelIndex int
		want         bool
	}{
		// DAQ-P-1604：Index 16/17 为大气辅助通道
		{"DAQ-P-1604 大气压通道(16)", DeviceDAQP1604, P1604PreAtmChannelIndex, true},
		{"DAQ-P-1604 大气温度通道(17)", DeviceDAQP1604, P1604PreAtmTempChannelIndex, true},
		{"DAQ-P-1604 常规压力通道(0)", DeviceDAQP1604, 0, false},
		{"DAQ-P-1604 常规压力通道(15)", DeviceDAQP1604, 15, false},

		// DAQ-P-1604Pre：与 1604 共用通道布局常量
		{"DAQ-P-1604Pre 大气压通道(16)", DeviceDAQP1604Pre, P1604PreAtmChannelIndex, true},
		{"DAQ-P-1604Pre 大气温度通道(17)", DeviceDAQP1604Pre, P1604PreAtmTempChannelIndex, true},
		{"DAQ-P-1604Pre 常规压力通道(7)", DeviceDAQP1604Pre, 7, false},

		// DAQ-P-1603：16 通道通用 AI，无大气辅助通道，Index 16 不应误判
		{"DAQ-P-1603 通道(16) 不视为大气压", DeviceDAQP1603, 16, false},
		{"DAQ-P-1603 通道(0)", DeviceDAQP1603, 0, false},

		// 其他设备类型：一律不视为大气辅助通道
		{"DSA3217 通道(16)", DeviceDSA3217, 16, false},
		{"SIMULATED 通道(17)", DeviceSimulated, 17, false},
		{"WTN_PXI 通道(16)", DeviceWTNPXI, 16, false},
		{"DAQ-T-1603 通道(17)", DeviceDaqT1603, 17, false},
		{"PACE1000 大气压通道(0)", DevicePACE1000, 0, true},
		{"PACE1000 非法通道(1)", DevicePACE1000, 1, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsAtmosphericChannel(tt.profileType, tt.channelIndex)
			if got != tt.want {
				t.Fatalf("IsAtmosphericChannel(%q, %d) = %v, want %v",
					tt.profileType, tt.channelIndex, got, tt.want)
			}
		})
	}
}
