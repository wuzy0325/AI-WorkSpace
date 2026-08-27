package http

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"cal1604/internal/application/batch"
	"cal1604/internal/application/calibration"
	"cal1604/internal/application/deviceconnect"
	"cal1604/internal/application/measurement"
	"cal1604/internal/application/multipress"
	"cal1604/internal/application/session"
	"cal1604/internal/config"
	"cal1604/internal/domain"
	apperrors "cal1604/internal/errors"
	"cal1604/internal/events"
	"cal1604/internal/report"
	"cal1604/internal/workflow"
	"fmt"
)

// deviceManager 定义 apiServer 对设备管理器的依赖接口。
// 同时兼容内存版 DeviceManager 和持久化版 PersistentDeviceManager。
type deviceManager interface {
	Upsert(dev domain.Device)
	UpdateStatus(id string, status domain.DeviceStatus) bool
	UpdateUnit(id string, unit string) bool
	Delete(id string)
	Get(id string) (domain.Device, bool)
	List() []domain.Device
	CheckUnitConsistency() (bool, []string)
}

type apiServer struct {
	deviceManager      deviceManager
	coordinator        *workflow.WorkflowCoordinator
	deviceConnector    deviceConnector
	connectConfig      deviceconnect.Config
	calibrationConfig  CalibrationRuntimeConfig
	calibrationService *calibration.Service
	multipressService  *multipress.Service
	sessionService     *session.Service
	measurementService *measurement.Service
	reportService      *report.Service
	batchService       *batch.Service
	configPath         string
	appConfig          *config.AppConfig
}

func (s *apiServer) currentToken() session.BindingToken {
	return s.sessionService.Token()
}

type deviceConnector interface {
	Connect(ctx context.Context, id string) (domain.Device, error)
	Disconnect(ctx context.Context, id string) (domain.Device, error)
	Remove(ctx context.Context, id string) error
}

type upsertDeviceRequest struct {
	ID          string                 `json:"id"`
	Name        string                 `json:"name"`
	Type        string                 `json:"type"`
	Model       string                 `json:"model"`
	Host        string                 `json:"host"`
	Port        int                    `json:"port"`
	Unit        string                 `json:"unit"`
	LocalAddr   string                 `json:"localAddr"`
	Status      string                 `json:"status"`
	IsSimulated bool                   `json:"isSimulated"`
	Channels    []domain.ChannelConfig `json:"channels"`
}

type setDeviceStatusRequest struct {
	ID     string `json:"id"`
	Status string `json:"status"`
}

type unitConsistencyPayload struct {
	Consistent bool     `json:"consistent"`
	Conflicts  []string `json:"conflicts"`
}

func (s *apiServer) listDevicesHandler(w http.ResponseWriter, _ *http.Request) {
	devices := s.deviceManager.List()
	if devices == nil {
		devices = make([]domain.Device, 0)
	}
	writeSuccess(w, http.StatusOK, devices)
}

func (s *apiServer) deviceStatusHandler(w http.ResponseWriter, r *http.Request) {
	req, err := decodeJSON[setDeviceStatusRequest](r)
	if err != nil {
		writeError(w, err)
		return
	}

	id := strings.TrimSpace(req.ID)
	status := domain.DeviceStatus(strings.TrimSpace(req.Status))
	if id == "" || !status.IsValid() {
		writeError(w, apperrors.ErrInvalidArgument)
		return
	}

	if status != domain.DeviceStatusError {
		writeError(w, fmt.Errorf("direct status update only supports 'error' status, use connect/disconnect for other states"))
		return
	}

	if ok := s.deviceManager.UpdateStatus(id, status); !ok {
		writeError(w, apperrors.ErrNotFound)
		return
	}

	dev, exists := s.deviceManager.Get(id)
	if !exists {
		writeError(w, apperrors.ErrNotFound)
		return
	}

	s.publishDeviceStatusChanged(dev)

	writeSuccess(w, http.StatusOK, map[string]string{"id": id, "status": string(status)})
}

func (s *apiServer) deviceConnectHandler(w http.ResponseWriter, r *http.Request) {
	id, ok := decodeDeviceIDRequest(r, w)
	if !ok {
		return
	}

	if s.deviceConnector == nil {
		writeError(w, errors.New("device connector is nil"))
		return
	}

	updated, err := s.deviceConnector.Connect(r.Context(), id)
	if errors.Is(err, apperrors.ErrNotFound) {
		writeError(w, apperrors.ErrNotFound)
		return
	}

	// 连接失败时仍返回设备快照，前端可直接展示 error 状态与失败原因。
	if err != nil {
		writeSuccess(w, http.StatusOK, updated)
		return
	}

	writeSuccess(w, http.StatusOK, updated)
}

