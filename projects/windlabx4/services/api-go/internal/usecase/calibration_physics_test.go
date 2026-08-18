package usecase

import (
	"math"
	"sync"
	"testing"

	"windlabx4/services/api-go/internal/core/calibration"
	"windlabx4/services/api-go/internal/core/device"
)

// physicsTestReader 可配置的 LatestDataReader mock，供 Task 13 物理快照测试使用。
//
// 设计动机：calibrationStatusLatestReader 是单值常量 mock，无法覆盖零流量/缺失通道/
// 多设备分摊等场景。此处用 map[deviceID]payload 让每个测试自定义返回数据，
// 并通过 ok 标志模拟设备离线（GetLatestData 返回 false → channelReader 返回 found=false）。
type physicsTestReader struct {
	payloads map[string]device.DataPayload
}

func (r *physicsTestReader) GetLatestData(deviceID string) (device.DataPayload, bool) {
	p, ok := r.payloads[deviceID]
	return p, ok
}

func (r *physicsTestReader) GetLatestTimestamp(_ string) (int64, bool) { return 0, false }

// buildPayload 把 (channelIndex → value) 映射打包成 DataPayload。
// ChannelIndices 与 Channels 严格按 map 任意顺序展开——valuesForChannelIndex
// 通过线性匹配 idx 查找，与顺序无关。
func buildPayload(values map[int]float64) device.DataPayload {
	payload := device.DataPayload{
		Channels:       make([]float64, 0, len(values)),
		ChannelIndices: make([]int, 0, len(values)),
	}
	for idx, v := range values {
		payload.ChannelIndices = append(payload.ChannelIndices, idx)
		payload.Channels = append(payload.Channels, v)
	}
	return payload
}

// ==================== 五孔探针物理快照 ====================

// TestCalibrationStatusPhysics_FiveHole_NonZero 验证五孔 Pt > Ps 时返回非零 Ma/V。
//
// 测试前置：构造五孔配置（9 通道齐全），reader 返回 Pt=80Pa 表压、Ps=15Pa 表压。
// 测试步骤：调用 Status()。
// 期待结果：LivePhysics 非 nil，MachNumber/Velocity 非 nil 且 > 0。
func TestCalibrationStatusPhysics_FiveHole_NonZero(t *testing.T) {
	reader := &physicsTestReader{
		payloads: map[string]device.DataPayload{
			"dev-1": buildPayload(map[int]float64{
				0: 100, 1: 200, 2: 100, 3: 100, 4: 100, // p1~p5
				5: 101325, 6: 20, // pAtm, tAtm
				7: 80, 8: 15, // pTotal(gauge), pStatic(gauge)
			}),
		},
	}
	manager := NewCalibrationManager(reader, nil, nil, nil)
	manager.currentConfig = fiveHolePhysicsTestConfig("cal-five-nonzero")
	// P1-3 修复后 LivePhysics 只在 running/paused 态组装，测试需显式设态
	manager.currentStatus.State = calibration.StateRunning

	status := manager.Status()
	if status.LivePhysics == nil {
		t.Fatal("LivePhysics 不应为 nil（五孔 + 通道齐全）")
	}
	if status.LivePhysics.MachNumber == nil {
		t.Fatal("MachNumber 不应为 nil（Pt > Ps 应计算非零 Ma）")
	}
	if *status.LivePhysics.MachNumber <= 0 {
		t.Errorf("Ma 期望 > 0, 实际 %v", *status.LivePhysics.MachNumber)
	}
	if status.LivePhysics.Velocity == nil || *status.LivePhysics.Velocity <= 0 {
		t.Errorf("Velocity 期望 > 0, 实际 %v", status.LivePhysics.Velocity)
	}
}

// TestCalibrationStatusPhysics_FiveHole_ZeroFlow 验证五孔 Pt == Ps 即零流量返回 &0/&0。
//
// 测试前置：五孔配置，reader 返回 Pt=Ps=80Pa 表压（ptAbs == psAbs）。
// 测试步骤：调用 Status()。
// 期待结果：LivePhysics.MachNumber == &0, Velocity == &0（非 nil，零值）。
//
// 此测试是 Task 12 零流量语义在 Status 路径的回归保护——
// 旧实现 Pt <= Ps 一律视为非法，导致 UI 在零流量时显示 "--" 而非 "0"。
func TestCalibrationStatusPhysics_FiveHole_ZeroFlow(t *testing.T) {
	reader := &physicsTestReader{
		payloads: map[string]device.DataPayload{
			"dev-1": buildPayload(map[int]float64{
				0: 100, 1: 200, 2: 100, 3: 100, 4: 100,
				5: 101325, 6: 20,
				7: 80, 8: 80, // pTotal == pStatic → ptAbs == psAbs
			}),
		},
	}
	manager := NewCalibrationManager(reader, nil, nil, nil)
	manager.currentConfig = fiveHolePhysicsTestConfig("cal-five-zero")
	manager.currentStatus.State = calibration.StateRunning

	status := manager.Status()
	if status.LivePhysics == nil {
		t.Fatal("LivePhysics 不应为 nil（零流量应返回 &0 而非 missing）")
	}
	if status.LivePhysics.MachNumber == nil {
		t.Fatal("MachNumber 不应为 nil（零流量是有效零，非缺失）")
	}
	if *status.LivePhysics.MachNumber != 0 {
		t.Errorf("Ma 期望 0 (零流量), 实际 %v", *status.LivePhysics.MachNumber)
	}
	if status.LivePhysics.Velocity == nil {
		t.Fatal("Velocity 不应为 nil（零流量是有效零，非缺失）")
	}
	if *status.LivePhysics.Velocity != 0 {
		t.Errorf("Velocity 期望 0 (零流量), 实际 %v", *status.LivePhysics.Velocity)
	}
}

