package domain

import (
	"errors"
	"fmt"
	"net"
	"time"
)

// DeviceType 表示设备类型。
type DeviceType string

const (
	// DeviceTypePressure 表示打压设备。
	DeviceTypePressure DeviceType = "pressure"
	// DeviceTypeMeasure 表示计量设备。
	DeviceTypeMeasure DeviceType = "measure"
)

var ErrInvalidDevice = errors.New("invalid device parameters")

// IsValid 报告设备类型是否合法。
func (t DeviceType) IsValid() bool {
	return t == DeviceTypePressure || t == DeviceTypeMeasure
}

// DeviceStatus 表示设备连接状态。
type DeviceStatus string

const (
	DeviceStatusDisconnected DeviceStatus = "disconnected"
	DeviceStatusConnecting   DeviceStatus = "connecting"
	DeviceStatusConnected    DeviceStatus = "connected"
	DeviceStatusError        DeviceStatus = "error"
)

// IsValid 报告设备状态是否合法。
func (s DeviceStatus) IsValid() bool {
	return s == DeviceStatusDisconnected || s == DeviceStatusConnecting ||
		s == DeviceStatusConnected || s == DeviceStatusError
}

// Validate 校验设备实体字段是否满足基本约束。
func (d Device) Validate() error {
	if d.ID == "" || !d.Type.IsValid() {
		return ErrInvalidDevice
	}
	if net.ParseIP(d.Host) == nil || d.Port < 1 || d.Port > 65535 {
		return ErrInvalidDevice
	}
	return nil
}

// ResolveStatus 根据请求状态和已有设备记录确定最终状态。
// 若请求未指定状态，则继承已有设备的状态，或默认为 Disconnected。
func ResolveStatus(requested DeviceStatus, existing Device, existed bool) DeviceStatus {
	if requested != "" {
		return requested
	}
	if existed && existing.Status != "" {
		return existing.Status
	}
	return DeviceStatusDisconnected
}

// ChannelConfig 描述设备单个通道的采集配置。
//
// 通用扩展：各设备型号按需使用。P1603（4-20mA 电流环）必须配置每通道
// rangeMin/rangeMax 才能把电流值映射为正确工程量（4mA→engMin、20mA→engMax）。
// 通道号 Index 为 1-based（与 1604Cal 计量业务层的通道语义一致，
// 例如 CollectData(channels []int) 中通道 1-16）。
type ChannelConfig struct {
	Index     int     `json:"index"`     // 通道号（1-based）
	Name      string  `json:"name"`      // 通道名（如 CH1）
	Enabled   bool    `json:"enabled"`   // 是否启用
	Unit      string  `json:"unit"`      // 工程单位（如 Pa/kPa）
	RangeMin  float64 `json:"rangeMin"`  // 工程量下限（对应 4mA）
	RangeMax  float64 `json:"rangeMax"`  // 工程量上限（对应 20mA）
	Precision int     `json:"precision"` // 显示精度（小数位）
	// TareOffset 该通道的软件校零偏移（原始工程量 - offset = 显示值）。
	// 校零后随设备配置持久化到本地（devices.json），设备重连后自动加载扣除，
	// 避免计量数据因零漂漂移。仅 P1603 等软件归零设备使用。
	TareOffset float64 `json:"tareOffset,omitempty"`
}

// DefaultP1603Channels 返回 P1603 的 16 通道默认配置（对齐 WindLabX4）。
// 默认 Pa 单位、量程 ±5000、精度 3、全启用；用户可在设备表单中按传感器量程修改。
func DefaultP1603Channels() []ChannelConfig {
	channels := make([]ChannelConfig, 16)
	for i := range channels {
		channels[i] = ChannelConfig{
			Index:     i + 1, // 1-based，与计量业务层一致
			Name:      fmt.Sprintf("CH%d", i+1),
			Enabled:   true,
			Unit:      "Pa",
			RangeMin:  -5000,
			RangeMax:  5000,
			Precision: 3,
		}
	}
	return channels
}

// Device 表示系统维护的设备配置实体。
type Device struct {
	ID     string       `json:"id"`
	Name   string       `json:"name"`
	Type   DeviceType   `json:"type"`
	Model  string       `json:"model"`
	Host   string       `json:"host"`
	Port   int          `json:"port"`
	Unit   string       `json:"unit"`
	Status DeviceStatus `json:"status"`

	// Channels 每通道采集配置（1-based Index）。通用扩展，
	// P1603 等需要按通道量程换算工程量的设备使用；其他型号可为空。
	Channels []ChannelConfig `json:"channels,omitempty"`

	// LocalAddr 指定 TCP 拨号绑定的本地 IP 地址。
	// 多网卡环境下，Windows 可能将流量路由到错误的网卡，导致连接超时。
	// 设置此字段后，TCP 拨号会绑定到指定本地地址，确保流量从正确的网卡发出。
	// 留空时由操作系统自动选择路由。
	LocalAddr string `json:"localAddr,omitempty"`

	// IsSimulated 为 true 时使用模拟驱动，不连接真实设备。
	IsSimulated bool `json:"isSimulated"`

	// LastErrorReason 记录最近一次连接/断连失败原因。
	LastErrorReason string `json:"lastErrorReason,omitempty"`
	// LastErrorAt 记录最近一次连接/断连失败时间。
	LastErrorAt *time.Time `json:"lastErrorAt,omitempty"`
}