func (s *apiServer) deviceDisconnectHandler(w http.ResponseWriter, r *http.Request) {
	id, ok := decodeDeviceIDRequest(r, w)
	if !ok {
		return
	}

	if s.deviceConnector == nil {
		writeError(w, errors.New("device connector is nil"))
		return
	}

	updated, err := s.deviceConnector.Disconnect(r.Context(), id)
	if errors.Is(err, apperrors.ErrNotFound) {
		writeError(w, apperrors.ErrNotFound)
		return
	}

	// 断连失败同样返回设备快照，便于前端统一展示错误信息。
	if err != nil {
		writeSuccess(w, http.StatusOK, updated)
		return
	}

	writeSuccess(w, http.StatusOK, updated)
}

func (s *apiServer) handleUpsertDevice(w http.ResponseWriter, r *http.Request) {
	req, err := decodeJSON[upsertDeviceRequest](r)
	if err != nil {
		writeError(w, err)
		return
	}

	deviceType := domain.DeviceType(strings.TrimSpace(req.Type))
	id := strings.TrimSpace(req.ID)
	host := strings.TrimSpace(req.Host)
	unit := strings.TrimSpace(req.Unit)

	requestedStatus := domain.DeviceStatus(strings.TrimSpace(req.Status))
	if req.Status != "" && !requestedStatus.IsValid() {
		writeError(w, apperrors.ErrInvalidArgument)
		return
	}

	old, existed := s.deviceManager.Get(id)
	status := domain.ResolveStatus(requestedStatus, old, existed)

	dev := domain.Device{
		ID:          id,
		Name:        strings.TrimSpace(req.Name),
		Type:        deviceType,
		Model:       strings.TrimSpace(req.Model),
		Host:        host,
		Port:        req.Port,
		Unit:        unit,
		LocalAddr:   strings.TrimSpace(req.LocalAddr),
		Status:      status,
		IsSimulated: req.IsSimulated,
		Channels:    req.Channels,
	}

	if err := dev.Validate(); err != nil {
		writeError(w, apperrors.ErrInvalidArgument)
		return
	}

	if existed {
		dev.LastErrorReason = old.LastErrorReason
		dev.LastErrorAt = old.LastErrorAt
		// 已连接设备的 unit 从硬件实时读取，编辑配置时不能覆盖
		if old.Status == domain.DeviceStatusConnected && old.Unit != "" {
			dev.Unit = old.Unit
		}
		// 校零偏移保护：前端 DTO 的通道无 tareOffset 字段，保存时若为 0
		// 且旧配置有非零偏移，则保留旧偏移，避免编辑配置把校零数据清空。
		mergeTareOffsets(&dev, old)
	}

	s.deviceManager.Upsert(dev)
	s.publishDeviceStatusChanged(dev)
	writeSuccess(w, http.StatusOK, dev)
}

func (s *apiServer) publishDeviceStatusChanged(dev domain.Device) {
	payload := map[string]any{
		"id":     dev.ID,
		"type":   string(dev.Type),
		"status": string(dev.Status),
	}
	if dev.LastErrorReason != "" {
		payload["errorReason"] = dev.LastErrorReason
	}
	if dev.LastErrorAt != nil {
		payload["lastErrorAt"] = dev.LastErrorAt
	}

	publishEvent(events.EventDeviceStatusChanged, payload)
}

// mergeTareOffsets 合并校零偏移：前端 DTO 通道无 tareOffset 字段，保存时
// 通道偏移为 0 但旧配置有非零偏移，则保留旧偏移，避免编辑配置清空校零数据。
func mergeTareOffsets(dev *domain.Device, old domain.Device) {
	if len(dev.Channels) == 0 || len(old.Channels) == 0 {
		return
	}
	oldByIndex := make(map[int]float64, len(old.Channels))
	for _, ch := range old.Channels {
		oldByIndex[ch.Index] = ch.TareOffset
	}
	for i := range dev.Channels {
		if dev.Channels[i].TareOffset == 0 {
			if off, ok := oldByIndex[dev.Channels[i].Index]; ok {
				dev.Channels[i].TareOffset = off
			}
		}
	}
}

func (s *apiServer) unitConsistencyHandler(w http.ResponseWriter, _ *http.Request) {
	consistent, conflicts := s.deviceManager.CheckUnitConsistency()
	writeSuccess(w, http.StatusOK, unitConsistencyPayload{
		Consistent: consistent,
		Conflicts:  conflicts,
	})
}

// handleDeleteDevice 删除指定设备，先断开连接并清理驱动资源。
func (s *apiServer) handleDeleteDevice(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeError(w, apperrors.ErrInvalidArgument)
		return
	}

	if s.deviceConnector != nil {
		_ = s.deviceConnector.Remove(r.Context(), id)
	}

	s.deviceManager.Delete(id)
	writeSuccess(w, http.StatusOK, map[string]string{"id": id})
}

func decodeDeviceIDRequest(r *http.Request, w http.ResponseWriter) (string, bool) {
	req, err := decodeJSON[setDeviceStatusRequest](r)
	if err != nil {
		writeError(w, err)
		return "", false
	}

	id := strings.TrimSpace(req.ID)
	if id == "" {
		writeError(w, apperrors.ErrInvalidArgument)
		return "", false
	}

	return id, true
}
