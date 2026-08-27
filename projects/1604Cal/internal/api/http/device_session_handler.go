package http

import (
	"net/http"
	"strings"

	apperrors "cal1604/internal/errors"
)

type setMeasureDeviceRequest struct {
	MeasureDeviceID  string   `json:"measureDeviceId"`
	MeasureDeviceIDs []string `json:"measureDeviceIds"`
	ModuleName       string   `json:"moduleName"`
}

type setDevicesRequest struct {
	MeasureDeviceID  string   `json:"measureDeviceId"`
	MeasureDeviceIDs []string `json:"measureDeviceIds"`
	PressureDeviceID string   `json:"pressureDeviceId"`
	ModuleName       string   `json:"moduleName"`
}

type pressureResponse struct {
	Pressure float64 `json:"pressure"`
}

type stabilityResponse struct {
	Stable bool `json:"stable"`
}

type setValveRequest struct {
	Status string `json:"status"`
}

type valveResponse struct {
	Status string `json:"status"`
}

type measureUnitResponse struct {
	Unit string `json:"unit"`
}

type setMeasureUnitRequest struct {
	Unit string `json:"unit"`
}

type calibrateZeroRequest struct {
	DeviceID string `json:"deviceId"`
	Channels []int  `json:"channels"`
}

type resetDeviceRequest struct {
	DeviceID string `json:"deviceId"`
}

type calibrateFullScaleRequest struct {
	Channels       []int   `json:"channels"`
	FullScaleValue float64 `json:"fullScaleValue"`
}

