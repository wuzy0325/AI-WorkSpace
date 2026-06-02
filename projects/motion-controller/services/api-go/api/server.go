package api

import (
	"net/http"

	motionhttp "shared.local/motion-control/go/httpapi"

	"motion-controller/services/api-go/internal/usecase"
)

type Deps struct {
	MotionManager *usecase.MotionManager
}

func NewRouter(deps Deps) http.Handler {
	mux := http.NewServeMux()

	if deps.MotionManager != nil {
		motionhttp.RegisterMotionRoutes(mux, deps.MotionManager)
	}

	return mux
}
