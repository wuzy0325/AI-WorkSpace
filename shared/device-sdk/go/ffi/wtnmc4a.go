//go:build windows

package ffi

import (
	"fmt"
	"log/slog"
	"sync"
	"syscall"
	"unsafe"
)

var (
	initOnce   sync.Once
	initErr    error
	wtnmc4a    *syscall.DLL
	devCreateA *syscall.Proc
	devRelease *syscall.Proc
	reset      *syscall.Proc
	setSV      *syscall.Proc
	setV       *syscall.Proc
	setA       *syscall.Proc
	setDec     *syscall.Proc
	setP       *syscall.Proc
	setLP      *syscall.Proc
	setEP      *syscall.Proc
	initLVDV   *syscall.Proc
	startLVDV  *syscall.Proc
	decStop    *syscall.Proc
	instStop   *syscall.Proc
	readLP     *syscall.Proc
	readEP     *syscall.Proc
	getRR1     *syscall.Proc
	startHome  *syscall.Proc
	clearLimit *syscall.Proc
	setPDirLim *syscall.Proc
	setMDirLim *syscall.Proc
)

// Init loads the WTNMC4A DLL and resolves all function pointers.
// Safe to call multiple times; subsequent calls are no-ops.
func Init(dllPath string) error {
	initOnce.Do(func() {
		var err error
		wtnmc4a, err = syscall.LoadDLL(dllPath)
		if err != nil {
			initErr = fmt.Errorf("failed to load DLL %s: %w", dllPath, err)
			return
		}

		devCreateA = wtnmc4a.MustFindProc("WTNMC4A_DEV_CreateA")
		devRelease = wtnmc4a.MustFindProc("WTNMC4A_DEV_Release")
		reset = wtnmc4a.MustFindProc("WTNMC4A_Reset")
		setSV = wtnmc4a.MustFindProc("WTNMC4A_SetSV")
		setV = wtnmc4a.MustFindProc("WTNMC4A_SetV")
		setA = wtnmc4a.MustFindProc("WTNMC4A_SetA")
		setDec = wtnmc4a.MustFindProc("WTNMC4A_SetDec")
		setP = wtnmc4a.MustFindProc("WTNMC4A_SetP")
		setLP = wtnmc4a.MustFindProc("WTNMC4A_SetLP")
		setEP = wtnmc4a.MustFindProc("WTNMC4A_SetEP")
		initLVDV = wtnmc4a.MustFindProc("WTNMC4A_InitLVDV")
		startLVDV = wtnmc4a.MustFindProc("WTNMC4A_StartLVDV")
		decStop = wtnmc4a.MustFindProc("WTNMC4A_DecStop")
		instStop = wtnmc4a.MustFindProc("WTNMC4A_InstStop")
		readLP = wtnmc4a.MustFindProc("WTNMC4A_ReadLP")
		readEP = wtnmc4a.MustFindProc("WTNMC4A_ReadEP")
		getRR1 = wtnmc4a.MustFindProc("WTNMC4A_GetRR1")
		startHome = wtnmc4a.MustFindProc("WTNMC4A_StartAutoHomeSearch")
		clearLimit = wtnmc4a.MustFindProc("WTNMC4A_ClearSoftwareLimit")
		setPDirLim = wtnmc4a.MustFindProc("WTNMC4A_SetPDirSoftwareLimit")
		setMDirLim = wtnmc4a.MustFindProc("WTNMC4A_SetMDirSoftwareLimit")

		slog.Info("WTNMC4A DLL loaded", "path", dllPath)
	})
	return initErr
}

// IsInitialized returns true if Init has been called successfully.
func IsInitialized() bool { return wtnmc4a != nil }

// callSetProc invokes a DLL procedure that takes (handle, axis, value) where
// value is a signed 32-bit integer ("long" in C). On amd64, syscall.Call
// arguments are passed in 64-bit registers; the DLL reads the low 32 bits.
// uintptr(int64) sign-extends correctly within int32 range. WTNMC4A values
// (speed 1-8000, pulse count ≤ 268435455) all fit comfortably in int32.

func callSetProc(proc *syscall.Proc, h uintptr, axis int, value int64) error {
	ret, _, _ := proc.Call(h, uintptr(axis), uintptr(value))
	if ret == 0 {
		return fmt.Errorf("WTNMC4A call failed")
	}
	return nil
}

// -- High-level wrappers (public API for adapter code) --

func WTNMC4ADEVCreate(ip string, sendTimeout, recvTimeout int) (uintptr, error) {
	ipPtr, err := syscall.BytePtrFromString(ip)
	if err != nil {
		return 0, fmt.Errorf("WTNMC4A invalid IP %q: %w", ip, err)
	}
	ret, _, _ := devCreateA.Call(uintptr(unsafe.Pointer(ipPtr)), uintptr(sendTimeout), uintptr(recvTimeout))
	if ret == 0 {
		return 0, fmt.Errorf("WTNMC4A_DEV_CreateA failed for %s", ip)
	}
	return ret, nil
}

