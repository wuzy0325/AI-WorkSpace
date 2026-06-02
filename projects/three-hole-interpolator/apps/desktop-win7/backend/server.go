package backend

import (
	"encoding/json"
	"net/http"
	"sync"

	three_interp "ai-workspace/shared/algorithms/go/threehole/interpolation"
)

type Server struct {
	mu          sync.RWMutex
	multiInterp *three_interp.ThreeHoleInterpolator
	prbFiles    []PrbFileInfo
}

func NewServer() *Server {
	return &Server{}
}

func RegisterRoutes(mux *http.ServeMux) *Server {
	s := NewServer()

	mux.HandleFunc("/api/load-prb", s.handleLoadPrb)
	mux.HandleFunc("/api/is-prb-loaded", s.handleIsPrbLoaded)
	mux.HandleFunc("/api/prb-files", s.handleGetPrbFiles)
	mux.HandleFunc("/api/mach-range", s.handleGetMachRange)
	mux.HandleFunc("/api/calculate", s.handleCalculate)
	mux.HandleFunc("/api/batch-calculate", s.handleBatchCalculate)
	mux.HandleFunc("/api/import-csv", s.handleImportCsv)
	mux.HandleFunc("/api/help", s.handleHelp)

	return s
}

func writeJSON(w http.ResponseWriter, statusCode int, data interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(statusCode)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(data)
}
