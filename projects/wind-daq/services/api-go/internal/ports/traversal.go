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

	// 列配置（v2 Open 路径必须应用，否则表头缺少通道列）：
	//   - SaveOptions     控制哪些列输出；nil 时由 adapter 兜底为全开
	//   - Channels        原始压力通道索引列表，用于稳定列顺序
	//   - ChannelLabels   通道索引→标签映射（如 {0:"P1", 16:"Patm"}），可空
	// 这些字段在 v2 装配下由 usecase 从 traversal.Config 复制；
	// 旧 sink 路径走 InitializeTraversal(config)，不消费这些字段，保持向后兼容。
	SaveOptions     *traversal.SaveOptions
	Channels        []int
	ChannelLabels   map[int]string
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
	// OutputPath 返回 Open 后实际落盘的 CSV 文件路径。
	// 撞名 -2/-3 场景下，实际路径可能与 session.Path 不同；调用方必须用本方法
	// 拿真实路径回写 snapshot.CSVPath / 派生 checkpoint 路径，否则崩溃恢复会
	// 打开错误的 CSV 文件污染旧数据。
	OutputPath() string
}

type TraversalResultLogPort interface {
	Open(ctx context.Context, session TraversalOutputSession) error
	AppendPrepared(ctx context.Context, result traversal.PointResult) error
	Sync(ctx context.Context) error
	ReadCommitted(ctx context.Context, commitSeq uint64) ([]traversal.PointResult, error)
	ValidateTail(ctx context.Context, commitSeq uint64) error
	TruncateAfter(ctx context.Context, commitSeq uint64) error
	Close(ctx context.Context) error
	// OutputPath 返回 Open 后实际落盘的结果日志路径。
	// 撞名场景下实际路径可能与 session.Path 不同，调用方应回写 snapshot.ResultLogPath。
	OutputPath() string
}

type TraversalCheckpointRef struct {
	TaskID string
	Path   string
}

// TraversalCheckpointPort 断点端口的完整生命周期接口。
//
// 生命周期：
//   - Save / Load / Find / Unregister 在任务运行期被 usecase 调用
//   - Close 在任务结束（Stop / Complete / Error）时由 finalizeSink 调用，
//     释放底层资源（文件句柄/锁）。FileCheckpointPort 当前无句柄资源，
//     Close 仍必须实现以满足接口契约，并便于未来切换到带句柄的实现。
type TraversalCheckpointPort interface {
	Save(ctx context.Context, checkpoint traversal.Checkpoint) error
	Load(ctx context.Context, taskID string) (traversal.Checkpoint, error)
	Find(ctx context.Context, taskID string) (TraversalCheckpointRef, bool, error)
	Unregister(ctx context.Context, taskID string) error
	Close(ctx context.Context) error
	// SetBasePath 在 csvPort.Open 成功后由 usecase 调用，
	// 把 basePath 从"预期 CSV 路径"切换为"实际落盘 CSV 路径"（含 -2/-3 撞名后缀）。
	// 实现必须保证后续 Save/Load/Find/Unregister 派生的路径与新 basePath 一致。
	SetBasePath(csvPath string)
}

// TraversalCheckpointPortFactory 按 SavePath 动态创建断点端口。
// 由于每个遍历任务的 SavePath 不同，断点文件路径需在 Start 时按 SavePath 确定，
// 不能在装配阶段静态注入。装配根提供工厂，usecase 在 Start 时调用。
//
// Create 返回 error 用于未来扩展（如基于 mmap / DB 的实现需要初始化连接）；
// 当前 FileCheckpointPort 实现始终返回 nil error。
type TraversalCheckpointPortFactory interface {
	// Create 按 basePath（通常为 config.SavePath）创建断点端口实例。
	Create(ctx context.Context, basePath string) (TraversalCheckpointPort, error)
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