// TestCalibrationStatusPhysics_FiveHole_MissingTotalStatic 验证 pTotal/pStatic 读取失败时返回 nil 字段。
//
// 测试前置：五孔配置，但 reader 对 dev-1 返回 false（设备离线）。
// 测试步骤：调用 Status()。
// 期待结果：LivePhysics 非 nil 但 MachNumber/Velocity 均为 nil（缺失语义）。
func TestCalibrationStatusPhysics_FiveHole_MissingTotalStatic(t *testing.T) {
	reader := &physicsTestReader{
		payloads: map[string]device.DataPayload{}, // dev-1 离线
	}
	manager := NewCalibrationManager(reader, nil, nil, nil)
	manager.currentConfig = fiveHolePhysicsTestConfig("cal-five-missing")
	manager.currentStatus.State = calibration.StateRunning

	status := manager.Status()
	if status.LivePhysics == nil {
		t.Fatal("LivePhysics 不应为 nil（配置了通道但读取失败应返回空快照）")
	}
	if status.LivePhysics.MachNumber != nil {
		t.Errorf("MachNumber 期望 nil (通道读取失败), 实际 %v", *status.LivePhysics.MachNumber)
	}
	if status.LivePhysics.Velocity != nil {
		t.Errorf("Velocity 期望 nil (通道读取失败), 实际 %v", *status.LivePhysics.Velocity)
	}
}

// ==================== 三孔探针物理快照 ====================

// TestCalibrationStatusPhysics_ThreeHole_NonZero 验证三孔 Pt > Ps 时返回非零 Ma/V。
//
// 测试前置：三孔配置（p1/p2/p3/pAtm/tAtm/pTotal/pStatic 全配置），Pt=1500, Ps=800。
// 测试步骤：调用 Status()。
// 期待结果：Ma > 0, V > 0。
func TestCalibrationStatusPhysics_ThreeHole_NonZero(t *testing.T) {
	reader := &physicsTestReader{
		payloads: map[string]device.DataPayload{
			"dev-1": buildPayload(map[int]float64{
				0: 100, 1: 300, 2: 200, // p1, p2, p3
				3: 101325, 4: 20, // pAtm, tAtm
				5: 1500, 6: 800, // pTotal, pStatic
			}),
		},
	}
	manager := NewCalibrationManager(reader, nil, nil, nil)
	manager.currentConfig = threeHolePhysicsTestConfig("cal-three-nonzero")
	manager.currentStatus.State = calibration.StateRunning

	status := manager.Status()
	if status.LivePhysics == nil || status.LivePhysics.MachNumber == nil {
		t.Fatalf("LivePhysics.MachNumber 不应为 nil, status=%+v", status.LivePhysics)
	}
	if *status.LivePhysics.MachNumber <= 0 {
		t.Errorf("Ma 期望 > 0, 实际 %v", *status.LivePhysics.MachNumber)
	}
}

// TestCalibrationStatusPhysics_ThreeHole_ZeroFlow 验证三孔 Pt == Ps 即零流量返回 &0。
func TestCalibrationStatusPhysics_ThreeHole_ZeroFlow(t *testing.T) {
	reader := &physicsTestReader{
		payloads: map[string]device.DataPayload{
			"dev-1": buildPayload(map[int]float64{
				0: 100, 1: 300, 2: 200,
				3: 101325, 4: 20,
				5: 500, 6: 500, // pTotal == pStatic
			}),
		},
	}
	manager := NewCalibrationManager(reader, nil, nil, nil)
	manager.currentConfig = threeHolePhysicsTestConfig("cal-three-zero")
	manager.currentStatus.State = calibration.StateRunning

	status := manager.Status()
	if status.LivePhysics == nil || status.LivePhysics.MachNumber == nil {
		t.Fatalf("Ma 不应为 nil (零流量是有效零), status=%+v", status.LivePhysics)
	}
	if *status.LivePhysics.MachNumber != 0 {
		t.Errorf("Ma 期望 0, 实际 %v", *status.LivePhysics.MachNumber)
	}
}

