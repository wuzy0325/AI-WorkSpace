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

type ControllerType string

const (
	ControllerTypeSimulated ControllerType = "SIMULATED-MC"
	ControllerTypeB140      ControllerType = "B140-MC"
	ControllerTypeWTNMC4A   ControllerType = "WTNMC4A-MC"
)

type AxisConfig struct {
	Name      AxisName  `json:"name"`
	Enabled   bool      `json:"enabled"`
	Kind      AxisKind  `json:"kind"`
	MaxSpeed  *float64  `json:"maxSpeed,omitempty"`
	MinLimit  *float64  `json:"minLimit,omitempty"`
	MaxLimit  *float64  `json:"maxLimit,omitempty"`
	Inverted  bool      `json:"inverted"`
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
	Name     AxisName `json:"name"`
	Position float64  `json:"position"`
	Velocity float64  `json:"velocity"`
	Moving   bool     `json:"moving"`
	Homed    bool     `json:"homed"`
	PosLimit bool     `json:"posLimit"`
	NegLimit bool     `json:"negLimit"`
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
