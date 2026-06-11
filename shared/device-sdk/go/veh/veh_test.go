//go:build windows

package veh

import (
	"syscall"
	"testing"
	"unsafe"
)

const testCode ExceptionCode = 0xE0000001

func TestVEHCallback(t *testing.T) {
	called := false

	Start(func(info *ExceptionInfo) Action {
		if info.Code == testCode {
			called = true
			return ActionContinueExecution
		}
		return ActionContinueSearch
	})
	defer Stop()

	handled := RaiseException(testCode)

	if !called {
		t.Fatal("VEH Go callback was not called")
	}
	if !handled {
		t.Fatal("exception should be reported as handled")
	}
}

func TestVEHCallbackMultiple(t *testing.T) {
	count := 0

	Start(func(info *ExceptionInfo) Action {
		if info.Code == testCode {
			count++
			return ActionContinueExecution
		}
		return ActionContinueSearch
	})
	defer Stop()

	for i := 0; i < 5; i++ {
		handled := RaiseException(testCode)
		if !handled {
			t.Fatal("exception not handled")
		}
	}

	if count != 5 {
		t.Fatalf("expected 5, got %d", count)
	}
}

func TestVEHCallbackInfo(t *testing.T) {
	var captured *ExceptionInfo

	Start(func(info *ExceptionInfo) Action {
		captured = info
		return ActionContinueExecution
	})
	defer Stop()

	RaiseException(testCode)

	if captured == nil {
		t.Fatal("exception info not captured")
	}
	if captured.Code != testCode {
		t.Fatalf("expected code 0x%X, got 0x%X", testCode, captured.Code)
	}
	t.Logf("exception code=0x%X addr=0x%X flags=0x%X",
		captured.Code, captured.Address, captured.Flags)
}

func TestVEHCallbackStop(t *testing.T) {
	called := false

	Start(func(info *ExceptionInfo) Action {
		called = true
		return ActionContinueExecution
	})

	handled := RaiseException(testCode)
	if !called || !handled {
		t.Fatal("handler should have been called before Stop")
	}

	Stop()
	called = false

	handled = RaiseException(testCode)
	if called {
		t.Fatal("handler should NOT be called after Stop")
	}
	if handled {
		t.Fatal("exception should NOT be handled after Stop")
	}
}

func TestVEHCallbackContinueSearch(t *testing.T) {
	handlerCalled := false

	Start(func(info *ExceptionInfo) Action {
		handlerCalled = true
		return ActionContinueSearch
	})
	defer Stop()

	handled := RaiseException(testCode)

	if !handlerCalled {
		t.Fatal("handler should be called")
	}
	if handled {
		t.Fatal("exception should NOT be handled when returning ActionContinueSearch")
	}
}

func TestVEHGuardPage(t *testing.T) {
	k32 := syscall.NewLazyDLL("kernel32.dll")
	virtualAlloc := k32.NewProc("VirtualAlloc")
	virtualProtect := k32.NewProc("VirtualProtect")
	virtualFree := k32.NewProc("VirtualFree")

	const (
		MEM_COMMIT     = 0x1000
		MEM_RESERVE    = 0x2000
		PAGE_READWRITE = 0x04
		PAGE_GUARD     = 0x100
		MEM_RELEASE    = 0x8000
	)

	addr, _, _ := virtualAlloc.Call(0, 4096, MEM_COMMIT|MEM_RESERVE, PAGE_READWRITE)
	if addr == 0 {
		t.Skip("VirtualAlloc failed")
	}
	defer virtualFree.Call(addr, 0, MEM_RELEASE)

	var oldProtect uint32
	virtualProtect.Call(addr, 4096, PAGE_READWRITE|PAGE_GUARD, uintptr(unsafe.Pointer(&oldProtect)))

	simpleStart()
	defer simpleStop()

	p := unsafe.Pointer(addr)
	_ = *(*byte)(p)

	t.Log("guard page exception intercepted and execution continued")
}

func TestVEHAccessViolationInfo(t *testing.T) {
	// Verify access violation fields are correctly parsed by raising
	// EXCEPTION_ACCESS_VIOLATION via RaiseException with fake parameters.
	var captured *ExceptionInfo

	Start(func(info *ExceptionInfo) Action {
		captured = info
		return ActionContinueExecution
	})
	defer Stop()

	// Use RaiseException with access violation parameters
	k32 := syscall.NewLazyDLL("kernel32.dll")
	raiseEx := k32.NewProc("RaiseException")
	raiseEx.Call(
		uintptr(EXCEPTION_ACCESS_VIOLATION),
		uintptr(0),
		uintptr(2),   // 2 parameters: access type + address
		uintptr(unsafe.Pointer(&[2]uintptr{
			uintptr(AccessWrite),
			0xDEADBEEF,
		})),
	)

	if captured == nil {
		t.Fatal("access violation not captured")
	}
	if captured.Code != EXCEPTION_ACCESS_VIOLATION {
		t.Fatalf("expected ACCESS_VIOLATION, got 0x%X", uint32(captured.Code))
	}
	if captured.AccessType != AccessWrite {
		t.Fatalf("expected AccessWrite, got %d", captured.AccessType)
	}
	if captured.AccessAddr != 0xDEADBEEF {
		t.Fatalf("expected 0xDEADBEEF, got 0x%X", captured.AccessAddr)
	}
	t.Logf("access violation info correct: code=0x%X type=%d addr=0x%X",
		captured.Code, captured.AccessType, captured.AccessAddr)
}
