//go:build windows

package veh

/*
#include <windows.h>

typedef int (*veh_callback_t)(DWORD code, ULONG_PTR addr, DWORD flags, ULONG_PTR accessAddr, int accessType);

extern void veh_start(void);
extern void veh_stop(void);
extern void veh_init_callback(void);
extern int veh_raise_exception(DWORD code, int *handled);

extern void veh_simple_start(void);
extern void veh_simple_stop(void);
extern int veh_simple_raise(DWORD code);
extern void veh_trigger_read(void *addr);

extern int goVEHCallback(DWORD, ULONG_PTR, DWORD, ULONG_PTR, int);
*/
import "C"
import (
	"sync/atomic"
	"unsafe"
)

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

var (
	handler atomic.Value
	started atomic.Bool
)

//export goVEHCallback
func goVEHCallback(code C.DWORD, addr C.ULONG_PTR, flags C.DWORD, accessAddr C.ULONG_PTR, accessType C.int) C.int {
	h, _ := handler.Load().(HandlerFunc)
	if h == nil {
		return 0
	}

	info := &ExceptionInfo{
		Code:       ExceptionCode(code),
		Address:    uintptr(addr),
		Flags:      uint32(flags),
		AccessAddr: uintptr(accessAddr),
		AccessType: AccessType(accessType),
	}

	action := h(info)
	return C.int(action)
}

func Start(h HandlerFunc) {
	if started.Swap(true) {
		return
	}

	handler.Store(h)
	C.veh_init_callback()
	C.veh_start()
}

func simpleStart()  { C.veh_simple_start() }
func simpleStop()   { C.veh_simple_stop() }
func simpleRaise(code uint32) bool { return C.veh_simple_raise(C.DWORD(code)) != 0 }
func triggerRead(addr unsafe.Pointer) { C.veh_trigger_read(addr) }

func RaiseException(code ExceptionCode) (handledByVEH bool) {
	var cHandled C.int
	C.veh_raise_exception(C.DWORD(code), &cHandled)
	return cHandled != 0
}

func Stop() {
	if !started.Swap(false) {
		return
	}

	C.veh_stop()
	handler.Store((HandlerFunc)(nil))
}
