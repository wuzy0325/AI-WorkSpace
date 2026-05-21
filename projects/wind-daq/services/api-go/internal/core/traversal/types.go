package traversal

type State string

const (
	StateIdle    State = "idle"
	StateRunning State = "running"
	StatePaused  State = "paused"
	StateStopped State = "stopped"
	StateError   State = "error"
)

type Point struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
	Z float64 `json:"z"`
}

type Config struct {
	TaskID   string  `json:"taskId"`
	DeviceID string  `json:"deviceId"`
	Channels []int   `json:"channels"`
	Path     []Point `json:"path"`
}

type PointResult struct {
	PointIndex int             `json:"pointIndex"`
	Point      Point           `json:"point"`
	Timestamp  int64           `json:"timestamp"`
	Values     map[int]float64 `json:"values"`
}

type Status struct {
	TaskID       string        `json:"taskId"`
	State        State         `json:"state"`
	CurrentPoint int           `json:"currentPoint"`
	TotalPoints  int           `json:"totalPoints"`
	Results      []PointResult `json:"results"`
	LastError    string        `json:"lastError,omitempty"`
}
