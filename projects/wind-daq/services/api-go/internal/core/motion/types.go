package motion

type AxisName string

type AxisStatus struct {
	Name     AxisName `json:"name"`
	Position float64  `json:"position"`
	Velocity float64  `json:"velocity"`
	Moving   bool     `json:"moving"`
	Homed    bool     `json:"homed"`
}

type ControllerStatus struct {
	ID               string       `json:"id"`
	Connected        bool         `json:"connected"`
	EmergencyStopped bool         `json:"emergencyStopped"`
	Axes             []AxisStatus `json:"axes"`
}
