//go:build !windows

package ffi

import "fmt"

// ============================================================
// WTNDAQ16H FFI 非 Windows 平台 stub
// 不实际加载 DLL，所有调用返回 ErrWTNDAQ16HPlatformNotSupported。
// 结构体与常量定义与 Windows 版本完全一致，便于跨平台编译。
// ============================================================

// ErrWTNDAQ16HPlatformNotSupported 非 Windows 平台调用 FFI 时返回。
var ErrWTNDAQ16HPlatformNotSupported = fmt.Errorf("WTNDAQ16H FFI only supported on Windows")

// ---- 常量（与 Windows 版本一致）----

const (
	WTNDAQ16H_AI_MAX_CHANNELS = 16

	WTNDAQ16H_AI_SAMPRANGE_N10_P10V = 0
	WTNDAQ16H_AI_SAMPRANGE_N5_P5V   = 1
	WTNDAQ16H_AI_SAMPRANGE_N1_P1V   = 2
	WTNDAQ16H_AI_SAMPRANGE_0_20mA   = 3

	WTNDAQ16H_AI_REFGND_DIFF = 0

	WTNDAQ16H_AI_CHAN_SCANMODE_CONTINUOUS = 0
	WTNDAQ16H_AI_CHAN_SCANMODE_GROUP      = 1

	WTNDAQ16H_AI_SAMPSIGNAL_AI   = 0
	WTNDAQ16H_AI_SAMPSIGNAL_0V   = 1
	WTNDAQ16H_AI_SAMPSIGNAL_D98V = 2
	WTNDAQ16H_AI_SAMPSIGNAL_4D9V = 3

	WTNDAQ16H_AI_SAMPMODE_ONE_DEMAND = 0
	WTNDAQ16H_AI_SAMPMODE_FINITE     = 2
	WTNDAQ16H_AI_SAMPMODE_CONTINUOUS = 3

	WTNDAQ16H_AI_CLKSRC_LOCAL = 0
	WTNDAQ16H_AI_CLKSRC_CLKIN = 1

	WTNDAQ16H_AI_TRIGDIR_FALLING = 0
	WTNDAQ16H_AI_TRIGDIR_RISING  = 1
	WTNDAQ16H_AI_TRIGDIR_CHANGE  = 2
)

// ---- 结构体（与 Windows 版本一致）----
// 字段顺序与类型必须与 wtn_daq16h.go 严格保持一致，
// 否则跨平台编译时调用代码会出现 ABI 不匹配。

type WTNDAQ16HAIChParam struct {
	Channel     uint32
	SampleRange uint32
	RefGround   uint32
	Reserved0   uint32
	Reserved1   uint32
	Reserved2   uint32
}

type WTNDAQ16HAIParam struct {
	SampChanCount uint32
	Reserved0     uint32
	CHParam       [16]WTNDAQ16HAIChParam
	ChanScanMode  uint32
	GroupLoops    uint32
	GroupInterval uint32
	SampleSignal  uint32
	SampleMode    uint32
	SampsPerChan  uint32
	SampleRate    float64
	ClockSource   uint32
	ClockOutput   uint32
	Reserved1     uint32
	Reserved2     uint32
	DTriggerEn    uint32
	DTriggerDir   uint32
	ATriggerEn    uint32
	ATriggerDir   uint32
	TriggerLevel  float32
	TriggerSens   uint32
	DelaySamps    uint32
	Reserved3     uint32
	Reserved4     uint32
	Reserved5     uint32
	Reserved6     uint32
	Reserved7     uint32
}

type WTNDAQ16HAIStatus struct {
	TaskDone             uint32
	Triggered            uint32
	TaskState            uint32
	AvailSampsPerChan    uint32
	MaxAvailSampsPerChan uint32
	BufSampsPerChan      uint32
	SampsPerChanAcquired int64
	HardOverflowCnt      uint32
	SoftOverflowCnt      uint32
	InitTaskCnt          uint32
	ReleaseTaskCnt       uint32
	StartTaskCnt         uint32
	StopTaskCnt          uint32
	TransRate            uint32
	Reserved0            uint32
	Reserved1            uint32
	Reserved2            uint32
}

type WTNDAQ16HAIVoltRangeInfo struct {
	SampleRange uint32
	Reserved0   uint32
	MaxVolt     float64
	MinVolt     float64
	Amplitude   float64
	HalfOfAmp   float64
	CodeWidth   float64
	OffsetVolt  float64
	OffsetCode  float64
	NeadCode    float64
	Desc        [16]byte
	Polarity    uint32
	CodeCount   uint32
	MaxCode     int32
	MinCode     int32
	Reserved1   uint32
	Reserved2   uint32
	Reserved3   uint32
	Reserved4   uint32
}

type WTNDAQ16HAIVoltGainInfo struct {
	SampleGain uint32
	Reserved0  uint32
	AmpFactor  float64
	Desc       [16]byte
	Reserved1  uint32
	Reserved2  uint32
	Reserved3  uint32
	Reserved4  uint32
}

// ---- stub 函数 ----

func InitWTNDAQ16H(dllPath string) error { return ErrWTNDAQ16HPlatformNotSupported }
func IsWTNDAQ16HInitialized() bool       { return false }

func WTNDAQ16HDevCreate(ip string, sendTimeout, recvTimeout int) (uintptr, error) {
	return 0, ErrWTNDAQ16HPlatformNotSupported
}
func WTNDAQ16HDevRelease(h uintptr) error { return ErrWTNDAQ16HPlatformNotSupported }
func WTNDAQ16HVerifyParam(h uintptr, p *WTNDAQ16HAIParam) error {
	return ErrWTNDAQ16HPlatformNotSupported
}
func WTNDAQ16HInitTask(h uintptr, p *WTNDAQ16HAIParam, sampEvent uintptr) error {
	return ErrWTNDAQ16HPlatformNotSupported
}
func WTNDAQ16HStartTask(h uintptr) error    { return ErrWTNDAQ16HPlatformNotSupported }
func WTNDAQ16HSendSoftTrig(h uintptr) error { return ErrWTNDAQ16HPlatformNotSupported }
func WTNDAQ16HGetStatus(h uintptr, p *WTNDAQ16HAIStatus) error {
	return ErrWTNDAQ16HPlatformNotSupported
}
func WTNDAQ16HReadBinary(h uintptr, binArray []uint16, readSampsPerChan uint32, timeout float64) (uint32, uint32, error) {
	return 0, 0, ErrWTNDAQ16HPlatformNotSupported
}
func WTNDAQ16HReadAnalog(h uintptr, anlgArray []float64, readSampsPerChan uint32, timeout float64) (uint32, uint32, error) {
	return 0, 0, ErrWTNDAQ16HPlatformNotSupported
}
func WTNDAQ16HStopTask(h uintptr) error    { return ErrWTNDAQ16HPlatformNotSupported }
func WTNDAQ16HReleaseTask(h uintptr) error { return ErrWTNDAQ16HPlatformNotSupported }
func WTNDAQ16HScaleBinToVolt(rangeInfo *WTNDAQ16HAIVoltRangeInfo, gainInfo *WTNDAQ16HAIVoltGainInfo, voltArray []float64, binArray []uint16, scaleSamps uint32) (uint32, error) {
	return 0, ErrWTNDAQ16HPlatformNotSupported
}
