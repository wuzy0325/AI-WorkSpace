package ports

import (
	"context"

	"wind-daq/services/api-go/internal/core/motion"
	"wind-daq/services/api-go/internal/core/traversal"
)

// MotionAccess 运动控制访问接口（遍历用例所需）
type MotionAccess interface {
	StatusAll(ctx context.Context) []motion.ControllerStatus
	MoveTo(ctx context.Context, id string, axis motion.AxisName, position float64) error
	Stop(ctx context.Context, id string, axis motion.AxisName) error
}

// TraversalPointSink 遍历点结果的写入端口（用于 CSV 落盘等）
//
// 生命周期：
//   - InitializeTraversal(config) 在 Start 阶段调用一次，可创建文件并写入表头；
//   - WriteTraversalPoint(point) 每完成一个点调用一次；
//   - FinalizeTraversal()        在 Stop/Complete/Error 任意终态调用一次，flush 并关闭。
//
// 实现可在 InitializeTraversal/FinalizeTraversal 中无操作，以保持兼容。
type TraversalPointSink interface {
	InitializeTraversal(config traversal.Config) error
	WriteTraversalPoint(point traversal.PointResult) error
	FinalizeTraversal() error
}

type TraversalResultStore interface {
	Save(taskID string, status traversal.Status) error
	Get(taskID string) (traversal.Status, bool)
}

type TraversalOutputMode string

const (
	TraversalOutputCreate TraversalOutputMode = "create"
	TraversalOutputResume TraversalOutputMode = "resume"
)

type TraversalOutputSession struct {
	TaskID       string
	Mode         TraversalOutputMode
	Path         string
	HeaderHash   string
	CommittedSeq uint64
}

type TraversalRowSummary struct {
	CommitSeq uint64
	RowHash   string
}

type TraversalOutputState struct {
	Path       string
	HeaderHash string
	Rows       int
	CommitSeq  uint64
	TailValid  bool
}

type TraversalCSVPort interface {
	Open(ctx context.Context, session TraversalOutputSession) error
	Append(ctx context.Context, result traversal.PointResult) (TraversalRowSummary, error)
	Sync(ctx context.Context) error
	Inspect(ctx context.Context) (TraversalOutputState, error)
	TruncateAfter(ctx context.Context, commitSeq uint64) error
	Close(ctx context.Context) error
}

type TraversalResultLogPort interface {
	Open(ctx context.Context, session TraversalOutputSession) error
	AppendPrepared(ctx context.Context, result traversal.PointResult) error
	Sync(ctx context.Context) error
	ReadCommitted(ctx context.Context, commitSeq uint64) ([]traversal.PointResult, error)
	ValidateTail(ctx context.Context, commitSeq uint64) error
	TruncateAfter(ctx context.Context, commitSeq uint64) error
	Close(ctx context.Context) error
}

type TraversalCheckpointRef struct {
	TaskID string
	Path   string
}

type TraversalCheckpointPort interface {
	Save(ctx context.Context, checkpoint traversal.Checkpoint) error
	Load(ctx context.Context, taskID string) (traversal.Checkpoint, error)
	Find(ctx context.Context, taskID string) (TraversalCheckpointRef, bool, error)
	Unregister(ctx context.Context, taskID string) error
}

// TraversalCheckpointPortFactory 按 SavePath 动态创建断点端口。
// 由于每个遍历任务的 SavePath 不同，断点文件路径需在 Start 时按 SavePath 确定，
// 不能在装配阶段静态注入。装配根提供工厂，usecase 在 Start 时调用。
type TraversalCheckpointPortFactory interface {
	// Create 按 basePath（通常为 config.SavePath）创建断点端口实例。
	Create(basePath string) TraversalCheckpointPort
}

// TraversalActiveIndex 活动任务索引：taskId → checkpointPath。
// 支持进程重启后发现未完成的遍历任务，实现断点续跑。
type TraversalActiveIndex interface {
	Register(ctx context.Context, taskID, checkpointPath string) error
	Find(ctx context.Context, taskID string) (TraversalCheckpointRef, bool, error)
	Unregister(ctx context.Context, taskID string) error
}

// CheckpointStore 断点文件存储端口
// 抽象断点文件的字节 I/O（Stat/Read/Write/Remove/Rename），
// 使 usecase 不直接依赖 os。实现见 adapters/storage.FileCheckpointStore。
type CheckpointStore interface {
	// Stat 返回路径是否存在；exists=false 表示文件不存在
	Stat(path string) (exists bool, err error)
	// Read 读取文件全部内容
	Read(path string) ([]byte, error)
	// Write 原子写入：先写 tmpPath 再 rename 到 path
	Write(path string, data []byte) error
	// Remove 删除文件（不存在时返回 nil）
	Remove(path string) error
}

// ChannelUnitProvider 提供指定设备通道的工程单位查询。
//
// 为什么需要此端口：遍历测试压力归一化（BuildRawPressure）需要查每个通道的 Unit
// 才能换算到 Pa，但 LatestDataReader（ports/calibration.go）只暴露 GetLatestData /
// GetLatestTimestamp，不暴露 ChannelConfig。通过此窄端口让 TraversalManager
// 在不直接依赖 usecase 兄弟包（DeviceManager）的前提下获得单位查询能力。
//
// 实现由 DeviceManager 提供（持有 profiles），装配点通过 SetUnitProvider 注入。
// 通道不存在或设备未找到时返回 error，调用方按 error 决定是否走降级路径。
type ChannelUnitProvider interface {
	// ChannelUnit 返回指定设备通道的工程单位字符串（如 "Pa"/"kPa"/"MPa"）。
	ChannelUnit(deviceID string, channelIndex int) (string, error)
}
