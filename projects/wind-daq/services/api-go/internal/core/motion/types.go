package motion

// ==================== 运动控制数据类型定义 ====================
// 定义多轴运动控制系统的数据类型
// 包括:轴名称、轴类型、配置、状态等

// AxisName 轴名称枚举
// 支持4个坐标轴(X/Y/Z/U)
type AxisName string

const (
	AxisX AxisName = "X" // X轴(横向)
	AxisY AxisName = "Y" // Y轴(纵向)
	AxisZ AxisName = "Z" // Z轴(垂直)
	AxisU AxisName = "U" // U轴(旋转/第四轴)
)

// AxisKind 轴类型
type AxisKind string

const (
	AxisKindLinear AxisKind = "LINEAR" // 线性轴(直线运动)
	AxisKindRotary AxisKind = "ROTARY" // 旋转轴(旋转运动)
)

// PositionSource 位置来源
// 当前位置值的获取来源
type PositionSource string

const (
	PosSourceRegister PositionSource = "register" // 寄存器(软件记录的位置)
	PosSourceEncoder  PositionSource = "encoder"  // 编码器(实际传感器读数)
)

// AxisEncoderCompensationConfig 编码器补偿参数
// 用于提高位置精度,补偿机械误差
type AxisEncoderCompensationConfig struct {
	Enabled   bool     `json:"enabled"`             // 是否启用补偿
	Tolerance *float64 `json:"tolerance,omitempty"` // 容差值
	MaxCycles *int     `json:"maxCycles,omitempty"` // 最大补偿循环次数
	SettleMs  *float64 `json:"settleMs,omitempty"`  // 稳定时间(毫秒)
	MinStep   *float64 `json:"minStep,omitempty"`   // 最小补偿步长
	TimeoutMs *float64 `json:"timeoutMs,omitempty"` // 超时时间(毫秒)
}

// AxisConfig 轴配置参数
// 定义单个轴的机械参数和运动参数
type AxisConfig struct {
	Name                AxisName                       `json:"name"`                          // 轴名称
	Enabled             bool                           `json:"enabled"`                       // 是否启用
	Kind                AxisKind                       `json:"kind"`                          // 轴类型(线性/旋转)
	StepsPerRev         *float64                       `json:"stepsPerRev,omitempty"`         // 每转脉冲数
	MicroSteps          *float64                       `json:"microSteps,omitempty"`          // 细分步数
	Lead                *float64                       `json:"lead,omitempty"`                // 导程(螺距)
	GearRatio           *float64                       `json:"gearRatio,omitempty"`           // 传动比(仅旋转轴使用)
	MaxSpeed            *float64                       `json:"maxSpeed,omitempty"`            // 最大速度
	MinLimit            *float64                       `json:"minLimit,omitempty"`            // 软限位下限
	MaxLimit            *float64                       `json:"maxLimit,omitempty"`            // 软限位上限
	Inverted            bool                           `json:"inverted"`                      // 方向反转
	EncoderInverted     bool                           `json:"encoderInverted"`               // 编码器方向反转
	PositionSource      PositionSource                 `json:"positionSource,omitempty"`      // 位置来源
	EncoderScale        *float64                       `json:"encoderScale,omitempty"`        // 编码器比例
	EncoderCompensation *AxisEncoderCompensationConfig `json:"encoderCompensation,omitempty"` // 补偿配置
}

// AxisStatus 轴运行时状态
// 包含轴的当前位置、运动状态、限位状态等
type AxisStatus struct {
	Name              AxisName `json:"name"`                        // 轴名称
	Position          float64  `json:"position"`                    // 当前位置
	Moving            bool     `json:"moving"`                      // 是否运动中
	Homed             bool     `json:"homed"`                       // 是否已回零
	PosLimit          bool     `json:"posLimit,omitempty"`          // 正限位触发
	NegLimit          bool     `json:"negLimit,omitempty"`          // 负限位触发
	Compensating      bool     `json:"compensating,omitempty"`      // 补偿中
	CompensationError string   `json:"compensationError,omitempty"` // 补偿错误信息
	PositionError     *float64 `json:"positionError,omitempty"`     // 位置误差
}

// MotionControllerType 运动控制器类型
type MotionControllerType string

const (
	MCSimulated MotionControllerType = "SIMULATED-MC" // 模拟控制器(测试用)
	MCB140      MotionControllerType = "B140-MC"      // B140控制器
	MCWTNMC4A   MotionControllerType = "WTNMC4A-MC"   // WTNMC4A控制器
)

// MotionControllerProfile 运动控制器配置模板
type MotionControllerProfile struct {
	ID          string               `json:"id"`                // 配置ID
	Name        string               `json:"name"`              // 配置名称
	Type        MotionControllerType `json:"type"`              // 控制器类型
	Address     string               `json:"address,omitempty"` // IP地址
	Port        int                  `json:"port,omitempty"`    // 端口号
	AutoConnect bool                 `json:"autoConnect"`       // 启动时自动连接
	Axes        []AxisConfig         `json:"axes"`              // 轴配置列表
}

// MotionControllerStatus 运动控制器运行时状态
type MotionControllerStatus struct {
	ID        string               `json:"id"`                  // 控制器ID
	Name      string               `json:"name"`                // 名称
	Type      MotionControllerType `json:"type"`                // 类型
	Connected bool                 `json:"connected"`           // 连接状态
	Axes      []AxisStatus         `json:"axes"`                // 各轴状态
	LastError string               `json:"lastError,omitempty"` // 最后错误
}

// MotionInstance 运行时实例(配置+状态)
type MotionInstance struct {
	Profile MotionControllerProfile // 配置
	Status  MotionControllerStatus  // 状态
}
