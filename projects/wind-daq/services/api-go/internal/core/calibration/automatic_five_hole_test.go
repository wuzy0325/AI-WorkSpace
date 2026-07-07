package calibration

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
)

type fakeCalibrationRuntime struct {
	values    map[string]float64
	moves     []string
	stopCalls int
}

func TestFiveHoleValidateConfigRequiresCursorDAQReferenceChannels(t *testing.T) {
	config := completeFiveHoleConfig()
	config.ProbeChannels = config.ProbeChannels[:6]

	err := NewFiveHoleAlgorithm().ValidateConfig(config)
	if err == nil {
		t.Fatal("expected missing channel roles to fail validation")
	}
	if !strings.Contains(err.Error(), "fiveHole.tAtm") || !strings.Contains(err.Error(), "fiveHole.pTotal") || !strings.Contains(err.Error(), "fiveHole.pTunnelStatic") {
		t.Fatalf("expected tAtm, pTotal and pTunnelStatic in error, got %v", err)
	}
}

func TestReadProbeChannelsToFiveHoleRawRequiresReferencePressures(t *testing.T) {
	channels := completeFiveHoleProbeChannels()[:7]
	reader := func(deviceID string, channelIndex int) (float64, bool) {
		v, ok := completeFiveHoleValues()[fmt.Sprintf("%s:%d", deviceID, channelIndex)]
		return v, ok
	}

	_, err := ReadProbeChannelsToFiveHoleRaw(channels, reader)
	if err == nil {
		t.Fatal("expected missing reference pressure channels to fail")
	}
	if !strings.Contains(err.Error(), "pTotal") || !strings.Contains(err.Error(), "pStatic") {
		t.Fatalf("expected pTotal and pStatic in error, got %v", err)
	}
}

func TestAutomaticFiveHoleCalibrationMovesAlphaBeforeBeta(t *testing.T) {
	config := completeFiveHoleConfig()
	config.MotionAxes = []MotionAxisConfig{
		{Name: "β", ControllerID: "motion-1", Axis: "Y"},
		{Name: "α", ControllerID: "motion-1", Axis: "X"},
	}
	config.Points = []CalPoint{{ID: 1, Coordinates: map[string]float64{"β": 2, "α": 1}}}
	runtime := &fakeCalibrationRuntime{values: completeFiveHoleValues()}

	engine := NewAutomaticCalibration(config, nil, runtime, nil)
	if err := engine.Start(NewFiveHoleAlgorithm()); err != nil {
		t.Fatalf("start calibration: %v", err)
	}

	expected := []string{"α=1", "β=2"}
	if !reflect.DeepEqual(runtime.moves, expected) {
		t.Fatalf("expected move order %v, got %v", expected, runtime.moves)
	}
}

func TestFiveHoleCsvSchemaMatchesCursorDAQColumns(t *testing.T) {
	schema := NewCsvSchema(completeFiveHoleConfig())
	header := schema.BuildHeader()
	expectedPrefix := []string{
		"点位编号", "α(°)", "β(°)", "P1(Pa)", "P2(Pa)", "P3(Pa)", "P4(Pa)", "P5(Pa)",
		"P∞(Pa)", "T∞(°C)", "Pt(Pa)", "Ps(Pa)", "Ma", "Kα", "Kβ", "CPT", "CPS", "采样次数", "标准差",
	}
	if len(header) != len(expectedPrefix)+16 {
		t.Fatalf("expected %d columns, got %d: %v", len(expectedPrefix)+16, len(header), header)
	}
	if !reflect.DeepEqual(header[:len(expectedPrefix)], expectedPrefix) {
		t.Fatalf("unexpected header prefix: %v", header[:len(expectedPrefix)])
	}
	if header[len(header)-16] != "CH1(Pa)" || header[len(header)-1] != "CH16(Pa)" {
		t.Fatalf("expected CH1..CH16 suffix, got %v", header[len(header)-16:])
	}
}

func TestFiveHoleCsvRecordIncludesCoordinatesAndScanValveChannels(t *testing.T) {
	pTotal := 80.0
	pStatic := 15.0
	mach := 0.2
	dp := &FiveHoleDataPoint{
		PointID:     3,
		Coordinates: map[string]float64{"α": 1.5, "β": -2.5},
		RawData: FiveHoleRawData{
			P1: 10, P2: 20, P3: 30, P4: 40, P5: 50, PAtm: 101325, TAtm: 25, PTotal: &pTotal, PStatic: &pStatic,
		},
		Coefficients:      FiveHoleCoefficients{Kalpha: 0.1, Kbeta: -0.2, CPT: 0.3, CPS: 0.4, MachNumber: &mach},
		SampleCount:       5,
		StdDev:            0.01,
		RawDeviceChannels: map[string][]float64{"dev-1": {1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}},
	}

	record := NewCsvSchema(completeFiveHoleConfig()).BuildRecord(dp)
	if record[0] != "3" || record[1] != "1.5" || record[2] != "-2.5" {
		t.Fatalf("expected point and coordinates in first columns, got %v", record[:3])
	}
	if record[len(record)-16] != "1.000" || record[len(record)-1] != "16.000" {
		t.Fatalf("expected scan valve channels in suffix, got %v", record[len(record)-16:])
	}
}

