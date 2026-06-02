package appcontext

import (
	"os"
	"path/filepath"

	"motion-controller/services/api-go/internal/usecase"
	"shared.local/device-sdk/go/motion/adapters/hardware"
	"shared.local/device-sdk/go/motion/core"
	"shared.local/device-sdk/go/motion/ports"
	motionprofile "shared.local/motion-control/go/profile"
)

// AppContext holds all core application services
type AppContext struct {
	MotionManager *usecase.MotionManager
	configDir     string
}

// NewAppContext creates and initializes all core services
func NewAppContext(configDir string) (*AppContext, error) {
	if configDir == "" {
		var err error
		configDir, err = os.UserConfigDir()
		if err != nil {
			configDir = "config"
		} else {
			configDir = filepath.Join(configDir, "motion-controller")
		}
	}

	if err := os.MkdirAll(configDir, 0755); err != nil {
		return nil, err
	}

	motionProfilePath := filepath.Join(configDir, "motion-profiles.json")
	motionProfileStore := motionprofile.NewFileMotionProfileStore(motionProfilePath)

	motionMgr := usecase.NewMotionManager(motionProfileStore, func(profile core.MotionControllerProfile) (ports.MotionController, error) {
		factory := hardware.NewDefaultMotionControllerFactory()
		return factory.Create(profile)
	})

	return &AppContext{
		MotionManager: motionMgr,
		configDir:     configDir,
	}, nil
}
