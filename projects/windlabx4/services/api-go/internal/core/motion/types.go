package motion

type AxisName string

const (
	AxisX AxisName = "X"
	AxisY AxisName = "Y"
	AxisZ AxisName = "Z"
	AxisU AxisName = "U"
)

type AxisKind string

const (
	AxisKindLinear AxisKind = "LINEAR"
	AxisKindRotary AxisKind = "ROTARY"
)

type PositionSource string

const (
	PositionSourceRegister PositionSource = "register"
	PositionSourceEncoder  PositionSource = "encoder"
)

type ControllerType string

const (
	ControllerTypeSimulated ControllerType = "SIMULATED-MC"
	ControllerTypeB140      ControllerType = "B140-MC"
	ControllerTypeWTNMC4A   ControllerType = "WTNMC4A-MC"
)

// AxisEncoderCompensationConfig 编码器补偿参数。
// 字段类型与 shared.local/device-sdk/go/motion/core 对齐，
// 避免 wrapper.go 中冗余的类型转换函数。
type AxisEncoderCompensationConfig struct {
	Enabled   bool    `json:"enabled"`
	Tolerance float64 `json:"tolerance"`
	MaxCycles int     `json:"maxCycles"`
	SettleMs  int     `json:"settleMs"`
	MinStep   float64 `json:"minStep"`
	TimeoutMs int     `json:"timeoutMs"`
}

// AxisConfig 单轴配置，包含机械、电气及运动限制参数。
type AxisConfig struct {
	Name                AxisName                       `json:"name"`
	Enabled             bool                           `json:"enabled"`
	Kind                AxisKind                       `json:"kind"`
	MaxSpeed            *float64                       `json:"maxSpeed,omitempty"`
	MinLimit            *float64                       `json:"minLimit,omitempty"`
	MaxLimit            *float64                       `json:"maxLimit,omitempty"`
	Inverted            bool                           `json:"inverted"`
	EncoderInverted     bool                           `json:"encoderInverted"`
	StepsPerRev         *float64                       `json:"stepsPerRev,omitempty"`
	MicroSteps          *float64                       `json:"microSteps,omitempty"`
	Lead                *float64                       `json:"lead,omitempty"`
	GearRatio           *float64                       `json:"gearRatio,omitempty"`
	PositionSource      PositionSource                 `json:"positionSource"`
	EncoderScale        *float64                       `json:"encoderScale,omitempty"`
	EncoderCompensation *AxisEncoderCompensationConfig `json:"encoderCompensation,omitempty"`
}

type MotionControllerProfile struct {
	ID          string         `json:"id"`
	Name        string         `json:"name"`
	Type        ControllerType `json:"type"`
	Address     string         `json:"address"`
	Port        int            `json:"port"`
	AutoConnect bool           `json:"autoConnect"`
	Axes        []AxisConfig   `json:"axes"`
}

type AxisStatus struct {
	Name              AxisName `json:"name"`
	Position          float64  `json:"position"`
	Velocity          float64  `json:"velocity"`
	Moving            bool     `json:"moving"`
	Homed             bool     `json:"homed"`
	PosLimit          bool     `json:"posLimit"`
	NegLimit          bool     `json:"negLimit"`
	Compensating      bool     `json:"compensating"`
	CompensationError string   `json:"compensationError,omitempty"`
	PositionError     float64  `json:"positionError"`
}

type ControllerStatus struct {
	ID               string         `json:"id"`
	Name             string         `json:"name"`
	Type             ControllerType `json:"type"`
	Connected        bool           `json:"connected"`
	EmergencyStopped bool           `json:"emergencyStopped"`
	Axes             []AxisStatus   `json:"axes"`
	LastError        string         `json:"lastError,omitempty"`
}
