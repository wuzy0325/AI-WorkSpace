package driver

import (
	"context"
	"fmt"
	"log"
	"time"
)

// ---------------------------------------------------------------------------
// ConST 811A 打压设备驱动 (标准SCPI)
// ---------------------------------------------------------------------------

type ConST811ADriver struct {
	constBaseDriver
}

func newConST811ADriverWithLocalAddr(host string, port int, localAddr string) *ConST811ADriver {
	return &ConST811ADriver{
		constBaseDriver: constBaseDriver{base: newTCPConnectionDriverWithLocalAddr("ConST 811A", host, port, localAddr)},
	}
}

func (d *ConST811ADriver) Connect(ctx context.Context) error {
	return d.constConnect(ctx, "PRESsure:MODule1:STABle?")
}

func (d *ConST811ADriver) SetTargetPressure(ctx context.Context, target float64) error {
	sendTarget := target
	if target == 0 {
		sendTarget = 0.0001
	}
	cmd := fmt.Sprintf("PRESsure:TARGet %.4f", sendTarget)
	log.Printf("[811A] SetTargetPressure target=%.4f send=%.4f cmd=%q", target, sendTarget, cmd)
	_, err := d.base.sendSCPICommand(ctx, cmd, 3*time.Second)
	if err != nil {
		return fmt.Errorf("set target pressure: %w", err)
	}
	return nil
}

func (d *ConST811ADriver) Stop(ctx context.Context) error {
	_, err := d.base.sendSCPICommand(ctx, "PRESsure:MODE VENT", 3*time.Second)
	if err != nil {
		return fmt.Errorf("stop pressure: %w", err)
	}
	return nil
}

func (d *ConST811ADriver) Exhaust(ctx context.Context) error {
	_, err := d.base.sendSCPICommand(ctx, "PRESsure:MODE VENT", 3*time.Second)
	if err != nil {
		return fmt.Errorf("exhaust: %w", err)
	}
	return nil
}

func (d *ConST811ADriver) ReadCurrentPressure(ctx context.Context) (float64, error) {
	resp, err := d.base.sendSCPICommand(ctx, "PRESsure0?", 3*time.Second)
	if err != nil {
		return 0, fmt.Errorf("read current pressure: %w", err)
	}
	return parseSCPIPressure(resp)
}

func (d *ConST811ADriver) ReadUnit(ctx context.Context) (string, error) {
	resp, err := d.base.sendSCPICommand(ctx, "PRESsure:MODule1:UNIT?", 3*time.Second)
	if err != nil {
		log.Printf("[811A] ReadUnit error: %v", err)
		return "", fmt.Errorf("read unit: %w", err)
	}
	unit := NormalizePressureUnit(parseConST811AUnit(resp))
	log.Printf("[811A] ReadUnit raw=%q parsed=%q", resp, unit)
	return unit, nil
}

func (d *ConST811ADriver) SetUnit(ctx context.Context, unit string) error {
	unitCode, ok := pressureUnitToCode811A(unit)
	if !ok {
		return fmt.Errorf("unsupported pressure unit: %s", unit)
	}
	cmd := fmt.Sprintf("PRESsure:MODule1:UNIT %s", unitCode)
	log.Printf("[811A] SetUnit %q → cmd=%q", unit, cmd)
	_, err := d.base.sendSCPICommand(ctx, cmd, 3*time.Second)
	if err != nil {
		return fmt.Errorf("set unit: %w", err)
	}
	return nil
}

func (d *ConST811ADriver) ReadStability(ctx context.Context) (bool, error) {
	return d.constReadStability(ctx, "PRESsure:MODule1:STABle?")
}

func (d *ConST811ADriver) IsStable(ctx context.Context) (bool, error) {
	return d.ReadStability(ctx)
}

func (d *ConST811ADriver) StartControl(ctx context.Context) error {
	_, err := d.base.sendSCPICommand(ctx, "PRESsure:MODE CONTROL", 3*time.Second)
	if err != nil {
		return fmt.Errorf("start pressure control: %w", err)
	}
	return nil
}
