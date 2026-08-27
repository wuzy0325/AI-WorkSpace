package driver

import (
	"context"
	"fmt"
	"log"
	"math"
	"strings"
	"time"
)

// ---------------------------------------------------------------------------
// ConST 820 打压设备驱动 (简化SCPI)
// ---------------------------------------------------------------------------

type ConST820Driver struct {
	constBaseDriver
}

const const820CommandApplyDelay = 250 * time.Millisecond

func newConST820DriverWithLocalAddr(host string, port int, localAddr string) *ConST820Driver {
	return &ConST820Driver{
		constBaseDriver: constBaseDriver{base: newTCPConnectionDriverWithLocalAddr("ConST 820", host, port, localAddr)},
	}
}

func (d *ConST820Driver) Connect(ctx context.Context) error {
	return d.constConnect(ctx, "OUTPut:PRESsure:STABle?")
}

func (d *ConST820Driver) SetTargetPressure(ctx context.Context, target float64) error {
	cmd := fmt.Sprintf("SOURce:PRESsure %.4f", target)
	log.Printf("[820] SetTargetPressure → cmd=%q", cmd)
	_, err := d.base.sendSCPICommandWithoutTerminator(ctx, cmd)
	if err != nil {
		log.Printf("[820] SetTargetPressure error: %v", err)
		return fmt.Errorf("set target pressure: %w", err)
	}
	if err := waitConST820CommandApplied(ctx); err != nil {
		return err
	}

	// 820 对超量程目标值可能静默拒绝并触发设备报警音，
	// 设置命令本身也没有响应，因此必须读取目标值确认设备真的接受了新值。
	resp, err := d.base.sendSCPICommand(ctx, "SOURce:PRESsure?", 3*time.Second)
	if err != nil {
		return fmt.Errorf("verify target pressure: %w", err)
	}
	actual, err := parseSCPIPressure(resp)
	if err != nil {
		return fmt.Errorf("verify target pressure: %w", err)
	}
	want := math.Round(target*10000) / 10000
	if math.Abs(actual-want) > 0.00005 {
		return fmt.Errorf("device rejected target pressure %.4f (actual target: %.4f)", want, actual)
	}
	return nil
}

func (d *ConST820Driver) Stop(ctx context.Context) error {
	cmd := "OUTPut:PRESsure:MODE VENT"
	log.Printf("[820] Stop → cmd=%q", cmd)
	_, err := d.base.sendSCPICommandWithoutTerminator(ctx, cmd)
	if err != nil {
		log.Printf("[820] Stop error: %v", err)
		return fmt.Errorf("stop pressure: %w", err)
	}
	return nil
}

func (d *ConST820Driver) Exhaust(ctx context.Context) error {
	cmd := "OUTPut:PRESsure:MODE VENT"
	log.Printf("[820] Exhaust → cmd=%q", cmd)
	_, err := d.base.sendSCPICommandWithoutTerminator(ctx, cmd)
	if err != nil {
		log.Printf("[820] Exhaust error: %v", err)
		return fmt.Errorf("exhaust: %w", err)
	}
	return nil
}

func (d *ConST820Driver) ReadCurrentPressure(ctx context.Context) (float64, error) {
	resp, err := d.base.sendSCPICommand(ctx, "MEASure:SCALar:PRESsure1?", 3*time.Second)
	if err != nil {
		log.Printf("[820] ReadCurrentPressure error: %v", err)
		return 0, fmt.Errorf("read current pressure: %w", err)
	}
	return parseSCPIPressure(resp)
}

func (d *ConST820Driver) ReadUnit(ctx context.Context) (string, error) {
	resp, err := d.base.sendSCPICommand(ctx, "UNIT:PRESsure?", 3*time.Second)
	if err != nil {
		log.Printf("[820] ReadUnit error: %v", err)
		return "", fmt.Errorf("read unit: %w", err)
	}
	unit := NormalizePressureUnit(parseConST820Unit(resp))
	log.Printf("[820] ReadUnit raw=%q parsed=%q", resp, unit)
	return unit, nil
}

func (d *ConST820Driver) SetUnit(ctx context.Context, unit string) error {
	unitCode, ok := pressureUnitToCode820(unit)
	if !ok {
		return fmt.Errorf("unsupported pressure unit: %s", unit)
	}
	want := NormalizePressureUnit(unit)
	cmd := fmt.Sprintf("UNIT:PRESsure %s", unitCode)
	log.Printf("[820] SetUnit %q → cmd=%q", unit, cmd)
	_, err := d.base.sendSCPICommandWithoutTerminator(ctx, cmd)
	if err != nil {
		return fmt.Errorf("set unit: %w", err)
	}
	if err := waitConST820CommandApplied(ctx); err != nil {
		return err
	}

	// 设置命令本身没有可靠的成功响应，必须以设备回读结果为准。
	got, err := d.ReadUnit(ctx)
	if err != nil {
		return fmt.Errorf("verify unit after set: %w", err)
	}
	if !strings.EqualFold(got, want) {
		return fmt.Errorf("device rejected unit %s (actual unit: %s)", want, got)
	}
	return nil
}

func waitConST820CommandApplied(ctx context.Context) error {
	timer := time.NewTimer(const820CommandApplyDelay)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("wait for 820 command: %w", ctx.Err())
	}
}

func (d *ConST820Driver) ReadStability(ctx context.Context) (bool, error) {
	return d.constReadStability(ctx, "OUTPut:PRESsure:STABle?")
}

func (d *ConST820Driver) IsStable(ctx context.Context) (bool, error) {
	return d.ReadStability(ctx)
}
