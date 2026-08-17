module windlabx4/apps/desktop-wails

go 1.25.0

require (
	github.com/wailsapp/wails/v3 v3.0.0-alpha2.106
	golang.org/x/sys v0.47.0
	shared.local/device-sdk/go v0.0.0
	windlabx4/services/api-go v0.0.0
)

replace (
	ai-workspace/shared/algorithms/go/fivehole => ../../../../shared/algorithms/go/fivehole
	ai-workspace/shared/algorithms/go/sevenhole => ../../../../shared/algorithms/go/sevenhole
	shared.local/device-sdk/go => ../../../../shared/device-sdk/go
	shared.local/motion-control/go => ../../../../shared/motion-control/go
	windlabx4/services/api-go => ../../services/api-go
)

require (
	ai-workspace/shared/algorithms/go/fivehole v0.0.0 // indirect
	ai-workspace/shared/algorithms/go/sevenhole v0.0.0 // indirect
	github.com/adrg/xdg v0.5.3 // indirect
	github.com/coder/websocket v1.8.14 // indirect
	github.com/go-ole/go-ole v1.3.0 // indirect
	github.com/godbus/dbus/v5 v5.2.2 // indirect
	github.com/jchv/go-winloader v0.0.0-20250406163304-c1995be93bd1 // indirect
	github.com/mattn/go-colorable v0.1.14 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/wailsapp/wails/webview2 v1.0.27 // indirect
	golang.org/x/text v0.40.0 // indirect
	shared.local/motion-control/go v0.0.0 // indirect
)
