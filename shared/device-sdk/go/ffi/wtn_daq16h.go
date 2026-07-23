//go:build windows

package ffi

import (
	"fmt"
	"shared.local/device-sdk/go/pkg/slog"
	"sync"
	"syscall"
	"unsafe"
)

// ============================================================
// WTNDAQ16H_64.dll FFI 封装
// 协议参考：C:\ART\WTNDAQ16H\Samples\VC\WTNDAQ16H.H
// 模式参考：同包 wtnmc4a.go（sync.Once 幂等加载 + syscall.Call）
// 调用约定：64 位 Windows 只有一种调用约定（Microsoft x64），
//           syscall.Syscall 默认即兼容头文件的 FAR PASCAL / WINAPI
// ============================================================

// DLL 句柄与函数指针。sync.Once 保证幂等加载，避免重复 LoadDLL。
var (
	daq16hOnce           sync.Once
	daq16hInitErr        error
	wtnDaq16h            *syscall.DLL
	daq16hDevCreateA     *syscall.Proc
	daq16hDevRelease     *syscall.Proc
	daq16hInitTask       *syscall.Proc
	daq16hStartTask      *syscall.Proc
	daq16hSendSoftTrig   *syscall.Proc
	daq16hGetStatus      *syscall.Proc
	daq16hReadBinary     *syscall.Proc
	daq16hReadAnalog     *syscall.Proc
	daq16hStopTask       *syscall.Proc
	daq16hReleaseTask    *syscall.Proc
	daq16hVerifyParam    *syscall.Proc
	daq16hScaleBinToVolt *syscall.Proc
)

// ErrWTNDAQ16HPlatformNotSupported 非 Windows 平台调用 FFI 时返回。
// Windows 版本中定义但不使用，保持与 stub 文件符号对称，便于跨平台代码引用。
var ErrWTNDAQ16HPlatformNotSupported = fmt.Errorf("WTNDAQ16H FFI only supported on Windows")

// ---- 常量（与 C 头文件 WTNDAQ16H.H 一致）----
// 保留 WTNDAQ16H_ 前缀以便与头文件宏定义一一对照，降低跨语言查证成本。

const (
	// 设备能力
	WTNDAQ16H_AI_MAX_CHANNELS = 16

	// nSampleRange（量程）
	WTNDAQ16H_AI_SAMPRANGE_N10_P10V = 0 // ±10V
	WTNDAQ16H_AI_SAMPRANGE_N5_P5V   = 1 // ±5V
	WTNDAQ16H_AI_SAMPRANGE_N1_P1V   = 2 // ±1V
	WTNDAQ16H_AI_SAMPRANGE_0_20mA   = 3 // 0-20mA

	// nRefGround（地参考方式）
	WTNDAQ16H_AI_REFGND_DIFF = 0 // 差分

	// nChanScanMode（通道扫描模式）
	WTNDAQ16H_AI_CHAN_SCANMODE_CONTINUOUS = 0 // 连续扫描
	WTNDAQ16H_AI_CHAN_SCANMODE_GROUP      = 1 // 分组扫描

	// nSampleSignal（采样信号源）
	WTNDAQ16H_AI_SAMPSIGNAL_AI   = 0 // AI 通道信号
	WTNDAQ16H_AI_SAMPSIGNAL_0V   = 1 // 0V（AGND）
	WTNDAQ16H_AI_SAMPSIGNAL_D98V = 2 // 0.98V（DC）
	WTNDAQ16H_AI_SAMPSIGNAL_4D9V = 3 // 4.9V（DC）

	// nSampleMode（采样模式）
	WTNDAQ16H_AI_SAMPMODE_ONE_DEMAND = 0 // 单次按需
	WTNDAQ16H_AI_SAMPMODE_FINITE     = 2 // 有限点数
	WTNDAQ16H_AI_SAMPMODE_CONTINUOUS = 3 // 连续采集

	// nClockSource（时钟源）
	WTNDAQ16H_AI_CLKSRC_LOCAL = 0 // 本地时钟（OSCCLK）
	WTNDAQ16H_AI_CLKSRC_CLKIN = 1 // 外部时钟（CLKIN）

	// nDTriggerDir / nATriggerDir（触发方向）
	WTNDAQ16H_AI_TRIGDIR_FALLING = 0 // 下降沿/低电平
	WTNDAQ16H_AI_TRIGDIR_RISING  = 1 // 上升沿/高电平
	WTNDAQ16H_AI_TRIGDIR_CHANGE  = 2 // 变化（双沿）
)

