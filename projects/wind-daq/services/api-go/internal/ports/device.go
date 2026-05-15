package ports

import "wind-daq/services/api-go/internal/core/device"

// ==================== 设备硬件接口 ====================
// 本接口定义了数据采集设备的通用行为，是适配各种硬件的抽象层
// 各硬件驱动(如DAQ-P1604、DAQ-T-1603)需实现此接口

// Device 设备抽象接口
// 定义数据采集设备的通用操作：连接、采集、数据回调
type Device interface {
	ID() string                       // 获取设备唯一标识符
	Config() device.DeviceConfig      // 获取设备配置信息
	Connect() error                   // 连接设备
	Disconnect() error                // 断开设备连接
	StartAcquisition() error          // 开始数据采集
	StopAcquisition() error           // 停止数据采集
	SetDataSink(sink device.DataSink) // 设置数据接收回调
	Status() device.DeviceStatus      // 获取设备当前状态
}

// ChannelUpdatable 支持热配置（在线修改通道）的设备接口
// 允许在设备采集过程中动态修改启用的通道
type ChannelUpdatable interface {
	UpdateChannels(channels []device.ChannelConfig) // 更新通道配置
	GetChannels() []device.ChannelConfig            // 获取当前通道配置
}

// CommandSendable 支持发送原始命令的设备接口
// 用于调试和特殊控制场景
type CommandSendable interface {
	SendCommand(command string, timeoutMs int) (string, error) // 发送原始命令并等待响应
}