// TestCalibrationStatusPhysics_ThreeHole_MissingTotalStatic 验证三孔未配置 pTotal/pStatic 时字段为 nil。
//
// 测试前置：三孔配置仅含 p1/p2/p3/pAtm/tAtm（pTotal/pStatic 可选，未配置）。
// 测试步骤：调用 Status()。
// 期待结果：LivePhysics 非 nil 但 Ma/V 为 nil（缺失语义）。
func TestCalibrationStatusPhysics_ThreeHole_MissingTotalStatic(t *testing.T) {
	reader := &physicsTestReader{
		payloads: map[string]device.DataPayload{
			"dev-1": buildPayload(map[int]float64{
				0: 100, 1: 300, 2: 200,
				3: 101325, 4: 20,
				// 无 pTotal/pStatic 通道
			}),
		},
	}
	manager := NewCalibrationManager(reader, nil, nil, nil)
	// 显式构造不含 pTotal/pStatic 的三孔配置（threeHolePhysicsTestConfig 含全部 7 通道）。
	manager.currentConfig = calibration.Config{
		TaskID: "cal-three-missing",
		Type:   string(calibration.TypeThreeHole),
		ProbeChannels: []calibration.ProbeChannel{
			{Role: "threeHole.p1", DeviceID: "dev-1", ChannelIndex: 0, Enabled: true},
			{Role: "threeHole.p2", DeviceID: "dev-1", ChannelIndex: 1, Enabled: true},
			{Role: "threeHole.p3", DeviceID: "dev-1", ChannelIndex: 2, Enabled: true},
			{Role: "threeHole.pAtm", DeviceID: "dev-1", ChannelIndex: 3, Enabled: true},
			{Role: "threeHole.tAtm", DeviceID: "dev-1", ChannelIndex: 4, Enabled: true},
			// 故意不配置 threeHole.pTotal / threeHole.pStatic
		},
	}
	manager.currentStatus.State = calibration.StateRunning

	status := manager.Status()
	if status.LivePhysics == nil {
		t.Fatal("LivePhysics 不应为 nil（三孔基础通道齐全）")
	}
	if status.LivePhysics.MachNumber != nil {
		t.Errorf("Ma 期望 nil (pTotal/pStatic 未配置), 实际 %v", *status.LivePhysics.MachNumber)
	}
	if status.LivePhysics.Velocity != nil {
		t.Errorf("Velocity 期望 nil, 实际 %v", *status.LivePhysics.Velocity)
	}
}

// ==================== 总压探针物理快照 ====================

// TestCalibrationStatusPhysics_TotalPressure_NonZero 验证总压 Pt > Ps 时返回非零 Ma/V。
//
// 测试前置：总压配置，PTunnelTotal=5000Pa 表压、PTunnelStatic=0Pa 表压（绝压即 atm）。
// 测试步骤：调用 Status()。
// 期待结果：Ma > 0, V > 0。
func TestCalibrationStatusPhysics_TotalPressure_NonZero(t *testing.T) {
	reader := &physicsTestReader{
		payloads: map[string]device.DataPayload{
			"dev-1": buildPayload(map[int]float64{
				0: 101325,        // pAtm
				1: 20,            // tAtm
				2: 5000,          // pTunnelTotal (gauge)
				3: 0,             // pTunnelStatic (gauge)
				4: 0,             // tTunnel (未配置，回退 tAtm)
				5: 5050,          // pProbeTotal
			}),
		},
	}
	manager := NewCalibrationManager(reader, nil, nil, nil)
	manager.currentConfig = totalPressurePhysicsTestConfig("cal-tp-nonzero")
	manager.currentStatus.State = calibration.StateRunning

	status := manager.Status()
	if status.LivePhysics == nil || status.LivePhysics.MachNumber == nil {
		t.Fatalf("Ma 不应为 nil, status=%+v", status.LivePhysics)
	}
	if *status.LivePhysics.MachNumber <= 0 {
		t.Errorf("Ma 期望 > 0, 实际 %v", *status.LivePhysics.MachNumber)
	}
}

// TestCalibrationStatusPhysics_TotalPressure_ZeroFlow 验证总压 Pt == Ps 即零流量返回 &0。
//
// 测试前置：总压配置，PTunnelTotal == PTunnelStatic = 1000Pa 表压。
// 测试步骤：调用 Status()。
// 期待结果：Ma == 0, V == 0（非 nil）。
func TestCalibrationStatusPhysics_TotalPressure_ZeroFlow(t *testing.T) {
	reader := &physicsTestReader{
		payloads: map[string]device.DataPayload{
			"dev-1": buildPayload(map[int]float64{
				0: 101325,
				1: 20,
				2: 1000, // pTunnelTotal
				3: 1000, // pTunnelStatic == pTunnelTotal
				4: 0,
				5: 1000,
			}),
		},
	}
	manager := NewCalibrationManager(reader, nil, nil, nil)
	manager.currentConfig = totalPressurePhysicsTestConfig("cal-tp-zero")
	manager.currentStatus.State = calibration.StateRunning

	status := manager.Status()
	if status.LivePhysics == nil || status.LivePhysics.MachNumber == nil {
		t.Fatalf("Ma 不应为 nil (零流量), status=%+v", status.LivePhysics)
	}
	if *status.LivePhysics.MachNumber != 0 {
		t.Errorf("Ma 期望 0, 实际 %v", *status.LivePhysics.MachNumber)
	}
	if status.LivePhysics.Velocity == nil || *status.LivePhysics.Velocity != 0 {
		t.Errorf("Velocity 期望 &0, 实际 %v", status.LivePhysics.Velocity)
	}
}

