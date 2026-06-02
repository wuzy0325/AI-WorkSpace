package backend

type PrbFileInfo struct {
	FilePath   string        `json:"filePath"`
	FileName   string        `json:"fileName"`
	MachNumber float64       `json:"machNumber"`
	ValidRange PrbValidRange `json:"validRange"`
}

type PrbValidRange struct {
	AlphaMin float64 `json:"alphaMin"`
	AlphaMax float64 `json:"alphaMax"`
	MachMin  float64 `json:"machMin"`
	MachMax  float64 `json:"machMax"`
}

type InterpolationInput struct {
	P1           float64 `json:"P1"`
	P2           float64 `json:"P2"`
	P3           float64 `json:"P3"`
	Patm         float64 `json:"Patm"`
	Tatm         float64 `json:"Tatm"`
	PressureMode string  `json:"pressureMode"`
}

type InterpolationResult struct {
	Alpha          float64 `json:"alpha"`
	MachNumber     float64 `json:"machNumber"`
	TotalPressure  float64 `json:"P0"`
	StaticPressure float64 `json:"Ps"`
	IterationCount int     `json:"iterationCount"`
	IsValid        bool    `json:"isValid"`
	Warning        string  `json:"warning,omitempty"`
}

type GenericResponse struct {
	Success bool   `json:"success"`
	Error   string `json:"error,omitempty"`
}

type LoadPrbResponse struct {
	Success bool           `json:"success"`
	Error   string         `json:"error,omitempty"`
	Data    *LoadPrbResult `json:"data,omitempty"`
}

type LoadPrbResult struct {
	Files     []PrbFileInfo `json:"files"`
	MachRange []float64     `json:"machRange"`
	Warnings  []string      `json:"warnings"`
}

type MachRangeResponse struct {
	Success bool      `json:"success"`
	Error   string    `json:"error,omitempty"`
	Data    []float64 `json:"data,omitempty"`
}

type CalculateResponse struct {
	Success bool                 `json:"success"`
	Error   string               `json:"error,omitempty"`
	Data    *InterpolationResult `json:"data,omitempty"`
}

type BatchCalculateResponse struct {
	Success bool                   `json:"success"`
	Error   string                 `json:"error,omitempty"`
	Data    []*InterpolationResult `json:"data,omitempty"`
}

type ImportCsvDataResponse struct {
	Success bool                 `json:"success"`
	Error   string               `json:"error,omitempty"`
	Data    []InterpolationInput `json:"data,omitempty"`
}
