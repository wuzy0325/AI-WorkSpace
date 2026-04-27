package api

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"

	"wind-daq/services/api-go/api/handler"
	"wind-daq/services/api-go/internal/adapters/config"
	"wind-daq/services/api-go/internal/adapters/report"
	"wind-daq/services/api-go/internal/adapters/ws"
	"wind-daq/services/api-go/internal/usecase"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

var wsUpgrader = websocket.Upgrader{}

type Deps struct {
	DeviceManager *usecase.DeviceManager
	AcqHub        *usecase.AcquisitionHub
	MotionManager *usecase.MotionManager
	CalibService  *usecase.CalibrationService
	TraversalSvc  *usecase.TraversalService
	StorageSvc    *usecase.StorageService
	ReportSvc     *report.Service
	ConfigMgr     *config.Manager
	ScanSvc       *usecase.ScanService
}

type Server struct {
	Engine *gin.Engine
	WSHub  *ws.Hub
	http   *http.Server
	Port   int
	dev    bool
	deps   Deps
}

func NewServer(port int, dev bool, wsHub *ws.Hub, deps Deps) *Server {
	if !dev {
		gin.SetMode(gin.ReleaseMode)
	}

	var allowedOrigins []string
	if dev {
		wsUpgrader.CheckOrigin = func(r *http.Request) bool { return true }
	} else {
		originEnv := os.Getenv("WS_ALLOWED_ORIGINS")
		if originEnv != "" {
			allowedOrigins = []string{originEnv}
		} else {
			allowedOrigins = []string{
				"http://localhost:5173",
				"http://localhost:8080",
				"https://localhost:5173",
				"wails://wind-daq",
			}
		}
		wsUpgrader.CheckOrigin = func(r *http.Request) bool {
			origin := r.Header.Get("Origin")
			if origin == "" {
				return true
			}
			for _, allowed := range allowedOrigins {
				if origin == allowed {
					return true
				}
			}
			return false
		}
	}

	engine := gin.New()
	engine.Use(Recovery(), Logger(), CORS())

	s := &Server{
		Engine: engine,
		WSHub:  wsHub,
		Port:   port,
		dev:    dev,
		deps:   deps,
	}
	return s
}

func (s *Server) SetupRoutesWithDeps(deps Deps) {
	s.deps = deps
	s.setupRoutes()
}

func (s *Server) Start(ctx context.Context) error {
	s.http = &http.Server{
		Addr:    fmt.Sprintf(":%d", s.Port),
		Handler: s.Engine,
	}

	go func() {
		<-ctx.Done()
		s.http.Shutdown(context.Background())
	}()

	slog.Info("HTTP server starting", "port", s.Port)
	if err := s.http.ListenAndServe(); err != http.ErrServerClosed {
		return err
	}
	return nil
}

func (s *Server) handleWebSocket(c *gin.Context) {
	conn, err := wsUpgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		slog.Error("WS upgrade error", "err", err)
		return
	}

	client := ws.NewClient(conn)
	s.WSHub.Register(client)

	go func() {
		client.HandleMessages()
		s.WSHub.Unregister(client)
	}()
}

func (s *Server) setupRoutes() {
	api := s.Engine.Group("/api")

	appH := handler.NewAppHandler()
	api.GET("/app/version", appH.GetVersion)

	deviceH := handler.NewDeviceHandler(s.deps.DeviceManager, s.deps.ScanSvc)
	api.GET("/device/profiles", deviceH.GetProfiles)
	api.GET("/device/instances", deviceH.GetInstances)
	api.GET("/device/status", deviceH.GetStatusAll)
	api.PUT("/device/profiles", deviceH.UpsertProfile)
	api.DELETE("/device/profiles/:id", deviceH.DeleteProfile)
	api.POST("/device/:id/connect", deviceH.Connect)
	api.POST("/device/:id/disconnect", deviceH.Disconnect)
	api.POST("/device/:id/startAcquisition", deviceH.StartAcquisition)
	api.POST("/device/:id/stopAcquisition", deviceH.StopAcquisition)
	api.POST("/device/scan", deviceH.Scan)
	api.PUT("/device/:id/unit", deviceH.SetUnit)
	api.GET("/device/capabilities", deviceH.GetCapabilities)
	api.GET("/device/:id/daqT1603Config", deviceH.GetDaqT1603Config)
	api.PUT("/device/:id/daqT1603Config", deviceH.ApplyDaqT1603Config)

	daqH := handler.NewDAQHandler(s.deps.AcqHub, s.deps.DeviceManager)
	api.POST("/daq/startAcquisition", daqH.StartAcquisition)
	api.POST("/daq/stopAcquisition", daqH.StopAcquisition)
	api.PUT("/daq/publishRate", daqH.SetPublishRate)
	api.GET("/daq/publishRate", daqH.GetPublishRate)

	motionH := handler.NewMotionHandler(s.deps.MotionManager)
	api.GET("/motion/profiles", motionH.GetProfiles)
	api.GET("/motion/status", motionH.GetStatusAll)
	api.PUT("/motion/profiles", motionH.UpsertProfile)
	api.DELETE("/motion/profiles/:id", motionH.DeleteProfile)
	api.POST("/motion/:id/connect", motionH.Connect)
	api.POST("/motion/:id/disconnect", motionH.Disconnect)
	api.POST("/motion/:id/moveTo", motionH.MoveTo)
	api.POST("/motion/:id/moveBy", motionH.MoveBy)
	api.POST("/motion/:id/jog", motionH.Jog)
	api.POST("/motion/:id/home", motionH.Home)
	api.POST("/motion/:id/stop", motionH.Stop)
	api.POST("/motion/:id/emergencyStop", motionH.EmergencyStop)
	api.POST("/motion/:id/definePosition", motionH.DefinePosition)

	calibH := handler.NewCalibrationHandler(s.deps.CalibService)
	api.POST("/calibration/start", calibH.Start)
	api.POST("/calibration/pause", calibH.Pause)
	api.POST("/calibration/resume", calibH.Resume)
	api.POST("/calibration/stop", calibH.Stop)
	api.GET("/calibration/status", calibH.GetStatus)
	api.PUT("/calibration/config", calibH.SaveConfig)
	api.GET("/calibration/config", calibH.GetConfig)

	travH := handler.NewTraversalHandler(s.deps.TraversalSvc)
	api.POST("/traversal/start", travH.Start)
	api.POST("/traversal/pause", travH.Pause)
	api.POST("/traversal/resume", travH.Resume)
	api.POST("/traversal/stop", travH.Stop)
	api.GET("/traversal/progress", travH.GetProgress)

	storageStore := config.NewStorageStore(s.deps.ConfigMgr)
	storageH := handler.NewStorageHandler(storageStore, s.deps.StorageSvc)
	api.GET("/storage/settings", storageH.GetSettings)
	api.PUT("/storage/settings", storageH.UpdateSettings)
	api.GET("/storage/status", storageH.GetStatus)
	api.POST("/storage/startRecording", storageH.StartRecording)
	api.POST("/storage/stopRecording", storageH.StopRecording)
	api.POST("/storage/pickDirectory", storageH.PickDirectory)

	reportH := handler.NewReportHandler(s.deps.ReportSvc)
	api.POST("/report/calibration", reportH.GenerateCalibrationReport)
	api.POST("/report/traversal", reportH.GenerateTraversalReport)
	api.POST("/report/generate", reportH.GenerateReport)

	scanH := handler.NewScanHandler(s.deps.ScanSvc)
	api.POST("/scan/devices", scanH.ScanDevices)

	s.Engine.GET("/ws", s.handleWebSocket)
}