// ---- 结构体（与 C 头文件严格对齐）----
// 字段顺序与头文件保持一致，Go 编译器按相同规则插入 padding，
// 保证与 C 端 ABI 兼容。F64 字段会触发 8 字节对齐，F32 字段 4 字节对齐。
// 所有 BOOL 字段用 uint32 表示（C 端 BOOL = int = 4 字节）。

// WTNDAQ16HAIChParam 对应 C 端 WTNDAQ16H_AI_CH_PARAM
// C 端大小：6 * 4 = 24 字节，无 padding。
type WTNDAQ16HAIChParam struct {
	Channel     uint32 // nChannel，通道号 [0, 15]
	SampleRange uint32 // nSampleRange，量程
	RefGround   uint32 // nRefGround，地参考方式
	Reserved0   uint32 // 保留
	Reserved1   uint32 // 保留
	Reserved2   uint32 // 保留
}

// WTNDAQ16HAIParam 对应 C 端 WTNDAQ16H_AI_PARAM
// 字段顺序严格对齐头文件。SampleRate（F64）前由 Go 编译器自动插入 4 字节 padding
// 以满足 8 字节对齐，与 C 端 ABI 一致。
type WTNDAQ16HAIParam struct {
	// 通道配置
	SampChanCount uint32                 // nSampChanCount，采样通道数 [1, 16]
	Reserved0     uint32                 // nReserved0
	CHParam       [16]WTNDAQ16HAIChParam // CHParam[16]，16 通道参数
	ChanScanMode  uint32                 // nChanScanMode
	GroupLoops    uint32                 // nGroupLoops
	GroupInterval uint32                 // nGroupInterval，组间隔微秒
	SampleSignal  uint32                 // nSampleSignal

	// 时钟参数
	SampleMode   uint32  // nSampleMode
	SampsPerChan uint32  // nSampsPerChan
	SampleRate   float64 // fSampleRate，采样率 sps
	ClockSource  uint32  // nClockSource
	ClockOutput  uint32  // bClockOutput
	Reserved1    uint32  // nReserved1
	Reserved2    uint32  // nReserved2

	// 触发参数
	DTriggerEn   uint32  // bDTriggerEn，数字触发使能
	DTriggerDir  uint32  // nDTriggerDir
	ATriggerEn   uint32  // bATriggerEn，模拟触发使能
	ATriggerDir  uint32  // nATriggerDir
	TriggerLevel float32 // fTriggerLevel
	TriggerSens  uint32  // nTriggerSens，触发灵敏度微秒
	DelaySamps   uint32  // nDelaySamps
	Reserved3    uint32  // nReserved3

	// 保留字段
	Reserved4 uint32
	Reserved5 uint32
	Reserved6 uint32
	Reserved7 uint32
}

// WTNDAQ16HAIStatus 对应 C 端 WTNDAQ16H_AI_STATUS
// SampsPerChanAcquired（I64）需 8 字节对齐，前面 7 个 uint32 = 28 字节后会有 4 字节 padding。
type WTNDAQ16HAIStatus struct {
	TaskDone             uint32 // bTaskDone
	Triggered            uint32 // bTriggered
	TaskState            uint32 // nTaskState
	AvailSampsPerChan    uint32 // nAvailSampsPerChan
	MaxAvailSampsPerChan uint32 // nMaxAvailSampsPerChan
	BufSampsPerChan      uint32 // nBufSampsPerChan
	SampsPerChanAcquired int64  // nSampsPerChanAcquired，I64，8 字节对齐
	HardOverflowCnt      uint32 // nHardOverflowCnt
	SoftOverflowCnt      uint32 // nSoftOverflowCnt
	InitTaskCnt          uint32 // nInitTaskCnt
	ReleaseTaskCnt       uint32 // nReleaseTaskCnt
	StartTaskCnt         uint32 // nStartTaskCnt
	StopTaskCnt          uint32 // nStopTaskCnt
	TransRate            uint32 // nTransRate
	Reserved0            uint32
	Reserved1            uint32
	Reserved2            uint32
}

