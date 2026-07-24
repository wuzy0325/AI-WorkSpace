module motion-controller/apps/desktop-wails

go 1.20

require (
	motion-controller/services/api-go v0.0.0
	shared.local/device-sdk/go v0.0.0
	shared.local/motion-control/go v0.0.0
)

replace (
	motion-controller/services/api-go => ../../services/api-go
	shared.local/device-sdk/go => ../../../../shared/device-sdk/go
	shared.local/motion-control/go => ../../../../shared/motion-control/go
)
