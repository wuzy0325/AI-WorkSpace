package core

// RecordingStatus 录制状态
type RecordingStatus int

const (
	RecordingIdle RecordingStatus = iota
	RecordingActive
	// RecordingStopping 表示已收到 Stop 请求但 writer goroutine 仍在 drain 队列
	RecordingStopping
)

// FileRotation 文件滚动配置（任一条件满足即滚动到新文件，0 表示不限制）
type FileRotation struct {
	MaxSizeBytes   int64 `json:"maxSizeBytes,omitempty"`   // 单文件最大字节数
	MaxDurationMs  int64 `json:"maxDurationMs,omitempty"`  // 单文件最大时长
	MaxRecordCount int64 `json:"maxRecordCount,omitempty"` // 单文件最大记录数
}

// StopConditions 录制自动停止条件（任一条件满足即停止整个录制，0 表示不限制）
type StopConditions struct {
	MaxDurationMs    int64 `json:"maxDurationMs,omitempty"`    // 录制最大时长
	MaxFileSizeBytes int64 `json:"maxFileSizeBytes,omitempty"` // 所有文件累计最大字节数
	MaxRecordCount   int64 `json:"maxRecordCount,omitempty"`   // 所有文件累计最大记录数
}

// RecordingConfig 录制配置（多设备共享）
// 仅支持 CSV 格式；历史 Binary 格式已移除（无读端、孤儿格式）
type RecordingConfig struct {
	OutputDir       string          `json:"outputDir"`
	FilePrefix      string          `json:"filePrefix"`
	Channels        []ChannelConfig `json:"channels"`         // 多设备合并的通道精度配置
	Rotation        FileRotation    `json:"rotation"`         // 文件滚动条件
	StopConditions  StopConditions  `json:"stopConditions"`   // 自动停止条件
	FlushIntervalMs int             `json:"flushIntervalMs"`  // bufio flush 间隔（默认 100ms）
	SyncIntervalSec int             `json:"syncIntervalSec"`  // fsync 间隔（默认 2s）
	QueueCapacity   int             `json:"queueCapacity"`    // 异步队列容量（默认 32768）
}

// RecordingSession 录制会话运行时状态
type RecordingSession struct {
	ID            string          `json:"id"`
	OutputDir     string          `json:"outputDir"`
	FilePrefix    string          `json:"filePrefix"`
	StartTimeMs   int64           `json:"startTimeMs"`
	SnapshotCount int64           `json:"snapshotCount"` // 已写入的快照数
	DroppedCount  int64           `json:"droppedCount"`  // 队列满时丢弃的快照数
	FileCount     int64           `json:"fileCount"`     // 已创建的文件数（含滚动）
	CurrentFile   string          `json:"currentFile,omitempty"` // 当前正在写入的文件完整路径（文件滚动时更新）
	LastError     string          `json:"lastError,omitempty"`
	Status        RecordingStatus `json:"status"`
}
