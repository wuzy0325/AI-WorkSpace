//go:build !windows

package veh

import "fmt"

type ExceptionCode uint32

const (
	EXCEPTION_ACCESS_VIOLATION         ExceptionCode = 0xC0000005
	EXCEPTION_BREAKPOINT               ExceptionCode = 0x80000003
	EXCEPTION_SINGLE_STEP              ExceptionCode = 0x80000004
	EXCEPTION_GUARD_PAGE               ExceptionCode = 0x80000001
	EXCEPTION_ILLEGAL_INSTRUCTION      ExceptionCode = 0xC000001D
	EXCEPTION_IN_PAGE_ERROR            ExceptionCode = 0xC0000006
	EXCEPTION_INTEGER_DIVIDE_BY_ZERO   ExceptionCode = 0xC0000094
	EXCEPTION_FLOAT_DIVIDE_BY_ZERO     ExceptionCode = 0xC000008E
	EXCEPTION_STACK_OVERFLOW           ExceptionCode = 0xC00000FD
	EXCEPTION_PRIV_INSTRUCTION         ExceptionCode = 0xC0000096
	EXCEPTION_ARRAY_BOUNDS_EXCEEDED    ExceptionCode = 0xC000008C
	EXCEPTION_DATATYPE_MISALIGNMENT    ExceptionCode = 0x80000002
	EXCEPTION_FLT_DENORMAL_OPERAND     ExceptionCode = 0xC000008D
	EXCEPTION_FLT_INEXACT_RESULT       ExceptionCode = 0xC000008F
	EXCEPTION_FLT_INVALID_OPERATION    ExceptionCode = 0xC0000090
	EXCEPTION_FLT_OVERFLOW             ExceptionCode = 0xC0000091
	EXCEPTION_FLT_STACK_CHECK          ExceptionCode = 0xC0000092
	EXCEPTION_FLT_UNDERFLOW            ExceptionCode = 0xC0000093
	EXCEPTION_INTEGER_OVERFLOW         ExceptionCode = 0xC0000095
	EXCEPTION_NONCONTINUABLE_EXCEPTION ExceptionCode = 0xC0000025
)

type AccessType int

const (
	AccessRead    AccessType = 0
	AccessWrite   AccessType = 1
	AccessExecute AccessType = 8
)

type ExceptionInfo struct {
	Code       ExceptionCode
	Address    uintptr
	Flags      uint32
	AccessAddr uintptr
	AccessType AccessType
}

type Action int

const (
	ActionContinueSearch    Action = iota
	ActionContinueExecution
)

type HandlerFunc func(info *ExceptionInfo) Action

func Start(h HandlerFunc) {
}

func Stop() {
}

func init() {
	_ = fmt.Errorf("veh: Vectored Exception Handler is only supported on Windows")
}