// WTNDAQ16HAIVoltRangeInfo 对应 C 端 WTNDAQ16H_AI_VOLT_RANGE_INFO
// ScaleBinToVolt 需要此结构体作为输入参数。
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
	Desc        [16]byte // char[16]
	Polarity    uint32
	CodeCount   uint32
	MaxCode     int32
	MinCode     int32
	Reserved1   uint32
	Reserved2   uint32
	Reserved3   uint32
	Reserved4   uint32
}

// WTNDAQ16HAIVoltGainInfo 对应 C 端 WTNDAQ16H_AI_VOLT_GAIN_INFO
// ScaleBinToVolt 的可选参数，传 nil 表示不使用增益。
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

// ---- DLL 加载 ----

// InitWTNDAQ16H 加载 WTNDAQ16H_64.dll 并解析全部 12 个 API 函数指针。
// 使用 sync.Once 保证幂等：多次调用只有第一次真正加载，后续直接返回首次结果。
// 调用方应在程序启动时调用一次，路径指向可执行文件同目录的 WTNDAQ16H_64.dll。
// 一旦失败（DLL 不存在或函数解析失败），后续调用返回同一错误，需重启程序才能重试。
func InitWTNDAQ16H(dllPath string) error {
	daq16hOnce.Do(func() {
		var err error
		wtnDaq16h, err = syscall.LoadDLL(dllPath)
		if err != nil {
			daq16hInitErr = fmt.Errorf("failed to load WTNDAQ16H DLL %s: %w", dllPath, err)
			return
		}

		daq16hDevCreateA = wtnDaq16h.MustFindProc("WTNDAQ16H_DEV_CreateA")
		daq16hDevRelease = wtnDaq16h.MustFindProc("WTNDAQ16H_DEV_Release")
		daq16hInitTask = wtnDaq16h.MustFindProc("WTNDAQ16H_AI_InitTask")
		daq16hStartTask = wtnDaq16h.MustFindProc("WTNDAQ16H_AI_StartTask")
		daq16hSendSoftTrig = wtnDaq16h.MustFindProc("WTNDAQ16H_AI_SendSoftTrig")
		daq16hGetStatus = wtnDaq16h.MustFindProc("WTNDAQ16H_AI_GetStatus")
		daq16hReadBinary = wtnDaq16h.MustFindProc("WTNDAQ16H_AI_ReadBinary")
		daq16hReadAnalog = wtnDaq16h.MustFindProc("WTNDAQ16H_AI_ReadAnalog")
		daq16hStopTask = wtnDaq16h.MustFindProc("WTNDAQ16H_AI_StopTask")
		daq16hReleaseTask = wtnDaq16h.MustFindProc("WTNDAQ16H_AI_ReleaseTask")
		daq16hVerifyParam = wtnDaq16h.MustFindProc("WTNDAQ16H_AI_VerifyParam")
		daq16hScaleBinToVolt = wtnDaq16h.MustFindProc("WTNDAQ16H_AI_ScaleBinToVolt")

		slog.Info("WTNDAQ16H DLL loaded", "path", dllPath)
	})
	return daq16hInitErr
}

// IsWTNDAQ16HInitialized 返回 DLL 是否已成功加载。
// 适配器层在调用 API 前应检查此状态，避免空指针 panic。
func IsWTNDAQ16HInitialized() bool { return wtnDaq16h != nil }

// ---- 高层封装（适配器层调用）----

// WTNDAQ16HDevCreate 建立 TCP 连接。
//   - ip：设备 IP 地址，如 "192.168.1.4"
//   - sendTimeout / recvTimeout：TCP 收发超时（毫秒），0 表示用 DLL 默认值（200ms）
//
// 返回 device handle，后续所有 API 都需要传入。失败返回 0 与错误。
func WTNDAQ16HDevCreate(ip string, sendTimeout, recvTimeout int) (uintptr, error) {
	ipPtr, err := syscall.BytePtrFromString(ip)
	if err != nil {
		return 0, fmt.Errorf("WTNDAQ16H invalid IP %q: %w", ip, err)
	}
	ret, _, _ := daq16hDevCreateA.Call(
		uintptr(unsafe.Pointer(ipPtr)),
		uintptr(sendTimeout),
		uintptr(recvTimeout),
	)
	// DEV_CreateA 失败返回 0（INVALID_HANDLE_VALUE 在实践中表现为 0）
	if ret == 0 {
		return 0, fmt.Errorf("WTNDAQ16H_DEV_CreateA failed for %s", ip)
	}
	return ret, nil
}

