package interpolation

type InterpolationInput struct {
	P1   float64 `json:"P1"`
	P2   float64 `json:"P2"`
	P3   float64 `json:"P3"`
	PAtm float64 `json:"Patm"`
	TAtm float64 `json:"Tatm"`
}

type InterpolationResult struct {
	Alpha          float64 `json:"alpha"`
	MachNumber     float64 `json:"machNumber"`
	TotalPressure  float64 `json:"P0"`
	StaticPressure float64 `json:"Ps"`
	IterationCount int     `json:"iterationCount"`
	Calculated     bool    `json:"calculated"`
	IsValid        bool    `json:"isValid"`
	Warning        string  `json:"warning,omitempty"`
}

type PrbValidRange struct {
	AlphaMin float64 `json:"alphaMin"`
	AlphaMax float64 `json:"alphaMax"`
	MachMin  float64 `json:"machMin"`
	MachMax  float64 `json:"machMax"`
}

type PrbFileInfo struct {
	FilePath   string        `json:"filePath"`
	FileName   string        `json:"fileName"`
	MachNumber float64       `json:"machNumber"`
	ValidRange PrbValidRange `json:"validRange"`
}

type PrbFileData struct {
	FilePath string
	Lines    []string
}

type LoadPrbResult struct {
	Files       []PrbFileInfo `json:"files"`
	MachNumbers []float64     `json:"machNumbers"`
	Warnings    []string      `json:"warnings"`
}

type Interpolator interface {
	IsLoaded() bool
	GetValidRange() PrbValidRange
	Calculate(input InterpolationInput) (InterpolationResult, error)
}
