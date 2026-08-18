// Package motiontask 提供运动任务的统一执行器（settle 等待 + cancel + pause + timeout）。
//
// 对应 Cursor DAQ MotionTaskExecutor.ts，把 traversal/calibration 等共用的
// "下发 → 等待到位 → 中途取消/暂停 → 超时失败" 模式抽取到一处，避免每个 usecase
// 各自重复实现 ticker + deadline 逻辑。
package motiontask

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// AxisChecker 单轴到位查询函数：返回 (settled, error)
//   - settled = true 表示该轴已到位
//   - error 非 nil 表示设备查询失败（执行器据此立即失败退出）
type AxisChecker func(ctx context.Context) (settled bool, err error)

// CancelChecker 中断检查函数：周期性调用以判断是否需要终止等待
//   - cancelled = true 表示外部请求停止/暂停（执行器返回 ErrCancelled）
type CancelChecker func() (cancelled bool, paused bool)

// Options 执行器配置
type Options struct {
	// Timeout 单次等待最大时长；≤0 取默认 120s
	Timeout time.Duration
	// PollInterval 轮询间隔；≤0 取默认 100ms
	PollInterval time.Duration
}

// 默认参数（与 Cursor DAQ MotionTaskExecutor 保持一致）
const (
	DefaultTimeout      = 120 * time.Second
	DefaultPollInterval = 100 * time.Millisecond
)

// 错误集合
var (
	ErrTimeout   = errors.New("motion settle timeout")
	ErrCancelled = errors.New("motion task cancelled")
	ErrPaused    = errors.New("motion task paused")
)

// WaitSettled 等待全部 axis 到位，期间周期性检查 cancel/pause
//   - axes:  每条对应一个轴的到位查询函数；任一查询失败立即返回错误
//   - cancelFn: 外部停止/暂停信号；nil 表示不监听外部信号
//   - opts: 超时/轮询配置；零值取默认
//
// 返回值：所有轴都 settled 时返回 nil；任一阶段失败/超时/取消返回对应 error。
func WaitSettled(ctx context.Context, axes []AxisChecker, cancelFn CancelChecker, opts Options) error {
	if opts.Timeout <= 0 {
		opts.Timeout = DefaultTimeout
	}
	if opts.PollInterval <= 0 {
		opts.PollInterval = DefaultPollInterval
	}
	if len(axes) == 0 {
		return nil
	}

	deadline := time.Now().Add(opts.Timeout)
	ticker := time.NewTicker(opts.PollInterval)
	defer ticker.Stop()

	for {
		// 优先检查 ctx / cancel 信号
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("motion wait: %w", err)
		}
		if cancelFn != nil {
			if cancelled, paused := cancelFn(); cancelled || paused {
				if paused {
					return ErrPaused
				}
				return ErrCancelled
			}
		}

		// 检查全部轴是否到位
		allSettled := true
		for _, check := range axes {
			settled, err := check(ctx)
			if err != nil {
				return fmt.Errorf("motion check failed: %w", err)
			}
			if !settled {
				allSettled = false
				break
			}
		}
		if allSettled {
			return nil
		}

		// 超时判断
		if time.Now().After(deadline) {
			return ErrTimeout
		}
		// 等待下一次轮询
		select {
		case <-ctx.Done():
			return fmt.Errorf("motion wait: %w", ctx.Err())
		case <-ticker.C:
		}
	}
}
