package usecase

import (
	"context"
	"testing"

	"wind-daq/services/api-go/internal/core/traversal"
	"wind-daq/services/api-go/internal/ports"
)

type traversalPortContractFake struct{}

func (traversalPortContractFake) Open(context.Context, ports.TraversalOutputSession) error {
	return nil
}
func (traversalPortContractFake) Append(context.Context, traversal.PointResult) (ports.TraversalRowSummary, error) {
	return ports.TraversalRowSummary{}, nil
}
func (traversalPortContractFake) Sync(context.Context) error { return nil }
func (traversalPortContractFake) Inspect(context.Context) (ports.TraversalOutputState, error) {
	return ports.TraversalOutputState{}, nil
}
func (traversalPortContractFake) TruncateAfter(context.Context, uint64) error { return nil }
func (traversalPortContractFake) Close(context.Context) error                 { return nil }
func (traversalPortContractFake) OutputPath() string                          { return "" }

func (traversalPortContractFake) AppendPrepared(context.Context, traversal.PointResult) error {
	return nil
}
func (traversalPortContractFake) ReadCommitted(context.Context, uint64) ([]traversal.PointResult, error) {
	return nil, nil
}
func (traversalPortContractFake) ValidateTail(context.Context, uint64) error { return nil }

func (traversalPortContractFake) Save(context.Context, traversal.Checkpoint) error { return nil }
func (traversalPortContractFake) Load(context.Context, string) (traversal.Checkpoint, error) {
	return traversal.Checkpoint{}, nil
}
func (traversalPortContractFake) Find(context.Context, string) (ports.TraversalCheckpointRef, bool, error) {
	return ports.TraversalCheckpointRef{}, false, nil
}
func (traversalPortContractFake) Unregister(context.Context, string) error { return nil }
func (traversalPortContractFake) SetBasePath(string)                       {}

func TestTraversalCSVPortContract(t *testing.T) {
	var port ports.TraversalCSVPort = traversalPortContractFake{}
	if err := port.Open(context.Background(), ports.TraversalOutputSession{Mode: ports.TraversalOutputCreate}); err != nil {
		t.Fatalf("open create session: %v", err)
	}
	if err := port.Open(context.Background(), ports.TraversalOutputSession{Mode: ports.TraversalOutputResume}); err != nil {
		t.Fatalf("open resume session: %v", err)
	}
}

func TestTraversalResultLogPortContract(t *testing.T) {
	var port ports.TraversalResultLogPort = traversalPortContractFake{}
	if err := port.AppendPrepared(context.Background(), traversal.PointResult{CommitSeq: 1}); err != nil {
		t.Fatalf("append prepared result: %v", err)
	}
}

func TestTraversalCheckpointPortContract(t *testing.T) {
	var port ports.TraversalCheckpointPort = traversalPortContractFake{}
	if err := port.Save(context.Background(), traversal.Checkpoint{TaskID: "task-1"}); err != nil {
		t.Fatalf("save checkpoint: %v", err)
	}
}
