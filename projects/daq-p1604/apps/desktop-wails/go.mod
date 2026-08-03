module daq-p1604

go 1.20

require (
	golang.org/x/sys v0.20.0
	nhooyr.io/websocket v1.8.17
	shared.local/device-sdk/go v0.0.0-00010101000000-000000000000
)

replace shared.local/device-sdk/go => ../../../../shared/device-sdk/go
