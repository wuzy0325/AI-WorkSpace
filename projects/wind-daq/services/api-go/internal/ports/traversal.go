package ports

import "wind-daq/services/api-go/internal/core/traversal"

type TraversalPointSink interface {
	WriteTraversalPoint(point traversal.PointResult) error
}

type TraversalResultStore interface {
	Save(taskID string, status traversal.Status) error
	Get(taskID string) (traversal.Status, bool)
}
