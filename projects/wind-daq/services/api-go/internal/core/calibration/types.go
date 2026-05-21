package calibration

type State string

const (
	StateIdle    State = "idle"
	StateRunning State = "running"
	StatePaused  State = "paused"
	StateStopped State = "stopped"
	StateError   State = "error"
)

type Config struct {
	TaskID         string    `json:"taskId"`
	DeviceID       string    `json:"deviceId"`
	Type           string    `json:"type"`
	Channels       []int     `json:"channels"`
	PressurePoints []float64 `json:"pressurePoints"`
	AverageSamples int       `json:"averageSamples"`
}

type PointResult struct {
	PointIndex     int             `json:"pointIndex"`
	TargetPressure float64         `json:"targetPressure"`
	Timestamp      int64           `json:"timestamp"`
	Values         map[int]float64 `json:"values"`
}

type Status struct {
	TaskID       string        `json:"taskId"`
	State        State         `json:"state"`
	CurrentPoint int           `json:"currentPoint"`
	TotalPoints  int           `json:"totalPoints"`
	Results      []PointResult `json:"results"`
	LastError    string        `json:"lastError,omitempty"`
}
