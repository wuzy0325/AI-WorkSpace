package runtime

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"wind-daq/services/api-go/internal/ports"
)

// TaskIDGenerator 服务端权威任务 ID 生成器（spec I5 / Task 14 装配）。
//
// dual task ID = probe 命名空间 + 毫秒时间戳 + 64bit 随机数：
// 不可由客户端推导，进程内及持久化范围全局唯一，probe1/probe2 不会在
// 结果 store、活动索引或 checkpoint 中发生键冲突。客户端提交的 task ID
// 在 registry Start 中被忽略并覆盖。
type TaskIDGenerator struct{}

var _ ports.TaskIDGenerator = (*TaskIDGenerator)(nil)

// NewTaskIDGenerator 创建任务 ID 生成器（无状态，可共享）。
func NewTaskIDGenerator() *TaskIDGenerator {
	return &TaskIDGenerator{}
}

// NewTaskID 生成 probe 命名空间的全局唯一任务 ID。
func (g *TaskIDGenerator) NewTaskID(ctx context.Context, probeID string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if probeID == "" {
		return "", fmt.Errorf("probe ID is required")
	}
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("generate task id: %w", err)
	}
	return fmt.Sprintf("%s-task-%d-%s", probeID, time.Now().UnixMilli(), hex.EncodeToString(b[:])), nil
}
