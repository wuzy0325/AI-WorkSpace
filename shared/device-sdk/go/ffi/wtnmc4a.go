//go:build windows

package ffi

import (
	"fmt"
	"log/slog"
	"syscall"
	"unsafe"
)

var (
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
	readCV     *syscall.Proc
	readCA     *syscall.Proc
	getRR1     *syscall.Proc
	startHome  *syscall.Proc
	clearLimit *syscall.Proc
	setPDirLim *syscall.Proc
	setMDirLim *syscall.Proc
)

type DeviceHandle uintptr

func Init(dllPath string) error {
	var err error
	wtnmc4a, err = syscall.LoadDLL(dllPath)
	if err != nil {
		return fmt.Errorf("failed to load DLL %s: %w", dllPath, err)
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
	readCV = wtnmc4a.MustFindProc("WTNMC4A_ReadCV")
	readCA = wtnmc4a.MustFindProc("WTNMC4A_ReadCA")
	getRR1 = wtnmc4a.MustFindProc("WTNMC4A_GetRR1Status")
	startHome = wtnmc4a.MustFindProc("WTNMC4A_StartAutoHomeSearch")
	clearLimit = wtnmc4a.MustFindProc("WTNMC4A_ClearSoftwareLimit")
	setPDirLim = wtnmc4a.MustFindProc("WTNMC4A_SetPDirSoftwareLimit")
	setMDirLim = wtnmc4a.MustFindProc("WTNMC4A_SetMDirSoftwareLimit")

	slog.Info("WTNMC4A DLL loaded", "path", dllPath)
	return nil
}

func CreateDevice(ip string, port, axis int) (DeviceHandle, error) {
	ipPtr, _ := syscall.BytePtrFromString(ip)
	ret, _, _ := devCreateA.Call(uintptr(unsafe.Pointer(ipPtr)), uintptr(port), uintptr(axis))
	if ret == 0 {
		return 0, fmt.Errorf("WTNMC4A_DEV_CreateA failed")
	}
	return DeviceHandle(ret), nil
}

func ReleaseDevice(h DeviceHandle) bool { ret, _, _ := devRelease.Call(uintptr(h)); return ret != 0 }
func Reset(h DeviceHandle) bool         { ret, _, _ := reset.Call(uintptr(h)); return ret != 0 }
func SetSV(h DeviceHandle, axis, value int) bool {
	ret, _, _ := setSV.Call(uintptr(h), uintptr(axis), uintptr(value)); return ret != 0
}
func SetV(h DeviceHandle, axis, value int) bool {
	ret, _, _ := setV.Call(uintptr(h), uintptr(axis), uintptr(value)); return ret != 0
}
func SetA(h DeviceHandle, axis, value int) bool {
	ret, _, _ := setA.Call(uintptr(h), uintptr(axis), uintptr(value)); return ret != 0
}
func SetDec(h DeviceHandle, axis, value int) bool {
	ret, _, _ := setDec.Call(uintptr(h), uintptr(axis), uintptr(value)); return ret != 0
}
func SetP(h DeviceHandle, axis, value int) bool {
	ret, _, _ := setP.Call(uintptr(h), uintptr(axis), uintptr(value)); return ret != 0
}
func ReadLP(h DeviceHandle, axis int) int  { ret, _, _ := readLP.Call(uintptr(h), uintptr(axis)); return int(ret) }
func ReadEP(h DeviceHandle, axis int) int  { ret, _, _ := readEP.Call(uintptr(h), uintptr(axis)); return int(ret) }
func DecStop(h DeviceHandle, axis int) bool { ret, _, _ := decStop.Call(uintptr(h), uintptr(axis)); return ret != 0 }
func InstStop(h DeviceHandle, axis int) bool { ret, _, _ := instStop.Call(uintptr(h), uintptr(axis)); return ret != 0 }
func StartAutoHomeSearch(h DeviceHandle, axis int) bool {
	ret, _, _ := startHome.Call(uintptr(h), uintptr(axis)); return ret != 0
}

type RR1Status struct {
	CMPP, CMPM, ASND, CNST, DSND bool
	IN0, IN1, IN2, IN3           bool
	LMTP, LMTM, ALARM, EMG       bool
}

func GetRR1Status(h DeviceHandle, axis int) RR1Status {
	var buf [4]byte
	getRR1.Call(uintptr(h), uintptr(axis), uintptr(unsafe.Pointer(&buf[0])))
	status := uint16(buf[0]) | uint16(buf[1])<<8
	return RR1Status{
		CMPP: (status&(1<<0)) != 0, CMPM: (status&(1<<1)) != 0,
		ASND: (status&(1<<2)) != 0, CNST: (status&(1<<3)) != 0,
		DSND: (status&(1<<4)) != 0, IN0: (status&(1<<5)) != 0,
		IN1: (status&(1<<6)) != 0, IN2: (status&(1<<7)) != 0,
		IN3: (status&(1<<8)) != 0, LMTP: (status&(1<<9)) != 0,
		LMTM: (status&(1<<10)) != 0, ALARM: (status&(1<<11)) != 0,
		EMG: (status&(1<<12)) != 0,
	}
}

func SetLP(h DeviceHandle, axis int, value int64) bool {
	ret, _, _ := setLP.Call(uintptr(h), uintptr(axis), uintptr(value)); return ret != 0
}
func SetEP(h DeviceHandle, axis int, value int64) bool {
	ret, _, _ := setEP.Call(uintptr(h), uintptr(axis), uintptr(value)); return ret != 0
}
func StartLVDV(h DeviceHandle, axis int) bool {
	ret, _, _ := startLVDV.Call(uintptr(h), uintptr(axis)); return ret != 0
}

// -- High-level wrappers --

func WTNMC4ADEVCreate(ip string, sendTimeout, recvTimeout int) (uintptr, error) {
	ipPtr, _ := syscall.BytePtrFromString(ip)
	ret, _, _ := devCreateA.Call(uintptr(unsafe.Pointer(ipPtr)), uintptr(sendTimeout), uintptr(recvTimeout))
	if ret == 0 { return 0, fmt.Errorf("WTNMC4A_DEV_CreateA failed") }
	return ret, nil
}
func WTNMC4ADEVRelease(h uintptr) bool                   { ret, _, _ := devRelease.Call(h); return ret != 0 }
func WTNMC4ASetP(h uintptr, axis int, value int64) error  { ret, _, _ := setP.Call(h, uintptr(axis), uintptr(value)); if ret == 0 { return fmt.Errorf("WTNMC4A_SetP failed") }; return nil }
func WTNMC4ASetV(h uintptr, axis int, value int64) error  { ret, _, _ := setV.Call(h, uintptr(axis), uintptr(value)); if ret == 0 { return fmt.Errorf("WTNMC4A_SetV failed") }; return nil }
func WTNMC4ASetLP(h uintptr, axis int, value int64) error { ret, _, _ := setLP.Call(h, uintptr(axis), uintptr(value)); if ret == 0 { return fmt.Errorf("WTNMC4A_SetLP failed") }; return nil }
func WTNMC4ASetEP(h uintptr, axis int, value int64) error { ret, _, _ := setEP.Call(h, uintptr(axis), uintptr(value)); if ret == 0 { return fmt.Errorf("WTNMC4A_SetEP failed") }; return nil }
func WTNMC4AReadLP(h uintptr, axis int) (int64, error)    { ret, _, _ := readLP.Call(h, uintptr(axis)); return int64(ret), nil }
func WTNMC4AReadEP(h uintptr, axis int) (int64, error)    { ret, _, _ := readEP.Call(h, uintptr(axis)); return int64(ret), nil }
func WTNMC4AStartLVDV(h uintptr, axis int) error          { ret, _, _ := startLVDV.Call(h, uintptr(axis)); if ret == 0 { return fmt.Errorf("WTNMC4A_StartLVDV failed") }; return nil }
func WTNMC4ADecStop(h uintptr, axis int) error            { ret, _, _ := decStop.Call(h, uintptr(axis)); if ret == 0 { return fmt.Errorf("WTNMC4A_DecStop failed") }; return nil }
func WTNMC4AInstStop(h uintptr, axis int) error           { ret, _, _ := instStop.Call(h, uintptr(axis)); if ret == 0 { return fmt.Errorf("WTNMC4A_InstStop failed") }; return nil }
func WTNMC4AStartAutoHomeSearch(h uintptr, axis int) error { ret, _, _ := startHome.Call(h, uintptr(axis)); if ret == 0 { return fmt.Errorf("WTNMC4A_StartAutoHomeSearch failed") }; return nil }
func WTNMC4AGetRR1Status(h uintptr, axis int) RR1Status   { return GetRR1Status(DeviceHandle(h), axis) }
