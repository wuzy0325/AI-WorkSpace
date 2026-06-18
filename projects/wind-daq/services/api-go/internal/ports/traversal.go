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