// TestCalibrationStatusPhysics_TotalPressure_TTunnelPriority 验证 TTunnel > TAtm 优先级。
//
// 测试前置：总压配置，TTunnel=30°C, TAtm=20°C（显著差异以便区分）。
// 测试步骤：调用 Status() 并通过对照公式验证 SAT 用 TTunnel 计算。
// 期待结果：Ma 相同（不依赖温度），V 基于 TTunnel 的 SAT 算出，比基于 TAtm 的 V 大约 1.7%。
func TestCalibrationStatusPhysics_TotalPressure_TTunnelPriority(t *testing.T) {
	reader := &physicsTestReader{
		payloads: map[string]device.DataPayload{
			"dev-1": buildPayload(map[int]float64{
				0: 101325,
				1: 20,   // tAtm
				2: 5000, // pTunnelTotal
				3: 0,    // pTunnelStatic
				4: 30,   // tTunnel (优先)
				5: 5050,
			}),
		},
	}
	manager := NewCalibrationManager(reader, nil, nil, nil)
	manager.currentConfig = totalPressurePhysicsTestConfig("cal-tp-ttunnel")
	manager.currentStatus.State = calibration.StateRunning

	status := manager.Status()
	if status.LivePhysics == nil || status.LivePhysics.Velocity == nil {
		t.Fatalf("Velocity 不应为 nil, status=%+v", status.LivePhysics)
	}

	// 构造对照：手动用 TAtm 计算 V，验证 status.V 更接近 TTunnel 公式结果。
	// V = Ma × 20.047 × √SAT, SAT = TAT / (1 + 0.2 × r × Ma²)
	calc := calibration.NewAtmosphericDataCalculator()
	ptAbs := 5000.0 + 101325.0
	psAbs := 0.0 + 101325.0
	ma, err := calc.CalculateMach(ptAbs, psAbs)
	if err != nil {
		t.Fatalf("对照 Mach 计算: %v", err)
	}

	// TTunnel 路径
	satTTunnel := calc.CalculateSAT(30+273.15, ma)
	vTTunnel := calc.CalculateTASByMach(ma, satTTunnel)

	// TAtm 路径
	satTAtm := calc.CalculateSAT(20+273.15, ma)
	vTAtm := calc.CalculateTASByMach(ma, satTAtm)

	got := *status.LivePhysics.Velocity
	if math.Abs(got-vTTunnel) > math.Abs(got-vTAtm) {
		t.Errorf("Velocity 应基于 TTunnel (%.2f) 而非 TAtm (%.2f), 实际 %.2f", vTTunnel, vTAtm, got)
	}
}

// ==================== 七孔探针物理快照 ====================

// TestCalibrationStatusPhysics_SevenHole_NonZero 验证七孔 Pt > Ps 时返回非零 Ma/V。
//
// 测试前置：七孔配置（11 通道齐全），PTotal=4000Pa 表压、PStatic=-50Pa 表压。
// 测试步骤：调用 Status()。
// 期待结果：Ma > 0, V > 0（与五孔/三孔/总压同口径，因为都委托 AtmosphericDataCalculator）。
func TestCalibrationStatusPhysics_SevenHole_NonZero(t *testing.T) {
	reader := &physicsTestReader{
		payloads: map[string]device.DataPayload{
			"dev-1": buildPayload(map[int]float64{
				0: 100, 1: 100, 2: 100, 3: 100, 4: 100, 5: 100, 6: 200, // p1~p7
				7: 98880, 8: 20, // pAtm, tAtm
				9: 4000, 10: -50, // pTotal, pStatic (gauge)
			}),
		},
	}
	manager := NewCalibrationManager(reader, nil, nil, nil)
	manager.currentConfig = sevenHolePhysicsTestConfig("cal-seven-nonzero")
	manager.currentStatus.State = calibration.StateRunning

	status := manager.Status()
	if status.LivePhysics == nil || status.LivePhysics.MachNumber == nil {
		t.Fatalf("Ma 不应为 nil, status=%+v", status.LivePhysics)
	}
	if *status.LivePhysics.MachNumber <= 0 {
		t.Errorf("Ma 期望 > 0, 实际 %v", *status.LivePhysics.MachNumber)
	}
}

// TestCalibrationStatusPhysics_SevenHole_ZeroFlow 验证七孔 Pt == Ps 即零流量返回 &0。
func TestCalibrationStatusPhysics_SevenHole_ZeroFlow(t *testing.T) {
	reader := &physicsTestReader{
		payloads: map[string]device.DataPayload{
			"dev-1": buildPayload(map[int]float64{
				0: 100, 1: 100, 2: 100, 3: 100, 4: 100, 5: 100, 6: 200,
				7: 98880, 8: 20,
				9: 500, 10: 500, // pTotal == pStatic
			}),
		},
	}
	manager := NewCalibrationManager(reader, nil, nil, nil)
	manager.currentConfig = sevenHolePhysicsTestConfig("cal-seven-zero")
	manager.currentStatus.State = calibration.StateRunning

	status := manager.Status()
	if status.LivePhysics == nil || status.LivePhysics.MachNumber == nil {
		t.Fatalf("Ma 不应为 nil (零流量), status=%+v", status.LivePhysics)
	}
	if *status.LivePhysics.MachNumber != 0 {
		t.Errorf("Ma 期望 0, 实际 %v", *status.LivePhysics.MachNumber)
	}
}

