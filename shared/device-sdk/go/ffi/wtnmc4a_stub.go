//go:build !windows

package ffi

import (
	"fmt"
	"unsafe"
)

type RR1Status struct {
	CMPP, CMPM, ASND, CNST, DSND bool
	IN0, IN1, IN2, IN3           bool
	LMTP, LMTM, ALARM, EMG       bool
}

var errNotWin = fmt.Errorf("WTNMC4A FFI only supported on Windows")

func Init(dllPath string) error                             { return errNotWin }
func IsInitialized() bool                                   { return false }

func WTNMC4ADEVCreate(ip string, sendTimeout, recvTimeout int) (uintptr, error) {
	return 0, errNotWin
}
func WTNMC4ADEVRelease(h uintptr)                              {}
func WTNMC4AReset(h uintptr) error                             { return errNotWin }
func WTNMC4ASetSV(h uintptr, axis int, value int64) error      { return errNotWin }
func WTNMC4ASetV(h uintptr, axis int, value int64) error       { return errNotWin }
func WTNMC4ASetA(h uintptr, axis int, value int64) error       { return errNotWin }
func WTNMC4ASetDec(h uintptr, axis int, value int64) error     { return errNotWin }
func WTNMC4ASetP(h uintptr, axis int, value int64) error       { return errNotWin }
func WTNMC4ASetLP(h uintptr, axis int, value int64) error      { return errNotWin }
func WTNMC4ASetEP(h uintptr, axis int, value int64) error      { return errNotWin }
func WTNMC4AReadLP(h uintptr, axis int) (int64, error)         { return 0, errNotWin }
func WTNMC4AReadEP(h uintptr, axis int) (int64, error)         { return 0, errNotWin }
func WTNMC4AStartLVDV(h uintptr, axis int) error               { return errNotWin }
func WTNMC4ADecStop(h uintptr, axis int) error                 { return errNotWin }
func WTNMC4AInstStop(h uintptr, axis int) error                { return errNotWin }
func WTNMC4AStartAutoHomeSearch(h uintptr, axis int) error     { return errNotWin }
func WTNMC4AInitLVDV(h uintptr, dataList, lcData unsafe.Pointer) error {
	return errNotWin
}
func WTNMC4AClearSoftwareLimit(h uintptr, axis int) error         { return errNotWin }
func WTNMC4ASetPDirSoftwareLimit(h uintptr, axis int, value int64) error {
	return errNotWin
}
func WTNMC4ASetMDirSoftwareLimit(h uintptr, axis int, value int64) error {
	return errNotWin
}
func WTNMC4AGetRR1Status(h uintptr, axis int) RR1Status { return RR1Status{} }
