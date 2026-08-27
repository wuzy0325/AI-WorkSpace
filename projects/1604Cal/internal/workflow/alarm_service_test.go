package workflow_test

import (
	"testing"

	"cal1604/internal/domain"
	"cal1604/internal/workflow"
)

func TestAlarmDecisionAllowsSupportedActions(t *testing.T) {
	svc := workflow.NewAlarmService()

	if err := svc.ValidateDecision("continue"); err != nil {
		t.Fatalf("continue should be valid, got %v", err)
	}

	if err := svc.ValidateDecision("skip"); err != nil {
		t.Fatalf("skip should be valid, got %v", err)
	}

	if err := svc.ValidateDecision("recollect"); err != nil {
		t.Fatalf("recollect should be valid, got %v", err)
	}

	if err := svc.ValidateDecision("stop"); err != nil {
		t.Fatalf("stop should be valid, got %v", err)
	}

	if err := svc.ValidateDecision("retry"); err == nil {
		t.Fatal("expected invalid decision to fail")
	}
}

func TestAlarmEvaluateDeviation(t *testing.T) {
	svc := workflow.NewAlarmService()

	result := svc.Evaluate(50, 52.35, 0.5)
	if !result.Triggered {
		t.Fatal("expected alarm to be triggered")
	}

	if result.DeviationPercent <= 0.5 {
		t.Fatalf("expected deviation > 0.5%%, got %.3f", result.DeviationPercent)
	}
}

func TestEvaluateMultiChannelSpanFormula(t *testing.T) {
	svc := workflow.NewAlarmService()

	cfg := domain.AlarmConfig{
		Enabled:            true,
		PrecisionThreshold: 0.001, // 0.1%
		EnabledChannels:    []int{1, 2},
	}

	// 量程 1 MPa (min=9.5, max=10.5)，precisionThreshold=0.001 → allowance=0.001
	// 通道偏差 0.0005 未超限，不触发报警
	channelData := map[int]float64{1: 10.0005, 2: 10.0003}
	result := svc.EvaluateMultiChannel(cfg, 10.0, 10.5, 9.5, channelData)
	if result.Triggered {
		t.Fatalf("expected no alarm, deviation 0.0005 < allowance 0.001")
	}

	// 通道偏差 0.002 超限，触发报警
	channelData = map[int]float64{1: 10.002, 2: 10.0}
	result = svc.EvaluateMultiChannel(cfg, 10.0, 10.5, 9.5, channelData)
	if !result.Triggered {
		t.Fatal("expected alarm, deviation 0.002 > allowance 0.001")
	}
	if len(result.OverLimitChannels) != 1 || result.OverLimitChannels[0] != 1 {
		t.Fatalf("expected channel 1 over limit, got %v", result.OverLimitChannels)
	}
}

func TestEvaluateMultiChannelZeroSpanFallback(t *testing.T) {
	svc := workflow.NewAlarmService()

	cfg := domain.AlarmConfig{
		Enabled:            true,
		PrecisionThreshold: 0.001,
		EnabledChannels:    []int{1},
	}

	// 量程为 0（maxPressure == minPressure），降级使用 |target| × precisionThreshold
	// target=100, allowance=0.1, 通道值 100.05 未超限
	channelData := map[int]float64{1: 100.05}
	result := svc.EvaluateMultiChannel(cfg, 100.0, 100.0, 100.0, channelData)
	if result.Triggered {
		t.Fatalf("expected no alarm with zero span fallback, deviation 0.05 < allowance 0.1")
	}

	// 通道值 100.15 超限
	channelData = map[int]float64{1: 100.15}
	result = svc.EvaluateMultiChannel(cfg, 100.0, 100.0, 100.0, channelData)
	if !result.Triggered {
		t.Fatal("expected alarm with zero span fallback, deviation 0.15 > allowance 0.1")
	}
}
