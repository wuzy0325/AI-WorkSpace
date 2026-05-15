package ports

import (
	"context"
	"fmt"
	"time"
)

// ==================== 设备扫描接口 ====================
// 用于在网络中自动发现数据采集设备

// DiscoveredDevice 发现的设备信息
type DiscoveredDevice struct {
	ID              string `json:"id"`                        // 设备ID(通常是MAC地址或序列号)
	IP              string `json:"address"`                   // IP地址
	Port            int    `json:"port"`                      // 端口号
	Type            string `json:"type"`                      // 设备类型
	Name            string `json:"name"`                      // 设备名称
	MACAddress      string `json:"macAddress,omitempty"`      // MAC地址
	SerialNumber    string `json:"serialNumber,omitempty"`    // 序列号
	FirmwareVersion string `json:"firmwareVersion,omitempty"` // 固件版本
	Model           string `json:"model,omitempty"`           // 型号
	SubnetMask      string `json:"subnetMask,omitempty"`      // 子网掩码
	Gateway         string `json:"gateway,omitempty"`         // 网关
	IPMode          string `json:"ipMode,omitempty"`          // IP模式(static/DHCP)
	TCPConnected    bool   `json:"tcpConnected,omitempty"`    // TCP连接状态
	IPAssigned      bool   `json:"ipAssigned,omitempty"`      // IP是否已分配
	RawResponse     string `json:"rawResponse,omitempty"`     // 原始响应字符串
}

func DeviceFingerprint(d DiscoveredDevice) string {
	return fmt.Sprintf("%s:%s:%d", d.Type, d.IP, d.Port)
}

// DeviceScanner 设备扫描器接口
// 在指定超时时间内扫描网络中的兼容设备
type DeviceScanner interface {
	// Scan 执行设备扫描
	// 参数: ctx 取消上下文
	// 参数: timeout 扫描超时时间
	// 返回: []DiscoveredDevice 发现的设备列表
	Scan(ctx context.Context, timeout time.Duration) ([]DiscoveredDevice, error)
}
