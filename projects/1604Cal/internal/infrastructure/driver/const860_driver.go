package driver

import (
	"context"
	"fmt"
	"log"
	"time"
)

// ---------------------------------------------------------------------------
// ConST 860 打压设备驱动 (标准SCPI，部分简化)
// ---------------------------------------------------------------------------

type ConST860Driver struct {
	constBaseDriver
}

func newConST860DriverWithLocalAddr(host string, port int, localAddr string) *ConST860Driver {
	return &ConST860Driver{
		constBaseDriver: constBaseDriver{base: newTCPConnectionDriverWithLocalAddr("ConST 860", host, port, localAddr)},
	}
}

func (d *ConST860Driver) Connect(ctx context.Context) error {
	return d.constConnect(ctx, "PRESsure:STABle?")
}

func (d *ConST860Driver) SetTargetPressure(ctx context.Context, target float64) error {
	cmd := fmt.Sprintf("PRESsure:TARGet %.4f", target)
	_, err := d.base.sendSCPICommand(ctx, cmd, 3*time.Second)
	if err != nil {
		return fmt.Errorf("set target pressure: %w", err)
	}
	return nil
}

func (d *ConST860Driver) Stop(ctx context.Context) error {
	_, err := d.base.sendSCPICommand(ctx, "PRESsure:MODule:CONTrol VENT", 3*time.Second)
	if err != nil {
		return fmt.Errorf("stop pressure: %w", err)
	}
	return nil
}

func (d *ConST860Driver) Exhaust(ctx context.Context) error {
	_, err := d.base.sendSCPICommand(ctx, "PRESsure:MODule:CONTrol VENT", 3*time.Second)
	if err != nil {
		return fmt.Errorf("exhaust: %w", err)
	}
	return nil
}

func (d *ConST860Driver) ReadCurrentPressure(ctx context.Context) (float64, error) {
	resp, err := d.base.sendSCPICommand(ctx, "PRESsure?", 3*time.Second)
	if err != nil {
		return 0, fmt.Errorf("read current pressure: %w", err)
	}
	return parseSCPIPressure(resp)
}

func (d *ConST860Driver) ReadUnit(ctx context.Context) (string, error) {
	resp, err := d.base.sendSCPICommand(ctx, "PRESsure:MODule:UNIT? 1", 3*time.Second)
	if err != nil {
		log.Printf("[860] ReadUnit error: %v", err)
		return "", fmt.Errorf("read unit: %w", err)
	}
	unit := NormalizePressureUnit(parseConSTGeneralUnit(resp))
	log.Printf("[860] ReadUnit raw=%q parsed=%q", resp, unit)
	return unit, nil
}

func (d *ConST860Driver) SetUnit(ctx context.Context, unit string) error {
	normalized := NormalizePressureUnit(unit)
	cmd := fmt.Sprintf("PRESsure:MODule:UNIT 1,%s", normalized)
	log.Printf("[860] SetUnit %q → cmd=%q", unit, cmd)
	_, err := d.base.sendSCPICommand(ctx, cmd, 3*time.Second)
	if err != nil {
		return fmt.Errorf("set unit: %w", err)
	}
	return nil
}

func (d *ConST860Driver) ReadStability(ctx context.Context) (bool, error) {
	return d.constReadStability(ctx, "PRESsure:STABle?")
}

func (d *ConST860Driver) IsStable(ctx context.Context) (bool, error) {
	return d.ReadStability(ctx)
}

func (d *ConST860Driver) StartControl(ctx context.Context) error {
	_, err := d.base.sendSCPICommand(ctx, "PRESsure:MODule:CONTrol CONTROL", 3*time.Second)
	if err != nil {
		return fmt.Errorf("start pressure control: %w", err)
	}
	return nil
}


