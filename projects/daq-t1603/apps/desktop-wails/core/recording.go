package core

type RecordingStatus int

const (
	RecordingIdle RecordingStatus = iota
	RecordingActive
)

type RecordingSession struct {
	ID            string          `json:"id"`
	OutputDir     string          `json:"outputDir"`
	FilePrefix    string          `json:"filePrefix"`
	StartTimeMs   int64           `json:"startTimeMs"`
	SnapshotCount int             `json:"snapshotCount"`
	Status        RecordingStatus `json:"status"`
	// DroppedCount 是录制队列背压丢弃的总帧数（跨设备聚合）。
	// 前端可据此显示数据完整性指标。
	DroppedCount int `json:"droppedCount"`
}

// BackpressureEvent 由 recorder 在丢帧时回调，上层转发到日志与前端事件。
// 字段语义：
//   - DeviceID：发生背压的设备 ID；
//   - QueueLen/QueueCap：当前队列长度/容量，反映瞬时压力；
//   - DroppedTotal：该设备累计丢帧数，用于趋势分析。
type BackpressureEvent struct {
	DeviceID     string `json:"deviceId"`
	QueueLen     int    `json:"queueLen"`
	QueueCap     int    `json:"queueCap"`
	DroppedTotal int64  `json:"droppedTotal"`
}