func WTNMC4ADEVRelease(h uintptr) {
	devRelease.Call(h)
}

func WTNMC4AReset(h uintptr) error {
	ret, _, _ := reset.Call(h)
	if ret == 0 {
		return fmt.Errorf("WTNMC4A_Reset failed")
	}
	return nil
}

func WTNMC4ASetSV(h uintptr, axis int, value int64) error { return callSetProc(setSV, h, axis, value) }
func WTNMC4ASetV(h uintptr, axis int, value int64) error  { return callSetProc(setV, h, axis, value) }
func WTNMC4ASetA(h uintptr, axis int, value int64) error  { return callSetProc(setA, h, axis, value) }
func WTNMC4ASetDec(h uintptr, axis int, value int64) error {
	return callSetProc(setDec, h, axis, value)
}
func WTNMC4ASetP(h uintptr, axis int, value int64) error  { return callSetProc(setP, h, axis, value) }
func WTNMC4ASetLP(h uintptr, axis int, value int64) error { return callSetProc(setLP, h, axis, value) }
func WTNMC4ASetEP(h uintptr, axis int, value int64) error { return callSetProc(setEP, h, axis, value) }

func WTNMC4AReadLP(h uintptr, axis int) (int64, error) {
	ret, _, _ := readLP.Call(h, uintptr(axis))
	return int64(ret), nil
}
func WTNMC4AReadEP(h uintptr, axis int) (int64, error) {
	ret, _, _ := readEP.Call(h, uintptr(axis))
	return int64(ret), nil
}

func WTNMC4AStartLVDV(h uintptr, axis int) error {
	ret, _, _ := startLVDV.Call(h, uintptr(axis))
	if ret == 0 {
		return fmt.Errorf("WTNMC4A_StartLVDV failed")
	}
	return nil
}

func WTNMC4ADecStop(h uintptr, axis int) error {
	ret, _, _ := decStop.Call(h, uintptr(axis))
	if ret == 0 {
		return fmt.Errorf("WTNMC4A_DecStop failed")
	}
	return nil
}

func WTNMC4AInstStop(h uintptr, axis int) error {
	ret, _, _ := instStop.Call(h, uintptr(axis))
	if ret == 0 {
		return fmt.Errorf("WTNMC4A_InstStop failed")
	}
	return nil
}

func WTNMC4AStartAutoHomeSearch(h uintptr, axis int) error {
	ret, _, _ := startHome.Call(h, uintptr(axis))
	if ret == 0 {
		return fmt.Errorf("WTNMC4A_StartAutoHomeSearch failed")
	}
	return nil
}

func WTNMC4AInitLVDV(h uintptr, dataList unsafe.Pointer, lcData unsafe.Pointer) error {
	ret, _, _ := initLVDV.Call(h, uintptr(dataList), uintptr(lcData))
	if ret == 0 {
		return fmt.Errorf("WTNMC4A_InitLVDV failed")
	}
	return nil
}

func WTNMC4AClearSoftwareLimit(h uintptr, axis int) error {
	ret, _, _ := clearLimit.Call(h, uintptr(axis))
	if ret == 0 {
		return fmt.Errorf("WTNMC4A_ClearSoftwareLimit failed")
	}
	return nil
}

func WTNMC4ASetPDirSoftwareLimit(h uintptr, axis int, value int64) error {
	return callSetProc(setPDirLim, h, axis, value)
}
func WTNMC4ASetMDirSoftwareLimit(h uintptr, axis int, value int64) error {
	return callSetProc(setMDirLim, h, axis, value)
}

type RR1Status struct {
	CMPP, CMPM, ASND, CNST, DSND bool
	IN0, IN1, IN2, IN3           bool
	LMTP, LMTM, ALARM, EMG       bool
}

// wtnmc4aRR1Raw matches WTNMC4A_PARA_RR1: 16 UINT fields, 64 bytes total.
type wtnmc4aRR1Raw struct {
	CMPP, CMPM, ASND, CNST, DSND uint32
	AASND, ACNST, ADSND          uint32
	IN0, IN1, IN2, IN3           uint32
	LMTP, LMTM, ALARM, EMG       uint32
}

func WTNMC4AGetRR1Status(h uintptr, axis int) (RR1Status, error) {
	var raw wtnmc4aRR1Raw
	ret, _, _ := getRR1.Call(h, uintptr(axis), uintptr(unsafe.Pointer(&raw)))
	if ret == 0 {
		return RR1Status{}, fmt.Errorf("WTNMC4A_GetRR1Status failed")
	}
	return RR1Status{
		CMPP: raw.CMPP != 0, CMPM: raw.CMPM != 0,
		ASND: raw.ASND != 0, CNST: raw.CNST != 0,
		DSND: raw.DSND != 0, IN0: raw.IN0 != 0,
		IN1: raw.IN1 != 0, IN2: raw.IN2 != 0,
		IN3: raw.IN3 != 0, LMTP: raw.LMTP != 0,
		LMTM: raw.LMTM != 0, ALARM: raw.ALARM != 0,
		EMG: raw.EMG != 0,
	}, nil
}
