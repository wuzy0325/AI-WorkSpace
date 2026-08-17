// DAQ-P-1603 采集测试工具（对齐厂家示例代码配置）
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/sys/windows"
	"shared.local/device-sdk/go/ffi"
)

func main() {
	ip := flag.String("ip", "192.168.3.102", "设备 IP 地址")
	rate := flag.Float64("rate", 500, "采样率 (Hz, 默认 500，对齐 WindLabX4 DAQP1603MaxSampleRate)")
	useCurrent := flag.Bool("current", true, "使用 0-20mA 电流模式（默认开启，对齐 WindLabX4）")
	dllPath := flag.String("dll", "", "WTNDAQ16H_64.dll 路径")
	continuous := flag.Duration("continuous", 0, "持续采集时长（如 30s, 1m），0 表示仅读 3 次。模拟 WindLabX4 readLoop 行为")
	nChans := flag.Int("chans", 16, "启用通道数（1-16）。WindLabX4 默认按 profile 配置，本工具默认 16 通道")
	WINDLABX4Mode := flag.Bool("WINDLABX4", false, "完全复刻 WindLabX4 调用路径（含 GetVoltRangeInfo + ScaleBinToVolt）")
	applyConfigMode := flag.Bool("applyconfig", false, "复刻 WindLabX4 ApplyConfig 路径：Connect 后再 ReleaseTask→VerifyParam→InitTask")
	multiThreadMode := flag.Bool("multithread", false, "复刻 WindLabX4 多 goroutine 调用：Connect/InitTask/StartTask 在主 goroutine，ReadBinary 在新 goroutine（验证 OS 线程亲和性假设）")
	flag.Parse()

	if *dllPath == "" {
		if exePath, err := os.Executable(); err == nil {
			*dllPath = filepath.Join(filepath.Dir(exePath), "WTNDAQ16H_64.dll")
		} else {
			*dllPath = "WTNDAQ16H_64.dll"
		}
	}

	sampleRange := uint32(ffi.WTNDAQ16H_AI_SAMPRANGE_N10_P10V)
	rangeLabel := "±10V"
	if *useCurrent {
		sampleRange = ffi.WTNDAQ16H_AI_SAMPRANGE_0_20mA
		rangeLabel = "0-20mA"
	}

	fmt.Printf("=== DAQ-P-1603 采集测试 ===\n")
	fmt.Printf("目标:   %s\n", *ip)
	fmt.Printf("采样率: %.0f Hz\n", *rate)
	fmt.Printf("量程:   %s\n", rangeLabel)
	fmt.Printf("通道数: %d\n", *nChans)
	fmt.Printf("WindLabX4 复刻模式: %v\n", *WINDLABX4Mode)
	fmt.Printf("ApplyConfig 复刻: %v\n", *applyConfigMode)
	fmt.Printf("多线程复刻: %v\n", *multiThreadMode)
	fmt.Printf("DLL:    %s\n\n", *dllPath)

	// Step 1: 加载 DLL
	fmt.Printf("[1/5] 加载 DLL ... ")
	if err := ffi.InitWTNDAQ16H(*dllPath); err != nil {
		fmt.Printf("失败 ❌ %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("成功 ✅ (主 goroutine OS线程ID=%d)\n", windows.GetCurrentThreadId())

	// Step 2: 连接（超时 200ms，对齐 C++ 默认值）
	fmt.Printf("[2/5] 连接 %s ... ", *ip)
	handle, err := ffi.WTNDAQ16HDevCreate(*ip, 200, 200)
	if err != nil {
		fmt.Printf("失败 ❌ %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("成功 ✅ (handle=0x%x)\n", handle)

	// Step 3: InitTask（按通道数构造参数，对齐 WindLabX4 buildAIParamLocked）
	fmt.Printf("[3/5] 初始化采集任务 ... ")
	param := buildAIParam(*rate, sampleRange, *nChans)
	if err := ffi.WTNDAQ16HVerifyParam(handle, &param); err != nil {
		_ = ffi.WTNDAQ16HDevRelease(handle)
		fmt.Printf("失败 ❌ (VerifyParam) %v\n", err)
		os.Exit(1)
	}
	if err := ffi.WTNDAQ16HInitTask(handle, &param, 0); err != nil {
		_ = ffi.WTNDAQ16HReleaseTask(handle)
		_ = ffi.WTNDAQ16HDevRelease(handle)
		fmt.Printf("失败 ❌ (InitTask) %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("成功 ✅\n")

	// ApplyConfig 复刻模式：模拟 WindLabX4 在 Connect 后再次调用 ApplyConfig
	// 触发 ReleaseTask → VerifyParam → InitTask 路径，验证是否污染 DLL 任务状态
	if *applyConfigMode {
		fmt.Printf("[3.5] 复刻 ApplyConfig: ReleaseTask → VerifyParam → InitTask ... ")
		if err := ffi.WTNDAQ16HReleaseTask(handle); err != nil {
			fmt.Printf("失败 ❌ (ReleaseTask) %v\n", err)
			os.Exit(1)
		}
		param2 := buildAIParam(*rate, sampleRange, *nChans)
		if err := ffi.WTNDAQ16HVerifyParam(handle, &param2); err != nil {
			fmt.Printf("失败 ❌ (VerifyParam2) %v\n", err)
			os.Exit(1)
		}
		if err := ffi.WTNDAQ16HInitTask(handle, &param2, 0); err != nil {
			fmt.Printf("失败 ❌ (InitTask2) %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("成功 ✅\n")
	}

	// Step 4: StartTask + SendSoftTrig（完全复刻 WindLabX4 driver startAndCheck）
	fmt.Printf("[4/5] 启动采集 (复刻 WindLabX4 startAndCheck) ... ")
	if err := ffi.WTNDAQ16HStartTask(handle); err != nil {
		_ = ffi.WTNDAQ16HReleaseTask(handle)
		_ = ffi.WTNDAQ16HDevRelease(handle)
		fmt.Printf("失败 ❌ (StartTask) %v\n", err)
		os.Exit(1)
	}
	// 诊断点 A：StartTask 后立即查 TaskState（对齐 WindLabX4 driver）
	var sA ffi.WTNDAQ16HAIStatus
	_ = ffi.WTNDAQ16HGetStatus(handle, &sA)
	fmt.Printf("\n  [诊断A] StartTask后 taskState=%d ", sA.TaskState)

	if err := ffi.WTNDAQ16HSendSoftTrig(handle); err != nil {
		_ = ffi.WTNDAQ16HStopTask(handle)
		_ = ffi.WTNDAQ16HReleaseTask(handle)
		_ = ffi.WTNDAQ16HDevRelease(handle)
		fmt.Printf("失败 ❌ (SendSoftTrig) %v\n", err)
		os.Exit(1)
	}
	// 诊断点 B：SendSoftTrig 后查 TaskState（对齐 WindLabX4 driver）
	var sB ffi.WTNDAQ16HAIStatus
	if err := ffi.WTNDAQ16HGetStatus(handle, &sB); err != nil {
		fmt.Printf("失败 ❌ (GetStatusB) %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("[诊断B] SendSoftTrig后 taskState=%d avail=%d acquired=%d\n",
		sB.TaskState, sB.AvailSampsPerChan, sB.SampsPerChanAcquired)
	if sB.TaskState != 1 {
		fmt.Printf("失败 ❌ TaskState=%d (期望1=running)\n", sB.TaskState)
		os.Exit(1)
	}
	fmt.Printf("成功 ✅\n\n")

	// readLoop 启动前 GetStatus（完全对齐 WindLabX4 driver readLoop 第 546-557 行）
	var preLoop ffi.WTNDAQ16HAIStatus
	if err := ffi.WTNDAQ16HGetStatus(handle, &preLoop); err != nil {
		fmt.Printf("[pre-loop] GetStatus 失败: %v\n", err)
	} else {
		fmt.Printf("[pre-loop] taskState=%d avail=%d acquired=%d\n",
			preLoop.TaskState, preLoop.AvailSampsPerChan, preLoop.SampsPerChanAcquired)
	}

	// WindLabX4 复刻模式：完全对齐 WindLabX4 readLoop 的换算逻辑（Go 端 U16→mA→工程量）
	currentBuf := make([]float64, *nChans)
	if *WINDLABX4Mode {
		fmt.Printf("[WindLabX4 复刻] 使用 Go 端换算（U16 → mA → 工程量）\n\n")
	}

	// Step 5: 读取数据
	const readSamps = uint32(1)
	binBuf := make([]uint16, *nChans) // 1 sample × nChans channels

	if *multiThreadMode && *continuous > 0 {
		// 多线程复刻模式：ReadBinary 在新 goroutine 执行，复刻 WindLabX4 driver readLoop goroutine。
		// 验证假设：WTNDAQ16H_64.dll 有 OS 线程亲和性，跨线程调用会导致采集引擎不工作
		// （表现为 taskState=1 但 sampsPerChanAcquired 永远不增长）。
		fmt.Printf("[5/5] 持续采集 %v（多线程模式：ReadBinary 在新 goroutine）...\n\n", *continuous)
		deadline := time.Now().Add(*continuous)
		startTime := time.Now()
		totalReads := 0
		totalTimeouts := 0
		lastReport := time.Now()

		for time.Now().Before(deadline) {
			// 在新 goroutine 中调用 ReadBinary（每次都是新 goroutine，可能跑在不同 OS 线程）
			type readResult struct {
				sampsRead uint32
				avail     uint32
				err       error
				tid       uint32
			}
			resultCh := make(chan readResult, 1)
			go func() {
				tid := windows.GetCurrentThreadId()
				sampsRead, avail, err := ffi.WTNDAQ16HReadBinary(handle, binBuf, readSamps, 10.0)
				resultCh <- readResult{sampsRead, avail, err, tid}
			}()
			r := <-resultCh
			if r.err != nil {
				fmt.Printf("  [错误] 读取失败: %v\n", r.err)
				break
			}
			if r.sampsRead == 0 {
				totalTimeouts++
				if totalTimeouts <= 3 {
					fmt.Printf("  [超时 %d] OS线程ID=%d avail=%d\n", totalTimeouts, r.tid, r.avail)
				}
			} else {
				totalReads++
			}
			if time.Since(lastReport) >= time.Second {
				elapsed := time.Since(startTime).Truncate(time.Second)
				fmt.Printf("  [%v] reads=%d timeouts=%d (最后一次读取线程ID=%d)\n",
					elapsed, totalReads, totalTimeouts, r.tid)
				lastReport = time.Now()
			}
		}
		fmt.Printf("\n  采集完成：总读取 %d 次，超时 %d 次\n\n", totalReads, totalTimeouts)
	} else if *continuous > 0 {
		// 持续采集模式：模拟 WindLabX4 readLoop 行为（单线程）
		// 同时通过 SampsPerChanAcquired 增量计算真实硬件采样率，
		// 用于验证 fSampleRate 设置值与 DLL 实际采样率是否一致。
		fmt.Printf("[5/5] 持续采集 %v（单线程模式，每秒打印状态）...\n\n", *continuous)
		deadline := time.Now().Add(*continuous)
		startTime := time.Now()
		totalReads := 0
		totalTimeouts := 0
		lastReport := time.Now()

		// 记录上一秒的 SampsPerChanAcquired 用于计算每秒真实采样率
		var prevAcquired int64
		// 起始基准：跳过首秒（采集启动稳定期 avail 不准）
		firstReport := true

		for time.Now().Before(deadline) {
			sampsRead, avail, err := ffi.WTNDAQ16HReadBinary(handle, binBuf, readSamps, 10.0)
			if err != nil {
				fmt.Printf("  [错误] 读取失败: %v\n", err)
				break
			}
			if sampsRead == 0 {
				totalTimeouts++
				if totalTimeouts == 1 {
					fmt.Printf("  [首次超时] avail=%d\n", avail)
				}
				continue
			}
			totalReads++

			// WindLabX4 复刻模式：Go 端 U16 → mA 换算
			if *WINDLABX4Mode {
				for i := 0; i < *nChans; i++ {
					currentBuf[i] = float64(binBuf[i]) / 65535.0 * 20.0
				}
			}

			if time.Since(lastReport) >= time.Second {
				elapsed := time.Since(startTime).Truncate(time.Second)

				// 查询 DLL 内部记录的真实采集数
				var st ffi.WTNDAQ16HAIStatus
				acquiredDelta := int64(0)
				actualRate := 0.0
				if err := ffi.WTNDAQ16HGetStatus(handle, &st); err == nil {
					if !firstReport {
						acquiredDelta = st.SampsPerChanAcquired - prevAcquired
						actualRate = float64(acquiredDelta)
					}
					prevAcquired = st.SampsPerChanAcquired
					firstReport = false
				}

				if *nChans >= 16 {
					fmt.Printf("  [%v] reads=%d timeouts=%d avail=%d acquired=%d Δacq=%d realRate=%.1fHz CH00=%d CH07=%d CH15=%d\n",
						elapsed, totalReads, totalTimeouts, avail, st.SampsPerChanAcquired, acquiredDelta, actualRate,
						binBuf[0], binBuf[7], binBuf[15])
				} else {
					fmt.Printf("  [%v] reads=%d timeouts=%d avail=%d acquired=%d Δacq=%d realRate=%.1fHz CH00=%d\n",
						elapsed, totalReads, totalTimeouts, avail, st.SampsPerChanAcquired, acquiredDelta, actualRate,
						binBuf[0])
				}
				lastReport = time.Now()
			}
		}
		fmt.Printf("\n  采集完成：总读取 %d 次，超时 %d 次\n\n", totalReads, totalTimeouts)
	} else {
		// 短测模式：读取 3 次
		fmt.Printf("[5/5] 读取采集数据（3 次，每次 1 sample/%d channels）...\n\n", *nChans)
		for i := 0; i < 3; i++ {
			sampsRead, avail, err := ffi.WTNDAQ16HReadBinary(handle, binBuf, readSamps, 10.0)
			if err != nil {
				fmt.Printf("  第 %d 次读取失败: %v\n", i+1, err)
				continue
			}
			if sampsRead == 0 {
				fmt.Printf("  第 %d 次: 超时，无数据 (avail=%d)\n", i+1, avail)
				continue
			}

			if *WINDLABX4Mode {
				for ch := 0; ch < *nChans; ch++ {
					currentBuf[ch] = float64(binBuf[ch]) / 65535.0 * 20.0
				}
			}

			fmt.Printf("  第 %d 次 (sampsRead=%d, avail=%d):\n", i+1, sampsRead, avail)
			fmt.Printf("    原始码值(U16): ")
			for ch := 0; ch < *nChans; ch++ {
				fmt.Printf("CH%02d=%5d  ", ch, binBuf[ch])
				if ch == 7 {
					fmt.Printf("\n                  ")
				}
			}
			if *WINDLABX4Mode {
				fmt.Printf("\n    电流值(mA):   ")
				for ch := 0; ch < *nChans; ch++ {
					fmt.Printf("CH%02d=%6.2f  ", ch, currentBuf[ch])
					if ch == 7 {
						fmt.Printf("\n                  ")
					}
				}
			}
			fmt.Printf("\n\n")
			time.Sleep(500 * time.Millisecond)
		}
	}

	// 清理
	_ = ffi.WTNDAQ16HStopTask(handle)
	_ = ffi.WTNDAQ16HReleaseTask(handle)
	_ = ffi.WTNDAQ16HDevRelease(handle)

	fmt.Printf("=== 测试完成 ✅ ===\n")
}

// buildAIParam 按 WindLabX4 buildAIParamLocked 的逻辑构造 AI 参数
// 差异：本工具按 -chans 参数启用前 N 个通道，WindLabX4 按 profile.Channels[].Enabled
func buildAIParam(rate float64, sampleRange uint32, nChans int) ffi.WTNDAQ16HAIParam {
	var p ffi.WTNDAQ16HAIParam

	if nChans < 1 {
		nChans = 1
	}
	if nChans > int(ffi.WTNDAQ16H_AI_MAX_CHANNELS) {
		nChans = int(ffi.WTNDAQ16H_AI_MAX_CHANNELS)
	}

	// 通道参数：启用前 nChans 个通道，统一量程
	p.SampChanCount = uint32(nChans)
	for i := 0; i < nChans; i++ {
		p.CHParam[i] = ffi.WTNDAQ16HAIChParam{
			Channel:     uint32(i),
			SampleRange: sampleRange,
			RefGround:   ffi.WTNDAQ16H_AI_REFGND_DIFF,
		}
	}
	p.ChanScanMode = ffi.WTNDAQ16H_AI_CHAN_SCANMODE_CONTINUOUS
	p.GroupLoops = 1
	p.GroupInterval = 1 // 厂家代码设为 1，0 会导致采集异常
	p.SampleSignal = ffi.WTNDAQ16H_AI_SAMPSIGNAL_AI

	// 时钟参数
	p.SampleMode = ffi.WTNDAQ16H_AI_SAMPMODE_CONTINUOUS
	p.SampsPerChan = 1024
	p.SampleRate = rate
	p.ClockSource = ffi.WTNDAQ16H_AI_CLKSRC_LOCAL
	p.ClockOutput = 0 // FALSE

	// 触发参数：禁用硬件触发，仅靠 SendSoftTrig 启动采集
	p.DTriggerEn = 0 // FALSE
	p.ATriggerEn = 0 // FALSE

	return p
}
