package ports

import "wind-daq/services/api-go/internal/core/motion"

// ==================== 运动控制器接口 ====================
// 定义多轴运动控制器的通用操作
// 适配不同品牌的运动控制器(如B140、WTNMC4A等)

// MotionController 运动控制器抽象接口
// 支持多轴独立/同步控制、点位运动、回零、寸动等操作
type MotionController interface {
	// ----------- 生命周期 -----------
	Connect() error    // 连接控制器
	Disconnect() error // 断开连接
	IsConnected() bool // 检查连接状态

	// ----------- 运动控制 -----------
	MoveTo(axis motion.AxisName, position float64) error             // 绝对运动到指定位置
	MoveBy(axis motion.AxisName, delta float64) error                // 相对运动(增量)
	Jog(axis motion.AxisName, direction string, speed float64) error // 寸动(连续运动)
	Home(axis motion.AxisName) error                                 // 回零(寻找原点)
	Stop(axis motion.AxisName) error                                 // 停止指定轴
	EmergencyStop() error                                            // 急停(所有轴)

	// ----------- 位置设置 -----------
	// 定义当前位置（用于重新初始化或校准）
	DefinePosition(axis motion.AxisName, position float64) error

	// ----------- 状态查询 -----------
	GetAxisStatus(axis motion.AxisName) (motion.AxisStatus, error) // 获取单轴状态
	GetAllAxisStatus() ([]motion.AxisStatus, error)                // 获取全部轴状态

	// ----------- 配置管理 -----------
	Profile() motion.MotionControllerProfile              // 获取控制器配置
	UpdateProfile(profile motion.MotionControllerProfile) // 更新控制器配置
}
