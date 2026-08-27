package session

import (
	"context"
	"fmt"
	"log"
	"time"

	"cal1604/internal/device"
)

// DriverResolver 提供统一的驱动解析逻辑：优先复用已连接驱动，否则从工厂创建新实例。
type DriverResolver struct {
	DeviceManager  device.DeviceStore
	DriverProvider device.ActiveDriverProvider
	Factory        device.DriverFactory
}

// ResolveMeasureDriver 解析计量设备驱动。
// 优先复用已连接的活跃驱动；若不存在则创建新驱动并建立 TCP 连接。
func (r *DriverResolver) ResolveMeasureDriver(measureDevID string) (device.MeasureDriver, error) {
	if r.DriverProvider != nil {
		if drv := r.DriverProvider.GetActiveDriver(measureDevID); drv != nil {
			if mDrv, ok := drv.(device.MeasureDriver); ok {
				return mDrv, nil
			}
		}
	}

	measureDev, ok := r.DeviceManager.Get(measureDevID)
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrDeviceNotFound, measureDevID)
	}

	// 无活跃驱动（设备重启后状态残留等场景），需创建新驱动并连接 TCP
	mDrv, err := r.Factory.CreateMeasureDriver(measureDev)
	if err != nil {
		return nil, fmt.Errorf("create measure driver: %w", err)
	}

	connectCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := mDrv.Connect(connectCtx); err != nil {
		return nil, fmt.Errorf("connect measure driver %s: %w", measureDevID, err)
	}

	return mDrv, nil
}

// ResolvePressureDriver 解析打压设备驱动。
// 优先复用已连接的活跃驱动；若不存在则创建新驱动并建立 TCP 连接。
func (r *DriverResolver) ResolvePressureDriver(pressureDevID string) (device.PressureDriver, error) {
	if r.DriverProvider != nil {
		if drv := r.DriverProvider.GetActiveDriver(pressureDevID); drv != nil {
			if pDrv, ok := drv.(device.PressureDriver); ok {
				log.Printf("[session] reuse active pressure driver id=%s type=%T", pressureDevID, pDrv)
				return pDrv, nil
			}
		}
	}

	pressureDev, ok := r.DeviceManager.Get(pressureDevID)
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrDeviceNotFound, pressureDevID)
	}
	pDrv, err := r.Factory.CreatePressureDriver(pressureDev)
	if err != nil {
		return nil, err
	}

	// 无活跃驱动时，连接新建的驱动
	connectCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := pDrv.Connect(connectCtx); err != nil {
		return nil, fmt.Errorf("connect pressure driver %s: %w", pressureDevID, err)
	}

	log.Printf("[session] create and connect pressure driver id=%s type=%T", pressureDevID, pDrv)
	return pDrv, nil
}