// TestCalibrationStatusPhysics_SevenHole_MissingTotalStatic 验证七孔 PTotal/PStatic 缺失时字段为 nil。
//
// 测试前置：七孔配置，但 reader 对 dev-1 返回 false。
// 测试步骤：调用 Status()。
// 期待结果：LivePhysics 非 nil 但 Ma/V 为 nil。
func TestCalibrationStatusPhysics_SevenHole_MissingTotalStatic(t *testing.T) {
	reader := &physicsTestReader{
		payloads: map[string]device.DataPayload{}, // dev-1 离线
	}
	manager := NewCalibrationManager(reader, nil, nil, nil)
	manager.currentConfig = sevenHolePhysicsTestConfig("cal-seven-missing")
	manager.currentStatus.State = calibration.StateRunning

	status := manager.Status()
	if status.LivePhysics == nil {
		t.Fatal("LivePhysics 不应为 nil（配置了通道但读取失败）")
	}
	if status.LivePhysics.MachNumber != nil {
		t.Errorf("Ma 期望 nil (读取失败), 实际 %v", *status.LivePhysics.MachNumber)
	}
}

// TestCalibrationStatusPhysics_SevenHole_TTunnelPriority 验证七孔 TTunnel > TAtm 优先级。
func TestCalibrationStatusPhysics_SevenHole_TTunnelPriority(t *testing.T) {
	reader := &physicsTestReader{
		payloads: map[string]device.DataPayload{
			"dev-1": buildPayload(map[int]float64{
				0: 100, 1: 100, 2: 100, 3: 100, 4: 100, 5: 100, 6: 200,
				7: 98880, 8: 20, // tAtm
				9: 4000, 10: -50,
				11: 30, // tTunnel（优先）
			}),
		},
	}
	manager := NewCalibrationManager(reader, nil, nil, nil)
	manager.currentConfig = sevenHolePhysicsTestConfigWithTTunnel("cal-seven-ttunnel")
	manager.currentStatus.State = calibration.StateRunning

	status := manager.Status()
	if status.LivePhysics == nil || status.LivePhysics.Velocity == nil {
		t.Fatalf("Velocity 不应为 nil, status=%+v", status.LivePhysics)
	}

	calc := calibration.NewAtmosphericDataCalculator()
	ptAbs := 4000.0 + 98880.0
	psAbs := -50.0 + 98880.0
	ma, err := calc.CalculateMach(ptAbs, psAbs)
	if err != nil {
		t.Fatalf("对照 Mach 计算: %v", err)
	}
	vTTunnel := calc.CalculateTASByMach(ma, calc.CalculateSAT(30+273.15, ma))
	vTAtm := calc.CalculateTASByMach(ma, calc.CalculateSAT(20+273.15, ma))

	got := *status.LivePhysics.Velocity
	if math.Abs(got-vTTunnel) > math.Abs(got-vTAtm) {
		t.Errorf("Velocity 应基于 TTunnel (%.2f) 而非 TAtm (%.2f), 实际 %.2f", vTTunnel, vTAtm, got)
	}
}

// ==================== 边界场景 ====================

// TestCalibrationStatusPhysics_IdleNoConfig 验证无配置时 LivePhysics 为 nil。
//
// 测试前置：manager 刚构造，currentConfig 为零值（Type="")。
// 测试步骤：调用 Status()。
// 期待结果：LivePhysics == nil（不返回空快照）。
func TestCalibrationStatusPhysics_IdleNoConfig(t *testing.T) {
	manager := NewCalibrationManager(&physicsTestReader{payloads: nil}, nil, nil, nil)
	status := manager.Status()
	if status.LivePhysics != nil {
		t.Errorf("LivePhysics 期望 nil (无配置), 实际 %+v", status.LivePhysics)
	}
}

// TestCalibrationStatusPhysics_TotalTemperatureUnsupported 验证总温类型不支持物理快照。
//
// 测试前置：currentConfig.Type = total-temperature。
// 测试步骤：调用 Status()。
// 期待结果：LivePhysics == nil（总温无 Pt/Ps 物理量概念）。
func TestCalibrationStatusPhysics_TotalTemperatureUnsupported(t *testing.T) {
	manager := NewCalibrationManager(&physicsTestReader{payloads: nil}, nil, nil, nil)
	manager.currentConfig = calibration.Config{
		TaskID: "cal-tt",
		Type:   string(calibration.TypeTotalTemperature),
		ProbeChannels: []calibration.ProbeChannel{
			{Role: "totalTemperature.probe", DeviceID: "dev-1", ChannelIndex: 0, Enabled: true},
		},
	}
	status := manager.Status()
	if status.LivePhysics != nil {
		t.Errorf("LivePhysics 期望 nil (总温不支持), 实际 %+v", status.LivePhysics)
	}
}

