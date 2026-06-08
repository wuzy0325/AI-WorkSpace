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

type TraversalPointSink interface {
	WriteTraversalPoint(point traversal.PointResult) error
}

type TraversalResultStore interface {
	Save(taskID string, status traversal.Status) error
	Get(taskID string) (traversal.Status, bool)
}
