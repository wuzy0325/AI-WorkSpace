// Package resourcelock 提供命名互斥锁（带 TTL）的 Go 实现，
// 对应 Cursor DAQ ResourceLockService.ts。
//
// 用途：在多个工作流（traversal / calibration 等）间互斥访问共享硬件资源
// （运动控制器、采集设备）。锁带 TTL 自动过期，避免崩溃后死锁。
package resourcelock

import (
	"errors"
	"fmt"
	"sync"
	"time"
)

// ErrLockHeld 表示资源已被其他持有者占用
var ErrLockHeld = errors.New("resource lock is currently held")

// lockEntry 单条锁记录
type lockEntry struct {
	holder   string
	acquired time.Time
	ttl      time.Duration
}

// expired 是否已过期（TTL <= 0 视为永不过期）
func (e *lockEntry) expired(now time.Time) bool {
	if e.ttl <= 0 {
		return false
	}
	return now.Sub(e.acquired) >= e.ttl
}

// Service 命名锁服务
type Service struct {
	mu    sync.Mutex
	locks map[string]*lockEntry
}

// New 创建一个空的锁服务
func New() *Service {
	return &Service{locks: make(map[string]*lockEntry)}
}

// Acquire 尝试获取锁
//   - resource:   资源名，例如 "workflow:traversal" / "workflow:calibration"
//   - holder:     持有者标识（任务 ID / 服务名）
//   - ttl:        锁有效期；≤0 表示永不过期（必须显式 Release）
//
// 锁已过期时自动取代旧持有者。若同一 holder 重新 Acquire 视为续约。
func (s *Service) Acquire(resource, holder string, ttl time.Duration) error {
	if resource == "" {
		return fmt.Errorf("resource name is required")
	}
	if holder == "" {
		return fmt.Errorf("holder is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	if existing, ok := s.locks[resource]; ok {
		if existing.holder == holder {
			// 续约：更新 acquired/ttl
			existing.acquired = now
			existing.ttl = ttl
			return nil
		}
		if !existing.expired(now) {
			return fmt.Errorf("%w: resource=%s holder=%s", ErrLockHeld, resource, existing.holder)
		}
		// 已过期 → 允许接管
	}
	s.locks[resource] = &lockEntry{holder: holder, acquired: now, ttl: ttl}
	return nil
}

// Release 释放锁；仅持有者可释放（持有者不匹配返回错误）
//   - 若锁不存在或已过期，视为成功（幂等）
func (s *Service) Release(resource, holder string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	existing, ok := s.locks[resource]
	if !ok {
		return nil
	}
	if existing.expired(time.Now()) {
		delete(s.locks, resource)
		return nil
	}
	if existing.holder != holder {
		return fmt.Errorf("release denied: resource=%s held by %s, not %s", resource, existing.holder, holder)
	}
	delete(s.locks, resource)
	return nil
}

// IsHeld 查询锁状态
func (s *Service) IsHeld(resource string) (held bool, holder string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	existing, ok := s.locks[resource]
	if !ok {
		return false, ""
	}
	if existing.expired(time.Now()) {
		delete(s.locks, resource)
		return false, ""
	}
	return true, existing.holder
}

// 全局单例（与 Cursor DAQ 一致：进程内共享）
var (
	defaultOnce sync.Once
	defaultSvc  *Service
)

// Default 返回进程级默认锁服务
func Default() *Service {
	defaultOnce.Do(func() {
		defaultSvc = New()
	})
	return defaultSvc
}
