package usecase

import (
	"context"
	"time"
)

// withoutCancel 返回一个不受父 context 取消影响的副本（Go 1.21 标准库
// context.WithoutCancel 的 win7 兼容实现，Go 1.20 无此 API）。
// 用于回滚释放等必须在调用方取消后仍能完成的动作；保留父 context 的值，
// 但 Deadline/Done/Err 均被剥离，与标准库语义一致。
func withoutCancel(parent context.Context) context.Context {
	if parent == nil {
		panic("cannot create context from nil parent")
	}
	return &withoutCancelCtx{parent}
}

type withoutCancelCtx struct {
	context.Context
}

func (c *withoutCancelCtx) Deadline() (deadline time.Time, ok bool) {
	return time.Time{}, false
}

func (c *withoutCancelCtx) Done() <-chan struct{} {
	return nil
}

func (c *withoutCancelCtx) Err() error {
	return nil
}

// cloneMap 浅拷贝 map（Go 1.21 标准库 maps.Clone 的 win7 兼容实现）。
func cloneMap[K comparable, V any](m map[K]V) map[K]V {
	if m == nil {
		return nil
	}
	cp := make(map[K]V, len(m))
	for k, v := range m {
		cp[k] = v
	}
	return cp
}
