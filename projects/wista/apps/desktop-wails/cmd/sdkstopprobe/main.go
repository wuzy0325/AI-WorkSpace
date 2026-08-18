package main

import (
	"fmt"
	"sync/atomic"
	"time"

	"shared.local/device-sdk/go/daq/core"
	"shared.local/device-sdk/go/daq/hardware"
)

func main() {
	profile := core.Profile{
		ID:        "probe",
		Name:      "probe",
		Type:      "wista",
		Address:   "192.168.1.10",
		Port:      9000,
		Transport: "tcp",
		DaqT1603Config: core.DaqT1603HardwareConfig{
			ChannelMask: "FFFF",
		},
	}

	d := hardware.NewDAQT1603(profile)
	var frames int64
	d.OnLog(func(le hardware.LogEntry) {
		fmt.Printf("[%s] [log %s/%s] %s | %s\n", time.Now().Format("15:04:05.000"), le.Level, le.Category, le.Message, le.Detail)
	})
	d.SetDataSink(func(p core.DataPayload) { atomic.AddInt64(&frames, 1) })
	d.OnReadLoopExit(func(err error) {
		fmt.Printf("[%s] [CALLBACK onReadLoopExit] err=%v\n", time.Now().Format("15:04:05.000"), err)
	})

	err := d.Connect()
	fmt.Printf("[%s] Connect: %v\n", time.Now().Format("15:04:05.000"), err)
	if err != nil {
		return
	}

	for i := 1; i <= 3; i++ {
		fmt.Printf("=== Round %d ===\n", i)
		t0 := time.Now()
		err = d.StartAcquisition()
		fmt.Printf("[%s] StartAcquisition: err=%v elapsed=%v\n", time.Now().Format("15:04:05.000"), err, time.Since(t0).Round(time.Millisecond))
		if err != nil {
			continue
		}
		time.Sleep(1500 * time.Millisecond)
		f := atomic.LoadInt64(&frames)
		fmt.Printf("[%s] frames so far=%d\n", time.Now().Format("15:04:05.000"), f)
		t0 = time.Now()
		err = d.StopAcquisition()
		fmt.Printf("[%s] StopAcquisition: err=%v elapsed=%v\n", time.Now().Format("15:04:05.000"), err, time.Since(t0).Round(time.Millisecond))
		time.Sleep(300 * time.Millisecond)
	}

	t0 := time.Now()
	err = d.Disconnect()
	fmt.Printf("[%s] Disconnect: err=%v elapsed=%v status=%s\n", time.Now().Format("15:04:05.000"), err, time.Since(t0).Round(time.Millisecond), d.Status().Connection)

	t0 = time.Now()
	err = d.Connect()
	fmt.Printf("[%s] Reconnect: err=%v elapsed=%v status=%s\n", time.Now().Format("15:04:05.000"), err, time.Since(t0).Round(time.Millisecond), d.Status().Connection)

	t0 = time.Now()
	err = d.StartAcquisition()
	fmt.Printf("[%s] Start after reconnect: err=%v elapsed=%v\n", time.Now().Format("15:04:05.000"), err, time.Since(t0).Round(time.Millisecond))
	if err == nil {
		time.Sleep(1500 * time.Millisecond)
		f := atomic.LoadInt64(&frames)
		fmt.Printf("[%s] frames after reconnect=%d\n", time.Now().Format("15:04:05.000"), f)
		t0 = time.Now()
		err = d.StopAcquisition()
		fmt.Printf("[%s] Stop: err=%v elapsed=%v\n", time.Now().Format("15:04:05.000"), err, time.Since(t0).Round(time.Millisecond))
	}
	d.Disconnect()
	fmt.Println("done")
}