// WTNDAQ16HDevRelease 释放设备连接。重复调用安全（DLL 内部容错）。
func WTNDAQ16HDevRelease(h uintptr) error {
	ret, _, _ := daq16hDevRelease.Call(h)
	if ret == 0 {
		return fmt.Errorf("WTNDAQ16H_DEV_Release failed")
	}
	return nil
}

// WTNDAQ16HVerifyParam 校验 AI 参数合法性。
// 在 InitTask 之前调用，避免非法参数导致 DLL 内部状态异常。
// 失败时 DLL 会写日志到 WTNDAQ16H.log，错误信息需查日志。
func WTNDAQ16HVerifyParam(h uintptr, p *WTNDAQ16HAIParam) error {
	ret, _, _ := daq16hVerifyParam.Call(h, uintptr(unsafe.Pointer(p)))
	if ret == 0 {
		return fmt.Errorf("WTNDAQ16H_AI_VerifyParam rejected the parameter (check WTNDAQ16H.log)")
	}
	return nil
}

// WTNDAQ16HInitTask 初始化采集任务。
//   - sampEvent：可选的采样事件句柄，传 0 表示不需要事件通知（轮询模式）
//
// 调用前应先调用 VerifyParam 校验参数。
func WTNDAQ16HInitTask(h uintptr, p *WTNDAQ16HAIParam, sampEvent uintptr) error {
	ret, _, _ := daq16hInitTask.Call(h, uintptr(unsafe.Pointer(p)), sampEvent)
	if ret == 0 {
		return fmt.Errorf("WTNDAQ16H_AI_InitTask failed")
	}
	return nil
}

// WTNDAQ16HStartTask 启动采集。需在 InitTask 之后调用。
func WTNDAQ16HStartTask(h uintptr) error {
	ret, _, _ := daq16hStartTask.Call(h)
	if ret == 0 {
		return fmt.Errorf("WTNDAQ16H_AI_StartTask failed")
	}
	return nil
}

// WTNDAQ16HSendSoftTrig 发送软件触发。
// 连续采集模式下一般也需要触发一次以启动数据流。
func WTNDAQ16HSendSoftTrig(h uintptr) error {
	ret, _, _ := daq16hSendSoftTrig.Call(h)
	if ret == 0 {
		return fmt.Errorf("WTNDAQ16H_AI_SendSoftTrig failed")
	}
	return nil
}

// WTNDAQ16HGetStatus 读取采集任务状态。p 由调用方分配。
func WTNDAQ16HGetStatus(h uintptr, p *WTNDAQ16HAIStatus) error {
	ret, _, _ := daq16hGetStatus.Call(h, uintptr(unsafe.Pointer(p)))
	if ret == 0 {
		return fmt.Errorf("WTNDAQ16H_AI_GetStatus failed")
	}
	return nil
}

// WTNDAQ16HReadBinary 读取原始 ADC 码值（U16）。
//   - binArray：调用方分配的缓冲区，长度 >= readSampsPerChan * 通道数
//   - readSampsPerChan：每通道期望读取的采样数
//   - timeout：超时秒数（建议 10.0）
//
// 返回实际每通道读取数与缓冲区可用数。
// 注意：ret==FALSE 时仍可能返回部分数据（timeout 场景），调用方应判断 sampsRead 决定是否使用。
func WTNDAQ16HReadBinary(h uintptr, binArray []uint16, readSampsPerChan uint32, timeout float64) (sampsRead, availSamps uint32, err error) {
	if len(binArray) == 0 {
		return 0, 0, fmt.Errorf("WTNDAQ16H_AI_ReadBinary: empty buffer")
	}
	var sampsRead_, availSamps_ uint32
	ret, _, _ := daq16hReadBinary.Call(
		h,
		uintptr(unsafe.Pointer(&binArray[0])),
		uintptr(readSampsPerChan),
		uintptr(unsafe.Pointer(&sampsRead_)),
		uintptr(unsafe.Pointer(&availSamps_)),
		uintptr(unsafe.Pointer(&timeout)),
	)
	if ret == 0 {
		return sampsRead_, availSamps_, fmt.Errorf("WTNDAQ16H_AI_ReadBinary failed or timeout (read=%d, avail=%d)", sampsRead_, availSamps_)
	}
	return sampsRead_, availSamps_, nil
}

