package driver

import (
	"math"
	"math/rand"
	"sync"
	"time"
)

// CircuitState 断路器状态。
type CircuitState string

const (
	CircuitClosed    CircuitState = "closed"
	CircuitOpen      CircuitState = "open"
	CircuitHalfOpen  CircuitState = "half_open"
)

// CircuitBreaker 断路器，防止故障设备影响系统，连续失败达到阈值后自动隔离。
type CircuitBreaker struct {
	mu                  sync.Mutex
	state               CircuitState
	failureCount        int
	lastFailureTime     time.Time
	halfOpenAttempts    int
	threshold           int
	timeout             time.Duration
	halfOpenMaxAttempts int
}

// CircuitBreakerConfig 断路器配置。
type CircuitBreakerConfig struct {
	Threshold           int
	Timeout             time.Duration
	HalfOpenMaxAttempts int
}

// DefaultCircuitBreakerConfig 返回默认断路器配置。
func DefaultCircuitBreakerConfig() CircuitBreakerConfig {
	return CircuitBreakerConfig{
		Threshold:           5,
		Timeout:             30 * time.Second,
		HalfOpenMaxAttempts: 3,
	}
}

// NewCircuitBreaker 创建断路器。
func NewCircuitBreaker(cfg CircuitBreakerConfig) *CircuitBreaker {
	if cfg.HalfOpenMaxAttempts <= 0 {
		cfg.HalfOpenMaxAttempts = 3
	}
	return &CircuitBreaker{
		state:               CircuitClosed,
		threshold:           cfg.Threshold,
		timeout:             cfg.Timeout,
		halfOpenMaxAttempts: cfg.HalfOpenMaxAttempts,
	}
}

// RecordSuccess 记录成功，重置失败计数和半开尝试。
func (cb *CircuitBreaker) RecordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.failureCount = 0
	cb.halfOpenAttempts = 0
	if cb.state == CircuitHalfOpen {
		cb.state = CircuitClosed
	}
}

// RecordFailure 记录失败，可能触发断路器打开。
func (cb *CircuitBreaker) RecordFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.failureCount++
	cb.lastFailureTime = time.Now()

	if cb.state == CircuitHalfOpen {
		cb.halfOpenAttempts++
		if cb.halfOpenAttempts >= cb.halfOpenMaxAttempts {
			cb.state = CircuitOpen
		}
	} else if cb.failureCount >= cb.threshold {
		cb.state = CircuitOpen
	}
}

// AllowRequest 检查是否允许请求通过。
func (cb *CircuitBreaker) AllowRequest() bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	if cb.state == CircuitOpen && !cb.lastFailureTime.IsZero() {
		if time.Since(cb.lastFailureTime) >= cb.timeout {
			cb.state = CircuitHalfOpen
			cb.halfOpenAttempts = 0
			return true
		}
		return false
	}
	return true
}

// State 返回当前状态。
func (cb *CircuitBreaker) State() CircuitState {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	if cb.state == CircuitOpen && !cb.lastFailureTime.IsZero() {
		if time.Since(cb.lastFailureTime) >= cb.timeout {
			cb.state = CircuitHalfOpen
			cb.halfOpenAttempts = 0
		}
	}
	return cb.state
}

// Reset 重置断路器到初始状态。
func (cb *CircuitBreaker) Reset() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.state = CircuitClosed
	cb.failureCount = 0
	cb.halfOpenAttempts = 0
	cb.lastFailureTime = time.Time{}
}

// ---------------------------------------------------------------------------
// RetryStrategy 指数退避重试策略
// ---------------------------------------------------------------------------

// RetryConfig 重试策略配置。
type RetryConfig struct {
	MaxAttempts    int
	BaseIntervalMs int
	MaxIntervalMs  int
	Multiplier     float64
	Jitter         bool
	JitterFactor   float64
}

// DefaultRetryConfig 返回默认重试配置。
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxAttempts:    5,
		BaseIntervalMs: 1000,
		MaxIntervalMs:  30000,
		Multiplier:     2,
		Jitter:         true,
		JitterFactor:   0.1,
	}
}

// RetryStrategy 指数退避重试策略。
type RetryStrategy struct {
	cfg RetryConfig
	rng *rand.Rand
}

// NewRetryStrategy 创建重试策略。
func NewRetryStrategy(cfg RetryConfig) *RetryStrategy {
	if cfg.MaxIntervalMs <= 0 {
		cfg.MaxIntervalMs = 60000
	}
	if cfg.Multiplier <= 0 {
		cfg.Multiplier = 2
	}
	return &RetryStrategy{
		cfg: cfg,
		rng: rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

// NextDelay 返回第 attemptCount 次重试的延迟（毫秒），返回 -1 表示停止重试。
func (rs *RetryStrategy) NextDelay(attemptCount int) int {
	if attemptCount >= rs.cfg.MaxAttempts {
		return -1
	}

	delay := float64(rs.cfg.BaseIntervalMs) * math.Pow(rs.cfg.Multiplier, float64(attemptCount))
	if float64(delay) > float64(rs.cfg.MaxIntervalMs) {
		delay = float64(rs.cfg.MaxIntervalMs)
	}

	if rs.cfg.Jitter {
		delay = delay * (1 + rs.rng.Float64()*rs.cfg.JitterFactor)
	}

	return int(math.Floor(delay))
}

// ShouldRetry 判断是否应该继续重试。
func (rs *RetryStrategy) ShouldRetry(attemptCount int) bool {
	return attemptCount < rs.cfg.MaxAttempts
}

// RemainingAttempts 返回剩余重试次数。
func (rs *RetryStrategy) RemainingAttempts(attemptCount int) int {
	if attemptCount >= rs.cfg.MaxAttempts {
		return 0
	}
	return rs.cfg.MaxAttempts - attemptCount
}