// TestCalibrationStatusPhysics_StaleClearing 验证不保留 stale physics。
//
// 测试前置：第一次 reader 返回有效数据 → physics 非 nil；之后 reader 离线。
// 测试步骤：第一次 Status()，移除 reader 数据，第二次 Status()。
// 期待结果：第二次 LivePhysics.MachNumber == nil（不是第一次的旧值）。
//
// 此测试保护核心不变量：LivePhysics 每次即时计算，不缓存到 currentStatus。
func TestCalibrationStatusPhysics_StaleClearing(t *testing.T) {
	reader := &physicsTestReader{
		payloads: map[string]device.DataPayload{
			"dev-1": buildPayload(map[int]float64{
				0: 100, 1: 200, 2: 100, 3: 100, 4: 100,
				5: 101325, 6: 20,
				7: 80, 8: 15,
			}),
		},
	}
	manager := NewCalibrationManager(reader, nil, nil, nil)
	manager.currentConfig = fiveHolePhysicsTestConfig("cal-stale")
	manager.currentStatus.State = calibration.StateRunning

	first := manager.Status()
	if first.LivePhysics == nil || first.LivePhysics.MachNumber == nil {
		t.Fatalf("第一次 Status 应返回非 nil Ma, status=%+v", first.LivePhysics)
	}

	// 设备离线 → 通道读取失败
	reader.payloads = map[string]device.DataPayload{}

	second := manager.Status()
	if second.LivePhysics == nil {
		t.Fatal("第二次 Status LivePhysics 不应为 nil（应返回空快照而非整体省略）")
	}
	if second.LivePhysics.MachNumber != nil {
		t.Errorf("第二次 Ma 期望 nil (stale 应清除), 实际 %v", *second.LivePhysics.MachNumber)
	}
}

// TestCalibrationStatusPhysics_TerminalStatesSkipLivePhysics 验证终态（completed/error/stopped）
// 下即使 reader 仍在线、currentConfig 仍指向旧任务，Status() 也不应组装 LivePhysics。
//
// 回归保护（review P1 缺陷）：
//   - 旧实现无条件调用 resolveLivePhysics(config)，终态时仍返回最后一帧实时 Ma/V。
//   - 前端 updateStatusFromBackend 终态分支会 stopStatusPolling，导致这一帧 stale physics
//     永久停留在 UI 上，给操作员"任务还在跑"的错觉。
//   - 修复：只在 running/paused 时组装 LivePhysics；终态下 status.LivePhysics 保持 nil。
//
// 测试前置：reader 仍在线返回有效数据，currentConfig 五孔通道齐全；分别把 currentStatus.State
// 设为 completed/error/stopped。
// 测试步骤：调用 Status()。
// 期待结果：三种终态下 LivePhysics 均为 nil。
func TestCalibrationStatusPhysics_TerminalStatesSkipLivePhysics(t *testing.T) {
	reader := &physicsTestReader{
		payloads: map[string]device.DataPayload{
			"dev-1": buildPayload(map[int]float64{
				0: 100, 1: 200, 2: 100, 3: 100, 4: 100,
				5: 101325, 6: 20,
				7: 80, 8: 15,
			}),
		},
	}

	terminalStates := []calibration.State{
		calibration.StateCompleted,
		calibration.StateError,
		calibration.StateStopped,
	}

	for _, state := range terminalStates {
		t.Run(string(state), func(t *testing.T) {
			manager := NewCalibrationManager(reader, nil, nil, nil)
			manager.currentConfig = fiveHolePhysicsTestConfig("cal-terminal-" + string(state))
			manager.currentStatus.State = state

			status := manager.Status()
			if status.LivePhysics != nil {
				t.Errorf("终态 %s 下 LivePhysics 应为 nil（避免 stale 残留）, 实际 %+v",
					state, status.LivePhysics)
			}
		})
	}
}

// TestCalibrationStatusPhysics_RunningAndPausedIncludeLivePhysics 验证 running/paused 态
// 仍正常组装 LivePhysics——确保 P1-3 修复未误伤正常路径。
//
// 测试前置：reader 在线返回有效数据，currentConfig 五孔通道齐全；currentStatus.State 设为
// running 或 paused。
// 测试步骤：调用 Status()。
// 期待结果：LivePhysics 非 nil，MachNumber 非 nil（Pt > Ps 应计算非零 Ma）。
func TestCalibrationStatusPhysics_RunningAndPausedIncludeLivePhysics(t *testing.T) {
	reader := &physicsTestReader{
		payloads: map[string]device.DataPayload{
			"dev-1": buildPayload(map[int]float64{
				0: 100, 1: 200, 2: 100, 3: 100, 4: 100,
				5: 101325, 6: 20,
				7: 80, 8: 15,
			}),
		},
	}

	activeStates := []calibration.State{calibration.StateRunning, calibration.StatePaused}

	for _, state := range activeStates {
		t.Run(string(state), func(t *testing.T) {
			manager := NewCalibrationManager(reader, nil, nil, nil)
			manager.currentConfig = fiveHolePhysicsTestConfig("cal-active-" + string(state))
			manager.currentStatus.State = state

			status := manager.Status()
			if status.LivePhysics == nil || status.LivePhysics.MachNumber == nil {
				t.Fatalf("活跃态 %s 下 LivePhysics.MachNumber 不应为 nil, status=%+v",
					state, status.LivePhysics)
			}
			if *status.LivePhysics.MachNumber <= 0 {
				t.Errorf("活跃态 %s 下 Ma 期望 > 0, 实际 %v", state, *status.LivePhysics.MachNumber)
			}
		})
	}
}