// WTNDAQ16HReadAnalog 读取已换算的电压数据（F64）。
// 与 ReadBinary 参数语义一致，区别仅在于返回数据已换算为电压（单位 V）。
func WTNDAQ16HReadAnalog(h uintptr, anlgArray []float64, readSampsPerChan uint32, timeout float64) (sampsRead, availSamps uint32, err error) {
	if len(anlgArray) == 0 {
		return 0, 0, fmt.Errorf("WTNDAQ16H_AI_ReadAnalog: empty buffer")
	}
	var sampsRead_, availSamps_ uint32
	ret, _, _ := daq16hReadAnalog.Call(
		h,
		uintptr(unsafe.Pointer(&anlgArray[0])),
		uintptr(readSampsPerChan),
		uintptr(unsafe.Pointer(&sampsRead_)),
		uintptr(unsafe.Pointer(&availSamps_)),
		uintptr(unsafe.Pointer(&timeout)),
	)
	if ret == 0 {
		return sampsRead_, availSamps_, fmt.Errorf("WTNDAQ16H_AI_ReadAnalog failed or timeout")
	}
	return sampsRead_, availSamps_, nil
}

// WTNDAQ16HStopTask 停止采集。重复调用安全。
func WTNDAQ16HStopTask(h uintptr) error {
	ret, _, _ := daq16hStopTask.Call(h)
	if ret == 0 {
		return fmt.Errorf("WTNDAQ16H_AI_StopTask failed")
	}
	return nil
}

// WTNDAQ16HReleaseTask 释放采集任务资源。
// 必须在 StopTask 之后、DEV_Release 之前调用，顺序不能颠倒。
func WTNDAQ16HReleaseTask(h uintptr) error {
	ret, _, _ := daq16hReleaseTask.Call(h)
	if ret == 0 {
		return fmt.Errorf("WTNDAQ16H_AI_ReleaseTask failed")
	}
	return nil
}

// WTNDAQ16HScaleBinToVolt 将原始 ADC 码值换算为电压值。
//   - rangeInfo：从 GetVoltRangeInfo 获取的量程信息（必填）
//   - gainInfo：从 GetVoltGainInfo 获取的增益信息，传 nil 表示不使用增益
//   - voltArray / binArray：调用方分配，长度需一致且 >= scaleSamps
//   - scaleSamps：要换算的采样数
//
// 返回实际换算的采样数。
func WTNDAQ16HScaleBinToVolt(rangeInfo *WTNDAQ16HAIVoltRangeInfo, gainInfo *WTNDAQ16HAIVoltGainInfo, voltArray []float64, binArray []uint16, scaleSamps uint32) (sampsScaled uint32, err error) {
	if len(voltArray) == 0 || len(binArray) == 0 {
		return 0, fmt.Errorf("WTNDAQ16H_AI_ScaleBinToVolt: empty buffer")
	}
	var sampsScaled_ uint32
	var gainPtr uintptr
	if gainInfo != nil {
		gainPtr = uintptr(unsafe.Pointer(gainInfo))
	}
	ret, _, _ := daq16hScaleBinToVolt.Call(
		uintptr(unsafe.Pointer(rangeInfo)),
		gainPtr,
		uintptr(unsafe.Pointer(&voltArray[0])),
		uintptr(unsafe.Pointer(&binArray[0])),
		uintptr(scaleSamps),
		uintptr(unsafe.Pointer(&sampsScaled_)),
	)
	if ret == 0 {
		return sampsScaled_, fmt.Errorf("WTNDAQ16H_AI_ScaleBinToVolt failed")
	}
	return sampsScaled_, nil
}
