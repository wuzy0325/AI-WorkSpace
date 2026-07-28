// Package runtime 提供基于进程内共享运行时状态（resourcelock.Service）的
// lease adapter：只做锁协议翻译，不含业务逻辑。
//
// 对应 docs/specs/dual-traversal-spec.md I2：
//   - WorkflowLease：registry 以固定 holder 持有全局 workflow:traversal lease；
//   - ControllerLease：controller-scoped resource + opaque token holder，
//     token-checked Acquire/Renew/Release。
package runtime

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"time"

	"wind-daq/services/api-go/internal/core/resourcelock"
	"wind-daq/services/api-go/internal/ports"
)

// WorkflowTraversalResource 全局遍历工作流锁资源名（与 usecase 既有约定一致）。
const WorkflowTraversalResource = "workflow:traversal"

// controllerResourceKeyPrefix 控制器资源锁名前缀，后接 controllerID。
const controllerResourceKeyPrefix = "controller:traversal:"

// ErrUnknownLeaseToken lease token 未知（从未签发或已释放）。
var ErrUnknownLeaseToken = errors.New("unknown lease token")

// ErrLeaseNotHeld lease 已不由该 token/holder 持有（过期或被新 session 接管）。
var ErrLeaseNotHeld = errors.New("lease not held")

func controllerResourceKey(controllerID string) string {
	return controllerResourceKeyPrefix + controllerID
}

// WorkflowLease 固定 resource 的全局工作流 lease adapter。
type WorkflowLease struct {
	svc *resourcelock.Service
}

var _ ports.WorkflowLeasePort = (*WorkflowLease)(nil)

// NewWorkflowLease 基于共享锁服务创建工作流 lease adapter。
func NewWorkflowLease(svc *resourcelock.Service) *WorkflowLease {
	return &WorkflowLease{svc: svc}
}

// Acquire 获取全局工作流 lease；被其它 holder 占用且未过期时返回冲突错误。
func (l *WorkflowLease) Acquire(ctx context.Context, holder string, ttl time.Duration) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return l.svc.Acquire(WorkflowTraversalResource, holder, ttl)
}

// Renew 续约既有 lease；holder 必须仍是当前持有者。
// 不直接调用 svc.Acquire 兜底：未持有时 Acquire 会隐式重建 lease，违背续约语义。
func (l *WorkflowLease) Renew(ctx context.Context, holder string, ttl time.Duration) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	held, current := l.svc.IsHeld(WorkflowTraversalResource)
	if !held || current != holder {
		return fmt.Errorf("%w: workflow lease holder=%s", ErrLeaseNotHeld, holder)
	}
	return l.svc.Acquire(WorkflowTraversalResource, holder, ttl)
}

// Release 释放 lease；holder 不匹配返回错误，不存在/已过期幂等成功。
func (l *WorkflowLease) Release(ctx context.Context, holder string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return l.svc.Release(WorkflowTraversalResource, holder)
}

// controllerLeaseEntry 已签发 token 的登记信息。
type controllerLeaseEntry struct {
	resource string
	holder   string // 诊断用身份（如 session/probe 标识）
}

// ControllerLease controller-scoped、opaque token 校验的资源 lease adapter。
//
// Acquire 以 crypto/rand 生成的 opaque token 作为底层锁的唯一 holder，
// token 不可由 probe ID/controllerID 推导；Renew/Release 只认 token。
// 旧 generation token 在 lease 过期被接管后不能续约或释放新 session 的 lease。
type ControllerLease struct {
	svc *resourcelock.Service
	mu  sync.Mutex
	// issued 已签发 token → 资源/诊断身份。session 数量有界（每控制器一个活跃
	// lease），过期未释放的残留条目不影响正确性，故不做后台清理。
	issued map[string]controllerLeaseEntry
}

var _ ports.ControllerLeasePort = (*ControllerLease)(nil)

// NewControllerLease 基于共享锁服务创建控制器 lease adapter。
func NewControllerLease(svc *resourcelock.Service) *ControllerLease {
	return &ControllerLease{svc: svc, issued: make(map[string]controllerLeaseEntry)}
}

// Acquire 原子预占 controllerID 对应的控制器资源，成功返回 opaque leaseToken。
// 底层 svc.Acquire 持锁判断+写入，并发争抢同一控制器时只有一个调用成功。
func (l *ControllerLease) Acquire(ctx context.Context, controllerID, holder string, ttl time.Duration) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if controllerID == "" {
		return "", errors.New("controller lease acquire: controller ID is required")
	}
	token, err := newLeaseToken()
	if err != nil {
		return "", err
	}
	resource := controllerResourceKey(controllerID)
	if err := l.svc.Acquire(resource, token, ttl); err != nil {
		return "", fmt.Errorf("controller lease acquire %s: %w", controllerID, err)
	}
	l.mu.Lock()
	l.issued[token] = controllerLeaseEntry{resource: resource, holder: holder}
	l.mu.Unlock()
	return token, nil
}

// Renew 续约 leaseToken 对应的 lease；token 未知、已过期或已被接管时返回错误。
func (l *ControllerLease) Renew(ctx context.Context, leaseToken string, ttl time.Duration) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	entry, err := l.entryFor(leaseToken)
	if err != nil {
		return err
	}
	// 先确认仍由本 token 持有，再续约；避免过期后 Acquire 语义隐式重建 lease。
	// 与 svc.Acquire 之间存在 TOCTOU 窗口，但 svc.Acquire 同 holder 才续约、
	// 被接管时返回 ErrLockHeld，故最坏结果是续约失败而非误续约他人 lease。
	held, current := l.svc.IsHeld(entry.resource)
	if !held || current != leaseToken {
		return fmt.Errorf("%w: controller lease token=%s", ErrLeaseNotHeld, leaseToken)
	}
	return l.svc.Acquire(entry.resource, leaseToken, ttl)
}

// Release 释放 leaseToken 对应的 lease；token 未知或已非当前持有者时返回错误。
func (l *ControllerLease) Release(ctx context.Context, leaseToken string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	entry, err := l.entryFor(leaseToken)
	if err != nil {
		return err
	}
	if err := l.svc.Release(entry.resource, leaseToken); err != nil {
		return fmt.Errorf("controller lease release: %w", err)
	}
	l.mu.Lock()
	delete(l.issued, leaseToken)
	l.mu.Unlock()
	return nil
}

func (l *ControllerLease) entryFor(leaseToken string) (controllerLeaseEntry, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	entry, ok := l.issued[leaseToken]
	if !ok {
		return controllerLeaseEntry{}, fmt.Errorf("%w: %s", ErrUnknownLeaseToken, leaseToken)
	}
	return entry, nil
}

// newLeaseToken 生成 opaque lease token（128 bit 随机数，不可由外部身份推导）。
func newLeaseToken() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("generate lease token: %w", err)
	}
	return "cleas-" + hex.EncodeToString(b[:]), nil
}