// TestCalibrationStatusPhysics_DoesNotPollutePersistentStatus 验证不污染持久化 currentStatus。
//
// 测试前置：manager.currentStatus.LivePhysics 显式设为 nil（默认），currentConfig 五孔。
// 测试步骤：调用 Status()。
// 期待结果：返回的 status.LivePhysics 非 nil，但 manager.currentStatus.LivePhysics 仍为 nil。
//
// 此测试保护不变量：LivePhysics 是临时计算结果，绝不持久化。
func TestCalibrationStatusPhysics_DoesNotPollutePersistentStatus(t *testing.T) {
	reader := &physicsTestReader{
		payloads: map[string]device.DataPayload{
			"dev-1": buildPayload(map[int]float64{
				0: 100, 1: 200, 2: 100, 3: 100, 4: 100,
				5: 101325, 6: 20,
				7: 80, 8: 15,
			}),
		},
	}
	manager := NewCalibrationManager(reader, nil, nil, nil)
	manager.currentConfig = fiveHolePhysicsTestConfig("cal-no-pollute")
	manager.currentStatus.State = calibration.StateRunning

	returned := manager.Status()
	if returned.LivePhysics == nil {
		t.Fatal("返回的 LivePhysics 不应为 nil")
	}

	manager.mu.RLock()
	persistent := manager.currentStatus.LivePhysics
	manager.mu.RUnlock()
	if persistent != nil {
		t.Errorf("持久化 currentStatus.LivePhysics 必须始终为 nil, 实际 %+v", persistent)
	}
}

// TestCalibrationStatusPhysics_MultiDevice 验证多设备通道分摊正确解析。
//
// 测试前置：五孔配置，p1~p5 在 dev-1，pAtm/tAtm/pTotal/pStatic 在 dev-2。
// 测试步骤：调用 Status()。
// 期待结果：LivePhysics.MachNumber 非 nil（跨设备读取都成功）。
func TestCalibrationStatusPhysics_MultiDevice(t *testing.T) {
	reader := &physicsTestReader{
		payloads: map[string]device.DataPayload{
			"dev-1": buildPayload(map[int]float64{
				0: 100, 1: 200, 2: 100, 3: 100, 4: 100,
			}),
			"dev-2": buildPayload(map[int]float64{
				0: 101325, 1: 20, 2: 80, 3: 15,
			}),
		},
	}
	manager := NewCalibrationManager(reader, nil, nil, nil)
	manager.currentConfig = calibration.Config{
		TaskID: "cal-multi",
		Type:   string(calibration.TypeFiveHole),
		ProbeChannels: []calibration.ProbeChannel{
			{Role: "fiveHole.p1", DeviceID: "dev-1", ChannelIndex: 0, Enabled: true},
			{Role: "fiveHole.p2", DeviceID: "dev-1", ChannelIndex: 1, Enabled: true},
			{Role: "fiveHole.p3", DeviceID: "dev-1", ChannelIndex: 2, Enabled: true},
			{Role: "fiveHole.p4", DeviceID: "dev-1", ChannelIndex: 3, Enabled: true},
			{Role: "fiveHole.p5", DeviceID: "dev-1", ChannelIndex: 4, Enabled: true},
			{Role: "fiveHole.pAtm", DeviceID: "dev-2", ChannelIndex: 0, Enabled: true},
			{Role: "fiveHole.tAtm", DeviceID: "dev-2", ChannelIndex: 1, Enabled: true},
			{Role: "fiveHole.pTotal", DeviceID: "dev-2", ChannelIndex: 2, Enabled: true},
			{Role: "fiveHole.pTunnelStatic", DeviceID: "dev-2", ChannelIndex: 3, Enabled: true},
		},
	}
	manager.currentStatus.State = calibration.StateRunning

	status := manager.Status()
	if status.LivePhysics == nil || status.LivePhysics.MachNumber == nil {
		t.Fatalf("多设备 Ma 不应为 nil, status=%+v", status.LivePhysics)
	}
}

// TestCalibrationStatusPhysics_RaceSafety 验证并发 Status() 调用无数据竞争。
//
// 测试前置：五孔配置 + reader 有效数据。
// 测试步骤：10 个 goroutine 各 50 次 Status() 并发调用。
// 期待结果：无 panic，-race 检测无 data race 报告。
func TestCalibrationStatusPhysics_RaceSafety(t *testing.T) {
	reader := &physicsTestReader{
		payloads: map[string]device.DataPayload{
			"dev-1": buildPayload(map[int]float64{
				0: 100, 1: 200, 2: 100, 3: 100, 4: 100,
				5: 101325, 6: 20,
				7: 80, 8: 15,
			}),
		},
	}
	manager := NewCalibrationManager(reader, nil, nil, nil)
	manager.currentConfig = fiveHolePhysicsTestConfig("cal-race")

	const goroutines = 10
	const iterations = 50
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				_ = manager.Status()
			}
		}()
	}
	wg.Wait()
}

// ==================== 辅助构造函数 ====================

