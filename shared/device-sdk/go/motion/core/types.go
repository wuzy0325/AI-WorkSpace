package core

type AxisName string

type AxisNameSet map[AxisName]bool

const (
	AxisX AxisName = "X"
	AxisY AxisName = "Y"
	AxisZ AxisName = "Z"
	AxisU AxisName = "U"
)

// AxisEncoderCompensationConfig encoder compensation configuration
type AxisEncoderCompensationConfig struct {
	Enabled   bool    `json:"enabled"`
	Tolerance float64 `json:"tolerance"`
	MaxCycles int     `json:"maxCycles"`
	SettleMs  int     `json:"settleMs"`
	MinStep   float64 `json:"minStep"`
	TimeoutMs int     `json:"timeoutMs"`
}

// AxisConfig axis configuration
type AxisConfig struct {
	Name                AxisName                       `json:"name"`
	Enabled             bool                           `json:"enabled"`
	Kind                AxisKind                       `json:"kind"`
	// StepsPerRev step angle in degrees (e.g. 1.8 for a 200-step/rev motor). Despite the name,
	// this field stores degrees-per-step, NOT steps-per-revolution.
	StepsPerRev         *float64                       `json:"stepsPerRev,omitempty"`
	MicroSteps          *int                           `json:"microSteps,omitempty"`
	Lead                *float64                       `json:"lead,omitempty"`
	GearRatio           *float64                       `json:"gearRatio,omitempty"`
	MaxSpeed            *float64                       `json:"maxSpeed,omitempty"`
	MinLimit            *float64                       `json:"minLimit,omitempty"`
	MaxLimit            *float64                       `json:"maxLimit,omitempty"`
	Inverted            bool                           `json:"inverted"`
	EncoderInverted     bool                           `json:"encoderInverted"`
	PositionSource      PositionSource                 `json:"positionSource"`
	EncoderScale        *float64                       `json:"encoderScale,omitempty"`
	EncoderCompensation *AxisEncoderCompensationConfig `json:"encoderCompensation,omitempty"`
}

// AxisKind axis type
type AxisKind string

const (
	AxisKindLinear AxisKind = "LINEAR"
	AxisKindRotary AxisKind = "ROTARY"
)

// PositionSource position source
type PositionSource string

const (
	PositionSourceRegister PositionSource = "register"
	PositionSourceEncoder  PositionSource = "encoder"
)

// ControllerType controller type
type ControllerType string

const (
	ControllerTypeSimulated ControllerType = "SIMULATED-MC"
	ControllerTypeB140      ControllerType = "B140-MC"
	ControllerTypeWTNMC4A   ControllerType = "WTNMC4A-MC"
)

// AxisStatus axis status
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

// ControllerStatus controller status
type ControllerStatus struct {
	ID               string         `json:"id"`
	Name             string         `json:"name"`
	Type             ControllerType `json:"type"`
	Connected        bool           `json:"connected"`
	EmergencyStopped bool           `json:"emergencyStopped"`
	Axes             []AxisStatus   `json:"axes"`
	LastError        string         `json:"lastError,omitempty"`
}

// MotionControllerProfile motion controller profile
type MotionControllerProfile struct {
	ID          string         `json:"id"`
	Name        string         `json:"name"`
	Type        ControllerType `json:"type"`
	Address     string         `json:"address"`
	Port        int            `json:"port"`
	AutoConnect bool           `json:"autoConnect"`
	Axes        []AxisConfig   `json:"axes"`
}
