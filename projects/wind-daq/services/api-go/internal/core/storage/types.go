// Package storage 定义 wind-daq 数据保存领域的核心类型。
//
// 这些类型属于核心领域模型，不依赖任何外部框架或 I/O 实现，
// 由 usecase 与 adapters 层共同使用。
package storage

import (
	"wind-daq/services/api-go/internal/core/device"
)

// StopConditions 自动停止条件。
// 任意条件满足时，sink 应停止接收新数据并触发 StorageRecorder.Stop。
// 全部字段为零值表示不限制（永久录制直到用户手动停止）。
type StopConditions struct {
	// MaxDurationMs 录制时长上限（毫秒）
	MaxDurationMs int64 `json:"maxDurationMs,omitempty"`
	// MaxFileSizeBytes 单文件大小上限（字节）
	// 注意：对启用滚动的场景，该字段同时表示滚动阈值；
	// 未启用滚动时表示录制总大小上限，达到后停止录制。
	MaxFileSizeBytes int64 `json:"maxFileSizeBytes,omitempty"`
	// MaxRecordCount 累计记录条数上限
	MaxRecordCount int64 `json:"maxRecordCount,omitempty"`
}

// FileRotation 文件滚动保存配置。
// 启用后，sink 在达到大小或时长阈值时关闭当前文件并创建新文件继续录制，
// 不影响录制会话的整体生命周期。
type FileRotation struct {
	// Enabled 是否启用文件滚动
	Enabled bool `json:"enabled"`
	// MaxFileSizeBytes 单文件大小阈值（字节），达到后滚动到新文件
	MaxFileSizeBytes int64 `json:"maxFileSizeBytes"`
	// MaxDurationMs 单文件时长阈值（毫秒），达到后滚动到新文件
	MaxDurationMs int64 `json:"maxDurationMs"`
}

// RecordingConfig 录制会话配置。
// 由 UI 通过 API/Wails binding 传入，贯穿 usecase -> sink。
type RecordingConfig struct {
	// OutputDir 输出目录（绝对路径或相对路径，由后端装配层统一解析）
	OutputDir string `json:"outputDir"`
	// FilePrefix 文件名前缀，最终文件名形如 <prefix>-YYYYMMDD-HHMMSS-NNN.csv
	FilePrefix string `json:"filePrefix"`
	// Format 存储格式："csv"（默认）或 "binary"
	Format string `json:"format,omitempty"`
	// StopConditions 自动停止条件（可选）
	StopConditions StopConditions `json:"stopConditions,omitempty"`
	// FileRotation 文件滚动配置（可选）
	FileRotation FileRotation `json:"fileRotation,omitempty"`
	// AutoStartOnAcquisition 是否在采集启动时自动开始录制
	// 该字段由 UI 写入配置文件，由编排层读取并触发；sink 自身不消费。
	AutoStartOnAcquisition bool `json:"autoStartOnAcquisition,omitempty"`

	// DeviceChannels 按设备 ID 注入的通道元数据，供 sink 构造按设备类型分支的 CSV 表头。
	//
	// 设计动机：
	//   - DataPayload 只携带 Channels []float64 + ChannelIndices []int，不含 Unit/Name/SensorType
	//   - DAQ-P-1603 等设备要求 CSV 表头按通道类型动态生成（如 CH01_Pa, CH02_degC）
	//   - sink 在首帧冻结列布局时一次性消费此映射，本会话内不再变更
	//
	// 注入时机：
	//   - server.go /api/storage/start 入口从 DeviceManager.GetProfiles() 收集所有 profile 的 channels
	//   - 录制中后连接的设备若未在此映射中，sink 回退到通用 CH01..CHnn 表头（保持兼容）
	//
	// 可选字段；为空时所有设备使用通用表头。
	DeviceChannels map[string][]device.ChannelConfig `json:"deviceChannels,omitempty"`
}

// RecordingStatus 录制会话运行时状态。
// 由 sink 维护并通过 atomic 或 mutex 暴露给上层，用于 UI 实时展示与错误反馈。
type RecordingStatus struct {
	// Recording 是否正在录制
	Recording bool `json:"recording"`
	// OutputDir 当前输出目录
	OutputDir string `json:"outputDir,omitempty"`
	// CurrentFile 当前写入的文件名（不含目录）
	CurrentFile string `json:"currentFile,omitempty"`
	// FileSize 当前文件累计字节数
	FileSize int64 `json:"fileSize"`
	// FileCount 本会话已滚动的文件数（含当前文件，从 1 开始）
	FileCount int64 `json:"fileCount"`
	// RecordCount 本会话累计写入的"记录"数。
	// 语义统一为"通道值数"（1 payload × N 通道 = N 条记录）：
	//   - CSV sink：1 条记录 = 1 行（含 1 个通道值），与 CSV 行数一致
	//   - Binary sink：1 条记录 = 1 个 float32 通道槽位（1 payload × N 通道 = N 槽位）
	// 注意：此值不是 payload 帧数。如需帧数请用 RecordCount / 通道数。
	RecordCount int64 `json:"recordCount"`
	// DurationMs 本会话累计录制时长（毫秒）
	DurationMs int64 `json:"durationMs"`
	// DroppedCount 因队列满被丢弃的 payload 数
	DroppedCount int64 `json:"droppedCount"`
	// LastError 最近一次 I/O 错误描述（空表示无错误）
	LastError string `json:"lastError,omitempty"`
}
