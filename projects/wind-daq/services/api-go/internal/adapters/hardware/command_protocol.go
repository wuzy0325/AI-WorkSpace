package hardware

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

const DefaultCommandTimeoutMs = 2000

var nextPendingID uint64

type PendingResponse struct {
	ID           uint64
	Command      string
	Resolve      func(response string)
	Reject       func(err error)
	TimeoutTimer *time.Timer
}

type CommandProtocol struct {
	mu               sync.Mutex
	pendingResponses []PendingResponse
}

func NewCommandProtocol() *CommandProtocol {
	return &CommandProtocol{}
}

func (cp *CommandProtocol) Enqueue(pending PendingResponse) {
	cp.mu.Lock()
	defer cp.mu.Unlock()
	cp.pendingResponses = append(cp.pendingResponses, pending)
}

func (cp *CommandProtocol) Dequeue() (PendingResponse, bool) {
	cp.mu.Lock()
	defer cp.mu.Unlock()
	if len(cp.pendingResponses) == 0 {
		return PendingResponse{}, false
	}
	p := cp.pendingResponses[0]
	cp.pendingResponses = cp.pendingResponses[1:]
	return p, true
}

func (cp *CommandProtocol) RemoveByID(id uint64) {
	cp.mu.Lock()
	defer cp.mu.Unlock()
	for i, p := range cp.pendingResponses {
		if p.ID == id {
			cp.pendingResponses = append(cp.pendingResponses[:i], cp.pendingResponses[i+1:]...)
			return
		}
	}
}

func (cp *CommandProtocol) Len() int {
	cp.mu.Lock()
	defer cp.mu.Unlock()
	return len(cp.pendingResponses)
}

func (cp *CommandProtocol) Clear() {
	cp.mu.Lock()
	defer cp.mu.Unlock()
	for _, p := range cp.pendingResponses {
		if p.TimeoutTimer != nil {
			p.TimeoutTimer.Stop()
		}
	}
	cp.pendingResponses = nil
}

func (cp *CommandProtocol) SendCommandAndWait(command string, sendFn func() error, timeoutMs int) (string, error) {
	if timeoutMs <= 0 {
		timeoutMs = DefaultCommandTimeoutMs
	}

	pendingID := atomic.AddUint64(&nextPendingID, 1)
	resultCh := make(chan string, 1)
	errCh := make(chan error, 1)

	pending := PendingResponse{
		ID:      pendingID,
		Command: command,
		Resolve: func(response string) {
			select {
			case resultCh <- response:
			default:
			}
		},
		Reject: func(err error) {
			select {
			case errCh <- err:
			default:
			}
		},
	}

	pending.TimeoutTimer = time.AfterFunc(time.Duration(timeoutMs)*time.Millisecond, func() {
		cp.RemoveByID(pendingID)
		pending.Reject(fmt.Errorf("timeout waiting for response to %q", command))
	})

	cp.Enqueue(pending)

	if err := sendFn(); err != nil {
		pending.TimeoutTimer.Stop()
		cp.RemoveByID(pendingID)
		return "", fmt.Errorf("send command: %w", err)
	}

	select {
	case resp := <-resultCh:
		return resp, nil
	case err := <-errCh:
		return "", err
	}
}

func (cp *CommandProtocol) DispatchResponse(response string) bool {
	cp.mu.Lock()
	if len(cp.pendingResponses) == 0 {
		cp.mu.Unlock()
		return false
	}
	p := cp.pendingResponses[0]
	cp.pendingResponses = cp.pendingResponses[1:]
	cp.mu.Unlock()

	if p.TimeoutTimer != nil {
		p.TimeoutTimer.Stop()
	}
	p.Resolve(response)
	return true
}