func (s *apiServer) sessionSetDevicesHandler(w http.ResponseWriter, r *http.Request) {
	req, err := decodeJSON[setDevicesRequest](r)
	if err != nil {
		writeError(w, err)
		return
	}

	measureDevIDs := req.MeasureDeviceIDs
	if len(measureDevIDs) == 0 && req.MeasureDeviceID != "" {
		measureDevIDs = []string{req.MeasureDeviceID}
	}
	if len(measureDevIDs) == 0 {
		writeError(w, apperrors.ErrInvalidArgument)
		return
	}

	moduleName := req.ModuleName
	if moduleName == "" {
		moduleName = "measurement"
	}

	_, err = s.sessionService.BindMeasureDevices(measureDevIDs, req.PressureDeviceID, moduleName)
	if err != nil {
		writeError(w, err)
		return
	}
	// 绑定成功后记录设备集合，供下次启动恢复勾选。
	s.persistLastDevices(measureDevIDs, req.PressureDeviceID)

	writeSuccess(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *apiServer) sessionSetMeasureDeviceHandler(w http.ResponseWriter, r *http.Request) {
	req, err := decodeJSON[setMeasureDeviceRequest](r)
	if err != nil {
		writeError(w, err)
		return
	}

	measureDevIDs := req.MeasureDeviceIDs
	if len(measureDevIDs) == 0 && req.MeasureDeviceID != "" {
		measureDevIDs = []string{req.MeasureDeviceID}
	}
	if len(measureDevIDs) == 0 {
		writeError(w, apperrors.ErrInvalidArgument)
		return
	}

	moduleName := req.ModuleName
	if moduleName == "" {
		moduleName = "measurement"
	}

	pressureDevID := s.sessionService.PressureDeviceID()
	_, err = s.sessionService.BindMeasureDevices(measureDevIDs, pressureDevID, moduleName)
	if err != nil {
		writeError(w, err)
		return
	}
	// 绑定成功后记录设备集合，供下次启动恢复勾选（保留当前打压设备绑定）。
	s.persistLastDevices(measureDevIDs, pressureDevID)

	writeSuccess(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *apiServer) sessionUnbindMeasureDevicesHandler(w http.ResponseWriter, _ *http.Request) {
	s.sessionService.UnbindMeasureDevices()
	writeSuccess(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *apiServer) sessionReadPressureHandler(w http.ResponseWriter, r *http.Request) {
	pressure, err := s.sessionService.ReadPressure(r.Context(), s.currentToken())
	if err != nil {
		writeError(w, err)
		return
	}

	writeSuccess(w, http.StatusOK, pressureResponse{Pressure: pressure})
}

func (s *apiServer) sessionReadStabilityHandler(w http.ResponseWriter, r *http.Request) {
	stable, err := s.sessionService.ReadStability(r.Context(), s.currentToken())
	if err != nil {
		writeError(w, err)
		return
	}

	writeSuccess(w, http.StatusOK, stabilityResponse{Stable: stable})
}

func (s *apiServer) sessionReadMeasureDataHandler(w http.ResponseWriter, r *http.Request) {
	// devices 为设备维度完整结果（deviceID -> 通道数据）；
	// data 保留首个绑定设备数据，兼容旧前端单设备字段。
	devices, err := s.sessionService.ReadMeasureDataAllDevices(r.Context(), s.currentToken())
	if err != nil {
		writeError(w, err)
		return
	}

	var data []float64
	if ids := s.sessionService.MeasureDeviceIDs(); len(ids) > 0 {
		data = devices[ids[0]]
	}

	writeSuccess(w, http.StatusOK, map[string]any{"data": data, "devices": devices})
}

func (s *apiServer) sessionGetValveHandler(w http.ResponseWriter, r *http.Request) {
	status, err := s.sessionService.ReadValveStatus(r.Context(), s.currentToken())
	if err != nil {
		writeError(w, err)
		return
	}
	writeSuccess(w, http.StatusOK, valveResponse{Status: status})
}

func (s *apiServer) sessionGetValveAllHandler(w http.ResponseWriter, r *http.Request) {
	devices, err := s.sessionService.ReadValveStatusAllDevices(r.Context(), s.currentToken())
	if err != nil {
		writeError(w, err)
		return
	}
	writeSuccess(w, http.StatusOK, map[string]any{"devices": devices})
}

func (s *apiServer) sessionSetValveHandler(w http.ResponseWriter, r *http.Request) {
	req, err := decodeJSON[setValveRequest](r)
	if err != nil {
		writeError(w, err)
		return
	}

	normalizedStatus, ok := normalizeValveStatus(req.Status)
	if !ok {
		writeError(w, apperrors.ErrInvalidArgument)
		return
	}

	// 多设备批次阀门状态必须整批一致（启动门禁校验所有设备），
	// 因此阀门写命令下发到全部已绑定计量设备；单设备行为不变。
	if err := s.sessionService.SetValveStatusAllDevices(r.Context(), s.currentToken(), normalizedStatus); err != nil {
		writeError(w, err)
		return
	}
	// 设备应答 "A" 即视为命令已接收。
	// 真实硬件状态由前端在收到响应后短延时主动 GET /valve 校核，
	// 避免在 handler 内同步执行第二次 3s I/O，把用户感知延迟翻倍。
	writeSuccess(w, http.StatusOK, valveResponse{Status: normalizedStatus})
}

func (s *apiServer) sessionCalibrateZeroHandler(w http.ResponseWriter, r *http.Request) {
	req, err := decodeJSON[calibrateZeroRequest](r)
	if err != nil {
		writeError(w, err)
		return
	}
	if len(req.Channels) == 0 {
		writeError(w, apperrors.ErrInvalidArgument)
		return
	}

	values, err := s.sessionService.CalibrateZeroForDevice(r.Context(), s.currentToken(), req.DeviceID, req.Channels)
	if err != nil {
		writeError(w, err)
		return
	}

	writeSuccess(w, http.StatusOK, map[string]any{"data": values})
}

func (s *apiServer) sessionCalibrateFullScaleHandler(w http.ResponseWriter, r *http.Request) {
	req, err := decodeJSON[calibrateFullScaleRequest](r)
	if err != nil {
		writeError(w, err)
		return
	}
	if len(req.Channels) == 0 || req.FullScaleValue <= 0 {
		writeError(w, apperrors.ErrInvalidArgument)
		return
	}

	values, err := s.sessionService.CalibrateFullScale(r.Context(), s.currentToken(), req.Channels, req.FullScaleValue)
	if err != nil {
		writeError(w, err)
		return
	}

	writeSuccess(w, http.StatusOK, map[string]any{"values": values})
}

func normalizeValveStatus(status string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "calibration":
		return "calibration", true
	case "measurement":
		return "measurement", true
	default:
		return "", false
	}
}

func (s *apiServer) sessionGetMeasureUnitHandler(w http.ResponseWriter, r *http.Request) {
	unit, err := s.sessionService.ReadMeasureUnit(r.Context(), s.currentToken())
	if err != nil {
		writeError(w, err)
		return
	}
	writeSuccess(w, http.StatusOK, measureUnitResponse{Unit: unit})
}

func (s *apiServer) sessionGetMeasureUnitAllHandler(w http.ResponseWriter, r *http.Request) {
	devices, err := s.sessionService.ReadMeasureUnitAllDevices(r.Context(), s.currentToken())
	if err != nil {
		writeError(w, err)
		return
	}
	writeSuccess(w, http.StatusOK, map[string]any{"devices": devices})
}

func (s *apiServer) sessionSetMeasureUnitHandler(w http.ResponseWriter, r *http.Request) {
	req, err := decodeJSON[setMeasureUnitRequest](r)
	if err != nil {
		writeError(w, err)
		return
	}
	if req.Unit == "" {
		writeError(w, apperrors.ErrInvalidArgument)
		return
	}

	if err := s.sessionService.SetMeasureUnit(r.Context(), s.currentToken(), req.Unit); err != nil {
		writeError(w, err)
		return
	}
	writeSuccess(w, http.StatusOK, measureUnitResponse(req))
}

func (s *apiServer) sessionSetMeasureUnitAllHandler(w http.ResponseWriter, r *http.Request) {
	req, err := decodeJSON[setMeasureUnitRequest](r)
	if err != nil {
		writeError(w, err)
		return
	}
	if strings.TrimSpace(req.Unit) == "" {
		writeError(w, apperrors.ErrInvalidArgument)
		return
	}
	if err := s.sessionService.SetMeasureUnitAllDevices(r.Context(), s.currentToken(), req.Unit); err != nil {
		writeError(w, err)
		return
	}
	writeSuccess(w, http.StatusOK, measureUnitResponse(req))
}

func (s *apiServer) sessionUnitConsistencyHandler(w http.ResponseWriter, _ *http.Request) {
	consistent, conflicts := s.sessionService.CheckUnitConsistency()
	writeSuccess(w, http.StatusOK, map[string]any{
		"consistent": consistent,
		"conflicts":  conflicts,
	})
}

func (s *apiServer) sessionReadDeviceInfoHandler(w http.ResponseWriter, r *http.Request) {
	info, err := s.sessionService.ReadDeviceInfo(r.Context(), s.currentToken())
	if err != nil {
		writeError(w, err)
		return
	}

	writeSuccess(w, http.StatusOK, map[string]any{"info": info})
}

func (s *apiServer) sessionResetDeviceHandler(w http.ResponseWriter, r *http.Request) {
	// deviceId 可省略：空请求体保持"复位首个计量设备"的兼容语义。
	req, err := decodeJSONOptional[resetDeviceRequest](r)
	if err != nil {
		writeError(w, err)
		return
	}
	if err := s.sessionService.ResetDeviceForDevice(r.Context(), s.currentToken(), req.DeviceID); err != nil {
		writeError(w, err)
		return
	}

	writeSuccess(w, http.StatusOK, map[string]string{"status": "ok"})
}
