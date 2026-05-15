package device

import "time"

// ==================== 设备类型定义 ====================

// DeviceType 数据采集设备类型
// 支持：模拟设备、DAQ-P1604、DAQ-T-1603、WTN_PXI
type DeviceType string

const (
	DeviceSimulated DeviceType = "SIMULATED"  // 模拟设备（用于测试）
	DeviceDAQP1604  DeviceType = "DAQ-P-1604" // 温特纳DAQ-P1604 多通道数据采集仪
	DeviceDAQT1603  DeviceType = "DAQ-T-1603" // DAQ-T-1603 热电偶采集仪
	DeviceWTNPXI    DeviceType = "WTN_PXI"    // 温特纳PXI数据采集系统
)

// ==================== 通信方式定义 ====================

// TransportType 设备通信传输类型
type TransportType string

const (
	TransportTCP    TransportType = "tcp"    // TCP/IP网络通信
	TransportSerial TransportType = "serial" // 串口通信(RS232/RS485)
)

// ==================== 设备状态定义 ====================

// DeviceState 设备连接状态枚举
type DeviceState string

const (
	StateDisconnected DeviceState = "disconnected" // 未连接
	StateConnecting   DeviceState = "connecting"   // 连接中
	StateConnected    DeviceState = "connected"    // 已连接
	StateAcquiring    DeviceState = "acquiring"    // 数据采集中
	StateError        DeviceState = "error"        // 错误状态
)

// ConnectionStatus 前端展示用的连接状态字符串
type ConnectionStatus string

const (
	ConnectionDisconnected ConnectionStatus = "Disconnected" // 未连接
	ConnectionConnecting   ConnectionStatus = "Connecting"   // 连接中
	ConnectionConnected    ConnectionStatus = "Connected"    // 已连接
	ConnectionError        ConnectionStatus = "Error"        // 连接错误
)

// StateToConnection 将内部状态转换为前端显示状态
// 参数: state 设备内部状态
// 返回: ConnectionStatus 前端显示用的连接状态
func StateToConnection(state DeviceState) ConnectionStatus {
	switch state {
	case StateConnected, StateAcquiring:
		return ConnectionConnected
	case StateConnecting:
		return ConnectionConnecting
	case StateError:
		return ConnectionError
	default:
		return ConnectionDisconnected
	}
}

// ==================== 设备配置结构 ====================

// DeviceConfig 设备运行时配置（从DeviceProfile简化而来，用于创建设备实例）
type DeviceConfig struct {
	ID           string          `json:"id"`                   // 设备唯一标识符
	Name         string          `json:"name"`                 // 设备名称
	Type         DeviceType      `json:"type"`                 // 设备类型
	Transport    TransportType   `json:"transport,omitempty"`  // 通信方式:tcp/serial
	Address      string          `json:"address,omitempty"`    // TCP IP地址
	Port         int             `json:"port,omitempty"`       // TCP 端口号
	SerialPort   string          `json:"serialPort,omitempty"` // 串口名称如COM1
	BaudRate     int             `json:"baudRate,omitempty"`   // 串口波特率
	SamplingRate int             `json:"samplingRate"`         // 采样率(Hz)
	Channels     []ChannelConfig `json:"channels,omitempty"`   // 通道配置列表
}

// DataPayload 设备采集的数据包（从设备推送上来的一次采集数据）
type DataPayload struct {
	DeviceID       string    `json:"deviceId"`       // 设备ID
	Timestamp      int64     `json:"timestamp"`      // 时间戳(毫秒)
	Channels       []float64 `json:"channels"`       // 通道数据值数组
	ChannelIndices []int     `json:"channelIndices"` // 对应通道的索引号
}

// DeviceStatus 设备当前运行状态（用于前端展示和状态查询）
type DeviceStatus struct {
	ID           string           `json:"id"`                  // 设备ID
	Name         string           `json:"name"`                // 设备名称
	Type         DeviceType       `json:"type"`                // 设备类型
	Connection   ConnectionStatus `json:"connection"`          // 连接状态
	Acquiring    bool             `json:"acquiring"`           // 是否正在采集
	SamplingRate int              `json:"samplingRate"`        // 当前采样率
	LastError    string           `json:"lastError,omitempty"` // 最近错误信息
}

// DeviceInstance 当前活跃的设备实例信息
type DeviceInstance struct {
	ProfileID string `json:"profileId"`           // 关联的配置ID
	Status    string `json:"status"`              // 连接状态字符串
	Acquiring bool   `json:"acquiring"`           // 是否正在采集
	LastError string `json:"lastError,omitempty"` // 最后错误信息
}