func (f *fakeCalibrationRuntime) GetChannelValue(deviceID string, channelIndex int) (float64, bool) {
	v, ok := f.values[fmt.Sprintf("%s:%d", deviceID, channelIndex)]
	return v, ok
}

func (f *fakeCalibrationRuntime) MoveToPosition(axis MotionAxisConfig, position float64) error {
	f.moves = append(f.moves, fmt.Sprintf("%s=%g", axis.Name, position))
	return nil
}

func (f *fakeCalibrationRuntime) WaitForMotionComplete() error { return nil }

func (f *fakeCalibrationRuntime) StopMotion() error {
	f.stopCalls++
	return nil
}

func completeFiveHoleProbeChannels() []ProbeChannel {
	return []ProbeChannel{
		{Role: "fiveHole.p1", Name: "P1", DeviceID: "dev-1", ChannelIndex: 1, Enabled: true},
		{Role: "fiveHole.p2", Name: "P2", DeviceID: "dev-1", ChannelIndex: 2, Enabled: true},
		{Role: "fiveHole.p3", Name: "P3", DeviceID: "dev-1", ChannelIndex: 3, Enabled: true},
		{Role: "fiveHole.p4", Name: "P4", DeviceID: "dev-1", ChannelIndex: 4, Enabled: true},
		{Role: "fiveHole.p5", Name: "P5", DeviceID: "dev-1", ChannelIndex: 5, Enabled: true},
		{Role: "fiveHole.pAtm", Name: "Patm", DeviceID: "dev-1", ChannelIndex: 16, Enabled: true},
		{Role: "fiveHole.tAtm", Name: "Tatm", DeviceID: "dev-1", ChannelIndex: 17, Enabled: true},
		{Role: "fiveHole.pTotal", Name: "Pt", DeviceID: "dev-1", ChannelIndex: 18, Enabled: true},
		{Role: "fiveHole.pTunnelStatic", Name: "Ps", DeviceID: "dev-1", ChannelIndex: 19, Enabled: true},
	}
}

func completeFiveHoleValues() map[string]float64 {
	return map[string]float64{
		"dev-1:1": 10, "dev-1:2": 20, "dev-1:3": 30, "dev-1:4": 40, "dev-1:5": 50,
		"dev-1:16": 101325, "dev-1:17": 25, "dev-1:18": 80, "dev-1:19": 15,
	}
}

func completeFiveHoleConfig() Config {
	return Config{
		TaskID:          "cal-1",
		Type:            string(TypeFiveHole),
		SamplesPerPoint: 1,
		ProbeChannels:   completeFiveHoleProbeChannels(),
		Points:          []CalPoint{{ID: 1, Coordinates: map[string]float64{"α": 0, "β": 0}}},
	}
}

// 注意：原 TestProbeChannelUnmarshalNestedFrontendShape 已迁移到
// adapters/config/calibration_config_decoder_test.go（TestDecodeCalibrationConfig_NestedFrontendShape），
// 因为 core 层不再做字节级 I/O，解码逻辑由 adapters/config 层的 DecodeCalibrationConfig 负责。

func TestAutomaticFiveHoleCalibrationUsesConfiguredProbeChannels(t *testing.T) {
	config := completeFiveHoleConfig()
	runtime := &fakeCalibrationRuntime{values: completeFiveHoleValues()}

	engine := NewAutomaticCalibration(config, nil, runtime, nil)
	if err := engine.Start(NewFiveHoleAlgorithm()); err != nil {
		t.Fatalf("start calibration: %v", err)
	}

	points := engine.GetDataPoints()
	if len(points) != 1 {
		t.Fatalf("expected 1 point, got %d", len(points))
	}
	fiveHolePoint, ok := points[0].(*FiveHoleDataPoint)
	if !ok {
		t.Fatalf("expected FiveHoleDataPoint, got %T", points[0])
	}
	if fiveHolePoint.RawData.P1 != 10 || fiveHolePoint.RawData.P5 != 50 || fiveHolePoint.RawData.PAtm != 101325 {
		t.Fatalf("unexpected raw data: %+v", fiveHolePoint.RawData)
	}
}

func TestAutomaticCalibrationInvokesOnDataPointForEachPoint(t *testing.T) {
	config := completeFiveHoleConfig()
	config.TaskID = "cal-realtime"
	config.Points = []CalPoint{
		{ID: 1, Coordinates: map[string]float64{"α": 0, "β": 0}},
		{ID: 2, Coordinates: map[string]float64{"α": 5, "β": 0}},
	}
	runtime := &fakeCalibrationRuntime{values: completeFiveHoleValues()}

	var received []DataPoint
	sink := func(dp DataPoint) { received = append(received, dp) }
	engine := NewAutomaticCalibration(config, nil, runtime, sink)
	if err := engine.Start(NewFiveHoleAlgorithm()); err != nil {
		t.Fatalf("start calibration: %v", err)
	}
	if len(received) != 2 {
		t.Fatalf("expected onDataPoint called 2 times, got %d", len(received))
	}
}
