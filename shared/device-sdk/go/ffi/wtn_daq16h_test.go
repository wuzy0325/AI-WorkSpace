package ffi

import (
	"runtime"
	"testing"
	"unsafe"
)

// TestInitWTNDAQ16H_DLLNotFound 验证 DLL 路径不存在时返回错误。
// Windows 平台真实加载会失败，非 Windows 平台 stub 总是返回 ErrWTNDAQ16HPlatformNotSupported。
//
// 注意：由于 sync.Once 的特性，此测试运行后 InitWTNDAQ16H 的全局状态会被缓存。
// 后续在同一进程内调用 InitWTNDAQ16H（即使传真实路径）也会返回缓存的错误。
// 这不影响生产（生产进程与测试进程隔离），但开发时如需手动验证真实加载，需重启程序。
func TestInitWTNDAQ16H_DLLNotFound(t *testing.T) {
	err := InitWTNDAQ16H("C:\\nonexistent\\WTNDAQ16H_64.dll")
	if err == nil {
		t.Fatal("expected error when DLL path does not exist, got nil")
	}

	if runtime.GOOS != "windows" && err != ErrWTNDAQ16HPlatformNotSupported {
		t.Errorf("non-windows platform: expected ErrWTNDAQ16HPlatformNotSupported, got %v", err)
	}

	if IsWTNDAQ16HInitialized() {
		t.Error("IsWTNDAQ16HInitialized should return false after failed init")
	}
}

// TestWTNDAQ16HAIChParam_Size 验证通道参数结构体大小。
// C 端为 6 * U32 = 24 字节，无 padding。
// 此测试在所有平台运行，确保 stub 与 Windows 版本结构体定义一致。
func TestWTNDAQ16HAIChParam_Size(t *testing.T) {
	var c WTNDAQ16HAIChParam
	if size := unsafe.Sizeof(c); size != 24 {
		t.Errorf("WTNDAQ16HAIChParam size = %d, expected 24 (6 * uint32)", size)
	}
}

// TestWTNDAQ16HAIParam_Alignment 验证 AI_PARAM 结构体对齐。
// FFI 安全的关键检查：若 Go 端结构体与 C 端 ABI 不一致，
// DLL 内部读写会越界或读错字段，导致崩溃或数据错乱。
// 这里只验证对齐规则，不硬编码精确大小（避免编译器差异导致 flaky）。
func TestWTNDAQ16HAIParam_Alignment(t *testing.T) {
	var p WTNDAQ16HAIParam

	// SampleRate（F64）必须 8 字节对齐
	if off := unsafe.Offsetof(p.SampleRate); off%8 != 0 {
		t.Errorf("SampleRate offset = %d, must be 8-byte aligned (F64 alignment)", off)
	}

	// 结构体大小必须是最大对齐（8）的倍数
	if size := unsafe.Sizeof(p); size%8 != 0 {
		t.Errorf("WTNDAQ16HAIParam size = %d, must be multiple of 8 (F64 alignment)", size)
	}

	// CHParam 数组偏移应紧跟 Reserved0（offset 8）
	if off := unsafe.Offsetof(p.CHParam); off != 8 {
		t.Errorf("CHParam offset = %d, expected 8 (after SampChanCount + Reserved0)", off)
	}
}

// TestWTNDAQ16HAIStatus_Alignment 验证 AI_STATUS 结构体对齐。
// SampsPerChanAcquired（I64）需 8 字节对齐。
func TestWTNDAQ16HAIStatus_Alignment(t *testing.T) {
	var s WTNDAQ16HAIStatus

	if off := unsafe.Offsetof(s.SampsPerChanAcquired); off%8 != 0 {
		t.Errorf("SampsPerChanAcquired offset = %d, must be 8-byte aligned (I64 alignment)", off)
	}

	if size := unsafe.Sizeof(s); size%8 != 0 {
		t.Errorf("WTNDAQ16HAIStatus size = %d, must be multiple of 8 (I64 alignment)", size)
	}
}

// TestWTNDAQ16HAIVoltRangeInfo_Alignment 验证量程信息结构体对齐。
func TestWTNDAQ16HAIVoltRangeInfo_Alignment(t *testing.T) {
	var r WTNDAQ16HAIVoltRangeInfo

	// 第一个 F64 字段 MaxVolt 必须 8 字节对齐
	if off := unsafe.Offsetof(r.MaxVolt); off%8 != 0 {
		t.Errorf("MaxVolt offset = %d, must be 8-byte aligned", off)
	}

	if size := unsafe.Sizeof(r); size%8 != 0 {
		t.Errorf("WTNDAQ16HAIVoltRangeInfo size = %d, must be multiple of 8", size)
	}
}
