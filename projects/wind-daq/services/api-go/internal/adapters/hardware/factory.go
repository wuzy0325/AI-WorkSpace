package hardware

import (
	"fmt"

	"wind-daq/services/api-go/internal/core/device"
	"wind-daq/services/api-go/internal/core/motion"
	"wind-daq/services/api-go/internal/ports"
)

// ==================== 设备驱动工厂 ====================
// 根据设备配置/运动控制器配置创建对应的驱动实例
// 工厂模式:根据类型自动选择正确的驱动实现

// CreateDevice 根据设备配置创建设备驱动实例
// 参数: config 设备配置(包含类型、连接参数等)
// 返回: (ports.Device, error) 设备驱动接口,错误信息
func CreateDevice(config device.DeviceConfig) (ports.Device, error) {
	switch config.Type {
	case device.DeviceSimulated:
		return NewSimulatedDevice(config), nil
	case device.DeviceDAQP1604:
		return NewDAQP1604Device(config), nil
	case device.DeviceDAQT1603:
		return NewDAQT1603Device(config), nil
	case device.DeviceWTNPXI:
		return nil, fmt.Errorf("WTN_PXI driver not yet implemented")
	default:
		return nil, fmt.Errorf("unknown device type: %s", config.Type)
	}
}

// CreateMotionController 根据配置创建运动控制器驱动
// 参数: profile 运动控制器配置
// 返回: ports.MotionController 运动控制器接口
func CreateMotionController(profile motion.MotionControllerProfile) ports.MotionController {
	switch profile.Type {
	case motion.MCB140, motion.MCWTNMC4A:
		// TODO: implement real B140 and WTNMC4A drivers
		return NewSimulatedMotionController(profile)
	default:
		return NewSimulatedMotionController(profile)
	}
}
