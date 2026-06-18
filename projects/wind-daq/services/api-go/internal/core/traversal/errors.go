// Package traversal: domain-specific sentinel errors for traversal workflow.
//
// 与 Cursor DAQ TraversalErrors.ts 对齐，但采用 Go 习惯：用 sentinel error +
// errors.Is/As 区分类别。每个 sentinel 都关联一个 ErrorCode，便于统一上报。
package traversal

import (
	"errors"
	"fmt"
)

// 各类 sentinel error；上层用 errors.Is(err, ErrSentinelMotion) 等区分类别
var (
	ErrSentinelMotion        = errors.New("traversal motion error")
	ErrSentinelSampling      = errors.New("traversal sampling error")
	ErrSentinelPersistence   = errors.New("traversal persistence error")
	ErrSentinelInterpolation = errors.New("traversal interpolation error")
	ErrSentinelCancelled     = errors.New("traversal cancelled")
	ErrSentinelPaused        = errors.New("traversal paused")
)

// CodedError 带 ErrorCode 的领域错误
// 同时实现 Unwrap，支持 errors.Is/As 判断类别 sentinel
type CodedError struct {
	Code    ErrorCode
	Message string
	cause   error // 包装的根因（可为 nil）
}

// Error 满足 error 接口
func (e *CodedError) Error() string {
	if e.cause != nil {
		return fmt.Sprintf("[%s] %s: %v", e.Code, e.Message, e.cause)
	}
	return fmt.Sprintf("[%s] %s", e.Code, e.Message)
}

// Unwrap 返回 sentinel 父类与 cause 之一（按 errors.Is 顺序优先匹配 sentinel）
// Go 1.20+ 支持多 unwrap：返回 []error
func (e *CodedError) Unwrap() []error {
	parents := []error{}
	switch e.Code {
	case ErrMotionFailed:
		parents = append(parents, ErrSentinelMotion)
	case ErrAcquisitionFailed:
		parents = append(parents, ErrSentinelSampling)
	case ErrSaveFailed:
		parents = append(parents, ErrSentinelPersistence)
	case ErrInterpolationFailed:
		parents = append(parents, ErrSentinelInterpolation)
	}
	if e.cause != nil {
		parents = append(parents, e.cause)
	}
	return parents
}

// NewCodedError 构造带错误码的领域错误
func NewCodedError(code ErrorCode, format string, args ...any) *CodedError {
	return &CodedError{Code: code, Message: fmt.Sprintf(format, args...)}
}

// WrapCodedError 包装根因
func WrapCodedError(code ErrorCode, cause error, format string, args ...any) *CodedError {
	return &CodedError{Code: code, Message: fmt.Sprintf(format, args...), cause: cause}
}
