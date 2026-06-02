package ports

import (
	"context"

	"shared.local/device-sdk/go/motion/core"

	"wind-daq/services/api-go/internal/core/traversal"
)

// MotionAccess 运动控制访问接口（遍历用例所需）
type MotionAccess interface {
	StatusAll(ctx context.Context) []core.ControllerStatus
	MoveTo(ctx context.Context, id string, axis core.AxisName, position float64) error
	Stop(ctx context.Context, id string, axis core.AxisName) error
}

type TraversalPointSink interface {
	WriteTraversalPoint(point traversal.PointResult) error
}

type TraversalResultStore interface {
	Save(taskID string, status traversal.Status) error
	Get(taskID string) (traversal.Status, bool)
}