func fiveHolePhysicsTestConfig(taskID string) calibration.Config {
	return calibration.Config{
		TaskID: taskID,
		Type:   string(calibration.TypeFiveHole),
		ProbeChannels: []calibration.ProbeChannel{
			{Role: "fiveHole.p1", DeviceID: "dev-1", ChannelIndex: 0, Enabled: true},
			{Role: "fiveHole.p2", DeviceID: "dev-1", ChannelIndex: 1, Enabled: true},
			{Role: "fiveHole.p3", DeviceID: "dev-1", ChannelIndex: 2, Enabled: true},
			{Role: "fiveHole.p4", DeviceID: "dev-1", ChannelIndex: 3, Enabled: true},
			{Role: "fiveHole.p5", DeviceID: "dev-1", ChannelIndex: 4, Enabled: true},
			{Role: "fiveHole.pAtm", DeviceID: "dev-1", ChannelIndex: 5, Enabled: true},
			{Role: "fiveHole.tAtm", DeviceID: "dev-1", ChannelIndex: 6, Enabled: true},
			{Role: "fiveHole.pTotal", DeviceID: "dev-1", ChannelIndex: 7, Enabled: true},
			{Role: "fiveHole.pTunnelStatic", DeviceID: "dev-1", ChannelIndex: 8, Enabled: true},
		},
	}
}

func threeHolePhysicsTestConfig(taskID string) calibration.Config {
	return calibration.Config{
		TaskID: taskID,
		Type:   string(calibration.TypeThreeHole),
		ProbeChannels: []calibration.ProbeChannel{
			{Role: "threeHole.p1", DeviceID: "dev-1", ChannelIndex: 0, Enabled: true},
			{Role: "threeHole.p2", DeviceID: "dev-1", ChannelIndex: 1, Enabled: true},
			{Role: "threeHole.p3", DeviceID: "dev-1", ChannelIndex: 2, Enabled: true},
			{Role: "threeHole.pAtm", DeviceID: "dev-1", ChannelIndex: 3, Enabled: true},
			{Role: "threeHole.tAtm", DeviceID: "dev-1", ChannelIndex: 4, Enabled: true},
			{Role: "threeHole.pTotal", DeviceID: "dev-1", ChannelIndex: 5, Enabled: true},
			{Role: "threeHole.pStatic", DeviceID: "dev-1", ChannelIndex: 6, Enabled: true},
		},
	}
}

func totalPressurePhysicsTestConfig(taskID string) calibration.Config {
	return calibration.Config{
		TaskID: taskID,
		Type:   string(calibration.TypeTotalPressure),
		ProbeChannels: []calibration.ProbeChannel{
			{Role: "totalPressure.pAtm", DeviceID: "dev-1", ChannelIndex: 0, Enabled: true},
			{Role: "totalPressure.tAtm", DeviceID: "dev-1", ChannelIndex: 1, Enabled: true},
			{Role: "totalPressure.pTunnelTotal", DeviceID: "dev-1", ChannelIndex: 2, Enabled: true},
			{Role: "totalPressure.pTunnelStatic", DeviceID: "dev-1", ChannelIndex: 3, Enabled: true},
			{Role: "totalPressure.tTunnel", DeviceID: "dev-1", ChannelIndex: 4, Enabled: true},
			{Role: "totalPressure.pProbeTotal", DeviceID: "dev-1", ChannelIndex: 5, Enabled: true},
		},
	}
}

func sevenHolePhysicsTestConfig(taskID string) calibration.Config {
	return calibration.Config{
		TaskID: taskID,
		Type:   string(calibration.TypeSevenHole),
		ProbeChannels: []calibration.ProbeChannel{
			{Role: "sevenHole.p1", DeviceID: "dev-1", ChannelIndex: 0, Enabled: true},
			{Role: "sevenHole.p2", DeviceID: "dev-1", ChannelIndex: 1, Enabled: true},
			{Role: "sevenHole.p3", DeviceID: "dev-1", ChannelIndex: 2, Enabled: true},
			{Role: "sevenHole.p4", DeviceID: "dev-1", ChannelIndex: 3, Enabled: true},
			{Role: "sevenHole.p5", DeviceID: "dev-1", ChannelIndex: 4, Enabled: true},
			{Role: "sevenHole.p6", DeviceID: "dev-1", ChannelIndex: 5, Enabled: true},
			{Role: "sevenHole.p7", DeviceID: "dev-1", ChannelIndex: 6, Enabled: true},
			{Role: "sevenHole.pAtm", DeviceID: "dev-1", ChannelIndex: 7, Enabled: true},
			{Role: "sevenHole.tAtm", DeviceID: "dev-1", ChannelIndex: 8, Enabled: true},
			{Role: "sevenHole.pTotal", DeviceID: "dev-1", ChannelIndex: 9, Enabled: true},
			{Role: "sevenHole.pTunnelStatic", DeviceID: "dev-1", ChannelIndex: 10, Enabled: true},
		},
	}
}

// sevenHolePhysicsTestConfigWithTTunnel 在 sevenHolePhysicsTestConfig 基础上追加 tTunnel 通道。
// 七孔 SevenHoleRawData 的 TTunnel 是 *float64，配置 + reader 都需要该通道才会赋值。
func sevenHolePhysicsTestConfigWithTTunnel(taskID string) calibration.Config {
	cfg := sevenHolePhysicsTestConfig(taskID)
	cfg.ProbeChannels = append(cfg.ProbeChannels, calibration.ProbeChannel{
		Role: "sevenHole.tTunnel", DeviceID: "dev-1", ChannelIndex: 11, Enabled: true,
	})
	return cfg
}
