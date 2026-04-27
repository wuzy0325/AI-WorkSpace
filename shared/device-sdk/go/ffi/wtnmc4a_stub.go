//go:build !windows

package ffi

import "fmt"

type DeviceHandle uintptr

type RR1Status struct {
	CMPP, CMPM, ASND, CNST, DSND bool
	IN0, IN1, IN2, IN3           bool
	LMTP, LMTM, ALARM, EMG       bool
}

func Init(dllPath string) error                          { return fmt.Errorf("WTNMC4A FFI only supported on Windows") }
func CreateDevice(ip string, port, axis int) (DeviceHandle, error) { return 0, fmt.Errorf("WTNMC4A FFI only supported on Windows") }
func ReleaseDevice(h DeviceHandle) bool                  { return false }
func Reset(h DeviceHandle) bool                          { return false }
func SetSV(h DeviceHandle, axis, value int) bool         { return false }
func SetV(h DeviceHandle, axis, value int) bool          { return false }
func SetA(h DeviceHandle, axis, value int) bool          { return false }
func SetDec(h DeviceHandle, axis, value int) bool        { return false }
func SetP(h DeviceHandle, axis, value int) bool          { return false }
func SetLP(h DeviceHandle, axis int, value int64) bool   { return false }
func SetEP(h DeviceHandle, axis int, value int64) bool   { return false }
func StartLVDV(h DeviceHandle, axis int) bool            { return false }
func ReadLP(h DeviceHandle, axis int) int                { return 0 }
func ReadEP(h DeviceHandle, axis int) int                { return 0 }
func DecStop(h DeviceHandle, axis int) bool              { return false }
func InstStop(h DeviceHandle, axis int) bool             { return false }
func StartAutoHomeSearch(h DeviceHandle, axis int) bool  { return false }
func GetRR1Status(h DeviceHandle, axis int) RR1Status    { return RR1Status{} }

func WTNMC4ADEVCreate(ip string, sendTimeout, recvTimeout int) (uintptr, error) { return 0, fmt.Errorf("WTNMC4A FFI only supported on Windows") }
func WTNMC4ADEVRelease(h uintptr) bool                    { return false }
func WTNMC4ASetP(h uintptr, axis int, value int64) error  { return errNotWin }
func WTNMC4ASetV(h uintptr, axis int, value int64) error  { return errNotWin }
func WTNMC4ASetLP(h uintptr, axis int, value int64) error { return errNotWin }
func WTNMC4ASetEP(h uintptr, axis int, value int64) error { return errNotWin }
func WTNMC4AReadLP(h uintptr, axis int) (int64, error)    { return 0, errNotWin }
func WTNMC4AReadEP(h uintptr, axis int) (int64, error)    { return 0, errNotWin }
func WTNMC4AStartLVDV(h uintptr, axis int) error          { return errNotWin }
func WTNMC4ADecStop(h uintptr, axis int) error            { return errNotWin }
func WTNMC4AInstStop(h uintptr, axis int) error           { return errNotWin }
func WTNMC4AStartAutoHomeSearch(h uintptr, axis int) error { return errNotWin }
func WTNMC4AGetRR1Status(h uintptr, axis int) RR1Status   { return RR1Status{} }

var errNotWin = fmt.Errorf("WTNMC4A FFI only supported on Windows")