// DeviceProfile 设备配置模板（保存到文件/数据库的配置）
type DeviceProfile struct {
	ID             string                  `json:"id"`                       // 配置唯一ID
	Name           string                  `json:"name"`                     // 配置名称
	Type           DeviceType              `json:"type"`                     // 设备类型
	MacAddress     string                  `json:"macAddress,omitempty"`     // MAC地址
	Transport      TransportType           `json:"transport,omitempty"`      // 通信方式
	Address        string                  `json:"address,omitempty"`        // IP地址
	Port           int                     `json:"port,omitempty"`           // 端口
	SerialPort     string                  `json:"serialPort,omitempty"`     // 串口
	BaudRate       int                     `json:"baudRate,omitempty"`       // 波特率
	SamplingRate   int                     `json:"samplingRate"`             // 采样率
	AutoConnect    bool                    `json:"autoConnect"`              // 启动时自动连接
	Channels       []ChannelConfig         `json:"channels"`                 // 通道配置
	DaqT1603Config *DaqT1603HardwareConfig `json:"daqT1603Config,omitempty"` // DAQ-T-1603专用配置
}

// DataSink 数据接收回调函数类型
// 参数: payload 采集到的数据包
type DataSink func(payload DataPayload)

// ==================== 通道配置结构 ====================

// ChannelConfig 单个通道的配置信息
type ChannelConfig struct {
	Index     int      `json:"index"`               // 通道索引号(从0开始)
	Name      string   `json:"name"`                // 通道名称
	Enabled   bool     `json:"enabled"`             // 是否启用
	Unit      string   `json:"unit,omitempty"`      // 单位如V/mA/℃
	Precision int      `json:"precision,omitempty"` // 显示精度(小数位数)
	RangeMin  *float64 `json:"rangeMin,omitempty"`  // 量程下限
	RangeMax  *float64 `json:"rangeMax,omitempty"`  // 量程上限
}

// ==================== 热电偶类型定义 ====================

// ThermocoupleType 热电偶类型（用于DAQ-T-1603）
type ThermocoupleType string

const (
	TCTypeK      ThermocoupleType = "K"      // K型热电偶(-200~1300℃)
	TCTypeB      ThermocoupleType = "B"      // B型热电偶(0~1800℃)
	TCTypeE      ThermocoupleType = "E"      // E型热电偶(-200~900℃)
	TCTypeJ      ThermocoupleType = "J"      // J型热电偶(-200~1200℃)
	TCTypeT      ThermocoupleType = "T"      // T型热电偶(-200~400℃)
	TCTypeS      ThermocoupleType = "S"      // S型热电偶(0~1600℃)
	TCTypeN      ThermocoupleType = "N"      // N型热电偶(-200~1300℃)
	TCTypeR      ThermocoupleType = "R"      // R型热电偶(0~1600℃)
	TCTypeC      ThermocoupleType = "C"      // C型热电偶(0~2300℃)
	TCTypeWRE325 ThermocoupleType = "WRE325" // 钨铼325型热电偶
	TCTypeWRE526 ThermocoupleType = "WRE526" // 钨铼526型热电偶
	TCTypeWRE520 ThermocoupleType = "WRE520" // 钨铼520型热电偶
)

// ThermocoupleChannelConfig 热电偶通道专用配置（继承ChannelConfig并添加热电偶类型）
type ThermocoupleChannelConfig struct {
	ChannelConfig                     // 嵌入基础通道配置
	ThermocoupleType ThermocoupleType `json:"thermocoupleType,omitempty"` // 热电偶类型
}

// ==================== DAQ-T-1603 硬件配置 ====================

// DaqT1603HardwareConfig DAQ-T-1603设备专用硬件参数配置
type DaqT1603HardwareConfig struct {
	ThermocoupleTypes []ThermocoupleType `json:"thermocoupleTypes"` // 各通道热电偶类型列表
	ChannelMask       string             `json:"channelMask"`       // 通道掩码如FFFFFFFF
	SamplingRate      int                `json:"samplingRate"`      // 采样率
	BinaryFormat      bool               `json:"binaryFormat"`      // 是否使用二进制格式
	AverageCount      int                `json:"averageCount"`      // 平均次数
	TriggerMode       int                `json:"triggerMode"`       // 触发模式
	TriggerEdge       int                `json:"triggerEdge"`       // 触发边沿
	TriggerCount      int                `json:"triggerCount"`      // 触发次数
	OpenCircuitCheck  string             `json:"openCircuitCheck"`  // 开路检测设置
}

// NowMs 获取当前时间戳（毫秒）
// 返回: int64 当前Unix时间戳(毫秒)
func NowMs() int64 {
	return time.Now().UnixMilli()
}
