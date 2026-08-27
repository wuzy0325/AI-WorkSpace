package driver

import (
	"fmt"
	"strings"

	"cal1604/internal/device"
	"cal1604/internal/domain"
)

// Factory 按设备型号创建对应连接驱动（Adapter + Factory）。
type Factory struct{}

// NewFactory 创建连接驱动工厂。
func NewFactory() *Factory {
	return &Factory{}
}

// Create 根据设备模型返回连接驱动实例。
func (f *Factory) Create(dev domain.Device) (device.ConnectionDriver, error) {
	if dev.IsSimulated {
		switch dev.Type {
		case domain.DeviceTypeMeasure:
			return NewSimulatedMeasureDriver(), nil
		case domain.DeviceTypePressure:
			return NewSimulatedPressureDriver(), nil
		default:
			return nil, fmt.Errorf("unsupported simulated device type: %s", dev.Type)
		}
	}
	model := normalizeModel(dev.Model)
	switch model {
	case "WTN1604":
		return newWTN1604DriverWithLocalAddr(dev.Host, dev.Port, dev.LocalAddr), nil
	case "DAQ-P-1603", "P1603":
		return NewP1603Driver(dev), nil
	case "CONST811A", "811A":
		return newConST811ADriverWithLocalAddr(dev.Host, dev.Port, dev.LocalAddr), nil
	case "CONST820", "820":
		return newConST820DriverWithLocalAddr(dev.Host, dev.Port, dev.LocalAddr), nil
	case "CONST860", "860":
		return newConST860DriverWithLocalAddr(dev.Host, dev.Port, dev.LocalAddr), nil
	case "SPC4000":
		return newSPC4000DriverWithLocalAddr(dev.Host, dev.Port, dev.LocalAddr), nil
	case "SIMULATED", "GENERIC_SIMULATOR", "SIMULATOR":
		switch dev.Type {
		case domain.DeviceTypeMeasure:
			return NewSimulatedMeasureDriver(), nil
		case domain.DeviceTypePressure:
			return NewSimulatedPressureDriver(), nil
		default:
			return nil, fmt.Errorf("unsupported simulated device type: %s", dev.Type)
		}
	default:
		return nil, fmt.Errorf("unsupported device model: %s", dev.Model)
	}
}

// CreateMeasureDriver 根据设备模型返回计量设备驱动。
func (f *Factory) CreateMeasureDriver(dev domain.Device) (device.MeasureDriver, error) {
	if dev.IsSimulated {
		return NewSimulatedMeasureDriver(), nil
	}
	model := normalizeModel(dev.Model)
	switch model {
	case "WTN1604":
		return newWTN1604DriverWithLocalAddr(dev.Host, dev.Port, dev.LocalAddr), nil
	case "DAQ-P-1603", "P1603":
		return NewP1603Driver(dev), nil
	case "SIMULATED", "GENERIC_SIMULATOR", "SIMULATOR":
		return NewSimulatedMeasureDriver(), nil
	default:
		return nil, fmt.Errorf("unsupported measure device model: %s", dev.Model)
	}
}

// CreatePressureDriver 根据设备模型返回打压设备驱动。
func (f *Factory) CreatePressureDriver(dev domain.Device) (device.PressureDriver, error) {
	if dev.IsSimulated {
		return NewSimulatedPressureDriver(), nil
	}
	model := normalizeModel(dev.Model)
	switch model {
	case "CONST811A", "811A":
		return newConST811ADriverWithLocalAddr(dev.Host, dev.Port, dev.LocalAddr), nil
	case "CONST820", "820":
		return newConST820DriverWithLocalAddr(dev.Host, dev.Port, dev.LocalAddr), nil
	case "CONST860", "860":
		return newConST860DriverWithLocalAddr(dev.Host, dev.Port, dev.LocalAddr), nil
	case "SPC4000":
		return newSPC4000DriverWithLocalAddr(dev.Host, dev.Port, dev.LocalAddr), nil
	case "SIMULATED", "GENERIC_SIMULATOR", "SIMULATOR":
		return NewSimulatedPressureDriver(), nil
	default:
		return nil, fmt.Errorf("unsupported pressure device model: %s", dev.Model)
	}
}

func normalizeModel(raw string) string {
	trimmed := strings.TrimSpace(raw)
	upper := strings.ToUpper(trimmed)
	return strings.ReplaceAll(upper, " ", "")
}
